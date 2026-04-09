// Package craftingrecipes provides CRID labels for crafting queues.
//
// If EmpireItems recipe metadata is unavailable, it degrades gracefully to
// generic labels like `CRID 123` instead of failing the build.
package craftingrecipes

import "fmt"

// Meta is retained for compatibility if richer recipe metadata is restored later.
type Meta struct {
	CRID         int
	RecipeGroup  int
	QueueType    int
	RewardIDs    string
	CraftingSecs int
}

// MetaForCRID currently falls back to unavailable metadata.
func MetaForCRID(crid int) (Meta, bool) {
	return Meta{}, false
}

// ShortLabel is a compact dashboard label.
// Without EmpireItems recipe metadata, fall back to a generic CRID label.
func ShortLabel(crid int) string {
	return fmt.Sprintf("CRID %d", crid)
}
