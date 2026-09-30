package GameData

import (
	"math/rand"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"CitadelDesktop/Server/State"
)

// humanizeReference is Humanize exactly as it was before the digit check and the
// keyword prefilter: every expression, in order, on every input.
func humanizeReference(labels IdentifierLabels, text string) string {
	text = legacyUserFacingIdentifierAnnotation.ReplaceAllString(text, "")
	for _, pattern := range userFacingIdentifierPatterns {
		text = pattern.expression.ReplaceAllStringFunc(text, func(match string) string {
			if strings.Contains(match, ":") {
				return match
			}
			parts := pattern.expression.FindStringSubmatch(match)
			if len(parts) != 2 {
				return match
			}
			id, err := strconv.ParseInt(parts[1], 10, 64)
			if err != nil || id < 0 {
				return match
			}
			if resolved, found := pattern.resolve(labels, id); found {
				return resolved
			}
			return unresolvedIdentifierLabel(match)
		})
	}
	return legacyUserFacingIdentifierAnnotation.ReplaceAllString(text, "")
}

func humanizeTestLabels() IdentifierLabels {
	state := State.NewGameState()
	state.Castles[10] = State.CastleState{ID: 10, Name: "Main Keep"}
	state.Commanders[7] = State.CommanderState{ID: 7, Name: "Duke Bold"}
	// Resolved names can themselves look like identifier phrases, or be non-ASCII.
	state.Castles[11] = State.CastleState{ID: 11, Name: "Caſtle 5 keep"}
	state.Castles[12] = State.CastleState{ID: 12, Name: "commander 7"}
	store, _ := DecodeStore([]byte(`{"versionInfo":[],"buildings":[{"wodID":201,"name":"bakery","level":8}],"units":[{"wodID":489,"name":"elitecrossbowman","level":6}],"resources":[{"resourceID":1,"name":"currency1","JSONKey":"C1"}]}`), SourceMetadata{})
	language, _ := DecodeLanguage([]byte(`{"bakery_name":"Bakery","elitecrossbowman_name":"Veteran Crossbowman","currency1_name":"Coins"}`), LanguageMetadata{})
	return NewIdentifierLabels(state, store, language)
}

func TestHumanizeMatchesTheUnoptimisedImplementation(t *testing.T) {
	labels := humanizeTestLabels()
	fixed := []string{
		"", "   ", "Waiting for the game connection to finish loading", "Refresh movements for invasion launch reconciliation",
		"castle 10", "Castle ID 10 has no free commander 7", "Castle 10:12 is busy", "commander 7 is away; commander 8 too",
		"building definition 201 needs unit definition 489", "resource 1 shortage", "Tool 5 and tools definition 6",
		"Evacuating 250 troops from castle 10", "3 incoming attacks", "attack (ID 42) failed (Item ID 9)",
		"CASTLE 10 / Castle 11 / castle11", "castle  10", "castle\t10", "Storm isle 906 and event camp 3", "event 103",
		"Item 5 item 6 ITEM 7", "player 12 alliance 99", "movement 50 hit skill 3", "recipe 401 ", "ends with a number 42",
		"unicode caſtle 10 and Kingdom 0", "Kingdom 0 and kingdom 0", "tŏols 5", "castle 99999999999999999999",
		"castle -5", "Castle ID 0", "castle 11 done", "castle 12 done", "Evacuating castle 11 and castle 12 with commander 7", "no digits at all but castle keyword", "12345",
	}
	for _, text := range fixed {
		if got, want := labels.Humanize(text), humanizeReference(labels, text); got != want {
			t.Fatalf("Humanize(%q) = %q, reference %q", text, got, want)
		}
	}

	words := []string{"castle", "Castle", "commander", "unit", "units definition", "troop", "tools", "tool", "item", "id", "ID", "kingdom", "event", "camp", "building",
		"instance", "definition", "resource", "movement", "player", "0", "7", "10", "489", "201", "1", "42", "99999999999", ":", ":12", "(", ")", "(ID 5)", "of", "the", "and",
		"caſtle", "K", "ï", "日本", "\t", "  ", "-", "5th"}
	random := rand.New(rand.NewSource(44))
	for iteration := 0; iteration < 20_000; iteration++ {
		parts := make([]string, 1+random.Intn(7))
		for index := range parts {
			parts[index] = words[random.Intn(len(words))]
		}
		separator := []string{" ", " ", " ", "", "  "}[random.Intn(5)]
		text := strings.Join(parts, separator)
		if got, want := labels.Humanize(text), humanizeReference(labels, text); got != want {
			t.Fatalf("Humanize(%q) = %q, reference %q", text, got, want)
		}
	}
}

func TestHumanizeSkipsTheRegexPassForTextWithoutDigits(t *testing.T) {
	labels := humanizeTestLabels()
	if !containsASCIIDigit("castle 1") || containsASCIIDigit("castle one") || containsASCIIDigit("") || containsASCIIDigit("١٢") {
		t.Fatal("containsASCIIDigit misclassifies its input")
	}
	// Every pattern needs a digit: prove it for the whole table rather than assume it.
	digit := regexp.MustCompile(`[0-9]`)
	for _, pattern := range userFacingIdentifierPatterns {
		if !digit.MatchString(pattern.expression.String()) {
			t.Fatalf("pattern %s can match without a digit; the no-digit fast path is unsound", pattern.expression)
		}
	}
	if !digit.MatchString(legacyUserFacingIdentifierAnnotation.String()) {
		t.Fatal("the legacy annotation pattern can match without a digit")
	}
	text := "Waiting for the castle commander to finish"
	if labels.Humanize(text) != text {
		t.Fatal("digit-free text changed")
	}
	// A pointer-identical result: no allocation for the common case.
	if allocations := testing.AllocsPerRun(100, func() { _ = labels.Humanize(text) }); allocations != 0 {
		t.Fatalf("digit-free Humanize allocated %.0f times", allocations)
	}
}

var humanizeBenchmarkTexts = map[string]string{
	"short-no-digits": "Waiting for game state",
	"long-no-digits":  "Waiting for the latest game state to finish loading before evacuating troops",
	"digits-no-ids":   "Evacuating 250 troops from the castle in 45 seconds",
	"one-identifier":  "Castle 10 has no free commander",
	"two-identifiers": "Evacuating from castle 10 with commander 7",
}

func BenchmarkHumanize(b *testing.B) {
	labels := humanizeTestLabels()
	for name, text := range humanizeBenchmarkTexts {
		b.Run(name+"/fast", func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				_ = labels.Humanize(text)
			}
		})
		b.Run(name+"/reference", func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				_ = humanizeReference(labels, text)
			}
		})
	}
}
