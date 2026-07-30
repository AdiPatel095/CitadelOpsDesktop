package Intent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"CitadelDesktop/Server/State"
)

// A plan whose staleness the planner cannot observe would otherwise be retried
// forever, and every retry re-runs the plan's command dependencies against the
// game. The engine bounds the loop and surfaces the stale cause instead.
func TestEngineBoundsStaleReplansThePlannerCannotResolve(t *testing.T) {
	store := State.NewStore(State.NewGameState())
	registry := NewRegistry()
	plans := 0
	if err := registry.Register(Definition{
		Name: "test.stale",
		Planner: func(context.Context, PlanningContext, json.RawMessage) (Plan, error) {
			plans++
			return Plan{Steps: []Step{{Name: "Always stale", Action: "test.stale.action"}}}, nil
		},
	}); err != nil {
		t.Fatal(err)
	}
	engine := NewEngine(registry, store, nil, &performanceSender{}, nil)
	attempts := 0
	if err := engine.RegisterAction("test.stale.action", func(context.Context, json.RawMessage) error {
		attempts++
		return fmt.Errorf("%w: the dependency observed a newer world", ErrPlanStale)
	}); err != nil {
		t.Fatal(err)
	}

	receipt := engine.Submit(context.Background(), Request{ID: "stale-loop", Name: "test.stale"})

	if receipt.Status != StatusFailed {
		t.Fatalf("receipt status = %q, want %q (error %q)", receipt.Status, StatusFailed, receipt.Error)
	}
	if !strings.Contains(receipt.Error, ErrPlanStale.Error()) {
		t.Fatalf("receipt error %q does not carry the stale cause", receipt.Error)
	}
	if !strings.Contains(receipt.Error, "the dependency observed a newer world") {
		t.Fatalf("receipt error %q dropped the underlying reason", receipt.Error)
	}
	want := maximumStaleReplans + 1
	if plans != want {
		t.Errorf("planner ran %d times, want %d", plans, want)
	}
	if attempts != want {
		t.Errorf("plan executed %d times, want %d", attempts, want)
	}
}

// A stale plan that clears on a retry must still complete normally.
func TestEngineStillRetriesStalenessThatClears(t *testing.T) {
	store := State.NewStore(State.NewGameState())
	registry := NewRegistry()
	if err := registry.Register(Definition{
		Name: "test.transient",
		Planner: func(context.Context, PlanningContext, json.RawMessage) (Plan, error) {
			return Plan{Steps: []Step{{Name: "Stale once", Action: "test.transient.action"}}}, nil
		},
	}); err != nil {
		t.Fatal(err)
	}
	engine := NewEngine(registry, store, nil, &performanceSender{}, nil)
	attempts := 0
	if err := engine.RegisterAction("test.transient.action", func(context.Context, json.RawMessage) error {
		attempts++
		if attempts == 1 {
			return fmt.Errorf("%w: a transient race", ErrPlanStale)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	receipt := engine.Submit(context.Background(), Request{ID: "stale-once", Name: "test.transient"})

	if receipt.Status != StatusSucceeded {
		t.Fatalf("receipt status = %q, want %q (error %q)", receipt.Status, StatusSucceeded, receipt.Error)
	}
	if attempts != 2 {
		t.Fatalf("plan executed %d times, want 2", attempts)
	}
}
