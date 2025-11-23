package models

type CommDBModel struct {
	IGNString  string         `json:"ignString"`
	CommActual CommSubDBModel `json:"commActual"`
}
type CommSubDBModel struct {
	//commDetails

	ID   int    `json:"id"`
	Name string `json:"name"`

	Equip1 int `json:"equip1"`
	Equip2 int `json:"equip2"`
	Equip3 int `json:"equip3"`
	Equip4 int `json:"equip4"`
	Hero   int `json:"hero"`
	Gem1   int `json:"gem1"`
	Gem2   int `json:"gem2"`
	Gem3   int `json:"gem3"`
	Gem4   int `json:"gem4"`

	//base stats
	MeleeCbtStr float64 `json:"meleeCbtStr"`
	RangeCbtStr float64 `json:"rangeCbtStr"`
	FrontCbtStr float64 `json:"frontCbtStr"`
	FlankCbtStr float64 `json:"flankCbtStr"`
	AllCbtStr   float64 `json:"allCbtStr"`
	CyCbtStr    float64 `json:"cyCbtStr"`
	WallStr     float64 `json:"wallStr"`
	GateStr     float64 `json:"gateStr"`
	MoatStr     float64 `json:"moatStr"`
	FlankLimit  float64 `json:"flankLimit"`
	FrontLimit  float64 `json:"frontLimit"`
	MeadStr     float64 `json:"meadStr"`
	HorrorStr   float64 `json:"horrorStr"`
	EliteStr    float64 `json:"eliteStr"`
	Wave        float64 `json:"wave"`
	Cooldown    float64 `json:"cooldown"`
	RelicStr    float64 `json:"relicStr"`
	BeserkerStr float64 `json:"beserkerStr"`
	MaidenSupp  float64 `json:"maidenSupp"`
	Travel      float64 `json:"travelStr"`
	Loot        float64 `json:"lootStr"`

	//NPCStats
	NPCMelee float64 `json:"NPCMelee"`
	NPCRange float64 `json:"NPCRange"`
	NPCFront float64 `json:"NPCFront"`
	NPCFlank float64 `json:"NPCFlank"`
	NPCCy    float64 `json:"NPCCy"`
	NPCWall  float64 `json:"NPCWall"`
	NPCGate  float64 `json:"NPCGate"`
	NPCMoat  float64 `json:"NPCMoat"`
	NPCGlory float64 `json:"NPCGlory"`

	//CastleLord/Pvp Stats
	CLMelee float64 `json:"CLMelee"`
	CLRange float64 `json:"CLRange"`
	CLFront float64 `json:"CLFront"`
	CLFlank float64 `json:"CLFlank"`
	CLCy    float64 `json:"CLCy"`
	CLWall  float64 `json:"CLWall"`
	CLGate  float64 `json:"CLGate"`
	CLMoat  float64 `json:"CLMoat"`
	CLLater float64 `json:"CLLater"`
	CLFire  float64 `json:"CLFire"`
	CLGlory float64 `json:"CLGlory"`
}

// CommDBUpdater defines the function signature for updating a CommSubDBModel field.
// The value is passed as a float32, so any necessary type conversions should be handled within the updater function.
type CommDBUpdater func(commDB *CommSubDBModel, value float64)

// StatUpdaterMap maps a stat ID to a function that updates the corresponding CommSubDBModel field.
var StatUpdaterMap = map[int]CommDBUpdater{

	//fixed stats
	1: func(c *CommSubDBModel, v float64) { c.MeleeCbtStr += v },
	2: func(c *CommSubDBModel, v float64) { c.RangeCbtStr += v },
	3: func(c *CommSubDBModel, v float64) { c.WallStr += v },
	4: func(c *CommSubDBModel, v float64) { c.GateStr += v },
	5: func(c *CommSubDBModel, v float64) { c.MoatStr += v },
	6: func(c *CommSubDBModel, v float64) { c.Travel += v },
	7: func(c *CommSubDBModel, v float64) { c.Loot += v },

	//random relic on base equipment
	101: func(c *CommSubDBModel, v float64) { c.MeleeCbtStr += v },
	102: func(c *CommSubDBModel, v float64) { c.RangeCbtStr += v },
	103: func(c *CommSubDBModel, v float64) { c.WallStr += v },
	104: func(c *CommSubDBModel, v float64) { c.GateStr += v },
	105: func(c *CommSubDBModel, v float64) { c.MoatStr += v },
	106: func(c *CommSubDBModel, v float64) { c.Travel += v },
	107: func(c *CommSubDBModel, v float64) { c.Loot += v },
	108: func(c *CommSubDBModel, v float64) { c.MeleeCbtStr += v },
	109: func(c *CommSubDBModel, v float64) { c.RangeCbtStr += v },
	110: func(c *CommSubDBModel, v float64) { c.WallStr += v },
	111: func(c *CommSubDBModel, v float64) { c.GateStr += v },
	112: func(c *CommSubDBModel, v float64) { c.MoatStr += v },
	113: func(c *CommSubDBModel, v float64) { c.Travel += v },
	114: func(c *CommSubDBModel, v float64) { c.Loot += v },
	115: func(c *CommSubDBModel, v float64) { c.FlankLimit += v },
	116: func(c *CommSubDBModel, v float64) { c.CyCbtStr += v },
	117: func(c *CommSubDBModel, v float64) { c.FrontLimit += v },
	118: func(c *CommSubDBModel, v float64) { c.AllCbtStr += v },
	119: func(c *CommSubDBModel, v float64) { c.FlankCbtStr += v },
	120: func(c *CommSubDBModel, v float64) { c.FrontCbtStr += v },
	121: func(c *CommSubDBModel, v float64) { c.MaidenSupp += v },

	//gem stats castle lord
	309: func(c *CommSubDBModel, v float64) { c.CLWall += v },
	310: func(c *CommSubDBModel, v float64) { c.CLGate += v },
	311: func(c *CommSubDBModel, v float64) { c.CLMelee += v },
	312: func(c *CommSubDBModel, v float64) { c.CLRange += v },
	313: func(c *CommSubDBModel, v float64) { c.CLFlank += v },
	314: func(c *CommSubDBModel, v float64) { c.CLCy += v },
	315: func(c *CommSubDBModel, v float64) { c.CLFront += v },
	316: func(c *CommSubDBModel, v float64) { c.CLLater += v },
	317: func(c *CommSubDBModel, v float64) { c.CLMoat += v },
	318: func(c *CommSubDBModel, v float64) { c.CLFire += v },
	319: func(c *CommSubDBModel, v float64) { c.CLGlory += v },

	//gem stats npc
	320: func(c *CommSubDBModel, v float64) { c.NPCGlory += v },
	209: func(c *CommSubDBModel, v float64) { c.NPCWall += v },
	210: func(c *CommSubDBModel, v float64) { c.NPCGate += v },
	211: func(c *CommSubDBModel, v float64) { c.NPCMelee += v },
	212: func(c *CommSubDBModel, v float64) { c.NPCRange += v },
	213: func(c *CommSubDBModel, v float64) { c.NPCFlank += v },
	214: func(c *CommSubDBModel, v float64) { c.NPCCy += v },
	215: func(c *CommSubDBModel, v float64) { c.NPCFront += v },
	216: func(c *CommSubDBModel, v float64) { c.NPCMoat += v },

	//hero castle lord
	806: func(c *CommSubDBModel, v float64) { c.CLLater += v },
	807: func(c *CommSubDBModel, v float64) { c.CLMoat += v },
	808: func(c *CommSubDBModel, v float64) { c.CLWall += v },
	809: func(c *CommSubDBModel, v float64) { c.CLGate += v },
	810: func(c *CommSubDBModel, v float64) { c.CLCy += v },
	811: func(c *CommSubDBModel, v float64) { c.CLFlank += v },
	812: func(c *CommSubDBModel, v float64) { c.CLFront += v },
	813: func(c *CommSubDBModel, v float64) { c.CLMelee += v },
	814: func(c *CommSubDBModel, v float64) { c.CLRange += v },
	815: func(c *CommSubDBModel, v float64) { c.CLFire += v },
	816: func(c *CommSubDBModel, v float64) { c.CLGlory += v },

	//hero bonus
	20012: func(c *CommSubDBModel, v float64) { c.EliteStr += v },
	20013: func(c *CommSubDBModel, v float64) { c.HorrorStr += v },
	20015: func(c *CommSubDBModel, v float64) { c.BeserkerStr += v },
	20016: func(c *CommSubDBModel, v float64) { c.RelicStr += v },
	20017: func(c *CommSubDBModel, v float64) { c.MeadStr += v },
	20018: func(c *CommSubDBModel, v float64) { c.Wave += v },
	20019: func(c *CommSubDBModel, v float64) { c.Cooldown += v },
	20020: func(c *CommSubDBModel, v float64) { c.Travel += v },
}
