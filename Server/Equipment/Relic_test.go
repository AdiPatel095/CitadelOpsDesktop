package Equipment

import (
	"testing"

	"CitadelDesktop/Server/State"
)

func TestCommanderRelic2EffectTotalExcludesLegacyAndMalformedRelicShapes(t *testing.T) {
	gameState := State.NewGameState()
	gameState.Commanders[7] = State.CommanderState{
		ID: 7,
		Equipment: map[string]State.EquipmentInstanceID{
			"1": 101,
			"2": 102,
			"3": 103,
		},
	}
	gameState.Inventory.Equipment[101] = State.EquipmentInstance{
		ID: 101, Slot: 1, RarityID: 5,
		Effects: State.EquipmentEffects{
			{DefinitionID: 2106, Values: []float64{42}},
			{DefinitionID: 1, Values: []float64{1}},
			{DefinitionID: 2, Values: []float64{1}},
			{DefinitionID: 3, Values: []float64{1}},
		},
	}
	gameState.Inventory.Equipment[102] = State.EquipmentInstance{
		ID: 102, Slot: 6, RarityID: 15,
		Effects: State.EquipmentEffects{
			{DefinitionID: 2106, Values: []float64{8}},
			{DefinitionID: 1, Values: []float64{1}},
			{DefinitionID: 2, Values: []float64{1}},
			{DefinitionID: 3, Values: []float64{1}},
			{DefinitionID: 4, Values: []float64{1}},
			{DefinitionID: 5, Values: []float64{1}},
		},
	}
	gameState.Inventory.Equipment[103] = State.EquipmentInstance{
		ID: 103, Slot: 2, RarityID: 4,
		Effects: State.EquipmentEffects{{DefinitionID: 2106, Values: []float64{100}}},
	}

	value, found := CommanderRelic2EffectTotal(gameState, 7, 2106)
	if !found || value != 50 {
		t.Fatalf("Relic 2.0 speed total = %.0f found=%t, want 50/true", value, found)
	}
}
