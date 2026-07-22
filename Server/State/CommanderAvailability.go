package State

import "time"

const commanderMovementReturnGrace = 5 * time.Second

// CommanderHasActiveMovementAt independently verifies commander occupancy from
// the canonical movement snapshot. This protects launch paths when a leader
// roster refresh briefly disagrees with the movement list.
func CommanderHasActiveMovementAt(gameState GameState, commanderID CommanderID, now time.Time) bool {
	if now.IsZero() {
		now = time.Now().UTC()
	}
	for _, movement := range gameState.Movements {
		if movement.CommanderID == nil || *movement.CommanderID != commanderID ||
			!MovementOwnedByCurrentPlayer(gameState, movement) || !CommanderMovementActiveAt(movement, now) {
			continue
		}
		return true
	}
	return false
}

func MovementOwnedByCurrentPlayer(gameState GameState, movement MovementState) bool {
	if movement.OwnerPlayerID > 0 {
		return gameState.Player.ID > 0 && movement.OwnerPlayerID == gameState.Player.ID
	}
	if movement.SourceCastleID <= 0 {
		return false
	}
	_, ownedSource := gameState.Castles[movement.SourceCastleID]
	return ownedSource
}

func CommanderMovementActiveAt(movement MovementState, now time.Time) bool {
	if movement.Direction == 0 && movement.WaitSeconds > 0 {
		return true
	}
	completion := CommanderMovementReleaseAt(movement)
	return completion == nil || completion.IsZero() || completion.After(now)
}

// CommanderMovementReleaseAt uses the authoritative return time when present.
// Outbound attack frames normally expose only arrival and travel time, so the
// conservative fallback covers the expected return trip plus a short settle
// window. This keeps a scoped gam reply from briefly releasing a busy leader.
func CommanderMovementReleaseAt(movement MovementState) *time.Time {
	if movement.Direction == 0 && movement.WaitSeconds > 0 {
		return nil
	}
	if movement.ReturnsAt != nil && !movement.ReturnsAt.IsZero() {
		releaseAt := movement.ReturnsAt.UTC()
		return &releaseAt
	}
	if movement.Direction != 0 || movement.ArrivesAt == nil || movement.ArrivesAt.IsZero() {
		return movement.ProjectedCompletionAt()
	}
	releaseAt := movement.ArrivesAt.UTC()
	if movement.TravelSeconds > 0 {
		releaseAt = releaseAt.Add(time.Duration(movement.TravelSeconds) * time.Second)
	}
	releaseAt = releaseAt.Add(commanderMovementReturnGrace)
	return &releaseAt
}
