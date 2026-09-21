package GameData

import "testing"

func TestDefinitionNameKeyUsesIdentityNotRenderedEnglish(t *testing.T) {
	store, err := DecodeStore([]byte(`{"versionInfo":[],"buildings":[{"wodID":201,"name":"bakery"}],"units":[{"wodID":489,"name":"elitecrossbowman","_display_name":"User-like {admin}"}],"resources":[{"resourceID":1,"name":"currency1"}]}`), SourceMetadata{})
	if err != nil {
		t.Fatal(err)
	}
	language, err := DecodeLanguage([]byte(`{"bakery_name":"Bakery","elitecrossbowman_name":"Veteran Crossbowman","currency1_name":"Coins","User-like {admin}":"Wrong reverse match"}`), LanguageMetadata{})
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		collection string
		id         int64
		key        string
	}{{"units", 489, "elitecrossbowman_name"}, {"buildings", 201, "bakery_name"}, {"resources", 1, "currency1_name"}, {"players", 489, ""}, {"units", 999, ""}} {
		if got := store.DefinitionNameKey(language, tc.collection, tc.id); got != tc.key {
			t.Fatalf("%s/%d: %q, want %q", tc.collection, tc.id, got, tc.key)
		}
	}
	if store.DefinitionNameKey(nil, "units", 489) != "" {
		t.Fatal("unverified key exposed")
	}
}

func TestFeastNameKeyPreservesOfficialCaseAndType(t *testing.T) {
	language, err := DecodeLanguage([]byte(`{"dialog_festival_bigLevel2Event":"Hearty king's feast","dialog_festival_fourthEvent":"Aristocratic banquet"}`), LanguageMetadata{})
	if err != nil {
		t.Fatal(err)
	}
	if got := FeastNameKey(language, AutoBuyerFeast{Type: "bigLevel2", Name: "Custom fallback"}); got != "dialog_festival_bigLevel2Event" {
		t.Fatal(got)
	}
	if FeastNameKey(language, AutoBuyerFeast{Type: "biglevel2"}) != "" {
		t.Fatal("invented case-insensitive source key")
	}
}

func TestDefinitionNameKeyUsesOfficialResourceAliases(t *testing.T) {
	store, err := DecodeStore([]byte(`{"versionInfo":[],"buildings":[],"units":[],"resources":[{"resourceID":2,"name":"currency2","JSONKey":"C2"},{"resourceID":3,"name":"unresolved","assetName":"woodAsset"}],"currencies":[{"currencyID":8,"Name":"tokens","assetName":"tokenAsset"},{"currencyID":9,"Name":"unresolved","assetName":"specialAsset"}]}`), SourceMetadata{})
	if err != nil {
		t.Fatal(err)
	}
	language, err := DecodeLanguage([]byte(`{"gold":"Rubies","currency_name_woodAsset":"Wood","currency_name_tokens":"Preferred tokens","tokens_name":"Other tokens","currency_name_specialAsset":"Special tokens"}`), LanguageMetadata{})
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		collection string
		id         int64
		key        string
	}{{"resources", 2, "gold"}, {"resources", 3, "currency_name_woodAsset"}, {"currencies", 8, "currency_name_tokens"}, {"currencies", 9, "currency_name_specialAsset"}} {
		if got := store.DefinitionNameKey(language, tc.collection, tc.id); got != tc.key {
			t.Fatalf("%s/%d: %q, want %q", tc.collection, tc.id, got, tc.key)
		}
	}
}
