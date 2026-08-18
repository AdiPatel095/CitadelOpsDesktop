package State

import (
	"encoding/json"
	"fmt"
	"os"
	"testing"
	"time"
)

func benchmarkSnapshot(benchmark *testing.B) GameState {
	benchmark.Helper()
	dataDir := os.Getenv("CITADEL_BENCHMARK_DATA_DIR")
	if dataDir == "" {
		dataDir = "../../Data"
	}
	state, err := LoadSnapshot(dataDir)
	if err != nil {
		benchmark.Skipf("load state fixture from %s: %v", dataDir, err)
	}
	return state
}

func BenchmarkStoreSnapshotCurrentData(benchmark *testing.B) {
	state := benchmarkSnapshot(benchmark)
	store := NewStore(state)
	benchmark.ReportAllocs()
	benchmark.ResetTimer()
	for benchmark.Loop() {
		_ = store.Snapshot()
	}
}

func BenchmarkStorePlanningViewCurrentData(benchmark *testing.B) {
	state := benchmarkSnapshot(benchmark)
	store := NewStore(state)
	benchmark.ReportAllocs()
	benchmark.ResetTimer()
	for benchmark.Loop() {
		_ = store.PlanningView()
	}
}

func BenchmarkStoreIngestObservationViewCurrentData(benchmark *testing.B) {
	state := benchmarkSnapshot(benchmark)
	store := NewStore(state)
	benchmark.ReportAllocs()
	benchmark.ResetTimer()
	for benchmark.Loop() {
		_ = store.IngestObservationView()
	}
}

func BenchmarkMarshalCurrentStateSnapshot(benchmark *testing.B) {
	state := benchmarkSnapshot(benchmark)
	benchmark.ReportAllocs()
	benchmark.ResetTimer()
	for benchmark.Loop() {
		if _, err := json.Marshal(state); err != nil {
			benchmark.Fatal(err)
		}
	}
}

func BenchmarkMarshalClientStateSnapshotCurrentData(benchmark *testing.B) {
	state := benchmarkSnapshot(benchmark)
	snapshot := NewClientStateSnapshot(state)
	benchmark.ReportAllocs()
	benchmark.ResetTimer()
	for benchmark.Loop() {
		if _, err := json.Marshal(snapshot); err != nil {
			benchmark.Fatal(err)
		}
	}
}

func BenchmarkStoreApplyCurrentData(benchmark *testing.B) {
	state := benchmarkSnapshot(benchmark)
	store := NewStore(state)
	benchmark.ReportAllocs()
	benchmark.ResetTimer()
	for benchmark.Loop() {
		if _, err := store.Apply(func(state *GameState) ([]string, bool, error) {
			state.Session.Generation++
			return []string{"session"}, true, nil
		}); err != nil {
			benchmark.Fatal(err)
		}
	}
}

func BenchmarkStoreApplyWithoutMapMutationCurrentData(benchmark *testing.B) {
	state := benchmarkSnapshot(benchmark)
	store := NewStore(state)
	benchmark.ReportAllocs()
	benchmark.ResetTimer()
	for benchmark.Loop() {
		if _, err := store.ApplyWithoutMapMutation(func(state *GameState) ([]string, bool, error) {
			state.Session.Generation++
			return []string{"session"}, true, nil
		}); err != nil {
			benchmark.Fatal(err)
		}
	}
}

func BenchmarkStoreApplyPlayerComponentCurrentData(benchmark *testing.B) {
	state := benchmarkSnapshot(benchmark)
	store := NewStore(state)
	writes := Components(ComponentPlayer)
	benchmark.ReportAllocs()
	benchmark.ResetTimer()
	for benchmark.Loop() {
		if _, err := store.ApplyComponents(writes, func(state *GameState) ([]string, bool, error) {
			state.Player.Level++
			return []string{"player"}, true, nil
		}); err != nil {
			benchmark.Fatal(err)
		}
	}
}

func BenchmarkStoreApplyObservationComponentCurrentData(benchmark *testing.B) {
	state := benchmarkSnapshot(benchmark)
	store := NewStore(state)
	writes := Components(ComponentSession, ComponentObservations)
	benchmark.ReportAllocs()
	benchmark.ResetTimer()
	for benchmark.Loop() {
		if _, err := store.ApplyComponents(writes, func(state *GameState) ([]string, bool, error) {
			observation := state.Observations["benchmark"]
			observation.Count++
			state.Observations["benchmark"] = observation
			return []string{"protocol"}, true, nil
		}); err != nil {
			benchmark.Fatal(err)
		}
	}
}

func BenchmarkStoreApplySessionComponentCurrentData(benchmark *testing.B) {
	state := benchmarkSnapshot(benchmark)
	store := NewStore(state)
	writes := Components(ComponentSession)
	benchmark.ReportAllocs()
	benchmark.ResetTimer()
	for benchmark.Loop() {
		if _, err := store.ApplyComponents(writes, func(state *GameState) ([]string, bool, error) {
			state.Session.Generation++
			return []string{"session"}, true, nil
		}); err != nil {
			benchmark.Fatal(err)
		}
	}
}

func BenchmarkStoreApplyCastleComponentCurrentData(benchmark *testing.B) {
	state := benchmarkSnapshot(benchmark)
	var castleID CastleID
	var resourceID ResourceID
	for id := range state.Castles {
		castleID = id
		for id := range state.Castles[id].Resources {
			resourceID = id
			break
		}
		break
	}
	if castleID == 0 {
		benchmark.Skip("fixture has no castle")
	}
	if resourceID == 0 {
		resourceID = 1
		castle := state.Castles[castleID]
		if castle.Resources == nil {
			castle.Resources = map[ResourceID]ResourceBalance{}
		}
		castle.Resources[resourceID] = ResourceBalance{Amount: 1}
		state.Castles[castleID] = castle
	}
	store := NewStore(state)
	writes := Components(ComponentCastles)
	benchmark.ReportAllocs()
	benchmark.ResetTimer()
	for benchmark.Loop() {
		if _, err := store.ApplyComponents(writes, func(state *GameState) ([]string, bool, error) {
			castle, found := state.MutableCastleParts(castleID, CastlePartResources)
			if !found {
				return nil, false, fmt.Errorf("castle %d disappeared", castleID)
			}
			balance := castle.Resources[resourceID]
			balance.Amount++
			castle.Resources[resourceID] = balance
			state.SetCastleParts(castleID, castle, CastlePartResources)
			return []string{"castles", "resources"}, true, nil
		}); err != nil {
			benchmark.Fatal(err)
		}
	}
}

func BenchmarkMarshalCastleDeltaCurrentData(benchmark *testing.B) {
	state := benchmarkSnapshot(benchmark)
	var castleID CastleID
	for id := range state.Castles {
		castleID = id
		break
	}
	if castleID == 0 {
		benchmark.Skip("fixture has no castle")
	}
	store := NewStore(state)
	event, err := store.ApplyComponents(Components(ComponentCastles), func(state *GameState) ([]string, bool, error) {
		castle, found := state.MutableCastleParts(castleID, CastlePartResources)
		if !found {
			return nil, false, fmt.Errorf("castle %d disappeared", castleID)
		}
		for resourceID, balance := range castle.Resources {
			balance.Amount++
			castle.Resources[resourceID] = balance
			break
		}
		state.SetCastleParts(castleID, castle, CastlePartResources)
		return []string{"castles", "resources"}, true, nil
	})
	if err != nil {
		benchmark.Fatal(err)
	}
	benchmark.ReportAllocs()
	benchmark.ResetTimer()
	for benchmark.Loop() {
		if _, err := json.Marshal(event); err != nil {
			benchmark.Fatal(err)
		}
	}
}

func BenchmarkMarshalClientCastleDeltaRepeatedCurrentData(benchmark *testing.B) {
	benchmarkClientCastleDelta(benchmark, true)
}

func BenchmarkMarshalClientCastleDeltaUncachedCurrentData(benchmark *testing.B) {
	benchmarkClientCastleDelta(benchmark, false)
}

func benchmarkClientCastleDelta(benchmark *testing.B, cached bool) {
	state := benchmarkSnapshot(benchmark)
	var castleID CastleID
	for id := range state.Castles {
		castleID = id
		break
	}
	if castleID == 0 {
		benchmark.Skip("fixture has no castle")
	}
	store := NewStore(state)
	event, err := store.ApplyComponents(Components(ComponentCastles), func(state *GameState) ([]string, bool, error) {
		castle, found := state.MutableCastleParts(castleID, CastlePartResources)
		if !found {
			return nil, false, fmt.Errorf("castle %d disappeared", castleID)
		}
		state.SetCastleParts(castleID, castle, CastlePartResources)
		return []string{"castles"}, true, nil
	})
	if err != nil {
		benchmark.Fatal(err)
	}
	benchmark.ReportAllocs()
	benchmark.ResetTimer()
	for benchmark.Loop() {
		var err error
		if cached {
			_, err = ClientEventPayload(event)
		} else {
			_, err = json.Marshal(ClientEvent(event))
		}
		if err != nil {
			benchmark.Fatal(err)
		}
	}
}

func BenchmarkStoreApplyMovementComponentCurrentData(benchmark *testing.B) {
	state := benchmarkSnapshot(benchmark)
	var movementID MovementID
	for id := range state.Movements {
		movementID = id
		break
	}
	if movementID == 0 {
		movementID = 1
		state.Movements[movementID] = MovementState{ID: movementID, Units: map[UnitID]int64{1: 1}}
	}
	store := NewStore(state)
	writes := Components(ComponentMovements)
	benchmark.ReportAllocs()
	benchmark.ResetTimer()
	for benchmark.Loop() {
		if _, err := store.ApplyComponents(writes, func(state *GameState) ([]string, bool, error) {
			movement, _ := state.LookupMovement(movementID)
			movement.TargetX++
			state.SetMovement(movementID, movement)
			return []string{"movements"}, true, nil
		}); err != nil {
			benchmark.Fatal(err)
		}
	}
}

func BenchmarkStoreApplyInventoryComponentCurrentData(benchmark *testing.B) {
	state := benchmarkSnapshot(benchmark)
	store := NewStore(state)
	writes := Components(ComponentInventory)
	benchmark.ReportAllocs()
	benchmark.ResetTimer()
	for benchmark.Loop() {
		if _, err := store.ApplyComponents(writes, func(state *GameState) ([]string, bool, error) {
			state.MutableInventoryConstructionItems()[1]++
			return []string{"inventory"}, true, nil
		}); err != nil {
			benchmark.Fatal(err)
		}
	}
}

func BenchmarkMarshalInventoryDeltaCurrentData(benchmark *testing.B) {
	state := benchmarkSnapshot(benchmark)
	store := NewStore(state)
	event, err := store.ApplyComponents(Components(ComponentInventory), func(state *GameState) ([]string, bool, error) {
		state.MutableInventoryConstructionItems()[1]++
		return []string{"inventory"}, true, nil
	})
	if err != nil {
		benchmark.Fatal(err)
	}
	benchmark.ReportAllocs()
	benchmark.ResetTimer()
	for benchmark.Loop() {
		if _, err := json.Marshal(event); err != nil {
			benchmark.Fatal(err)
		}
	}
}

func BenchmarkStoreApplyStormMetadataCurrentData(benchmark *testing.B) {
	state := benchmarkSnapshot(benchmark)
	store := NewStore(state)
	writes := Components(ComponentStorm)
	benchmark.ReportMetric(float64(store.ReadOnlyView().StormTargetCount()), "targets")
	benchmark.ReportAllocs()
	benchmark.ResetTimer()
	for benchmark.Loop() {
		if _, err := store.ApplyComponents(writes, func(state *GameState) ([]string, bool, error) {
			state.Storm.LunaShopPending = !state.Storm.LunaShopPending
			return []string{"storm"}, true, nil
		}); err != nil {
			benchmark.Fatal(err)
		}
	}
}

func benchmarkComponentManifest(referenceCount int) componentManifest {
	manifest := componentManifest{Files: map[string]string{}, MapFiles: make(map[string]string, referenceCount)}
	for index := 0; index < referenceCount; index++ {
		manifest.MapFiles[fmt.Sprintf("0:%d", index)] = fmt.Sprintf("map-0-%d-00000000000000000001.json", index)
	}
	return manifest
}

func BenchmarkComponentManifestCleanupReferences2883(benchmark *testing.B) {
	const references = 2_883
	manifest := benchmarkComponentManifest(references)
	previous := componentManifestReferences(manifest)
	retained := componentManifestReferences(manifest)
	benchmark.ReportMetric(references, "refs")
	benchmark.ReportAllocs()
	benchmark.ResetTimer()
	for benchmark.Loop() {
		count := 0
		for filename := range previous {
			if _, found := retained[filename]; found {
				count++
			}
		}
		if count != references {
			benchmark.Fatalf("retained references = %d, want %d", count, references)
		}
	}
}

// BenchmarkComponentManifestCleanupLegacy2883 preserves the former quadratic
// cleanup shape for a direct one-iteration comparison. Production now builds
// the retained reference set once, outside the previous-reference loop.
func BenchmarkComponentManifestCleanupLegacy2883(benchmark *testing.B) {
	const references = 2_883
	manifest := benchmarkComponentManifest(references)
	previous := componentManifestReferences(manifest)
	benchmark.ReportMetric(references, "refs")
	benchmark.ReportAllocs()
	benchmark.ResetTimer()
	for benchmark.Loop() {
		count := 0
		for filename := range previous {
			if _, found := componentManifestReferences(manifest)[filename]; found {
				count++
			}
		}
		if count != references {
			benchmark.Fatalf("retained references = %d, want %d", count, references)
		}
	}
}

// BenchmarkCloneStormComponentLegacyCurrentData measures the former revision
// cost: every unrelated Storm scalar update deep-copied the full authoritative
// target observation map before the reducer ran.
func BenchmarkCloneStormComponentLegacyCurrentData(benchmark *testing.B) {
	state := NewStore(benchmarkSnapshot(benchmark)).ReadOnlyView()
	benchmark.ReportAllocs()
	benchmark.ResetTimer()
	benchmark.ReportMetric(float64(state.StormTargetCount()), "targets")
	for benchmark.Loop() {
		_ = cloneGameStateComponents(state, Components(ComponentStorm))
	}
}

func BenchmarkCloneEachComponentCurrentData(benchmark *testing.B) {
	state := NewStore(benchmarkSnapshot(benchmark)).ReadOnlyView()
	for component := Component(0); component < componentCount; component++ {
		component := component
		benchmark.Run(component.String(), func(benchmark *testing.B) {
			benchmark.ReportAllocs()
			for benchmark.Loop() {
				_ = cloneGameStateForMutation(state, Components(component))
			}
		})
	}
}

func benchmarkLargeMapState() GameState {
	state := NewGameState()
	observedAt := time.Date(2026, time.August, 13, 12, 0, 0, 0, time.UTC)
	remaining := 47_000
	for kingdomID := KingdomID(0); remaining > 0; kingdomID++ {
		region := map[string]MapObservation{}
		for index := 0; index < 10_000 && remaining > 0; index++ {
			x, y := index%1000, index/1000+int(kingdomID)*100
			region[fmt.Sprintf("%d:%d", x, y)] = MapObservation{
				KingdomID: kingdomID, X: x, Y: y, TypeID: MapTypeKingdomTower,
				TowerVictoryCount: int64(index), ObservedAt: observedAt,
			}
			remaining--
		}
		state.Map[kingdomID] = region
	}
	return state
}

func BenchmarkStoreApplyMapCoordinate47000(benchmark *testing.B) {
	store := NewStore(benchmarkLargeMapState())
	writes := Components(ComponentWorldMap)
	benchmark.ReportMetric(float64(47_000), "nodes")
	benchmark.ReportAllocs()
	benchmark.ResetTimer()
	for benchmark.Loop() {
		if _, err := store.ApplyComponents(writes, func(state *GameState) ([]string, bool, error) {
			observation, _ := state.LookupMapObservation(0, "1:0")
			observation.TowerVictoryCount++
			return []string{"map"}, state.SetMapObservation(observation), nil
		}); err != nil {
			benchmark.Fatal(err)
		}
	}
}

func BenchmarkStoreApplyLegacyMapClone47000(benchmark *testing.B) {
	store := NewStore(benchmarkLargeMapState())
	benchmark.ReportMetric(float64(47_000), "nodes")
	benchmark.ReportAllocs()
	benchmark.ResetTimer()
	for benchmark.Loop() {
		if _, err := store.Apply(func(state *GameState) ([]string, bool, error) {
			observation := state.Map[0]["1:0"]
			observation.TowerVictoryCount++
			state.Map[0]["1:0"] = observation
			return []string{"map"}, true, nil
		}); err != nil {
			benchmark.Fatal(err)
		}
	}
}

func benchmarkSharedStormWorld(nodeCount int, stormCount int) GameState {
	state := NewGameState()
	state.Castles[1] = CastleState{ID: 1, KingdomID: stormKingdomID}
	region := &worldFactRegion{}
	observedAt := time.Date(2026, time.August, 13, 12, 0, 0, 0, time.UTC)
	for index := 0; index < nodeCount; index++ {
		key := fmt.Sprintf("%d:%d", index%1000, index/1000)
		fact := WorldMapFact{
			KingdomID: stormKingdomID, X: index % 1000, Y: index / 1000,
			TypeID: 1, OwnerID: PlayerID(index + 1), ObservedAt: observedAt,
		}
		if index < stormCount {
			fact.TypeID = MapTypeStormFort
			fact.OwnerID = -403
			fact.Event = &WorldEventMapFact{Level: 80}
		}
		addWorldFact(region, key, fact)
	}
	state.sharedMap = &worldMapGeneration{values: worldFactMap{stormKingdomID: region}}
	return state
}

func BenchmarkRangeStormTargetsIndexed47000(benchmark *testing.B) {
	benchmarkRangeStormTargets(benchmark, true)
}

func BenchmarkRangeStormTargetsFullScan47000(benchmark *testing.B) {
	benchmarkRangeStormTargets(benchmark, false)
}

func benchmarkRangeStormTargets(benchmark *testing.B, indexed bool) {
	const targets = 24
	state := benchmarkSharedStormWorld(47_000, targets)
	benchmark.ReportMetric(47_000, "world-nodes")
	benchmark.ReportMetric(targets, "storm-targets")
	benchmark.ReportAllocs()
	benchmark.ResetTimer()
	for benchmark.Loop() {
		count := 0
		if indexed {
			state.RangeStormTargets(func(string, MapObservation) bool {
				count++
				return true
			})
		} else {
			state.RangeMapObservations(stormKingdomID, func(_ string, observation MapObservation) bool {
				if isStormMapType(observation.TypeID) {
					count++
				}
				return true
			})
		}
		if count != targets {
			benchmark.Fatalf("Storm target count = %d, want %d", count, targets)
		}
	}
}

func benchmarkMixedPrivateMap() GameState {
	state := NewGameState()
	observedAt := time.Date(2026, time.August, 13, 12, 0, 0, 0, time.UTC)
	region := make(map[string]MapObservation, 47_000)
	for index := 0; index < 47_000; index++ {
		typeID := MapTypeKingdomTower
		if index < 24 {
			typeID = MapTypeRift
		}
		x, y := index%1000, index/1000
		region[fmt.Sprintf("%d:%d", x, y)] = MapObservation{
			KingdomID: 0, X: x, Y: y, TypeID: typeID, ObservedAt: observedAt,
		}
	}
	state.Map[0] = region
	return NewStore(state).ReadOnlyView()
}

func BenchmarkRangeRiftTargetsIndexed47000(benchmark *testing.B) {
	benchmarkRangeRiftTargets(benchmark, true)
}

func BenchmarkRangeRiftTargetsFullScan47000(benchmark *testing.B) {
	benchmarkRangeRiftTargets(benchmark, false)
}

func benchmarkRangeRiftTargets(benchmark *testing.B, indexed bool) {
	const targets = 24
	state := benchmarkMixedPrivateMap()
	benchmark.ReportMetric(47_000, "map-nodes")
	benchmark.ReportMetric(targets, "rift-targets")
	benchmark.ReportAllocs()
	benchmark.ResetTimer()
	for benchmark.Loop() {
		count := 0
		if indexed {
			state.RangeMapObservationsByKind(0, MapProjectionRift, func(string, MapObservation) bool {
				count++
				return true
			})
		} else {
			state.RangeMapObservations(0, func(_ string, observation MapObservation) bool {
				if observation.TypeID == MapTypeRift {
					count++
				}
				return true
			})
		}
		if count != targets {
			benchmark.Fatalf("Rift target count = %d, want %d", count, targets)
		}
	}
}
