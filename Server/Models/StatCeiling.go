package Models

// CommStatCeilings defines the maximum values for different stat categories.
// It mirrors the structure of CommStatModel to allow for easy ceiling application.
type CommStatCeilings struct {
	// Base Stats
	MeleeCbtStr float64
	RangeCbtStr float64
	FrontCbtStr float64
	FlankCbtStr float64
	AllCbtStr   float64
	CyCbtStr    float64
	WallStr     float64
	GateStr     float64
	MoatStr     float64
	FlankLimit  float64
	FrontLimit  float64
	MeadStr     float64
	HorrorStr   float64
	EliteStr    float64
	Wave        float64
	Cooldown    float64
	RelicStr    float64
	BeserkerStr float64
	MaidenSupp  float64
	Travel      float64
	Loot        float64

	// NPC Stats
	NPCMelee float64
	NPCRange float64
	NPCFront float64
	NPCFlank float64
	NPCCy    float64
	NPCWall  float64
	NPCGate  float64
	NPCMoat  float64
	NPCGlory float64

	// CastleLord/PVP Stats
	CLMelee float64
	CLRange float64
	CLFront float64
	CLFlank float64
	CLCy    float64
	CLWall  float64
	CLGate  float64
	CLMoat  float64
	CLLater float64
	CLFire  float64
	CLGlory float64
}

var CommBaseStatCeilings = CommStatCeilings{
	// Base Stats - A value of 0 means no ceiling is applied.
	MeleeCbtStr: 140.0,
	RangeCbtStr: 140.0,
	FrontCbtStr: 20.0,
	FlankCbtStr: 20.0,
	AllCbtStr:   20.0,
	CyCbtStr:    100.0,
	WallStr:     160.0,
	GateStr:     160.0,
	MoatStr:     120.0,
	FlankLimit:  50.0,
	FrontLimit:  50.0,
	MaidenSupp:  1050.0,
	Travel:      100.0,
	Loot:        50.0,
	MeadStr:     20.0,
	HorrorStr:   50.0,
	EliteStr:    50.0,
	Wave:        1.0,
	Cooldown:    50.0,
	RelicStr:    50.0,
	BeserkerStr: 50.0,

	// CastleLord/PVP Stats
	CLMelee: 100.0,
	CLRange: 100.0,
	CLFront: 80.0,
	CLFlank: 80.0,
	CLCy:    120.0,
	CLWall:  120.0,
	CLGate:  120.0,
	CLMoat:  60.0,
	CLLater: 180.0,
	CLFire:  100.0,
	CLGlory: 60.0,
}

// Define the specific ceilings for each equipment group.
// You can adjust these values as needed.
var CommBaseEquipCeilings = CommStatCeilings{
	// Base Stats - A value of 0 means no ceiling is applied.
	MeleeCbtStr: 140.0,
	RangeCbtStr: 140.0,
	FrontCbtStr: 20.0,
	FlankCbtStr: 20.0,
	AllCbtStr:   20.0,
	CyCbtStr:    100.0,
	WallStr:     160.0,
	GateStr:     160.0,
	MoatStr:     120.0,
	FlankLimit:  50.0,
	FrontLimit:  50.0,
	MeadStr:     0.0,
	HorrorStr:   0.0,
	EliteStr:    0.0,
	Wave:        0.0,
	Cooldown:    1000.0,
	RelicStr:    0.0,
	BeserkerStr: 0.0,
	MaidenSupp:  1050.0,
	Travel:      150.0,
	Loot:        50.0,

	// NPC Stats
	NPCMelee: 0.0,
	NPCRange: 0.0,
	NPCFront: 0.0,
	NPCFlank: 0.0,
	NPCCy:    0.0,
	NPCWall:  0.0,
	NPCGate:  0.0,
	NPCMoat:  0.0,
	NPCGlory: 0.0,

	// CastleLord/PVP Stats
	CLMelee: 0.0,
	CLRange: 0.0,
	CLFront: 0.0,
	CLFlank: 0.0,
	CLCy:    0.0,
	CLWall:  0.0,
	CLGate:  0.0,
	CLMoat:  0.0,
	CLLater: 0.0,
	CLFire:  0.0,
	CLGlory: 0.0,
}

var CommHeroEquipCeilings = CommStatCeilings{
	// Base Stats
	MeleeCbtStr: 0.0,
	RangeCbtStr: 0.0,
	FrontCbtStr: 0.0,
	FlankCbtStr: 0.0,
	AllCbtStr:   0.0,
	CyCbtStr:    0.0,
	WallStr:     0.0,
	GateStr:     0.0,
	MoatStr:     0.0,
	FlankLimit:  0.0,
	FrontLimit:  0.0,
	MeadStr:     20.0,
	HorrorStr:   50.0,
	EliteStr:    50.0,
	Wave:        1.0,
	Cooldown:    50.0,
	RelicStr:    50.0,
	BeserkerStr: 50.0,
	MaidenSupp:  0.0,
	Travel:      1000.0,
	Loot:        0.0,

	// NPC Stats
	NPCMelee: 0.0,
	NPCRange: 0.0,
	NPCFront: 0.0,
	NPCFlank: 0.0,
	NPCCy:    0.0,
	NPCWall:  0.0,
	NPCGate:  0.0,
	NPCMoat:  0.0,
	NPCGlory: 0.0,

	// CastleLord/PVP Stats
	CLMelee: 50.0,
	CLRange: 50.0,
	CLFront: 40.0,
	CLFlank: 40.0,
	CLCy:    60.0,
	CLWall:  60.0,
	CLGate:  60.0,
	CLMoat:  30.0,
	CLLater: 90.0,
	CLFire:  50.0,
	CLGlory: 30.0,
}

var CommGemCeilings = CommStatCeilings{
	// Base Stats - A value of 0 means no ceiling is applied.
	MeleeCbtStr: 140.0,
	RangeCbtStr: 140.0,
	FrontCbtStr: 20.0,
	FlankCbtStr: 20.0,
	AllCbtStr:   20.0,
	CyCbtStr:    100.0,
	WallStr:     160.0,
	GateStr:     160.0,
	MoatStr:     120.0,
	FlankLimit:  50.0,
	FrontLimit:  50.0,
	MeadStr:     0.0,
	HorrorStr:   0.0,
	EliteStr:    0.0,
	Wave:        0.0,
	Cooldown:    1000.0,
	RelicStr:    0.0,
	BeserkerStr: 0.0,
	MaidenSupp:  1050.0,
	Travel:      100.0,
	Loot:        50.0,

	// NPC Stats
	NPCMelee: 0.0,
	NPCRange: 0.0,
	NPCFront: 0.0,
	NPCFlank: 0.0,
	NPCCy:    0.0,
	NPCWall:  0.0,
	NPCGate:  0.0,
	NPCMoat:  0.0,
	NPCGlory: 0.0,

	// CastleLord/PVP Stats
	CLMelee: 50.0,
	CLRange: 50.0,
	CLFront: 40.0,
	CLFlank: 40.0,
	CLCy:    60.0,
	CLWall:  60.0,
	CLGate:  60.0,
	CLMoat:  30.0,
	CLLater: 90.0,
	CLFire:  50.0,
	CLGlory: 30.0,
}

type CastStatCeilings struct {

	//base stats
	MeleeCbtStr   float64
	RangeCbtStr   float64
	OpCbtStr      float64
	MainCbtStr    float64
	CyCbtStr      float64
	AllCbtStr     float64
	FrontCbtStr   float64
	FlankCbtStr   float64
	WallStr       float64
	GateStr       float64
	MoatStr       float64
	WallLimit     float64
	ProtectorSupp float64
	Loot          float64
	Recruit       float64
	MeadProd      float64
	Research      float64
	Hospital      float64
	Construction  float64
	BaseRes       float64
	KingRes       float64
	PO            float64
	ResTransport  float64
	HoneyProd     float64
	MeadStorage   float64
	HoneyStorage  float64

	//npc stats
	NPCMelee     float64
	NPCRange     float64
	NPCFront     float64
	NPCFlank     float64
	NPCCy        float64
	NPCWall      float64
	NPCGate      float64
	NPCMoat      float64
	NPCWallLimit float64

	//cl stats
	CLMelee     float64
	CLRange     float64
	CLCy        float64
	CLWall      float64
	CLGate      float64
	CLMoat      float64
	CLWallLimit float64
	CLFire      float64
	CLGlory     float64
	CLEarly     float64
}

var CastBaseStatCeilings = CastStatCeilings{
	MeleeCbtStr:   140.0,
	RangeCbtStr:   140.0,
	WallStr:       160.0,
	GateStr:       160.0,
	MoatStr:       120.0,
	WallLimit:     50.0,
	ProtectorSupp: 1050.0,
	Loot:          50.0,
	CLMelee:       100.0,
	CLRange:       100.0,
	CLCy:          120.0,
	CLWall:        120.0,
	CLGate:        120.0,
	CLMoat:        60.0,
	CLWallLimit:   50.0,
	CLFire:        100.0,
	CLGlory:       60.0,
	CLEarly:       180.0,
}

var CastBaseEquipCeilings = CastStatCeilings{
	MeleeCbtStr: 140.0,
	RangeCbtStr: 140.0,
	WallStr:     160.0,
	GateStr:     160.0,
	MoatStr:     120.0,
	WallLimit:   50.0,
	Loot:        50.0,
}

var CastHeroEquipCeilings = CastStatCeilings{
	OpCbtStr:     50.0,
	MainCbtStr:   50.0,
	Recruit:      50.0,
	MeadProd:     50.0,
	Research:     50.0,
	Hospital:     50.0,
	Construction: 50.0,
	BaseRes:      50.0,
	KingRes:      50.0,
	PO:           50.0,
	ResTransport: 50.0,
	HoneyProd:    50.0,
	MeadStorage:  50.0,
	HoneyStorage: 50.0,
	NPCMelee:     50.0,
	NPCRange:     50.0,
	NPCFront:     50.0,
	NPCFlank:     50.0,
	NPCCy:        60.0,
	NPCWall:      60.0,
	NPCGate:      60.0,
	NPCMoat:      30.0,
	NPCWallLimit: 50.0,
	CLMelee:      50.0,
	CLRange:      50.0,
	CLCy:         60.0,
	CLWall:       60.0,
	CLGate:       60.0,
	CLMoat:       30.0,
	CLWallLimit:  50.0,
	CLFire:       50.0,
	CLGlory:      30.0,
	CLEarly:      90.0,
}

var CastGemCeilings = CastStatCeilings{
	MeleeCbtStr:  140.0,
	RangeCbtStr:  140.0,
	WallStr:      160.0,
	GateStr:      160.0,
	MoatStr:      120.0,
	WallLimit:    50.0,
	Loot:         50.0,
	NPCMelee:     50.0,
	NPCRange:     50.0,
	NPCFront:     50.0,
	NPCFlank:     50.0,
	NPCCy:        60.0,
	NPCWall:      60.0,
	NPCGate:      60.0,
	NPCMoat:      30.0,
	NPCWallLimit: 50.0,
	CLMelee:      50.0,
	CLRange:      50.0,
	CLCy:         60.0,
	CLWall:       60.0,
	CLGate:       60.0,
	CLMoat:       30.0,
	CLWallLimit:  50.0,
	CLFire:       50.0,
	CLGlory:      30.0,
	CLEarly:      90.0,
}
