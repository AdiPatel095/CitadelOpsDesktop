package Reports

type Player struct {
	ID       int64  `json:"id,omitempty"`
	Name     string `json:"name,omitempty"`
	Alliance string `json:"alliance,omitempty"`
	Dummy    *bool  `json:"dummy,omitempty"`
}

type Castle struct {
	ID         int64  `json:"id,omitempty"`
	Name       string `json:"name,omitempty"`
	KingdomID  int    `json:"kingdomID,omitempty"`
	TypeID     int    `json:"typeID,omitempty"`
	X          int    `json:"x,omitempty"`
	Y          int    `json:"y,omitempty"`
	WallLevel  int    `json:"wallLevel,omitempty"`
	GateLevel  int    `json:"gateLevel,omitempty"`
	MoatLevel  int    `json:"moatLevel,omitempty"`
	KeepLevel  int    `json:"keepLevel,omitempty"`
	TowerLevel int    `json:"towerLevel,omitempty"`
}

type UnitCount struct {
	WodID  int64 `json:"wodID"`
	Amount int64 `json:"amount"`
}

type SpySection struct {
	Index int         `json:"index"`
	Name  string      `json:"name"`
	Units []UnitCount `json:"units"`
	Total int64       `json:"total"`
}

type SpyCastellan struct {
	Level                  int   `json:"level,omitempty"`
	GeneralID              int64 `json:"generalID,omitempty"`
	Effects                any   `json:"effects,omitempty"`
	Equipment              any   `json:"equipment,omitempty"`
	SkillIDs               any   `json:"skillIDs,omitempty"`
	GeneralAbilitySkillIDs any   `json:"generalAbilitySkillIDs,omitempty"`
}

type SpyReport struct {
	ID                   string        `json:"id"`
	MID                  int64         `json:"mid"`
	CapturedAtUnixMillis int64         `json:"capturedAtUnixMillis"`
	Status               string        `json:"status"`
	Accuracy             int           `json:"accuracy,omitempty"`
	Risk                 int           `json:"risk,omitempty"`
	SpyCount             int           `json:"spyCount,omitempty"`
	GuardCount           int           `json:"guardCount,omitempty"`
	Target               Player        `json:"target"`
	Source               Player        `json:"source"`
	Castle               Castle        `json:"castle"`
	Setup                []SpySection  `json:"setup,omitempty"`
	TotalTroops          int64         `json:"totalTroops,omitempty"`
	Castellan            *SpyCastellan `json:"castellan,omitempty"`
}

type BattleCombatant struct {
	PlayerID   int64  `json:"playerID,omitempty"`
	AllianceID int64  `json:"allianceID,omitempty"`
	Dummy      bool   `json:"dummy,omitempty"`
	Name       string `json:"name,omitempty"`
	Alliance   string `json:"alliance,omitempty"`
	CastleName string `json:"castleName,omitempty"`
	Role       string `json:"role,omitempty"`
}

type BattleMetrics struct {
	AttackerSent      int64   `json:"attackerSent,omitempty"`
	AttackerLost      int64   `json:"attackerLost,omitempty"`
	AttackersKilled   int64   `json:"attackersKilled,omitempty"`
	DefenderStationed int64   `json:"defenderStationed,omitempty"`
	DefenderLost      int64   `json:"defenderLost,omitempty"`
	DefendersKilled   int64   `json:"defendersKilled,omitempty"`
	AttackTradeRatio  float64 `json:"attackTradeRatio,omitempty"`
	DefenseTradeRatio float64 `json:"defenseTradeRatio,omitempty"`
}

type BattleItemDetail struct {
	Side   string `json:"side,omitempty"`
	Phase  string `json:"phase,omitempty"`
	Lane   string `json:"lane,omitempty"`
	WodID  int64  `json:"wodID,omitempty"`
	Amount int64  `json:"amount,omitempty"`
	Lost   int64  `json:"lost,omitempty"`
	Used   int64  `json:"used,omitempty"`
}

type BattleEffect struct {
	DefinitionID int64     `json:"definitionId"`
	Values       []float64 `json:"values"`
	Source       string    `json:"source,omitempty"`
	Side         string    `json:"side,omitempty"`
}

type BattleWaveLane struct {
	Lane                string             `json:"lane"`
	Result              string             `json:"result,omitempty"`
	AttackerLost        int64              `json:"attackerLost,omitempty"`
	DefenderLost        int64              `json:"defenderLost,omitempty"`
	AttackerStart       int64              `json:"attackerStart,omitempty"`
	DefenderStart       int64              `json:"defenderStart,omitempty"`
	AttackerUnitDetails []BattleItemDetail `json:"attackerUnitDetails,omitempty"`
	DefenderUnitDetails []BattleItemDetail `json:"defenderUnitDetails,omitempty"`
	AttackerToolDetails []BattleItemDetail `json:"attackerToolDetails,omitempty"`
	DefenderToolDetails []BattleItemDetail `json:"defenderToolDetails,omitempty"`
}

type BattleWave struct {
	Index  int              `json:"index"`
	Wave   int              `json:"wave"`
	Result string           `json:"result,omitempty"`
	Lanes  []BattleWaveLane `json:"lanes,omitempty"`
}

type BattleReport struct {
	ID                    string             `json:"id"`
	ReportID              string             `json:"reportID"`
	AccountUID            int64              `json:"accountUID,omitempty"`
	WorldID               string             `json:"worldID,omitempty"`
	PlayerID              int64              `json:"playerID,omitempty"`
	BattleKey             string             `json:"battleKey,omitempty"`
	AutomationFeature     string             `json:"automationFeature,omitempty"`
	MovementID            int64              `json:"movementId,omitempty"`
	EventID               int64              `json:"eventId,omitempty"`
	EventActivity         string             `json:"eventActivity,omitempty"`
	EventOccurrenceEndsAt string             `json:"eventOccurrenceEndsAt,omitempty"`
	MID                   int64              `json:"mid"`
	LID                   int64              `json:"lid"`
	KingdomID             int                `json:"kingdomID"`
	KingdomKnown          bool               `json:"kingdomKnown,omitempty"`
	TargetX               int                `json:"targetX,omitempty"`
	TargetY               int                `json:"targetY,omitempty"`
	TargetName            string             `json:"targetName,omitempty"`
	CastleName            string             `json:"castleName,omitempty"`
	BattleTypeID          int                `json:"battleTypeID,omitempty"`
	BattleType            string             `json:"battleType,omitempty"`
	TargetTypeID          int                `json:"targetTypeID,omitempty"`
	TargetType            string             `json:"targetType,omitempty"`
	OccurredAt            string             `json:"occurredAt"`
	DateMs                int64              `json:"dateMs"`
	Result                string             `json:"result"`
	Role                  string             `json:"role,omitempty"`
	Attacker              *BattleCombatant   `json:"attacker,omitempty"`
	Defender              *BattleCombatant   `json:"defender,omitempty"`
	Metrics               BattleMetrics      `json:"metrics"`
	ToolsUsed             int64              `json:"toolsUsed,omitempty"`
	GallantryPoints       int64              `json:"gallantryPoints,omitempty"`
	Loot                  map[string]int64   `json:"loot,omitempty"`
	TopUnits              []BattleItemDetail `json:"topUnits,omitempty"`
	SupportTools          []BattleItemDetail `json:"supportTools,omitempty"`
	CommanderEffects      []BattleEffect     `json:"commanderEffects,omitempty"`
	CastellanEffects      []BattleEffect     `json:"castellanEffects,omitempty"`
	Waves                 []BattleWave       `json:"waves,omitempty"`
}

type BattleAnalyticsReport struct {
	ID                    string           `json:"id"`
	MID                   int64            `json:"mid"`
	LID                   int64            `json:"lid"`
	MovementID            int64            `json:"movementId,omitempty"`
	AutomationFeature     string           `json:"automationFeature,omitempty"`
	EventID               int64            `json:"eventId,omitempty"`
	EventActivity         string           `json:"eventActivity,omitempty"`
	EventOccurrenceEndsAt string           `json:"eventOccurrenceEndsAt,omitempty"`
	OccurredAt            string           `json:"occurredAt"`
	DateMs                int64            `json:"dateMs"`
	Result                string           `json:"result"`
	Role                  string           `json:"role,omitempty"`
	TroopsSent            int64            `json:"troopsSent,omitempty"`
	OwnTroopLosses        int64            `json:"ownTroopLosses,omitempty"`
	ToolsUsed             int64            `json:"toolsUsed,omitempty"`
	GallantryPoints       int64            `json:"gallantryPoints,omitempty"`
	LootTotal             int64            `json:"lootTotal,omitempty"`
	Loot                  map[string]int64 `json:"loot,omitempty"`
	TargetPlayerID        int64            `json:"targetPlayerID,omitempty"`
	TargetName            string           `json:"targetName,omitempty"`
	TargetTypeID          int              `json:"targetTypeID,omitempty"`
	TargetType            string           `json:"targetType,omitempty"`
	KingdomID             int              `json:"kingdomID,omitempty"`
	TargetX               int              `json:"targetX,omitempty"`
	TargetY               int              `json:"targetY,omitempty"`
}
