package GameFunctions

import (
	"CitadelDesktop/Server/GameCommands"
	"CitadelDesktop/Server/GameParser"
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
	gamWaitTimeout = 12 * time.Second
	ainSettleDelay = 1500 * time.Millisecond
	sdiCdsGap      = 250 * time.Millisecond
)

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
	log.Println("[AutoBird] loop started")
	defer log.Println("[AutoBird] loop stopped")

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		if !ResponseRegistry.LoginStatus {
			time.Sleep(2 * time.Second)
			continue
		}

		gs := Models.GetGameState()
		pid := gs.PlayerID

		// --- Refresh movements ---
		if !GameParser.SendGAMAndWait(gamWaitTimeout) {
			log.Printf("[AutoBird] GAM refresh timed out")
			time.Sleep(30 * time.Second)
			continue
		}

		// --- Reconcile logged birds ---
		file := sentbird.Load()
		if pid != 0 {
			file.PlayerID = pid
		}
		remaining, nextReconcile := reconcileBirds(gs, file.Birds)
		sentbird.ReplaceBirds(file.PlayerID, remaining)
		log.Printf("[AutoBird] reconciliation: %d bird(s) remain logged", len(remaining))

		// --- Alliance / bird targets ---
		GameCommands.SendAIN(gs.Alliance.AID)
		time.Sleep(ainSettleDelay)

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

		// --- Send birds from each castle ---
		for _, loc := range gs.Alliance.PlayerCastleLocations {
			select {
			case <-ctx.Done():
				return
			default:
			}
			if !ResponseRegistry.LoginStatus {
				break
			}
			castleID := loc.CastleID
			c := gs.GetCastleByID(castleID)
			if c == nil || int(c.Aid) != castleID {
				continue
			}
			bird := closestBirdTarget(gs, loc)
			if bird == nil {
				continue
			}
			if !GameParser.FocusPlayerCastleTroops(loc.KingdomID, castleID, loc.X, loc.Y) {
				log.Printf("[AutoBird] focus failed castle %d", castleID)
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
			GameCommands.SendSDI(bird.X, bird.Y, loc.X, loc.Y)
			// CDS requires an LID derived from the SDI response (gaa.AI[17]).
			// Wait briefly for the SDI parser to capture it.
			sdiSentAt := time.Now().UnixNano()
			lid := 0
			deadline := time.Now().Add(2 * time.Second)
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
			if !GameCommands.SendCDSUntilSuccess(castleID, bird.X, bird.Y, lid, delayH, gs.GlobalResources.PTT, troopsJSON) {
				log.Printf("[AutoBird] CDS failed (both HBW/PTT pairs) castle %d -> (%d,%d) gamePTT=%.0f", castleID, bird.X, bird.Y, gs.GlobalResources.PTT)
				continue
			}

			sentbird.Append(file.PlayerID, sentbird.LoggedBird{
				SourceCastleID: castleID,
				SourceKID:      loc.KingdomID,
				TargetX:        bird.X,
				TargetY:        bird.Y,
				Troops:         copyIntMap(sendMap),
				SentAtUnix:     time.Now().Unix(),
			})
			log.Printf("[AutoBird] sent bird from castle %d -> (%d,%d) units=%v", castleID, bird.X, bird.Y, sendMap)
			time.Sleep(400 * time.Millisecond)
		}

		// Refresh movements after sends (best-effort).
		_ = GameParser.SendGAMAndWait(gamWaitTimeout)
		maxTT := maxMovementTT(gs)
		roundTrip := time.Duration(maxTT*2) * time.Second
		if roundTrip < time.Minute {
			roundTrip = time.Minute
		}
		wakeFromSend := time.Now().Add(roundTrip + rndDelay)

		var sleepUntil time.Time
		if !nextReconcile.IsZero() && nextReconcile.Before(wakeFromSend) {
			sleepUntil = nextReconcile
		} else {
			sleepUntil = wakeFromSend
		}
		if sleepUntil.Before(time.Now().Add(10 * time.Second)) {
			sleepUntil = time.Now().Add(10 * time.Second)
		}
		setNextWakeUnixMs(sleepUntil)
		log.Printf("[AutoBird] sleeping until %v", sleepUntil.Format(time.RFC3339))

		select {
		case <-ctx.Done():
			return
		case <-time.After(time.Until(sleepUntil)):
		}
		clearNextWake()
	}
}

func maxMovementTT(gs *Models.GameState) int {
	max := 0
	for _, m := range gs.Movement.ActiveMovements {
		if m.TT > max {
			max = m.TT
		}
	}
	return max
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

func movementMatchesBird(m Models.GAMMovement, b sentbird.LoggedBird, loc *Models.PlayerCastleLocation) bool {
	if m.TargetX != b.TargetX || m.TargetY != b.TargetY {
		return false
	}
	if !troopMapsEqual(troopMapFromGAM(m.TroopArray), b.Troops) {
		return false
	}
	if m.SID == b.SourceCastleID {
		return true
	}
	if loc != nil && m.SourceX == loc.X && m.SourceY == loc.Y && m.KID == loc.KingdomID {
		return true
	}
	return false
}

func findCastleLoc(gs *Models.GameState, castleID int) *Models.PlayerCastleLocation {
	for i := range gs.Alliance.PlayerCastleLocations {
		L := &gs.Alliance.PlayerCastleLocations[i]
		if L.CastleID == castleID {
			return L
		}
	}
	return nil
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

func movementStillActive(m Models.GAMMovement, b sentbird.LoggedBird, loc *Models.PlayerCastleLocation) bool {
	if !movementMatchesBird(m, b, loc) {
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
		loc := findCastleLoc(gs, b.SourceCastleID)
		c := gs.GetCastleByID(b.SourceCastleID)

		activeMov := false
		var matched *Models.GAMMovement
		for i := range gs.Movement.ActiveMovements {
			m := &gs.Movement.ActiveMovements[i]
			if movementStillActive(*m, b, loc) {
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

func closestBirdTarget(gs *Models.GameState, loc Models.PlayerCastleLocation) *Models.BirdLocation {
	var best *Models.BirdLocation
	bestD := 0
	for i := range gs.Alliance.BirdLocations {
		bl := &gs.Alliance.BirdLocations[i]
		if bl.KingdomID != loc.KingdomID {
			continue
		}
		d := manhattan(loc.X, loc.Y, bl.X, bl.Y)
		if best == nil || d < bestD {
			best = bl
			bestD = d
		}
	}
	return best
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
