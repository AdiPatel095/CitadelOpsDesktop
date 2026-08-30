package GameData

import "testing"

func TestAutomationCatalogsAreSharedTypedProjections(t *testing.T) {
	store, err := DecodeStore([]byte(`{
		"versionInfo":[],"buildings":[],
		"units":[{"wodID":10},{"wodID":11,"slotTypes":"tool"}],
		"constructionItems":[
			{"constructionItemID":101,"constructionItemGroupID":7,"name":"Food","level":1,"slotTypeID":2,"duration":3600,"effects":"5&10"},
			{"constructionItemID":102,"constructionItemGroupID":7,"name":"Food","level":2,"slotTypeID":2,"duration":3600,"effects":"5&20"}
		],
		"subscriptionsBuffs":[{"subscriptionTypeID":"3","effects":"189&5,200&+7"}]
	}`), SourceMetadata{})
	if err != nil {
		t.Fatal(err)
	}
	first, err := store.ConstructionItemCatalog()
	if err != nil {
		t.Fatal(err)
	}
	second, err := store.ConstructionItemCatalog()
	if err != nil || first != second {
		t.Fatalf("construction catalog was not shared: first=%p second=%p err=%v", first, second, err)
	}
	definition, found := first.DefinitionView(101)
	if !found || definition.GroupID != 7 || definition.Level != 1 || definition.Slot != 2 || !definition.Temporary {
		t.Fatalf("construction definition = %#v found=%t", definition, found)
	}
	if tiers := first.TiersView(definition.VariantKey); len(tiers) != 2 || tiers[0].ID != 102 || tiers[1].ID != 101 {
		t.Fatalf("construction tiers = %#v", tiers)
	}
	if isTool, found := store.UnitIsTool(10); !found || isTool {
		t.Fatalf("combat unit kind = tool %t found %t", isTool, found)
	}
	if isTool, found := store.UnitIsTool(11); !found || !isTool {
		t.Fatalf("tool unit kind = tool %t found %t", isTool, found)
	}
	effects := store.SubscriptionEffectsView(3)
	if len(effects) != 2 || effects[0] != (SubscriptionEffect{ID: 189, Value: 5}) ||
		effects[1] != (SubscriptionEffect{ID: 200, Value: 7, Decorated: true}) {
		t.Fatalf("subscription effects = %#v", effects)
	}
}
