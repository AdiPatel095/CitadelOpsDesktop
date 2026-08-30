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
	active := false
	gameState.RangeMovements(func(_ MovementID, movement MovementState) bool {
		if IsIncomingPlayerAttack(gameState, movement, now) {
			active = true
			return false
		}
		return true
	})
	return active
}

// IsOutgoingPlayerAttack accepts only a fully identified, active PvP attack
// launched from one of the current player's castles. It intentionally excludes
// stationing, espionage, market, support, NPC, and returning movements.
func IsOutgoingPlayerAttack(gameState GameState, movement MovementState, now time.Time) bool {
	if now.IsZero() || gameState.Player.ID <= 0 || movement.Direction != 0 || movement.TypeID != 0 ||
		movement.OwnerPlayerID != gameState.Player.ID || movement.TargetPlayerID <= 0 ||
		movement.TargetPlayerID == gameState.Player.ID || !playerCastleMovementType(movement.SourceTypeID) ||
		movement.SourceCastleID <= 0 || !playerCastleMovementType(movement.TargetTypeID) ||
		movement.TargetCastleID <= 0 || movement.ArrivesAt == nil || !movement.ArrivesAt.After(now) {
		return false
	}
	source, owned := gameState.Castles[movement.SourceCastleID]
	return owned && source.ID == movement.SourceCastleID && source.SlotType == movement.SourceTypeID
}

func playerCastleMovementType(typeID int) bool {
	switch typeID {
	case 1, 3, 4, 5, 6, 12, 22:
		return true
	default:
		return false
	}
}
