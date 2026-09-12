package App

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"CitadelDesktop/Server/AttackPresets"
	"CitadelDesktop/Server/GameData"
	"CitadelDesktop/Server/Ingest"
	"CitadelDesktop/Server/Intent"
	"CitadelDesktop/Server/Outbound"
	"CitadelDesktop/Server/Protocol"
	"CitadelDesktop/Server/State"
)

func TestInvasionAttackResolvesFreshLaneCapacityAndAcceptsCommanderZero(t *testing.T) {
	gameData, err := GameData.DecodeStore([]byte(`{
		"versionInfo":[],
		"units":[{"wodID":216},{"wodID":217}],
		"buildings":[],
		"effects":[
			{"effectID":2110,"name":"relicAttackUnitAmountFront","effectTypeID":34,"capID":1010},
			{"effectID":2807,"name":"relicAttackUnitAmountFrontPVP","effectTypeID":34,"capID":1707,"areaTypeID":"1,3,4,10,12,15,21"},
			{"effectID":700,"name":"attackUnitAmountReinforcementBonus","effectTypeID":179,"capID":99}
		],
		"legendskills":[{"skillID":106,"effectType":"additionalUnitAmountOnFront","totalEffectValue":25}],
		"effectCaps":[{"capID":99}]
	}`), GameData.SourceMetadata{ItemVersion: "test"})
	if err != nil {
		t.Fatal(err)
	}
	unitID, supportUnitID := int64(216), int64(217)
	now := time.Now().UTC()
	gameState := State.NewGameState()
	gameState.Castles[1] = State.CastleState{
		ID: 1, KingdomID: 0, X: 1164, Y: 1167,
		Units: State.CastleUnits{Stationed: map[State.UnitID]int64{216: 1_000, 217: 240}},
	}
	gameState.Commanders[0] = State.CommanderState{
		ID: 0, Available: true, Equipment: map[string]State.EquipmentInstanceID{"1": 1001, "2": 1003, "6": 1002},
	}
	gameState.Inventory.Equipment[1001] = State.EquipmentInstance{
		ID: 1001, Effects: State.EquipmentEffects{{DefinitionID: 2110, Values: []float64{45}}},
	}
	gameState.Inventory.Equipment[1002] = State.EquipmentInstance{
		ID: 1002, Effects: State.EquipmentEffects{{DefinitionID: 2807, Values: []float64{25.1}}},
	}
	gameState.Inventory.Equipment[1003] = State.EquipmentInstance{
		ID: 1003, Effects: State.EquipmentEffects{{DefinitionID: 700, Values: []float64{240}}},
	}
	gameState.Player.LegendSkills = State.LegendSkillState{ActiveIDs: []int64{106}, ObservedAt: now}
	gameState.EventScores.ActiveEventID = 71
	gameState.EventScores.ByEvent[71] = State.ScalableEventScore{
		EventID: 71, DifficultyID: 1, RemainingSec: 7_200, ObservedAt: now,
	}
	gameState.Map[0] = map[string]State.MapObservation{
		"1165:1166": {
			KingdomID: 0, TypeID: 21, X: 1165, Y: 1166, ObjectID: 70, Level: 70,
			InvasionAvailabilityKnown: true, ObservedAt: now,
		},
	}
	gameState.AttackDialog = State.AttackDialogState{
		SourceCastleID: 1, KingdomID: 0,
		Target: State.AttackDialogTarget{
			TypeID: 21, X: 1165, Y: 1166, ObjectID: 70, InvasionAvailabilityKnown: true,
		},
		ActiveEffects: []State.AttackDialogEffect{},
		ObservedAt:    now,
	}
	request := invasionAttackRequest{
		SourceCastleID: 1, EventID: 71,
		EventEndsAt: State.ScalableEventEndsAt(gameState.EventScores.ByEvent[71]),
		ScoreTarget: 5_000_000, MinimumRemainingSec: 1_800,
		TargetTypeID: 21, KingdomID: 0, TargetX: 1165, TargetY: 1166, TargetObjectID: 70,
		Preset: AttackPresets.Preset{
			Name: "Trial",
			Waves: []AttackPresets.Wave{{
				Middle: AttackPresets.Lane{Troops: []AttackPresets.Slot{{ItemID: &unitID, Quantity: 1_000}}},
			}},
			CourtyardSupport: AttackPresets.CourtyardSupport{
				Troops: []AttackPresets.Slot{{ItemID: &supportUnitID, Quantity: 1_000}},
			},
		},
	}
	arguments, _ := json.Marshal(request)
	plan, err := planInvasionAttack(context.Background(), Intent.PlanningContext{State: gameState, GameData: gameData}, arguments)
	if err != nil {
		t.Fatal(err)
	}
	var deferred *Intent.Step
	consumeIndex, captureIndex := -1, -1
	foundRefresh, foundTargetGuard, foundCapture := false, false, false
	for index := range plan.Steps {
		step := &plan.Steps[index]
		if step.Opcode == "gaa" {
			foundRefresh = true
		}
		if step.Action == "invasion.target.guard" {
			foundTargetGuard = true
		}
		if step.Action == "invasion.attack.capture" {
			foundCapture = true
			captureIndex = index
		}
		if step.Action == "invasion.target.consume" {
			consumeIndex = index
		}
		if step.Resolver == "invasion.attack.build" {
			deferred = step
		}
	}
	if deferred == nil || deferred.CommandDependencies == nil || deferred.CommandDependencies.Opcode != "cra" {
		t.Fatalf("invasion attack did not defer formation building behind CRA send dependencies: %#v", plan.Steps)
	}
	if !foundRefresh || !foundTargetGuard || !foundCapture {
		t.Fatalf("invasion attack does not refresh, verify, and capture its target: %#v", plan.Steps)
	}
	if consumeIndex < 0 || captureIndex < 0 || consumeIndex > captureIndex {
		t.Fatalf("accepted CRA must consume its target before fallible movement capture: %#v", plan.Steps)
	}
	resolved, err := (&Application{}).resolveInvasionAttackStep(
		context.Background(), Intent.PlanningContext{State: gameState, GameData: gameData}, deferred.ResolverArguments,
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(resolved.StaleCodes) != 1 || resolved.StaleCodes[0] != 95 ||
		resolved.PreDispatchAction != "invasion.target.reserve" ||
		resolved.DefinitiveSendFailureAction != "invasion.target.release" ||
		resolved.DefinitiveResponseFailureAction != "invasion.target.release" ||
		resolved.StaleResponseAction != "invasion.target.cooldown" ||
		!resolved.ResponseProjectionFailureIndeterminate {
		t.Fatalf("resolved invasion CRA does not preserve and rotate response outcomes: %#v", resolved)
	}
	dependencies, err := (&Application{}).resolveCRACommandDependencies(
		context.Background(), Intent.PlanningContext{State: gameState}, Intent.Step{
			Payload: deferred.CommandDependencies.Payload,
			Command: Protocol.Command{Opcode: "cra", Payload: deferred.CommandDependencies.Payload},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	foundCooldownProbe := false
	for index, dependency := range dependencies.Steps {
		if dependency.Opcode != "adi" {
			continue
		}
		foundCooldownProbe = len(dependency.StaleCodes) == 1 && dependency.StaleCodes[0] == 95 &&
			dependency.PreDispatchAction == "invasion.target.reserve" &&
			dependency.DefinitiveSendFailureAction == "invasion.target.release" &&
			dependency.DefinitiveResponseFailureAction == "invasion.target.release" &&
			dependency.StaleResponseAction == "invasion.target.cooldown" &&
			index+1 < len(dependencies.Steps) && dependencies.Steps[index+1].Action == "invasion.target.release"
	}
	if !foundCooldownProbe {
		t.Fatalf("invasion ADI dependency does not preserve code-95 target rotation: %#v", dependencies.Steps)
	}
	var body attackBody
	if err := json.Unmarshal(resolved.Command.Payload, &body); err != nil {
		t.Fatal(err)
	}
	if body.Leader != 0 || len(body.Waves) != 1 {
		t.Fatalf("unexpected invasion attack body: %#v", body)
	}
	if body.Waves[0].Middle.Units[0] != (attackPair{216, 375}) {
		t.Fatalf("front was not capped to the freshly resolved capacity: %#v", body.Waves[0].Middle.Units)
	}
	if len(body.SupportTroops) != 8 || body.SupportTroops[0] != (attackPair{217, 240}) {
		t.Fatalf("courtyard support was not capped to the freshly resolved capacity: %#v", body.SupportTroops)
	}
}

func TestInvasionAttackDoesNotPlanDuringPurchasedProtectionMode(t *testing.T) {
	now := time.Now().UTC()
	gameState := State.NewGameState()
	gameState.Player.ProtectionMode = State.PlayerProtectionModeState{
		ModeState: 1, RemainingSec: 3_600, ObservedAt: now,
	}
	_, err := planInvasionAttack(t.Context(), Intent.PlanningContext{State: gameState}, json.RawMessage(`{}`))
	if err == nil || err.Error() != "invasion attacks are disabled while Protection Mode is preparing or active" {
		t.Fatalf("protected invasion plan error = %v", err)
	}
}

func TestInvasionCRACommandDependenciesEndWithInvasionAttackGuard(t *testing.T) {
	gameState, gameData, arguments := invasionReservationLifecycleFixture(t)
	launchStep := productionInvasionLaunchStep(t, gameState, gameData, arguments)
	dependencies, err := (&Application{}).resolveCRACommandDependencies(
		t.Context(), Intent.PlanningContext{State: gameState}, Intent.Step{
			Payload: launchStep.CommandDependencies.Payload,
			Command: Protocol.Command{
				Opcode: "cra", Payload: launchStep.CommandDependencies.Payload,
			},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(dependencies.Steps) == 0 {
		t.Fatal("production invasion CRA has no command dependencies")
	}
	guard := dependencies.Steps[len(dependencies.Steps)-1]
	if guard.Action != "invasion.attack.guard" {
		t.Fatalf("last invasion CRA dependency action = %q, want invasion.attack.guard", guard.Action)
	}
	var wantArguments, gotArguments any
	if err := json.Unmarshal(launchStep.ResolverArguments, &wantArguments); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(guard.ActionArguments, &gotArguments); err != nil {
		t.Fatal(err)
	}
	wantJSON, _ := json.Marshal(wantArguments)
	gotJSON, _ := json.Marshal(gotArguments)
	if string(gotJSON) != string(wantJSON) {
		t.Fatalf("final invasion guard arguments = %s, want full resolved request %s", gotJSON, wantJSON)
	}
}

func TestInvasionAttackGuardRejectsChangedLaunchBoundaries(t *testing.T) {
	tests := []struct {
		name      string
		mutate    func(*State.GameState)
		wantError string
	}{
		{
			name: "score target reached",
			mutate: func(gameState *State.GameState) {
				score := gameState.EventScores.ByEvent[71]
				score.PlayerScore = 1_000
				gameState.EventScores.ByEvent[71] = score
			},
			wantError: "score target reached",
		},
		{
			name: "event cutoff reached",
			mutate: func(gameState *State.GameState) {
				score := gameState.EventScores.ByEvent[71]
				score.RemainingSec = 30
				gameState.EventScores.ByEvent[71] = score
			},
			wantError: "seconds remaining",
		},
		{
			name: "recurring event occurrence changed",
			mutate: func(gameState *State.GameState) {
				score := gameState.EventScores.ByEvent[71]
				score.ObservedAt = time.Now().UTC()
				score.RemainingSec = int64((24 * time.Hour) / time.Second)
				gameState.EventScores.ByEvent[71] = score
			},
			wantError: "occurrence",
		},
		{
			name: "commander held by unresolved invasion launch",
			mutate: func(gameState *State.GameState) {
				now := time.Now().UTC()
				gameState.Invasion.ReserveTarget(State.InvasionTargetReservation{
					KingdomID: 0, EventID: 71,
					OccurrenceEndsAt: State.ScalableEventEndsAt(gameState.EventScores.ByEvent[71]),
					TargetTypeID:     State.MapTypeForeignLord, X: 202, Y: 203,
					SourceCastleID: 1, CommanderID: 7, CommanderKnown: true,
					OperationID: "indeterminate-cra", ReservedAt: now,
				})
			},
			wantError: "commander",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			gameState, gameData, arguments := invasionReservationLifecycleFixture(t)
			launchStep := productionInvasionLaunchStep(t, gameState, gameData, arguments)
			dependencies, err := (&Application{}).resolveCRACommandDependencies(
				t.Context(), Intent.PlanningContext{State: gameState}, Intent.Step{
					Payload: launchStep.CommandDependencies.Payload,
					Command: Protocol.Command{
						Opcode: "cra", Payload: launchStep.CommandDependencies.Payload,
					},
				},
			)
			if err != nil {
				t.Fatal(err)
			}
			guard := dependencies.Steps[len(dependencies.Steps)-1]
			if guard.Action != "invasion.attack.guard" {
				t.Fatalf("last invasion CRA dependency action = %q, want invasion.attack.guard", guard.Action)
			}
			application := &Application{State: State.NewStore(gameState)}
			if err := application.guardInvasionAttack(t.Context(), guard.ActionArguments); err != nil {
				t.Fatalf("unchanged launch boundary failed final guard: %v", err)
			}

			test.mutate(&gameState)
			application.State = State.NewStore(gameState)
			err = application.guardInvasionAttack(t.Context(), guard.ActionArguments)
			if err == nil || !strings.Contains(err.Error(), test.wantError) {
				t.Fatalf("changed launch boundary error = %v, want %q", err, test.wantError)
			}
		})
	}
}

func TestInvasionAttackUsesLiveEventFortificationCurrencies(t *testing.T) {
	now := time.Now().UTC()
	gameState := State.NewGameState()
	gameState.Invasion.FortifyCurrencies = []string{"GTO", "STO", "ST"}
	gameState.Castles[1] = State.CastleState{ID: 1, KingdomID: 0}
	gameState.EventScores.ActiveEventID = 103
	gameState.EventScores.ByEvent[103] = State.ScalableEventScore{
		EventID: 103, RemainingSec: 3_600, ObservedAt: now,
	}
	gameState.Map[0] = map[string]State.MapObservation{
		"206:937": {
			KingdomID: 0, TypeID: 34, X: 206, Y: 937, ObjectID: 70, Level: 70,
			InvasionAvailabilityKnown: true,
		},
	}
	request := invasionAttackRequest{
		SourceCastleID: 1, EventID: 103,
		EventEndsAt: State.ScalableEventEndsAt(gameState.EventScores.ByEvent[103]),
		ScoreTarget: 1, TargetTypeID: 34,
		KingdomID: 0, TargetX: 206, TargetY: 937, FortifyCurrency: "KM", HorseTravelBoostID: -1,
	}
	arguments, _ := json.Marshal(request)
	_, _, _, err := invasionAttackContext(Intent.PlanningContext{State: gameState}, arguments)
	if err == nil || err.Error() != "fortification currency KM is unavailable for event 103" {
		t.Fatalf("unsupported Bloodcrow fortification error = %v", err)
	}

	request.FortifyCurrency = "ST"
	arguments, _ = json.Marshal(request)
	resolved, _, _, err := invasionAttackContext(Intent.PlanningContext{State: gameState}, arguments)
	if err != nil || resolved.FortifyCurrency != "ST" {
		t.Fatalf("Samurai-token Bloodcrow fortification = %+v err=%v", resolved, err)
	}

	gameState.Invasion.FortifyCurrencies = []string{"GTO", "STO", "KT"}
	request.FortifyCurrency = "KT"
	arguments, _ = json.Marshal(request)
	resolved, _, _, err = invasionAttackContext(Intent.PlanningContext{State: gameState}, arguments)
	if err != nil || resolved.FortifyCurrency != "KT" {
		t.Fatalf("server-supplied Bloodcrow fortification = %+v err=%v", resolved, err)
	}

	gameState.Invasion.FortifyCurrencies = nil
	_, _, _, err = invasionAttackContext(Intent.PlanningContext{State: gameState}, arguments)
	if err == nil || err.Error() != `unsupported invasion fortification currency "KT"` {
		t.Fatalf("unobserved dynamic fortification error = %v", err)
	}
}

func TestGuardInvasionTargetRequiresLaunchTimeMapObservation(t *testing.T) {
	now := time.Now().UTC()
	gameState := State.NewGameState()
	gameState.Castles[1] = State.CastleState{ID: 1, KingdomID: 0, X: 100, Y: 100}
	gameState.Commanders[7] = State.CommanderState{ID: 7, Available: true}
	gameState.EventScores.ActiveEventID = 71
	gameState.EventScores.ByEvent[71] = State.ScalableEventScore{
		EventID: 71, PlayerScore: 0, RemainingSec: 3_600, ObservedAt: now,
	}
	gameState.Map[0] = map[string]State.MapObservation{
		"101:100": {
			KingdomID: 0, TypeID: 21, X: 101, Y: 100, ObjectID: 70, Level: 70,
			InvasionAvailabilityKnown: true, ObservedAt: now,
		},
	}
	request := resolvedInvasionAttackRequest{
		invasionAttackRequest: invasionAttackRequest{
			SourceCastleID: 1, EventID: 71,
			EventEndsAt: State.ScalableEventEndsAt(gameState.EventScores.ByEvent[71]),
			ScoreTarget: 1_000, MinimumRemainingSec: 60,
			TargetTypeID: 21, KingdomID: 0, TargetX: 101, TargetY: 100, TargetObjectID: 70,
		},
		CommanderID: 7,
	}
	arguments, _ := json.Marshal(invasionTargetVerificationRequest{Request: request, RefreshStartedAt: now.Add(time.Second)})
	application := &Application{State: State.NewStore(gameState)}
	if err := application.guardInvasionTarget(t.Context(), arguments); err == nil {
		t.Fatal("stale invasion target passed launch-time refresh guard")
	}

	target := gameState.Map[0]["101:100"]
	target.ObservedAt = now.Add(2 * time.Second)
	gameState.Map[0]["101:100"] = target
	application.State = State.NewStore(gameState)
	if err := application.guardInvasionTarget(t.Context(), arguments); err != nil {
		t.Fatalf("fresh invasion target failed launch-time refresh guard: %v", err)
	}
}

func TestCaptureInvasionLaunchTracksActiveEvent(t *testing.T) {
	now := time.Date(2026, 7, 21, 13, 45, 0, 0, time.UTC)
	arrivesAt := now.Add(90 * time.Second)
	commanderID := State.CommanderID(4)
	gameState := State.NewGameState()
	gameState.EventScores.ActiveEventID = 103
	gameState.EventScores.ByEvent[103] = State.ScalableEventScore{
		EventID: 103, RemainingSec: 7_200, ObservedAt: now,
	}
	occurrenceEndsAt := State.ScalableEventEndsAt(gameState.EventScores.ByEvent[103])
	gameState.Movements[88] = State.MovementState{
		ID: 88, Direction: 0, SourceCastleID: 1, CommanderID: &commanderID, KingdomID: 0,
		TargetTypeID: 34, TargetX: 120, TargetY: 121, StartedAt: now,
		ArrivesAt: &arrivesAt, ObservedAt: now,
	}
	gameState.Invasion.ReserveTarget(State.InvasionTargetReservation{
		KingdomID: 0, EventID: 103, OccurrenceEndsAt: occurrenceEndsAt,
		TargetTypeID: 34, X: 120, Y: 121, SourceCastleID: 1,
		CommanderID: commanderID, CommanderKnown: true,
		OperationID: "capture-invasion", ReservedAt: now.Add(-time.Second),
	})
	application := &Application{State: State.NewStore(gameState)}
	arguments, _ := json.Marshal(resolvedInvasionAttackRequest{
		invasionAttackRequest: invasionAttackRequest{
			SourceCastleID: 1, EventID: 103, EventEndsAt: occurrenceEndsAt,
			TargetTypeID: 34, KingdomID: 0, TargetX: 120, TargetY: 121,
		},
		CommanderID: commanderID,
	})
	ctx := Outbound.WithMetadata(t.Context(), Outbound.Metadata{OperationID: "capture-invasion"})
	if err := application.captureInvasionLaunch(ctx, arguments); err != nil {
		t.Fatal(err)
	}
	if err := application.captureInvasionLaunch(ctx, arguments); err != nil {
		t.Fatal(err)
	}
	activity := application.State.Snapshot().EventScores.ActivityByEvent[103]
	if activity.Invasion.Launches != 1 || len(activity.PendingAttacks) != 1 ||
		activity.PendingAttacks[0].Kind != State.EventActivityInvasion {
		t.Fatalf("unexpected Bloodcrow activity: %#v", activity)
	}
	analytics := application.State.Snapshot().AttackAnalytics.PendingAttacks
	if len(analytics) != 1 || analytics[0].FeatureID != State.AttackFeatureAutoInvasion ||
		analytics[0].MovementID != 88 || analytics[0].TargetX != 120 || analytics[0].TargetY != 121 {
		t.Fatalf("unexpected invasion target settlement tracking: %#v", analytics)
	}
	if _, reserved := application.State.Snapshot().Invasion.TargetReservation(0, 120, 121); reserved {
		t.Fatal("confirmed invasion movement did not release its pre-dispatch reservation")
	}
}

func TestInvasionTargetPreDispatchReservationAndDefinitiveRelease(t *testing.T) {
	application := &Application{State: State.NewStore(State.NewGameState())}
	commanderID := State.CommanderID(0)
	arguments, _ := json.Marshal(struct {
		KingdomID        State.KingdomID    `json:"KID"`
		EventID          int64              `json:"_citadelEventId"`
		OccurrenceEndsAt time.Time          `json:"_citadelEventEndsAt"`
		TargetTypeID     int                `json:"_citadelTargetTypeId"`
		TargetX          int                `json:"TX"`
		TargetY          int                `json:"TY"`
		SourceCastleID   State.CastleID     `json:"_citadelSourceCastleId"`
		SourceX          int                `json:"SX"`
		SourceY          int                `json:"SY"`
		CommanderID      *State.CommanderID `json:"LID"`
	}{
		KingdomID: 0, EventID: 71, OccurrenceEndsAt: time.Now().UTC().Add(time.Hour),
		TargetTypeID: State.MapTypeForeignLord, TargetX: 120, TargetY: 121,
		SourceCastleID: 1, SourceX: 100, SourceY: 101, CommanderID: &commanderID,
	})
	ctx := Outbound.WithMetadata(t.Context(), Outbound.Metadata{OperationID: "reserve-invasion"})
	if err := application.reserveInvasionTarget(ctx, arguments); err != nil {
		t.Fatal(err)
	}
	if err := application.reserveInvasionTarget(ctx, arguments); err != nil {
		t.Fatalf("owning operation could not repeat its reservation: %v", err)
	}
	if reservation, found := application.State.Snapshot().Invasion.TargetReservation(0, 120, 121); !found ||
		reservation.OperationID != "reserve-invasion" || !reservation.SourceKnown ||
		reservation.SourceX != 100 || reservation.SourceY != 101 {
		t.Fatalf("pre-dispatch reservation = %#v, found=%t", reservation, found)
	}
	var otherTarget map[string]any
	if err := json.Unmarshal(arguments, &otherTarget); err != nil {
		t.Fatal(err)
	}
	otherTarget["TX"] = 122
	otherTarget["TY"] = 123
	otherArguments, _ := json.Marshal(otherTarget)
	otherContext := Outbound.WithMetadata(t.Context(), Outbound.Metadata{OperationID: "reserve-invasion-2"})
	if err := application.reserveInvasionTarget(otherContext, otherArguments); !errors.Is(err, Intent.ErrPlanStale) {
		t.Fatalf("same commander reserved a second target: %v", err)
	}
	if _, found := application.State.Snapshot().Invasion.TargetReservation(0, 122, 123); found {
		t.Fatal("second target retained a duplicate commander reservation")
	}
	if err := application.releaseInvasionTarget(
		Outbound.WithMetadata(t.Context(), Outbound.Metadata{OperationID: "another-operation"}), arguments,
	); err != nil {
		t.Fatal(err)
	}
	if _, found := application.State.Snapshot().Invasion.TargetReservation(0, 120, 121); !found {
		t.Fatal("another operation released the target reservation")
	}
	if err := application.releaseInvasionTarget(ctx, arguments); err != nil {
		t.Fatal(err)
	}
	if _, found := application.State.Snapshot().Invasion.TargetReservation(0, 120, 121); found {
		t.Fatal("definitive send failure did not release the target reservation")
	}
}

func TestInvasionTargetReservationLifecycleThroughIntentEngine(t *testing.T) {
	tests := []struct {
		name             string
		opcode           string
		responseCode     int
		sendFailure      string
		wantStatus       Intent.Status
		wantPlannerCalls int
		wantReserved     bool
		wantUnavailable  bool
		wantSafetyLock   bool
	}{
		{
			name: "ADI code 95 releases reservation and marks target unavailable", opcode: "adi", responseCode: 95,
			wantStatus: Intent.StatusFailed, wantPlannerCalls: 2, wantUnavailable: true,
		},
		{
			name: "CRA code 95 locks lane and retains reservation for review", opcode: "cra", responseCode: 95,
			wantStatus: Intent.StatusFailed, wantPlannerCalls: 2, wantReserved: true, wantSafetyLock: true,
		},
		{
			name: "CRA code 91 locks lane and retains reservation for review", opcode: "cra", responseCode: 91,
			wantStatus: Intent.StatusFailed, wantPlannerCalls: 2, wantReserved: true, wantSafetyLock: true,
		},
		{
			name: "definitive CRA send failure clears reservation", opcode: "cra", sendFailure: "definitive",
			wantStatus: Intent.StatusFailed, wantPlannerCalls: 2,
		},
		{
			name: "indeterminate CRA send survives restart", opcode: "cra", sendFailure: "indeterminate",
			wantStatus: Intent.StatusIndeterminate, wantPlannerCalls: 2, wantReserved: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			gameState, gameData, arguments := invasionReservationLifecycleFixture(t)
			dataDir := t.TempDir()
			stateStore := State.NewStore(gameState)
			application := &Application{DataDir: dataDir, State: stateStore}

			plan, err := planInvasionAttack(
				t.Context(), Intent.PlanningContext{State: gameState, GameData: gameData}, arguments,
			)
			if err != nil {
				t.Fatal(err)
			}
			var launchStep Intent.Step
			for _, candidate := range plan.Steps {
				if candidate.Resolver == "invasion.attack.build" {
					launchStep = candidate
					break
				}
			}
			if launchStep.Resolver != "invasion.attack.build" || launchStep.CommandDependencies == nil {
				t.Fatalf("production invasion plan has no deferred CRA launch: %#v", plan.Steps)
			}
			craStep, err := application.resolveInvasionAttackStep(
				t.Context(), Intent.PlanningContext{State: gameState, GameData: gameData}, launchStep.ResolverArguments,
			)
			if err != nil {
				t.Fatal(err)
			}
			routePayload := launchStep.CommandDependencies.Payload
			dependencies, err := application.resolveCRACommandDependencies(
				t.Context(), Intent.PlanningContext{State: gameState}, Intent.Step{
					Payload: routePayload,
					Command: Protocol.Command{Opcode: "cra", Payload: routePayload},
				},
			)
			if err != nil {
				t.Fatal(err)
			}
			var adiStep Intent.Step
			for _, dependency := range dependencies.Steps {
				if dependency.Opcode == "adi" {
					adiStep = dependency
					break
				}
			}
			if adiStep.Opcode != "adi" {
				t.Fatalf("production CRA dependencies have no ADI step: %#v", dependencies.Steps)
			}
			adiReservation, err := decodeInvasionTargetReservationRequest(adiStep.PreDispatchArguments)
			if err != nil || adiReservation.TargetTypeID != State.MapTypeForeignLord ||
				adiReservation.TargetX != 1165 || adiReservation.TargetY != 1166 ||
				adiReservation.SourceCastleID != 1 || !adiReservation.TargetOnly ||
				adiReservation.CommanderKnown || adiReservation.CommanderID != 0 {
				t.Fatalf(
					"production ADI reservation arguments = %+v from %s, err=%v",
					adiReservation, adiStep.PreDispatchArguments, err,
				)
			}

			step := craStep
			if test.opcode == "adi" {
				step = adiStep
			}
			pipeline := Ingest.NewPipeline(stateStore, nil, Ingest.NewRegistry())
			sender := &invasionReservationLifecycleSender{
				application:  application,
				pipeline:     pipeline,
				responseCode: test.responseCode,
				kingdomID:    0,
				targetX:      1165,
				targetY:      1166,
			}
			switch test.sendFailure {
			case "definitive":
				sender.sendErr = errors.New("simulated socket rejection before write")
			case "indeterminate":
				sender.sendErr = Outbound.MarkIndeterminate(errors.New("simulated uncertain wire write"))
			}

			plannerCalls := 0
			registry := Intent.NewRegistry()
			intentName := "test.invasion.reservation." + test.opcode
			if err := registry.Register(Intent.Definition{
				Name: intentName, Effect: Intent.EffectLaunch,
				Planner: func(_ context.Context, input Intent.PlanningContext, _ json.RawMessage) (Intent.Plan, error) {
					plannerCalls++
					if _, reserved := input.State.Invasion.TargetReservation(0, 1165, 1166); reserved {
						return Intent.Plan{}, fmt.Errorf("%w: invasion target has an unresolved launch", Intent.ErrPlanStale)
					}
					if input.State.Invasion.TargetUnavailable(0, 1165, 1166) {
						return Intent.Plan{}, fmt.Errorf("%w: invasion target is on cooldown", Intent.ErrPlanStale)
					}
					return Intent.Plan{Steps: []Intent.Step{step}}, nil
				},
			}); err != nil {
				t.Fatal(err)
			}
			engine := Intent.NewEngine(registry, stateStore, nil, sender, pipeline)
			engine.SetLaneSafetyPersistence(application.saveStateEvent)
			if err := engine.RegisterAction("invasion.target.reserve", application.reserveInvasionTarget); err != nil {
				t.Fatal(err)
			}
			if err := engine.RegisterAction("invasion.target.release", application.releaseInvasionTarget); err != nil {
				t.Fatal(err)
			}
			if err := engine.RegisterAction("invasion.target.cooldown", application.cooldownInvasionTarget); err != nil {
				t.Fatal(err)
			}

			operationID := "invasion-lifecycle-" + test.opcode + "-" + test.sendFailure
			if test.responseCode > 0 {
				operationID = fmt.Sprintf("invasion-lifecycle-%s-%d", test.opcode, test.responseCode)
			}
			receipt := engine.Submit(t.Context(), Intent.Request{
				ID: operationID, Name: intentName, Actor: "automation:autoInvasion", AutomationLane: "autoInvasion",
			})
			if receipt.Status != test.wantStatus {
				t.Fatalf("reservation lifecycle receipt = %#v", receipt)
			}
			if plannerCalls != test.wantPlannerCalls {
				t.Fatalf("planner calls = %d, want %d; receipt=%#v", plannerCalls, test.wantPlannerCalls, receipt)
			}
			if test.responseCode == 95 && !test.wantSafetyLock && (receipt.Failure == nil || receipt.Failure.Kind != Intent.FailureStaleState) {
				t.Fatalf("code-95 failure = %#v, want stale state", receipt.Failure)
			}
			if test.responseCode == 91 && (receipt.Failure == nil || receipt.Failure.GameCode == nil || *receipt.Failure.GameCode != 91) {
				t.Fatalf("code-91 failure = %#v", receipt.Failure)
			}
			if sender.markerErr != nil {
				t.Fatal(sender.markerErr)
			}
			if !sender.markerObservedBeforeSend || !sender.persistedMarkerObservedBeforeSend {
				t.Fatalf(
					"reservation before %s send: memory=%t persisted=%t",
					test.opcode, sender.markerObservedBeforeSend, sender.persistedMarkerObservedBeforeSend,
				)
			}
			if len(sender.opcodes) != 1 || sender.opcodes[0] != test.opcode {
				t.Fatalf("sent opcodes = %v, want [%s]", sender.opcodes, test.opcode)
			}

			inMemoryReservation, inMemoryReserved := stateStore.ReadOnlyView().Invasion.TargetReservation(0, 1165, 1166)
			persisted, err := State.LoadSnapshot(dataDir)
			if err != nil {
				t.Fatal(err)
			}
			persistedReservation, persistedReserved := persisted.Invasion.TargetReservation(0, 1165, 1166)
			if got := persisted.Automations["autoInvasion"].SafetyLock.Active(time.Now()); got != test.wantSafetyLock {
				t.Fatalf("persisted safety lock=%t, want=%t", got, test.wantSafetyLock)
			}
			if inMemoryReserved != test.wantReserved || persistedReserved != test.wantReserved {
				t.Fatalf(
					"final reservation: memory=%t persisted=%t want=%t",
					inMemoryReserved, persistedReserved, test.wantReserved,
				)
			}
			inMemoryUnavailable := stateStore.ReadOnlyView().Invasion.TargetUnavailable(0, 1165, 1166)
			persistedUnavailable := persisted.Invasion.TargetUnavailable(0, 1165, 1166)
			if inMemoryUnavailable != test.wantUnavailable || persistedUnavailable != test.wantUnavailable {
				t.Fatalf(
					"final cooldown state: memory=%t persisted=%t want=%t",
					inMemoryUnavailable, persistedUnavailable, test.wantUnavailable,
				)
			}
			if test.wantReserved {
				if inMemoryReservation.OperationID != operationID || persistedReservation.OperationID != operationID {
					t.Fatalf(
						"reservation operation IDs: memory=%q persisted=%q want=%q",
						inMemoryReservation.OperationID, persistedReservation.OperationID, operationID,
					)
				}
				if !inMemoryReservation.SourceKnown || inMemoryReservation.SourceX != 1164 || inMemoryReservation.SourceY != 1167 ||
					!persistedReservation.SourceKnown || persistedReservation.SourceX != 1164 || persistedReservation.SourceY != 1167 {
					t.Fatalf("reservation source coordinates: memory=%#v persisted=%#v", inMemoryReservation, persistedReservation)
				}
				if test.opcode == "adi" {
					if inMemoryReservation.CommanderKnown || persistedReservation.CommanderKnown ||
						State.InvasionCommanderReserved(stateStore.ReadOnlyView(), 7) {
						t.Fatalf("target-only ADI reservation held commander 7: memory=%#v persisted=%#v", inMemoryReservation, persistedReservation)
					}
				} else if !inMemoryReservation.CommanderKnown || !persistedReservation.CommanderKnown {
					t.Fatalf("indeterminate CRA reservation lost commander binding: memory=%#v persisted=%#v", inMemoryReservation, persistedReservation)
				}
			}
			if test.sendFailure == "indeterminate" {
				restarted := State.NewStore(persisted)
				reservation, reserved := restarted.ReadOnlyView().Invasion.TargetReservation(0, 1165, 1166)
				if !reserved || reservation.OperationID != operationID {
					t.Fatalf("restarted reservation = %#v, found=%t", reservation, reserved)
				}
			}
		})
	}
}

type invasionReservationLifecycleSender struct {
	application                       *Application
	pipeline                          *Ingest.Pipeline
	responseCode                      int
	sendErr                           error
	kingdomID                         State.KingdomID
	targetX                           int
	targetY                           int
	opcodes                           []string
	markerObservedBeforeSend          bool
	persistedMarkerObservedBeforeSend bool
	markerErr                         error
}

func (*invasionReservationLifecycleSender) Ready() bool               { return true }
func (*invasionReservationLifecycleSender) Namespace() string         { return "EmpireEx_21" }
func (*invasionReservationLifecycleSender) CorrelatesResponses() bool { return true }

func (sender *invasionReservationLifecycleSender) Send(ctx context.Context, payload []byte) error {
	command, err := Protocol.Decode(string(payload), Protocol.DirectionOutbound, time.Now().UTC())
	if err != nil {
		return err
	}
	sender.opcodes = append(sender.opcodes, command.Opcode)
	metadata := Outbound.MetadataFromContext(ctx)
	if metadata.OperationID == "" || metadata.ResponseToken == "" {
		sender.markerErr = fmt.Errorf("missing correlated send metadata: %+v", metadata)
	} else if reservation, reserved := sender.application.State.ReadOnlyView().Invasion.TargetReservation(
		sender.kingdomID, sender.targetX, sender.targetY,
	); !reserved || reservation.OperationID != metadata.OperationID {
		sender.markerErr = fmt.Errorf("in-memory pre-send reservation = %#v, found=%t", reservation, reserved)
	} else {
		sender.markerObservedBeforeSend = true
	}
	persisted, loadErr := State.LoadSnapshot(sender.application.DataDir)
	if loadErr != nil {
		if sender.markerErr == nil {
			sender.markerErr = fmt.Errorf("load pre-send reservation: %w", loadErr)
		}
	} else if reservation, reserved := persisted.Invasion.TargetReservation(
		sender.kingdomID, sender.targetX, sender.targetY,
	); !reserved || reservation.OperationID != metadata.OperationID {
		if sender.markerErr == nil {
			sender.markerErr = fmt.Errorf("persisted pre-send reservation = %#v, found=%t", reservation, reserved)
		}
	} else {
		sender.persistedMarkerObservedBeforeSend = true
	}
	if sender.sendErr != nil {
		return sender.sendErr
	}
	if sender.responseCode <= 0 {
		return fmt.Errorf("response code is required for %s", command.Opcode)
	}
	code := sender.responseCode
	observed := sender.pipeline.ObserveFrame(Protocol.Frame{
		Direction: Protocol.DirectionInbound, Namespace: command.Namespace, Opcode: command.Opcode,
		ResponseCode: &code, ReceivedAt: time.Now().UTC(), ResponseToken: metadata.ResponseToken,
		CausationOperationID: metadata.OperationID,
	})
	go func() { _, _ = sender.pipeline.CommitFrame(context.Background(), observed) }()
	return nil
}

func productionInvasionLaunchStep(
	t *testing.T,
	gameState State.GameState,
	gameData *GameData.Store,
	arguments json.RawMessage,
) Intent.Step {
	t.Helper()
	plan, err := planInvasionAttack(
		t.Context(), Intent.PlanningContext{State: gameState, GameData: gameData}, arguments,
	)
	if err != nil {
		t.Fatal(err)
	}
	for _, step := range plan.Steps {
		if step.Resolver == "invasion.attack.build" && step.CommandDependencies != nil &&
			step.CommandDependencies.Opcode == "cra" {
			return step
		}
	}
	t.Fatalf("production invasion plan has no deferred CRA launch: %#v", plan.Steps)
	return Intent.Step{}
}

func invasionReservationLifecycleFixture(t *testing.T) (State.GameState, *GameData.Store, json.RawMessage) {
	t.Helper()
	gameData, err := GameData.DecodeStore([]byte(`{
		"versionInfo":[],
		"units":[{"wodID":216}],
		"buildings":[],
		"effects":[],
		"legendskills":[],
		"effectCaps":[]
	}`), GameData.SourceMetadata{ItemVersion: "invasion-reservation-test"})
	if err != nil {
		t.Fatal(err)
	}
	unitID := int64(216)
	now := time.Now().UTC()
	gameState := State.NewGameState()
	gameState.Castles[1] = State.CastleState{
		ID: 1, KingdomID: 0, X: 1164, Y: 1167,
		Units: State.CastleUnits{Stationed: map[State.UnitID]int64{216: 100}},
	}
	gameState.Commanders[7] = State.CommanderState{ID: 7, Available: true}
	gameState.Player.LegendSkills = State.LegendSkillState{ObservedAt: now}
	gameState.EventScores.ActiveEventID = 71
	gameState.EventScores.ByEvent[71] = State.ScalableEventScore{
		EventID: 71, DifficultyID: 1, PlayerScore: 0, RemainingSec: 120, ObservedAt: now,
	}
	gameState.Map[0] = map[string]State.MapObservation{
		"1165:1166": {
			KingdomID: 0, TypeID: State.MapTypeForeignLord, X: 1165, Y: 1166,
			ObjectID: 70, Level: 70, InvasionAvailabilityKnown: true, ObservedAt: now,
		},
	}
	gameState.AttackDialog = State.AttackDialogState{
		SourceCastleID: 1, KingdomID: 0, ObservedAt: now,
		Target: State.AttackDialogTarget{
			TypeID: State.MapTypeForeignLord, X: 1165, Y: 1166,
			ObjectID: 70, InvasionAvailabilityKnown: true,
		},
		ActiveEffects: []State.AttackDialogEffect{},
	}
	request := invasionAttackRequest{
		SourceCastleID: 1, EventID: 71, ScoreTarget: 1_000, MinimumRemainingSec: 60,
		TargetTypeID: State.MapTypeForeignLord, KingdomID: 0,
		TargetX: 1165, TargetY: 1166, TargetObjectID: 70,
		Preset: AttackPresets.Preset{
			Name: "Reservation lifecycle",
			Waves: []AttackPresets.Wave{{
				Middle: AttackPresets.Lane{Troops: []AttackPresets.Slot{{ItemID: &unitID, Quantity: 1}}},
			}},
		},
	}
	arguments, err := json.Marshal(request)
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]any
	if err := json.Unmarshal(arguments, &fields); err != nil {
		t.Fatal(err)
	}
	fields["eventEndsAt"] = State.ScalableEventEndsAt(gameState.EventScores.ByEvent[71])
	arguments, err = json.Marshal(fields)
	if err != nil {
		t.Fatal(err)
	}
	return gameState, gameData, arguments
}

func TestCaptureInvasionLaunchRejectsMovementOlderThanReservation(t *testing.T) {
	now := time.Date(2026, 9, 9, 12, 0, 0, 0, time.UTC)
	occurrenceEndsAt := now.Add(2 * time.Hour)
	arrivesAt := now.Add(time.Minute)
	commanderID := State.CommanderID(4)
	state := State.NewGameState()
	state.EventScores.ActiveEventID = 103
	state.EventScores.ByEvent[103] = State.ScalableEventScore{
		EventID: 103, RemainingSec: 7_200, ObservedAt: now,
	}
	state.Movements[88] = State.MovementState{
		ID: 88, Direction: 0, SourceCastleID: 1, CommanderID: &commanderID,
		KingdomID: 0, TargetTypeID: 34, TargetX: 120, TargetY: 121,
		StartedAt: now.Add(-5 * time.Minute), ArrivesAt: &arrivesAt, ObservedAt: now.Add(time.Second),
	}
	state.Invasion.ReserveTarget(State.InvasionTargetReservation{
		KingdomID: 0, EventID: 103, OccurrenceEndsAt: occurrenceEndsAt,
		TargetTypeID: 34, X: 120, Y: 121, SourceCastleID: 1,
		CommanderID: commanderID, CommanderKnown: true,
		OperationID: "new-cra", ReservedAt: now,
	})
	application := &Application{State: State.NewStore(state)}
	arguments, _ := json.Marshal(resolvedInvasionAttackRequest{
		invasionAttackRequest: invasionAttackRequest{
			SourceCastleID: 1, EventID: 103, EventEndsAt: occurrenceEndsAt,
			TargetTypeID: 34, KingdomID: 0, TargetX: 120, TargetY: 121,
		},
		CommanderID: commanderID,
	})
	ctx := Outbound.WithMetadata(t.Context(), Outbound.Metadata{OperationID: "new-cra"})
	if err := application.captureInvasionLaunch(ctx, arguments); err == nil {
		t.Fatal("historical matching movement was accepted as the new CRA launch")
	}
	snapshot := application.State.Snapshot()
	if _, reserved := snapshot.Invasion.TargetReservation(0, 120, 121); !reserved ||
		len(snapshot.AttackAnalytics.PendingAttacks) != 0 {
		t.Fatalf("historical movement changed launch state: invasion=%#v analytics=%#v", snapshot.Invasion, snapshot.AttackAnalytics)
	}
}

func TestInvasionReservationReconciliationRetainsFullMarkerAfterScopedGAMOmission(t *testing.T) {
	now := time.Now().UTC()
	occurrenceEndsAt := now.Add(time.Hour)
	reservedAt := now.Add(-State.InvasionTargetReservationReconcileGrace - time.Second)
	state := State.NewGameState()
	state.Castles[1] = State.CastleState{ID: 1, KingdomID: 0, X: 100, Y: 100, Focused: true}
	state.EventScores.ActiveEventID = 71
	state.EventScores.ByEvent[71] = State.ScalableEventScore{
		EventID: 71, RemainingSec: 3_600, ObservedAt: now,
	}
	state.Invasion.ReserveTarget(State.InvasionTargetReservation{
		KingdomID: 0, EventID: 71, OccurrenceEndsAt: occurrenceEndsAt,
		TargetTypeID: State.MapTypeForeignLord, X: 101, Y: 102,
		SourceCastleID: 1, CommanderID: 7, CommanderKnown: true,
		OperationID: "uncertain-cra", ReservedAt: reservedAt,
	})
	arguments, _ := json.Marshal(invasionTargetReconcileRequest{
		FocusCastleID: 1, KingdomID: 0, EventID: 71, OccurrenceEndsAt: occurrenceEndsAt,
		TargetTypeID: State.MapTypeForeignLord,
		TargetX:      101, TargetY: 102, OperationID: "uncertain-cra", ReservedAt: reservedAt,
	})
	plan, err := planInvasionTargetReconcile(t.Context(), Intent.PlanningContext{State: state}, arguments)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Steps) != 2 || plan.Steps[0].Opcode != "gam" ||
		plan.Steps[0].ResponseBarrier != Intent.ResponseBarrierCommitted ||
		plan.Steps[1].Action != "invasion.target.reconcile" {
		t.Fatalf("invasion reconciliation evidence plan = %#v", plan.Steps)
	}
	var verification invasionTargetReconcileRequest
	if err := json.Unmarshal(plan.Steps[1].ActionArguments, &verification); err != nil || verification.ReconcileStartedAt.IsZero() {
		t.Fatalf("reconciliation boundary = %#v err=%v", verification, err)
	}
	state.MovementSnapshot.ObservedAt = verification.ReconcileStartedAt.Add(time.Second)
	state.Map[0] = map[string]State.MapObservation{
		"101:102": {
			KingdomID: 0, TypeID: State.MapTypeForeignLord, X: 101, Y: 102, Level: 70,
			InvasionAvailabilityKnown: true, ObservedAt: verification.ReconcileStartedAt.Add(2 * time.Second),
		},
	}
	application := &Application{State: State.NewStore(state)}
	if err := application.reconcileInvasionTargetReservation(t.Context(), plan.Steps[1].ActionArguments); err != nil {
		t.Fatal(err)
	}
	retained, reserved := application.State.Snapshot().Invasion.TargetReservation(0, 101, 102)
	if !reserved || !retained.CommanderKnown || retained.CommanderID != 7 || retained.ReconcileAttempts != 1 ||
		!retained.ReconcileAfter.After(verification.ReconcileStartedAt) {
		t.Fatalf("scoped GAM omission weakened full reservation: %#v, found=%t", retained, reserved)
	}
}

func TestInvasionReservationReconciliationRecordsMatchedMovementAfterHAC(t *testing.T) {
	now := time.Now().UTC()
	reservedAt := now.Add(-State.InvasionTargetReservationReconcileGrace - time.Second)
	commanderID := State.CommanderID(0)
	arrivesAt := now.Add(time.Minute)
	state := State.NewGameState()
	state.Castles[1] = State.CastleState{ID: 1, KingdomID: 0, X: 100, Y: 100, Focused: true}
	state.EventScores.ActiveEventID = 71
	state.EventScores.ByEvent[71] = State.ScalableEventScore{EventID: 71, RemainingSec: 3_600, ObservedAt: now}
	occurrenceEndsAt := State.ScalableEventEndsAt(state.EventScores.ByEvent[71])
	state.Invasion.FortifiedTargets[State.InvasionTargetKey(0, 101, 102)] = "STO"
	state.Invasion.ReserveTarget(State.InvasionTargetReservation{
		KingdomID: 0, EventID: 71, OccurrenceEndsAt: occurrenceEndsAt,
		TargetTypeID: State.MapTypeForeignLord, X: 101, Y: 102,
		SourceCastleID: 1, CommanderID: commanderID, CommanderKnown: true,
		OperationID: "accepted-indeterminate-cra", ReservedAt: reservedAt,
	})
	state.Movements[9] = State.MovementState{
		ID: 9, Direction: 0, SourceCastleID: 1, CommanderID: &commanderID,
		KingdomID: 0, TargetTypeID: State.MapTypeForeignLord, TargetX: 101, TargetY: 102,
		StartedAt: reservedAt.Add(time.Second), ObservedAt: now, ArrivesAt: &arrivesAt, TravelSeconds: 60,
	}
	store := State.NewStore(state)
	registry := Ingest.NewRegistry()
	if err := Ingest.RegisterCoreReducers(registry); err != nil {
		t.Fatal(err)
	}
	pipeline := Ingest.NewPipeline(store, nil, registry)
	code := 0
	observed := pipeline.ObserveFrame(Protocol.Frame{
		Opcode: "hac", Direction: Protocol.DirectionInbound, ResponseCode: &code, ReceivedAt: now,
		Payload: json.RawMessage(`{"CP":[[101,102]]}`),
	})
	if _, err := pipeline.CommitFrame(t.Context(), observed); err != nil {
		t.Fatal(err)
	}
	view := store.ReadOnlyView()
	if _, reserved := view.Invasion.TargetReservation(0, 101, 102); !reserved ||
		!view.Invasion.TargetUnavailable(0, 101, 102) || len(view.Invasion.FortifiedTargets) != 0 {
		t.Fatalf("HAC did not preserve the accounting boundary: %#v", view.Invasion)
	}

	arguments, _ := json.Marshal(invasionTargetReconcileRequest{
		FocusCastleID: 1, KingdomID: 0, EventID: 71, OccurrenceEndsAt: occurrenceEndsAt,
		TargetTypeID: State.MapTypeForeignLord,
		TargetX:      101, TargetY: 102, OperationID: "accepted-indeterminate-cra", ReservedAt: reservedAt,
	})
	plan, err := planInvasionTargetReconcile(t.Context(), Intent.PlanningContext{State: view}, arguments)
	if err != nil {
		t.Fatal(err)
	}
	var verification invasionTargetReconcileRequest
	if err := json.Unmarshal(plan.Steps[len(plan.Steps)-1].ActionArguments, &verification); err != nil {
		t.Fatal(err)
	}
	_, err = store.ApplyComponents(State.Components(State.ComponentMovementSnapshot), func(current *State.GameState) ([]string, bool, error) {
		current.MovementSnapshot.ObservedAt = verification.ReconcileStartedAt.Add(time.Second)
		return []string{"movement-snapshot"}, true, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	application := &Application{State: store}
	if err := application.reconcileInvasionTargetReservation(t.Context(), plan.Steps[len(plan.Steps)-1].ActionArguments); err != nil {
		t.Fatal(err)
	}
	result := store.ReadOnlyView()
	activity, activityFound := result.LookupEventActivity(71)
	if _, reserved := result.Invasion.TargetReservation(0, 101, 102); reserved ||
		!activityFound || activity.Invasion.Launches != 1 || len(activity.PendingAttacks) != 1 ||
		len(result.AttackAnalytics.PendingAttacks) != 1 || result.AttackAnalytics.PendingAttacks[0].MovementID != 9 {
		t.Fatalf("matched reconciliation did not record exactly one launch: invasion=%#v activity=%#v analytics=%#v", result.Invasion, activity, result.AttackAnalytics)
	}
}

func TestGuardInvasionTargetForgetsUnconfirmedTarget(t *testing.T) {
	now := time.Now().UTC()
	gameState := State.NewGameState()
	gameState.Castles[1] = State.CastleState{ID: 1, KingdomID: 0, X: 100, Y: 100}
	gameState.Commanders[7] = State.CommanderState{ID: 7, Available: true}
	gameState.EventScores.ActiveEventID = 71
	gameState.EventScores.ByEvent[71] = State.ScalableEventScore{EventID: 71, RemainingSec: 3_600, ObservedAt: now}
	gameState.Map[0] = map[string]State.MapObservation{
		"101:100": {
			KingdomID: 0, TypeID: 21, X: 101, Y: 100, ObjectID: 70, Level: 70,
			InvasionAvailabilityKnown: true, ObservedAt: now,
		},
	}
	gameState.Invasion.FortifiedTargets[State.InvasionTargetKey(0, 101, 100)] = "STO"
	request := resolvedInvasionAttackRequest{
		invasionAttackRequest: invasionAttackRequest{
			SourceCastleID: 1, EventID: 71,
			EventEndsAt: State.ScalableEventEndsAt(gameState.EventScores.ByEvent[71]),
			ScoreTarget: 1_000, MinimumRemainingSec: 60,
			TargetTypeID: 21, KingdomID: 0, TargetX: 101, TargetY: 100, TargetObjectID: 70,
		},
		CommanderID: 7,
	}
	arguments, _ := json.Marshal(invasionTargetVerificationRequest{Request: request, RefreshStartedAt: now.Add(time.Second)})
	application := &Application{State: State.NewStore(gameState)}
	if err := application.guardInvasionTarget(t.Context(), arguments); err == nil {
		t.Fatal("unconfirmed invasion target passed the guard")
	}
	// The 1x1 launch-time refresh is authoritative for the tile: the phantom
	// must be gone so the immediate re-evaluation rotates instead of re-picking
	// the same vanished castle until the next full sweep.
	if _, exists := application.State.ReadOnlyView().LookupMapObservation(0, "101:100"); exists {
		t.Fatal("guard left the vanished invasion castle in map state")
	}
	if len(application.State.ReadOnlyView().Invasion.FortifiedTargets) != 0 {
		t.Fatal("guard left stale fortification on the vanished invasion castle")
	}
}

func TestForgetInvasionTargetPreservesConcurrentNewerObservation(t *testing.T) {
	now := time.Now().UTC()
	stale := State.MapObservation{
		KingdomID: 0, TypeID: State.MapTypeForeignLord, X: 101, Y: 100,
		ObjectID: 70, Level: 70, InvasionAvailabilityKnown: true, ObservedAt: now,
	}
	newer := stale
	newer.ObjectID = 71
	newer.Level = 71
	newer.ObservedAt = now.Add(time.Second)
	gameState := State.NewGameState()
	gameState.Map[0] = map[string]State.MapObservation{"101:100": newer}
	gameState.Invasion.FortifiedTargets[State.InvasionTargetKey(0, 101, 100)] = "STO"
	application := &Application{State: State.NewStore(gameState)}

	application.forgetInvasionTarget(stale)
	result := application.State.ReadOnlyView()
	retained, found := result.LookupMapObservation(0, "101:100")
	if !found || retained != newer {
		t.Fatalf("stale cleanup erased concurrent GAA observation: %#v, found=%t", retained, found)
	}
	if result.Invasion.FortifiedTargets[State.InvasionTargetKey(0, 101, 100)] != "STO" {
		t.Fatal("stale cleanup erased fortification attached to concurrent observation")
	}
}

func TestConsumeInvasionTargetPreservesNewerSameLevelReplacement(t *testing.T) {
	now := time.Now().UTC()
	replacement := State.MapObservation{
		KingdomID: 0, TypeID: State.MapTypeForeignLord, X: 101, Y: 100,
		ObjectID: 70, Level: 70, InvasionAvailabilityKnown: true, ObservedAt: now.Add(time.Second),
	}
	gameState := State.NewGameState()
	gameState.Map[0] = map[string]State.MapObservation{"101:100": replacement}
	gameState.Invasion.FortifiedTargets[State.InvasionTargetKey(0, 101, 100)] = "STO"
	gameState.Invasion.ReserveTarget(State.InvasionTargetReservation{
		KingdomID: 0, TargetTypeID: State.MapTypeForeignLord, X: 101, Y: 100,
		OperationID: "consume-newer", ReservedAt: now,
	})
	application := &Application{State: State.NewStore(gameState)}
	arguments, _ := json.Marshal(map[string]any{
		"kingdomId": 0, "targetTypeId": State.MapTypeForeignLord,
		"targetX": 101, "targetY": 100, "targetObjectId": 70,
	})
	ctx := Outbound.WithMetadata(t.Context(), Outbound.Metadata{OperationID: "consume-newer"})
	if err := application.consumeInvasionTarget(ctx, arguments); err != nil {
		t.Fatal(err)
	}
	retained := application.State.ReadOnlyView()
	if current, exists := retained.LookupMapObservation(0, "101:100"); !exists || current != replacement {
		t.Fatalf("stale consume erased replacement target: %#v, found=%t", current, exists)
	}
	if retained.Invasion.FortifiedTargets[State.InvasionTargetKey(0, 101, 100)] != "STO" {
		t.Fatal("stale consume erased replacement fortification state")
	}
	if reservation, found := retained.Invasion.TargetReservation(0, 101, 100); !found || reservation.OperationID != "consume-newer" {
		t.Fatalf("consume changed durable launch reservation: %#v", reservation)
	}
}

func TestConsumeInvasionTargetRemovesObservationAtDispatchBoundary(t *testing.T) {
	now := time.Now().UTC()
	target := State.MapObservation{
		KingdomID: 0, TypeID: State.MapTypeForeignLord, X: 101, Y: 100,
		ObjectID: 70, Level: 70, InvasionAvailabilityKnown: true, ObservedAt: now.Add(-time.Second),
	}
	gameState := State.NewGameState()
	gameState.Map[0] = map[string]State.MapObservation{"101:100": target}
	gameState.Invasion.FortifiedTargets[State.InvasionTargetKey(0, 101, 100)] = "STO"
	gameState.Invasion.ReserveTarget(State.InvasionTargetReservation{
		KingdomID: 0, TargetTypeID: State.MapTypeForeignLord, X: 101, Y: 100,
		OperationID: "consume-current", ReservedAt: now,
	})
	application := &Application{State: State.NewStore(gameState)}
	arguments, _ := json.Marshal(map[string]any{
		"kingdomId": 0, "targetTypeId": State.MapTypeForeignLord,
		"targetX": 101, "targetY": 100, "targetObjectId": 70,
	})
	ctx := Outbound.WithMetadata(t.Context(), Outbound.Metadata{OperationID: "consume-current"})
	if err := application.consumeInvasionTarget(ctx, arguments); err != nil {
		t.Fatal(err)
	}
	consumed := application.State.ReadOnlyView()
	if _, exists := consumed.LookupMapObservation(0, "101:100"); exists {
		t.Fatal("exact accepted target survived consume")
	}
	if _, exists := consumed.Invasion.FortifiedTargets[State.InvasionTargetKey(0, 101, 100)]; exists {
		t.Fatal("exact accepted target retained fortification state")
	}
}

func TestBoundedInvasionScanIsAuthoritativeForItsWindowOnly(t *testing.T) {
	now := time.Now().UTC()
	old := now.Add(-5 * time.Minute)
	gameState := State.NewGameState()
	gameState.Castles[1] = State.CastleState{ID: 1, KingdomID: 0, X: 100, Y: 100, Focused: true}
	gameState.Invasion.LastScannedAt[1] = old
	gameState.Map[0] = map[string]State.MapObservation{
		// Inside the box, NOT returned by the refresh (stale) → must be dropped.
		"105:100": {KingdomID: 0, TypeID: 21, X: 105, Y: 100, ObjectID: 70, Level: 70, InvasionAvailabilityKnown: true, ObservedAt: old},
		// Inside the box, returned by the refresh (fresh) → must survive.
		"106:101": {KingdomID: 0, TypeID: 21, X: 106, Y: 101, ObjectID: 70, Level: 70, InvasionAvailabilityKnown: true, ObservedAt: now.Add(time.Second)},
		// Outside the box and stale → untouched (its full-scan eligibility is
		// decided by the full sweep, not by a neighborhood refresh elsewhere).
		"140:140": {KingdomID: 0, TypeID: 21, X: 140, Y: 140, ObjectID: 70, Level: 70, InvasionAvailabilityKnown: true, ObservedAt: old},
	}
	gameState.Invasion.FortifiedTargets[State.InvasionTargetKey(0, 105, 100)] = "STO"
	gameState.Invasion.FortifiedTargets[State.InvasionTargetKey(0, 106, 101)] = "STO"
	gameState.Invasion.FortifiedTargets[State.InvasionTargetKey(0, 140, 140)] = "STO"
	application := &Application{State: State.NewStore(gameState)}
	arguments, _ := json.Marshal(invasionMapScanRequest{
		SourceCastleID: 1, Radius: 50, ScanStartedAt: now,
		Bounds: &State.StormMapBounds{X1: 81, Y1: 76, X2: 129, Y2: 124},
	})
	plan, err := planInvasionMapScan(t.Context(), Intent.PlanningContext{State: gameState}, arguments)
	if err != nil {
		t.Fatal(err)
	}
	gaa := 0
	for _, step := range plan.Steps {
		if step.Opcode == "gaa" {
			gaa++
			if string(step.Command.Payload) != `{"KID":0,"AX1":81,"AY1":76,"AX2":129,"AY2":124}` {
				t.Fatalf("bounded scan window payload = %s", step.Command.Payload)
			}
		}
	}
	if gaa != 1 {
		t.Fatalf("bounded scan must be exactly one gaa window, got %d", gaa)
	}
	if err := application.captureInvasionScan(t.Context(), arguments); err != nil {
		t.Fatal(err)
	}
	view := application.State.ReadOnlyView()
	if _, exists := view.LookupMapObservation(0, "105:100"); exists {
		t.Fatal("stale castle inside the refreshed window survived")
	}
	if _, exists := view.LookupMapObservation(0, "106:101"); !exists {
		t.Fatal("confirmed castle inside the refreshed window was dropped")
	}
	if _, exists := view.LookupMapObservation(0, "140:140"); !exists {
		t.Fatal("castle outside the refreshed window was dropped")
	}
	if _, exists := view.Invasion.FortifiedTargets[State.InvasionTargetKey(0, 105, 100)]; exists {
		t.Fatal("bounded refresh retained fortification for a vanished castle")
	}
	if _, exists := view.Invasion.FortifiedTargets[State.InvasionTargetKey(0, 106, 101)]; !exists {
		t.Fatal("bounded refresh cleared fortification for a confirmed castle")
	}
	if _, exists := view.Invasion.FortifiedTargets[State.InvasionTargetKey(0, 140, 140)]; !exists {
		t.Fatal("bounded refresh cleared fortification outside its window")
	}
	if !view.Invasion.LastScannedAt[1].Equal(old) {
		t.Fatalf("neighborhood refresh must not advance the full-scan clock, got %v", view.Invasion.LastScannedAt[1])
	}
}

func TestFullInvasionScanRemovesMissingInRangeTargetAndFortification(t *testing.T) {
	now := time.Now().UTC()
	old := now.Add(-5 * time.Minute)
	gameState := State.NewGameState()
	gameState.Castles[1] = State.CastleState{ID: 1, KingdomID: 0, X: 100, Y: 100, Focused: true}
	gameState.Map[0] = map[string]State.MapObservation{
		"105:100": {
			KingdomID: 0, TypeID: State.MapTypeForeignLord, X: 105, Y: 100,
			ObjectID: 70, Level: 70, InvasionAvailabilityKnown: true, ObservedAt: old,
		},
		"106:100": {
			KingdomID: 0, TypeID: State.MapTypeForeignLord, X: 106, Y: 100,
			ObjectID: 70, Level: 70, InvasionAvailabilityKnown: true, ObservedAt: now.Add(time.Second),
		},
		"151:100": {
			KingdomID: 0, TypeID: State.MapTypeForeignLord, X: 151, Y: 100,
			ObjectID: 70, Level: 70, InvasionAvailabilityKnown: true, ObservedAt: old,
		},
	}
	for _, x := range []int{105, 106, 151} {
		gameState.Invasion.FortifiedTargets[State.InvasionTargetKey(0, x, 100)] = "STO"
	}
	gameState.Invasion.FortifiedTargets[State.InvasionTargetKey(0, 107, 100)] = "STO"
	application := &Application{State: State.NewStore(gameState)}
	arguments, _ := json.Marshal(invasionMapScanRequest{
		SourceCastleID: 1, Radius: 50, ScanStartedAt: now,
	})
	if err := application.captureInvasionScan(t.Context(), arguments); err != nil {
		t.Fatal(err)
	}
	view := application.State.ReadOnlyView()
	if _, exists := view.LookupMapObservation(0, "105:100"); exists {
		t.Fatal("full scan retained a missing in-radius invasion castle")
	}
	if _, fortified := view.Invasion.FortifiedTargets[State.InvasionTargetKey(0, 105, 100)]; fortified {
		t.Fatal("full scan retained fortification for a missing invasion castle")
	}
	if _, fortified := view.Invasion.FortifiedTargets[State.InvasionTargetKey(0, 107, 100)]; fortified {
		t.Fatal("full scan retained an orphaned in-radius fortification marker")
	}
	if _, exists := view.LookupMapObservation(0, "106:100"); !exists {
		t.Fatal("full scan removed a freshly confirmed in-radius invasion castle")
	}
	if _, fortified := view.Invasion.FortifiedTargets[State.InvasionTargetKey(0, 106, 100)]; !fortified {
		t.Fatal("full scan cleared fortification for a confirmed invasion castle")
	}
	if _, exists := view.LookupMapObservation(0, "151:100"); !exists {
		t.Fatal("full scan removed a stale invasion castle outside its radius")
	}
	if _, fortified := view.Invasion.FortifiedTargets[State.InvasionTargetKey(0, 151, 100)]; !fortified {
		t.Fatal("full scan cleared fortification outside its radius")
	}
	if !view.Invasion.LastScannedAt[1].Equal(now) {
		t.Fatalf("full scan clock = %v, want %v", view.Invasion.LastScannedAt[1], now)
	}

	_, err := application.State.ApplyComponents(
		State.Components(State.ComponentWorldMap),
		func(state *State.GameState) ([]string, bool, error) {
			changed := state.SetMapObservation(State.MapObservation{
				KingdomID: 0, TypeID: State.MapTypeForeignLord, X: 105, Y: 100,
				ObjectID: 80, Level: 80, InvasionAvailabilityKnown: true, ObservedAt: now.Add(2 * time.Second),
			})
			return []string{"map-invasion"}, changed, nil
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	view = application.State.ReadOnlyView()
	if _, exists := view.LookupMapObservation(0, "105:100"); !exists {
		t.Fatal("same-coordinate invasion castle did not reappear")
	}
	if _, fortified := view.Invasion.FortifiedTargets[State.InvasionTargetKey(0, 105, 100)]; fortified {
		t.Fatal("same-coordinate reappearance inherited stale fortification")
	}
}
