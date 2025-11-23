package Models

import "sync"

type PlayerCastleInfo struct {
	MainCastleName    string  `json:"main_castle_name"`
	Outpost1Name      string  `json:"outpost_1_name"`
	Outpost2Name      string  `json:"outpost_2_name"`
	Outpost3Name      string  `json:"outpost_3_name"`
	IceCastleName     string  `json:"ice_castle_name"`
	DesertCastleName  string  `json:"desert_castle_name"`
	DungeonCastleName string  `json:"dungeon_castle_name"`
	StormCastleName   string  `json:"storm_castle_name"`
	MainCastleAID     float64 `json:"main_castle_aid"`
	Outpost1AID       float64 `json:"outpost_1_aid"`
	Outpost2AID       float64 `json:"outpost_2_aid"`
	Outpost3AID       float64 `json:"outpost_3_aid"`
	IceCastleAID      float64 `json:"ice_castle_aid"`
	DesertCastleAID   float64 `json:"desert_castle_aid"`
	DungeonCastleAID  float64 `json:"dungeon_castle_aid"`
	StormCastleAID    float64 `json:"storm_castle_aid"`

	//MainCastle
	MainCastleAmount     CastleResourcesAmount `json:"main_castle_amount"`
	MainCastleProduction CastleProductionTotal `json:"main_castle_production"`
	MainCastleStorage    CastleStorageMax      `json:"main_castle_storage"`

	//Outpost1
	Outpost1Amount     CastleResourcesAmount `json:"outpost_1_amount"`
	Outpost1Production CastleProductionTotal `json:"outpost_1_production"`
	Outpost1Storage    CastleStorageMax      `json:"outpost_1_storage"`

	//Outpost2
	Outpost2Amount     CastleResourcesAmount `json:"outpost_2_amount"`
	Outpost2Production CastleProductionTotal `json:"outpost_2_production"`
	Outpost2Storage    CastleStorageMax      `json:"outpost_2_storage"`

	//Outpost3
	Outpost3Amount     CastleResourcesAmount `json:"outpost_3_amount"`
	Outpost3Production CastleProductionTotal `json:"outpost_3_production"`
	Outpost3Storage    CastleStorageMax      `json:"outpost_3_storage"`

	//IceCastle
	IceCastleAmount     CastleResourcesAmount `json:"ice_castle_amount"`
	IceCastleProduction CastleProductionTotal `json:"ice_castle_production"`
	IceCastleStorage    CastleStorageMax      `json:"ice_castle_storage"`

	//DesertCastle
	DesertCastleAmount     CastleResourcesAmount `json:"desert_castle_amount"`
	DesertCastleProduction CastleProductionTotal `json:"desert_castle_production"`
	DesertCastleStorage    CastleStorageMax      `json:"desert_castle_storage"`

	//DungeonCastle
	DungeonCastleAmount     CastleResourcesAmount `json:"dungeon_castle_amount"`
	DungeonCastleProduction CastleProductionTotal `json:"dungeon_castle_production"`
	DungeonCastleStorage    CastleStorageMax      `json:"dungeon_castle_storage"`

	//StormCastle
	StormCastleAmount     CastleResourcesAmount `json:"storm_castle_amount"`
	StormCastleProduction CastleProductionTotal `json:"storm_castle_production"`
	StormCastleStorage    CastleStorageMax      `json:"storm_castle_storage"`
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

var (
	instanceCastleInfo *PlayerCastleInfo
	onceCastleInfo     sync.Once
)

// GetPlayerGlobalResources returns the singleton instance of PlayerGlobalResources.
func GetPlayerCastleInfo() *PlayerCastleInfo {
	onceCastleInfo.Do(func() {
		instanceCastleInfo = &PlayerCastleInfo{}
	})
	return instanceCastleInfo
}
