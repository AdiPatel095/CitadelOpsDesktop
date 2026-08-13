package WorldIntel

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/url"
	"sort"
	"strings"
	"time"
)

const (
	SchemaVersion        = 1
	MaximumPlayers       = 500
	MaximumAlliances     = 100
	MaximumHoldings      = 2_000
	MaximumEventRankings = 1_000
	MaximumEventScores   = 1_000
	MaximumPublicMetrics = 64
	captureBucket        = 15 * time.Minute
	eventEndBucket       = 5 * time.Minute
)

// PlayerPublicProfile contains additional fields that GGE includes in public
// leaderboard player cards. IDs are retained instead of trying to resolve
// localized title text on the collector.
type PlayerPublicProfile struct {
	AchievementPoints float64 `json:"achievementPoints,omitempty"`
	HighestGlory      float64 `json:"highestGlory,omitempty"`
	AllianceRank      int     `json:"allianceRank,omitempty"`
	TitlePrefixID     int64   `json:"titlePrefixId,omitempty"`
	TitleSuffixID     int64   `json:"titleSuffixId,omitempty"`
	TitleID           int64   `json:"titleId,omitempty"`
	BestRank          int64   `json:"bestRank,omitempty"`
	Ruined            *bool   `json:"ruined,omitempty"`
}

// PublicMetricObservation represents a public, player-scoped highscore. The
// map key on PlayerObservation is stable across observations (for example,
// "storm-points" or "decoration-gacha-spins"), while the wire list and league
// IDs preserve enough provenance to audit the source.
type PublicMetricObservation struct {
	Label      string    `json:"label"`
	Value      float64   `json:"value"`
	Rank       int64     `json:"rank,omitempty"`
	Unit       string    `json:"unit,omitempty"`
	Source     string    `json:"source,omitempty"`
	ListType   int64     `json:"listType,omitempty"`
	LeagueID   int64     `json:"leagueId,omitempty"`
	EventID    int64     `json:"eventId,omitempty"`
	MetricID   int64     `json:"metricId,omitempty"`
	ObservedAt time.Time `json:"observedAt"`
	ValidUntil time.Time `json:"validUntil,omitempty"`
}

type PlayerObservation struct {
	WorldID       string                             `json:"worldId"`
	PlayerID      int64                              `json:"playerId"`
	Name          string                             `json:"name"`
	AllianceID    int64                              `json:"allianceId,omitempty"`
	AllianceName  string                             `json:"allianceName,omitempty"`
	Level         int                                `json:"level,omitempty"`
	LegendLevel   int                                `json:"legendLevel,omitempty"`
	Might         float64                            `json:"might,omitempty"`
	Glory         float64                            `json:"glory,omitempty"`
	WeeklyLoot    float64                            `json:"weeklyLoot,omitempty"`
	Honor         float64                            `json:"honor,omitempty"`
	PublicProfile *PlayerPublicProfile               `json:"publicProfile,omitempty"`
	PublicMetrics map[string]PublicMetricObservation `json:"publicMetrics,omitempty"`
	Source        string                             `json:"source"`
	ObservedAt    time.Time                          `json:"observedAt"`
}

type AllianceObservation struct {
	WorldID       string                             `json:"worldId"`
	AllianceID    int64                              `json:"allianceId"`
	Name          string                             `json:"name"`
	MemberCount   int                                `json:"memberCount,omitempty"`
	TotalMight    float64                            `json:"totalMight,omitempty"`
	PublicMetrics map[string]PublicMetricObservation `json:"publicMetrics,omitempty"`
	Source        string                             `json:"source"`
	ObservedAt    time.Time                          `json:"observedAt"`
}

type HoldingObservation struct {
	WorldID    string    `json:"worldId"`
	AllianceID int64     `json:"allianceId"`
	PlayerID   int64     `json:"playerId"`
	CastleID   int64     `json:"castleId"`
	KingdomID  int64     `json:"kingdomId"`
	X          int       `json:"x"`
	Y          int       `json:"y"`
	SlotType   int       `json:"slotType"`
	ObservedAt time.Time `json:"observedAt"`
}

type EventRankingObservation struct {
	WorldID      string    `json:"worldId"`
	EventID      int64     `json:"eventId"`
	AllianceID   int64     `json:"allianceId"`
	AllianceName string    `json:"allianceName"`
	Rank         int64     `json:"rank"`
	Score        int64     `json:"score"`
	MemberCount  int64     `json:"memberCount,omitempty"`
	FamePoints   int64     `json:"famePoints,omitempty"`
	ObservedAt   time.Time `json:"observedAt"`
}

type EventScoreObservation struct {
	WorldID      string    `json:"worldId"`
	OccurrenceID string    `json:"occurrenceId"`
	EventID      int64     `json:"eventId"`
	EventKey     string    `json:"eventKey"`
	EventName    string    `json:"eventName"`
	ListType     int64     `json:"listType"`
	LeagueID     int64     `json:"leagueId"`
	BoardKey     string    `json:"boardKey,omitempty"`
	PlayerID     int64     `json:"playerId"`
	PlayerName   string    `json:"playerName"`
	AllianceID   int64     `json:"allianceId,omitempty"`
	AllianceName string    `json:"allianceName,omitempty"`
	Rank         int64     `json:"rank"`
	Score        int64     `json:"score,omitempty"`
	ScoreKnown   bool      `json:"scoreKnown"`
	ScoreUnit    string    `json:"scoreUnit"`
	RunStartedOn string    `json:"runStartedOn"`
	EventEndsAt  time.Time `json:"eventEndsAt"`
	Source       string    `json:"source"`
	ObservedAt   time.Time `json:"observedAt"`
}

type ObservationBatch struct {
	SchemaVersion int                       `json:"schemaVersion"`
	BatchID       string                    `json:"batchId"`
	WorldID       string                    `json:"worldId"`
	CapturedAt    time.Time                 `json:"capturedAt"`
	Players       []PlayerObservation       `json:"players"`
	Alliances     []AllianceObservation     `json:"alliances"`
	Holdings      []HoldingObservation      `json:"holdings"`
	EventRankings []EventRankingObservation `json:"eventRankings"`
	EventScores   []EventScoreObservation   `json:"eventScores,omitempty"`
}

type InstallationRegistration struct {
	InstallationID string `json:"installationId"`
	Secret         string `json:"secret"`
	ClientVersion  string `json:"clientVersion,omitempty"`
}

type IngestResponse struct {
	Accepted bool   `json:"accepted"`
	BatchID  string `json:"batchId"`
}

type DesktopStatus struct {
	Enabled            bool   `json:"enabled"`
	CollectionMode     string `json:"collectionMode"`
	WorldID            string `json:"worldId,omitempty"`
	Endpoint           string `json:"endpoint"`
	PublicFieldsOnly   bool   `json:"publicFieldsOnly"`
	OfficialSourceOnly bool   `json:"officialSourceOnly"`
}

type SearchResult struct {
	Type           string                             `json:"type"`
	WorldID        string                             `json:"worldId"`
	ID             int64                              `json:"id"`
	Name           string                             `json:"name"`
	AllianceID     int64                              `json:"allianceId,omitempty"`
	AllianceName   string                             `json:"allianceName,omitempty"`
	Level          int                                `json:"level,omitempty"`
	LegendLevel    int                                `json:"legendLevel,omitempty"`
	Might          float64                            `json:"might,omitempty"`
	Glory          float64                            `json:"glory,omitempty"`
	WeeklyLoot     float64                            `json:"weeklyLoot,omitempty"`
	Honor          float64                            `json:"honor,omitempty"`
	PublicProfile  *PlayerPublicProfile               `json:"publicProfile,omitempty"`
	PublicMetrics  map[string]PublicMetricObservation `json:"publicMetrics,omitempty"`
	MemberCount    int                                `json:"memberCount,omitempty"`
	LastObservedAt time.Time                          `json:"lastObservedAt"`
}

type SearchResponse struct {
	WorldID string         `json:"worldId"`
	Query   string         `json:"query"`
	Results []SearchResult `json:"results"`
}

type PlayerProfile struct {
	Current PlayerObservation   `json:"current"`
	History []PlayerObservation `json:"history"`
}

type AllianceProfile struct {
	Current  AllianceObservation   `json:"current"`
	History  []AllianceObservation `json:"history"`
	Members  []PlayerObservation   `json:"members"`
	Holdings []HoldingObservation  `json:"holdings"`
}

type RankingEntry struct {
	Rank           int       `json:"rank"`
	Type           string    `json:"type"`
	WorldID        string    `json:"worldId"`
	ID             int64     `json:"id"`
	Name           string    `json:"name"`
	AllianceID     int64     `json:"allianceId,omitempty"`
	AllianceName   string    `json:"allianceName,omitempty"`
	Level          int       `json:"level,omitempty"`
	LegendLevel    int       `json:"legendLevel,omitempty"`
	Might          float64   `json:"might,omitempty"`
	Glory          float64   `json:"glory,omitempty"`
	WeeklyLoot     float64   `json:"weeklyLoot,omitempty"`
	Honor          float64   `json:"honor,omitempty"`
	MemberCount    int       `json:"memberCount,omitempty"`
	Value          float64   `json:"value"`
	LastObservedAt time.Time `json:"lastObservedAt"`
}

type RankingResponse struct {
	WorldID string         `json:"worldId"`
	Type    string         `json:"type"`
	Metric  string         `json:"metric"`
	Entries []RankingEntry `json:"entries"`
}

type RankingMetricDefinition struct {
	Metric           string     `json:"metric"`
	Label            string     `json:"label"`
	Unit             string     `json:"unit,omitempty"`
	Source           string     `json:"source"`
	PopulatedRows    int64      `json:"populatedRows"`
	LatestObservedAt *time.Time `json:"latestObservedAt,omitempty"`
}

type RankingMetricCatalogResponse struct {
	WorldID string                    `json:"worldId"`
	Type    string                    `json:"type"`
	Metrics []RankingMetricDefinition `json:"metrics"`
}

type EventRun struct {
	OccurrenceID    string    `json:"occurrenceId"`
	WorldID         string    `json:"worldId"`
	EventID         int64     `json:"eventId"`
	EventKey        string    `json:"eventKey"`
	EventName       string    `json:"eventName"`
	ScoreUnit       string    `json:"scoreUnit"`
	RunStartedOn    string    `json:"runStartedOn"`
	EventEndsAt     time.Time `json:"eventEndsAt"`
	FirstObservedAt time.Time `json:"firstObservedAt"`
	LastObservedAt  time.Time `json:"lastObservedAt"`
	Participants    int64     `json:"participants"`
	ScoreRows       int64     `json:"scoreRows"`
}

type EventRunListResponse struct {
	WorldID  string     `json:"worldId"`
	EventKey string     `json:"eventKey,omitempty"`
	Runs     []EventRun `json:"runs"`
}

type EventRunRankingResponse struct {
	Run     EventRun                `json:"run"`
	Entries []EventScoreObservation `json:"entries"`
}

type PlayerEventScoreResponse struct {
	WorldID      string                  `json:"worldId"`
	PlayerID     int64                   `json:"playerId"`
	EventKey     string                  `json:"eventKey,omitempty"`
	OccurrenceID string                  `json:"occurrenceId,omitempty"`
	History      []EventScoreObservation `json:"history"`
}

type WorldCoverage struct {
	WorldID          string     `json:"worldId"`
	Players          int64      `json:"players"`
	Alliances        int64      `json:"alliances"`
	Holdings         int64      `json:"holdings"`
	EventRuns        int64      `json:"eventRuns"`
	EventScores      int64      `json:"eventScores"`
	ObservationCount int64      `json:"observationCount"`
	FirstObservedAt  *time.Time `json:"firstObservedAt,omitempty"`
	LastObservedAt   *time.Time `json:"lastObservedAt,omitempty"`
}

type CoverageResponse struct {
	Worlds []WorldCoverage `json:"worlds"`
}

func NormalizeWorldID(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	candidate := value
	hasScheme := strings.Contains(value, "://")
	if !hasScheme {
		candidate = "//" + value
	}
	if parsed, err := url.Parse(candidate); err == nil && parsed.Hostname() != "" {
		host := strings.ToLower(parsed.Hostname())
		if port := parsed.Port(); port != "" {
			scheme := strings.ToLower(parsed.Scheme)
			standardPort := !hasScheme && (port == "80" || port == "443") ||
				(scheme == "https" || scheme == "wss") && port == "443" ||
				(scheme == "http" || scheme == "ws") && port == "80"
			if !standardPort {
				host += ":" + port
			}
		}
		return host
	}
	return strings.ToLower(strings.TrimSuffix(value, "/"))
}

func FinalizeBatch(batch ObservationBatch) (ObservationBatch, error) {
	batch.SchemaVersion = SchemaVersion
	batch.WorldID = NormalizeWorldID(batch.WorldID)
	batch.CapturedAt = batch.CapturedAt.UTC().Truncate(time.Second)
	batch.BatchID = ""
	normalizeBatch(&batch)
	if err := ValidateBatch(batch); err != nil {
		return ObservationBatch{}, err
	}
	payload, err := json.Marshal(batch)
	if err != nil {
		return ObservationBatch{}, fmt.Errorf("encode observation batch identity: %w", err)
	}
	digest := sha256.Sum256(payload)
	batch.BatchID = hex.EncodeToString(digest[:])
	return batch, nil
}

func ValidateFinalizedBatch(batch ObservationBatch) error {
	provided := strings.ToLower(strings.TrimSpace(batch.BatchID))
	if len(provided) != sha256.Size*2 {
		return fmt.Errorf("batchId must be a SHA-256 digest")
	}
	rebuilt, err := FinalizeBatch(batch)
	if err != nil {
		return err
	}
	if rebuilt.BatchID != provided {
		return fmt.Errorf("batchId does not match the observation payload")
	}
	return nil
}

func ValidateBatch(batch ObservationBatch) error {
	if batch.SchemaVersion != SchemaVersion {
		return fmt.Errorf("unsupported observation schema %d", batch.SchemaVersion)
	}
	if batch.WorldID == "" || len(batch.WorldID) > 255 {
		return fmt.Errorf("worldId is required and must be at most 255 characters")
	}
	if batch.CapturedAt.IsZero() {
		return fmt.Errorf("capturedAt is required")
	}
	if len(batch.Players) > MaximumPlayers || len(batch.Alliances) > MaximumAlliances ||
		len(batch.Holdings) > MaximumHoldings || len(batch.EventRankings) > MaximumEventRankings ||
		len(batch.EventScores) > MaximumEventScores {
		return fmt.Errorf("observation batch exceeds the allowed entity limits")
	}
	if len(batch.Players)+len(batch.Alliances)+len(batch.Holdings)+len(batch.EventRankings)+len(batch.EventScores) == 0 {
		return fmt.Errorf("observation batch is empty")
	}
	for index, player := range batch.Players {
		if player.WorldID != batch.WorldID || player.PlayerID <= 0 || player.Name == "" || len(player.Name) > 160 ||
			player.ObservedAt.IsZero() || !validSource(player.Source) || player.Level < 0 || player.LegendLevel < 0 ||
			player.Might < 0 || player.Glory < 0 || player.WeeklyLoot < 0 || player.Honor < 0 ||
			observationTooFarAhead(player.ObservedAt, batch.CapturedAt) {
			return fmt.Errorf("players[%d] is invalid", index)
		}
		if player.PublicProfile != nil && (player.PublicProfile.AchievementPoints < 0 || player.PublicProfile.HighestGlory < 0 ||
			player.PublicProfile.AllianceRank < 0 || player.PublicProfile.TitlePrefixID < 0 || player.PublicProfile.TitleSuffixID < 0 ||
			player.PublicProfile.TitleID < 0 || player.PublicProfile.BestRank < 0) {
			return fmt.Errorf("players[%d].publicProfile is invalid", index)
		}
		if len(player.PublicMetrics) > MaximumPublicMetrics {
			return fmt.Errorf("players[%d].publicMetrics exceeds the allowed metric limit", index)
		}
		for key, metric := range player.PublicMetrics {
			if err := validatePublicMetric(key, metric, batch.CapturedAt); err != nil {
				return fmt.Errorf("players[%d].publicMetrics[%q] is invalid", index, key)
			}
		}
	}
	for index, alliance := range batch.Alliances {
		if alliance.WorldID != batch.WorldID || alliance.AllianceID <= 0 || alliance.Name == "" || len(alliance.Name) > 160 ||
			alliance.ObservedAt.IsZero() || !validSource(alliance.Source) || alliance.MemberCount < 0 || alliance.TotalMight < 0 ||
			observationTooFarAhead(alliance.ObservedAt, batch.CapturedAt) {
			return fmt.Errorf("alliances[%d] is invalid", index)
		}
		if len(alliance.PublicMetrics) > MaximumPublicMetrics {
			return fmt.Errorf("alliances[%d].publicMetrics exceeds the allowed metric limit", index)
		}
		for key, metric := range alliance.PublicMetrics {
			if err := validatePublicMetric(key, metric, batch.CapturedAt); err != nil {
				return fmt.Errorf("alliances[%d].publicMetrics[%q] is invalid", index, key)
			}
		}
	}
	for index, holding := range batch.Holdings {
		if holding.WorldID != batch.WorldID || holding.AllianceID <= 0 || holding.PlayerID <= 0 || holding.CastleID <= 0 ||
			holding.KingdomID < 0 || holding.X < 0 || holding.Y < 0 || holding.SlotType < 0 || holding.ObservedAt.IsZero() ||
			observationTooFarAhead(holding.ObservedAt, batch.CapturedAt) {
			return fmt.Errorf("holdings[%d] is invalid", index)
		}
	}
	for index, ranking := range batch.EventRankings {
		if ranking.WorldID != batch.WorldID || ranking.EventID <= 0 || ranking.AllianceID <= 0 || ranking.AllianceName == "" ||
			len(ranking.AllianceName) > 160 || ranking.Rank <= 0 || ranking.Score < 0 || ranking.ObservedAt.IsZero() ||
			observationTooFarAhead(ranking.ObservedAt, batch.CapturedAt) {
			return fmt.Errorf("eventRankings[%d] is invalid", index)
		}
	}
	for index, score := range batch.EventScores {
		startedOn, dateErr := time.Parse(time.DateOnly, score.RunStartedOn)
		if score.WorldID != batch.WorldID || score.OccurrenceID != EventOccurrenceID(batch.WorldID, score.EventID, score.EventEndsAt) ||
			score.EventID <= 0 || !validEventKey(score.EventKey) || score.EventName == "" || len(score.EventName) > 120 ||
			score.ListType <= 0 || score.LeagueID < -1 || len(score.BoardKey) > 40 || score.PlayerID <= 0 ||
			score.PlayerName == "" || len(score.PlayerName) > 160 || score.AllianceID < 0 || len(score.AllianceName) > 160 ||
			score.Rank <= 0 || score.Score < 0 || score.ScoreUnit == "" || len(score.ScoreUnit) > 32 ||
			(!score.ScoreKnown && score.Score != 0) ||
			score.Source != "gge-highscore" || score.ObservedAt.IsZero() || score.EventEndsAt.IsZero() ||
			dateErr != nil || startedOn.After(score.ObservedAt.UTC().Truncate(24*time.Hour)) ||
			startedOn.Before(score.ObservedAt.UTC().AddDate(0, -3, 0).Truncate(24*time.Hour)) ||
			score.EventEndsAt.Before(score.ObservedAt.Add(-10*time.Minute)) ||
			score.EventEndsAt.After(score.ObservedAt.Add(90*24*time.Hour+10*time.Minute)) ||
			observationTooFarAhead(score.ObservedAt, batch.CapturedAt) {
			return fmt.Errorf("eventScores[%d] is invalid", index)
		}
	}
	return nil
}

func NormalizeEventEndsAt(value time.Time) time.Time {
	if value.IsZero() {
		return time.Time{}
	}
	return value.UTC().Add(eventEndBucket / 2).Truncate(eventEndBucket)
}

func EventOccurrenceID(worldID string, eventID int64, eventEndsAt time.Time) string {
	identity := fmt.Sprintf("%s\x00%d\x00%s", NormalizeWorldID(worldID), eventID, NormalizeEventEndsAt(eventEndsAt).Format(time.RFC3339))
	digest := sha256.Sum256([]byte(identity))
	return hex.EncodeToString(digest[:])
}

func validEventKey(value string) bool {
	if value == "" || len(value) > 80 || value[0] == '-' || value[len(value)-1] == '-' {
		return false
	}
	for _, character := range value {
		if (character < 'a' || character > 'z') && (character < '0' || character > '9') && character != '-' {
			return false
		}
	}
	return true
}

func observationTooFarAhead(observedAt time.Time, capturedAt time.Time) bool {
	// CapturedAt is bucketed for deterministic desktop batches, so a valid row
	// can be almost one capture bucket newer than the batch timestamp.
	return observedAt.After(capturedAt.Add(captureBucket + 10*time.Minute))
}

func validatePublicMetric(key string, metric PublicMetricObservation, capturedAt time.Time) error {
	if key == "" || len(key) > 80 || metric.Label == "" || len(metric.Label) > 120 || metric.Value < 0 ||
		metric.Rank < 0 || len(metric.Unit) > 32 || !validPublicMetricSource(metric.Source) ||
		metric.ListType < 0 || metric.LeagueID < -1 || metric.EventID < 0 || metric.MetricID < 0 ||
		metric.ObservedAt.IsZero() || observationTooFarAhead(metric.ObservedAt, capturedAt) ||
		(!metric.ValidUntil.IsZero() && (metric.ValidUntil.Before(metric.ObservedAt) || metric.ValidUntil.After(capturedAt.AddDate(0, 2, 0)))) {
		return fmt.Errorf("public metric is outside the supported bounds")
	}
	return nil
}

func validPublicMetricSource(value string) bool {
	// Schema v1 originally accepted an optional, bounded source string. Keep
	// omitted and legacy collector values valid so upgrading does not reject an
	// otherwise valid persisted batch. Current collectors still emit the two
	// canonical GGE source names.
	return len(value) <= 32
}

func normalizePublicMetrics(metrics map[string]PublicMetricObservation) map[string]PublicMetricObservation {
	if len(metrics) == 0 {
		return nil
	}
	normalized := make(map[string]PublicMetricObservation, len(metrics))
	for key, metric := range metrics {
		key = strings.ToLower(cleanText(key, 80))
		metric.Label = cleanText(metric.Label, 120)
		metric.Unit = strings.ToLower(cleanText(metric.Unit, 32))
		metric.Source = strings.ToLower(cleanText(metric.Source, 32))
		metric.ObservedAt = metric.ObservedAt.UTC().Truncate(time.Second)
		if !metric.ValidUntil.IsZero() {
			metric.ValidUntil = metric.ValidUntil.UTC().Truncate(time.Second)
		}
		if key != "" {
			normalized[key] = metric
		}
	}
	return normalized
}

func normalizeBatch(batch *ObservationBatch) {
	for index := range batch.Players {
		row := &batch.Players[index]
		row.WorldID = batch.WorldID
		row.Name = cleanText(row.Name, 160)
		row.AllianceName = cleanText(row.AllianceName, 160)
		row.Source = strings.TrimSpace(row.Source)
		row.ObservedAt = row.ObservedAt.UTC().Truncate(time.Second)
		row.PublicMetrics = normalizePublicMetrics(row.PublicMetrics)
	}
	for index := range batch.Alliances {
		row := &batch.Alliances[index]
		row.WorldID = batch.WorldID
		row.Name = cleanText(row.Name, 160)
		row.Source = strings.TrimSpace(row.Source)
		row.ObservedAt = row.ObservedAt.UTC().Truncate(time.Second)
		row.PublicMetrics = normalizePublicMetrics(row.PublicMetrics)
	}
	for index := range batch.Holdings {
		row := &batch.Holdings[index]
		row.WorldID = batch.WorldID
		row.ObservedAt = row.ObservedAt.UTC().Truncate(time.Second)
	}
	for index := range batch.EventScores {
		row := &batch.EventScores[index]
		row.WorldID = batch.WorldID
		row.EventKey = strings.ToLower(cleanText(row.EventKey, 80))
		row.EventName = cleanText(row.EventName, 120)
		row.BoardKey = strings.ToLower(cleanText(row.BoardKey, 40))
		row.PlayerName = cleanText(row.PlayerName, 160)
		row.AllianceName = cleanText(row.AllianceName, 160)
		row.ScoreUnit = strings.ToLower(cleanText(row.ScoreUnit, 32))
		row.Source = strings.ToLower(cleanText(row.Source, 32))
		row.ObservedAt = row.ObservedAt.UTC().Truncate(time.Second)
		row.EventEndsAt = NormalizeEventEndsAt(row.EventEndsAt)
		if startedOn, err := time.Parse(time.DateOnly, strings.TrimSpace(row.RunStartedOn)); err == nil {
			row.RunStartedOn = startedOn.Format(time.DateOnly)
		} else if !row.ObservedAt.IsZero() {
			row.RunStartedOn = row.ObservedAt.Format(time.DateOnly)
		}
		row.OccurrenceID = EventOccurrenceID(batch.WorldID, row.EventID, row.EventEndsAt)
	}
	for index := range batch.EventRankings {
		row := &batch.EventRankings[index]
		row.WorldID = batch.WorldID
		row.AllianceName = cleanText(row.AllianceName, 160)
		row.ObservedAt = row.ObservedAt.UTC().Truncate(time.Second)
	}
	sort.Slice(batch.Players, func(i, j int) bool { return batch.Players[i].PlayerID < batch.Players[j].PlayerID })
	sort.Slice(batch.Alliances, func(i, j int) bool { return batch.Alliances[i].AllianceID < batch.Alliances[j].AllianceID })
	sort.Slice(batch.Holdings, func(i, j int) bool {
		if batch.Holdings[i].AllianceID != batch.Holdings[j].AllianceID {
			return batch.Holdings[i].AllianceID < batch.Holdings[j].AllianceID
		}
		if batch.Holdings[i].PlayerID != batch.Holdings[j].PlayerID {
			return batch.Holdings[i].PlayerID < batch.Holdings[j].PlayerID
		}
		return batch.Holdings[i].CastleID < batch.Holdings[j].CastleID
	})
	sort.Slice(batch.EventRankings, func(i, j int) bool {
		if batch.EventRankings[i].EventID != batch.EventRankings[j].EventID {
			return batch.EventRankings[i].EventID < batch.EventRankings[j].EventID
		}
		return batch.EventRankings[i].Rank < batch.EventRankings[j].Rank
	})
	sort.Slice(batch.EventScores, func(i, j int) bool {
		left, right := batch.EventScores[i], batch.EventScores[j]
		if left.OccurrenceID != right.OccurrenceID {
			return left.OccurrenceID < right.OccurrenceID
		}
		if left.ListType != right.ListType {
			return left.ListType < right.ListType
		}
		if left.LeagueID != right.LeagueID {
			return left.LeagueID < right.LeagueID
		}
		if left.Rank != right.Rank {
			return left.Rank < right.Rank
		}
		return left.PlayerID < right.PlayerID
	})
}

func validSource(value string) bool {
	switch value {
	case "account", "alliance", "event-ranking", "leaderboard":
		return true
	default:
		return false
	}
}

func cleanText(value string, maximum int) string {
	value = strings.TrimSpace(strings.Join(strings.Fields(value), " "))
	if len(value) > maximum {
		value = value[:maximum]
	}
	return value
}
