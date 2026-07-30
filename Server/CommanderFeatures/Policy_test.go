package CommanderFeatures

import (
	"testing"

	"CitadelDesktop/Server/State"
)

func TestCandidatesApplyAssignmentsAndEquipmentRequirements(t *testing.T) {
	gameState := State.NewGameState()
	gameState.Commanders[1] = State.CommanderState{
		ID: 1, Equipment: map[string]State.EquipmentInstanceID{"1": 101},
	}
	gameState.Commanders[2] = State.CommanderState{
		ID: 2, Equipment: map[string]State.EquipmentInstanceID{"1": 102},
	}
	gameState.Commanders[3] = State.CommanderState{ID: 3, Equipment: map[string]State.EquipmentInstanceID{}}
	gameState.Inventory.Equipment[101] = State.EquipmentInstance{
		ID: 101, Effects: State.EquipmentEffects{{
			DefinitionID: 2114, Values: []float64{215, 1000},
		}},
	}
	gameState.Inventory.Equipment[102] = State.EquipmentInstance{
		ID: 102, Effects: State.EquipmentEffects{{
			DefinitionID: 2114, Values: []float64{215, 850},
		}},
	}
	minimum := 900.0
	configuration := Configuration{
		Assignments: map[string][]State.CommanderID{},
		Requirements: map[string][]CommanderRequirement{
			"autoTowers": {{
				Kind: RequirementKindEquipmentEffect, EffectDefinitionID: 2114,
				UnitID: 215, MinimumValue: &minimum,
			}},
		},
	}

	candidates, restricted := Candidates(gameState, configuration, "autoTowers")
	if !restricted || len(candidates) != 1 || candidates[0] != 1 {
		t.Fatalf("requirement candidates = %#v, restricted = %t", candidates, restricted)
	}

	configuration.Assignments["autoTowers"] = []State.CommanderID{2, 3}
	candidates, restricted = Candidates(gameState, configuration, "autoTowers")
	if !restricted || len(candidates) != 0 {
		t.Fatalf("assigned requirement candidates = %#v, restricted = %t", candidates, restricted)
	}
}

func TestCandidatesSupportEventSpecificBonusUnitEffects(t *testing.T) {
	gameState := State.NewGameState()
	gameState.Commanders[0] = State.CommanderState{
		ID: 0, Equipment: map[string]State.EquipmentInstanceID{"1": 100},
	}
	gameState.Inventory.Equipment[100] = State.EquipmentInstance{
		ID: 100, Effects: State.EquipmentEffects{{
			DefinitionID: 769, Values: []float64{450},
		}},
	}
	minimum := 400.0
	maximum := 500.0
	configuration := Configuration{
		Requirements: map[string][]CommanderRequirement{
			"riftReplay": {{
				Kind: RequirementKindEquipmentEffect, EffectDefinitionID: 769,
				MinimumValue: &minimum, MaximumValue: &maximum,
			}},
		},
	}

	candidates, restricted := Candidates(gameState, configuration, "riftReplay")
	if !restricted || len(candidates) != 1 || candidates[0] != 0 {
		t.Fatalf("event requirement candidates = %#v, restricted = %t", candidates, restricted)
	}
}

func TestUnknownRequirementKindsFailClosed(t *testing.T) {
	gameState := State.NewGameState()
	gameState.Commanders[0] = State.CommanderState{ID: 0, Equipment: map[string]State.EquipmentInstanceID{}}
	configuration := Configuration{
		Requirements: map[string][]CommanderRequirement{
			"autoStorm": {{Kind: "futureEventEquipment"}},
		},
	}

	candidates, restricted := Candidates(gameState, configuration, "autoStorm")
	if !restricted || len(candidates) != 0 {
		t.Fatalf("unknown requirement candidates = %#v, restricted = %t", candidates, restricted)
	}
}
