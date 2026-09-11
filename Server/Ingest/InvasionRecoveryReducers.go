package Ingest

import (
	"context"
	"sort"
	"time"

	"CitadelDesktop/Server/GameData"
	"CitadelDesktop/Server/Protocol"
	"CitadelDesktop/Server/State"
)

const invasionLaunchDurabilityDomain = "invasion-launch-durable"

type InvasionLaunchReconciliation struct {
	EventChanged       bool
	AnalyticsChanged   bool
	ReportChanged      bool
	ReservationChanged bool
}

func (result InvasionLaunchReconciliation) Changed() bool {
	return result.EventChanged || result.AnalyticsChanged || result.ReportChanged || result.ReservationChanged
}

// ReconcileInvasionReservationMovement accounts only a movement that satisfies
// the reservation's event occurrence, source castle/coordinates, commander,
// target, and post-reservation observation boundary.
func ReconcileInvasionReservationMovement(
	gameState *State.GameState,
	reservation State.InvasionTargetReservation,
	movement State.MovementState,
) (InvasionLaunchReconciliation, error) {
	if gameState == nil {
		return InvasionLaunchReconciliation{}, nil
	}
	matched, found := State.InvasionReservationMovement(*gameState, reservation)
	if !found || matched.ID != movement.ID {
		return InvasionLaunchReconciliation{}, nil
	}
	launchedAt := reservation.ReservedAt.UTC()
	arrivesAt := time.Time{}
	if movement.Direction == 0 && movement.ArrivesAt != nil {
		if !movement.StartedAt.IsZero() {
			launchedAt = movement.StartedAt.UTC()
		}
		arrivesAt = movement.ArrivesAt.UTC()
	} else if movement.Direction == 1 && !movement.StartedAt.IsZero() {
		arrivesAt = movement.StartedAt.UTC()
	}
	result := InvasionLaunchReconciliation{}
	result.EventChanged = State.RecordEventAttackLaunchForOccurrence(
		gameState, reservation.EventID, reservation.OccurrenceEndsAt, State.EventAttackRecord{
			MovementID: movement.ID, Kind: State.EventActivityInvasion, KingdomID: reservation.KingdomID,
			TargetTypeID: reservation.TargetTypeID, TargetX: reservation.X, TargetY: reservation.Y,
			LaunchedAt: launchedAt, ArrivesAt: arrivesAt,
		})
	result.AnalyticsChanged = State.RecordAttackFeatureLaunch(gameState, State.AttackFeatureLaunch{
		MovementID: movement.ID, FeatureID: State.AttackFeatureAutoInvasion, KingdomID: reservation.KingdomID,
		TargetTypeID: reservation.TargetTypeID, TargetX: reservation.X, TargetY: reservation.Y,
		LaunchedAt: launchedAt, ArrivesAt: arrivesAt,
	})
	var err error
	result.ReportChanged, err = ReconcileRetainedBattleCapturesForRecoveredLaunch(gameState, movement.ID)
	if err != nil {
		return InvasionLaunchReconciliation{}, err
	}
	result.ReservationChanged = gameState.Invasion.ReleaseTargetReservation(
		reservation.KingdomID, reservation.X, reservation.Y, reservation.OperationID,
	)
	return result, nil
}

func reduceInvasionReservationMovements(
	_ context.Context,
	frame Protocol.Frame,
	gameState *State.GameState,
	_ *GameData.Store,
) ([]string, bool, error) {
	if !frameSucceeded(frame) || gameState == nil || len(gameState.Invasion.TargetReservations) == 0 {
		return nil, false, nil
	}
	keys := make([]string, 0, len(gameState.Invasion.TargetReservations))
	for key := range gameState.Invasion.TargetReservations {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	changed := false
	reportChanged := false
	for _, key := range keys {
		reservation, exists := gameState.Invasion.TargetReservations[key]
		if !exists {
			continue
		}
		movement, matched := State.InvasionReservationMovement(*gameState, reservation)
		if !matched {
			continue
		}
		result, err := ReconcileInvasionReservationMovement(gameState, reservation, movement)
		if err != nil {
			return nil, false, err
		}
		changed = changed || result.Changed()
		reportChanged = reportChanged || result.ReportChanged
	}
	if !changed {
		return nil, false, nil
	}
	domains := []string{
		"attack-analytics", "event-scores", "invasion", "movements",
		invasionLaunchDurabilityDomain,
	}
	if reportChanged {
		domains = append(domains, "reports")
	}
	return domains, true, nil
}
