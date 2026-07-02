package equipment

import "math"

type CommStatModel struct {
	//commDetails

	ID   float64 `json:"id"`
	Name string  `json:"name"`

	Equip1 float64 `json:"equip1"`
	Equip2 float64 `json:"equip2"`
	Equip3 float64 `json:"equip3"`
	Equip4 float64 `json:"equip4"`
	Hero   float64 `json:"hero"`
	Gem1   float64 `json:"gem1"`
	Gem2   float64 `json:"gem2"`
	Gem3   float64 `json:"gem3"`
	Gem4   float64 `json:"gem4"`

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
	Travel      float64 `json:"travel"`
	Loot        float64 `json:"loot"`

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

	ExtraStats []EquipmentExtraStat `json:"extraStats,omitempty"`
}

// CommDBUpdater defines the function signature for updating a CommStatModel field.
// The value is passed as a float32, so any necessary type conversions should be handled within the updater function.
type CommDBUpdater func(commDB *CommStatModel, value float64)

// StatUpdaterMap maps a stat ID to a function that updates the corresponding CommStatModel field.
var CommStatUpdaterMap = map[float64]CommDBUpdater{

	//fixed stats
	1: func(c *CommStatModel, v float64) { c.MeleeCbtStr += v },
	2: func(c *CommStatModel, v float64) { c.RangeCbtStr += v },
	3: func(c *CommStatModel, v float64) { c.WallStr += v },
	4: func(c *CommStatModel, v float64) { c.GateStr += v },
	5: func(c *CommStatModel, v float64) { c.MoatStr += v },
	6: func(c *CommStatModel, v float64) { c.Travel += v },
	7: func(c *CommStatModel, v float64) { c.Loot += v },

	//random relic on base equipment
	101: func(c *CommStatModel, v float64) { c.MeleeCbtStr += v },
	102: func(c *CommStatModel, v float64) { c.RangeCbtStr += v },
	103: func(c *CommStatModel, v float64) { c.WallStr += v },
	104: func(c *CommStatModel, v float64) { c.GateStr += v },
	105: func(c *CommStatModel, v float64) { c.MoatStr += v },
	106: func(c *CommStatModel, v float64) { c.Travel += v },
	107: func(c *CommStatModel, v float64) { c.Loot += v },
	108: func(c *CommStatModel, v float64) { c.MeleeCbtStr += v },
	109: func(c *CommStatModel, v float64) { c.RangeCbtStr += v },
	110: func(c *CommStatModel, v float64) { c.WallStr += v },
	111: func(c *CommStatModel, v float64) { c.GateStr += v },
	112: func(c *CommStatModel, v float64) { c.MoatStr += v },
	113: func(c *CommStatModel, v float64) { c.Travel += v },
	114: func(c *CommStatModel, v float64) { c.Loot += v },
	115: func(c *CommStatModel, v float64) { c.FlankLimit += v },
	116: func(c *CommStatModel, v float64) { c.CyCbtStr += v },
	117: func(c *CommStatModel, v float64) { c.FrontLimit += v },
	118: func(c *CommStatModel, v float64) { c.AllCbtStr += v },
	119: func(c *CommStatModel, v float64) { c.FrontCbtStr += v },
	120: func(c *CommStatModel, v float64) { c.FlankCbtStr += v },
	121: func(c *CommStatModel, v float64) { c.MaidenSupp += v },

	//relic samurai / npc scoped stats
	501: func(c *CommStatModel, v float64) { c.NPCWall += v },
	502: func(c *CommStatModel, v float64) { c.NPCGate += v },
	503: func(c *CommStatModel, v float64) { c.NPCMelee += v },
	504: func(c *CommStatModel, v float64) { c.NPCRange += v },
	505: func(c *CommStatModel, v float64) { c.NPCFlank += v },
	506: func(c *CommStatModel, v float64) { c.NPCCy += v },
	507: func(c *CommStatModel, v float64) { c.NPCWall += v },
	508: func(c *CommStatModel, v float64) { c.NPCGate += v },
	509: func(c *CommStatModel, v float64) { c.NPCMelee += v },
	510: func(c *CommStatModel, v float64) { c.NPCRange += v },
	511: func(c *CommStatModel, v float64) { c.NPCFlank += v },
	512: func(c *CommStatModel, v float64) { c.NPCCy += v },
	513: func(c *CommStatModel, v float64) { c.NPCFront += v },
	514: func(c *CommStatModel, v float64) { c.NPCMoat += v },

	//gem stats castle lord
	309: func(c *CommStatModel, v float64) { c.CLWall += v },
	310: func(c *CommStatModel, v float64) { c.CLGate += v },
	311: func(c *CommStatModel, v float64) { c.CLMelee += v },
	312: func(c *CommStatModel, v float64) { c.CLRange += v },
	313: func(c *CommStatModel, v float64) { c.CLFlank += v },
	314: func(c *CommStatModel, v float64) { c.CLCy += v },
	315: func(c *CommStatModel, v float64) { c.CLFront += v },
	316: func(c *CommStatModel, v float64) { c.CLLater += v },
	317: func(c *CommStatModel, v float64) { c.CLMoat += v },
	318: func(c *CommStatModel, v float64) { c.CLFire += v },
	319: func(c *CommStatModel, v float64) { c.CLGlory += v },

	//gem stats npc
	320: func(c *CommStatModel, v float64) { c.NPCGlory += v },
	209: func(c *CommStatModel, v float64) { c.NPCWall += v },
	210: func(c *CommStatModel, v float64) { c.NPCGate += v },
	211: func(c *CommStatModel, v float64) { c.NPCMelee += v },
	212: func(c *CommStatModel, v float64) { c.NPCRange += v },
	213: func(c *CommStatModel, v float64) { c.NPCFlank += v },
	214: func(c *CommStatModel, v float64) { c.NPCCy += v },
	215: func(c *CommStatModel, v float64) { c.NPCFront += v },
	216: func(c *CommStatModel, v float64) { c.NPCMoat += v },

	//hero castle lord
	806: func(c *CommStatModel, v float64) { c.CLLater += v },
	807: func(c *CommStatModel, v float64) { c.CLMoat += v },
	808: func(c *CommStatModel, v float64) { c.CLWall += v },
	809: func(c *CommStatModel, v float64) { c.CLGate += v },
	810: func(c *CommStatModel, v float64) { c.CLCy += v },
	811: func(c *CommStatModel, v float64) { c.CLFlank += v },
	812: func(c *CommStatModel, v float64) { c.CLFront += v },
	813: func(c *CommStatModel, v float64) { c.CLMelee += v },
	814: func(c *CommStatModel, v float64) { c.CLRange += v },
	815: func(c *CommStatModel, v float64) { c.CLFire += v },
	816: func(c *CommStatModel, v float64) { c.CLGlory += v },

	//hero bonus
	20012: func(c *CommStatModel, v float64) { c.EliteStr += v },
	20013: func(c *CommStatModel, v float64) { c.HorrorStr += v },
	20015: func(c *CommStatModel, v float64) { c.BeserkerStr += v },
	20016: func(c *CommStatModel, v float64) { c.RelicStr += v },
	20017: func(c *CommStatModel, v float64) { c.MeadStr += v },
	20018: func(c *CommStatModel, v float64) { c.Wave += v },
	20019: func(c *CommStatModel, v float64) { c.Cooldown += v },
	20020: func(c *CommStatModel, v float64) { c.Travel += v },
}

var CommStatArray []CommStatModel

var CommEquipCeiling = CommStatModel{
	ID:          0,
	Name:        "",
	Equip1:      0,
	Equip2:      0,
	Equip3:      0,
	Equip4:      0,
	Hero:        0,
	Gem1:        0,
	Gem2:        0,
	Gem3:        0,
	Gem4:        0,
	MeleeCbtStr: 140,
	RangeCbtStr: 140,
	FrontCbtStr: 20,
	FlankCbtStr: 20,
	AllCbtStr:   20,
	CyCbtStr:    100,
	WallStr:     160,
	GateStr:     160,
	MoatStr:     120,
	FlankLimit:  50,
	FrontLimit:  50,
	MeadStr:     0,
	HorrorStr:   0,
	EliteStr:    0,
	Wave:        0,
	Cooldown:    0,
	RelicStr:    0,
	BeserkerStr: 0,
	MaidenSupp:  1050,
	Travel:      100,
	Loot:        50,
	NPCMelee:    0,
	NPCRange:    0,
	NPCFront:    0,
	NPCFlank:    0,
	NPCCy:       0,
	NPCWall:     0,
	NPCGate:     0,
	NPCMoat:     0,
	NPCGlory:    0,
	CLMelee:     0,
	CLRange:     0,
	CLFront:     0,
	CLFlank:     0,
	CLCy:        0,
	CLWall:      0,
	CLGate:      0,
	CLMoat:      0,
	CLLater:     0,
	CLFire:      0,
	CLGlory:     0,
}

var CommHeroCeiling = CommStatModel{
	ID:          0,
	Name:        "",
	Equip1:      0,
	Equip2:      0,
	Equip3:      0,
	Equip4:      0,
	Hero:        0,
	Gem1:        0,
	Gem2:        0,
	Gem3:        0,
	Gem4:        0,
	MeleeCbtStr: 0,
	RangeCbtStr: 0,
	FrontCbtStr: 0,
	FlankCbtStr: 0,
	AllCbtStr:   0,
	CyCbtStr:    0,
	WallStr:     0,
	GateStr:     0,
	MoatStr:     0,
	FlankLimit:  0,
	FrontLimit:  0,
	MeadStr:     20,
	HorrorStr:   0,
	EliteStr:    0,
	Wave:        1,
	Cooldown:    0,
	RelicStr:    0,
	BeserkerStr: 0,
	MaidenSupp:  0,
	Travel:      0,
	Loot:        0,
	NPCMelee:    50,
	NPCRange:    50,
	NPCFront:    40,
	NPCFlank:    40,
	NPCCy:       60,
	NPCWall:     60,
	NPCGate:     60,
	NPCMoat:     30,
	NPCGlory:    120,
	CLMelee:     50,
	CLRange:     50,
	CLFront:     40,
	CLFlank:     40,
	CLCy:        60,
	CLWall:      60,
	CLGate:      60,
	CLMoat:      30,
	CLLater:     90,
	CLFire:      50,
	CLGlory:     30,
}

var CommGemCeiling = CommStatModel{
	ID:          0,
	Name:        "",
	Equip1:      0,
	Equip2:      0,
	Equip3:      0,
	Equip4:      0,
	Hero:        0,
	Gem1:        0,
	Gem2:        0,
	Gem3:        0,
	Gem4:        0,
	MeleeCbtStr: 140,
	RangeCbtStr: 140,
	FrontCbtStr: 0,
	FlankCbtStr: 0,
	AllCbtStr:   0,
	CyCbtStr:    100,
	WallStr:     160,
	GateStr:     160,
	MoatStr:     120,
	FlankLimit:  50,
	FrontLimit:  50,
	MeadStr:     0,
	HorrorStr:   0,
	EliteStr:    0,
	Wave:        0,
	Cooldown:    0,
	RelicStr:    0,
	BeserkerStr: 0,
	MaidenSupp:  0,
	Travel:      0,
	Loot:        0,
	NPCMelee:    50,
	NPCRange:    50,
	NPCFront:    40,
	NPCFlank:    40,
	NPCCy:       60,
	NPCWall:     60,
	NPCGate:     60,
	NPCMoat:     30,
	NPCGlory:    30,
	CLMelee:     50,
	CLRange:     50,
	CLFront:     40,
	CLFlank:     40,
	CLCy:        60,
	CLWall:      60,
	CLGate:      60,
	CLMoat:      30,
	CLLater:     90,
	CLFire:      50,
	CLGlory:     30,
}

func ApplyCommCeiling(srcComm *CommStatModel, ceilingComm *CommStatModel) {
	if ceilingComm.MeleeCbtStr > 0 {
		srcComm.MeleeCbtStr = math.Min(srcComm.MeleeCbtStr, ceilingComm.MeleeCbtStr)
	}
	if ceilingComm.RangeCbtStr > 0 {
		srcComm.RangeCbtStr = math.Min(srcComm.RangeCbtStr, ceilingComm.RangeCbtStr)
	}
	if ceilingComm.FrontCbtStr > 0 {
		srcComm.FrontCbtStr = math.Min(srcComm.FrontCbtStr, ceilingComm.FrontCbtStr)
	}
	if ceilingComm.FlankCbtStr > 0 {
		srcComm.FlankCbtStr = math.Min(srcComm.FlankCbtStr, ceilingComm.FlankCbtStr)
	}
	if ceilingComm.AllCbtStr > 0 {
		srcComm.AllCbtStr = math.Min(srcComm.AllCbtStr, ceilingComm.AllCbtStr)
	}
	if ceilingComm.CyCbtStr > 0 {
		srcComm.CyCbtStr = math.Min(srcComm.CyCbtStr, ceilingComm.CyCbtStr)
	}
	if ceilingComm.WallStr > 0 {
		srcComm.WallStr = math.Min(srcComm.WallStr, ceilingComm.WallStr)
	}
	if ceilingComm.GateStr > 0 {
		srcComm.GateStr = math.Min(srcComm.GateStr, ceilingComm.GateStr)
	}
	if ceilingComm.MoatStr > 0 {
		srcComm.MoatStr = math.Min(srcComm.MoatStr, ceilingComm.MoatStr)
	}
	if ceilingComm.FlankLimit > 0 {
		srcComm.FlankLimit = math.Min(srcComm.FlankLimit, ceilingComm.FlankLimit)
	}
	if ceilingComm.FrontLimit > 0 {
		srcComm.FrontLimit = math.Min(srcComm.FrontLimit, ceilingComm.FrontLimit)
	}
	if ceilingComm.MeadStr > 0 {
		srcComm.MeadStr = math.Min(srcComm.MeadStr, ceilingComm.MeadStr)
	}
	if ceilingComm.HorrorStr > 0 {
		srcComm.HorrorStr = math.Min(srcComm.HorrorStr, ceilingComm.HorrorStr)
	}
	if ceilingComm.EliteStr > 0 {
		srcComm.EliteStr = math.Min(srcComm.EliteStr, ceilingComm.EliteStr)
	}
	if ceilingComm.Wave > 0 {
		srcComm.Wave = math.Min(srcComm.Wave, ceilingComm.Wave)
	}
	if ceilingComm.Cooldown > 0 {
		srcComm.Cooldown = math.Min(srcComm.Cooldown, ceilingComm.Cooldown)
	}
	if ceilingComm.RelicStr > 0 {
		srcComm.RelicStr = math.Min(srcComm.RelicStr, ceilingComm.RelicStr)
	}
	if ceilingComm.BeserkerStr > 0 {
		srcComm.BeserkerStr = math.Min(srcComm.BeserkerStr, ceilingComm.BeserkerStr)
	}
	if ceilingComm.MaidenSupp > 0 {
		srcComm.MaidenSupp = math.Min(srcComm.MaidenSupp, ceilingComm.MaidenSupp)
	}
	if ceilingComm.Travel > 0 {
		srcComm.Travel = math.Min(srcComm.Travel, ceilingComm.Travel)
	}
	if ceilingComm.Loot > 0 {
		srcComm.Loot = math.Min(srcComm.Loot, ceilingComm.Loot)
	}
	if ceilingComm.NPCMelee > 0 {
		srcComm.NPCMelee = math.Min(srcComm.NPCMelee, ceilingComm.NPCMelee)
	}
	if ceilingComm.NPCRange > 0 {
		srcComm.NPCRange = math.Min(srcComm.NPCRange, ceilingComm.NPCRange)
	}
	if ceilingComm.NPCFront > 0 {
		srcComm.NPCFront = math.Min(srcComm.NPCFront, ceilingComm.NPCFront)
	}
	if ceilingComm.NPCFlank > 0 {
		srcComm.NPCFlank = math.Min(srcComm.NPCFlank, ceilingComm.NPCFlank)
	}
	if ceilingComm.NPCCy > 0 {
		srcComm.NPCCy = math.Min(srcComm.NPCCy, ceilingComm.NPCCy)
	}
	if ceilingComm.NPCWall > 0 {
		srcComm.NPCWall = math.Min(srcComm.NPCWall, ceilingComm.NPCWall)
	}
	if ceilingComm.NPCGate > 0 {
		srcComm.NPCGate = math.Min(srcComm.NPCGate, ceilingComm.NPCGate)
	}
	if ceilingComm.NPCMoat > 0 {
		srcComm.NPCMoat = math.Min(srcComm.NPCMoat, ceilingComm.NPCMoat)
	}
	if ceilingComm.NPCGlory > 0 {
		srcComm.NPCGlory = math.Min(srcComm.NPCGlory, ceilingComm.NPCGlory)
	}
	if ceilingComm.CLMelee > 0 {
		srcComm.CLMelee = math.Min(srcComm.CLMelee, ceilingComm.CLMelee)
	}
	if ceilingComm.CLRange > 0 {
		srcComm.CLRange = math.Min(srcComm.CLRange, ceilingComm.CLRange)
	}
	if ceilingComm.CLFront > 0 {
		srcComm.CLFront = math.Min(srcComm.CLFront, ceilingComm.CLFront)
	}
	if ceilingComm.CLFlank > 0 {
		srcComm.CLFlank = math.Min(srcComm.CLFlank, ceilingComm.CLFlank)
	}
	if ceilingComm.CLCy > 0 {
		srcComm.CLCy = math.Min(srcComm.CLCy, ceilingComm.CLCy)
	}
	if ceilingComm.CLWall > 0 {
		srcComm.CLWall = math.Min(srcComm.CLWall, ceilingComm.CLWall)
	}
	if ceilingComm.CLGate > 0 {
		srcComm.CLGate = math.Min(srcComm.CLGate, ceilingComm.CLGate)
	}
	if ceilingComm.CLMoat > 0 {
		srcComm.CLMoat = math.Min(srcComm.CLMoat, ceilingComm.CLMoat)
	}
	if ceilingComm.CLLater > 0 {
		srcComm.CLLater = math.Min(srcComm.CLLater, ceilingComm.CLLater)
	}
	if ceilingComm.CLFire > 0 {
		srcComm.CLFire = math.Min(srcComm.CLFire, ceilingComm.CLFire)
	}
	if ceilingComm.CLGlory > 0 {
		srcComm.CLGlory = math.Min(srcComm.CLGlory, ceilingComm.CLGlory)
	}
}
