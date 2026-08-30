package State

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"
)

func TestCanonicalWorldIDUsesOnlyNormalizedServerIdentity(t *testing.T) {
	for input, expected := range map[string]string{
		" WSS://EP-LIVE-US1-GAME.GOODGAMESTUDIOS.COM:443/socket ": "ep-live-us1-game.goodgamestudios.com",
		"https://World.Example:8443/private/path?ignored=true":    "world.example:8443",
		"WORLD-ONE": "world-one",
	} {
		if actual := CanonicalWorldID(input); actual != expected {
			t.Fatalf("CanonicalWorldID(%q) = %q, want %q", input, actual, expected)
		}
	}
}

func TestWorldMapSharesObjectiveFactsWithoutPrivateTargets(t *testing.T) {
	worlds := NewWorldMapStore()
	alphaInitial := NewGameState()
	alphaInitial.Account.WorldID = " WSS://WORLD.EXAMPLE/socket "
	alphaInitial.Player.ID = 101
	alpha := NewStoreWithWorldMap(alphaInitial, worlds)
	bravoInitial := NewGameState()
	bravoInitial.Account.WorldID = "wss://world.example/socket"
	bravoInitial.Player.ID = 202
	bravo := NewStoreWithWorldMap(bravoInitial, worlds)
	otherInitial := NewGameState()
	otherInitial.Account.WorldID = "wss://other.example/socket"
	other := NewStoreWithWorldMap(otherInitial, worlds)

	worldEvents, unsubscribe := worlds.Subscribe(4)
	defer unsubscribe()
	observedAt := time.Now().UTC()
	shared := MapObservation{
		KingdomID: 0, X: 100, Y: 101, TypeID: 1, OwnerID: 500, ObjectID: 700,
		Name: "Shared castle", ObservedAt: observedAt,
	}
	alphaEvent, err := alpha.ApplyComponents(Components(ComponentWorldMap), func(state *GameState) ([]string, bool, error) {
		return []string{"map"}, state.SetMapObservation(shared), nil
	})
	if err != nil {
		t.Fatal(err)
	}
	worldEvent := <-worldEvents
	if _, changed := bravo.AdoptWorldMap(worldEvent); !changed {
		t.Fatal("same-world account did not adopt objective map fact")
	}
	if _, changed := other.AdoptWorldMap(worldEvent); changed {
		t.Fatal("different-world account adopted objective map fact")
	}
	if got, found := bravo.ReadOnlyView().LookupMapObservation(0, "100:101"); !found || got.OwnerID != 500 {
		t.Fatalf("shared observation = %+v, found %t", got, found)
	}
	if _, found := other.ReadOnlyView().LookupMapObservation(0, "100:101"); found {
		t.Fatal("shared observation leaked across worlds")
	}
	if alphaEvent.Patch == nil || alphaEvent.Patch.Map != nil || alphaEvent.Patch.MapChanges == nil || len(*alphaEvent.Patch.MapChanges) != 1 {
		t.Fatalf("shared update was not a coordinate patch: %+v", alphaEvent.Patch)
	}

	private := MapObservation{
		KingdomID: 0, X: 200, Y: 201, TypeID: 2, OwnerID: 999,
		TowerVictoryCount: 12, ObservedAt: observedAt,
	}
	privateEvent, err := alpha.ApplyComponents(Components(ComponentWorldMap), func(state *GameState) ([]string, bool, error) {
		return []string{"map"}, state.SetMapObservation(private), nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, found := bravo.ReadOnlyView().LookupMapObservation(0, "200:201"); found {
		t.Fatal("account-relative tower target leaked to another account")
	}
	if privateEvent.Patch == nil || privateEvent.Patch.MapChanges == nil || len(*privateEvent.Patch.MapChanges) != 1 {
		t.Fatalf("private map update was not a coordinate patch: %+v", privateEvent.Patch)
	}
}

func TestWorldMapSharesStormFactsOnlyWithAccountsThatUnlockedKingdom(t *testing.T) {
	worlds := NewWorldMapStore()
	newAccount := func(playerID PlayerID, unlocked bool) *Store {
		initial := NewGameState()
		initial.Account.WorldID = "world-one"
		initial.Player.ID = playerID
		if unlocked {
			initial.Castles[CastleID(playerID)] = CastleState{
				ID: CastleID(playerID), KingdomID: stormKingdomID,
			}
		}
		return NewStoreWithWorldMap(initial, worlds)
	}
	alpha := newAccount(101, true)
	bravo := newAccount(202, true)
	locked := newAccount(303, false)
	events, unsubscribe := worlds.Subscribe(4)
	defer unsubscribe()

	observedAt := time.Now().UTC()
	fort := MapObservation{
		KingdomID: stormKingdomID, X: 612, Y: 667, TypeID: MapTypeStormFort,
		Level: 80, OwnerID: -403, ObjectID: 9001, StormIsleID: 10,
		StormVictoryCount: 7, StormCooldownRemaining: 120, ObservedAt: observedAt,
	}
	if _, err := alpha.ApplyComponents(Components(ComponentWorldMap), func(state *GameState) ([]string, bool, error) {
		return []string{"map"}, state.SetMapObservation(fort), nil
	}); err != nil {
		t.Fatal(err)
	}
	event := <-events
	if _, changed := bravo.AdoptWorldMap(event); !changed {
		t.Fatal("same-world account with Storm unlocked did not adopt shared fort")
	}
	if _, changed := locked.AdoptWorldMap(event); changed {
		t.Fatal("locked account received an observable Storm map revision")
	}
	if got, found := bravo.ReadOnlyView().LookupStormTarget("612:667"); !found || got != fort {
		t.Fatalf("shared Storm fort = %+v, found %t", got, found)
	}
	if alpha.ReadOnlyView().sharedMap == nil || alpha.ReadOnlyView().sharedMap != bravo.ReadOnlyView().sharedMap {
		t.Fatal("same-world accounts retained separate physical Storm generations")
	}
	if _, found := locked.ReadOnlyView().LookupMapObservation(stormKingdomID, "612:667"); found {
		t.Fatal("Storm fact leaked to an account without kingdom access")
	}

	if _, err := alpha.ApplyComponents(Components(ComponentStorm), func(state *GameState) ([]string, bool, error) {
		return []string{"storm"}, state.DeleteStormTarget("612:667"), nil
	}); err != nil {
		t.Fatal(err)
	}
	if _, found := alpha.ReadOnlyView().LookupStormTarget("612:667"); found {
		t.Fatal("account-private suppression did not hide consumed target")
	}
	if got, found := alpha.ReadOnlyView().LookupMapObservation(stormKingdomID, "612:667"); !found || got != fort {
		t.Fatalf("suppression removed shared fact: %+v, found %t", got, found)
	}
	if _, found := bravo.ReadOnlyView().LookupStormTarget("612:667"); !found {
		t.Fatal("one account's target consumption leaked into another account")
	}
}

func TestWorldMapStormIndexTracksTypeChangesAndDeletes(t *testing.T) {
	worlds := NewWorldMapStore()
	observedAt := time.Now().UTC()
	key := "612:667"
	storm := MapObservation{
		KingdomID: stormKingdomID, X: 612, Y: 667, TypeID: MapTypeStormFort,
		OwnerID: -403, ObservedAt: observedAt,
	}
	event := worlds.commit("world-one", nil, []MapChange{{
		KingdomID: stormKingdomID, Key: key, Observation: &storm,
	}})
	assertWorldStormKeys(t, event.generation, []string{key})

	castle := MapObservation{
		KingdomID: stormKingdomID, X: 612, Y: 667, TypeID: 1,
		OwnerID: 42, ObservedAt: observedAt.Add(time.Second),
	}
	event = worlds.commit("world-one", nil, []MapChange{{
		KingdomID: stormKingdomID, Key: key, Observation: &castle,
	}})
	assertWorldStormKeys(t, event.generation, nil)
	if fact, found := lookupWorldFact(event.generation.values, stormKingdomID, key); !found || fact.TypeID != 1 {
		t.Fatalf("replacement fact = %+v, found %t", fact, found)
	}

	storm.ObservedAt = observedAt.Add(2 * time.Second)
	event = worlds.commit("world-one", nil, []MapChange{{
		KingdomID: stormKingdomID, Key: key, Observation: &storm,
	}})
	assertWorldStormKeys(t, event.generation, []string{key})
	event = worlds.commit("world-one", nil, []MapChange{{
		KingdomID: stormKingdomID, Key: key, Deleted: true,
	}})
	assertWorldStormKeys(t, event.generation, nil)
}

func assertWorldStormKeys(t *testing.T, generation *worldMapGeneration, expected []string) {
	t.Helper()
	actual := []string{}
	if generation != nil {
		rangeWorldStormFacts(generation.values, stormKingdomID, func(key string, _ WorldMapFact) bool {
			actual = append(actual, key)
			return true
		})
	}
	slices.Sort(actual)
	slices.Sort(expected)
	if !slices.Equal(actual, expected) {
		t.Fatalf("indexed Storm keys = %v, want %v", actual, expected)
	}
}

func TestMapFeaturePartitionsTargetReadsAndCloneOnlyTouchedKind(t *testing.T) {
	initial := NewGameState()
	observedAt := time.Date(2026, time.August, 13, 12, 0, 0, 0, time.UTC)
	initial.Map[0] = map[string]MapObservation{
		"10:10": {KingdomID: 0, X: 10, Y: 10, TypeID: MapTypeKingdomTower, ObservedAt: observedAt},
		"20:20": {KingdomID: 0, X: 20, Y: 20, TypeID: MapTypeRift, ObservedAt: observedAt},
	}
	store := NewStore(initial)
	before := store.ReadOnlyView()
	regionBefore := before.mapOverlay.regions[0]
	towerBefore := regionBefore.kinds[MapProjectionTower]
	riftBefore := regionBefore.kinds[MapProjectionRift]

	if _, err := store.ApplyComponents(Components(ComponentWorldMap), func(state *GameState) ([]string, bool, error) {
		changed := state.SetMapObservation(MapObservation{
			KingdomID: 0, X: 10, Y: 10, TypeID: MapTypeKingdomTower,
			TowerVictoryCount: 2, ObservedAt: observedAt.Add(time.Second),
		})
		return []string{"map-tower"}, changed, nil
	}); err != nil {
		t.Fatal(err)
	}
	after := store.ReadOnlyView()
	regionAfter := after.mapOverlay.regions[0]
	if regionAfter.kinds[MapProjectionTower] == towerBefore {
		t.Fatal("tower mutation reused the immutable tower partition")
	}
	if regionAfter.kinds[MapProjectionRift] != riftBefore {
		t.Fatal("tower mutation cloned the untouched Rift partition")
	}
	towers, rifts := 0, 0
	after.RangeMapObservationsByKind(0, MapProjectionTower, func(_ string, observation MapObservation) bool {
		towers++
		if observation.TypeID != MapTypeKingdomTower {
			t.Fatalf("tower partition returned type %d", observation.TypeID)
		}
		return true
	})
	after.RangeMapObservationsByKind(0, MapProjectionRift, func(_ string, observation MapObservation) bool {
		rifts++
		if observation.TypeID != MapTypeRift {
			t.Fatalf("Rift partition returned type %d", observation.TypeID)
		}
		return true
	})
	if towers != 1 || rifts != 1 {
		t.Fatalf("targeted feature counts: towers=%d rifts=%d", towers, rifts)
	}
}

func TestPrivateCoordinateReplacementRemovesStaleSharedFactWithExactDomain(t *testing.T) {
	worlds := NewWorldMapStore()
	observedAt := time.Date(2026, time.August, 13, 12, 0, 0, 0, time.UTC)
	castle := MapObservation{
		KingdomID: 0, X: 10, Y: 10, TypeID: MapTypePlayerCastle,
		OwnerID: 42, ObservedAt: observedAt,
	}
	worlds.commit("world-one", nil, []MapChange{{KingdomID: 0, Key: "10:10", Observation: &castle}})
	tower := MapObservation{
		KingdomID: 0, X: 10, Y: 10, TypeID: MapTypeKingdomTower,
		ObservedAt: observedAt.Add(time.Second),
	}
	event := worlds.commitWithDomains(
		"world-one", nil,
		[]MapChange{{KingdomID: 0, Key: "10:10", Observation: &tower}},
		[]string{"map-tower"},
	)
	if len(event.Changes) != 1 || !event.Changes[0].Deleted || event.Changes[0].TypeID != MapTypePlayerCastle {
		t.Fatalf("shared replacement event = %+v", event.Changes)
	}
	if _, found := lookupWorldFact(event.generation.values, 0, "10:10"); found {
		t.Fatal("private replacement left a stale shared fact")
	}
	if len(event.Domains) != 1 || event.Domains[0] != "map-player-castle" {
		t.Fatalf("shared replacement domains = %v", event.Domains)
	}
}

func TestGameStateJSONMaterializesSharedAndPrivateMapLayers(t *testing.T) {
	worlds := NewWorldMapStore()
	initial := NewGameState()
	initial.Account.WorldID = "world-one"
	initial.Map[0] = map[string]MapObservation{
		"1:1": {KingdomID: 0, X: 1, Y: 1, TypeID: 1, OwnerID: 42, ObservedAt: time.Now().UTC()},
		"2:2": {KingdomID: 0, X: 2, Y: 2, TypeID: 2, TowerVictoryCount: 10, ObservedAt: time.Now().UTC()},
	}
	state := NewStoreWithWorldMap(initial, worlds).ReadOnlyView()
	raw, err := json.Marshal(state)
	if err != nil {
		t.Fatal(err)
	}
	var decoded GameState
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatal(err)
	}
	if len(decoded.Map[0]) != 2 || decoded.Map[0]["1:1"].OwnerID != 42 || decoded.Map[0]["2:2"].TowerVictoryCount != 10 {
		t.Fatalf("materialized map = %+v", decoded.Map)
	}
}

func TestWorldMapPersistenceRoundTripStoresOnlySharedFacts(t *testing.T) {
	directory := t.TempDir()
	worlds, err := OpenWorldMapStore(directory)
	if err != nil {
		t.Fatal(err)
	}
	worlds.StartPersistence()
	initial := NewGameState()
	initial.Account.WorldID = " World-One "
	store := NewStoreWithWorldMap(initial, worlds)
	now := time.Now().UTC().Truncate(time.Millisecond)
	shared := MapObservation{
		KingdomID: 0, X: 11, Y: 12, TypeID: 1, OwnerID: 88, ObjectID: 99,
		Name: "Durable castle", ObservedAt: now,
	}
	private := MapObservation{
		KingdomID: 0, X: 21, Y: 22, TypeID: 2, OwnerID: 88,
		TowerVictoryCount: 18, ObservedAt: now,
	}
	if _, err := store.ApplyComponents(Components(ComponentWorldMap), func(state *GameState) ([]string, bool, error) {
		return []string{"map"}, state.SetMapObservation(shared) && state.SetMapObservation(private), nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := worlds.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
	// Close is deliberately idempotent because the supervisor and a failed
	// startup cleanup can converge on the same process-owned store.
	if err := worlds.Close(context.Background()); err != nil {
		t.Fatal(err)
	}

	databasePath := filepath.Join(directory, "Shared", "State", "WorldMap.sqlite")
	info, err := os.Stat(databasePath)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("world-map database permissions = %o", info.Mode().Perm())
	}

	reopened, err := OpenWorldMapStore(directory)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if closeErr := reopened.Close(context.Background()); closeErr != nil {
			t.Errorf("close reopened world map: %v", closeErr)
		}
	}()
	loadedInitial := NewGameState()
	loadedInitial.Account.WorldID = "world-one"
	loaded := NewStoreWithWorldMap(loadedInitial, reopened).ReadOnlyView()
	if observation, exists := loaded.LookupMapObservation(0, "11:12"); !exists || observation.Name != shared.Name || !observation.ObservedAt.Equal(now) {
		t.Fatalf("persisted shared observation = %+v, exists %t", observation, exists)
	}
	if _, exists := loaded.LookupMapObservation(0, "21:22"); exists {
		t.Fatal("account-private tower progress was written to the shared database")
	}
}

func TestMapMutationCopiesOnlyTouchedRegionAndPreservesPriorGeneration(t *testing.T) {
	initial := NewGameState()
	initial.Map[0] = map[string]MapObservation{
		"1:1": {KingdomID: 0, X: 1, Y: 1, TypeID: 2, TowerVictoryCount: 1},
	}
	initial.Map[1] = map[string]MapObservation{
		"2:2": {KingdomID: 1, X: 2, Y: 2, TypeID: 2, TowerVictoryCount: 2},
	}
	store := NewStore(initial)
	before := store.ReadOnlyView()
	beforeTouchedRegion := before.mapOverlay.regions[0]
	beforeUntouchedRegion := before.mapOverlay.regions[1]
	updated, _ := before.LookupMapObservation(0, "1:1")
	updated.TowerVictoryCount = 3
	if _, err := store.ApplyComponents(Components(ComponentWorldMap), func(state *GameState) ([]string, bool, error) {
		return []string{"map"}, state.SetMapObservation(updated), nil
	}); err != nil {
		t.Fatal(err)
	}
	after := store.ReadOnlyView()
	prior, _ := before.LookupMapObservation(0, "1:1")
	if prior.TowerVictoryCount != 1 {
		t.Fatalf("prior immutable generation was mutated: %+v", prior)
	}
	current, _ := after.LookupMapObservation(0, "1:1")
	if current.TowerVictoryCount != 3 {
		t.Fatalf("current generation was not updated: %+v", current)
	}
	if beforeTouchedRegion == after.mapOverlay.regions[0] {
		t.Fatal("touched map region was not copied")
	}
	if beforeUntouchedRegion != after.mapOverlay.regions[1] {
		t.Fatal("untouched map region was unnecessarily copied")
	}
}

func TestIrrelevantMapObservationsAreRejectedAndPruned(t *testing.T) {
	state := NewGameState()
	irrelevant := MapObservation{KingdomID: 0, X: 4, Y: 5, TypeID: 999, OwnerID: 0}
	if state.SetMapObservation(irrelevant) {
		t.Fatal("irrelevant ownerless map node was retained")
	}
	irrelevant.OwnerID = 500
	if state.SetMapObservation(irrelevant) || ShareableMapObservation(irrelevant) {
		t.Fatal("unknown positive-owner map node bypassed the projection policy")
	}
	state.Map[0] = map[string]MapObservation{
		"4:5": irrelevant,
		"6:7": {KingdomID: 0, X: 6, Y: 7, TypeID: 2},
	}
	pruneIrrelevantMapObservations(&state)
	if _, exists := state.Map[0]["4:5"]; exists {
		t.Fatal("legacy irrelevant map node survived load pruning")
	}
	if _, exists := state.Map[0]["6:7"]; !exists {
		t.Fatal("actionable tower node was pruned")
	}
}

func TestMapProjectionStripsFieldsOutsideOfficialObjectKind(t *testing.T) {
	state := NewGameState()
	tower := MapObservation{
		KingdomID: 0, X: 10, Y: 11, TypeID: MapTypeKingdomTower,
		Name: "unused", OwnerID: 500, ObjectID: -1, Level: 70,
		TowerVictoryCount: 845, TowerCooldownRemaining: 60,
		EventCampID: 999, StormIsleID: 888, ObservedAt: time.Now().UTC(),
	}
	if !state.SetMapObservation(tower) {
		t.Fatal("tower observation was not retained")
	}
	stored := state.Map[0]["10:11"]
	if stored.Name != "" || stored.OwnerID != 0 || stored.EventCampID != 0 || stored.StormIsleID != 0 {
		t.Fatalf("tower retained unrelated object fields: %+v", stored)
	}
	if stored.ObjectID != -1 || stored.Level != 70 || stored.TowerVictoryCount != 845 || stored.TowerCooldownRemaining != 60 {
		t.Fatalf("tower projection lost required fields: %+v", stored)
	}
}
