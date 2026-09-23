package Buildings

import (
	"CitadelDesktop/Server/GameData"
	"CitadelDesktop/Server/State"
	"testing"
	"time"
)

func TestDemolitionRemainingOfficialDurationAndBoost(t *testing.T) {
	data, err := GameData.DecodeStore([]byte(`{"versionInfo":[],"units":[],"buildings":[{"wodID":249,"name":"FactionMarket","buildDuration":9000},{"wodID":250,"name":"OddDuration","buildDuration":9003}]}`), GameData.SourceMetadata{})
	if err != nil {
		t.Fatal(err)
	}
	catalog, _ := data.BuildingCatalog()
	now := time.Date(2026, 9, 23, 2, 16, 30, 0, time.UTC)
	hundred, twoHundred, zero := 100.0, 200.0, 0.0
	for _, tc := range []struct {
		name     string
		id       State.BuildingID
		boost    float64
		progress int64
		elapsed  time.Duration
		stopped  bool
		want     int64
		known    bool
	}{
		{"market before second skip", 249, hundred, 3622, 0, false, 878, true},
		{"market complete", 249, hundred, 4500, 0, false, 0, true},
		{"double speed", 249, twoHundred, 2000, 0, false, 250, true},
		{"integer truncation", 250, twoHundred, 0, 0, false, 2250, true},
		{"elapsed ceiling", 249, hundred, 4498, 1500 * time.Millisecond, false, 1, true},
		{"elapsed complete", 249, hundred, 4498, 2 * time.Second, false, 0, true},
		{"stopped does not accrue", 249, hundred, 4000, time.Minute, true, 500, true},
		{"missing boost", 249, 0, 0, 0, false, 0, false},
		{"zero boost", 249, zero, 0, 0, false, 0, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			castle := State.CastleState{}
			castle.Layout.ObservedAt = now.Add(-tc.elapsed)
			building := State.Building{DefinitionID: tc.id, ConstructionState: State.BuildingStateDisassembleInProgress, ConstructionBoostPercent: tc.boost, ProgressSec: tc.progress}
			if tc.stopped {
				building.ConstructionState = State.BuildingStateDisassembleStopped
			}
			got, known := DemolitionRemaining(castle, building, catalog, now)
			if got != tc.want || known != tc.known {
				t.Fatalf("got%d,%t want%d,%t", got, known, tc.want, tc.known)
			}
		})
	}
}
