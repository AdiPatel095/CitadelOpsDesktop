package GameData

import "testing"

func TestBerimondArmorerAttackToolsContainOnlyCapturedCoinProducts(t *testing.T) {
	store, err := DecodeStore([]byte(`{
		"versionInfo":[],
		"buildings":[],
		"units":[
			{"wodID":611,"typ":"Attack"},
			{"wodID":614,"typ":"Attack"},
			{"wodID":617,"typ":"Attack"},
			{"wodID":620,"typ":"Attack"},
			{"wodID":626,"typ":"Defence"}
		],
		"packages":[
			{"packageID":28,"packageType":"tool","comment1":"Scaling ladder","comment2":"Armorer","packagePriceC1":220,"unitID":614,"unitAmount":1,"minLevel":4},
			{"packageID":32,"packageType":"tool","comment1":"Battering ram","comment2":"Armorer","packagePriceC1":430,"unitID":611,"unitAmount":1,"minLevel":4},
			{"packageID":34,"packageType":"tool","comment1":"Castle gate reinforcement","comment2":"Armorer","packagePriceC1":1720,"unitID":626,"unitAmount":1,"minLevel":3},
			{"packageID":36,"packageType":"tool","comment1":"Mantlet","comment2":"Armorer","packagePriceC1":810,"unitID":620,"unitAmount":1,"minLevel":17},
			{"packageID":40,"packageType":"tool","comment1":"Wood bundle","comment2":"Armorer","packagePriceC1":1610,"unitID":617,"unitAmount":1,"minLevel":38},
			{"packageID":99,"packageType":"tool","comment1":"Ruby ladder","comment2":"Armorer","packagePriceC2":1,"unitID":614,"unitAmount":1}
		]
	}`), SourceMetadata{ItemVersion: "test"})
	if err != nil {
		t.Fatal(err)
	}
	items, err := store.BerimondArmorerAttackTools()
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 3 {
		t.Fatalf("armorer tools = %#v, want exactly three", items)
	}
	expected := []struct {
		toolID    int64
		packageID int64
		price     int64
	}{
		{BerimondScalingLadderToolID, 28, 220},
		{BerimondBatteringRamToolID, 32, 430},
		{BerimondMantletToolID, 36, 810},
	}
	for index, want := range expected {
		got := items[index]
		if got.ToolID != want.toolID || got.PackageID != want.packageID || got.CoinPrice != want.price {
			t.Fatalf("tool %d = %#v, want tool=%d package=%d price=%d", index, got, want.toolID, want.packageID, want.price)
		}
	}
	if _, found := store.BerimondArmorerAttackTool(617); found {
		t.Fatal("wood bundle must not be exposed as an Auto Beri purchasable tool")
	}
	if _, found := store.BerimondArmorerAttackTool(626); found {
		t.Fatal("defense gate reinforcement must not be exposed as an Auto Beri purchasable tool")
	}
}
