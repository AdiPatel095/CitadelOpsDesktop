package App

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"CitadelDesktop/Server/GameData"
	"CitadelDesktop/Server/Intent"
	"CitadelDesktop/Server/State"
)

func TestResolveBuildingPlacementAndStoreUseCapturedWireShapes(t *testing.T) {
	gameData := buildingIntentGameData(t)
	gameState := buildingIntentState()
	input := Intent.PlanningContext{State: gameState, GameData: gameData}

	placeArguments, err := json.Marshal(buildingPlacementResolverArguments{
		Kind: buildingMutationPlace,
		Request: buildingPlacementIntentRequest{
			CastleID: 10, DefinitionID: 301, X: 5, Y: 5, Rotation: 1,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	place, err := resolveBuildingPlacementStep(context.Background(), input, placeArguments)
	if err != nil {
		t.Fatal(err)
	}
	if place.Command.Opcode != "ebu" || string(place.Command.Payload) != `{"WID":301,"X":5,"Y":5,"R":1,"PWR":0,"PO":-1,"DOID":-1}` {
		t.Fatalf("placement command = %s %s", place.Command.Opcode, place.Command.Payload)
	}

	store, err := resolveBuildingStoreStep(context.Background(), input, json.RawMessage(`{"castleId":10,"buildingInstanceId":42}`))
	if err != nil {
		t.Fatal(err)
	}
	if store.Command.Opcode != "sob" || string(store.Command.Payload) != `{"OID":42}` {
		t.Fatalf("store command = %s %s", store.Command.Opcode, store.Command.Payload)
	}

	upgradeArguments, err := json.Marshal(buildingUpgradeResolverArguments{
		Request: buildingUpgradeIntentRequest{CastleID: 10, BuildingInstanceID: 42},
	})
	if err != nil {
		t.Fatal(err)
	}
	upgrade, err := resolveBuildingUpgradeStep(context.Background(), input, upgradeArguments)
	if err != nil {
		t.Fatal(err)
	}
	if upgrade.Command.Opcode != "eup" || string(upgrade.Command.Payload) != `{"OID":42,"PWR":0,"PO":-1}` {
		t.Fatalf("upgrade command = %s %s", upgrade.Command.Opcode, upgrade.Command.Payload)
	}

	demolish, err := resolveBuildingDemolishStep(
		context.Background(), input, json.RawMessage(`{"castleId":10,"buildingInstanceId":42}`),
	)
	if err != nil {
		t.Fatal(err)
	}
	if demolish.Command.Opcode != "edo" || string(demolish.Command.Payload) != `{"OID":42}` ||
		len(demolish.StaleCodes) != 1 || demolish.StaleCodes[0] != 147 {
		t.Fatalf("demolition command = %#v", demolish)
	}
}

func TestResolveBuildingDemolitionTreatsQueuedRaceAsStale(t *testing.T) {
	gameState := buildingIntentState()
	castle := gameState.Castles[10]
	building := castle.Layout.Objects[42]
	building.ConstructionState = State.BuildingStateDisassembleInProgress
	castle.Layout.Objects[42] = building
	castle.Buildings[42] = building
	castle.BuildingQueue.Slots[0] = State.BuildingConstructionQueueSlot{
		Index: 0, WireValue: 42, Status: State.BuildingQueueSlotOccupied, BuildingID: 42,
	}
	gameState.Castles[10] = castle

	_, err := resolveBuildingDemolishStep(context.Background(), Intent.PlanningContext{
		State: gameState, GameData: buildingIntentGameData(t),
	}, json.RawMessage(`{"castleId":10,"buildingInstanceId":42}`))
	if !errors.Is(err, Intent.ErrPlanStale) {
		t.Fatalf("queued demolition resolver error = %v", err)
	}
}

func TestBuildingUpgradeConfirmsExactPremiumQuoteForFixedHarbor(t *testing.T) {
	gameState := buildingIntentState()
	castle := gameState.Castles[10]
	harbor := State.Building{
		InstanceID: 32, DefinitionID: 45, GridX: -1, GridY: -1,
		ConstructionState: State.BuildingStateBuildCompleted, Level: 1, Layer: State.BuildingLayerFP,
	}
	castle.Layout.Fixed[32] = harbor
	gameState.Castles[10] = castle
	gameState.Player.Level = 70
	gameState.Player.Resources[2] = 50_000
	input := Intent.PlanningContext{
		State: gameState, GameData: buildingIntentGameData(t),
		ProtocolContext: State.ProtocolContextState{
			FocusedCastleID: 10, FocusSubcontext: State.FocusSubcontextMap, FocusEpoch: 2,
		},
	}

	plan, err := planBuildingUpgrade(context.Background(), input, json.RawMessage(
		`{"castleId":10,"buildingInstanceId":32,"allowPremium":true}`,
	))
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Steps) != 5 || plan.Steps[0].Opcode != "jca" || plan.Steps[1].Resolver != "building.upgrade.build" ||
		plan.Steps[2].Resolver != "building.upgrade.build" || plan.Steps[3].Opcode != "jaa" || plan.Steps[4].Action != "building.verify" {
		t.Fatalf("premium Harbor plan = %#v", plan.Steps)
	}
	if len(plan.Steps[1].SuccessCodes) != 1 || plan.Steps[1].SuccessCodes[0] != 440 ||
		string(plan.Steps[1].ExpectedResponsePayload) != `{"OID":32,"PWR":0,"PO":-1,"CC2T":12300}` {
		t.Fatalf("premium quote step = %#v", plan.Steps[1])
	}

	quote, err := resolveBuildingUpgradeStep(context.Background(), input, plan.Steps[1].ResolverArguments)
	if err != nil {
		t.Fatal(err)
	}
	if string(quote.Command.Payload) != `{"OID":32,"PWR":0,"PO":-1}` || len(quote.SuccessCodes) != 1 || quote.SuccessCodes[0] != 440 {
		t.Fatalf("premium quote command = %#v", quote)
	}
	confirm, err := resolveBuildingUpgradeStep(context.Background(), input, plan.Steps[2].ResolverArguments)
	if err != nil {
		t.Fatal(err)
	}
	if string(confirm.Command.Payload) != `{"OID":32,"PWR":1,"PO":-1}` {
		t.Fatalf("premium confirmation command = %#v", confirm)
	}
}

func TestResolveBuildingExpansionUsesCapturedResourceWireShape(t *testing.T) {
	gameState := buildingIntentState()
	castle := gameState.Castles[10]
	capacity := 100.0
	castle.Resources = map[State.ResourceID]State.ResourceBalance{
		3: {Amount: 100, Capacity: &capacity},
		4: {Amount: 100, Capacity: &capacity},
	}
	gameState.Castles[10] = castle

	step, err := resolveBuildingExpansionStep(context.Background(), Intent.PlanningContext{
		State: gameState, GameData: buildingIntentGameData(t),
	}, json.RawMessage(`{"castleId":10,"x":220,"y":220,"direction":1,"payment":"resources"}`))
	if err != nil {
		t.Fatal(err)
	}
	if step.Command.Opcode != "ebe" || string(step.Command.Payload) != `{"X":220,"Y":220,"R":1,"CT":1}` {
		t.Fatalf("expansion command = %s %s", step.Command.Opcode, step.Command.Payload)
	}
}

func TestResolveBuildingCollectExpansionGiftUsesCapturedWireShape(t *testing.T) {
	gameState := buildingIntentState()
	chest := State.Building{
		InstanceID: 46, DefinitionID: 520, GridX: 8, GridY: 3,
		ConstructionState: State.BuildingStateBuildCompleted, Layer: State.BuildingLayerBD, Placed: true,
	}
	castle := gameState.Castles[10]
	castle.Buildings[46] = chest
	castle.Layout.Objects[46] = chest
	gameState.Castles[10] = castle

	step, err := resolveBuildingCollectExpansionGiftStep(context.Background(), Intent.PlanningContext{
		State: gameState, GameData: buildingIntentGameData(t),
	}, json.RawMessage(`{"castleId":10,"buildingInstanceId":46}`))
	if err != nil {
		t.Fatal(err)
	}
	if step.Command.Opcode != "etc" || string(step.Command.Payload) != `{"OID":46}` {
		t.Fatalf("expansion gift command = %s %s", step.Command.Opcode, step.Command.Payload)
	}
}

func TestResolveBuildingExpansionRequiresExplicitPremiumOptIn(t *testing.T) {
	gameState := buildingIntentState()
	gameState.Player.Resources[2] = 500
	input := Intent.PlanningContext{State: gameState, GameData: buildingIntentGameData(t)}

	_, err := resolveBuildingExpansionStep(context.Background(), input, json.RawMessage(`{
		"castleId":10,"x":220,"y":220,"direction":1,"payment":"premium"
	}`))
	if err == nil || !strings.Contains(err.Error(), "allowPremium=true") {
		t.Fatalf("premium expansion error = %v", err)
	}
	step, err := resolveBuildingExpansionStep(context.Background(), input, json.RawMessage(`{
		"castleId":10,"x":220,"y":220,"direction":1,"payment":"premium","allowPremium":true
	}`))
	if err != nil {
		t.Fatal(err)
	}
	if string(step.Command.Payload) != `{"X":220,"Y":220,"R":1,"CT":0}` {
		t.Fatalf("premium expansion payload = %s", step.Command.Payload)
	}
}

func TestResolveBuildingConstructRejectsImplicitStorageConsumption(t *testing.T) {
	gameState := buildingIntentState()
	_, err := resolveBuildingPlacementStep(context.Background(), Intent.PlanningContext{
		State: gameState, GameData: buildingIntentGameData(t),
	}, json.RawMessage(`{"kind":"construct","request":{"castleId":10,"definitionId":301,"x":5,"y":5}}`))
	if err == nil {
		t.Fatal("construct unexpectedly accepted a definition already present in storage")
	}
}

func TestResolveBuildingFinishFreeAlwaysUsesExplicitFreeFlag(t *testing.T) {
	gameState := buildingIntentState()
	castle := gameState.Castles[10]
	building := castle.Layout.Objects[42]
	building.ConstructionState = State.BuildingStateBuildInProgress
	castle.Layout.Objects[42] = building
	castle.Buildings[42] = building
	castle.BuildingQueue.Slots[0] = State.BuildingConstructionQueueSlot{
		Index: 0, WireValue: 42, Status: State.BuildingQueueSlotOccupied, BuildingID: 42,
	}
	gameState.Castles[10] = castle

	step, err := resolveBuildingFinishFreeStep(context.Background(), Intent.PlanningContext{
		State: gameState, GameData: buildingIntentGameData(t),
	}, json.RawMessage(`{"castleId":10,"buildingInstanceId":42}`))
	if err != nil {
		t.Fatal(err)
	}
	if step.Command.Opcode != "fco" || string(step.Command.Payload) != `{"OID":42,"FS":1}` {
		t.Fatalf("free completion command = %s %s", step.Command.Opcode, step.Command.Payload)
	}
}

func TestResolveBuildingTimeSkipUsesOfficialCurrencyWireKey(t *testing.T) {
	gameState := buildingIntentState()
	castle := gameState.Castles[10]
	building := castle.Layout.Objects[42]
	building.ProgressSec = 15
	building.ConstructionState = State.BuildingStateUpgradeInProgress
	castle.Layout.Objects[42] = building
	castle.Buildings[42] = building
	castle.BuildingQueue.Slots[0] = State.BuildingConstructionQueueSlot{
		Index: 0, WireValue: 42, Status: State.BuildingQueueSlotOccupied, BuildingID: 42,
	}
	gameState.Castles[10] = castle

	step, err := resolveBuildingTimeSkipStep(context.Background(), Intent.PlanningContext{
		State: gameState, GameData: buildingIntentGameData(t),
	}, json.RawMessage(`{"castleId":10,"buildingInstanceId":42,"minutes":10,"minimumRemaining":5}`))
	if err != nil {
		t.Fatal(err)
	}
	if step.Command.Opcode != "msb" || string(step.Command.Payload) != `{"OID":42,"MST":"MS3"}` {
		t.Fatalf("time skip command = %s %s", step.Command.Opcode, step.Command.Payload)
	}

	plan, err := planBuildingTimeSkip(context.Background(), Intent.PlanningContext{
		State: gameState, GameData: buildingIntentGameData(t),
	}, json.RawMessage(`{"castleId":10,"buildingInstanceId":42,"minutes":10,"minimumRemaining":5}`))
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Steps) != 4 || plan.Steps[0].Resolver != "building.skip_time.build" ||
		len(plan.Steps[0].StaleCodes) != 1 || plan.Steps[0].StaleCodes[0] != 147 ||
		plan.Steps[1].Action != timeSkipConsumeAction || plan.Steps[2].Opcode != "jaa" ||
		plan.Steps[3].Action != "building.verify" {
		t.Fatalf("building time-skip plan = %#v", plan.Steps)
	}
	if len(step.StaleCodes) != 1 || step.StaleCodes[0] != 147 {
		t.Fatalf("building time-skip stale response codes = %#v", step.StaleCodes)
	}

	castle = gameState.Castles[10]
	castle.BuildingQueue.Slots[0] = State.BuildingConstructionQueueSlot{
		Index: 0, WireValue: -1, Status: State.BuildingQueueSlotAvailable,
	}
	gameState.Castles[10] = castle
	_, err = resolveBuildingTimeSkipStep(context.Background(), Intent.PlanningContext{
		State: gameState, GameData: buildingIntentGameData(t),
	}, json.RawMessage(`{"castleId":10,"buildingInstanceId":42,"minutes":10,"minimumRemaining":5}`))
	if !errors.Is(err, Intent.ErrPlanStale) {
		t.Fatalf("completed building time-skip resolution error = %v", err)
	}
}

func TestVerifyBuildingFinishFreeWaitsForAsyncCompletion(t *testing.T) {
	gameState := buildingIntentState()
	castle := gameState.Castles[10]
	building := castle.Layout.Objects[42]
	building.ProgressSec = 8
	building.ConstructionState = State.BuildingStateBuildInProgress
	castle.Layout.Objects[42] = building
	castle.Buildings[42] = building
	castle.BuildingQueue.Slots[0] = State.BuildingConstructionQueueSlot{
		Index: 0, WireValue: 42, Status: State.BuildingQueueSlotOccupied, BuildingID: 42,
	}
	gameState.Castles[10] = castle

	store := State.NewStore(gameState)
	application := &Application{State: store}
	mutationErr := make(chan error, 1)
	go func() {
		time.Sleep(10 * time.Millisecond)
		_, err := store.Apply(func(state *State.GameState) ([]string, bool, error) {
			castle := state.Castles[10]
			building := castle.Layout.Objects[42]
			building.ProgressSec = 0
			building.ConstructionState = State.BuildingStateBuildCompleted
			castle.Layout.Objects[42] = building
			castle.Buildings[42] = building
			castle.BuildingQueue.Slots[0] = State.BuildingConstructionQueueSlot{
				Index: 0, WireValue: -1, Status: State.BuildingQueueSlotAvailable,
			}
			state.Castles[10] = castle
			return []string{"castles", "building-queue"}, true, nil
		})
		mutationErr <- err
	}()

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	err := application.verifyBuildingMutation(ctx, json.RawMessage(`{
		"kind":"finish_free","castleId":10,"buildingInstanceId":42,"initialConstructionState":2
	}`))
	if err != nil {
		t.Fatal(err)
	}
	if err := <-mutationErr; err != nil {
		t.Fatal(err)
	}
}

func buildingIntentGameData(t *testing.T) *GameData.Store {
	t.Helper()
	gameData, err := GameData.DecodeStore([]byte(`{
		"versionInfo":{"version":"test"},
		"units":[],
		"resources":[
			{"resourceID":"2","JSONKey":"C2","name":"currency2"},
			{"resourceID":"3","JSONKey":"W","name":"wood"},
			{"resourceID":"4","JSONKey":"S","name":"stone"}
		],
		"currencies":[{"currencyID":"1003","Name":"10MinSkip","JSONKey":"MS3"}],
		"currencyMinutesSkipValues":[{"currencyID":"1003","MinutesSkipValue":"10","MinuteSkipIndex":"2"}],
		"expansions":[{"expansionID":"1","spaceIDs":"0","expansionLevel":"1","costWood":"1","costStone":"1","costC2":"200"}],
		"buildings":[
			{"wodID":"200","name":"Ground","group":"Ground","width":"10","height":"10"},
			{"wodID":"45","name":"Harbor","group":"FixedPositionBuilding","level":"1","width":"14","height":"7","forcedPosition":"1","upgradeWodID":"46"},
			{"wodID":"46","name":"Harbor","group":"FixedPositionBuilding","level":"2","width":"14","height":"7","forcedPosition":"1","downgradeWodID":"45","costC2":"12300"},
			{"wodID":"520","name":"TreasureChest","group":"Building","level":"1","width":"4","height":"4","movable":"0","destructable":"0"},
			{"wodID":"301","name":"StoredBuilding","group":"Building","level":"1","width":"2","height":"2","rotateType":"1","upgradeWodID":"302","storeable":"1","movable":"1","destructable":"1"},
			{"wodID":"302","name":"StoredBuilding","group":"Building","level":"2","width":"2","height":"2","rotateType":"1","downgradeWodID":"301","storeable":"1","movable":"1","destructable":"1"}
		]
	}`), GameData.SourceMetadata{ItemVersion: "test"})
	if err != nil {
		t.Fatal(err)
	}
	return gameData
}

func buildingIntentState() State.GameState {
	gameState := State.NewGameState()
	observedAt := time.Date(2026, 7, 14, 12, 0, 0, 0, time.UTC)
	ground := State.Building{InstanceID: 1, DefinitionID: 200, GridX: 0, GridY: 0, Layer: State.BuildingLayerBG, Placed: true}
	building := State.Building{InstanceID: 42, DefinitionID: 301, GridX: 1, GridY: 1, Layer: State.BuildingLayerBD, Placed: true}
	gameState.Castles[10] = State.CastleState{
		ID: 10, Focused: true,
		Buildings:         map[State.BuildingInstanceID]State.Building{1: ground, 42: building},
		ConstructionSlots: map[State.BuildingInstanceID][]State.ConstructionSlot{},
		Layout: State.CastleLayout{
			Ground: map[State.BuildingInstanceID]State.Building{1: ground}, Objects: map[State.BuildingInstanceID]State.Building{42: building},
			Fixed: map[State.BuildingInstanceID]State.Building{}, ObservedAt: observedAt,
		},
		BuildingQueue: State.BuildingConstructionQueue{
			SlotCount: 1, ObservedAt: observedAt,
			Slots: []State.BuildingConstructionQueueSlot{{Index: 0, WireValue: -1, Status: State.BuildingQueueSlotAvailable}},
		},
	}
	gameState.Inventory.Items["storage:1"] = map[int64]int64{301: 1}
	gameState.Player.Currencies[1003] = 10
	return gameState
}
