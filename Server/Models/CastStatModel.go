package Models

import "math"

type CastStatModel struct {
	Equip1   float64 `json:"equip1"`
	Equip2   float64 `json:"equip2"`
	Equip3   float64 `json:"equip3"`
	Equip4   float64 `json:"equip4"`
	Hero     float64 `json:"hero"`
	Gem1     float64 `json:"gem1"`
	Gem2     float64 `json:"gem2"`
	Gem3     float64 `json:"gem3"`
	Gem4     float64 `json:"gem4"`
	ID       float64 `json:"id"`
	CastleID float64 `json:"castleID"`
	Name     string  `json:"name"`

	MeleeCbtStr   float64 `json:"meleeCbtStr"`
	RangeCbtStr   float64 `json:"rangeCbtStr"`
	OpCbtStr      float64 `json:"opCbtStr"`
	MainCbtStr    float64 `json:"mainCbtStr"`
	CyCbtStr      float64 `json:"cyCbtStr"`
	AllCbtStr     float64 `json:"allCbtStr"`
	FrontCbtStr   float64 `json:"frontCbtStr"`
	FlankCbtStr   float64 `json:"flankCbtStr"`
	WallStr       float64 `json:"wallStr"`
	GateStr       float64 `json:"gateStr"`
	MoatStr       float64 `json:"moatStr"`
	WallLimit     float64 `json:"wallLimit"`
	ProtectorSupp float64 `json:"protectorSupp"`
	Loot          float64 `json:"lootStr"`
	Recruit       float64 `json:"recruit"`
	MeadProd      float64 `json:"meadProd"`
	Research      float64 `json:"research"`
	Hospital      float64 `json:"hospital"`
	Construction  float64 `json:"construction"`
	BaseRes       float64 `json:"baseRes"`
	KingRes       float64 `json:"kingRes"`
	PO            float64 `json:"po"`
	ResTransport  float64 `json:"resTransport"`
	HoneyProd     float64 `json:"honeyProd"`
	MeadStorage   float64 `json:"meadStorage"`
	HoneyStorage  float64 `json:"honeyStorage"`

	NPCMelee     float64 `json:"NPCMelee"`
	NPCRange     float64 `json:"NPCRange"`
	NPCFront     float64 `json:"NPCFront"`
	NPCFlank     float64 `json:"NPCFlank"`
	NPCCy        float64 `json:"NPCCy"`
	NPCWall      float64 `json:"NPCWall"`
	NPCGate      float64 `json:"NPCGate"`
	NPCMoat      float64 `json:"NPCMoat"`
	NPCWallLimit float64 `json:"NPCWallLimit"`

	CLMelee     float64 `json:"CLMelee"`
	CLRange     float64 `json:"CLRange"`
	CLCy        float64 `json:"CLCy"`
	CLWall      float64 `json:"CLWall"`
	CLGate      float64 `json:"CLGate"`
	CLMoat      float64 `json:"CLMoat"`
	CLWallLimit float64 `json:"CLWallLimit"`
	CLFire      float64 `json:"CLFire"`
	CLGlory     float64 `json:"CLGlory"`
	CLEarly     float64 `json:"CLEarly"`
}

type CastDBUpdater func(castDB *CastStatModel, value float64)

// CastStatUpdaterMap maps a stat ID to a function that updates the corresponding CastStatModel field.
var CastStatUpdaterMap = map[float64]CastDBUpdater{
	//fixed stats
	10001: func(c *CastStatModel, v float64) { c.Loot += v },
	10002: func(c *CastStatModel, v float64) { c.WallLimit += v },
	10003: func(c *CastStatModel, v float64) { c.GateStr += v },
	10004: func(c *CastStatModel, v float64) { c.MoatStr += v },
	10005: func(c *CastStatModel, v float64) { c.MeleeCbtStr += v },
	10006: func(c *CastStatModel, v float64) { c.RangeCbtStr += v },

	//random relic on base equipment
	10101: func(c *CastStatModel, v float64) { c.Loot += v },
	10102: func(c *CastStatModel, v float64) { c.WallStr += v },
	10103: func(c *CastStatModel, v float64) { c.GateStr += v },
	10108: func(c *CastStatModel, v float64) { c.WallStr += v },
	10109: func(c *CastStatModel, v float64) { c.GateStr += v },
	10110: func(c *CastStatModel, v float64) { c.MoatStr += v },
	10111: func(c *CastStatModel, v float64) { c.MeleeCbtStr += v },
	10112: func(c *CastStatModel, v float64) { c.RangeCbtStr += v },
	10113: func(c *CastStatModel, v float64) { c.WallLimit += v },
	10114: func(c *CastStatModel, v float64) { c.CyCbtStr += v },
	10115: func(c *CastStatModel, v float64) { c.AllCbtStr += v },
	10116: func(c *CastStatModel, v float64) { c.FrontCbtStr += v },
	10117: func(c *CastStatModel, v float64) { c.FlankCbtStr += v },
	10118: func(c *CastStatModel, v float64) { c.ProtectorSupp += v },

	//gem stats castle lord
	10301: func(c *CastStatModel, v float64) { c.WallLimit += v },
	10302: func(c *CastStatModel, v float64) { c.CyCbtStr += v },
	10303: func(c *CastStatModel, v float64) { c.CLWall += v },
	10304: func(c *CastStatModel, v float64) { c.CLGate += v },
	10305: func(c *CastStatModel, v float64) { c.CLMelee += v },
	10306: func(c *CastStatModel, v float64) { c.CLRange += v },
	10307: func(c *CastStatModel, v float64) { c.CLWallLimit += v },
	10308: func(c *CastStatModel, v float64) { c.CLCy += v },
	10309: func(c *CastStatModel, v float64) { c.CLWall += v },
	10310: func(c *CastStatModel, v float64) { c.CLGate += v },
	10311: func(c *CastStatModel, v float64) { c.CLMelee += v },
	10312: func(c *CastStatModel, v float64) { c.CLRange += v },
	10313: func(c *CastStatModel, v float64) { c.CLWallLimit += v },
	10314: func(c *CastStatModel, v float64) { c.CLCy += v },
	10315: func(c *CastStatModel, v float64) { c.CLEarly += v },
	10316: func(c *CastStatModel, v float64) { c.CLMoat += v },
	10317: func(c *CastStatModel, v float64) { c.CLFire += v },
	10318: func(c *CastStatModel, v float64) { c.CLGlory += v },

	//gem stats npc
	10201: func(c *CastStatModel, v float64) { c.WallLimit += v },
	10202: func(c *CastStatModel, v float64) { c.CyCbtStr += v },
	10203: func(c *CastStatModel, v float64) { c.NPCWall += v },
	10204: func(c *CastStatModel, v float64) { c.NPCGate += v },
	10205: func(c *CastStatModel, v float64) { c.NPCMelee += v },
	10206: func(c *CastStatModel, v float64) { c.NPCRange += v },
	10207: func(c *CastStatModel, v float64) { c.NPCWallLimit += v },
	10208: func(c *CastStatModel, v float64) { c.NPCCy += v },
	10209: func(c *CastStatModel, v float64) { c.NPCWall += v },
	10210: func(c *CastStatModel, v float64) { c.NPCGate += v },
	10211: func(c *CastStatModel, v float64) { c.NPCMelee += v },
	10212: func(c *CastStatModel, v float64) { c.NPCRange += v },
	10213: func(c *CastStatModel, v float64) { c.NPCWallLimit += v },
	10214: func(c *CastStatModel, v float64) { c.NPCCy += v },
	10215: func(c *CastStatModel, v float64) { c.NPCMoat += v },
	10216: func(c *CastStatModel, v float64) { c.Loot += v },
	10217: func(c *CastStatModel, v float64) { c.Loot += v },
	10218: func(c *CastStatModel, v float64) { c.Loot += v },
	10219: func(c *CastStatModel, v float64) { c.Loot += v },

	//hero castle lord
	10807: func(c *CastStatModel, v float64) { c.MainCbtStr += v },
	10808: func(c *CastStatModel, v float64) { c.OpCbtStr += v },
	10809: func(c *CastStatModel, v float64) { c.CLEarly += v },
	10810: func(c *CastStatModel, v float64) { c.CLMoat += v },
	10811: func(c *CastStatModel, v float64) { c.CLWall += v },
	10812: func(c *CastStatModel, v float64) { c.CLGate += v },
	10813: func(c *CastStatModel, v float64) { c.CLMelee += v },
	10814: func(c *CastStatModel, v float64) { c.CLRange += v },
	10815: func(c *CastStatModel, v float64) { c.CLWallLimit += v },
	10816: func(c *CastStatModel, v float64) { c.CLCy += v },
	10817: func(c *CastStatModel, v float64) { c.CLFire += v },
	10818: func(c *CastStatModel, v float64) { c.CLGlory += v },

	//hero npc
	10409: func(c *CastStatModel, v float64) { c.NPCWall += v },
	10410: func(c *CastStatModel, v float64) { c.NPCGate += v },
	10411: func(c *CastStatModel, v float64) { c.NPCMelee += v },
	10412: func(c *CastStatModel, v float64) { c.NPCRange += v },
	10413: func(c *CastStatModel, v float64) { c.NPCWallLimit += v },
	10414: func(c *CastStatModel, v float64) { c.NPCCy += v },
	10415: func(c *CastStatModel, v float64) { c.NPCMoat += v },
	10416: func(c *CastStatModel, v float64) { c.Loot += v },
	10417: func(c *CastStatModel, v float64) { c.Loot += v },

	//hero bonus
	30001: func(castDB *CastStatModel, v float64) { castDB.Research += v },
	30009: func(castDB *CastStatModel, v float64) { castDB.Recruit += v },
	30010: func(castDB *CastStatModel, v float64) { castDB.Hospital += v },
	30011: func(castDB *CastStatModel, v float64) { castDB.Construction += v },
	30012: func(castDB *CastStatModel, v float64) { castDB.BaseRes += v },
	30013: func(castDB *CastStatModel, v float64) { castDB.KingRes += v },
	30014: func(castDB *CastStatModel, v float64) { castDB.PO += v },
	30016: func(castDB *CastStatModel, v float64) { castDB.ResTransport += v },
	30017: func(castDB *CastStatModel, v float64) { castDB.MeadProd += v },
	30018: func(castDB *CastStatModel, v float64) { castDB.HoneyProd += v },
	30019: func(castDB *CastStatModel, v float64) { castDB.MeadStorage += v },
	30020: func(castDB *CastStatModel, v float64) { castDB.HoneyStorage += v },
}

var CastStatArray struct {
	MainCastleCast    CastStatModel `json:"mainCastleCast"`
	Outpost1Cast      CastStatModel `json:"outpost1Cast"`
	Outpost2Cast      CastStatModel `json:"outpost2Cast"`
	Outpost3Cast      CastStatModel `json:"outpost3Cast"`
	IceCastleCast     CastStatModel `json:"iceCastleCast"`
	DesertCastleCast  CastStatModel `json:"desertCastleCast"`
	DungeonCastleCast CastStatModel `json:"dungeonCastleCast"`
	StormCastleCast   CastStatModel `json:"stormCastleCast"`
	ExtraCast1        CastStatModel `json:"extraCast1"`
	ExtraCast2        CastStatModel `json:"extraCast2"`
	ExtraCast3        CastStatModel `json:"extraCast3"`
	ExtraCast4        CastStatModel `json:"extraCast4"`
	ExtraCast5        CastStatModel `json:"extraCast5"`
}

var CastEquipCeiling = CastStatModel{
	Equip1:   0,
	Equip2:   0,
	Equip3:   0,
	Equip4:   0,
	Hero:     0,
	Gem1:     0,
	Gem2:     0,
	Gem3:     0,
	Gem4:     0,
	ID:       0,
	CastleID: 0,
	Name:     "",

	MeleeCbtStr:   140,
	RangeCbtStr:   140,
	OpCbtStr:      25,
	MainCbtStr:    25,
	CyCbtStr:      100,
	AllCbtStr:     20,
	FrontCbtStr:   20,
	FlankCbtStr:   20,
	WallStr:       160,
	GateStr:       160,
	MoatStr:       120,
	WallLimit:     50,
	ProtectorSupp: 1050,
	Loot:          50,
	Recruit:       0,
	MeadProd:      0,
	Research:      0,
	Hospital:      0,
	Construction:  0,
	BaseRes:       0,
	KingRes:       0,
	PO:            0,
	ResTransport:  0,
	HoneyProd:     0,
	MeadStorage:   0,
	HoneyStorage:  0,

	NPCMelee:     0,
	NPCRange:     0,
	NPCFront:     0,
	NPCFlank:     0,
	NPCCy:        0,
	NPCWall:      0,
	NPCGate:      0,
	NPCMoat:      0,
	NPCWallLimit: 0,

	CLMelee:     0,
	CLRange:     0,
	CLCy:        0,
	CLWall:      0,
	CLGate:      0,
	CLMoat:      0,
	CLWallLimit: 0,
	CLFire:      0,
	CLGlory:     0,
	CLEarly:     0,
}

var CastHeroCeiling = CastStatModel{
	Equip1:   0,
	Equip2:   0,
	Equip3:   0,
	Equip4:   0,
	Hero:     0,
	Gem1:     0,
	Gem2:     0,
	Gem3:     0,
	Gem4:     0,
	ID:       0,
	CastleID: 0,
	Name:     "",

	MeleeCbtStr:   0,
	RangeCbtStr:   0,
	OpCbtStr:      0,
	MainCbtStr:    0,
	CyCbtStr:      0,
	AllCbtStr:     0,
	FrontCbtStr:   0,
	FlankCbtStr:   0,
	WallStr:       0,
	GateStr:       0,
	MoatStr:       0,
	WallLimit:     0,
	ProtectorSupp: 0,
	Loot:          0,
	Recruit:       0,
	MeadProd:      0,
	Research:      0,
	Hospital:      0,
	Construction:  0,
	BaseRes:       0,
	KingRes:       0,
	PO:            0,
	ResTransport:  0,
	HoneyProd:     0,
	MeadStorage:   0,
	HoneyStorage:  0,

	NPCMelee:     50,
	NPCRange:     50,
	NPCFront:     0,
	NPCFlank:     0,
	NPCCy:        60,
	NPCWall:      60,
	NPCGate:      60,
	NPCMoat:      30,
	NPCWallLimit: 40,

	CLMelee:     50,
	CLRange:     50,
	CLCy:        60,
	CLWall:      60,
	CLGate:      60,
	CLMoat:      30,
	CLWallLimit: 40,
	CLFire:      0,
	CLGlory:     30,
	CLEarly:     0,
}

var CastGemCeiling = CastStatModel{
	Equip1:   0,
	Equip2:   0,
	Equip3:   0,
	Equip4:   0,
	Hero:     0,
	Gem1:     0,
	Gem2:     0,
	Gem3:     0,
	Gem4:     0,
	ID:       0,
	CastleID: 0,
	Name:     "",

	MeleeCbtStr:   0,
	RangeCbtStr:   0,
	OpCbtStr:      0,
	MainCbtStr:    0,
	CyCbtStr:      0,
	AllCbtStr:     0,
	FrontCbtStr:   0,
	FlankCbtStr:   0,
	WallStr:       0,
	GateStr:       0,
	MoatStr:       0,
	WallLimit:     0,
	ProtectorSupp: 0,
	Loot:          0,
	Recruit:       0,
	MeadProd:      0,
	Research:      0,
	Hospital:      0,
	Construction:  0,
	BaseRes:       0,
	KingRes:       0,
	PO:            0,
	ResTransport:  0,
	HoneyProd:     0,
	MeadStorage:   0,
	HoneyStorage:  0,

	NPCMelee:     50,
	NPCRange:     50,
	NPCFront:     0,
	NPCFlank:     0,
	NPCCy:        60,
	NPCWall:      60,
	NPCGate:      60,
	NPCMoat:      30,
	NPCWallLimit: 40,

	CLMelee:     50,
	CLRange:     50,
	CLCy:        60,
	CLWall:      60,
	CLGate:      60,
	CLMoat:      30,
	CLWallLimit: 40,
	CLFire:      0,
	CLGlory:     30,
	CLEarly:     90,
}

func ApplyCastCeiling(srcCast *CastStatModel, ceilingCast *CastStatModel) {
	if ceilingCast.MeleeCbtStr > 0 {
		srcCast.MeleeCbtStr = math.Min(srcCast.MeleeCbtStr, ceilingCast.MeleeCbtStr)
	}
	if ceilingCast.RangeCbtStr > 0 {
		srcCast.RangeCbtStr = math.Min(srcCast.RangeCbtStr, ceilingCast.RangeCbtStr)
	}
	if ceilingCast.OpCbtStr > 0 {
		srcCast.OpCbtStr = math.Min(srcCast.OpCbtStr, ceilingCast.OpCbtStr)
	}
	if ceilingCast.MainCbtStr > 0 {
		srcCast.MainCbtStr = math.Min(srcCast.MainCbtStr, ceilingCast.MainCbtStr)
	}
	if ceilingCast.CyCbtStr > 0 {
		srcCast.CyCbtStr = math.Min(srcCast.CyCbtStr, ceilingCast.CyCbtStr)
	}
	if ceilingCast.AllCbtStr > 0 {
		srcCast.AllCbtStr = math.Min(srcCast.AllCbtStr, ceilingCast.AllCbtStr)
	}
	if ceilingCast.FrontCbtStr > 0 {
		srcCast.FrontCbtStr = math.Min(srcCast.FrontCbtStr, ceilingCast.FrontCbtStr)
	}
	if ceilingCast.FlankCbtStr > 0 {
		srcCast.FlankCbtStr = math.Min(srcCast.FlankCbtStr, ceilingCast.FlankCbtStr)
	}
	if ceilingCast.WallStr > 0 {
		srcCast.WallStr = math.Min(srcCast.WallStr, ceilingCast.WallStr)
	}
	if ceilingCast.GateStr > 0 {
		srcCast.GateStr = math.Min(srcCast.GateStr, ceilingCast.GateStr)
	}
	if ceilingCast.MoatStr > 0 {
		srcCast.MoatStr = math.Min(srcCast.MoatStr, ceilingCast.MoatStr)
	}
	if ceilingCast.WallLimit > 0 {
		srcCast.WallLimit = math.Min(srcCast.WallLimit, ceilingCast.WallLimit)
	}
	if ceilingCast.ProtectorSupp > 0 {
		srcCast.ProtectorSupp = math.Min(srcCast.ProtectorSupp, ceilingCast.ProtectorSupp)
	}
	if ceilingCast.Loot > 0 {
		srcCast.Loot = math.Min(srcCast.Loot, ceilingCast.Loot)
	}
	if ceilingCast.Recruit > 0 {
		srcCast.Recruit = math.Min(srcCast.Recruit, ceilingCast.Recruit)
	}
	if ceilingCast.MeadProd > 0 {
		srcCast.MeadProd = math.Min(srcCast.MeadProd, ceilingCast.MeadProd)
	}
	if ceilingCast.Research > 0 {
		srcCast.Research = math.Min(srcCast.Research, ceilingCast.Research)
	}
	if ceilingCast.Hospital > 0 {
		srcCast.Hospital = math.Min(srcCast.Hospital, ceilingCast.Hospital)
	}
	if ceilingCast.Construction > 0 {
		srcCast.Construction = math.Min(srcCast.Construction, ceilingCast.Construction)
	}
	if ceilingCast.BaseRes > 0 {
		srcCast.BaseRes = math.Min(srcCast.BaseRes, ceilingCast.BaseRes)
	}
	if ceilingCast.KingRes > 0 {
		srcCast.KingRes = math.Min(srcCast.KingRes, ceilingCast.KingRes)
	}
	if ceilingCast.PO > 0 {
		srcCast.PO = math.Min(srcCast.PO, ceilingCast.PO)
	}
	if ceilingCast.ResTransport > 0 {
		srcCast.ResTransport = math.Min(srcCast.ResTransport, ceilingCast.ResTransport)
	}
	if ceilingCast.HoneyProd > 0 {
		srcCast.HoneyProd = math.Min(srcCast.HoneyProd, ceilingCast.HoneyProd)
	}
	if ceilingCast.MeadStorage > 0 {
		srcCast.MeadStorage = math.Min(srcCast.MeadStorage, ceilingCast.MeadStorage)
	}
	if ceilingCast.HoneyStorage > 0 {
		srcCast.HoneyStorage = math.Min(srcCast.HoneyStorage, ceilingCast.HoneyStorage)
	}
	if ceilingCast.NPCMelee > 0 {
		srcCast.NPCMelee = math.Min(srcCast.NPCMelee, ceilingCast.NPCMelee)
	}
	if ceilingCast.NPCRange > 0 {
		srcCast.NPCRange = math.Min(srcCast.NPCRange, ceilingCast.NPCRange)
	}
	if ceilingCast.NPCFront > 0 {
		srcCast.NPCFront = math.Min(srcCast.NPCFront, ceilingCast.NPCFront)
	}
	if ceilingCast.NPCFlank > 0 {
		srcCast.NPCFlank = math.Min(srcCast.NPCFlank, ceilingCast.NPCFlank)
	}
	if ceilingCast.NPCCy > 0 {
		srcCast.NPCCy = math.Min(srcCast.NPCCy, ceilingCast.NPCCy)
	}
	if ceilingCast.NPCWall > 0 {
		srcCast.NPCWall = math.Min(srcCast.NPCWall, ceilingCast.NPCWall)
	}
	if ceilingCast.NPCGate > 0 {
		srcCast.NPCGate = math.Min(srcCast.NPCGate, ceilingCast.NPCGate)
	}
	if ceilingCast.NPCMoat > 0 {
		srcCast.NPCMoat = math.Min(srcCast.NPCMoat, ceilingCast.NPCMoat)
	}
	if ceilingCast.NPCWallLimit > 0 {
		srcCast.NPCWallLimit = math.Min(srcCast.NPCWallLimit, ceilingCast.NPCWallLimit)
	}
	if ceilingCast.CLMelee > 0 {
		srcCast.CLMelee = math.Min(srcCast.CLMelee, ceilingCast.CLMelee)
	}
	if ceilingCast.CLRange > 0 {
		srcCast.CLRange = math.Min(srcCast.CLRange, ceilingCast.CLRange)
	}
	if ceilingCast.CLCy > 0 {
		srcCast.CLCy = math.Min(srcCast.CLCy, ceilingCast.CLCy)
	}
	if ceilingCast.CLWall > 0 {
		srcCast.CLWall = math.Min(srcCast.CLWall, ceilingCast.CLWall)
	}
	if ceilingCast.CLGate > 0 {
		srcCast.CLGate = math.Min(srcCast.CLGate, ceilingCast.CLGate)
	}
	if ceilingCast.CLMoat > 0 {
		srcCast.CLMoat = math.Min(srcCast.CLMoat, ceilingCast.CLMoat)
	}
	if ceilingCast.CLWallLimit > 0 {
		srcCast.CLWallLimit = math.Min(srcCast.CLWallLimit, ceilingCast.CLWallLimit)
	}
	if ceilingCast.CLFire > 0 {
		srcCast.CLFire = math.Min(srcCast.CLFire, ceilingCast.CLFire)
	}
	if ceilingCast.CLGlory > 0 {
		srcCast.CLGlory = math.Min(srcCast.CLGlory, ceilingCast.CLGlory)
	}
	if ceilingCast.CLEarly > 0 {
		srcCast.CLEarly = math.Min(srcCast.CLEarly, ceilingCast.CLEarly)
	}
}
