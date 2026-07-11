package featureview

import (
	"CitadelDesktop/Server/GameCommands"
	"CitadelDesktop/Server/GameFocus"
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
	// autoBirdCDSLID is the fixed CDS "LID" for bird sends (SDI is still sent for route preview; LID is not parsed from it).
	autoBirdCDSLID = -14
	// Max time to wait for GGE login after ReloadGameTab (cooldowns can run up to ~5 min).
	sessionWaitAfterReload = 5*time.Minute + 10*time.Second
	// Minimum alliance member RPT (bird post time, seconds) for a castle to count as a bird target.
	autoBirdDefaultMinRPTDays = 3
)

// isBirdTargetCastleType returns true for alliance **ain** castle slot types we treat as valid bird posts.
// See GameParser.MapCastleTypes.go (CastleSlotMain, CastleSlotOutpost, CastleSlotForeign).
func isBirdTargetCastleType(ct int) bool {
	return GameParser.IsBirdTargetCastleSlot(ct)
}

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

// EnsureGameSessionOrReload returns true if the game reports an active session; otherwise reloads the tab and waits for login.
func EnsureGameSessionOrReload(ctx context.Context) bool {
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

		st := Models.GetSettingsState()
		if sleepUntil, blocked := featureScheduleBlockedUntil(st, "autoBird", time.Now(), 30*time.Second, 10*time.Second); blocked {
			setNextWakeUnixMs(sleepUntil)
			autoBirdLog("schedule", fmt.Sprintf("inactive; next check at %s", sleepUntil.Format(time.RFC3339)))
			select {
			case <-ctx.Done():
				return
			case <-time.After(time.Until(sleepUntil)):
			}
			clearNextWake()
			continue
		}

		// On each wake (including first iteration): require an active game session or reload the tab to establish one.
		if !EnsureGameSessionOrReload(ctx) {
			time.Sleep(30 * time.Second)
			continue
		}

		gs := Models.GetGameState()
		pid := gs.PlayerID

		if pid == 0 || gs.Alliance.AID <= 0 {
			autoBirdLog("wait_data", "Game state not fully loaded (PID or AID is 0). Waiting...")
			time.Sleep(5 * time.Second)
			continue
		}

		// --- Movements: use GAM data already parsed from server traffic (no outgoing **gam** request) ---

		// --- Reconcile logged birds ---
		file := sentbird.Load()
		if pid != 0 {
			file.PlayerID = pid
		}
		remaining, nextReconcile := reconcileBirds(gs, file.Birds, st)
		sentbird.ReplaceBirds(file.PlayerID, remaining)
		autoBirdLog("reconcile", fmt.Sprintf("%d bird(s) remain logged after reconciliation", len(remaining)))

		// --- Alliance bird posts (configurable min RPT from **ain**) → local list for distance calcs only ---
		autoBirdLog("ain_send", fmt.Sprintf("Sending AIN for AID=%d", gs.Alliance.AID))
		GameCommands.SendAIN(gs.Alliance.AID)
		time.Sleep(ainSettleDelay)
		autoBirdLog("ain_receive", fmt.Sprintf("BirdLocations count=%d", len(gs.Alliance.BirdLocations)))
		birdPosts := buildBirdPosts(gs, st)

		if len(birdPosts) == 0 {
			autoBirdLog("ain_receive", fmt.Sprintf("No bird targets found (either none with RPT>%dd, or AIN response was delayed). Retrying in 30s", autoBirdMinRPTDays(st)))
			time.Sleep(30 * time.Second)
			continue
		}

		minH, maxH := st.AutoBirdDelay.MinDelay, st.AutoBirdDelay.MaxDelay
		if minH < 1 {
			minH = 1
		}
		if maxH < minH {
			maxH = minH
		}
		rndH := minH
		if maxH > minH {
			rndH = minH + rng.Intn(maxH-minH+1)
		}
		rndDelay := time.Duration(rndH) * time.Hour

		// Track the farthest successful bird movement so the round sleeps for its full out-and-back
		// travel time plus the configured random wait.
		var maxTravelSeconds int
		// refreshedAIN gates a single mid-round AIN re-request: if a castle finds no same-kingdom post we
		// refetch alliance data once (in case this round's snapshot was stale) and retry before skipping.
		refreshedAIN := false

		// --- Send birds from each GameState.Castle slot that has AID + map location ---
		for _, slot := range []struct {
			c          *Models.PlayerCastleInfo
			defaultKID int
		}{
			{&gs.Castle.MainCastle, 0},
			{&gs.Castle.Outpost1, 0},
			{&gs.Castle.Outpost2, 0},
			{&gs.Castle.Outpost3, 0},
			{&gs.Castle.IceCastle, 2},
			{&gs.Castle.DesertCastle, 1},
			{&gs.Castle.DungeonCastle, 3},
			{&gs.Castle.StormCastle, 4},
			{&gs.Castle.Metropolis, 0},
			{&gs.Castle.Capital, 0},
		} {
			c := slot.c
			defaultKID := slot.defaultKID
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
			resolvedKID, sx, sy, ok := resolveAutoTCICastleMapCoords(gs, castleID, c)
			if !ok {
				autoBirdLog("castle_coords", fmt.Sprintf("skip castle %d: no map x/y on GameState yet", castleID))
				continue
			}
			// Birds cannot cross kingdoms: cds/sdi resolve the target on the focused (source) kingdom's
			// map, so only ever pick an alliance post in the SAME kingdom as the source castle. Match on
			// resolvedKID (the castle's real kingdom from map coords), not the slot's nominal default.
			post := pickBirdPostForCastle(birdPosts, resolvedKID, sx, sy)
			if post == nil && !refreshedAIN {
				// Fallback: this round's AIN snapshot has no same-kingdom post for this castle. Re-request
				// AIN once per round (in case the snapshot was stale/partial), rebuild the post list, and
				// retry. If the alliance genuinely has no post in this kingdom, the retry is still nil and
				// we skip — we never fall back to another kingdom (that would hit a non-alliance tile).
				refreshedAIN = true
				autoBirdLog("ain_refresh", fmt.Sprintf("no same-kingdom post for castle %d (kid=%d); re-requesting AIN", castleID, resolvedKID))
				GameCommands.SendAIN(gs.Alliance.AID)
				time.Sleep(ainSettleDelay)
				birdPosts = buildBirdPosts(gs, st)
				post = pickBirdPostForCastle(birdPosts, resolvedKID, sx, sy)
			}
			if post == nil {
				autoBirdLog("no_post", fmt.Sprintf("skip castle %d: no same-kingdom bird post (resolvedKid=%d slotKid=%d posts=%d)", castleID, resolvedKID, defaultKID, len(birdPosts)))
				continue
			}
			lease, ok := GameFocus.Acquire(ctx, GameFocus.Request{
				Owner:   GameFocus.OwnerAutoBird,
				Reason:  fmt.Sprintf("castle=%d", castleID),
				MaxHold: 30 * time.Second,
			})
			if !ok {
				return
			}
			leaseRevoked := false
			func() {
				defer lease.Release()

				if !GameParser.FocusPlayerCastleTroopsWithLease(lease, resolvedKID, castleID, sx, sy) {
					if !lease.Active() {
						leaseRevoked = true
						autoBirdLog("lease_revoked", fmt.Sprintf("castle %d", castleID))
						return
					}
					autoBirdLog("focus_fail", fmt.Sprintf("castle %d (kid=%d)", castleID, resolvedKID))
					return
				}
				sendMap := troopsToSend(gs, castleID, c)
				total := sumMap(sendMap)
				if total < st.AutoBirdDelay.MinSend {
					return
				}
				if len(sendMap) == 0 {
					return
				}

				troopsJSON := troopsMapToCDSJSON(sendMap)
				if !lease.Active() {
					leaseRevoked = true
					autoBirdLog("lease_revoked", fmt.Sprintf("castle %d sdi skipped", castleID))
					return
				}
				GameCommands.SendSDI(post.X, post.Y, sx, sy)
				time.Sleep(sdiCdsGap)
				if !GameCommands.SendCDSUntilSuccessWithLease(lease, castleID, post.X, post.Y, autoBirdCDSLID, rndH, gs.GlobalResources.PTT, troopsJSON, func(parts []string) {
					if len(parts) <= 5 || parts[4] != "0" {
						return
					}
					tt := GameParser.ExtractMaxTTSecondsFromGAMLikeJSON(parts[5])
					if tt <= 0 {
						return
					}
					maxTravelSeconds = autoBirdFarthestTravelSeconds(maxTravelSeconds, tt)
				}) {
					if !lease.Active() {
						leaseRevoked = true
						autoBirdLog("lease_revoked", fmt.Sprintf("castle %d cds skipped", castleID))
						return
					}
					autoBirdLog("cds_fail", fmt.Sprintf("castle %d -> (%d,%d) lid=%d gamePTT=%.0f", castleID, post.X, post.Y, autoBirdCDSLID, gs.GlobalResources.PTT))
					return
				}

				sentbird.Append(file.PlayerID, sentbird.LoggedBird{
					SourceCastleID: castleID,
					SourceKID:      resolvedKID,
					TargetX:        post.X,
					TargetY:        post.Y,
					Troops:         copyIntMap(sendMap),
					SentAtUnix:     time.Now().Unix(),
				})
				autoBirdLog("bird_sent", fmt.Sprintf("castle %d -> (%d,%d) units=%v", castleID, post.X, post.Y, sendMap))
				time.Sleep(50 * time.Millisecond)
			}()
			if leaseRevoked {
				break
			}
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

		// New sends take precedence: wait for the farthest movement's round trip plus the configured
		// random delay. If no movement was sent, use the existing reconciliation/fallback schedule.
		var sleepUntil time.Time
		switch {
		case maxTravelSeconds > 0:
			sleepUntil = time.Now().Add(autoBirdSleepDuration(maxTravelSeconds, rndDelay))
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
		autoBirdLog("sleep_until", fmt.Sprintf("%s (reconcileEarliest=%v newSend=%v farthestTT1way=%ds rnd=%dh)",
			sleepUntil.Format(time.RFC3339), !nextReconcile.IsZero(), maxTravelSeconds > 0, maxTravelSeconds, rndH))

		select {
		case <-ctx.Done():
			return
		case <-time.After(time.Until(sleepUntil)):
		}
		clearNextWake()
	}
}

func autoBirdSleepDuration(maxTravelSeconds int, waitDelay time.Duration) time.Duration {
	return 2*time.Duration(maxTravelSeconds)*time.Second + waitDelay
}

func autoBirdFarthestTravelSeconds(current, candidate int) int {
	if candidate > current {
		return candidate
	}
	return current
}

// birdPost is one alliance bird post (from **ain** / BirdLocations) for distance checks only.
type birdPost struct {
	KingdomID int
	X, Y      int
	BirdTime  int
}

func autoBirdMinRPTDays(st *Models.SettingsState) int {
	if st == nil {
		return autoBirdDefaultMinRPTDays
	}
	days := st.AutoBirdDelay.MinRPTDays
	if days < 0 {
		return 0
	}
	return days
}

func autoBirdMinRPTSeconds(st *Models.SettingsState) int {
	return autoBirdMinRPTDays(st) * 86400
}

func buildBirdPosts(gs *Models.GameState, st *Models.SettingsState) []birdPost {
	bl := gs.Alliance.BirdLocations
	minRPT := autoBirdMinRPTSeconds(st)
	minDays := autoBirdMinRPTDays(st)
	out := make([]birdPost, 0, len(bl))
	for i := range bl {
		b := &bl[i]
		if minRPT > 0 && b.BirdTime <= minRPT {
			autoBirdLog("filter", fmt.Sprintf("skipping bird post at (%d,%d) kid=%d: RPT=%ds below %dd minimum", b.X, b.Y, b.KingdomID, b.BirdTime, minDays))
			continue
		}
		if !isBirdTargetCastleType(b.CastleType) {
			autoBirdLog("filter", fmt.Sprintf("skipping bird post at (%d,%d) kid=%d: unsupported CastleType %d", b.X, b.Y, b.KingdomID, b.CastleType))
			continue
		}
		out = append(out, birdPost{
			KingdomID: b.KingdomID,
			X:         b.X,
			Y:         b.Y,
			BirdTime:  b.BirdTime,
		})
	}
	return out
}

// pickBirdPostForCastle returns the nearest alliance bird post in the SAME kingdom as the source
// castle, or nil if none exists. Birds cannot cross kingdoms: the cds target is resolved on the
// focused (source) kingdom's map, so a post from another kingdom would land on whatever castle sits
// at those coordinates in the source kingdom — almost always a non-alliance player. The castle is
// simply skipped when no same-kingdom post is available (no cross-kingdom fallback).
func pickBirdPostForCastle(targets []birdPost, srcKID, srcX, srcY int) *birdPost {
	return closestBirdPost(targets, srcKID, srcX, srcY)
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
func reconcileBirds(gs *Models.GameState, birds []sentbird.LoggedBird, st *Models.SettingsState) ([]sentbird.LoggedBird, time.Time) {
	var keep []sentbird.LoggedBird
	var earliest time.Time
	now := time.Now()
	minRPT := autoBirdMinRPTSeconds(st)
	activeMovements, _, _, _ := gs.Movement.Snapshot()

	for _, b := range birds {
		c := gs.GetCastleByID(b.SourceCastleID)
		srcKID, srcX, srcY, ok := 0, 0, 0, false
		if c != nil {
			srcKID, srcX, srcY, ok = resolveAutoTCICastleMapCoords(gs, b.SourceCastleID, c)
		}
		if !ok {
			srcKID = b.SourceKID
		}

		activeMov := false
		var matched *Models.GAMMovement
		for i := range activeMovements {
			m := &activeMovements[i]
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
					if minRPT > 0 && bl.BirdTime <= minRPT {
						continue
					}
					if !isBirdTargetCastleType(bl.CastleType) {
						continue
					}
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
