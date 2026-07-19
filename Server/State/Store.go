package State

import (
	"fmt"
	"sort"
	"sync"
	"sync/atomic"
	"time"
)

type Event struct {
	Sequence   uint64             `json:"sequence"`
	Gap        bool               `json:"gap,omitempty"`
	Revision   uint64             `json:"revision"`
	Domains    []string           `json:"domains"`
	Partitions []PartitionVersion `json:"partitions,omitempty"`
	OccurredAt time.Time          `json:"occurredAt"`
}

type Mutation func(state *GameState) (domains []string, changed bool, err error)

type ScopedChange struct {
	Domains    []string
	Partitions []PartitionKey
	Changed    bool
}

type ScopedMutation func(state *GameState) (ScopedChange, error)

type storeGeneration struct {
	state    GameState
	versions *partitionVersionSnapshot
	protocol ProtocolContextState
}

type Store struct {
	writeMu    sync.Mutex
	generation atomic.Pointer[storeGeneration]

	subMu       sync.RWMutex
	subscribers map[uint64]chan Event
	nextID      atomic.Uint64
}

func NewStore(initial GameState) *Store {
	if initial.SchemaVersion == 0 {
		initial = NewGameState()
	}
	normalizeStateMaps(&initial)
	normalizeFocusedCastle(&initial, 0)
	owned := cloneGameState(initial)
	store := &Store{subscribers: map[uint64]chan Event{}}
	store.generation.Store(&storeGeneration{
		state:    owned,
		versions: &partitionVersionSnapshot{values: map[string]PartitionVersion{}},
		protocol: initialProtocolContext(owned),
	})
	return store
}

func (store *Store) Revision() uint64 {
	generation := store.generation.Load()
	if generation == nil {
		return 0
	}
	return generation.state.Revision
}

func (store *Store) Snapshot() GameState {
	generation := store.generation.Load()
	if generation == nil {
		return NewGameState()
	}
	return cloneGameState(generation.state)
}

// SnapshotWithRevision returns a defensive snapshot and the revision from the
// same immutable generation. Callers must not combine separate Snapshot and
// Revision calls when the revision describes the returned body.
func (store *Store) SnapshotWithRevision() (GameState, uint64) {
	generation := store.generation.Load()
	if generation == nil {
		state := NewGameState()
		return state, state.Revision
	}
	return cloneGameState(generation.state), generation.state.Revision
}

// PlanningView returns one immutable state generation and its capability
// versions without cloning the full account. Callers must treat State as
// read-only. Store mutations always clone before changing a generation.
func (store *Store) PlanningView() PlanningView {
	generation := store.generation.Load()
	if generation == nil {
		state := NewGameState()
		return PlanningView{State: state}
	}
	return PlanningView{
		State:           generation.state,
		Partitions:      PartitionVersions{snapshot: generation.versions},
		ProtocolContext: generation.protocol,
	}
}

func (store *Store) PartitionVersions() []PartitionVersion {
	generation := store.generation.Load()
	if generation == nil {
		return []PartitionVersion{}
	}
	return PartitionVersions{snapshot: generation.versions}.List()
}

func (store *Store) ProtocolContext() ProtocolContextState {
	generation := store.generation.Load()
	if generation == nil {
		return ProtocolContextState{}
	}
	return generation.protocol
}

func (store *Store) Session() SessionState {
	generation := store.generation.Load()
	if generation == nil {
		return SessionState{}
	}
	return cloneSessionState(generation.state.Session)
}

func (store *Store) Apply(mutation Mutation) (Event, error) {
	if mutation == nil {
		return Event{}, fmt.Errorf("state mutation is required")
	}
	return store.ApplyScoped(func(state *GameState) (ScopedChange, error) {
		domains, changed, err := mutation(state)
		return ScopedChange{Domains: domains, Changed: changed}, err
	})
}

func (store *Store) ApplyScoped(mutation ScopedMutation) (Event, error) {
	if mutation == nil {
		return Event{}, fmt.Errorf("scoped state mutation is required")
	}
	store.writeMu.Lock()
	defer store.writeMu.Unlock()
	current := store.generation.Load()
	if current == nil {
		initial := NewGameState()
		current = &storeGeneration{
			state:    initial,
			versions: &partitionVersionSnapshot{values: map[string]PartitionVersion{}},
			protocol: initialProtocolContext(initial),
		}
	}
	candidate := cloneGameState(current.state)
	change, err := mutation(&candidate)
	if err != nil {
		return Event{}, err
	}
	if !change.Changed {
		return Event{Revision: current.state.Revision}, nil
	}
	normalizeFocusedCastle(&candidate, current.protocol.FocusedCastleID)
	candidate.Revision++
	candidate.UpdatedAt = time.Now().UTC()
	domains := normalizeDomains(change.Domains)
	partitions := append(defaultPartitionKeys(candidate, domains), change.Partitions...)
	partitionSnapshot, changedPartitions := advancePartitionVersions(
		current.versions, partitions, candidate.Revision, candidate.UpdatedAt,
	)
	protocol := nextProtocolContext(current.protocol, candidate, domains, change.Partitions, candidate.UpdatedAt)
	store.generation.Store(&storeGeneration{state: candidate, versions: partitionSnapshot, protocol: protocol})
	event := Event{
		Sequence: candidate.Revision, Revision: candidate.Revision, Domains: domains,
		Partitions: changedPartitions, OccurredAt: candidate.UpdatedAt,
	}
	store.publish(event)
	return event, nil
}

func normalizeFocusedCastle(state *GameState, preferred CastleID) {
	if state == nil || len(state.Castles) == 0 {
		return
	}
	selected := CastleID(0)
	if preferred > 0 && state.Castles[preferred].Focused {
		selected = preferred
	}
	if selected == 0 {
		for castleID, castle := range state.Castles {
			if castle.Focused && (selected == 0 || castleID < selected) {
				selected = castleID
			}
		}
	}
	if selected == 0 {
		return
	}
	for castleID, castle := range state.Castles {
		focused := castleID == selected
		if castle.Focused != focused {
			castle.Focused = focused
			state.Castles[castleID] = castle
		}
	}
}

func cloneSessionState(session SessionState) SessionState {
	session.CooldownUntil = cloneTimePointer(session.CooldownUntil)
	session.RetryAt = cloneTimePointer(session.RetryAt)
	return session
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
			continue
		default:
		}

		coalesced := event
		select {
		case pending := <-channel:
			coalesced = coalesceEvents(pending, event)
		default:
		}
		// The first send only fails when the buffered channel is full. After
		// removing one pending event, no other Store publisher can refill it:
		// ApplyScoped holds store.writeMu until publication completes.
		channel <- coalesced
	}
}

func coalesceEvents(left Event, right Event) Event {
	merged := Event{
		Sequence: left.Sequence,
		Gap:      true,
		Revision: left.Revision,
		Domains:  normalizeDomains(append(append([]string(nil), left.Domains...), right.Domains...)),
		Partitions: mergePartitionVersions(
			append(append([]PartitionVersion(nil), left.Partitions...), right.Partitions...),
		),
		OccurredAt: left.OccurredAt,
	}
	if right.Revision > merged.Revision {
		merged.Revision = right.Revision
	}
	if right.Sequence > merged.Sequence {
		merged.Sequence = right.Sequence
	}
	if right.OccurredAt.After(merged.OccurredAt) {
		merged.OccurredAt = right.OccurredAt
	}
	return merged
}

func mergePartitionVersions(versions []PartitionVersion) []PartitionVersion {
	latest := make(map[string]PartitionVersion, len(versions))
	for _, version := range versions {
		canonical := version.Key.Canonical()
		if current, found := latest[canonical]; !found || version.Version > current.Version {
			latest[canonical] = version
		}
	}
	keys := make([]string, 0, len(latest))
	for key := range latest {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	out := make([]PartitionVersion, 0, len(keys))
	for _, key := range keys {
		out = append(out, latest[key])
	}
	return out
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
	clone.Player.LegendSkills.ActiveIDs = append([]int64(nil), source.Player.LegendSkills.ActiveIDs...)
	clone.Player.LegendSkills.SceatSkillIDs = append([]int64(nil), source.Player.LegendSkills.SceatSkillIDs...)
	clone.Player.LegendSkills.SceatActivations = append([]SceatSkillActivation(nil), source.Player.LegendSkills.SceatActivations...)
	clone.Castles = make(map[CastleID]CastleState, len(source.Castles))
	for id, castle := range source.Castles {
		castle.Resources = make(map[ResourceID]ResourceBalance, len(source.Castles[id].Resources))
		for resourceID, balance := range source.Castles[id].Resources {
			balance.ProductionPerHour = cloneFloatPointer(balance.ProductionPerHour)
			balance.ConsumptionPerHour = cloneFloatPointer(balance.ConsumptionPerHour)
			balance.ConsumptionMultiplier = cloneFloatPointer(balance.ConsumptionMultiplier)
			balance.Capacity = cloneFloatPointer(balance.Capacity)
			castle.Resources[resourceID] = balance
		}
		castle.Units.Stationed = cloneMap(castle.Units.Stationed)
		castle.Units.Traveling = cloneMap(castle.Units.Traveling)
		castle.Units.Hospital = cloneMap(castle.Units.Hospital)
		castle.Units.SpecialHospital = cloneMap(castle.Units.SpecialHospital)
		castle.Units.Total = cloneMap(castle.Units.Total)
		castle.Defense.RangedUnitIDs = append([]UnitID{}, castle.Defense.RangedUnitIDs...)
		castle.Defense.MeleeUnitIDs = append([]UnitID{}, castle.Defense.MeleeUnitIDs...)
		castle.Defense.Inventory = cloneMap(castle.Defense.Inventory)
		castle.Defense.OpenGateUntil = cloneTimePointer(castle.Defense.OpenGateUntil)
		castle.Defense.Wall.Left.ToolSlots = append([]DefenseToolSlot{}, castle.Defense.Wall.Left.ToolSlots...)
		castle.Defense.Wall.Middle.ToolSlots = append([]DefenseToolSlot{}, castle.Defense.Wall.Middle.ToolSlots...)
		castle.Defense.Wall.Right.ToolSlots = append([]DefenseToolSlot{}, castle.Defense.Wall.Right.ToolSlots...)
		castle.Defense.Keep.PrimaryToolSlots = append([]DefenseToolSlot{}, castle.Defense.Keep.PrimaryToolSlots...)
		castle.Defense.Keep.SecondaryToolSlots = append([]DefenseToolSlot{}, castle.Defense.Keep.SecondaryToolSlots...)
		castle.Defense.Moat.LeftToolSlots = append([]DefenseToolSlot{}, castle.Defense.Moat.LeftToolSlots...)
		castle.Defense.Moat.MiddleToolSlots = append([]DefenseToolSlot{}, castle.Defense.Moat.MiddleToolSlots...)
		castle.Defense.Moat.RightToolSlots = append([]DefenseToolSlot{}, castle.Defense.Moat.RightToolSlots...)
		castle.Buildings = cloneMap(castle.Buildings)
		castle.Layout.Ground = cloneMap(castle.Layout.Ground)
		castle.Layout.Objects = cloneMap(castle.Layout.Objects)
		castle.Layout.Fixed = cloneMap(castle.Layout.Fixed)
		castle.BuildingQueue.Slots = append([]BuildingConstructionQueueSlot(nil), castle.BuildingQueue.Slots...)
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
		castle.QueueableProduction = make(map[int][]DefinitionRef, len(source.Castles[id].QueueableProduction))
		for lineID, definitions := range source.Castles[id].QueueableProduction {
			castle.QueueableProduction[lineID] = append([]DefinitionRef(nil), definitions...)
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
	clone.Generals = make(map[int64]GeneralState, len(source.Generals))
	for id, general := range source.Generals {
		general.ActiveSkillIDs = append([]int64(nil), general.ActiveSkillIDs...)
		clone.Generals[id] = general
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
		movement.MarketGoods = append([]KingdomTransportGood(nil), movement.MarketGoods...)
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
	clone.Rift.DeletedLaunchIDs = cloneMap(source.Rift.DeletedLaunchIDs)
	clone.Inventory.ConstructionItems = cloneMap(source.Inventory.ConstructionItems)
	clone.Inventory.ConstructionOffers = cloneMap(source.Inventory.ConstructionOffers)
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
	clone.Subscriptions = cloneMap(source.Subscriptions)
	clone.Market.Castles = make(map[CastleID]MarketCastleState, len(source.Market.Castles))
	for id, castle := range source.Market.Castles {
		castle.Resources = cloneMap(castle.Resources)
		castle.AreaEffects = append([]MarketAreaEffect(nil), castle.AreaEffects...)
		for index := range castle.AreaEffects {
			castle.AreaEffects[index].Values = append([]float64(nil), castle.AreaEffects[index].Values...)
		}
		clone.Market.Castles[id] = castle
	}
	clone.KingdomTransport.Unlocks = cloneMap(source.KingdomTransport.Unlocks)
	clone.KingdomTransport.Pending = append([]KingdomResourceTransport(nil), source.KingdomTransport.Pending...)
	for index := range clone.KingdomTransport.Pending {
		clone.KingdomTransport.Pending[index].Goods = append([]KingdomTransportGood(nil), source.KingdomTransport.Pending[index].Goods...)
	}
	clone.KingdomTransport.PendingUnits = append([]KingdomUnitTransport(nil), source.KingdomTransport.PendingUnits...)
	for index := range clone.KingdomTransport.PendingUnits {
		clone.KingdomTransport.PendingUnits[index].Units = append([]KingdomTransportUnit(nil), source.KingdomTransport.PendingUnits[index].Units...)
	}
	clone.Beri.TroopsByUnit = cloneMap(source.Beri.TroopsByUnit)
	clone.Alliance.Members = append([]AllianceMember{}, source.Alliance.Members...)
	clone.Alliance.Holdings = append([]AllianceHolding{}, source.Alliance.Holdings...)
	clone.Alliances = make(map[AllianceID]AllianceState, len(source.Alliances))
	for id, alliance := range source.Alliances {
		alliance.Members = append([]AllianceMember{}, alliance.Members...)
		alliance.Holdings = append([]AllianceHolding{}, alliance.Holdings...)
		clone.Alliances[id] = alliance
	}
	clone.Map = make(map[KingdomID]map[string]MapObservation, len(source.Map))
	for kingdomID, observations := range source.Map {
		clone.Map[kingdomID] = cloneMap(observations)
	}
	clone.TowerCooldowns = cloneMap(source.TowerCooldowns)
	clone.TowerQueue.EntriesByCastle = make(map[CastleID][]TowerQueueEntry, len(source.TowerQueue.EntriesByCastle))
	for castleID, entries := range source.TowerQueue.EntriesByCastle {
		clone.TowerQueue.EntriesByCastle[castleID] = append([]TowerQueueEntry(nil), entries...)
	}
	clone.TowerQueue.LastScannedAt = cloneMap(source.TowerQueue.LastScannedAt)
	clone.Invasion.LastScannedAt = cloneMap(source.Invasion.LastScannedAt)
	clone.Invasion.FortifiedTargets = cloneMap(source.Invasion.FortifiedTargets)
	clone.Storm.LastScannedAt = cloneMap(source.Storm.LastScannedAt)
	clone.Storm.Map.Targets = cloneMap(source.Storm.Map.Targets)
	clone.Storm.IslandReturns = make(map[string]StormIslandReturnState, len(source.Storm.IslandReturns))
	for key, operation := range source.Storm.IslandReturns {
		operation.Survivors = cloneMap(operation.Survivors)
		clone.Storm.IslandReturns[key] = operation
	}
	clone.NomadCamps.LastScannedAt = cloneMap(source.NomadCamps.LastScannedAt)
	clone.NomadCamps.Cooldowns = cloneMap(source.NomadCamps.Cooldowns)
	if source.NomadCamps.LockedTarget != nil {
		lockedTarget := *source.NomadCamps.LockedTarget
		clone.NomadCamps.LockedTarget = &lockedTarget
	}
	if source.NomadCamps.RBCTest != nil {
		rbcTest := *source.NomadCamps.RBCTest
		rbcTest.Launches = append([]NomadRBCTestLaunch(nil), source.NomadCamps.RBCTest.Launches...)
		clone.NomadCamps.RBCTest = &rbcTest
	}
	clone.Khan.Launches = append([]KhanLaunchState(nil), source.Khan.Launches...)
	clone.Khan.Taunts = cloneMap(source.Khan.Taunts)
	clone.AttackDialog.ActiveEffects = append([]AttackDialogEffect(nil), source.AttackDialog.ActiveEffects...)
	for index := range clone.AttackDialog.ActiveEffects {
		clone.AttackDialog.ActiveEffects[index].Values = append([]float64(nil), source.AttackDialog.ActiveEffects[index].Values...)
	}
	clone.AttackPresets = append([]AttackPreset(nil), source.AttackPresets...)
	for presetIndex := range clone.AttackPresets {
		for lane := range clone.AttackPresets[presetIndex].Units {
			clone.AttackPresets[presetIndex].Units[lane] = append([]AttackPresetStack(nil), source.AttackPresets[presetIndex].Units[lane]...)
			clone.AttackPresets[presetIndex].Tools[lane] = append([]AttackPresetStack(nil), source.AttackPresets[presetIndex].Tools[lane]...)
		}
	}
	clone.EventScores.ByEvent = cloneMap(source.EventScores.ByEvent)
	clone.EventScores.ShopByPackage = cloneMap(source.EventScores.ShopByPackage)
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
