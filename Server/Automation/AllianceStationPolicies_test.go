package Automation

import (
	"testing"
	"time"

	"CitadelDesktop/Server/State"
)

func TestProtectedHoldingsUseMemberProtectionAndKingdomDistance(t *testing.T) {
	alliance := State.AllianceState{
		Members: []State.AllianceMember{
			{PlayerID: 1, ReturnProtectionSec: 4 * 86_400},
			{PlayerID: 2, ReturnProtectionSec: 86_400},
		},
		Holdings: []State.AllianceHolding{
			{CastleID: 10, PlayerID: 1, KingdomID: 0, X: 20, Y: 20, SlotType: 1},
			{CastleID: 11, PlayerID: 1, KingdomID: 0, X: 12, Y: 12, SlotType: 4},
			{CastleID: 12, PlayerID: 2, KingdomID: 0, X: 11, Y: 11, SlotType: 1},
			{CastleID: 13, PlayerID: 1, KingdomID: 2, X: 10, Y: 10, SlotType: 12},
		},
	}
	holdings := protectedHoldings(alliance, 3)
	if len(holdings) != 3 {
		t.Fatalf("expected three protected holdings, got %#v", holdings)
	}
	nearest, ok := nearestHolding(holdings, State.CastleState{ID: 99, KingdomID: 0, X: 10, Y: 10})
	if !ok || nearest.CastleID != 11 {
		t.Fatalf("unexpected nearest holding: %#v", nearest)
	}
}

func TestIncomingThreatsOnlyIncludeHostileAttacksOnOwnedCastles(t *testing.T) {
	now := time.Now().UTC()
	arrives := now.Add(90 * time.Second)
	gameState := State.NewGameState()
	gameState.Player.ID = 7
	gameState.Castles[100] = State.CastleState{ID: 100}
	gameState.Movements[1] = State.MovementState{ID: 1, TypeID: 0, Direction: 0, OwnerPlayerID: 8, TargetCastleID: 100, ArrivesAt: &arrives}
	gameState.Movements[2] = State.MovementState{ID: 2, TypeID: 0, Direction: 0, OwnerPlayerID: 7, TargetCastleID: 100, ArrivesAt: &arrives}
	gameState.Movements[3] = State.MovementState{ID: 3, TypeID: 2, Direction: 0, OwnerPlayerID: 8, TargetCastleID: 100, ArrivesAt: &arrives}
	threats, count, earliest, latest := incomingThreats(gameState, now)
	if count != 1 || len(threats) != 1 || !earliest.Equal(arrives) || !latest.Equal(arrives) {
		t.Fatalf("unexpected threats: count=%d threats=%#v earliest=%v latest=%v", count, threats, earliest, latest)
	}
}
