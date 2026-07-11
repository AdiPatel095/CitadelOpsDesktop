package Ingest

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"CitadelDesktop/Server/Protocol"
	"CitadelDesktop/Server/State"
)

func TestCastleSnapshotKeepsConstructionSlots(t *testing.T) {
	gameState := State.NewGameState()
	gameState.Player.ID = 42
	gameState.Castles[100] = newCastleState(100)
	payload := json.RawMessage(`{
		"KID":0,
		"gca":{
			"A":[1,10,20,100,42,0,0,0,0,0,"Main"],
			"O":{"OID":42,"N":"Player"},
			"BG":[[196,4000,1,2,0,0,0,0,0,0,0,0,0,0,8]],
			"BD":[],
			"CI":[{"OID":4000,"CIL":[{"CID":379,"S":0},{"CID":725,"S":1,"RS":60}]}]
		},
		"gui":{"I":[[215,10]],"TU":[],"HI":[],"SHI":[]}
	}`)
	code := 0
	_, changed, err := reduceCastleSnapshot(context.Background(), Protocol.Frame{
		Direction: Protocol.DirectionInbound, Opcode: "jaa", ResponseCode: &code,
		Payload: payload, ReceivedAt: time.Now().UTC(),
	}, &gameState, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !changed {
		t.Fatal("castle snapshot did not report a change")
	}
	castle := gameState.Castles[100]
	if len(castle.ConstructionSlots) != 1 || len(castle.ConstructionSlots[4000]) != 2 {
		t.Fatalf("unexpected construction slots: %#v", castle.ConstructionSlots)
	}
	if remaining := castle.ConstructionSlots[4000][1].RemainingSec; remaining == nil || *remaining != 60 {
		t.Fatalf("remaining seconds were not parsed: %#v", remaining)
	}
}

func TestConstructionSlotsSurviveLaterTransactions(t *testing.T) {
	gameState := State.NewGameState()
	gameState.Castles[100] = newCastleState(100)
	gameState.Castles[100] = func(castle State.CastleState) State.CastleState {
		castle.ConstructionSlots[4000] = []State.ConstructionSlot{{DefinitionID: 379, Slot: 0}}
		return castle
	}(gameState.Castles[100])
	store := State.NewStore(gameState)
	pipeline := NewPipeline(store, nil, NewRegistry())
	if _, err := pipeline.HandleFrame(context.Background(), Protocol.Frame{
		Direction: Protocol.DirectionInbound, Opcode: "unknown", ReceivedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatal(err)
	}
	if slots := store.Snapshot().Castles[100].ConstructionSlots[4000]; len(slots) != 1 {
		t.Fatalf("construction slots were lost after a later transaction: %#v", slots)
	}
}
