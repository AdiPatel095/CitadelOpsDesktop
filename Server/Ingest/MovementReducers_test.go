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
			{"M":{"MID":51,"PT":2,"TT":20,"D":0,"T":0,"KID":0,"OID":1,"TID":99,"SA":[0,10,11,100,1],"TA":[0,20,21,300,99]},"UM":{"L":{"ID":8}}}
		]}`),
	}, &gameState, nil)
	if err != nil || !changed {
		t.Fatalf("movement snapshot: changed=%t err=%v", changed, err)
	}
	if len(gameState.Movements) != 2 || gameState.Movements[50].TargetPlayerID != 99 {
		t.Fatalf("foreign target movement was not retained: %#v", gameState.Movements)
	}
	if !gameState.Commanders[7].Available {
		t.Fatal("foreign movement marked the current player's commander unavailable")
	}
	if gameState.Commanders[8].Available {
		t.Fatal("current player movement did not mark its commander unavailable")
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
