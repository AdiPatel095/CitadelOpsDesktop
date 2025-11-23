package models

type CastDBModel struct {
	IGNString  string         `json:"ignString"`
	CastActual CastSubDBModel `json:"castActual"`
}
type CastSubDBModel struct {
	Equip1 int `json:"equip1"`
	Equip2 int `json:"equip2"`
	Equip3 int `json:"equip3"`
	Equip4 int `json:"equip4"`
	Hero   int `json:"hero"`
	Gem1   int `json:"gem1"`
	Gem2   int `json:"gem2"`
	Gem3   int `json:"gem3"`
	Gem4   int `json:"gem4"`

	ID             int    `json:"id"`
	Name           string `json:"name"`
	CastlePosition int    `json:"castlePosition"`

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

type CastDBUpdater func(castDB *CastSubDBModel, value float64)

// CastStatUpdaterMap maps a stat ID to a function that updates the corresponding CastSubDBModel field.
var CastStatUpdaterMap = map[int]CastDBUpdater{
	//fixed stats
	10001: func(c *CastSubDBModel, v float64) { c.Loot += v },
	10002: func(c *CastSubDBModel, v float64) { c.WallLimit += v },
	10003: func(c *CastSubDBModel, v float64) { c.GateStr += v },
	10004: func(c *CastSubDBModel, v float64) { c.MoatStr += v },
	10005: func(c *CastSubDBModel, v float64) { c.MeleeCbtStr += v },
	10006: func(c *CastSubDBModel, v float64) { c.RangeCbtStr += v },

	//random relic on base equipment
	10101: func(c *CastSubDBModel, v float64) { c.Loot += v },
	10102: func(c *CastSubDBModel, v float64) { c.WallStr += v },
	10103: func(c *CastSubDBModel, v float64) { c.GateStr += v },
	10108: func(c *CastSubDBModel, v float64) { c.WallStr += v },
	10109: func(c *CastSubDBModel, v float64) { c.GateStr += v },
	10110: func(c *CastSubDBModel, v float64) { c.MoatStr += v },
	10111: func(c *CastSubDBModel, v float64) { c.MeleeCbtStr += v },
	10112: func(c *CastSubDBModel, v float64) { c.RangeCbtStr += v },
	10113: func(c *CastSubDBModel, v float64) { c.WallLimit += v },
	10114: func(c *CastSubDBModel, v float64) { c.CyCbtStr += v },
	10115: func(c *CastSubDBModel, v float64) { c.AllCbtStr += v },
	10116: func(c *CastSubDBModel, v float64) { c.FrontCbtStr += v },
	10117: func(c *CastSubDBModel, v float64) { c.FlankCbtStr += v },
	10118: func(c *CastSubDBModel, v float64) { c.ProtectorSupp += v },

	//gem stats castle lord
	10301: func(c *CastSubDBModel, v float64) { c.WallLimit += v },
	10302: func(c *CastSubDBModel, v float64) { c.CyCbtStr += v },
	10303: func(c *CastSubDBModel, v float64) { c.CLWall += v },
	10304: func(c *CastSubDBModel, v float64) { c.CLGate += v },
	10305: func(c *CastSubDBModel, v float64) { c.CLMelee += v },
	10306: func(c *CastSubDBModel, v float64) { c.CLRange += v },
	10307: func(c *CastSubDBModel, v float64) { c.CLWallLimit += v },
	10308: func(c *CastSubDBModel, v float64) { c.CLCy += v },
	10309: func(c *CastSubDBModel, v float64) { c.CLWall += v },
	10310: func(c *CastSubDBModel, v float64) { c.CLGate += v },
	10311: func(c *CastSubDBModel, v float64) { c.CLMelee += v },
	10312: func(c *CastSubDBModel, v float64) { c.CLRange += v },
	10313: func(c *CastSubDBModel, v float64) { c.CLWallLimit += v },
	10314: func(c *CastSubDBModel, v float64) { c.CLCy += v },
	10315: func(c *CastSubDBModel, v float64) { c.CLEarly += v },
	10316: func(c *CastSubDBModel, v float64) { c.CLMoat += v },
	10317: func(c *CastSubDBModel, v float64) { c.CLFire += v },
	10318: func(c *CastSubDBModel, v float64) { c.CLGlory += v },

	//gem stats npc
	10201: func(c *CastSubDBModel, v float64) { c.WallLimit += v },
	10202: func(c *CastSubDBModel, v float64) { c.CyCbtStr += v },
	10203: func(c *CastSubDBModel, v float64) { c.NPCWall += v },
	10204: func(c *CastSubDBModel, v float64) { c.NPCGate += v },
	10205: func(c *CastSubDBModel, v float64) { c.NPCMelee += v },
	10206: func(c *CastSubDBModel, v float64) { c.NPCRange += v },
	10207: func(c *CastSubDBModel, v float64) { c.NPCWallLimit += v },
	10208: func(c *CastSubDBModel, v float64) { c.NPCCy += v },
	10209: func(c *CastSubDBModel, v float64) { c.NPCWall += v },
	10210: func(c *CastSubDBModel, v float64) { c.NPCGate += v },
	10211: func(c *CastSubDBModel, v float64) { c.NPCMelee += v },
	10212: func(c *CastSubDBModel, v float64) { c.NPCRange += v },
	10213: func(c *CastSubDBModel, v float64) { c.NPCWallLimit += v },
	10214: func(c *CastSubDBModel, v float64) { c.NPCCy += v },
	10215: func(c *CastSubDBModel, v float64) { c.NPCMoat += v },
	10216: func(c *CastSubDBModel, v float64) { c.Loot += v },
	10217: func(c *CastSubDBModel, v float64) { c.Loot += v },
	10218: func(c *CastSubDBModel, v float64) { c.Loot += v },
	10219: func(c *CastSubDBModel, v float64) { c.Loot += v },

	//hero castle lord
	10807: func(c *CastSubDBModel, v float64) { c.MainCbtStr += v },
	10808: func(c *CastSubDBModel, v float64) { c.OpCbtStr += v },
	10809: func(c *CastSubDBModel, v float64) { c.CLEarly += v },
	10810: func(c *CastSubDBModel, v float64) { c.CLMoat += v },
	10811: func(c *CastSubDBModel, v float64) { c.CLWall += v },
	10812: func(c *CastSubDBModel, v float64) { c.CLGate += v },
	10813: func(c *CastSubDBModel, v float64) { c.CLMelee += v },
	10814: func(c *CastSubDBModel, v float64) { c.CLRange += v },
	10815: func(c *CastSubDBModel, v float64) { c.CLWallLimit += v },
	10816: func(c *CastSubDBModel, v float64) { c.CLCy += v },
	10817: func(c *CastSubDBModel, v float64) { c.CLFire += v },
	10818: func(c *CastSubDBModel, v float64) { c.CLGlory += v },

	//hero npc
	10409: func(c *CastSubDBModel, v float64) { c.NPCWall += v },
	10410: func(c *CastSubDBModel, v float64) { c.NPCGate += v },
	10411: func(c *CastSubDBModel, v float64) { c.NPCMelee += v },
	10412: func(c *CastSubDBModel, v float64) { c.NPCRange += v },
	10413: func(c *CastSubDBModel, v float64) { c.NPCWallLimit += v },
	10414: func(c *CastSubDBModel, v float64) { c.NPCCy += v },
	10415: func(c *CastSubDBModel, v float64) { c.NPCMoat += v },
	10416: func(c *CastSubDBModel, v float64) { c.Loot += v },
	10417: func(c *CastSubDBModel, v float64) { c.Loot += v },

	//hero bonus
	30001: func(castDB *CastSubDBModel, v float64) { castDB.Research += v },
	30009: func(castDB *CastSubDBModel, v float64) { castDB.Recruit += v },
	30010: func(castDB *CastSubDBModel, v float64) { castDB.Hospital += v },
	30011: func(castDB *CastSubDBModel, v float64) { castDB.Construction += v },
	30012: func(castDB *CastSubDBModel, v float64) { castDB.BaseRes += v },
	30013: func(castDB *CastSubDBModel, v float64) { castDB.KingRes += v },
	30014: func(castDB *CastSubDBModel, v float64) { castDB.PO += v },
	30016: func(castDB *CastSubDBModel, v float64) { castDB.ResTransport += v },
	30017: func(castDB *CastSubDBModel, v float64) { castDB.MeadProd += v },
	30018: func(castDB *CastSubDBModel, v float64) { castDB.HoneyProd += v },
	30019: func(castDB *CastSubDBModel, v float64) { castDB.MeadStorage += v },
	30020: func(castDB *CastSubDBModel, v float64) { castDB.HoneyStorage += v },
}
