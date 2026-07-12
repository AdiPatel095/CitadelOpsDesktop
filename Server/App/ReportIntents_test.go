package App

import (
	"encoding/json"
	"testing"

	"CitadelDesktop/Server/Intent"
	"CitadelDesktop/Server/State"
)

func TestPlanSpyReportShareBuildsAllianceRecipients(t *testing.T) {
	gameState := State.NewGameState()
	gameState.Player.ID = 1
	gameState.Alliance.Members = []State.AllianceMember{
		{PlayerID: 1}, {PlayerID: 2}, {PlayerID: 2}, {PlayerID: 3},
	}
	gameState.Reports.SpyCaptures[99] = State.SpyReportCapture{MessageID: 99, Payload: json.RawMessage(`{"MID":99}`)}

	plan, err := planSpyReportShare(t.Context(), Intent.PlanningContext{State: gameState}, json.RawMessage(`{"messageId":99}`))
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Steps) != 1 || plan.Steps[0].Opcode != "mfs" {
		t.Fatalf("share plan = %#v", plan)
	}
	var payload struct {
		MessageID  int64            `json:"MID"`
		Recipients []State.PlayerID `json:"PID"`
	}
	if err := json.Unmarshal(plan.Steps[0].Command.Payload, &payload); err != nil {
		t.Fatal(err)
	}
	if payload.MessageID != 99 || len(payload.Recipients) != 2 || payload.Recipients[0] != 2 || payload.Recipients[1] != 3 {
		t.Fatalf("share payload = %+v", payload)
	}
}
