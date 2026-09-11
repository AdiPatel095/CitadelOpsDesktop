package Automation

import (
	"encoding/json"
	"testing"
	"time"

	"CitadelDesktop/Server/Configuration"
	"CitadelDesktop/Server/Intent"
	"CitadelDesktop/Server/State"
)

func TestInvasionRecoveryPolicyIsCoreAndAttributedToAutoInvasion(t *testing.T) {
	policy := NewInvasionRecoveryPolicy()
	if _, ok := any(policy).(CorePolicy); !ok {
		t.Fatal("invasion recovery must bypass feature enable and schedule gates")
	}
	if !policyEnabled(policy, map[string]bool{}, State.GameState{}) || policyScheduleKey(policy) != "" || policyActorID(policy) != "autoInvasion" {
		t.Fatalf("recovery policy routing: enabled=%t schedule=%q actor=%q",
			policyEnabled(policy, map[string]bool{}, State.GameState{}), policyScheduleKey(policy), policyActorID(policy))
	}
}

func TestInvasionRecoveryProbesEarlyAndCapturesShortReturn(t *testing.T) {
	reservedAt := time.Date(2026, 9, 9, 12, 0, 0, 0, time.UTC)
	now := reservedAt.Add(3 * time.Second)
	gameState := coordinatorReadyState()
	gameState.Player.ID = 10
	gameState.Castles[1] = State.CastleState{ID: 1, KingdomID: 0, X: 100, Y: 100}
	gameState.EventScores.ActiveEventID = foreignLordsEventID
	gameState.EventScores.ByEvent[foreignLordsEventID] = State.ScalableEventScore{
		EventID: foreignLordsEventID, RemainingSec: 3_600, ObservedAt: reservedAt,
	}
	occurrenceEndsAt := State.ScalableEventEndsAt(gameState.EventScores.ByEvent[foreignLordsEventID])
	commanderID := State.CommanderID(7)
	gameState.Invasion.ReserveTarget(State.InvasionTargetReservation{
		KingdomID: 0, EventID: foreignLordsEventID, OccurrenceEndsAt: occurrenceEndsAt,
		TargetTypeID: foreignLordsMapTypeID, X: 101, Y: 102,
		SourceCastleID: 1, SourceX: 100, SourceY: 100, SourceKnown: true,
		CommanderID: commanderID, CommanderKnown: true,
		OperationID: "short-cra", ReservedAt: reservedAt,
	})
	gameState.MovementSnapshot.ObservedAt = reservedAt.Add(-time.Second)
	policy := NewInvasionRecoveryPolicy()
	decision, err := policy.Evaluate(t.Context(), Snapshot{State: gameState, Now: now})
	if err != nil || decision.Request == nil || decision.Request.Name != "game.refresh_movements" {
		t.Fatalf("early recovery probe = %#v err=%v", decision, err)
	}

	returnsAt := now.Add(time.Second)
	gameState.Movements[9] = State.MovementState{
		ID: 9, Direction: 1, SourceTypeID: foreignLordsMapTypeID, SourceX: 101, SourceY: 102,
		TargetCastleID: 1, TargetX: 100, TargetY: 100, CommanderID: &commanderID, KingdomID: 0,
		StartedAt: reservedAt.Add(time.Second), ObservedAt: now, ReturnsAt: &returnsAt,
	}
	gameState.MovementSnapshot.ObservedAt = now
	decision, err = policy.Evaluate(t.Context(), Snapshot{State: gameState, Now: now})
	if err != nil || decision.Request == nil || decision.Request.Name != "invasion.target.reconcile" {
		t.Fatalf("short return recovery = %#v err=%v", decision, err)
	}
	var boundary struct {
		MatchedMovementID State.MovementID `json:"matchedMovementId"`
	}
	if err := json.Unmarshal(decision.Request.Arguments, &boundary); err != nil || boundary.MatchedMovementID != 9 {
		t.Fatalf("short return recovery boundary = %+v err=%v", boundary, err)
	}
}

func TestInvasionRecoveryPrefersLaterExactMovementOverEarlierDueReservation(t *testing.T) {
	reservedAt := time.Date(2026, 9, 9, 12, 0, 0, 0, time.UTC)
	now := reservedAt.Add(State.InvasionTargetReservationReconcileGrace + time.Second)
	gameState := coordinatorReadyState()
	gameState.Castles[1] = State.CastleState{ID: 1, KingdomID: 0, X: 100, Y: 100}
	gameState.EventScores.ByEvent[foreignLordsEventID] = State.ScalableEventScore{
		EventID: foreignLordsEventID, RemainingSec: 3_600, ObservedAt: reservedAt,
	}
	occurrenceEndsAt := State.ScalableEventEndsAt(gameState.EventScores.ByEvent[foreignLordsEventID])
	for index, x := range []int{101, 102} {
		gameState.Invasion.ReserveTarget(State.InvasionTargetReservation{
			KingdomID: 0, EventID: foreignLordsEventID, OccurrenceEndsAt: occurrenceEndsAt,
			TargetTypeID: foreignLordsMapTypeID, X: x, Y: 100,
			SourceCastleID: 1, SourceX: 100, SourceY: 100, SourceKnown: true,
			CommanderID: State.CommanderID(7 + index), CommanderKnown: true,
			OperationID: []string{"due-a", "matched-b"}[index], ReservedAt: reservedAt,
		})
	}
	commanderID := State.CommanderID(8)
	arrivesAt := now.Add(time.Minute)
	gameState.Movements[9] = State.MovementState{
		ID: 9, Direction: 0, SourceCastleID: 1, SourceX: 100, SourceY: 100,
		CommanderID: &commanderID, KingdomID: 0, TargetTypeID: foreignLordsMapTypeID,
		TargetX: 102, TargetY: 100, StartedAt: reservedAt.Add(time.Second),
		ObservedAt: now, ArrivesAt: &arrivesAt,
	}
	decision, err := NewInvasionRecoveryPolicy().Evaluate(t.Context(), Snapshot{State: gameState, Now: now})
	if err != nil || decision.Request == nil || decision.Request.Name != "invasion.target.reconcile" {
		t.Fatalf("positive-first recovery decision = %#v err=%v", decision, err)
	}
	var boundary struct {
		TargetX           int              `json:"targetX"`
		MatchedMovementID State.MovementID `json:"matchedMovementId"`
	}
	if err := json.Unmarshal(decision.Request.Arguments, &boundary); err != nil ||
		boundary.TargetX != 102 || boundary.MatchedMovementID != 9 {
		t.Fatalf("positive-first recovery boundary = %+v err=%v", boundary, err)
	}
}

func TestInvasionRecoveryExhaustionStopsPollingUntilNewOccurrence(t *testing.T) {
	reservedAt := time.Date(2026, 9, 9, 12, 0, 0, 0, time.UTC)
	now := reservedAt.Add(2 * time.Minute)
	gameState := coordinatorReadyState()
	gameState.EventScores.ByEvent[foreignLordsEventID] = State.ScalableEventScore{
		EventID: foreignLordsEventID, RemainingSec: 3_600, ObservedAt: reservedAt,
	}
	occurrenceEndsAt := State.ScalableEventEndsAt(gameState.EventScores.ByEvent[foreignLordsEventID])
	gameState.Invasion.ReserveTarget(State.InvasionTargetReservation{
		KingdomID: 0, EventID: foreignLordsEventID, OccurrenceEndsAt: occurrenceEndsAt,
		TargetTypeID: foreignLordsMapTypeID, X: 101, Y: 102,
		SourceCastleID: 1, CommanderID: 7, CommanderKnown: true,
		OperationID: "exhausted-cra", ReservedAt: reservedAt,
		ReconcileAttempts:   State.InvasionTargetReservationMaxReconcileAttempts,
		RecoveryExhaustedAt: now.Add(-time.Second),
	})
	policy := NewInvasionRecoveryPolicy()
	for _, evaluatedAt := range []time.Time{now, now.Add(time.Minute)} {
		decision, err := policy.Evaluate(t.Context(), Snapshot{State: gameState, Now: evaluatedAt})
		if err != nil || decision.Request != nil || decision.Status != "blocked" ||
			decision.Metrics["recoveryExhausted"] != 1 || decision.Metrics["unresolvedLaunches"] != 1 {
			t.Fatalf("exhausted recovery at %s = %#v err=%v", evaluatedAt, decision, err)
		}
	}
}

func TestCoordinatorRunsPersistedInvasionRecoveryWhenParentDisabledOrScheduleClosed(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	for _, test := range []struct {
		name      string
		enabled   json.RawMessage
		scheduler json.RawMessage
	}{
		{name: "parent disabled", enabled: json.RawMessage(`{"auto_invasion":false}`), scheduler: json.RawMessage(`{}`)},
		{
			name: "parent schedule closed", enabled: json.RawMessage(`{"auto_invasion":true}`),
			scheduler: json.RawMessage(`{
				"featureSchedules":{"autoInvasion":{"enabled":true,"timeZone":"UTC","slots":[]}}
			}`),
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			gameState := coordinatorReadyState()
			gameState.Player.ID = 10
			gameState.EventScores.ActiveEventID = foreignLordsEventID
			gameState.EventScores.ByEvent[foreignLordsEventID] = State.ScalableEventScore{
				EventID: foreignLordsEventID, RemainingSec: 3_600, ObservedAt: now,
			}
			occurrenceEndsAt := State.ScalableEventEndsAt(gameState.EventScores.ByEvent[foreignLordsEventID])
			reservedAt := now.Add(-5 * time.Second)
			commanderID := State.CommanderID(7)
			gameState.Invasion.ReserveTarget(State.InvasionTargetReservation{
				KingdomID: 0, EventID: foreignLordsEventID, OccurrenceEndsAt: occurrenceEndsAt,
				TargetTypeID: foreignLordsMapTypeID, X: 101, Y: 102,
				SourceCastleID: 1, CommanderID: commanderID, CommanderKnown: true,
				OperationID: "persisted-indeterminate-cra", ReservedAt: reservedAt,
			})
			arrivesAt := now.Add(time.Minute)
			gameState.Movements[9] = State.MovementState{
				ID: 9, Direction: 0, SourceCastleID: 1, CommanderID: &commanderID,
				KingdomID: 0, TargetTypeID: foreignLordsMapTypeID, TargetX: 101, TargetY: 102,
				StartedAt: reservedAt.Add(time.Second), ObservedAt: now, ArrivesAt: &arrivesAt,
			}
			state := State.NewStore(gameState)
			configuration, err := Configuration.Open(t.TempDir(), map[string]json.RawMessage{
				"automation.enabled": test.enabled,
				"scheduler":          test.scheduler,
			})
			if err != nil {
				t.Fatal(err)
			}
			policy := NewInvasionRecoveryPolicy()
			submitter := &coordinatorTestSubmitter{calls: make(chan Intent.Request, 1)}
			coordinator := NewCoordinator(state, configuration, nil, submitter, policy)
			coordinator.evaluate(t.Context(), map[string]*policyRuntime{policy.ID(): {}}, make(chan operationResult, 1))

			request := waitForCoordinatorRequest(t, submitter.calls)
			if request.Name != "invasion.target.reconcile" || request.Actor != "automation:autoInvasion" {
				t.Fatalf("recovery request = %#v", request)
			}
			var boundary struct {
				MatchedMovementID State.MovementID `json:"matchedMovementId"`
			}
			if err := json.Unmarshal(request.Arguments, &boundary); err != nil || boundary.MatchedMovementID != 9 {
				t.Fatalf("recovery boundary = %+v err=%v", boundary, err)
			}
		})
	}
}
