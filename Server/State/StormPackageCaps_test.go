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

func TestStormPackageCapExpiresAtProvenKRSEventEnd(t *testing.T) {
	now := time.Date(2026, 10, 3, 18, 0, 0, 0, time.UTC)
	for _, duration := range []time.Duration{time.Hour, 30 * 24 * time.Hour} {
		t.Run(duration.String(), func(t *testing.T) {
			state := NewGameState()
			state.Session.ConnectionGeneration = 9
			state.Session.ChangedAt = now.Add(-time.Hour)
			end := now.Add(duration)
			state.KingdomTransport.Unlocks[4] = KingdomTransportUnlock{KingdomID: 4, Unlocked: true, EventEndsAt: end, EventEndObservedAt: now.Add(-time.Minute), EventEndConnectionGeneration: 9}
			state.BlockStormPackage(910040, -1, 910126, 4, now)
			block := state.Storm.PackageCapBlocks[stormPackageCapKey(910040, -1, 910126)]
			if !block.EventEndsAt.Equal(end) || !block.ExpiresAt.Equal(end) {
				t.Fatalf("cap expiry=%v, want proven end %v", block.ExpiresAt, end)
			}
			if !state.StormPackageBlocked(910040, -1, 910126, end.Add(-time.Nanosecond)) {
				t.Fatal("block lifted before event end")
			}
			if state.StormPackageBlocked(910040, -1, 910126, end) || state.StormPackageBlocked(910040, -1, 910126, end.Add(time.Second)) {
				t.Fatal("block outlived event end")
			}
			// Missing countdowns or a reconnect cannot extend a captured deadline.
			state.KingdomTransport.Unlocks[4] = KingdomTransportUnlock{KingdomID: 4}
			state.Session.ConnectionGeneration++
			state.ReconcileStormPackageCapEventEnd(now.Add(time.Minute))
			if !state.Storm.PackageCapBlocks[stormPackageCapKey(910040, -1, 910126)].ExpiresAt.Equal(end) {
				t.Fatal("lost deadline extended cap block")
			}
			state.ReconcileStormPackageCapEventEnd(end)
			if len(state.Storm.PackageCapBlocks) != 0 {
				t.Fatal("expired block retained")
			}
		})
	}
}

func TestStormPackageCapUnknownOrStaleEndUsesBoundedFallback(t *testing.T) {
	now := time.Date(2026, 10, 3, 18, 0, 0, 0, time.UTC)
	for _, tc := range []struct {
		name   string
		change func(*GameState)
	}{
		{"missing", func(state *GameState) { delete(state.KingdomTransport.Unlocks, 4) }},
		{"wrong connection", func(state *GameState) { state.Session.ConnectionGeneration++ }},
		{"before session", func(state *GameState) { state.Session.ChangedAt = now }},
		{"future observation", func(state *GameState) {
			row := state.KingdomTransport.Unlocks[4]
			row.EventEndObservedAt = now.Add(time.Second)
			state.KingdomTransport.Unlocks[4] = row
		}},
		{"closed", func(state *GameState) {
			row := state.KingdomTransport.Unlocks[4]
			row.Unlocked = false
			state.KingdomTransport.Unlocks[4] = row
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			state := NewGameState()
			state.Session.ConnectionGeneration = 9
			state.Session.ChangedAt = now.Add(-time.Hour)
			state.KingdomTransport.Unlocks[4] = KingdomTransportUnlock{KingdomID: 4, Unlocked: true, EventEndsAt: now.Add(time.Hour), EventEndObservedAt: now.Add(-time.Minute), EventEndConnectionGeneration: 9}
			tc.change(&state)
			state.BlockStormPackage(910040, -1, 910126, 4, now)
			block := state.Storm.PackageCapBlocks[stormPackageCapKey(910040, -1, 910126)]
			if !block.EventEndsAt.IsZero() || !block.ExpiresAt.Equal(now.Add(7*24*time.Hour)) {
				t.Fatal("unknown/stale event end authorized expiry")
			}
		})
	}
}
