package featureview

import (
	"context"
	"fmt"
	"log"
	"sort"
	"sync"
	"time"

	"CitadelDesktop/Server/Automation"
	"CitadelDesktop/Server/GameCommands"
	"CitadelDesktop/Server/GameFocus"
	"CitadelDesktop/Server/GameParser"
	"CitadelDesktop/Server/Logging"
	"CitadelDesktop/Server/Models"
	"CitadelDesktop/Server/ResponseRegistry"
)

const (
	autoStationCDSLID           = -14
	autoStationDelayHours       = 1
	autoStationClearSnapshots   = 3
	autoStationEvacuationRepeat = 10 * time.Second
	autoStationRecallRetry      = 10 * time.Second
	autoStationMovementGrace    = 30 * time.Second
)

type autoStationThreat struct {
	CastleID        int
	Count           int
	EarliestSeconds int
	EarliestImpact  int64
	LatestImpact    int64
}

type autoStationCastleRuntime struct {
	LastEvacuationAttempt time.Time
	LastObservedVersion   uint64
	ClearSnapshots        int
	LastError             string
}

var autoStationRuntime struct {
	sync.Mutex
	cancel           context.CancelFunc
	generation       uint64
	state            string
	threatCount      int
	nextImpactUnixMs int64
	detail           string
	threatCastles    map[int]struct{}
}

func autoStationLog(event, detail string) {
	if detail != "" {
		log.Printf("[AutoStation] %s: %s", event, detail)
	} else {
		log.Printf("[AutoStation] %s", event)
	}
	Logging.AppendAutoStationLine(event, detail)
}

// IsAutoStationRunning reports whether the global attack monitor is enabled.
func IsAutoStationRunning() bool {
	autoStationRuntime.Lock()
	running := autoStationRuntime.cancel != nil
	autoStationRuntime.Unlock()
	return running
}

// IsCastleUnderAutoStationThreat lets other automation avoid moving troops from a threatened castle.
func IsCastleUnderAutoStationThreat(castleID int) bool {
	autoStationRuntime.Lock()
	_, threatened := autoStationRuntime.threatCastles[castleID]
	autoStationRuntime.Unlock()
	return threatened
}

// GetAutoStationStatus returns the status payload shown in the global header.
func GetAutoStationStatus() (enabled bool, state string, threatCount int, nextImpactUnixMs int64, detail string) {
	autoStationRuntime.Lock()
	enabled = autoStationRuntime.cancel != nil
	state = autoStationRuntime.state
	if state == "" {
		state = "off"
	}
	threatCount = autoStationRuntime.threatCount
	nextImpactUnixMs = autoStationRuntime.nextImpactUnixMs
	detail = autoStationRuntime.detail
	autoStationRuntime.Unlock()
	return
}

// StartAutoStation starts monitoring authoritative GAM snapshots for hostile incoming attacks.
func StartAutoStation() {
	autoStationRuntime.Lock()
	if autoStationRuntime.cancel != nil {
		autoStationRuntime.Unlock()
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	autoStationRuntime.generation++
	generation := autoStationRuntime.generation
	autoStationRuntime.cancel = cancel
	autoStationRuntime.state = "armed"
	autoStationRuntime.detail = "Monitoring incoming attacks"
	autoStationRuntime.threatCastles = make(map[int]struct{})
	autoStationRuntime.Unlock()
	if ResponseRegistry.SendAutoStationStatusFunc != nil {
		go ResponseRegistry.SendAutoStationStatusFunc(true, "armed", 0, 0, "Monitoring incoming attacks")
	}
	go runAutoStation(ctx, generation)
}

// StopAutoStation stops monitoring. Already-stationed troops retain the one-hour fallback timer.
func StopAutoStation() {
	autoStationRuntime.Lock()
	if autoStationRuntime.cancel == nil {
		autoStationRuntime.Unlock()
		return
	}
	cancel := autoStationRuntime.cancel
	autoStationRuntime.cancel = nil
	autoStationRuntime.generation++
	autoStationRuntime.state = "off"
	autoStationRuntime.threatCount = 0
	autoStationRuntime.nextImpactUnixMs = 0
	autoStationRuntime.detail = ""
	autoStationRuntime.threatCastles = make(map[int]struct{})
	autoStationRuntime.Unlock()
	cancel()
	Automation.CancelOwner(Automation.OwnerAutoStation)
	if ResponseRegistry.SendAutoStationStatusFunc != nil {
		go ResponseRegistry.SendAutoStationStatusFunc(false, "off", 0, 0, "")
	}
}

func publishAutoStationStatus(generation uint64, state string, threatCount int, nextImpactUnixMs int64, detail string, threatened map[int]struct{}) {
	autoStationRuntime.Lock()
	if autoStationRuntime.cancel == nil || autoStationRuntime.generation != generation {
		autoStationRuntime.Unlock()
		return
	}
	changed := autoStationRuntime.state != state ||
		autoStationRuntime.threatCount != threatCount ||
		autoStationRuntime.nextImpactUnixMs != nextImpactUnixMs ||
		autoStationRuntime.detail != detail
	autoStationRuntime.state = state
	autoStationRuntime.threatCount = threatCount
	autoStationRuntime.nextImpactUnixMs = nextImpactUnixMs
	autoStationRuntime.detail = detail
	autoStationRuntime.threatCastles = copyAutoStationCastleSet(threatened)
	autoStationRuntime.Unlock()
	if changed && ResponseRegistry.SendAutoStationStatusFunc != nil {
		go ResponseRegistry.SendAutoStationStatusFunc(true, state, threatCount, nextImpactUnixMs, detail)
	}
}

func copyAutoStationCastleSet(input map[int]struct{}) map[int]struct{} {
	out := make(map[int]struct{}, len(input))
	for castleID := range input {
		out[castleID] = struct{}{}
	}
	return out
}

func runAutoStation(ctx context.Context, generation uint64) {
	autoStationLog("loop_start", "monitoring incoming attacks")
	defer autoStationLog("loop_stop", "monitor stopped")

	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()
	var playerID int
	var tracked []autoStationMovement
	runtimeByCastle := make(map[int]*autoStationCastleRuntime)
	var lastGAMRequest time.Time
	var lastAINRefresh time.Time

	for {
		if !runAutoStationCycle(ctx, generation, &playerID, &tracked, runtimeByCastle, &lastGAMRequest, &lastAINRefresh) {
			return
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func runAutoStationCycle(
	ctx context.Context,
	generation uint64,
	playerID *int,
	tracked *[]autoStationMovement,
	runtimeByCastle map[int]*autoStationCastleRuntime,
	lastGAMRequest *time.Time,
	lastAINRefresh *time.Time,
) bool {
	select {
	case <-ctx.Done():
		return false
	default:
	}

	gs := Models.GetGameState()
	if gs.PlayerID <= 0 || !ResponseRegistry.LoginStatus {
		publishAutoStationStatus(generation, "waiting", 0, 0, "Waiting for the game session", nil)
		return true
	}
	if *playerID != gs.PlayerID {
		*playerID = gs.PlayerID
		*tracked = loadAutoStationMovements(gs.PlayerID)
		for _, movement := range *tracked {
			runtimeByCastle[movement.SourceCastleID] = &autoStationCastleRuntime{}
		}
		autoStationLog("state_load", fmt.Sprintf("player=%d movements=%d", gs.PlayerID, len(*tracked)))
	}

	now := time.Now()
	incoming, ready, lastSnapshotUnix, snapshotVersion := gs.Movement.IncomingSnapshot()
	owned, _, ownedReady, _ := gs.Movement.Snapshot()
	versionResetChanged := false
	if snapshotVersion > 0 {
		for i := range *tracked {
			if (*tracked)[i].SnapshotVersion > snapshotVersion {
				(*tracked)[i].SnapshotVersion = 0
				versionResetChanged = true
			}
		}
		for _, runtime := range runtimeByCastle {
			if runtime.LastObservedVersion > snapshotVersion {
				runtime.LastObservedVersion = 0
				runtime.ClearSnapshots = 0
			}
		}
	}
	var reconciled bool
	*tracked, reconciled = reconcileAutoStationMovements(*tracked, owned, ownedReady, lastSnapshotUnix, snapshotVersion, now)
	if reconciled {
		persistAutoStationMovements(*playerID, *tracked)
	}

	threats, threatenedCastles, threatCount, earliestRemaining, nextImpactUnixMs := collectAutoStationThreats(gs, incoming, now)
	config := Models.GetSettingsState().AutoStation.Normalize()
	if threatCount > 0 {
		publishAutoStationStatus(generation, "threat", threatCount, nextImpactUnixMs,
			fmt.Sprintf("Incoming attack detected (%ds remaining)", earliestRemaining), threatenedCastles)
	}
	nearTrigger := threatCount > 0 && earliestRemaining <= config.LeadTimeSec+60
	pollInterval := 10 * time.Second
	if !ready {
		pollInterval = 2 * time.Second
	} else if nearTrigger {
		pollInterval = time.Second
	} else if threatCount > 0 {
		pollInterval = 3 * time.Second
	} else if len(*tracked) > 0 {
		pollInterval = 2 * time.Second
	}
	if lastGAMRequest.IsZero() || now.Sub(*lastGAMRequest) >= pollInterval {
		if GameParser.RequestGAMSnapshot() {
			*lastGAMRequest = now
		}
	}

	if threatCount > 0 && (lastAINRefresh.IsZero() || now.Sub(*lastAINRefresh) >= 30*time.Second) {
		refreshAutoStationAlliance(gs, lastAINRefresh)
	}

	trackedChanged := versionResetChanged || extendAutoStationSafeWindows(*tracked, threats)
	actionError := ""
	fresh := ready && snapshotVersion > 0 && lastSnapshotUnix > 0 && now.Unix()-lastSnapshotUnix <= 5
	castleIDs := sortedAutoStationThreatCastleIDs(threats)
	for _, castleID := range castleIDs {
		threat := threats[castleID]
		runtime := runtimeByCastle[castleID]
		if runtime == nil {
			runtime = &autoStationCastleRuntime{}
			runtimeByCastle[castleID] = runtime
		}
		runtime.ClearSnapshots = 0
		runtime.LastObservedVersion = snapshotVersion
		if threat.EarliestSeconds > config.LeadTimeSec {
			continue
		}
		if !fresh {
			actionError = "Confirming a fresh movement snapshot before evacuation"
			continue
		}
		if now.Sub(runtime.LastEvacuationAttempt) < autoStationEvacuationRepeat {
			if runtime.LastError != "" {
				actionError = runtime.LastError
			}
			continue
		}
		runtime.LastEvacuationAttempt = now
		publishAutoStationStatus(generation, "evacuating", threatCount, nextImpactUnixMs,
			fmt.Sprintf("Evacuating castle %d", castleID), threatenedCastles)
		created, result, err := evacuateAutoStationCastle(ctx, gs, config, threat, snapshotVersion, lastAINRefresh)
		if err != nil {
			actionError = err.Error()
			runtime.LastError = actionError
			autoStationLog("evacuation_error", fmt.Sprintf("castle=%d error=%v", castleID, err))
			continue
		}
		runtime.LastError = ""
		autoStationLog("evacuation", fmt.Sprintf("castle=%d %s", castleID, result))
		if created != nil {
			*tracked = append(*tracked, *created)
			trackedChanged = true
			GameParser.RequestGAMSnapshot()
		}
	}

	trackedCastles := make(map[int]struct{})
	safeAfterByCastle := make(map[int]int64)
	for _, movement := range *tracked {
		trackedCastles[movement.SourceCastleID] = struct{}{}
		if movement.SafeAfterUnix > safeAfterByCastle[movement.SourceCastleID] {
			safeAfterByCastle[movement.SourceCastleID] = movement.SafeAfterUnix
		}
	}
	for castleID := range trackedCastles {
		if _, threatened := threats[castleID]; threatened {
			continue
		}
		runtime := runtimeByCastle[castleID]
		if runtime == nil {
			runtime = &autoStationCastleRuntime{LastObservedVersion: snapshotVersion}
			runtimeByCastle[castleID] = runtime
			continue
		}
		if ready && lastSnapshotUnix >= safeAfterByCastle[castleID] && snapshotVersion > runtime.LastObservedVersion {
			runtime.LastObservedVersion = snapshotVersion
			runtime.ClearSnapshots++
		}
	}

	if config.RecallWhenClear {
		for i := range *tracked {
			movement := &(*tracked)[i]
			runtime := runtimeByCastle[movement.SourceCastleID]
			if movement.Returning || runtime == nil || runtime.ClearSnapshots < autoStationClearSnapshots {
				continue
			}
			if now.Unix() < movement.SafeAfterUnix+5 || now.Unix()-movement.LastRecallAttemptUnix < int64(autoStationRecallRetry/time.Second) {
				continue
			}
			publishAutoStationStatus(generation, "recalling", 0, 0,
				fmt.Sprintf("Recalling troops from castle %d", movement.SourceCastleID), threatenedCastles)
			movement.LastRecallAttemptUnix = now.Unix()
			trackedChanged = true
			if recallAutoStationMovement(movement.MID) {
				movement.Returning = true
				autoStationLog("recall", fmt.Sprintf("mid=%d castle=%d", movement.MID, movement.SourceCastleID))
				GameParser.RequestGAMSnapshot()
			} else {
				actionError = fmt.Sprintf("Recall failed for movement %d; retrying", movement.MID)
				autoStationLog("recall_error", fmt.Sprintf("mid=%d castle=%d", movement.MID, movement.SourceCastleID))
			}
		}
	}

	if trackedChanged {
		persistAutoStationMovements(*playerID, *tracked)
	}

	state, detail := autoStationDisplayState(threats, *tracked, config, actionError)
	publishAutoStationStatus(generation, state, threatCount, nextImpactUnixMs, detail, threatenedCastles)
	return true
}

func collectAutoStationThreats(gs *Models.GameState, incoming []Models.GAMMovement, now time.Time) (map[int]autoStationThreat, map[int]struct{}, int, int, int64) {
	threats := make(map[int]autoStationThreat)
	castles := make(map[int]struct{})
	count := 0
	earliestRemaining := int(^uint(0) >> 1)
	var nextImpact int64
	for _, movement := range incoming {
		if movement.D != 0 || movement.MovementType != 0 || movement.TT <= 0 ||
			!gs.IsKnownPlayerCastleID(movement.TargetCastleID) {
			continue
		}
		remaining := movement.TT - movement.EffectivePT(now.Unix())
		if remaining <= 0 {
			continue
		}
		impact := now.Unix() + int64(remaining)
		threat := threats[movement.TargetCastleID]
		if threat.Count == 0 || remaining < threat.EarliestSeconds {
			threat.EarliestSeconds = remaining
			threat.EarliestImpact = impact
		}
		if impact > threat.LatestImpact {
			threat.LatestImpact = impact
		}
		threat.CastleID = movement.TargetCastleID
		threat.Count++
		threats[movement.TargetCastleID] = threat
		castles[movement.TargetCastleID] = struct{}{}
		count++
		if remaining < earliestRemaining {
			earliestRemaining = remaining
			nextImpact = impact * 1000
		}
	}
	if count == 0 {
		earliestRemaining = 0
	}
	return threats, castles, count, earliestRemaining, nextImpact
}

func sortedAutoStationThreatCastleIDs(threats map[int]autoStationThreat) []int {
	ids := make([]int, 0, len(threats))
	for castleID := range threats {
		ids = append(ids, castleID)
	}
	sort.Ints(ids)
	return ids
}

func refreshAutoStationAlliance(gs *Models.GameState, lastRefresh *time.Time) {
	if gs.Alliance.AID <= 0 || !ResponseRegistry.IsGameWebSocketReady() {
		return
	}
	stateKey := Automation.StateEntity("alliance", gs.Alliance.AID)
	previous := Automation.StateSnapshot(stateKey).Version
	if !GameCommands.QueueFeatureRefresh(Automation.OwnerAutoStation, GameCommands.AINPayload(gs.Alliance.AID), nil) {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	_, _ = Automation.AwaitStateAfter(ctx, stateKey, previous)
	cancel()
	*lastRefresh = time.Now()
}

func autoStationBirdPosts(gs *Models.GameState, config Models.AutoStationConfig, sourceKID, sourceX, sourceY int) []birdPost {
	minimum := config.MinRPTDays * 86400
	posts := make([]birdPost, 0, len(gs.Alliance.BirdLocations))
	for _, location := range gs.Alliance.BirdLocations {
		if location.KingdomID != sourceKID || !isBirdTargetCastleType(location.CastleType) {
			continue
		}
		if location.X <= 0 || location.Y <= 0 {
			continue
		}
		if minimum > 0 && location.BirdTime <= minimum {
			continue
		}
		if location.X == sourceX && location.Y == sourceY {
			continue
		}
		posts = append(posts, birdPost{
			KingdomID: location.KingdomID,
			X:         location.X,
			Y:         location.Y,
			BirdTime:  location.BirdTime,
		})
	}
	return posts
}

func evacuateAutoStationCastle(
	ctx context.Context,
	gs *Models.GameState,
	config Models.AutoStationConfig,
	threat autoStationThreat,
	snapshotVersion uint64,
	lastAINRefresh *time.Time,
) (*autoStationMovement, string, error) {
	if time.Now().Unix() >= threat.LatestImpact {
		return nil, "", fmt.Errorf("attack timing changed before evacuation; refreshing movements")
	}
	castle := gs.GetCastleByID(threat.CastleID)
	if castle == nil {
		return nil, "", fmt.Errorf("castle %d is not available in game state", threat.CastleID)
	}
	kingdomID, sourceX, sourceY, ok := resolveAutoTCICastleMapCoords(gs, threat.CastleID, castle)
	if !ok {
		return nil, "", fmt.Errorf("castle %d has no known map coordinates", threat.CastleID)
	}
	if lastAINRefresh.IsZero() || time.Since(*lastAINRefresh) > 10*time.Second {
		refreshAutoStationAlliance(gs, lastAINRefresh)
	}
	if time.Now().Unix() >= threat.LatestImpact {
		return nil, "", fmt.Errorf("attack landed while refreshing station targets")
	}
	post := closestBirdPost(autoStationBirdPosts(gs, config, kingdomID, sourceX, sourceY), kingdomID, sourceX, sourceY)
	if post == nil {
		return nil, "", fmt.Errorf("castle %d has no protected same-kingdom alliance station target", threat.CastleID)
	}

	acquireWait := 10 * time.Second
	untilImpact := time.Until(time.Unix(threat.EarliestImpact, 0))
	if untilImpact > time.Second && untilImpact < acquireWait+time.Second {
		acquireWait = untilImpact - time.Second
	}
	if acquireWait < time.Second {
		acquireWait = time.Second
	}
	acquireCtx, cancel := context.WithTimeout(ctx, acquireWait)
	defer cancel()
	lease, ok := GameFocus.Acquire(acquireCtx, GameFocus.Request{
		Owner:    GameFocus.OwnerAutoStation,
		Priority: GameFocus.PriorityAutoStation,
		Reason:   fmt.Sprintf("incoming attack castle=%d", threat.CastleID),
		MaxHold:  30 * time.Second,
		Deadline: time.Unix(threat.EarliestImpact, 0),
		Claims:   []GameFocus.Claim{GameFocus.CastleClaim(threat.CastleID, "movement")},
	})
	if !ok {
		return nil, "", fmt.Errorf("castle %d focus is busy; evacuation will retry", threat.CastleID)
	}
	defer lease.Release()
	if !GameParser.FocusPlayerCastleTroopsWithLease(lease, kingdomID, threat.CastleID, sourceX, sourceY) {
		return nil, "", fmt.Errorf("castle %d troop refresh failed", threat.CastleID)
	}

	send := make(map[int]int)
	for unitID, amount := range castle.Troops.TroopsI {
		if amount <= 0 || !Models.IsTroop(unitID) {
			continue
		}
		available := amount - config.DefenseAmount(threat.CastleID, unitID)
		if available > 0 {
			send[unitID] = available
		}
	}
	if len(send) == 0 {
		return nil, "no troops above the configured defense reserve", nil
	}
	if !lease.Active() {
		return nil, "", fmt.Errorf("castle %d focus lease was revoked", threat.CastleID)
	}
	if time.Now().Unix() >= threat.LatestImpact {
		return nil, "", fmt.Errorf("attack landed before castle %d could evacuate", threat.CastleID)
	}
	GameCommands.QueueFeaturePayload(
		Automation.OwnerAutoStation,
		GameCommands.SDIPayload(post.X, post.Y, sourceX, sourceY),
		lease,
	)
	select {
	case <-ctx.Done():
		return nil, "", ctx.Err()
	case <-time.After(250 * time.Millisecond):
	}
	var movementMID, travelSeconds int
	success := GameCommands.SendCDSUntilSuccessWithLease(
		lease,
		threat.CastleID,
		post.X,
		post.Y,
		autoStationCDSLID,
		autoStationDelayHours,
		gs.GlobalResources.PTT,
		troopsMapToCDSJSON(send),
		func(parts []string) {
			if len(parts) <= 5 || parts[4] != "0" {
				return
			}
			movementMID, travelSeconds, _ = GameParser.MovementMIDAndTTFromGAMLikeJSON(parts[5])
		},
	)
	if !success {
		return nil, "", fmt.Errorf("castle %d station command failed", threat.CastleID)
	}
	result := fmt.Sprintf("sent %d troops to %d:%d (travel=%ds)", sumMap(send), post.X, post.Y, travelSeconds)
	if movementMID <= 0 {
		return nil, result + "; movement id unavailable, using one-hour fallback", nil
	}
	return &autoStationMovement{
		MID:             movementMID,
		SourceCastleID:  threat.CastleID,
		TargetX:         post.X,
		TargetY:         post.Y,
		SentAtUnix:      time.Now().Unix(),
		SafeAfterUnix:   threat.LatestImpact,
		SnapshotVersion: snapshotVersion,
	}, result, nil
}

func extendAutoStationSafeWindows(movements []autoStationMovement, threats map[int]autoStationThreat) bool {
	changed := false
	for i := range movements {
		threat, ok := threats[movements[i].SourceCastleID]
		if ok && threat.LatestImpact > movements[i].SafeAfterUnix {
			movements[i].SafeAfterUnix = threat.LatestImpact
			changed = true
		}
	}
	return changed
}

func reconcileAutoStationMovements(
	tracked []autoStationMovement,
	owned []Models.GAMMovement,
	ready bool,
	lastSnapshotUnix int64,
	snapshotVersion uint64,
	now time.Time,
) ([]autoStationMovement, bool) {
	byMID := make(map[int]Models.GAMMovement, len(owned))
	for _, movement := range owned {
		byMID[movement.MID] = movement
	}
	kept := make([]autoStationMovement, 0, len(tracked))
	changed := false
	for _, movement := range tracked {
		active, found := byMID[movement.MID]
		if found {
			if active.D == 1 && !movement.Returning {
				movement.Returning = true
				changed = true
			}
			kept = append(kept, movement)
			continue
		}
		snapshotAfterSend := ready && snapshotVersion > movement.SnapshotVersion && lastSnapshotUnix >= movement.SentAtUnix
		if snapshotAfterSend && now.Sub(time.Unix(movement.SentAtUnix, 0)) >= autoStationMovementGrace {
			changed = true
			autoStationLog("movement_clear", fmt.Sprintf("mid=%d castle=%d", movement.MID, movement.SourceCastleID))
			continue
		}
		kept = append(kept, movement)
	}
	return kept, changed
}

func recallAutoStationMovement(mid int) bool {
	if mid <= 0 || !ResponseRegistry.IsGameWebSocketReady() {
		return false
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	err := Automation.RunWork(ctx, Automation.WorkItem{
		DedupeKey: fmt.Sprintf("autoStation:recall:%d", mid),
		Request: Automation.Request{
			Owner:        Automation.OwnerAutoStation,
			Priority:     Automation.PriorityAutoStation,
			Reason:       fmt.Sprintf("recall movement=%d", mid),
			Claims:       []Automation.Claim{Automation.MovementClaim(mid)},
			MaxHold:      5 * time.Second,
			PreemptLower: true,
		},
		Run: func(workCtx context.Context, lease *Automation.Lease) error {
			waiter := ResponseRegistry.Global.RegisterWaiterMatching("mcm", 5*time.Second, func(parts []string) bool {
				payload, ok := GameParser.Payload(parts)
				if !ok {
					return false
				}
				responseMID, _, ok := GameParser.MovementMIDAndTTFromGAMLikeJSON(payload)
				return ok && responseMID == mid
			}, nil)
			defer waiter.Cleanup()
			if !GameCommands.QueueFeaturePayload(Automation.OwnerAutoStation, GameCommands.MCMPayload(mid), lease) {
				return Automation.ErrWorkCancelled
			}
			response, waitErr := waiter.WaitWithContext(workCtx)
			if waitErr != nil {
				return waitErr
			}
			if len(response) <= 4 || response[4] != "0" {
				return fmt.Errorf("mcm recall rejected")
			}
			return nil
		},
	})
	return err == nil
}

func persistAutoStationMovements(playerID int, movements []autoStationMovement) {
	if err := saveAutoStationMovements(playerID, movements); err != nil {
		autoStationLog("state_save_error", err.Error())
	}
}

func autoStationDisplayState(
	threats map[int]autoStationThreat,
	movements []autoStationMovement,
	config Models.AutoStationConfig,
	actionError string,
) (string, string) {
	if actionError != "" {
		if actionError == "Confirming a fresh movement snapshot before evacuation" {
			return "waiting", actionError
		}
		return "error", actionError
	}
	if len(threats) > 0 {
		earliest := int(^uint(0) >> 1)
		for _, threat := range threats {
			if threat.EarliestSeconds < earliest {
				earliest = threat.EarliestSeconds
			}
		}
		if earliest <= config.LeadTimeSec {
			return "protected", fmt.Sprintf("Threat active; evacuation window reached (%ds remaining)", earliest)
		}
		return "threat", fmt.Sprintf("Incoming attack detected (%ds remaining)", earliest)
	}
	if len(movements) > 0 {
		returning := 0
		for _, movement := range movements {
			if movement.Returning {
				returning++
			}
		}
		if returning > 0 {
			return "recalling", fmt.Sprintf("%d evacuation movement(s) returning", returning)
		}
		if config.RecallWhenClear {
			return "protected", "Threat cleared; confirming before recall"
		}
		return "protected", "Troops stationed with the one-hour fallback"
	}
	return "armed", "Monitoring incoming attacks"
}
