package GameData

import "testing"

func TestFortressCatalogResolvesCooldownsDirewolfAndCheapestShopOrder(t *testing.T) {
	store, err := DecodeStore([]byte(`{
		"versionInfo":[],"buildings":[],
		"units":[{"wodID":277,"type":"Elitetinoswolves","name":"Eventunit","comment1":"Nomad Shop"}],
		"effects":[
			{"effectID":2106,"name":"relicSpeedBonus","effectTypeID":15,"capID":1006},
			{"effectID":426,"name":"speedBonus","effectTypeID":15,"capID":99}
		],
		"effectCaps":[{"capID":1006,"maxTotalBonus":100}],
		"globalEffects":[{"ID":10,"globalEffectID":2,"name":"SpeedBoost","effects":"426&60","boostValue":60}],
		"bossdungeons":[
			{"kID":2,"dungeonlevel":21,"cooldownDelay":86400,"playerCooldownDelay":432000},
			{"kID":1,"dungeonlevel":45,"cooldownDelay":86400,"playerCooldownDelay":432000},
			{"kID":3,"dungeonlevel":55,"cooldownDelay":86400,"playerCooldownDelay":432000}
		],
		"resources":[],
		"currencies":[{"currencyID":37,"JSONKey":"KT","Name":"KhanTablet"}],
		"packages":[
			{"packageID":3858,"packageType":"soldier","unitID":277,"unitAmount":100,"stock":50,"sortOrder":34,"costKhanTablet":4680,"comment1":"Nomad EDS Shop (2023) - Khan Tablets"},
			{"packageID":3857,"packageType":"soldier","unitID":277,"unitAmount":100,"stock":50,"sortOrder":33,"costKhanTablet":2340,"comment1":"Nomad EDS Shop (2023) - Khan Tablets"},
			{"packageID":9999,"packageType":"soldier","unitID":49,"unitAmount":100,"stock":50,"costKhanTablet":1,"comment1":"Nomad EDS Shop (2023) - Khan Tablets"}
		],
		"feasts":[]
	}`), SourceMetadata{ItemVersion: "test"})
	if err != nil {
		t.Fatal(err)
	}
	definitions, err := store.KingdomFortressDefinitions()
	if err != nil || len(definitions) != 3 {
		t.Fatalf("fortress definitions = %#v err=%v", definitions, err)
	}
	if definitions[0].KingdomID != 1 || definitions[0].Level != 45 || definitions[0].GlobalCooldownSec != 86400 || definitions[0].PersonalCooldownSec != 432000 {
		t.Fatalf("unexpected first fortress definition: %#v", definitions[0])
	}
	direwolf, err := store.FortressDirewolf()
	if err != nil || direwolf.UnitID != 277 || direwolf.Name != "Direwolf" {
		t.Fatalf("Direwolf = %#v err=%v", direwolf, err)
	}
	packages, err := store.FortressDirewolfPackages()
	if err != nil || len(packages) != 2 {
		t.Fatalf("Direwolf packages = %#v err=%v", packages, err)
	}
	if packages[0].PackageID != 3857 || packages[0].UnitAmount != 100 || packages[0].Price.Amount != 2340 || packages[1].PackageID != 3858 {
		t.Fatalf("Direwolf package order = %#v", packages)
	}
	speed, err := store.FortressSpeed()
	if err != nil || speed.RelicEffectID != 2106 || speed.RelicCapID != 1006 || speed.RelicMaximumPercent != 100 ||
		speed.DailyGlobalEffectID != 2 || speed.DailyEffectID != 426 || speed.DailyBoostPercent != 60 {
		t.Fatalf("fortress speed contract = %#v err=%v", speed, err)
	}
	relic, err := store.FortressRelicSpeed()
	if err != nil || relic.RelicEffectID != 2106 || relic.RelicMaximumPercent != 100 {
		t.Fatalf("fortress Relic speed contract = %#v err=%v", relic, err)
	}
}

func TestFortressRelicSpeedDoesNotRequireTheOptionalGlobalBoosterCatalog(t *testing.T) {
	store, err := DecodeStore([]byte(`{
		"versionInfo":[],"buildings":[],"units":[],
		"effects":[{"effectID":2106,"name":"relicSpeedBonus","effectTypeID":15,"capID":1006}],
		"effectCaps":[{"capID":1006,"maxTotalBonus":100}]
	}`), SourceMetadata{ItemVersion: "test"})
	if err != nil {
		t.Fatal(err)
	}
	relic, err := store.FortressRelicSpeed()
	if err != nil || relic.RelicMaximumPercent != 100 {
		t.Fatalf("independent Fortress Relic speed contract = %#v err=%v", relic, err)
	}
	if _, err := store.FortressSpeed(); err == nil {
		t.Fatal("combined booster contract unexpectedly succeeded without its global-effect catalogs")
	}
}
