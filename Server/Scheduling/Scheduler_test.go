package Scheduling

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"CitadelDesktop/Server/Intent"
	"CitadelDesktop/Server/State"
)

type successfulSubmitter struct{}

func (successfulSubmitter) Submit(_ context.Context, request Intent.Request) Intent.Receipt {
	return Intent.Receipt{ID: "executed", Intent: request.Name, Status: Intent.StatusSucceeded}
}

func TestSchedulerExecutesPersistedIntentAtDueTime(t *testing.T) {
	store := State.NewStore(State.NewGameState())
	scheduler := NewScheduler(store, successfulSubmitter{})
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
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("scheduled operation did not execute: %#v", store.Snapshot().Scheduled["test"])
}
