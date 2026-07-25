package App

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"CitadelDesktop/Server/Intent"
	"CitadelDesktop/Server/Protocol"
	"CitadelDesktop/Server/State"
)

func TestConstructionShopContextMatchesCapturedWireSequence(t *testing.T) {
	castle := State.CastleState{ID: 10, KingdomID: 2}
	steps := constructionShopContextSteps(castle)
	if len(steps) != 2 || steps[0].Opcode != "aec" || steps[1].Opcode != "gbc" {
		t.Fatalf("construction context = %#v", steps)
	}
	for _, step := range steps {
		if step.ResumePolicy != Intent.ResumeRebuild {
			t.Fatalf("context step %q is not rebuilt on resume", step.Name)
		}
	}
	steps[0].Command.Namespace = "EmpireEx_21"
	wire, err := Protocol.Encode(steps[0].Command)
	if err != nil {
		t.Fatal(err)
	}
	if string(wire) != "%xt%EmpireEx_21%aec%1%{}%" {
		t.Fatalf("aec wire = %s", wire)
	}
	if payload := string(steps[1].Command.Payload); payload != `{"CID":10,"KID":2}` {
		t.Fatalf("gbc payload = %s", payload)
	}
}

func TestStationRouteContextIsFullyRestartable(t *testing.T) {
	source := State.CastleState{X: 10, Y: 20}
	target := State.AllianceHolding{X: 30, Y: 40}
	steps := stationRouteContextSteps(source, target)
	if len(steps) != 1 || steps[0].Opcode != "sdi" || steps[0].AwaitOpcode != "sdi" {
		t.Fatalf("station context = %#v", steps)
	}
	if steps[0].ResumePolicy != Intent.ResumeRebuild {
		t.Fatalf("station context is not restartable: %#v", steps)
	}
}

func TestCastleContextSkipsJAAWhenTargetIsAlreadyFocused(t *testing.T) {
	castle := State.CastleState{ID: 10, KingdomID: 2, X: 12, Y: 34, Focused: true}
	if steps := castleContextSteps(castle); len(steps) != 0 {
		t.Fatalf("focused castle context = %#v, want no JAA", steps)
	}

	castle.Focused = false
	steps := castleContextSteps(castle)
	if len(steps) != 1 || steps[0].Opcode != "jaa" {
		t.Fatalf("unfocused castle context = %#v, want one JAA", steps)
	}
}

func TestPlanCastleFocusTreatsCurrentTargetAsSatisfied(t *testing.T) {
	gameState := State.NewGameState()
	gameState.Castles[10] = State.CastleState{ID: 10, Focused: true}
	plan, err := planCastleFocus(
		context.Background(), Intent.PlanningContext{State: gameState}, json.RawMessage(`{"castleId":10}`),
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Steps) != 0 {
		t.Fatalf("focused castle plan = %#v, want no JAA", plan.Steps)
	}
}

func TestAttackCastleContextUsesJAAForSelectionAndJCAForRefocus(t *testing.T) {
	focused := attackCastleContextStep(State.CastleState{ID: 10, KingdomID: 2, X: 12, Y: 34, Focused: true})
	if focused.Opcode != "jca" || focused.AwaitOpcode != "jaa" || string(focused.Command.Payload) != `{"CID":10,"KID":2}` {
		t.Fatalf("focused attack context = %#v", focused)
	}
	if focused.ResponseBarrier != Intent.ResponseBarrierCommitted {
		t.Fatalf("focused attack context barrier = %q, want committed", focused.ResponseBarrier)
	}

	selected := attackCastleContextStep(State.CastleState{ID: 10, KingdomID: 2, X: 12, Y: 34})
	if selected.Opcode != "jaa" || selected.AwaitOpcode != "jaa" || string(selected.Command.Payload) != `{"PX":12,"PY":34,"KID":2}` {
		t.Fatalf("selected attack context = %#v", selected)
	}
	if selected.ResponseBarrier != Intent.ResponseBarrierCommitted {
		t.Fatalf("selected attack context barrier = %q, want committed", selected.ResponseBarrier)
	}
}

func TestCRASetupContextRefreshesWorldMapAttackDialogThenPresets(t *testing.T) {
	steps, err := craSetupContextSteps([]byte(`{"SX":12,"SY":34,"TX":56,"TY":78,"KID":2}`))
	if err != nil {
		t.Fatal(err)
	}
	if len(steps) != 4 || steps[0].Action != "game.ui.close" || steps[1].Opcode != "gbl" ||
		steps[2].Opcode != "adi" || steps[3].Opcode != "gas" {
		t.Fatalf("CRA setup context = %#v", steps)
	}
	for _, step := range steps {
		if step.ResumePolicy != Intent.ResumeRebuild {
			t.Fatalf("context step %q is not rebuilt on resume", step.Name)
		}
	}
	if string(steps[1].Command.Payload) != `{}` {
		t.Fatalf("gbl payload = %s", steps[1].Command.Payload)
	}
	if string(steps[2].Command.Payload) != `{"SX":12,"SY":34,"TX":56,"TY":78,"KID":2}` {
		t.Fatalf("adi payload = %s", steps[2].Command.Payload)
	}
	if string(steps[3].Command.Payload) != `{}` {
		t.Fatalf("gas payload = %s", steps[3].Command.Payload)
	}
}

func TestCRACommandDependenciesOwnSetupAndAuthoritativeGuard(t *testing.T) {
	payload := json.RawMessage(`{"SX":12,"SY":34,"TX":56,"TY":78,"KID":2}`)
	application := &Application{}
	dependencies, err := application.resolveCRACommandDependencies(
		t.Context(), Intent.PlanningContext{}, Intent.Step{Command: Protocol.Command{Opcode: "cra", Payload: payload}},
	)
	if err != nil {
		t.Fatal(err)
	}
	if dependencies.Key != "2:12:34:56:78" || len(dependencies.Steps) != 5 ||
		dependencies.Steps[0].Action != "game.ui.close" || dependencies.Steps[1].Opcode != "gbl" ||
		dependencies.Steps[2].Opcode != "adi" || dependencies.Steps[3].Opcode != "gas" ||
		dependencies.Steps[4].Action != "attack.cra.send.guard" {
		t.Fatalf("CRA command dependencies = %#v", dependencies)
	}
}

func TestTowerCRACommandDependenciesRefreshMovementsBeforeSetup(t *testing.T) {
	state := State.NewGameState()
	state.Map[2] = map[string]State.MapObservation{
		"56:78": {KingdomID: 2, TypeID: kingdomTowerMapTypeID, X: 56, Y: 78},
	}
	payload := json.RawMessage(`{"SX":12,"SY":34,"TX":56,"TY":78,"KID":2,"LID":17}`)
	dependencies, err := (&Application{}).resolveCRACommandDependencies(
		t.Context(), Intent.PlanningContext{State: state},
		Intent.Step{Command: Protocol.Command{Opcode: "cra", Payload: payload}},
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(dependencies.Steps) != 6 || dependencies.Steps[0].Opcode != "gam" ||
		dependencies.Steps[0].ResponseBarrier != Intent.ResponseBarrierCommitted ||
		dependencies.Steps[1].Action != "game.ui.close" || dependencies.Steps[5].Action != "attack.cra.send.guard" {
		t.Fatalf("tower CRA dependencies = %#v", dependencies.Steps)
	}
	var guard craSendGuardRequest
	if err := json.Unmarshal(dependencies.Steps[5].ActionArguments, &guard); err != nil {
		t.Fatal(err)
	}
	if guard.CommanderID == nil || *guard.CommanderID != 17 || guard.MovementsObservedAfter.IsZero() {
		t.Fatalf("tower CRA guard = %#v", guard)
	}
}

func TestDeferredCRACommandDependenciesResolveCommanderAndRefreshMovements(t *testing.T) {
	payload := json.RawMessage(`{"SX":12,"SY":34,"TX":56,"TY":78,"KID":0}`)
	dependencies, err := (&Application{}).resolveCRACommandDependencies(
		t.Context(), Intent.PlanningContext{State: State.NewGameState()},
		Intent.Step{
			Command:           Protocol.Command{Opcode: "cra", Payload: payload},
			ResolverArguments: json.RawMessage(`{"commanderId":7}`),
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(dependencies.Steps) != 6 || dependencies.Steps[0].Opcode != "gam" ||
		dependencies.Steps[5].Action != "attack.cra.send.guard" {
		t.Fatalf("deferred CRA dependencies = %#v", dependencies.Steps)
	}
	var guard craSendGuardRequest
	if err := json.Unmarshal(dependencies.Steps[5].ActionArguments, &guard); err != nil {
		t.Fatal(err)
	}
	if guard.CommanderID == nil || *guard.CommanderID != 7 || guard.MovementsObservedAfter.IsZero() {
		t.Fatalf("deferred CRA guard = %#v", guard)
	}
}

func TestCRACommandDependenciesRejectSettlingTowerBeforeADI(t *testing.T) {
	now := time.Now().UTC()
	state := State.NewGameState()
	state.AttackAnalytics.PendingAttacks = []State.AttackFeatureLaunch{{
		MovementID: 10, FeatureID: State.AttackFeatureAutoTowers, KingdomID: 2,
		TargetTypeID: kingdomTowerMapTypeID, TargetX: 56, TargetY: 78,
		LaunchedAt: now.Add(-5 * time.Minute), ArrivesAt: now.Add(-time.Second),
	}}
	payload := json.RawMessage(`{"SX":12,"SY":34,"TX":56,"TY":78,"KID":2}`)
	_, err := (&Application{}).resolveCRACommandDependencies(
		t.Context(), Intent.PlanningContext{State: state},
		Intent.Step{Command: Protocol.Command{Opcode: "cra", Payload: payload}},
	)
	if !errors.Is(err, Intent.ErrPlanStale) {
		t.Fatalf("settling tower dependency error = %v, want stale plan before ADI", err)
	}
}

func TestCRASendGuardRejectsPendingOrPositiveCooldown(t *testing.T) {
	now := time.Now().UTC()
	state := State.NewGameState()
	state.Castles[1] = State.CastleState{ID: 1, KingdomID: 0, X: 12, Y: 34}
	state.AttackDialog = State.AttackDialogState{
		SourceCastleID: 1, KingdomID: 0, ObservedAt: now.Add(time.Second),
		Target: State.AttackDialogTarget{TypeID: kingdomTowerMapTypeID, X: 56, Y: 78},
	}
	state.Map[0] = map[string]State.MapObservation{
		"56:78": {KingdomID: 0, TypeID: kingdomTowerMapTypeID, X: 56, Y: 78, ObservedAt: now},
	}
	commanderID := State.CommanderID(17)
	state.Commanders[commanderID] = State.CommanderState{ID: commanderID, Available: true}
	state.MovementSnapshot = State.MovementSnapshot{Version: 1, ObservedAt: now.Add(time.Second)}
	state.TowerCooldowns["0:56:78"] = State.TowerCooldownState{
		KingdomID: 0, X: 56, Y: 78, LastSuccessfulBattleAt: now, PendingCooldownRefresh: true,
	}
	application := &Application{State: State.NewStore(state)}
	arguments, _ := json.Marshal(craSendGuardRequest{
		SourceX: 12, SourceY: 34, TargetX: 56, TargetY: 78, KingdomID: 0, CommanderID: &commanderID,
		DialogObservedAt: now, MovementsObservedAfter: now,
	})
	if err := application.guardCRASend(context.Background(), arguments); err == nil {
		t.Fatal("CRA send guard accepted a target awaiting its post-victory cooldown refresh")
	}
	_, _ = application.State.Apply(func(current *State.GameState) ([]string, bool, error) {
		cooldown := current.TowerCooldowns["0:56:78"]
		cooldown.PendingCooldownRefresh = false
		current.TowerCooldowns["0:56:78"] = cooldown
		current.AttackDialog.Target.TowerCooldownRemaining = 300
		return []string{"attack_dialog", "tower-cooldowns"}, true, nil
	})
	if err := application.guardCRASend(context.Background(), arguments); err == nil {
		t.Fatal("CRA send guard accepted a positive authoritative cooldown")
	}
	_, _ = application.State.Apply(func(current *State.GameState) ([]string, bool, error) {
		current.AttackDialog.Target.TowerCooldownRemaining = 0
		return []string{"attack_dialog"}, true, nil
	})
	if err := application.guardCRASend(context.Background(), arguments); err != nil {
		t.Fatalf("CRA send guard rejected a freshly confirmed ready target: %v", err)
	}
}

func TestCRASendGuardTreatsKhanCooldownAsRetryableStaleState(t *testing.T) {
	now := time.Now().UTC()
	commanderID := State.CommanderID(17)
	state := State.NewGameState()
	state.Castles[1] = State.CastleState{ID: 1, KingdomID: 0, X: 12, Y: 34}
	state.Commanders[commanderID] = State.CommanderState{ID: commanderID, Available: true}
	state.MovementSnapshot = State.MovementSnapshot{Version: 1, ObservedAt: now.Add(time.Second)}
	state.AttackDialog = State.AttackDialogState{
		SourceCastleID: 1, KingdomID: 0, ObservedAt: now.Add(time.Second),
		Target: State.AttackDialogTarget{
			TypeID: khanCampTypeID, X: 56, Y: 78, EventCampID: 1146, EventCampCooldownRemaining: 194,
		},
	}
	state.Map[0] = map[string]State.MapObservation{
		"56:78": {
			KingdomID: 0, TypeID: khanCampTypeID, X: 56, Y: 78,
			EventCampID: 1146, EventCampCooldownRemaining: 194, ObservedAt: now.Add(time.Second),
		},
	}
	application := &Application{State: State.NewStore(state)}
	arguments, _ := json.Marshal(craSendGuardRequest{
		SourceX: 12, SourceY: 34, TargetX: 56, TargetY: 78, KingdomID: 0, CommanderID: &commanderID,
		DialogObservedAt: now, MovementsObservedAfter: now,
	})
	if err := application.guardCRASend(t.Context(), arguments); !errors.Is(err, Intent.ErrPlanStale) {
		t.Fatalf("Khan cooldown guard error = %v, want retryable stale state", err)
	}
}

func TestCRASendGuardTreatsPendingKhanLandingAsRetryableStaleState(t *testing.T) {
	now := time.Now().UTC()
	commanderID := State.CommanderID(17)
	state := State.NewGameState()
	state.Castles[1] = State.CastleState{ID: 1, KingdomID: 0, X: 12, Y: 34}
	state.Commanders[commanderID] = State.CommanderState{ID: commanderID, Available: true}
	state.MovementSnapshot = State.MovementSnapshot{Version: 1, ObservedAt: now.Add(time.Second)}
	state.AttackDialog = State.AttackDialogState{
		SourceCastleID: 1, KingdomID: 0, ObservedAt: now.Add(time.Second),
		Target: State.AttackDialogTarget{
			TypeID: khanCampTypeID, X: 56, Y: 78, EventCampID: 1146,
		},
	}
	state.Map[0] = map[string]State.MapObservation{
		"56:78": {
			KingdomID: 0, TypeID: khanCampTypeID, X: 56, Y: 78,
			EventCampID: 1146, ObservedAt: now.Add(time.Second),
		},
	}
	state.NomadCamps.Cooldowns["0:56:78"] = State.NomadCampCooldownState{
		KingdomID: 0, X: 56, Y: 78, LastSuccessfulBattleAt: now, PendingCooldownRefresh: true,
	}
	application := &Application{State: State.NewStore(state)}
	arguments, _ := json.Marshal(craSendGuardRequest{
		SourceX: 12, SourceY: 34, TargetX: 56, TargetY: 78, KingdomID: 0, CommanderID: &commanderID,
		DialogObservedAt: now, MovementsObservedAfter: now,
	})
	if err := application.guardCRASend(t.Context(), arguments); !errors.Is(err, Intent.ErrPlanStale) {
		t.Fatalf("pending Khan landing guard error = %v, want retryable stale state", err)
	}
}

func TestCRASendGuardRejectsTowerCommanderLostAfterFreshMovementSnapshot(t *testing.T) {
	now := time.Now().UTC()
	commanderID := State.CommanderID(17)
	state := State.NewGameState()
	state.Castles[1] = State.CastleState{ID: 1, KingdomID: 0, X: 12, Y: 34}
	state.Commanders[commanderID] = State.CommanderState{ID: commanderID, Available: false}
	state.MovementSnapshot = State.MovementSnapshot{Version: 2, ObservedAt: now.Add(time.Second)}
	state.AttackDialog = State.AttackDialogState{
		SourceCastleID: 1, KingdomID: 0, ObservedAt: now.Add(time.Second),
		Target: State.AttackDialogTarget{TypeID: kingdomTowerMapTypeID, X: 56, Y: 78},
	}
	state.Map[0] = map[string]State.MapObservation{
		"56:78": {KingdomID: 0, TypeID: kingdomTowerMapTypeID, X: 56, Y: 78, ObservedAt: now},
	}
	application := &Application{State: State.NewStore(state)}
	arguments, _ := json.Marshal(craSendGuardRequest{
		SourceX: 12, SourceY: 34, TargetX: 56, TargetY: 78, KingdomID: 0, CommanderID: &commanderID,
		DialogObservedAt: now, MovementsObservedAfter: now,
	})
	if err := application.guardCRASend(t.Context(), arguments); err == nil || !strings.Contains(err.Error(), "no longer available") {
		t.Fatalf("tower commander guard error = %v", err)
	}
}

func TestCRASendGuardRejectsActiveMovementWhenRosterSaysAvailable(t *testing.T) {
	now := time.Now().UTC()
	arrivesAt := now.Add(10 * time.Minute)
	commanderID := State.CommanderID(7)
	state := State.NewGameState()
	state.Player.ID = 1
	state.Castles[1] = State.CastleState{ID: 1, KingdomID: 0, X: 12, Y: 34}
	state.Commanders[commanderID] = State.CommanderState{ID: commanderID, Available: true}
	state.Movements[50] = State.MovementState{
		ID: 50, Direction: 0, OwnerPlayerID: 1, SourceCastleID: 1,
		CommanderID: &commanderID, ArrivesAt: &arrivesAt,
	}
	state.MovementSnapshot = State.MovementSnapshot{Version: 2, ObservedAt: now.Add(time.Second)}
	state.AttackDialog = State.AttackDialogState{
		SourceCastleID: 1, KingdomID: 0, ObservedAt: now.Add(time.Second),
		Target: State.AttackDialogTarget{TypeID: 34, X: 56, Y: 78},
	}
	application := &Application{State: State.NewStore(state)}
	arguments, _ := json.Marshal(craSendGuardRequest{
		SourceX: 12, SourceY: 34, TargetX: 56, TargetY: 78, KingdomID: 0, CommanderID: &commanderID,
		DialogObservedAt: now, MovementsObservedAfter: now,
	})
	if err := application.guardCRASend(t.Context(), arguments); !errors.Is(err, Intent.ErrPlanStale) {
		t.Fatalf("active commander guard error = %v, want stale plan", err)
	}
}
