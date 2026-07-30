package Ingest

import (
	"encoding/json"
	"slices"
	"testing"
	"time"

	"CitadelDesktop/Server/GameData"
	"CitadelDesktop/Server/Protocol"
	"CitadelDesktop/Server/State"
)

func TestApplyLeadersNormalizesEquipmentAndCommanderZero(t *testing.T) {
	gameState := State.NewGameState()
	gameState.Player.ID = 1
	commanderID := State.CommanderID(0)
	gameState.Movements[12] = State.MovementState{ID: 12, OwnerPlayerID: 1, CommanderID: &commanderID}
	gameState.Castles[99] = State.CastleState{ID: 99, Name: "Main"}
	payload := json.RawMessage(`{
		"C":[{"ID":0,"VIS":0,"N":"Rift1","GID":101,"EQ":[[1001,1,2,5,0,[[164,75,[510]]],1375,1086,20,-1,457,1,[1,0,0,[457,9,0,0,[[301,50,[22]]],12]]]]}],
		"B":[{"ID":1,"LICID":99,"N":"","EQ":[[2001,2,1,0,0,[[268,[175]]],1535,1101,3,-1,521,2]]}]
	}`)
	changed, err := applyLeaders(payload, &gameState, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !changed {
		t.Fatal("leader payload did not change state")
	}
	commander, ok := gameState.Commanders[0]
	if !ok {
		t.Fatal("commander id 0 was not retained")
	}
	if commander.Available {
		t.Fatal("commander with an active movement is marked available")
	}
	if commander.Name != "Rift1" || commander.VisiblePosition != 1 || commander.GeneralID != 101 || commander.Equipment["1"] != 1001 {
		t.Fatalf("unexpected commander: %+v", commander)
	}
	if commander.Gems["1"] != 457 {
		t.Fatalf("commander gem identity = %d, want 457", commander.Gems["1"])
	}
	equipment := gameState.Inventory.Equipment[1001]
	if equipment.DefinitionID != 1375 || equipment.Level != 20 || equipment.SetID != 1086 {
		t.Fatalf("unexpected equipment: %+v", equipment)
	}
	if effect := equipment.Effects[0]; effect.WireID != 164 || effect.DefinitionID != 164 || effect.RollPercent == nil || *effect.RollPercent != 75 || len(effect.Values) != 1 || effect.Values[0] != 510 {
		t.Fatalf("equipment effects = %#v", equipment.Effects)
	}
	gem := gameState.Inventory.Gems[457]
	if gem.DefinitionID != 457 || gem.TypeID != 9 || gem.Level != 12 || gem.Slot != 1 || gem.EquipmentInstanceID != 1001 {
		t.Fatalf("unexpected gem: %+v", gem)
	}
	if effect := gem.Effects[0]; effect.WireID != 301 || effect.DefinitionID != 301 || effect.RollPercent == nil || *effect.RollPercent != 50 || len(effect.Values) != 1 || effect.Values[0] != 22 {
		t.Fatalf("gem effects = %#v", gem.Effects)
	}
	castellan := gameState.Castellans[1]
	if castellan.CastleID != 99 || castellan.Name != "Main" || castellan.Equipment["2"] != 2001 {
		t.Fatalf("unexpected castellan: %+v", castellan)
	}
}

func TestReduceGeneralsCapturesActiveSkillIDs(t *testing.T) {
	gameState := State.NewGameState()
	observedAt := time.Date(2026, time.July, 14, 12, 0, 0, 0, time.UTC)
	frame := testSuccessfulFrame("gie", `{
		"G":[
			{"GID":101,"SIDS":[101036,"10110333",-1]},
			{"GID":102,"SIDS":[]}
		]
	}`)
	frame.ReceivedAt = observedAt
	_, changed, err := reduceGenerals(t.Context(), frame, &gameState, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !changed || len(gameState.Generals) != 2 {
		t.Fatalf("generals = %#v", gameState.Generals)
	}
	general := gameState.Generals[101]
	if !slices.Equal(general.ActiveSkillIDs, []int64{101036, 10110333}) || !general.ObservedAt.Equal(observedAt) {
		t.Fatalf("general 101 = %#v", general)
	}
}

func TestReduceGemStoragePreservesEquippedGemsAndReplacesStorage(t *testing.T) {
	gameState := State.NewGameState()
	gameState.Inventory.Gems[77] = State.GemInstance{ID: 77, WearerKind: "commander", WearerID: 2, Effects: State.EquipmentEffects{}}
	gameState.Inventory.Gems[88] = State.GemInstance{ID: 88, Effects: State.EquipmentEffects{}}
	frame := testSuccessfulFrame("ggm", `{
		"GEM":[[484,2],[517,6]],
		"RGEM":[[2642217,132,6,3850,[[10311,58,[13.3]],[10314,85,[22.1]]],4]]
	}`)
	_, changed, err := reduceGemStorage(t.Context(), frame, &gameState, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !changed || gameState.Inventory.GemStacks[484] != 2 || gameState.Inventory.GemStacks[517] != 6 {
		t.Fatalf("gem stacks = %#v", gameState.Inventory.GemStacks)
	}
	if _, exists := gameState.Inventory.Gems[88]; exists {
		t.Fatal("stale storage gem was retained")
	}
	if _, exists := gameState.Inventory.Gems[77]; !exists {
		t.Fatal("equipped gem was removed by storage refresh")
	}
	gem := gameState.Inventory.Gems[2642217]
	if gem.TypeID != 132 || gem.Level != 4 || len(gem.Effects) != 2 {
		t.Fatalf("stored relic gem = %+v", gem)
	}
}

func TestParseEquipmentResolvesWireEffectsThroughOfficialCatalog(t *testing.T) {
	store, err := GameData.DecodeStore([]byte(`{
		"versionInfo":{},"buildings":[],"units":[],
		"equipment_effects":[{"equipmentEffectID":"164","effectID":"457"}],
		"relicEffects":[{"id":"121","effectID":"2114"}],
		"effects":[{"effectID":"457"},{"effectID":"2114"}]
	}`), GameData.SourceMetadata{})
	if err != nil {
		t.Fatal(err)
	}
	var relicRow []json.RawMessage
	if err := json.Unmarshal([]byte(`[1001,1,2,5,0,[[121,50,[215,525]]],-1,-1,0,-1,-1,0]`), &relicRow); err != nil {
		t.Fatal(err)
	}
	relic, _, ok := parseEquipment(relicRow, "", 0, store)
	if !ok || relic.Effects[0].WireID != 121 || relic.Effects[0].DefinitionID != 2114 {
		t.Fatalf("relic effects = %#v", relic.Effects)
	}
	var normalRow []json.RawMessage
	if err := json.Unmarshal([]byte(`[1002,1,2,0,0,[[164,[10]]],10,-1,0,-1,-1,0]`), &normalRow); err != nil {
		t.Fatal(err)
	}
	normal, _, ok := parseEquipment(normalRow, "", 0, store)
	if !ok || normal.Effects[0].WireID != 164 || normal.Effects[0].DefinitionID != 457 {
		t.Fatalf("normal effects = %#v", normal.Effects)
	}
}

func TestConstructionInventoryUsesConstructionItemIDs(t *testing.T) {
	gameState := State.NewGameState()
	frame := testSuccessfulFrame("gii", `{"CI":[[42,3],[99,0]]}`)
	_, changed, err := reduceConstructionInventory(t.Context(), frame, &gameState, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !changed || gameState.Inventory.ConstructionItems[42] != 3 {
		t.Fatalf("construction inventory = %#v", gameState.Inventory.ConstructionItems)
	}
}

func testSuccessfulFrame(opcode string, payload string) Protocol.Frame {
	code := 0
	return Protocol.Frame{Opcode: opcode, ResponseCode: &code, Payload: json.RawMessage(payload)}
}
