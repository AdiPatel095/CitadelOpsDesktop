package castle

// CraftingSlotBundle is **PS** or **QS** from **crin** / **crst**.
// CRID contains recipes, BV is the output bonus percentage, RCT is active craft time remaining,
// and RUT contains one seven-day lease timer for each rented slot in that bundle.
type CraftingSlotBundle struct {
	CRID []int     `json:"crid"`
	BV   []float64 `json:"bv,omitempty"`
	RUT  []int     `json:"rentRemainingSec,omitempty"`
	RCT  []int     `json:"craftRemainingSec,omitempty"`
}

// CraftingBuildingSnapshot is one **CBI** entry: sovereign crafting building on a castle (refinery, toolsmith, dragon, …).
type CraftingBuildingSnapshot struct {
	KID          int                `json:"kid"`
	AID          int                `json:"aid"`
	OID          int                `json:"oid"`
	WID          int                `json:"wid"`
	CQID         int                `json:"cqid"`
	S            int                `json:"state,omitempty"`
	ObservedUnix int64              `json:"observedUnix,omitempty"`
	PS           CraftingSlotBundle `json:"ps"`
	QS           CraftingSlotBundle `json:"qs"`
}

// CraftingEntitlements is the effective crafting availability returned in one **crin** CAI entry.
// Recipe ids are effect 616, recipe groups are effect 377, and effect 373 is an output bonus by CQID.
type CraftingEntitlements struct {
	EnabledRecipeIDs      []int           `json:"enabledRecipeIds,omitempty"`
	EnabledRecipeGroupIDs []int           `json:"enabledRecipeGroupIds,omitempty"`
	OutputBoostByQueue    map[int]float64 `json:"outputBoostByQueue,omitempty"`
}
