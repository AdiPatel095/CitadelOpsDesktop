package AllianceTargets

import "time"

type AllianceOption struct {
	ExternalID  string `json:"externalId"`
	AllianceID  int64  `json:"allianceId"`
	Name        string `json:"name"`
	Rank        int    `json:"rank"`
	Might       int64  `json:"might"`
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
	PlayerID         int64   `json:"playerId"`
	Name             string  `json:"name"`
	Might            int64   `json:"might"`
	UnderBird        bool    `json:"underBird"`
	RPTSeconds       int     `json:"rptSeconds"`
	BirdUntil        string  `json:"birdUntil,omitempty"`
	UpdatedAt        string  `json:"updatedAt,omitempty"`
	TargetCastle     Castle  `json:"targetCastle"`
	ClosestOwnCastle Castle  `json:"closestOwnCastle"`
	Distance         float64 `json:"distance"`
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

type View struct {
	Server           string           `json:"server"`
	Alliances        []AllianceOption `json:"alliances"`
	SelectedAlliance *AllianceOption  `json:"selectedAlliance,omitempty"`
	Targets          []Target         `json:"targets"`
	Spies            SpyAvailability  `json:"spies"`
	FetchedAt        time.Time        `json:"fetchedAt"`
}
