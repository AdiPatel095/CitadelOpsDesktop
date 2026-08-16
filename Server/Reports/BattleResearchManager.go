package Reports

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"CitadelDesktop/Server/Configuration"
	"CitadelDesktop/Server/GameData"
	"CitadelDesktop/Server/Ingest"
	"CitadelDesktop/Server/Intent"
	"CitadelDesktop/Server/Protocol"
	"CitadelDesktop/Server/State"
)

const (
	battleResearchActor              = "background:battle-research-beta"
	battleResearchTickInterval       = time.Second
	battleResearchMovementPoll       = 5 * time.Second
	battleResearchUploadRetry        = 30 * time.Second
	battleResearchSpyRetry           = time.Minute
	battleResearchFormationMatch     = 2 * time.Minute
	battleResearchFormationExpiry    = 10 * time.Minute
	battleResearchReportMatchWindow  = 15 * time.Minute
	battleResearchMaximumSpyAttempts = 3
)

type battleResearchFrameSource interface {
	SubscribeFrames(int) (<-chan Protocol.CommittedFrame, func())
}

type battleResearchIntentSubmitter interface {
	Submit(context.Context, Intent.Request) Intent.Receipt
}

type battleResearchArchiveEvent struct {
	spy           *SpyReport
	spyCapture    *State.SpyReportCapture
	battle        *BattleReport
	battleCapture *State.BattleReportCapture
}

type BattleResearchManager struct {
	state         *State.Store
	configuration *Configuration.Store
	gameData      *GameData.Manager
	intents       battleResearchIntentSubmitter
	frames        battleResearchFrameSource
	store         *SQLiteStore
	cloud         *CloudClient

	mu                 sync.RWMutex
	trials             map[string]BattleResearchTrial
	lastMovementPollAt time.Time
	lastUploadAttempt  time.Time
	lastError          string
	events             chan battleResearchArchiveEvent
	done               chan struct{}
	started            atomic.Bool
	workers            sync.WaitGroup
}

func NewBattleResearchManager(
	state *State.Store,
	configuration *Configuration.Store,
	gameData *GameData.Manager,
	intents battleResearchIntentSubmitter,
	frames battleResearchFrameSource,
	store *SQLiteStore,
	cloud *CloudClient,
) *BattleResearchManager {
	manager := &BattleResearchManager{
		state: state, configuration: configuration, gameData: gameData, intents: intents,
		frames: frames, store: store, cloud: cloud,
		trials: map[string]BattleResearchTrial{}, events: make(chan battleResearchArchiveEvent, 128),
		done: make(chan struct{}),
	}
	if store != nil {
		if trials, err := store.ListBattleResearchTrials(context.Background(), battleResearchTrialLoadLimit); err == nil {
			for _, trial := range trials {
				manager.trials[trial.ID] = trial
			}
		} else {
			manager.lastError = err.Error()
		}
	}
	return manager
}

func (manager *BattleResearchManager) ObserveSpyReport(report SpyReport, capture State.SpyReportCapture) {
	if manager == nil {
		return
	}
	reportCopy := report
	captureCopy := capture
	select {
	case manager.events <- battleResearchArchiveEvent{spy: &reportCopy, spyCapture: &captureCopy}:
	default:
		manager.setLastError("battle research report queue is full")
	}
}

func (manager *BattleResearchManager) ObserveBattleReport(report BattleReport, capture State.BattleReportCapture) {
	if manager == nil {
		return
	}
	reportCopy := report
	captureCopy := capture
	select {
	case manager.events <- battleResearchArchiveEvent{battle: &reportCopy, battleCapture: &captureCopy}:
	default:
		manager.setLastError("battle research report queue is full")
	}
}

func (manager *BattleResearchManager) Run(ctx context.Context) {
	if manager == nil {
		return
	}
	if !manager.started.CompareAndSwap(false, true) {
		return
	}
	defer close(manager.done)
	if manager.state == nil || manager.configuration == nil || manager.gameData == nil || manager.intents == nil ||
		manager.frames == nil || manager.store == nil {
		return
	}
	frames, unsubscribe := manager.frames.SubscribeFrames(512)
	defer unsubscribe()
	configurationEvents, unsubscribeConfiguration := manager.configuration.Subscribe(8)
	defer unsubscribeConfiguration()
	var tickTimer *time.Timer
	var tickChannel <-chan time.Time
	scheduleTick := func() {
		if tickTimer != nil {
			if !tickTimer.Stop() {
				select {
				case <-tickTimer.C:
				default:
				}
			}
			tickTimer = nil
			tickChannel = nil
		}
		if !manager.currentConfiguration().Active() {
			return
		}
		tickTimer = time.NewTimer(battleResearchTickInterval)
		tickChannel = tickTimer.C
	}
	scheduleTick()
	for {
		select {
		case <-ctx.Done():
			if tickTimer != nil {
				tickTimer.Stop()
			}
			return
		case frame := <-frames:
			manager.handleFrame(ctx, frame)
		case event := <-manager.events:
			manager.handleArchiveEvent(ctx, event)
		case event := <-configurationEvents:
			if event.Gap || strings.EqualFold(strings.TrimSpace(event.Section), BattleResearchConfigurationSection) {
				scheduleTick()
			}
		case now := <-tickChannel:
			tickTimer = nil
			tickChannel = nil
			manager.tick(ctx, now.UTC())
			scheduleTick()
		}
	}
}

func (manager *BattleResearchManager) Wait() {
	if manager == nil || manager.done == nil || !manager.started.Load() {
		return
	}
	<-manager.done
	manager.workers.Wait()
}

func (manager *BattleResearchManager) Status() BattleResearchStatus {
	if manager == nil {
		return BattleResearchStatus{
			Beta: true, RequiredConsentVersion: BattleResearchConsentVersion,
			Calculator: researchCalculatorInfo(), State: "disabled",
		}
	}
	configuration := manager.currentConfiguration()
	status := BattleResearchStatus{
		Beta: true, Enabled: configuration.Active(), ConsentVersion: configuration.ConsentVersion,
		RequiredConsentVersion: BattleResearchConsentVersion, Calculator: researchCalculatorInfo(), State: "disabled",
	}
	manager.mu.RLock()
	status.LastMovementPollAt = manager.lastMovementPollAt
	status.LastError = manager.lastError
	trials := make([]BattleResearchTrial, 0, len(manager.trials))
	for _, trial := range manager.trials {
		trials = append(trials, trial)
		if trial.CompletedAt.IsZero() {
			if trial.Phase != "expired" && !strings.HasPrefix(trial.Phase, "missed_") {
				status.ActiveTrials++
			}
		} else {
			status.CompletedTrials++
		}
		if trial.UploadState == "pending" {
			status.PendingUploads++
		}
	}
	manager.mu.RUnlock()
	if configuration.Enabled && !configuration.Active() {
		status.State = "consent-update-required"
	} else if configuration.Active() {
		status.State = "waiting-for-session"
		if manager.state != nil {
			snapshot := manager.state.ReadOnlyView()
			if snapshot.Session.LoggedIn && snapshot.Session.SocketReady &&
				snapshot.Session.BaselineGeneration == snapshot.Session.Generation {
				status.State = "observing"
			}
		}
	}
	sort.Slice(trials, func(left, right int) bool {
		if !trials[left].UpdatedAt.Equal(trials[right].UpdatedAt) {
			return trials[left].UpdatedAt.After(trials[right].UpdatedAt)
		}
		return trials[left].ID > trials[right].ID
	})
	if len(trials) > 12 {
		trials = trials[:12]
	}
	status.Trials = make([]BattleResearchTrialSummary, 0, len(trials))
	for _, trial := range trials {
		summary := BattleResearchTrialSummary{
			ID: trial.ID, Phase: trial.Phase, TargetX: trial.Formation.TargetX, TargetY: trial.Formation.TargetY,
			KingdomID: trial.Formation.KingdomID, CreatedAt: trial.CreatedAt, UpdatedAt: trial.UpdatedAt,
			Prediction: trial.Prediction, UploadState: trial.UploadState, LastError: trial.LastError,
		}
		if trial.Movement != nil {
			summary.MovementID = int64(trial.Movement.ID)
			summary.ArrivesAt = trial.Movement.ArrivesAt
		}
		if trial.Battle != nil {
			summary.ActualResult = trial.Battle.Report.Result
			attackerLost := trial.Battle.Report.Metrics.AttackerLost
			defenderLost := trial.Battle.Report.Metrics.DefenderLost
			summary.ActualAttackerLost = &attackerLost
			summary.ActualDefenderLost = &defenderLost
		}
		status.Trials = append(status.Trials, summary)
	}
	return status
}

func (manager *BattleResearchManager) handleFrame(ctx context.Context, committed Protocol.CommittedFrame) {
	if !manager.currentConfiguration().Active() || committed.Frame.Direction != Protocol.DirectionOutbound ||
		!strings.EqualFold(committed.Frame.Opcode, "cra") || len(committed.Frame.Payload) == 0 {
		return
	}
	formation, err := parseBattleResearchFormation(committed.Frame.Payload, committed.Frame.ReceivedAt)
	if err != nil {
		return
	}
	snapshot := manager.state.ReadOnlyView()
	if snapshot.Player.ID <= 0 || !snapshot.Session.LoggedIn || !snapshot.Session.SocketReady ||
		snapshot.Session.BaselineGeneration != snapshot.Session.Generation {
		return
	}
	trial := BattleResearchTrial{
		Version: 1, Phase: "formation_captured", AccountUID: snapshot.Account.UID,
		WorldID: snapshot.Account.WorldID, PlayerID: snapshot.Player.ID,
		CreatedAt: formation.CapturedAt, UpdatedAt: formation.CapturedAt,
		Formation: formation, AttackerContext: captureBattleResearchAttackerContext(snapshot, formation),
		UploadState: "not-ready",
	}
	trial.ID = battleResearchTrialID(trial)
	manager.mu.Lock()
	if _, exists := manager.trials[trial.ID]; !exists {
		manager.trials[trial.ID] = trial
		if err := manager.store.SaveBattleResearchTrial(ctx, trial); err != nil {
			manager.lastError = err.Error()
		}
	}
	manager.mu.Unlock()
}

func (manager *BattleResearchManager) tick(ctx context.Context, now time.Time) {
	configuration := manager.currentConfiguration()
	if !configuration.Active() {
		return
	}
	snapshot := manager.state.ReadOnlyView()
	ready := snapshot.Session.LoggedIn && snapshot.Session.SocketReady &&
		snapshot.Session.BaselineGeneration == snapshot.Session.Generation
	if ready && manager.needsMovementPoll(now) && manager.movementPollDue(now) {
		manager.pollMovements(ctx, now)
		snapshot = manager.state.ReadOnlyView()
	}
	manager.reconcileTrials(ctx, snapshot, configuration, now)
	if now.Sub(manager.lastUploadTime()) >= battleResearchUploadRetry {
		manager.uploadNext(ctx, now)
	}
}

func (manager *BattleResearchManager) pollMovements(ctx context.Context, now time.Time) {
	requestContext, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	receipt := manager.intents.Submit(requestContext, Intent.Request{
		Name: "game.refresh_movements", Actor: battleResearchActor,
	})
	manager.mu.Lock()
	manager.lastMovementPollAt = now
	if receipt.Status != Intent.StatusSucceeded {
		manager.lastError = "movement poll: " + strings.TrimSpace(receipt.Error)
	}
	manager.mu.Unlock()
}

func (manager *BattleResearchManager) reconcileTrials(
	ctx context.Context,
	snapshot State.GameState,
	configuration BattleResearchConfiguration,
	now time.Time,
) {
	manager.mu.Lock()
	defer manager.mu.Unlock()
	assigned := map[State.MovementID]struct{}{}
	for _, trial := range manager.trials {
		if trial.Movement != nil {
			assigned[trial.Movement.ID] = struct{}{}
		}
	}
	ids := make([]string, 0, len(manager.trials))
	for id := range manager.trials {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		trial := manager.trials[id]
		changed := false
		if trial.Movement == nil {
			if movement, found := matchBattleResearchMovement(trial, snapshot, assigned, now); found {
				movementCopy := movement
				trial.Movement = &movementCopy
				trial.Phase = "movement_observed"
				trial.LastError = ""
				assigned[movement.ID] = struct{}{}
				changed = true
			} else if now.Sub(trial.Formation.CapturedAt) > battleResearchFormationExpiry {
				trial.Phase = "expired"
				trial.LastError = "No matching outgoing PvP attack movement was observed within ten minutes."
				changed = true
			}
		}
		if trial.Movement != nil && trial.PreSpy == nil && trial.Battle == nil &&
			(trial.PreSpyRequestedAt.IsZero() || now.After(trial.NextActionAt)) {
			if trial.Movement.ArrivesAt != nil && now.Before(*trial.Movement.ArrivesAt) &&
				trial.PreSpyAttempts < battleResearchMaximumSpyAttempts {
				changed = manager.requestSpyLocked(ctx, &trial, configuration.SpyCount, false, now) || changed
			} else if trial.Movement.ArrivesAt != nil && !now.Before(*trial.Movement.ArrivesAt) {
				trial.Phase = "missed_pre_spy"
				trial.LastError = "A pre-battle spy report was not captured before impact."
				changed = true
			}
		}
		if trial.PreSpy != nil && trial.Prediction == nil && trial.Battle == nil && trial.Movement != nil &&
			trial.Movement.ArrivesAt != nil && now.Before(*trial.Movement.ArrivesAt) {
			if data, ready := manager.gameData.Current(); ready {
				prediction, err := BuildBattleResearchPrediction(
					trial.Formation, trial.AttackerContext, *trial.Movement, trial.PreSpy.Report, data, now,
				)
				if err == nil {
					trial.Prediction = &prediction
					trial.Phase = "prediction_saved"
					trial.LastError = ""
					changed = true
				} else {
					trial.LastError = err.Error()
					changed = true
				}
			}
		}
		if trial.Battle != nil && trial.PostSpy == nil &&
			(trial.PostSpyRequestedAt.IsZero() || now.After(trial.NextActionAt)) &&
			trial.PostSpyAttempts < battleResearchMaximumSpyAttempts {
			changed = manager.requestSpyLocked(ctx, &trial, configuration.SpyCount, true, now) || changed
		}
		if changed {
			trial.UpdatedAt = now
			manager.trials[id] = trial
			if err := manager.store.SaveBattleResearchTrial(ctx, trial); err != nil {
				manager.lastError = err.Error()
			}
		}
	}
}

func (manager *BattleResearchManager) requestSpyLocked(
	ctx context.Context,
	trial *BattleResearchTrial,
	spyCount int,
	post bool,
	now time.Time,
) bool {
	if trial == nil || trial.Movement == nil {
		return false
	}
	if spyCount < 1 || spyCount > 100 {
		spyCount = 1
	}
	if post {
		trial.PostSpyRequestedAt = now
		trial.PostSpyAttempts++
		trial.Phase = "post_spy_requested"
	} else {
		trial.PreSpyRequestedAt = now
		trial.PreSpyAttempts++
		trial.Phase = "pre_spy_requested"
	}
	trial.NextActionAt = now.Add(battleResearchSpyRetry)
	trial.UpdatedAt = now
	manager.trials[trial.ID] = *trial
	if err := manager.store.SaveBattleResearchTrial(ctx, *trial); err != nil {
		trial.LastError = err.Error()
		manager.lastError = err.Error()
		return true
	}
	arguments := mustResearchJSON(map[string]any{
		"sourceCastleId": int64(trial.Movement.SourceCastleID),
		"targetX":        trial.Movement.TargetX, "targetY": trial.Movement.TargetY,
		"kingdomId": int64(trial.Movement.KingdomID), "spyCount": spyCount,
	})
	trialID := trial.ID
	manager.workers.Add(1)
	go func() {
		defer manager.workers.Done()
		manager.submitSpy(ctx, trialID, arguments, post)
	}()
	return true
}

func (manager *BattleResearchManager) submitSpy(ctx context.Context, trialID string, arguments json.RawMessage, post bool) {
	requestContext, cancel := context.WithTimeout(ctx, 30*time.Second)
	receipt := manager.intents.Submit(requestContext, Intent.Request{
		Name: "spy.launch", Actor: battleResearchActor, Arguments: arguments,
	})
	cancel()
	manager.mu.Lock()
	defer manager.mu.Unlock()
	trial, exists := manager.trials[trialID]
	if !exists {
		return
	}
	// The report can arrive before the asynchronous intent receipt. Do not let a
	// late failure receipt move a successfully captured trial back into retry.
	if (post && trial.PostSpy != nil) || (!post && trial.PreSpy != nil) {
		return
	}
	if receipt.Status != Intent.StatusSucceeded {
		trial.LastError = "spy launch: " + strings.TrimSpace(receipt.Error)
		manager.lastError = trial.LastError
		if post {
			trial.Phase = "post_spy_retry_wait"
		} else {
			trial.Phase = "pre_spy_retry_wait"
		}
	} else {
		trial.LastError = ""
	}
	trial.UpdatedAt = time.Now().UTC()
	manager.trials[trialID] = trial
	if err := manager.store.SaveBattleResearchTrial(context.Background(), trial); err != nil {
		manager.lastError = err.Error()
	}
}

func (manager *BattleResearchManager) handleArchiveEvent(ctx context.Context, event battleResearchArchiveEvent) {
	if !manager.currentConfiguration().Active() {
		return
	}
	if event.spy != nil && event.spyCapture != nil {
		manager.handleSpyReport(ctx, *event.spy, *event.spyCapture)
	}
	if event.battle != nil && event.battleCapture != nil {
		manager.handleBattleReport(ctx, *event.battle, *event.battleCapture)
	}
}

func (manager *BattleResearchManager) handleSpyReport(ctx context.Context, report SpyReport, capture State.SpyReportCapture) {
	capturedAt := time.UnixMilli(report.CapturedAtUnixMillis).UTC()
	manager.mu.Lock()
	defer manager.mu.Unlock()
	ids := manager.matchingTrialIDs(func(trial BattleResearchTrial) bool {
		return trial.Movement != nil && researchSpyMatchesTrial(report, trial)
	})
	for _, post := range []bool{true, false} {
		for _, id := range ids {
			trial := manager.trials[id]
			requestedAt := trial.PreSpyRequestedAt
			waiting := trial.PreSpy == nil && trial.Battle == nil
			purpose := "pre-battle"
			if post {
				requestedAt = trial.PostSpyRequestedAt
				waiting = trial.Battle != nil && trial.PostSpy == nil
				purpose = "post-battle"
			}
			if !waiting || requestedAt.IsZero() || capturedAt.Before(requestedAt.Add(-2*time.Second)) {
				continue
			}
			snapshot := &BattleResearchSpySnapshot{
				Purpose: purpose, CapturedAt: capturedAt, Report: report, RawCapture: capture,
			}
			if post {
				trial.PostSpy = snapshot
				if trial.Prediction != nil {
					trial.Phase = "complete"
					trial.CompletedAt = capturedAt
					trial.UploadState = "pending"
				} else {
					trial.Phase = "missed_prediction"
					trial.LastError = "The battle resolved before a pre-impact prediction could be saved."
				}
			} else {
				trial.PreSpy = snapshot
				if trial.Movement.ArrivesAt != nil && !capturedAt.Before(*trial.Movement.ArrivesAt) {
					trial.Phase = "missed_pre_spy"
					trial.LastError = "The pre-battle spy report arrived after impact."
				} else {
					trial.Phase = "pre_spy_captured"
					trial.LastError = ""
				}
			}
			trial.UpdatedAt = capturedAt
			manager.trials[id] = trial
			if err := manager.store.SaveBattleResearchTrial(ctx, trial); err != nil {
				manager.lastError = err.Error()
			}
			return
		}
	}
}

func (manager *BattleResearchManager) handleBattleReport(ctx context.Context, report BattleReport, capture State.BattleReportCapture) {
	if report.Role != "attacker" || report.Attacker == nil || report.Attacker.PlayerID <= 0 {
		return
	}
	manager.mu.Lock()
	defer manager.mu.Unlock()
	ids := manager.matchingTrialIDs(func(trial BattleResearchTrial) bool {
		return trial.Battle == nil && trial.Movement != nil && researchBattleMatchesTrial(report, trial)
	})
	if len(ids) == 0 {
		return
	}
	id := ids[0]
	trial := manager.trials[id]
	capturedAt := capture.CapturedAt.UTC()
	if capturedAt.IsZero() {
		capturedAt = time.UnixMilli(report.DateMs).UTC()
	}
	trial.Battle = &BattleResearchOutcome{CapturedAt: capturedAt, Report: report, RawCapture: capture}
	trial.Phase = "battle_captured"
	trial.UpdatedAt = capturedAt
	trial.LastError = ""
	manager.trials[id] = trial
	if err := manager.store.SaveBattleResearchTrial(ctx, trial); err != nil {
		manager.lastError = err.Error()
	}
}

func (manager *BattleResearchManager) uploadNext(ctx context.Context, now time.Time) {
	manager.mu.Lock()
	manager.lastUploadAttempt = now
	if manager.cloud == nil {
		manager.mu.Unlock()
		return
	}
	var selected *BattleResearchTrial
	for _, trial := range manager.trials {
		if trial.UploadState != "pending" {
			continue
		}
		if selected == nil || trial.CompletedAt.Before(selected.CompletedAt) {
			copyValue := trial
			selected = &copyValue
		}
	}
	manager.mu.Unlock()
	if selected == nil {
		return
	}
	uploadContext, cancel := context.WithTimeout(ctx, 30*time.Second)
	err := manager.cloud.UploadBattleResearch(uploadContext, *selected)
	cancel()
	manager.mu.Lock()
	defer manager.mu.Unlock()
	trial, exists := manager.trials[selected.ID]
	if !exists || trial.UploadState != "pending" {
		return
	}
	trial.UpdatedAt = now
	if err != nil {
		trial.LastError = err.Error()
		manager.lastError = err.Error()
	} else {
		trial.UploadState = "uploaded"
		trial.UploadedAt = now
		trial.LastError = ""
	}
	manager.trials[trial.ID] = trial
	if saveErr := manager.store.SaveBattleResearchTrial(ctx, trial); saveErr != nil {
		manager.lastError = saveErr.Error()
	}
}

func (manager *BattleResearchManager) currentConfiguration() BattleResearchConfiguration {
	configuration := BattleResearchConfiguration{SpyCount: 1}
	if manager == nil || manager.configuration == nil {
		return configuration
	}
	raw, found := manager.configuration.Section(BattleResearchConfigurationSection)
	if found {
		_ = json.Unmarshal(raw, &configuration)
	}
	if configuration.SpyCount < 1 || configuration.SpyCount > 100 {
		configuration.SpyCount = 1
	}
	return configuration
}

func (manager *BattleResearchManager) movementPollDue(now time.Time) bool {
	manager.mu.RLock()
	last := manager.lastMovementPollAt
	manager.mu.RUnlock()
	return last.IsZero() || now.Sub(last) >= battleResearchMovementPoll
}

func (manager *BattleResearchManager) needsMovementPoll(now time.Time) bool {
	manager.mu.RLock()
	defer manager.mu.RUnlock()
	for _, trial := range manager.trials {
		if trial.Movement == nil && trial.Phase != "expired" &&
			now.Sub(trial.Formation.CapturedAt) <= battleResearchFormationExpiry {
			return true
		}
	}
	return false
}

func (manager *BattleResearchManager) lastUploadTime() time.Time {
	manager.mu.RLock()
	last := manager.lastUploadAttempt
	manager.mu.RUnlock()
	return last
}

func (manager *BattleResearchManager) setLastError(message string) {
	if manager == nil {
		return
	}
	manager.mu.Lock()
	manager.lastError = strings.TrimSpace(message)
	manager.mu.Unlock()
}

func (manager *BattleResearchManager) matchingTrialIDs(matches func(BattleResearchTrial) bool) []string {
	ids := make([]string, 0)
	for id, trial := range manager.trials {
		if matches(trial) {
			ids = append(ids, id)
		}
	}
	sort.Slice(ids, func(left, right int) bool {
		return manager.trials[ids[left]].CreatedAt.After(manager.trials[ids[right]].CreatedAt)
	})
	return ids
}

func matchBattleResearchMovement(
	trial BattleResearchTrial,
	snapshot State.GameState,
	assigned map[State.MovementID]struct{},
	now time.Time,
) (State.MovementState, bool) {
	var selected State.MovementState
	best := battleResearchFormationMatch + time.Second
	snapshot.RangeMovements(func(_ State.MovementID, movement State.MovementState) bool {
		if _, used := assigned[movement.ID]; used || !State.IsOutgoingPlayerAttack(snapshot, movement, now) ||
			movement.KingdomID != trial.Formation.KingdomID || movement.TargetX != trial.Formation.TargetX ||
			movement.TargetY != trial.Formation.TargetY || movement.SourceX != trial.Formation.SourceX ||
			movement.SourceY != trial.Formation.SourceY {
			return true
		}
		if movement.CommanderID != nil && *movement.CommanderID != trial.Formation.CommanderID {
			return true
		}
		observedLaunch := movement.StartedAt
		if observedLaunch.IsZero() {
			observedLaunch = movement.ObservedAt
		}
		delta := observedLaunch.Sub(trial.Formation.CapturedAt)
		if delta < 0 {
			delta = -delta
		}
		if delta <= battleResearchFormationMatch && delta < best {
			selected = movement
			best = delta
		}
		return true
	})
	return selected, selected.ID > 0
}

func researchSpyMatchesTrial(report SpyReport, trial BattleResearchTrial) bool {
	if trial.Movement == nil || report.Castle.KingdomID != int(trial.Movement.KingdomID) ||
		report.Castle.X != trial.Movement.TargetX || report.Castle.Y != trial.Movement.TargetY {
		return false
	}
	if report.Castle.ID > 0 && trial.Movement.TargetCastleID > 0 && report.Castle.ID != int64(trial.Movement.TargetCastleID) {
		return false
	}
	return report.Target.ID <= 0 || trial.Movement.TargetPlayerID <= 0 ||
		report.Target.ID == int64(trial.Movement.TargetPlayerID)
}

func researchBattleMatchesTrial(report BattleReport, trial BattleResearchTrial) bool {
	if trial.Movement == nil || report.Attacker == nil || report.Attacker.PlayerID != int64(trial.PlayerID) {
		return false
	}
	if report.MovementID > 0 && report.MovementID == int64(trial.Movement.ID) {
		return true
	}
	if report.KingdomID != int(trial.Movement.KingdomID) || report.TargetX != trial.Movement.TargetX ||
		report.TargetY != trial.Movement.TargetY || trial.Movement.ArrivesAt == nil {
		return false
	}
	delta := time.UnixMilli(report.DateMs).Sub(*trial.Movement.ArrivesAt)
	if delta < 0 {
		delta = -delta
	}
	return delta <= battleResearchReportMatchWindow
}

func captureBattleResearchAttackerContext(
	snapshot State.GameState,
	formation BattleResearchFormation,
) BattleResearchAttackerContext {
	context := BattleResearchAttackerContext{
		CapturedAt: formation.CapturedAt, CatalogVersion: snapshot.CatalogVersion,
		LegendSkills: snapshot.Player.LegendSkills,
	}
	commander, found := snapshot.Commanders[formation.CommanderID]
	if !found {
		return context
	}
	context.Commander = commander
	if general, exists := snapshot.Generals[commander.GeneralID]; exists {
		generalCopy := general
		context.General = &generalCopy
	}
	equipmentIDs := map[State.EquipmentInstanceID]struct{}{}
	for _, equipmentID := range commander.Equipment {
		if equipmentID > 0 {
			equipmentIDs[equipmentID] = struct{}{}
		}
	}
	for equipmentID := range equipmentIDs {
		if equipment, exists := snapshot.Inventory.Equipment[equipmentID]; exists {
			context.Equipment = append(context.Equipment, equipment)
		}
	}
	gemIDs := map[State.GemInstanceID]struct{}{}
	for _, gemID := range commander.Gems {
		if gemID > 0 {
			gemIDs[gemID] = struct{}{}
		}
	}
	for gemID, gem := range snapshot.Inventory.Gems {
		if _, direct := gemIDs[gemID]; direct {
			context.Gems = append(context.Gems, gem)
			continue
		}
		if _, equipped := equipmentIDs[gem.EquipmentInstanceID]; equipped {
			context.Gems = append(context.Gems, gem)
		}
	}
	dialog := snapshot.AttackDialog
	if dialog.SourceCastleID > 0 && dialog.KingdomID == formation.KingdomID &&
		dialog.Target.X == formation.TargetX && dialog.Target.Y == formation.TargetY &&
		!dialog.ObservedAt.IsZero() && formation.CapturedAt.Sub(dialog.ObservedAt) <= 15*time.Minute {
		dialogCopy := dialog
		context.AttackDialog = &dialogCopy
	}
	return context
}

func battleResearchTrialID(trial BattleResearchTrial) string {
	identity := fmt.Sprintf("%d|%s|%d|%d|%s", trial.AccountUID, trial.WorldID, trial.PlayerID,
		trial.Formation.CapturedAt.UnixNano(), string(trial.Formation.Raw))
	sum := sha256.Sum256([]byte(identity))
	return "battle-beta-" + hex.EncodeToString(sum[:12])
}

func mustResearchJSON(value any) json.RawMessage {
	raw, _ := json.Marshal(value)
	return raw
}

var _ ArchiveObserver = (*BattleResearchManager)(nil)
var _ battleResearchFrameSource = (*Ingest.Pipeline)(nil)
