package Models

type PlayerCastleInfo struct {
	Name       string                `json:"castleName"`
	Aid        float64               `json:"aid"`
	Amount     CastleResourcesAmount `json:"amount"`
	Production CastleProductionTotal `json:"production"`
	Storage    CastleStorageMax      `json:"storage"`
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
	WoodProd  float64 `json:"wood_prod"`
	StoneProd float64 `json:"stone_prod"`
	FoodProd  float64 `json:"food_prod"`
	CoalProd  float64 `json:"coal_prod"`
	OilProd   float64 `json:"oil_prod"`
	GlassProd float64 `json:"glass_prod"`
	IronProd  float64 `json:"iron_prod"`
	HoneyProd float64 `json:"honey_prod"`
	MeadProd  float64 `json:"mead_prod"`
	BeefProd  float64 `json:"beef_prod"`
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

var MainCastleResources PlayerCastleInfo
var Outpost1Resources PlayerCastleInfo
var Outpost2Resources PlayerCastleInfo
var Outpost3Resources PlayerCastleInfo
var IceCastleResources PlayerCastleInfo
var DesertCastleResources PlayerCastleInfo
var DungeonCastleResources PlayerCastleInfo
var StormCastleResources PlayerCastleInfo
