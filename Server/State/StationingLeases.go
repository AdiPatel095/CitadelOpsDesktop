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

// KhanAutoStationYieldActiveAt reports whether Auto Khan must yield to an
// authoritative Auto Station operation. Movement ids are the ownership
// boundary: batch ids replace the legacy scalar id, and route similarity alone
// never makes an unrelated movement part of the operation.
func KhanAutoStationYieldActiveAt(gameState GameState, now time.Time) bool {
	if now.IsZero() {
		now = time.Now().UTC()
	}
	for _, operation := range gameState.Stationing {
		if operation.Purpose != "autoStation" {
			continue
		}
		if operation.SafeAfter != nil && now.Before(operation.SafeAfter.Add(5*time.Second)) {
			return true
		}
		movementIDs := operation.MovementIDs
		if len(movementIDs) == 0 && operation.MovementID > 0 {
			movementIDs = []MovementID{operation.MovementID}
		}
		for _, movementID := range movementIDs {
			if movementID <= 0 {
				continue
			}
			movement, found := gameState.LookupMovement(movementID)
			if found && khanStationMovementActiveAt(movement, now) {
				return true
			}
		}
	}
	return false
}

func khanStationMovementActiveAt(movement MovementState, now time.Time) bool {
	var completion *time.Time
	switch movement.Direction {
	case 0:
		completion = movement.ArrivesAt
	case 1:
		completion = movement.ReturnsAt
	default:
		return true
	}
	return completion == nil || completion.IsZero() || completion.After(now)
}
