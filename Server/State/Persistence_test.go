package State

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestComponentSnapshotWritesOnlyDirtyComponentsAfterBootstrap(t *testing.T) {
	directory := t.TempDir()
	initial := NewGameState()
	initial.Player.ID = 99
	initial.Player.Level = 10
	store := NewStore(initial)

	playerEvent, err := store.ApplyComponents(Components(ComponentPlayer), func(state *GameState) ([]string, bool, error) {
		state.Player.Level = 11
		return []string{"player"}, true, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := SaveComponentSnapshot(directory, playerEvent, Components(playerEvent.Components...)); err != nil {
		t.Fatal(err)
	}
	first, err := readComponentManifest(directory)
	if err != nil {
		t.Fatal(err)
	}
	if len(first.Files) != len(AllComponents.List()) {
		t.Fatalf("bootstrap files = %d, want %d", len(first.Files), len(AllComponents.List()))
	}

	sessionEvent, err := store.ApplyComponents(Components(ComponentSession), func(state *GameState) ([]string, bool, error) {
		state.Session.Generation = 7
		state.Session.ServerURL = "wss://example.test/socket"
		return []string{"session"}, true, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := SaveComponentSnapshot(directory, sessionEvent, Components(sessionEvent.Components...)); err != nil {
		t.Fatal(err)
	}
	second, err := readComponentManifest(directory)
	if err != nil {
		t.Fatal(err)
	}
	for _, component := range AllComponents.List() {
		name := component.String()
		if component == ComponentSession {
			if first.Files[name] == second.Files[name] {
				t.Fatalf("dirty %s component file did not advance", name)
			}
			continue
		}
		if first.Files[name] != second.Files[name] {
			t.Fatalf("clean %s component file changed from %q to %q", name, first.Files[name], second.Files[name])
		}
	}
	if _, err := os.Stat(filepath.Join(componentStatePath(directory), first.Files[ComponentSession.String()])); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("superseded session component was not removed: %v", err)
	}
	manifestInfo, err := os.Stat(filepath.Join(componentStatePath(directory), componentManifestName))
	if err != nil {
		t.Fatal(err)
	}
	if manifestInfo.Mode().Perm() != 0o600 {
		t.Fatalf("component manifest permissions = %o", manifestInfo.Mode().Perm())
	}

	loaded, err := LoadSnapshot(directory)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Revision != sessionEvent.Revision || loaded.Player.Level != 11 {
		t.Fatalf("loaded component state = revision %d player %+v", loaded.Revision, loaded.Player)
	}
	if loaded.Session.Generation != 7 || loaded.Session.ServerURL != "wss://example.test/socket" || loaded.Session.Status != "stopped" {
		t.Fatalf("loaded session migration = %+v", loaded.Session)
	}
}

func TestComponentSnapshotWriterReusesLastDurableManifest(t *testing.T) {
	directory := t.TempDir()
	store := NewStore(NewGameState())
	writer := NewComponentSnapshotWriter(directory)
	first, err := store.ApplyComponents(Components(ComponentPlayer), func(state *GameState) ([]string, bool, error) {
		state.Player.Level = 10
		return []string{"player"}, true, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := writer.Save(first, Components(first.Components...)); err != nil {
		t.Fatal(err)
	}
	if writer.current == nil || writer.current.Revision != first.Revision {
		t.Fatalf("cached manifest after bootstrap = %#v", writer.current)
	}
	// A single account process owns this directory. Corrupting the on-disk copy
	// here proves the second save uses the last known-durable in-memory manifest
	// rather than rereading and decoding it.
	manifestPath := filepath.Join(componentStatePath(directory), componentManifestName)
	if err := os.WriteFile(manifestPath, []byte("not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	second, err := store.ApplyComponents(Components(ComponentSession), func(state *GameState) ([]string, bool, error) {
		state.Session.Generation = 2
		return []string{"session"}, true, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := writer.Save(second, Components(second.Components...)); err != nil {
		t.Fatalf("cached-manifest save: %v", err)
	}
	loaded, err := LoadSnapshot(directory)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Revision != second.Revision || loaded.Player.Level != 10 || loaded.Session.Generation != 2 {
		t.Fatalf("cached-manifest round trip = revision %d player %+v session %+v", loaded.Revision, loaded.Player, loaded.Session)
	}
}

func TestComponentSnapshotPersistsOnlyDirtyCastleAndInventoryPartitions(t *testing.T) {
	directory := t.TempDir()
	initial := NewGameState()
	initial.Castles[11] = CastleState{ID: 11, Name: "one", Resources: map[ResourceID]ResourceBalance{1: {Amount: 10}}}
	initial.Castles[22] = CastleState{ID: 22, Name: "two", Resources: map[ResourceID]ResourceBalance{1: {Amount: 20}}}
	initial.Inventory.ConstructionItems[101] = 3
	initial.Inventory.Equipment[501] = EquipmentInstance{ID: 501, Level: 1, Effects: EquipmentEffects{}}
	store := NewStore(initial)

	bootstrap, err := store.ApplyComponents(Components(ComponentPlayer), func(state *GameState) ([]string, bool, error) {
		state.Player.Level++
		return []string{"player"}, true, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := SaveComponentSnapshot(directory, bootstrap, Components(bootstrap.Components...)); err != nil {
		t.Fatal(err)
	}
	first, err := readComponentManifest(directory)
	if err != nil {
		t.Fatal(err)
	}
	if !first.Partitioned[ComponentCastles.String()] || !first.Partitioned[ComponentInventory.String()] ||
		len(first.CastleFiles) != 2 || len(first.InventoryFiles) != len(inventoryPersistenceParts) {
		t.Fatalf("partitioned bootstrap manifest = %#v", first)
	}

	castleEvent, err := store.ApplyComponents(Components(ComponentCastles), func(state *GameState) ([]string, bool, error) {
		castle, found := state.MutableCastle(11)
		if !found {
			return nil, false, errors.New("castle 11 missing")
		}
		balance := castle.Resources[1]
		balance.Amount = 15
		castle.Resources[1] = balance
		state.SetCastle(11, castle)
		return []string{"castles", "resources"}, true, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := SaveComponentSnapshot(directory, castleEvent, Components(castleEvent.Components...)); err != nil {
		t.Fatal(err)
	}
	second, err := readComponentManifest(directory)
	if err != nil {
		t.Fatal(err)
	}
	if first.CastleFiles["11"] == second.CastleFiles["11"] {
		t.Fatal("dirty castle shard did not advance")
	}
	if first.CastleFiles["22"] != second.CastleFiles["22"] {
		t.Fatal("clean castle shard was rewritten")
	}
	for part, filename := range first.InventoryFiles {
		if second.InventoryFiles[part] != filename {
			t.Fatalf("castle save rewrote inventory part %s", part)
		}
	}

	inventoryEvent, err := store.ApplyComponents(Components(ComponentInventory), func(state *GameState) ([]string, bool, error) {
		state.MutableInventoryConstructionItems()[101] = 2
		return []string{"inventory", "construction-items"}, true, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := SaveComponentSnapshot(directory, inventoryEvent, Components(inventoryEvent.Components...)); err != nil {
		t.Fatal(err)
	}
	third, err := readComponentManifest(directory)
	if err != nil {
		t.Fatal(err)
	}
	if second.InventoryFiles["construction-items"] == third.InventoryFiles["construction-items"] {
		t.Fatal("dirty construction inventory part did not advance")
	}
	for part, filename := range second.InventoryFiles {
		if part != "construction-items" && third.InventoryFiles[part] != filename {
			t.Fatalf("clean inventory part %s was rewritten", part)
		}
	}
	if second.CastleFiles["11"] != third.CastleFiles["11"] || second.CastleFiles["22"] != third.CastleFiles["22"] {
		t.Fatal("inventory save rewrote castle shards")
	}

	loaded, err := LoadSnapshot(directory)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Castles[11].Resources[1].Amount != 15 || loaded.Castles[22].Resources[1].Amount != 20 ||
		loaded.Inventory.ConstructionItems[101] != 2 || loaded.Inventory.Equipment[501].Level != 1 {
		t.Fatalf("partitioned snapshot round trip = castles %#v inventory %#v", loaded.Castles, loaded.Inventory)
	}
}

func TestComponentSnapshotPersistsOnlyDirtyMapShard(t *testing.T) {
	directory := t.TempDir()
	initial := NewGameState()
	observedAt := time.Now().UTC()
	firstKey := "10:10"
	secondX := 11
	secondKey := fmt.Sprintf("%d:10", secondX)
	for mapShardIndex(secondKey) == mapShardIndex(firstKey) {
		secondX++
		secondKey = fmt.Sprintf("%d:10", secondX)
	}
	initial.Map[0] = map[string]MapObservation{
		firstKey:  {KingdomID: 0, X: 10, Y: 10, TypeID: MapTypeKingdomTower, Level: 1, ObservedAt: observedAt},
		secondKey: {KingdomID: 0, X: secondX, Y: 10, TypeID: MapTypeKingdomTower, Level: 2, ObservedAt: observedAt},
	}
	store := NewStore(initial)
	bootstrap, err := store.ApplyComponents(Components(ComponentPlayer), func(state *GameState) ([]string, bool, error) {
		state.Player.Level++
		return []string{"player"}, true, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := SaveComponentSnapshot(directory, bootstrap, Components(bootstrap.Components...)); err != nil {
		t.Fatal(err)
	}
	first, err := readComponentManifest(directory)
	if err != nil {
		t.Fatal(err)
	}
	if !first.Partitioned[ComponentWorldMap.String()] || len(first.MapFiles) != 2 {
		t.Fatalf("partitioned map bootstrap = %#v", first)
	}
	firstShard := mapPersistenceShardKey(0, mapShardIndex(firstKey))
	secondShard := mapPersistenceShardKey(0, mapShardIndex(secondKey))

	mapEvent, err := store.ApplyComponents(Components(ComponentWorldMap), func(state *GameState) ([]string, bool, error) {
		changed := state.SetMapObservation(MapObservation{
			KingdomID: 0, X: 10, Y: 10, TypeID: MapTypeKingdomTower, Level: 3, ObservedAt: observedAt.Add(time.Minute),
		})
		return []string{"map"}, changed, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := SaveComponentSnapshot(directory, mapEvent, Components(mapEvent.Components...)); err != nil {
		t.Fatal(err)
	}
	second, err := readComponentManifest(directory)
	if err != nil {
		t.Fatal(err)
	}
	if first.MapFiles[firstShard] == second.MapFiles[firstShard] {
		t.Fatal("dirty map shard did not advance")
	}
	if first.MapFiles[secondShard] != second.MapFiles[secondShard] {
		t.Fatal("clean map shard was rewritten")
	}
	loaded, err := LoadSnapshot(directory)
	if err != nil {
		t.Fatal(err)
	}
	if observation, found := loaded.LookupMapObservation(0, firstKey); !found || observation.Level != 3 {
		t.Fatalf("dirty map observation = %+v, found %v", observation, found)
	}
	if observation, found := loaded.LookupMapObservation(0, secondKey); !found || observation.Level != 2 {
		t.Fatalf("clean map observation = %+v, found %v", observation, found)
	}
}

func TestPersistenceBatchKeepsMapShardKeysAcrossLaterRevisions(t *testing.T) {
	directory := t.TempDir()
	initial := NewGameState()
	observedAt := time.Date(2026, time.August, 13, 12, 0, 0, 0, time.UTC)
	first := MapObservation{
		KingdomID: 0, X: 10, Y: 10, TypeID: MapTypeKingdomTower, Level: 1, ObservedAt: observedAt,
	}
	second := MapObservation{
		KingdomID: 0, X: 11, Y: 10, TypeID: MapTypeKingdomTower, Level: 2, ObservedAt: observedAt,
	}
	firstKey := fmt.Sprintf("%d:%d", first.X, first.Y)
	secondKey := fmt.Sprintf("%d:%d", second.X, second.Y)
	for mapShardIndex(secondKey) == mapShardIndex(firstKey) {
		second.X++
		secondKey = fmt.Sprintf("%d:%d", second.X, second.Y)
	}
	initial.Map[0] = map[string]MapObservation{
		firstKey:  first,
		secondKey: second,
	}
	store := NewStore(initial)

	bootstrap, err := store.ApplyComponents(Components(ComponentPlayer), func(state *GameState) ([]string, bool, error) {
		state.Player.Level++
		return []string{"player"}, true, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := SaveComponentSnapshot(directory, bootstrap, Components(bootstrap.Components...)); err != nil {
		t.Fatal(err)
	}
	before, err := readComponentManifest(directory)
	if err != nil {
		t.Fatal(err)
	}
	firstShard := mapPersistenceShardKey(0, mapShardIndex(firstKey))
	secondShard := mapPersistenceShardKey(0, mapShardIndex(secondKey))

	first.Level = 3
	first.ObservedAt = observedAt.Add(time.Minute)
	mapEvent, err := store.ApplyComponents(Components(ComponentWorldMap), func(state *GameState) ([]string, bool, error) {
		return []string{"map"}, state.SetMapObservation(first), nil
	})
	if err != nil {
		t.Fatal(err)
	}
	playerEvent, err := store.ApplyComponents(Components(ComponentPlayer), func(state *GameState) ([]string, bool, error) {
		state.Player.Level++
		return []string{"player"}, true, nil
	})
	if err != nil {
		t.Fatal(err)
	}

	var batch PersistenceBatch
	if !batch.Accumulate(mapEvent) || !batch.Accumulate(playerEvent) {
		t.Fatal("durable state events were not accumulated")
	}
	if revision, err := batch.Flush(directory); err != nil {
		t.Fatal(err)
	} else if revision != playerEvent.Revision {
		t.Fatalf("persisted revision = %d, want %d", revision, playerEvent.Revision)
	}
	after, err := readComponentManifest(directory)
	if err != nil {
		t.Fatal(err)
	}
	if before.MapFiles[firstShard] == after.MapFiles[firstShard] {
		t.Fatal("dirty map shard did not advance with the grouped revision")
	}
	if before.MapFiles[secondShard] != after.MapFiles[secondShard] {
		t.Fatal("clean map shard was rewritten after a later non-map revision")
	}
	if batch.Revision() != 0 {
		t.Fatalf("successful persistence batch retained revision %d", batch.Revision())
	}
}

func TestPersistenceBatchSkipsConnectionOnlyMovementSnapshot(t *testing.T) {
	store := NewStore(NewGameState())
	event, err := store.ApplyComponents(Components(ComponentMovementSnapshot), func(state *GameState) ([]string, bool, error) {
		state.MovementSnapshot = MovementSnapshot{
			Version: 1, ConnectionGeneration: 4, ObservedAt: time.Now().UTC(),
		}
		return []string{"movement-snapshot"}, true, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	var batch PersistenceBatch
	if batch.Accumulate(event) || batch.Revision() != 0 {
		t.Fatalf("connection-only movement marker became durable: %+v", batch)
	}
}

func TestComponentSnapshotPersistsCompactStormSuppressionSeparately(t *testing.T) {
	directory := t.TempDir()
	initial := NewGameState()
	// Keep the persisted observation inside the three-day Storm retention
	// window; a fixed calendar date makes this round-trip test expire over time.
	observedAt := time.Now().UTC().Add(-time.Hour).Truncate(time.Second)
	target := MapObservation{
		KingdomID: stormKingdomID, X: 612, Y: 667, TypeID: MapTypeStormFort,
		StormIsleID: 7, ObservedAt: observedAt,
	}
	initial.Map[stormKingdomID] = map[string]MapObservation{"612:667": target}
	initial.Storm.Map.Targets["612:667"] = target
	store := NewStore(initial)

	bootstrap, err := store.ApplyComponents(Components(ComponentPlayer), func(state *GameState) ([]string, bool, error) {
		state.Player.Level++
		return []string{"player"}, true, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := SaveComponentSnapshot(directory, bootstrap, Components(bootstrap.Components...)); err != nil {
		t.Fatal(err)
	}
	first, err := readComponentManifest(directory)
	if err != nil {
		t.Fatal(err)
	}
	if !first.Partitioned[ComponentStorm.String()] || len(first.StormFiles) != 2 {
		t.Fatalf("partitioned Storm bootstrap = %#v", first)
	}
	firstTargets := first.StormFiles["targets"]

	metadataEvent, err := store.ApplyComponents(Components(ComponentStorm), func(state *GameState) ([]string, bool, error) {
		state.Storm.LunaShopPending = true
		return []string{"storm"}, true, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := SaveComponentSnapshot(directory, metadataEvent, Components(metadataEvent.Components...)); err != nil {
		t.Fatal(err)
	}
	second, err := readComponentManifest(directory)
	if err != nil {
		t.Fatal(err)
	}
	if second.StormFiles["targets"] != firstTargets {
		t.Fatal("metadata-only Storm revision rewrote target membership")
	}
	if second.StormFiles["metadata"] == first.StormFiles["metadata"] {
		t.Fatal("dirty Storm metadata did not advance")
	}

	loaded, err := LoadSnapshot(directory)
	if err != nil {
		t.Fatal(err)
	}
	loadedView := NewStore(loaded).ReadOnlyView()
	if tracked, found := loadedView.LookupStormTarget("612:667"); !found || tracked != target {
		t.Fatalf("migrated Storm target = %#v, found %t", tracked, found)
	}
	if !loaded.Storm.LunaShopPending {
		t.Fatal("persisted Storm metadata was not restored")
	}

	deleteEvent, err := store.ApplyComponents(Components(ComponentStorm), func(state *GameState) ([]string, bool, error) {
		return []string{"storm"}, state.DeleteStormTarget("612:667"), nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := SaveComponentSnapshot(directory, deleteEvent, Components(deleteEvent.Components...)); err != nil {
		t.Fatal(err)
	}
	third, err := readComponentManifest(directory)
	if err != nil {
		t.Fatal(err)
	}
	if third.StormFiles["targets"] == second.StormFiles["targets"] {
		t.Fatal("dirty Storm membership did not advance")
	}
	loaded, err = LoadSnapshot(directory)
	if err != nil {
		t.Fatal(err)
	}
	loadedView = NewStore(loaded).ReadOnlyView()
	if _, found := loadedView.LookupStormTarget("612:667"); found {
		t.Fatal("account-private Storm suppression was not restored")
	}
	if observation, found := loadedView.LookupMapObservation(stormKingdomID, "612:667"); !found || observation != target {
		t.Fatalf("private suppression removed shared-compatible map fact: %#v, found %t", observation, found)
	}
}

func TestSnapshotRoundTripResetsSession(t *testing.T) {
	directory := t.TempDir()
	state := NewGameState()
	state.Revision = 42
	state.Player.ID = 99
	state.Session.Status = "connected"
	state.Session.LoggedIn = true
	state.Session.SocketReady = true
	state.Session.Generation = 7
	state.Session.BaselineGeneration = 7
	state.Session.ConnectionGeneration = 3
	state.Session.ServerURL = "wss://ep-live-us1-game.example.test/socket"
	state.MovementSnapshot = MovementSnapshot{
		Version: 9, ConnectionGeneration: 3, ObservedAt: time.Now().UTC(),
	}
	state.Castles[7] = CastleState{ID: 7, Name: "Test", Resources: map[ResourceID]ResourceBalance{}}
	state.TowerQueue.CursorVersion = 0
	state.TowerQueue.ConfirmedLaunchesByCastle[7] = 12
	islandReturnKey := StormIslandReturnKey(4, 101, 102)
	state.Storm.IslandReturns[islandReturnKey] = StormIslandReturnState{
		KingdomID: 4, SourceCastleID: 7, TargetX: 101, TargetY: 102, IslandObjectID: 777,
		ReportID: 202, Status: StormIslandReturnReady, LeaveBehind: 1, Survivors: map[UnitID]int64{10: 4, 12: 5},
	}
	if err := SaveSnapshot(directory, state); err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadSnapshot(directory)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Revision != 42 || loaded.Player.ID != 99 || loaded.Castles[7].Name != "Test" {
		t.Fatalf("unexpected loaded state: %#v", loaded)
	}
	loadedReturn := loaded.Storm.IslandReturns[islandReturnKey]
	if loadedReturn.Status != StormIslandReturnReady || loadedReturn.ReportID != 202 || loadedReturn.Survivors[12] != 5 {
		t.Fatalf("pending Storm island return was not restored: %#v", loadedReturn)
	}
	if loaded.Session.Status != "stopped" || loaded.Session.LoggedIn {
		t.Fatalf("session was not reset: %#v", loaded.Session)
	}
	if loaded.Session.Generation != state.Session.Generation {
		t.Fatalf("session generation = %d, want %d", loaded.Session.Generation, state.Session.Generation)
	}
	if loaded.Session.BaselineGeneration != 0 || loaded.Session.ConnectionGeneration != 0 {
		t.Fatalf("persisted live session epoch was retained: %#v", loaded.Session)
	}
	if loaded.MovementSnapshot != (MovementSnapshot{}) {
		t.Fatalf("persisted movement connection marker was retained: %#v", loaded.MovementSnapshot)
	}
	if loaded.Session.ServerURL != state.Session.ServerURL {
		t.Fatalf("last server URL was not retained: %#v", loaded.Session)
	}
	if loaded.Account.WorldID != state.Session.ServerURL || loaded.Account.PlayerID != state.Player.ID {
		t.Fatalf("snapshot account binding = %#v", loaded.Account)
	}
	if loaded.TowerQueue.CursorVersion != TowerQueueCursorVersion ||
		len(loaded.TowerQueue.ConfirmedLaunchesByCastle) != 0 {
		t.Fatalf("tower cursor state was not initialized cleanly: %#v", loaded.TowerQueue)
	}
	info, err := os.Stat(snapshotPath(directory))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("snapshot permissions = %o", info.Mode().Perm())
	}
}

func TestSnapshotLoadMovesInspectedAllianceOutOfOwnSlot(t *testing.T) {
	directory := t.TempDir()
	state := NewGameState()
	state.Player.AllianceID = 9
	state.Alliance = AllianceState{ID: 10, Name: "Inspected", Members: []AllianceMember{}, Holdings: []AllianceHolding{}}
	if err := SaveSnapshot(directory, state); err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadSnapshot(directory)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Alliance.ID != 9 || loaded.Alliance.Name != "" {
		t.Fatalf("own alliance = %+v", loaded.Alliance)
	}
	if loaded.Alliances[10].Name != "Inspected" {
		t.Fatalf("alliance directory = %+v", loaded.Alliances)
	}
}

func TestSnapshotLoadRemovesOwnKhanReturnsFromTauntHistory(t *testing.T) {
	directory := t.TempDir()
	now := time.Now().UTC()
	state := NewGameState()
	state.Khan.Launches = []KhanLaunchState{{CommanderID: 24, MovementID: 100, ArrivesAt: now}}
	state.Khan.Taunts[100] = KhanTauntState{MovementID: 100, ObservedAt: now, ImpactAt: now.Add(time.Minute)}
	state.Khan.ResolvedTaunts = []KhanTauntState{
		{MovementID: 100, ObservedAt: now, ImpactAt: now.Add(time.Minute)},
		{MovementID: 200, ObservedAt: now, ImpactAt: now.Add(2 * time.Minute)},
	}
	state.Khan.TauntsObserved = 3
	state.Khan.TauntsResolved = 2
	state.Khan.TauntsTriggered = 3
	state.Khan.TauntCounterVersion = 0
	state.Khan.LastTauntResolvedAt = now.Add(3 * time.Minute)
	if err := SaveSnapshot(directory, state); err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadSnapshot(directory)
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded.Khan.Taunts) != 0 || len(loaded.Khan.ResolvedTaunts) != 1 ||
		loaded.Khan.ResolvedTaunts[0].MovementID != 200 ||
		loaded.Khan.TauntsTriggered != 1 || loaded.Khan.TauntsObserved != 1 || loaded.Khan.TauntsResolved != 1 ||
		loaded.Khan.TauntCounterVersion != KhanTauntCounterVersion ||
		!loaded.Khan.LastTauntResolvedAt.Equal(now.Add(2*time.Minute)) {
		t.Fatalf("reconciled Khan taunts = %#v", loaded.Khan)
	}
}
