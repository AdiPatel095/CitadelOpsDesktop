package Equipment

import (
	"fmt"
	"reflect"
	"runtime"
	"testing"
	"weak"

	"CitadelDesktop/Server/GameData"
)

func rulesTestStore(t *testing.T, cap int, digest string) *GameData.Store {
	t.Helper()
	store, err := GameData.DecodeStore([]byte(fmt.Sprintf(`{
		"versionInfo":{},"buildings":[],"units":[],
		"effectCaps":[{"capID":"23","maxTotalBonus":"%d"}],
		"effecttypes":[{"effectTypeID":"10","name":"MeleeAttack","sortCategory":"3","sortGroup":"1"}],
		"effects":[{"effectID":"9001","effectTypeID":"10","capID":"23","areaTypeID":"1,2,3,4,5,6"}]
	}`, cap)), GameData.SourceMetadata{ItemVersion: digest, DigestSHA256: digest})
	if err != nil {
		t.Fatal(err)
	}
	return store
}

func TestOfficialRulesAreBuiltOncePerStoreAndFollowTheStore(t *testing.T) {
	first := rulesTestStore(t, 90, "a")
	second := rulesTestStore(t, 30, "b")

	firstRules := loadOfficialRules(first)
	if got := firstRules.caps[9001].max; got != 90 {
		t.Fatalf("first store cap = %v, want 90", got)
	}
	if again := loadOfficialRules(first); reflect.ValueOf(again.caps).Pointer() != reflect.ValueOf(firstRules.caps).Pointer() {
		t.Fatal("official rules were rebuilt for an unchanged store")
	}
	// A new store switches the optimizer to the new data; the old store's entry is not consulted.
	secondRules := loadOfficialRules(second)
	if got := secondRules.caps[9001].max; got != 30 {
		t.Fatalf("second store cap = %v, want 30 (stale rules from the first store)", got)
	}
	if got := loadOfficialRules(first).caps[9001].max; got != 90 {
		t.Fatalf("first store cap changed to %v after the second store was used", got)
	}
}

// A refresh replaces the store every six hours; nothing the optimizer keeps may
// pin the retired one.
func TestOfficialRulesDoNotPinARetiredStore(t *testing.T) {
	retired := rulesTestStore(t, 90, "old")
	current := rulesTestStore(t, 30, "new")
	loadOfficialRules(retired)
	loadOfficialRules(current)
	pointer := weak.Make(retired)
	retired = nil

	for attempt := 0; attempt < 10 && pointer.Value() != nil; attempt++ {
		runtime.GC()
	}
	if pointer.Value() != nil {
		t.Fatal("retired game-data store is still reachable after the optimizer used its replacement")
	}
	if got := loadOfficialRules(current).caps[9001].max; got != 30 {
		t.Fatalf("current store cap = %v, want 30", got)
	}
	runtime.KeepAlive(current)
}

func TestOfficialRulesWithoutGameDataAreEmpty(t *testing.T) {
	rules := loadOfficialRules(nil)
	if len(rules.caps) != 0 || len(rules.effects) != 0 {
		t.Fatalf("rules without game data = %#v", rules)
	}
}
