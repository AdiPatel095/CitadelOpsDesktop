package App

import (
	"context"
	"encoding/json"
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
	if len(attackPlan.Steps) != 4 || attackPlan.Steps[0].Opcode != "jaa" || attackPlan.Steps[0].AwaitOpcode != "jaa" ||
		attackPlan.Steps[1].Action != "tower.attack.guard" || attackPlan.Steps[2].Resolver != "tower.attack.build" ||
		attackPlan.Steps[2].CommandDependencies == nil || attackPlan.Steps[2].CommandDependencies.Opcode != "cra" ||
		attackPlan.Steps[3].Action != "tower.queue.consume" {
		t.Fatalf("unexpected atomic tower plan: %#v", attackPlan.Steps)
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
	if len(plan.Steps) != 4 || plan.Steps[0].Opcode != "jaa" || plan.Steps[2].Resolver != "tower.attack.build" ||
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
