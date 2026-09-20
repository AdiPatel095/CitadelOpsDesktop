package Equipment

import (
	"strings"
	"testing"

	"CitadelDesktop/Server/GameData"
	"CitadelDesktop/Server/State"
)

func TestQuoteReconfigurationUsesExactDeduplicatedTransition(t *testing.T) {
	gameData := reconfigurationCostGameData(t, `"200"`)
	gameState := State.NewGameState()
	current := map[string]State.EquipmentInstanceID{"1": 101, "2": 102, "3": 103, "4": 104}
	target := map[string]State.EquipmentInstanceID{"1": 201, "2": 102, "3": 103, "4": 104}
	for slot := 1; slot <= 4; slot++ {
		currentID := State.EquipmentInstanceID(100 + slot)
		gameState.Inventory.Equipment[currentID] = State.EquipmentInstance{ID: currentID, Slot: slot, TypeID: 2, RelicKnown: true}
	}
	gameState.Inventory.Equipment[201] = State.EquipmentInstance{ID: 201, Slot: 1, TypeID: 2, RelicKnown: true}
	gameState.Inventory.Gems[-101] = State.GemInstance{ID: -101, DefinitionID: 494, EquipmentInstanceID: 101}

	transition, err := BuildReconfigurationTransition(gameState, current, target, map[string]State.GemInstanceID{"1": -101})
	if err != nil {
		t.Fatal(err)
	}
	quote, err := QuoteReconfiguration(gameData, transition)
	if err != nil {
		t.Fatal(err)
	}
	if quote.RubyExtractionCount != 1 || quote.MaximumRubySpend != 200 || quote.SocketInsertionCount != 1 || quote.RelicExtractionCount != 0 {
		t.Fatalf("quote = %+v", quote)
	}
	untouched, err := BuildReconfigurationTransition(gameState, current, current, map[string]State.GemInstanceID{"1": -101})
	if err != nil {
		t.Fatal(err)
	}
	untouchedQuote, err := QuoteReconfiguration(gameData, untouched)
	if err != nil {
		t.Fatal(err)
	}
	if untouchedQuote.MaximumRubySpend != 0 || untouchedQuote.RubyExtractionCount != 0 || untouchedQuote.SocketInsertionCount != 0 {
		t.Fatalf("untouched quote = %+v", untouchedQuote)
	}
}

func TestQuoteReconfigurationFailsClosedOnInvalidRemovalCost(t *testing.T) {
	for _, cost := range []string{`"missing"`, `"-1"`} {
		gameData := reconfigurationCostGameData(t, cost)
		transition := ReconfigurationTransition{
			GemsToDetach: map[State.GemInstanceID]State.GemInstance{
				-101: {ID: -101, DefinitionID: 494, EquipmentInstanceID: 101},
			},
		}
		if _, err := QuoteReconfiguration(gameData, transition); err == nil || !strings.Contains(err.Error(), "removalCostC2") {
			t.Fatalf("cost %s error = %v", cost, err)
		}
	}
}

func reconfigurationCostGameData(t testing.TB, removalCost string) *GameData.Store {
	t.Helper()
	store, err := GameData.DecodeStore([]byte(`{
		"versionInfo":{},"buildings":[],"units":[],
		"resources":[{"resourceID":"2","JSONKey":"C2"}],
		"gems":[{"gemID":"494","gemLevelID":"0"}],
		"gemlevels":[{"gemLevelID":"0","removalCostC2":`+removalCost+`}]
	}`), GameData.SourceMetadata{ItemVersion: "test", DigestSHA256: "cost-catalog"})
	if err != nil {
		t.Fatal(err)
	}
	return store
}
