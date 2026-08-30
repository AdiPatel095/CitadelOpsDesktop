package App

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"CitadelDesktop/Server/GameData"
	"CitadelDesktop/Server/Intent"
	"CitadelDesktop/Server/State"
)

func TestRiftMaidenRunStartPersistsExactGoalAndBusyCandidates(t *testing.T) {
	gameData, err := GameData.DecodeStore([]byte(`{
		"versionInfo":[],"buildings":[],"units":[{"wodID":216}]
	}`), GameData.SourceMetadata{ItemVersion: "test"})
	if err != nil {
		t.Fatal(err)
	}
	gameState := State.NewGameState()
	gameState.Castles[1] = State.CastleState{
		ID: 1, SlotType: 1, KingdomID: 0, X: 7, Y: 8,
		Units: State.CastleUnits{Stationed: map[State.UnitID]int64{216: 99}},
	}
	gameState.Map[0] = map[string]State.MapObservation{
		"10:20": {KingdomID: 0, X: 10, Y: 20, TypeID: riftMapTypeID},
	}
	gameState.Commanders[5] = State.CommanderState{ID: 5, Available: false}
	gameState.Inventory.Equipment[5] = State.EquipmentInstance{
		ID: 5, RarityID: 5, WearerID: 5, WearerKind: "commander",
		Effects: State.EquipmentEffects{{WireID: maidenSupportEffectID, Values: []float64{500}}},
	}
	arguments := json.RawMessage(`{
		"attackCount":50,"unitWodID":216,"horseTravelBoostId":-1,"commanderIds":[5]
	}`)
	plan, err := planRiftMaidenRunStart(
		context.Background(), Intent.PlanningContext{State: gameState, GameData: gameData}, arguments,
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Steps) != 1 || plan.Steps[0].Action != "rift.maiden_run.start" ||
		!containsString(plan.Claims, "rift-launch:maiden-wave") {
		t.Fatalf("start plan = %#v", plan)
	}
	application := &Application{State: State.NewStore(gameState)}
	if err := application.startRiftMaidenRun(context.Background(), plan.Steps[0].ActionArguments); err != nil {
		t.Fatal(err)
	}
	run := application.State.Snapshot().Rift.MaidenRun
	if run == nil || run.Status != "running" || run.RequestedAttacks != 50 || run.AttacksLaunched != 0 ||
		run.SourceCastleID != 1 || run.SourceX != 7 || run.SourceY != 8 || run.TargetX != 10 || run.TargetY != 20 ||
		len(run.CommanderIDs) != 1 || run.CommanderIDs[0] != 5 {
		t.Fatalf("persisted run = %#v", run)
	}
	cancel, err := planRiftMaidenRunCancel(
		context.Background(), Intent.PlanningContext{State: application.State.Snapshot()},
		json.RawMessage(`{"runId":"`+run.ID+`"}`),
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := application.cancelRiftMaidenRun(context.Background(), cancel.Steps[0].ActionArguments); err != nil {
		t.Fatal(err)
	}
	if status := application.State.Snapshot().Rift.MaidenRun.Status; status != "cancelled" {
		t.Fatalf("cancelled run status = %q", status)
	}
}

func TestRiftMaidenRunLaunchCannotExceedRemainingGoal(t *testing.T) {
	gameData, err := GameData.DecodeStore([]byte(`{
		"versionInfo":[],"buildings":[],"units":[{"wodID":216}]
	}`), GameData.SourceMetadata{ItemVersion: "test"})
	if err != nil {
		t.Fatal(err)
	}
	gameState := State.NewGameState()
	gameState.Castles[1] = State.CastleState{
		ID: 1, SlotType: 1, KingdomID: 0, X: 7, Y: 8,
		Units: State.CastleUnits{Stationed: map[State.UnitID]int64{216: 99}},
	}
	gameState.Map[0] = map[string]State.MapObservation{
		"10:20": {KingdomID: 0, X: 10, Y: 20, TypeID: riftMapTypeID},
	}
	for _, id := range []State.CommanderID{5, 6} {
		gameState.Commanders[id] = State.CommanderState{ID: id, Available: true}
		gameState.Inventory.Equipment[State.EquipmentInstanceID(id)] = State.EquipmentInstance{
			ID: State.EquipmentInstanceID(id), RarityID: 5, WearerID: int64(id), WearerKind: "commander",
			Effects: State.EquipmentEffects{{WireID: maidenSupportEffectID, Values: []float64{500}}},
		}
	}
	gameState.Rift.MaidenRun = &State.RiftMaidenRunState{
		ID: "run", Status: "running", RequestedAttacks: 3, AttacksLaunched: 2,
		UnitID: 216, HorseTravelBoostID: -1, CommanderIDs: []State.CommanderID{5, 6},
		SourceCastleID: 1, SourceX: 7, SourceY: 8, KingdomID: 0, TargetX: 10, TargetY: 20,
	}
	_, err = planMaidenCommsWave(
		context.Background(), Intent.PlanningContext{State: gameState, GameData: gameData},
		json.RawMessage(`{
			"runId":"run","unitWodID":216,"horseTravelBoostId":-1,
			"commanderSelection":{"candidates":[5,6],"count":2,"strategy":"first_available"}
		}`),
	)
	if err == nil || !strings.Contains(err.Error(), "only 1 probe(s) remaining") {
		t.Fatalf("over-goal launch error = %v", err)
	}
}
