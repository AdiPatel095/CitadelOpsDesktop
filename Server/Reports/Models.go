package Reports

type Player struct {
	ID       int64  `json:"id,omitempty"`
	Name     string `json:"name,omitempty"`
	Alliance string `json:"alliance,omitempty"`
	Dummy    *bool  `json:"dummy,omitempty"`
}

type Castle struct {
	ID        int64  `json:"id,omitempty"`
	Name      string `json:"name,omitempty"`
	KingdomID int    `json:"kingdomID,omitempty"`
	TypeID    int    `json:"typeID,omitempty"`
	X         int    `json:"x,omitempty"`
	Y         int    `json:"y,omitempty"`
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
	Level     int `json:"level,omitempty"`
	Wall      int `json:"wall,omitempty"`
	Gate      int `json:"gate,omitempty"`
	Moat      int `json:"moat,omitempty"`
	Courtyard int `json:"courtyard,omitempty"`
	WallSlots int `json:"wallSlots,omitempty"`
	Effects   any `json:"effects,omitempty"`
	Equipment any `json:"equipment,omitempty"`
	SkillIDs  any `json:"skillIDs,omitempty"`
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
	WodID  int64  `json:"wodID,omitempty"`
	Amount int64  `json:"amount,omitempty"`
	Lost   int64  `json:"lost,omitempty"`
}

type BattleReport struct {
	ID         string             `json:"id"`
	ReportID   string             `json:"reportID"`
	BattleKey  string             `json:"battleKey,omitempty"`
	MID        int64              `json:"mid"`
	LID        int64              `json:"lid"`
	KingdomID  int                `json:"kingdomID,omitempty"`
	TargetX    int                `json:"targetX,omitempty"`
	TargetY    int                `json:"targetY,omitempty"`
	TargetName string             `json:"targetName,omitempty"`
	CastleName string             `json:"castleName,omitempty"`
	BattleType string             `json:"battleType,omitempty"`
	OccurredAt string             `json:"occurredAt"`
	DateMs     int64              `json:"dateMs"`
	Result     string             `json:"result"`
	Role       string             `json:"role,omitempty"`
	Attacker   *BattleCombatant   `json:"attacker,omitempty"`
	Defender   *BattleCombatant   `json:"defender,omitempty"`
	Metrics    BattleMetrics      `json:"metrics"`
	TopUnits   []BattleItemDetail `json:"topUnits,omitempty"`
}
