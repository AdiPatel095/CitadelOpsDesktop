package App

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

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

func TestResolveConstructionEquipRejectsExpiredAttachedTargetSlot(t *testing.T) {
	gameState := constructionIntentState()
	remaining := 0
	castle := gameState.Castles[10]
	castle.ConstructionSlots[100] = []State.ConstructionSlot{{DefinitionID: 101, Slot: 0, RemainingSec: &remaining}}
	gameState.Castles[10] = castle

	step, err := resolveConstructionEquipStep(context.Background(), Intent.PlanningContext{
		State: gameState, GameData: constructionIntentGameData(t),
	}, json.RawMessage(`{"castleId":10,"buildingInstanceId":100,"constructionItemId":102,"slot":0}`))
	if err == nil || !strings.Contains(err.Error(), "already has a construction item equipped") {
		t.Fatalf("equip error = %v", err)
	}
	if step.Command.Opcode == "rpc" {
		t.Fatalf("resolver produced rejected RPC: %+v", step)
	}
}

func TestConstructionEquipEngineDoesNotSendRPCAgainstCapturedOccupiedSlots(t *testing.T) {
	gameData := capturedConstructionIntentGameData(t)
	tests := []struct {
		name       string
		buildingID State.BuildingInstanceID
		targetID   State.ConstructionItemID
		slots      []State.ConstructionSlot
	}{
		{
			name: "30403 blocks 30401", buildingID: 4094, targetID: 30401,
			slots: []State.ConstructionSlot{{DefinitionID: 214, Slot: 0}, {DefinitionID: 30403, Slot: 0, RemainingSec: constructionIntentIntPointer(0)}},
		},
		{
			name: "30482 blocks 30481", buildingID: 6875, targetID: 30481,
			slots: []State.ConstructionSlot{{DefinitionID: 30482, Slot: 0, RemainingSec: constructionIntentIntPointer(0)}},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			now := time.Now().UTC()
			gameState := State.NewGameState()
			gameState.Inventory.ConstructionItemsObservedAt = now
			gameState.Inventory.ConstructionItems[test.targetID] = 1
			gameState.Castles[10] = State.CastleState{
				ID: 10, Focused: true,
				Buildings: map[State.BuildingInstanceID]State.Building{
					test.buildingID: {InstanceID: test.buildingID, DefinitionID: 200},
				},
				ConstructionSlots:           map[State.BuildingInstanceID][]State.ConstructionSlot{test.buildingID: test.slots},
				ConstructionSlotsObservedAt: now,
			}
			arguments, err := json.Marshal(constructionEquipRequest{
				CastleID: 10, BuildingInstanceID: test.buildingID, DefinitionID: test.targetID, Slot: 0,
			})
			if err != nil {
				t.Fatal(err)
			}
			registry := Intent.NewRegistry()
			if err := registry.Register(Intent.Definition{
				Name: "test.construction.equip", Effect: Intent.EffectWrite,
				Planner: func(context.Context, Intent.PlanningContext, json.RawMessage) (Intent.Plan, error) {
					return Intent.Plan{Steps: []Intent.Step{{
						Name: "Resolve construction equip", Resolver: "construction.equip.build", ResolverArguments: arguments,
					}}}, nil
				},
			}); err != nil {
				t.Fatal(err)
			}
			sender := &constructionIntentCountingSender{}
			engine := Intent.NewEngine(
				registry, State.NewStore(gameState), constructionIntentGameDataProvider{store: gameData}, sender, nil,
			)
			if err := engine.RegisterStepResolver("construction.equip.build", resolveConstructionEquipStep); err != nil {
				t.Fatal(err)
			}
			receipt := engine.Submit(t.Context(), Intent.Request{Name: "test.construction.equip"})
			if receipt.Status != Intent.StatusFailed || !strings.Contains(receipt.DiagnosticError(), "already has a construction item equipped") {
				t.Fatalf("engine receipt = %#v", receipt)
			}
			if sender.sends != 0 {
				t.Fatalf("engine sent %d command(s), want no RPC", sender.sends)
			}
		})
	}
}

func TestResolveConstructionEquipFailsClosedForStaleOrMalformedSlotData(t *testing.T) {
	t.Run("stale snapshot", func(t *testing.T) {
		gameState := constructionIntentState()
		castle := gameState.Castles[10]
		castle.ConstructionSlotsObservedAt = time.Now().UTC().Add(-constructionSlotSnapshotMaxAge)
		gameState.Castles[10] = castle
		step, err := resolveConstructionEquipStep(t.Context(), Intent.PlanningContext{
			State: gameState, GameData: constructionIntentGameData(t),
		}, json.RawMessage(`{"castleId":10,"buildingInstanceId":100,"constructionItemId":102,"slot":0}`))
		if err == nil || !strings.Contains(err.Error(), "slots for castle 10 are stale") || step.Command.Opcode == "rpc" {
			t.Fatalf("stale resolver result: step=%+v err=%v", step, err)
		}
	})

	t.Run("unknown attached definition", func(t *testing.T) {
		gameState := constructionIntentState()
		castle := gameState.Castles[10]
		castle.ConstructionSlots[100] = []State.ConstructionSlot{{DefinitionID: 999999, Slot: 0}}
		gameState.Castles[10] = castle
		step, err := resolveConstructionEquipStep(t.Context(), Intent.PlanningContext{
			State: gameState, GameData: constructionIntentGameData(t),
		}, json.RawMessage(`{"castleId":10,"buildingInstanceId":100,"constructionItemId":102,"slot":0}`))
		if err == nil || !strings.Contains(err.Error(), "already has a construction item equipped") || step.Command.Opcode == "rpc" {
			t.Fatalf("unknown resolver result: step=%+v err=%v", step, err)
		}
	})

	t.Run("attached definition with negative slot type", func(t *testing.T) {
		gameData, err := GameData.DecodeStore([]byte(`{
			"versionInfo":[],"buildings":[],"units":[],
			"constructionItems":[
				{"constructionItemID":102,"duration":3600,"slotTypeID":0},
				{"constructionItemID":104,"duration":3600,"slotTypeID":-1}
			]
		}`), GameData.SourceMetadata{ItemVersion: "test"})
		if err != nil {
			t.Fatal(err)
		}
		gameState := constructionIntentState()
		castle := gameState.Castles[10]
		castle.ConstructionSlots[100] = []State.ConstructionSlot{{DefinitionID: 104, Slot: 0}}
		gameState.Castles[10] = castle
		step, err := resolveConstructionEquipStep(t.Context(), Intent.PlanningContext{
			State: gameState, GameData: gameData,
		}, json.RawMessage(`{"castleId":10,"buildingInstanceId":100,"constructionItemId":102,"slot":0}`))
		if err == nil || !strings.Contains(err.Error(), "already has a construction item equipped") || step.Command.Opcode == "rpc" {
			t.Fatalf("negative-slot resolver result: step=%+v err=%v", step, err)
		}
	})

	t.Run("target without official slot type", func(t *testing.T) {
		gameData, err := GameData.DecodeStore([]byte(`{
			"versionInfo":[],"buildings":[],"units":[],
			"constructionItems":[{"constructionItemID":102,"duration":3600}]
		}`), GameData.SourceMetadata{ItemVersion: "test"})
		if err != nil {
			t.Fatal(err)
		}
		step, err := resolveConstructionEquipStep(t.Context(), Intent.PlanningContext{
			State: constructionIntentState(), GameData: gameData,
		}, json.RawMessage(`{"castleId":10,"buildingInstanceId":100,"constructionItemId":102,"slot":0}`))
		if err == nil || !strings.Contains(err.Error(), "no valid official slot type") || step.Command.Opcode == "rpc" {
			t.Fatalf("malformed resolver result: step=%+v err=%v", step, err)
		}
	})
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
		ConstructionSlots:           map[State.BuildingInstanceID][]State.ConstructionSlot{},
		ConstructionSlotsObservedAt: time.Now().UTC(),
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

type constructionIntentGameDataProvider struct{ store *GameData.Store }

func (provider constructionIntentGameDataProvider) Current() (*GameData.Store, bool) {
	return provider.store, provider.store != nil
}

type constructionIntentCountingSender struct{ sends int }

func (*constructionIntentCountingSender) Ready() bool       { return true }
func (*constructionIntentCountingSender) Namespace() string { return "EmpireEx_21" }
func (sender *constructionIntentCountingSender) Send(context.Context, []byte) error {
	sender.sends++
	return nil
}

func constructionIntentIntPointer(value int) *int { return &value }

func capturedConstructionIntentGameData(t *testing.T) *GameData.Store {
	t.Helper()
	gameData, err := GameData.DecodeStore([]byte(`{
		"versionInfo":[],"buildings":[{"wodID":200,"constructionItemGroupIDs":"1"}],"units":[],
		"constructionItems":[
			{"constructionItemID":214,"constructionItemGroupID":1,"name":"marketCarriages","level":1,"slotTypeID":1},
			{"constructionItemID":30401,"constructionItemGroupID":1,"name":"pirateMarket","duration":345600,"level":2,"slotTypeID":0},
			{"constructionItemID":30403,"constructionItemGroupID":1,"name":"pirateMarket","duration":345600,"level":4,"slotTypeID":0},
			{"constructionItemID":30481,"constructionItemGroupID":1,"name":"piratesWoodcutter","duration":345600,"level":2,"slotTypeID":0},
			{"constructionItemID":30482,"constructionItemGroupID":1,"name":"piratesWoodcutter","duration":345600,"level":3,"slotTypeID":0}
		]
	}`), GameData.SourceMetadata{ItemVersion: "786.03"})
	if err != nil {
		t.Fatal(err)
	}
	return gameData
}
