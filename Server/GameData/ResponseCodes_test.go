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
	incompatibleTools := ResolveResponseCode(store, "CRA", 91)
	if incompatibleTools.Source != ResponseCodeObserved || incompatibleTools.Kind != ResponseCodeContext ||
		incompatibleTools.ExpectedState || !strings.Contains(incompatibleTools.Message, "incompatible tools") ||
		!strings.Contains(incompatibleTools.Recovery, "attack preset") {
		t.Fatalf("CRA incompatible-tools response meaning = %#v", incompatibleTools)
	}
	if unrelated := ResolveResponseCode(store, "xyz", 91); unrelated.Source != ResponseCodeUnknown {
		t.Fatalf("opcode-scoped CRA response meaning leaked to another opcode = %#v", unrelated)
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
	if meaning := store.ResponseCodeMeanings("cra")[91]; meaning.Source != ResponseCodeObserved {
		t.Fatalf("CRA incompatible-tools response code map = %#v", meaning)
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

func TestResolveEquipmentEnchantResponseCodesFromOfficialClient(t *testing.T) {
	tests := []struct {
		code             int
		messageFragment  string
		kind             ResponseCodeKind
		expectedState    bool
		recoveryFragment string
	}{
		{226, "enchantment level is too high", ResponseCodeStaleState, true, "maximum enchantment level"},
		{227, "enchantment attempt failed", "", true, "Retry the same level"},
		{236, "cannot be enchanted", ResponseCodeContext, true, "allows to be enchanted"},
	}
	for _, opcode := range []string{"ERE", "eqe"} {
		for _, test := range tests {
			meaning := ResolveResponseCode(nil, opcode, test.code)
			if meaning.Source != ResponseCodeOfficialClient || meaning.Code != test.code ||
				!strings.Contains(meaning.Message, test.messageFragment) || meaning.Kind != test.kind ||
				meaning.ExpectedState != test.expectedState || !strings.Contains(meaning.Recovery, test.recoveryFragment) {
				t.Errorf("%s %d meaning = %#v", opcode, test.code, meaning)
			}
		}
	}

	if expansion := ResolveResponseCode(nil, "ebe", 227); expansion.Source != ResponseCodeUnknown {
		t.Fatalf("enchant-specific code leaked to EBE = %#v", expansion)
	}

	store, err := DecodeLanguage([]byte(`{"other":"language value"}`), LanguageMetadata{Language: "en"})
	if err != nil {
		t.Fatal(err)
	}
	meanings := store.ResponseCodeMeanings(" ERE ")
	for _, code := range []int{226, 227, 236} {
		if meanings[code].Source != ResponseCodeOfficialClient {
			t.Errorf("ERE response-code catalog omitted official-client code %d: %#v", code, meanings)
		}
	}
}

func TestResolveEquipmentEnchantGuidanceKeepsOfficialLanguageText(t *testing.T) {
	store, err := DecodeLanguage([]byte(`{
		"errorCode_222":"General is travelling with a commander/castellan.",
		"errorCode_227":"Localized enchantment failure."
	}`), LanguageMetadata{Language: "en"})
	if err != nil {
		t.Fatal(err)
	}

	for _, opcode := range []string{"ere", "EQE"} {
		travelling := ResolveResponseCode(store, opcode, 222)
		if travelling.Source != ResponseCodeOfficial ||
			travelling.Message != "General is travelling with a commander/castellan." ||
			travelling.Kind != ResponseCodeAvailability || !travelling.ExpectedState ||
			!strings.Contains(travelling.Recovery, "commander or castellan") {
			t.Errorf("%s official travelling guidance = %#v", opcode, travelling)
		}

		failed := ResolveResponseCode(store, opcode, 227)
		if failed.Source != ResponseCodeOfficial || failed.Message != "Localized enchantment failure." ||
			!failed.ExpectedState || !strings.Contains(failed.Recovery, "Retry the same level") {
			t.Errorf("%s localized enchantment failure = %#v", opcode, failed)
		}
	}

	expansion := ResolveResponseCode(store, "ebe", 222)
	if expansion.Source != ResponseCodeOfficial || expansion.Kind != "" ||
		expansion.ExpectedState || expansion.Recovery != "" {
		t.Fatalf("enchant-specific 222 guidance leaked to EBE = %#v", expansion)
	}
}

func TestResolveExpansionDirectionGuidanceKeepsOfficialLanguageText(t *testing.T) {
	store, err := DecodeLanguage([]byte(`{
		"errorCode_263":"This area is already full. Please select another direction."
	}`), LanguageMetadata{Language: "en"})
	if err != nil {
		t.Fatal(err)
	}

	expansion := ResolveResponseCode(store, "EBE", 263)
	if expansion.Source != ResponseCodeOfficial ||
		expansion.Message != "This area is already full. Please select another direction." ||
		expansion.Kind != ResponseCodeContext || !expansion.ExpectedState ||
		!strings.Contains(expansion.Recovery, "different expansion direction") {
		t.Fatalf("official expansion direction guidance = %#v", expansion)
	}
	if unrelated := ResolveResponseCode(store, "xyz", 263); unrelated.Kind != "" || unrelated.ExpectedState || unrelated.Recovery != "" {
		t.Fatalf("expansion-specific guidance leaked = %#v", unrelated)
	}
}

func TestResolveAllianceHelpDuplicateIsOpcodeScoped(t *testing.T) {
	meaning := ResolveResponseCode(nil, " AHR ", 273)
	if meaning.Source != ResponseCodeOfficialClient || meaning.Kind != ResponseCodeStaleState ||
		!meaning.ExpectedState || !strings.Contains(meaning.Message, "duplicate") {
		t.Fatalf("official AHR mapping = %#v", meaning)
	}
	for _, opcode := range []string{"ahh", "aha", "msd", "future"} {
		if got := ResolveResponseCode(nil, opcode, 273); got.Source != ResponseCodeUnknown || got.ExpectedState {
			t.Fatalf("AHR mapping leaked to %s: %#v", opcode, got)
		}
	}
}
