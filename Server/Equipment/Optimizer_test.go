package Equipment

import (
	"strconv"
	"testing"

	"CitadelDesktop/Server/State"
)

func TestOptimizeSelectsBestCanonicalEffectsBySlot(t *testing.T) {
	gameState := State.NewGameState()
	gameState.Commanders[0] = State.CommanderState{
		ID: 0, Available: true, Equipment: map[string]State.EquipmentInstanceID{}, Gems: map[string]State.GemInstanceID{},
	}
	for slot := 1; slot <= 4; slot++ {
		weakID := State.EquipmentInstanceID(100 + slot)
		strongID := State.EquipmentInstanceID(200 + slot)
		gameState.Inventory.Equipment[weakID] = optimizerTestItem(weakID, slot, 10)
		gameState.Inventory.Equipment[strongID] = optimizerTestItem(strongID, slot, float64(20+slot))
		leader := gameState.Commanders[0]
		leader.Equipment[strconv.Itoa(slot)] = weakID
		gameState.Commanders[0] = leader
	}
	for slot := 1; slot <= 2; slot++ {
		id := State.GemInstanceID(300 + slot)
		gameState.Inventory.Gems[id] = State.GemInstance{
			ID: id, CompatibleWearerID: 2, CombatMode: "pvp",
			Effects: State.EquipmentEffects{{WireID: 301, DefinitionID: 9001, Values: []float64{5}}},
		}
		leader := gameState.Commanders[0]
		leader.Gems[strconv.Itoa(slot)] = id
		gameState.Commanders[0] = leader
	}

	result, err := Optimize(gameState, nil, OptimizeRequest{
		LeaderKind: "commander", LeaderID: 0, CombatMode: "pvp",
		Priorities: []Priority{{EffectID: 9001, Tier: 1, Position: 0}},
	})
	if err != nil {
		t.Fatal(err)
	}
	for slot := 1; slot <= 4; slot++ {
		want := State.EquipmentInstanceID(200 + slot)
		if got := result.Proposed.Equipment[strconv.Itoa(slot)]; got != want {
			t.Fatalf("slot %d = %d, want %d", slot, got, want)
		}
	}
	if result.Proposed.Score <= result.Current.Score {
		t.Fatalf("proposed score %.0f did not improve current %.0f", result.Proposed.Score, result.Current.Score)
	}
	for slot := 1; slot <= 2; slot++ {
		if result.Proposed.Gems[strconv.Itoa(slot)] != result.Current.Gems[strconv.Itoa(slot)] {
			t.Fatalf("equal-scoring gem in slot %d moved: current %#v proposed %#v", slot, result.Current.Gems, result.Proposed.Gems)
		}
	}
}

func optimizerTestItem(id State.EquipmentInstanceID, slot int, value float64) State.EquipmentInstance {
	return State.EquipmentInstance{
		ID: id, Slot: slot, TypeID: 2,
		Effects: State.EquipmentEffects{{WireID: 1, DefinitionID: 9001, Values: []float64{value}}},
	}
}
