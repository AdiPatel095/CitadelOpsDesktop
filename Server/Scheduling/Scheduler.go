package Scheduling

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"CitadelDesktop/Server/Intent"
	"CitadelDesktop/Server/State"
)

type Request struct {
	ID        string          `json:"id"`
	Intent    string          `json:"intent"`
	Actor     string          `json:"actor,omitempty"`
	Arguments json.RawMessage `json:"arguments"`
	ExecuteAt time.Time       `json:"executeAt"`
}

type IntentSubmitter interface {
	Submit(context.Context, Intent.Request) Intent.Receipt
}

type IntentCanceller interface {
	Cancel(string) bool
}

type runningExecution struct {
	version     uint64
	operationID string
	cancel      context.CancelFunc
}

type Scheduler struct {
	state   *State.Store
	intents IntentSubmitter

	mu      sync.Mutex
	running map[string]runningExecution
	started atomic.Bool
}

func NewScheduler(state *State.Store, intents IntentSubmitter) *Scheduler {
	return &Scheduler{state: state, intents: intents, running: map[string]runningExecution{}}
}

func (scheduler *Scheduler) Schedule(request Request) error {
	request.ID = strings.TrimSpace(request.ID)
	request.Intent = strings.TrimSpace(request.Intent)
	if scheduler == nil || scheduler.state == nil {
		return fmt.Errorf("operation scheduler is unavailable")
	}
	if request.ID == "" || request.Intent == "" || request.ExecuteAt.IsZero() {
		return fmt.Errorf("scheduled operation id, intent, and executeAt are required")
	}
	request.Actor = "scheduler:" + request.Intent
	if len(request.Arguments) == 0 {
		request.Arguments = json.RawMessage(`{}`)
	}
	if !json.Valid(request.Arguments) {
		return fmt.Errorf("scheduled operation arguments are not valid JSON")
	}
	var previousVersion uint64
	var previousExists bool
	var changed bool
	_, err := scheduler.state.ApplyWithoutMapMutation(func(gameState *State.GameState) ([]string, bool, error) {
		now := time.Now().UTC()
		current, exists := gameState.Scheduled[request.ID]
		worldID, playerID := State.BoundAccount(*gameState)
		if exists && (current.Status == "scheduled" || current.Status == "running") &&
			current.Intent == request.Intent && current.ExecuteAt.Equal(request.ExecuteAt.UTC()) &&
			bytes.Equal(current.Arguments, request.Arguments) &&
			strings.EqualFold(strings.TrimSpace(current.WorldID), worldID) && current.PlayerID == playerID {
			return nil, false, nil
		}
		previousVersion = current.Version
		previousExists = exists
		version := current.Version + 1
		if !exists {
			version = 1
		}
		next := State.ScheduledOperation{
			ID: request.ID, Version: version, Intent: request.Intent, Actor: request.Actor,
			Arguments: append([]byte(nil), request.Arguments...), ExecuteAt: request.ExecuteAt.UTC(),
			WorldID: worldID, PlayerID: playerID, CreatedAt: current.CreatedAt, UpdatedAt: now, Status: "scheduled",
		}
		if next.CreatedAt.IsZero() {
			next.CreatedAt = now
		}
		gameState.Scheduled[request.ID] = next
		changed = true
		return []string{"scheduled-operations"}, true, nil
	})
	if err == nil && changed && previousExists {
		scheduler.cancelRunning(request.ID, previousVersion)
	}
	return err
}

func (scheduler *Scheduler) Cancel(id string) error {
	id = strings.TrimSpace(id)
	if scheduler == nil || scheduler.state == nil || id == "" {
		return fmt.Errorf("scheduled operation id is required")
	}
	cancelled := false
	var version uint64
	_, err := scheduler.state.ApplyWithoutMapMutation(func(gameState *State.GameState) ([]string, bool, error) {
		operation, exists := gameState.Scheduled[id]
		if !exists || operation.Status != "scheduled" && operation.Status != "running" {
			return nil, false, nil
		}
		version = operation.Version
		operation.Status = "cancelled"
		operation.LastError = ""
		operation.UpdatedAt = time.Now().UTC()
		gameState.Scheduled[id] = operation
		cancelled = true
		return []string{"scheduled-operations"}, true, nil
	})
	if err == nil && cancelled {
		scheduler.cancelRunning(id, version)
	}
	return err
}

func (scheduler *Scheduler) Run(ctx context.Context) {
	if scheduler == nil || scheduler.state == nil || scheduler.intents == nil {
		return
	}
	if !scheduler.started.CompareAndSwap(false, true) {
		return
	}
	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()
	scheduler.dispatchDue(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			scheduler.dispatchDue(ctx)
		}
	}
}

func (scheduler *Scheduler) dispatchDue(ctx context.Context) {
	snapshot := scheduler.state.ReadOnlyView()
	ids := make([]string, 0, len(snapshot.Scheduled))
	for id := range snapshot.Scheduled {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	now := time.Now().UTC()
	for _, id := range ids {
		operation := snapshot.Scheduled[id]
		if operation.Status != "scheduled" && operation.Status != "running" || operation.ExecuteAt.After(now) {
			continue
		}
		if !scheduledOperationMatchesAccount(snapshot, operation) {
			continue
		}
		operationID := scheduledIntentOperationID(snapshot, operation)
		executionContext, cancelExecution := context.WithCancel(ctx)
		scheduler.mu.Lock()
		if _, running := scheduler.running[id]; running {
			scheduler.mu.Unlock()
			cancelExecution()
			continue
		}
		scheduler.running[id] = runningExecution{
			version: operation.Version, operationID: operationID, cancel: cancelExecution,
		}
		scheduler.mu.Unlock()
		claimed := false
		claimedOperation := operation
		_, _ = scheduler.state.ApplyWithoutMapMutation(func(gameState *State.GameState) ([]string, bool, error) {
			current, exists := gameState.Scheduled[id]
			if !exists || current.Version != operation.Version ||
				current.Status != "scheduled" && current.Status != "running" || current.ExecuteAt.After(now) ||
				!scheduledOperationMatchesAccount(*gameState, current) {
				return nil, false, nil
			}
			if current.WorldID == "" {
				current.WorldID, _ = State.BoundAccount(*gameState)
			}
			if current.PlayerID == 0 {
				_, current.PlayerID = State.BoundAccount(*gameState)
			}
			if current.LastOperationID != "" {
				operationID = current.LastOperationID
			} else {
				current.LastOperationID = operationID
			}
			current.Status = "running"
			current.LastError = ""
			current.UpdatedAt = time.Now().UTC()
			gameState.Scheduled[id] = current
			claimedOperation = current
			claimed = true
			return []string{"scheduled-operations"}, true, nil
		})
		if !claimed {
			scheduler.clearRunning(id, operation.Version)
			continue
		}
		scheduler.mu.Lock()
		if running, exists := scheduler.running[id]; exists && running.version == operation.Version {
			running.operationID = operationID
			scheduler.running[id] = running
		}
		scheduler.mu.Unlock()
		go scheduler.execute(executionContext, claimedOperation)
	}
}

func (scheduler *Scheduler) execute(ctx context.Context, operation State.ScheduledOperation) {
	defer scheduler.clearRunning(operation.ID, operation.Version)
	receipt := scheduler.intents.Submit(ctx, Intent.Request{
		ID: operation.LastOperationID, Name: operation.Intent, Actor: "scheduler:" + operation.Intent,
		Arguments: append([]byte(nil), operation.Arguments...),
	})
	_, _ = scheduler.state.ApplyWithoutMapMutation(func(gameState *State.GameState) ([]string, bool, error) {
		current, exists := gameState.Scheduled[operation.ID]
		if !exists || current.Version != operation.Version {
			return nil, false, nil
		}
		if current.LastOperationID != operation.LastOperationID {
			return nil, false, nil
		}
		if receipt.ID != "" {
			current.LastOperationID = receipt.ID
		}
		switch receipt.Status {
		case Intent.StatusSucceeded:
			current.Status = "succeeded"
			current.LastError = ""
		case Intent.StatusCancelled:
			current.Status = "cancelled"
			current.LastError = receipt.Error
		case Intent.StatusPartiallySucceeded, Intent.StatusIndeterminate, Intent.StatusReconciling:
			current.Status = "reconciliation_required"
			current.LastError = receipt.Error
		default:
			current.Status = "failed"
			current.LastError = receipt.Error
			if current.LastError == "" {
				current.LastError = fmt.Sprintf("intent completed with status %s", receipt.Status)
			}
		}
		current.UpdatedAt = time.Now().UTC()
		gameState.Scheduled[operation.ID] = current
		return []string{"scheduled-operations"}, true, nil
	})
}

func (scheduler *Scheduler) cancelRunning(id string, version uint64) {
	scheduler.mu.Lock()
	running, exists := scheduler.running[id]
	scheduler.mu.Unlock()
	if !exists || running.version != version {
		return
	}
	running.cancel()
	if canceller, ok := scheduler.intents.(IntentCanceller); ok {
		canceller.Cancel(running.operationID)
	}
}

func (scheduler *Scheduler) clearRunning(id string, version uint64) {
	scheduler.mu.Lock()
	running, exists := scheduler.running[id]
	if exists && running.version == version {
		delete(scheduler.running, id)
	}
	scheduler.mu.Unlock()
	if exists && running.version == version {
		running.cancel()
	}
}

func scheduledOperationMatchesAccount(gameState State.GameState, operation State.ScheduledOperation) bool {
	worldID, playerID := State.BoundAccount(gameState)
	if operation.PlayerID > 0 && operation.PlayerID != playerID {
		return false
	}
	operationWorldID := strings.TrimSpace(operation.WorldID)
	return operationWorldID == "" || strings.EqualFold(operationWorldID, worldID)
}

func scheduledIntentOperationID(gameState State.GameState, operation State.ScheduledOperation) string {
	if operation.LastOperationID != "" {
		return operation.LastOperationID
	}
	worldID := strings.ToLower(strings.TrimSpace(operation.WorldID))
	if worldID == "" {
		boundWorldID, _ := State.BoundAccount(gameState)
		worldID = strings.ToLower(strings.TrimSpace(boundWorldID))
	}
	playerID := operation.PlayerID
	if playerID == 0 {
		_, playerID = State.BoundAccount(gameState)
	}
	scope := worldID + "\x00" + strconv.FormatInt(int64(playerID), 10) + "\x00" + operation.ID
	digest := sha256.Sum256([]byte(scope))
	return fmt.Sprintf("schedule:%x:v%d", digest[:12], operation.Version)
}
