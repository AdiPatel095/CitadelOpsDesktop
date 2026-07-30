package Intent

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"CitadelDesktop/Server/Outbound"
	"CitadelDesktop/Server/State"
)

type performanceSender struct{}

func (*performanceSender) Ready() bool { return true }

func (*performanceSender) Namespace() string { return "EmpireEx_21" }

func (*performanceSender) Send(context.Context, []byte) error { return nil }

type performanceRouterSender struct {
	router *Outbound.Router
}

func (*performanceRouterSender) Ready() bool { return true }

func (*performanceRouterSender) Namespace() string { return "EmpireEx_21" }

func (sender *performanceRouterSender) Send(ctx context.Context, payload []byte) error {
	return sender.router.Send(ctx, payload)
}

func BenchmarkEngineIntentToTransportCurrentData(benchmark *testing.B) {
	state, err := State.LoadSnapshot("../../Data")
	if err != nil {
		benchmark.Skipf("load current state fixture: %v", err)
	}
	state.Session.Generation = 1
	state.Session.BaselineGeneration = 1
	state.Session.ConnectionGeneration = 1
	state.Session.LoggedIn = true
	state.Session.SocketReady = true
	store := State.NewStore(state)
	registry := NewRegistry()
	if err := registry.Register(Definition{
		Name: "benchmark.send",
		Planner: func(context.Context, PlanningContext, json.RawMessage) (Plan, error) {
			return Plan{Steps: []Step{{Opcode: "benchmark", Payload: json.RawMessage(`{}`)}}}, nil
		},
	}); err != nil {
		benchmark.Fatal(err)
	}
	engine := NewEngine(registry, store, nil, &performanceSender{}, nil)
	benchmark.ReportAllocs()
	benchmark.ResetTimer()
	for index := 0; benchmark.Loop(); index++ {
		request := Request{ID: fmt.Sprintf("benchmark-operation-%d", index), Name: "benchmark.send"}
		receipt := engine.Submit(context.Background(), request)
		if receipt.Status != StatusSucceeded {
			benchmark.Fatalf("intent failed: %s", receipt.Error)
		}
	}
}

func BenchmarkEngineIntentThroughRouterCurrentData(benchmark *testing.B) {
	state, err := State.LoadSnapshot("../../Data")
	if err != nil {
		benchmark.Skipf("load current state fixture: %v", err)
	}
	state.Session.Generation = 1
	state.Session.BaselineGeneration = 1
	state.Session.ConnectionGeneration = 1
	state.Session.LoggedIn = true
	state.Session.SocketReady = true
	store := State.NewStore(state)
	registry := NewRegistry()
	if err := registry.Register(Definition{
		Name: "benchmark.routed-send",
		Planner: func(context.Context, PlanningContext, json.RawMessage) (Plan, error) {
			return Plan{Steps: []Step{{Opcode: "benchmark", Payload: json.RawMessage(`{}`)}}}, nil
		},
	}); err != nil {
		benchmark.Fatal(err)
	}
	router := Outbound.NewRouter(benchmark.Context(), Outbound.Config{
		Ready: func() bool { return true },
		Send:  func(context.Context, []byte) error { return nil },
	})
	defer router.Close()
	engine := NewEngine(registry, store, nil, &performanceRouterSender{router: router}, nil)
	benchmark.ReportAllocs()
	benchmark.ResetTimer()
	for index := 0; benchmark.Loop(); index++ {
		request := Request{ID: fmt.Sprintf("benchmark-routed-operation-%d", index), Name: "benchmark.routed-send"}
		receipt := engine.Submit(context.Background(), request)
		if receipt.Status != StatusSucceeded {
			benchmark.Fatalf("intent failed: %s", receipt.Error)
		}
	}
}

func BenchmarkEngineIntentToTransportWithDurableStore(benchmark *testing.B) {
	registry := NewRegistry()
	if err := registry.Register(Definition{
		Name: "benchmark.durable-send", Effect: EffectWrite,
		Planner: func(context.Context, PlanningContext, json.RawMessage) (Plan, error) {
			return Plan{Steps: []Step{{Opcode: "benchmark", Payload: json.RawMessage(`{}`)}}}, nil
		},
	}); err != nil {
		benchmark.Fatal(err)
	}
	operationStore, err := OpenOperationStore(benchmark.TempDir())
	if err != nil {
		benchmark.Fatal(err)
	}
	defer operationStore.Close()
	engine := NewEngine(registry, State.NewStore(State.NewGameState()), nil, &performanceSender{}, nil)
	if err := engine.SetOperationStore(benchmark.Context(), operationStore); err != nil {
		benchmark.Fatal(err)
	}
	benchmark.ReportAllocs()
	benchmark.ResetTimer()
	for index := 0; benchmark.Loop(); index++ {
		receipt := engine.Submit(context.Background(), Request{
			ID: fmt.Sprintf("benchmark-durable-operation-%d", index), Name: "benchmark.durable-send",
		})
		if receipt.Status != StatusSucceeded {
			benchmark.Fatalf("intent failed: %s", receipt.Error)
		}
	}
}
