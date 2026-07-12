package Ingest

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"CitadelDesktop/Server/Protocol"
	"CitadelDesktop/Server/State"
)

func TestAllianceReducerCapturesProtectedHoldings(t *testing.T) {
	gameState := State.NewGameState()
	gameState.Player.ID = 42
	code := 0
	frame := Protocol.Frame{
		Opcode: "ain", Direction: Protocol.DirectionInbound, ResponseCode: &code,
		ReceivedAt: time.Now().UTC(),
		Payload:    json.RawMessage(`{"A":{"AID":9,"N":"Test","M":[{"OID":42,"N":"Player","RPT":259200,"AP":[[0,100,12,34,1],[2,200,45,67,12]]}]}}`),
	}
	domains, changed, err := reduceAllianceInfo(context.Background(), frame, &gameState, nil)
	if err != nil || !changed {
		t.Fatalf("reduce alliance: changed=%v domains=%v err=%v", changed, domains, err)
	}
	if len(gameState.Alliance.Members) != 1 || gameState.Alliance.Members[0].ReturnProtectionSec != 259200 {
		t.Fatalf("unexpected alliance member: %#v", gameState.Alliance.Members)
	}
	if len(gameState.Alliance.Holdings) != 2 {
		t.Fatalf("unexpected holdings: %#v", gameState.Alliance.Holdings)
	}
	if gameState.Alliances[9].ID != 9 {
		t.Fatalf("alliance directory = %#v", gameState.Alliances)
	}
	holding := gameState.Alliance.Holdings[1]
	if holding.PlayerID != 42 || holding.CastleID != 200 || holding.KingdomID != 2 || holding.X != 45 || holding.Y != 67 || holding.SlotType != 12 {
		t.Fatalf("unexpected holding: %#v", holding)
	}
}

func TestAllianceReducerDoesNotReplaceOwnAllianceDuringInspection(t *testing.T) {
	gameState := State.NewGameState()
	gameState.Player.ID = 42
	gameState.Player.AllianceID = 9
	gameState.Alliance = State.AllianceState{ID: 9, Name: "Own", Members: []State.AllianceMember{}, Holdings: []State.AllianceHolding{}}
	code := 0
	frame := Protocol.Frame{
		Opcode: "ain", Direction: Protocol.DirectionInbound, ResponseCode: &code,
		ReceivedAt: time.Now().UTC(),
		Payload:    json.RawMessage(`{"A":{"AID":10,"N":"Target","M":[{"OID":77,"N":"Other","RPT":0,"AP":[[0,200,45,67,1]]}]}}`),
	}
	if _, changed, err := reduceAllianceInfo(t.Context(), frame, &gameState, nil); err != nil || !changed {
		t.Fatalf("reduce inspected alliance: changed=%t err=%v", changed, err)
	}
	if gameState.Alliance.ID != 9 || gameState.Alliance.Name != "Own" {
		t.Fatalf("own alliance was replaced: %+v", gameState.Alliance)
	}
	if gameState.Alliances[10].Name != "Target" || len(gameState.Alliances[10].Holdings) != 1 {
		t.Fatalf("inspected alliance missing: %+v", gameState.Alliances[10])
	}
}
