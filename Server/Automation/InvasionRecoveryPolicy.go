package Automation

import (
	"context"
	"encoding/json"
	"sort"
	"time"

	"CitadelDesktop/Server/Intent"
	"CitadelDesktop/Server/State"
)

const invasionRecoveryMovementProbeDelay = 2 * time.Second

// InvasionRecoveryPolicy owns only durable CRA reservations. Keeping recovery
// in a core lane lets an accepted-but-indeterminate launch be accounted after
// Auto Invasion is disabled, its weekly schedule closes, or its attack lane
// enters a failure backoff.
type InvasionRecoveryPolicy struct{}

func NewInvasionRecoveryPolicy() *InvasionRecoveryPolicy { return &InvasionRecoveryPolicy{} }

func (*InvasionRecoveryPolicy) ID() string         { return "autoInvasionRecovery" }
func (*InvasionRecoveryPolicy) EnabledKey() string { return "" }
func (*InvasionRecoveryPolicy) ActorID() string    { return "autoInvasion" }
func (*InvasionRecoveryPolicy) CorePolicy()        {}

func (*InvasionRecoveryPolicy) WakeDomains() []string {
	return []string{"castles", "event-scores", "invasion", "map-invasion", "movement-snapshot", "movements", "reports"}
}

func (*InvasionRecoveryPolicy) Evaluate(_ context.Context, snapshot Snapshot) (Decision, error) {
	if decision, required := invasionReservationReconciliationDecision(snapshot, 0); required {
		return decision, nil
	}
	if probeAt, required := nextInvasionMovementProbe(snapshot); required {
		if !snapshot.Now.Before(probeAt) {
			return Decision{
				Status: "ready", Detail: "Check for an unresolved Auto Invasion launch",
				NextCheckAt:         snapshot.Now.Add(2 * time.Second),
				Metrics:             map[string]float64{"unresolvedLaunches": 1, "movementProbes": 1},
				Request:             &Intent.Request{Name: "game.refresh_movements", Arguments: json.RawMessage(`{}`)},
				ReevaluateOnSuccess: true, ReevaluateOnStale: true,
			}, nil
		}
		return Decision{
			Status: "waiting", Detail: "Waiting to check an unresolved Auto Invasion launch",
			NextCheckAt: probeAt,
		}, nil
	}
	next := nextInvasionReservationReconciliation(snapshot)
	if next.IsZero() {
		if reservation, exhausted := firstExhaustedInvasionReservation(snapshot.State); exhausted {
			nextCheckAt := time.Time{}
			if reservation.OccurrenceEndsAt.After(snapshot.Now) {
				nextCheckAt = reservation.OccurrenceEndsAt
			}
			return Decision{
				Status: "blocked", Detail: "An Auto Invasion launch could not be proven after scoped movement checks; " +
					"its commander and target remain reserved to prevent a duplicate attack until a new event occurrence is observed",
				NextCheckAt: nextCheckAt, EventDriven: true,
				Metrics: map[string]float64{"unresolvedLaunches": 1, "recoveryExhausted": 1},
			}, nil
		}
		return Decision{
			Status: "idle", Detail: "No unresolved Auto Invasion launches", EventDriven: true,
		}, nil
	}
	if !next.After(snapshot.Now) {
		next = snapshot.Now.Add(defaultRetry)
	}
	return Decision{
		Status: "waiting", Detail: "Waiting to verify an unresolved Auto Invasion launch",
		NextCheckAt: next,
	}, nil
}

func nextInvasionMovementProbe(snapshot Snapshot) (time.Time, bool) {
	keys := make([]string, 0, len(snapshot.State.Invasion.TargetReservations))
	for key := range snapshot.State.Invasion.TargetReservations {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	var next time.Time
	for _, key := range keys {
		reservation := snapshot.State.Invasion.TargetReservations[key]
		if !reservation.CommanderKnown || !reservation.RecoveryExhaustedAt.IsZero() ||
			reservation.OperationID == "" || reservation.ReservedAt.IsZero() {
			continue
		}
		if _, matched := State.InvasionReservationMovement(snapshot.State, reservation); matched {
			continue
		}
		probeAt := reservation.ReservedAt.Add(invasionRecoveryMovementProbeDelay)
		dueAt := reservation.ReservedAt.Add(State.InvasionTargetReservationReconcileGrace)
		if reservation.ReconcileAfter.After(dueAt) {
			dueAt = reservation.ReconcileAfter
		}
		if !snapshot.Now.Before(dueAt) || !snapshot.State.MovementSnapshot.ObservedAt.Before(probeAt) {
			continue
		}
		if next.IsZero() || probeAt.Before(next) {
			next = probeAt
		}
	}
	return next, !next.IsZero()
}

func nextInvasionReservationReconciliation(snapshot Snapshot) time.Time {
	var next time.Time
	for _, reservation := range snapshot.State.Invasion.TargetReservations {
		if reservation.OperationID == "" || reservation.ReservedAt.IsZero() ||
			!reservation.RecoveryExhaustedAt.IsZero() {
			continue
		}
		dueAt := reservation.ReservedAt.Add(State.InvasionTargetReservationReconcileGrace)
		if reservation.ReconcileAfter.After(dueAt) {
			dueAt = reservation.ReconcileAfter
		}
		if next.IsZero() || dueAt.Before(next) {
			next = dueAt
		}
	}
	return next
}

func firstExhaustedInvasionReservation(gameState State.GameState) (State.InvasionTargetReservation, bool) {
	keys := make([]string, 0, len(gameState.Invasion.TargetReservations))
	for key := range gameState.Invasion.TargetReservations {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		reservation := gameState.Invasion.TargetReservations[key]
		if reservation.CommanderKnown && !reservation.RecoveryExhaustedAt.IsZero() {
			return reservation, true
		}
	}
	return State.InvasionTargetReservation{}, false
}

var _ Policy = (*InvasionRecoveryPolicy)(nil)
var _ CorePolicy = (*InvasionRecoveryPolicy)(nil)
var _ ActorIDPolicy = (*InvasionRecoveryPolicy)(nil)
var _ StateWakePolicy = (*InvasionRecoveryPolicy)(nil)
