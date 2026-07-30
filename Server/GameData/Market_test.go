package GameData

import "testing"

func TestMarketProjectionUsesOfficialCapacityDefinitions(t *testing.T) {
	store, err := DecodeStore([]byte(`{
		"versionInfo":[],
		"buildings":[{"wodID":137,"name":"Market","marketCarriages":5}],
		"units":[{"wodID":1}],
		"constructionItems":[{"constructionItemID":205,"marketCarriages":7}],
		"levelBoosters":[{"boosterType":11,"level":1,"boostPercentage":7}],
		"effects":[
			{"effectID":90,"name":"marketCarriageCapacityBoost"},
			{"effectID":91,"name":"marketCarriageCapacityBonus"}
		]
	}`), SourceMetadata{ItemVersion: "test"})
	if err != nil {
		t.Fatal(err)
	}
	if barrows, err := store.MarketBarrowsForBuilding(137); err != nil || barrows != 5 {
		t.Fatalf("building barrows = %d, err=%v", barrows, err)
	}
	if barrows, err := store.MarketBarrowsForConstructionItem(205); err != nil || barrows != 7 {
		t.Fatalf("construction barrows = %d, err=%v", barrows, err)
	}
	capacity, err := store.MarketCapacity(1, []MarketEffect{
		{EffectID: 90, Values: []float64{15}},
		{EffectID: 91, Values: []float64{5}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if capacity.CaravanBoostPercent != 7 || capacity.AreaCapacityBoostPercent != 15 || capacity.CapacityBonus != 5 || capacity.CapacityPerBarrow != 129 {
		t.Fatalf("unexpected capacity: %+v", capacity)
	}
}
