package State

import (
	"fmt"
	"sort"
	"sync"
	"sync/atomic"
	"time"
)

type Event struct {
	Revision   uint64    `json:"revision"`
	Domains    []string  `json:"domains"`
	OccurredAt time.Time `json:"occurredAt"`
}

type Mutation func(state *GameState) (domains []string, changed bool, err error)

type Store struct {
	mu    sync.RWMutex
	state GameState

	subMu       sync.RWMutex
	subscribers map[uint64]chan Event
	nextID      atomic.Uint64
}

func NewStore(initial GameState) *Store {
	if initial.SchemaVersion == 0 {
		initial = NewGameState()
	}
	return &Store{state: initial, subscribers: map[uint64]chan Event{}}
}

func (store *Store) Revision() uint64 {
	store.mu.RLock()
	defer store.mu.RUnlock()
	return store.state.Revision
}

func (store *Store) Snapshot() GameState {
	store.mu.RLock()
	defer store.mu.RUnlock()
	return cloneGameState(store.state)
}

func (store *Store) Apply(mutation Mutation) (Event, error) {
	if mutation == nil {
		return Event{}, fmt.Errorf("state mutation is required")
	}
	store.mu.Lock()
	candidate := cloneGameState(store.state)
	domains, changed, err := mutation(&candidate)
	if err != nil {
		store.mu.Unlock()
		return Event{}, err
	}
	if !changed {
		store.mu.Unlock()
		return Event{Revision: store.state.Revision}, nil
	}
	candidate.Revision++
	candidate.UpdatedAt = time.Now().UTC()
	domains = normalizeDomains(domains)
	store.state = candidate
	event := Event{Revision: candidate.Revision, Domains: domains, OccurredAt: candidate.UpdatedAt}
	store.mu.Unlock()
	store.publish(event)
	return event, nil
}

func (store *Store) Subscribe(buffer int) (<-chan Event, func()) {
	if buffer < 1 {
		buffer = 1
	}
	id := store.nextID.Add(1)
	channel := make(chan Event, buffer)
	store.subMu.Lock()
	store.subscribers[id] = channel
	store.subMu.Unlock()
	var once sync.Once
	cancel := func() {
		once.Do(func() {
			store.subMu.Lock()
			delete(store.subscribers, id)
			store.subMu.Unlock()
		})
	}
	return channel, cancel
}

func (store *Store) publish(event Event) {
	store.subMu.RLock()
	defer store.subMu.RUnlock()
	for _, channel := range store.subscribers {
		select {
		case channel <- event:
		default:
		}
	}
}

func normalizeDomains(domains []string) []string {
	set := map[string]struct{}{}
	for _, domain := range domains {
		if domain != "" {
			set[domain] = struct{}{}
		}
	}
	out := make([]string, 0, len(set))
	for domain := range set {
		out = append(out, domain)
	}
	sort.Strings(out)
	return out
}

func cloneGameState(source GameState) GameState {
	clone := source
	clone.Player.Resources = cloneMap(source.Player.Resources)
	clone.Player.Currencies = cloneMap(source.Player.Currencies)
	clone.Castles = make(map[CastleID]CastleState, len(source.Castles))
	for id, castle := range source.Castles {
		castle.Resources = make(map[ResourceID]ResourceBalance, len(source.Castles[id].Resources))
		for resourceID, balance := range source.Castles[id].Resources {
			balance.ProductionPerHour = cloneFloatPointer(balance.ProductionPerHour)
			balance.Capacity = cloneFloatPointer(balance.Capacity)
			castle.Resources[resourceID] = balance
		}
		castle.Units.Stationed = cloneMap(castle.Units.Stationed)
		castle.Units.Traveling = cloneMap(castle.Units.Traveling)
		castle.Units.Hospital = cloneMap(castle.Units.Hospital)
		castle.Units.SpecialHospital = cloneMap(castle.Units.SpecialHospital)
		castle.Units.Total = cloneMap(castle.Units.Total)
		castle.Buildings = cloneMap(castle.Buildings)
		castle.ConstructionSlots = make(map[BuildingInstanceID][]ConstructionSlot, len(castle.ConstructionSlots))
		for buildingID, slots := range source.Castles[id].ConstructionSlots {
			clonedSlots := append([]ConstructionSlot(nil), slots...)
			for index := range clonedSlots {
				clonedSlots[index].RemainingSec = cloneIntPointer(clonedSlots[index].RemainingSec)
			}
			castle.ConstructionSlots[buildingID] = clonedSlots
		}
		castle.Production = make(map[int]ProductionQueue, len(castle.Production))
		for lineID, queue := range source.Castles[id].Production {
			queue.Active = cloneQueueItemPointer(queue.Active)
			queue.Queued = cloneQueueItems(queue.Queued)
			castle.Production[lineID] = queue
		}
		castle.Crafting.Buildings = make(map[BuildingInstanceID]CraftingBuilding, len(source.Castles[id].Crafting.Buildings))
		for buildingID, building := range source.Castles[id].Crafting.Buildings {
			building.ActiveSlotRentals = append([]int{}, building.ActiveSlotRentals...)
			building.QueueSlotRentals = append([]int{}, building.QueueSlotRentals...)
			building.Active = cloneCraftingQueue(building.Active)
			building.Queued = cloneCraftingQueue(building.Queued)
			castle.Crafting.Buildings[buildingID] = building
		}
		castle.Crafting.EnabledRecipeIDs = append([]int64{}, source.Castles[id].Crafting.EnabledRecipeIDs...)
		castle.Crafting.EnabledRecipeGroupIDs = append([]int64{}, source.Castles[id].Crafting.EnabledRecipeGroupIDs...)
		castle.Crafting.OutputBoostByQueueType = cloneMap(source.Castles[id].Crafting.OutputBoostByQueueType)
		clone.Castles[id] = castle
	}
	clone.Commanders = make(map[CommanderID]CommanderState, len(source.Commanders))
	for id, commander := range source.Commanders {
		commander.Equipment = cloneMap(commander.Equipment)
		commander.Gems = cloneMap(commander.Gems)
		clone.Commanders[id] = commander
	}
	clone.Castellans = make(map[CastellanID]CastellanState, len(source.Castellans))
	for id, castellan := range source.Castellans {
		castellan.Equipment = cloneMap(castellan.Equipment)
		castellan.Gems = cloneMap(castellan.Gems)
		clone.Castellans[id] = castellan
	}
	clone.Movements = make(map[MovementID]MovementState, len(source.Movements))
	for id, movement := range source.Movements {
		movement.Units = cloneMap(movement.Units)
		movement.ArrivesAt = cloneTimePointer(movement.ArrivesAt)
		movement.ReturnsAt = cloneTimePointer(movement.ReturnsAt)
		movement.CommanderID = cloneCommanderIDPointer(movement.CommanderID)
		clone.Movements[id] = movement
	}
	clone.Stationing = make(map[string]StationingOperation, len(source.Stationing))
	for id, operation := range source.Stationing {
		operation.Units = cloneMap(operation.Units)
		operation.SafeAfter = cloneTimePointer(operation.SafeAfter)
		clone.Stationing[id] = operation
	}
	clone.Scheduled = make(map[string]ScheduledOperation, len(source.Scheduled))
	for id, operation := range source.Scheduled {
		operation.Arguments = append([]byte(nil), operation.Arguments...)
		clone.Scheduled[id] = operation
	}
	clone.Rift.Launches = make(map[string]RiftLaunch, len(source.Rift.Launches))
	for id, launch := range source.Rift.Launches {
		launch.Body = append([]byte(nil), launch.Body...)
		clone.Rift.Launches[id] = launch
	}
	clone.Inventory.ConstructionItems = cloneMap(source.Inventory.ConstructionItems)
	clone.Inventory.Equipment = make(map[EquipmentInstanceID]EquipmentInstance, len(source.Inventory.Equipment))
	for id, item := range source.Inventory.Equipment {
		item.Effects = cloneEquipmentEffects(item.Effects)
		clone.Inventory.Equipment[id] = item
	}
	clone.Inventory.Gems = make(map[GemInstanceID]GemInstance, len(source.Inventory.Gems))
	for id, gem := range source.Inventory.Gems {
		gem.Effects = cloneEquipmentEffects(gem.Effects)
		clone.Inventory.Gems[id] = gem
	}
	clone.Inventory.GemStacks = cloneMap(source.Inventory.GemStacks)
	clone.Inventory.Items = make(map[string]map[int64]int64, len(source.Inventory.Items))
	for collection, items := range source.Inventory.Items {
		clone.Inventory.Items[collection] = cloneMap(items)
	}
	clone.Alliance.Members = append([]AllianceMember{}, source.Alliance.Members...)
	clone.Alliance.Holdings = append([]AllianceHolding{}, source.Alliance.Holdings...)
	clone.Map = make(map[KingdomID]map[string]MapObservation, len(source.Map))
	for kingdomID, observations := range source.Map {
		clone.Map[kingdomID] = cloneMap(observations)
	}
	clone.CommandContext.ProductionObservedAt = cloneTimePointer(source.CommandContext.ProductionObservedAt)
	clone.Automations = make(map[string]AutomationState, len(source.Automations))
	for id, automation := range source.Automations {
		automation.NextCheckAt = cloneTimePointer(automation.NextCheckAt)
		automation.LastRunAt = cloneTimePointer(automation.LastRunAt)
		automation.Metrics = cloneMap(automation.Metrics)
		clone.Automations[id] = automation
	}
	clone.Reports.Notices = cloneMap(source.Reports.Notices)
	clone.Reports.SpyCaptures = make(map[int64]SpyReportCapture, len(source.Reports.SpyCaptures))
	for id, capture := range source.Reports.SpyCaptures {
		capture.Payload = append([]byte(nil), capture.Payload...)
		clone.Reports.SpyCaptures[id] = capture
	}
	clone.Reports.BattleCaptures = make(map[int64]BattleReportCapture, len(source.Reports.BattleCaptures))
	for id, capture := range source.Reports.BattleCaptures {
		capture.Summary = append([]byte(nil), capture.Summary...)
		capture.Waves = append([]byte(nil), capture.Waves...)
		capture.Details = append([]byte(nil), capture.Details...)
		clone.Reports.BattleCaptures[id] = capture
	}
	clone.Observations = cloneMap(source.Observations)
	return clone
}

func cloneQueueItems(source []QueueItem) []QueueItem {
	clone := append([]QueueItem{}, source...)
	for index := range clone {
		clone[index].StartedAt = cloneTimePointer(clone[index].StartedAt)
		clone[index].CompletesAt = cloneTimePointer(clone[index].CompletesAt)
	}
	return clone
}

func cloneQueueItemPointer(source *QueueItem) *QueueItem {
	if source == nil {
		return nil
	}
	clone := *source
	clone.StartedAt = cloneTimePointer(source.StartedAt)
	clone.CompletesAt = cloneTimePointer(source.CompletesAt)
	return &clone
}

func cloneCraftingQueue(source []CraftingQueueItem) []CraftingQueueItem {
	clone := append([]CraftingQueueItem{}, source...)
	for index := range clone {
		clone[index].RemainingSec = cloneIntPointer(clone[index].RemainingSec)
		clone[index].RuntimeSec = cloneIntPointer(clone[index].RuntimeSec)
	}
	return clone
}

func cloneMap[Key comparable, Value any](source map[Key]Value) map[Key]Value {
	if source == nil {
		return map[Key]Value{}
	}
	clone := make(map[Key]Value, len(source))
	for key, value := range source {
		clone[key] = value
	}
	return clone
}

func cloneIntPointer(source *int) *int {
	if source == nil {
		return nil
	}
	value := *source
	return &value
}

func cloneCommanderIDPointer(source *CommanderID) *CommanderID {
	if source == nil {
		return nil
	}
	value := *source
	return &value
}

func cloneEquipmentEffects(source EquipmentEffects) EquipmentEffects {
	clone := append(EquipmentEffects(nil), source...)
	for index := range clone {
		clone[index].RollPercent = cloneFloatPointer(source[index].RollPercent)
		clone[index].Values = append([]float64(nil), source[index].Values...)
	}
	return clone
}

func cloneTimePointer(source *time.Time) *time.Time {
	if source == nil {
		return nil
	}
	value := *source
	return &value
}

func cloneFloatPointer(source *float64) *float64 {
	if source == nil {
		return nil
	}
	value := *source
	return &value
}
