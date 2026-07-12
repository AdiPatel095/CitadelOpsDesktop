package Scheduling

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"
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

type Scheduler struct {
	state   *State.Store
	intents IntentSubmitter

	mu      sync.Mutex
	running map[string]struct{}
}

func NewScheduler(state *State.Store, intents IntentSubmitter) *Scheduler {
	return &Scheduler{state: state, intents: intents, running: map[string]struct{}{}}
}

func (scheduler *Scheduler) Schedule(request Request) error {
	request.ID = strings.TrimSpace(request.ID)
	request.Intent = strings.TrimSpace(request.Intent)
	request.Actor = strings.TrimSpace(request.Actor)
	if scheduler == nil || scheduler.state == nil {
		return fmt.Errorf("operation scheduler is unavailable")
	}
	if request.ID == "" || request.Intent == "" || request.ExecuteAt.IsZero() {
		return fmt.Errorf("scheduled operation id, intent, and executeAt are required")
	}
	if request.Actor == "" {
		request.Actor = "scheduler"
	}
	if len(request.Arguments) == 0 {
		request.Arguments = json.RawMessage(`{}`)
	}
	if !json.Valid(request.Arguments) {
		return fmt.Errorf("scheduled operation arguments are not valid JSON")
	}
	_, err := scheduler.state.Apply(func(gameState *State.GameState) ([]string, bool, error) {
		now := time.Now().UTC()
		current := gameState.Scheduled[request.ID]
		next := State.ScheduledOperation{
			ID: request.ID, Intent: request.Intent, Actor: request.Actor,
			Arguments: append([]byte(nil), request.Arguments...), ExecuteAt: request.ExecuteAt.UTC(),
			CreatedAt: current.CreatedAt, Status: "scheduled",
		}
		if next.CreatedAt.IsZero() {
			next.CreatedAt = now
		}
		gameState.Scheduled[request.ID] = next
		return []string{"scheduled-operations"}, true, nil
	})
	return err
}

func (scheduler *Scheduler) Cancel(id string) error {
	id = strings.TrimSpace(id)
	if scheduler == nil || scheduler.state == nil || id == "" {
		return fmt.Errorf("scheduled operation id is required")
	}
	_, err := scheduler.state.Apply(func(gameState *State.GameState) ([]string, bool, error) {
		operation, exists := gameState.Scheduled[id]
		if !exists || operation.Status == "cancelled" {
			return nil, false, nil
		}
		operation.Status = "cancelled"
		operation.LastError = ""
		gameState.Scheduled[id] = operation
		return []string{"scheduled-operations"}, true, nil
	})
	return err
}

func (scheduler *Scheduler) Run(ctx context.Context) {
	if scheduler == nil || scheduler.state == nil || scheduler.intents == nil {
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
	snapshot := scheduler.state.Snapshot()
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
		scheduler.mu.Lock()
		if _, running := scheduler.running[id]; running {
			scheduler.mu.Unlock()
			continue
		}
		scheduler.running[id] = struct{}{}
		scheduler.mu.Unlock()
		claimed := false
		_, _ = scheduler.state.Apply(func(gameState *State.GameState) ([]string, bool, error) {
			current, exists := gameState.Scheduled[id]
			if !exists || current.Status != "scheduled" && current.Status != "running" || current.ExecuteAt.After(now) {
				return nil, false, nil
			}
			current.Status = "running"
			current.LastError = ""
			gameState.Scheduled[id] = current
			claimed = true
			return []string{"scheduled-operations"}, true, nil
		})
		if !claimed {
			scheduler.clearRunning(id)
			continue
		}
		go scheduler.execute(ctx, operation)
	}
}

func (scheduler *Scheduler) execute(ctx context.Context, operation State.ScheduledOperation) {
	receipt := scheduler.intents.Submit(ctx, Intent.Request{
		Name: operation.Intent, Actor: operation.Actor, Arguments: append([]byte(nil), operation.Arguments...),
	})
	_, _ = scheduler.state.Apply(func(gameState *State.GameState) ([]string, bool, error) {
		current, exists := gameState.Scheduled[operation.ID]
		if !exists || current.Status == "cancelled" {
			return nil, false, nil
		}
		if !current.ExecuteAt.Equal(operation.ExecuteAt) || string(current.Arguments) != string(operation.Arguments) {
			return nil, false, nil
		}
		current.LastOperationID = receipt.ID
		if receipt.Status == Intent.StatusSucceeded {
			current.Status = "succeeded"
			current.LastError = ""
		} else {
			current.Status = "failed"
			current.LastError = receipt.Error
		}
		gameState.Scheduled[operation.ID] = current
		return []string{"scheduled-operations"}, true, nil
	})
	scheduler.clearRunning(operation.ID)
}

func (scheduler *Scheduler) clearRunning(id string) {
	scheduler.mu.Lock()
	delete(scheduler.running, id)
	scheduler.mu.Unlock()
}
