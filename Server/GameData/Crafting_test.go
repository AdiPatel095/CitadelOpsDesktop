package GameData

import (
	"encoding/json"
	"testing"
)

func TestCraftingResourceNameUsesOfficialCurrencyLocalization(t *testing.T) {
	language, err := DecodeLanguage([]byte(`{
		"currency_name_refinedLumber":"Refined wood",
		"currency_name_sceatToken":"Sceat",
		"currency_name_currency1":"Coins",
		"subscription_rubies":"Rubies",
		"wood":"Wood"
	}`), LanguageMetadata{Language: "en", Version: "test"})
	if err != nil {
		t.Fatal(err)
	}
	manager := &Manager{language: language}

	tests := []struct {
		internalName string
		jsonKey      string
		field        string
		want         string
	}{
		{internalName: "RefinedLumber", jsonKey: "RL", field: "Name", want: "Refined wood"},
		{internalName: "SceatToken", jsonKey: "STP", field: "Name", want: "Sceat"},
		{internalName: "currency1", jsonKey: "C1", field: "name", want: "Coins"},
		{internalName: "currency2", jsonKey: "C2", field: "name", want: "Rubies"},
		{internalName: "wood", jsonKey: "W", field: "name", want: "Wood"},
		{internalName: "LegendaryMaterial", jsonKey: "LM", field: "Name", want: "Legendary Material"},
	}

	for _, test := range tests {
		t.Run(test.internalName, func(t *testing.T) {
			record := Record{
				test.field: json.RawMessage(`"` + test.internalName + `"`),
				"JSONKey":  json.RawMessage(`"` + test.jsonKey + `"`),
			}
			if got := manager.craftingResourceName(record, test.internalName, test.jsonKey); got != test.want {
				t.Fatalf("crafting resource name = %q, want %q", got, test.want)
			}
		})
	}
}

func TestCraftingRecipeCostsResolveNamedFieldsToLiveResourceIDs(t *testing.T) {
	store, err := DecodeStore([]byte(`{
		"versionInfo":[],"buildings":[],"units":[],
		"resources":[
			{"resourceID":8,"name":"glass","JSONKey":"G"},
			{"resourceID":10,"name":"iron","JSONKey":"I"}
		],
		"craftingRecipes":[{
			"craftingRecipeId":110,"costGlass":21200,"costIron":19200
		}]
	}`), SourceMetadata{ItemVersion: "test"})
	if err != nil {
		t.Fatal(err)
	}
	costs, err := CraftingRecipeCosts(store, 110)
	if err != nil {
		t.Fatal(err)
	}
	if len(costs) != 2 || costs[0].ResourceID != 8 || costs[0].JSONKey != "G" || costs[0].Amount != 21200 ||
		costs[1].ResourceID != 10 || costs[1].JSONKey != "I" || costs[1].Amount != 19200 {
		t.Fatalf("crafting costs = %#v", costs)
	}
	cached, err := CraftingRecipeCostsView(store, 110)
	if err != nil || len(cached) != 2 {
		t.Fatalf("cached crafting costs = %#v err=%v", cached, err)
	}
	definition, found := store.CraftingRecipeView(110)
	if !found || len(definition.Costs) != 2 {
		t.Fatalf("cached crafting definition = %#v", definition)
	}
	costs[0].Amount = 0
	unchanged, _ := CraftingRecipeCostsView(store, 110)
	if unchanged[0].Amount != 21200 {
		t.Fatalf("public mutation reached cached crafting costs: %#v", unchanged)
	}
}

func TestResourceLookupsReuseOfficialProjection(t *testing.T) {
	store, err := DecodeStore([]byte(`{
		"versionInfo":[],"buildings":[],"units":[],
		"resources":[
			{"resourceID":1,"name":"coins","JSONKey":"C1"},
			{"resourceID":8,"name":"glass","JSONKey":"G"}
		],
		"currencies":[{"currencyID":9,"Name":"token","JSONKey":"STP"}]
	}`), SourceMetadata{ItemVersion: "test"})
	if err != nil {
		t.Fatal(err)
	}
	if id, found := store.ResourceIDForJSONKey("c1"); !found || id != 1 {
		t.Fatalf("C1 resource = %d found=%t", id, found)
	}
	if key, found := store.ResourceJSONKey(8); !found || key != "G" {
		t.Fatalf("resource 8 key = %q found=%t", key, found)
	}
	if id, found := store.CurrencyIDForJSONKey("stp"); !found || id != 9 {
		t.Fatalf("STP currency = %d found=%t", id, found)
	}
}
