package App

import (
	"context"
	"encoding/json"
	"testing"

	"CitadelDesktop/Server/Intent"
)

func TestPlanMapQueryClaimsCastleFocus(t *testing.T) {
	plan, err := planMapQuery(context.Background(), Intent.PlanningContext{}, json.RawMessage(
		`{"kingdomId":2,"x1":10,"y1":20,"x2":30,"y2":40}`,
	))
	if err != nil {
		t.Fatal(err)
	}

	claims := make(map[string]bool, len(plan.Claims))
	for _, claim := range plan.Claims {
		claims[claim] = true
	}
	if !claims["castle-focus"] || !claims["map:2"] {
		t.Fatalf("map query claims = %#v", plan.Claims)
	}
	if len(plan.Steps) != 1 || plan.Steps[0].Command.Opcode != "gaa" {
		t.Fatalf("map query steps = %#v", plan.Steps)
	}
}
