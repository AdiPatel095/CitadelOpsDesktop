package GameData

import "testing"

func TestDefenseToolShopPackagesRejectRubyPricesAndDecodeNonPremiumCurrencies(t *testing.T) {
	store, err := DecodeStore([]byte(`{
		"versionInfo":{"version":{"@value":"test"}},
		"buildings":[],
		"units":[],
		"currencies":[
			{"currencyID":1,"JSONKey":"KT","Name":"KhanTablet"},
			{"currencyID":36,"JSONKey":"STO","Name":"SilverToken"}
		],
		"packages":[
			{"packageID":10,"packageType":"tool","unitID":731,"unitAmount":1,"packagePriceC1":25},
			{"packageID":11,"packageType":"tool","unitID":731,"unitAmount":5,"costKhanTablet":20},
			{"packageID":12,"packageType":"tool","unitID":731,"unitAmount":100,"costSilverToken":17,"notRebuyable":1},
			{"packageID":13,"packageType":"tool","unitID":731,"unitAmount":1,"packagePriceC2":1},
			{"packageID":14,"packageType":"tool","unitID":731,"unitAmount":1,"packagePriceC1":10,"packagePriceC2":2},
			{"packageID":15,"packageType":"tool","unitID":731,"unitAmount":1,"costUnknownToken":3}
		]
	}`), SourceMetadata{})
	if err != nil {
		t.Fatal(err)
	}
	packages, err := store.DefenseToolShopPackages(731)
	if err != nil {
		t.Fatal(err)
	}
	if len(packages) != 3 {
		t.Fatalf("packages = %#v", packages)
	}
	if packages[0].PackageID != 10 || packages[0].PriceScope != DefenseToolPricePlayerResource || packages[0].PriceID != 1 {
		t.Fatalf("coin package = %#v", packages[0])
	}
	if packages[1].PackageID != 11 || packages[1].PriceScope != DefenseToolPriceCurrency || packages[1].PriceID != 1 {
		t.Fatalf("Khan package = %#v", packages[1])
	}
	if packages[2].PackageID != 12 || packages[2].PriceID != 36 || packages[2].Stock != 1 {
		t.Fatalf("one-time Silver package = %#v", packages[2])
	}
}
