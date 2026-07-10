package featureview

import (
	"testing"
	"time"
)

func TestAutoBirdFarthestTravelSecondsIsOrderIndependent(t *testing.T) {
	orders := [][]int{
		{2 * 60 * 60, 30 * 60},
		{30 * 60, 2 * 60 * 60},
	}

	for _, travelTimes := range orders {
		farthest := 0
		for _, travelSeconds := range travelTimes {
			farthest = autoBirdFarthestTravelSeconds(farthest, travelSeconds)
		}
		if farthest != 2*60*60 {
			t.Fatalf("farthest travel for %v = %d seconds, want %d", travelTimes, farthest, 2*60*60)
		}
	}
}

func TestAutoBirdSleepDuration(t *testing.T) {
	tests := []struct {
		name              string
		travelSeconds     int
		waitDelay         time.Duration
		wantSleepDuration time.Duration
	}{
		{
			name:              "farthest movement round trip plus wait hours",
			travelSeconds:     2 * 60 * 60,
			waitDelay:         6 * time.Hour,
			wantSleepDuration: 10 * time.Hour,
		},
		{
			name:              "short movement uses exact travel time",
			travelSeconds:     10,
			waitDelay:         time.Hour,
			wantSleepDuration: time.Hour + 20*time.Second,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := autoBirdSleepDuration(tt.travelSeconds, tt.waitDelay)
			if got != tt.wantSleepDuration {
				t.Fatalf("autoBirdSleepDuration(%d, %s) = %s, want %s",
					tt.travelSeconds, tt.waitDelay, got, tt.wantSleepDuration)
			}
		})
	}
}
