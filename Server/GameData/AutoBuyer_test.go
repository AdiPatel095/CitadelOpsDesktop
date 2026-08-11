package GameData

import "testing"

func TestAutoBuyerCatalogOnlyExposesBoundedUnambiguousPurchases(t *testing.T) {
	store, err := DecodeStore([]byte(`{
		"versionInfo":{"version":{"@value":"test"}},
		"buildings":[],
		"units":[],
		"resources":[
			{"resourceID":1,"JSONKey":"C1","name":"Coins"},
			{"resourceID":2,"JSONKey":"C2","name":"Rubies"},
			{"resourceID":5,"JSONKey":"F","name":"Food","assetName":"Food"}
		],
		"currencies":[
			{"currencyID":36,"JSONKey":"STO","Name":"SilverToken"},
			{"currencyID":37,"JSONKey":"KT","Name":"KhanTablet"},
			{"currencyID":70,"JSONKey":"RCO","Name":"RiftCoin"}
		],
		"packages":[
			{"packageID":100,"comment1":"Central Silver Shop","comment2":"Weekly chest","stock":5,"costSilverToken":10},
			{"packageID":101,"comment1":"Master Blacksmith - Ruby Shop","comment2":"Ruby chest","stock":2,"packagePriceC2":150},
			{"packageID":102,"comment1":"ARE Blacksmith - Rift Coin Package","comment2":"Rift chest","stock":1,"costRiftCoin":25},
			{"packageID":103,"comment1":"Nomad EDS Shop","comment2":"Nomad Invasion Vendor","stock":3,"costKhanTablet":40},
			{"packageID":104,"comment1":"Merchant food","comment2":"Traveling Merchant weekly","stock":2,"packagePriceFood":500},
			{"packageID":105,"comment1":"Master Blacksmith unlimited","stock":0,"costSilverToken":1},
			{"packageID":106,"comment1":"Central Gold Shop","stock":1,"costSilverToken":1,"packagePriceC2":1},
			{"packageID":107,"comment1":"Central Gold Shop","stock":1,"costUnknown":1}
		],
		"feasts":[
			{"feastID":0,"comment":"Food feast","duration":21600,"productionBoost":80,"costFood":80000},
			{"feastID":1,"comment":"Ruby feast","duration":21600,"productionBoost":120,"costC2":250},
			{"feastID":2,"comment":"Ambiguous feast","duration":21600,"costFood":1,"costC2":1}
		]
	}`), SourceMetadata{ItemVersion: "test"})
	if err != nil {
		t.Fatal(err)
	}

	catalog, err := store.AutoBuyerCatalog()
	if err != nil {
		t.Fatal(err)
	}
	if len(catalog.Packages) != 5 {
		t.Fatalf("supported packages = %#v", catalog.Packages)
	}
	master, found := store.AutoBuyerPackage(AutoBuyerShopMasterBlacksmith, 100)
	if !found || master.TableID != AutoBuyerMasterBlacksmithTableID || master.Price.Scope != AutoBuyerPriceCurrency ||
		master.Price.CurrencyID != 36 || master.Price.Amount != 10 || master.RequiresEvent || master.Name != "Weekly chest" {
		t.Fatalf("Master Blacksmith package = %#v found=%t", master, found)
	}
	premium, found := store.AutoBuyerPackage(AutoBuyerShopMasterBlacksmith, 101)
	if !found || !premium.Price.Premium || premium.Price.Scope != AutoBuyerPricePlayerResource || premium.Price.ResourceID != 2 {
		t.Fatalf("ruby package = %#v found=%t", premium, found)
	}
	rift, found := store.AutoBuyerPackage(AutoBuyerShopRift, 102)
	if !found || !rift.RequiresEvent || rift.TableID != AutoBuyerMasterBlacksmithTableID || rift.Price.CurrencyID != 70 {
		t.Fatalf("Rift package = %#v found=%t", rift, found)
	}
	nomad, found := store.AutoBuyerPackage(AutoBuyerShopNomad, 103)
	if !found || !nomad.RequiresEvent || nomad.TableID != AutoBuyerNomadTableID {
		t.Fatalf("Nomad package = %#v found=%t", nomad, found)
	}
	merchant, found := store.AutoBuyerPackage(AutoBuyerShopTravelingMerchant, 104)
	if !found || merchant.Price.Scope != AutoBuyerPriceCastleResource || merchant.Price.ResourceID != 5 {
		t.Fatalf("Traveling Merchant package = %#v found=%t", merchant, found)
	}
	for _, packageID := range []int64{105, 106, 107} {
		if _, found := store.AutoBuyerPackage(AutoBuyerShopMasterBlacksmith, packageID); found {
			t.Fatalf("unsafe package %d was exposed", packageID)
		}
	}
	if len(catalog.Feasts) != 2 {
		t.Fatalf("supported feasts = %#v", catalog.Feasts)
	}
	food, found := store.AutoBuyerFeast(0)
	if !found || food.DurationSec != 21600 || food.Price.ResourceID != 5 || food.Price.Premium {
		t.Fatalf("food feast = %#v found=%t", food, found)
	}
	ruby, found := store.AutoBuyerFeast(1)
	if !found || !ruby.Price.Premium || ruby.Price.Amount != 250 {
		t.Fatalf("ruby feast = %#v found=%t", ruby, found)
	}
	if catalog.TimedOffers.Supported || catalog.TimedOffers.Reason == "" {
		t.Fatalf("timed-offer capability must fail closed: %#v", catalog.TimedOffers)
	}
}

func TestAutoBuyerSpecialistsAreFixedSevenDayRubyRenewals(t *testing.T) {
	specialists := autoBuyerSpecialists()
	if len(specialists) != 9 {
		t.Fatalf("specialists = %#v", specialists)
	}
	for _, specialist := range specialists {
		if specialist.DurationSec != 7*24*60*60 || specialist.BaseRubyCost <= 0 || specialist.Opcode == "" {
			t.Fatalf("invalid specialist = %#v", specialist)
		}
	}
}
