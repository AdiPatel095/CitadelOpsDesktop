package PrivateMetrics

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"CitadelDesktop/Server/GameData"
	"CitadelDesktop/Server/History"
	"CitadelDesktop/Server/Reports"
	"CitadelDesktop/Server/State"
)

const reportHistoryLimit = 10_000

var ErrRuntimeNotReady = errors.New("private metrics runtime is not ready")

type ReportReader interface {
	Recent(context.Context, Reports.BattleReportQuery) ([]Reports.BattleAnalyticsReport, error)
}

// ReportChangeObserver is implemented by report readers that can report a
// process-local write generation. When available, the sample builder re-reads
// battle report analytics only after the store actually changed instead of
// decoding the complete report history for every published sample.
type ReportChangeObserver interface {
	AnalyticsGeneration() uint64
}

// FeatureAnalyticsReader is implemented by report readers that can serve the
// narrow attacker-report projection directly. It halves the cost of a history
// refresh compared with decoding complete analytics rows through Recent.
type FeatureAnalyticsReader interface {
	RecentFeatureAnalytics(context.Context, Reports.BattleReportQuery) ([]Reports.FeatureAnalyticsRow, error)
}

// SampleBuilder produces complete private samples for one account runtime. It
// keeps a compact, account-bound projection of the feature report history so
// steady-state samples cost a rolling-window pass over in-memory records
// rather than a full analytics query and decode.
type SampleBuilder struct {
	state    *State.Store
	gameData *GameData.Manager
	reports  ReportReader
	features featureReportCache
}

func NewSampleBuilder(store *State.Store, gameData *GameData.Manager, reports ReportReader) *SampleBuilder {
	return &SampleBuilder{state: store, gameData: gameData, reports: reports}
}

// BuildSample builds one sample without retaining any cache. Long-lived
// publishers should use a SampleBuilder instead.
func BuildSample(
	ctx context.Context,
	store *State.Store,
	gameData *GameData.Manager,
	reports ReportReader,
	observedAt time.Time,
) (Sample, error) {
	return NewSampleBuilder(store, gameData, reports).Build(ctx, observedAt)
}

func (builder *SampleBuilder) Build(ctx context.Context, observedAt time.Time) (Sample, error) {
	if builder == nil || builder.state == nil {
		return Sample{}, fmt.Errorf("%w: state store is unavailable", ErrRuntimeNotReady)
	}
	if observedAt.IsZero() {
		observedAt = time.Now().UTC()
	} else {
		observedAt = observedAt.UTC()
	}
	snapshot := builder.state.ReadOnlyView()
	worldID := State.CanonicalWorldID(snapshot.Account.WorldID)
	sessionWorldID := State.CanonicalWorldID(snapshot.Session.ServerURL)
	if snapshot.Account.UID <= 0 || snapshot.Account.PlayerID <= 0 || snapshot.Player.ID <= 0 ||
		snapshot.Account.PlayerID != snapshot.Player.ID || worldID == "" || sessionWorldID == "" ||
		worldID != sessionWorldID || snapshot.Account.BoundAt.IsZero() {
		return Sample{}, fmt.Errorf("%w: account identity is not authoritatively bound", ErrRuntimeNotReady)
	}
	if !snapshot.Session.LoggedIn || !snapshot.Session.SocketReady || snapshot.Session.Generation == 0 ||
		snapshot.Session.Generation != snapshot.Session.BaselineGeneration || snapshot.Session.ConnectionGeneration == 0 {
		return Sample{}, fmt.Errorf("%w: session generation is not ready", ErrRuntimeNotReady)
	}

	playerSample := History.NewPlayerSampleAt(snapshot, builder.gameData, observedAt)
	features, err := builder.buildFeatureMetrics(ctx, snapshot, observedAt)
	if err != nil {
		return Sample{}, err
	}
	publicCandidate := publicPlayerCandidate(snapshot)
	return Sample{
		ObservedAt: observedAt, StateRevision: snapshot.Revision,
		SessionGeneration: snapshot.Session.Generation, BaselineGeneration: snapshot.Session.BaselineGeneration,
		ConnectionGeneration: snapshot.Session.ConnectionGeneration,
		Account: AccountBinding{
			AccountUID: snapshot.Account.UID, WorldID: worldID,
			PlayerID: int64(snapshot.Player.ID), BoundAt: snapshot.Account.BoundAt.UTC(),
		},
		Player: PlayerMetrics{
			Name: snapshot.Player.Name, AllianceID: int64(snapshot.Player.AllianceID),
			Level: snapshot.Player.Level, LegendLevel: snapshot.Player.LegendLevel,
			Might: playerSample.Might, Glory: playerSample.Glory, Gallantry: playerSample.Gallantry,
			TroopsTotal: playerSample.TroopsTotal, TroopsStationed: playerSample.TroopsStationed,
			TroopsTraveling: playerSample.TroopsTraveling, TroopsHospital: playerSample.TroopsHospital,
			TroopsByUnit: cloneInt64Map(playerSample.TroopsByUnit),
			Coins:        playerSample.Coins, Rubies: playerSample.Rubies,
			Currencies: cloneFloat64Map(playerSample.Currencies),
		},
		Features: features, PublicCandidate: publicCandidate,
	}, nil
}

func (builder *SampleBuilder) buildFeatureMetrics(
	ctx context.Context,
	snapshot State.GameState,
	now time.Time,
) (FeatureMetrics, error) {
	metrics := FeatureMetrics{
		ReportWindows: FeatureReportWindows{
			Last24Hours: []FeatureTotals{}, Last7Days: []FeatureTotals{},
			Last30Days: []FeatureTotals{}, All: []FeatureTotals{},
		},
		EventScores: []EventScoreMetrics{}, EventActivities: []EventActivityMetrics{},
	}
	if builder.reports != nil {
		history, err := builder.features.load(ctx, builder.reports, Reports.BattleReportQuery{
			AccountUID: snapshot.Account.UID, WorldID: snapshot.Account.WorldID,
			PlayerID: int64(snapshot.Player.ID), Limit: reportHistoryLimit,
		})
		if err != nil {
			return FeatureMetrics{}, fmt.Errorf("read private feature analytics: %w", err)
		}
		metrics.ReportAnalyticsPresent = true
		metrics.ReportHistoryTruncated = history.truncated
		metrics.ReportCount = history.count
		metrics.ReportsObservedAt = history.latest
		metrics.ReportWindows = history.windows(now)
	}

	snapshot.RangeScalableEventScores(func(_ int64, score State.ScalableEventScore) bool {
		metrics.EventScores = append(metrics.EventScores, EventScoreMetrics{
			EventID: score.EventID, EventType: score.EventType, Name: score.Name,
			DifficultyID: score.DifficultyID, PlayerScore: score.PlayerScore, AllianceScore: score.AllianceScore,
			PlayerRank: score.PlayerRank, AllianceRank: score.AllianceRank,
			LeagueID: score.LeagueID, AllianceLeagueID: score.AllianceLeagueID,
			RewardPagesReached: score.RewardPagesReached, RewardPagesTotal: score.RewardPagesTotal,
			NextRewardScore: score.NextRewardScore, ObservedAt: score.ObservedAt.UTC(),
			OccurrenceEndsAt: State.ScalableEventEndsAt(score),
		})
		return true
	})
	snapshot.RangeEventActivities(func(_ int64, activity State.EventActivityState) bool {
		metrics.EventActivities = append(metrics.EventActivities, EventActivityMetrics{
			EventID: activity.EventID, OccurrenceEndsAt: activity.OccurrenceEndsAt.UTC(),
			ObservedFrom: activity.ObservedFrom.UTC(), Invasion: combatTotals(activity.Invasion),
			Camp: combatTotals(activity.Camp), Advisor: combatTotals(activity.Advisor),
			Khan: combatTotals(activity.Khan), KhanDefense: combatTotals(activity.KhanDefense),
		})
		return true
	})
	sort.Slice(metrics.EventScores, func(left, right int) bool {
		return metrics.EventScores[left].EventID < metrics.EventScores[right].EventID
	})
	sort.Slice(metrics.EventActivities, func(left, right int) bool {
		return metrics.EventActivities[left].EventID < metrics.EventActivities[right].EventID
	})
	return metrics, nil
}

// featureReportCache holds the compact attacker report projection for one
// account binding. Records are ordered newest first, exactly as the analytics
// store returns them, so each rolling window is a prefix of the slice.
type featureReportCache struct {
	mu      sync.Mutex
	current *featureReportHistory
}

type featureReportHistory struct {
	accountKey string
	generation uint64
	tracked    bool
	records    []compactFeatureReport
	truncated  bool
	count      int64
	latest     time.Time
}

type compactFeatureReport struct {
	featureID   string
	occurredAt  int64
	outcome     reportOutcome
	troopsSent  int64
	troopLosses int64
	toolsUsed   int64
	gallantry   int64
	lootTotal   int64
	loot        []lootEntry
}

type lootEntry struct {
	key    string
	amount int64
}

type reportOutcome uint8

const (
	reportOutcomeOther reportOutcome = iota
	reportOutcomeVictory
	reportOutcomeDefeat
)

func (cache *featureReportCache) load(
	ctx context.Context,
	reader ReportReader,
	query Reports.BattleReportQuery,
) (*featureReportHistory, error) {
	accountKey := fmt.Sprintf("%d\x00%s\x00%d", query.AccountUID, strings.TrimSpace(query.WorldID), query.PlayerID)
	observer, tracked := reader.(ReportChangeObserver)
	var generation uint64
	if tracked {
		// Read the generation before querying so a write that races the query
		// forces one more refresh instead of being missed.
		generation = observer.AnalyticsGeneration()
	}
	cache.mu.Lock()
	defer cache.mu.Unlock()
	if current := cache.current; current != nil && current.tracked && tracked &&
		current.accountKey == accountKey && current.generation == generation {
		return current, nil
	}
	var history *featureReportHistory
	if lean, ok := reader.(FeatureAnalyticsReader); ok {
		rows, err := lean.RecentFeatureAnalytics(ctx, query)
		if err != nil {
			return nil, err
		}
		history = compactFeatureRows(rows)
	} else {
		rows, err := reader.Recent(ctx, query)
		if err != nil {
			return nil, err
		}
		history = compactFeatureReports(rows)
	}
	history.accountKey = accountKey
	history.generation = generation
	history.tracked = tracked
	if tracked {
		cache.current = history
	} else {
		cache.current = nil
	}
	return history, nil
}

// featureHistoryAccumulator builds the compact projection from either the
// complete analytics rows (Recent) or the narrow feature rows. Feature IDs and
// loot keys are interned so ten thousand records share a handful of strings.
type featureHistoryAccumulator struct {
	history  *featureReportHistory
	interned map[string]string
	latest   time.Time
}

func newFeatureHistoryAccumulator(capacity int) *featureHistoryAccumulator {
	return &featureHistoryAccumulator{
		history:  &featureReportHistory{records: make([]compactFeatureReport, 0, capacity)},
		interned: make(map[string]string, 16),
	}
}

func (accumulator *featureHistoryAccumulator) intern(value string) string {
	if canonical, found := accumulator.interned[value]; found {
		return canonical
	}
	accumulator.interned[value] = value
	return value
}

// add appends one attacker report. It returns false when the row is not an
// attacker report for an analytics feature or has no usable timestamp.
func (accumulator *featureHistoryAccumulator) add(
	featureID string,
	occurredAt time.Time,
	result string,
	troopsSent int64,
	troopLosses int64,
	toolsUsed int64,
	gallantry int64,
	lootTotal int64,
	loot []lootEntry,
) {
	accumulator.history.count++
	if occurredAt.After(accumulator.latest) {
		accumulator.latest = occurredAt
	}
	if len(loot) > 1 {
		sort.Slice(loot, func(left, right int) bool { return loot[left].key < loot[right].key })
	}
	accumulator.history.records = append(accumulator.history.records, compactFeatureReport{
		featureID: accumulator.intern(featureID), occurredAt: occurredAt.UnixNano(),
		outcome:    reportOutcomeOf(result),
		troopsSent: troopsSent, troopLosses: troopLosses,
		toolsUsed: toolsUsed, gallantry: gallantry, lootTotal: lootTotal, loot: loot,
	})
}

func (accumulator *featureHistoryAccumulator) finish(fetched int) *featureReportHistory {
	history := accumulator.history
	history.truncated = fetched >= reportHistoryLimit
	// The store orders newest first; keep that invariant explicit so the
	// window pass can close a window at the first record older than its cutoff.
	sort.SliceStable(history.records, func(left, right int) bool {
		return history.records[left].occurredAt > history.records[right].occurredAt
	})
	history.latest = accumulator.latest.UTC()
	return history
}

func analyticsFeatureID(rawFeature string, role string) (string, bool) {
	featureID := strings.TrimSpace(rawFeature)
	if !strings.EqualFold(strings.TrimSpace(role), "attacker") ||
		!State.IsReportAnalyticsFeature(State.AttackFeatureID(featureID)) {
		return "", false
	}
	return featureID, true
}

func compactFeatureReports(rows []Reports.BattleAnalyticsReport) *featureReportHistory {
	accumulator := newFeatureHistoryAccumulator(len(rows))
	for _, report := range rows {
		featureID, ok := analyticsFeatureID(report.AutomationFeature, report.Role)
		if !ok {
			continue
		}
		occurredAt := reportOccurredAt(report)
		if occurredAt.IsZero() {
			continue
		}
		var loot []lootEntry
		if len(report.Loot) > 0 {
			loot = make([]lootEntry, 0, len(report.Loot))
			for key, amount := range report.Loot {
				if normalized := strings.TrimSpace(key); normalized != "" && amount != 0 {
					loot = append(loot, lootEntry{key: accumulator.intern(normalized), amount: amount})
				}
			}
		}
		accumulator.add(
			featureID, occurredAt, report.Result, report.TroopsSent, report.OwnTroopLosses,
			report.ToolsUsed, report.GallantryPoints, report.LootTotal, loot,
		)
	}
	return accumulator.finish(len(rows))
}

func compactFeatureRows(rows []Reports.FeatureAnalyticsRow) *featureReportHistory {
	accumulator := newFeatureHistoryAccumulator(len(rows))
	for _, row := range rows {
		featureID, ok := analyticsFeatureID(row.AutomationFeature, "attacker")
		if !ok {
			continue
		}
		occurredAt, err := time.Parse(time.RFC3339Nano, strings.TrimSpace(row.OccurredAt))
		if err != nil {
			continue
		}
		accumulator.add(
			featureID, occurredAt.UTC(), row.Result, row.TroopsSent, row.OwnTroopLosses,
			row.ToolsUsed, row.GallantryPoints, row.LootTotal, decodeLoot(row.LootJSON, accumulator.intern),
		)
	}
	return accumulator.finish(len(rows))
}

// decodeLoot turns the stored loot document into interned entries. Loot is
// written by encoding/json from map[string]int64, so a flat object of string
// keys and integer values is the only shape produced; anything else falls back
// to the standard decoder so no legitimate document is ever misread.
func decodeLoot(payload []byte, intern func(string) string) []lootEntry {
	if len(payload) == 0 {
		return nil
	}
	if entries, ok := parseFlatIntegerObject(payload, intern); ok {
		return entries
	}
	var decoded map[string]int64
	if err := json.Unmarshal(payload, &decoded); err != nil || len(decoded) == 0 {
		return nil
	}
	entries := make([]lootEntry, 0, len(decoded))
	for key, amount := range decoded {
		if normalized := strings.TrimSpace(key); normalized != "" && amount != 0 {
			entries = append(entries, lootEntry{key: intern(normalized), amount: amount})
		}
	}
	return entries
}

// parseFlatIntegerObject parses `{"key":123,"other":-4}` without reflection or
// intermediate maps. It reports false on any escape sequence, non-integer
// value, nesting, or malformed input so the caller can use encoding/json.
func parseFlatIntegerObject(payload []byte, intern func(string) string) ([]lootEntry, bool) {
	position := 0
	skipSpace := func() {
		for position < len(payload) {
			switch payload[position] {
			case ' ', '\t', '\n', '\r':
				position++
			default:
				return
			}
		}
	}
	skipSpace()
	if position >= len(payload) || payload[position] != '{' {
		return nil, false
	}
	position++
	var entries []lootEntry
	first := true
	for {
		skipSpace()
		if position >= len(payload) {
			return nil, false
		}
		if payload[position] == '}' {
			position++
			break
		}
		if !first {
			if payload[position] != ',' {
				return nil, false
			}
			position++
			skipSpace()
			if position >= len(payload) {
				return nil, false
			}
		}
		first = false
		if payload[position] != '"' {
			return nil, false
		}
		position++
		keyStart := position
		for position < len(payload) && payload[position] != '"' {
			if payload[position] == '\\' || payload[position] < 0x20 {
				return nil, false
			}
			position++
		}
		if position >= len(payload) {
			return nil, false
		}
		key := string(payload[keyStart:position])
		position++
		skipSpace()
		if position >= len(payload) || payload[position] != ':' {
			return nil, false
		}
		position++
		skipSpace()
		valueStart := position
		if position < len(payload) && payload[position] == '-' {
			position++
		}
		digitsStart := position
		for position < len(payload) && payload[position] >= '0' && payload[position] <= '9' {
			position++
		}
		if position == digitsStart || position-digitsStart > 19 {
			return nil, false
		}
		amount, err := strconv.ParseInt(string(payload[valueStart:position]), 10, 64)
		if err != nil {
			return nil, false
		}
		if normalized := strings.TrimSpace(key); normalized != "" && amount != 0 {
			entries = append(entries, lootEntry{key: intern(normalized), amount: amount})
		}
	}
	skipSpace()
	if position != len(payload) {
		return nil, false
	}
	return entries, true
}

func reportOutcomeOf(result string) reportOutcome {
	switch strings.ToLower(strings.TrimSpace(result)) {
	case "victory", "won", "win":
		return reportOutcomeVictory
	case "defeat", "lost", "loss":
		return reportOutcomeDefeat
	default:
		return reportOutcomeOther
	}
}

// windows aggregates the rolling report windows at the given instant. Records
// are newest first, so a window stops accumulating at the first record older
// than its cutoff.
func (history *featureReportHistory) windows(now time.Time) FeatureReportWindows {
	if history == nil {
		return FeatureReportWindows{
			Last24Hours: []FeatureTotals{}, Last7Days: []FeatureTotals{},
			Last30Days: []FeatureTotals{}, All: []FeatureTotals{},
		}
	}
	type window struct {
		cutoff int64
		open   bool
		items  map[string]*FeatureTotals
	}
	windows := [4]window{
		{cutoff: now.Add(-24 * time.Hour).UnixNano(), open: true, items: map[string]*FeatureTotals{}},
		{cutoff: now.Add(-7 * 24 * time.Hour).UnixNano(), open: true, items: map[string]*FeatureTotals{}},
		{cutoff: now.Add(-30 * 24 * time.Hour).UnixNano(), open: true, items: map[string]*FeatureTotals{}},
		{open: true, items: map[string]*FeatureTotals{}},
	}
	for index := range history.records {
		record := &history.records[index]
		for windowIndex := range windows {
			current := &windows[windowIndex]
			if !current.open {
				continue
			}
			if windowIndex < 3 && record.occurredAt < current.cutoff {
				current.open = false
				continue
			}
			addCompactFeatureReport(current.items, record)
		}
	}
	return FeatureReportWindows{
		Last24Hours: sortedFeatureTotals(windows[0].items),
		Last7Days:   sortedFeatureTotals(windows[1].items),
		Last30Days:  sortedFeatureTotals(windows[2].items),
		All:         sortedFeatureTotals(windows[3].items),
	}
}

func addCompactFeatureReport(values map[string]*FeatureTotals, record *compactFeatureReport) {
	current := values[record.featureID]
	if current == nil {
		current = &FeatureTotals{FeatureID: record.featureID, Loot: map[string]int64{}}
		values[record.featureID] = current
	}
	current.Battles++
	switch record.outcome {
	case reportOutcomeVictory:
		current.Victories++
	case reportOutcomeDefeat:
		current.Defeats++
	}
	current.TroopsSent += record.troopsSent
	current.TroopLosses += record.troopLosses
	current.ToolsUsed += record.toolsUsed
	current.GallantryPoints += record.gallantry
	current.LootTotal += record.lootTotal
	for _, entry := range record.loot {
		current.Loot[entry.key] += entry.amount
	}
	occurredAt := time.Unix(0, record.occurredAt).UTC()
	if current.FirstOccurredAt.IsZero() || occurredAt.Before(current.FirstOccurredAt) {
		current.FirstOccurredAt = occurredAt
	}
	if current.LastOccurredAt.IsZero() || occurredAt.After(current.LastOccurredAt) {
		current.LastOccurredAt = occurredAt
	}
}

func sortedFeatureTotals(values map[string]*FeatureTotals) []FeatureTotals {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	result := make([]FeatureTotals, 0, len(keys))
	for _, key := range keys {
		result = append(result, *values[key])
	}
	return result
}

func reportOccurredAt(report Reports.BattleAnalyticsReport) time.Time {
	if observedAt, err := time.Parse(time.RFC3339Nano, strings.TrimSpace(report.OccurredAt)); err == nil {
		return observedAt.UTC()
	}
	if report.DateMs > 0 {
		return time.UnixMilli(report.DateMs).UTC()
	}
	return time.Time{}
}

func publicPlayerCandidate(snapshot State.GameState) PublicPlayerCandidate {
	fields := []string{"allianceId", "level", "legendLevel", "might", "glory"}
	if strings.TrimSpace(snapshot.Player.Name) != "" {
		fields = append(fields, "name")
	}
	allianceName := ""
	if snapshot.Player.AllianceID > 0 {
		if snapshot.Alliance.ID == snapshot.Player.AllianceID {
			allianceName = strings.TrimSpace(snapshot.Alliance.Name)
		} else if alliance, found := snapshot.Alliances[snapshot.Player.AllianceID]; found {
			allianceName = strings.TrimSpace(alliance.Name)
		}
	}
	if allianceName != "" {
		fields = append(fields, "allianceName")
	}
	sort.Strings(fields)
	return PublicPlayerCandidate{
		PresentFields: fields, Name: strings.TrimSpace(snapshot.Player.Name),
		AllianceID: int64(snapshot.Player.AllianceID), AllianceName: allianceName,
		Level: snapshot.Player.Level, LegendLevel: snapshot.Player.LegendLevel,
		Might: snapshot.Player.Might, Glory: snapshot.Player.Glory,
	}
}

func combatTotals(value State.EventCombatTotals) CombatTotals {
	return CombatTotals{
		Launches: value.Launches, Battles: value.Battles, Victories: value.Victories,
		Defeats: value.Defeats, TroopLosses: value.TroopLosses,
		ToolsUsed: value.ToolsUsed, Loot: value.Loot,
	}
}

func cloneInt64Map(values map[string]int64) map[string]int64 {
	result := make(map[string]int64, len(values))
	for key, value := range values {
		result[key] = value
	}
	return result
}

func cloneFloat64Map(values map[string]float64) map[string]float64 {
	result := make(map[string]float64, len(values))
	for key, value := range values {
		result[key] = value
	}
	return result
}
