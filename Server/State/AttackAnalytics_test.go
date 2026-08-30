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
