package State

import "time"

// CommanderMovementReturnGrace is how long after a movement's nominal end the
// commander stays busy. The game keeps listing a return movement until its own
// tick finalises the arrival and rejects any launch with that commander in the
// meantime (CRA 256, "commander already assigned to an active movement").
// Live cell logs (2026-08-18) show every 256 of the day was a launch fired
// 0.1–0.3 s after the extrapolated return time — once with the movement still
// listed at PT == TT — so a launch must wait past the game's own bookkeeping.
const CommanderMovementReturnGrace = 5 * time.Second

// CommanderHasActiveMovementAt independently verifies commander occupancy from
// the canonical movement snapshot. This protects launch paths when a leader
// roster refresh briefly disagrees with the movement list.
func CommanderHasActiveMovementAt(gameState GameState, commanderID CommanderID, now time.Time) bool {
	if now.IsZero() {
		now = time.Now().UTC()
	}
	active := false
	gameState.RangeMovements(func(_ MovementID, movement MovementState) bool {
		if movement.CommanderID == nil || *movement.CommanderID != commanderID ||
			!MovementOwnedByCurrentPlayer(gameState, movement) || !CommanderMovementActiveAt(movement, now) {
			return true
		}
		active = true
		return false
	})
	return active
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

// CommanderMovementReleaseAt is when the commander on a movement may be
// launched again: the movement's nominal end plus CommanderMovementReturnGrace.
// The nominal end is the authoritative return time when present; outbound
// attack frames normally expose only arrival and travel time, so the fallback
// covers the expected return trip. If the game listed the movement at or after
// its nominal end (the arrival not yet finalised server-side), the release is
// pushed to that sighting plus the grace, so the commander stays busy until a
// later reply drops the movement — bounded, so a stalled poll cannot pin him.
func CommanderMovementReleaseAt(movement MovementState) *time.Time {
	if movement.Direction == 0 && movement.WaitSeconds > 0 {
		return nil
	}
	nominal := commanderMovementNominalEndAt(movement)
	if nominal == nil {
		return nil
	}
	releaseAt := nominal.Add(CommanderMovementReturnGrace)
	if !movement.ObservedAt.IsZero() && !movement.ObservedAt.Before(*nominal) {
		if seenRelease := movement.ObservedAt.UTC().Add(CommanderMovementReturnGrace); seenRelease.After(releaseAt) {
			releaseAt = seenRelease
		}
	}
	return &releaseAt
}

// commanderMovementNominalEndAt is the game's own end time for the movement as
// far as we can tell: the return time when known, else arrival plus the return
// trip for an outbound leg, else the projected completion.
func commanderMovementNominalEndAt(movement MovementState) *time.Time {
	if movement.ReturnsAt != nil && !movement.ReturnsAt.IsZero() {
		nominal := movement.ReturnsAt.UTC()
		return &nominal
	}
	if movement.Direction != 0 || movement.ArrivesAt == nil || movement.ArrivesAt.IsZero() {
		projected := movement.ProjectedCompletionAt()
		if projected == nil || projected.IsZero() {
			return nil
		}
		nominal := projected.UTC()
		return &nominal
	}
	nominal := movement.ArrivesAt.UTC()
	if movement.TravelSeconds > 0 {
		nominal = nominal.Add(time.Duration(movement.TravelSeconds) * time.Second)
	}
	return &nominal
}
