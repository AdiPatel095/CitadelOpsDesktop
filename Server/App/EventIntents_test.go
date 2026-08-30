package App

import (
	"context"
	"encoding/json"
	"testing"

	"CitadelDesktop/Server/Intent"
	"CitadelDesktop/Server/State"
)

func TestPlanEventRankingRefreshUsesNomadAllianceLeague(t *testing.T) {
	gameState := State.NewGameState()
	gameState.EventScores.ByEvent[72] = State.ScalableEventScore{EventID: 72, LeagueID: 5, AllianceLeagueID: 1}
	plan, err := planEventRankingRefresh(
		context.Background(), Intent.PlanningContext{State: gameState}, json.RawMessage(`{"eventId":72}`),
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Claims) != 1 || plan.Claims[0] != "event:72" {
		t.Fatalf("ranking claims = %#v", plan.Claims)
	}
	if len(plan.Steps) != 2 || plan.Steps[1].Opcode != "hgh" {
		t.Fatalf("ranking steps = %#v", plan.Steps)
	}
	var payload struct {
		LeaderboardID int64  `json:"LID"`
		ListType      int64  `json:"LT"`
		SearchValue   string `json:"SV"`
	}
	if err := json.Unmarshal(plan.Steps[1].Command.Payload, &payload); err != nil {
		t.Fatal(err)
	}
	if payload.LeaderboardID != 1 {
		t.Fatalf("ranking leaderboard id = %d", payload.LeaderboardID)
	}
	if payload.ListType != 47 {
		t.Fatalf("ranking list type = %d", payload.ListType)
	}
	if payload.SearchValue != "-1" {
		t.Fatalf("ranking search value = %q", payload.SearchValue)
	}
}
