package PrivateMetrics

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"CitadelDesktop/Server/Reports"
	"CitadelDesktop/Server/State"
)

// seedAnalyticsStore writes count attacker reports spread across the analytics
// features and the last 45 days so every rolling window is populated.
func seedAnalyticsStore(tb testing.TB, count int, now time.Time) *Reports.SQLiteStore {
	tb.Helper()
	store, err := Reports.OpenSQLiteStore(tb.TempDir())
	if err != nil {
		tb.Fatal(err)
	}
	tb.Cleanup(func() { _ = store.Close() })
	features := []State.AttackFeatureID{
		State.AttackFeatureAutoTowers, State.AttackFeatureAutoStorm, State.AttackFeatureAutoKhan,
		State.AttackFeatureAutoNomad, State.AttackFeatureAutoInvasion, State.AttackFeatureAutoAdvisor,
	}
	reports := make([]Reports.BattleReport, 0, count)
	for index := 0; index < count; index++ {
		occurredAt := now.Add(-time.Duration(index) * (45 * 24 * time.Hour / time.Duration(count)))
		result := "Victory"
		if index%7 == 0 {
			result = "Defeat"
		}
		reports = append(reports, Reports.BattleReport{
			ID: fmt.Sprintf("report-%d", index), AccountUID: 7001, WorldID: "wss://world.example:443/socket",
			PlayerID: 42, AutomationFeature: string(features[index%len(features)]), MID: int64(index),
			LID: int64(index), OccurredAt: occurredAt.Format(time.RFC3339Nano), DateMs: occurredAt.UnixMilli(),
			Result: result, Role: "attacker",
			Attacker:  &Reports.BattleCombatant{PlayerID: 42, Name: "Player Forty Two"},
			Metrics:   Reports.BattleMetrics{AttackerSent: 400 + int64(index%50), AttackerLost: int64(index % 40)},
			ToolsUsed: int64(index % 9), GallantryPoints: int64(index % 3),
			Loot: map[string]int64{"resource:1": int64(100 + index%50), "resource:2": int64(index % 30), "resource:3": 5},
		})
	}
	if err := store.SaveMany(context.Background(), reports); err != nil {
		tb.Fatal(err)
	}
	return store
}

func BenchmarkBuildSampleWith10kReports(b *testing.B) {
	now := time.Now().UTC()
	state := readyPrivateMetricsState(b, now)
	store := seedAnalyticsStore(b, 10_000, now)
	ctx := context.Background()
	b.ReportAllocs()
	b.ResetTimer()
	for index := 0; index < b.N; index++ {
		sample, err := BuildSample(ctx, state, nil, store, now.Add(time.Duration(index)*time.Minute))
		if err != nil {
			b.Fatal(err)
		}
		if sample.Features.ReportCount == 0 {
			b.Fatal("no reports aggregated")
		}
	}
}

func BenchmarkRecentReports10k(b *testing.B) {
	now := time.Now().UTC()
	store := seedAnalyticsStore(b, 10_000, now)
	ctx := context.Background()
	b.ReportAllocs()
	b.ResetTimer()
	for index := 0; index < b.N; index++ {
		rows, err := store.Recent(ctx, Reports.BattleReportQuery{
			AccountUID: 7001, WorldID: "wss://world.example:443/socket", PlayerID: 42, Limit: reportHistoryLimit,
		})
		if err != nil {
			b.Fatal(err)
		}
		if len(rows) == 0 {
			b.Fatal("no rows")
		}
	}
}

func BenchmarkSampleJSONSize(b *testing.B) {
	now := time.Now().UTC()
	state := readyPrivateMetricsState(b, now)
	store := seedAnalyticsStore(b, 10_000, now)
	sample, err := BuildSample(context.Background(), state, nil, store, now)
	if err != nil {
		b.Fatal(err)
	}
	payload, err := json.Marshal(PublishRequest{Sample: sample})
	if err != nil {
		b.Fatal(err)
	}
	b.ReportMetric(float64(len(payload)), "bytes/sample")
	b.ReportAllocs()
	b.ResetTimer()
	for index := 0; index < b.N; index++ {
		if _, err := json.Marshal(PublishRequest{Sample: sample}); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkSampleBuilderCachedWith10kReports is the steady-state publisher
// cost: the analytics store is unchanged between samples, so only the rolling
// window pass over the compact in-memory projection runs.
func BenchmarkSampleBuilderCachedWith10kReports(b *testing.B) {
	now := time.Now().UTC()
	state := readyPrivateMetricsState(b, now)
	store := seedAnalyticsStore(b, 10_000, now)
	builder := NewSampleBuilder(state, nil, store)
	ctx := context.Background()
	if _, err := builder.Build(ctx, now); err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for index := 0; index < b.N; index++ {
		sample, err := builder.Build(ctx, now.Add(time.Duration(index)*time.Minute))
		if err != nil {
			b.Fatal(err)
		}
		if sample.Features.ReportCount == 0 {
			b.Fatal("no reports aggregated")
		}
	}
}

func BenchmarkRecentFeatureAnalytics10k(b *testing.B) {
	now := time.Now().UTC()
	store := seedAnalyticsStore(b, 10_000, now)
	ctx := context.Background()
	b.ReportAllocs()
	b.ResetTimer()
	for index := 0; index < b.N; index++ {
		rows, err := store.RecentFeatureAnalytics(ctx, Reports.BattleReportQuery{
			AccountUID: 7001, WorldID: "wss://world.example:443/socket", PlayerID: 42, Limit: reportHistoryLimit,
		})
		if err != nil {
			b.Fatal(err)
		}
		if len(rows) == 0 {
			b.Fatal("no rows")
		}
	}
}
