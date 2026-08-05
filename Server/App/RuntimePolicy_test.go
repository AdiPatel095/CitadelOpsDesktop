package App

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"CitadelDesktop/Server/Configuration"
	"CitadelDesktop/Server/GameData"
	"CitadelDesktop/Server/Intent"
	"CitadelDesktop/Server/Outbound"
	"CitadelDesktop/Server/Reports"
	"CitadelDesktop/Server/Session"
	"CitadelDesktop/Server/State"
)

func TestRuntimeSchedulerSettingsDriveHarnessPolicies(t *testing.T) {
	configuration, err := Configuration.Open(t.TempDir(), map[string]json.RawMessage{
		"scheduler":         json.RawMessage(`{"minAttackDelay":4.5,"maxAttackDelay":4.5,"botLocked":true,"attackPriorities":{"autoTowers":82}}`),
		"session.reconnect": json.RawMessage(`{"relogDelaySec":420}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	application := &Application{Configuration: configuration}
	if delay := application.attackLaunchDelay(); delay != 4500*time.Millisecond {
		t.Fatalf("attack delay = %s", delay)
	}
	if !application.automationLocked() {
		t.Fatal("bot lock setting was not applied")
	}
	if delay := application.relogDelay(); delay != 7*time.Minute {
		t.Fatalf("relog delay = %s", delay)
	}
	if weight := application.attackAdmissionWeight(Intent.Request{}, Intent.Admission{Module: "autoTowers"}); weight != 82 {
		t.Fatalf("Auto Towers admission weight = %d", weight)
	}
	if weight := application.attackAdmissionWeight(Intent.Request{}, Intent.Admission{Module: "futureModule"}); weight != 50 {
		t.Fatalf("default admission weight = %d", weight)
	}
}

func TestRuntimeRelogDelayDefaultsAndClamps(t *testing.T) {
	if delay := (&Application{}).relogDelay(); delay != 5*time.Minute {
		t.Fatalf("default relog delay = %s", delay)
	}
	configuration, err := Configuration.Open(t.TempDir(), map[string]json.RawMessage{
		"session.reconnect": json.RawMessage(`{"relogDelaySec":1}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	if delay := (&Application{Configuration: configuration}).relogDelay(); delay != time.Minute {
		t.Fatalf("minimum relog delay = %s", delay)
	}
}

func TestExecutionGateYieldsAutomatedWorkWhileBotIsLocked(t *testing.T) {
	controller := Session.NewController(context.Background(), Session.NewUnavailableTransport(), nil, nil)
	controller.SetAutomationLocked(true)
	application := &Application{Session: controller}
	plan := Intent.Plan{Claims: []string{"game:equipment"}}

	start := time.Now()
	if err := application.executionGate(
		context.Background(), Intent.Request{Actor: "ui"}, plan, Intent.ExecutionBeforeClaims,
	); err != nil {
		t.Fatal(err)
	}
	if elapsed := time.Since(start); elapsed > 20*time.Millisecond {
		t.Fatalf("manual UI intent was delayed by %s", elapsed)
	}

	if err := application.executionGate(
		context.Background(), Intent.Request{Actor: "automation:autoRecruit"}, plan, Intent.ExecutionBeforeStep,
	); !errors.Is(err, Outbound.ErrAutomationLocked) {
		t.Fatalf("active automation gate error = %v", err)
	}

	start = time.Now()
	go func() {
		time.Sleep(35 * time.Millisecond)
		controller.SetAutomationLocked(false)
	}()
	if err := application.executionGate(
		context.Background(), Intent.Request{Actor: "automation:autoRecruit"}, plan, Intent.ExecutionBeforeClaims,
	); err != nil {
		t.Fatal(err)
	}
	if elapsed := time.Since(start); elapsed < 20*time.Millisecond {
		t.Fatalf("background automation ignored bot lock after %s", elapsed)
	}
}

func TestExecutionGateFailsWritesClosedWhenPersistenceIsUnavailable(t *testing.T) {
	application := &Application{statePersistenceErr: errors.New("disk full")}
	if err := application.executionGate(
		context.Background(), Intent.Request{Actor: "ui"}, Intent.Plan{Effect: Intent.EffectWrite},
		Intent.ExecutionBeforeClaims,
	); err == nil || !strings.Contains(err.Error(), "durable storage is unavailable") {
		t.Fatalf("write gate error = %v", err)
	}
	if err := application.executionGate(
		context.Background(), Intent.Request{Actor: "ui"}, Intent.Plan{Effect: Intent.EffectRead},
		Intent.ExecutionBeforeClaims,
	); err != nil {
		t.Fatalf("read was blocked by persistence health: %v", err)
	}
}

func TestExecutionGateDoesNotBlockWritesForReportPersistenceFailure(t *testing.T) {
	store, err := Reports.OpenSQLiteStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	err = store.Save(t.Context(), Reports.BattleReport{
		ID: "10-20", ReportID: "10-20", AccountUID: 44, PlayerID: 1,
		MID: 10, LID: 20, OccurredAt: time.Now().UTC().Format(time.RFC3339Nano),
		Attacker: &Reports.BattleCombatant{PlayerID: 1},
		Defender: &Reports.BattleCombatant{PlayerID: -2, Dummy: true},
	})
	if err == nil {
		t.Fatal("closed report store did not record a persistence failure")
	}
	application := &Application{ReportStore: store}
	if err := application.PersistenceError(); err == nil || !strings.Contains(err.Error(), "report analytics persistence") {
		t.Fatalf("combined persistence health = %v", err)
	}
	if err := application.executionGate(
		context.Background(), Intent.Request{Actor: "ui"}, Intent.Plan{Effect: Intent.EffectWrite},
		Intent.ExecutionBeforeClaims,
	); err != nil {
		t.Fatalf("report analytics failure blocked a game write: %v", err)
	}
}

func TestExecutionGateRechecksAutoBeriGallantryBooster(t *testing.T) {
	configuration, err := Configuration.Open(t.TempDir(), map[string]json.RawMessage{
		"automation.autoBeriWorld": json.RawMessage(`{"requireActiveGallantryBooster":true}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	gameState := State.NewGameState()
	gameState.Market.BoostersObservedAt = time.Now().UTC()
	application := &Application{Configuration: configuration, State: State.NewStore(gameState)}
	request := Intent.Request{Actor: "automation:autoBeriWorld"}
	plan := Intent.Plan{Effect: Intent.EffectRead}

	err = application.executionGate(t.Context(), request, plan, Intent.ExecutionBeforeStep)
	if !errors.Is(err, Intent.ErrPlanStale) || !strings.Contains(err.Error(), "boi ID 24") {
		t.Fatalf("inactive Auto Beri gate error = %v", err)
	}
	if err := application.executionGate(
		t.Context(), Intent.Request{Actor: "ui"}, plan, Intent.ExecutionBeforeStep,
	); err != nil {
		t.Fatalf("manual work was blocked by Auto Beri booster gate: %v", err)
	}

	_, err = application.State.Apply(func(state *State.GameState) ([]string, bool, error) {
		state.Market.Boosters[GameData.GallantryPointsBoosterID] = State.MarketBoosterState{
			ID:           GameData.GallantryPointsBoosterID,
			BonusPercent: 400,
			ExpiresAt:    time.Now().UTC().Add(3 * time.Hour),
		}
		return []string{"boosters"}, true, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := application.executionGate(t.Context(), request, plan, Intent.ExecutionBeforeStep); err != nil {
		t.Fatalf("active Gallantry booster was blocked: %v", err)
	}
}

func TestExecutionGateHonorsExplicitCommanderAssignmentsForAttackModules(t *testing.T) {
	configuration, err := Configuration.Open(t.TempDir(), map[string]json.RawMessage{
		commanderFeatureSection: json.RawMessage(`{"version":1,"assignments":{"autoStorm":[16,17]}}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	application := &Application{Configuration: configuration}
	plan := Intent.Plan{
		Effect:    Intent.EffectLaunch,
		Claims:    []string{"commander:16", "leader:commander:16"},
		Admission: &Intent.Admission{Class: Intent.AdmissionAttackLaunch, Module: "autoStorm"},
	}
	if err := application.executionGate(
		context.Background(), Intent.Request{Actor: "automation:autoStorm"}, plan, Intent.ExecutionBeforeClaims,
	); err != nil {
		t.Fatalf("assigned commander was blocked: %v", err)
	}

	plan.Claims = []string{"commander:0", "leader:commander:0"}
	if err := application.executionGate(
		context.Background(), Intent.Request{Actor: "automation:autoStorm"}, plan, Intent.ExecutionBeforeClaims,
	); err == nil || !strings.Contains(err.Error(), "commander 0 is not assigned to autoStorm") {
		t.Fatalf("unassigned commander gate error = %v", err)
	}

	plan.Admission.Module = "autoTowers"
	if err := application.executionGate(
		context.Background(), Intent.Request{Actor: "automation:autoTowers"}, plan, Intent.ExecutionBeforeClaims,
	); err != nil {
		t.Fatalf("default all-commander assignment was blocked: %v", err)
	}
}

func TestExecutionGateEnforcesCommanderEquipmentRequirements(t *testing.T) {
	configuration, err := Configuration.Open(t.TempDir(), map[string]json.RawMessage{
		commanderFeatureSection: json.RawMessage(`{
			"version":2,
			"assignments":{},
			"requirements":{"autoStorm":[{
				"kind":"equipmentEffect",
				"effectDefinitionId":22012,
				"unitId":195,
				"minimumValue":18
			}]}
		}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	gameState := State.NewGameState()
	gameState.Commanders[16] = State.CommanderState{
		ID: 16, Equipment: map[string]State.EquipmentInstanceID{"1": 1001},
	}
	gameState.Inventory.Equipment[1001] = State.EquipmentInstance{
		ID: 1001, Effects: State.EquipmentEffects{{
			DefinitionID: 22012, Values: []float64{195, 16},
		}},
	}
	application := &Application{Configuration: configuration, State: State.NewStore(gameState)}
	plan := Intent.Plan{
		Effect:    Intent.EffectLaunch,
		Claims:    []string{"commander:16", "leader:commander:16"},
		Admission: &Intent.Admission{Class: Intent.AdmissionAttackLaunch, Module: "autoStorm"},
	}
	if err := application.executionGate(
		context.Background(), Intent.Request{Actor: "automation:autoStorm"}, plan, Intent.ExecutionBeforeClaims,
	); err == nil || !strings.Contains(err.Error(), "does not meet the autoStorm equipment requirement") {
		t.Fatalf("equipment requirement gate error = %v", err)
	}
}
