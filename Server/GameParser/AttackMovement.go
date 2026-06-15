package GameParser

import (
	"strconv"
	"sync"
	"time"

	"CitadelDesktop/Server/GameCommands"
	"CitadelDesktop/Server/Logging"
	"CitadelDesktop/Server/Models"
	gamestate "CitadelDesktop/Server/Models/GameState"
	riftattack "CitadelDesktop/Server/Models/RiftAttack"
	"CitadelDesktop/Server/ResponseRegistry"
)

// gamAutoPollIntervalSec is how often we re-request **gam** while any owned attack leg is present.
// The game does not reliably push the return leg; polling fetches ground truth so the commander frees.
const gamAutoPollIntervalSec = 10

// completionGraceSec is the wall-clock backstop past a leg's TT before it counts as expired.
const completionGraceSec = 60

// unknownMaxAgeSec caps how long a leg with no TT may live before it is force-expired.
const unknownMaxAgeSec = 1800

// lastGamAutoPollUnix is read/written only by the single poll-ticker goroutine.
var lastGamAutoPollUnix int64

// movementReconcileMu serializes writers that mutate ActiveMovements + CommanderByMID
// (GAM/MRM parse and the poll ticker). Readers elsewhere remain lock-free as before.
var movementReconcileMu sync.Mutex

var movementTickerOnce sync.Once

func init() {
	gamestate.EvaluateCommanderWireBusy = func(gs *gamestate.GameState, commanderWireID int) bool {
		return CommanderMarching(gs, commanderWireID, 0, 0, 0, 0, false)
	}
}

func coordsMatch(ax, ay, bx, by int) bool {
	return ax == bx && ay == by
}

// sourceIsRift reports a leg whose source tile is the world Rift (i.e. a homeward Rift attack return).
func sourceIsRift(m Models.GAMMovement) bool {
	rift, _, ok := Models.GetMapState().FindRift()
	if !ok {
		return false
	}
	return coordsMatch(m.SourceX, m.SourceY, rift.X, rift.Y)
}

// legBelongsToUs reports whether a movement's source or target tile is one of our castles.
// Strict: when our castle list is not yet known, nothing belongs to us (returns false).
func legBelongsToUs(gs *Models.GameState, m Models.GAMMovement) bool {
	if gs == nil {
		return false
	}
	for _, loc := range gs.Alliance.PlayerCastleLocations {
		if coordsMatch(m.SourceX, m.SourceY, loc.X, loc.Y) || coordsMatch(m.TargetX, m.TargetY, loc.X, loc.Y) {
			return true
		}
	}
	return false
}

// legExpired is the single wall-clock predicate shared by the busy gate and list cleanup, so the two
// can never disagree. Primary completion is still a real frame removing the leg; this is only the
// bounded safety net so nothing locks a commander or lingers in the list forever.
func legExpired(m Models.GAMMovement, now int64) bool {
	if m.TT > 0 {
		isReturn := m.D == 1 || sourceIsRift(m)
		if isReturn && m.EffectivePT(now) >= m.TT+completionGraceSec {
			return true
		}
		// Outbound completion cap. Commander/rift legs use out-and-back (TT*2) so a stuck commander frees
		// promptly. A support/bird leg instead sits "posted" at its destination for TWD seconds after
		// arriving (PT runs TT -> TT+TWD), so it must live until TT+TWD; otherwise it would expire ~TT*2
		// into a multi-hour post and vanish from the list until the return leg appears.
		outboundCap := m.TT * 2
		if m.CommanderID < 0 && !sourceIsRift(m) && m.TWD > 0 {
			outboundCap = m.TT + m.TWD
		}
		if m.EffectivePT(now) >= outboundCap+completionGraceSec {
			return true
		}
		return false
	}
	// Unknown duration: cap absolute wall age.
	return m.ReceivedUnix > 0 && now-m.ReceivedUnix >= unknownMaxAgeSec
}

func movementLegsForCommander(gs *Models.GameState, commanderWireID int) []Models.GAMMovement {
	if gs == nil || commanderWireID < 0 {
		return nil
	}
	var legs []Models.GAMMovement
	for _, m := range gs.Movement.ActiveMovements {
		if m.CommanderID == commanderWireID {
			legs = append(legs, m)
		}
	}
	return legs
}

// hasOutstandingAttackMovement reports whether any leg is a commander attack or a Rift return, so the
// poll loop keeps requesting **gam** until the march is fully reflected as gone. Caller holds the mutex.
func hasOutstandingAttackMovement(gs *Models.GameState) bool {
	if gs == nil {
		return false
	}
	for _, m := range gs.Movement.ActiveMovements {
		if m.CommanderID >= 0 || sourceIsRift(m) {
			return true
		}
	}
	return false
}

func intFromBody(body map[string]interface{}, key string) int {
	if body == nil {
		return 0
	}
	v, ok := body[key].(float64)
	if !ok {
		return 0
	}
	return int(v)
}

// savedLaunchCommanderForRiftReturn finds a saved Rift launch whose home (SX/SY) matches the return
// leg's target (home), returning its commander LID. Best-effort fallback for an orphan return that
// never carried UM.L.ID and has no MID memory.
func savedLaunchCommanderForRiftReturn(m Models.GAMMovement) (int, bool) {
	for _, launch := range riftattack.Load().Launches {
		lid := riftattack.CommanderIDFromLaunch(launch)
		if lid < 0 {
			continue
		}
		sx, sy := intFromBody(launch.Body, "SX"), intFromBody(launch.Body, "SY")
		if coordsMatch(m.TargetX, m.TargetY, sx, sy) {
			return lid, true
		}
	}
	return 0, false
}

// rebindCommanderIDs keeps CommanderByMID in sync with ActiveMovements and re-attaches orphan legs
// (a return reusing the outbound MID, or a known Rift return). Caller holds movementReconcileMu.
func rebindCommanderIDs(gs *Models.GameState) {
	if gs == nil {
		return
	}
	if gs.Movement.CommanderByMID == nil {
		gs.Movement.CommanderByMID = make(map[int]int)
	}
	// 1. Learn MID -> LID from any leg the game identified with a commander.
	for _, m := range gs.Movement.ActiveMovements {
		if m.CommanderID >= 0 && m.MID != 0 {
			gs.Movement.CommanderByMID[m.MID] = m.CommanderID
		}
	}
	// 2. Re-attach orphan legs from memory, else best-effort by saved Rift launch.
	for i := range gs.Movement.ActiveMovements {
		m := &gs.Movement.ActiveMovements[i]
		if m.CommanderID >= 0 {
			continue
		}
		if lid, ok := gs.Movement.CommanderByMID[m.MID]; ok && m.MID != 0 {
			m.CommanderID = lid
			continue
		}
		if sourceIsRift(*m) {
			if lid, ok := savedLaunchCommanderForRiftReturn(*m); ok {
				m.CommanderID = lid
				if m.MID != 0 {
					gs.Movement.CommanderByMID[m.MID] = lid
				}
			}
		}
	}
	// 3. GC entries whose MID is no longer present.
	present := make(map[int]struct{}, len(gs.Movement.ActiveMovements))
	for _, m := range gs.Movement.ActiveMovements {
		present[m.MID] = struct{}{}
	}
	for mid := range gs.Movement.CommanderByMID {
		if _, ok := present[mid]; !ok {
			delete(gs.Movement.CommanderByMID, mid)
		}
	}
}

// expireStaleMovements drops every wall-clock-expired leg (returns true if any removed), also GCing
// its CommanderByMID entry. Bounded safety net so legs auto-clear without a frame. Caller holds the mutex.
func expireStaleMovements(gs *Models.GameState, now int64) bool {
	if gs == nil || len(gs.Movement.ActiveMovements) == 0 {
		return false
	}
	kept := make([]Models.GAMMovement, 0, len(gs.Movement.ActiveMovements))
	changed := false
	for _, m := range gs.Movement.ActiveMovements {
		if legExpired(m, now) {
			changed = true
			if gs.Movement.CommanderByMID != nil && m.MID != 0 {
				delete(gs.Movement.CommanderByMID, m.MID)
			}
			Logging.RiftLogf("movement_expire", "MID=%d LID=%d aged out (D=%d PT=%d TT=%d)", m.MID, m.CommanderID, m.D, m.PT, m.TT)
			continue
		}
		kept = append(kept, m)
	}
	if changed {
		gs.Movement.ActiveMovements = kept
	}
	return changed
}

// CommanderMarching reports whether wire LID has a live (non-expired) attack leg in ActiveMovements.
// The home/target coords and useLaunchCoords are kept for signature compatibility with the Rift CRA
// launch flow; commander identity is already attached by rebindCommanderIDs at parse time.
func CommanderMarching(gs *Models.GameState, wireID, homeX, homeY, targetX, targetY int, useLaunchCoords bool) bool {
	if gs == nil || wireID < 0 {
		return false
	}
	now := time.Now().Unix()
	for _, m := range gs.Movement.ActiveMovements {
		if m.CommanderID != wireID {
			continue
		}
		if !legExpired(m, now) {
			return true
		}
	}
	return false
}

// CommanderMarchingForLaunch keeps the launch-coords entry point used by the Rift CRA flow.
func CommanderMarchingForLaunch(gs *Models.GameState, wireID, homeX, homeY, targetX, targetY int) bool {
	return CommanderMarching(gs, wireID, homeX, homeY, targetX, targetY, true)
}

// startMovementPollTicker runs a single background loop while the app is up: it re-polls **gam** at the
// configured interval and applies the bounded expiry so completed legs auto-clear even if the game
// stops pushing frames. It never fabricates completion ahead of the data.
func startMovementPollTicker() {
	movementTickerOnce.Do(func() {
		go func() {
			ticker := time.NewTicker(2 * time.Second)
			defer ticker.Stop()
			for range ticker.C {
				pollAndExpireTick()
			}
		}()
	})
}

func pollAndExpireTick() {
	maybeAutoPollGAM()

	gs := Models.GetGameState()
	movementReconcileMu.Lock()
	changed := expireStaleMovements(gs, time.Now().Unix())
	if changed {
		rebindCommanderIDs(gs)
	}
	movementReconcileMu.Unlock()

	MaybeNotifyRiftCRALaunchBusyChanged()
	if changed && NotifyMovementChanged != nil {
		NotifyMovementChanged()
	}
}

// maybeAutoPollGAM requests a fresh **gam** (at most once per gamAutoPollIntervalSec) while any owned
// attack leg is still present, so the return/completion arrives as real data without a manual refresh.
func maybeAutoPollGAM() {
	if !ResponseRegistry.LoginStatus {
		return
	}
	gs := Models.GetGameState()

	movementReconcileMu.Lock()
	hasAttack := hasOutstandingAttackMovement(gs)
	movementReconcileMu.Unlock()
	if !hasAttack {
		return
	}

	now := time.Now().Unix()
	if now-lastGamAutoPollUnix < gamAutoPollIntervalSec {
		return
	}
	lastGamAutoPollUnix = now
	GameCommands.SendGAM()
}

func empireExResponseCode(messageParts []string) (int, bool) {
	if len(messageParts) <= 4 {
		return 0, false
	}
	code, err := strconv.Atoi(messageParts[4])
	if err != nil {
		return 0, false
	}
	return code, true
}

func ParseCRAResponse(messageParts []string, payload string) {
	ParseGAMMessage(payload)
	code, ok := empireExResponseCode(messageParts)
	if !ok || code != 0 {
		return
	}
	RecordRiftCRASuccess(payload)
}

func IsCommanderWireIDBusy(gs *Models.GameState, commanderWireID int) bool {
	return CommanderMarching(gs, commanderWireID, 0, 0, 0, 0, false)
}

func BusyCommanderWireIDs(gs *Models.GameState) []int {
	if gs == nil {
		return nil
	}
	seen := make(map[int]struct{})
	for _, m := range gs.Movement.ActiveMovements {
		if m.CommanderID < 0 {
			continue
		}
		if CommanderMarching(gs, m.CommanderID, 0, 0, 0, 0, false) {
			seen[m.CommanderID] = struct{}{}
		}
	}
	if len(seen) == 0 {
		return nil
	}
	out := make([]int, 0, len(seen))
	for id := range seen {
		out = append(out, id)
	}
	return out
}
