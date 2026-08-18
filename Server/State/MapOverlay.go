package State

const accountMapShardCount = 256

// accountMapGeneration is the compact physical form of the account-private
// map overlay. The logical GameState view is still materialized as
// map[kingdom]["x:y"] at serialization and defensive snapshot boundaries.
type accountMapGeneration struct {
	regions map[KingdomID]*accountMapRegion
}

type accountMapRegion struct {
	// Each retained official feature owns its physical shards. A row is stored
	// once, while feature readers can traverse only their partition instead of
	// scanning every retained map object and filtering by TypeID.
	kinds [mapProjectionKindCount]*accountMapKindRegion
}

type accountMapKindRegion struct {
	shards [accountMapShardCount]map[string]MapObservation
}

type mutableAccountMapRegion struct {
	region       *accountMapRegion
	mutableKinds [mapProjectionKindCount]*mutableAccountMapKindRegion
}

type mutableAccountMapKindRegion struct {
	region       *accountMapKindRegion
	clonedShards [accountMapShardCount]bool
}

func accountMapFromWorldMap(source WorldMap) *accountMapGeneration {
	generation := &accountMapGeneration{regions: map[KingdomID]*accountMapRegion{}}
	for kingdomID, observations := range source {
		for key, observation := range observations {
			kind, retained := MapProjectionKindForType(observation.TypeID)
			if !retained {
				continue
			}
			region := generation.regions[kingdomID]
			if region == nil {
				region = &accountMapRegion{}
				generation.regions[kingdomID] = region
			}
			kindRegion := region.kinds[kind]
			if kindRegion == nil {
				kindRegion = &accountMapKindRegion{}
				region.kinds[kind] = kindRegion
			}
			shard := mapShardIndex(key)
			if kindRegion.shards[shard] == nil {
				kindRegion.shards[shard] = map[string]MapObservation{}
			}
			kindRegion.shards[shard][key] = observation
		}
	}
	return generation
}

func mapShardIndex(key string) uint8 {
	// FNV-1a is stable, fast for short coordinate keys, and keeps the physical
	// partition independent from game coordinate ranges.
	hash := uint32(2166136261)
	for index := 0; index < len(key); index++ {
		hash ^= uint32(key[index])
		hash *= 16777619
	}
	return uint8(hash)
}

func (state *GameState) compactMapOverlay() {
	if state == nil || len(state.Map) == 0 {
		if state != nil && state.mapOverlay == nil {
			state.mapOverlay = &accountMapGeneration{regions: map[KingdomID]*accountMapRegion{}}
		}
		return
	}
	if state.mapOverlay == nil || len(state.mapOverlay.regions) == 0 {
		state.mapOverlay = accountMapFromWorldMap(state.Map)
		state.Map = nil
		return
	}
	materialized := state.materializedPrivateMap()
	state.mapOverlay = accountMapFromWorldMap(materialized)
	state.Map = nil
}

func (state GameState) lookupPrivateMapObservation(kingdomID KingdomID, key string) (MapObservation, bool) {
	if observation, exists := state.Map[kingdomID][key]; exists {
		return observation, true
	}
	if state.mapOverlay == nil {
		return MapObservation{}, false
	}
	region := state.mapOverlay.regions[kingdomID]
	if region == nil {
		return MapObservation{}, false
	}
	shard := mapShardIndex(key)
	for kind := MapProjectionNone + 1; kind < mapProjectionKindCount; kind++ {
		if kindRegion := region.kinds[kind]; kindRegion != nil {
			if observation, exists := kindRegion.shards[shard][key]; exists {
				return observation, true
			}
		}
	}
	return MapObservation{}, false
}

func (state GameState) hasPrivateMapObservations(kingdomID KingdomID) bool {
	if len(state.Map[kingdomID]) > 0 {
		return true
	}
	if state.mapOverlay == nil {
		return false
	}
	region := state.mapOverlay.regions[kingdomID]
	if region == nil {
		return false
	}
	for kind := MapProjectionNone + 1; kind < mapProjectionKindCount; kind++ {
		if kindRegion := region.kinds[kind]; kindRegion != nil {
			for _, shard := range kindRegion.shards {
				if len(shard) > 0 {
					return true
				}
			}
		}
	}
	return false
}

func (state GameState) rangePrivateMapObservations(kingdomID KingdomID, visit func(string, MapObservation) bool) bool {
	return state.rangePrivateMapObservationsByKind(kingdomID, MapProjectionNone, visit)
}

func (state GameState) rangePrivateMapObservationsByKind(
	kingdomID KingdomID,
	kind MapProjectionKind,
	visit func(string, MapObservation) bool,
) bool {
	if visit == nil {
		return true
	}
	var seen map[string]struct{}
	if len(state.Map[kingdomID]) > 0 {
		seen = make(map[string]struct{}, len(state.Map[kingdomID]))
	}
	for key, observation := range state.Map[kingdomID] {
		seen[key] = struct{}{}
		observationKind, retained := MapProjectionKindForType(observation.TypeID)
		if !retained || kind != MapProjectionNone && observationKind != kind {
			continue
		}
		if !visit(key, observation) {
			return false
		}
	}
	if state.mapOverlay == nil || state.mapOverlay.regions[kingdomID] == nil {
		return true
	}
	region := state.mapOverlay.regions[kingdomID]
	first, last := MapProjectionNone+1, mapProjectionKindCount
	if kind > MapProjectionNone && kind < mapProjectionKindCount {
		first, last = kind, kind+1
	}
	for currentKind := first; currentKind < last; currentKind++ {
		kindRegion := region.kinds[currentKind]
		if kindRegion == nil {
			continue
		}
		for _, shard := range kindRegion.shards {
			for key, observation := range shard {
				if _, shadowed := seen[key]; shadowed {
					continue
				}
				if !visit(key, observation) {
					return false
				}
			}
		}
	}
	return true
}

func (state GameState) materializedPrivateMap() WorldMap {
	result := make(WorldMap, len(state.Map))
	if state.mapOverlay != nil {
		for kingdomID, region := range state.mapOverlay.regions {
			observations := map[string]MapObservation{}
			for kind := MapProjectionNone + 1; kind < mapProjectionKindCount; kind++ {
				kindRegion := region.kinds[kind]
				if kindRegion == nil {
					continue
				}
				for _, shard := range kindRegion.shards {
					for key, observation := range shard {
						observations[key] = observation
					}
				}
			}
			if len(observations) > 0 {
				result[kingdomID] = observations
			}
		}
	}
	for kingdomID, observations := range state.Map {
		region := result[kingdomID]
		if region == nil {
			region = map[string]MapObservation{}
			result[kingdomID] = region
		}
		for key, observation := range observations {
			region[key] = observation
		}
	}
	return result
}

func (state *GameState) prepareMapMutation(source GameState) {
	base := source.mapOverlay
	if base == nil {
		base = accountMapFromWorldMap(source.Map)
	}
	state.Map = nil
	state.mapOverlay = &accountMapGeneration{regions: cloneMap(base.regions)}
	state.mapMutationCOW = true
	state.mutableMapRegions = map[KingdomID]*mutableAccountMapRegion{}
}

func (state *GameState) mutableMapShard(
	kingdomID KingdomID,
	kind MapProjectionKind,
	key string,
) map[string]MapObservation {
	if !state.mapMutationCOW {
		if state.Map == nil {
			state.Map = WorldMap{}
		}
		if state.Map[kingdomID] == nil {
			state.Map[kingdomID] = map[string]MapObservation{}
		}
		return state.Map[kingdomID]
	}
	mutable := state.mutableMapRegions[kingdomID]
	if mutable == nil {
		var region accountMapRegion
		if current := state.mapOverlay.regions[kingdomID]; current != nil {
			region = *current
		}
		mutable = &mutableAccountMapRegion{region: &region}
		state.mutableMapRegions[kingdomID] = mutable
		state.mapOverlay.regions[kingdomID] = mutable.region
	}
	if kind <= MapProjectionNone || kind >= mapProjectionKindCount {
		panic("invalid map projection kind")
	}
	mutableKind := mutable.mutableKinds[kind]
	if mutableKind == nil {
		var kindRegion accountMapKindRegion
		if current := mutable.region.kinds[kind]; current != nil {
			kindRegion = *current
		}
		mutableKind = &mutableAccountMapKindRegion{region: &kindRegion}
		mutable.mutableKinds[kind] = mutableKind
		mutable.region.kinds[kind] = mutableKind.region
	}
	shard := mapShardIndex(key)
	if !mutableKind.clonedShards[shard] {
		mutableKind.region.shards[shard] = cloneMap(mutableKind.region.shards[shard])
		mutableKind.clonedShards[shard] = true
	}
	if mutableKind.region.shards[shard] == nil {
		mutableKind.region.shards[shard] = map[string]MapObservation{}
	}
	return mutableKind.region.shards[shard]
}

func (state GameState) privateMapKingdomIDs(add func(KingdomID)) {
	for kingdomID := range state.Map {
		add(kingdomID)
	}
	if state.mapOverlay == nil {
		return
	}
	for kingdomID, region := range state.mapOverlay.regions {
		found := false
		for kind := MapProjectionNone + 1; kind < mapProjectionKindCount && !found; kind++ {
			if kindRegion := region.kinds[kind]; kindRegion != nil {
				for _, shard := range kindRegion.shards {
					if len(shard) > 0 {
						add(kingdomID)
						found = true
						break
					}
				}
			}
		}
	}
}

func (state GameState) rangePrivateMapShards(visit func(KingdomID, uint8)) {
	if visit == nil {
		return
	}
	seen := map[string]struct{}{}
	state.privateMapKingdomIDs(func(kingdomID KingdomID) {
		if state.mapOverlay != nil {
			if region := state.mapOverlay.regions[kingdomID]; region != nil {
				for kind := MapProjectionNone + 1; kind < mapProjectionKindCount; kind++ {
					if kindRegion := region.kinds[kind]; kindRegion != nil {
						for shard, values := range kindRegion.shards {
							if len(values) == 0 {
								continue
							}
							persistenceKey := mapPersistenceShardKey(kingdomID, uint8(shard))
							if _, exists := seen[persistenceKey]; !exists {
								seen[persistenceKey] = struct{}{}
								visit(kingdomID, uint8(shard))
							}
						}
					}
				}
			}
		}
		for key := range state.Map[kingdomID] {
			shard := mapShardIndex(key)
			persistenceKey := mapPersistenceShardKey(kingdomID, shard)
			if _, exists := seen[persistenceKey]; !exists {
				seen[persistenceKey] = struct{}{}
				visit(kingdomID, shard)
			}
		}
	})
}

func (state GameState) privateMapShard(kingdomID KingdomID, shard uint8) map[string]MapObservation {
	result := map[string]MapObservation{}
	// Persist one physical shard without scanning the other 255. State.Map is
	// normally empty after compactMapOverlay, but retain its overlay semantics
	// for compatibility snapshots that have not yet been compacted.
	if state.mapOverlay != nil {
		if region := state.mapOverlay.regions[kingdomID]; region != nil {
			for kind := MapProjectionNone + 1; kind < mapProjectionKindCount; kind++ {
				if kindRegion := region.kinds[kind]; kindRegion != nil {
					for key, observation := range kindRegion.shards[shard] {
						result[key] = observation
					}
				}
			}
		}
	}
	for key, observation := range state.Map[kingdomID] {
		if mapShardIndex(key) == shard {
			result[key] = observation
		}
	}
	return result
}
