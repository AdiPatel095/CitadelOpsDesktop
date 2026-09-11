package App

import (
	"encoding/json"
	"reflect"
	"testing"
	"time"

	"CitadelDesktop/Server/Automation"
	"CitadelDesktop/Server/Configuration"
	"CitadelDesktop/Server/GameData"
	"CitadelDesktop/Server/Intent"
	"CitadelDesktop/Server/Outbound"
	"CitadelDesktop/Server/Protocol"
	"CitadelDesktop/Server/State"
)

func TestInvasionEventOccurrenceFlowsFromPolicyThroughPersistedReservation(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	gameState, gameData, configuration := invasionOccurrenceFixture(t, now)
	eventEndsAt := State.ScalableEventEndsAt(gameState.EventScores.ByEvent[71])

	decision, err := Automation.NewAutoInvasionPolicy().Evaluate(t.Context(), Automation.Snapshot{
		State: gameState, GameData: gameData, Configuration: configuration, Now: now,
	})
	if err != nil || decision.Request == nil || decision.Request.Name != "invasion.attack" {
		t.Fatalf("invasion occurrence policy decision = %#v, err=%v", decision, err)
	}
	var policyRequest invasionAttackRequest
	if err := json.Unmarshal(decision.Request.Arguments, &policyRequest); err != nil {
		t.Fatal(err)
	}
	if !policyRequest.EventEndsAt.Equal(eventEndsAt) {
		t.Fatalf("policy event occurrence = %s, want %s", policyRequest.EventEndsAt, eventEndsAt)
	}

	plan, err := planInvasionAttack(
		t.Context(), Intent.PlanningContext{State: gameState, GameData: gameData}, decision.Request.Arguments,
	)
	if err != nil {
		t.Fatal(err)
	}
	var launch Intent.Step
	for _, step := range plan.Steps {
		if step.Resolver == "invasion.attack.build" {
			launch = step
			break
		}
	}
	if launch.Resolver != "invasion.attack.build" || launch.CommandDependencies == nil {
		t.Fatalf("invasion plan has no deferred CRA launch: %#v", plan.Steps)
	}
	var planned resolvedInvasionAttackRequest
	if err := json.Unmarshal(launch.ResolverArguments, &planned); err != nil {
		t.Fatal(err)
	}
	if !planned.EventEndsAt.Equal(eventEndsAt) {
		t.Fatalf("planned event occurrence = %s, want %s", planned.EventEndsAt, eventEndsAt)
	}

	dependencies, err := (&Application{}).resolveCRACommandDependencies(
		t.Context(), Intent.PlanningContext{State: gameState}, Intent.Step{
			Payload:           launch.CommandDependencies.Payload,
			ResolverArguments: launch.ResolverArguments,
			Command: Protocol.Command{
				Opcode: "cra", Payload: launch.CommandDependencies.Payload,
			},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(dependencies.Steps) == 0 {
		t.Fatal("invasion CRA dependencies are empty")
	}
	finalDependency := dependencies.Steps[len(dependencies.Steps)-1]
	if finalDependency.Action != "invasion.attack.guard" {
		t.Fatalf("final invasion dependency action = %q, want invasion.attack.guard: %#v", finalDependency.Action, dependencies.Steps)
	}
	var guarded resolvedInvasionAttackRequest
	if err := json.Unmarshal(finalDependency.ActionArguments, &guarded); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(guarded, planned) {
		t.Fatalf("final invasion guard lost resolved request: guarded=%#v planned=%#v", guarded, planned)
	}

	application := &Application{DataDir: t.TempDir(), State: State.NewStore(gameState)}
	resolved, err := application.resolveInvasionAttackStep(
		t.Context(), Intent.PlanningContext{State: gameState, GameData: gameData}, launch.ResolverArguments,
	)
	if err != nil {
		t.Fatal(err)
	}
	ctx := Outbound.WithMetadata(t.Context(), Outbound.Metadata{OperationID: "occurrence-flow"})
	if err := application.reserveInvasionTarget(ctx, resolved.PreDispatchArguments); err != nil {
		t.Fatal(err)
	}
	reservation, reserved := application.State.ReadOnlyView().Invasion.TargetReservation(0, 101, 100)
	if !reserved || !reservation.OccurrenceEndsAt.Equal(eventEndsAt) {
		t.Fatalf("in-memory reservation occurrence = %#v, found=%t", reservation, reserved)
	}
	persisted, err := State.LoadSnapshot(application.DataDir)
	if err != nil {
		t.Fatal(err)
	}
	persistedReservation, persistedFound := persisted.Invasion.TargetReservation(0, 101, 100)
	if !persistedFound || persistedReservation.OperationID != "occurrence-flow" ||
		!persistedReservation.OccurrenceEndsAt.Equal(eventEndsAt) {
		t.Fatalf("persisted reservation occurrence = %#v, found=%t", persistedReservation, persistedFound)
	}
}

func TestInvasionCapturePersistsReducerAccountingFence(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	gameState := State.NewGameState()
	gameState.EventScores.ByEvent[71] = State.ScalableEventScore{
		EventID: 71, RemainingSec: 7_200, ObservedAt: now,
	}
	occurrenceEndsAt := State.ScalableEventEndsAt(gameState.EventScores.ByEvent[71])
	commanderID := State.CommanderID(7)
	arrivesAt := now.Add(time.Minute)
	gameState.Movements[9] = State.MovementState{
		ID: 9, Direction: 0, SourceCastleID: 1, CommanderID: &commanderID,
		KingdomID: 0, TargetTypeID: State.MapTypeForeignLord, TargetX: 101, TargetY: 100,
		ObservedAt: now, StartedAt: now.Add(-time.Second), ArrivesAt: &arrivesAt,
	}
	record := State.EventAttackRecord{
		MovementID: 9, Kind: State.EventActivityInvasion, KingdomID: 0,
		TargetTypeID: State.MapTypeForeignLord, TargetX: 101, TargetY: 100,
		LaunchedAt: now, ArrivesAt: arrivesAt,
	}
	if !State.RecordEventAttackLaunchForOccurrence(&gameState, 71, occurrenceEndsAt, record) ||
		!State.RecordAttackFeatureLaunch(&gameState, State.AttackFeatureLaunch{
			MovementID: 9, FeatureID: State.AttackFeatureAutoInvasion, KingdomID: 0,
			TargetTypeID: State.MapTypeForeignLord, TargetX: 101, TargetY: 100,
			LaunchedAt: now, ArrivesAt: arrivesAt,
		}) {
		t.Fatal("could not stage movement-reducer accounting")
	}
	// Reproduce the rollover race: the next SEI occurrence arrives after the
	// movement reducer accounts the old one but before the post-send action.
	gameState.SetScalableEventScore(71, State.ScalableEventScore{
		EventID: 71, RemainingSec: 14_400, ObservedAt: now,
	})
	dataDir := t.TempDir()
	application := &Application{DataDir: dataDir, State: State.NewStore(gameState)}
	arguments, _ := json.Marshal(resolvedInvasionAttackRequest{
		invasionAttackRequest: invasionAttackRequest{
			SourceCastleID: 1, EventID: 71, EventEndsAt: occurrenceEndsAt,
			TargetTypeID: State.MapTypeForeignLord, KingdomID: 0, TargetX: 101, TargetY: 100,
		},
		CommanderID: commanderID,
	})
	if err := application.captureInvasionLaunch(t.Context(), arguments); err != nil {
		t.Fatal(err)
	}

	loaded, err := State.LoadSnapshot(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	activity, found := loaded.LookupEventActivity(71)
	if !found || activity.Invasion.Launches != 1 || len(activity.PendingAttacks) != 1 ||
		len(loaded.AttackAnalytics.PendingAttacks) != 1 || loaded.Revision == 0 {
		t.Fatalf("durable reducer accounting fence: revision=%d activity=%#v analytics=%#v",
			loaded.Revision, activity, loaded.AttackAnalytics)
	}
}

func TestInvasionReconciliationReleasesPriorOccurrenceWithoutAttributingMovement(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	gameState, gameData, configuration := invasionOccurrenceFixture(t, now)
	currentEndsAt := State.ScalableEventEndsAt(gameState.EventScores.ByEvent[71])
	priorEndsAt := currentEndsAt.Add(-7 * 24 * time.Hour)
	reservedAt := now.Add(-State.InvasionTargetReservationReconcileGrace - 5*time.Second)
	reconcileAfter := reservedAt.Add(State.InvasionTargetReservationReconcileGrace)
	commanderID := State.CommanderID(0)
	arrivesAt := now.Add(time.Minute)

	gameState.EventScores.ActivityByEvent[71] = State.EventActivityState{
		EventID: 71, OccurrenceEndsAt: currentEndsAt, ObservedFrom: now,
	}
	gameState.Invasion.ReserveTarget(State.InvasionTargetReservation{
		KingdomID: 0, EventID: 71, OccurrenceEndsAt: priorEndsAt,
		TargetTypeID: State.MapTypeForeignLord, X: 101, Y: 100,
		SourceCastleID: 1, CommanderID: commanderID, CommanderKnown: true,
		OperationID: "prior-occurrence-cra", ReservedAt: reservedAt, ReconcileAfter: reconcileAfter,
	})
	gameState.Movements[9] = State.MovementState{
		ID: 9, Direction: 0, SourceCastleID: 1, CommanderID: &commanderID,
		KingdomID: 0, TargetTypeID: State.MapTypeForeignLord, TargetX: 101, TargetY: 100,
		StartedAt: reservedAt.Add(time.Second), ObservedAt: now.Add(-time.Second),
		ArrivesAt: &arrivesAt, TravelSeconds: 60,
	}

	decision, err := Automation.NewInvasionRecoveryPolicy().Evaluate(t.Context(), Automation.Snapshot{
		State: gameState, GameData: gameData, Configuration: configuration, Now: now,
	})
	if err != nil || decision.Request == nil || decision.Request.Name != "invasion.target.reconcile" {
		t.Fatalf("prior-occurrence reconciliation decision = %#v, err=%v", decision, err)
	}
	var requested invasionTargetReconcileRequest
	if err := json.Unmarshal(decision.Request.Arguments, &requested); err != nil {
		t.Fatal(err)
	}
	if !requested.OccurrenceEndsAt.Equal(priorEndsAt) ||
		State.SameEventOccurrence(requested.OccurrenceEndsAt, currentEndsAt) {
		t.Fatalf("reconciliation request occurrence = %s, prior=%s current=%s", requested.OccurrenceEndsAt, priorEndsAt, currentEndsAt)
	}

	plan, err := planInvasionTargetReconcile(
		t.Context(), Intent.PlanningContext{State: gameState}, decision.Request.Arguments,
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Steps) != 1 || plan.Steps[0].Action != "invasion.target.reconcile" || plan.Steps[0].Opcode != "" {
		t.Fatalf("prior-occurrence reconciliation plan = %#v", plan.Steps)
	}
	verificationArguments := plan.Steps[0].ActionArguments
	var verification invasionTargetReconcileRequest
	if err := json.Unmarshal(verificationArguments, &verification); err != nil {
		t.Fatal(err)
	}
	if !verification.OccurrenceEndsAt.Equal(priorEndsAt) || !verification.ReconcileStartedAt.IsZero() {
		t.Fatalf("planned reconciliation boundary = %#v", verification)
	}

	store := State.NewStore(gameState)
	application := &Application{State: store}
	if err := application.reconcileInvasionTargetReservation(t.Context(), verificationArguments); err != nil {
		t.Fatal(err)
	}

	result := store.ReadOnlyView()
	if _, reserved := result.Invasion.TargetReservation(0, 101, 100); reserved {
		t.Fatal("fresh reconciliation evidence retained the prior-occurrence reservation")
	}
	activity, found := result.LookupEventActivity(71)
	if !found || !State.SameEventOccurrence(activity.OccurrenceEndsAt, currentEndsAt) ||
		activity.Invasion.Launches != 0 || len(activity.LaunchIDs) != 0 || len(activity.PendingAttacks) != 0 {
		t.Fatalf("prior-occurrence movement was attributed to current event: %#v, found=%t", activity, found)
	}
	if len(result.AttackAnalytics.LaunchIDs) != 0 || len(result.AttackAnalytics.PendingAttacks) != 0 {
		t.Fatalf("prior-occurrence movement was attributed to attack analytics: %#v", result.AttackAnalytics)
	}
}

func TestInvasionReconciliationRecordsLostSourceReturnWithoutRefocus(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	gameState, gameData, configuration := invasionOccurrenceFixture(t, now)
	occurrenceEndsAt := State.ScalableEventEndsAt(gameState.EventScores.ByEvent[71])
	reservedAt := now.Add(-5 * time.Second)
	reconcileAfter := reservedAt.Add(State.InvasionTargetReservationReconcileGrace)
	commanderID := State.CommanderID(0)
	arrivesAt := now.Add(time.Minute)

	delete(gameState.Castles, 1)
	gameState.Castles[2] = State.CastleState{
		ID: 2, Name: "Fallback", KingdomID: 0, X: 200, Y: 200, Focused: true,
	}
	gameState.Invasion.ReserveTarget(State.InvasionTargetReservation{
		KingdomID: 0, EventID: 71, OccurrenceEndsAt: occurrenceEndsAt,
		TargetTypeID: State.MapTypeForeignLord, X: 101, Y: 100,
		SourceCastleID: 1, SourceX: 100, SourceY: 100, SourceKnown: true,
		CommanderID: commanderID, CommanderKnown: true,
		OperationID: "lost-source-cra", ReservedAt: reservedAt, ReconcileAfter: reconcileAfter,
	})
	gameState.Movements[9] = State.MovementState{
		ID: 9, Direction: 1, SourceTypeID: State.MapTypeForeignLord, SourceX: 101, SourceY: 100,
		TargetCastleID: 0, TargetX: 100, TargetY: 100, CommanderID: &commanderID, KingdomID: 0,
		StartedAt: reservedAt.Add(time.Second), ObservedAt: now,
		ReturnsAt: &arrivesAt, TravelSeconds: 60,
	}

	decision, err := Automation.NewInvasionRecoveryPolicy().Evaluate(t.Context(), Automation.Snapshot{
		State: gameState, GameData: gameData, Configuration: configuration, Now: now,
	})
	if err != nil || decision.Request == nil || decision.Request.Name != "invasion.target.reconcile" {
		t.Fatalf("lost-source reconciliation decision = %#v, err=%v", decision, err)
	}
	var request invasionTargetReconcileRequest
	if err := json.Unmarshal(decision.Request.Arguments, &request); err != nil {
		t.Fatal(err)
	}
	if request.FocusCastleID != 0 || request.MatchedMovementID != 9 {
		t.Fatalf("positive-evidence reconciliation boundary = %#v", request)
	}
	reservation, reserved := gameState.Invasion.TargetReservation(0, 101, 100)
	if !reserved || reservation.SourceCastleID != 1 {
		t.Fatalf("persisted attribution source = %#v, found=%t", reservation, reserved)
	}

	plan, err := planInvasionTargetReconcile(
		t.Context(), Intent.PlanningContext{State: gameState}, decision.Request.Arguments,
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Steps) != 1 || plan.Steps[0].Action != "invasion.target.reconcile" || plan.Steps[0].Opcode != "" {
		t.Fatalf("positive-evidence reconciliation plan = %#v", plan.Steps)
	}
	verificationArguments := plan.Steps[0].ActionArguments
	var verification invasionTargetReconcileRequest
	if err := json.Unmarshal(verificationArguments, &verification); err != nil {
		t.Fatal(err)
	}
	store := State.NewStore(gameState)
	application := &Application{State: store}
	if err := application.reconcileInvasionTargetReservation(t.Context(), verificationArguments); err != nil {
		t.Fatal(err)
	}
	result := store.ReadOnlyView()
	activity, found := result.LookupEventActivity(71)
	if _, reserved := result.Invasion.TargetReservation(0, 101, 100); reserved ||
		!found || activity.Invasion.Launches != 1 || len(activity.PendingAttacks) != 1 ||
		activity.PendingAttacks[0].MovementID != 9 || len(result.AttackAnalytics.PendingAttacks) != 1 {
		t.Fatalf("fallback-focus attribution result: invasion=%#v activity=%#v analytics=%#v", result.Invasion, activity, result.AttackAnalytics)
	}
}

func TestInvasionReconciliationPersistsFullMarkerBackoffAfterScopedGAMOmission(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	gameState, _, _ := invasionOccurrenceFixture(t, now)
	occurrenceEndsAt := State.ScalableEventEndsAt(gameState.EventScores.ByEvent[71])
	reservedAt := now.Add(-State.InvasionTargetReservationReconcileGrace - time.Second)
	gameState.Invasion.ReserveTarget(State.InvasionTargetReservation{
		KingdomID: 0, EventID: 71, OccurrenceEndsAt: occurrenceEndsAt,
		TargetTypeID: State.MapTypeForeignLord, X: 101, Y: 100,
		SourceCastleID: 1, CommanderID: 0, CommanderKnown: true,
		OperationID: "unknown-target-cra", ReservedAt: reservedAt,
	})
	arguments, _ := json.Marshal(invasionTargetReconcileRequest{
		FocusCastleID: 1, KingdomID: 0, EventID: 71, OccurrenceEndsAt: occurrenceEndsAt,
		TargetTypeID: State.MapTypeForeignLord, TargetX: 101, TargetY: 100,
		OperationID: "unknown-target-cra", ReservedAt: reservedAt,
	})
	plan, err := planInvasionTargetReconcile(t.Context(), Intent.PlanningContext{State: gameState}, arguments)
	if err != nil {
		t.Fatal(err)
	}
	verificationArguments := plan.Steps[len(plan.Steps)-1].ActionArguments
	var verification invasionTargetReconcileRequest
	if err := json.Unmarshal(verificationArguments, &verification); err != nil {
		t.Fatal(err)
	}
	gameState.MovementSnapshot.ObservedAt = verification.ReconcileStartedAt.Add(time.Second)
	target := gameState.Map[0]["101:100"]
	target.ObjectID = 0
	target.Level = 0
	target.InvasionAvailabilityKnown = false
	target.ObservedAt = verification.ReconcileStartedAt.Add(2 * time.Second)
	gameState.Map[0]["101:100"] = target

	dataDir := t.TempDir()
	application := &Application{DataDir: dataDir, State: State.NewStore(gameState)}
	if err := application.reconcileInvasionTargetReservation(t.Context(), verificationArguments); err != nil {
		t.Fatal(err)
	}
	reservation, found := application.State.ReadOnlyView().Invasion.TargetReservation(0, 101, 100)
	if !found || !reservation.ReconcileAfter.After(verification.ReconcileStartedAt) ||
		!reservation.CommanderKnown || reservation.CommanderID != 0 || reservation.ReconcileAttempts != 1 ||
		!State.InvasionCommanderReserved(application.State.ReadOnlyView(), 0) {
		t.Fatalf("scoped GAM reconciliation backoff = %#v, found=%t", reservation, found)
	}
	loaded, err := State.LoadSnapshot(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	persisted, found := loaded.Invasion.TargetReservation(0, 101, 100)
	if !found || !persisted.ReconcileAfter.Equal(reservation.ReconcileAfter) ||
		!persisted.CommanderKnown || persisted.CommanderID != 0 || persisted.ReconcileAttempts != 1 {
		t.Fatalf("persisted scoped GAM backoff = %#v, found=%t", persisted, found)
	}
}

func TestTargetOnlyInvasionReservationReconcilesWithoutMovementProbe(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	gameState, _, _ := invasionOccurrenceFixture(t, now)
	occurrenceEndsAt := State.ScalableEventEndsAt(gameState.EventScores.ByEvent[71])
	reservedAt := now.Add(-State.InvasionTargetReservationReconcileGrace - time.Second)
	gameState.Invasion.ReserveTarget(State.InvasionTargetReservation{
		KingdomID: 0, EventID: 71, OccurrenceEndsAt: occurrenceEndsAt,
		TargetTypeID: State.MapTypeForeignLord, X: 101, Y: 100,
		SourceCastleID: 1, OperationID: "target-only-adi", ReservedAt: reservedAt,
	})
	arguments, _ := json.Marshal(invasionTargetReconcileRequest{
		FocusCastleID: 1, KingdomID: 0, EventID: 71, OccurrenceEndsAt: occurrenceEndsAt,
		TargetTypeID: State.MapTypeForeignLord, TargetX: 101, TargetY: 100,
		OperationID: "target-only-adi", ReservedAt: reservedAt,
	})
	plan, err := planInvasionTargetReconcile(t.Context(), Intent.PlanningContext{State: gameState}, arguments)
	if err != nil {
		t.Fatal(err)
	}
	for _, step := range plan.Steps {
		if step.Opcode == "gam" {
			t.Fatalf("target-only ADI reservation scheduled a movement probe: %#v", plan.Steps)
		}
	}
	verificationArguments := plan.Steps[len(plan.Steps)-1].ActionArguments
	var verification invasionTargetReconcileRequest
	if err := json.Unmarshal(verificationArguments, &verification); err != nil {
		t.Fatal(err)
	}
	target := gameState.Map[0]["101:100"]
	target.ObservedAt = verification.ReconcileStartedAt.Add(time.Second)
	gameState.Map[0]["101:100"] = target
	application := &Application{State: State.NewStore(gameState)}
	if err := application.reconcileInvasionTargetReservation(t.Context(), verificationArguments); err != nil {
		t.Fatal(err)
	}
	if _, reserved := application.State.ReadOnlyView().Invasion.TargetReservation(0, 101, 100); reserved {
		t.Fatal("fresh target availability did not release target-only ADI reservation")
	}
}

func TestInvasionReconciliationAccountsJustEndedOccurrence(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	gameState, gameData, configuration := invasionOccurrenceFixture(t, now)
	occurrenceEndsAt := now
	reservedAt := now.Add(-State.InvasionTargetReservationReconcileGrace - time.Second)
	commanderID := State.CommanderID(0)
	arrivesAt := now.Add(time.Minute)
	gameState.EventScores.ActiveEventID = 0
	gameState.EventScores.ByEvent[71] = State.ScalableEventScore{EventID: 71, ObservedAt: now}
	gameState.Invasion.ReserveTarget(State.InvasionTargetReservation{
		KingdomID: 0, EventID: 71, OccurrenceEndsAt: occurrenceEndsAt,
		TargetTypeID: State.MapTypeForeignLord, X: 101, Y: 100,
		SourceCastleID: 1, CommanderID: commanderID, CommanderKnown: true,
		OperationID: "event-ended-cra", ReservedAt: reservedAt,
	})
	gameState.Movements[9] = State.MovementState{
		ID: 9, Direction: 0, SourceCastleID: 1, CommanderID: &commanderID,
		KingdomID: 0, TargetTypeID: State.MapTypeForeignLord, TargetX: 101, TargetY: 100,
		StartedAt: reservedAt.Add(time.Second), ObservedAt: now, ArrivesAt: &arrivesAt,
	}
	decision, err := Automation.NewInvasionRecoveryPolicy().Evaluate(t.Context(), Automation.Snapshot{
		State: gameState, GameData: gameData, Configuration: configuration, Now: now,
	})
	if err != nil || decision.Request == nil || decision.Request.Name != "invasion.target.reconcile" {
		t.Fatalf("ended-occurrence recovery decision = %#v, err=%v", decision, err)
	}
	plan, err := planInvasionTargetReconcile(
		t.Context(), Intent.PlanningContext{State: gameState}, decision.Request.Arguments,
	)
	if err != nil {
		t.Fatal(err)
	}
	verificationArguments := plan.Steps[len(plan.Steps)-1].ActionArguments
	var verification invasionTargetReconcileRequest
	if err := json.Unmarshal(verificationArguments, &verification); err != nil {
		t.Fatal(err)
	}
	gameState.MovementSnapshot.ObservedAt = verification.ReconcileStartedAt.Add(time.Second)
	application := &Application{State: State.NewStore(gameState)}
	if err := application.reconcileInvasionTargetReservation(t.Context(), verificationArguments); err != nil {
		t.Fatal(err)
	}
	result := application.State.ReadOnlyView()
	activity, found := result.LookupEventActivity(71)
	if _, reserved := result.Invasion.TargetReservation(0, 101, 100); reserved || !found ||
		!activity.OccurrenceEndsAt.Equal(occurrenceEndsAt) || activity.Invasion.Launches != 1 ||
		len(activity.PendingAttacks) != 1 || len(result.AttackAnalytics.PendingAttacks) != 1 {
		t.Fatalf("ended-occurrence accounting result: invasion=%#v activity=%#v analytics=%#v", result.Invasion, activity, result.AttackAnalytics)
	}
}

func TestInvasionReconciliationDoesNotAttributeReportWithoutMovement(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	gameState, gameData, configuration := invasionOccurrenceFixture(t, now)
	gameState.Player.ID = 10
	occurrenceEndsAt := State.ScalableEventEndsAt(gameState.EventScores.ByEvent[71])
	reservedAt := now.Add(-20 * time.Second)
	commanderID := State.CommanderID(0)
	gameState.Invasion.ReserveTarget(State.InvasionTargetReservation{
		KingdomID: 0, EventID: 71, OccurrenceEndsAt: occurrenceEndsAt,
		TargetTypeID: State.MapTypeForeignLord, X: 101, Y: 100,
		SourceCastleID: 1, SourceX: 100, SourceY: 100, SourceKnown: true,
		CommanderID: commanderID, CommanderKnown: true,
		OperationID: "report-only-cra", ReservedAt: reservedAt,
	})
	gameState.Reports.BattleCaptures[501] = State.BattleReportCapture{
		MessageID: 501, ReportID: 601, ToolsUsed: 7,
		OccurredAt: now.Add(-10 * time.Second), CapturedAt: now.Add(-9 * time.Second),
		Summary: json.RawMessage(`{"MID":501,"LID":601,"PBI":[[10,0,18560,-765],[-1002,1,19281,-19281]],"AI":{"AT":21,"K":0,"X":101,"Y":100}}`),
		Details: json.RawMessage(`{"LID":601,"W":[[[10,[[[1,100,-12]],[[702,7,-7]]] ],[-603,[[],[]]]]]}`),
	}
	directory := t.TempDir()
	if err := State.SaveSnapshot(directory, gameState); err != nil {
		t.Fatal(err)
	}
	restarted, err := State.LoadSnapshot(directory)
	if err != nil {
		t.Fatal(err)
	}
	if restarted.MovementCount() != 0 {
		t.Fatalf("restart unexpectedly retained movement evidence: %#v", restarted.Movements)
	}

	decision, err := Automation.NewInvasionRecoveryPolicy().Evaluate(t.Context(), Automation.Snapshot{
		State: restarted, GameData: gameData, Configuration: configuration, Now: now,
	})
	if err != nil || decision.Request == nil || decision.Request.Name != "game.refresh_movements" {
		t.Fatalf("report-only recovery decision = %#v err=%v", decision, err)
	}
	if _, reserved := restarted.Invasion.TargetReservation(0, 101, 100); !reserved {
		t.Fatal("report-only recovery released the durable reservation")
	}
	report, reportFound := restarted.LookupBattleReportCapture(501)
	if !reportFound || report.MovementID != 0 || report.AutomationFeature != "" || report.EventID != 0 ||
		len(restarted.AttackAnalytics.LaunchIDs) != 0 {
		t.Fatalf("report-only recovery attributed an unproven launch: analytics=%#v report=%#v",
			restarted.AttackAnalytics, report)
	}
}

func invasionOccurrenceFixture(
	t *testing.T,
	now time.Time,
) (State.GameState, *GameData.Store, Configuration.Snapshot) {
	t.Helper()
	gameData, err := GameData.DecodeStore([]byte(`{
		"versionInfo":[],"units":[{"wodID":216}],"buildings":[],"effects":[],"legendskills":[],
		"eventAutoScalingDifficulties":[{"difficultyID":8,"eventID":71,"difficultyTypeID":1,"isLocked":0}]
	}`), GameData.SourceMetadata{ItemVersion: "invasion-occurrence-test"})
	if err != nil {
		t.Fatal(err)
	}
	gameState := State.NewGameState()
	gameState.Castles[1] = State.CastleState{
		ID: 1, Name: "Main", KingdomID: 0, X: 100, Y: 100, Focused: true,
		Units: State.CastleUnits{Stationed: map[State.UnitID]int64{216: 5_000}},
	}
	gameState.Commanders[0] = State.CommanderState{ID: 0, Available: true}
	gameState.Player.LegendSkills.ObservedAt = now
	gameState.EventScores.ActiveEventID = 71
	gameState.EventScores.ByEvent[71] = State.ScalableEventScore{
		EventID: 71, DifficultyID: 8, PlayerScore: 0, RemainingSec: 7_200, ObservedAt: now,
	}
	gameState.Invasion.LastScannedAt[1] = now
	gameState.Map[0] = map[string]State.MapObservation{
		"101:100": {
			KingdomID: 0, TypeID: State.MapTypeForeignLord, X: 101, Y: 100,
			ObjectID: 70, Level: 70, InvasionAvailabilityKnown: true, ObservedAt: now,
		},
	}
	gameState.AttackDialog = State.AttackDialogState{
		SourceCastleID: 1, KingdomID: 0, ObservedAt: now,
		Target: State.AttackDialogTarget{
			TypeID: State.MapTypeForeignLord, X: 101, Y: 100,
			ObjectID: 70, InvasionAvailabilityKnown: true,
		},
	}
	configuration := Configuration.Snapshot{Sections: map[string]json.RawMessage{
		"automation.autoInvasion": json.RawMessage(`{
			"version":1,"sourceCastleId":1,"presetId":"trial",
			"foreignLordsDifficultyId":8,"bloodcrowDifficultyId":108,
			"scoreTarget":5000000,"minimumRemainingSec":1800,
			"checkIntervalSec":30,"mapRefreshIntervalSec":300
		}`),
		"attacks.presets": json.RawMessage(`{
			"version":1,"presets":[{"id":"trial","name":"Trial","waves":[{
				"L":{"troops":[],"tools":[]},
				"M":{"troops":[{"itemId":216,"quantity":1000}],"tools":[]},
				"R":{"troops":[],"tools":[]}
			}]}]
		}`),
	}}
	return gameState, gameData, configuration
}
