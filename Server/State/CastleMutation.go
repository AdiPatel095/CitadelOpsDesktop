package State

import "sort"

// CastleMutationPart mirrors official castle payload domains while allowing
// tenant storage and deltas to own each domain independently.
type CastleMutationPart uint16

const (
	CastlePartIdentity CastleMutationPart = 1 << iota
	CastlePartResources
	CastlePartUnits
	CastlePartDefense
	CastlePartBuildings
	CastlePartConstruction
	CastlePartProduction
	CastlePartCrafting
	AllCastleMutationParts = CastlePartIdentity | CastlePartResources | CastlePartUnits | CastlePartDefense |
		CastlePartBuildings | CastlePartConstruction | CastlePartProduction | CastlePartCrafting
)

func (state *GameState) prepareCastleMutation(source GameState) {
	state.Castles = cloneMap(source.Castles)
	state.castleMutationCOW = true
	state.mutableCastles = map[CastleID]CastleMutationPart{}
	state.pendingCastleChanges = map[CastleID]CastleMutationPart{}
	state.replaceCastles = false
}

// MutableCastle returns an owned deep copy of one castle the first time that
// castle is requested during a component mutation. Callers must write the
// returned value back to state.Castles after changing it.
func (state *GameState) MutableCastle(id CastleID) (CastleState, bool) {
	return state.MutableCastleParts(id, AllCastleMutationParts)
}

func (state *GameState) MutableCastleParts(id CastleID, parts CastleMutationPart) (CastleState, bool) {
	if state == nil {
		return CastleState{}, false
	}
	castle, exists := state.Castles[id]
	if !exists {
		return CastleState{}, false
	}
	parts &= AllCastleMutationParts
	if !state.castleMutationCOW || parts == 0 {
		return castle, true
	}
	owned := state.mutableCastles[id]
	missing := parts &^ owned
	if missing == 0 {
		state.pendingCastleChanges[id] |= parts
		return castle, true
	}
	castle = cloneCastleStateParts(castle, missing)
	state.Castles[id] = castle
	state.mutableCastles[id] = owned | missing
	state.pendingCastleChanges[id] |= parts
	return castle, true
}

// SetCastle replaces one logical game castle while retaining every other
// castle generation. It records the exact key for transport deltas.
func (state *GameState) SetCastle(id CastleID, castle CastleState) {
	state.SetCastleParts(id, castle, AllCastleMutationParts)
}

// SetCastleParts records replacement values built without mutating a shared
// nested map. Existing nested maps must be owned through MutableCastleParts.
func (state *GameState) SetCastleParts(id CastleID, castle CastleState, parts CastleMutationPart) {
	if state == nil || id <= 0 {
		return
	}
	if state.Castles == nil {
		state.Castles = map[CastleID]CastleState{}
	}
	state.Castles[id] = castle
	if state.castleMutationCOW {
		state.pendingCastleChanges[id] |= parts & AllCastleMutationParts
	}
}

func (state *GameState) DeleteCastle(id CastleID) {
	if state == nil || id <= 0 {
		return
	}
	delete(state.Castles, id)
	if state.castleMutationCOW {
		state.pendingCastleChanges[id] = AllCastleMutationParts
	}
}

// ReplaceCastles is reserved for authoritative full castle-list snapshots.
func (state *GameState) ReplaceCastles(castles map[CastleID]CastleState) {
	if state == nil {
		return
	}
	state.Castles = castles
	if state.castleMutationCOW {
		state.replaceCastles = true
		state.pendingCastleChanges = map[CastleID]CastleMutationPart{}
		state.mutableCastles = map[CastleID]CastleMutationPart{}
	}
}

func (state *GameState) castleChangeIDs() []CastleID {
	if state == nil || len(state.pendingCastleChanges) == 0 {
		return nil
	}
	ids := make([]CastleID, 0, len(state.pendingCastleChanges))
	for id := range state.pendingCastleChanges {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(left, right int) bool { return ids[left] < ids[right] })
	return ids
}

func (state *GameState) castleChangeParts() map[CastleID]CastleMutationPart {
	if state == nil || len(state.pendingCastleChanges) == 0 {
		return nil
	}
	parts := make(map[CastleID]CastleMutationPart, len(state.pendingCastleChanges))
	for id, value := range state.pendingCastleChanges {
		parts[id] = value
	}
	return parts
}

func cloneCastleState(castle CastleState) CastleState {
	return cloneCastleStateParts(castle, AllCastleMutationParts)
}

func cloneCastleStateParts(castle CastleState, parts CastleMutationPart) CastleState {
	source := castle
	if parts&CastlePartResources != 0 {
		castle.Resources = make(map[ResourceID]ResourceBalance, len(source.Resources))
		for resourceID, balance := range source.Resources {
			balance.ProductionPerHour = cloneFloatPointer(balance.ProductionPerHour)
			balance.ConsumptionPerHour = cloneFloatPointer(balance.ConsumptionPerHour)
			balance.ConsumptionMultiplier = cloneFloatPointer(balance.ConsumptionMultiplier)
			balance.Capacity = cloneFloatPointer(balance.Capacity)
			castle.Resources[resourceID] = balance
		}
	}
	if parts&CastlePartUnits != 0 {
		castle.Units.Stationed = cloneMap(source.Units.Stationed)
		castle.Units.Traveling = cloneMap(source.Units.Traveling)
		castle.Units.Hospital = cloneMap(source.Units.Hospital)
		castle.Units.SpecialHospital = cloneMap(source.Units.SpecialHospital)
		castle.Units.Total = cloneMap(source.Units.Total)
	}
	if parts&CastlePartDefense != 0 {
		castle.Defense.RangedUnitIDs = append([]UnitID{}, source.Defense.RangedUnitIDs...)
		castle.Defense.MeleeUnitIDs = append([]UnitID{}, source.Defense.MeleeUnitIDs...)
		castle.Defense.Inventory = cloneMap(source.Defense.Inventory)
		castle.Defense.OpenGateUntil = cloneTimePointer(source.Defense.OpenGateUntil)
		castle.Defense.Wall.Left.ToolSlots = append([]DefenseToolSlot{}, source.Defense.Wall.Left.ToolSlots...)
		castle.Defense.Wall.Middle.ToolSlots = append([]DefenseToolSlot{}, source.Defense.Wall.Middle.ToolSlots...)
		castle.Defense.Wall.Right.ToolSlots = append([]DefenseToolSlot{}, source.Defense.Wall.Right.ToolSlots...)
		castle.Defense.Keep.PrimaryToolSlots = append([]DefenseToolSlot{}, source.Defense.Keep.PrimaryToolSlots...)
		castle.Defense.Keep.SecondaryToolSlots = append([]DefenseToolSlot{}, source.Defense.Keep.SecondaryToolSlots...)
		castle.Defense.Moat.LeftToolSlots = append([]DefenseToolSlot{}, source.Defense.Moat.LeftToolSlots...)
		castle.Defense.Moat.MiddleToolSlots = append([]DefenseToolSlot{}, source.Defense.Moat.MiddleToolSlots...)
		castle.Defense.Moat.RightToolSlots = append([]DefenseToolSlot{}, source.Defense.Moat.RightToolSlots...)
	}
	if parts&CastlePartBuildings != 0 {
		castle.Buildings = cloneMap(source.Buildings)
		castle.BuildingProduction = make(map[BuildingInstanceID]BuildingProduction, len(source.BuildingProduction))
		for buildingID, production := range source.BuildingProduction {
			production.PercentByResource = cloneMap(production.PercentByResource)
			castle.BuildingProduction[buildingID] = production
		}
		castle.Layout.Ground = cloneMap(source.Layout.Ground)
		castle.Layout.Objects = cloneMap(source.Layout.Objects)
		castle.Layout.Fixed = cloneMap(source.Layout.Fixed)
	}
	if parts&CastlePartConstruction != 0 {
		castle.BuildingQueue.Slots = append([]BuildingConstructionQueueSlot(nil), source.BuildingQueue.Slots...)
		castle.ConstructionSlots = make(map[BuildingInstanceID][]ConstructionSlot, len(source.ConstructionSlots))
		for buildingID, slots := range source.ConstructionSlots {
			clonedSlots := append([]ConstructionSlot(nil), slots...)
			for index := range clonedSlots {
				clonedSlots[index].RemainingSec = cloneIntPointer(clonedSlots[index].RemainingSec)
			}
			castle.ConstructionSlots[buildingID] = clonedSlots
		}
	}
	if parts&CastlePartProduction != 0 {
		castle.Production = make(map[int]ProductionQueue, len(source.Production))
		for lineID, queue := range source.Production {
			queue.Active = cloneQueueItemPointer(queue.Active)
			queue.Queued = cloneQueueItems(queue.Queued)
			castle.Production[lineID] = queue
		}
		castle.QueueableProduction = make(map[int][]DefinitionRef, len(source.QueueableProduction))
		for lineID, definitions := range source.QueueableProduction {
			castle.QueueableProduction[lineID] = append([]DefinitionRef(nil), definitions...)
		}
	}
	if parts&CastlePartCrafting != 0 {
		castle.Crafting.Buildings = make(map[BuildingInstanceID]CraftingBuilding, len(source.Crafting.Buildings))
		for buildingID, building := range source.Crafting.Buildings {
			building.ActiveSlotRentals = append([]int{}, building.ActiveSlotRentals...)
			building.QueueSlotRentals = append([]int{}, building.QueueSlotRentals...)
			building.Active = cloneCraftingQueue(building.Active)
			building.Queued = cloneCraftingQueue(building.Queued)
			castle.Crafting.Buildings[buildingID] = building
		}
		castle.Crafting.EnabledRecipeIDs = append([]int64{}, source.Crafting.EnabledRecipeIDs...)
		castle.Crafting.EnabledRecipeGroupIDs = append([]int64{}, source.Crafting.EnabledRecipeGroupIDs...)
		castle.Crafting.OutputBoostByQueueType = cloneMap(source.Crafting.OutputBoostByQueueType)
	}
	return castle
}
