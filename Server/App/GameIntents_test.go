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

func TestResolveConstructionEquipRejectsBuildingWithEquippedItem(t *testing.T) {
	gameData := constructionIntentGameData(t)
	remaining := 1
	gameState := constructionIntentState()
	castle := gameState.Castles[10]
	castle.ConstructionSlots[100] = []State.ConstructionSlot{{DefinitionID: 101, Slot: 0, RemainingSec: &remaining}}
	gameState.Castles[10] = castle

	_, err := resolveConstructionEquipStep(context.Background(), Intent.PlanningContext{
		State: gameState, GameData: gameData,
	}, json.RawMessage(`{"castleId":10,"buildingInstanceId":100,"constructionItemId":102,"slot":0}`))
	if err == nil || !strings.Contains(err.Error(), "already has a construction item equipped") {
		t.Fatalf("equip error = %v", err)
	}
}

func TestResolveConstructionEquipRejectsOccupiedSlotWithoutRemainingSeconds(t *testing.T) {
	gameState := constructionIntentState()
	castle := gameState.Castles[10]
	castle.ConstructionSlots[100] = []State.ConstructionSlot{{DefinitionID: 101, Slot: 0}}
	gameState.Castles[10] = castle

	_, err := resolveConstructionEquipStep(context.Background(), Intent.PlanningContext{
		State: gameState, GameData: constructionIntentGameData(t),
	}, json.RawMessage(`{"castleId":10,"buildingInstanceId":100,"constructionItemId":102,"slot":0}`))
	if err == nil || !strings.Contains(err.Error(), "already has a construction item equipped") {
		t.Fatalf("equip error = %v", err)
	}
}

func TestResolveConstructionEquipRejectsItemObservedAfterPlanning(t *testing.T) {
	gameData := constructionIntentGameData(t)
	gameState := constructionIntentState()
	plan, err := planConstructionEquip(context.Background(), Intent.PlanningContext{
		State: gameState, GameData: gameData,
	}, json.RawMessage(`{"castleId":10,"buildingInstanceId":100,"constructionItemId":102,"slot":0}`))
	if err != nil {
		t.Fatal(err)
	}
	castle := gameState.Castles[10]
	castle.ConstructionSlots[100] = []State.ConstructionSlot{{DefinitionID: 101, Slot: 0}}
	gameState.Castles[10] = castle

	_, err = resolveConstructionEquipStep(context.Background(), Intent.PlanningContext{
		State: gameState, GameData: gameData,
	}, plan.Steps[len(plan.Steps)-1].ResolverArguments)
	if err == nil || !strings.Contains(err.Error(), "already has a construction item equipped") {
		t.Fatalf("resolved equip error = %v", err)
	}
}

func TestResolveConstructionEquipRejectsNonTemporaryEquippedItem(t *testing.T) {
	gameState := constructionIntentState()
	castle := gameState.Castles[10]
	castle.ConstructionSlots[100] = []State.ConstructionSlot{{DefinitionID: 103, Slot: 0}}
	gameState.Castles[10] = castle

	_, err := resolveConstructionEquipStep(context.Background(), Intent.PlanningContext{
		State: gameState, GameData: constructionIntentGameData(t),
	}, json.RawMessage(`{"castleId":10,"buildingInstanceId":100,"constructionItemId":102,"slot":0}`))
	if err == nil || !strings.Contains(err.Error(), "already has a construction item equipped") {
		t.Fatalf("equip error = %v", err)
	}
}

func TestResolveConstructionEquipAllowsOccupiedDifferentSlot(t *testing.T) {
	gameData, err := GameData.DecodeStore([]byte(`{
		"versionInfo":[],"buildings":[],"units":[],
		"constructionItems":[
			{"constructionItemID":101,"duration":3600,"slotTypeID":0},
			{"constructionItemID":103,"slotTypeID":1}
		]
	}`), GameData.SourceMetadata{ItemVersion: "test"})
	if err != nil {
		t.Fatal(err)
	}
	gameState := constructionIntentState()
	castle := gameState.Castles[10]
	castle.ConstructionSlots[100] = []State.ConstructionSlot{{DefinitionID: 103, Slot: 0}}
	gameState.Castles[10] = castle

	step, err := resolveConstructionEquipStep(context.Background(), Intent.PlanningContext{
		State: gameState, GameData: gameData,
	}, json.RawMessage(`{"castleId":10,"buildingInstanceId":100,"constructionItemId":101,"slot":0}`))
	if err != nil {
		t.Fatal(err)
	}
	if got := string(step.Command.Payload); got != `{"OID":100,"CID":101,"SID":0,"M":0,"KID":0,"AID":10}` {
		t.Fatalf("equip payload = %s", got)
	}
}

func TestResolveConstructionEquipAllowsExpiredTargetSlot(t *testing.T) {
	gameState := constructionIntentState()
	remaining := 0
	castle := gameState.Castles[10]
	castle.ConstructionSlots[100] = []State.ConstructionSlot{{DefinitionID: 101, Slot: 0, RemainingSec: &remaining}}
	gameState.Castles[10] = castle

	if _, err := resolveConstructionEquipStep(context.Background(), Intent.PlanningContext{
		State: gameState, GameData: constructionIntentGameData(t),
	}, json.RawMessage(`{"castleId":10,"buildingInstanceId":100,"constructionItemId":102,"slot":0}`)); err != nil {
		t.Fatal(err)
	}
}

func TestPlanConstructionUpgradeDoesNotCrossReusedGroupVariant(t *testing.T) {
	gameData, err := GameData.DecodeStore([]byte(`{
		"versionInfo":[],"buildings":[],"units":[],
		"constructionItems":[
			{"constructionItemID":101,"constructionItemGroupID":18,"name":"AnniversaryDwelling","duration":3600,"effects":"10&5+0","level":1,"slotTypeID":1},
			{"constructionItemID":201,"constructionItemGroupID":18,"name":"BlackFridayDwelling","duration":3600,"effects":"10&10+0","level":2,"slotTypeID":1}
		]
	}`), GameData.SourceMetadata{ItemVersion: "test"})
	if err != nil {
		t.Fatal(err)
	}
	remaining := 60
	gameState := constructionIntentState()
	castle := gameState.Castles[10]
	castle.ConstructionSlots[100] = []State.ConstructionSlot{{DefinitionID: 101, Slot: 0, RemainingSec: &remaining}}
	gameState.Castles[10] = castle

	_, err = planConstructionUpgrade(context.Background(), Intent.PlanningContext{
		State: gameState, GameData: gameData,
	}, json.RawMessage(`{"castleId":10,"buildingInstanceId":100,"constructionItemId":101,"slot":0,"offerCode":2000}`))
	if err == nil || !strings.Contains(err.Error(), "does not match official target level 0") {
		t.Fatalf("upgrade error = %v", err)
	}
}

func TestResolveConstructionUpgradeUsesExactCIDWhenWireSlotsMatch(t *testing.T) {
	gameData, err := GameData.DecodeStore([]byte(`{
		"versionInfo":[],"buildings":[],"units":[],
		"constructionItems":[
			{"constructionItemID":101,"constructionItemGroupID":1,"name":"Temporary","duration":3600,"level":1,"slotTypeID":0},
			{"constructionItemID":102,"constructionItemGroupID":1,"name":"Temporary","duration":3600,"level":2,"slotTypeID":0},
			{"constructionItemID":201,"constructionItemGroupID":2,"name":"Permanent","level":1,"slotTypeID":1}
		]
	}`), GameData.SourceMetadata{ItemVersion: "test"})
	if err != nil {
		t.Fatal(err)
	}
	remaining := 60
	gameState := constructionIntentState()
	castle := gameState.Castles[10]
	castle.ConstructionSlots[100] = []State.ConstructionSlot{
		{DefinitionID: 201, Slot: 0},
		{DefinitionID: 101, Slot: 0, RemainingSec: &remaining},
	}
	gameState.Castles[10] = castle

	step, err := resolveConstructionUpgradeStep(context.Background(), Intent.PlanningContext{
		State: gameState, GameData: gameData,
	}, json.RawMessage(`{"castleId":10,"buildingInstanceId":100,"constructionItemId":101,"slot":0,"offerCode":2000}`))
	if err != nil {
		t.Fatal(err)
	}
	if got := string(step.Command.Payload); got != `{"OID":100,"SUC":2000,"SID":0,"KID":0,"AID":10,"CID":101}` {
		t.Fatalf("upgrade payload = %s", got)
	}
}

func constructionIntentState() State.GameState {
	gameState := State.NewGameState()
	gameState.Castles[10] = State.CastleState{
		ID: 10,
		Buildings: map[State.BuildingInstanceID]State.Building{
			100: {InstanceID: 100, DefinitionID: 200},
		},
		ConstructionSlots: map[State.BuildingInstanceID][]State.ConstructionSlot{},
	}
	return gameState
}

func constructionIntentGameData(t *testing.T) *GameData.Store {
	t.Helper()
	gameData, err := GameData.DecodeStore([]byte(`{
		"versionInfo":[],"buildings":[],"units":[],
		"constructionItems":[
			{"constructionItemID":101,"duration":3600,"slotTypeID":0},
			{"constructionItemID":102,"duration":3600,"slotTypeID":0},
			{"constructionItemID":103,"duration":0,"decoPoints":100,"slotTypeID":0}
		]
	}`), GameData.SourceMetadata{ItemVersion: "test"})
	if err != nil {
		t.Fatal(err)
	}
	return gameData
}
