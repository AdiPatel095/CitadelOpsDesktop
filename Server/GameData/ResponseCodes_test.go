package GameData

import (
	"strings"
	"testing"
)

func TestResponseCodesExtractsOfficialLanguageMap(t *testing.T) {
	store, err := DecodeLanguage([]byte(`{
		"errorCode_10":"Not enough coins.",
		"errorCode_109":"All market barrows are moving.",
		"errorCode_invalid":"Ignore me.",
		"other":"Not a response code."
	}`), LanguageMetadata{Language: "en"})
	if err != nil {
		t.Fatal(err)
	}

	codes := store.ResponseCodes()
	if len(codes) != 2 || codes[10] != "Not enough coins." || codes[109] != "All market barrows are moving." {
		t.Fatalf("response code map = %#v", codes)
	}
	codes[10] = "changed"
	if message, _ := store.ResponseCode(10); message != "Not enough coins." {
		t.Fatalf("response code map mutated language store: %q", message)
	}
}

func TestResolveResponseCodeDistinguishesOfficialObservedAndUnknown(t *testing.T) {
	store, err := DecodeLanguage([]byte(`{"errorCode_109":"All market barrows are moving."}`), LanguageMetadata{Language: "en"})
	if err != nil {
		t.Fatal(err)
	}

	official := ResolveResponseCode(store, "mbr", 109)
	if official.Source != ResponseCodeOfficial || official.Message != "All market barrows are moving." {
		t.Fatalf("official response meaning = %#v", official)
	}
	observed := ResolveResponseCode(store, "HRU", 53)
	if observed.Source != ResponseCodeObserved || !strings.Contains(observed.Message, "castle focus") ||
		observed.Kind != ResponseCodeContext || !observed.ExpectedState || observed.Recovery == "" {
		t.Fatalf("observed response meaning = %#v", observed)
	}
	fortification := ResolveResponseCode(store, "RAE", 327)
	if fortification.Source != ResponseCodeObserved || !strings.Contains(fortification.Message, "fortification currency") {
		t.Fatalf("fortification response meaning = %#v", fortification)
	}
	commanderBusy := ResolveResponseCode(store, "CRA", 256)
	if commanderBusy.Source != ResponseCodeObserved || !strings.Contains(commanderBusy.Message, "commander") {
		t.Fatalf("CRA busy-commander response meaning = %#v", commanderBusy)
	}
	transportGone := ResolveResponseCode(store, "MSK", 182)
	if transportGone.Source != ResponseCodeObserved || !strings.Contains(transportGone.Message, "no longer available") {
		t.Fatalf("MSK unavailable-transport response meaning = %#v", transportGone)
	}
	if unrelated := ResolveResponseCode(store, "xyz", 182); unrelated.Source != ResponseCodeUnknown {
		t.Fatalf("opcode-scoped MSK response meaning leaked to another opcode = %#v", unrelated)
	}
	unknown := ResolveResponseCode(store, "xyz", 999)
	if unknown.Source != ResponseCodeUnknown {
		t.Fatalf("unknown response meaning = %#v", unknown)
	}

	meanings := store.ResponseCodeMeanings("hru")
	if meanings[109].Source != ResponseCodeOfficial || meanings[53].Source != ResponseCodeObserved {
		t.Fatalf("combined response code map = %#v", meanings)
	}
	if meaning := store.ResponseCodeMeanings("rae")[327]; meaning.Source != ResponseCodeObserved {
		t.Fatalf("RAE response code map = %#v", meaning)
	}
	if meaning := store.ResponseCodeMeanings("cra")[256]; meaning.Source != ResponseCodeObserved {
		t.Fatalf("CRA response code map = %#v", meaning)
	}
	if meaning := store.ResponseCodeMeanings("msk")[182]; meaning.Source != ResponseCodeObserved {
		t.Fatalf("MSK response code map = %#v", meaning)
	}
}

func TestResolveResponseCodeAddsContextualRecoveryWithoutReplacingOfficialText(t *testing.T) {
	store, err := DecodeLanguage([]byte(`{
		"errorCode_53":"The selected context is unavailable.",
		"errorCode_55":"You do not have enough resources.",
		"errorCode_95":"The target is still on cooldown."
	}`), LanguageMetadata{Language: "en"})
	if err != nil {
		t.Fatal(err)
	}

	focus := ResolveResponseCode(store, "hru", 53)
	if focus.Source != ResponseCodeOfficial || focus.Message != "The selected context is unavailable." ||
		focus.Kind != ResponseCodeContext || !focus.ExpectedState || !strings.Contains(focus.Recovery, "castle focus") {
		t.Fatalf("official focus response = %#v", focus)
	}
	cooldown := ResolveResponseCode(store, "adi", 95)
	if cooldown.Source != ResponseCodeOfficial || cooldown.Kind != ResponseCodeCooldown ||
		!cooldown.ExpectedState || !strings.Contains(cooldown.Recovery, "cooldown") {
		t.Fatalf("official cooldown response = %#v", cooldown)
	}
	shop := ResolveResponseCode(store, "sbp", 55)
	if shop.Kind != ResponseCodeAvailability || !shop.ExpectedState || !strings.Contains(shop.Recovery, "shop currency") {
		t.Fatalf("shop availability response = %#v", shop)
	}
	unrelated := ResolveResponseCode(store, "xyz", 55)
	if unrelated.Kind != "" || unrelated.ExpectedState || unrelated.Recovery != "" {
		t.Fatalf("opcode-scoped shop guidance leaked = %#v", unrelated)
	}
}
