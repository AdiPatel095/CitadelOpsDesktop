package State

import "time"

// ConstructionItemInventoryLimit mirrors the official client's
// ConstructionItemConst.INVENTORY_SOFTCAP (5000; HARDCAP is 6000). The client
// derives "space left" as SOFTCAP − owned total and lets the server's csp
// answer override it — see ConstructionItemInventorySpaceLeft.
const ConstructionItemInventoryLimit int64 = 5000

// constructionSpaceLeftFreshness bounds how long a server-reported space-left
// value stays authoritative before the local softcap estimate takes over.
const constructionSpaceLeftFreshness = 30 * time.Minute

func ConstructionItemInventoryCount(items map[ConstructionItemID]int64) int64 {
	var total int64
	for _, amount := range items {
		if amount > 0 {
			total += amount
		}
	}
	return total
}

// ConstructionItemInventorySpaceLeft is the remaining construction-item
// inventory room. The server's own answer (S2C "csp", the value the official
// client consults before every buy dialog) wins while fresh; otherwise the
// estimate is the softcap minus the observed inventory total, exactly as the
// client computes it on inventory parse.
func ConstructionItemInventorySpaceLeft(inventory InventoryState, now time.Time) int64 {
	if !inventory.ConstructionSpaceLeftObservedAt.IsZero() &&
		(now.IsZero() || now.Sub(inventory.ConstructionSpaceLeftObservedAt) <= constructionSpaceLeftFreshness) {
		return inventory.ConstructionSpaceLeft
	}
	return ConstructionItemInventoryLimit - ConstructionItemInventoryCount(inventory.ConstructionItems)
}
