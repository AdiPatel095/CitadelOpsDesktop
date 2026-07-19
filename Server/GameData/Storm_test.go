package GameData

import "testing"

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
