package Intent

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"CitadelDesktop/Server/State"
)

// This policy is deliberately independent of error text/ExpectedState. A known
// meaning does not authorize automatic recovery. Add only reviewed opcode/code
// pairs here; absence is a persistent lock, including for newly added commands.
func rejectionAllowsRecovery(opcode string, code int) bool {
	switch strings.ToLower(strings.TrimSpace(opcode)) {
	case "adi":
		return code == 95
	case "ere", "eqe":
		return code == 227
	case "bup":
		return code == 87
	default:
		return false
	}
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
	// Preserve the emergency MSD policy. Other unknown rejections and CRA 256
	// require review; no timer, setting change, or reconnect clears them.
	if lock.Opcode == "msd" {
		lock.Reason = "msd_rejection"
		lock.Until = lock.ObservedAt.Add(30 * time.Minute)
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
	if !lock.Until.IsZero() && time.Now().Before(lock.Until) {
		return fmt.Errorf("the mandatory MSD cooldown must expire before this lock can be cleared")
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
