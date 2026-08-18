package State

import (
	"fmt"
	"sort"
	"time"
)

const stormKingdomID KingdomID = 4

// stormMutationPart describes the independently owned mutable portions of the
// official Storm domain. Scalar map metadata and shop state live directly in
// StormState; the two nested account maps are copied only when a reducer asks
// to mutate them.
type stormMutationPart uint8

const (
	stormLastScannedMutable stormMutationPart = 1 << iota
	stormIslandReturnsMutable
)

// stormTargetGeneration is the tenant-private negative overlay on the shared
// Storm map. Ordinarily it is empty. A coordinate is added only after this
// account launches at that target and is removed by its next authoritative
// targeted observation. Shared observations therefore exist once per process
// while private action history cannot hide a target from another account.
type stormTargetGeneration struct {
	// Suppressions are normally empty or contain only an in-flight target, so a
	// sparse shard directory saves the former fixed 256 map headers per account.
	shards map[uint8]map[string]struct{}
}

func stormTargetGenerationFromKeys(keys []string) *stormTargetGeneration {
	generation := &stormTargetGeneration{shards: map[uint8]map[string]struct{}{}}
	for _, key := range keys {
		if key == "" {
			continue
		}
		shard := mapShardIndex(key)
		if generation.shards[shard] == nil {
			generation.shards[shard] = map[string]struct{}{}
		}
		generation.shards[shard][key] = struct{}{}
	}
	return generation
}

// initializeStormTargets compacts a legacy logical target map at the account
// runtime boundary. Legacy snapshots sometimes retained a newer target row
// only in Storm.Map.Targets, so migration first reconciles it into the official
// map domain. Current component snapshots restore only private suppressions.
func (state *GameState) initializeStormTargets() {
	if state == nil || state.stormTargets != nil {
		return
	}
	for key, observation := range state.Storm.Map.Targets {
		if key == "" || observation.KingdomID != stormKingdomID ||
			(observation.TypeID != MapTypeStormIsland && observation.TypeID != MapTypeStormFort) {
			continue
		}
		current, found := state.LookupMapObservation(stormKingdomID, key)
		if found && current.ObservedAt.After(observation.ObservedAt) {
			continue
		}
		if state.Map == nil {
			state.Map = WorldMap{}
		}
		if state.Map[stormKingdomID] == nil {
			state.Map[stormKingdomID] = map[string]MapObservation{}
		}
		state.Map[stormKingdomID][key] = observation
	}
	state.stormTargets = stormTargetGenerationFromKeys(state.Storm.Map.suppressedTargets)
	state.Storm.Map.Targets = nil
	state.Storm.Map.suppressedTargets = nil
}

func (state *GameState) prepareStormMutation(source GameState) {
	state.Storm = source.Storm
	state.Storm.Map.Targets = nil
	base := source.stormTargets
	if base == nil {
		base = stormTargetGenerationFromKeys(source.Storm.Map.suppressedTargets)
	}
	state.stormTargets = &stormTargetGeneration{shards: cloneMap(base.shards)}
	state.stormMutationCOW = true
	state.mutableStormParts = 0
	state.mutableStormTargetShards = [4]uint64{}
	state.pendingStormTargetChanges = map[string]struct{}{}
	state.replaceStormTargets = false
}

func (state *GameState) MutableStormLastScannedAt() map[CastleID]time.Time {
	if state == nil {
		return nil
	}
	if state.stormMutationCOW && state.mutableStormParts&stormLastScannedMutable == 0 {
		state.Storm.LastScannedAt = cloneMap(state.Storm.LastScannedAt)
		state.mutableStormParts |= stormLastScannedMutable
	}
	if state.Storm.LastScannedAt == nil {
		state.Storm.LastScannedAt = map[CastleID]time.Time{}
	}
	return state.Storm.LastScannedAt
}

func (state *GameState) MutableStormIslandReturns() map[string]StormIslandReturnState {
	if state == nil {
		return nil
	}
	if state.stormMutationCOW && state.mutableStormParts&stormIslandReturnsMutable == 0 {
		returns := make(map[string]StormIslandReturnState, len(state.Storm.IslandReturns))
		for key, operation := range state.Storm.IslandReturns {
			operation.Survivors = cloneMap(operation.Survivors)
			returns[key] = operation
		}
		state.Storm.IslandReturns = returns
		state.mutableStormParts |= stormIslandReturnsMutable
	}
	if state.Storm.IslandReturns == nil {
		state.Storm.IslandReturns = map[string]StormIslandReturnState{}
	}
	return state.Storm.IslandReturns
}

func (state GameState) stormTargetSuppressed(key string) bool {
	if state.stormTargets == nil || key == "" {
		return false
	}
	_, found := state.stormTargets.shards[mapShardIndex(key)][key]
	return found
}

func (state GameState) hasStormTargetKey(key string) bool {
	if key == "" || state.stormTargetSuppressed(key) {
		return false
	}
	if observation, found := state.Storm.Map.Targets[key]; found {
		return observation.TypeID == MapTypeStormIsland || observation.TypeID == MapTypeStormFort
	}
	observation, found := state.LookupMapObservation(stormKingdomID, key)
	return found && (observation.TypeID == MapTypeStormIsland || observation.TypeID == MapTypeStormFort)
}

// LookupStormTarget resolves one authoritative target through the official map
// domain. Callers see the same MapObservation model as the game payload while
// tenant storage keeps only one physical observation copy.
func (state GameState) LookupStormTarget(key string) (MapObservation, bool) {
	if observation, found := state.Storm.Map.Targets[key]; found {
		return observation, true
	}
	if !state.hasStormTargetKey(key) {
		return MapObservation{}, false
	}
	observation, found := state.LookupMapObservation(stormKingdomID, key)
	if !found || (observation.TypeID != MapTypeStormIsland && observation.TypeID != MapTypeStormFort) {
		return MapObservation{}, false
	}
	return observation, true
}

// RangeStormMapObservations traverses the official Storm feature partition,
// including account-suppressed targets. Full-scan reconciliation uses this
// view; attack selection uses RangeStormTargets below.
func (state GameState) RangeStormMapObservations(visit func(string, MapObservation) bool) {
	if visit == nil {
		return
	}
	var seen map[string]struct{}
	if len(state.Storm.Map.Targets) > 0 {
		seen = make(map[string]struct{}, len(state.Storm.Map.Targets))
	}
	for key, observation := range state.Storm.Map.Targets {
		if current, found := state.LookupMapObservation(stormKingdomID, key); found && current.ObservedAt.After(observation.ObservedAt) {
			observation = current
		}
		seen[key] = struct{}{}
		if !visit(key, observation) {
			return
		}
	}
	state.RangeMapObservationsByKind(stormKingdomID, MapProjectionStorm, func(key string, observation MapObservation) bool {
		if seen != nil {
			if _, duplicate := seen[key]; duplicate {
				return true
			}
		}
		return visit(key, observation)
	})
}

func (state GameState) RangeStormTargets(visit func(string, MapObservation) bool) {
	if visit == nil {
		return
	}
	state.RangeStormMapObservations(func(key string, observation MapObservation) bool {
		if state.stormTargetSuppressed(key) {
			return true
		}
		return visit(key, observation)
	})
}

func (state GameState) StormTargetCount() int {
	count := 0
	state.RangeStormTargets(func(string, MapObservation) bool {
		count++
		return true
	})
	return count
}

func (state GameState) materializedStormTargets() map[string]MapObservation {
	targets := map[string]MapObservation{}
	state.RangeStormTargets(func(key string, observation MapObservation) bool {
		targets[key] = observation
		return true
	})
	return targets
}

func (state *GameState) mutableStormTargetShard(key string) map[string]struct{} {
	if state == nil || key == "" {
		return nil
	}
	if state.stormTargets == nil {
		state.stormTargets = stormTargetGenerationFromKeys(state.Storm.Map.suppressedTargets)
		state.Storm.Map.Targets = nil
		state.Storm.Map.suppressedTargets = nil
	}
	shard := mapShardIndex(key)
	if state.stormTargets.shards == nil {
		state.stormTargets.shards = map[uint8]map[string]struct{}{}
	}
	word, bit := shard/64, uint(shard%64)
	if !state.stormMutationCOW || state.mutableStormTargetShards[word]&(uint64(1)<<bit) == 0 {
		state.stormTargets.shards[shard] = cloneMap(state.stormTargets.shards[shard])
		if state.stormMutationCOW {
			state.mutableStormTargetShards[word] |= uint64(1) << bit
		}
	}
	if state.stormTargets.shards[shard] == nil {
		state.stormTargets.shards[shard] = map[string]struct{}{}
	}
	return state.stormTargets.shards[shard]
}

func stormTargetKey(observation MapObservation) string {
	return fmt.Sprintf("%d:%d", observation.X, observation.Y)
}

// SetStormTarget clears this account's negative overlay after a fresh map row.
// The observation itself must already have been retained in shared map state.
func (state *GameState) SetStormTarget(observation MapObservation) bool {
	if state == nil || observation.KingdomID != stormKingdomID ||
		(observation.TypeID != MapTypeStormIsland && observation.TypeID != MapTypeStormFort) {
		return false
	}
	key := stormTargetKey(observation)
	if !state.stormTargetSuppressed(key) {
		return false
	}
	delete(state.mutableStormTargetShard(key), key)
	if state.stormMutationCOW && !state.replaceStormTargets {
		state.pendingStormTargetChanges[key] = struct{}{}
	}
	return true
}

func (state *GameState) DeleteStormTarget(key string) bool {
	if state == nil || !state.hasStormTargetKey(key) {
		return false
	}
	state.mutableStormTargetShard(key)[key] = struct{}{}
	if state.stormMutationCOW && !state.replaceStormTargets {
		state.pendingStormTargetChanges[key] = struct{}{}
	}
	return true
}

// RefreshStormTargetObservation preserves the public, game-shaped value map
// for standalone reducer tests and legacy callers. Compact tenant generations
// retain membership only, so their latest value is already supplied by the
// official map update and this method performs no duplicate write.
func (state *GameState) RefreshStormTargetObservation(observation MapObservation) bool {
	if state == nil || observation.KingdomID != stormKingdomID {
		return false
	}
	key := stormTargetKey(observation)
	if current, legacy := state.Storm.Map.Targets[key]; legacy {
		if current.ObservedAt.After(observation.ObservedAt) || current == observation {
			return false
		}
		if observation.TypeID != MapTypeStormIsland && observation.TypeID != MapTypeStormFort {
			delete(state.Storm.Map.Targets, key)
			return true
		}
		state.Storm.Map.Targets[key] = observation
		return true
	}
	if observation.TypeID != MapTypeStormIsland && observation.TypeID != MapTypeStormFort {
		return state.DeleteStormTarget(key)
	}
	if state.stormTargetSuppressed(key) {
		return state.SetStormTarget(observation)
	}
	return false
}

func (state *GameState) ReplaceStormMap(value StormMapState) {
	if state == nil {
		return
	}
	targets := value.Targets
	value.Targets = nil
	value.suppressedTargets = nil
	state.Storm.Map = value
	state.stormTargets = stormTargetGenerationFromKeys(nil)
	for _, observation := range targets {
		state.SetMapObservation(observation)
	}
	if state.stormMutationCOW {
		state.replaceStormTargets = true
		state.pendingStormTargetChanges = map[string]struct{}{}
		state.mutableStormTargetShards = [4]uint64{}
	}
}

func (state GameState) stormTargetChangeKeys() []string {
	keys := make([]string, 0, len(state.pendingStormTargetChanges))
	for key := range state.pendingStormTargetChanges {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func (state GameState) stormTargetKeys() []string {
	keys := []string{}
	if state.stormTargets != nil {
		for _, shard := range state.stormTargets.shards {
			for key := range shard {
				keys = append(keys, key)
			}
		}
	}
	sort.Strings(keys)
	return keys
}
