package State

import (
	"testing"
	"time"
)

func TestMovementOccupiesMapTargetAcrossOutboundAndReturnRoutes(t *testing.T) {
	now := time.Date(2026, 9, 9, 12, 0, 0, 0, time.UTC)
	arrivesAt := now.Add(time.Minute)
	returnsAt := now.Add(2 * time.Minute)
	target := MapTargetKey{KingdomID: 0, TypeID: MapTypeForeignLord, X: 101, Y: 102}

	tests := []struct {
		name     string
		movement MovementState
		wanted   bool
	}{
		{
			name: "outbound target endpoint",
			movement: MovementState{
				Direction: 0, KingdomID: 0, TargetTypeID: MapTypeForeignLord,
				TargetX: 101, TargetY: 102, ArrivesAt: &arrivesAt, TravelSeconds: 60,
			},
			wanted: true,
		},
		{
			name: "return source endpoint",
			movement: MovementState{
				Direction: 1, TypeID: 2, KingdomID: 0, SourceTypeID: MapTypeForeignLord,
				SourceX: 101, SourceY: 102, TargetX: 10, TargetY: 11, ReturnsAt: &returnsAt,
			},
			wanted: true,
		},
		{
			name: "return destination endpoint",
			movement: MovementState{
				Direction: 1, KingdomID: 0, SourceTypeID: MapTypeForeignLord,
				SourceX: 50, SourceY: 51, TargetX: 101, TargetY: 102, ReturnsAt: &returnsAt,
			},
			wanted: true,
		},
		{
			name: "outbound source endpoint",
			movement: MovementState{
				Direction: 0, KingdomID: 0, SourceTypeID: MapTypeForeignLord,
				SourceX: 101, SourceY: 102, TargetTypeID: 2, TargetX: 10, TargetY: 11,
				ArrivesAt: &arrivesAt,
			},
			wanted: true,
		},
		{
			name: "missing endpoint type fails closed by coordinate",
			movement: MovementState{
				Direction: 0, KingdomID: 0, TargetX: 101, TargetY: 102,
				ArrivesAt: &arrivesAt, TravelSeconds: 60,
			},
			wanted: true,
		},
		{
			name: "known type mismatch still occupies coordinate",
			movement: MovementState{
				Direction: 0, KingdomID: 0, TargetTypeID: MapTypeBloodcrow,
				TargetX: 101, TargetY: 102, ArrivesAt: &arrivesAt,
			},
			wanted: true,
		},
		{
			name: "unsupported direction",
			movement: MovementState{
				Direction: 2, KingdomID: 0, TargetTypeID: MapTypeForeignLord,
				TargetX: 101, TargetY: 102,
			},
			wanted: false,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := MovementOccupiesMapTargetAt(test.movement, target, now); got != test.wanted {
				t.Fatalf("occupied=%t, want %t", got, test.wanted)
			}
		})
	}

	completed := MovementState{
		Direction: 1, KingdomID: 0, SourceTypeID: MapTypeForeignLord,
		SourceX: 101, SourceY: 102, ReturnsAt: &returnsAt,
	}
	if MovementOccupiesMapTargetAt(completed, target, returnsAt.Add(CommanderMovementReturnGrace+time.Second)) {
		t.Fatal("completed return kept invasion target occupied past bookkeeping grace")
	}
}

func TestInvasionTargetAvailabilityUsesNewestAuthoritativeObservation(t *testing.T) {
	state := NewGameState()
	hiddenAt := time.Date(2026, 9, 9, 12, 0, 0, 0, time.UTC)
	if !state.Invasion.MarkTargetUnavailable(0, 101, 102, hiddenAt) ||
		!state.Invasion.TargetUnavailable(0, 101, 102) {
		t.Fatal("hidden invasion target was not retained")
	}
	if state.Invasion.MarkTargetAvailable(0, 101, 102, hiddenAt.Add(-time.Second)) {
		t.Fatal("older map observation cleared a newer hidden-target signal")
	}
	if state.Invasion.MarkTargetAvailable(0, 101, 102, hiddenAt) {
		t.Fatal("same-time map observation cleared a hidden-target signal")
	}
	if !state.Invasion.MarkTargetAvailable(0, 101, 102, hiddenAt.Add(time.Second)) ||
		state.Invasion.TargetUnavailable(0, 101, 102) {
		t.Fatal("newer map reappearance did not restore invasion target")
	}
}

func TestInvasionTargetReservationRequiresNewEvidenceOrOwningOperation(t *testing.T) {
	state := NewGameState()
	reservedAt := time.Date(2026, 9, 9, 12, 0, 0, 0, time.UTC)
	reservation := InvasionTargetReservation{
		KingdomID: 0, TargetTypeID: MapTypeForeignLord, X: 101, Y: 102,
		OperationID: "attack-1", ReservedAt: reservedAt,
	}
	if !state.Invasion.ReserveTarget(reservation) {
		t.Fatal("invasion target was not reserved")
	}
	if _, found := state.Invasion.TargetReservation(0, 101, 102); !found {
		t.Fatal("invasion target reservation was not retained")
	}
	if state.Invasion.ReleaseTargetReservation(0, 101, 102, "attack-2") {
		t.Fatal("another operation released the reservation")
	}
	reconcileAfter := reservedAt.Add(time.Minute)
	if state.Invasion.DeferTargetReservation(0, 101, 102, "attack-2", reconcileAfter) {
		t.Fatal("another operation deferred the reservation")
	}
	if !state.Invasion.DeferTargetReservation(0, 101, 102, "attack-1", reconcileAfter) {
		t.Fatal("owning operation did not defer the reservation")
	}
	if state.Invasion.DeferTargetReservation(0, 101, 102, "attack-1", reservedAt) {
		t.Fatal("reservation reconciliation backoff regressed")
	}
	if state.Invasion.ReleaseTargetReservationObservedAfter(0, 101, 102, reservedAt) {
		t.Fatal("same-time evidence released the reservation")
	}
	if !state.Invasion.ReleaseTargetReservationObservedAfter(0, 101, 102, reservedAt.Add(time.Second)) {
		t.Fatal("newer authoritative evidence did not release the reservation")
	}

	state.Invasion.ReserveTarget(reservation)
	if !state.Invasion.ReleaseTargetReservation(0, 101, 102, "attack-1") {
		t.Fatal("owning operation did not release the reservation")
	}

	state.Invasion.ReserveTarget(reservation)
	store := NewStore(state)
	snapshot := store.Snapshot()
	delete(snapshot.Invasion.TargetReservations, InvasionTargetKey(0, 101, 102))
	if _, found := store.Snapshot().Invasion.TargetReservation(0, 101, 102); !found {
		t.Fatal("store snapshot aliased the durable invasion reservation map")
	}
}

func TestInvasionCommanderReservationDistinguishesLaunchFromTargetLock(t *testing.T) {
	reservedAt := time.Date(2026, 9, 9, 12, 0, 0, 0, time.UTC)
	state := NewGameState()
	reservation := InvasionTargetReservation{
		KingdomID: 0, EventID: 71, OccurrenceEndsAt: reservedAt.Add(time.Hour),
		TargetTypeID: MapTypeForeignLord, X: 101, Y: 102,
		SourceCastleID: 1, CommanderID: 7, CommanderKnown: true,
		OperationID: "indeterminate-cra", ReservedAt: reservedAt,
	}
	state.Invasion.ReserveTarget(reservation)
	if !InvasionCommanderReserved(state, 7) {
		t.Fatal("occurrence-bound CRA reservation did not hold its commander")
	}
	if state.Invasion.DeferTargetReservation(0, 101, 102, "indeterminate-cra", reservedAt.Add(time.Minute)) {
		t.Fatal("scoped movement absence downgraded a full CRA reservation")
	}
	if !state.Invasion.BackoffTargetReservation(0, 101, 102, "indeterminate-cra", reservedAt.Add(time.Minute)) {
		t.Fatal("could not back off full CRA reservation")
	}
	backedOff, found := state.Invasion.TargetReservation(0, 101, 102)
	if !found || !backedOff.CommanderKnown || backedOff.CommanderID != 7 || !InvasionCommanderReserved(state, 7) {
		t.Fatalf("full CRA backoff lost commander 7: %#v", backedOff)
	}

	state.Invasion.ReleaseTargetReservation(0, 101, 102, "indeterminate-cra")
	reservation.CommanderID = 0
	reservation.CommanderKnown = false
	reservation.OperationID = "target-only-adi"
	reservation.ReconcileAfter = time.Time{}
	state.Invasion.ReserveTarget(reservation)
	if !state.Invasion.DeferTargetReservation(0, 101, 102, "target-only-adi", reservedAt.Add(time.Minute)) {
		t.Fatal("could not defer target-only ADI reservation")
	}
	deferred, found := state.Invasion.TargetReservation(0, 101, 102)
	if !found || deferred.CommanderKnown || InvasionCommanderReserved(state, 7) {
		t.Fatalf("target-only lock unexpectedly owns a commander: %#v", deferred)
	}

	state.Invasion.ReleaseTargetReservation(0, 101, 102, "target-only-adi")
	reservation.CommanderID = 7
	reservation.CommanderKnown = true
	reservation.OperationID = "indeterminate-cra"
	reservation.OccurrenceEndsAt = time.Time{}
	state.Invasion.ReserveTarget(reservation)
	if InvasionCommanderReserved(state, 7) {
		t.Fatal("legacy reservation without an event boundary held a commander")
	}
}

func TestInvasionReservationMovementRequiresBoundSourceCommanderAndNewLaunch(t *testing.T) {
	reservedAt := time.Date(2026, 9, 9, 12, 0, 0, 0, time.UTC)
	commanderID := CommanderID(0)
	arrivesAt := reservedAt.Add(time.Minute)
	state := NewGameState()
	state.Castles[1] = CastleState{ID: 1, KingdomID: 0, X: 100, Y: 100}
	state.EventScores.ActiveEventID = 71
	state.SetScalableEventScore(71, ScalableEventScore{
		EventID: 71, RemainingSec: 7_200, ObservedAt: reservedAt,
	})
	state.Movements[10] = MovementState{
		ID: 10, Direction: 0, SourceCastleID: 1, SourceX: 100, SourceY: 100, CommanderID: &commanderID,
		KingdomID: 0, TargetTypeID: MapTypeForeignLord, TargetX: 101, TargetY: 102,
		StartedAt: reservedAt.Add(time.Second), ObservedAt: reservedAt.Add(2 * time.Second), ArrivesAt: &arrivesAt,
	}
	reservation := InvasionTargetReservation{
		KingdomID: 0, EventID: 71, OccurrenceEndsAt: reservedAt.Add(7_200 * time.Second),
		TargetTypeID: MapTypeForeignLord, X: 101, Y: 102,
		SourceCastleID: 1, SourceX: 100, SourceY: 100, SourceKnown: true,
		CommanderID: 0, CommanderKnown: true, ReservedAt: reservedAt,
	}
	if movement, found := InvasionReservationMovement(state, reservation); !found || movement.ID != 10 {
		t.Fatalf("matching reserved movement = %#v found=%t", movement, found)
	}
	returnMovement := MovementState{
		ID: 10, Direction: 1, SourceTypeID: MapTypeForeignLord, SourceX: 101, SourceY: 102,
		TargetCastleID: 1, TargetX: 100, TargetY: 100, CommanderID: &commanderID, KingdomID: 0,
		StartedAt: reservedAt.Add(20 * time.Second), ObservedAt: reservedAt.Add(21 * time.Second),
		ReturnsAt: &arrivesAt, TravelSeconds: 60,
	}
	state.Movements[10] = returnMovement
	delete(state.Castles, 1)
	if movement, found := InvasionReservationMovement(state, reservation); !found || movement.ID != 10 {
		t.Fatalf("short exact return movement was not recovered: %#v found=%t", movement, found)
	}
	returnMovement.StartedAt = reservedAt.Add(10 * time.Minute)
	returnMovement.ObservedAt = reservedAt.Add(11 * time.Minute)
	state.Movements[10] = returnMovement
	if _, found := InvasionReservationMovement(state, reservation); found {
		t.Fatal("late same-route return movement was treated as launch proof")
	}

	otherSource := returnMovement
	otherSource.Direction = 0
	otherSource.SourceCastleID = 2
	otherSource.SourceX = 200
	otherSource.SourceY = 200
	otherSource.TargetTypeID = MapTypeForeignLord
	otherSource.TargetX = 101
	otherSource.TargetY = 102
	otherSource.ArrivesAt = &arrivesAt
	otherSource.ReturnsAt = nil
	state.Movements[10] = otherSource
	if _, found := InvasionReservationMovement(state, reservation); found {
		t.Fatal("another source castle's movement matched the reservation")
	}

	old := otherSource
	old.SourceCastleID = 1
	old.StartedAt = reservedAt.Add(-time.Minute)
	state.Movements[10] = old
	if _, found := InvasionReservationMovement(state, reservation); found {
		t.Fatal("historical commander movement matched the new reservation")
	}

	late := otherSource
	late.SourceCastleID = 1
	late.SourceX = 100
	late.SourceY = 100
	late.StartedAt = reservedAt.Add(InvasionTargetReservationReconcileGrace + time.Second)
	late.ObservedAt = late.StartedAt.Add(time.Second)
	state.Movements[10] = late
	if _, found := InvasionReservationMovement(state, reservation); found {
		t.Fatal("late same-route outbound movement matched the reservation")
	}
	late.StartedAt = time.Time{}
	state.Movements[10] = late
	if _, found := InvasionReservationMovement(state, reservation); found {
		t.Fatal("movement without a start boundary matched the reservation")
	}

	unknownType := otherSource
	unknownType.SourceCastleID = 1
	unknownType.SourceX = 100
	unknownType.SourceY = 100
	unknownType.StartedAt = reservedAt.Add(time.Second)
	unknownType.ObservedAt = reservedAt.Add(2 * time.Second)
	unknownType.TargetTypeID = 0
	state.Movements[10] = unknownType
	if _, found := InvasionReservationMovement(state, reservation); found {
		t.Fatal("unknown movement target type matched the invasion reservation")
	}
	unknownType.TargetTypeID = -1
	state.Movements[10] = unknownType
	if _, found := InvasionReservationMovement(state, reservation); found {
		t.Fatal("negative movement target type matched the invasion reservation")
	}

	reservation.CommanderKnown = false
	if _, found := InvasionReservationMovement(state, reservation); found {
		t.Fatal("legacy reservation without commander identity attributed a movement")
	}
}

func TestInvasionMapProjectionRetainsAvailabilityButClientHidesPrivateLocks(t *testing.T) {
	observedAt := time.Date(2026, 9, 9, 12, 0, 0, 0, time.UTC)
	projected, retained := projectMapObservation(MapObservation{
		KingdomID: 0, TypeID: MapTypeForeignLord, X: 101, Y: 102, Level: 70,
		InvasionAvailabilityKnown: true, InvasionProtected: true, ObservedAt: observedAt,
	})
	if !retained || !projected.InvasionAvailabilityKnown || !projected.InvasionProtected {
		t.Fatalf("invasion map projection lost availability: %#v retained=%t", projected, retained)
	}
	state := NewGameState()
	state.Invasion.MarkTargetUnavailable(0, 101, 102, observedAt)
	state.Invasion.ReserveTarget(InvasionTargetReservation{
		KingdomID: 0, EventID: 71, TargetTypeID: MapTypeForeignLord, X: 101, Y: 102,
		OperationID: "private-operation", ReservedAt: observedAt,
	})
	client := state.clientStateProjection()
	if len(client.Invasion.UnavailableTargets) != 0 || len(client.Invasion.TargetReservations) != 0 {
		t.Fatalf("private invasion locks leaked to client state: %#v", client.Invasion)
	}
	if len(state.Invasion.UnavailableTargets) != 1 || len(state.Invasion.TargetReservations) != 1 {
		t.Fatal("client projection mutated durable invasion state")
	}
}

func TestNewStoreInvalidatesLegacyInvasionScanClock(t *testing.T) {
	now := time.Date(2026, 9, 9, 12, 0, 0, 0, time.UTC)
	state := NewGameState()
	state.Invasion.LastScannedAt[1] = now
	state.Map[0] = map[string]MapObservation{
		"101:102": {
			KingdomID: 0, TypeID: MapTypeForeignLord, X: 101, Y: 102,
			Level: 70, ObservedAt: now,
		},
	}
	if scannedAt := NewStore(state).Snapshot().Invasion.LastScannedAt[1]; !scannedAt.IsZero() {
		t.Fatalf("legacy invasion scan clock survived schema migration: %s", scannedAt)
	}
}
