package GameData

import "testing"

func TestExpansionCatalogIndexesSpaceLevelAndAlternativeCosts(t *testing.T) {
	store, err := DecodeStore([]byte(`{
		"versionInfo":[],"buildings":[],"units":[],
		"resources":[
			{"resourceID":"2","JSONKey":"C2","name":"currency2"},
			{"resourceID":"3","JSONKey":"W","name":"wood"},
			{"resourceID":"4","JSONKey":"S","name":"stone"}
		],
		"currencies":[],
		"expansions":[
			{"expansionID":"45","spaceIDs":"1,3","expansionLevel":"7","costWood":"7092","costStone":"7092","costC2":"1400"},
			{"expansionID":"112","spaceIDs":"4","expansionLevel":"7","costWood":"5914","costStone":"5914","costC2":"1400"}
		]
	}`), SourceMetadata{ItemVersion: "test"})
	if err != nil {
		t.Fatal(err)
	}
	catalog, err := store.ExpansionCatalog()
	if err != nil {
		t.Fatal(err)
	}
	shared, found := catalog.Definition(3, 7)
	if !found || shared.ID != 45 || len(shared.SpaceIDs) != 2 {
		t.Fatalf("shared expansion = %#v", shared)
	}
	storm, found := catalog.Definition(4, 7)
	if !found || storm.ID != 112 || len(storm.Costs) != 3 {
		t.Fatalf("storm expansion = %#v", storm)
	}
	costs := map[string]BuildingCost{}
	for _, cost := range storm.Costs {
		costs[cost.Key] = cost
	}
	if costs["W"].DefinitionID != 3 || costs["S"].DefinitionID != 4 || !costs["C2"].Premium {
		t.Fatalf("expansion costs = %#v", costs)
	}
}
