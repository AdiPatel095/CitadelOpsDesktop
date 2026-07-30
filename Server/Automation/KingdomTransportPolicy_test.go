package Automation

import (
	"testing"
	"time"

	"CitadelDesktop/Server/State"
)

func TestOwnedKingdomTransportDecisionKeepsSkipOwnershipIsolated(t *testing.T) {
	now := time.Date(2026, 7, 22, 12, 0, 0, 0, time.UTC)
	gameState := State.NewGameState()
	gameState.KingdomTransport.Pending = []State.KingdomResourceTransport{{KingdomID: 4, RemainingSec: 3_500}}
	gameState.KingdomTransport.ResourceWorkflows[4] = State.KingdomResourceTransportWorkflow{
		Owner: autoFoodBalanceTransportOwner, KingdomID: 4, SourceCastleID: 10, TargetCastleID: 20, LaunchedAt: now,
	}
	gameState.Player.Currencies[50] = 2
	snapshot := Snapshot{State: gameState, GameData: craftingLogisticsGameData(t), Now: now}

	foodDecision, ready := ownedKingdomTransportDecision(
		autoFoodBalanceTransportOwner, "Auto Food", true, []string{"MS5"}, nil, snapshot,
	)
	if !ready || foodDecision.Request == nil || foodDecision.Request.Name != "resource.kingdom.skip" {
		t.Fatalf("Auto Food did not manage its own shipment: %#v ready=%t", foodDecision, ready)
	}
	if sceatDecision, sceatReady := ownedKingdomTransportDecision(
		autoSceatTransportOwner, "Auto Sceat Resources", true, []string{"MS5"}, nil, snapshot,
	); sceatReady || sceatDecision.Request != nil {
		t.Fatalf("Auto Sceat Resources managed Auto Food's shipment: %#v ready=%t", sceatDecision, sceatReady)
	}
}

func TestOwnedKingdomTransportDecisionRefreshesOnlyItsCompletedDestination(t *testing.T) {
	now := time.Date(2026, 7, 22, 12, 0, 0, 0, time.UTC)
	gameState := State.NewGameState()
	gameState.KingdomTransport.ResourceWorkflows[4] = State.KingdomResourceTransportWorkflow{
		Owner: autoFoodBalanceTransportOwner, KingdomID: 4, SourceCastleID: 10, TargetCastleID: 20, LaunchedAt: now,
	}
	snapshot := Snapshot{State: gameState, Now: now}

	decision, ready := ownedKingdomTransportDecision(
		autoFoodBalanceTransportOwner, "Auto Food", false, nil, nil, snapshot,
	)
	if !ready || decision.Request == nil || decision.Request.Name != "resource.kingdom.settle" {
		t.Fatalf("completed Auto Food shipment was not settled: %#v ready=%t", decision, ready)
	}
	if other, otherReady := ownedKingdomTransportDecision(
		autoSceatTransportOwner, "Auto Sceat Resources", true, []string{"MS5"}, nil, snapshot,
	); otherReady || other.Request != nil {
		t.Fatalf("Auto Sceat Resources settled Auto Food's shipment: %#v ready=%t", other, otherReady)
	}
}
