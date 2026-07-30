package AllianceTargets

const TargetPageSize = 20

type Query struct {
	Search           string
	Status           string
	Sort             string
	Direction        string
	Page             int
	IncludeAlliances bool
}

type AllianceOption struct {
	ExternalID  string `json:"externalId"`
	AllianceID  int64  `json:"allianceId"`
	Name        string `json:"name"`
	Rank        int    `json:"rank"`
	PlayerCount int    `json:"playerCount"`
}

type Castle struct {
	CastleID int64  `json:"castleId,omitempty"`
	Name     string `json:"name"`
	TypeName string `json:"typeName,omitempty"`
	X        int    `json:"x"`
	Y        int    `json:"y"`
	TypeID   int    `json:"type,omitempty"`
}

type Target struct {
	PlayerID         int64             `json:"playerId"`
	Name             string            `json:"name"`
	Level            int               `json:"level,omitempty"`
	LegendLevel      int               `json:"legendLevel,omitempty"`
	Might            int64             `json:"might"`
	UnderBird        bool              `json:"underBird"`
	RPTSeconds       int               `json:"rptSeconds"`
	BirdUntil        string            `json:"birdUntil,omitempty"`
	UpdatedAt        string            `json:"updatedAt,omitempty"`
	TargetCastle     Castle            `json:"targetCastle"`
	ClosestOwnCastle Castle            `json:"closestOwnCastle"`
	Distance         float64           `json:"distance"`
	SpyReport        *SpyReportSummary `json:"spyReport,omitempty"`
}

type SpyReportSummary struct {
	CapturedAtUnixMillis int64  `json:"capturedAtUnixMillis"`
	Status               string `json:"status"`
	TotalTroops          int64  `json:"totalTroops"`
}

type Tavern struct {
	Level    int `json:"level"`
	Capacity int `json:"capacity"`
}

type SpyAvailability struct {
	Capacity           int      `json:"capacity"`
	Active             int      `json:"active"`
	Available          int      `json:"available"`
	BuildingRowsLoaded bool     `json:"buildingRowsLoaded"`
	SourceCastle       Castle   `json:"sourceCastle"`
	Taverns            []Tavern `json:"taverns"`
}

type AllianceOptionView struct {
	ExternalID  string `json:"externalId"`
	Name        string `json:"name"`
	Rank        int    `json:"rank"`
	PlayerCount int    `json:"playerCount"`
}

type SelectedAllianceView struct {
	ExternalID string `json:"externalId"`
	AllianceID int64  `json:"allianceId"`
	Name       string `json:"name"`
}

type TargetCastleView struct {
	CastleID int64  `json:"castleId,omitempty"`
	Name     string `json:"name"`
	TypeName string `json:"typeName,omitempty"`
	TypeID   int    `json:"typeId,omitempty"`
	X        int    `json:"x"`
	Y        int    `json:"y"`
}

type OwnCastleView struct {
	Name string `json:"name"`
	X    int    `json:"x"`
	Y    int    `json:"y"`
}

type TargetView struct {
	PlayerID         int64             `json:"playerId"`
	Name             string            `json:"name"`
	Level            int               `json:"level,omitempty"`
	LegendLevel      int               `json:"legendLevel,omitempty"`
	Might            int64             `json:"might"`
	UnderBird        bool              `json:"underBird"`
	RPTSeconds       int               `json:"rptSeconds"`
	TargetCastle     TargetCastleView  `json:"targetCastle"`
	ClosestOwnCastle OwnCastleView     `json:"closestOwnCastle"`
	Distance         float64           `json:"distance"`
	SpyReport        *SpyReportSummary `json:"spyReport,omitempty"`
}

type SpyAction struct {
	CanLaunch      bool  `json:"canLaunch"`
	Available      int   `json:"available"`
	SourceCastleID int64 `json:"sourceCastleId,omitempty"`
}

type View struct {
	Server           string                `json:"server"`
	Alliances        []AllianceOptionView  `json:"alliances"`
	SelectedAlliance *SelectedAllianceView `json:"selectedAlliance,omitempty"`
	Targets          []TargetView          `json:"targets"`
	TotalTargets     int                   `json:"totalTargets"`
	Page             int                   `json:"page"`
	PageSize         int                   `json:"pageSize"`
	PageCount        int                   `json:"pageCount"`
	CanInspect       bool                  `json:"canInspect"`
	Spies            SpyAction             `json:"spies"`
}
