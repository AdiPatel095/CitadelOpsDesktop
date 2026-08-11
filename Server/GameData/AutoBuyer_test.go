package GameData

import (
	"strings"
	"testing"
)

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

func TestLocalizedAutoBuyerCatalogUsesOfficialRewardDisplayNames(t *testing.T) {
	store, err := DecodeStore([]byte(`{
		"versionInfo":{"version":{"@value":"test"}},
		"buildings":[{"wodID":2366,"name":"Deco","type":"HorsetailFlag","level":1}],
		"units":[
			{"wodID":715,"name":"Eventunit","type":"Rankrewardrange","comment2":"Rankrewardrange"},
			{"wodID":266,"name":"Workshop","type":"SceatFameShield","comment2":"Armored Siege Tower"}
		],
		"resources":[
			{"resourceID":1,"JSONKey":"W","name":"Wood","assetName":"Wood"},
			{"resourceID":2,"JSONKey":"S","name":"Stone","assetName":"Stone"},
			{"resourceID":3,"JSONKey":"C2","name":"currency2"}
		],
		"currencies":[
			{"currencyID":36,"JSONKey":"STO","Name":"SilverToken","assetName":"SilverToken"},
			{"currencyID":40,"JSONKey":"SCT","Name":"SceatToken","assetName":"SceatToken"},
			{"currencyID":30001,"JSONKey":"MCF","Name":"CommonFinesand","assetName":"CommonFinesand"}
		],
		"constructionItems":[
			{"constructionItemID":30000,"comment1":"Temporary","name":"winterBakery","level":1}
		],
		"equipments":[
			{"equipmentID":541,"setID":63,"slotID":1,"comment2":"Internal robe label"},
			{"equipmentID":542,"setID":63,"slotID":2,"comment2":"Internal blade label"}
		],
		"gems":[
			{"gemID":187,"gemLevelID":8,"wearerID":1,"triggerChance":100,"effects":"13&10"}
		],
		"effects":[{"effectID":13,"name":"defenseUnitAmountWall"}],
		"lootBoxes":[{"lootBoxID":33,"name":"HoL8BoxLittle","rarity":1}],
		"rewardBags":[{
			"bagID":31,"focused":1,"focusedMaterialID":30001,"addCommonFinesand":39,
			"variableCommonBricks":1
		}],
		"packages":[
			{"packageID":200,"packageType":"soldier","comment1":"Internal Soldier","comment2":"Central Silver Shop","stock":1,"costSilverToken":1,"unitID":715,"unitAmount":100},
			{"packageID":201,"packageType":"tool","comment1":"Internal Tool","comment2":"Central Silver Shop","stock":1,"costSilverToken":1,"unitID":266,"unitAmount":5},
			{"packageID":202,"packageType":"constructionItem","comment1":"Internal Build Item","comment2":"Central Silver Shop","stock":1,"costSilverToken":1,"constructionItemID":30000,"constructionItemAmount":1},
			{"packageID":203,"packageType":"deco","comment1":"Internal Decoration","comment2":"Central Silver Shop","stock":1,"costSilverToken":1,"buildingID":2366,"buildingAmount":1},
			{"packageID":204,"packageType":"item","comment1":"Internal Equipment","comment2":"Central Silver Shop","stock":1,"costSilverToken":1,"equipmentIDs":541,"equipmentAmount":1},
			{"packageID":205,"packageType":"gem","comment1":"Internal Gem","comment2":"Central Silver Shop","stock":1,"costSilverToken":1,"gemIDs":187,"gemAmount":1},
			{"packageID":206,"packageType":"lootBox","comment1":"Internal Loot Box","comment2":"Central Silver Shop","stock":1,"packagePriceC2":150,"lootBox":"33+1"},
			{"packageID":207,"packageType":"currency","comment1":"Internal Currency","comment2":"Central Silver Shop","stock":1,"costSilverToken":1,"addSceatToken":20},
			{"packageID":208,"packageType":"resources","comment1":"Internal Resources","comment2":"Central Silver Shop","stock":1,"costSilverToken":1,"amountWood":7000,"amountStone":7000},
			{"packageID":209,"packageType":"rewardBag","comment1":"Internal Bag","comment2":"Central Silver Shop","stock":1,"costSilverToken":1,"rewardBags":"31+1"},
			{"packageID":210,"packageType":"VIP","comment1":"Internal VIP Points","comment2":"Central Silver Shop","stock":1,"costSilverToken":1,"vipPoints":2000},
			{"packageID":211,"packageType":"VIP","comment1":"Internal VIP Time","comment2":"Central Silver Shop","stock":1,"costSilverToken":1,"vipTime":86400},
			{"packageID":212,"packageType":"relicItem","comment1":"Internal Relic","comment2":"Central Silver Shop","stock":1,"costSilverToken":1,"relicEquipments":"1@70,100"},
			{"packageID":213,"packageType":"relicGem","comment1":"Internal Relic Gem","comment2":"Central Silver Shop","stock":1,"costSilverToken":1,"relicEquipments":"3001@70,100"},
			{"packageID":214,"packageType":"packageBundle","comment1":"Internal Bundle","comment2":"Central Silver Shop","stock":1,"costSilverToken":1,"packageIDs":"301,302"},
			{"packageID":301,"packageType":"item","equipmentIDs":541,"equipmentAmount":1},
			{"packageID":302,"packageType":"item","equipmentIDs":542,"equipmentAmount":1}
		],
		"feasts":[]
	}`), SourceMetadata{ItemVersion: "test"})
	if err != nil {
		t.Fatal(err)
	}
	language, err := DecodeLanguage([]byte(`{
		"rankrewardrange_name":"Deathly horror",
		"sceatfameshield_name":"Armored siege tower",
		"ci_appearance_winterBakery":"Winter bakery",
		"deco_HorsetailFlag_name":"Horsetail banner",
		"equipment_unique_541":"Robe of dark rituals",
		"equipment_unique_542":"Sacrificial blade",
		"gem_effect_name_gemDefenseUnitAmountWall_100":"Gem of the rampart: {0}",
		"mysteryBox_boxName_HoL8BoxLittle_1":"Small dragon chest",
		"currency_name_SilverToken":"Silver pieces",
		"currency_name_SceatToken":"Sceat",
		"currency_name_CommonFinesand":"Fine sand",
		"currency_name_Wood":"Wood",
		"currency_name_Stone":"Stone",
		"gold":"Rubies",
		"vipPoints_name":"VIP points",
		"vipTime_name":"VIP time",
		"equipment_set_63":"Apparatus of the Summoner's Apprentice",
		"relicequip_dialog_ArmorGeneral_desc":"Relic armor for commanders",
		"relicequip_dialog_GemGeneral_desc":"Relic gem for commanders"
	}`), LanguageMetadata{Language: "en", Version: "test"})
	if err != nil {
		t.Fatal(err)
	}

	catalog, err := store.LocalizedAutoBuyerCatalog(language)
	if err != nil {
		t.Fatal(err)
	}
	products := map[int64]AutoBuyerPackage{}
	for _, product := range catalog.Packages {
		products[product.PackageID] = product
		expectedPriceName := "Silver pieces"
		if product.PackageID == 206 {
			expectedPriceName = "Rubies"
		}
		if product.Price.Name != expectedPriceName {
			t.Fatalf("package %d price name = %q, want %q", product.PackageID, product.Price.Name, expectedPriceName)
		}
		if strings.Contains(product.Name, "Internal") {
			t.Fatalf("package %d retained developer label %q", product.PackageID, product.Name)
		}
	}
	expected := map[int64]string{
		200: "100 × Deathly horror",
		201: "5 × Armored siege tower",
		202: "Winter bakery (level 1)",
		203: "Horsetail banner",
		204: "Robe of dark rituals",
		205: "Gem of the rampart: 10",
		206: "Small dragon chest",
		207: "20 × Sceat",
		208: "Stone + Wood",
		209: "Fine sand material bag",
		210: "2,000 VIP points",
		211: "1 day of VIP time",
		212: "Relic armor for commanders",
		213: "Relic gem for commanders",
		214: "Apparatus of the Summoner's Apprentice",
	}
	for packageID, expectedName := range expected {
		product, found := products[packageID]
		if !found {
			t.Fatalf("localized package %d was not exposed", packageID)
		}
		if product.Name != expectedName {
			t.Errorf("package %d name = %q, want %q", packageID, product.Name, expectedName)
		}
	}
	if detail := products[208].Detail; detail != "7,000 × Stone · 7,000 × Wood" {
		t.Errorf("resource bundle detail = %q", detail)
	}
	if detail := products[209].Detail; detail != "39 × Fine sand · plus random build-item materials" {
		t.Errorf("material bag detail = %q", detail)
	}
	if detail := products[214].Detail; detail != "2-piece equipment set" {
		t.Errorf("equipment set detail = %q", detail)
	}
}
