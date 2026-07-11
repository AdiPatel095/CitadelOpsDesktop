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
		castle.Queues = make(map[string][]QueueItem, len(castle.Queues))
		for name, items := range source.Castles[id].Queues {
			clonedItems := append([]QueueItem(nil), items...)
			for index := range clonedItems {
				clonedItems[index].StartedAt = cloneTimePointer(clonedItems[index].StartedAt)
				clonedItems[index].CompletesAt = cloneTimePointer(clonedItems[index].CompletesAt)
			}
			castle.Queues[name] = clonedItems
		}
		clone.Castles[id] = castle
	}
	clone.Commanders = make(map[CommanderID]CommanderState, len(source.Commanders))
	for id, commander := range source.Commanders {
		commander.Equipment = cloneMap(commander.Equipment)
		commander.Gems = cloneMap(commander.Gems)
		clone.Commanders[id] = commander
	}
	clone.Movements = make(map[MovementID]MovementState, len(source.Movements))
	for id, movement := range source.Movements {
		movement.Units = cloneMap(movement.Units)
		movement.ArrivesAt = cloneTimePointer(movement.ArrivesAt)
		movement.ReturnsAt = cloneTimePointer(movement.ReturnsAt)
		clone.Movements[id] = movement
	}
	clone.Inventory.ConstructionItems = cloneMap(source.Inventory.ConstructionItems)
	clone.Inventory.Equipment = make(map[EquipmentInstanceID]EquipmentInstance, len(source.Inventory.Equipment))
	for id, item := range source.Inventory.Equipment {
		item.Effects = cloneMap(item.Effects)
		clone.Inventory.Equipment[id] = item
	}
	clone.Inventory.Gems = make(map[GemInstanceID]GemInstance, len(source.Inventory.Gems))
	for id, gem := range source.Inventory.Gems {
		gem.Effects = cloneMap(gem.Effects)
		clone.Inventory.Gems[id] = gem
	}
	clone.Inventory.Items = make(map[string]map[int64]int64, len(source.Inventory.Items))
	for collection, items := range source.Inventory.Items {
		clone.Inventory.Items[collection] = cloneMap(items)
	}
	clone.Alliance.Members = append([]AllianceMember(nil), source.Alliance.Members...)
	clone.Map = make(map[KingdomID]map[string]MapObservation, len(source.Map))
	for kingdomID, observations := range source.Map {
		clone.Map[kingdomID] = cloneMap(observations)
	}
	clone.Observations = cloneMap(source.Observations)
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
