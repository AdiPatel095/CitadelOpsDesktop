package State

import "sort"

const towerCooldownShardCount = 256

type towerCooldownGeneration struct {
	shards [towerCooldownShardCount]map[string]TowerCooldownState
}

func towerCooldownGenerationFromMap(source map[string]TowerCooldownState) *towerCooldownGeneration {
	generation := &towerCooldownGeneration{}
	for key, cooldown := range source {
		if key == "" {
			continue
		}
		shard := mapShardIndex(key)
		if generation.shards[shard] == nil {
			generation.shards[shard] = map[string]TowerCooldownState{}
		}
		generation.shards[shard][key] = cooldown
	}
	return generation
}

func (state *GameState) initializeTowerCooldowns() {
	if state == nil || state.towerCooldowns != nil {
		return
	}
	state.towerCooldowns = towerCooldownGenerationFromMap(state.TowerCooldowns)
	state.TowerCooldowns = nil
}

func (state *GameState) prepareTowerCooldownMutation(source GameState) {
	base := source.towerCooldowns
	if base == nil {
		base = towerCooldownGenerationFromMap(source.TowerCooldowns)
	}
	generation := *base
	state.TowerCooldowns = nil
	state.towerCooldowns = &generation
	state.towerCooldownMutationCOW = true
	state.mutableTowerCooldownShards = [4]uint64{}
	state.pendingTowerCooldownChanges = map[string]struct{}{}
	state.replaceTowerCooldowns = false
}

func (state GameState) LookupTowerCooldown(key string) (TowerCooldownState, bool) {
	if cooldown, found := state.TowerCooldowns[key]; found {
		return cooldown, true
	}
	if state.towerCooldowns == nil || key == "" {
		return TowerCooldownState{}, false
	}
	cooldown, found := state.towerCooldowns.shards[mapShardIndex(key)][key]
	return cooldown, found
}

func (state GameState) RangeTowerCooldowns(visit func(string, TowerCooldownState) bool) {
	if visit == nil {
		return
	}
	seen := map[string]struct{}{}
	for key, cooldown := range state.TowerCooldowns {
		seen[key] = struct{}{}
		if !visit(key, cooldown) {
			return
		}
	}
	if state.towerCooldowns == nil {
		return
	}
	for _, shard := range state.towerCooldowns.shards {
		for key, cooldown := range shard {
			if _, duplicate := seen[key]; duplicate {
				continue
			}
			if !visit(key, cooldown) {
				return
			}
		}
	}
}

func (state GameState) TowerCooldownCount() int {
	count := 0
	state.RangeTowerCooldowns(func(string, TowerCooldownState) bool {
		count++
		return true
	})
	return count
}

func (state GameState) materializedTowerCooldowns() map[string]TowerCooldownState {
	result := map[string]TowerCooldownState{}
	state.RangeTowerCooldowns(func(key string, cooldown TowerCooldownState) bool {
		result[key] = cooldown
		return true
	})
	return result
}

func (state *GameState) mutableTowerCooldownShard(key string) map[string]TowerCooldownState {
	if state.towerCooldowns == nil {
		state.towerCooldowns = towerCooldownGenerationFromMap(state.TowerCooldowns)
		state.TowerCooldowns = nil
	}
	shard := mapShardIndex(key)
	word, bit := shard/64, uint(shard%64)
	if !state.towerCooldownMutationCOW || state.mutableTowerCooldownShards[word]&(uint64(1)<<bit) == 0 {
		state.towerCooldowns.shards[shard] = cloneMap(state.towerCooldowns.shards[shard])
		if state.towerCooldownMutationCOW {
			state.mutableTowerCooldownShards[word] |= uint64(1) << bit
		}
	}
	if state.towerCooldowns.shards[shard] == nil {
		state.towerCooldowns.shards[shard] = map[string]TowerCooldownState{}
	}
	return state.towerCooldowns.shards[shard]
}

func (state *GameState) SetTowerCooldown(key string, cooldown TowerCooldownState) bool {
	if state == nil || key == "" {
		return false
	}
	if current, found := state.LookupTowerCooldown(key); found && current == cooldown {
		return false
	}
	if !state.towerCooldownMutationCOW && state.towerCooldowns == nil {
		if state.TowerCooldowns == nil {
			state.TowerCooldowns = map[string]TowerCooldownState{}
		}
		state.TowerCooldowns[key] = cooldown
		return true
	}
	state.mutableTowerCooldownShard(key)[key] = cooldown
	if !state.replaceTowerCooldowns {
		state.pendingTowerCooldownChanges[key] = struct{}{}
	}
	return true
}

func (state *GameState) DeleteTowerCooldown(key string) bool {
	if state == nil || key == "" {
		return false
	}
	if _, found := state.LookupTowerCooldown(key); !found {
		return false
	}
	if !state.towerCooldownMutationCOW && state.towerCooldowns == nil {
		delete(state.TowerCooldowns, key)
		return true
	}
	delete(state.mutableTowerCooldownShard(key), key)
	if !state.replaceTowerCooldowns {
		state.pendingTowerCooldownChanges[key] = struct{}{}
	}
	return true
}

func (state *GameState) ReplaceTowerCooldowns(values map[string]TowerCooldownState) {
	if state == nil {
		return
	}
	if !state.towerCooldownMutationCOW && state.towerCooldowns == nil {
		state.TowerCooldowns = values
		return
	}
	state.TowerCooldowns = nil
	state.towerCooldowns = towerCooldownGenerationFromMap(values)
	state.replaceTowerCooldowns = true
	state.pendingTowerCooldownChanges = map[string]struct{}{}
	state.mutableTowerCooldownShards = [4]uint64{}
}

func (state GameState) towerCooldownChangeKeys() []string {
	keys := make([]string, 0, len(state.pendingTowerCooldownChanges))
	for key := range state.pendingTowerCooldownChanges {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func (state GameState) rangeTowerCooldownShards(visit func(uint8)) {
	if visit == nil {
		return
	}
	seen := [4]uint64{}
	state.RangeTowerCooldowns(func(key string, _ TowerCooldownState) bool {
		shard := mapShardIndex(key)
		word, bit := shard/64, uint(shard%64)
		if seen[word]&(uint64(1)<<bit) == 0 {
			seen[word] |= uint64(1) << bit
			visit(shard)
		}
		return true
	})
}

func (state GameState) towerCooldownShard(shard uint8) map[string]TowerCooldownState {
	result := map[string]TowerCooldownState{}
	state.RangeTowerCooldowns(func(key string, cooldown TowerCooldownState) bool {
		if mapShardIndex(key) == shard {
			result[key] = cooldown
		}
		return true
	})
	return result
}
