// Package PrivateMetrics publishes account-private player and feature metrics
// from one bound runtime to CitadelOpsBackend. It deliberately has no read or
// query surface: dashboards read persisted data through the backend, never by
// reaching into another account runtime.
package PrivateMetrics

import "time"

const SchemaVersion = 1

// Grant is an in-memory-only credential installed by the orchestrator for one
// exact runtime placement. It must never be persisted or returned by status
// APIs.
type Grant struct {
	Token     string    `json:"token"`
	ExpiresAt time.Time `json:"expiresAt"`
}

// Placement is the backend-owned identity and fencing context for one runtime.
// Grant is used only as the outbound Authorization header and is omitted from
// every published body.
type Placement struct {
	CellID          string
	TenantID        string
	RuntimeID       string
	PlacementEpoch  uint64
	DesiredRevision uint64
	LeaseExpiresAt  time.Time
	Grant           Grant
}

type AccountBinding struct {
	AccountUID int64     `json:"accountUid"`
	WorldID    string    `json:"worldId"`
	PlayerID   int64     `json:"playerId"`
	BoundAt    time.Time `json:"boundAt"`
}

// PlayerMetrics is the complete private My Stats projection. Zero values are
// authoritative because a ready sample is complete; consumers must not use
// zero as a missing-value sentinel.
type PlayerMetrics struct {
	Name            string             `json:"name,omitempty"`
	AllianceID      int64              `json:"allianceId"`
	Level           int                `json:"level"`
	LegendLevel     int                `json:"legendLevel"`
	Might           float64            `json:"might"`
	Glory           float64            `json:"glory"`
	Gallantry       float64            `json:"gallantry"`
	TroopsTotal     int64              `json:"troopsTotal"`
	TroopsStationed int64              `json:"troopsStationed"`
	TroopsTraveling int64              `json:"troopsTraveling"`
	TroopsHospital  int64              `json:"troopsHospital"`
	TroopsByUnit    map[string]int64   `json:"troopsByUnit"`
	Coins           float64            `json:"coins"`
	Rubies          float64            `json:"rubies"`
	Currencies      map[string]float64 `json:"currencies"`
}

// PublicPlayerCandidate is a deliberately narrow copy of fields that may be
// considered by the backend's provenance resolver. The private player payload
// itself is never forwarded to World Intelligence.
type PublicPlayerCandidate struct {
	PresentFields []string `json:"presentFields"`
	Name          string   `json:"name,omitempty"`
	AllianceID    int64    `json:"allianceId"`
	AllianceName  string   `json:"allianceName,omitempty"`
	Level         int      `json:"level"`
	LegendLevel   int      `json:"legendLevel"`
	Might         float64  `json:"might"`
	Glory         float64  `json:"glory"`
}

type FeatureTotals struct {
	FeatureID       string           `json:"featureId"`
	Battles         int64            `json:"battles"`
	Victories       int64            `json:"victories"`
	Defeats         int64            `json:"defeats"`
	TroopsSent      int64            `json:"troopsSent"`
	TroopLosses     int64            `json:"troopLosses"`
	ToolsUsed       int64            `json:"toolsUsed"`
	GallantryPoints int64            `json:"gallantryPoints"`
	LootTotal       int64            `json:"lootTotal"`
	Loot            map[string]int64 `json:"loot"`
	FirstOccurredAt time.Time        `json:"firstOccurredAt,omitempty"`
	LastOccurredAt  time.Time        `json:"lastOccurredAt,omitempty"`
}

type FeatureReportWindows struct {
	Last24Hours []FeatureTotals `json:"last24Hours"`
	Last7Days   []FeatureTotals `json:"last7Days"`
	Last30Days  []FeatureTotals `json:"last30Days"`
	All         []FeatureTotals `json:"all"`
}

type EventScoreMetrics struct {
	EventID            int64     `json:"eventId"`
	EventType          string    `json:"eventType,omitempty"`
	Name               string    `json:"name,omitempty"`
	DifficultyID       int64     `json:"difficultyId,omitempty"`
	PlayerScore        int64     `json:"playerScore"`
	AllianceScore      int64     `json:"allianceScore"`
	PlayerRank         int64     `json:"playerRank"`
	AllianceRank       int64     `json:"allianceRank"`
	LeagueID           int64     `json:"leagueId"`
	AllianceLeagueID   int64     `json:"allianceLeagueId"`
	RewardPagesReached int       `json:"rewardPagesReached"`
	RewardPagesTotal   int       `json:"rewardPagesTotal"`
	NextRewardScore    int64     `json:"nextRewardScore"`
	ObservedAt         time.Time `json:"observedAt"`
	OccurrenceEndsAt   time.Time `json:"occurrenceEndsAt,omitempty"`
}

type CombatTotals struct {
	Launches    int64 `json:"launches"`
	Battles     int64 `json:"battles"`
	Victories   int64 `json:"victories"`
	Defeats     int64 `json:"defeats"`
	TroopLosses int64 `json:"troopLosses"`
	ToolsUsed   int64 `json:"toolsUsed"`
	Loot        int64 `json:"loot"`
}

type EventActivityMetrics struct {
	EventID          int64        `json:"eventId"`
	OccurrenceEndsAt time.Time    `json:"occurrenceEndsAt,omitempty"`
	ObservedFrom     time.Time    `json:"observedFrom,omitempty"`
	Invasion         CombatTotals `json:"invasion"`
	Camp             CombatTotals `json:"camp"`
	Advisor          CombatTotals `json:"advisor"`
	Khan             CombatTotals `json:"khan"`
	KhanDefense      CombatTotals `json:"khanDefense"`
}

type FeatureMetrics struct {
	ReportAnalyticsPresent bool                   `json:"reportAnalyticsPresent"`
	ReportHistoryTruncated bool                   `json:"reportHistoryTruncated"`
	ReportCount            int64                  `json:"reportCount"`
	ReportsObservedAt      time.Time              `json:"reportsObservedAt,omitempty"`
	ReportWindows          FeatureReportWindows   `json:"reportWindows"`
	EventScores            []EventScoreMetrics    `json:"eventScores"`
	EventActivities        []EventActivityMetrics `json:"eventActivities"`
}

// Sample is one complete private current-state projection. CitadelOpsBackend
// can both upsert its current row and append the same body to sample history.
type Sample struct {
	SampleID             string                `json:"sampleId"`
	ObservedAt           time.Time             `json:"observedAt"`
	StateRevision        uint64                `json:"stateRevision"`
	SessionGeneration    uint64                `json:"sessionGeneration"`
	BaselineGeneration   uint64                `json:"baselineGeneration"`
	ConnectionGeneration uint64                `json:"connectionGeneration"`
	Account              AccountBinding        `json:"account"`
	Player               PlayerMetrics         `json:"player"`
	Features             FeatureMetrics        `json:"features"`
	PublicCandidate      PublicPlayerCandidate `json:"publicCandidate"`
}

// PublishRequest contains no credential material. The bearer grant is carried
// only in the HTTP Authorization header.
type PublishRequest struct {
	SchemaVersion   int       `json:"schemaVersion"`
	CellID          string    `json:"cellId"`
	TenantID        string    `json:"tenantId"`
	RuntimeID       string    `json:"runtimeId"`
	PlacementEpoch  uint64    `json:"placementEpoch"`
	DesiredRevision uint64    `json:"desiredRevision"`
	LeaseExpiresAt  time.Time `json:"leaseExpiresAt"`
	Sample          Sample    `json:"sample"`
}
