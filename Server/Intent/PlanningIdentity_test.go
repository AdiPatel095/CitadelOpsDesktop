package Intent

import (
	"CitadelDesktop/Server/Protocol"
	"context"
	"encoding/json"
	"testing"
)

func TestEnginePassesRuntimeFeatureIdentityToPlannerAndResolver(t *testing.T) {
	engine, _, _, _ := supportBatchEngine(t, []int{0})
	assertIdentity := func(input PlanningContext) {
		t.Helper()
		if input.AutomationLane != "autoStation" || input.IntentName != "test.identity" {
			t.Fatalf("lost runtime identity: lane=%q intent=%q", input.AutomationLane, input.IntentName)
		}
	}
	if err := engine.registry.Register(Definition{Name: "test.identity", Effect: EffectWrite, Planner: func(_ context.Context, input PlanningContext, _ json.RawMessage) (Plan, error) {
		assertIdentity(input)
		return Plan{Steps: []Step{{Resolver: "test.identity.resolve", AwaitOpcode: "cds"}}}, nil
	}}); err != nil {
		t.Fatal(err)
	}
	if err := engine.RegisterStepResolver("test.identity.resolve", func(_ context.Context, input PlanningContext, _ json.RawMessage) (Step, error) {
		assertIdentity(input)
		return Step{Opcode: "cds", Command: Protocol.Command{Opcode: "cds", Payload: json.RawMessage(`{"A":[[1,1]]}`)}, AwaitOpcode: "cds", SuccessCodes: []int{0}}, nil
	}); err != nil {
		t.Fatal(err)
	}
	receipt := engine.Submit(t.Context(), Request{Name: "test.identity", AutomationLane: "autoStation"})
	if receipt.Status != StatusSucceeded {
		t.Fatalf("identity operation: %s", receipt.DiagnosticError())
	}
	var request Request
	if err := json.Unmarshal([]byte(`{"name":"troops.station","automationLane":"autoStation"}`), &request); err != nil {
		t.Fatal(err)
	}
	if request.AutomationLane != "" {
		t.Fatal("API JSON forged runtime lane")
	}
}
