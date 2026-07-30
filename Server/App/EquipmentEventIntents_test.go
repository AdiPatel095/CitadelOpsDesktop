package App

import (
	"encoding/json"
	"strings"
	"testing"

	"CitadelDesktop/Server/GameData"
	"CitadelDesktop/Server/Intent"
	"CitadelDesktop/Server/State"
)

func TestPlanEquipmentEventApplyClearsCommanderBeforeEquippingOwnedPartialSet(t *testing.T) {
	gameState := State.NewGameState()
	leader := State.CommanderState{
		ID: 7, Available: true,
		Equipment: map[string]State.EquipmentInstanceID{},
		Gems:      map[string]State.GemInstanceID{},
	}
	for index, slot := range baseEquipmentSlots {
		id := State.EquipmentInstanceID(11 + index)
		leader.Equipment[slotKey(slot)] = id
		gameState.Inventory.Equipment[id] = State.EquipmentInstance{
			ID: id, DefinitionID: State.EquipmentID(1000 + index), Slot: slot, TypeID: 2,
			WearerKind: "commander", WearerID: 7,
		}
	}
	gameState.Commanders[7] = leader
	gameState.Inventory.Equipment[101] = State.EquipmentInstance{
		ID: 101, DefinitionID: 1390, Slot: 1, TypeID: 2, SetID: 1087,
	}
	gameState.Inventory.Equipment[104] = State.EquipmentInstance{
		ID: 104, DefinitionID: 1393, Slot: 4, TypeID: 2, SetID: 1087,
	}
	gameState.Inventory.Gems[-900] = State.GemInstance{
		ID: -900, DefinitionID: 900, EquipmentInstanceID: 101,
	}
	gameState.Inventory.GemStacks[462] = 1

	plan, err := planEquipmentEventApply(
		t.Context(),
		Intent.PlanningContext{State: gameState, GameData: equipmentEventGameData(t)},
		json.RawMessage(`{"commanderId":7,"event":"nomad"}`),
	)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(plan.Summary, "Nomad Invasion") || !strings.Contains(plan.Summary, "2 equipment and 1 gems") {
		t.Fatalf("summary = %q", plan.Summary)
	}
	for index, expectedID := range []int64{11, 12, 13, 14, 15} {
		if index >= len(plan.Steps) || plan.Steps[index].Opcode != "eeq" {
			t.Fatalf("step %d = %#v", index, plan.Steps[index])
		}
		var payload struct {
			EquipmentID int64 `json:"EID"`
			Equip       int   `json:"E"`
		}
		if err := json.Unmarshal(plan.Steps[index].Command.Payload, &payload); err != nil {
			t.Fatal(err)
		}
		if payload.EquipmentID != expectedID || payload.Equip != 0 {
			t.Fatalf("clear step %d payload = %#v", index, payload)
		}
	}
	if !hasEquipmentCommand(plan, 101, 1) || !hasEquipmentCommand(plan, 104, 1) {
		t.Fatalf("event equipment was not mounted: %#v", plan.Steps)
	}
	if !hasStandardGemCommand(plan, 462, 101) {
		t.Fatalf("event gem was not socketed from the standard stack: %#v", plan.Steps)
	}
}

func TestPlanEquipmentEventApplyChoosesOneMostCompleteHollowMoonTier(t *testing.T) {
	gameState := State.NewGameState()
	gameState.Commanders[0] = State.CommanderState{
		ID: 0, Available: true,
		Equipment: map[string]State.EquipmentInstanceID{},
		Gems:      map[string]State.GemInstanceID{},
	}
	bronzeDefinitions := map[int]State.EquipmentID{1: 1462, 2: 1463, 3: 1461, 4: 1464, 6: 1465}
	for index, slot := range baseEquipmentSlots {
		id := State.EquipmentInstanceID(200 + index)
		gameState.Inventory.Equipment[id] = State.EquipmentInstance{
			ID: id, DefinitionID: bronzeDefinitions[slot], Slot: slot, TypeID: 2, SetID: 1094,
		}
	}
	silverDefinitions := map[int]State.EquipmentID{1: 1467, 2: 1468, 3: 1466, 4: 1469}
	for index, slot := range []int{1, 2, 3, 4} {
		id := State.EquipmentInstanceID(300 + index)
		gameState.Inventory.Equipment[id] = State.EquipmentInstance{
			ID: id, DefinitionID: silverDefinitions[slot], Slot: slot, TypeID: 2, SetID: 1095,
		}
	}
	gameState.Inventory.Equipment[400] = State.EquipmentInstance{
		ID: 400, DefinitionID: 1471, Slot: 3, TypeID: 2, SetID: 1096,
	}
	gameState.Inventory.GemStacks[490] = 1
	for definitionID := State.GemID(494); definitionID <= 497; definitionID++ {
		gameState.Inventory.GemStacks[definitionID] = 1
	}

	plan, err := planEquipmentEventApply(
		t.Context(),
		Intent.PlanningContext{State: gameState, GameData: equipmentEventGameData(t)},
		json.RawMessage(`{"commanderId":0,"event":"hollow_moon_pvp"}`),
	)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(plan.Summary, "Bronze Hollow Moon PvP") {
		t.Fatalf("summary = %q", plan.Summary)
	}
	for index := range baseEquipmentSlots {
		if !hasEquipmentCommand(plan, int64(200+index), 1) {
			t.Fatalf("bronze item %d was not equipped", 200+index)
		}
	}
	for id := int64(300); id <= 303; id++ {
		if hasEquipmentCommand(plan, id, 1) {
			t.Fatalf("silver item %d was mixed into the bronze loadout", id)
		}
	}
	if hasEquipmentCommand(plan, 400, 1) {
		t.Fatal("gold equipment was mixed into the bronze loadout")
	}
}

func TestPlanEquipmentEventApplyDoesNotStripCommanderWithoutAvailableEventGear(t *testing.T) {
	gameState := State.NewGameState()
	gameState.Commanders[3] = State.CommanderState{
		ID: 3, Available: true,
		Equipment: map[string]State.EquipmentInstanceID{"1": 11},
		Gems:      map[string]State.GemInstanceID{},
	}
	gameState.Inventory.Equipment[11] = State.EquipmentInstance{
		ID: 11, DefinitionID: 1000, Slot: 1, TypeID: 2, WearerKind: "commander", WearerID: 3,
	}
	gameState.Inventory.Equipment[21] = State.EquipmentInstance{
		ID: 21, DefinitionID: 1395, Slot: 1, TypeID: 2, SetID: 1088,
		WearerKind: "commander", WearerID: 9,
	}
	_, err := planEquipmentEventApply(
		t.Context(),
		Intent.PlanningContext{State: gameState, GameData: equipmentEventGameData(t)},
		json.RawMessage(`{"commanderId":3,"event":"samurai"}`),
	)
	if err == nil || !strings.Contains(err.Error(), "no available Samurai Invasion equipment") {
		t.Fatalf("error = %v", err)
	}
}

func equipmentEventGameData(t *testing.T) *GameData.Store {
	t.Helper()
	store, err := GameData.DecodeStore([]byte(`{
		"versionInfo":[],
		"buildings":[],
		"units":[],
		"equipments":[
			{"equipmentID":"1390","setID":"1087","wearerID":"2","slotID":"1"},
			{"equipmentID":"1391","setID":"1087","wearerID":"2","slotID":"2"},
			{"equipmentID":"1392","setID":"1087","wearerID":"2","slotID":"3"},
			{"equipmentID":"1393","setID":"1087","wearerID":"2","slotID":"4"},
			{"equipmentID":"1394","setID":"1087","wearerID":"2","slotID":"6"},
			{"equipmentID":"1395","setID":"1088","wearerID":"2","slotID":"1"},
			{"equipmentID":"1396","setID":"1088","wearerID":"2","slotID":"2"},
			{"equipmentID":"1397","setID":"1088","wearerID":"2","slotID":"3"},
			{"equipmentID":"1398","setID":"1088","wearerID":"2","slotID":"4"},
			{"equipmentID":"1399","setID":"1088","wearerID":"2","slotID":"6"},
			{"equipmentID":"1461","setID":"1094","wearerID":"2","slotID":"3"},
			{"equipmentID":"1462","setID":"1094","wearerID":"2","slotID":"1"},
			{"equipmentID":"1463","setID":"1094","wearerID":"2","slotID":"2"},
			{"equipmentID":"1464","setID":"1094","wearerID":"2","slotID":"4"},
			{"equipmentID":"1465","setID":"1094","wearerID":"2","slotID":"6"},
			{"equipmentID":"1466","setID":"1095","wearerID":"2","slotID":"3"},
			{"equipmentID":"1467","setID":"1095","wearerID":"2","slotID":"1"},
			{"equipmentID":"1468","setID":"1095","wearerID":"2","slotID":"2"},
			{"equipmentID":"1469","setID":"1095","wearerID":"2","slotID":"4"},
			{"equipmentID":"1470","setID":"1095","wearerID":"2","slotID":"6"},
			{"equipmentID":"1471","setID":"1096","wearerID":"2","slotID":"3"},
			{"equipmentID":"1472","setID":"1096","wearerID":"2","slotID":"1"},
			{"equipmentID":"1473","setID":"1096","wearerID":"2","slotID":"2"},
			{"equipmentID":"1474","setID":"1096","wearerID":"2","slotID":"4"},
			{"equipmentID":"1475","setID":"1096","wearerID":"2","slotID":"6"}
		],
		"gems":[
			{"gemID":"462","setID":"1087","wearerID":"2"},
			{"gemID":"463","setID":"1087","wearerID":"2"},
			{"gemID":"464","setID":"1087","wearerID":"2"},
			{"gemID":"465","setID":"1087","wearerID":"2"},
			{"gemID":"466","setID":"1088","wearerID":"2"},
			{"gemID":"467","setID":"1088","wearerID":"2"},
			{"gemID":"468","setID":"1088","wearerID":"2"},
			{"gemID":"469","setID":"1088","wearerID":"2"},
			{"gemID":"490","setID":"1094","wearerID":"2"},
			{"gemID":"491","setID":"1094","wearerID":"2"},
			{"gemID":"492","setID":"1094","wearerID":"2"},
			{"gemID":"493","setID":"1094","wearerID":"2"},
			{"gemID":"494","setID":"1095","wearerID":"2"},
			{"gemID":"495","setID":"1095","wearerID":"2"},
			{"gemID":"496","setID":"1095","wearerID":"2"},
			{"gemID":"497","setID":"1095","wearerID":"2"},
			{"gemID":"498","setID":"1096","wearerID":"2"},
			{"gemID":"499","setID":"1096","wearerID":"2"},
			{"gemID":"500","setID":"1096","wearerID":"2"},
			{"gemID":"501","setID":"1096","wearerID":"2"}
		]
	}`), GameData.SourceMetadata{ItemVersion: "test"})
	if err != nil {
		t.Fatal(err)
	}
	return store
}

func slotKey(slot int) string {
	return string(rune('0' + slot))
}

func hasEquipmentCommand(plan Intent.Plan, equipmentID int64, equip int) bool {
	for _, step := range plan.Steps {
		if step.Opcode != "eeq" {
			continue
		}
		var payload struct {
			EquipmentID int64 `json:"EID"`
			Equip       int   `json:"E"`
		}
		if json.Unmarshal(step.Command.Payload, &payload) == nil && payload.EquipmentID == equipmentID && payload.Equip == equip {
			return true
		}
	}
	return false
}

func hasStandardGemCommand(plan Intent.Plan, gemID int64, equipmentID int64) bool {
	for _, step := range plan.Steps {
		if step.Opcode != "bge" {
			continue
		}
		var payload struct {
			GemID       int64 `json:"GID"`
			EquipmentID int64 `json:"EID"`
			RelicGem    int   `json:"RGEM"`
		}
		if json.Unmarshal(step.Command.Payload, &payload) == nil &&
			payload.GemID == gemID && payload.EquipmentID == equipmentID && payload.RelicGem == 0 {
			return true
		}
	}
	return false
}
