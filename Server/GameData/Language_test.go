package GameData

import "testing"

func TestResolveManyDoesNotGuessAmbiguousCaseFold(t *testing.T) {
	store, err := DecodeLanguage([]byte(`{
		"SceatSuppAttackWaves_name":"War wagon",
		"sceatSuppAttackWaves_name":"Warwagon",
		"Unique_Name":"Unique value"
	}`), LanguageMetadata{Language: "en"})
	if err != nil {
		t.Fatal(err)
	}

	resolved := store.ResolveMany([]string{
		"SceatSuppAttackWaves_name", "sceatSuppAttackWaves_name", "SCEATSUPPATTACKWAVES_NAME", "unique_name",
	})
	if resolved["SceatSuppAttackWaves_name"] != "War wagon" || resolved["sceatSuppAttackWaves_name"] != "Warwagon" {
		t.Fatalf("exact values were not preserved: %#v", resolved)
	}
	if _, exists := resolved["SCEATSUPPATTACKWAVES_NAME"]; exists {
		t.Fatalf("ambiguous folded key was resolved: %#v", resolved)
	}
	if resolved["unique_name"] != "Unique value" {
		t.Fatalf("unique folded key was not resolved: %#v", resolved)
	}
}
