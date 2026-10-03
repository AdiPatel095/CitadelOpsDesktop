package State

import (
	"testing"
	"time"
)

func TestStormPackageCapResetProof(t *testing.T) {
	now := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	for _, tc := range []struct {
		name   string
		castle CastleID
		table  int64
		offers map[PackageID]int64
		at     time.Time
		want   bool
	}{
		{"omission", 910040, -1, map[PackageID]int64{}, now.Add(time.Second), true},
		{"still capped", 910040, -1, map[PackageID]int64{910126: 4}, now.Add(time.Second), true},
		{"older below cap", 910040, -1, map[PackageID]int64{910126: 0}, now.Add(-time.Second), true},
		{"same time below cap", 910040, -1, map[PackageID]int64{910126: 0}, now, true},
		{"explicit below cap", 910040, -1, map[PackageID]int64{910126: 3}, now.Add(time.Second), false},
		{"different castle history", 910041, -1, map[PackageID]int64{910126: 0}, now.Add(time.Second), true},
		{"different table history", 910040, 910042, map[PackageID]int64{910126: 0}, now.Add(time.Second), true},
		{"seven day ceiling", 910040, -1, map[PackageID]int64{}, now.Add(7 * 24 * time.Hour), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			state := NewGameState()
			state.BlockStormPackage(910040, -1, 910126, 4, now)
			state.ReconcileStormPackageCaps(tc.castle, tc.table, tc.offers, tc.at)
			if got := state.StormPackageBlocked(910040, -1, 910126, now.Add(time.Second)); got != tc.want {
				t.Fatalf("blocked=%v want %v", got, tc.want)
			}
		})
	}
	state := NewGameState()
	state.BlockStormPackage(910040, -1, 910126, 4, now)
	for _, scope := range []struct {
		castle    CastleID
		table     int64
		packageID PackageID
	}{{910041, -1, 910126}, {910040, 910042, 910126}, {910040, -1, 910127}} {
		if state.StormPackageBlocked(scope.castle, scope.table, scope.packageID, now) {
			t.Fatal("block escaped occurrence/package scope")
		}
	}
	if state.StormPackageBlocked(910040, -1, 910126, now.Add(7*24*time.Hour)) {
		t.Fatal("block is permanent")
	}
}

func TestStormPackageCapCopyOnWriteAndSnapshot(t *testing.T) {
	now := time.Now().UTC()
	initial := NewGameState()
	initial.BlockStormPackage(910040, -1, 910126, 4, now)
	store := NewStore(&initial)
	old := store.ReadOnlyView()
	snapshot := store.Snapshot()
	for key := range snapshot.Storm.PackageCapBlocks {
		delete(snapshot.Storm.PackageCapBlocks, key)
	}
	current := store.ReadOnlyView()
	if !current.StormPackageBlocked(910040, -1, 910126, now) {
		t.Fatal("snapshot mutated store")
	}
	_, err := store.ApplyComponents(Components(ComponentStorm), func(next *GameState) ([]string, bool, error) {
		changed := next.ReconcileStormPackageCaps(910040, -1, map[PackageID]int64{910126: 0}, now.Add(time.Second))
		return []string{"storm"}, changed, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if !old.StormPackageBlocked(910040, -1, 910126, now) {
		t.Fatal("mutation changed prior immutable generation")
	}
	current = store.ReadOnlyView()
	if current.StormPackageBlocked(910040, -1, 910126, now) {
		t.Fatal("new generation retained reset block")
	}
}

func TestStormPackageCapSurvivesComponentSnapshot(t *testing.T) {
	directory := t.TempDir()
	now := time.Now().UTC()
	initial := NewGameState()
	store := NewStore(&initial)
	event, err := store.ApplyComponents(Components(ComponentStorm), func(state *GameState) ([]string, bool, error) {
		state.BlockStormPackage(910040, -1, 910126, 4, now)
		return []string{"storm"}, true, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := SaveComponentSnapshot(directory, event, Components(ComponentStorm)); err != nil {
		t.Fatal(err)
	}
	restored, err := LoadSnapshot(directory)
	if err != nil {
		t.Fatal(err)
	}
	if !restored.StormPackageBlocked(910040, -1, 910126, now) {
		t.Fatal("snapshot/restart lifted cap block")
	}
}
