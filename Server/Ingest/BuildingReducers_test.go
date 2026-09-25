package Ingest

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"CitadelDesktop/Server/Protocol"
	"CitadelDesktop/Server/State"
)

func TestBuildingProductionReducerKeepsBreweryPercentage(t *testing.T) {
	gameState := State.NewGameState()
	castle := newCastleState(100)
	castle.Focused = true
	gameState.Castles[castle.ID] = castle
	code := 0
	observedAt := time.Date(2026, 7, 27, 20, 0, 0, 0, time.UTC)

	_, changed, err := reduceBuildingProduction(context.Background(), Protocol.Frame{
		Direction: Protocol.DirectionInbound, Opcode: "abpi", ResponseCode: &code, ReceivedAt: observedAt,
		Payload: json.RawMessage(`{"OID":4126,"PA":{"MEAD":45},"MS":{"F":0,"HONEY":0}}`),
	}, &gameState, nil)
	if err != nil {
		t.Fatal(err)
	}
	production := gameState.Castles[100].BuildingProduction[4126]
	if !changed || production.PercentByResource["MEAD"] != 45 || !production.ObservedAt.Equal(observedAt) {
		t.Fatalf("brewery production percentage was not retained: changed=%t production=%#v", changed, production)
	}
}

func TestBuildingMutationReducersReconcileLayoutQueueAndRemoval(t *testing.T) {
	gameState := State.NewGameState()
	castle := newCastleState(100)
	castle.Focused = true
	gameState.Castles[castle.ID] = castle
	code := 0
	observedAt := time.Date(2026, 7, 14, 12, 0, 0, 0, time.UTC)

	_, changed, err := reduceBuildingMutation(context.Background(), Protocol.Frame{
		Direction: Protocol.DirectionInbound, Opcode: "ebu", ResponseCode: &code, ReceivedAt: observedAt,
		Payload: json.RawMessage(`{"NO":[301,43,10,20,1,0,4,100,-1,-1,0,0,0,0,1,-1],"scl":{"OIDL":[43,-1],"SSC":2}}`),
	}, &gameState, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !changed {
		t.Fatal("buy-object response did not report a state change")
	}
	castle = gameState.Castles[100]
	building, found := castle.Layout.Objects[43]
	if !found || building.DefinitionID != 301 || building.GridX != 10 || building.GridY != 20 || building.Rotation != 1 || !building.Placed {
		t.Fatalf("unexpected constructed building: %#v", building)
	}
	if len(castle.BuildingQueue.Slots) != 2 || castle.BuildingQueue.Slots[0].BuildingID != 43 ||
		castle.BuildingQueue.Slots[1].Status != State.BuildingQueueSlotAvailable || !castle.BuildingQueue.ObservedAt.Equal(observedAt) {
		t.Fatalf("unexpected building queue: %#v", castle.BuildingQueue)
	}

	castle.ConstructionSlots[43] = []State.ConstructionSlot{{DefinitionID: 725, Slot: 1}}
	gameState.Castles[100] = castle
	_, changed, err = reduceBuildingMutation(context.Background(), Protocol.Frame{
		Direction: Protocol.DirectionInbound, Opcode: "emo", ResponseCode: &code, ReceivedAt: observedAt.Add(time.Second),
		Payload: json.RawMessage(`{"MO":[301,43,12,21,2,0,4,100,-1,-1,0,0,0,0,1,-1]}`),
	}, &gameState, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !changed {
		t.Fatal("move-object response did not report a state change")
	}
	castle = gameState.Castles[100]
	building = castle.Layout.Objects[43]
	if building.GridX != 12 || building.GridY != 21 || building.Rotation != 2 {
		t.Fatalf("unexpected moved building: %#v", building)
	}
	if slots := castle.ConstructionSlots[43]; len(slots) != 1 || slots[0].DefinitionID != 725 {
		t.Fatalf("building update discarded construction items: %#v", slots)
	}

	_, changed, err = reduceBuildingMutation(context.Background(), Protocol.Frame{
		Direction: Protocol.DirectionInbound, Opcode: "eup", ResponseCode: &code, ReceivedAt: observedAt.Add(2 * time.Second),
		Payload: json.RawMessage(`{"O":[[301,43,12,21,2,0,13,100,200,-1,0,0,0,0,1,-1]],"scl":{"OIDL":[43,-1],"SSC":2}}`),
	}, &gameState, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !changed || gameState.Castles[100].Layout.Objects[43].ConstructionState != State.BuildingStateUpgradeInProgress {
		t.Fatalf("upgrade start was not reconciled: %#v", gameState.Castles[100].Layout.Objects[43])
	}

	_, changed, err = reduceBuildingMutation(context.Background(), Protocol.Frame{
		Direction: Protocol.DirectionInbound, Opcode: "ego", ResponseCode: &code, ReceivedAt: observedAt.Add(3 * time.Second),
		Payload: json.RawMessage(`{"F":1,"O":[302,43,12,21,2,0,4,100,-1,-1,0,0,0,0,2,-1]}`),
	}, &gameState, nil)
	if err != nil {
		t.Fatal(err)
	}
	upgraded := gameState.Castles[100].Layout.Objects[43]
	if !changed || upgraded.DefinitionID != 302 || upgraded.ConstructionState != State.BuildingStateBuildCompleted || upgraded.Level != 2 {
		t.Fatalf("upgrade completion was not reconciled: %#v", upgraded)
	}
	if slots := gameState.Castles[100].ConstructionSlots[43]; len(slots) != 1 || slots[0].DefinitionID != 725 {
		t.Fatalf("upgrade completion discarded construction items: %#v", slots)
	}

	_, changed, err = reduceBuildingMutation(context.Background(), Protocol.Frame{
		Direction: Protocol.DirectionInbound, Opcode: "sob", ResponseCode: &code, ReceivedAt: observedAt.Add(4 * time.Second),
		Payload: json.RawMessage(`{"OID":43}`),
	}, &gameState, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !changed {
		t.Fatal("store-object response did not report a state change")
	}
	castle = gameState.Castles[100]
	if _, found := castle.Layout.Objects[43]; found {
		t.Fatalf("stored building remains in layout: %#v", castle.Layout.Objects[43])
	}
	if _, found := castle.ConstructionSlots[43]; found {
		t.Fatalf("stored building retains construction slots: %#v", castle.ConstructionSlots[43])
	}
}

func TestBuildingMutationReducerRemovesCompletedDisassembly(t *testing.T) {
	gameState := State.NewGameState()
	castle := newCastleState(100)
	castle.Focused = true
	building := State.Building{
		InstanceID: 43, DefinitionID: 301, GridX: 12, GridY: 21, Layer: State.BuildingLayerBD,
		ConstructionState: State.BuildingStateDisassembleInProgress, Placed: true,
	}
	castle.Buildings[43] = building
	castle.Layout.Objects[43] = building
	castle.ConstructionSlots[43] = []State.ConstructionSlot{{DefinitionID: 725, Slot: 1}}
	gameState.Castles[castle.ID] = castle
	code := 0

	_, changed, err := reduceBuildingMutation(context.Background(), Protocol.Frame{
		Direction: Protocol.DirectionInbound, Opcode: "ego", ResponseCode: &code, ReceivedAt: time.Now().UTC(),
		Payload: json.RawMessage(`{"F":0,"O":[301,43,12,21,0,4,8,100,100,-1,0,0,0,0,1,-1]}`),
	}, &gameState, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !changed {
		t.Fatal("completed disassembly did not report a state change")
	}
	castle = gameState.Castles[100]
	if _, found := castle.Layout.Objects[43]; found {
		t.Fatalf("completed disassembly remains in layout: %#v", castle.Layout.Objects[43])
	}
	if _, found := castle.ConstructionSlots[43]; found {
		t.Fatalf("completed disassembly retains construction items: %#v", castle.ConstructionSlots[43])
	}
}

func TestBuildingMutationReducerRemovesCollectedExpansionGift(t *testing.T) {
	gameState := State.NewGameState()
	castle := newCastleState(100)
	castle.Focused = true
	chest := State.Building{
		InstanceID: 46, DefinitionID: 520, GridX: 8, GridY: 3,
		ConstructionState: State.BuildingStateBuildCompleted, Layer: State.BuildingLayerBD, Placed: true,
	}
	castle.Buildings[46] = chest
	castle.Layout.Objects[46] = chest
	gameState.Castles[castle.ID] = castle
	code := 0

	_, changed, err := reduceBuildingMutation(context.Background(), Protocol.Frame{
		Direction: Protocol.DirectionInbound, Opcode: "etc", ResponseCode: &code, ReceivedAt: time.Now().UTC(),
		Payload: json.RawMessage(`{"RID":1114,"OID":46}`),
	}, &gameState, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !changed {
		t.Fatal("extension treasure collection did not report a state change")
	}
	castle = gameState.Castles[100]
	if _, found := castle.Layout.Objects[46]; found {
		t.Fatalf("collected expansion gift remains in layout: %#v", castle.Layout.Objects[46])
	}
}

func TestBuildingConstructionBoostFullAndSparseRows(t *testing.T) {
	raw := json.RawMessage(`[[249,15,1,2,0,3622,6,100,100,-1,0,0,0,0,1,-1]]`)
	buildings, layout := parseCastleLayout(nil, raw, nil)
	if buildings[15].ConstructionBoostPercent != 100 || layout.Objects[15].ConstructionBoostPercent != 100 {
		t.Fatal("full JAA lost row8 construction boost")
	}
	castle := State.CastleState{Buildings: buildings, Layout: layout}
	var row []json.RawMessage
	_ = json.Unmarshal([]byte(`[249,15,1,2,0,4500,6,100]`), &row)
	applyBuildingMutationRow(&castle, row, nil)
	if castle.Layout.Objects[15].ConstructionBoostPercent != 100 {
		t.Fatal("sparse mutation cleared known construction boost")
	}
	_ = json.Unmarshal([]byte(`[249,15,1,2,0,100,6,100,200]`), &row)
	applyBuildingMutationRow(&castle, row, nil)
	if castle.Layout.Objects[15].ConstructionBoostPercent != 200 {
		t.Fatal("mutation did not update construction boost")
	}
	_ = json.Unmarshal([]byte(`[249,15,1,2,0,100,6,100,"invalid"]`), &row)
	applyBuildingMutationRow(&castle, row, nil)
	if castle.Layout.Objects[15].ConstructionBoostPercent != 0 {
		t.Fatal("malformed present boost retained stale value")
	}
	previous := castle.Layout.Objects[15]
	previous.ConstructionBoostPercent = 100
	previous.ConstructionState = State.BuildingStateBuildCompleted
	castle.Layout.Objects[15], castle.Buildings[15] = previous, previous
	_ = json.Unmarshal([]byte(`[249,15,1,2,0,0,6,100]`), &row)
	applyBuildingMutationRow(&castle, row, nil)
	if castle.Layout.Objects[15].ConstructionBoostPercent != 0 {
		t.Fatal("new demolition inherited earlier operation boost")
	}
	buildings, _ = parseCastleLayout(nil, json.RawMessage(`[[249,15,1,2,0,100,6,100]]`), nil)
	if buildings[15].ConstructionBoostPercent != 0 {
		t.Fatal("full snapshot fabricated missing boost")
	}
}
