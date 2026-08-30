package GameData

import "testing"

func TestStormFortAttacksRemainingUsesOfficialMaximum(t *testing.T) {
	store, err := DecodeStore([]byte(`{
		"versionInfo":[],"buildings":[],"units":[],
		"isles":[{"IsleID":7,"type":"DUNGEON","dungeonlevel":60,"maxCountVictories":10,"countVictories":"0#1#2#3#4#5#6#7#8#9"}]
	}`), SourceMetadata{ItemVersion: "test"})
	if err != nil {
		t.Fatal(err)
	}
	definition, found := store.StormIsle(7)
	if !found || definition.MaximumVictoryCount != 10 {
		t.Fatalf("Storm fort definition = %#v", definition)
	}
	remaining, known := StormFortAttacksRemaining(definition, 7)
	if !known || remaining != 3 {
		t.Fatalf("remaining attacks = %d known=%t, want 3 true", remaining, known)
	}
}

func TestStormIsleViewIsCachedAndPublicResultIsDefensive(t *testing.T) {
	store, err := DecodeStore([]byte(`{
		"versionInfo":[],"buildings":[],"units":[],
		"isles":[{"IsleID":7,"type":"DUNGEON","dungeonlevel":60,"countVictories":"0#1#2"}]
	}`), SourceMetadata{ItemVersion: "test"})
	if err != nil {
		t.Fatal(err)
	}
	first, found := store.StormIsleView(7)
	if !found || len(first.VictoryCounts) != 3 {
		t.Fatalf("cached Storm definition = %#v", first)
	}
	second, found := store.StormIsleView(7)
	if !found || &first.VictoryCounts[0] != &second.VictoryCounts[0] {
		t.Fatal("Storm view did not reuse its immutable decoded slice")
	}
	public, found := store.StormIsle(7)
	if !found {
		t.Fatal("public Storm definition is missing")
	}
	public.VictoryCounts[0] = 99
	unchanged, _ := store.StormIsleView(7)
	if unchanged.VictoryCounts[0] != 0 {
		t.Fatalf("public mutation reached immutable Storm view: %#v", unchanged.VictoryCounts)
	}
}

func TestStormShopPackagesOnlyIncludeCurrentLiveInventory(t *testing.T) {
	store, err := DecodeStore([]byte(`{
		"versionInfo":[],
		"buildings":[],
		"units":[],
		"packages":[
			{"packageID":244,"comment1":"Bodkin arrowheads","comment2":"Luna's trade boat","packageType":"tool","packagePriceAquamarine":1520},
			{"packageID":245,"comment1":"War horn","comment2":"Luna's trade boat","packageType":"tool","packagePriceAquamarine":2960},
			{"packageID":3119,"comment1":"Silver Coins","comment2":"Luna's trade boat","packageType":"currency","packagePriceAquamarine":10000,"stock":3},
			{"packageID":2798,"comment1":"Soft Currency - Expensive Unlimited","comment2":"Luna's trade boat","packageType":"currency","packagePriceAquamarine":75000,"amountC1":2000000}
		]
	}`), SourceMetadata{ItemVersion: "test"})
	if err != nil {
		t.Fatal(err)
	}

	packages := store.StormShopPackages()
	if len(packages) != 3 || packages[0].ProductID != 3119 || packages[1].ProductID != 245 || packages[2].ProductID != 2798 {
		t.Fatalf("current Luna packages = %#v", packages)
	}
	if _, found := store.StormShopPackage(244); found {
		t.Fatal("historical Luna package 244 was accepted")
	}
}

func TestStormCastleOptionsUseOfficialKingdomAndLevel(t *testing.T) {
	store, err := DecodeStore([]byte(`{
		"versionInfo":[],"buildings":[],"units":[],
		"prebuiltcastles":[
			{"preBuiltCastleID":"16","comment2":"CheapCamp","spaceIDs":"4","minLevel":35,"costWood":10000,"costStone":10000,"costFood":2500,"costC1":5000},
			{"preBuiltCastleID":"18","comment2":"C2Camp","spaceIDs":"4","minLevel":35,"costC2":59000},
			{"preBuiltCastleID":"99","comment2":"Other","spaceIDs":"10","minLevel":1,"costWood":1},
			{"preBuiltCastleID":"100","comment2":"HighLevel","spaceIDs":"4","minLevel":70,"costWood":1}
		]
	}`), SourceMetadata{ItemVersion: "test"})
	if err != nil {
		t.Fatal(err)
	}
	options := store.StormCastleOptions(69)
	if len(options) != 2 || options[0].ID != 16 || options[0].CostCoins != 5000 ||
		options[1].ID != 18 || options[1].CostPremium != 59000 {
		t.Fatalf("Storm castle options = %#v", options)
	}
	if _, found := store.StormCastleOption(100, 69); found {
		t.Fatal("level-locked Storm castle was accepted")
	}
}
