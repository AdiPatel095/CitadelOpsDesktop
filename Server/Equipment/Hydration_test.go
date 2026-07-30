package Equipment

import (
	"testing"

	"CitadelDesktop/Server/GameData"
	"CitadelDesktop/Server/State"
)

func TestHydrateStateMigratesLegacySocketAndUsesOfficialGemData(t *testing.T) {
	gameData, err := GameData.DecodeStore([]byte(`{
		"versionInfo":{},"buildings":[],"units":[],
		"gems":[{"gemID":"520","wearerID":"1","setID":"1101","comment1":"PvP gem","effects":"519&15,528&15,533&3"}],
		"effects":[
			{"effectID":"519","isPvPFight":"1"},
			{"effectID":"528","isPvPFight":"1"},
			{"effectID":"533","isPvPFight":"1"}
		],
		"relicTypes":[]
	}`), GameData.SourceMetadata{})
	if err != nil {
		t.Fatal(err)
	}
	gameState := State.NewGameState()
	gameState.Castellans[1] = State.CastellanState{
		ID:        1,
		Equipment: map[string]State.EquipmentInstanceID{"1": 100},
		Gems:      map[string]State.GemInstanceID{"1": 100},
	}
	gameState.Inventory.Equipment[100] = State.EquipmentInstance{
		ID: 100, Slot: 1, TypeID: 1, WearerKind: "castellan", WearerID: 1,
		Effects: State.EquipmentEffects{},
	}
	gameState.Inventory.Gems[100] = State.GemInstance{
		ID: 100, DefinitionID: 520, Slot: 1, WearerKind: "castellan", WearerID: 1,
		Effects: State.EquipmentEffects{},
	}

	if !HydrateState(&gameState, gameData) {
		t.Fatal("legacy state was not hydrated")
	}
	gem, found := gameState.Inventory.Gems[-100]
	if !found {
		t.Fatalf("canonical socket gem missing: %#v", gameState.Inventory.Gems)
	}
	if gem.ID != -100 || gem.EquipmentInstanceID != 100 || gem.DefinitionID != 520 {
		t.Fatalf("gem identity = %+v", gem)
	}
	if gem.CompatibleWearerID != 1 || gem.CombatMode != "pvp" || gem.SetID != 1101 || len(gem.Effects) != 3 {
		t.Fatalf("official gem fields = %+v", gem)
	}
	if gameState.Castellans[1].Gems["1"] != -100 {
		t.Fatalf("leader socket = %d", gameState.Castellans[1].Gems["1"])
	}
	if HydrateState(&gameState, gameData) {
		t.Fatal("already canonical state changed on a second hydration")
	}
}

func TestHydrateStateRestoresRelicGemInstanceIdentity(t *testing.T) {
	gameState := State.NewGameState()
	gameState.Commanders[2] = State.CommanderState{
		ID: 2, Available: true,
		Equipment: map[string]State.EquipmentInstanceID{"1": 1000},
		Gems:      map[string]State.GemInstanceID{"1": 1000},
	}
	gameState.Inventory.Equipment[1000] = State.EquipmentInstance{
		ID: 1000, Slot: 1, TypeID: 2, RarityID: 5, WearerKind: "commander", WearerID: 2,
		Effects: State.EquipmentEffects{},
	}
	gameState.Inventory.Gems[1000] = State.GemInstance{
		ID: 1000, DefinitionID: 4000001, WearerKind: "commander", WearerID: 2,
		Effects: State.EquipmentEffects{{WireID: 301, DefinitionID: 9001, Values: []float64{25}}},
	}

	if !HydrateState(&gameState, nil) {
		t.Fatal("legacy relic gem was not migrated")
	}
	gem, found := gameState.Inventory.Gems[4000001]
	if !found || gem.ID != 4000001 || gem.EquipmentInstanceID != 1000 {
		t.Fatalf("relic gem = %+v, found %t", gem, found)
	}
	if gameState.Commanders[2].Gems["1"] != 4000001 {
		t.Fatalf("leader relic socket = %d", gameState.Commanders[2].Gems["1"])
	}
}
