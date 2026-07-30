package State

import "time"

func StationMovementReleaseAt(movement MovementState) *time.Time {
	if movement.ReturnsAt != nil && !movement.ReturnsAt.IsZero() {
		releasesAt := movement.ReturnsAt.UTC()
		return &releasesAt
	}
	if movement.Direction != 0 || movement.ArrivesAt == nil || movement.ArrivesAt.IsZero() {
		return movement.ProjectedCompletionAt()
	}
	releasesAt := movement.ArrivesAt.UTC().Add(
		time.Duration(max(0, movement.WaitSeconds)+max(0, movement.TravelSeconds)) * time.Second,
	)
	return &releasesAt
}

func StationMovementActiveAt(movement MovementState, now time.Time) bool {
	if now.IsZero() {
		now = time.Now().UTC()
	}
	releasesAt := StationMovementReleaseAt(movement)
	return releasesAt != nil && !releasesAt.IsZero() && releasesAt.After(now)
}

func TrackedStationMovementReleaseAt(gameState GameState, movement MovementState) *time.Time {
	for _, operation := range gameState.Stationing {
		if operation.MatchesMovement(movement) {
			return StationMovementReleaseAt(movement)
		}
	}
	return nil
}

func TrackedStationMovementActiveAt(gameState GameState, movement MovementState, now time.Time) bool {
	releasesAt := TrackedStationMovementReleaseAt(gameState, movement)
	return releasesAt != nil && !releasesAt.IsZero() && releasesAt.After(now)
}
