package Intent

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"sync"
	"time"

	"CitadelDesktop/Server/Buildings"
	"CitadelDesktop/Server/GameData"
	"CitadelDesktop/Server/State"
	"encoding/json"
)

// This policy is deliberately independent of error text/ExpectedState. A known
// meaning does not authorize automatic recovery. Add only reviewed opcode/code
// pairs in the shared State policy; absence is a 30-minute originating-lane lock.
func rejectionAllowsRecovery(opcode string, code int) bool {
	return State.AutomationRejectionWhitelisted(opcode, code)
}

type laneSafetyContextKey struct{}

type laneSafety struct {
	mu      sync.Mutex
	persist func(context.Context, State.Event) error
}

type LaneLockedError struct {
	Lock  State.AutomationSafetyLock
	Cause error
}

func (err *LaneLockedError) Error() string { return err.Lock.Detail() }
func (err *LaneLockedError) Unwrap() error { return err.Cause }

// SetLaneSafetyPersistence is called during application composition. Locks are
// part of the durable account profile, not the transient policy scheduler.
func (engine *Engine) SetLaneSafetyPersistence(persist func(context.Context, State.Event) error) {
	engine.laneSafety.persist = persist
}

// RefreshAutomationLaneLocks migrates saved incidents before automation starts.
// Preserve the rejection receipt and original timer. Persist failure aborts startup.
func (engine *Engine) RefreshAutomationLaneLocks() error {
	engine.laneSafety.mu.Lock()
	defer engine.laneSafety.mu.Unlock()
	store, ok := engine.state.(interface {
		ApplyComponents(State.ComponentSet, State.Mutation) (State.Event, error)
	})
	if !ok || engine.laneSafety.persist == nil {
		return fmt.Errorf("automation safety refresh requires durable state")
	}
	now := time.Now().UTC()
	event, err := store.ApplyComponents(State.Components(State.ComponentAutomations), func(state *State.GameState) ([]string, bool, error) {
		changed := false
		for lane, current := range state.Automations {
			lock := current.SafetyLock
			if lock.OperationID == "" || !lock.ClearedAt.IsZero() {
				continue
			}
			if lock.ObservedAt.IsZero() {
				if !lock.Until.IsZero() {
					lock.ObservedAt = lock.Until.Add(-State.AutomationSafetyLockDuration)
				} else {
					// No trustworthy original time: start one durable bounded timer.
					lock.ObservedAt = now
				}
			}
			meaning := engine.unsuccessfulResponseCode(lock.Opcode, lock.Code).(*ResponseCodeError).Meaning
			if meaning.Source != GameData.ResponseCodeUnknown && (lock.Meaning == "" || meaning.Source == GameData.ResponseCodeOfficial) {
				lock.Meaning, lock.MeaningSource = meaning.Message, string(meaning.Source)
			}
			lock.Until = lock.ExpiresAt()
			if State.AutomationRejectionWhitelisted(lock.Opcode, lock.Code) {
				lock.ClearedAt, lock.ReviewedBy, lock.Review = now, "policy:whitelist", "Automatically exempted by the exact opcode/code whitelist."
			} else if !now.Before(lock.Until) {
				lock.ClearedAt, lock.ReviewedBy, lock.Review = lock.Until, "policy:expiry", "Original rejection is at least 30 minutes old."
			}
			next := current
			next.SafetyLock = lock
			if lock.Active(now) {
				next.Status, next.Detail, next.LastError = "gated", lock.Detail(), lock.Detail()
				next.NextCheckAt = &lock.Until
			} else if current.Status == "gated" && strings.HasPrefix(current.Detail, "Safety lock after ") {
				next.Status, next.Detail, next.LastError = "waiting", "Safety lock released by policy; waiting for normal prerequisites", ""
				next.NextCheckAt = nil
			}
			if !reflect.DeepEqual(current, next) {
				next.UpdatedAt = now
				state.Automations[lane] = next
				changed = true
			}
		}
		return []string{"automation-safety"}, changed, nil
	})
	if err != nil || event.Patch == nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := engine.laneSafety.persist(ctx, event); err != nil {
		engine.recordPersistenceFailure(err)
		return fmt.Errorf("persist automation safety refresh: %w", err)
	}
	return nil
}

func requestLane(request Request) string {
	if !strings.HasPrefix(strings.TrimSpace(request.Actor), "automation:") {
		return ""
	}
	return strings.TrimSpace(request.AutomationLane)
}

func (engine *Engine) AutomationLaneLock(lane string) State.AutomationSafetyLock {
	if engine.state == nil || lane == "" {
		return State.AutomationSafetyLock{}
	}
	return engine.state.ReadOnlyView().Automations[lane].SafetyLock
}

func (engine *Engine) checkLaneSafety(request Request) error {
	// Actor names can identify a whole feature. Never guess a lane from them.
	if strings.HasPrefix(strings.TrimSpace(request.Actor), "automation:") && requestLane(request) == "" {
		return fmt.Errorf("automation request requires an explicit originating lane")
	}
	engine.laneSafety.mu.Lock()
	defer engine.laneSafety.mu.Unlock()
	lock := engine.AutomationLaneLock(requestLane(request))
	if lock.Active(time.Now().UTC()) {
		return &LaneLockedError{Lock: lock}
	}
	return nil
}

// guardRejection runs before retry, stale-plan handling, or compensating actions.
// Cancellation must not erase the evidence or prevent persisting the lock.
func (engine *Engine) guardRejection(ctx context.Context, err error) error {
	request, _ := ctx.Value(laneSafetyContextKey{}).(Request)
	lane := requestLane(request)
	var response *ResponseCodeError
	var locked *LaneLockedError
	if lane == "" || errors.As(err, &locked) || !errors.As(err, &response) || response.Meaning.Code == 0 || rejectionAllowsRecovery(response.Opcode, response.Meaning.Code) {
		return err
	}
	engine.laneSafety.mu.Lock()
	defer engine.laneSafety.mu.Unlock()
	if lock := engine.AutomationLaneLock(lane); lock.Active(time.Now().UTC()) {
		return &LaneLockedError{Lock: lock}
	}
	lock := State.AutomationSafetyLock{Lane: lane, Opcode: strings.ToLower(strings.TrimSpace(response.Opcode)), Code: response.Meaning.Code, OperationID: request.ID, Intent: request.Name, ObservedAt: time.Now().UTC(), Reason: "unclassified_rejection"}
	if response.Meaning.Source != GameData.ResponseCodeUnknown {
		lock.Meaning, lock.MeaningSource = response.Meaning.Message, string(response.Meaning.Source)
	}
	lock.Context = engine.rubyRejectionContext(request, lock)
	lock.Until = lock.ObservedAt.Add(State.AutomationSafetyLockDuration)
	// Classification is diagnostic; every non-whitelisted rejection has one TTL.
	if lock.Opcode == "msd" {
		lock.Reason = "msd_rejection"
	} else if lock.Opcode == "cra" && lock.Code == 256 {
		lock.Reason = "hazardous_rejection"
	}
	if saveErr := engine.writeLaneLock(lock); saveErr != nil {
		engine.recordPersistenceFailure(saveErr)
		return &LaneLockedError{Lock: lock, Cause: errors.Join(response, saveErr)}
	}
	return &LaneLockedError{Lock: lock, Cause: response}
}

func (engine *Engine) writeLaneLock(lock State.AutomationSafetyLock) error {
	store, ok := engine.state.(interface {
		ApplyComponents(State.ComponentSet, State.Mutation) (State.Event, error)
	})
	if !ok {
		return fmt.Errorf("automation safety state cannot be persisted")
	}
	event, err := store.ApplyComponents(State.Components(State.ComponentAutomations), func(state *State.GameState) ([]string, bool, error) {
		if state.Automations == nil {
			state.Automations = map[string]State.AutomationState{}
		}
		current := state.Automations[lock.Lane]
		current.ID = lock.Lane
		current.SafetyLock = lock
		current.UpdatedAt = time.Now().UTC()
		if lock.Active(current.UpdatedAt) {
			current.Status = "gated"
			current.Detail = lock.Detail()
			current.LastError = current.Detail
			current.LastOperationID = lock.OperationID
			current.NextCheckAt = nil
		} else {
			current.Status = "waiting"
			current.Detail = "Safety lock reviewed and cleared"
			current.LastError = ""
		}
		state.Automations[lock.Lane] = current
		return []string{"automation-safety"}, true, nil
	})
	if err != nil {
		return err
	}
	if engine.laneSafety.persist == nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return engine.laneSafety.persist(ctx, event)
}

// ClearAutomationLaneLock is an explicit reviewed action, conditional on the
// triggering receipt so a stale screen cannot clear a newer incident.
func (engine *Engine) ClearAutomationLaneLock(lane, operationID, review, actor string) error {
	if strings.HasPrefix(strings.ToLower(strings.TrimSpace(actor)), "automation:") || strings.TrimSpace(review) == "" || len(review) > 1000 {
		return fmt.Errorf("a manual safety review of at most 1000 characters is required")
	}
	engine.laneSafety.mu.Lock()
	defer engine.laneSafety.mu.Unlock()
	lock := engine.AutomationLaneLock(strings.TrimSpace(lane))
	if lock.OperationID == "" || lock.OperationID != strings.TrimSpace(operationID) || !lock.ClearedAt.IsZero() {
		return fmt.Errorf("the safety incident changed; refresh before reviewing it")
	}
	if lock.Active(time.Now().UTC()) {
		return fmt.Errorf("the mandatory 30-minute lane cooldown must expire before this lock can be cleared")
	}
	original := lock
	lock.ClearedAt = time.Now().UTC()
	lock.Review = strings.TrimSpace(review)
	lock.ReviewedBy = actor
	if err := engine.writeLaneLock(lock); err != nil {
		_ = engine.writeLaneLock(original)
		engine.recordPersistenceFailure(err)
		return err
	}
	return nil
}

// Context comes from the upgrade target and current game setting. EUP.CC2T is
// a quoted purchase price and must never be treated as the confirmation setting.
func (engine *Engine) rubyRejectionContext(request Request, lock State.AutomationSafetyLock) string {
	if lock.Opcode != "eup" || lock.Code != 440 || request.Name != "building.upgrade" || engine.gameData == nil {
		return ""
	}
	var args struct {
		CastleID   State.CastleID           `json:"castleId"`
		BuildingID State.BuildingInstanceID `json:"buildingInstanceId"`
	}
	if json.Unmarshal(request.Arguments, &args) != nil {
		return ""
	}
	state := engine.state.ReadOnlyView()
	setting := state.Player.RubyConfirmation
	if !setting.Current(state.Session) {
		return ""
	}
	data, ok := engine.gameData.Current()
	if !ok || data == nil {
		return ""
	}
	catalog, err := data.BuildingCatalog()
	if err != nil {
		return ""
	}
	castle := state.Castles[args.CastleID]
	building, ok := castle.Layout.Objects[args.BuildingID]
	if !ok {
		building, ok = castle.Layout.Fixed[args.BuildingID]
	}
	if !ok {
		return ""
	}
	current, ok := catalog.Definition(int64(building.DefinitionID))
	if !ok {
		return ""
	}
	next, ok := catalog.Definition(current.UpgradeDefinitionID)
	if !ok {
		return ""
	}
	var costs []Buildings.CostStatus
	for _, cost := range next.Costs {
		costs = append(costs, Buildings.CostStatus{Required: cost.Amount, Premium: cost.Premium})
	}
	if blocker := Buildings.RubyUpgradeBlocker(state, costs); blocker != nil && blocker.Code == "ruby_confirmation_required" {
		return blocker.Message
	}
	return ""
}
