package Ingest

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"CitadelDesktop/Server/Protocol"
	"CitadelDesktop/Server/State"
)

func TestProductionSnapshotUsesFocusedCastleAndPreservesCapacity(t *testing.T) {
	gameState := State.NewGameState()
	castle := newCastleState(77)
	castle.Focused = true
	gameState.Castles[castle.ID] = castle
	responseCode := 0
	frame := Protocol.Frame{
		Direction: Protocol.DirectionInbound, Opcode: "spl", ResponseCode: &responseCode,
		ReceivedAt: time.Date(2026, 7, 11, 12, 0, 0, 0, time.UTC),
		Payload: json.RawMessage(`{
			"PS":{"WID":489,"TUA":6,"RCT":75},
			"QS":[{"P":{"WID":489,"TUA":324}},{"P":{"WID":489,"TUA":444}},{"SI":{"RUT":-1}}],
			"LID":0
		}`),
	}
	domains, changed, err := reduceProductionSnapshot(context.Background(), frame, &gameState, nil)
	if err != nil {
		t.Fatalf("reduce production snapshot: %v", err)
	}
	if !changed || len(domains) == 0 {
		t.Fatal("expected production state change")
	}
	queue := gameState.Castles[77].Production[0]
	if queue.Capacity != 3 {
		t.Fatalf("capacity = %d, want 3", queue.Capacity)
	}
	if queue.Active == nil || queue.Active.Definition.ID != 489 || queue.Active.Amount != 6 {
		t.Fatalf("unexpected active item: %#v", queue.Active)
	}
	if len(queue.Queued) != 2 || queue.Queued[1].Amount != 444 {
		t.Fatalf("unexpected queued items: %#v", queue.Queued)
	}
}

func TestProductionCommandLearnsSessionKeyFromOutboundFrame(t *testing.T) {
	gameState := State.NewGameState()
	frame := Protocol.Frame{
		Direction: Protocol.DirectionOutbound, Opcode: "bup",
		ReceivedAt: time.Date(2026, 7, 11, 12, 0, 0, 0, time.UTC),
		Payload:    json.RawMessage(`{"LID":0,"WID":489,"AMT":444,"SK":91,"AID":77}`),
	}
	_, changed, err := reduceProductionCommandContext(context.Background(), frame, &gameState, nil)
	if err != nil {
		t.Fatalf("reduce production context: %v", err)
	}
	if !changed || gameState.CommandContext.ProductionSessionKey != 91 {
		t.Fatalf("production session key = %d, want 91", gameState.CommandContext.ProductionSessionKey)
	}
	if gameState.CommandContext.ProductionObservedAt == nil || !gameState.CommandContext.ProductionObservedAt.Equal(frame.ReceivedAt) {
		t.Fatalf("unexpected observed time: %v", gameState.CommandContext.ProductionObservedAt)
	}
}
