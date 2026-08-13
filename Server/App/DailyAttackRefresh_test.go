package App

import (
	"encoding/json"
	"testing"

	"CitadelDesktop/Server/Intent"
)

func TestPlanDailyAttackRefreshRequestsAuthoritativeServerCount(t *testing.T) {
	plan, err := planDailyAttackRefresh(t.Context(), Intent.PlanningContext{}, json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("plan daily attack refresh: %v", err)
	}
	if len(plan.Steps) != 1 {
		t.Fatalf("steps = %d, want 1", len(plan.Steps))
	}
	step := plan.Steps[0]
	if step.Opcode != "gai" || step.AwaitOpcode != "gai" || string(step.Command.Payload) != `{}` {
		t.Fatalf("daily attack refresh step = %#v", step)
	}
	if step.ResponseBarrier != Intent.ResponseBarrierCommitted {
		t.Fatalf("response barrier = %q, want committed", step.ResponseBarrier)
	}
}
