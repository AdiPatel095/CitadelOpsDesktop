package State

import (
	"sort"
	"time"
)

func (state *GameState) prepareTowerQueueMutation(source GameState) {
	state.TowerQueue = source.TowerQueue
	state.TowerQueue.EntriesByCastle = cloneMap(source.TowerQueue.EntriesByCastle)
	state.TowerQueue.LastScannedAt = cloneMap(source.TowerQueue.LastScannedAt)
	state.TowerQueue.LastAttemptedAt = cloneMap(source.TowerQueue.LastAttemptedAt)
	state.TowerQueue.ConfirmedLaunchesByCastle = cloneMap(source.TowerQueue.ConfirmedLaunchesByCastle)
	state.TowerQueue.CapacityByCastle = cloneMap(source.TowerQueue.CapacityByCastle)
	state.towerQueueMutationCOW = true
	state.mutableTowerQueueEntries = map[CastleID]struct{}{}
	state.pendingTowerQueueCastles = map[CastleID]struct{}{}
	state.replaceTowerQueue = false
}

func cloneTowerQueueEntries(source []TowerQueueEntry) []TowerQueueEntry {
	entries := append([]TowerQueueEntry(nil), source...)
	for index := range entries {
		entries[index].DeferredUntil = cloneTimePointer(entries[index].DeferredUntil)
	}
	return entries
}

func (state *GameState) MutableTowerQueueEntries(castleID CastleID) []TowerQueueEntry {
	if state == nil || castleID <= 0 {
		return nil
	}
	if state.TowerQueue.EntriesByCastle == nil {
		state.TowerQueue.EntriesByCastle = map[CastleID][]TowerQueueEntry{}
	}
	if state.towerQueueMutationCOW {
		if _, owned := state.mutableTowerQueueEntries[castleID]; !owned {
			state.TowerQueue.EntriesByCastle[castleID] = cloneTowerQueueEntries(
				state.TowerQueue.EntriesByCastle[castleID],
			)
			state.mutableTowerQueueEntries[castleID] = struct{}{}
		}
		state.pendingTowerQueueCastles[castleID] = struct{}{}
	}
	return state.TowerQueue.EntriesByCastle[castleID]
}

func (state *GameState) SetTowerQueueEntries(castleID CastleID, entries []TowerQueueEntry) {
	if state == nil || castleID <= 0 {
		return
	}
	if state.TowerQueue.EntriesByCastle == nil {
		state.TowerQueue.EntriesByCastle = map[CastleID][]TowerQueueEntry{}
	}
	state.TowerQueue.EntriesByCastle[castleID] = entries
	state.markTowerQueueCastle(castleID)
	if state.towerQueueMutationCOW {
		state.mutableTowerQueueEntries[castleID] = struct{}{}
	}
}

func (state *GameState) SetTowerQueueLastScannedAt(castleID CastleID, value time.Time) {
	if state == nil || castleID <= 0 {
		return
	}
	if state.TowerQueue.LastScannedAt == nil {
		state.TowerQueue.LastScannedAt = map[CastleID]time.Time{}
	}
	state.TowerQueue.LastScannedAt[castleID] = value
	state.markTowerQueueCastle(castleID)
}

func (state *GameState) SetTowerQueueLastAttemptedAt(castleID CastleID, value time.Time) {
	if state == nil || castleID <= 0 {
		return
	}
	if state.TowerQueue.LastAttemptedAt == nil {
		state.TowerQueue.LastAttemptedAt = map[CastleID]time.Time{}
	}
	state.TowerQueue.LastAttemptedAt[castleID] = value
	state.markTowerQueueCastle(castleID)
}

func (state *GameState) IncrementTowerQueueConfirmedLaunches(castleID CastleID) {
	if state == nil || castleID <= 0 {
		return
	}
	if state.TowerQueue.ConfirmedLaunchesByCastle == nil {
		state.TowerQueue.ConfirmedLaunchesByCastle = map[CastleID]int64{}
	}
	state.TowerQueue.ConfirmedLaunchesByCastle[castleID]++
	state.markTowerQueueCastle(castleID)
}

func (state *GameState) SetTowerQueueCapacity(castleID CastleID, value TowerCapacityObservation) bool {
	if state == nil || castleID <= 0 {
		return false
	}
	if state.TowerQueue.CapacityByCastle == nil {
		state.TowerQueue.CapacityByCastle = map[CastleID]TowerCapacityObservation{}
	}
	if state.TowerQueue.CapacityByCastle[castleID] == value {
		return false
	}
	state.TowerQueue.CapacityByCastle[castleID] = value
	state.markTowerQueueCastle(castleID)
	return true
}

func (state *GameState) markTowerQueueCastle(castleID CastleID) {
	if state != nil && state.towerQueueMutationCOW && !state.replaceTowerQueue && castleID > 0 {
		state.pendingTowerQueueCastles[castleID] = struct{}{}
	}
}

func (state *GameState) ReplaceTowerQueue(value TowerQueueState) {
	if state == nil {
		return
	}
	state.TowerQueue = value
	if state.towerQueueMutationCOW {
		state.replaceTowerQueue = true
		state.pendingTowerQueueCastles = map[CastleID]struct{}{}
		state.mutableTowerQueueEntries = map[CastleID]struct{}{}
	}
}

func (state GameState) towerQueueChangeCastleIDs() []CastleID {
	ids := make([]CastleID, 0, len(state.pendingTowerQueueCastles))
	for id := range state.pendingTowerQueueCastles {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(left, right int) bool { return ids[left] < ids[right] })
	return ids
}

func (state GameState) towerQueueCastleIDs() []CastleID {
	set := map[CastleID]struct{}{}
	for id := range state.TowerQueue.EntriesByCastle {
		set[id] = struct{}{}
	}
	for id := range state.TowerQueue.LastScannedAt {
		set[id] = struct{}{}
	}
	for id := range state.TowerQueue.LastAttemptedAt {
		set[id] = struct{}{}
	}
	for id := range state.TowerQueue.ConfirmedLaunchesByCastle {
		set[id] = struct{}{}
	}
	for id := range state.TowerQueue.CapacityByCastle {
		set[id] = struct{}{}
	}
	ids := make([]CastleID, 0, len(set))
	for id := range set {
		if id > 0 {
			ids = append(ids, id)
		}
	}
	sort.Slice(ids, func(left, right int) bool { return ids[left] < ids[right] })
	return ids
}
