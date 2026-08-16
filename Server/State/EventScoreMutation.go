package State

import (
	"maps"
	"sort"
	"time"
)

const eventScoreShardCount = 256

// eventScoreRecord keeps the three official per-event projections together in
// tenant storage. The public EventScoreState remains unchanged; only the
// physical representation is sharded so one event update never copies every
// other event's Go map buckets.
type eventScoreRecord struct {
	Score       ScalableEventScore
	Activity    EventActivityState
	Ranking     EventRankingState
	HasScore    bool
	HasActivity bool
	HasRanking  bool
}

type eventScoreGeneration struct {
	shards [eventScoreShardCount]map[int64]eventScoreRecord
}

func eventScoreShardIndex(eventID int64) uint8 {
	value := uint64(eventID)
	value ^= value >> 33
	value *= 0xff51afd7ed558ccd
	value ^= value >> 33
	return uint8(value)
}

func eventScoreGenerationFromState(source EventScoreState) *eventScoreGeneration {
	generation := &eventScoreGeneration{}
	set := func(eventID int64, update func(*eventScoreRecord)) {
		if eventID <= 0 {
			return
		}
		shard := eventScoreShardIndex(eventID)
		if generation.shards[shard] == nil {
			generation.shards[shard] = map[int64]eventScoreRecord{}
		}
		record := generation.shards[shard][eventID]
		update(&record)
		generation.shards[shard][eventID] = record
	}
	for eventID, score := range source.ByEvent {
		value := score
		set(eventID, func(record *eventScoreRecord) { record.Score, record.HasScore = value, true })
	}
	for eventID, activity := range source.ActivityByEvent {
		value := activity
		set(eventID, func(record *eventScoreRecord) { record.Activity, record.HasActivity = value, true })
	}
	for eventID, ranking := range source.RankingByEvent {
		value := ranking
		set(eventID, func(record *eventScoreRecord) { record.Ranking, record.HasRanking = value, true })
	}
	return generation
}

func (state *GameState) initializeEventScores() {
	if state == nil || state.eventScoreRecords != nil {
		return
	}
	state.eventScoreRecords = eventScoreGenerationFromState(state.EventScores)
	state.EventScores.ByEvent = nil
	state.EventScores.ActivityByEvent = nil
	state.EventScores.RankingByEvent = nil
}

func (state *GameState) prepareEventScoreMutation(source GameState) {
	state.EventScores = source.EventScores
	base := source.eventScoreRecords
	if base == nil {
		base = eventScoreGenerationFromState(source.EventScores)
	}
	generation := *base
	state.eventScoreRecords = &generation
	state.EventScores.ByEvent = nil
	state.EventScores.ActivityByEvent = nil
	state.EventScores.RankingByEvent = nil
	state.eventScoreMutationCOW = true
	state.mutableEventScoreShards = [4]uint64{}
	state.pendingEventScoreIDs = map[int64]struct{}{}
	state.eventScoreMetadataDirty = false
	state.eventScoreShopDirty = false
	state.replaceEventScores = false
}

func cloneEventActivity(activity EventActivityState) EventActivityState {
	activity.FortifyCurrencies = append([]string(nil), activity.FortifyCurrencies...)
	activity.LaunchIDs = append([]MovementID(nil), activity.LaunchIDs...)
	activity.PendingAttacks = append([]EventAttackRecord(nil), activity.PendingAttacks...)
	activity.ProcessedReportIDs = append([]int64(nil), activity.ProcessedReportIDs...)
	return activity
}

func cloneEventRanking(ranking EventRankingState) EventRankingState {
	ranking.Entries = append([]EventRankingEntry(nil), ranking.Entries...)
	return ranking
}

func (state GameState) eventScoreRecord(eventID int64) (eventScoreRecord, bool) {
	if eventID <= 0 {
		return eventScoreRecord{}, false
	}
	if state.eventScoreRecords != nil {
		record, found := state.eventScoreRecords.shards[eventScoreShardIndex(eventID)][eventID]
		return record, found
	}
	record := eventScoreRecord{}
	if score, found := state.EventScores.ByEvent[eventID]; found {
		record.Score, record.HasScore = score, true
	}
	if activity, found := state.EventScores.ActivityByEvent[eventID]; found {
		record.Activity, record.HasActivity = activity, true
	}
	if ranking, found := state.EventScores.RankingByEvent[eventID]; found {
		record.Ranking, record.HasRanking = ranking, true
	}
	return record, record.HasScore || record.HasActivity || record.HasRanking
}

func (state *GameState) mutableEventScoreRecord(eventID int64) eventScoreRecord {
	if state.eventScoreRecords == nil {
		state.eventScoreRecords = eventScoreGenerationFromState(state.EventScores)
		state.EventScores.ByEvent = nil
		state.EventScores.ActivityByEvent = nil
		state.EventScores.RankingByEvent = nil
	}
	shard := eventScoreShardIndex(eventID)
	word, bit := shard/64, uint(shard%64)
	if !state.eventScoreMutationCOW || state.mutableEventScoreShards[word]&(uint64(1)<<bit) == 0 {
		state.eventScoreRecords.shards[shard] = cloneMap(state.eventScoreRecords.shards[shard])
		if state.eventScoreMutationCOW {
			state.mutableEventScoreShards[word] |= uint64(1) << bit
		}
	}
	if state.eventScoreRecords.shards[shard] == nil {
		state.eventScoreRecords.shards[shard] = map[int64]eventScoreRecord{}
	}
	return state.eventScoreRecords.shards[shard][eventID]
}

func (state *GameState) storeEventScoreRecord(eventID int64, record eventScoreRecord) {
	shard := eventScoreShardIndex(eventID)
	if !record.HasScore && !record.HasActivity && !record.HasRanking {
		delete(state.eventScoreRecords.shards[shard], eventID)
	} else {
		state.eventScoreRecords.shards[shard][eventID] = record
	}
	state.markEventScore(eventID)
}

func (state GameState) LookupScalableEventScore(eventID int64) (ScalableEventScore, bool) {
	record, found := state.eventScoreRecord(eventID)
	return record.Score, found && record.HasScore
}

func (state GameState) LookupEventActivity(eventID int64) (EventActivityState, bool) {
	record, found := state.eventScoreRecord(eventID)
	return record.Activity, found && record.HasActivity
}

func (state GameState) LookupEventRanking(eventID int64) (EventRankingState, bool) {
	record, found := state.eventScoreRecord(eventID)
	return record.Ranking, found && record.HasRanking
}

func (state GameState) RangeScalableEventScores(visit func(int64, ScalableEventScore) bool) {
	state.rangeEventScoreRecords(func(eventID int64, record eventScoreRecord) bool {
		return !record.HasScore || visit(eventID, record.Score)
	})
}

func (state GameState) RangeEventActivities(visit func(int64, EventActivityState) bool) {
	state.rangeEventScoreRecords(func(eventID int64, record eventScoreRecord) bool {
		return !record.HasActivity || visit(eventID, record.Activity)
	})
}

func (state GameState) RangeEventRankings(visit func(int64, EventRankingState) bool) {
	state.rangeEventScoreRecords(func(eventID int64, record eventScoreRecord) bool {
		return !record.HasRanking || visit(eventID, record.Ranking)
	})
}

func (state GameState) rangeEventScoreRecords(visit func(int64, eventScoreRecord) bool) {
	if visit == nil {
		return
	}
	generation := state.eventScoreRecords
	if generation == nil {
		generation = eventScoreGenerationFromState(state.EventScores)
	}
	for _, shard := range generation.shards {
		for eventID, record := range shard {
			if !visit(eventID, record) {
				return
			}
		}
	}
}

func (state *GameState) markEventScore(eventID int64) {
	if state != nil && state.eventScoreMutationCOW && !state.replaceEventScores && eventID > 0 {
		state.pendingEventScoreIDs[eventID] = struct{}{}
	}
}

func (state *GameState) SetScalableEventScore(eventID int64, score ScalableEventScore) {
	if state == nil || eventID <= 0 {
		return
	}
	if !state.eventScoreMutationCOW && state.eventScoreRecords == nil {
		if state.EventScores.ByEvent == nil {
			state.EventScores.ByEvent = map[int64]ScalableEventScore{}
		}
		state.EventScores.ByEvent[eventID] = score
		return
	}
	record := state.mutableEventScoreRecord(eventID)
	record.Score, record.HasScore = score, true
	state.storeEventScoreRecord(eventID, record)
}

func (state *GameState) DeleteScalableEventScore(eventID int64) {
	if state == nil || eventID <= 0 {
		return
	}
	if !state.eventScoreMutationCOW && state.eventScoreRecords == nil {
		delete(state.EventScores.ByEvent, eventID)
		return
	}
	record := state.mutableEventScoreRecord(eventID)
	record.Score, record.HasScore = ScalableEventScore{}, false
	state.storeEventScoreRecord(eventID, record)
}

func (state *GameState) MutableEventActivity(eventID int64) (EventActivityState, bool) {
	activity, found := state.LookupEventActivity(eventID)
	if !found {
		return EventActivityState{}, false
	}
	return cloneEventActivity(activity), true
}

func (state *GameState) SetEventActivity(eventID int64, activity EventActivityState) {
	if state == nil || eventID <= 0 {
		return
	}
	if !state.eventScoreMutationCOW && state.eventScoreRecords == nil {
		if state.EventScores.ActivityByEvent == nil {
			state.EventScores.ActivityByEvent = map[int64]EventActivityState{}
		}
		state.EventScores.ActivityByEvent[eventID] = activity
		return
	}
	record := state.mutableEventScoreRecord(eventID)
	record.Activity, record.HasActivity = activity, true
	state.storeEventScoreRecord(eventID, record)
}

func (state *GameState) DeleteEventActivity(eventID int64) {
	if state == nil || eventID <= 0 {
		return
	}
	if !state.eventScoreMutationCOW && state.eventScoreRecords == nil {
		delete(state.EventScores.ActivityByEvent, eventID)
		return
	}
	record := state.mutableEventScoreRecord(eventID)
	record.Activity, record.HasActivity = EventActivityState{}, false
	state.storeEventScoreRecord(eventID, record)
}

func (state *GameState) MutableEventRanking(eventID int64) (EventRankingState, bool) {
	ranking, found := state.LookupEventRanking(eventID)
	if !found {
		return EventRankingState{}, false
	}
	return cloneEventRanking(ranking), true
}

func (state *GameState) SetEventRanking(eventID int64, ranking EventRankingState) {
	if state == nil || eventID <= 0 {
		return
	}
	if !state.eventScoreMutationCOW && state.eventScoreRecords == nil {
		if state.EventScores.RankingByEvent == nil {
			state.EventScores.RankingByEvent = map[int64]EventRankingState{}
		}
		state.EventScores.RankingByEvent[eventID] = ranking
		return
	}
	record := state.mutableEventScoreRecord(eventID)
	record.Ranking, record.HasRanking = ranking, true
	state.storeEventScoreRecord(eventID, record)
}

func (state *GameState) DeleteEventRanking(eventID int64) {
	if state == nil || eventID <= 0 {
		return
	}
	if !state.eventScoreMutationCOW && state.eventScoreRecords == nil {
		delete(state.EventScores.RankingByEvent, eventID)
		return
	}
	record := state.mutableEventScoreRecord(eventID)
	record.Ranking, record.HasRanking = EventRankingState{}, false
	state.storeEventScoreRecord(eventID, record)
}

func (state *GameState) SetActiveEventID(eventID int64) bool {
	if state == nil || state.EventScores.ActiveEventID == eventID {
		return false
	}
	state.EventScores.ActiveEventID = eventID
	if state.eventScoreMutationCOW {
		state.eventScoreMetadataDirty = true
	}
	return true
}

func (state *GameState) ReplaceEventInventory(inventory EventInventoryState) bool {
	if state == nil {
		return false
	}
	inventory.ObservedAt = inventory.ObservedAt.UTC().Truncate(time.Minute)
	if inventory.ActiveByEvent == nil {
		inventory.ActiveByEvent = map[int64]EventAvailability{}
	} else {
		inventory.ActiveByEvent = cloneMap(inventory.ActiveByEvent)
	}
	current := state.EventScores.Inventory
	if current.ObservedAt.Equal(inventory.ObservedAt) && maps.Equal(current.ActiveByEvent, inventory.ActiveByEvent) {
		return false
	}
	state.EventScores.Inventory = inventory
	if state.eventScoreMutationCOW {
		state.eventScoreMetadataDirty = true
	}
	return true
}

func (state *GameState) ReplaceEventShopRoutes(routes map[PackageID]EventShopRoute) bool {
	if state == nil || maps.Equal(state.EventScores.ShopByPackage, routes) {
		return false
	}
	state.EventScores.ShopByPackage = routes
	if state.eventScoreMutationCOW {
		state.eventScoreShopDirty = true
	}
	return true
}

func (state *GameState) ReplaceEventScores(value EventScoreState) {
	if state == nil {
		return
	}
	state.EventScores = value
	state.EventScores.Inventory.ActiveByEvent = cloneMap(value.Inventory.ActiveByEvent)
	state.eventScoreRecords = eventScoreGenerationFromState(value)
	state.EventScores.ByEvent = nil
	state.EventScores.ActivityByEvent = nil
	state.EventScores.RankingByEvent = nil
	if state.eventScoreMutationCOW {
		state.replaceEventScores = true
		state.pendingEventScoreIDs = map[int64]struct{}{}
		state.mutableEventScoreShards = [4]uint64{}
		state.eventScoreMetadataDirty = true
		state.eventScoreShopDirty = true
	}
}

func (state GameState) materializedEventScores() EventScoreState {
	result := EventScoreState{
		ActiveEventID: state.EventScores.ActiveEventID,
		ByEvent:       map[int64]ScalableEventScore{}, ShopByPackage: cloneMap(state.EventScores.ShopByPackage),
		ActivityByEvent: map[int64]EventActivityState{}, RankingByEvent: map[int64]EventRankingState{},
		Inventory: EventInventoryState{
			ObservedAt:    state.EventScores.Inventory.ObservedAt,
			ActiveByEvent: cloneMap(state.EventScores.Inventory.ActiveByEvent),
		},
	}
	state.rangeEventScoreRecords(func(eventID int64, record eventScoreRecord) bool {
		if record.HasScore {
			result.ByEvent[eventID] = record.Score
		}
		if record.HasActivity {
			result.ActivityByEvent[eventID] = cloneEventActivity(record.Activity)
		}
		if record.HasRanking {
			result.RankingByEvent[eventID] = cloneEventRanking(record.Ranking)
		}
		return true
	})
	return result
}

func (state GameState) eventScoreChangeIDs() []int64 {
	ids := make([]int64, 0, len(state.pendingEventScoreIDs))
	for id := range state.pendingEventScoreIDs {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(left, right int) bool { return ids[left] < ids[right] })
	return ids
}

func (state GameState) eventScoreIDs() []int64 {
	ids := []int64{}
	state.rangeEventScoreRecords(func(eventID int64, _ eventScoreRecord) bool {
		ids = append(ids, eventID)
		return true
	})
	sort.Slice(ids, func(left, right int) bool { return ids[left] < ids[right] })
	return ids
}
