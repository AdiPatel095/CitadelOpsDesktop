package Equipment

import (
	"strconv"
	"testing"

	"CitadelDesktop/Server/GameData"
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

func TestOptimizeScoresOfficialSetBonusesDuringSearch(t *testing.T) {
	gameData, err := GameData.DecodeStore([]byte(`{
		"versionInfo":{},"buildings":[],"units":[],
		"effects":[{"effectID":"9001"}],
		"equipment_sets":[{"setID":"77","neededItems":"2","effects":"9001&100"}]
	}`), GameData.SourceMetadata{})
	if err != nil {
		t.Fatal(err)
	}
	gameState := State.NewGameState()
	gameState.Revision = 17
	gameState.Commanders[0] = State.CommanderState{
		ID: 0, Available: true, Equipment: map[string]State.EquipmentInstanceID{}, Gems: map[string]State.GemInstanceID{},
	}
	for slot := 1; slot <= 4; slot++ {
		if slot <= 2 {
			gameState.Inventory.Equipment[State.EquipmentInstanceID(100+slot)] = optimizerTestItem(State.EquipmentInstanceID(100+slot), slot, 10)
			gameState.Inventory.Equipment[State.EquipmentInstanceID(200+slot)] = State.EquipmentInstance{
				ID: State.EquipmentInstanceID(200 + slot), Slot: slot, TypeID: 2, SetID: 77,
			}
			continue
		}
		gameState.Inventory.Equipment[State.EquipmentInstanceID(100+slot)] = optimizerTestItem(State.EquipmentInstanceID(100+slot), slot, 0)
	}

	result, err := Optimize(gameState, gameData, OptimizeRequest{
		LeaderKind: "commander", LeaderID: 0, CombatMode: "pvp",
		Priorities: []Priority{{EffectID: 9001, Tier: 1, Position: 0}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.StateRevision != 17 {
		t.Fatalf("state revision = %d, want 17", result.StateRevision)
	}
	for slot := 1; slot <= 2; slot++ {
		want := State.EquipmentInstanceID(200 + slot)
		if got := result.Proposed.Equipment[strconv.Itoa(slot)]; got != want {
			t.Fatalf("set slot %d = %d, want %d", slot, got, want)
		}
	}
	if result.Proposed.Score != 1_000_000 {
		t.Fatalf("set loadout score = %.0f, want 1000000", result.Proposed.Score)
	}
}

func TestOptimizeKeepsCurrentLoadoutWhenNoPriorityEffectIsAvailable(t *testing.T) {
	gameState := State.NewGameState()
	leader := State.CommanderState{
		ID: 0, Available: true, Equipment: map[string]State.EquipmentInstanceID{}, Gems: map[string]State.GemInstanceID{},
	}
	for slot := 1; slot <= 4; slot++ {
		currentID := State.EquipmentInstanceID(100 + slot)
		alternativeID := State.EquipmentInstanceID(200 + slot)
		leader.Equipment[strconv.Itoa(slot)] = currentID
		current := optimizerTestItem(currentID, slot, 0)
		current.Effects[0].DefinitionID = 9002
		current.WearerKind, current.WearerID = "commander", 0
		alternative := optimizerTestItem(alternativeID, slot, 0)
		alternative.Effects[0].DefinitionID = 9002
		gameState.Inventory.Equipment[currentID] = current
		gameState.Inventory.Equipment[alternativeID] = alternative
	}
	gameState.Commanders[0] = leader

	result, err := Optimize(gameState, nil, OptimizeRequest{
		LeaderKind: "commander", LeaderID: 0, CombatMode: "pvp",
		Priorities: []Priority{{EffectID: 9001, Tier: 1, Position: 0}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Current.Score != 0 || result.Proposed.Score != 0 {
		t.Fatalf("scores = current %.0f, proposed %.0f", result.Current.Score, result.Proposed.Score)
	}
	for slot := 1; slot <= 4; slot++ {
		key := strconv.Itoa(slot)
		if got, want := result.Proposed.Equipment[key], leader.Equipment[key]; got != want {
			t.Fatalf("slot %d = %d, want current equipment %d", slot, got, want)
		}
	}
}

func BenchmarkOptimizeLargeStorage(b *testing.B) {
	gameState := largeOptimizerState(1_000, 2_000)
	request := OptimizeRequest{
		LeaderKind: "commander", LeaderID: 0, CombatMode: "pvp",
		Priorities: []Priority{{EffectID: 9001, Tier: 1, Position: 0}, {EffectID: 9002, Tier: 2, Position: 0}},
	}
	b.ReportAllocs()
	b.ResetTimer()
	for index := 0; index < b.N; index++ {
		if _, err := Optimize(gameState, nil, request); err != nil {
			b.Fatal(err)
		}
	}
}

func largeOptimizerState(equipmentPerSlot int, gemCount int) State.GameState {
	gameState := State.NewGameState()
	leader := State.CommanderState{
		ID: 0, Available: true, Equipment: map[string]State.EquipmentInstanceID{}, Gems: map[string]State.GemInstanceID{},
	}
	for slot := 1; slot <= 4; slot++ {
		for index := 0; index < equipmentPerSlot; index++ {
			id := State.EquipmentInstanceID(slot*1_000_000 + index)
			gameState.Inventory.Equipment[id] = State.EquipmentInstance{
				ID: id, Slot: slot, TypeID: 2,
				Effects: State.EquipmentEffects{
					{WireID: 1, DefinitionID: 9001, Values: []float64{float64(index % 101)}},
					{WireID: 2, DefinitionID: 9002, Values: []float64{float64(index % 37)}},
				},
			}
			if index == 0 {
				leader.Equipment[strconv.Itoa(slot)] = id
				item := gameState.Inventory.Equipment[id]
				item.WearerKind, item.WearerID = "commander", 0
				gameState.Inventory.Equipment[id] = item
			}
		}
	}
	for index := 0; index < gemCount; index++ {
		id := State.GemInstanceID(10_000_000 + index)
		gameState.Inventory.Gems[id] = State.GemInstance{
			ID: id, CompatibleWearerID: 2, CombatMode: "pvp",
			Effects: State.EquipmentEffects{
				{WireID: 301, DefinitionID: 9001, Values: []float64{float64(index % 67)}},
				{WireID: 302, DefinitionID: 9002, Values: []float64{float64(index % 29)}},
			},
		}
	}
	gameState.Commanders[0] = leader
	return gameState
}
