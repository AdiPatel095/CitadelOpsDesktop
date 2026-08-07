package App

import (
	"encoding/json"
	"testing"

	"CitadelDesktop/Server/Intent"
)

func TestPlanWorldIntelLeaderboardPageCapturesBoundedHGHResponse(t *testing.T) {
	arguments := json.RawMessage(`{"listType":6,"levelCategory":4,"searchValue":25}`)
	plan, err := planWorldIntelLeaderboardPage(t.Context(), Intent.PlanningContext{}, arguments)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Steps) != 1 || plan.Steps[0].Opcode != "hgh" || plan.Steps[0].AwaitOpcode != "hgh" ||
		!plan.Steps[0].CaptureResponse {
		t.Fatalf("unexpected plan: %#v", plan)
	}
	if len(plan.Resources) != 1 || plan.Resources[0].Scope != Intent.ResourceScopeSession ||
		plan.Resources[0].ResourceKind != "leaderboard" {
		t.Fatalf("resources = %#v", plan.Resources)
	}
	var payload struct {
		ListType    int64  `json:"LT"`
		LevelID     int64  `json:"LID"`
		SearchValue string `json:"SV"`
	}
	if err := json.Unmarshal(plan.Steps[0].Command.Payload, &payload); err != nil {
		t.Fatal(err)
	}
	if payload.ListType != 6 || payload.LevelID != 4 || payload.SearchValue != "25" {
		t.Fatalf("payload = %#v", payload)
	}
}

func TestPlanWorldIntelLeaderboardPageRejectsUnsupportedLeaderboard(t *testing.T) {
	_, err := planWorldIntelLeaderboardPage(
		t.Context(), Intent.PlanningContext{}, json.RawMessage(`{"listType":47,"levelCategory":1,"searchValue":5}`),
	)
	if err == nil {
		t.Fatal("expected unsupported event leaderboard to be rejected")
	}
}
