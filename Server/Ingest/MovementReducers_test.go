package Ingest

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"CitadelDesktop/Server/Protocol"
	"CitadelDesktop/Server/State"
)

func TestMovementReducerRetainsForeignMovementsWithoutOccupyingOwnCommanders(t *testing.T) {
	gameState := State.NewGameState()
	gameState.Player.ID = 1
	gameState.Castles[100] = newCastleState(100)
	gameState.Commanders[7] = State.CommanderState{ID: 7, Available: true}
	gameState.Commanders[8] = State.CommanderState{ID: 8, Available: true}
	code := 0
	_, changed, err := newMovementReducer(true)(context.Background(), Protocol.Frame{
		Opcode: "gam", Direction: Protocol.DirectionInbound, ResponseCode: &code, ReceivedAt: time.Now().UTC(),
		Payload: json.RawMessage(`{"M":[
			{"M":{"MID":50,"PT":2,"TT":20,"D":0,"T":0,"KID":0,"OID":2,"TID":99,"SA":[0,10,11,200,2],"TA":[0,20,21,300,99]},"UM":{"L":{"ID":7}}},
			{"M":{"MID":51,"PT":2,"TT":20,"D":0,"T":0,"KID":0,"OID":1,"TID":99,"SA":[0,10,11,100,1],"TA":[0,20,21,300,99]},"UM":{"TWD":21600,"L":{"ID":8}}}
		]}`),
	}, &gameState, nil)
	if err != nil || !changed {
		t.Fatalf("movement snapshot: changed=%t err=%v", changed, err)
	}
	if len(gameState.Movements) != 2 || gameState.Movements[50].TargetPlayerID != 99 {
		t.Fatalf("foreign target movement was not retained: %#v", gameState.Movements)
	}
	if gameState.Movements[51].WaitSeconds != 21600 {
		t.Fatalf("station wait = %d, want 21600", gameState.Movements[51].WaitSeconds)
	}
	if !gameState.Commanders[7].Available {
		t.Fatal("foreign movement marked the current player's commander unavailable")
	}
	if gameState.Commanders[8].Available {
		t.Fatal("current player movement did not mark its commander unavailable")
	}
}

func TestParseMovementKeepsGameReportedStationWaitActive(t *testing.T) {
	observedAt := time.Date(2026, 7, 22, 16, 0, 0, 0, time.UTC)
	movement, ok := parseMovement(json.RawMessage(`{
		"M":{"MID":51,"PT":1163,"TT":896,"D":0,"T":1,"KID":0,"OID":1,"TID":99,"SA":[0,10,11,100,1],"TA":[0,20,21,300,99]},
		"UM":{"PWD":267,"TWD":21600,"L":{"ID":8}}
	}`), observedAt, nil)
	if !ok {
		t.Fatal("station movement did not parse")
	}
	wantArrival := observedAt.Add(-267 * time.Second)
	if movement.ArrivesAt == nil || !movement.ArrivesAt.Equal(wantArrival) {
		t.Fatalf("station arrival = %v, want %s", movement.ArrivesAt, wantArrival)
	}
	if movement.WaitSeconds != 21600 {
		t.Fatalf("station wait = %d, want 21600", movement.WaitSeconds)
	}
	if !movementActiveAt(movement, observedAt) {
		t.Fatal("station movement expired while the game-reported wait was active")
	}
}

func TestReconcileExpiredMovementsReleasesCompletedStationWait(t *testing.T) {
	now := time.Now().UTC()
	arrivedAt := now.Add(-2 * time.Hour)
	gameState := State.NewGameState()
	gameState.Movements[50] = State.MovementState{
		ID: 50, Direction: 0, TravelSeconds: 600, WaitSeconds: 3600, ArrivesAt: &arrivedAt,
	}

	if !ReconcileExpiredMovements(&gameState, now) {
		t.Fatal("completed station wait was not reconciled")
	}
	if _, exists := gameState.Movements[50]; exists {
		t.Fatal("completed station wait remained active past its projected return")
	}
}

func TestMovementReducerPreservesPopulatedSnapshotAcrossTrailingEmptyFrame(t *testing.T) {
	gameState := State.NewGameState()
	gameState.Player.ID = 1
	gameState.Castles[100] = newCastleState(100)
	gameState.Commanders[7] = State.CommanderState{ID: 7, Available: true}
	code := 0
	receivedAt := time.Now().UTC()
	reducer := newMovementReducer(true)

	_, _, err := reducer(context.Background(), Protocol.Frame{
		Opcode: "gam", Direction: Protocol.DirectionInbound, ResponseCode: &code, ReceivedAt: receivedAt,
		Payload: json.RawMessage(`{"M":[
			{"M":{"MID":50,"PT":2,"TT":20,"D":0,"T":0,"KID":0,"OID":1,"TID":99,"SA":[0,10,11,100,1],"TA":[0,20,21,300,99]},"UM":{"L":{"ID":7}}}
		]}`),
	}, &gameState, nil)
	if err != nil {
		t.Fatalf("populated movement frame: %v", err)
	}

	_, _, err = reducer(context.Background(), Protocol.Frame{
		Opcode: "gam", Direction: Protocol.DirectionInbound, ResponseCode: &code,
		ReceivedAt: receivedAt.Add(50 * time.Millisecond), Payload: json.RawMessage(`{"M":[],"O":[]}`),
	}, &gameState, nil)
	if err != nil {
		t.Fatalf("trailing empty movement frame: %v", err)
	}
	if _, exists := gameState.Movements[50]; !exists {
		t.Fatal("trailing empty gam frame erased the populated movement snapshot")
	}
	if gameState.Commanders[7].Available {
		t.Fatal("trailing empty gam frame released an active commander")
	}
}

func TestMovementReducerFirstLiveSnapshotDropsRecoveredMovement(t *testing.T) {
	now := time.Now().UTC()
	arrivesAt := now.Add(-time.Second)
	gameState := State.NewGameState()
	gameState.Player.ID = 1
	gameState.Castles[100] = newCastleState(100)
	gameState.Session.ConnectionGeneration = 1
	gameState.Commanders[7] = State.CommanderState{ID: 7, Available: false}
	commanderID := State.CommanderID(7)
	gameState.Movements[50] = State.MovementState{
		ID: 50, Direction: 0, OwnerPlayerID: 1, SourceCastleID: 100,
		CommanderID: &commanderID, TravelSeconds: 800, ArrivesAt: &arrivesAt,
		ObservedAt: now.Add(-10 * time.Minute),
	}
	code := 0

	_, _, err := newMovementReducer(true)(context.Background(), Protocol.Frame{
		Opcode: "gam", Direction: Protocol.DirectionInbound, ResponseCode: &code,
		ReceivedAt: now, Payload: json.RawMessage(`{"M":[],"O":[]}`),
	}, &gameState, nil)
	if err != nil {
		t.Fatalf("first live movement snapshot: %v", err)
	}
	if len(gameState.Movements) != 0 || !gameState.Commanders[7].Available {
		t.Fatalf("recovered movement survived live baseline: movements=%+v commander=%+v", gameState.Movements, gameState.Commanders[7])
	}
}

func TestMovementReducerNewConnectionDropsPriorMovement(t *testing.T) {
	gameState := State.NewGameState()
	gameState.Player.ID = 1
	gameState.Castles[100] = newCastleState(100)
	gameState.Session.ConnectionGeneration = 1
	gameState.Commanders[7] = State.CommanderState{ID: 7, Available: true}
	code := 0
	now := time.Now().UTC()
	reducer := newMovementReducer(true)

	_, _, err := reducer(context.Background(), Protocol.Frame{
		Opcode: "gam", Direction: Protocol.DirectionInbound, ResponseCode: &code, ReceivedAt: now,
		Payload: json.RawMessage(`{"M":[
			{"M":{"MID":50,"PT":2,"TT":800,"D":0,"T":0,"KID":4,"OID":1,"TID":-1,"SA":[12,10,11,100,1],"TA":[25,20,21,4,-1]},"UM":{"L":{"ID":7}}}
		]}`),
	}, &gameState, nil)
	if err != nil {
		t.Fatalf("first connection movement snapshot: %v", err)
	}

	gameState.Session.ConnectionGeneration = 2
	_, _, err = reducer(context.Background(), Protocol.Frame{
		Opcode: "gam", Direction: Protocol.DirectionInbound, ResponseCode: &code,
		ReceivedAt: now.Add(time.Second), Payload: json.RawMessage(`{"M":[],"O":[]}`),
	}, &gameState, nil)
	if err != nil {
		t.Fatalf("new connection movement snapshot: %v", err)
	}
	if len(gameState.Movements) != 0 || !gameState.Commanders[7].Available {
		t.Fatalf("prior connection movement survived new baseline: movements=%+v commander=%+v", gameState.Movements, gameState.Commanders[7])
	}
}

func TestMovementReducerPreservesOwnedCommanderAcrossScopedSnapshotOmission(t *testing.T) {
	gameState := State.NewGameState()
	gameState.Player.ID = 1
	gameState.Castles[100] = newCastleState(100)
	gameState.Commanders[7] = State.CommanderState{ID: 7, Available: true}
	code := 0
	receivedAt := time.Now().UTC()
	reducer := newMovementReducer(true)

	_, _, err := reducer(context.Background(), Protocol.Frame{
		Opcode: "gam", Direction: Protocol.DirectionInbound, ResponseCode: &code, ReceivedAt: receivedAt,
		Payload: json.RawMessage(`{"M":[
			{"M":{"MID":50,"PT":2,"TT":20,"D":0,"T":0,"KID":0,"OID":1,"TID":99,"SA":[0,10,11,100,1],"TA":[0,20,21,300,99]},"UM":{"L":{"ID":7}}}
		]}`),
	}, &gameState, nil)
	if err != nil {
		t.Fatalf("populated movement frame: %v", err)
	}

	_, _, err = reducer(context.Background(), Protocol.Frame{
		Opcode: "gam", Direction: Protocol.DirectionInbound, ResponseCode: &code,
		ReceivedAt: receivedAt.Add(time.Second), Payload: json.RawMessage(`{"M":[],"O":[]}`),
	}, &gameState, nil)
	if err != nil {
		t.Fatalf("standalone empty movement frame: %v", err)
	}
	if _, exists := gameState.Movements[50]; !exists {
		t.Fatal("scoped gam frame erased an active owned commander movement")
	}
	if gameState.Commanders[7].Available {
		t.Fatal("scoped gam frame released an active owned commander")
	}

	_, _, err = reducer(context.Background(), Protocol.Frame{
		Opcode: "gam", Direction: Protocol.DirectionInbound, ResponseCode: &code,
		ReceivedAt: receivedAt.Add(time.Minute), Payload: json.RawMessage(`{"M":[],"O":[]}`),
	}, &gameState, nil)
	if err != nil {
		t.Fatalf("post-return empty movement frame: %v", err)
	}
	if len(gameState.Movements) != 0 {
		t.Fatalf("post-return gam frame retained completed movement: %#v", gameState.Movements)
	}
	if !gameState.Commanders[7].Available {
		t.Fatal("post-return gam frame did not release the commander")
	}
}

func TestMovementReducerPreservesMarketBarrowLeaseAcrossScopedSnapshotOmission(t *testing.T) {
	gameState := State.NewGameState()
	gameState.Player.ID = 1
	gameState.Castles[100] = newCastleState(100)
	code := 0
	receivedAt := time.Now().UTC()
	reducer := newMovementReducer(true)

	_, _, err := reducer(context.Background(), Protocol.Frame{
		Opcode: "gam", Direction: Protocol.DirectionInbound, ResponseCode: &code, ReceivedAt: receivedAt,
		Payload: json.RawMessage(`{"M":[
			{"M":{"MID":50,"PT":2,"TT":20,"D":0,"T":0,"KID":0,"OID":1,"TID":1,"SA":[0,10,11,100,1],"TA":[0,20,21,200,1]},"MM":{"C":10,"G":[]}}
		]}`),
	}, &gameState, nil)
	if err != nil {
		t.Fatalf("market movement frame: %v", err)
	}

	_, _, err = reducer(context.Background(), Protocol.Frame{
		Opcode: "gam", Direction: Protocol.DirectionInbound, ResponseCode: &code,
		ReceivedAt: receivedAt.Add(time.Second), Payload: json.RawMessage(`{"M":[],"O":[]}`),
	}, &gameState, nil)
	if err != nil {
		t.Fatalf("scoped empty movement frame: %v", err)
	}
	if _, exists := gameState.Movements[50]; !exists {
		t.Fatal("scoped gam frame released active market barrows")
	}

	_, _, err = reducer(context.Background(), Protocol.Frame{
		Opcode: "gam", Direction: Protocol.DirectionInbound, ResponseCode: &code,
		ReceivedAt: receivedAt.Add(time.Minute), Payload: json.RawMessage(`{"M":[],"O":[]}`),
	}, &gameState, nil)
	if err != nil {
		t.Fatalf("post-return empty movement frame: %v", err)
	}
	if _, exists := gameState.Movements[50]; exists {
		t.Fatal("completed market barrow lease survived its return time")
	}
}

func TestMovementReducerPreservesStationLeaseAcrossScopedSnapshotOmission(t *testing.T) {
	gameState := State.NewGameState()
	gameState.Player.ID = 1
	gameState.Castles[100] = newCastleState(100)
	gameState.Stationing["autoBird:100"] = State.StationingOperation{
		ID: "autoBird:100", Purpose: "autoBird", SourceCastleID: 100, TargetCastleID: 200,
	}
	code := 0
	receivedAt := time.Now().UTC()
	reducer := newMovementReducer(true)

	_, _, err := reducer(context.Background(), Protocol.Frame{
		Opcode: "gam", Direction: Protocol.DirectionInbound, ResponseCode: &code, ReceivedAt: receivedAt,
		Payload: json.RawMessage(`{"M":[
			{"M":{"MID":50,"PT":2,"TT":600,"D":0,"T":1,"KID":0,"OID":1,"TID":99,"SA":[0,10,11,100,1],"TA":[0,20,21,200,99]},"UM":{"TWD":21600,"L":{"ID":-14}}}
		]}`),
	}, &gameState, nil)
	if err != nil {
		t.Fatalf("station movement frame: %v", err)
	}

	_, _, err = reducer(context.Background(), Protocol.Frame{
		Opcode: "gam", Direction: Protocol.DirectionInbound, ResponseCode: &code,
		ReceivedAt: receivedAt.Add(time.Minute), Payload: json.RawMessage(`{"M":[],"O":[]}`),
	}, &gameState, nil)
	if err != nil {
		t.Fatalf("scoped empty movement frame: %v", err)
	}
	if _, exists := gameState.Movements[50]; !exists {
		t.Fatal("scoped gam frame released active AutoBird stationing")
	}

	_, _, err = reducer(context.Background(), Protocol.Frame{
		Opcode: "gam", Direction: Protocol.DirectionInbound, ResponseCode: &code,
		ReceivedAt: receivedAt.Add(6*time.Hour + 20*time.Minute), Payload: json.RawMessage(`{"M":[],"O":[]}`),
	}, &gameState, nil)
	if err != nil {
		t.Fatalf("post-return empty movement frame: %v", err)
	}
	if _, exists := gameState.Movements[50]; exists {
		t.Fatal("completed AutoBird station lease survived its projected return")
	}
}

func TestMovementReducerReplacesRetainedOutboundWithObservedReturn(t *testing.T) {
	gameState := State.NewGameState()
	gameState.Player.ID = 1
	gameState.Castles[100] = newCastleState(100)
	gameState.Commanders[7] = State.CommanderState{ID: 7, Available: true}
	code := 0
	observedAt := time.Now().UTC()
	reducer := newMovementReducer(true)

	_, _, err := reducer(context.Background(), Protocol.Frame{
		Opcode: "gam", Direction: Protocol.DirectionInbound, ResponseCode: &code, ReceivedAt: observedAt,
		Payload: json.RawMessage(`{"M":[
			{"M":{"MID":50,"PT":795,"TT":800,"D":0,"T":0,"KID":4,"OID":1,"TID":-1,"SA":[12,10,11,100,1],"TA":[25,20,21,4,-1]},"UM":{"L":{"ID":7}}}
		]}`),
	}, &gameState, nil)
	if err != nil {
		t.Fatalf("outbound movement frame: %v", err)
	}
	if _, exists := gameState.Movements[50]; !exists {
		t.Fatal("outbound movement was not retained before its return was observed")
	}

	returnObservedAt := observedAt.Add(time.Second)
	_, _, err = reducer(context.Background(), Protocol.Frame{
		Opcode: "gam", Direction: Protocol.DirectionInbound, ResponseCode: &code, ReceivedAt: returnObservedAt,
		Payload: json.RawMessage(`{"M":[
			{"M":{"MID":51,"PT":10,"TT":170,"D":1,"T":2,"KID":4,"OID":1,"TID":1,"SA":[25,20,21,4,-1],"TA":[12,10,11,100,1]},"UM":{"L":{"ID":7}}}
		]}`),
	}, &gameState, nil)
	if err != nil {
		t.Fatalf("return movement frame: %v", err)
	}
	if _, exists := gameState.Movements[50]; exists {
		t.Fatal("observed return left the superseded outbound movement active")
	}
	returnMovement, exists := gameState.Movements[51]
	if !exists || returnMovement.ReturnsAt == nil {
		t.Fatalf("observed return movement = %+v, exists=%t", returnMovement, exists)
	}
	if gameState.Commanders[7].Available {
		t.Fatal("commander was released before the observed return completed")
	}

	ReconcileExpiredMovements(&gameState, returnMovement.ReturnsAt.Add(time.Second))
	if len(gameState.Movements) != 0 || !gameState.Commanders[7].Available {
		t.Fatalf("completed return did not release commander: movements=%+v commander=%+v", gameState.Movements, gameState.Commanders[7])
	}
}

func TestCommanderAvailabilityUsesOwnedSourceWhenMovementOmitsOwner(t *testing.T) {
	gameState := State.NewGameState()
	gameState.Castles[100] = newCastleState(100)
	commanderID := State.CommanderID(7)
	gameState.Movements[50] = State.MovementState{ID: 50, SourceCastleID: 100, CommanderID: &commanderID}
	if commanderAvailable(&gameState, commanderID) {
		t.Fatal("movement from an owned source did not occupy its commander")
	}
}

func TestReconcileExpiredMovementsFreesCommander(t *testing.T) {
	now := time.Now().UTC()
	returnedAt := now.Add(-time.Second)
	activeUntil := now.Add(time.Minute)
	gameState := State.NewGameState()
	gameState.Player.ID = 1
	gameState.Castles[100] = newCastleState(100)
	gameState.Commanders[7] = State.CommanderState{ID: 7, Available: false}
	gameState.Commanders[8] = State.CommanderState{ID: 8, Available: false}
	commander7 := State.CommanderID(7)
	commander8 := State.CommanderID(8)
	gameState.Movements[50] = State.MovementState{
		ID: 50, Direction: 1, OwnerPlayerID: 1, TargetCastleID: 100,
		CommanderID: &commander7, ReturnsAt: &returnedAt,
	}
	gameState.Movements[51] = State.MovementState{
		ID: 51, Direction: 1, OwnerPlayerID: 1, TargetCastleID: 100,
		CommanderID: &commander8, ReturnsAt: &activeUntil,
	}

	if !ReconcileExpiredMovements(&gameState, now) {
		t.Fatal("expired movement reconciliation reported no change")
	}
	if _, exists := gameState.Movements[50]; exists {
		t.Fatal("expired return movement was retained")
	}
	if _, exists := gameState.Movements[51]; !exists {
		t.Fatal("active return movement was removed")
	}
	if !gameState.Commanders[7].Available {
		t.Fatal("returned commander was not released")
	}
	if gameState.Commanders[8].Available {
		t.Fatal("active commander was released early")
	}
}

func TestReconcileExpiredMovementsKeepsOutboundCommanderForReturnTrip(t *testing.T) {
	now := time.Now().UTC()
	arrivedAt := now.Add(-5 * time.Second)
	gameState := State.NewGameState()
	gameState.Player.ID = 1
	gameState.Castles[100] = newCastleState(100)
	gameState.Commanders[7] = State.CommanderState{ID: 7, Available: false}
	commanderID := State.CommanderID(7)
	gameState.Movements[50] = State.MovementState{
		ID: 50, Direction: 0, OwnerPlayerID: 1, SourceCastleID: 100,
		CommanderID: &commanderID, TravelSeconds: 20, ArrivesAt: &arrivedAt,
	}

	if ReconcileExpiredMovements(&gameState, now) {
		t.Fatal("outbound movement was reconciled before its estimated return")
	}
	if _, exists := gameState.Movements[50]; !exists || gameState.Commanders[7].Available {
		t.Fatalf("outbound commander was released early: movement=%t commander=%+v", exists, gameState.Commanders[7])
	}
}

func TestParseMovementCapturesActiveSpyCount(t *testing.T) {
	raw := json.RawMessage(`{
		"M":{"MID":50,"PT":2,"TT":20,"D":0,"T":3,"KID":0,"OID":1,"TID":2,"SA":[2,10,11,100,1],"TA":[2,20,21,200,2]},
		"S":{"SC":7}
	}`)
	movement, ok := parseMovement(raw, time.Now().UTC(), nil)
	if !ok {
		t.Fatal("spy movement did not parse")
	}
	if movement.TypeID != 3 || movement.SourceTypeID != 2 || movement.TargetTypeID != 2 || movement.SpyCount != 7 || movement.SourceCastleID != 100 {
		t.Fatalf("spy movement = %+v", movement)
	}
}

func TestParseMovementCapturesOfficialMarketGoods(t *testing.T) {
	raw := json.RawMessage(`{
		"M":{"MID":80,"PT":3,"TT":120,"D":0,"KID":0,"SA":[0,10,20,100,1],"TA":[0,30,40,200,1]},
		"MM":{"C":4,"G":[["W",2400]]}
	}`)
	movement, ok := parseMovement(raw, time.Now().UTC(), runtimeTestGameData(t))
	if !ok {
		t.Fatal("market movement did not parse")
	}
	if movement.MarketBarrows != 4 || len(movement.MarketGoods) != 1 || movement.MarketGoods[0].ResourceID != 3 || movement.MarketGoods[0].Amount != 2400 {
		t.Fatalf("market movement = %+v", movement)
	}
}
