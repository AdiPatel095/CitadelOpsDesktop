package App

import (
	"encoding/json"
	"testing"
	"time"

	"CitadelDesktop/Server/GameData"
	"CitadelDesktop/Server/Intent"
	"CitadelDesktop/Server/State"
)

func TestPlanBeriTransferUsesRefreshedAmountAndCanonicalWireShape(t *testing.T) {
	gameData, err := GameData.DecodeStore([]byte(`{
		"versionInfo":[],"buildings":[{"wodID":1}],"units":[{"wodID":10}]
	}`), GameData.SourceMetadata{ItemVersion: "test"})
	if err != nil {
		t.Fatal(err)
	}
	observedAt := time.Date(2026, 7, 11, 12, 0, 0, 0, time.UTC)
	gameState := State.NewGameState()
	castle := State.CastleState{ID: 100, KingdomID: 0, SlotType: 1}
	castle.Units.Stationed = map[State.UnitID]int64{10: 50}
	gameState.Castles[100] = castle
	gameState.Beri = State.BeriState{
		AvailableTroops: 25, TroopsByUnit: map[State.UnitID]int64{10: 25}, ObservedAt: observedAt,
	}
	plan, err := planBeriTransfer(t.Context(), Intent.PlanningContext{State: gameState, GameData: gameData}, json.RawMessage(`{
		"sourceCastleId":100,"wireCastleId":-1,"unitId":10
	}`))
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Steps) != 4 || plan.Steps[0].Opcode != "kut" || plan.Steps[2].Opcode != "msk" {
		t.Fatalf("unexpected Beri plan: %#v", plan.Steps)
	}
	var payload struct {
		SourceID  int       `json:"SCID"`
		SourceKID int       `json:"SKID"`
		TargetKID int       `json:"TKID"`
		CastleID  int       `json:"CID"`
		Troops    [][]int64 `json:"A"`
	}
	if err := json.Unmarshal(plan.Steps[0].Command.Payload, &payload); err != nil {
		t.Fatal(err)
	}
	if payload.SourceID != 100 || payload.SourceKID != 0 || payload.TargetKID != 10 || payload.CastleID != -1 ||
		len(payload.Troops) != 1 || payload.Troops[0][0] != 10 || payload.Troops[0][1] != 25 {
		t.Fatalf("unexpected kut payload: %#v", payload)
	}
}
