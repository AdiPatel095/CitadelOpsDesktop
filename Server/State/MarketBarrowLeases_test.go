package State

import (
	"testing"
	"time"
)

func TestMarketBarrowLeaseUsesReturnLegAndCapsStaleAvailability(t *testing.T) {
	now := time.Date(2026, 7, 22, 23, 30, 0, 0, time.UTC)
	returnsAt := now.Add(10 * time.Minute)
	gameState := NewGameState()
	gameState.Player.ID = 1
	gameState.Castles[10] = CastleState{ID: 10}
	gameState.Movements[50] = MovementState{
		ID: 50, Direction: 1, OwnerPlayerID: 1, SourceCastleID: 10,
		MarketBarrows: 75, ReturnsAt: &returnsAt,
	}
	market := MarketCastleState{CastleID: 10, TotalBarrows: 100, AvailableBarrows: 100}

	lease := MarketBarrowLeaseAt(gameState, 10, now)
	if lease.Barrows != 75 || !lease.ReleasesAt.Equal(returnsAt) {
		t.Fatalf("market lease = %+v", lease)
	}
	if available := AvailableMarketBarrowsAt(gameState, market, now); available != 25 {
		t.Fatalf("lease-adjusted available barrows = %d, want 25", available)
	}
	if available := AvailableMarketBarrowsAt(gameState, market, returnsAt); available != 100 {
		t.Fatalf("returned barrows remained leased: %d", available)
	}
}

func TestMarketBarrowOutboundLeaseProjectsTheReturnTrip(t *testing.T) {
	now := time.Date(2026, 7, 22, 23, 30, 0, 0, time.UTC)
	arrivesAt := now.Add(5 * time.Minute)
	movement := MovementState{
		Direction: 0, MarketBarrows: 10, TravelSeconds: 300, ArrivesAt: &arrivesAt,
	}
	want := arrivesAt.Add(5 * time.Minute)
	releasesAt := MarketBarrowMovementReleaseAt(movement)
	if releasesAt == nil || !releasesAt.Equal(want) {
		t.Fatalf("outbound lease release = %v, want %s", releasesAt, want)
	}
	if !MarketBarrowMovementActiveAt(movement, want.Add(-time.Millisecond)) || MarketBarrowMovementActiveAt(movement, want) {
		t.Fatal("outbound market lease did not expire at its projected return")
	}
}
