package State

import "time"

type MarketBarrowLease struct {
	Barrows    int
	ReleasesAt time.Time
}

func MarketBarrowMovementReleaseAt(movement MovementState) *time.Time {
	if movement.MarketBarrows <= 0 {
		return nil
	}
	if movement.ReturnsAt != nil && !movement.ReturnsAt.IsZero() {
		releasesAt := movement.ReturnsAt.UTC()
		return &releasesAt
	}
	if movement.Direction == 0 && movement.ArrivesAt != nil && !movement.ArrivesAt.IsZero() {
		releasesAt := movement.ArrivesAt.UTC()
		if movement.TravelSeconds > 0 {
			releasesAt = releasesAt.Add(time.Duration(movement.TravelSeconds) * time.Second)
		}
		return &releasesAt
	}
	return movement.ProjectedCompletionAt()
}

func MarketBarrowMovementActiveAt(movement MovementState, now time.Time) bool {
	if now.IsZero() {
		now = time.Now().UTC()
	}
	releasesAt := MarketBarrowMovementReleaseAt(movement)
	return releasesAt != nil && !releasesAt.IsZero() && releasesAt.After(now)
}

func MarketBarrowLeaseAt(gameState GameState, castleID CastleID, now time.Time) MarketBarrowLease {
	if now.IsZero() {
		now = time.Now().UTC()
	}
	lease := MarketBarrowLease{}
	for _, movement := range gameState.Movements {
		if movement.SourceCastleID != castleID || movement.MarketBarrows <= 0 ||
			!MovementOwnedByCurrentPlayer(gameState, movement) || !MarketBarrowMovementActiveAt(movement, now) {
			continue
		}
		lease.Barrows += movement.MarketBarrows
		releasesAt := MarketBarrowMovementReleaseAt(movement)
		if releasesAt != nil && (lease.ReleasesAt.IsZero() || releasesAt.Before(lease.ReleasesAt)) {
			lease.ReleasesAt = releasesAt.UTC()
		}
	}
	return lease
}

func NextMarketBarrowLeaseRelease(gameState GameState, now time.Time) time.Time {
	if now.IsZero() {
		now = time.Now().UTC()
	}
	var next time.Time
	for _, movement := range gameState.Movements {
		if movement.MarketBarrows <= 0 || !MovementOwnedByCurrentPlayer(gameState, movement) ||
			!MarketBarrowMovementActiveAt(movement, now) {
			continue
		}
		releasesAt := MarketBarrowMovementReleaseAt(movement)
		if releasesAt != nil && (next.IsZero() || releasesAt.Before(next)) {
			next = releasesAt.UTC()
		}
	}
	return next
}

func AvailableMarketBarrowsAt(gameState GameState, market MarketCastleState, now time.Time) int {
	available := max(0, market.AvailableBarrows)
	lease := MarketBarrowLeaseAt(gameState, market.CastleID, now)
	if market.TotalBarrows > 0 {
		available = min(available, max(0, market.TotalBarrows-lease.Barrows))
	} else if lease.Barrows > 0 {
		available = 0
	}
	return available
}
