package GameFunctions

import (
	"CitadelDesktop/Server/GameCommands"
	"CitadelDesktop/Server/GameParser"
	"CitadelDesktop/Server/Logging"
	"CitadelDesktop/Server/Models"
	sentbird "CitadelDesktop/Server/Models/SentBird"
	"CitadelDesktop/Server/ResponseRegistry"
	"context"
	"fmt"
	"log"
	"math/rand"
	"strings"
	"sync"
	"time"
)

var (
	autoBirdCancel     context.CancelFunc
	autoBirdMu         sync.Mutex
	autoBirdNextWakeUp int64 // unix ms, 0 if unknown
	rng                = rand.New(rand.NewSource(time.Now().UnixNano()))
)

const (
	ainSettleDelay = 1500 * time.Millisecond
	sdiCdsGap      = 250 * time.Millisecond
	// Max time to wait for GGE login after ReloadGameTab (cooldowns can run up to ~5 min).
	sessionWaitAfterReload = 5*time.Minute + 10*time.Second
)

func autoBirdLog(event, detail string) {
	if detail != "" {
		log.Printf("[AutoBird] %s: %s", event, detail)
	} else {
		log.Printf("[AutoBird] %s", event)
	}
	Logging.AppendAutoBirdLine(event, detail)
}

// waitForGameLogin polls ResponseRegistry.LoginStatus until true, deadline, or ctx done.
func waitForGameLogin(ctx context.Context, maxWait time.Duration) bool {
	deadline := time.Now().Add(maxWait)
	for {
		if ResponseRegistry.LoginStatus {
			return true
		}
		if !time.Now().Before(deadline) {
			return ResponseRegistry.LoginStatus
		}
		select {
		case <-ctx.Done():
			return false
		case <-time.After(1 * time.Second):
		}
	}
}

// ensureGameSessionOrReload returns true if the game reports an active session; otherwise reloads the tab and waits for login.
func ensureGameSessionOrReload(ctx context.Context) bool {
	if ResponseRegistry.LoginStatus {
		return true
	}
	autoBirdLog("session_check", "no active game session; reloading game tab")
	ResponseRegistry.ReloadGameTab()
	if waitForGameLogin(ctx, sessionWaitAfterReload) {
		autoBirdLog("session_check", "game session active after reload")
		return true
	}
	autoBirdLog("session_check", "still disconnected after reload wait")
	return false
}

// IsAutoBirdRunning reports whether the AutoBird loop is active.
func IsAutoBirdRunning() bool {
	autoBirdMu.Lock()
	defer autoBirdMu.Unlock()
	return autoBirdCancel != nil
}

// GetAutoBirdNextWakeUp returns next wake time in unix milliseconds (0 if not sleeping / unknown).
func GetAutoBirdNextWakeUp() int64 {
	autoBirdMu.Lock()
	defer autoBirdMu.Unlock()
	return autoBirdNextWakeUp
}

func setNextWakeUnixMs(t time.Time) {
	autoBirdMu.Lock()
	autoBirdNextWakeUp = t.UnixMilli()
	autoBirdMu.Unlock()
	if ResponseRegistry.SendAutoBirdStatusFunc != nil {
		go ResponseRegistry.SendAutoBirdStatusFunc(true, autoBirdNextWakeUp)
	}
}

func clearNextWake() {
	autoBirdMu.Lock()
	autoBirdNextWakeUp = 0
	autoBirdMu.Unlock()
	if ResponseRegistry.SendAutoBirdStatusFunc != nil {
		go ResponseRegistry.SendAutoBirdStatusFunc(true, 0)
	}
}

// StartAutoBird starts the AutoBird goroutine.
func StartAutoBird() {
	autoBirdMu.Lock()
	defer autoBirdMu.Unlock()
	if autoBirdCancel != nil {
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	autoBirdCancel = cancel
	go runAutoBird(ctx)
}

// StopAutoBird stops AutoBird.
func StopAutoBird() {
	autoBirdMu.Lock()
	defer autoBirdMu.Unlock()
	if autoBirdCancel != nil {
		autoBirdCancel()
		autoBirdCancel = nil
		autoBirdNextWakeUp = 0
		if ResponseRegistry.SendAutoBirdStatusFunc != nil {
			go ResponseRegistry.SendAutoBirdStatusFunc(false, 0)
		}
	}
}

func runAutoBird(ctx context.Context) {
	autoBirdLog("loop_start", "goroutine running")
	defer autoBirdLog("loop_stop", "goroutine exiting")

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		// On each wake (including first iteration): require an active game session or reload the tab to establish one.
		if !ensureGameSessionOrReload(ctx) {
			time.Sleep(30 * time.Second)
			continue
		}

		gs := Models.GetGameState()
		pid := gs.PlayerID

		// --- Movements: use GAM data already parsed from server traffic (no outgoing **gam** request) ---

		// --- Reconcile logged birds ---
		file := sentbird.Load()
		if pid != 0 {
			file.PlayerID = pid
		}
		remaining, nextReconcile := reconcileBirds(gs, file.Birds)
		sentbird.ReplaceBirds(file.PlayerID, remaining)
		autoBirdLog("reconcile", fmt.Sprintf("%d bird(s) remain logged after reconciliation", len(remaining)))

		// --- Alliance bird posts (RPT>0 members from **ain**) → local list for distance calcs only ---
		GameCommands.SendAIN(gs.Alliance.AID)
		time.Sleep(ainSettleDelay)
		birdPosts := buildBirdPosts(gs)

		st := Models.GetSettingsState()
		minH, maxH := st.AutoBirdDelay.MinDelay, st.AutoBirdDelay.MaxDelay
		if maxH < minH {
			maxH = minH
		}
		rndH := minH
		if maxH > minH {
			rndH = minH + rng.Intn(maxH-minH+1)
		}
		rndDelay := time.Duration(rndH) * time.Hour

		// Per successful **cds**, we compute wake = now + 2×TT + rndDelay for that send.
		// Multiple castles in one round: **last** successful send overwrites (end of last bird's window), not max TT at end of loop.
		var wakeNewSends time.Time
		var maxTTLogged int

		// --- Send birds from each GameState.Castle slot that has AID + map location ---
		for _, c := range []*Models.PlayerCastleInfo{
			&gs.Castle.MainCastle,
			&gs.Castle.Outpost1,
			&gs.Castle.Outpost2,
			&gs.Castle.Outpost3,
			&gs.Castle.IceCastle,
			&gs.Castle.DesertCastle,
			&gs.Castle.DungeonCastle,
			&gs.Castle.StormCastle,
			&gs.Castle.Metropolis,
			&gs.Castle.Capital,
		} {
			select {
			case <-ctx.Done():
				return
			default:
			}
			if !ResponseRegistry.LoginStatus {
				break
			}
			if c.Aid <= 0 {
				continue
			}
			castleID := int(c.Aid)
			kid, sx, sy, ok := castleMapCoords(gs, castleID, c)
			if !ok {
				autoBirdLog("castle_coords", fmt.Sprintf("skip castle %d: no map x/y on GameState yet", castleID))
				continue
			}
			post := closestBirdPost(birdPosts, kid, sx, sy)
			if post == nil {
				continue
			}
			if !GameParser.FocusPlayerCastleTroops(kid, castleID, sx, sy) {
				autoBirdLog("focus_fail", fmt.Sprintf("castle %d (kid=%d)", castleID, kid))
				continue
			}
			sendMap := troopsToSend(gs, castleID, c)
			total := sumMap(sendMap)
			if total < st.AutoBirdDelay.MinSend {
				continue
			}
			if len(sendMap) == 0 {
				continue
			}

			delayH := rndH
			if delayH < 1 {
				delayH = 1
			}
			troopsJSON := troopsMapToCDSJSON(sendMap)
			GameCommands.SendSDI(post.X, post.Y, sx, sy)
			// CDS requires an LID derived from the SDI response (gaa.AI[17]).
			// Wait briefly for the SDI parser to capture it.
			sdiSentAt := time.Now().UnixNano()
			lid := 0
			deadline := time.Now().Add(25 * time.Millisecond)
			for time.Now().Before(deadline) {
				if s, ok := gs.GetLastSDI(castleID); ok && s.ReceivedUnix >= sdiSentAt {
					lid = s.LID
					break
				}
				time.Sleep(25 * time.Millisecond)
			}
			if lid == 0 {
				// Empirically, some kingdoms return 0 here; keep it as-is.
				time.Sleep(sdiCdsGap)
			}
			if !GameCommands.SendCDSUntilSuccess(castleID, post.X, post.Y, lid, delayH, gs.GlobalResources.PTT, troopsJSON, func(parts []string) {
				if len(parts) <= 5 || parts[4] != "0" {
					return
				}
				tt := GameParser.ExtractMaxTTSecondsFromGAMLikeJSON(parts[5])
				if tt <= 0 {
					return
				}
				if tt > maxTTLogged {
					maxTTLogged = tt
				}
				roundTrip := time.Duration(tt*2) * time.Second
				if roundTrip < time.Minute {
					roundTrip = time.Minute
				}
				wakeNewSends = time.Now().Add(roundTrip + rndDelay)
			}) {
				autoBirdLog("cds_fail", fmt.Sprintf("castle %d -> (%d,%d) lid=%d gamePTT=%.0f", castleID, post.X, post.Y, lid, gs.GlobalResources.PTT))
				continue
			}

			sentbird.Append(file.PlayerID, sentbird.LoggedBird{
				SourceCastleID: castleID,
				SourceKID:      kid,
				TargetX:        post.X,
				TargetY:        post.Y,
				Troops:         copyIntMap(sendMap),
				SentAtUnix:     time.Now().Unix(),
			})
			autoBirdLog("bird_sent", fmt.Sprintf("castle %d -> (%d,%d) units=%v", castleID, post.X, post.Y, sendMap))
			time.Sleep(50 * time.Millisecond)
		}

		if !ResponseRegistry.LoginStatus {
			autoBirdLog("session_lost", "lost during castle round; reloading game tab")
			ResponseRegistry.ReloadGameTab()
			if waitForGameLogin(ctx, sessionWaitAfterReload) {
				autoBirdLog("session_lost", "game session restored; restarting round")
				clearNextWake()
				continue
			}
			autoBirdLog("session_lost", "still disconnected after reload wait; retry in 30s")
			clearNextWake()
			time.Sleep(61 * time.Second)
			continue
		}

		// Sleep policy:
		// - **Reconciled** birds (still in transit from the log): wake at the **earliest** re-check time (nextReconcile).
		// - **New** sends this round: wake at the **last** send's deadline (each cds overwrites; final send wins).
		// When both apply, wake at whichever comes first so we do not oversleep past the next reconcile check.
		var sleepUntil time.Time
		switch {
		case !wakeNewSends.IsZero():
			sleepUntil = wakeNewSends
			if !nextReconcile.IsZero() && nextReconcile.Before(sleepUntil) {
				sleepUntil = nextReconcile
			}
		case !nextReconcile.IsZero():
			sleepUntil = nextReconcile
		default:
			// No CDS TT this round and nothing to reconcile: sleep until the random delay only.
			// Do not use max TT from ActiveMovements — it can be an unrelated alliance march.
			sleepUntil = time.Now().Add(rndDelay)
		}
		if sleepUntil.Before(time.Now().Add(10 * time.Second)) {
			sleepUntil = time.Now().Add(10 * time.Second)
		}
		setNextWakeUnixMs(sleepUntil)
		autoBirdLog("sleep_until", fmt.Sprintf("%s (reconcileEarliest=%v newSendLatest=%v cdsTT1way~=%ds rnd=%dh)",
			sleepUntil.Format(time.RFC3339), !nextReconcile.IsZero(), !wakeNewSends.IsZero(), maxTTLogged, rndH))

		select {
		case <-ctx.Done():
			return
		case <-time.After(time.Until(sleepUntil)):
		}
		clearNextWake()
	}
}

// birdPost is one alliance bird post (from **ain** / BirdLocations: RPT>0) for distance checks only.
type birdPost struct {
	KingdomID int
	X, Y      int
	BirdTime  int
}

func buildBirdPosts(gs *Models.GameState) []birdPost {
	bl := gs.Alliance.BirdLocations
	out := make([]birdPost, 0, len(bl))
	for i := range bl {
		b := &bl[i]
		out = append(out, birdPost{
			KingdomID: b.KingdomID,
			X:         b.X,
			Y:         b.Y,
			BirdTime:  b.BirdTime,
		})
	}
	return out
}

// castleMapCoords returns kingdom + map tile: gcl fields on the castle slot, then JAA Troops, then ResolveCastleMapCoords / locations.
func castleMapCoords(gs *Models.GameState, castleID int, c *Models.PlayerCastleInfo) (kingdomID, x, y int, ok bool) {
	if c == nil {
		return 0, 0, 0, false
	}
	if c.MapX != 0 || c.MapY != 0 {
		return c.MapKingdomID, c.MapX, c.MapY, true
	}
	t := c.Troops
	if t.X != 0 || t.Y != 0 {
		return t.KingdomID, t.X, t.Y, true
	}
	px, py, found := gs.ResolveCastleMapCoords(castleID, t.KingdomID)
	if !found {
		return 0, 0, 0, false
	}
	kid := t.KingdomID
	for _, loc := range gs.Alliance.PlayerCastleLocations {
		if loc.CastleID != castleID {
			continue
		}
		if loc.X == px && loc.Y == py {
			return loc.KingdomID, px, py, true
		}
		if kid == 0 {
			kid = loc.KingdomID
		}
	}
	if kid != 0 {
		return kid, px, py, true
	}
	for _, loc := range gs.Alliance.PlayerCastleLocations {
		if loc.CastleID == castleID {
			return loc.KingdomID, px, py, true
		}
	}
	return 0, px, py, true
}

func closestBirdPost(targets []birdPost, kingdomID, srcX, srcY int) *birdPost {
	var best *birdPost
	bestD := 0
	for i := range targets {
		t := &targets[i]
		if t.KingdomID != kingdomID {
			continue
		}
		d := manhattan(srcX, srcY, t.X, t.Y)
		if best == nil || d < bestD {
			best = t
			bestD = d
		}
	}
	return best
}

func troopMapFromGAM(a [][]int) map[int]int {
	m := make(map[int]int)
	for _, p := range a {
		if len(p) < 2 {
			continue
		}
		if p[0] > 0 && p[1] > 0 {
			m[p[0]] += p[1]
		}
	}
	return m
}

func troopMapsEqual(a, b map[int]int) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if b[k] != v {
			return false
		}
	}
	return true
}

func movementMatchesBird(m Models.GAMMovement, b sentbird.LoggedBird, srcKID, srcX, srcY int) bool {
	if m.TargetX != b.TargetX || m.TargetY != b.TargetY {
		return false
	}
	if !troopMapsEqual(troopMapFromGAM(m.TroopArray), b.Troops) {
		return false
	}
	if m.SID == b.SourceCastleID {
		return true
	}
	if srcX != 0 || srcY != 0 {
		if m.SourceX == srcX && m.SourceY == srcY && m.KID == srcKID {
			return true
		}
	}
	return false
}

func tuCoversBird(c *Models.PlayerCastleInfo, b sentbird.LoggedBird) bool {
	if c == nil {
		return false
	}
	tu := c.Troops.TroopsTU
	if tu == nil {
		return false
	}
	for u, need := range b.Troops {
		if need <= 0 {
			continue
		}
		if tu[u] < need {
			return false
		}
	}
	return true
}

func movementStillActive(m Models.GAMMovement, b sentbird.LoggedBird, srcKID, srcX, srcY int) bool {
	if !movementMatchesBird(m, b, srcKID, srcX, srcY) {
		return false
	}
	if m.TT <= 0 {
		return true
	}
	if m.D == 0 {
		// Outbound leg finished at target; list may still show row until return is tracked.
		if m.PT >= m.TT {
			return false
		}
		return true
	}
	// Return leg (D==1): done when PT >= TT
	if m.PT >= m.TT {
		return false
	}
	return true
}

// reconcileBirds returns birds still in transit and the earliest time we should re-check (zero if none).
func reconcileBirds(gs *Models.GameState, birds []sentbird.LoggedBird) ([]sentbird.LoggedBird, time.Time) {
	var keep []sentbird.LoggedBird
	var earliest time.Time
	now := time.Now()

	for _, b := range birds {
		c := gs.GetCastleByID(b.SourceCastleID)
		srcKID, srcX, srcY, ok := 0, 0, 0, false
		if c != nil {
			srcKID, srcX, srcY, ok = castleMapCoords(gs, b.SourceCastleID, c)
		}
		if !ok {
			srcKID = b.SourceKID
		}

		activeMov := false
		var matched *Models.GAMMovement
		for i := range gs.Movement.ActiveMovements {
			m := &gs.Movement.ActiveMovements[i]
			if movementStillActive(*m, b, srcKID, srcX, srcY) {
				activeMov = true
				matched = m
				break
			}
		}
		tuFallback := !activeMov && tuCoversBird(c, b)

		if activeMov || tuFallback {
			keep = append(keep, b)
			if matched != nil {
				m := matched
				var remSec int
				if m.TT <= 0 {
					remSec = 3600
				} else if m.D == 0 {
					remSec = (m.TT - m.PT)
					if remSec < 0 {
						remSec = 0
					}
					remSec += m.TT
				} else {
					remSec = m.TT - m.PT
					if remSec < 0 {
						remSec = 0
					}
				}
				t := now.Add(time.Duration(remSec) * time.Second)
				if earliest.IsZero() || t.Before(earliest) {
					earliest = t
				}
			} else if tuFallback {
				maxBird := 0
				for i := range gs.Alliance.BirdLocations {
					bl := gs.Alliance.BirdLocations[i]
					if bl.KingdomID == b.SourceKID && bl.BirdTime > maxBird {
						maxBird = bl.BirdTime
					}
				}
				if maxBird <= 0 {
					maxBird = 3600
				}
				t := now.Add(time.Duration(maxBird*2) * time.Second)
				if earliest.IsZero() || t.Before(earliest) {
					earliest = t
				}
			}
		}
	}
	return keep, earliest
}

func manhattan(ax, ay, bx, by int) int {
	dx := ax - bx
	if dx < 0 {
		dx = -dx
	}
	dy := ay - by
	if dy < 0 {
		dy = -dy
	}
	return dx + dy
}

func troopsToSend(gs *Models.GameState, castleID int, c *Models.PlayerCastleInfo) map[int]int {
	st := Models.GetSettingsState()
	out := make(map[int]int)
	for uid, cnt := range c.Troops.TroopsI {
		if cnt <= 0 || !Models.IsTroop(uid) {
			continue
		}
		keep, _ := st.BirdIgnoreList.GetSaveAmount(castleID, uid)
		avail := cnt - keep
		if avail > 0 {
			out[uid] = avail
		}
	}
	return out
}

func sumMap(m map[int]int) int {
	s := 0
	for _, v := range m {
		s += v
	}
	return s
}

func copyIntMap(m map[int]int) map[int]int {
	o := make(map[int]int, len(m))
	for k, v := range m {
		o[k] = v
	}
	return o
}

func troopsMapToCDSJSON(m map[int]int) string {
	var b strings.Builder
	b.WriteByte('[')
	first := true
	for id, n := range m {
		if n <= 0 {
			continue
		}
		if !first {
			b.WriteByte(',')
		}
		first = false
		fmt.Fprintf(&b, "[%d,%d]", id, n)
	}
	b.WriteByte(']')
	return b.String()
}
