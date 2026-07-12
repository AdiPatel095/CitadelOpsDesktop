package Automation

import (
	"context"
	"encoding/json"
	"reflect"
	"sort"
	"strings"
	"time"

	"CitadelDesktop/Server/Configuration"
	"CitadelDesktop/Server/GameData"
	"CitadelDesktop/Server/Intent"
	"CitadelDesktop/Server/State"
)

const (
	coordinatorTick     = 2 * time.Second
	stateChangeDebounce = 250 * time.Millisecond
	defaultRetry        = 30 * time.Second
)

type Coordinator struct {
	state         *State.Store
	configuration *Configuration.Store
	gameData      GameDataProvider
	intents       IntentSubmitter
	policies      []Policy
}

type policyRuntime struct {
	nextCheck time.Time
	running   bool
}

type operationResult struct {
	policyID  string
	receipt   Intent.Receipt
	followUp  *Intent.Receipt
	nextCheck time.Time
	detail    string
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
	stateEvents, unsubscribeState := coordinator.state.Subscribe(256)
	defer unsubscribeState()
	configurationEvents, unsubscribeConfiguration := coordinator.configuration.Subscribe(32)
	defer unsubscribeConfiguration()
	ticker := time.NewTicker(coordinatorTick)
	defer ticker.Stop()
	results := make(chan operationResult, len(coordinator.policies)*2+1)
	runtime := make(map[string]*policyRuntime, len(coordinator.policies))
	for _, policy := range coordinator.policies {
		runtime[policy.ID()] = &policyRuntime{}
	}
	var debounce *time.Timer
	var debounceChannel <-chan time.Time
	evaluate := func(force bool) {
		coordinator.evaluate(ctx, runtime, results, force)
	}
	evaluate(true)
	for {
		select {
		case <-ctx.Done():
			if debounce != nil {
				debounce.Stop()
			}
			return
		case event := <-stateEvents:
			if !meaningfulStateEvent(event) {
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
			evaluate(false)
		case <-configurationEvents:
			for _, current := range runtime {
				if !current.running {
					current.nextCheck = time.Time{}
				}
			}
			evaluate(true)
		case result := <-results:
			current := runtime[result.policyID]
			if current == nil {
				continue
			}
			current.running = false
			current.nextCheck = result.nextCheck
			coordinator.recordReceipt(result)
		case <-ticker.C:
			evaluate(false)
		}
	}
}

func (coordinator *Coordinator) evaluate(
	ctx context.Context,
	runtime map[string]*policyRuntime,
	results chan<- operationResult,
	force bool,
) {
	now := time.Now().UTC()
	configuration := coordinator.configuration.Snapshot()
	state := coordinator.state.Snapshot()
	var gameDataStore = coordinator.currentGameData()
	enabled := enabledFeatures(configuration)
	for _, policy := range coordinator.policies {
		current := runtime[policy.ID()]
		if current == nil || current.running {
			continue
		}
		isEnabled := enabled[policy.EnabledKey()]
		if !isEnabled {
			current.nextCheck = time.Time{}
			coordinator.recordDecision(policy.ID(), false, Decision{Status: "disabled", Detail: "Automation is disabled"})
			continue
		}
		if !force && !current.nextCheck.IsZero() && now.Before(current.nextCheck) {
			continue
		}
		if !state.Session.SocketReady || !state.Session.LoggedIn {
			current.nextCheck = now.Add(defaultRetry)
			coordinator.recordDecision(policy.ID(), true, Decision{
				Status: "waiting", Detail: "Waiting for a logged-in game websocket", NextCheckAt: current.nextCheck,
			})
			continue
		}
		if allowed, next := scheduleAllows(configuration, policy.ID(), now); !allowed {
			if next.IsZero() {
				next = now.Add(defaultRetry)
			}
			current.nextCheck = next
			coordinator.recordDecision(policy.ID(), true, Decision{
				Status: "scheduled", Detail: "Outside the configured weekly schedule", NextCheckAt: next,
			})
			continue
		}
		decision, err := policy.Evaluate(ctx, Snapshot{
			State: state, Configuration: configuration, GameData: gameDataStore, Now: now,
		})
		if err != nil {
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
			coordinator.recordDecision(policy.ID(), true, decision)
			continue
		}
		request := *decision.Request
		request.Actor = "automation:" + policy.ID()
		var followUp *Intent.Request
		if decision.FollowUp != nil {
			copy := *decision.FollowUp
			copy.Actor = "automation:" + policy.ID()
			followUp = &copy
		}
		current.running = true
		coordinator.recordDecision(policy.ID(), true, Decision{
			Status: "running", Detail: decision.Detail, NextCheckAt: decision.NextCheckAt, Metrics: decision.Metrics,
		})
		go func(policyID string, request Intent.Request, followUp *Intent.Request, nextCheck time.Time, detail string) {
			receipt := coordinator.intents.Submit(ctx, request)
			var followUpReceipt *Intent.Receipt
			if receipt.Status == Intent.StatusSucceeded && followUp != nil {
				result := coordinator.intents.Submit(ctx, *followUp)
				followUpReceipt = &result
			}
			select {
			case <-ctx.Done():
			case results <- operationResult{
				policyID: policyID, receipt: receipt, followUp: followUpReceipt,
				nextCheck: nextCheck, detail: detail,
			}:
			}
		}(policy.ID(), request, followUp, decision.NextCheckAt, decision.Detail)
	}
}

func (coordinator *Coordinator) currentGameData() *GameData.Store {
	if coordinator.gameData == nil {
		return nil
	}
	store, _ := coordinator.gameData.Current()
	return store
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
		current.NextCheckAt = timePointer(result.nextCheck)
		now := time.Now().UTC()
		current.LastRunAt = &now
		if result.receipt.Status == Intent.StatusSucceeded &&
			(result.followUp == nil || result.followUp.Status == Intent.StatusSucceeded) {
			current.Status = "idle"
			current.Detail = result.detail
			current.LastError = ""
		} else {
			current.Status = "error"
			current.Detail = "Automation operation failed"
			current.LastError = result.receipt.Error
			if result.followUp != nil && result.followUp.Status != Intent.StatusSucceeded {
				current.LastError = result.followUp.Error
			}
		}
		return current
	})
}

func (coordinator *Coordinator) updateAutomation(id string, update func(State.AutomationState) State.AutomationState) {
	_, _ = coordinator.state.Apply(func(gameState *State.GameState) ([]string, bool, error) {
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

func enabledFeatures(snapshot Configuration.Snapshot) map[string]bool {
	result := map[string]bool{}
	raw := snapshot.Sections["automation.enabled"]
	_ = json.Unmarshal(raw, &result)
	return result
}

func meaningfulStateEvent(event State.Event) bool {
	for _, domain := range event.Domains {
		if domain != "protocol" && domain != "automation" {
			return true
		}
	}
	return false
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
