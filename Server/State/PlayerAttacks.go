package State

import "time"

// IsIncomingPlayerAttack accepts only a fully identified, active attack from
// another player onto one of the current player's castles.
func IsIncomingPlayerAttack(gameState GameState, movement MovementState, now time.Time) bool {
	if now.IsZero() || gameState.Player.ID <= 0 ||
		movement.Direction != 0 || movement.TypeID != 0 ||
		movement.OwnerPlayerID <= 0 || movement.OwnerPlayerID == gameState.Player.ID ||
		!playerCastleMovementType(movement.SourceTypeID) || movement.SourceCastleID <= 0 ||
		!playerCastleMovementType(movement.TargetTypeID) || movement.TargetCastleID <= 0 ||
		movement.TargetPlayerID != gameState.Player.ID ||
		movement.ArrivesAt == nil || !movement.ArrivesAt.After(now) {
		return false
	}
	target, owned := gameState.Castles[movement.TargetCastleID]
	return owned && target.ID == movement.TargetCastleID && target.SlotType == movement.TargetTypeID
}

func HasIncomingPlayerAttack(gameState GameState, now time.Time) bool {
	for _, movement := range gameState.Movements {
		if IsIncomingPlayerAttack(gameState, movement, now) {
			return true
		}
	}
	return false
}

func playerCastleMovementType(typeID int) bool {
	switch typeID {
	case 1, 3, 4, 5, 6, 12, 22:
		return true
	default:
		return false
	}
}
