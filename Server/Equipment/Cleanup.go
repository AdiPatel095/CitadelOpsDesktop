package Equipment

import (
	"time"

	"CitadelDesktop/Server/State"
)

const CleanupSnapshotFreshness = 60 * time.Second

type CleanupCounts struct {
	Equipment int
	Gems      int64
}

// CleanupStorageFresh reports whether both storage views used by automatic
// cleanup came from recent successful inbound responses.
func CleanupStorageFresh(gameState State.GameState, now time.Time) bool {
	return StorageSnapshotFresh(gameState, "gei", now) &&
		StorageSnapshotFresh(gameState, "ggm", now)
}

func StorageSnapshotFresh(gameState State.GameState, opcode string, now time.Time) bool {
	observation, ok := gameState.Observations[opcode]
	observedAt := observation.SuccessfulInboundAt()
	return ok && !observedAt.IsZero() && now.Sub(observedAt) <= CleanupSnapshotFreshness
}

// AutomaticCleanupCounts applies the same protected-item boundary as the sell
// intent: worn equipment, relics, look items, and post-2026 items are retained.
func AutomaticCleanupCounts(gameState State.GameState) CleanupCounts {
	counts := CleanupCounts{}
	for _, item := range gameState.Inventory.Equipment {
		if item.WearerKind == "" && MatchesNonRelicEquipmentSale(item, false, false) {
			counts.Equipment++
		}
	}
	for id, amount := range gameState.Inventory.GemStacks {
		if amount > 0 && MatchesNonRelicGemStackSale(id, false) {
			counts.Gems += amount
		}
	}
	return counts
}

func MatchesNonRelicEquipmentSale(item State.EquipmentInstance, sellLookItems bool, sellPost2026 bool) bool {
	if item.RarityID == 5 || item.RarityID == 15 || item.Slot == 5 && !sellLookItems {
		return false
	}
	// Some ordinary storage rows do not include a catalog definition. The
	// parser then retains the instance ID as the only stable identity; that
	// must not make an otherwise eligible item look like post-2026 equipment.
	definitionUnknown := item.ID > 0 && item.DefinitionID == State.EquipmentID(item.ID)
	return definitionUnknown || int64(item.DefinitionID) < 1366 || sellPost2026
}

func MatchesNonRelicGemStackSale(id State.GemID, sellPost2026 bool) bool {
	return int64(id) < 450 || sellPost2026
}
