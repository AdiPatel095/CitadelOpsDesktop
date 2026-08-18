package State

import (
	"reflect"
	"sort"
	"time"
)

const movementShardCount = 256

type movementGeneration struct {
	shards [movementShardCount]map[MovementID]MovementState
}

func movementShardIndex(id MovementID) uint8 {
	value := uint64(id)
	value ^= value >> 33
	value *= 0xff51afd7ed558ccd
	value ^= value >> 33
	return uint8(value)
}

func movementGenerationFromMap(source map[MovementID]MovementState) *movementGeneration {
	generation := &movementGeneration{}
	for id, movement := range source {
		if id <= 0 {
			continue
		}
		shard := movementShardIndex(id)
		if generation.shards[shard] == nil {
			generation.shards[shard] = map[MovementID]MovementState{}
		}
		generation.shards[shard][id] = movement
	}
	return generation
}

func (state *GameState) initializeMovements() {
	if state == nil || state.movementRecords != nil {
		return
	}
	state.movementRecords = movementGenerationFromMap(state.Movements)
	state.Movements = nil
}

func (state *GameState) prepareMovementMutation(source GameState) {
	base := source.movementRecords
	if base == nil {
		base = movementGenerationFromMap(source.Movements)
	}
	generation := *base
	state.movementRecords = &generation
	state.Movements = nil
	state.movementMutationCOW = true
	state.mutableMovementShards = [4]uint64{}
	state.pendingMovementChanges = map[MovementID]struct{}{}
	state.replaceMovements = false
}

func (state *GameState) mutableMovementShard(id MovementID) map[MovementID]MovementState {
	if state.movementRecords == nil {
		state.movementRecords = movementGenerationFromMap(state.Movements)
		state.Movements = nil
	}
	shard := movementShardIndex(id)
	word, bit := shard/64, uint(shard%64)
	if !state.movementMutationCOW || state.mutableMovementShards[word]&(uint64(1)<<bit) == 0 {
		state.movementRecords.shards[shard] = cloneMap(state.movementRecords.shards[shard])
		if state.movementMutationCOW {
			state.mutableMovementShards[word] |= uint64(1) << bit
		}
	}
	if state.movementRecords.shards[shard] == nil {
		state.movementRecords.shards[shard] = map[MovementID]MovementState{}
	}
	return state.movementRecords.shards[shard]
}

func (state GameState) LookupMovement(id MovementID) (MovementState, bool) {
	if id <= 0 {
		return MovementState{}, false
	}
	if state.movementRecords != nil {
		movement, found := state.movementRecords.shards[movementShardIndex(id)][id]
		return movement, found
	}
	movement, found := state.Movements[id]
	return movement, found
}

func (state GameState) RangeMovements(visit func(MovementID, MovementState) bool) {
	if visit == nil {
		return
	}
	if state.movementRecords == nil {
		for id, movement := range state.Movements {
			if !visit(id, movement) {
				return
			}
		}
		return
	}
	for _, shard := range state.movementRecords.shards {
		for id, movement := range shard {
			if !visit(id, movement) {
				return
			}
		}
	}
}

func (state GameState) MovementCount() int {
	count := 0
	state.RangeMovements(func(_ MovementID, _ MovementState) bool {
		count++
		return true
	})
	return count
}

func (state *GameState) markMovement(id MovementID) {
	if state != nil && state.movementMutationCOW && !state.replaceMovements && id > 0 {
		state.pendingMovementChanges[id] = struct{}{}
	}
}

func (state *GameState) SetMovement(id MovementID, movement MovementState) {
	if state == nil || id <= 0 {
		return
	}
	if !state.movementMutationCOW && state.movementRecords == nil {
		if state.Movements == nil {
			state.Movements = map[MovementID]MovementState{}
		}
		state.Movements[id] = movement
		return
	}
	state.mutableMovementShard(id)[id] = movement
	state.markMovement(id)
}

func (state *GameState) DeleteMovement(id MovementID) {
	if state == nil || id <= 0 {
		return
	}
	if !state.movementMutationCOW && state.movementRecords == nil {
		delete(state.Movements, id)
		return
	}
	delete(state.mutableMovementShard(id), id)
	state.markMovement(id)
}

func (state *GameState) ReplaceMovements(value map[MovementID]MovementState) bool {
	if state == nil {
		return false
	}
	if !state.movementMutationCOW && state.movementRecords == nil {
		changed := !reflect.DeepEqual(state.Movements, value)
		state.Movements = value
		return changed
	}
	if state.movementMutationCOW {
		changed := false
		for id, movement := range value {
			current, found := state.LookupMovement(id)
			if found && reflect.DeepEqual(current, movement) {
				continue
			}
			state.SetMovement(id, movement)
			changed = true
		}
		removals := []MovementID{}
		state.RangeMovements(func(id MovementID, _ MovementState) bool {
			if _, retained := value[id]; !retained {
				removals = append(removals, id)
			}
			return true
		})
		for _, id := range removals {
			state.DeleteMovement(id)
			changed = true
		}
		state.Movements = nil
		return changed
	}
	state.movementRecords = movementGenerationFromMap(value)
	state.Movements = nil
	if state.movementMutationCOW {
		state.replaceMovements = true
		state.pendingMovementChanges = map[MovementID]struct{}{}
		state.mutableMovementShards = [4]uint64{}
	}
	return true
}

func cloneMovementState(movement MovementState) MovementState {
	movement.Units = cloneMap(movement.Units)
	movement.MarketGoods = append([]KingdomTransportGood(nil), movement.MarketGoods...)
	movement.ArrivesAt = cloneTimePointer(movement.ArrivesAt)
	movement.ReturnsAt = cloneTimePointer(movement.ReturnsAt)
	movement.CommanderID = cloneCommanderIDPointer(movement.CommanderID)
	return movement
}

func (state GameState) materializedMovements() map[MovementID]MovementState {
	result := make(map[MovementID]MovementState, state.MovementCount())
	state.RangeMovements(func(id MovementID, movement MovementState) bool {
		result[id] = cloneMovementState(movement)
		return true
	})
	return result
}

func (state GameState) movementViewMap() map[MovementID]MovementState {
	result := make(map[MovementID]MovementState, state.MovementCount())
	state.RangeMovements(func(id MovementID, movement MovementState) bool {
		result[id] = movement
		return true
	})
	return result
}

// MovementViewMap is a transient immutable-value index for algorithms whose
// official-game reconciliation genuinely needs set algebra over all movements.
// It copies only the small map index, never movement payloads.
func (state GameState) MovementViewMap() map[MovementID]MovementState {
	return state.movementViewMap()
}

func (state GameState) movementChangeIDs() []MovementID {
	ids := make([]MovementID, 0, len(state.pendingMovementChanges))
	for id := range state.pendingMovementChanges {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(left, right int) bool { return ids[left] < ids[right] })
	return ids
}

func (operation StationingOperation) ActiveInState(state GameState, now time.Time) bool {
	if operation.MovementID > 0 {
		movement, found := state.LookupMovement(operation.MovementID)
		return found && operation.MatchesMovement(movement) && StationMovementActiveAt(movement, now)
	}
	active := false
	state.RangeMovements(func(_ MovementID, movement MovementState) bool {
		if operation.MatchesMovement(movement) && StationMovementActiveAt(movement, now) {
			active = true
			return false
		}
		return true
	})
	if active {
		return true
	}
	if operation.SuccessCooldownUntil != nil && operation.SuccessCooldownUntil.After(now) {
		return true
	}
	return !operation.UpdatedAt.IsZero() && now.Before(operation.UpdatedAt.Add(30*time.Second))
}
