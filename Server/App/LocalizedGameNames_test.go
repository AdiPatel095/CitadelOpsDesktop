package App

import (
	"CitadelDesktop/Server/GameData"
	"CitadelDesktop/Server/Intent"
	"CitadelDesktop/Server/Localization"
	"strings"
	"testing"
)

func TestSpecialistWholeMessageVariantsRetainTypedAmounts(t *testing.T) {
	for _, id := range []int{0, 1, 2, 3, 4, 5, 6, 8, 10} {
		original := Localization.New("server.app.renew_p_by_days.310dce39", "Renew {p0} by 7 days within a {p1}-ruby ceiling", Localization.Params{"p0": "custom fallback", "p1": 625})
		result := specialistNameDescriptor(original, GameData.AutoBuyerSpecialist{ID: id})
		if result == nil || strings.Contains(result.Fallback, "{p0}") || result.Params["p1"] != 625 {
			t.Fatalf("variant %d: %+v", id, result)
		}
		if _, ok := result.Params["p0"]; ok {
			t.Fatal("obsolete custom noun argument retained")
		}
		if original.Params["p0"] != "custom fallback" {
			t.Fatal("source descriptor mutated")
		}
	}
	if specialistNameDescriptor(Localization.New("server.app.renew_p_by_days.310dce39", "source", nil), GameData.AutoBuyerSpecialist{ID: 999}) != nil {
		t.Fatal("unknown role masked")
	}
}

func TestConstructionItemNameKeepsLevelInOuterTemplate(t *testing.T) {
	store, err := GameData.DecodeStore([]byte(`{"versionInfo":[],"buildings":[],"units":[],"constructionItems":[{"constructionItemID":301,"name":"bakeryBoost","comment1":"secondary","level":3}]}`), GameData.SourceMetadata{})
	if err != nil {
		t.Fatal(err)
	}
	language, err := GameData.DecodeLanguage([]byte(`{"ci_primary_bakeryBoost":"Primary","ci_secondary_bakeryBoost":"Bakery item"}`), GameData.LanguageMetadata{})
	if err != nil {
		t.Fatal(err)
	}
	result := constructionPurchaseDescriptor(Intent.PlanningContext{GameData: store, Language: language}, 301, "Bakery item", 3, 2, "Castle {admin}")
	if result.GameParams["item"].Key != "ci_secondary_bakeryBoost" || result.Params["level"] != int64(3) || result.Params["castle"] != "Castle {admin}" {
		t.Fatalf("lost identity/level/user content: %+v", result)
	}
	if strings.Contains(result.GameParams["item"].Fallback, "level") || !strings.Contains(result.Fallback, "{level, number}") {
		t.Fatal("level mixed into atomic noun")
	}
}

func TestUnitPackageDescriptorKeepsBothQuantities(t *testing.T) {
	store, err := GameData.DecodeStore([]byte(`{"versionInfo":[],"buildings":[],"units":[{"wodID":489,"name":"elitecrossbowman"}],"resources":[{"resourceID":2,"name":"currency2","JSONKey":"C2"}]}`), GameData.SourceMetadata{})
	if err != nil {
		t.Fatal(err)
	}
	language, err := GameData.DecodeLanguage([]byte(`{"elitecrossbowman_name":"Veteran Crossbowman","gold":"Rubies"}`), GameData.LanguageMetadata{})
	if err != nil {
		t.Fatal(err)
	}
	input := Intent.PlanningContext{GameData: store, Language: language}
	product := GameData.AutoBuyerPackage{PackageType: "soldier", UnitID: 489, UnitAmount: 25, Name: "Opaque package comment", Price: GameData.AutoBuyerPrice{ResourceID: 2, Name: "Rubies", Amount: 30}}
	result := packagePurchaseDescriptor(input, product, 2)
	if result == nil || result.Params["amount"] != int64(2) || result.Params["unitAmount"] != int64(25) || result.Params["cost"] != int64(60) || result.GameParams["unit"].Key != "elitecrossbowman_name" || result.GameParams["currency"].Key != "gold" {
		t.Fatalf("lost package semantics: %+v", result)
	}
	for _, kind := range []string{"packagebundle", "unknown", ""} {
		product.PackageType = kind
		if packagePurchaseDescriptor(input, product, 2) != nil {
			t.Fatalf("%q with unit fields presented as atomic", kind)
		}
	}
	product.PackageType = "tool"
	if packagePurchaseDescriptor(input, product, 2) == nil {
		t.Fatal("supported tool package rejected")
	}
	product.UnitID = 0
	if packagePurchaseDescriptor(input, product, 2) != nil {
		t.Fatal("opaque bundle presented as translated")
	}
}

func TestConstructionItemMissingKeyUsesExplicitIDTemplate(t *testing.T) {
	for _, level := range []int64{0, 3} {
		message := constructionPurchaseDescriptor(Intent.PlanningContext{}, 301, "Untranslated name", level, 2, "Castle {admin}")
		if message == nil || message.Params["itemID"] != "301" || message.Params["castle"] != "Castle {admin}" || strings.Contains(message.Fallback, "{item}") {
			t.Fatalf("missing-key noun hidden: %+v", message)
		}
		if _, ok := message.Params["item"]; ok {
			t.Fatal("English noun retained in primitive params")
		}
		if level > 0 && message.Params["level"] != level {
			t.Fatal("level omitted")
		}
	}
}

func TestDefenseToolPriceDescriptorRequiresScopedOfficialIdentity(t *testing.T) {
	store, err := GameData.DecodeStore([]byte(`{"versionInfo":[],"buildings":[],"units":[],"resources":[{"resourceID":1,"name":"currency1","JSONKey":"C1"}],"currencies":[{"currencyID":7,"Name":"Medals"}]}`), GameData.SourceMetadata{})
	if err != nil {
		t.Fatal(err)
	}
	language, err := GameData.DecodeLanguage([]byte(`{"currency_name_currency1":"Coins","currency_name_Medals":"Medals"}`), GameData.LanguageMetadata{})
	if err != nil {
		t.Fatal(err)
	}
	input := Intent.PlanningContext{GameData: store, Language: language}
	for _, tc := range []struct {
		scope string
		id    int64
		key   string
	}{
		{GameData.DefenseToolPricePlayerResource, 1, "currency_name_currency1"},
		{GameData.DefenseToolPriceCastleResource, 1, "currency_name_currency1"},
		{GameData.DefenseToolPriceCurrency, 7, "currency_name_Medals"},
		{"unknown", 1, ""}, {GameData.DefenseToolPriceCurrency, 999, ""},
	} {
		message := Localization.New("test.price", "Balance: {price}", Localization.Params{"price": "legacy"})
		result := defenseToolPriceDescriptor(message, input, "price", GameData.DefenseToolShopPackage{PriceScope: tc.scope, PriceID: tc.id, PriceName: "legacy"})
		if tc.key == "" {
			if result != nil {
				t.Fatalf("unknown noun masked: %+v", result)
			}
			continue
		}
		if result == nil || result.GameParams["price"].Key != tc.key {
			t.Fatalf("scope %s: %+v", tc.scope, result)
		}
	}
}
