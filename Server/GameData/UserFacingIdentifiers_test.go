package GameData

import (
	"strings"
	"testing"

	"CitadelDesktop/Server/State"
)

func TestIdentifierLabelsHumanizeKnownRuntimeAndOfficialIDs(t *testing.T) {
	store, err := DecodeStore([]byte(`{
		"versionInfo":[],
		"buildings":[{"wodID":201,"name":"bakery","level":8}],
		"units":[
			{"wodID":489,"name":"elitecrossbowman","level":6},
			{"wodID":614,"name":"meadMace","slotTypes":"1"}
		],
		"constructionItems":[{"constructionItemID":301,"name":"bakeryBoost","level":3}],
		"craftingRecipes":[{"craftingRecipeId":401,"type":"honey","level":2}],
		"equipments":[{"equipmentID":501,"_display_name":"Relic Armor"}],
		"gems":[{"gemID":601,"_display_name":"Fire Gem"}],
		"resources":[{"resourceID":1,"name":"currency1","JSONKey":"C1"}],
		"currencies":[{"currencyID":2,"Name":"premiumCurrency","JSONKey":"C2"}],
		"packages":[{"packageID":701,"comment1":"War horn bundle"}],
		"horses":[{"wodID":801,"comment1":"Fast horse"}],
		"kingdoms":[{"kID":0,"name":"empire"}],
		"legendskills":[{"skillID":901,"name":"masterBuilder"}],
		"prebuiltcastles":[{"preBuiltCastleID":902,"comment2":"Berimond Vanguard"}],
		"achievements":[{"achievementID":903,"name":"architect"}],
		"rewards":[{"rewardID":904,"comment1":"Honey reward"}],
		"effects":[{"effectID":905,"name":"travelSpeed"}],
		"events":[{"eventID":103,"comment1":"Red Alliance Alien Invasion","eventType":"RedAllianceAlienInvasion"}],
		"eventAutoScalingDifficulties":[{"difficultyID":1001,"eventID":103,"difficultyTypeID":8}],
		"eventAutoScalingDifficultyTypes":[{"difficultyTypeID":8,"name":"expertPlus"}],
		"isles":[{"IsleID":906,"type":"VILLAGEWOOD","dungeonlevel":70,"fixedLootWood":40000}]
	}`), SourceMetadata{})
	if err != nil {
		t.Fatal(err)
	}
	language, err := DecodeLanguage([]byte(`{
		"bakery_name":"Bakery",
		"elitecrossbowman_name":"Veteran Crossbowman",
		"meadMace_name":"Mead Mace",
		"bakeryBoost_name":"Bakery Production Item",
		"honey_name":"Honey",
		"currency1_name":"Coins",
		"premiumCurrency_name":"Rubies",
		"empire_name":"Great Empire",
		"masterBuilder_name":"Master Builder",
		"architect_name":"Architect",
		"travelSpeed_name":"Travel Speed",
		"event_title_103":"War of the Realms"
	}`), LanguageMetadata{})
	if err != nil {
		t.Fatal(err)
	}
	state := State.NewGameState()
	state.Castles[10] = State.CastleState{
		ID: 10, Name: "Main Keep",
		Buildings: map[State.BuildingInstanceID]State.Building{
			1001: {InstanceID: 1001, DefinitionID: 201, Level: 8},
		},
		Units: State.CastleUnits{
			Stationed: map[State.UnitID]int64{489: 10},
			Total:     map[State.UnitID]int64{489: 10},
		},
	}
	state.Commanders[20] = State.CommanderState{ID: 20, Name: "The Bold"}
	state.Castellans[30] = State.CastellanState{ID: 30, CastleID: 10}
	state.Inventory.Equipment[40] = State.EquipmentInstance{ID: 40, DefinitionID: 501}
	state.Inventory.Gems[50] = State.GemInstance{ID: 50, DefinitionID: 601, Level: 7}
	state.Inventory.ConstructionItems[301] = 1
	state.Player.ID, state.Player.Name = 60, "Castle Lord"
	state.Alliance = State.AllianceState{ID: 70, Name: "The Citadel"}
	state.Movements[80] = State.MovementState{ID: 80, SourceCastleID: 10, TargetX: 44, TargetY: 55}
	state.EventScores.ByEvent[103] = State.ScalableEventScore{
		EventID: 103, LocalizationKey: "event_title_103", DifficultyID: 1001, DifficultyTypeName: "expertPlus",
	}

	message := strings.Join([]string{
		"castle 10", "commander 20", "castellan 30", "equipment 40", "gem 50",
		"building 1001", "building definition 201", "construction item 301",
		"crafting recipe 401", "unit 489", "tool 614", "resource 1", "currency 2",
		"package 701", "horse 801", "castle 999",
		"kingdom 0", "Hall of Legends skill 901", "camp 902", "achievement 903",
		"reward 904", "effect 905", "player 60", "alliance 70", "movement 80",
		"event 103", "difficulty 1001", "Storm isle 906", "Auto Bird target 10", "gem carrier 40",
	}, "; ")
	got := NewIdentifierLabels(state, store, language).Humanize(message)
	for _, want := range []string{
		"Main Keep", "The Bold", "Castellan of Main Keep", "Relic Armor", "Fire Gem level 7",
		"Bakery level 8", "Bakery Production Item level 3", "Honey level 2",
		"Veteran Crossbowman", "Mead Mace", "Coins", "Rubies", "War Horn Bundle", "Fast Horse",
		"the castle", "Great Empire", "Master Builder", "Berimond Vanguard", "Architect",
		"Honey Reward", "Travel Speed", "Castle Lord", "The Citadel",
		"Movement from Main Keep to 44:55", "War of the Realms", "Expert Plus",
		"Small Wood island level 70", "Auto Bird target Main Keep", "gem carrier Relic Armor",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("humanized message %q does not contain %q", got, want)
		}
	}
	if strings.Contains(strings.ToLower(got), " id ") || strings.Contains(got, "999") {
		t.Fatalf("humanized message still exposes identifiers: %q", got)
	}
}

func TestIdentifierLabelsHideUnknownIDsWithoutChangingBareNumbers(t *testing.T) {
	labels := NewIdentifierLabels(State.NewGameState(), nil, nil)
	message := "attack 12:34 has 50 troops; camp 600:555 is ready; castle 999 and item 888 are unavailable"
	want := "attack 12:34 has 50 troops; camp 600:555 is ready; the castle and item are unavailable"
	if got := labels.Humanize(message); got != want {
		t.Fatalf("unknown identifier cleanup = %q, want %q", got, want)
	}
}

func TestIdentifierLabelsRemoveLegacyIDAnnotationsAndDefinitionIDs(t *testing.T) {
	labels := NewIdentifierLabels(State.NewGameState(), nil, nil)
	message := "Queue 5 stacks of 40 tools definition 614 with The Bold (commander ID 20)"
	if got, want := labels.Humanize(message), "Queue 5 stacks of 40 tools with The Bold"; got != want {
		t.Fatalf("legacy identifier cleanup = %q, want %q", got, want)
	}
}
