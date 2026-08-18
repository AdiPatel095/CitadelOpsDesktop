package State

import (
	"fmt"
	"sort"
)

// PersistenceBatch accumulates the exact keyed changes covered by a group
// commit while retaining only the newest immutable generation. Its zero value
// is ready to use.
//
// Keeping the change keys matters: a dirty partitioned component with no keys
// is intentionally interpreted by the snapshot writer as a complete replace.
// Dropping an earlier event's keys while batching would therefore turn one map
// coordinate update into a rewrite of every persisted map shard.
type PersistenceBatch struct {
	latest Event
	dirty  ComponentSet

	mapChanges         map[string]MapChange
	replaceMap         bool
	castleIDs          map[CastleID]struct{}
	castleParts        map[CastleID]CastleMutationPart
	replaceCastles     bool
	inventoryParts     inventoryMutationPart
	replaceInventory   bool
	equipmentIDs       map[EquipmentInstanceID]struct{}
	replaceEquipment   bool
	gemIDs             map[GemInstanceID]struct{}
	replaceGems        bool
	itemKeys           map[string]struct{}
	replaceItems       bool
	stormTargetKeys    map[string]struct{}
	replaceStorm       bool
	towerCooldownKeys  map[string]struct{}
	replaceCooldowns   bool
	towerQueueCastles  map[CastleID]struct{}
	replaceTowerQueue  bool
	reportMessageIDs   map[int64]struct{}
	replaceReports     bool
	eventScoreIDs      map[int64]struct{}
	eventScoreMeta     bool
	eventScoreShop     bool
	replaceEventScores bool
	movementIDs        map[MovementID]struct{}
	replaceMovements   bool
}

// Accumulate adds one committed state event to the pending group commit. It
// returns false for events that have no durable component patch.
func (batch *PersistenceBatch) Accumulate(event Event) bool {
	if batch == nil || event.Patch == nil || event.generation == nil || len(event.Components) == 0 {
		return false
	}
	// MovementSnapshot proves synchronization with one live socket. It is sent
	// to dashboard clients but reset during recovery, so persisting every GAM
	// marker would create disk work with no recoverable value.
	durable := Components(event.Components...) &^ Components(ComponentMovementSnapshot)
	if durable == 0 {
		return false
	}
	batch.dirty = batch.dirty.Union(durable)
	if batch.latest.generation == nil || event.Revision >= batch.latest.Revision {
		batch.latest = event
	}

	if event.replaceMap {
		batch.replaceMap = true
		batch.mapChanges = nil
	} else if !batch.replaceMap {
		if len(event.mapChanges) > 0 && batch.mapChanges == nil {
			batch.mapChanges = make(map[string]MapChange, len(event.mapChanges))
		}
		for _, change := range event.mapChanges {
			if change.Key != "" {
				batch.mapChanges[mapChangeKey(change.KingdomID, change.Key)] = change
			}
		}
	}

	if event.replaceCastles {
		batch.replaceCastles = true
		batch.castleIDs = nil
		batch.castleParts = nil
	} else if !batch.replaceCastles {
		addSetValues(&batch.castleIDs, event.castleIDs)
		if len(event.castleParts) > 0 && batch.castleParts == nil {
			batch.castleParts = make(map[CastleID]CastleMutationPart, len(event.castleParts))
		}
		for id, parts := range event.castleParts {
			batch.castleParts[id] |= parts
		}
	}

	batch.inventoryParts |= event.inventoryParts
	if event.replaceInventory {
		batch.replaceInventory = true
		batch.equipmentIDs = nil
		batch.gemIDs = nil
		batch.itemKeys = nil
	}
	if event.replaceEquipment {
		batch.replaceEquipment = true
		batch.equipmentIDs = nil
	} else if !batch.replaceInventory && !batch.replaceEquipment {
		addSetValues(&batch.equipmentIDs, event.equipmentIDs)
	}
	if event.replaceGems {
		batch.replaceGems = true
		batch.gemIDs = nil
	} else if !batch.replaceInventory && !batch.replaceGems {
		addSetValues(&batch.gemIDs, event.gemIDs)
	}
	if event.replaceItems {
		batch.replaceItems = true
		batch.itemKeys = nil
	} else if !batch.replaceInventory && !batch.replaceItems {
		addSetValues(&batch.itemKeys, event.itemKeys)
	}

	if event.replaceStorm {
		batch.replaceStorm = true
		batch.stormTargetKeys = nil
	} else if !batch.replaceStorm {
		addSetValues(&batch.stormTargetKeys, event.stormTargetKeys)
	}
	if event.replaceCooldowns {
		batch.replaceCooldowns = true
		batch.towerCooldownKeys = nil
	} else if !batch.replaceCooldowns {
		addSetValues(&batch.towerCooldownKeys, event.towerCooldownKeys)
	}
	if event.replaceTowerQueue {
		batch.replaceTowerQueue = true
		batch.towerQueueCastles = nil
	} else if !batch.replaceTowerQueue {
		addSetValues(&batch.towerQueueCastles, event.towerQueueCastles)
	}
	if event.replaceReports {
		batch.replaceReports = true
		batch.reportMessageIDs = nil
	} else if !batch.replaceReports {
		addSetValues(&batch.reportMessageIDs, event.reportMessageIDs)
	}
	batch.eventScoreMeta = batch.eventScoreMeta || event.eventScoreMeta
	batch.eventScoreShop = batch.eventScoreShop || event.eventScoreShop
	if event.replaceEventScores {
		batch.replaceEventScores = true
		batch.eventScoreIDs = nil
	} else if !batch.replaceEventScores {
		addSetValues(&batch.eventScoreIDs, event.eventScoreIDs)
	}
	if event.replaceMovements {
		batch.replaceMovements = true
		batch.movementIDs = nil
	} else if !batch.replaceMovements {
		addSetValues(&batch.movementIDs, event.movementIDs)
	}
	return true
}

// Revision is the newest revision currently waiting for persistence.
func (batch *PersistenceBatch) Revision() uint64 {
	if batch == nil {
		return 0
	}
	return batch.latest.Revision
}

// Flush writes the pending batch and resets it only after the new manifest is
// durable. A failed flush remains retryable with the same generation and keys.
func (batch *PersistenceBatch) Flush(dataDir string) (uint64, error) {
	return batch.FlushWithWriter(NewComponentSnapshotWriter(dataDir))
}

// FlushWithWriter reuses the account-owned durable manifest across group
// commits. A failed flush retains both the batch and the last known-good
// manifest so the exact same generation remains retryable.
func (batch *PersistenceBatch) FlushWithWriter(writer *ComponentSnapshotWriter) (uint64, error) {
	if batch == nil || batch.dirty == 0 || batch.latest.generation == nil {
		return 0, nil
	}
	event := batch.persistenceEvent()
	revision := event.Revision
	if writer == nil {
		return revision, fmt.Errorf("component snapshot writer is required")
	}
	if err := writer.Save(event, batch.dirty); err != nil {
		return revision, err
	}
	*batch = PersistenceBatch{}
	return revision, nil
}

func (batch *PersistenceBatch) persistenceEvent() Event {
	event := batch.latest
	event.Components = batch.dirty.List()
	event.mapChanges = mapChangeSetValues(batch.mapChanges)
	event.replaceMap = batch.replaceMap
	event.castleIDs = sortedSetValues(batch.castleIDs)
	event.castleParts = cloneCastleParts(batch.castleParts)
	event.replaceCastles = batch.replaceCastles
	event.inventoryParts = batch.inventoryParts
	event.replaceInventory = batch.replaceInventory
	event.equipmentIDs = sortedSetValues(batch.equipmentIDs)
	event.replaceEquipment = batch.replaceEquipment
	event.gemIDs = sortedSetValues(batch.gemIDs)
	event.replaceGems = batch.replaceGems
	event.itemKeys = sortedSetValues(batch.itemKeys)
	event.replaceItems = batch.replaceItems
	event.stormTargetKeys = sortedSetValues(batch.stormTargetKeys)
	event.replaceStorm = batch.replaceStorm
	event.towerCooldownKeys = sortedSetValues(batch.towerCooldownKeys)
	event.replaceCooldowns = batch.replaceCooldowns
	event.towerQueueCastles = sortedSetValues(batch.towerQueueCastles)
	event.replaceTowerQueue = batch.replaceTowerQueue
	event.reportMessageIDs = sortedSetValues(batch.reportMessageIDs)
	event.replaceReports = batch.replaceReports
	event.eventScoreIDs = sortedSetValues(batch.eventScoreIDs)
	event.eventScoreMeta = batch.eventScoreMeta
	event.eventScoreShop = batch.eventScoreShop
	event.replaceEventScores = batch.replaceEventScores
	event.movementIDs = sortedSetValues(batch.movementIDs)
	event.replaceMovements = batch.replaceMovements
	return event
}

func addSetValues[T comparable](destination *map[T]struct{}, values []T) {
	if len(values) == 0 {
		return
	}
	if *destination == nil {
		*destination = make(map[T]struct{}, len(values))
	}
	for _, value := range values {
		(*destination)[value] = struct{}{}
	}
}

func sortedSetValues[T ~int64 | ~string](values map[T]struct{}) []T {
	result := make([]T, 0, len(values))
	for value := range values {
		result = append(result, value)
	}
	sort.Slice(result, func(left, right int) bool { return result[left] < result[right] })
	return result
}

func mapChangeSetValues(values map[string]MapChange) []MapChange {
	changes := make([]MapChange, 0, len(values))
	for _, change := range values {
		changes = append(changes, change)
	}
	return normalizeMapChanges(changes)
}

func cloneCastleParts(source map[CastleID]CastleMutationPart) map[CastleID]CastleMutationPart {
	if len(source) == 0 {
		return nil
	}
	result := make(map[CastleID]CastleMutationPart, len(source))
	for id, parts := range source {
		result[id] = parts
	}
	return result
}
