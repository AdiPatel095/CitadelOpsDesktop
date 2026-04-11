package castle

type PlayerCastleInfo struct {
	Name string  `json:"castleName"`
	Aid  float64 `json:"aid"`
	// MapKingdomID, MapX, MapY are world-map position from gbd **gcl** (all player castles). Not troop-panel data.
	MapKingdomID int                   `json:"mapKingdomID,omitempty"`
	MapX         int                   `json:"mapX,omitempty"`
	MapY         int                   `json:"mapY,omitempty"`
	Amount       CastleResourcesAmount `json:"amount"`
	Production   CastleProductionTotal `json:"production"`
	Storage      CastleStorageMax      `json:"storage"`
	Troops       CastleTroopData       `json:"troops"`
	// BGRows / BDRows are parsed from JAA gca (same row shape as BuildingParser); not merged.
	BGRows []BuildingData `json:"bgRows,omitempty"`
	BDRows []BuildingData `json:"bdRows,omitempty"`
	// SlotProductionByLID is parsed from **spl** / **bup**.spl keyed by Empire LID (0=barracks/recruit, 1=tool workshop, …).
	SlotProductionByLID map[int]*BarracksProductionQueue `json:"slotProductionByLID,omitempty"`
	// CraftingQueues is parsed from **crin** (full list) / **crst** (single building); CRIDs index craftingRecipes.json.
	CraftingQueues []CraftingBuildingSnapshot `json:"craftingQueues,omitempty"`
}

// SetCastleBuildingRows stores JAA ground (BG) and building (BD) rows separately.
func SetCastleBuildingRows(c *PlayerCastleInfo, bg, bd []BuildingData) {
	if c == nil {
		return
	}
	c.BGRows = bg
	c.BDRows = bd
}

// AllBuildingRows returns BG then BD (copy-free append chain for iteration).
func (c *PlayerCastleInfo) AllBuildingRows() []BuildingData {
	if c == nil {
		return nil
	}
	n := len(c.BGRows) + len(c.BDRows)
	if n == 0 {
		return nil
	}
	out := make([]BuildingData, 0, n)
	out = append(out, c.BGRows...)
	out = append(out, c.BDRows...)
	return out
}

// BuildingData holds information about a specific building instance in a castle.
type BuildingData struct {
	BuildingID int    `json:"buildingID"` // This is WID
	OID        int    `json:"oid"`
	Name       string `json:"name"`
	Level      int    `json:"level"`
	X          int    `json:"x"`
	Y          int    `json:"y"`
	R          int    `json:"r"` // JAA gca BG/BD row index 4 (rotation); EBU R
}

// CastleTroopData holds troop counts for a castle, embedded directly on the castle object.
type CastleTroopData struct {
	KingdomID   int         `json:"kingdomID"`
	X           int         `json:"x"`
	Y           int         `json:"y"`
	TroopsI     map[int]int `json:"troopsI"`     // Units currently in castle
	TroopsTU    map[int]int `json:"troopsTU"`    // Units travelling
	TroopsHI    map[int]int `json:"troopsHI"`    // Units in hospital
	TroopsSHI   map[int]int `json:"troopsSHI"`   // Units in special hospital
	TroopsMixed map[int]int `json:"troopsMixed"` // Combined I + TU
}

type CastleResourcesAmount struct {
	WoodAmount  float64 `json:"wood_amount"`
	StoneAmount float64 `json:"stone_amount"`
	FoodAmount  float64 `json:"food_amount"`
	CoalAmount  float64 `json:"coal_amount"`
	OilAmount   float64 `json:"oil_amount"`
	GlassAmount float64 `json:"glass_amount"`
	IronAmount  float64 `json:"iron_amount"`
	HoneyAmount float64 `json:"honey_amount"`
	MeadAmount  float64 `json:"mead_amount"`
	BeefAmount  float64 `json:"beef_amount"`
}

type CastleProductionTotal struct {
	WoodProd        float64 `json:"wood_prod"`
	StoneProd       float64 `json:"stone_prod"`
	FoodProd        float64 `json:"food_prod"`
	CoalProd        float64 `json:"coal_prod"`
	OilProd         float64 `json:"oil_prod"`
	GlassProd       float64 `json:"glass_prod"`
	IronProd        float64 `json:"iron_prod"`
	HoneyProd       float64 `json:"honey_prod"`
	MeadProd        float64 `json:"mead_prod"`
	BeefProd        float64 `json:"beef_prod"`
	FoodConsumption float64 `json:"food_consumption"`
	MeadConsumption float64 `json:"mead_consumption"`
	BeefConsumption float64 `json:"beef_consumption"`
}

type CastleStorageMax struct {
	WoodMax  float64 `json:"wood_max"`
	StoneMax float64 `json:"stone_max"`
	FoodMax  float64 `json:"food_max"`
	CoalMax  float64 `json:"coal_max"`
	OilMax   float64 `json:"oil_max"`
	GlassMax float64 `json:"glass_max"`
	IronMax  float64 `json:"iron_max"`
	HoneyMax float64 `json:"honey_max"`
	MeadMax  float64 `json:"mead_max"`
	BeefMax  float64 `json:"beef_max"`
}
