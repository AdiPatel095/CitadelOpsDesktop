package App

import (
	"CitadelDesktop/Server/State"
	"context"
	"fmt"
	"testing"
	"time"
)

// Compare cooperative discovery with the actual legacy scanner, including
// expansion far beyond the seed. This is a fleet/scan simulation, not I/O.
func TestSharedFortressScanMatchesLegacyAdaptiveCoverage(t *testing.T) {
	source := State.CastleState{X: 450, Y: 450}
	content := func(w towerMapWindow) bool { return w.X1/90 >= 4 && w.X1/90 <= 12 && w.Y1/90 >= 4 && w.Y1/90 <= 10 }
	legacy, err := discoverFullFortressMap(t.Context(), source, func(_ context.Context, w towerMapWindow) (bool, error) { return content(w), nil })
	if err != nil {
		t.Fatal(err)
	}
	for _, n := range []int{1, 2, 6, 8} {
		t.Run(fmt.Sprint(n), func(t *testing.T) {
			store := State.NewWorldMapStore()
			scope := State.MapScanScope{Kind: "fortress", WorldID: "world-one", KingdomID: 1}
			now := time.Date(2026, 10, 2, 12, 0, 0, 0, time.UTC)
			seen := map[State.StormMapBounds]bool{}
			requests := map[string]int{}
			for tick := 0; tick < 100; tick++ {
				at := now.Add(time.Duration(tick) * time.Second)
				for i := 0; i < n; i++ {
					store.RegisterFortressScanner(fmt.Sprint(i), scope, source.X, source.Y, time.Hour, at)
				}
				for i := 0; i < n; i++ {
					id := fmt.Sprint(i)
					lease := store.AcquireFortressScan(id, scope, at)
					for _, w := range lease.Windows {
						if seen[w] {
							t.Fatal("duplicate coverage")
						}
						seen[w] = true
						requests[id]++
						if requests[id] > len(legacy.Windows) {
							t.Fatal("account exceeded actual legacy request count")
						}
						if err := store.CompleteFortressWindow(id, scope, lease.LeaseID, w, content(towerMapWindow{X1: w.X1, Y1: w.Y1, X2: w.X2, Y2: w.Y2}), at); err != nil {
							t.Fatal(err)
						}
					}
				}
			}
			if len(seen) != len(legacy.Windows) {
				t.Fatalf("cooperative %d windows, legacy %d", len(seen), len(legacy.Windows))
			}
			for _, w := range legacy.Windows {
				if !seen[State.StormMapBounds{X1: w.X1, Y1: w.Y1, X2: w.X2, Y2: w.Y2}] {
					t.Fatalf("missing legacy window %+v", w)
				}
			}
		})
	}
}
