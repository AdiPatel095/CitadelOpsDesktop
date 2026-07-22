package App

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"CitadelDesktop/Server/GameData"
	"CitadelDesktop/Server/Intent"
	"CitadelDesktop/Server/State"
)

func TestTowerAttackWaitsForAdmissionThenResolvesFullFlanksFromFreshContext(t *testing.T) {
	gameData, err := GameData.DecodeStore([]byte(`{
		"versionInfo":[],"units":[],"buildings":[],
		"effects":[{"effectID":2108,"name":"relicAttackUnitAmountFlank","effectTypeID":28,"capID":1008}]
	}`), GameData.SourceMetadata{ItemVersion: "test"})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	gameState := State.NewGameState()
	gameState.Castles[1] = State.CastleState{
		ID: 1, KingdomID: 0, X: 100, Y: 100,
		Units: State.CastleUnits{Stationed: map[State.UnitID]int64{77: 2_000}},
	}
	gameState.Commanders[5] = State.CommanderState{
		ID: 5, Available: true, Equipment: map[string]State.EquipmentInstanceID{"1": 5001},
	}
	gameState.Inventory.Equipment[5001] = State.EquipmentInstance{
		ID: 5001, Effects: State.EquipmentEffects{{DefinitionID: 2108, Values: []float64{45}}},
	}
	gameState.Map[0] = map[string]State.MapObservation{}
	gameState.Map[0]["101:100"] = State.MapObservation{
		KingdomID: 0, X: 101, Y: 100, TypeID: kingdomTowerMapTypeID,
		TowerVictoryCount: 845, Level: 81, ObservedAt: now,
	}
	arguments := json.RawMessage(`{"sourceCastleId":1,"kingdomId":0,"targetX":101,"targetY":100,"unitId":77}`)

	attackPlan, err := planTowerAttack(context.Background(), Intent.PlanningContext{State: gameState, GameData: gameData}, arguments)
	if err != nil {
		t.Fatal(err)
	}
	if attackPlan.Admission == nil || attackPlan.Admission.Class != Intent.AdmissionAttackLaunch || attackPlan.Admission.Module != "autoTowers" {
		t.Fatalf("unexpected tower admission: %#v", attackPlan.Admission)
	}
	if len(attackPlan.Steps) != 5 || attackPlan.Steps[0].Action != "tower.queue.defer" ||
		attackPlan.Steps[1].Opcode != "jaa" || attackPlan.Steps[1].AwaitOpcode != "jaa" ||
		attackPlan.Steps[2].Resolver != "tower.attack.build" ||
		attackPlan.Steps[2].CommandDependencies == nil || attackPlan.Steps[2].CommandDependencies.Opcode != "cra" ||
		attackPlan.Steps[3].Action != "attack.analytics.capture" || attackPlan.Steps[4].Action != "tower.queue.consume" {
		t.Fatalf("unexpected atomic tower plan: %#v", attackPlan.Steps)
	}
	var dependencyRoute struct {
		CommanderID          State.CommanderID `json:"LID"`
		TowerCapacityCapture json.RawMessage   `json:"towerCapacityCapture"`
	}
	if err := json.Unmarshal(attackPlan.Steps[2].CommandDependencies.Payload, &dependencyRoute); err != nil {
		t.Fatal(err)
	}
	if dependencyRoute.CommanderID != 5 || len(dependencyRoute.TowerCapacityCapture) == 0 {
		t.Fatalf("tower CRA dependency route = %#v", dependencyRoute)
	}

	gameState.AttackDialog = State.AttackDialogState{
		SourceCastleID: 1, KingdomID: 0,
		Target:     State.AttackDialogTarget{TypeID: kingdomTowerMapTypeID, X: 101, Y: 100},
		ObservedAt: now,
	}
	resolved, err := (&Application{}).resolveTowerAttackStep(
		context.Background(), Intent.PlanningContext{State: gameState, GameData: gameData}, attackPlan.Steps[2].ResolverArguments,
	)
	if err != nil {
		t.Fatal(err)
	}
	if resolved.Opcode != "cra" || resolved.AwaitOpcode != "cra" {
		t.Fatalf("unexpected resolved tower launch: %#v", resolved)
	}
	var body struct {
		CommanderID State.CommanderID `json:"LID"`
		Waves       []attackWave      `json:"A"`
	}
	if err := json.Unmarshal(resolved.Command.Payload, &body); err != nil {
		t.Fatal(err)
	}
	if body.CommanderID != 5 || len(body.Waves) != 1 {
		t.Fatalf("unexpected tower attack body: %#v", body)
	}
	wave := body.Waves[0]
	if wave.Left.Units[0] != (attackPair{77, 93}) || wave.Right.Units[0] != (attackPair{77, 93}) {
		t.Fatalf("tower attack did not fill both flanks: %#v", wave)
	}
	if wave.Middle.Units[0] != (attackPair{-1, 0}) {
		t.Fatalf("tower attack should leave the center empty: %#v", wave.Middle)
	}
}

func TestTowerLaunchUsesFreshAtomicAttackContext(t *testing.T) {
	gameData, err := GameData.DecodeStore([]byte(`{
		"versionInfo":[],"units":[],"buildings":[],"effects":[]
	}`), GameData.SourceMetadata{ItemVersion: "test"})
	if err != nil {
		t.Fatal(err)
	}
	gameState := State.NewGameState()
	gameState.Castles[1] = State.CastleState{
		ID: 1, KingdomID: 0, X: 100, Y: 100,
		Units: State.CastleUnits{Stationed: map[State.UnitID]int64{77: 2_000}},
	}
	gameState.Commanders[5] = State.CommanderState{ID: 5, Available: true}
	gameState.Map[0] = map[string]State.MapObservation{}
	gameState.Map[0]["101:100"] = State.MapObservation{
		KingdomID: 0, X: 101, Y: 100, TypeID: kingdomTowerMapTypeID, TowerVictoryCount: 845, Level: 81,
	}
	gameState.AttackDialog = State.AttackDialogState{
		SourceCastleID: 1, KingdomID: 0,
		Target: State.AttackDialogTarget{TypeID: kingdomTowerMapTypeID, X: 102, Y: 100},
	}
	plan, err := planTowerLaunch(context.Background(), Intent.PlanningContext{State: gameState, GameData: gameData}, json.RawMessage(`{"sourceCastleId":1,"kingdomId":0,"targetX":101,"targetY":100,"unitId":77}`))
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Steps) != 5 || plan.Steps[0].Action != "tower.queue.defer" || plan.Steps[1].Opcode != "jaa" ||
		plan.Steps[2].Resolver != "tower.attack.build" ||
		plan.Steps[2].CommandDependencies == nil || plan.Steps[2].CommandDependencies.Opcode != "cra" {
		t.Fatalf("tower.launch did not use atomic attack context: %#v", plan.Steps)
	}
}

func TestTowerTroopGuardRequiresBothFullFlanks(t *testing.T) {
	source := State.CastleState{
		ID: 1, Name: "Tower Castle",
		Units: State.CastleUnits{Stationed: map[State.UnitID]int64{77: 185}},
	}
	if err := requireTowerAttackUnits(source, 77, 186); err == nil {
		t.Fatal("tower troop guard accepted an incomplete formation")
	}
	source.Units.Stationed[77] = 186
	if err := requireTowerAttackUnits(source, 77, 186); err != nil {
		t.Fatalf("tower troop guard rejected exact stock: %v", err)
	}
}

func TestFreshTowerTroopGuardMakesShortageReplan(t *testing.T) {
	source := State.CastleState{
		ID: 1, Name: "Tower Castle",
		Units: State.CastleUnits{Stationed: map[State.UnitID]int64{77: 185}},
	}
	if err := requireFreshTowerAttackUnits(source, 77, 186); !errors.Is(err, Intent.ErrPlanStale) {
		t.Fatalf("fresh tower troop shortage should make the plan stale: %v", err)
	}
	source.Units.Stationed[77] = 186
	if err := requireFreshTowerAttackUnits(source, 77, 186); err != nil {
		t.Fatalf("fresh tower troop guard rejected exact stock: %v", err)
	}
}

func TestTowerAttackCommanderLossMakesPlanStale(t *testing.T) {
	gameData, err := GameData.DecodeStore([]byte(`{
		"versionInfo":[],"units":[],"buildings":[],"effects":[]
	}`), GameData.SourceMetadata{ItemVersion: "test"})
	if err != nil {
		t.Fatal(err)
	}
	state := State.NewGameState()
	state.Castles[1] = State.CastleState{ID: 1, KingdomID: 0, X: 100, Y: 100}
	state.Commanders[22] = State.CommanderState{ID: 22, Available: false}
	state.Map[0] = map[string]State.MapObservation{
		"101:100": {KingdomID: 0, X: 101, Y: 100, TypeID: kingdomTowerMapTypeID, Level: 81},
	}
	_, _, _, err = resolveTowerAttackCapacity(Intent.PlanningContext{State: state, GameData: gameData}, towerLaunchRequest{
		SourceCastleID: 1, KingdomID: 0, TargetX: 101, TargetY: 100, UnitID: 77,
	}, 22, false)
	if !errors.Is(err, Intent.ErrPlanStale) {
		t.Fatalf("busy commander should make the tower plan stale: %v", err)
	}
}

func TestTowerAttackBecomesNoOpWhenTroopsChangeBeforeAdmission(t *testing.T) {
	gameData, err := GameData.DecodeStore([]byte(`{
		"versionInfo":[],"units":[],"buildings":[],"effects":[]
	}`), GameData.SourceMetadata{ItemVersion: "test"})
	if err != nil {
		t.Fatal(err)
	}
	gameState := State.NewGameState()
	gameState.Castles[1] = State.CastleState{
		ID: 1, KingdomID: 0, X: 100, Y: 100,
		Units: State.CastleUnits{Stationed: map[State.UnitID]int64{77: 1}},
	}
	gameState.Commanders[5] = State.CommanderState{ID: 5, Available: true}
	gameState.Map[0] = map[string]State.MapObservation{
		"101:100": {KingdomID: 0, X: 101, Y: 100, TypeID: kingdomTowerMapTypeID, Level: 81},
	}
	plan, err := planTowerAttack(t.Context(), Intent.PlanningContext{
		State: gameState, GameData: gameData,
	}, json.RawMessage(`{"sourceCastleId":1,"kingdomId":0,"targetX":101,"targetY":100,"unitId":77,"commanderIds":[5]}`))
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Steps) != 1 || plan.Steps[0].Action != "tower.queue.defer" || !strings.Contains(plan.Summary, "Skip tower attack") {
		t.Fatalf("tower shortage plan = %#v", plan)
	}
}

func TestTowerAttackReplanUsesFreshDialogCapacityInsteadOfRepeatingSetup(t *testing.T) {
	gameData, err := GameData.DecodeStore([]byte(`{
		"versionInfo":[],"units":[],"buildings":[],
		"effects":[{"effectID":66,"name":"attackUnitAmountFlank","effectTypeID":28,"capID":99}],
		"effectCaps":[{"capID":99}]
	}`), GameData.SourceMetadata{ItemVersion: "test"})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	gameState := State.NewGameState()
	gameState.Castles[1] = State.CastleState{
		ID: 1, KingdomID: 0, X: 100, Y: 100,
		Units: State.CastleUnits{Stationed: map[State.UnitID]int64{77: 200}},
	}
	gameState.Commanders[5] = State.CommanderState{ID: 5, Available: true}
	gameState.Map[0] = map[string]State.MapObservation{
		"101:100": {
			KingdomID: 0, X: 101, Y: 100, TypeID: kingdomTowerMapTypeID,
			TowerVictoryCount: 845, Level: 81, ObservedAt: now,
		},
	}
	gameState.AttackDialog = State.AttackDialogState{
		SourceCastleID: 1, KingdomID: 0,
		Target:        State.AttackDialogTarget{TypeID: kingdomTowerMapTypeID, X: 101, Y: 100},
		ActiveEffects: []State.AttackDialogEffect{{EffectID: 66, Values: []float64{100}, Source: "CI"}},
		ObservedAt:    now,
	}
	request := towerLaunchRequest{
		SourceCastleID: 1, KingdomID: 0, TargetX: 101, TargetY: 100, UnitID: 77,
	}
	base, _, _, err := resolveTowerAttackCapacity(
		Intent.PlanningContext{State: gameState, GameData: gameData}, request, 5, false,
	)
	if err != nil {
		t.Fatal(err)
	}
	fresh, _, _, err := resolveTowerAttackCapacity(
		Intent.PlanningContext{State: gameState, GameData: gameData}, request, 5, true,
	)
	if err != nil {
		t.Fatal(err)
	}
	if base.Capacity.Left+base.Capacity.Right > 200 || fresh.Capacity.Left+fresh.Capacity.Right <= 200 {
		t.Fatalf("capacity fixture does not cross available stock: base=%#v fresh=%#v", base.Capacity, fresh.Capacity)
	}
	observation, err := towerCapacityObservation(
		Intent.PlanningContext{State: gameState, GameData: gameData}, request, 5,
	)
	if err != nil {
		t.Fatal(err)
	}
	wantAdditional := fresh.Capacity.Left + fresh.Capacity.Right - base.Capacity.Left - base.Capacity.Right
	if observation.AdditionalUnits != wantAdditional ||
		observation.FullFlankUnits != fresh.Capacity.Left+fresh.Capacity.Right ||
		!observation.ObservedAt.Equal(now) {
		t.Fatalf("fresh tower capacity observation = %#v", observation)
	}

	plan, err := planTowerAttack(t.Context(), Intent.PlanningContext{
		State: gameState, GameData: gameData,
	}, json.RawMessage(`{"sourceCastleId":1,"kingdomId":0,"targetX":101,"targetY":100,"unitId":77,"commanderIds":[5]}`))
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Steps) != 1 || plan.Steps[0].Action != "tower.queue.defer" || !strings.Contains(plan.Summary, "Skip tower attack") {
		t.Fatalf("fresh-dialog shortage plan = %#v", plan)
	}
}

func TestTowerAttackSkipsPendingSettlementAndKnownCooldownBeforeADI(t *testing.T) {
	now := time.Now().UTC()
	gameState := State.NewGameState()
	gameState.Castles[1] = State.CastleState{ID: 1, KingdomID: 0, X: 100, Y: 100}
	gameState.Map[0] = map[string]State.MapObservation{
		"101:100": {
			KingdomID: 0, X: 101, Y: 100, TypeID: kingdomTowerMapTypeID,
			TowerVictoryCount: 845, Level: 81, ObservedAt: now,
		},
	}
	gameState.AttackAnalytics.PendingAttacks = []State.AttackFeatureLaunch{{
		MovementID: 10, FeatureID: State.AttackFeatureAutoTowers, KingdomID: 0,
		TargetTypeID: kingdomTowerMapTypeID, TargetX: 101, TargetY: 100,
		LaunchedAt: now.Add(-5 * time.Minute), ArrivesAt: now.Add(-time.Second),
	}}
	arguments := json.RawMessage(`{"sourceCastleId":1,"kingdomId":0,"targetX":101,"targetY":100,"unitId":77}`)

	plan, err := planTowerAttack(t.Context(), Intent.PlanningContext{State: gameState}, arguments)
	if err != nil || len(plan.Steps) != 1 || plan.Steps[0].Action != "tower.queue.defer" || !strings.Contains(plan.Summary, "awaiting settlement") {
		t.Fatalf("pending target plan = %#v err=%v", plan, err)
	}
	gameState.AttackAnalytics.PendingAttacks = nil
	target := gameState.Map[0]["101:100"]
	target.TowerCooldownRemaining = 300
	gameState.Map[0]["101:100"] = target
	plan, err = planTowerAttack(t.Context(), Intent.PlanningContext{State: gameState}, arguments)
	if err != nil || len(plan.Steps) != 1 || plan.Steps[0].Action != "tower.queue.defer" || !strings.Contains(plan.Summary, "on cooldown") {
		t.Fatalf("cooldown target plan = %#v err=%v", plan, err)
	}
}
