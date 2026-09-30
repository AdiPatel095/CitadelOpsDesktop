package State

import (
	"context"
	"testing"
	"time"
)

func TestStormScanWindowRetention(t *testing.T) {
	t.Run("grid shift expires in memory and SQLite", func(t *testing.T) {
		directory := t.TempDir()
		store, err := OpenWorldMapStore(directory)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = store.Close(context.Background()) })
		now := time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC)
		fact := MapObservation{KingdomID: stormKingdomID, X: 200, Y: 600, TypeID: MapTypeStormFort, ObservedAt: now.Add(-time.Minute)}
		store.commit("world-one", nil, []MapChange{{KingdomID: stormKingdomID, Key: "200:600", Observation: &fact}})
		store.AcquireStormScan("alpha", "world-one", stormKingdomID, now)
		lease := store.AcquireStormScan("alpha", "world-one", stormKingdomID, now.Add(sharedStormRosterSettleDelay+time.Millisecond))
		completedAt := now.Add(3 * time.Second)
		prior := store.Snapshot("world-one")
		if _, err := store.CompleteStormScan("alpha", "world-one", stormKingdomID, lease.LeaseID, lease.Windows, now, completedAt); err != nil {
			t.Fatal(err)
		}
		generation := store.Snapshot("world-one")
		beforePlan := sharedStormScanPlan(prior, stormKingdomID)
		plan := sharedStormScanPlan(generation, stormKingdomID)
		if beforePlan.Bounds == plan.Bounds {
			t.Fatal("fixture did not shift the grid")
		}
		planKeys := map[string]bool{}
		for i, bounds := range plan.Windows {
			planKeys[stormScanPlanWindowKey(plan, i, bounds)] = true
		}
		oldKeys := []string{}
		for _, bounds := range lease.Windows {
			key := stormScanWindowKey(bounds)
			if !planKeys[key] {
				oldKeys = append(oldKeys, key)
			}
		}
		if len(oldKeys) == 0 {
			t.Fatal("grid shift left no obsolete keys")
		}
		for _, key := range oldKeys {
			if _, ok := generation.stormWindows[stormKingdomID][key]; !ok {
				t.Fatal("recent obsolete key pruned")
			}
		}
		if err := store.flushPersistence(t.Context()); err != nil {
			t.Fatal(err)
		}
		// At exactly four hours obsolete keys remain, even when the plan completes again.
		at := completedAt.Add(4 * time.Hour)
		complete := func(at time.Time) {
			t.Helper()
			lease := store.AcquireStormScan("alpha", "world-one", stormKingdomID, at.Add(-3*time.Second))
			if len(lease.Windows) == 0 {
				lease = store.AcquireStormScan("alpha", "world-one", stormKingdomID, at)
			}
			if len(lease.Windows) == 0 {
				t.Fatal("no windows leased")
			}
			if _, err := store.CompleteStormScan("alpha", "world-one", stormKingdomID, lease.LeaseID, lease.Windows, at, at); err != nil {
				t.Fatal(err)
			}
		}
		complete(at)
		for _, key := range oldKeys {
			if _, ok := store.Snapshot("world-one").stormWindows[stormKingdomID][key]; !ok {
				t.Fatal("boundary key pruned")
			}
		}
		complete(at.Add(2*time.Hour + time.Millisecond))
		for _, key := range oldKeys {
			if _, ok := store.Snapshot("world-one").stormWindows[stormKingdomID][key]; ok {
				t.Fatal("expired obsolete key retained")
			}
			if _, ok := generation.stormWindows[stormKingdomID][key]; !ok {
				t.Fatal("prune changed prior immutable generation")
			}
		}
		if err := store.flushPersistence(t.Context()); err != nil {
			t.Fatal(err)
		}
		reopened, err := OpenWorldMapStore(directory)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = reopened.Close(context.Background()) }()
		windows := reopened.Snapshot("world-one").stormWindows[stormKingdomID]
		for _, key := range oldKeys {
			if _, ok := windows[key]; ok {
				t.Fatalf("obsolete key %s restored from SQLite", key)
			}
		}
		for key := range planKeys {
			if _, ok := windows[key]; !ok {
				t.Fatalf("plan key %s missing from SQLite", key)
			}
		}
	})
	t.Run("plan keys and recent non-plan keys survive partial completion", func(t *testing.T) {
		store := NewWorldMapStore()
		now := time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC)
		plan := stormScanPlanForBounds(initialStormScanBounds())
		windows := map[string]StormScanWindowState{}
		for _, bounds := range plan.Windows {
			windows[stormScanWindowKey(bounds)] = StormScanWindowState{Bounds: bounds, CompletedAt: now.Add(-5 * time.Hour)}
		}
		old := StormMapBounds{X1: 1, Y1: 1, X2: 101, Y2: 101}
		recent := StormMapBounds{X1: 2, Y1: 2, X2: 102, Y2: 102}
		boundary := StormMapBounds{X1: 3, Y1: 3, X2: 103, Y2: 103}
		windows[stormScanWindowKey(old)] = StormScanWindowState{Bounds: old, CompletedAt: now.Add(-5 * time.Hour)}
		windows[stormScanWindowKey(recent)] = StormScanWindowState{Bounds: recent, CompletedAt: now.Add(-time.Hour)}
		windows[stormScanWindowKey(boundary)] = StormScanWindowState{Bounds: boundary, CompletedAt: now.Add(-4 * time.Hour)}
		store.worlds["world-one"] = &worldMapGeneration{values: worldFactMap{}, stormPlans: map[KingdomID]stormScanPlan{stormKingdomID: plan}, stormWindows: map[KingdomID]map[string]StormScanWindowState{stormKingdomID: windows}}
		store.AcquireStormScan("alpha", "world-one", stormKingdomID, now.Add(-3*time.Second))
		store.AcquireStormScan("bravo", "world-one", stormKingdomID, now.Add(-3*time.Second))
		lease := store.AcquireStormScan("alpha", "world-one", stormKingdomID, now)
		if len(lease.Windows) >= len(plan.Windows) {
			t.Fatal("fixture did not lease a partial plan")
		}
		if _, err := store.CompleteStormScan("alpha", "world-one", stormKingdomID, lease.LeaseID, lease.Windows, now, now); err != nil {
			t.Fatal(err)
		}
		next := store.Snapshot("world-one").stormWindows[stormKingdomID]
		if _, ok := next[stormScanWindowKey(old)]; ok {
			t.Fatal("obsolete old window survived")
		}
		for _, bounds := range append(append([]StormMapBounds{}, plan.Windows...), recent, boundary) {
			if _, ok := next[stormScanWindowKey(bounds)]; !ok {
				t.Fatalf("kept window %v pruned", bounds)
			}
		}
		if _, ok := windows[stormScanWindowKey(old)]; !ok {
			t.Fatal("prune mutated prior generation")
		}
	})
	t.Run("failed flush requeues deletes and newer entry wins", func(t *testing.T) {
		store, err := OpenWorldMapStore(t.TempDir())
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = store.Close(context.Background()) }()
		bounds := StormMapBounds{X1: 1, Y1: 1, X2: 101, Y2: 101}
		key := stormScanWindowKey(bounds)
		now := time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC)
		store.queueStormScanPersistence("world-one", stormKingdomID, []StormMapBounds{bounds}, now)
		if err := store.flushPersistence(t.Context()); err != nil {
			t.Fatal(err)
		}
		store.queueStormScanDeletes("world-one", stormKingdomID, []string{key})
		ctx, cancel := context.WithCancel(t.Context())
		cancel()
		if err := store.flushPersistence(ctx); err == nil {
			t.Fatal("cancelled flush succeeded")
		}
		pendingKey := persistedWorldMapKey("world-one", stormKingdomID, "scan:"+key)
		if pending, ok := store.dirtyStormScans[pendingKey]; !ok || !pending.Deleted {
			t.Fatal("failed flush lost delete")
		}
		if err := store.flushPersistence(t.Context()); err != nil {
			t.Fatal(err)
		}
		var count int
		if err := store.db.QueryRow(`SELECT COUNT(*) FROM world_storm_scan_windows WHERE world_id=? AND kingdom_id=? AND window_key=?`, "world-one", stormKingdomID, key).Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count != 0 {
			t.Fatal("requeued delete did not reach SQLite")
		}
		store.queueStormScanDeletes("world-one", stormKingdomID, []string{key})
		store.queueStormScanPersistence("world-one", stormKingdomID, []StormMapBounds{bounds}, now.Add(time.Hour))
		if err := store.flushPersistence(t.Context()); err != nil {
			t.Fatal(err)
		}
		var at int64
		if err := store.db.QueryRow(`SELECT completed_at_ms FROM world_storm_scan_windows WHERE world_id=? AND kingdom_id=? AND window_key=?`, "world-one", stormKingdomID, key).Scan(&at); err != nil {
			t.Fatal(err)
		}
		if at != now.Add(time.Hour).UnixMilli() {
			t.Fatal("old delete overwrote newer entry")
		}
	})
}
