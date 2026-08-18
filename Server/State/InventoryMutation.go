package State

import (
	"sort"
	"time"
)

type inventoryMutationPart uint8

const (
	inventoryConstructionItemsMutable inventoryMutationPart = 1 << iota
	inventoryConstructionOffersMutable
	inventoryEquipmentMutable
	inventoryGemsMutable
	inventoryGemStacksMutable
	inventoryItemsMutable
)

func cloneConstructionOfferSnapshots(
	source map[CastleID]ConstructionOfferSnapshot,
) map[CastleID]ConstructionOfferSnapshot {
	cloned := make(map[CastleID]ConstructionOfferSnapshot, len(source))
	for castleID, snapshot := range source {
		snapshot.Offers = cloneMap(snapshot.Offers)
		cloned[castleID] = snapshot
	}
	return cloned
}

func (state *GameState) prepareInventoryMutation(source GameState) {
	state.Inventory = source.Inventory
	state.inventoryMutationCOW = true
	state.mutableInventoryParts = 0
	state.pendingEquipmentChanges = map[EquipmentInstanceID]struct{}{}
	state.replaceInventoryEquipment = false
	state.pendingGemChanges = map[GemInstanceID]struct{}{}
	state.replaceInventoryGems = false
	state.pendingInventoryItemChanges = map[string]struct{}{}
	state.replaceInventoryItems = false
}

func (state *GameState) MutableInventoryConstructionItems() map[ConstructionItemID]int64 {
	if state == nil {
		return nil
	}
	if state.inventoryMutationCOW && state.mutableInventoryParts&inventoryConstructionItemsMutable == 0 {
		state.Inventory.ConstructionItems = cloneMap(state.Inventory.ConstructionItems)
		state.mutableInventoryParts |= inventoryConstructionItemsMutable
	}
	if state.Inventory.ConstructionItems == nil {
		state.Inventory.ConstructionItems = map[ConstructionItemID]int64{}
	}
	return state.Inventory.ConstructionItems
}

func (state *GameState) MutableInventoryConstructionOffers() map[PackageID]int64 {
	if state == nil {
		return nil
	}
	if state.inventoryMutationCOW && state.mutableInventoryParts&inventoryConstructionOffersMutable == 0 {
		state.Inventory.ConstructionOffers = cloneMap(state.Inventory.ConstructionOffers)
		state.Inventory.ConstructionOffersByCastle = cloneMap(state.Inventory.ConstructionOffersByCastle)
		if castleID := state.Inventory.ConstructionOffersCastleID; castleID > 0 {
			snapshot, found := state.Inventory.ConstructionOffersByCastle[castleID]
			if found && snapshot.KingdomID == state.Inventory.ConstructionOffersKingdomID {
				snapshot.Offers = state.Inventory.ConstructionOffers
				state.Inventory.ConstructionOffersByCastle[castleID] = snapshot
			}
		}
		state.mutableInventoryParts |= inventoryConstructionOffersMutable
	}
	if state.Inventory.ConstructionOffers == nil {
		state.Inventory.ConstructionOffers = map[PackageID]int64{}
	}
	return state.Inventory.ConstructionOffers
}

// ConstructionOffersFor returns the most recent official purchase counters
// for exactly one castle/kingdom context. The legacy current response remains
// a compatibility fallback for profiles written before the scoped index.
func (state GameState) ConstructionOffersFor(
	castleID CastleID,
	kingdomID KingdomID,
) (map[PackageID]int64, time.Time, bool) {
	if castleID > 0 {
		if snapshot, found := state.Inventory.ConstructionOffersByCastle[castleID]; found &&
			snapshot.CastleID == castleID && snapshot.KingdomID == kingdomID {
			return snapshot.Offers, snapshot.ObservedAt, true
		}
	}
	if state.Inventory.ConstructionOffersCastleID == castleID &&
		state.Inventory.ConstructionOffersKingdomID == kingdomID {
		return state.Inventory.ConstructionOffers, state.Inventory.ConstructionOffersObservedAt, true
	}
	return nil, time.Time{}, false
}

// MutableInventoryEquipment owns only the equipment index. Existing immutable
// values and their effects stay shared until a caller replaces an entry.
func (state *GameState) MutableInventoryEquipment() map[EquipmentInstanceID]EquipmentInstance {
	equipment := state.ownedInventoryEquipment()
	if state != nil && state.inventoryMutationCOW {
		state.replaceInventoryEquipment = true
		state.pendingEquipmentChanges = map[EquipmentInstanceID]struct{}{}
	}
	return equipment
}

func (state *GameState) ownedInventoryEquipment() map[EquipmentInstanceID]EquipmentInstance {
	if state == nil {
		return nil
	}
	if state.inventoryMutationCOW && state.mutableInventoryParts&inventoryEquipmentMutable == 0 {
		state.Inventory.Equipment = cloneMap(state.Inventory.Equipment)
		state.mutableInventoryParts |= inventoryEquipmentMutable
	}
	if state.Inventory.Equipment == nil {
		state.Inventory.Equipment = map[EquipmentInstanceID]EquipmentInstance{}
	}
	return state.Inventory.Equipment
}

// MutableInventoryGems owns only the gem index. Callers replace complete gem
// values and must not mutate an Effects slice retained from an old value.
func (state *GameState) MutableInventoryGems() map[GemInstanceID]GemInstance {
	gems := state.ownedInventoryGems()
	if state != nil && state.inventoryMutationCOW {
		state.replaceInventoryGems = true
		state.pendingGemChanges = map[GemInstanceID]struct{}{}
	}
	return gems
}

func (state *GameState) ownedInventoryGems() map[GemInstanceID]GemInstance {
	if state == nil {
		return nil
	}
	if state.inventoryMutationCOW && state.mutableInventoryParts&inventoryGemsMutable == 0 {
		state.Inventory.Gems = cloneMap(state.Inventory.Gems)
		state.mutableInventoryParts |= inventoryGemsMutable
	}
	if state.Inventory.Gems == nil {
		state.Inventory.Gems = map[GemInstanceID]GemInstance{}
	}
	return state.Inventory.Gems
}

func (state *GameState) MutableInventoryGemStacks() map[GemID]int64 {
	if state == nil {
		return nil
	}
	if state.inventoryMutationCOW && state.mutableInventoryParts&inventoryGemStacksMutable == 0 {
		state.Inventory.GemStacks = cloneMap(state.Inventory.GemStacks)
		state.mutableInventoryParts |= inventoryGemStacksMutable
	}
	if state.Inventory.GemStacks == nil {
		state.Inventory.GemStacks = map[GemID]int64{}
	}
	return state.Inventory.GemStacks
}

// MutableInventoryItems owns the collection index. Storage reducers replace a
// complete collection, leaving every unchanged collection map shared.
func (state *GameState) MutableInventoryItems() map[string]map[int64]int64 {
	items := state.ownedInventoryItems()
	if state != nil && state.inventoryMutationCOW {
		state.replaceInventoryItems = true
		state.pendingInventoryItemChanges = map[string]struct{}{}
	}
	return items
}

func (state *GameState) ownedInventoryItems() map[string]map[int64]int64 {
	if state == nil {
		return nil
	}
	if state.inventoryMutationCOW && state.mutableInventoryParts&inventoryItemsMutable == 0 {
		state.Inventory.Items = cloneMap(state.Inventory.Items)
		state.mutableInventoryParts |= inventoryItemsMutable
	}
	if state.Inventory.Items == nil {
		state.Inventory.Items = map[string]map[int64]int64{}
	}
	return state.Inventory.Items
}

func (state *GameState) SetInventoryEquipment(id EquipmentInstanceID, item EquipmentInstance) {
	if state == nil || id == 0 {
		return
	}
	state.ownedInventoryEquipment()[id] = item
	if state.inventoryMutationCOW && !state.replaceInventoryEquipment {
		state.pendingEquipmentChanges[id] = struct{}{}
	}
}

func (state *GameState) DeleteInventoryEquipment(id EquipmentInstanceID) {
	if state == nil || id == 0 {
		return
	}
	delete(state.ownedInventoryEquipment(), id)
	if state.inventoryMutationCOW && !state.replaceInventoryEquipment {
		state.pendingEquipmentChanges[id] = struct{}{}
	}
}

func (state *GameState) SetInventoryGem(id GemInstanceID, gem GemInstance) {
	if state == nil || id == 0 {
		return
	}
	state.ownedInventoryGems()[id] = gem
	if state.inventoryMutationCOW && !state.replaceInventoryGems {
		state.pendingGemChanges[id] = struct{}{}
	}
}

func (state *GameState) DeleteInventoryGem(id GemInstanceID) {
	if state == nil || id == 0 {
		return
	}
	delete(state.ownedInventoryGems(), id)
	if state.inventoryMutationCOW && !state.replaceInventoryGems {
		state.pendingGemChanges[id] = struct{}{}
	}
}

func (state *GameState) SetInventoryItemsCollection(collection string, items map[int64]int64) {
	if state == nil || collection == "" {
		return
	}
	state.ownedInventoryItems()[collection] = items
	if state.inventoryMutationCOW && !state.replaceInventoryItems {
		state.pendingInventoryItemChanges[collection] = struct{}{}
	}
}

func (state *GameState) inventoryEquipmentChangeIDs() []EquipmentInstanceID {
	ids := make([]EquipmentInstanceID, 0, len(state.pendingEquipmentChanges))
	for id := range state.pendingEquipmentChanges {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(left, right int) bool { return ids[left] < ids[right] })
	return ids
}

func (state *GameState) inventoryGemChangeIDs() []GemInstanceID {
	ids := make([]GemInstanceID, 0, len(state.pendingGemChanges))
	for id := range state.pendingGemChanges {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(left, right int) bool { return ids[left] < ids[right] })
	return ids
}

func (state *GameState) inventoryItemChangeKeys() []string {
	keys := make([]string, 0, len(state.pendingInventoryItemChanges))
	for key := range state.pendingInventoryItemChanges {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func (state *GameState) ReplaceInventoryConstructionItems(items map[ConstructionItemID]int64, observedAt time.Time) {
	if state == nil {
		return
	}
	state.Inventory.ConstructionItems = items
	state.Inventory.ConstructionItemsObservedAt = observedAt
	if state.inventoryMutationCOW {
		state.mutableInventoryParts |= inventoryConstructionItemsMutable
	}
}

// SetInventoryConstructionSpaceLeft records the server's construction-item
// inventory space-left answer (csp). It rides the construction-items part.
func (state *GameState) SetInventoryConstructionSpaceLeft(spaceLeft int64, observedAt time.Time) {
	if state == nil {
		return
	}
	state.Inventory.ConstructionSpaceLeft = spaceLeft
	state.Inventory.ConstructionSpaceLeftObservedAt = observedAt
	if state.inventoryMutationCOW {
		state.mutableInventoryParts |= inventoryConstructionItemsMutable
	}
}

func (state *GameState) ReplaceInventoryConstructionOffers(
	offers map[PackageID]int64,
	observedAt time.Time,
	castleID CastleID,
	kingdomID KingdomID,
) {
	if state == nil {
		return
	}
	if state.inventoryMutationCOW && state.mutableInventoryParts&inventoryConstructionOffersMutable == 0 {
		state.Inventory.ConstructionOffersByCastle = cloneMap(state.Inventory.ConstructionOffersByCastle)
	}
	if state.Inventory.ConstructionOffersByCastle == nil {
		state.Inventory.ConstructionOffersByCastle = map[CastleID]ConstructionOfferSnapshot{}
	}
	state.Inventory.ConstructionOffers = offers
	state.Inventory.ConstructionOffersObservedAt = observedAt
	state.Inventory.ConstructionOffersCastleID = castleID
	state.Inventory.ConstructionOffersKingdomID = kingdomID
	if castleID > 0 {
		state.Inventory.ConstructionOffersByCastle[castleID] = ConstructionOfferSnapshot{
			Offers: offers, ObservedAt: observedAt, CastleID: castleID, KingdomID: kingdomID,
		}
	}
	if state.inventoryMutationCOW {
		state.mutableInventoryParts |= inventoryConstructionOffersMutable
	}
}

func (state *GameState) ReplaceInventoryEquipment(equipment map[EquipmentInstanceID]EquipmentInstance) {
	if state == nil {
		return
	}
	state.Inventory.Equipment = equipment
	if state.inventoryMutationCOW {
		state.mutableInventoryParts |= inventoryEquipmentMutable
		state.replaceInventoryEquipment = true
		state.pendingEquipmentChanges = map[EquipmentInstanceID]struct{}{}
	}
}

func (state *GameState) ReplaceInventoryGems(gems map[GemInstanceID]GemInstance) {
	if state == nil {
		return
	}
	state.Inventory.Gems = gems
	if state.inventoryMutationCOW {
		state.mutableInventoryParts |= inventoryGemsMutable
		state.replaceInventoryGems = true
		state.pendingGemChanges = map[GemInstanceID]struct{}{}
	}
}

func (state *GameState) ReplaceInventoryGemStacks(stacks map[GemID]int64) {
	if state == nil {
		return
	}
	state.Inventory.GemStacks = stacks
	if state.inventoryMutationCOW {
		state.mutableInventoryParts |= inventoryGemStacksMutable
	}
}
