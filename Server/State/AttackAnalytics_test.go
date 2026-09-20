package State

import (
	"testing"
	"time"
)

func TestMergeAutoStormLaunchHistoryPreservesLaunchTimeAndLargestTroopCount(t *testing.T) {
	now := time.Date(2026, time.July, 29, 12, 0, 0, 0, time.UTC)
	arrivesAt := now.Add(-time.Hour)
	gameState := NewGameState()
	gameState.AttackAnalytics.RecentAutoStormLaunches = []AttackFeatureLaunch{{
		MovementID: 10, FeatureID: AttackFeatureAutoStorm, KingdomID: 4,
		TroopCount: 500, LaunchedAt: now.Add(-2 * time.Hour), ArrivesAt: arrivesAt,
	}}

	if MergeAutoStormLaunchHistory(&gameState, []AttackFeatureLaunch{{
		MovementID: 10, FeatureID: AttackFeatureAutoStorm, KingdomID: 4,
		TroopCount: 450, LaunchedAt: now.Add(-time.Hour),
	}}, now) {
		t.Fatal("later report duplicate changed the confirmed launch record")
	}
	launches := gameState.AttackAnalytics.RecentAutoStormLaunches
	if len(launches) != 1 || launches[0].TroopCount != 500 ||
		!launches[0].LaunchedAt.Equal(now.Add(-2*time.Hour)) ||
		!launches[0].ArrivesAt.Equal(arrivesAt) {
		t.Fatalf("merged launch = %#v", launches)
	}
}

func TestTowerAdvisorTimeSkipUsageIsIdempotentAndResetScoped(t *testing.T) {
	now := time.Date(2026, time.September, 3, 14, 0, 0, 0, time.UTC)
	gameState := NewGameState()
	first := TowerAdvisorTimeSkipUsage{MovementID: 10, TimeSkips: 4, UsedAt: now.Add(-2 * time.Hour)}
	if !RecordTowerAdvisorTimeSkipUsage(&gameState, first, now) {
		t.Fatal("first confirmed Advisor Time Skip usage was not recorded")
	}
	if RecordTowerAdvisorTimeSkipUsage(&gameState, first, now) {
		t.Fatal("duplicate movement changed Advisor Time Skip usage")
	}
	if !RecordTowerAdvisorTimeSkipUsage(&gameState, TowerAdvisorTimeSkipUsage{
		MovementID: 11, TimeSkips: 2, UsedAt: now.Add(-30 * time.Minute),
	}, now) {
		t.Fatal("second confirmed Advisor Time Skip usage was not recorded")
	}

	if used, exact := TowerAdvisorTimeSkipsUsedSince(gameState, now.Add(-3*time.Hour), now); !exact || used != 6 {
		t.Fatalf("current server-day usage = %d exact=%t, want 6", used, exact)
	}
	if used, exact := TowerAdvisorTimeSkipsUsedSince(gameState, now.Add(-time.Hour), now); !exact || used != 2 {
		t.Fatalf("post-reset usage = %d exact=%t, want 2", used, exact)
	}
	if _, exact := TowerAdvisorTimeSkipsUsedSince(gameState, now.Add(-TowerAdvisorTimeSkipUsageRetention-time.Second), now); exact {
		t.Fatal("usage older than retained evidence was reported as exact")
	}
	gameState.AttackAnalytics.RecentTowerAdvisorTimeSkips = append(
		gameState.AttackAnalytics.RecentTowerAdvisorTimeSkips,
		TowerAdvisorTimeSkipUsage{MovementID: 12, TimeSkips: 1, UsedAt: now.Add(time.Minute)},
	)
	if _, exact := TowerAdvisorTimeSkipsUsedSince(gameState, now.Add(-3*time.Hour), now); exact {
		t.Fatal("future Advisor Time Skip evidence was reported as exact")
	}
}
