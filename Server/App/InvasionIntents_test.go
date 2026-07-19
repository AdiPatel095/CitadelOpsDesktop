package App

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"CitadelDesktop/Server/AttackCapacity"
	"CitadelDesktop/Server/AttackPresets"
	"CitadelDesktop/Server/GameData"
	"CitadelDesktop/Server/Intent"
	"CitadelDesktop/Server/State"
)

func TestInvasionAttackResolvesFreshLaneCapacityAndAcceptsCommanderZero(t *testing.T) {
	gameData, err := GameData.DecodeStore([]byte(`{
		"versionInfo":[],
		"units":[{"wodID":216}],
		"buildings":[],
		"effects":[
			{"effectID":2110,"name":"relicAttackUnitAmountFront","effectTypeID":34,"capID":1010},
			{"effectID":2807,"name":"relicAttackUnitAmountFrontPVP","effectTypeID":34,"capID":1707,"areaTypeID":"1,3,4,10,12,15,21"}
		],
		"legendskills":[{"skillID":106,"effectType":"additionalUnitAmountOnFront","totalEffectValue":25}],
		"effectCaps":[{"capID":99}]
	}`), GameData.SourceMetadata{ItemVersion: "test"})
	if err != nil {
		t.Fatal(err)
	}
	unitID := int64(216)
	now := time.Now().UTC()
	gameState := State.NewGameState()
	gameState.Castles[1] = State.CastleState{
		ID: 1, KingdomID: 0, X: 1164, Y: 1167,
		Units: State.CastleUnits{Stationed: map[State.UnitID]int64{216: 1_000}},
	}
	gameState.Commanders[0] = State.CommanderState{
		ID: 0, Available: true, Equipment: map[string]State.EquipmentInstanceID{"1": 1001, "6": 1002},
	}
	gameState.Inventory.Equipment[1001] = State.EquipmentInstance{
		ID: 1001, Effects: State.EquipmentEffects{{DefinitionID: 2110, Values: []float64{45}}},
	}
	gameState.Inventory.Equipment[1002] = State.EquipmentInstance{
		ID: 1002, Effects: State.EquipmentEffects{{DefinitionID: 2807, Values: []float64{25.1}}},
	}
	gameState.Player.LegendSkills = State.LegendSkillState{ActiveIDs: []int64{106}, ObservedAt: now}
	gameState.Map[0] = map[string]State.MapObservation{
		"1165:1166": {KingdomID: 0, TypeID: 21, X: 1165, Y: 1166, ObjectID: 70, ObservedAt: now},
	}
	gameState.AttackDialog = State.AttackDialogState{
		SourceCastleID: 1, KingdomID: 0,
		Target:        State.AttackDialogTarget{TypeID: 21, X: 1165, Y: 1166, ObjectID: 70},
		ActiveEffects: []State.AttackDialogEffect{},
		ObservedAt:    now,
	}
	request := invasionAttackRequest{
		SourceCastleID: 1, EventID: 71, ScoreTarget: 5_000_000, MinimumRemainingSec: 1_800,
		TargetTypeID: 21, KingdomID: 0, TargetX: 1165, TargetY: 1166, TargetObjectID: 70,
		Preset: AttackPresets.Preset{Name: "Trial", Waves: []AttackPresets.Wave{{
			Middle: AttackPresets.Lane{Troops: []AttackPresets.Slot{{ItemID: &unitID, Quantity: 1_000}}},
		}}},
	}
	arguments, _ := json.Marshal(request)
	plan, err := planInvasionAttack(context.Background(), Intent.PlanningContext{State: gameState, GameData: gameData}, arguments)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Steps) != 3 || plan.Steps[1].Resolver != "invasion.attack.build" ||
		plan.Steps[1].CommandDependencies == nil || plan.Steps[1].CommandDependencies.Opcode != "cra" {
		t.Fatalf("invasion attack did not defer formation building behind CRA send dependencies: %#v", plan.Steps)
	}
	resolved, err := (&Application{}).resolveInvasionAttackStep(
		context.Background(), Intent.PlanningContext{State: gameState, GameData: gameData}, plan.Steps[1].ResolverArguments,
	)
	if err != nil {
		t.Fatal(err)
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
}

func TestLimitAttackSetupToCapacityPreservesSlotPriorityPerWave(t *testing.T) {
	first, second := int64(216), int64(217)
	setup := attackSetupRequest{Waves: []attackSetupWaveRequest{{
		Left: attackSetupLaneRequest{Troops: []attackSetupSlotRequest{
			{ItemID: &first, Quantity: 50}, {ItemID: &second, Quantity: 50},
		}},
	}}}

	limited := limitAttackSetupToCapacity(setup, AttackCapacity.LaneCapacity{Left: 64, Front: 192, Right: 64})
	if limited.Waves[0].Left.Troops[0].Quantity != 50 || limited.Waves[0].Left.Troops[1].Quantity != 14 {
		t.Fatalf("unexpected limited left flank: %#v", limited.Waves[0].Left.Troops)
	}
	if setup.Waves[0].Left.Troops[1].Quantity != 50 {
		t.Fatal("capacity limiting mutated the saved preset")
	}
}
