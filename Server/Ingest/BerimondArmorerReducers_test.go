package Ingest

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"CitadelDesktop/Server/GameData"
	"CitadelDesktop/Server/Protocol"
	"CitadelDesktop/Server/State"
)

func TestSBPResponseRefreshesFocusedBerimondToolInventory(t *testing.T) {
	gameData, err := GameData.DecodeStore([]byte(`{
		"versionInfo":[],
		"buildings":[],
		"units":[{"wodID":10},{"wodID":611},{"wodID":614},{"wodID":620}]
	}`), GameData.SourceMetadata{ItemVersion: "test"})
	if err != nil {
		t.Fatal(err)
	}
	gameState := State.NewGameState()
	gameState.Castles[989] = State.CastleState{
		ID: 989, KingdomID: 10, Focused: true,
		Units: State.CastleUnits{Stationed: map[State.UnitID]int64{614: 1}},
	}
	registry := NewRegistry()
	if err := RegisterCoreReducers(registry); err != nil {
		t.Fatal(err)
	}
	store := State.NewStore(gameState)
	pipeline := NewPipeline(store, staticGameDataProvider{store: gameData}, registry)
	observedAt := time.Date(2026, 7, 29, 21, 13, 23, 0, time.UTC)
	code := 0
	_, err = pipeline.HandleFrame(context.Background(), Protocol.Frame{
		Opcode: "sbp", Direction: Protocol.DirectionInbound, ResponseCode: &code, ReceivedAt: observedAt,
		Payload: json.RawMessage(`{
			"gcu":{"C1":1001320344},
			"gui":{"I":[[10,818],[614,2],[611,1]],"SHI":[],"HI":[],"TU":[]},
			"grc":{"AID":989},
			"PID":28,"AMT":1
		}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	castle := store.Snapshot().Castles[989]
	if castle.Units.Stationed[614] != 2 || castle.Units.Stationed[611] != 1 {
		t.Fatalf("sbp tool inventory was not applied: %#v", castle.Units.Stationed)
	}
	if !castle.UnitsObservedAt.Equal(observedAt) {
		t.Fatalf("units observed at = %s, want %s", castle.UnitsObservedAt, observedAt)
	}

	_, err = pipeline.HandleFrame(context.Background(), Protocol.Frame{
		Opcode: "sbp", Direction: Protocol.DirectionInbound, ResponseCode: &code, ReceivedAt: observedAt.Add(time.Second),
		Payload: json.RawMessage(`{
			"gui":{"I":[[614,99]],"SHI":[],"HI":[],"TU":[]},
			"grc":{"AID":999},
			"PID":28,"AMT":1
		}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	castle = store.Snapshot().Castles[989]
	if castle.Units.Stationed[614] != 2 || !castle.UnitsObservedAt.Equal(observedAt) {
		t.Fatalf("mismatched shop castle overwrote focused inventory: %#v at %s", castle.Units.Stationed, castle.UnitsObservedAt)
	}
}
