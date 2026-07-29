package App

import (
	"encoding/json"
	"testing"
	"time"

	"CitadelDesktop/Server/GameData"
	"CitadelDesktop/Server/Intent"
	"CitadelDesktop/Server/State"
)

func TestPlanAutoBirdRunsAINBeforeJAAForOneCastle(t *testing.T) {
	now := time.Now().UTC()
	gameState := State.NewGameState()
	gameState.Player.ProtectionMode.ObservedAt = now
	gameState.Alliance.ID = 9
	gameState.Castles[10] = State.CastleState{
		ID: 10, KingdomID: 0, X: 20, Y: 30, Focused: true,
	}
	discover, err := planAutoBirdDiscover(t.Context(), Intent.PlanningContext{State: gameState}, json.RawMessage(`{
		"sourceCastleId":10,"minimumRPTDays":3,
		"minimumDelayHours":6,"maximumDelayHours":12
	}`))
	if err != nil {
		t.Fatal(err)
	}
	if len(discover.Steps) != 2 ||
		discover.Steps[0].Opcode != "ain" ||
		discover.Steps[0].ResponseBarrier != Intent.ResponseBarrierCommitted ||
		discover.Steps[1].Action != "auto_bird.target.capture" {
		t.Fatalf("Auto Bird target-discovery steps = %#v", discover.Steps)
	}
	var request autoBirdCycleRequest
	if err := json.Unmarshal(discover.Steps[1].ActionArguments, &request); err != nil {
		t.Fatal(err)
	}
	if request.AllianceRefreshAt.IsZero() {
		t.Fatal("Auto Bird discovery did not record its AIN cutoff")
	}

	gameState.Stationing["autoBird:10"] = State.StationingOperation{
		ID: "autoBird:10", Purpose: "autoBird", Phase: State.StationingPhaseTargetReady,
		SourceCastleID: 10, TargetCastleID: 20, DelayHours: 8, WaitSeconds: 8 * 3600,
		AllianceObservedAt: now, UpdatedAt: now,
	}
	gameState.Alliance.Holdings = []State.AllianceHolding{{
		CastleID: 20, KingdomID: 0, X: 40, Y: 50, SlotType: 1,
	}}
	prepare, err := planAutoBirdPrepare(t.Context(), Intent.PlanningContext{State: gameState}, json.RawMessage(`{
		"sourceCastleId":10,"minimumRPTDays":3,
		"minimumDelayHours":6,"maximumDelayHours":12
	}`))
	if err != nil {
		t.Fatal(err)
	}
	if len(prepare.Steps) != 2 ||
		prepare.Steps[0].Opcode != "jca" ||
		prepare.Steps[0].AwaitOpcode != "jaa" ||
		prepare.Steps[0].ResponseBarrier != Intent.ResponseBarrierCommitted ||
		prepare.Steps[1].Action != "auto_bird.manifest.capture" {
		t.Fatalf("Auto Bird troop-preparation steps = %#v", prepare.Steps)
	}
	if err := json.Unmarshal(prepare.Steps[1].ActionArguments, &request); err != nil {
		t.Fatal(err)
	}
	if request.UnitsRefreshAt.IsZero() {
		t.Fatal("Auto Bird preparation did not record its JAA cutoff")
	}
}

func TestAutoBirdTargetThenManifestRecordRandomWaitAndFreshTroops(t *testing.T) {
	now := time.Date(2026, 7, 29, 14, 0, 0, 0, time.UTC)
	gameState, gameData := autoBirdIntentTestState(t, now)
	gameState.Castles[10] = State.CastleState{
		ID: 10, KingdomID: 0, X: 10, Y: 10, UnitsObservedAt: now,
		Units: State.CastleUnits{Stationed: map[State.UnitID]int64{
			489: 100,
			735: 999,
		}},
	}
	request := autoBirdCycleRequest{
		SourceCastleID: 10, TrackingID: "autoBird:10", MinimumRPTDays: 3,
		MinimumDelayHours: 6, MaximumDelayHours: 12, MinimumSend: 50,
		Reserves:          []stationUnitRequest{{UnitID: 489, Amount: 25}},
		AllianceRefreshAt: now.Add(-time.Second),
	}
	target := discoveredAutoBirdOperation(
		gameState, State.StationingOperation{}, request, 8, now,
	)
	if target.Phase != State.StationingPhaseTargetReady ||
		target.TargetCastleID != 20 || target.DelayHours != 8 || target.WaitSeconds != 8*3600 {
		t.Fatalf("discovered Auto Bird target = %#v", target)
	}
	request.UnitsRefreshAt = now.Add(-time.Second)
	request.ExpectedTargetCastle = 20
	operation := preparedAutoBirdManifest(gameState, gameData, target, request, now)
	if operation.Phase != State.StationingPhaseDispatchReady ||
		operation.TargetCastleID != 20 ||
		operation.DelayHours != 8 ||
		operation.WaitSeconds != 8*3600 {
		t.Fatalf("prepared Auto Bird operation = %#v", operation)
	}
	if len(operation.Units) != 1 || operation.Units[489] != 75 {
		t.Fatalf("fresh Auto Bird manifest = %#v, want troop 489 amount 75", operation.Units)
	}
	if !operation.AllianceObservedAt.Equal(now) || !operation.UnitsObservedAt.Equal(now) {
		t.Fatalf(
			"fresh observation times = alliance %s units %s",
			operation.AllianceObservedAt, operation.UnitsObservedAt,
		)
	}
}

func TestAutoBirdDiscoveryWaitsWithoutRunningJAAWhenAINHasNoTarget(t *testing.T) {
	now := time.Date(2026, 7, 29, 14, 0, 0, 0, time.UTC)
	gameState, _ := autoBirdIntentTestState(t, now)
	gameState.Alliance.Holdings = nil
	gameState.Castles[10] = State.CastleState{ID: 10, KingdomID: 0, X: 10, Y: 10}
	request := autoBirdCycleRequest{
		SourceCastleID: 10, TrackingID: "autoBird:10", MinimumRPTDays: 3,
		MinimumDelayHours: 6, MaximumDelayHours: 12,
		AllianceRefreshAt: now.Add(-time.Second),
	}
	operation := discoveredAutoBirdOperation(
		gameState, State.StationingOperation{}, request, 7, now,
	)
	if operation.Phase != State.StationingPhaseWaiting ||
		operation.NextAttemptAt == nil || !operation.NextAttemptAt.After(now) ||
		operation.TargetCastleID != 0 {
		t.Fatalf("target-less Auto Bird discovery = %#v", operation)
	}
}

func TestPreparedAutoBirdOperationWaitsOnlyThatCastleWhenNoTroopsAreEligible(t *testing.T) {
	now := time.Date(2026, 7, 29, 14, 0, 0, 0, time.UTC)
	gameState, gameData := autoBirdIntentTestState(t, now)
	gameState.Castles[10] = State.CastleState{
		ID: 10, KingdomID: 0, X: 10, Y: 10, UnitsObservedAt: now,
		Units: State.CastleUnits{Stationed: map[State.UnitID]int64{735: 999}},
	}
	request := autoBirdCycleRequest{
		SourceCastleID: 10, TrackingID: "autoBird:10", MinimumRPTDays: 3,
		MinimumDelayHours: 6, MaximumDelayHours: 12,
		AllianceRefreshAt: now.Add(-time.Second),
	}
	target := discoveredAutoBirdOperation(
		gameState, State.StationingOperation{}, request, 7, now,
	)
	request.UnitsRefreshAt = now.Add(-time.Second)
	request.ExpectedTargetCastle = 20
	operation := preparedAutoBirdManifest(gameState, gameData, target, request, now)
	if operation.Phase != State.StationingPhaseWaiting ||
		operation.NextAttemptAt == nil || !operation.NextAttemptAt.After(now) {
		t.Fatalf("troop-less Auto Bird operation = %#v", operation)
	}
	if operation.MovementID != 0 || operation.ExpectedReturnAt != nil {
		t.Fatalf("troop-less castle was marked as launched: %#v", operation)
	}
}

func TestPlanAutoBirdDispatchGuardsPreparedContextAndRecordsMovement(t *testing.T) {
	now := time.Now().UTC()
	gameState, gameData := autoBirdIntentTestState(t, now)
	gameState.Castles[10] = State.CastleState{
		ID: 10, KingdomID: 0, X: 10, Y: 10, Focused: true, UnitsObservedAt: now,
		Units: State.CastleUnits{Stationed: map[State.UnitID]int64{489: 100}},
	}
	gameState.Stationing["autoBird:10"] = State.StationingOperation{
		ID: "autoBird:10", Purpose: "autoBird", Phase: State.StationingPhaseDispatchReady,
		SourceCastleID: 10, TargetCastleID: 20, Units: map[State.UnitID]int64{489: 100},
		DelayHours: 8, UnitsObservedAt: now, UpdatedAt: now,
	}
	plan, err := planAutoBirdDispatch(t.Context(), Intent.PlanningContext{
		State: gameState, GameData: gameData,
	}, json.RawMessage(`{
		"sourceCastleId":10,"trackingId":"autoBird:10",
		"minimumDelayHours":6,"maximumDelayHours":12
	}`))
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Steps) != 6 ||
		plan.Steps[0].Action != "auto_bird.dispatch.guard" ||
		plan.Steps[1].Opcode != "sdi" ||
		plan.Steps[2].Resolver != "auto_bird.dispatch.build" ||
		plan.Steps[3].Action != "auto_bird.movement.capture" ||
		plan.Steps[4].Opcode != "gam" ||
		plan.Steps[4].ResponseBarrier != Intent.ResponseBarrierCommitted ||
		plan.Steps[5].Action != "auto_bird.movement.capture" {
		t.Fatalf("Auto Bird dispatch steps = %#v", plan.Steps)
	}
}

func TestAutoBirdDispatchGuardReturnsOnlyThatCastleToJAAPhaseWhenFocusChanges(t *testing.T) {
	now := time.Now().UTC()
	gameState, _ := autoBirdIntentTestState(t, now)
	gameState.Castles[10] = State.CastleState{
		ID: 10, KingdomID: 0, X: 10, Y: 10, Focused: false, UnitsObservedAt: now,
	}
	gameState.Stationing["autoBird:10"] = State.StationingOperation{
		ID: "autoBird:10", Purpose: "autoBird", Phase: State.StationingPhaseDispatchReady,
		SourceCastleID: 10, TargetCastleID: 20, Units: map[State.UnitID]int64{489: 100},
		DelayHours: 8, UnitsObservedAt: now, UpdatedAt: now,
	}
	application := &Application{State: State.NewStore(gameState)}
	arguments, _ := json.Marshal(autoBirdCycleRequest{
		SourceCastleID: 10, TrackingID: "autoBird:10",
		MinimumDelayHours: 6, MaximumDelayHours: 12,
	})
	if err := application.guardAutoBirdDispatch(t.Context(), arguments); err == nil {
		t.Fatal("changed castle focus unexpectedly passed the Auto Bird dispatch guard")
	}
	operation := application.State.Snapshot().Stationing["autoBird:10"]
	if operation.Phase != State.StationingPhaseTargetReady ||
		len(operation.Units) != 0 || !operation.UnitsObservedAt.IsZero() {
		t.Fatalf("focus-changed Auto Bird operation = %#v", operation)
	}
}

func TestResolveAutoBirdDispatchRebuildsEveryEligibleTroopFromLatestJAA(t *testing.T) {
	now := time.Now().UTC()
	gameState, gameData := autoBirdIntentTestState(t, now)
	gameState.Castles[10] = State.CastleState{
		ID: 10, KingdomID: 0, X: 10, Y: 10, Focused: true, UnitsObservedAt: now,
		Units: State.CastleUnits{Stationed: map[State.UnitID]int64{
			215: 100,
			489: 50,
			735: 999,
		}},
	}
	gameState.Stationing["autoBird:10"] = State.StationingOperation{
		ID: "autoBird:10", Purpose: "autoBird", Phase: State.StationingPhaseDispatchReady,
		SourceCastleID: 10, TargetCastleID: 20, Units: map[State.UnitID]int64{489: 10},
		DelayHours: 8, UnitsObservedAt: now, UpdatedAt: now,
	}
	request, _ := json.Marshal(autoBirdCycleRequest{
		SourceCastleID: 10, TrackingID: "autoBird:10",
		MinimumDelayHours: 6, MaximumDelayHours: 12, MinimumSend: 100,
		Reserves:          []stationUnitRequest{{UnitID: 215, Amount: 10}},
		DispatchStartedAt: now.Add(-time.Second), ExpectedTargetCastle: 20,
	})
	application := &Application{State: State.NewStore(gameState)}
	step, err := application.resolveAutoBirdDispatchStep(t.Context(), Intent.PlanningContext{
		State: gameState, GameData: gameData,
	}, request)
	if err != nil {
		t.Fatal(err)
	}
	if step.Opcode != "cds" || step.ResponseBarrier != Intent.ResponseBarrierCommitted {
		t.Fatalf("resolved Auto Bird command = %#v", step)
	}
	var payload struct {
		Wait  int        `json:"WT"`
		Units [][2]int64 `json:"A"`
	}
	if err := json.Unmarshal(step.Command.Payload, &payload); err != nil {
		t.Fatal(err)
	}
	want := [][2]int64{{215, 90}, {489, 50}}
	if payload.Wait != 8 || len(payload.Units) != len(want) {
		t.Fatalf("resolved Auto Bird payload = %#v", payload)
	}
	for index := range want {
		if payload.Units[index] != want[index] {
			t.Fatalf("resolved Auto Bird units = %#v, want %#v", payload.Units, want)
		}
	}
}

func TestResolveAutoBirdDispatchDefersOnlyThatCastleWhenFreshJAAIsEmpty(t *testing.T) {
	now := time.Now().UTC()
	gameState, gameData := autoBirdIntentTestState(t, now)
	gameState.Castles[10] = State.CastleState{
		ID: 10, KingdomID: 0, X: 10, Y: 10, Focused: true, UnitsObservedAt: now,
		Units: State.CastleUnits{Stationed: map[State.UnitID]int64{735: 999}},
	}
	gameState.Stationing["autoBird:10"] = State.StationingOperation{
		ID: "autoBird:10", Purpose: "autoBird", Phase: State.StationingPhaseDispatchReady,
		SourceCastleID: 10, TargetCastleID: 20, Units: map[State.UnitID]int64{489: 100},
		DelayHours: 8, UnitsObservedAt: now, UpdatedAt: now,
	}
	application := &Application{State: State.NewStore(gameState)}
	request, _ := json.Marshal(autoBirdCycleRequest{
		SourceCastleID: 10, TrackingID: "autoBird:10",
		MinimumDelayHours: 6, MaximumDelayHours: 12,
		DispatchStartedAt: now.Add(-time.Second), ExpectedTargetCastle: 20,
	})
	if _, err := application.resolveAutoBirdDispatchStep(t.Context(), Intent.PlanningContext{
		State: gameState, GameData: gameData,
	}, request); err == nil {
		t.Fatal("empty dispatch JAA unexpectedly produced a CDS command")
	}
	operation := application.State.Snapshot().Stationing["autoBird:10"]
	if operation.Phase != State.StationingPhaseWaiting ||
		operation.NextAttemptAt == nil || !operation.NextAttemptAt.After(now) ||
		len(operation.Units) != 0 {
		t.Fatalf("deferred Auto Bird castle = %#v", operation)
	}
}

func TestCaptureAutoBirdMovementRecordsTravelWaitAndExpectedReturn(t *testing.T) {
	now := time.Now().UTC()
	arrivesAt := now.Add(10 * time.Minute)
	expectedReturn := arrivesAt.Add(8*time.Hour + 10*time.Minute)
	gameState := State.NewGameState()
	gameState.Alliance.Holdings = []State.AllianceHolding{{
		CastleID: 20, KingdomID: 0, X: 40, Y: 50, SlotType: 1,
	}}
	gameState.Stationing["autoBird:10"] = State.StationingOperation{
		ID: "autoBird:10", Purpose: "autoBird", Phase: State.StationingPhaseDispatchReady,
		SourceCastleID: 10, TargetCastleID: 20, Units: map[State.UnitID]int64{489: 100},
		DelayHours: 8, WaitSeconds: 8 * 3600, UpdatedAt: now,
	}
	gameState.Movements[30] = State.MovementState{
		ID: 30, Direction: 0, SourceCastleID: 10, TargetX: 40, TargetY: 50,
		TravelSeconds: 600, WaitSeconds: 8 * 3600, ArrivesAt: &arrivesAt,
		ObservedAt: now, Units: map[State.UnitID]int64{489: 125},
	}
	application := &Application{State: State.NewStore(gameState)}
	arguments, _ := json.Marshal(autoBirdCycleRequest{
		SourceCastleID: 10, TrackingID: "autoBird:10",
		MinimumDelayHours: 6, MaximumDelayHours: 12,
		DispatchStartedAt: now.Add(-time.Second),
	})
	if err := application.captureAutoBirdMovement(t.Context(), arguments); err != nil {
		t.Fatal(err)
	}
	operation := application.State.Snapshot().Stationing["autoBird:10"]
	if operation.Phase != State.StationingPhaseAway ||
		operation.MovementID != 30 ||
		operation.TravelSeconds != 600 ||
		operation.WaitSeconds != 8*3600 ||
		operation.ExpectedReturnAt == nil ||
		!operation.ExpectedReturnAt.Equal(expectedReturn) {
		t.Fatalf("recorded Auto Bird movement = %#v", operation)
	}
	if operation.Units[489] != 125 ||
		operation.NextAttemptAt == nil || !operation.NextAttemptAt.Equal(expectedReturn) {
		t.Fatalf("recorded Auto Bird return state = %#v", operation)
	}
}

func autoBirdIntentTestState(t *testing.T, now time.Time) (State.GameState, *GameData.Store) {
	t.Helper()
	gameData, err := GameData.DecodeStore([]byte(`{
		"versionInfo":[],"buildings":[],"units":[
			{"wodID":215},{"wodID":489},
			{"wodID":735,"toolCategory":"Premium","slotTypes":"1,2,9"}
		]
	}`), GameData.SourceMetadata{ItemVersion: "test"})
	if err != nil {
		t.Fatal(err)
	}
	gameState := State.NewGameState()
	gameState.Player.ProtectionMode.ObservedAt = now
	gameState.Alliance = State.AllianceState{
		ID: 9, ObservedAt: now,
		Members: []State.AllianceMember{{PlayerID: 1, ReturnProtectionSec: 4 * 86_400}},
		Holdings: []State.AllianceHolding{{
			CastleID: 20, PlayerID: 1, KingdomID: 0, X: 20, Y: 20, SlotType: 1,
		}},
	}
	return gameState, gameData
}
