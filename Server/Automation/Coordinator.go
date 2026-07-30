package Automation

import (
	"bytes"
	"context"
	"encoding/json"
	"reflect"
	"slices"
	"sort"
	"strings"
	"sync/atomic"
	"time"

	"CitadelDesktop/Server/Configuration"
	"CitadelDesktop/Server/GameData"
	"CitadelDesktop/Server/Intent"
	"CitadelDesktop/Server/State"
)

const (
	coordinatorTick          = 2 * time.Second
	stateChangeDebounce      = 250 * time.Millisecond
	defaultRetry             = 30 * time.Second
	boundedAttackIntentTTL   = 2 * time.Minute
	maxImmediatePolicyRuns   = 32
	stateEventBuffer         = 256
	configurationEventBuffer = 64
)

type Coordinator struct {
	state                      *State.Store
	configuration              *Configuration.Store
	gameData                   GameDataProvider
	intents                    IntentSubmitter
	policies                   []Policy
	stateWakeByDomain          map[string][]string
	urgentWakeByDomain         map[string][]string
	configurationWakeBySection map[string][]string
	started                    atomic.Bool
}

type policyRuntime struct {
	nextCheck                     time.Time
	running                       bool
	runtimeWakePending            bool
	stateProgressPending          bool
	configurationWakePending      bool
	configurationRebuildPending   bool
	immediateRuns                 int
	evaluatedStateRevision        uint64
	evaluatedConfigRevision       uint64
	evaluatedConfiguration        string
	evaluatedDerivedConfiguration string
	evaluatedSessionReady         bool
	evaluatedSessionKnown         bool
	evaluatedSessionGeneration    uint64
	lastDecisionFingerprint       string
	rejectRepeatedDecision        bool
	submissionBlockedUntil        time.Time
	blockedDecisionFingerprint    string
	failureBlockedUntil           time.Time
	runningScheduleKey            string
	runningSessionGeneration      uint64
	allowedConfigurationChange    string
	cancelRun                     context.CancelFunc
}

type operationResult struct {
	policyID            string
	receipt             Intent.Receipt
	followUp            *Intent.Receipt
	failureFallback     *Intent.Receipt
	nextCheck           time.Time
	detail              string
	failureDetail       string
	reevaluateOnSuccess bool
	reevaluateOnStale   bool
}

func NewCoordinator(
	state *State.Store,
	configuration *Configuration.Store,
	gameData GameDataProvider,
	intents IntentSubmitter,
	policies ...Policy,
) *Coordinator {
	filtered := make([]Policy, 0, len(policies))
	for _, policy := range policies {
		if policy != nil && strings.TrimSpace(policy.ID()) != "" {
			filtered = append(filtered, policy)
		}
	}
	sort.Slice(filtered, func(left, right int) bool { return filtered[left].ID() < filtered[right].ID() })
	return &Coordinator{
		state: state, configuration: configuration, gameData: gameData, intents: intents, policies: filtered,
		stateWakeByDomain:          indexPolicyWakeDomains(filtered),
		urgentWakeByDomain:         indexPolicyUrgentWakeDomains(filtered),
		configurationWakeBySection: indexPolicyWakeSections(filtered),
	}
}

func (coordinator *Coordinator) PolicyIDs() []string {
	if coordinator == nil {
		return nil
	}
	ids := make([]string, 0, len(coordinator.policies))
	for _, policy := range coordinator.policies {
		ids = append(ids, policy.ID())
	}
	return ids
}

func (coordinator *Coordinator) Run(ctx context.Context) {
	if coordinator == nil || coordinator.state == nil || coordinator.configuration == nil || coordinator.intents == nil {
		return
	}
	if !coordinator.started.CompareAndSwap(false, true) {
		return
	}
	stateEvents, unsubscribeState := coordinator.state.Subscribe(stateEventBuffer)
	defer unsubscribeState()
	configurationEvents, unsubscribeConfiguration := coordinator.configuration.Subscribe(configurationEventBuffer)
	defer unsubscribeConfiguration()
	ticker := time.NewTicker(coordinatorTick)
	defer ticker.Stop()
	results := make(chan operationResult, len(coordinator.policies)*2+1)
	runtime := make(map[string]*policyRuntime, len(coordinator.policies))
	for _, policy := range coordinator.policies {
		runtime[policy.ID()] = &policyRuntime{}
	}
	var expirationTimer *time.Timer
	var expirationChannel <-chan time.Time
	resetExpirationTimer := func() {
		if expirationTimer != nil {
			expirationTimer.Stop()
		}
		expirationTimer = nil
		expirationChannel = nil
		now := time.Now().UTC()
		next := nextAutomationExpiration(coordinator.configuration.Snapshot(), now)
		if next.IsZero() {
			return
		}
		delay := next.Sub(now)
		if delay <= 0 {
			delay = 100 * time.Millisecond
		}
		expirationTimer = time.NewTimer(delay)
		expirationChannel = expirationTimer.C
	}
	defer func() {
		if expirationTimer != nil {
			expirationTimer.Stop()
		}
	}()
	var debounce *time.Timer
	var debounceChannel <-chan time.Time
	evaluate := func() {
		coordinator.evaluate(ctx, runtime, results)
	}
	handleStateEvent := func(event State.Event) (bool, bool) {
		if !meaningfulStateEvent(event) {
			return false, false
		}
		if stateEventHasDomain(event, "session") {
			coordinator.cancelRunsForUnavailableSession(runtime, coordinator.state.ReadOnlyView())
		}
		return wakePoliciesForStateEvent(
			runtime, coordinator.stateWakeByDomain, coordinator.urgentWakeByDomain, event,
		)
	}
	stopDebounce := func() {
		if debounce == nil {
			return
		}
		debounce.Stop()
		debounce = nil
		debounceChannel = nil
	}
	handleConfigurationEvent := func(event Configuration.Event) bool {
		coordinator.cancelDisallowedPolicyRuns(runtime, event, time.Now().UTC())
		return coordinator.wakePoliciesForConfigurationEvent(runtime, event)
	}
	coordinator.expireTimedAutomations(time.Now().UTC())
	resetExpirationTimer()
	evaluate()
	for {
		select {
		case <-ctx.Done():
			if debounce != nil {
				debounce.Stop()
			}
			return
		case event := <-stateEvents:
			woke, urgent := handleStateEvent(event)
			if !woke {
				continue
			}
			if urgent {
				// A declared urgent domain trades coalescing for reaction time:
				// evaluate on the observing event so a short game-side window is
				// not spent waiting out unrelated state churn.
				stopDebounce()
				evaluate()
				continue
			}
			if debounce == nil {
				debounce = time.NewTimer(stateChangeDebounce)
				debounceChannel = debounce.C
			} else {
				if !debounce.Stop() {
					select {
					case <-debounce.C:
					default:
					}
				}
				debounce.Reset(stateChangeDebounce)
			}
		case <-debounceChannel:
			debounce = nil
			debounceChannel = nil
			evaluate()
		case event := <-configurationEvents:
			resetExpirationTimer()
			if handleConfigurationEvent(event) {
				evaluate()
			}
		case <-expirationChannel:
			expirationTimer = nil
			expirationChannel = nil
			coordinator.expireTimedAutomations(time.Now().UTC())
			resetExpirationTimer()
			evaluate()
		case result := <-results:
			current := runtime[result.policyID]
			if current == nil {
				continue
			}
			stateWake := drainStateEvents(stateEvents, stateEventBuffer, func(event State.Event) bool {
				// Completion already reevaluates every woken policy, so a drained
				// urgent domain needs no separate immediate pass.
				woke, _ := handleStateEvent(event)
				return woke
			})
			configurationWake := drainConfigurationEvents(
				configurationEvents,
				configurationEventBuffer,
				handleConfigurationEvent,
			)
			result, wakeImmediately := completePolicyRun(current, result, time.Now().UTC())
			coordinator.recordReceipt(result)
			if wakeImmediately || stateWake || configurationWake {
				evaluate()
			}
		case <-ticker.C:
			evaluate()
		}
	}
}

func (coordinator *Coordinator) evaluate(
	ctx context.Context,
	runtime map[string]*policyRuntime,
	results chan<- operationResult,
) {
	now := time.Now().UTC()
	configuration := coordinator.configuration.Snapshot()
	state := coordinator.state.ReadOnlyView()
	coordinator.cancelRunsDisallowedByConfiguration(runtime, configuration, now)
	coordinator.cancelRunsForUnavailableSession(runtime, state)
	var gameDataStore = coordinator.currentGameData()
	enabled := enabledFeatures(configuration, now)
	sessionReady := automationSessionReady(state.Session)
	for _, policy := range coordinator.policies {
		current := runtime[policy.ID()]
		if current == nil || current.running {
			continue
		}
		isEnabled := enabled[policy.EnabledKey()]
		configurationFingerprint := policyConfigurationFingerprint(policy, configuration)
		derivedConfigurationFingerprint := policyDerivedConfigurationFingerprint(policy, configuration)
		previouslyEvaluated := current.evaluatedSessionKnown
		if previouslyEvaluated && current.evaluatedDerivedConfiguration != derivedConfigurationFingerprint &&
			policyHasConfigurationDerivedState(policy) {
			current.configurationRebuildPending = true
		}
		if current.configurationRebuildPending {
			derived, supportsDerivedState := policy.(ConfigurationDerivedStatePolicy)
			if !supportsDerivedState {
				current.configurationRebuildPending = false
			} else {
				_, err := coordinator.state.ApplyWithoutMapMutation(func(gameState *State.GameState) ([]string, bool, error) {
					domains, changed := derived.ResetConfigurationDerivedState(gameState)
					return domains, changed, nil
				})
				if err != nil {
					current.nextCheck = now.Add(defaultRetry)
					coordinator.recordDecision(policy.ID(), true, Decision{
						Status: "blocked", Detail: "Could not rebuild settings-derived work: " + err.Error(),
						NextCheckAt: current.nextCheck,
					})
					continue
				}
				current.configurationRebuildPending = false
				state = coordinator.state.ReadOnlyView()
				sessionReady = automationSessionReady(state.Session)
			}
		}
		if !isEnabled {
			current.evaluatedStateRevision = state.Revision
			current.evaluatedConfigRevision = configuration.Revision
			current.evaluatedConfiguration = configurationFingerprint
			current.evaluatedDerivedConfiguration = derivedConfigurationFingerprint
			current.evaluatedSessionReady = sessionReady
			current.evaluatedSessionKnown = true
			current.evaluatedSessionGeneration = state.Session.Generation
			resetContinuation(current)
			current.failureBlockedUntil = time.Time{}
			current.nextCheck = time.Time{}
			coordinator.recordDecision(policy.ID(), false, Decision{Status: "disabled", Detail: "Automation is disabled"})
			continue
		}
		configurationChanged := current.evaluatedConfiguration != configurationFingerprint
		sessionChanged := !current.evaluatedSessionKnown || current.evaluatedSessionReady != sessionReady ||
			current.evaluatedSessionGeneration != state.Session.Generation
		if !configurationChanged && !sessionChanged && !current.nextCheck.IsZero() && now.Before(current.nextCheck) {
			continue
		}
		if configurationChanged || sessionChanged {
			resetContinuation(current)
			current.failureBlockedUntil = time.Time{}
		}
		current.evaluatedStateRevision = state.Revision
		current.evaluatedConfigRevision = configuration.Revision
		current.evaluatedConfiguration = configurationFingerprint
		current.evaluatedDerivedConfiguration = derivedConfigurationFingerprint
		current.evaluatedSessionReady = sessionReady
		current.evaluatedSessionKnown = true
		current.evaluatedSessionGeneration = state.Session.Generation
		if !sessionReady {
			resetContinuation(current)
			current.nextCheck = now.Add(defaultRetry)
			detail := "Waiting for a logged-in game websocket"
			if state.Session.SocketReady && state.Session.LoggedIn {
				detail = "Waiting for the current game-session baseline"
			}
			coordinator.recordDecision(policy.ID(), true, Decision{
				Status: "waiting", Detail: detail, NextCheckAt: current.nextCheck,
			})
			continue
		}
		if allowed, next := scheduleAllows(configuration, policyScheduleKey(policy), now); !allowed {
			resetContinuation(current)
			if next.IsZero() {
				next = now.Add(defaultRetry)
			}
			current.nextCheck = next
			coordinator.recordDecision(policy.ID(), true, Decision{
				Status: "scheduled", Detail: "Outside the configured weekly schedule", NextCheckAt: next,
			})
			continue
		}
		if current.failureBlockedUntil.After(now) {
			current.nextCheck = current.failureBlockedUntil
			coordinator.recordDecision(policy.ID(), true, Decision{
				Status: "waiting", Detail: "Safety pause after a failed automation operation", NextCheckAt: current.nextCheck,
			})
			continue
		}
		current.failureBlockedUntil = time.Time{}
		decision, err := policy.Evaluate(ctx, Snapshot{
			State: state, Configuration: configuration, GameData: gameDataStore, Now: now,
			PolicyConfigurationChanged: previouslyEvaluated && configurationChanged,
		})
		if err != nil {
			resetContinuation(current)
			current.nextCheck = now.Add(defaultRetry)
			coordinator.recordDecision(policy.ID(), true, Decision{
				Status: "blocked", Detail: err.Error(), NextCheckAt: current.nextCheck,
			})
			continue
		}
		if decision.NextCheckAt.IsZero() {
			decision.NextCheckAt = now.Add(defaultRetry)
		}
		current.nextCheck = decision.NextCheckAt
		if decision.Request == nil {
			resetContinuation(current)
			coordinator.recordDecision(policy.ID(), true, decision)
			continue
		}
		fingerprint := decisionRequestFingerprint(decision)
		if current.rejectRepeatedDecision && fingerprint == current.lastDecisionFingerprint {
			current.rejectRepeatedDecision = false
			pauseUntil := now.Add(defaultRetry)
			if pauseUntil.After(current.submissionBlockedUntil) {
				current.submissionBlockedUntil = pauseUntil
			}
			current.blockedDecisionFingerprint = fingerprint
		}
		if current.submissionBlockedUntil.After(now) &&
			current.blockedDecisionFingerprint != "" &&
			fingerprint != current.blockedDecisionFingerprint {
			current.submissionBlockedUntil = time.Time{}
			current.blockedDecisionFingerprint = ""
		}
		if current.submissionBlockedUntil.After(now) {
			current.immediateRuns = 0
			current.nextCheck = current.submissionBlockedUntil
			coordinator.recordDecision(policy.ID(), true, Decision{
				Status:      "waiting",
				Detail:      "Safety pause after a continuous command chain: " + decision.Detail,
				NextCheckAt: current.nextCheck,
				Metrics:     decision.Metrics,
			})
			continue
		}
		current.submissionBlockedUntil = time.Time{}
		current.blockedDecisionFingerprint = ""
		current.rejectRepeatedDecision = false
		request := *decision.Request
		request.Actor = "automation:" + policyActorID(policy)
		var followUp *Intent.Request
		if decision.FollowUp != nil {
			copy := *decision.FollowUp
			copy.Actor = "automation:" + policyActorID(policy)
			followUp = &copy
		}
		var failureFallback *Intent.Request
		if decision.FailureFallback != nil {
			copy := *decision.FailureFallback
			copy.Actor = "automation:" + policyActorID(policy)
			failureFallback = &copy
		}
		current.running = true
		current.lastDecisionFingerprint = fingerprint
		current.runningScheduleKey = strings.TrimSpace(decision.ScheduleKey)
		current.runningSessionGeneration = state.Session.Generation
		current.allowedConfigurationChange = configurationFollowUpFingerprint(policy, decision, configuration)
		operationContext, cancelOperation := policyOperationContext(ctx, policy.ID())
		current.cancelRun = cancelOperation
		coordinator.recordDecision(policy.ID(), true, Decision{
			Status: "running", Detail: decision.Detail, NextCheckAt: decision.NextCheckAt, Metrics: decision.Metrics,
		})
		go func(
			policyID string,
			operationContext context.Context,
			cancelOperation context.CancelFunc,
			request Intent.Request,
			followUp *Intent.Request,
			failureFallback *Intent.Request,
			nextCheck time.Time,
			detail string,
			failureDetail string,
			reevaluateOnSuccess bool,
			reevaluateOnStale bool,
		) {
			defer cancelOperation()
			receipt := coordinator.intents.Submit(operationContext, request)
			var followUpReceipt *Intent.Receipt
			var failureFallbackReceipt *Intent.Receipt
			if receipt.Status == Intent.StatusSucceeded && followUp != nil {
				result := coordinator.intents.Submit(operationContext, *followUp)
				followUpReceipt = &result
			} else if failureFallback != nil && statusRunsFailureFallback(receipt.Status) {
				result := coordinator.intents.Submit(operationContext, *failureFallback)
				failureFallbackReceipt = &result
			}
			select {
			case <-ctx.Done():
			case results <- operationResult{
				policyID: policyID, receipt: receipt, followUp: followUpReceipt,
				failureFallback: failureFallbackReceipt,
				nextCheck:       nextCheck, detail: detail, failureDetail: failureDetail,
				reevaluateOnSuccess: reevaluateOnSuccess,
				reevaluateOnStale:   reevaluateOnStale,
			}:
			}
		}(
			policy.ID(), operationContext, cancelOperation, request, followUp, failureFallback,
			decision.NextCheckAt, decision.Detail, decision.FailureDetail, decision.ReevaluateOnSuccess,
			decision.ReevaluateOnStale,
		)
	}
}

func policyOperationContext(ctx context.Context, policyID string) (context.Context, context.CancelFunc) {
	switch strings.TrimSpace(policyID) {
	case "autoTowers", "autoBeriWorldAttack":
		return context.WithTimeout(ctx, boundedAttackIntentTTL)
	default:
		return context.WithCancel(ctx)
	}
}

func (coordinator *Coordinator) currentGameData() *GameData.Store {
	if coordinator.gameData == nil {
		return nil
	}
	store, _ := coordinator.gameData.Current()
	return store
}

func (coordinator *Coordinator) cancelDisallowedPolicyRuns(
	runtime map[string]*policyRuntime,
	event Configuration.Event,
	now time.Time,
) {
	section := strings.TrimSpace(event.Section)
	if coordinator.configuration == nil ||
		!event.Gap && section != "automation.enabled" && section != "scheduler" {
		return
	}
	configuration := coordinator.configuration.Snapshot()
	coordinator.cancelRunsDisallowedByConfiguration(runtime, configuration, now)
}

func (coordinator *Coordinator) wakePoliciesForConfigurationEvent(
	runtime map[string]*policyRuntime,
	event Configuration.Event,
) bool {
	if coordinator.configuration == nil {
		return false
	}
	section := strings.TrimSpace(event.Section)
	candidates := map[string]struct{}{}
	if event.Gap || section == "automation.enabled" || section == "scheduler" {
		for _, policy := range coordinator.policies {
			candidates[policy.ID()] = struct{}{}
		}
	} else {
		for _, policyID := range coordinator.configurationWakeBySection[section] {
			candidates[policyID] = struct{}{}
		}
	}
	configuration := coordinator.configuration.Snapshot()
	wokeIdle := false
	for _, policy := range coordinator.policies {
		if _, candidate := candidates[policy.ID()]; !candidate {
			continue
		}
		current := runtime[policy.ID()]
		latestFingerprint := policyConfigurationFingerprint(policy, configuration)
		if current == nil || event.Revision <= current.evaluatedConfigRevision ||
			current.evaluatedConfiguration == latestFingerprint {
			continue
		}
		if policyConfigurationChangeInvalidatesDerivedState(policy, section, event.Gap) &&
			current.allowedConfigurationChange != latestFingerprint {
			current.configurationRebuildPending = true
		}
		if current.allowedConfigurationChange == latestFingerprint {
			current.evaluatedDerivedConfiguration = policyDerivedConfigurationFingerprint(policy, configuration)
		}
		if current.running {
			current.configurationWakePending = true
			if current.allowedConfigurationChange != latestFingerprint && current.cancelRun != nil {
				current.cancelRun()
			}
			continue
		}
		resetContinuation(current)
		current.nextCheck = time.Time{}
		wokeIdle = true
	}
	return wokeIdle
}

func (coordinator *Coordinator) cancelRunsDisallowedByConfiguration(
	runtime map[string]*policyRuntime,
	configuration Configuration.Snapshot,
	now time.Time,
) {
	enabled := enabledFeatures(configuration, now)
	for _, policy := range coordinator.policies {
		current := runtime[policy.ID()]
		if current == nil || !current.running || current.cancelRun == nil {
			continue
		}
		latestFingerprint := policyConfigurationFingerprint(policy, configuration)
		if current.evaluatedConfiguration != latestFingerprint {
			current.configurationWakePending = true
			if current.evaluatedDerivedConfiguration != policyDerivedConfigurationFingerprint(policy, configuration) &&
				current.allowedConfigurationChange != latestFingerprint {
				current.configurationRebuildPending = true
			}
			if current.allowedConfigurationChange != latestFingerprint {
				current.cancelRun()
				continue
			}
		}
		allowedBySchedule, _ := scheduleAllows(configuration, policyScheduleKey(policy), now)
		if scheduleKey := strings.TrimSpace(current.runningScheduleKey); allowedBySchedule &&
			scheduleKey != "" && scheduleKey != policyScheduleKey(policy) {
			allowedBySchedule, _ = scheduleAllows(configuration, scheduleKey, now)
		}
		if !enabled[policy.EnabledKey()] || !allowedBySchedule {
			current.configurationWakePending = true
			current.cancelRun()
		}
	}
}

func (coordinator *Coordinator) cancelRunsForUnavailableSession(
	runtime map[string]*policyRuntime,
	state State.GameState,
) {
	for _, current := range runtime {
		if current == nil || !current.running || current.cancelRun == nil {
			continue
		}
		if automationSessionReady(state.Session) && current.runningSessionGeneration == state.Session.Generation {
			continue
		}
		current.runtimeWakePending = true
		current.cancelRun()
	}
}

func (coordinator *Coordinator) recordDecision(id string, enabled bool, decision Decision) {
	coordinator.updateAutomation(id, func(current State.AutomationState) State.AutomationState {
		current.ID = id
		current.Enabled = enabled
		current.Status = decision.Status
		if current.Status == "" {
			current.Status = "idle"
		}
		current.Detail = decision.Detail
		current.NextCheckAt = timePointer(decision.NextCheckAt)
		current.Metrics = copyMetrics(decision.Metrics)
		if current.Status != "blocked" {
			current.LastError = ""
		}
		return current
	})
}

func (coordinator *Coordinator) recordReceipt(result operationResult) {
	coordinator.updateAutomation(result.policyID, func(current State.AutomationState) State.AutomationState {
		current.ID = result.policyID
		current.Enabled = true
		current.LastOperationID = result.receipt.ID
		if result.failureFallback != nil {
			current.LastOperationID = result.failureFallback.ID
		}
		current.NextCheckAt = timePointer(result.nextCheck)
		now := time.Now().UTC()
		current.LastRunAt = &now
		if operationResultSucceeded(result) {
			current.Status = "idle"
			current.Detail = result.detail
			if result.failureFallback != nil && strings.TrimSpace(result.failureDetail) != "" {
				current.Detail = result.failureDetail
			}
			current.LastError = ""
		} else {
			current.Status = "error"
			current.Detail = "Automation operation failed"
			current.LastError = result.receipt.Error
			if result.followUp != nil && result.followUp.Status != Intent.StatusSucceeded {
				current.LastError = result.followUp.Error
			}
			if result.failureFallback != nil && result.failureFallback.Status != Intent.StatusSucceeded {
				if current.LastError != "" && result.failureFallback.Error != "" {
					current.LastError += "; failure fallback: " + result.failureFallback.Error
				} else {
					current.LastError = result.failureFallback.Error
				}
			}
		}
		return current
	})
}

func operationResultSucceeded(result operationResult) bool {
	if result.failureFallback != nil {
		return result.failureFallback.Status == Intent.StatusSucceeded
	}
	return result.receipt.Status == Intent.StatusSucceeded &&
		(result.followUp == nil || result.followUp.Status == Intent.StatusSucceeded)
}

func statusRunsFailureFallback(status Intent.Status) bool {
	switch status {
	case Intent.StatusFailed, Intent.StatusPartiallySucceeded, Intent.StatusIndeterminate:
		return true
	default:
		return false
	}
}

func operationResultRetryableStale(result operationResult) bool {
	if !result.reevaluateOnStale ||
		result.receipt.Status != Intent.StatusFailed && result.receipt.Status != Intent.StatusPartiallySucceeded {
		return false
	}
	return strings.Contains(result.receipt.Error, Intent.ErrPlanStale.Error())
}

func completePolicyRun(current *policyRuntime, result operationResult, now time.Time) (operationResult, bool) {
	if current.cancelRun != nil {
		current.cancelRun()
		current.cancelRun = nil
	}
	current.running = false
	current.runningScheduleKey = ""
	current.runningSessionGeneration = 0
	current.allowedConfigurationChange = ""
	runtimeWakePending := current.runtimeWakePending
	stateProgressPending := current.stateProgressPending
	configurationWakePending := current.configurationWakePending
	current.runtimeWakePending = false
	current.stateProgressPending = false
	current.configurationWakePending = false
	succeeded := operationResultSucceeded(result)
	retryableStale := operationResultRetryableStale(result)
	if succeeded || retryableStale {
		current.failureBlockedUntil = time.Time{}
	} else {
		retryAt := result.nextCheck
		minimumRetryAt := now.Add(defaultRetry)
		if retryAt.IsZero() || retryAt.Before(minimumRetryAt) || result.policyID == "autoInvasion" || result.policyID == "autoTowers" {
			retryAt = minimumRetryAt
		}
		current.failureBlockedUntil = retryAt
		result.nextCheck = retryAt
	}
	authoritativeProgress := runtimeWakePending || stateProgressPending || configurationWakePending
	if succeeded && result.reevaluateOnSuccess && !authoritativeProgress {
		current.rejectRepeatedDecision = true
	}
	immediate := runtimeWakePending || configurationWakePending || succeeded && result.reevaluateOnSuccess || retryableStale
	if !immediate {
		resetContinuation(current)
		current.nextCheck = result.nextCheck
		return result, false
	}

	if authoritativeProgress {
		resetContinuation(current)
	} else {
		current.immediateRuns++
	}
	result.nextCheck = time.Time{}
	if !authoritativeProgress && current.immediateRuns >= maxImmediatePolicyRuns {
		current.immediateRuns = 0
		current.rejectRepeatedDecision = false
		current.submissionBlockedUntil = now.Add(defaultRetry)
		current.blockedDecisionFingerprint = ""
	}
	current.nextCheck = result.nextCheck
	return result, true
}

func resetContinuation(current *policyRuntime) {
	current.immediateRuns = 0
	current.lastDecisionFingerprint = ""
	current.rejectRepeatedDecision = false
	current.submissionBlockedUntil = time.Time{}
	current.blockedDecisionFingerprint = ""
}

func (coordinator *Coordinator) updateAutomation(id string, update func(State.AutomationState) State.AutomationState) {
	_, _ = coordinator.state.ApplyWithoutMapMutation(func(gameState *State.GameState) ([]string, bool, error) {
		if gameState.Automations == nil {
			gameState.Automations = map[string]State.AutomationState{}
		}
		current := gameState.Automations[id]
		next := update(current)
		next.UpdatedAt = current.UpdatedAt
		if reflect.DeepEqual(current, next) {
			return nil, false, nil
		}
		next.UpdatedAt = time.Now().UTC()
		gameState.Automations[id] = next
		return []string{"automation"}, true, nil
	})
}

func automationSessionReady(session State.SessionState) bool {
	return session.SocketReady && session.LoggedIn && session.Generation > 0 &&
		session.BaselineGeneration == session.Generation
}

func meaningfulStateEvent(event State.Event) bool {
	for _, domain := range event.Domains {
		normalized := strings.ToLower(strings.TrimSpace(domain))
		if normalized != "protocol" && normalized != "automation" {
			return true
		}
	}
	return false
}

func stateEventHasDomain(event State.Event, wanted string) bool {
	wanted = strings.ToLower(strings.TrimSpace(wanted))
	for _, domain := range event.Domains {
		if strings.ToLower(strings.TrimSpace(domain)) == wanted {
			return true
		}
	}
	return false
}

func indexPolicyWakeDomains(policies []Policy) map[string][]string {
	indexed := map[string][]string{}
	for _, policy := range policies {
		declared, ok := policy.(StateWakePolicy)
		if !ok {
			continue
		}
		seen := map[string]struct{}{}
		for _, value := range declared.WakeDomains() {
			domain := strings.ToLower(strings.TrimSpace(value))
			if domain == "" || domain == "protocol" || domain == "automation" || domain == "session" {
				continue
			}
			if _, duplicate := seen[domain]; duplicate {
				continue
			}
			seen[domain] = struct{}{}
			indexed[domain] = append(indexed[domain], policy.ID())
		}
	}
	for domain := range indexed {
		sort.Strings(indexed[domain])
	}
	return indexed
}

// indexPolicyUrgentWakeDomains keeps only urgent domains the policy already
// wakes on, so an urgent list that drifts away from WakeDomains cannot quietly
// promote an event the policy never observes.
func indexPolicyUrgentWakeDomains(policies []Policy) map[string][]string {
	indexed := map[string][]string{}
	for _, policy := range policies {
		declared, ok := policy.(UrgentWakePolicy)
		if !ok {
			continue
		}
		woken := map[string]struct{}{}
		if wake, supportsWakeDomains := policy.(StateWakePolicy); supportsWakeDomains {
			for _, value := range wake.WakeDomains() {
				woken[strings.ToLower(strings.TrimSpace(value))] = struct{}{}
			}
		}
		seen := map[string]struct{}{}
		for _, value := range declared.UrgentWakeDomains() {
			domain := strings.ToLower(strings.TrimSpace(value))
			if domain == "" || domain == "protocol" || domain == "automation" || domain == "session" {
				continue
			}
			if _, wakes := woken[domain]; !wakes {
				continue
			}
			if _, duplicate := seen[domain]; duplicate {
				continue
			}
			seen[domain] = struct{}{}
			indexed[domain] = append(indexed[domain], policy.ID())
		}
	}
	for domain := range indexed {
		sort.Strings(indexed[domain])
	}
	return indexed
}

func indexPolicyWakeSections(policies []Policy) map[string][]string {
	indexed := map[string][]string{}
	for _, policy := range policies {
		declared, ok := policy.(ConfigurationWakePolicy)
		if !ok {
			continue
		}
		seen := map[string]struct{}{}
		for _, value := range declared.WakeSections() {
			section := strings.TrimSpace(value)
			if section == "" || section == "automation.enabled" || section == "scheduler" {
				continue
			}
			if _, duplicate := seen[section]; duplicate {
				continue
			}
			seen[section] = struct{}{}
			indexed[section] = append(indexed[section], policy.ID())
		}
	}
	for section := range indexed {
		sort.Strings(indexed[section])
	}
	return indexed
}

func policyConfigurationChangeInvalidatesDerivedState(policy Policy, section string, gap bool) bool {
	declared, ok := policy.(ConfigurationDerivedStatePolicy)
	if !ok {
		return false
	}
	section = strings.TrimSpace(section)
	for _, value := range declared.ConfigurationDerivedStateSections() {
		derivedSection := strings.TrimSpace(value)
		if derivedSection != "" && (gap || derivedSection == section) {
			return true
		}
	}
	return false
}

func policyHasConfigurationDerivedState(policy Policy) bool {
	return policyConfigurationChangeInvalidatesDerivedState(policy, "", true)
}

func policyDerivedConfigurationFingerprint(policy Policy, configuration Configuration.Snapshot) string {
	declared, ok := policy.(ConfigurationDerivedStatePolicy)
	if !ok {
		return ""
	}
	sections := map[string]struct{}{}
	for _, value := range declared.ConfigurationDerivedStateSections() {
		if section := strings.TrimSpace(value); section != "" {
			sections[section] = struct{}{}
		}
	}
	keys := make([]string, 0, len(sections))
	for section := range sections {
		keys = append(keys, section)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys)*2)
	for _, section := range keys {
		parts = append(parts, section, string(configuration.Sections[section]))
	}
	return strings.Join(parts, "\x00")
}

func policyConfigurationFingerprint(policy Policy, configuration Configuration.Snapshot) string {
	enabled := configuredEnabledFeatures(configuration)[policy.EnabledKey()]
	parts := []string{"enabled", "false", "schedule", policyScheduleConfiguration(policyScheduleKey(policy), configuration.Sections["scheduler"])}
	if enabled {
		parts[1] = "true"
	}
	sections := map[string]struct{}{}
	if declared, ok := policy.(ConfigurationWakePolicy); ok {
		for _, value := range declared.WakeSections() {
			if section := strings.TrimSpace(value); section != "" && section != "automation.enabled" && section != "scheduler" {
				sections[section] = struct{}{}
			}
		}
	}
	keys := make([]string, 0, len(sections))
	for section := range sections {
		keys = append(keys, section)
	}
	sort.Strings(keys)
	for _, section := range keys {
		parts = append(parts, section, string(configuration.Sections[section]))
	}
	return strings.Join(parts, "\x00")
}

func policyScheduleKey(policy Policy) string {
	if declared, ok := policy.(ScheduleKeyPolicy); ok {
		if key := strings.TrimSpace(declared.ScheduleKey()); key != "" {
			return key
		}
	}
	return policy.ID()
}

func policyActorID(policy Policy) string {
	if declared, ok := policy.(ActorIDPolicy); ok {
		if id := strings.TrimSpace(declared.ActorID()); id != "" {
			return id
		}
	}
	return policy.ID()
}

func configurationFollowUpFingerprint(
	policy Policy,
	decision Decision,
	configuration Configuration.Snapshot,
) string {
	if decision.FollowUp == nil || strings.TrimSpace(decision.FollowUp.Name) != "config.update" {
		return ""
	}
	var update struct {
		Section string          `json:"section"`
		Value   json.RawMessage `json:"value"`
	}
	if json.Unmarshal(decision.FollowUp.Arguments, &update) != nil || strings.TrimSpace(update.Section) == "" || len(update.Value) == 0 {
		return ""
	}
	var compact bytes.Buffer
	if json.Compact(&compact, update.Value) != nil {
		return ""
	}
	projected := configuration
	projected.Sections = make(map[string]json.RawMessage, len(configuration.Sections)+1)
	for section, value := range configuration.Sections {
		projected.Sections[section] = value
	}
	projected.Sections[strings.TrimSpace(update.Section)] = append(json.RawMessage(nil), compact.Bytes()...)
	return policyConfigurationFingerprint(policy, projected)
}

func policyScheduleConfiguration(policyID string, raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var document struct {
		FeatureSchedules map[string]json.RawMessage `json:"featureSchedules"`
	}
	if err := json.Unmarshal(raw, &document); err != nil {
		return "invalid\x00" + string(raw)
	}
	keys := make([]string, 0)
	prefix := policyID + ":"
	for key := range document.FeatureSchedules {
		if key == policyID || strings.HasPrefix(key, prefix) {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys)*2)
	for _, key := range keys {
		parts = append(parts, key, string(document.FeatureSchedules[key]))
	}
	return strings.Join(parts, "\x00")
}

// wakePoliciesForStateEvent reports whether the event woke an idle policy, and
// whether any of those wakes came from a domain that policy declared urgent.
func wakePoliciesForStateEvent(
	runtime map[string]*policyRuntime,
	indexed map[string][]string,
	urgent map[string][]string,
	event State.Event,
) (bool, bool) {
	wokeIdle := false
	wokeUrgently := false
	wake := func(current *policyRuntime, session bool) bool {
		if current == nil || event.Revision <= current.evaluatedStateRevision {
			return false
		}
		if current.running {
			if session {
				current.runtimeWakePending = true
			} else {
				current.stateProgressPending = true
			}
			return false
		}
		resetContinuation(current)
		current.nextCheck = time.Time{}
		wokeIdle = true
		return true
	}
	for _, value := range event.Domains {
		domain := strings.ToLower(strings.TrimSpace(value))
		if domain == "session" {
			for _, current := range runtime {
				wake(current, true)
			}
			return wokeIdle, false
		}
		for _, policyID := range indexed[domain] {
			// A running policy records relevant response progress but only an
			// explicit completion continuation may override deliberate pacing.
			if wake(runtime[policyID], false) && slices.Contains(urgent[domain], policyID) {
				wokeUrgently = true
			}
		}
	}
	return wokeIdle, wokeUrgently
}

func wakePoliciesForConfigurationEvent(
	runtime map[string]*policyRuntime,
	indexed map[string][]string,
	event Configuration.Event,
) bool {
	section := strings.TrimSpace(event.Section)
	return wakePolicyIDsForConfigurationEvent(runtime, indexed[section], event)
}

func wakePolicyIDsForConfigurationEvent(
	runtime map[string]*policyRuntime,
	policyIDs []string,
	event Configuration.Event,
) bool {
	wokeIdle := false
	for _, policyID := range policyIDs {
		current := runtime[policyID]
		if current == nil || event.Revision <= current.evaluatedConfigRevision {
			continue
		}
		if current.running {
			current.configurationWakePending = true
			continue
		}
		resetContinuation(current)
		current.nextCheck = time.Time{}
		wokeIdle = true
	}
	return wokeIdle
}

func drainStateEvents(events <-chan State.Event, limit int, handle func(State.Event) bool) bool {
	wokeIdle := false
	for index := 0; index < limit; index++ {
		select {
		case event, open := <-events:
			if !open {
				return wokeIdle
			}
			wokeIdle = handle(event) || wokeIdle
		default:
			return wokeIdle
		}
	}
	return wokeIdle
}

func drainConfigurationEvents(
	events <-chan Configuration.Event,
	limit int,
	handle func(Configuration.Event) bool,
) bool {
	wokeIdle := false
	for index := 0; index < limit; index++ {
		select {
		case event, open := <-events:
			if !open {
				return wokeIdle
			}
			wokeIdle = handle(event) || wokeIdle
		default:
			return wokeIdle
		}
	}
	return wokeIdle
}

func decisionRequestFingerprint(decision Decision) string {
	if decision.Request == nil {
		return ""
	}
	parts := []string{decision.Request.Name, string(decision.Request.Arguments), decision.ScheduleKey, "no-follow-up"}
	if decision.FollowUp != nil {
		parts[3] = "follow-up"
		parts = append(parts, decision.FollowUp.Name, string(decision.FollowUp.Arguments))
	}
	parts = append(parts, "no-failure-fallback")
	if decision.FailureFallback != nil {
		parts[len(parts)-1] = "failure-fallback"
		parts = append(parts, decision.FailureFallback.Name, string(decision.FailureFallback.Arguments))
	}
	return strings.Join(parts, "\x00")
}

func timePointer(value time.Time) *time.Time {
	if value.IsZero() {
		return nil
	}
	copy := value
	return &copy
}

func copyMetrics(source map[string]float64) map[string]float64 {
	if len(source) == 0 {
		return map[string]float64{}
	}
	clone := make(map[string]float64, len(source))
	for key, value := range source {
		clone[key] = value
	}
	return clone
}
