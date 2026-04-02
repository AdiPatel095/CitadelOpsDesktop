package castle

// CraftingSlotBundle is **PS** or **QS** from **crin** / **crst** (parallel CRID + optional BV quantities).
// When **CRID** is empty but **RUT** is present (common in **crai** / JAA-style snapshots), the server copies RUT into CRID for slot alignment; labels may show numeric ids not in craftingRecipes.json.
type CraftingSlotBundle struct {
	CRID []int     `json:"crid"`
	BV   []float64 `json:"bv,omitempty"`
}

// CraftingBuildingSnapshot is one **CBI** entry: sovereign crafting building on a castle (refinery, toolsmith, dragon, …).
type CraftingBuildingSnapshot struct {
	KID  int                `json:"kid"`
	AID  int                `json:"aid"`
	OID  int                `json:"oid"`
	WID  int                `json:"wid"`
	CQID int                `json:"cqid"`
	PS   CraftingSlotBundle `json:"ps"`
	QS   CraftingSlotBundle `json:"qs"`
}
