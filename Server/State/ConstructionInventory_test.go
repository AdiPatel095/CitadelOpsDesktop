package State

import (
	"testing"
	"time"
)

func TestConstructionItemInventorySpaceLeftPrefersFreshServerAnswer(t *testing.T) {
	now := time.Date(2026, 8, 18, 12, 0, 0, 0, time.UTC)
	inventory := InventoryState{
		// A local count well over the OLD, wrong 1000 cap must not read as full:
		// the official softcap is 5000.
		ConstructionItems: map[ConstructionItemID]int64{1: 700, 2: 319},
	}
	if got := ConstructionItemInventorySpaceLeft(inventory, now); got != ConstructionItemInventoryLimit-1019 {
		t.Fatalf("estimate = %d, want softcap-1019 = %d", got, ConstructionItemInventoryLimit-1019)
	}

	// The server's csp answer wins while fresh, even when it disagrees with
	// the local count (server-side expiry of temporary items is invisible here).
	inventory.ConstructionSpaceLeft = 42
	inventory.ConstructionSpaceLeftObservedAt = now.Add(-5 * time.Minute)
	if got := ConstructionItemInventorySpaceLeft(inventory, now); got != 42 {
		t.Fatalf("fresh server answer should win, got %d", got)
	}

	// Once stale, the estimate takes over again.
	inventory.ConstructionSpaceLeftObservedAt = now.Add(-2 * time.Hour)
	if got := ConstructionItemInventorySpaceLeft(inventory, now); got != ConstructionItemInventoryLimit-1019 {
		t.Fatalf("stale server answer must yield to the estimate, got %d", got)
	}
}
