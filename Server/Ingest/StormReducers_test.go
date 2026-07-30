package Ingest

import (
	"encoding/json"
	"testing"
	"time"

	"CitadelDesktop/Server/GameData"
	"CitadelDesktop/Server/Protocol"
	"CitadelDesktop/Server/State"
)

func TestStormShopResponseReconcilesSuccessWithoutResourceSnapshot(t *testing.T) {
	gameData := stormReducerGameData(t)
	gameState := stormReducerState(10_000)
	outboundAt := time.Date(2026, 7, 22, 0, 16, 0, 0, time.UTC)
	outbound := Protocol.Frame{
		Direction: Protocol.DirectionOutbound, Opcode: "sbp", ReceivedAt: outboundAt,
		Payload: json.RawMessage(`{"PID":248,"BT":3,"TID":-1,"AMT":1,"KID":4,"AID":-1}`),
	}
	if _, changed, err := reduceStormShopCommand(t.Context(), outbound, &gameState, gameData); err != nil || !changed {
		t.Fatalf("capture Storm purchase changed=%t err=%v", changed, err)
	}
	responseCode := 0
	inbound := Protocol.Frame{
		Direction: Protocol.DirectionInbound, Opcode: "sbp", ResponseCode: &responseCode,
		ReceivedAt: outboundAt.Add(time.Second), Payload: json.RawMessage(`{"PID":248,"AMT":1}`),
	}
	if _, changed, err := reduceStormShopResponse(t.Context(), inbound, &gameState, gameData); err != nil || !changed {
		t.Fatalf("reconcile Storm purchase changed=%t err=%v", changed, err)
	}
	if got := gameState.Castles[40].Resources[GameData.StormAquamarineID].Amount; got != 8_430 {
		t.Fatalf("Aquamarine after omitted grc = %.0f, want 8430", got)
	}
	if gameState.Storm.LunaShopPending {
		t.Fatal("Storm purchase remained pending after response")
	}
}

func TestStormShopResponseUsesServerBalanceOnInsufficientResources(t *testing.T) {
	gameData := stormReducerGameData(t)
	gameState := stormReducerState(10_000)
	outboundAt := time.Date(2026, 7, 22, 0, 16, 0, 0, time.UTC)
	outbound := Protocol.Frame{
		Direction: Protocol.DirectionOutbound, Opcode: "sbp", ReceivedAt: outboundAt,
		Payload: json.RawMessage(`{"PID":248,"BT":3,"TID":-1,"AMT":1,"KID":4,"AID":-1}`),
	}
	if _, _, err := reduceStormShopCommand(t.Context(), outbound, &gameState, gameData); err != nil {
		t.Fatal(err)
	}
	responseCode := 55
	inbound := Protocol.Frame{
		Direction: Protocol.DirectionInbound, Opcode: "sbp", ResponseCode: &responseCode,
		ReceivedAt: outboundAt.Add(time.Second), Payload: json.RawMessage(`{"AS":104.0}`),
	}
	if _, changed, err := reduceStormShopResponse(t.Context(), inbound, &gameState, gameData); err != nil || !changed {
		t.Fatalf("reconcile insufficient resources changed=%t err=%v", changed, err)
	}
	if got := gameState.Castles[40].Resources[GameData.StormAquamarineID].Amount; got != 104 {
		t.Fatalf("Aquamarine from code 55 = %.0f, want 104", got)
	}
}

func stormReducerGameData(t *testing.T) *GameData.Store {
	t.Helper()
	gameData, err := GameData.DecodeStore([]byte(`{
		"versionInfo":[],"buildings":[],"units":[],
		"packages":[{"packageID":248,"comment1":"Mead","comment2":"Luna's trade boat","packageType":"resource","packagePriceAquamarine":1570}]
	}`), GameData.SourceMetadata{ItemVersion: "test"})
	if err != nil {
		t.Fatal(err)
	}
	return gameData
}

func stormReducerState(aquamarine float64) State.GameState {
	gameState := State.NewGameState()
	gameState.Castles[40] = State.CastleState{
		ID: 40, KingdomID: GameData.StormKingdomID, Focused: true,
		Resources: map[State.ResourceID]State.ResourceBalance{
			GameData.StormAquamarineID: {Amount: aquamarine},
		},
	}
	return gameState
}
