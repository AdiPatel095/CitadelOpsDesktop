package Intent

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"CitadelDesktop/Server/State"
)

func TestEngineCancelsRunningOperationByID(t *testing.T) {
	registry := NewRegistry()
	if err := registry.Register(Definition{
		Name: "test.block", Effect: EffectWrite,
		Planner: func(context.Context, PlanningContext, json.RawMessage) (Plan, error) {
			return Plan{Claims: []string{"test"}, Steps: []Step{{Name: "Block", Action: "test.block"}}}, nil
		},
	}); err != nil {
		t.Fatal(err)
	}
	engine := NewEngine(registry, State.NewStore(State.NewGameState()), nil, nil, nil)
	started := make(chan struct{})
	if err := engine.RegisterAction("test.block", func(ctx context.Context, _ json.RawMessage) error {
		close(started)
		<-ctx.Done()
		return ctx.Err()
	}); err != nil {
		t.Fatal(err)
	}
	result := make(chan Receipt, 1)
	go func() {
		result <- engine.Submit(context.Background(), Request{ID: "cancel-me", Name: "test.block"})
	}()
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("operation did not start")
	}
	if !engine.Cancel("cancel-me") {
		t.Fatal("running operation was not cancelled")
	}
	select {
	case receipt := <-result:
		if receipt.Status != StatusCancelled {
			t.Fatalf("cancelled operation status = %q, error = %q", receipt.Status, receipt.Error)
		}
	case <-time.After(time.Second):
		t.Fatal("cancelled operation did not stop")
	}
	if engine.Cancel("cancel-me") {
		t.Fatal("completed operation remained cancellable")
	}
}
