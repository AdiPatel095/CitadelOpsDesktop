package Scheduling

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"CitadelDesktop/Server/Intent"
	"CitadelDesktop/Server/State"
)

type successfulSubmitter struct {
	requests chan Intent.Request
}

func (submitter successfulSubmitter) Submit(_ context.Context, request Intent.Request) Intent.Receipt {
	if submitter.requests != nil {
		submitter.requests <- request
	}
	return Intent.Receipt{ID: "executed", Intent: request.Name, Status: Intent.StatusSucceeded}
}

type cancellableSubmitter struct {
	requests      chan Intent.Request
	indeterminate bool
}

func (submitter cancellableSubmitter) Submit(ctx context.Context, request Intent.Request) Intent.Receipt {
	if submitter.requests != nil {
		submitter.requests <- request
	}
	<-ctx.Done()
	status := Intent.StatusCancelled
	if submitter.indeterminate {
		status = Intent.StatusIndeterminate
	}
	return Intent.Receipt{ID: request.ID, Intent: request.Name, Status: status, Error: ctx.Err().Error()}
}

type rescheduleSubmitter struct {
	requests chan Intent.Request
}

func (submitter rescheduleSubmitter) Submit(ctx context.Context, request Intent.Request) Intent.Receipt {
	submitter.requests <- request
	if string(request.Arguments) == `{"value":1}` {
		<-ctx.Done()
		return Intent.Receipt{
			ID: request.ID, Intent: request.Name, Status: Intent.StatusCancelled, Error: ctx.Err().Error(),
		}
	}
	return Intent.Receipt{ID: request.ID, Intent: request.Name, Status: Intent.StatusSucceeded}
}

func TestSchedulerExecutesPersistedIntentAtDueTime(t *testing.T) {
	store := State.NewStore(State.NewGameState())
	requests := make(chan Intent.Request, 1)
	scheduler := NewScheduler(store, successfulSubmitter{requests: requests})
	if err := scheduler.Schedule(Request{
		ID: "test", Intent: "example", Arguments: json.RawMessage(`{"value":1}`),
		ExecuteAt: time.Now().UTC().Add(25 * time.Millisecond),
	}); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go scheduler.Run(ctx)
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		operation := store.Snapshot().Scheduled["test"]
		if operation.Status == "succeeded" {
			if operation.LastOperationID != "executed" {
				t.Fatalf("unexpected receipt: %#v", operation)
			}
			request := <-requests
			if request.Actor != "scheduler:example" {
				t.Fatalf("scheduled actor = %q", request.Actor)
			}
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("scheduled operation did not execute: %#v", store.Snapshot().Scheduled["test"])
}

func TestSchedulerCancelPropagatesToActiveIntent(t *testing.T) {
	gameState := State.NewGameState()
	gameState.Session.ServerURL = "https://world.example"
	gameState.Player.ID = 42
	store := State.NewStore(gameState)
	requests := make(chan Intent.Request, 1)
	scheduler := NewScheduler(store, cancellableSubmitter{requests: requests})
	if err := scheduler.Schedule(Request{
		ID: "cancel-test", Intent: "example", Arguments: json.RawMessage(`{"value":1}`),
		ExecuteAt: time.Now().UTC().Add(-time.Second),
	}); err != nil {
		t.Fatal(err)
	}
	scheduler.dispatchDue(t.Context())
	request := receiveScheduledRequest(t, requests)
	running := store.Snapshot().Scheduled["cancel-test"]
	if request.ID == "" || running.Status != "running" || running.LastOperationID != request.ID {
		t.Fatalf("active scheduled operation was not durably linked: request=%+v state=%+v", request, running)
	}
	if err := scheduler.Cancel("cancel-test"); err != nil {
		t.Fatal(err)
	}
	waitForScheduledStatus(t, store, "cancel-test", "cancelled")
	waitForSchedulerIdle(t, scheduler, "cancel-test")
}

func TestSchedulerCancelAfterPossibleSendRequiresReconciliation(t *testing.T) {
	gameState := State.NewGameState()
	gameState.Player.ID = 42
	store := State.NewStore(gameState)
	requests := make(chan Intent.Request, 1)
	scheduler := NewScheduler(store, cancellableSubmitter{requests: requests, indeterminate: true})
	if err := scheduler.Schedule(Request{
		ID: "ambiguous-cancel", Intent: "example", Arguments: json.RawMessage(`{}`),
		ExecuteAt: time.Now().UTC().Add(-time.Second),
	}); err != nil {
		t.Fatal(err)
	}
	scheduler.dispatchDue(t.Context())
	_ = receiveScheduledRequest(t, requests)
	if err := scheduler.Cancel("ambiguous-cancel"); err != nil {
		t.Fatal(err)
	}
	waitForScheduledStatus(t, store, "ambiguous-cancel", "reconciliation_required")
}

func TestSchedulerRescheduleVersionsAndCancelsOldExecution(t *testing.T) {
	gameState := State.NewGameState()
	gameState.Session.ServerURL = "https://world.example"
	gameState.Player.ID = 42
	store := State.NewStore(gameState)
	requests := make(chan Intent.Request, 2)
	scheduler := NewScheduler(store, rescheduleSubmitter{requests: requests})
	if err := scheduler.Schedule(Request{
		ID: "reschedule-test", Intent: "example", Arguments: json.RawMessage(`{"value":1}`),
		ExecuteAt: time.Now().UTC().Add(-time.Second),
	}); err != nil {
		t.Fatal(err)
	}
	scheduler.dispatchDue(t.Context())
	first := receiveScheduledRequest(t, requests)
	if err := scheduler.Schedule(Request{
		ID: "reschedule-test", Intent: "example", Arguments: json.RawMessage(`{"value":2}`),
		ExecuteAt: time.Now().UTC().Add(-time.Second),
	}); err != nil {
		t.Fatal(err)
	}
	waitForSchedulerIdle(t, scheduler, "reschedule-test")
	rescheduled := store.Snapshot().Scheduled["reschedule-test"]
	if rescheduled.Version != 2 || rescheduled.Status != "scheduled" || rescheduled.LastOperationID != "" {
		t.Fatalf("rescheduled definition = %+v", rescheduled)
	}
	scheduler.dispatchDue(t.Context())
	second := receiveScheduledRequest(t, requests)
	if first.ID == second.ID || string(second.Arguments) != `{"value":2}` {
		t.Fatalf("scheduled operation IDs/arguments were not versioned: first=%+v second=%+v", first, second)
	}
	waitForScheduledStatus(t, store, "reschedule-test", "succeeded")
}

func TestScheduledOperationIDIsScopedByAccountAndVersion(t *testing.T) {
	firstState := State.NewGameState()
	firstState.Session.ServerURL = "https://world-a.example"
	firstState.Player.ID = 42
	secondState := firstState
	secondState.Player.ID = 43
	operation := State.ScheduledOperation{ID: "same", Version: 1}
	first := scheduledIntentOperationID(firstState, operation)
	second := scheduledIntentOperationID(secondState, operation)
	operation.Version = 2
	third := scheduledIntentOperationID(firstState, operation)
	if first == second || first == third || first == "" || second == "" || third == "" {
		t.Fatalf("operation IDs are not account/version scoped: %q %q %q", first, second, third)
	}
}

func receiveScheduledRequest(t *testing.T, requests <-chan Intent.Request) Intent.Request {
	t.Helper()
	select {
	case request := <-requests:
		return request
	case <-time.After(time.Second):
		t.Fatal("scheduled intent was not submitted")
		return Intent.Request{}
	}
}

func waitForScheduledStatus(t *testing.T, store *State.Store, id string, status string) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if operation := store.Snapshot().Scheduled[id]; operation.Status == status {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("scheduled operation %q did not reach %q: %+v", id, status, store.Snapshot().Scheduled[id])
}

func waitForSchedulerIdle(t *testing.T, scheduler *Scheduler, id string) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		scheduler.mu.Lock()
		_, running := scheduler.running[id]
		scheduler.mu.Unlock()
		if !running {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("scheduled operation %q remained active", id)
}
