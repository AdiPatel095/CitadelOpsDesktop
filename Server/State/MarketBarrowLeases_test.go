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
		ID: 50, Direction: 1, OwnerPlayerID: 1, SourceCastleID: 20, TargetCastleID: 10,
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

func TestMarketBarrowLeaseKeepsHomeFleetReservedAcrossReturnTransition(t *testing.T) {
	now := time.Date(2026, 9, 12, 17, 14, 46, 0, time.UTC)
	arrivesAt := now.Add(81 * time.Second)
	returnsAt := arrivesAt.Add(81 * time.Second)
	state := NewGameState()
	state.Player.ID = 1
	market := MarketCastleState{CastleID: 10, TotalBarrows: 125, AvailableBarrows: 125}
	for i, carts := range []int{90, 26, 7, 2} {
		id := MovementID(i + 1)
		state.Movements[id] = MovementState{
			ID: id, Direction: 0, OwnerPlayerID: 1, SourceCastleID: 10, TargetCastleID: 20,
			MarketBarrows: carts, TravelSeconds: 81, ArrivesAt: &arrivesAt,
		}
	}
	check := func(at time.Time) {
		t.Helper()
		if lease := MarketBarrowLeaseAt(state, 10, at); lease.Barrows != 125 || !lease.ReleasesAt.Equal(returnsAt) {
			t.Fatalf("home fleet reservation = %+v", lease)
		}
		if lease := MarketBarrowLeaseAt(state, 20, at); lease.Barrows != 0 {
			t.Fatalf("recipient incorrectly owns returning carts: %+v", lease)
		}
		if available := AvailableMarketBarrowsAt(state, market, at); available != 0 {
			t.Fatalf("home carts reused before returning: %d", available)
		}
	}
	check(now)
	for id, movement := range state.Movements {
		movement.Direction = 1
		movement.SourceCastleID, movement.TargetCastleID = 20, 10
		movement.ArrivesAt, movement.ReturnsAt = nil, &returnsAt
		state.Movements[id] = movement
		check(arrivesAt)
	}
	// Another player's returning carts must not reserve this account's fleet.
	state.Movements[99] = MovementState{ID: 99, Direction: 1, OwnerPlayerID: 2,
		SourceCastleID: 20, TargetCastleID: 10, MarketBarrows: 50, ReturnsAt: &returnsAt}
	check(returnsAt.Add(-time.Nanosecond))
	if available := AvailableMarketBarrowsAt(state, market, returnsAt); available != 125 {
		t.Fatalf("completed return did not release fleet: %d", available)
	}
}
