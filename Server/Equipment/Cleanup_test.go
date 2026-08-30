package Equipment

import (
	"testing"
	"time"

	"CitadelDesktop/Server/State"
)

func TestAutomaticCleanupCountsUseProtectedSaleBoundary(t *testing.T) {
	gameState := State.NewGameState()
	gameState.Inventory.Equipment[101] = State.EquipmentInstance{ID: 101, DefinitionID: 100, Slot: 1}
	gameState.Inventory.Equipment[2000] = State.EquipmentInstance{ID: 2000, DefinitionID: 2000, Slot: 2}
	gameState.Inventory.Equipment[103] = State.EquipmentInstance{ID: 103, DefinitionID: 1366, Slot: 3}
	gameState.Inventory.Equipment[104] = State.EquipmentInstance{ID: 104, DefinitionID: 100, Slot: 4, RarityID: 5}
	gameState.Inventory.Equipment[105] = State.EquipmentInstance{ID: 105, DefinitionID: 100, Slot: 4, RarityID: 15}
	gameState.Inventory.Equipment[106] = State.EquipmentInstance{ID: 106, DefinitionID: 100, Slot: 5}
	gameState.Inventory.Equipment[107] = State.EquipmentInstance{
		ID: 107, DefinitionID: 100, Slot: 1, WearerKind: "commander", WearerID: 7,
	}
	gameState.Inventory.GemStacks[100] = 2
	gameState.Inventory.GemStacks[449] = 3
	gameState.Inventory.GemStacks[450] = 4
	gameState.Inventory.GemStacks[900] = -1

	counts := AutomaticCleanupCounts(gameState)
	if counts.Equipment != 2 || counts.Gems != 5 {
		t.Fatalf("cleanup counts = %+v, want 2 equipment and 5 gems", counts)
	}
	if !MatchesNonRelicEquipmentSale(gameState.Inventory.Equipment[106], true, false) {
		t.Fatal("explicit look-item sale did not include slot 5")
	}
	if !MatchesNonRelicEquipmentSale(gameState.Inventory.Equipment[103], false, true) {
		t.Fatal("explicit post-2026 equipment sale did not include definition 1366")
	}
	if !MatchesNonRelicGemStackSale(450, true) {
		t.Fatal("explicit post-2026 gem sale did not include definition 450")
	}
}

func TestCleanupStorageFreshRequiresBothSuccessfulSnapshots(t *testing.T) {
	now := time.Date(2026, 8, 25, 12, 0, 0, 0, time.UTC)
	gameState := State.NewGameState()
	gameState.Observations["gei"] = State.ProtocolObservation{LastSuccessfulInboundAt: now.Add(-59 * time.Second)}
	gameState.Observations["ggm"] = State.ProtocolObservation{LastSuccessfulInboundAt: now.Add(-59 * time.Second)}
	if !CleanupStorageFresh(gameState, now) {
		t.Fatal("recent equipment and gem snapshots were treated as stale")
	}
	gameState.Observations["ggm"] = State.ProtocolObservation{LastSuccessfulInboundAt: now.Add(-61 * time.Second)}
	if CleanupStorageFresh(gameState, now) {
		t.Fatal("stale gem snapshot was treated as fresh")
	}
}
