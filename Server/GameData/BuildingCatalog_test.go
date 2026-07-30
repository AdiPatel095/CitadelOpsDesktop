package GameData

import "testing"

func TestBuildingCatalogDecodesPlanningFieldsAndCosts(t *testing.T) {
	store, err := DecodeStore([]byte(`{
		"versionInfo":{"version":"test"},
		"units":[],
		"resources":[
			{"resourceID":"1","JSONKey":"C1","name":"currency1"},
			{"resourceID":"2","JSONKey":"C2","name":"currency2"},
			{"resourceID":"3","JSONKey":"W","name":"wood"}
		],
		"currencies":[{"currencyID":"10","JSONKey":"KM","Name":"KhanMedal"}],
		"quests":[{"questID":"99","conditions":"buildings+2+42#expansions+1","requiredLevel":"10","shownKingdomID":"0","triggerKingdomID":"0","xp":"25"}],
		"buildings":[{
			"wodID":"42","name":"TestBuilding","type":"Level1","group":"Building",
			"shopCategory":"CIVIL","buildingGroundType":"CIVIL","level":"1","width":"5","height":"4",
			"rotateType":"1","upgradeWodID":"43","requiredLevel":"12","requiredLegendLevel":"3",
			"maximumCount":"2","kIDs":"0,1","eventIDs":"3#4","onlyInAreaTypes":"1,4",
			"buildDuration":"60","storeable":"1","movable":"0","unlockIDs":"7,8",
			"costWood":"100","costC2":"5","costKhanMedal":"9","Moral":"22"
		}]
	}`), SourceMetadata{ItemVersion: "test"})
	if err != nil {
		t.Fatal(err)
	}
	catalog, err := store.BuildingCatalog()
	if err != nil {
		t.Fatal(err)
	}
	definition, found := catalog.Definition(42)
	if !found {
		t.Fatal("building definition was not indexed")
	}
	if definition.Width != 5 || definition.Height != 4 || definition.GroundType != "CIVIL" ||
		definition.RequiredLevel == nil || *definition.RequiredLevel != 12 || len(definition.EventIDs) != 2 ||
		len(definition.UnlockIDs) != 2 || definition.Storeable == nil || !*definition.Storeable ||
		definition.Movable == nil || *definition.Movable || len(definition.QuestObjectives) != 1 ||
		definition.QuestObjectives[0].QuestID != 99 || definition.QuestObjectives[0].RequiredCount != 2 {
		t.Fatalf("planning fields were not decoded: %#v", definition)
	}
	if definition.Values["Moral"] != 22 {
		t.Fatalf("building values = %#v", definition.Values)
	}
	costs := map[string]BuildingCost{}
	for _, cost := range definition.Costs {
		costs[cost.Field] = cost
	}
	if costs["costWood"].Scope != BuildingCostCastleResource || costs["costWood"].DefinitionID != 3 ||
		costs["costC2"].Scope != BuildingCostPlayerResource || !costs["costC2"].Premium ||
		costs["costKhanMedal"].Scope != BuildingCostCurrency || costs["costKhanMedal"].DefinitionID != 10 {
		t.Fatalf("cost references were not resolved: %#v", costs)
	}
}
