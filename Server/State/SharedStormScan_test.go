package State

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestSharedStormScanPartitionsAndReassignsAnonymousWork(t *testing.T) {
	worlds := NewWorldMapStore()
	now := time.Date(2026, time.August, 13, 12, 0, 0, 0, time.UTC)
	windows := sharedStormCandidateWindows(nil, stormKingdomID)

	if assignment := worlds.AcquireStormScan("alpha", "world-one", stormKingdomID, now); len(assignment.Windows) != 0 {
		t.Fatal("first participant scanned before the roster settle window")
	}
	if assignment := worlds.AcquireStormScan("bravo", "world-one", stormKingdomID, now.Add(500*time.Millisecond)); len(assignment.Windows) != 0 {
		t.Fatal("second participant scanned before the roster settle window")
	}
	leaseAt := now.Add(3 * time.Second)
	alpha := worlds.AcquireStormScan("alpha", "world-one", stormKingdomID, leaseAt)
	bravo := worlds.AcquireStormScan("bravo", "world-one", stormKingdomID, leaseAt)
	if alpha.ParticipantCount != 2 || bravo.ParticipantCount != 2 || len(alpha.Windows) != 13 || len(bravo.Windows) != 12 {
		t.Fatalf("partition sizes: alpha=%+v bravo=%+v", alpha, bravo)
	}
	seen := map[string]string{}
	for owner, assignment := range map[string]StormScanAssignment{"alpha": alpha, "bravo": bravo} {
		for _, window := range assignment.Windows {
			key := stormScanWindowKey(window)
			if prior := seen[key]; prior != "" {
				t.Fatalf("window %s leased to both %s and %s", key, prior, owner)
			}
			seen[key] = owner
		}
	}
	if len(seen) != len(windows) {
		t.Fatalf("leased union = %d windows, want %d", len(seen), len(windows))
	}
	completedAt := leaseAt.Add(time.Second)
	if _, err := worlds.CompleteStormScan("alpha", "world-one", stormKingdomID, alpha.LeaseID, alpha.Windows, leaseAt, completedAt); err != nil {
		t.Fatal(err)
	}
	partial := stormScanCoverage(worlds.Snapshot("world-one"), stormKingdomID, windows, 2, completedAt)
	if partial.Complete() || partial.FreshWindowCount != len(alpha.Windows) || partial.WindowCount != len(windows) {
		t.Fatalf("partial completion was reported as full coverage: %+v", partial)
	}
	if _, err := worlds.CompleteStormScan("bravo", "world-one", stormKingdomID, bravo.LeaseID, bravo.Windows, leaseAt, completedAt); err != nil {
		t.Fatal(err)
	}
	coverage := stormScanCoverage(worlds.Snapshot("world-one"), stormKingdomID, windows, 2, completedAt)
	if !coverage.Complete() || coverage.FreshWindowCount != len(windows) {
		t.Fatalf("completed coverage = %+v", coverage)
	}
	if assignment := worlds.AcquireStormScan("alpha", "world-one", stormKingdomID, completedAt.Add(time.Minute)); len(assignment.Windows) != 0 {
		t.Fatal("fresh windows were leased again")
	}

	worlds.UnregisterStormScanner("bravo")
	dueAt := completedAt.Add(sharedStormScanRefreshInterval + time.Second)
	if assignment := worlds.AcquireStormScan("alpha", "world-one", stormKingdomID, dueAt); len(assignment.Windows) != 0 {
		t.Fatal("changed roster did not settle before reassignment")
	}
	reassigned := worlds.AcquireStormScan(
		"alpha", "world-one", stormKingdomID, dueAt.Add(sharedStormRosterSettleDelay+time.Millisecond),
	)
	if reassigned.ParticipantCount != 1 || len(reassigned.Windows) != len(windows) {
		t.Fatalf("reassigned coverage = %+v", reassigned)
	}
}

func TestSharedStormScanParticipantsNeverCrossWorlds(t *testing.T) {
	worlds := NewWorldMapStore()
	now := time.Date(2026, time.August, 13, 12, 0, 0, 0, time.UTC)
	worlds.AcquireStormScan("alpha", "world-one", stormKingdomID, now)
	worlds.AcquireStormScan("bravo", "world-two", stormKingdomID, now)
	alpha := worlds.AcquireStormScan(
		"alpha", "world-one", stormKingdomID, now.Add(sharedStormRosterSettleDelay+time.Millisecond),
	)
	bravo := worlds.AcquireStormScan(
		"bravo", "world-two", stormKingdomID, now.Add(sharedStormRosterSettleDelay+time.Millisecond),
	)
	if alpha.ParticipantCount != 1 || bravo.ParticipantCount != 1 || len(alpha.Windows) != 25 || len(bravo.Windows) != 25 {
		t.Fatalf("cross-world participants were combined: alpha=%+v bravo=%+v", alpha, bravo)
	}
}

func TestSharedStormCoverageReadsCachedGeometryWithoutAllocating(t *testing.T) {
	worlds := NewWorldMapStore()
	observedAt := time.Date(2026, time.August, 13, 12, 0, 0, 0, time.UTC)
	storm := MapObservation{
		KingdomID: stormKingdomID, X: 400, Y: 650, TypeID: MapTypeStormFort,
		StormIsleID: 10, ObservedAt: observedAt,
	}
	event := worlds.commit("world-one", nil, []MapChange{{
		KingdomID: stormKingdomID, Key: "400:650", Observation: &storm,
	}})
	plan, cached := event.generation.stormPlans[stormKingdomID]
	if !cached || len(plan.Windows) != 30 {
		t.Fatalf("cached Storm plan = %+v, found=%t", plan, cached)
	}
	state := NewGameState()
	state.Castles[1] = CastleState{ID: 1, KingdomID: stormKingdomID}
	state.sharedMap = event.generation
	state.worldSharing = true
	var coverage StormScanCoverage
	allocations := testing.AllocsPerRun(100, func() {
		coverage = state.SharedStormScanCoverage(stormKingdomID, observedAt)
	})
	if allocations != 0 {
		t.Fatalf("cached coverage allocated %.2f objects per read", allocations)
	}
	if coverage.WindowCount != len(plan.Windows) {
		t.Fatalf("cached coverage = %+v, want %d windows", coverage, len(plan.Windows))
	}
}

func TestSharedStormCompletionPublishesOneActionableDomainWithoutMapChanges(t *testing.T) {
	worlds := NewWorldMapStore()
	initial := NewGameState()
	initial.Account.WorldID = "world-one"
	initial.Castles[1] = CastleState{ID: 1, KingdomID: stormKingdomID}
	observer := NewStoreWithWorldMap(initial, worlds)
	now := time.Date(2026, time.August, 13, 12, 0, 0, 0, time.UTC)
	worlds.AcquireStormScan("alpha", "world-one", stormKingdomID, now)
	leaseAt := now.Add(sharedStormRosterSettleDelay + time.Millisecond)
	lease := worlds.AcquireStormScan("alpha", "world-one", stormKingdomID, leaseAt)
	if len(lease.Windows) != 25 {
		t.Fatalf("single-account lease = %+v", lease)
	}
	worldEvent, err := worlds.CompleteStormScan(
		"alpha", "world-one", stormKingdomID, lease.LeaseID, lease.Windows,
		leaseAt, leaseAt.Add(time.Second),
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(worldEvent.Changes) != 0 {
		t.Fatalf("empty-world completion produced map changes: %+v", worldEvent.Changes)
	}
	event, changed := observer.AdoptWorldMap(worldEvent)
	if !changed || len(event.Domains) != 1 || event.Domains[0] != "storm-scan" {
		t.Fatalf("completion adoption = changed=%t event=%+v", changed, event)
	}
	if len(event.Components) != 0 {
		t.Fatalf("coverage-only completion dirtied account components: %v", event.Components)
	}
}

func TestSharedStormScanPrunesOnlyOmittedRowsInsideCompletedLease(t *testing.T) {
	worlds := NewWorldMapStore()
	worldID := "world-one"
	startedAt := time.Date(2026, time.August, 13, 12, 0, 0, 0, time.UTC)
	insideStale := MapObservation{
		KingdomID: stormKingdomID, X: 610, Y: 610, TypeID: MapTypeStormFort,
		StormIsleID: 7, ObservedAt: startedAt.Add(-time.Minute),
	}
	insideFresh := MapObservation{
		KingdomID: stormKingdomID, X: 620, Y: 620, TypeID: MapTypeStormFort,
		StormIsleID: 8, ObservedAt: startedAt.Add(time.Second),
	}
	outside := MapObservation{
		KingdomID: stormKingdomID, X: 750, Y: 650, TypeID: MapTypeStormFort,
		StormIsleID: 9, ObservedAt: startedAt.Add(-time.Minute),
	}
	worlds.commit(worldID, nil, []MapChange{
		{KingdomID: stormKingdomID, Key: "610:610", Observation: &insideStale},
		{KingdomID: stormKingdomID, Key: "620:620", Observation: &insideFresh},
		{KingdomID: stormKingdomID, Key: "750:650", Observation: &outside},
	})
	worlds.AcquireStormScan("alpha", worldID, stormKingdomID, startedAt)
	worlds.AcquireStormScan("bravo", worldID, stormKingdomID, startedAt)
	lease := worlds.AcquireStormScan(
		"alpha", worldID, stormKingdomID, startedAt.Add(sharedStormRosterSettleDelay+time.Millisecond),
	)
	if len(lease.Windows) != 13 {
		t.Fatalf("lease = %+v", lease)
	}
	event, err := worlds.CompleteStormScan(
		"alpha", worldID, stormKingdomID, lease.LeaseID, lease.Windows, startedAt, startedAt.Add(2*time.Second),
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(event.Changes) != 1 || event.Changes[0].Key != "610:610" || !event.Changes[0].Deleted {
		t.Fatalf("authoritative omissions = %+v", event.Changes)
	}
	generation := worlds.Snapshot(worldID)
	if _, found := lookupWorldFact(generation.values, stormKingdomID, "610:610"); found {
		t.Fatal("omitted stale row survived completed lease")
	}
	if _, found := lookupWorldFact(generation.values, stormKingdomID, "620:620"); !found {
		t.Fatal("fresh row inside lease was pruned")
	}
	if _, found := lookupWorldFact(generation.values, stormKingdomID, "750:650"); !found {
		t.Fatal("row outside lease was pruned")
	}
}

func TestSharedStormScanCoveragePersistsWithoutParticipantIdentity(t *testing.T) {
	directory := t.TempDir()
	worlds, err := OpenWorldMapStore(directory)
	if err != nil {
		t.Fatal(err)
	}
	worlds.StartPersistence()
	now := time.Date(2026, time.August, 13, 12, 0, 0, 0, time.UTC)
	windows := sharedStormCandidateWindows(nil, stormKingdomID)
	worlds.AcquireStormScan("private-account-key", "world-one", stormKingdomID, now)
	lease := worlds.AcquireStormScan(
		"private-account-key", "world-one", stormKingdomID,
		now.Add(sharedStormRosterSettleDelay+time.Millisecond),
	)
	event, err := worlds.CompleteStormScan(
		"private-account-key", "world-one", stormKingdomID, lease.LeaseID, lease.Windows,
		now, now.Add(3*time.Second),
	)
	if err != nil {
		t.Fatal(err)
	}
	encodedEvent, err := json.Marshal(event)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(encodedEvent, []byte("private-account-key")) {
		t.Fatal("private participant identity leaked into the shared event")
	}
	if err := worlds.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(filepath.Join(directory, "Shared", "State"))
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		raw, readErr := os.ReadFile(filepath.Join(directory, "Shared", "State", entry.Name()))
		if readErr != nil {
			t.Fatal(readErr)
		}
		if bytes.Contains(raw, []byte("private-account-key")) {
			t.Fatalf("private participant identity leaked into %s", entry.Name())
		}
	}
	reopened, err := OpenWorldMapStore(directory)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = reopened.Close(context.Background()) }()
	coverage := stormScanCoverage(
		reopened.Snapshot("world-one"), stormKingdomID, windows, 0, now.Add(time.Minute),
	)
	if !coverage.Complete() || coverage.ParticipantCount != 0 {
		t.Fatalf("persisted anonymous coverage = %+v", coverage)
	}
	if len(reopened.stormParticipants) != 0 || len(reopened.stormLeases) != 0 {
		t.Fatal("private participant identity was restored into shared state")
	}
}
