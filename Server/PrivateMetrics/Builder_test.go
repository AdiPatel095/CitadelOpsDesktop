package PrivateMetrics

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"CitadelDesktop/Server/Reports"
	"CitadelDesktop/Server/State"
)

type stubReportReader struct {
	reports []Reports.BattleAnalyticsReport
	err     error
	query   Reports.BattleReportQuery
}

type aggregateReportReader struct {
	rows        map[Reports.ResourceViewKey][]Reports.ResourceAggregate
	recentCalls atomic.Int64
}

func (reader *aggregateReportReader) Recent(_ context.Context, _ Reports.BattleReportQuery) ([]Reports.BattleAnalyticsReport, error) {
	reader.recentCalls.Add(1)
	return nil, errors.New("raw reports must not be read when resource aggregates are available")
}

func (reader *aggregateReportReader) ResourceAggregates(_ context.Context, query Reports.ResourceAggregateQuery) ([]Reports.ResourceAggregate, error) {
	return append([]Reports.ResourceAggregate(nil), reader.rows[query.ViewKey]...), nil
}

func (reader *stubReportReader) Recent(_ context.Context, query Reports.BattleReportQuery) ([]Reports.BattleAnalyticsReport, error) {
	reader.query = query
	return append([]Reports.BattleAnalyticsReport(nil), reader.reports...), reader.err
}

func TestBuildSampleMergesMyStatsAndPrivateFeatureStats(t *testing.T) {
	now := time.Date(2026, time.August, 16, 12, 0, 0, 0, time.UTC)
	store := readyPrivateMetricsState(t, now)
	reports := &stubReportReader{reports: []Reports.BattleAnalyticsReport{
		{
			AutomationFeature: string(State.AttackFeatureAutoTowers), Role: "attacker", Result: "Victory",
			OccurredAt: now.Add(-time.Hour).Format(time.RFC3339Nano), TroopsSent: 500,
			OwnTroopLosses: 12, ToolsUsed: 4, LootTotal: 200, Loot: map[string]int64{"resource:1": 200},
		},
		{
			AutomationFeature: string(State.AttackFeatureAutoStorm), Role: "attacker", Result: "Defeat",
			OccurredAt: now.Add(-8 * 24 * time.Hour).Format(time.RFC3339Nano), TroopsSent: 250,
			OwnTroopLosses: 250, ToolsUsed: 8, LootTotal: 5, Loot: map[string]int64{"resource:2": 5},
		},
		{
			AutomationFeature: string(State.AttackFeatureAutoTowers), Role: "defender", Result: "Victory",
			OccurredAt: now.Add(-time.Hour).Format(time.RFC3339Nano), LootTotal: 999,
		},
	}}

	sample, err := BuildSample(t.Context(), store, nil, reports, now)
	if err != nil {
		t.Fatal(err)
	}
	if sample.Account.AccountUID != 7001 || sample.Account.WorldID != "world.example" || sample.Account.PlayerID != 42 {
		t.Fatalf("account binding = %+v", sample.Account)
	}
	if sample.Player.Might != 12345 || sample.Player.Glory != 678 || sample.Player.Gallantry != 90 ||
		sample.Player.TroopsTotal != 19 || sample.Player.TroopsByUnit["10"] != 15 ||
		sample.Player.Coins != 5000 || sample.Player.Rubies != 44 ||
		sample.Player.Currencies["resource:1"] != 5000 || sample.Player.Currencies["resource:37"] != 88 ||
		sample.Player.Currencies["currency:9"] != 7 || sample.Player.Currencies["currency:30"] != 99 {
		t.Fatalf("player metrics = %+v", sample.Player)
	}
	for _, zeroBalanceKey := range []string{"resource:12", "currency:22"} {
		if amount, found := sample.Player.Currencies[zeroBalanceKey]; !found || amount != 0 {
			t.Fatalf("player metrics omitted zero balance %s: %+v", zeroBalanceKey, sample.Player.Currencies)
		}
	}
	if reports.query.AccountUID != 7001 || reports.query.WorldID != "wss://world.example:443/socket" ||
		reports.query.PlayerID != 42 || reports.query.Limit != reportHistoryLimit {
		t.Fatalf("report query = %+v", reports.query)
	}
	if sample.Features.ReportCount != 2 || !sample.Features.ReportAnalyticsPresent || sample.Features.ReportHistoryTruncated ||
		len(sample.Features.ReportWindows.Last24Hours) != 1 ||
		len(sample.Features.ReportWindows.Last7Days) != 1 ||
		len(sample.Features.ReportWindows.Last30Days) != 2 ||
		len(sample.Features.ReportWindows.All) != 2 {
		t.Fatalf("feature windows = %+v", sample.Features)
	}
	towers := sample.Features.ReportWindows.Last24Hours[0]
	if towers.FeatureID != string(State.AttackFeatureAutoTowers) || towers.Victories != 1 ||
		towers.TroopLosses != 12 || towers.Loot["resource:1"] != 200 {
		t.Fatalf("tower rollup = %+v", towers)
	}
	if len(sample.Features.EventScores) != 1 || sample.Features.EventScores[0].PlayerScore != 321 ||
		len(sample.Features.EventActivities) != 1 || sample.Features.EventActivities[0].Invasion.Victories != 3 {
		t.Fatalf("event feature metrics = %+v", sample.Features)
	}
	publicPayload, err := json.Marshal(sample.PublicCandidate)
	if err != nil {
		t.Fatal(err)
	}
	for _, privateField := range []string{"troops", "currencies", "gallantry", "feature"} {
		if strings.Contains(strings.ToLower(string(publicPayload)), privateField) {
			t.Fatalf("public candidate exposed %q: %s", privateField, publicPayload)
		}
	}
}

func TestBuildSampleUsesMinuteAggregatesBeyondRawReportLimit(t *testing.T) {
	now := time.Date(2026, time.September, 1, 20, 0, 0, 0, time.UTC)
	reader := &aggregateReportReader{rows: map[Reports.ResourceViewKey][]Reports.ResourceAggregate{
		Reports.ResourceViewTower: {{
			ViewKey: Reports.ResourceViewTower, BucketStart: now.Add(-time.Hour).Truncate(time.Minute),
			BucketSeconds: 60, ReportCount: 12_050, Victories: 12_000, Defeats: 50,
			TroopsSent: 500_000, TroopLosses: 1_000, ToolsUsed: 25_000,
			LootTotal: 900_000, Resources: map[string]int64{"W": 900_000},
			FirstOccurredAt: now.Add(-time.Hour), LastOccurredAt: now.Add(-time.Hour + 59*time.Second),
		}},
	}}
	sample, err := BuildSample(t.Context(), readyPrivateMetricsState(t, now), nil, reader, now)
	if err != nil {
		t.Fatal(err)
	}
	if reader.recentCalls.Load() != 0 || sample.Features.ReportHistoryTruncated || sample.Features.ReportCount != 12_050 {
		t.Fatalf("aggregate feature metadata = %+v; raw calls=%d", sample.Features, reader.recentCalls.Load())
	}
	if len(sample.Features.ReportWindows.Last24Hours) != 1 {
		t.Fatalf("aggregate windows = %+v", sample.Features.ReportWindows)
	}
	towers := sample.Features.ReportWindows.Last24Hours[0]
	if towers.Battles != 12_050 || towers.Victories != 12_000 || towers.Defeats != 50 || towers.Loot["W"] != 900_000 {
		t.Fatalf("tower aggregate rollup = %+v", towers)
	}
}

func TestBuildSampleFailsClosedUntilBindingAndSessionMatch(t *testing.T) {
	now := time.Now().UTC()
	tests := []struct {
		name   string
		mutate func(*State.GameState)
	}{
		{name: "player mismatch", mutate: func(state *State.GameState) { state.Account.PlayerID++ }},
		{name: "world mismatch", mutate: func(state *State.GameState) { state.Account.WorldID = "other.example" }},
		{name: "not logged in", mutate: func(state *State.GameState) { state.Session.LoggedIn = false }},
		{name: "socket not ready", mutate: func(state *State.GameState) { state.Session.SocketReady = false }},
		{name: "baseline mismatch", mutate: func(state *State.GameState) { state.Session.BaselineGeneration++ }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			store := readyPrivateMetricsState(t, now)
			_, err := store.ApplyComponents(State.Components(State.ComponentAccount, State.ComponentSession), func(state *State.GameState) ([]string, bool, error) {
				test.mutate(state)
				return []string{"test"}, true, nil
			})
			if err != nil {
				t.Fatal(err)
			}
			if _, err := BuildSample(t.Context(), store, nil, nil, now); !errors.Is(err, ErrRuntimeNotReady) {
				t.Fatalf("BuildSample error = %v", err)
			}
		})
	}
}

func TestBuildSampleDoesNotReplaceCompleteStatsWhenFeatureReadFails(t *testing.T) {
	store := readyPrivateMetricsState(t, time.Now().UTC())
	reader := &stubReportReader{err: errors.New("database unavailable")}
	if _, err := BuildSample(t.Context(), store, nil, reader, time.Now().UTC()); err == nil ||
		!strings.Contains(err.Error(), "feature analytics") {
		t.Fatalf("BuildSample error = %v", err)
	}
}

func readyPrivateMetricsState(t testing.TB, now time.Time) *State.Store {
	t.Helper()
	store := State.NewStore(State.NewGameState())
	_, err := store.ApplyComponents(State.Components(
		State.ComponentSession, State.ComponentAccount, State.ComponentPlayer,
		State.ComponentCastles, State.ComponentAlliance, State.ComponentEventScores,
	), func(state *State.GameState) ([]string, bool, error) {
		state.Session.ServerURL = "world.example"
		state.Session.LoggedIn = true
		state.Session.SocketReady = true
		state.Session.Generation = 8
		state.Session.BaselineGeneration = 8
		state.Session.ConnectionGeneration = 3
		state.Account = State.AccountBindingState{
			UID: 7001, WorldID: "wss://world.example:443/socket", PlayerID: 42, BoundAt: now.Add(-time.Hour),
		}
		state.Player.ID = 42
		state.Player.Name = "Player Forty Two"
		state.Player.AllianceID = 11
		state.Player.Level = 70
		state.Player.LegendLevel = 950
		state.Player.Might = 12345
		state.Player.Glory = 678
		state.Player.Gallantry = 90
		state.Player.Resources = map[State.ResourceID]float64{1: 5000, 2: 44, 12: 0, 37: 88}
		state.Player.Currencies = map[State.CurrencyID]float64{9: 7, 22: 0, 30: 99}
		state.Alliance = State.AllianceState{ID: 11, Name: "The Alliance"}
		state.SetCastle(1, State.CastleState{ID: 1, Units: State.CastleUnits{
			Stationed: map[State.UnitID]int64{10: 12}, Traveling: map[State.UnitID]int64{10: 3},
			Hospital: map[State.UnitID]int64{20: 4}, SpecialHospital: map[State.UnitID]int64{},
		}})
		state.SetScalableEventScore(71, State.ScalableEventScore{
			EventID: 71, EventType: "invasion", PlayerScore: 321, AllianceScore: 654,
			ObservedAt: now.Add(-time.Minute), RemainingSec: 3600,
		})
		state.SetEventActivity(71, State.EventActivityState{
			EventID: 71, OccurrenceEndsAt: now.Add(time.Hour), ObservedFrom: now.Add(-2 * time.Hour),
			Invasion: State.EventCombatTotals{Battles: 4, Victories: 3, Defeats: 1, Loot: 900},
		})
		return []string{"test-ready"}, true, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return store
}

type trackedStubReportReader struct {
	stubReportReader
	generation atomic.Uint64
	calls      atomic.Int64
}

func (reader *trackedStubReportReader) AnalyticsGeneration() uint64 { return reader.generation.Load() }

func (reader *trackedStubReportReader) Recent(ctx context.Context, query Reports.BattleReportQuery) ([]Reports.BattleAnalyticsReport, error) {
	reader.calls.Add(1)
	return reader.stubReportReader.Recent(ctx, query)
}

func TestSampleBuilderReusesFeatureReportsUntilAnalyticsChange(t *testing.T) {
	now := time.Date(2026, time.August, 16, 12, 0, 0, 0, time.UTC)
	store := readyPrivateMetricsState(t, now)
	reader := &trackedStubReportReader{stubReportReader: stubReportReader{reports: []Reports.BattleAnalyticsReport{
		{
			AutomationFeature: string(State.AttackFeatureAutoTowers), Role: "attacker", Result: "Victory",
			OccurredAt: now.Add(-23 * time.Hour).Format(time.RFC3339Nano), TroopsSent: 500, LootTotal: 200,
			Loot: map[string]int64{"resource:1": 200},
		},
	}}}
	builder := NewSampleBuilder(store, nil, reader)

	first, err := builder.Build(t.Context(), now)
	if err != nil {
		t.Fatal(err)
	}
	if len(first.Features.ReportWindows.Last24Hours) != 1 || first.Features.ReportCount != 1 {
		t.Fatalf("initial windows = %+v", first.Features.ReportWindows)
	}
	// Two hours later the same report has aged out of the 24h window but stays
	// in the 7d window; this must roll without re-reading the analytics store.
	later, err := builder.Build(t.Context(), now.Add(2*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if len(later.Features.ReportWindows.Last24Hours) != 0 || len(later.Features.ReportWindows.Last7Days) != 1 ||
		later.Features.ReportWindows.Last7Days[0].Loot["resource:1"] != 200 {
		t.Fatalf("rolled windows = %+v", later.Features.ReportWindows)
	}
	if calls := reader.calls.Load(); calls != 1 {
		t.Fatalf("analytics were read %d times for an unchanged store", calls)
	}

	reader.reports = append(reader.reports, Reports.BattleAnalyticsReport{
		AutomationFeature: string(State.AttackFeatureAutoStorm), Role: "attacker", Result: "Defeat",
		OccurredAt: now.Add(time.Hour).Format(time.RFC3339Nano), TroopsSent: 100, OwnTroopLosses: 100,
	})
	unchanged, err := builder.Build(t.Context(), now.Add(2*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if unchanged.Features.ReportCount != 1 || reader.calls.Load() != 1 {
		t.Fatal("builder re-read analytics although the store generation did not advance")
	}
	reader.generation.Add(1)
	refreshed, err := builder.Build(t.Context(), now.Add(2*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if refreshed.Features.ReportCount != 2 || reader.calls.Load() != 2 ||
		len(refreshed.Features.ReportWindows.Last24Hours) != 1 ||
		refreshed.Features.ReportWindows.Last24Hours[0].FeatureID != string(State.AttackFeatureAutoStorm) ||
		refreshed.Features.ReportWindows.Last24Hours[0].Defeats != 1 {
		t.Fatalf("refreshed windows = %+v (calls %d)", refreshed.Features.ReportWindows, reader.calls.Load())
	}
	if !refreshed.Features.ReportsObservedAt.Equal(now.Add(time.Hour)) {
		t.Fatalf("reports observed at = %v", refreshed.Features.ReportsObservedAt)
	}
}

// recentOnlyReader hides the lean and change-tracking interfaces so the
// builder must take the complete-row Recent path.
type recentOnlyReader struct{ store *Reports.SQLiteStore }

func (reader recentOnlyReader) Recent(ctx context.Context, query Reports.BattleReportQuery) ([]Reports.BattleAnalyticsReport, error) {
	return reader.store.Recent(ctx, query)
}

func TestSampleBuilderCachedLeanWindowsMatchCompleteRowAggregation(t *testing.T) {
	now := time.Now().UTC()
	store := seedAnalyticsStore(t, 600, now)
	state := readyPrivateMetricsState(t, now)
	cached := NewSampleBuilder(state, nil, store)
	if _, err := cached.Build(t.Context(), now); err != nil {
		t.Fatal(err)
	}
	for _, offset := range []time.Duration{0, 90 * time.Minute, 3 * 24 * time.Hour, 20 * 24 * time.Hour} {
		observedAt := now.Add(offset)
		lean, err := cached.Build(t.Context(), observedAt)
		if err != nil {
			t.Fatal(err)
		}
		complete, err := BuildSample(t.Context(), state, nil, recentOnlyReader{store: store}, observedAt)
		if err != nil {
			t.Fatal(err)
		}
		leanJSON, _ := json.Marshal(lean.Features)
		completeJSON, _ := json.Marshal(complete.Features)
		if string(leanJSON) != string(completeJSON) {
			t.Fatalf("lean cached feature metrics diverged at +%s:\n%s\n%s", offset, leanJSON, completeJSON)
		}
	}
	if store.AnalyticsGeneration() == 0 {
		t.Fatal("seeded analytics store did not advance its generation")
	}
}

func TestDecodeLootMatchesEncodingJSON(t *testing.T) {
	intern := func(value string) string { return value }
	documents := []map[string]int64{
		{}, {"resource:1": 100}, {"resource:1": 100, "resource:2": 5, "currency:9": -3},
		{"": 4, "resource:3": 0, " padded ": 7}, {"resource:1": 9223372036854775807},
	}
	for _, document := range documents {
		payload, err := json.Marshal(document)
		if err != nil {
			t.Fatal(err)
		}
		fast, ok := parseFlatIntegerObject(payload, intern)
		if !ok {
			t.Fatalf("fast path rejected %s", payload)
		}
		expected := map[string]int64{}
		for key, amount := range document {
			if normalized := strings.TrimSpace(key); normalized != "" && amount != 0 {
				expected[normalized] += amount
			}
		}
		got := map[string]int64{}
		for _, entry := range fast {
			got[entry.key] += entry.amount
		}
		if len(got) != len(expected) {
			t.Fatalf("decoded %v from %s, want %v", got, payload, expected)
		}
		for key, amount := range expected {
			if got[key] != amount {
				t.Fatalf("decoded %v from %s, want %v", got, payload, expected)
			}
		}
	}
	for _, malformed := range []string{
		`{"a":1.5}`, `{"a":"1"}`, `{"a\u0041":1}`, `{"a":{"b":1}}`, `[1,2]`, `{"a":1,}`, `{"a":1}x`, `{"a":}`,
		`{"a":99999999999999999999}`, `{"a":1 "b":2}`, `null`, ``,
	} {
		if entries, ok := parseFlatIntegerObject([]byte(malformed), intern); ok {
			t.Fatalf("fast path accepted malformed %q as %v", malformed, entries)
		}
	}
	// The public decoder must still recover documents the fast path declines.
	escaped := decodeLoot([]byte(`{"a\u0041":12}`), intern)
	if len(escaped) != 1 || escaped[0].key != "aA" || escaped[0].amount != 12 {
		t.Fatalf("fallback decode = %+v", escaped)
	}
	if entries := decodeLoot([]byte(`not json`), intern); entries != nil {
		t.Fatalf("invalid loot decoded as %+v", entries)
	}
}
