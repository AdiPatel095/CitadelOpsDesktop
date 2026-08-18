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

func TestBuildingCatalogImmutableViewsCachePathsAndKeepPublicCopiesDefensive(t *testing.T) {
	catalog := testBuildingPathCatalog()
	first, found := catalog.UpgradePathView(1, 3)
	if !found || len(first) != 3 {
		t.Fatalf("upgrade path = %#v, found %t", first, found)
	}
	second, found := catalog.UpgradePathView(1, 3)
	if !found || &first[0] != &second[0] {
		t.Fatal("immutable upgrade path was not reused")
	}
	construction, found := catalog.ConstructionPathView(3)
	if !found || len(construction) != 3 || construction[0].ID != 1 {
		t.Fatalf("construction path = %#v, found %t", construction, found)
	}

	publicPath, found := catalog.UpgradePath(1, 3)
	if !found {
		t.Fatal("public upgrade path was not found")
	}
	publicPath[0].Values["capacity"] = 999
	publicPath[0].Costs[0].Amount = 999
	publicAgain, _ := catalog.UpgradePath(1, 3)
	if publicAgain[0].Values["capacity"] != 1 || publicAgain[0].Costs[0].Amount != 10 {
		t.Fatalf("public path mutation reached catalog data: %#v", publicAgain[0])
	}

	definition, found := catalog.Definition(1)
	if !found {
		t.Fatal("public definition was not found")
	}
	definition.Values["capacity"] = 777
	view, _ := catalog.DefinitionView(1)
	if view.Values["capacity"] != 1 {
		t.Fatalf("public definition mutation reached immutable view: %#v", view.Values)
	}
}

func testBuildingPathCatalog() *BuildingCatalog {
	definitions := []BuildingDefinition{
		{ID: 1, UpgradeDefinitionID: 2, Values: map[string]float64{"capacity": 1}, Costs: []BuildingCost{{Amount: 10}}},
		{ID: 2, UpgradeDefinitionID: 3, DowngradeDefinitionID: 1, Values: map[string]float64{"capacity": 2}},
		{ID: 3, DowngradeDefinitionID: 2, Values: map[string]float64{"capacity": 3}},
	}
	byID := make(map[int64]BuildingDefinition, len(definitions))
	for _, definition := range definitions {
		byID[definition.ID] = definition
	}
	return &BuildingCatalog{definitions: definitions, byID: byID}
}

func BenchmarkBuildingCatalogDefinitionClone(benchmark *testing.B) {
	catalog := testBuildingPathCatalog()
	benchmark.ReportAllocs()
	for benchmark.Loop() {
		if _, found := catalog.Definition(1); !found {
			benchmark.Fatal("definition missing")
		}
	}
}

func BenchmarkBuildingCatalogDefinitionView(benchmark *testing.B) {
	catalog := testBuildingPathCatalog()
	benchmark.ReportAllocs()
	for benchmark.Loop() {
		if _, found := catalog.DefinitionView(1); !found {
			benchmark.Fatal("definition missing")
		}
	}
}

func BenchmarkBuildingCatalogUpgradePathClone(benchmark *testing.B) {
	catalog := testBuildingPathCatalog()
	benchmark.ReportAllocs()
	for benchmark.Loop() {
		if _, found := catalog.UpgradePath(1, 3); !found {
			benchmark.Fatal("path missing")
		}
	}
}

func BenchmarkBuildingCatalogUpgradePathView(benchmark *testing.B) {
	catalog := testBuildingPathCatalog()
	if _, found := catalog.UpgradePathView(1, 3); !found {
		benchmark.Fatal("path missing")
	}
	benchmark.ReportAllocs()
	benchmark.ResetTimer()
	for benchmark.Loop() {
		if _, found := catalog.UpgradePathView(1, 3); !found {
			benchmark.Fatal("path missing")
		}
	}
}
