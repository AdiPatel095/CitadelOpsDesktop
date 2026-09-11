package State

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// InvasionTargetReservationReconcileGrace bounds exact launch evidence and
// spaces positive-evidence recovery probes for a newly dispatched command.
const InvasionTargetReservationReconcileGrace = 30 * time.Second

// InvasionTargetReservationMaxReconcileAttempts bounds scoped GAM polling.
// Exhaustion keeps the no-replay lock intact until positive movement evidence
// or a new event occurrence arrives.
const InvasionTargetReservationMaxReconcileAttempts = 3

// MapTargetKey identifies the map endpoint occupied by an army movement.
// Outbound movements point at their TA endpoint; return movements point back
// to the attacked target through their SA endpoint.
type MapTargetKey struct {
	KingdomID KingdomID
	TypeID    int
	X         int
	Y         int
}

func MovementMapTarget(movement MovementState) (MapTargetKey, bool) {
	switch movement.Direction {
	case 0:
		return MapTargetKey{
			KingdomID: movement.KingdomID, TypeID: movement.TargetTypeID,
			X: movement.TargetX, Y: movement.TargetY,
		}, true
	case 1:
		return MapTargetKey{
			KingdomID: movement.KingdomID, TypeID: movement.SourceTypeID,
			X: movement.SourceX, Y: movement.SourceY,
		}, true
	default:
		return MapTargetKey{}, false
	}
}

// MovementMapEndpoints returns both endpoints because the game disables alien
// target actions while any movement is registered either to or from that map
// coordinate. Exact launch attribution intentionally continues to use the
// direction-specific MovementMapTarget above.
func MovementMapEndpoints(movement MovementState) [2]MapTargetKey {
	return [2]MapTargetKey{
		{
			KingdomID: movement.KingdomID, TypeID: movement.SourceTypeID,
			X: movement.SourceX, Y: movement.SourceY,
		},
		{
			KingdomID: movement.KingdomID, TypeID: movement.TargetTypeID,
			X: movement.TargetX, Y: movement.TargetY,
		},
	}
}

// MovementOccupiesMapTargetAt matches either source or target endpoint and
// keeps it occupied through the complete route plus the game's short
// post-return bookkeeping grace. Occupancy follows the game's coordinate-only
// TO/FROM checks; endpoint type can be stale when a map slot changes identity.
func MovementOccupiesMapTargetAt(movement MovementState, target MapTargetKey, now time.Time) bool {
	if movement.Direction != 0 && movement.Direction != 1 {
		return false
	}
	touchesTarget := false
	for _, endpoint := range MovementMapEndpoints(movement) {
		if endpoint.KingdomID == target.KingdomID && endpoint.X == target.X && endpoint.Y == target.Y {
			touchesTarget = true
			break
		}
	}
	if !touchesTarget {
		return false
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	releaseAt := CommanderMovementReleaseAt(movement)
	return releaseAt == nil || releaseAt.IsZero() || releaseAt.After(now)
}

func AnyActiveMovementAtMapTarget(gameState GameState, target MapTargetKey, now time.Time) bool {
	occupied := false
	gameState.RangeMovements(func(_ MovementID, movement MovementState) bool {
		if MovementOccupiesMapTargetAt(movement, target, now) {
			occupied = true
			return false
		}
		return true
	})
	return occupied
}

// InvasionReservationMovement returns the newest movement that can only have
// come from the reserved Auto Invasion launch. Target coordinates alone are
// insufficient because another player may attack the same event castle. New
// reservations therefore bind the source castle, its launch coordinates, and
// commander as well. A return leg is accepted only when its impact/start also
// falls inside the narrow dispatch-reconciliation window; a later same-route
// manual return cannot satisfy a stale reservation.
func InvasionReservationMovement(
	gameState GameState,
	reservation InvasionTargetReservation,
) (MovementState, bool) {
	if reservation.SourceCastleID <= 0 || !reservation.CommanderKnown || reservation.ReservedAt.IsZero() ||
		reservation.EventID <= 0 || reservation.OccurrenceEndsAt.IsZero() {
		return MovementState{}, false
	}
	occurrence, known := gameState.LookupEventOccurrence(reservation.EventID)
	if known && !SameEventOccurrence(reservation.OccurrenceEndsAt, occurrence.EndsAt) {
		return MovementState{}, false
	}
	var selected MovementState
	gameState.RangeMovements(func(_ MovementID, movement MovementState) bool {
		target, found := MovementMapTarget(movement)
		if !found || target.KingdomID != reservation.KingdomID || target.X != reservation.X || target.Y != reservation.Y ||
			reservation.TargetTypeID <= 0 || target.TypeID != reservation.TargetTypeID ||
			movement.CommanderID == nil || *movement.CommanderID != reservation.CommanderID ||
			movement.ObservedAt.IsZero() || !movement.ObservedAt.After(reservation.ReservedAt) {
			return true
		}
		switch movement.Direction {
		case 0:
			if !invasionMovementCastleEndpointMatches(
				gameState, reservation.SourceCastleID, reservation.SourceX, reservation.SourceY, reservation.SourceKnown,
				movement.SourceCastleID, movement.SourceX, movement.SourceY,
			) {
				return true
			}
		case 1:
			if !invasionMovementCastleEndpointMatches(
				gameState, reservation.SourceCastleID, reservation.SourceX, reservation.SourceY, reservation.SourceKnown,
				movement.TargetCastleID, movement.TargetX, movement.TargetY,
			) {
				return true
			}
		default:
			return true
		}
		// PT is integral seconds, so the reconstructed start can precede the
		// pre-wire reservation by a fraction of a second. For a return frame this
		// is the impact/return-leg start, so only genuinely short attacks remain
		// eligible as unique recovery evidence.
		if movement.StartedAt.IsZero() ||
			movement.StartedAt.Before(reservation.ReservedAt.Add(-2*time.Second)) ||
			movement.StartedAt.After(reservation.ReservedAt.Add(InvasionTargetReservationReconcileGrace)) {
			return true
		}
		if !known && movement.StartedAt.After(reservation.OccurrenceEndsAt.Add(10*time.Minute)) {
			// With no retained score/activity occurrence, accept only an outbound
			// or return movement whose start is still bounded to the reservation's
			// event. A later recurrence must never satisfy an old marker.
			return true
		}
		if selected.ID == 0 || movement.ObservedAt.After(selected.ObservedAt) ||
			movement.ObservedAt.Equal(selected.ObservedAt) && movement.ID > selected.ID {
			selected = movement
		}
		return true
	})
	return selected, selected.ID > 0
}

func invasionMovementCastleEndpointMatches(
	gameState GameState,
	expectedCastleID CastleID,
	expectedX int,
	expectedY int,
	expectedCoordinatesKnown bool,
	observedCastleID CastleID,
	x int,
	y int,
) bool {
	if observedCastleID > 0 && observedCastleID != expectedCastleID {
		return false
	}
	if expectedCoordinatesKnown {
		return expectedX == x && expectedY == y
	}
	if observedCastleID > 0 {
		return true
	}
	castle, exists := gameState.Castles[expectedCastleID]
	return exists && castle.X == x && castle.Y == y
}

func InvasionTargetKey(kingdomID KingdomID, x, y int) string {
	return fmt.Sprintf("%d:%d:%d", kingdomID, x, y)
}

func ParseInvasionTargetKey(key string) (KingdomID, int, int, bool) {
	parts := strings.Split(key, ":")
	if len(parts) != 3 {
		return 0, 0, 0, false
	}
	kingdomID, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		return 0, 0, 0, false
	}
	x, err := strconv.Atoi(parts[1])
	if err != nil {
		return 0, 0, 0, false
	}
	y, err := strconv.Atoi(parts[2])
	if err != nil {
		return 0, 0, 0, false
	}
	return KingdomID(kingdomID), x, y, true
}

func (state InvasionState) TargetUnavailable(kingdomID KingdomID, x, y int) bool {
	_, unavailable := state.UnavailableTargets[InvasionTargetKey(kingdomID, x, y)]
	return unavailable
}

func (state *InvasionState) ClearTargetFortification(kingdomID KingdomID, x, y int) bool {
	if state == nil || state.FortifiedTargets == nil {
		return false
	}
	key := InvasionTargetKey(kingdomID, x, y)
	if _, fortified := state.FortifiedTargets[key]; !fortified {
		return false
	}
	delete(state.FortifiedTargets, key)
	return true
}

func (state *InvasionState) MarkTargetUnavailable(kingdomID KingdomID, x, y int, observedAt time.Time) bool {
	if state == nil {
		return false
	}
	if observedAt.IsZero() {
		observedAt = time.Now().UTC()
	} else {
		observedAt = observedAt.UTC()
	}
	if state.UnavailableTargets == nil {
		state.UnavailableTargets = map[string]time.Time{}
	}
	key := InvasionTargetKey(kingdomID, x, y)
	if current, exists := state.UnavailableTargets[key]; exists && !observedAt.After(current) {
		return false
	}
	state.UnavailableTargets[key] = observedAt
	return true
}

func (state *InvasionState) MarkTargetAvailable(kingdomID KingdomID, x, y int, observedAt time.Time) bool {
	if state == nil || state.UnavailableTargets == nil {
		return false
	}
	if observedAt.IsZero() {
		observedAt = time.Now().UTC()
	} else {
		observedAt = observedAt.UTC()
	}
	key := InvasionTargetKey(kingdomID, x, y)
	unavailableAt, exists := state.UnavailableTargets[key]
	if !exists || !observedAt.After(unavailableAt) {
		return false
	}
	delete(state.UnavailableTargets, key)
	return true
}

func (state InvasionState) TargetReservation(
	kingdomID KingdomID,
	x int,
	y int,
) (InvasionTargetReservation, bool) {
	reservation, reserved := state.TargetReservations[InvasionTargetKey(kingdomID, x, y)]
	return reservation, reserved
}

// InvasionCommanderReserved reports whether a complete, unresolved CRA launch
// marker owns the commander. Target-only ADI/cooldown locks and malformed
// legacy markers do not hold a commander.
func InvasionCommanderReserved(gameState GameState, commanderID CommanderID) bool {
	for _, reservation := range gameState.Invasion.TargetReservations {
		if reservation.CommanderKnown && reservation.CommanderID == commanderID &&
			reservation.EventID > 0 && !reservation.OccurrenceEndsAt.IsZero() &&
			reservation.SourceCastleID > 0 && reservation.OperationID != "" && !reservation.ReservedAt.IsZero() &&
			!reservation.ReservedAt.After(reservation.OccurrenceEndsAt.Add(10*time.Minute)) {
			return true
		}
	}
	return false
}

func (state *InvasionState) ReserveTarget(reservation InvasionTargetReservation) bool {
	if state == nil {
		return false
	}
	if reservation.ReservedAt.IsZero() {
		reservation.ReservedAt = time.Now().UTC()
	} else {
		reservation.ReservedAt = reservation.ReservedAt.UTC()
	}
	if !reservation.OccurrenceEndsAt.IsZero() {
		reservation.OccurrenceEndsAt = reservation.OccurrenceEndsAt.UTC()
	}
	if !reservation.ReconcileAfter.IsZero() {
		reservation.ReconcileAfter = reservation.ReconcileAfter.UTC()
	}
	if !reservation.RecoveryExhaustedAt.IsZero() {
		reservation.RecoveryExhaustedAt = reservation.RecoveryExhaustedAt.UTC()
	}
	if state.TargetReservations == nil {
		state.TargetReservations = map[string]InvasionTargetReservation{}
	}
	key := InvasionTargetKey(reservation.KingdomID, reservation.X, reservation.Y)
	if _, exists := state.TargetReservations[key]; exists {
		return false
	}
	state.TargetReservations[key] = reservation
	return true
}

// BackoffTargetReservation postpones another reconciliation attempt without
// weakening the reservation. A GAM reply is scoped and its omission of a
// movement cannot prove that an indeterminate CRA launch did not happen, so a
// complete launch marker must retain both its target and commander binding.
func (state *InvasionState) BackoffTargetReservation(
	kingdomID KingdomID,
	x int,
	y int,
	operationID string,
	reconcileAfter time.Time,
) bool {
	if state == nil || state.TargetReservations == nil || reconcileAfter.IsZero() {
		return false
	}
	key := InvasionTargetKey(kingdomID, x, y)
	reservation, exists := state.TargetReservations[key]
	if !exists || operationID == "" || reservation.OperationID != operationID {
		return false
	}
	reconcileAfter = reconcileAfter.UTC()
	if !reservation.ReconcileAfter.IsZero() && !reconcileAfter.After(reservation.ReconcileAfter) {
		return false
	}
	reservation.ReconcileAfter = reconcileAfter
	reservation.ReconcileAttempts++
	if reservation.CommanderKnown &&
		reservation.ReconcileAttempts >= InvasionTargetReservationMaxReconcileAttempts &&
		reservation.RecoveryExhaustedAt.IsZero() {
		reservation.RecoveryExhaustedAt = time.Now().UTC()
	}
	state.TargetReservations[key] = reservation
	return true
}

// DeferTargetReservation postpones a target-only ADI/cooldown lock. It refuses
// to downgrade a complete CRA launch marker because scoped movement snapshots
// cannot prove that the commander is reusable.
func (state *InvasionState) DeferTargetReservation(
	kingdomID KingdomID,
	x int,
	y int,
	operationID string,
	reconcileAfter time.Time,
) bool {
	if state == nil || state.TargetReservations == nil || reconcileAfter.IsZero() {
		return false
	}
	key := InvasionTargetKey(kingdomID, x, y)
	reservation, exists := state.TargetReservations[key]
	if !exists || operationID == "" || reservation.OperationID != operationID || reservation.CommanderKnown {
		return false
	}
	reconcileAfter = reconcileAfter.UTC()
	if !reservation.ReconcileAfter.IsZero() && !reconcileAfter.After(reservation.ReconcileAfter) {
		return false
	}
	reservation.ReconcileAfter = reconcileAfter
	state.TargetReservations[key] = reservation
	return true
}

// ReleaseTargetReservation removes a reservation owned by operationID. An
// empty operationID is an explicit unconditional release for state repair.
func (state *InvasionState) ReleaseTargetReservation(
	kingdomID KingdomID,
	x int,
	y int,
	operationID string,
) bool {
	if state == nil || state.TargetReservations == nil {
		return false
	}
	key := InvasionTargetKey(kingdomID, x, y)
	current, exists := state.TargetReservations[key]
	if !exists || operationID != "" && current.OperationID != operationID {
		return false
	}
	delete(state.TargetReservations, key)
	return true
}

func (state *InvasionState) ReleaseTargetReservationObservedAfter(
	kingdomID KingdomID,
	x int,
	y int,
	observedAt time.Time,
) bool {
	if state == nil || state.TargetReservations == nil || observedAt.IsZero() {
		return false
	}
	key := InvasionTargetKey(kingdomID, x, y)
	current, exists := state.TargetReservations[key]
	if !exists || !observedAt.UTC().After(current.ReservedAt) {
		return false
	}
	delete(state.TargetReservations, key)
	return true
}
