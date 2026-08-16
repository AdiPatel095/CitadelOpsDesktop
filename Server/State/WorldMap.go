package State

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/url"
	"reflect"
	"sort"
	"strings"
	"sync"
	"time"
)

type WorldMap map[KingdomID]map[string]MapObservation

// WorldMapFact is the compact process-shared representation. Account-relative
// progress fields do not exist here, so they cannot leak and do not consume
// RAM for every objective castle observation.
type WorldMapFact struct {
	KingdomID  KingdomID `json:"kingdomId"`
	X          int       `json:"x"`
	Y          int       `json:"y"`
	TypeID     int       `json:"typeId"`
	Name       string    `json:"name,omitempty"`
	OwnerID    PlayerID  `json:"ownerId"`
	ObjectID   int64     `json:"objectId,omitempty"`
	ObservedAt time.Time `json:"observedAt"`
	// Event is allocated only for shared event-map rows. Keeping this out of
	// the base value avoids paying Storm fields for every player castle in a
	// large world while still storing one actionable Storm row per process.
	Event *WorldEventMapFact `json:"event,omitempty"`
}

type WorldEventMapFact struct {
	Level                  int   `json:"level,omitempty"`
	StormIsleID            int64 `json:"stormIsleId,omitempty"`
	StormVictoryCount      int64 `json:"stormVictoryCount,omitempty"`
	StormCooldownRemaining int   `json:"stormCooldownRemaining,omitempty"`
}

type worldFactMap map[KingdomID]*worldFactRegion

type worldFactRegion struct {
	// Shared facts use the same single-copy feature partitioning as account map
	// overlays. Player castles and Storm targets therefore never share a scan
	// path, and no duplicate secondary key index is needed.
	kinds [mapProjectionKindCount]*worldFactKindRegion
}

type worldFactKindRegion struct {
	shards [accountMapShardCount]map[string]WorldMapFact
}

type mutableWorldFactRegion struct {
	region       *worldFactRegion
	mutableKinds [mapProjectionKindCount]*mutableWorldFactKindRegion
}

type mutableWorldFactKindRegion struct {
	region       *worldFactKindRegion
	clonedShards [accountMapShardCount]bool
}

// MapChange is a coordinate-level account or shared-world update. A deleted
// change has no observation; otherwise Observation is the complete value for
// that coordinate.
type MapChange struct {
	KingdomID KingdomID `json:"kingdomId"`
	Key       string    `json:"key"`
	// TypeID keeps deletion deltas consumer-filterable after the value itself
	// has been removed. Upserts continue to use Observation.TypeID.
	TypeID             int             `json:"typeId,omitempty"`
	Observation        *MapObservation `json:"observation,omitempty"`
	Deleted            bool            `json:"deleted,omitempty"`
	expectedObservedAt time.Time
}

type worldMapGeneration struct {
	version      uint64
	updatedAt    time.Time
	values       worldFactMap
	stormWindows map[KingdomID]map[string]StormScanWindowState
	stormPlans   map[KingdomID]stormScanPlan
}

type WorldMapEvent struct {
	WorldID   string
	Version   uint64
	UpdatedAt time.Time
	Changes   []MapChange
	// Domains preserve the producer's targeted wake classification across
	// account runtimes. They never contain account identity and are not durable.
	Domains    []string
	KingdomIDs []KingdomID
	Source     *Store
	generation *worldMapGeneration
}

// WorldMapStore owns immutable, objective world facts once per process. It
// intentionally excludes account-relative PvE and event progress.
type WorldMapStore struct {
	mu          sync.RWMutex
	worlds      map[string]*worldMapGeneration
	subscribers map[uint64]chan WorldMapEvent
	nextID      uint64

	db               *sql.DB
	persistMu        sync.Mutex
	dirtyFacts       map[string]persistedWorldMapChange
	dirtyStormScans  map[string]persistedStormScanWindow
	persistenceErr   error
	persistWake      chan struct{}
	persistStop      chan struct{}
	persistDone      chan struct{}
	persistStartOnce sync.Once
	persistStopOnce  sync.Once
	dbCloseOnce      sync.Once
	dbCloseErr       error

	stormParticipants map[string]map[string]time.Time
	stormRosters      map[string]stormScanRoster
	stormLeases       map[string]stormScanLease
	nextStormLease    uint64
}

func NewWorldMapStore() *WorldMapStore {
	return &WorldMapStore{
		worlds:            map[string]*worldMapGeneration{},
		subscribers:       map[uint64]chan WorldMapEvent{},
		dirtyFacts:        map[string]persistedWorldMapChange{},
		dirtyStormScans:   map[string]persistedStormScanWindow{},
		persistWake:       make(chan struct{}, 1),
		persistStop:       make(chan struct{}),
		persistDone:       make(chan struct{}),
		stormParticipants: map[string]map[string]time.Time{},
		stormRosters:      map[string]stormScanRoster{},
		stormLeases:       map[string]stormScanLease{},
	}
}

func CanonicalWorldID(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	candidate := raw
	hasScheme := strings.Contains(raw, "://")
	if !hasScheme {
		candidate = "//" + raw
	}
	if parsed, err := url.Parse(candidate); err == nil && parsed.Hostname() != "" {
		host := strings.ToLower(parsed.Hostname())
		if port := parsed.Port(); port != "" {
			scheme := strings.ToLower(parsed.Scheme)
			standard := !hasScheme && (port == "80" || port == "443") ||
				(scheme == "https" || scheme == "wss") && port == "443" ||
				(scheme == "http" || scheme == "ws") && port == "80"
			if !standard {
				host += ":" + port
			}
		}
		return host
	}
	return strings.ToLower(raw)
}

func gameStateWorldID(state GameState) string {
	worldID := CanonicalWorldID(state.Account.WorldID)
	if worldID == "" {
		worldID = CanonicalWorldID(state.Session.ServerURL)
	}
	return worldID
}

func (store *WorldMapStore) Snapshot(worldID string) *worldMapGeneration {
	if store == nil || CanonicalWorldID(worldID) == "" {
		return nil
	}
	store.mu.RLock()
	generation := store.worlds[CanonicalWorldID(worldID)]
	store.mu.RUnlock()
	return generation
}

func (store *WorldMapStore) commit(worldID string, source *Store, changes []MapChange) WorldMapEvent {
	return store.commitWithDomains(worldID, source, changes, nil)
}

func (store *WorldMapStore) commitWithDomains(
	worldID string,
	source *Store,
	changes []MapChange,
	domains []string,
) WorldMapEvent {
	worldID = CanonicalWorldID(worldID)
	if store == nil || worldID == "" {
		return WorldMapEvent{}
	}
	store.mu.Lock()
	current := store.worlds[worldID]
	if current == nil {
		current = &worldMapGeneration{
			values: worldFactMap{}, stormWindows: map[KingdomID]map[string]StormScanWindowState{},
			stormPlans: initialStormScanPlans(),
		}
	}
	nextValues := cloneMap(current.values)
	changedRegions := map[KingdomID]*mutableWorldFactRegion{}
	applied := make([]MapChange, 0, len(changes))
	for _, change := range normalizeMapChanges(changes) {
		if change.Key == "" {
			continue
		}
		currentFact, exists := lookupWorldFact(current.values, change.KingdomID, change.Key)
		if change.Deleted {
			if !exists {
				continue
			}
			if !change.expectedObservedAt.IsZero() && currentFact.ObservedAt.After(change.expectedObservedAt) {
				continue
			}
		} else {
			if change.Observation == nil {
				continue
			}
			if !ShareableMapObservation(*change.Observation) {
				// A newer account-private object at the coordinate shadows an older
				// public fact for every account. Remove that stale shared fact while
				// retaining the new private value only in the producing account.
				if !exists || currentFact.ObservedAt.After(change.Observation.ObservedAt) {
					continue
				}
				change = MapChange{
					KingdomID: change.KingdomID, Key: change.Key, TypeID: currentFact.TypeID,
					Deleted: true, expectedObservedAt: currentFact.ObservedAt,
				}
			} else {
				fact := worldMapFact(*change.Observation)
				if exists && currentFact.ObservedAt.After(fact.ObservedAt) {
					continue
				}
				if exists && reflect.DeepEqual(currentFact, fact) {
					continue
				}
			}
		}
		mutable := changedRegions[change.KingdomID]
		if mutable == nil {
			var region worldFactRegion
			if existing := current.values[change.KingdomID]; existing != nil {
				region = *existing
			}
			mutable = &mutableWorldFactRegion{region: &region}
			changedRegions[change.KingdomID] = mutable
			nextValues[change.KingdomID] = mutable.region
		}
		oldKind, hadOldKind := MapProjectionKindForType(currentFact.TypeID)
		if exists && hadOldKind && (change.Deleted || change.Observation == nil || change.Observation.TypeID != currentFact.TypeID) {
			delete(mutable.mutableWorldFactShard(oldKind, change.Key), change.Key)
		}
		if !change.Deleted {
			newKind, retained := MapProjectionKindForType(change.Observation.TypeID)
			if !retained {
				continue
			}
			mutable.mutableWorldFactShard(newKind, change.Key)[change.Key] = worldMapFact(*change.Observation)
		}
		applied = append(applied, change)
	}
	if len(applied) == 0 {
		generation := current
		store.worlds[worldID] = generation
		store.mu.Unlock()
		return WorldMapEvent{WorldID: worldID, Version: generation.version, UpdatedAt: generation.updatedAt, Source: source, generation: generation}
	}
	now := time.Now().UTC()
	next := &worldMapGeneration{
		version: current.version + 1, updatedAt: now, values: nextValues,
		stormWindows: current.stormWindows, stormPlans: updateStormScanPlans(current.stormPlans, nextValues, applied),
	}
	store.worlds[worldID] = next
	event := WorldMapEvent{
		WorldID: worldID, Version: next.version, UpdatedAt: now,
		Changes: applied, Domains: worldMapDomains(domains, applied), KingdomIDs: mapChangeKingdomIDs(applied),
		Source: source, generation: next,
	}
	subscribers := make([]chan WorldMapEvent, 0, len(store.subscribers))
	for _, channel := range store.subscribers {
		subscribers = append(subscribers, channel)
	}
	store.mu.Unlock()
	store.queuePersistence(event)
	for _, channel := range subscribers {
		channel <- event
	}
	return event
}

func mapChangeKingdomIDs(changes []MapChange) []KingdomID {
	seen := map[KingdomID]struct{}{}
	for _, change := range changes {
		seen[change.KingdomID] = struct{}{}
	}
	ids := make([]KingdomID, 0, len(seen))
	for kingdomID := range seen {
		ids = append(ids, kingdomID)
	}
	sort.Slice(ids, func(left, right int) bool { return ids[left] < ids[right] })
	return ids
}

func worldMapDomains(proposed []string, changes []MapChange) []string {
	domains := make([]string, 0, len(proposed))
	genericMap := false
	for _, domain := range proposed {
		normalized := strings.ToLower(strings.TrimSpace(domain))
		switch normalized {
		case "storm-scan-progress", "storm-scan":
			return []string{normalized}
		case "map":
			genericMap = true
		}
	}
	for _, change := range changes {
		typeID := change.TypeID
		if change.Observation != nil {
			typeID = change.Observation.TypeID
		}
		if domain, retained := MapDomainForType(typeID); retained {
			domains = append(domains, domain)
		}
	}
	if genericMap {
		domains = append(domains, "map")
	}
	return normalizeDomains(domains)
}

func lookupWorldFact(values worldFactMap, kingdomID KingdomID, key string) (WorldMapFact, bool) {
	region := values[kingdomID]
	if region == nil {
		return WorldMapFact{}, false
	}
	shard := mapShardIndex(key)
	for kind := MapProjectionNone + 1; kind < mapProjectionKindCount; kind++ {
		if kindRegion := region.kinds[kind]; kindRegion != nil {
			if fact, exists := kindRegion.shards[shard][key]; exists {
				return fact, true
			}
		}
	}
	return WorldMapFact{}, false
}

func rangeWorldFacts(values worldFactMap, kingdomID KingdomID, visit func(string, WorldMapFact) bool) bool {
	region := values[kingdomID]
	if region == nil {
		return true
	}
	for kind := MapProjectionNone + 1; kind < mapProjectionKindCount; kind++ {
		kindRegion := region.kinds[kind]
		if kindRegion == nil {
			continue
		}
		for _, shard := range kindRegion.shards {
			for key, fact := range shard {
				if !visit(key, fact) {
					return false
				}
			}
		}
	}
	return true
}

func rangeWorldFactsByKind(
	values worldFactMap,
	kingdomID KingdomID,
	kind MapProjectionKind,
	visit func(string, WorldMapFact) bool,
) bool {
	region := values[kingdomID]
	if region == nil || visit == nil || kind <= MapProjectionNone || kind >= mapProjectionKindCount {
		return true
	}
	kindRegion := region.kinds[kind]
	if kindRegion == nil {
		return true
	}
	for _, shard := range kindRegion.shards {
		for key, fact := range shard {
			if !visit(key, fact) {
				return false
			}
		}
	}
	return true
}

func rangeWorldStormFacts(values worldFactMap, kingdomID KingdomID, visit func(string, WorldMapFact) bool) bool {
	return rangeWorldFactsByKind(values, kingdomID, MapProjectionStorm, visit)
}

func isStormMapType(typeID int) bool {
	return typeID == MapTypeStormIsland || typeID == MapTypeStormFort
}

func (mutable *mutableWorldFactRegion) mutableWorldFactShard(
	kind MapProjectionKind,
	key string,
) map[string]WorldMapFact {
	if mutable == nil || mutable.region == nil || kind <= MapProjectionNone || kind >= mapProjectionKindCount {
		panic("invalid shared map projection mutation")
	}
	mutableKind := mutable.mutableKinds[kind]
	if mutableKind == nil {
		var kindRegion worldFactKindRegion
		if current := mutable.region.kinds[kind]; current != nil {
			kindRegion = *current
		}
		mutableKind = &mutableWorldFactKindRegion{region: &kindRegion}
		mutable.mutableKinds[kind] = mutableKind
		mutable.region.kinds[kind] = mutableKind.region
	}
	shard := mapShardIndex(key)
	if !mutableKind.clonedShards[shard] {
		mutableKind.region.shards[shard] = cloneMap(mutableKind.region.shards[shard])
		mutableKind.clonedShards[shard] = true
	}
	if mutableKind.region.shards[shard] == nil {
		mutableKind.region.shards[shard] = map[string]WorldMapFact{}
	}
	return mutableKind.region.shards[shard]
}

func addWorldFact(region *worldFactRegion, key string, fact WorldMapFact) {
	if region == nil || key == "" {
		return
	}
	kind, retained := MapProjectionKindForType(fact.TypeID)
	if !retained {
		return
	}
	kindRegion := region.kinds[kind]
	if kindRegion == nil {
		kindRegion = &worldFactKindRegion{}
		region.kinds[kind] = kindRegion
	}
	shard := mapShardIndex(key)
	if kindRegion.shards[shard] == nil {
		kindRegion.shards[shard] = map[string]WorldMapFact{}
	}
	kindRegion.shards[shard][key] = fact
}

func (store *WorldMapStore) Subscribe(buffer int) (<-chan WorldMapEvent, func()) {
	if buffer < 1 {
		buffer = 1
	}
	channel := make(chan WorldMapEvent, buffer)
	store.mu.Lock()
	store.nextID++
	id := store.nextID
	store.subscribers[id] = channel
	store.mu.Unlock()
	var once sync.Once
	return channel, func() {
		once.Do(func() {
			store.mu.Lock()
			delete(store.subscribers, id)
			store.mu.Unlock()
		})
	}
}

// ShareableMapObservation is deliberately conservative. A positive owner is
// an objective world identity; ownerless/negative NPC and event nodes can have
// account-specific levels, cooldowns, victory counts, availability, or map
// layouts and remain in the account overlay.
func ShareableMapObservation(observation MapObservation) bool {
	policy, retained := MapProjectionFor(observation.TypeID)
	return retained && (policy.ShareForWorld || policy.ShareWhenOwned && observation.OwnerID > 0)
}

func worldMapFact(observation MapObservation) WorldMapFact {
	fact := WorldMapFact{
		KingdomID: observation.KingdomID,
		X:         observation.X, Y: observation.Y, TypeID: observation.TypeID,
		Name: observation.Name, OwnerID: observation.OwnerID, ObjectID: observation.ObjectID,
		ObservedAt: observation.ObservedAt,
	}
	if observation.TypeID == MapTypeStormIsland || observation.TypeID == MapTypeStormFort {
		fact.Event = &WorldEventMapFact{
			Level: observation.Level, StormIsleID: observation.StormIsleID,
			StormVictoryCount:      observation.StormVictoryCount,
			StormCooldownRemaining: observation.StormCooldownRemaining,
		}
	}
	return fact
}

func (fact WorldMapFact) observation() MapObservation {
	observation := MapObservation{
		KingdomID: fact.KingdomID,
		X:         fact.X, Y: fact.Y, TypeID: fact.TypeID,
		Name: fact.Name, OwnerID: fact.OwnerID, ObjectID: fact.ObjectID,
		ObservedAt: fact.ObservedAt,
	}
	if fact.Event != nil {
		observation.Level = fact.Event.Level
		observation.StormIsleID = fact.Event.StormIsleID
		observation.StormVictoryCount = fact.Event.StormVictoryCount
		observation.StormCooldownRemaining = fact.Event.StormCooldownRemaining
	}
	return observation
}

// RetainMapObservation defines the durable map surface. These are the only
// account-relative target types consumed by current attack, cooldown, event,
// or Rift logic. Positive-owner objects are retained as objective identity
// facts for cross-account reuse and target-name enrichment.
func RetainMapObservation(observation MapObservation) bool {
	_, retained := MapProjectionFor(observation.TypeID)
	return retained
}

func pruneIrrelevantMapObservations(state *GameState) {
	if state == nil {
		return
	}
	now := time.Now().UTC()
	for kingdomID, observations := range state.Map {
		for key, observation := range observations {
			projected, retained := projectMapObservation(observation)
			if !retained || mapObservationExpired(projected, now) {
				delete(observations, key)
				continue
			}
			if !reflect.DeepEqual(projected, observation) {
				observations[key] = projected
			}
		}
		if len(observations) == 0 {
			delete(state.Map, kingdomID)
		}
	}
	for _, candidate := range mapRetentionRemovals(
		privateMapRetentionCandidates(*state), now, accountPrivateMapRetentionLimit,
	) {
		delete(state.Map[candidate.kingdomID], candidate.key)
		if len(state.Map[candidate.kingdomID]) == 0 {
			delete(state.Map, candidate.kingdomID)
		}
	}
}

func extractShareableMapObservations(state *GameState) []MapChange {
	if state == nil || !state.worldSharing {
		return nil
	}
	changes := []MapChange{}
	for kingdomID, observations := range state.Map {
		for key, observation := range observations {
			if !ShareableMapObservation(observation) {
				continue
			}
			value := observation
			changes = append(changes, MapChange{KingdomID: kingdomID, Key: key, Observation: &value})
			delete(observations, key)
		}
		if len(observations) == 0 {
			delete(state.Map, kingdomID)
		}
	}
	return normalizeMapChanges(changes)
}

// AdoptWorldMap advances an account to a shared-world generation without
// cloning any account component. The account receives its own revision and a
// coordinate delta, while its private overlay remains untouched.
func (store *Store) AdoptWorldMap(worldEvent WorldMapEvent) (Event, bool) {
	if store == nil || worldEvent.generation == nil || worldEvent.Source == store {
		return Event{}, false
	}
	store.writeMu.Lock()
	defer store.writeMu.Unlock()
	current := store.generation.Load()
	if current == nil || gameStateWorldID(*current.state) != CanonicalWorldID(worldEvent.WorldID) {
		return Event{}, false
	}
	if current.state.sharedMap != nil && current.state.sharedMap.version >= worldEvent.Version {
		return Event{}, false
	}
	changes := make([]MapChange, 0, len(worldEvent.Changes))
	for _, change := range normalizeMapChanges(worldEvent.Changes) {
		if !accountCanAccessSharedKingdom(*current.state, change.KingdomID) {
			continue
		}
		changes = append(changes, change)
	}
	candidateValue := *current.state
	candidate := &candidateValue
	candidate.sharedMap = worldEvent.generation
	candidate.worldSharing = true
	candidate.pendingMapChanges = nil
	var domains []string
	if len(changes) > 0 {
		domains = worldMapDomains(worldEvent.Domains, changes)
	} else {
		for _, kingdomID := range worldEvent.KingdomIDs {
			if accountCanAccessSharedKingdom(*current.state, kingdomID) {
				domains = worldMapDomains(worldEvent.Domains, nil)
				break
			}
		}
	}
	if len(changes) == 0 && len(domains) == 0 {
		// Keep the process-owned generation current even when this account has no
		// access to the changed kingdom or only anonymous scan coverage changed.
		// Neither case is observable account state, so it must not create a
		// revision, frontend notice, or persistence write.
		store.generation.Store(&storeGeneration{
			state: candidate, versions: current.versions, protocol: current.protocol,
		})
		return Event{}, false
	}
	candidate.Revision++
	candidate.UpdatedAt = time.Now().UTC()
	partitionSnapshot, changedPartitions := advancePartitionVersions(
		current.versions, defaultPartitionKeys(*candidate, domains), candidate.Revision, candidate.UpdatedAt,
	)
	protocol := nextProtocolContext(current.protocol, *candidate, domains, nil, FocusSubcontext(""), candidate.UpdatedAt)
	next := &storeGeneration{state: candidate, versions: partitionSnapshot, protocol: protocol}
	store.generation.Store(next)
	components := []Component{}
	if len(changes) > 0 {
		components = []Component{ComponentWorldMap}
	}
	event := Event{
		Sequence: candidate.Revision, Revision: candidate.Revision, Domains: domains,
		Components: components, Partitions: changedPartitions,
		OccurredAt: candidate.UpdatedAt, generation: next, mapChanges: changes,
	}
	if len(changes) > 0 {
		event.Patch = componentPatch(next, Components(ComponentWorldMap), componentChanges{mapChanges: changes})
	}
	event.clientEncoding = &clientEventEncoding{}
	store.publish(event)
	return event, true
}

func (state GameState) LookupMapObservation(kingdomID KingdomID, key string) (MapObservation, bool) {
	if observation, exists := state.lookupPrivateMapObservation(kingdomID, key); exists {
		return observation, true
	}
	if state.sharedMap != nil && accountCanAccessSharedKingdom(state, kingdomID) {
		fact, exists := lookupWorldFact(state.sharedMap.values, kingdomID, key)
		return fact.observation(), exists
	}
	return MapObservation{}, false
}

func (state GameState) RangeMapObservations(kingdomID KingdomID, visit func(string, MapObservation) bool) {
	if visit == nil {
		return
	}
	if state.sharedMap != nil && accountCanAccessSharedKingdom(state, kingdomID) {
		privateShadows := state.hasPrivateMapObservations(kingdomID)
		completed := rangeWorldFacts(state.sharedMap.values, kingdomID, func(key string, fact WorldMapFact) bool {
			if privateShadows {
				if _, shadowed := state.lookupPrivateMapObservation(kingdomID, key); shadowed {
					return true
				}
			}
			if !visit(key, fact.observation()) {
				return false
			}
			return true
		})
		if !completed {
			return
		}
	}
	state.rangePrivateMapObservations(kingdomID, visit)
}

// RangeMapObservationsByKind traverses only the physical feature partition a
// caller requested. Private rows still shadow shared facts at the same
// coordinate, preserving the logical GameState view without a full-map scan.
func (state GameState) RangeMapObservationsByKind(
	kingdomID KingdomID,
	kind MapProjectionKind,
	visit func(string, MapObservation) bool,
) {
	if visit == nil || kind <= MapProjectionNone || kind >= mapProjectionKindCount {
		return
	}
	if state.sharedMap != nil && accountCanAccessSharedKingdom(state, kingdomID) {
		privateShadows := state.hasPrivateMapObservations(kingdomID)
		completed := rangeWorldFactsByKind(state.sharedMap.values, kingdomID, kind, func(key string, fact WorldMapFact) bool {
			if privateShadows {
				if _, shadowed := state.lookupPrivateMapObservation(kingdomID, key); shadowed {
					return true
				}
			}
			return visit(key, fact.observation())
		})
		if !completed {
			return
		}
	}
	state.rangePrivateMapObservationsByKind(kingdomID, kind, visit)
}

func (state GameState) MapKingdomIDs() []KingdomID {
	set := make(map[KingdomID]struct{}, len(state.Map))
	state.privateMapKingdomIDs(func(kingdomID KingdomID) { set[kingdomID] = struct{}{} })
	if state.sharedMap != nil {
		for kingdomID := range state.sharedMap.values {
			if accountCanAccessSharedKingdom(state, kingdomID) {
				set[kingdomID] = struct{}{}
			}
		}
	}
	ids := make([]KingdomID, 0, len(set))
	for kingdomID := range set {
		ids = append(ids, kingdomID)
	}
	sort.Slice(ids, func(left, right int) bool { return ids[left] < ids[right] })
	return ids
}

func (state *GameState) SetMapObservation(observation MapObservation) bool {
	if state == nil {
		return false
	}
	state.requireMapWrite()
	var retained bool
	observation, retained = projectMapObservation(observation)
	if !retained {
		return false
	}
	key := fmt.Sprintf("%d:%d", observation.X, observation.Y)
	current, exists := state.LookupMapObservation(observation.KingdomID, key)
	if exists && reflect.DeepEqual(current, observation) {
		return false
	}
	newKind, _ := MapProjectionKindForType(observation.TypeID)
	if state.worldSharing && ShareableMapObservation(observation) {
		if private, privateExists := state.lookupPrivateMapObservation(observation.KingdomID, key); privateExists {
			if oldKind, retained := MapProjectionKindForType(private.TypeID); retained {
				delete(state.mutableMapShard(observation.KingdomID, oldKind, key), key)
			}
		}
	} else {
		if private, privateExists := state.lookupPrivateMapObservation(observation.KingdomID, key); privateExists && private.TypeID != observation.TypeID {
			if oldKind, retained := MapProjectionKindForType(private.TypeID); retained && oldKind != newKind {
				delete(state.mutableMapShard(observation.KingdomID, oldKind, key), key)
			}
		}
		state.mutableMapShard(observation.KingdomID, newKind, key)[key] = observation
	}
	state.recordMapChange(MapChange{
		KingdomID: observation.KingdomID, Key: key, TypeID: observation.TypeID, Observation: &observation,
	})
	return true
}

func (state *GameState) DeleteMapObservation(kingdomID KingdomID, key string) bool {
	if state == nil || key == "" {
		return false
	}
	state.requireMapWrite()
	observation, exists := state.LookupMapObservation(kingdomID, key)
	if !exists {
		return false
	}
	if _, private := state.lookupPrivateMapObservation(kingdomID, key); private {
		if kind, retained := MapProjectionKindForType(observation.TypeID); retained {
			delete(state.mutableMapShard(kingdomID, kind, key), key)
		}
	}
	state.recordMapChange(MapChange{KingdomID: kingdomID, Key: key, TypeID: observation.TypeID, Deleted: true})
	return true
}

// ReplaceMapState clears the account overlay and asks the next state event to
// replace the client's materialized map. It is used only for an authoritative
// account/world reset; normal map traffic remains coordinate-delta based.
func (state *GameState) ReplaceMapState() {
	if state == nil {
		return
	}
	state.requireMapWrite()
	state.Map = WorldMap{}
	state.mapOverlay = &accountMapGeneration{regions: map[KingdomID]*accountMapRegion{}}
	state.pendingMapChanges = nil
	state.replaceMap = true
	state.mapMutationCOW = true
	state.mutableMapRegions = map[KingdomID]*mutableAccountMapRegion{}
}

func (state *GameState) requireMapWrite() {
	if state.mutationWrites != 0 && !state.mutationWrites.Has(ComponentWorldMap) {
		panic("state map mutation was not declared")
	}
}

func (state *GameState) recordMapChange(change MapChange) {
	if state.pendingMapChanges == nil {
		state.pendingMapChanges = map[string]MapChange{}
	}
	state.pendingMapChanges[mapChangeKey(change.KingdomID, change.Key)] = change
}

func (state GameState) mapChanges() []MapChange {
	changes := make([]MapChange, 0, len(state.pendingMapChanges))
	for _, change := range state.pendingMapChanges {
		changes = append(changes, change)
	}
	return normalizeMapChanges(changes)
}

func normalizeMapChanges(changes []MapChange) []MapChange {
	latest := make(map[string]MapChange, len(changes))
	for _, change := range changes {
		if change.Key != "" {
			latest[mapChangeKey(change.KingdomID, change.Key)] = change
		}
	}
	keys := make([]string, 0, len(latest))
	for key := range latest {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	out := make([]MapChange, 0, len(keys))
	for _, key := range keys {
		out = append(out, latest[key])
	}
	return out
}

func mapChangeKey(kingdomID KingdomID, key string) string {
	return fmt.Sprintf("%020d:%s", kingdomID, key)
}

func (state GameState) materializedMap() WorldMap {
	private := state.materializedPrivateMap()
	if state.sharedMap == nil {
		return private
	}
	result := make(WorldMap, len(state.sharedMap.values)+len(private))
	for kingdomID := range state.sharedMap.values {
		if !accountCanAccessSharedKingdom(state, kingdomID) {
			continue
		}
		observations := map[string]MapObservation{}
		rangeWorldFacts(state.sharedMap.values, kingdomID, func(key string, fact WorldMapFact) bool {
			observations[key] = fact.observation()
			return true
		})
		if len(observations) > 0 {
			result[kingdomID] = observations
		}
	}
	for kingdomID, observations := range private {
		kingdom := result[kingdomID]
		if kingdom == nil {
			kingdom = map[string]MapObservation{}
			result[kingdomID] = kingdom
		}
		for key, observation := range observations {
			kingdom[key] = observation
		}
	}
	return result
}

// accountCanAccessSharedKingdom is the final no-leak boundary for process
// facts. A world match alone is insufficient: an account receives a kingdom's
// shared rows only after its own private state proves that kingdom is unlocked
// or contains one of its castles.
func accountCanAccessSharedKingdom(state GameState, kingdomID KingdomID) bool {
	if kingdomID == 0 {
		return true
	}
	for _, castle := range state.Castles {
		if castle.KingdomID == kingdomID {
			return true
		}
	}
	unlock, found := state.KingdomTransport.Unlocks[kingdomID]
	return found && (unlock.Unlocked || unlock.Created)
}

func (state GameState) MarshalJSON() ([]byte, error) {
	type wireState GameState
	return json.Marshal(struct {
		wireState
		Map WorldMap `json:"map"`
	}{wireState: wireState(state), Map: state.materializedMap()})
}

func (state *GameState) UnmarshalJSON(raw []byte) error {
	type wireState GameState
	var document struct {
		wireState
		Map WorldMap `json:"map"`
	}
	if err := json.Unmarshal(raw, &document); err != nil {
		return err
	}
	*state = GameState(document.wireState)
	state.Map = document.Map
	state.mapOverlay = nil
	return nil
}
