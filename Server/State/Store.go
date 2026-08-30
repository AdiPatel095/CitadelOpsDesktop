package State

import (
	"fmt"
	"reflect"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

type Event struct {
	Sequence           uint64             `json:"sequence"`
	Gap                bool               `json:"gap,omitempty"`
	Revision           uint64             `json:"revision"`
	Domains            []string           `json:"domains"`
	Components         []Component        `json:"components,omitempty"`
	Partitions         []PartitionVersion `json:"partitions,omitempty"`
	OccurredAt         time.Time          `json:"occurredAt"`
	Patch              *ComponentPatch    `json:"patch,omitempty"`
	generation         *storeGeneration
	mapChanges         []MapChange
	replaceMap         bool
	castleIDs          []CastleID
	castleParts        map[CastleID]CastleMutationPart
	replaceCastles     bool
	inventoryParts     inventoryMutationPart
	replaceInventory   bool
	equipmentIDs       []EquipmentInstanceID
	replaceEquipment   bool
	gemIDs             []GemInstanceID
	replaceGems        bool
	itemKeys           []string
	replaceItems       bool
	stormTargetKeys    []string
	replaceStorm       bool
	towerCooldownKeys  []string
	replaceCooldowns   bool
	towerQueueCastles  []CastleID
	replaceTowerQueue  bool
	reportMessageIDs   []int64
	replaceReports     bool
	eventScoreIDs      []int64
	eventScoreMeta     bool
	eventScoreShop     bool
	replaceEventScores bool
	movementIDs        []MovementID
	replaceMovements   bool
	clientEncoding     *clientEventEncoding
}

type Mutation func(state *GameState) (domains []string, changed bool, err error)

type ScopedChange struct {
	Domains         []string
	Partitions      []PartitionKey
	FocusSubcontext FocusSubcontext
	Changed         bool
	// DirtyComponents narrows the conservative mutation write set to the
	// components that actually changed. This controls patches and persistence;
	// Writes still controls the pre-mutation COW safety boundary.
	DirtyComponents    ComponentSet
	DirtyComponentsSet bool
}

type ScopedMutation func(state *GameState) (ScopedChange, error)

type storeGeneration struct {
	// state is allocated once for the generation and then treated as immutable.
	// Keeping the pointer here avoids copying the large top-level GameState a
	// second time when a prepared candidate is committed.
	state    *GameState
	versions *partitionVersionSnapshot
	protocol ProtocolContextState
}

type Store struct {
	writeMu    sync.Mutex
	generation atomic.Pointer[storeGeneration]

	subMu       sync.RWMutex
	subscribers map[uint64]chan Event
	nextID      atomic.Uint64
	worldMaps   *WorldMapStore
}

func NewStore(initial GameState) *Store {
	return newStore(initial, nil)
}

func NewStoreWithWorldMap(initial GameState, worldMaps *WorldMapStore) *Store {
	return newStore(initial, worldMaps)
}

func newStore(initial GameState, worldMaps *WorldMapStore) *Store {
	if initial.SchemaVersion == 0 {
		initial = NewGameState()
	}
	normalizeStateMaps(&initial)
	normalizeFocusedCastle(&initial, 0)
	owned := cloneGameState(initial)
	owned.initializeStormTargets()
	owned.initializeTowerCooldowns()
	owned.initializeReports()
	owned.initializeEventScores()
	owned.initializeMovements()
	store := &Store{subscribers: map[uint64]chan Event{}, worldMaps: worldMaps}
	if worldMaps != nil {
		pruneIrrelevantMapObservations(&owned)
		worldID := gameStateWorldID(owned)
		if worldID != "" {
			owned.worldSharing = true
			changes := extractShareableMapObservations(&owned)
			worldEvent := worldMaps.commit(worldID, store, changes)
			owned.sharedMap = worldEvent.generation
			if owned.sharedMap == nil {
				owned.sharedMap = worldMaps.Snapshot(worldID)
			}
		}
	}
	owned.compactMapOverlay()
	owned.pendingMapChanges = nil
	store.generation.Store(&storeGeneration{
		state:    &owned,
		versions: &partitionVersionSnapshot{},
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
	return cloneGameState(*generation.state)
}

// ReadOnlyView returns the current immutable generation without cloning it.
// Callers must not mutate the returned state or any of its nested values.
func (store *Store) ReadOnlyView() GameState {
	generation := store.generation.Load()
	if generation == nil {
		return NewGameState()
	}
	return *generation.state
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
	return cloneGameState(*generation.state), generation.state.Revision
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
		State:           *generation.state,
		Partitions:      PartitionVersions{snapshot: generation.versions},
		ProtocolContext: generation.protocol,
	}
}

// IngestObservationView is the bounded identity/session projection needed to
// bind a decoded transport frame to the generation that observed it. It avoids
// copying the full top-level GameState on every websocket frame while keeping
// all fields sourced from one immutable generation.
type IngestObservationView struct {
	ServerURL            string
	PlayerID             PlayerID
	CatalogVersion       string
	SessionGeneration    uint64
	ConnectionGeneration uint64
	FocusEpoch           uint64
	FocusedCastleID      CastleID
}

func (store *Store) IngestObservationView() IngestObservationView {
	generation := store.generation.Load()
	if generation == nil || generation.state == nil {
		return IngestObservationView{}
	}
	state := generation.state
	return IngestObservationView{
		ServerURL: strings.TrimSpace(state.Session.ServerURL), PlayerID: state.Player.ID,
		CatalogVersion: state.CatalogVersion, SessionGeneration: generation.protocol.SessionGeneration,
		ConnectionGeneration: generation.protocol.ConnectionGeneration,
		FocusEpoch:           generation.protocol.FocusEpoch, FocusedCastleID: generation.protocol.FocusedCastleID,
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

// ObserveProtocolFocus advances the ephemeral protocol focus epoch without
// creating a GameState revision or dirty component. It is used by successful
// context-setting frames whose payload has no retained state projection.
func (store *Store) ObserveProtocolFocus(subcontext FocusSubcontext, observedAt time.Time) {
	if store == nil || subcontext == FocusSubcontextUnknown {
		return
	}
	store.writeMu.Lock()
	defer store.writeMu.Unlock()
	current := store.generation.Load()
	if current == nil {
		return
	}
	if observedAt.IsZero() {
		observedAt = time.Now().UTC()
	}
	protocol := nextProtocolContext(current.protocol, *current.state, nil, nil, subcontext, observedAt)
	if protocol != current.protocol {
		store.generation.Store(&storeGeneration{state: current.state, versions: current.versions, protocol: protocol})
	}
}

// ObserveRecruitmentBUP records one committed recruitment enqueue in the
// exact castle-focus epoch that accepted it. Once a correlated AHR succeeds,
// later BUPs in that uninterrupted epoch remain covered only while exact
// lifecycle evidence is pending or inside its bounded completion grace.
func (store *Store) ObserveRecruitmentBUP(
	castleID CastleID,
	sessionGeneration uint64,
	connectionGeneration uint64,
	focusEpoch uint64,
) bool {
	if store == nil || castleID <= 0 || sessionGeneration == 0 || connectionGeneration == 0 || focusEpoch == 0 {
		return false
	}
	store.writeMu.Lock()
	defer store.writeMu.Unlock()
	current := store.generation.Load()
	if current == nil || current.protocol.SessionGeneration != sessionGeneration ||
		current.protocol.ConnectionGeneration != connectionGeneration ||
		current.protocol.FocusedCastleID != castleID ||
		current.protocol.FocusSubcontext != FocusSubcontextCastle ||
		current.protocol.FocusEpoch != focusEpoch {
		return false
	}
	protocol := current.protocol
	if protocol.RecruitmentBUPCastleID != castleID || protocol.RecruitmentBUPFocusEpoch != focusEpoch {
		protocol.RecruitmentBUPCastleID = castleID
		protocol.RecruitmentBUPFocusEpoch = focusEpoch
		protocol.RecruitmentBUPSerial = 0
		protocol.RecruitmentAHRCoveredSerial = 0
		protocol.RecruitmentAHRFocusCovered = false
		protocol.RecruitmentAHRPending = false
	}
	epochAlreadyCovered := protocol.RecruitmentAHRFocusCovered &&
		protocol.RecruitmentAHRCoveredSerial == protocol.RecruitmentBUPSerial &&
		RecruitmentAllianceHelpCovers(*current.state, castleID, time.Now().UTC(), 0)
	if !epochAlreadyCovered {
		protocol.RecruitmentAHRFocusCovered = false
	}
	protocol.RecruitmentBUPSerial++
	if epochAlreadyCovered {
		protocol.RecruitmentAHRCoveredSerial = protocol.RecruitmentBUPSerial
	}
	store.generation.Store(&storeGeneration{state: current.state, versions: current.versions, protocol: protocol})
	return true
}

// ObserveRecruitmentAHRCovered marks every committed recruitment BUP in the
// exact current focus epoch as covered after its correlated AHR succeeds.
func (store *Store) ObserveRecruitmentAHRCovered(
	castleID CastleID,
	sessionGeneration uint64,
	connectionGeneration uint64,
	focusEpoch uint64,
	bupSerial uint64,
) bool {
	if store == nil || castleID <= 0 || sessionGeneration == 0 || connectionGeneration == 0 ||
		focusEpoch == 0 || bupSerial == 0 {
		return false
	}
	store.writeMu.Lock()
	defer store.writeMu.Unlock()
	current := store.generation.Load()
	if current == nil || current.protocol.SessionGeneration != sessionGeneration ||
		current.protocol.ConnectionGeneration != connectionGeneration ||
		current.protocol.FocusedCastleID != castleID ||
		current.protocol.FocusSubcontext != FocusSubcontextCastle ||
		current.protocol.FocusEpoch != focusEpoch ||
		current.protocol.RecruitmentBUPCastleID != castleID ||
		current.protocol.RecruitmentBUPFocusEpoch != focusEpoch ||
		current.protocol.RecruitmentBUPSerial != bupSerial ||
		current.protocol.RecruitmentAHRCoveredSerial >= bupSerial ||
		!RecruitmentAllianceHelpCovers(*current.state, castleID, time.Now().UTC(), 0) {
		return false
	}
	protocol := current.protocol
	protocol.RecruitmentAHRCoveredSerial = bupSerial
	protocol.RecruitmentAHRFocusCovered = true
	protocol.RecruitmentAHRPending = false
	store.generation.Store(&storeGeneration{state: current.state, versions: current.versions, protocol: protocol})
	return true
}

// PrepareStandaloneRecruitmentAHR binds the next standalone AHR to the exact
// current castle-focus epoch before it is dispatched. A focus transition
// clears this marker, preventing a later reconciliation from attaching the
// response to a new epoch.
func (store *Store) PrepareStandaloneRecruitmentAHR(
	castleID CastleID,
	sessionGeneration uint64,
	connectionGeneration uint64,
	focusEpoch uint64,
) bool {
	if store == nil || castleID <= 0 || sessionGeneration == 0 || connectionGeneration == 0 || focusEpoch == 0 {
		return false
	}
	store.writeMu.Lock()
	defer store.writeMu.Unlock()
	current := store.generation.Load()
	if current == nil || current.protocol.SessionGeneration != sessionGeneration ||
		current.protocol.ConnectionGeneration != connectionGeneration ||
		current.protocol.FocusedCastleID != castleID ||
		current.protocol.FocusSubcontext != FocusSubcontextCastle ||
		current.protocol.FocusEpoch != focusEpoch {
		return false
	}
	protocol := current.protocol
	if protocol.RecruitmentBUPCastleID != castleID || protocol.RecruitmentBUPFocusEpoch != focusEpoch {
		protocol.RecruitmentBUPCastleID = castleID
		protocol.RecruitmentBUPFocusEpoch = focusEpoch
		protocol.RecruitmentBUPSerial = 0
		protocol.RecruitmentAHRCoveredSerial = 0
	}
	protocol.RecruitmentAHRFocusCovered = false
	protocol.RecruitmentAHRPending = true
	store.generation.Store(&storeGeneration{state: current.state, versions: current.versions, protocol: protocol})
	return true
}

// ObserveStandaloneRecruitmentAHRCovered links a correlated standalone AHR to
// the exact current castle-focus epoch. Unlike the causal BUP marker, this may
// establish coverage before the epoch has any BUP serial; the first later BUP
// can inherit it only while exact current lifecycle evidence still covers.
func (store *Store) ObserveStandaloneRecruitmentAHRCovered(
	castleID CastleID,
	sessionGeneration uint64,
	connectionGeneration uint64,
	focusEpoch uint64,
	observedAt time.Time,
) bool {
	if store == nil || castleID <= 0 || sessionGeneration == 0 || connectionGeneration == 0 ||
		focusEpoch == 0 || observedAt.IsZero() {
		return false
	}
	store.writeMu.Lock()
	defer store.writeMu.Unlock()
	current := store.generation.Load()
	if current == nil || current.protocol.SessionGeneration != sessionGeneration ||
		current.protocol.ConnectionGeneration != connectionGeneration ||
		current.protocol.FocusedCastleID != castleID ||
		current.protocol.FocusSubcontext != FocusSubcontextCastle ||
		current.protocol.FocusEpoch != focusEpoch ||
		current.protocol.RecruitmentBUPCastleID != castleID ||
		current.protocol.RecruitmentBUPFocusEpoch != focusEpoch ||
		!current.protocol.RecruitmentAHRPending ||
		!RecruitmentAllianceHelpCovers(*current.state, castleID, observedAt, 0) {
		return false
	}
	protocol := current.protocol
	if protocol.RecruitmentBUPCastleID != castleID || protocol.RecruitmentBUPFocusEpoch != focusEpoch {
		protocol.RecruitmentBUPCastleID = castleID
		protocol.RecruitmentBUPFocusEpoch = focusEpoch
		protocol.RecruitmentBUPSerial = 0
	}
	protocol.RecruitmentAHRCoveredSerial = protocol.RecruitmentBUPSerial
	protocol.RecruitmentAHRFocusCovered = true
	protocol.RecruitmentAHRPending = false
	store.generation.Store(&storeGeneration{state: current.state, versions: current.versions, protocol: protocol})
	return true
}

func (store *Store) Session() SessionState {
	generation := store.generation.Load()
	if generation == nil {
		return SessionState{}
	}
	return cloneSessionState(generation.state.Session)
}

// AccountIdentity reports the bound game identity of this profile: the
// canonical world plus the game player ID, which together stay stable for the
// life of the game account no matter which hosted account or runtime plays
// it. ok is false until the session has bound and confirmed the identity.
func (store *Store) AccountIdentity() (worldID string, playerID int64, ok bool) {
	generation := store.generation.Load()
	if generation == nil {
		return "", 0, false
	}
	account := generation.state.Account
	world := CanonicalWorldID(account.WorldID)
	if world == "" || account.PlayerID <= 0 || account.BoundAt.IsZero() {
		return "", 0, false
	}
	return world, int64(account.PlayerID), true
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

// ApplyWithoutMapMutation skips cloning the large world-map cache. The
// mutation must not modify GameState.Map or any map nested beneath it.
func (store *Store) ApplyWithoutMapMutation(mutation Mutation) (Event, error) {
	if mutation == nil {
		return Event{}, fmt.Errorf("state mutation is required")
	}
	return store.ApplyScopedWithoutMapMutation(func(state *GameState) (ScopedChange, error) {
		domains, changed, err := mutation(state)
		return ScopedChange{Domains: domains, Changed: changed}, err
	})
}

func (store *Store) ApplyScoped(mutation ScopedMutation) (Event, error) {
	return store.applyScoped(AllComponents, mutation)
}

// ApplyScopedWithoutMapMutation is the scoped form of
// ApplyWithoutMapMutation. The mutation may read GameState.Map but must not
// modify it or any nested map.
func (store *Store) ApplyScopedWithoutMapMutation(mutation ScopedMutation) (Event, error) {
	return store.applyScoped(AllAccountComponents, mutation)
}

// ApplyComponents copies only the declared top-level components before the
// mutation runs. The mutation may read the entire state but must modify only
// components present in writes.
func (store *Store) ApplyComponents(writes ComponentSet, mutation Mutation) (Event, error) {
	if mutation == nil {
		return Event{}, fmt.Errorf("state mutation is required")
	}
	return store.ApplyScopedComponents(writes, func(state *GameState) (ScopedChange, error) {
		domains, changed, err := mutation(state)
		return ScopedChange{Domains: domains, Changed: changed}, err
	})
}

// ApplyScopedComponents is the transactional selective-copy mutation path.
// Every production reducer should use this method with an explicit write set;
// the broad Apply methods remain only as compatibility guards for external and
// test callers.
func (store *Store) ApplyScopedComponents(writes ComponentSet, mutation ScopedMutation) (Event, error) {
	if writes == 0 {
		return Event{}, fmt.Errorf("state mutation write components are required")
	}
	if writes&^AllComponents != 0 {
		return Event{}, fmt.Errorf("state mutation contains unknown write components")
	}
	return store.applyScoped(writes, mutation)
}

func (store *Store) applyScoped(writes ComponentSet, mutation ScopedMutation) (Event, error) {
	if mutation == nil {
		return Event{}, fmt.Errorf("scoped state mutation is required")
	}
	store.writeMu.Lock()
	defer store.writeMu.Unlock()
	current := store.generation.Load()
	if current == nil {
		initial := NewGameState()
		current = &storeGeneration{
			state:    &initial,
			versions: &partitionVersionSnapshot{},
			protocol: initialProtocolContext(initial),
		}
	}
	candidate := cloneGameStateForMutation(*current.state, writes)
	candidate.mutationWrites = writes
	candidate.pendingMapChanges = nil
	change, err := mutation(candidate)
	if err != nil {
		return Event{}, err
	}
	if !change.Changed {
		if change.FocusSubcontext != FocusSubcontextUnknown {
			now := time.Now().UTC()
			protocol := nextProtocolContext(current.protocol, *current.state, nil, nil, change.FocusSubcontext, now)
			if protocol != current.protocol {
				store.generation.Store(&storeGeneration{
					state: current.state, versions: current.versions, protocol: protocol,
				})
			}
		}
		return Event{Revision: current.state.Revision}, nil
	}
	effectiveWrites := writes
	if change.DirtyComponentsSet {
		if change.DirtyComponents&^writes != 0 {
			return Event{}, fmt.Errorf("dirty components exceed declared mutation writes")
		}
		effectiveWrites = change.DirtyComponents
	}
	if effectiveWrites == 0 {
		return Event{}, fmt.Errorf("changed state mutation has no dirty components")
	}
	if effectiveWrites.Has(ComponentCastles) {
		normalizeFocusedCastle(candidate, current.protocol.FocusedCastleID)
	}
	castleCOW := candidate.castleMutationCOW
	castleIDs := candidate.castleChangeIDs()
	castleParts := candidate.castleChangeParts()
	replaceCastles := effectiveWrites.Has(ComponentCastles) && (!castleCOW || candidate.replaceCastles)
	inventoryCOW := candidate.inventoryMutationCOW
	inventoryParts := candidate.mutableInventoryParts
	replaceInventory := effectiveWrites.Has(ComponentInventory) && !inventoryCOW
	equipmentIDs := candidate.inventoryEquipmentChangeIDs()
	replaceEquipment := candidate.replaceInventoryEquipment
	gemIDs := candidate.inventoryGemChangeIDs()
	replaceGems := candidate.replaceInventoryGems
	itemKeys := candidate.inventoryItemChangeKeys()
	replaceItems := candidate.replaceInventoryItems
	stormTargetKeys := candidate.stormTargetChangeKeys()
	replaceStorm := candidate.replaceStormTargets
	towerCooldownKeys := candidate.towerCooldownChangeKeys()
	replaceCooldowns := candidate.replaceTowerCooldowns
	towerQueueCastles := candidate.towerQueueChangeCastleIDs()
	replaceTowerQueue := candidate.replaceTowerQueue
	reportMessageIDs := candidate.reportMessageChangeIDs()
	replaceReports := candidate.replaceReports
	eventScoreIDs := candidate.eventScoreChangeIDs()
	eventScoreMeta := candidate.eventScoreMetadataDirty
	eventScoreShop := candidate.eventScoreShopDirty
	replaceEventScores := candidate.replaceEventScores
	if candidate.movementMutationCOW && candidate.Movements != nil {
		candidate.ReplaceMovements(candidate.Movements)
	}
	movementIDs := candidate.movementChangeIDs()
	replaceMovements := candidate.replaceMovements
	mapChanges := candidate.mapChanges()
	mapCOW := candidate.mapMutationCOW
	replaceMap := candidate.replaceMap
	if store.worldMaps != nil {
		currentWorldID := gameStateWorldID(*current.state)
		nextWorldID := gameStateWorldID(*candidate)
		worldChanged := currentWorldID != nextWorldID
		replaceMap = worldChanged || candidate.replaceMap
		if worldChanged && !effectiveWrites.Has(ComponentWorldMap) {
			candidate.prepareMapMutation(*current.state)
			candidate.mutationWrites |= Components(ComponentWorldMap)
			effectiveWrites |= Components(ComponentWorldMap)
		}
		candidate.worldSharing = nextWorldID != ""
		if candidate.worldSharing {
			if effectiveWrites.Has(ComponentWorldMap) {
				mapChanges = normalizeMapChanges(append(mapChanges, extractShareableMapObservations(candidate)...))
				worldEvent := store.worldMaps.commitWithDomains(nextWorldID, store, mapChanges, change.Domains)
				candidate.sharedMap = worldEvent.generation
				if candidate.sharedMap == nil {
					candidate.sharedMap = store.worldMaps.Snapshot(nextWorldID)
				}
			}
		} else {
			candidate.sharedMap = nil
		}
	}
	if effectiveWrites.Has(ComponentWorldMap) && mapCOW && !replaceMap && len(mapChanges) == 0 {
		effectiveWrites &^= Components(ComponentWorldMap)
	}
	if effectiveWrites.Has(ComponentCastles) && castleCOW && !replaceCastles && len(castleIDs) == 0 {
		effectiveWrites &^= Components(ComponentCastles)
	}
	if effectiveWrites.Has(ComponentInventory) && inventoryCOW && inventoryParts == 0 {
		effectiveWrites &^= Components(ComponentInventory)
	}
	if effectiveWrites.Has(ComponentMovements) && candidate.movementMutationCOW && !replaceMovements && len(movementIDs) == 0 {
		effectiveWrites &^= Components(ComponentMovements)
	}
	if effectiveWrites.Has(ComponentTowerCooldowns) && candidate.towerCooldownMutationCOW &&
		!replaceCooldowns && len(towerCooldownKeys) == 0 {
		effectiveWrites &^= Components(ComponentTowerCooldowns)
	}
	if effectiveWrites.Has(ComponentTowerQueue) && candidate.towerQueueMutationCOW &&
		!replaceTowerQueue && len(towerQueueCastles) == 0 {
		effectiveWrites &^= Components(ComponentTowerQueue)
	}
	if effectiveWrites.Has(ComponentEventScores) && candidate.eventScoreMutationCOW &&
		!replaceEventScores && len(eventScoreIDs) == 0 && !eventScoreMeta && !eventScoreShop {
		effectiveWrites &^= Components(ComponentEventScores)
	}
	if effectiveWrites.Has(ComponentAttackAnalytics) && candidate.attackAnalyticsMutationCOW &&
		candidate.mutableAttackAnalyticsParts == 0 {
		effectiveWrites &^= Components(ComponentAttackAnalytics)
	}
	// Multi-projection official envelopes declare a conservative write union.
	// Once keyed mutation logs have removed untouched large collections, compare
	// the remaining bounded components so one changed projection does not cause
	// unrelated client patches or persistence writes. The account baseline is
	// the deliberate full-refresh exception and avoids this comparison pass.
	if effectiveWrites != AllComponents && effectiveWrites&(effectiveWrites-1) != 0 {
		if effectiveWrites.Has(ComponentCatalog) && candidate.CatalogVersion == current.state.CatalogVersion &&
			candidate.LanguageVersion == current.state.LanguageVersion {
			effectiveWrites &^= Components(ComponentCatalog)
		}
		if effectiveWrites.Has(ComponentSession) && reflect.DeepEqual(candidate.Session, current.state.Session) {
			effectiveWrites &^= Components(ComponentSession)
		}
		if effectiveWrites.Has(ComponentAccount) && reflect.DeepEqual(candidate.Account, current.state.Account) {
			effectiveWrites &^= Components(ComponentAccount)
		}
		if effectiveWrites.Has(ComponentPlayer) && reflect.DeepEqual(candidate.Player, current.state.Player) {
			effectiveWrites &^= Components(ComponentPlayer)
		}
		if effectiveWrites.Has(ComponentGenerals) && reflect.DeepEqual(candidate.Generals, current.state.Generals) {
			effectiveWrites &^= Components(ComponentGenerals)
		}
		if effectiveWrites.Has(ComponentCastellans) && reflect.DeepEqual(candidate.Castellans, current.state.Castellans) {
			effectiveWrites &^= Components(ComponentCastellans)
		}
		if effectiveWrites.Has(ComponentMovementSnapshot) && reflect.DeepEqual(candidate.MovementSnapshot, current.state.MovementSnapshot) {
			effectiveWrites &^= Components(ComponentMovementSnapshot)
		}
		if effectiveWrites.Has(ComponentStationing) && reflect.DeepEqual(candidate.Stationing, current.state.Stationing) {
			effectiveWrites &^= Components(ComponentStationing)
		}
		if effectiveWrites.Has(ComponentScheduled) && reflect.DeepEqual(candidate.Scheduled, current.state.Scheduled) {
			effectiveWrites &^= Components(ComponentScheduled)
		}
		if effectiveWrites.Has(ComponentRift) && reflect.DeepEqual(candidate.Rift, current.state.Rift) {
			effectiveWrites &^= Components(ComponentRift)
		}
		if effectiveWrites.Has(ComponentSubscriptions) && reflect.DeepEqual(candidate.Subscriptions, current.state.Subscriptions) {
			effectiveWrites &^= Components(ComponentSubscriptions)
		}
		if effectiveWrites.Has(ComponentMarket) && reflect.DeepEqual(candidate.Market, current.state.Market) {
			effectiveWrites &^= Components(ComponentMarket)
		}
		if effectiveWrites.Has(ComponentKingdomTransport) && reflect.DeepEqual(candidate.KingdomTransport, current.state.KingdomTransport) {
			effectiveWrites &^= Components(ComponentKingdomTransport)
		}
		if effectiveWrites.Has(ComponentAlliance) && reflect.DeepEqual(candidate.Alliance, current.state.Alliance) {
			effectiveWrites &^= Components(ComponentAlliance)
		}
		if effectiveWrites.Has(ComponentAlliances) && reflect.DeepEqual(candidate.Alliances, current.state.Alliances) {
			effectiveWrites &^= Components(ComponentAlliances)
		}
		if effectiveWrites.Has(ComponentAllianceHelp) && reflect.DeepEqual(candidate.AllianceHelpRequests, current.state.AllianceHelpRequests) {
			effectiveWrites &^= Components(ComponentAllianceHelp)
		}
		if effectiveWrites.Has(ComponentInvasion) && reflect.DeepEqual(candidate.Invasion, current.state.Invasion) {
			effectiveWrites &^= Components(ComponentInvasion)
		}
		if effectiveWrites.Has(ComponentDailyAttacks) && reflect.DeepEqual(candidate.DailyAttacks, current.state.DailyAttacks) {
			effectiveWrites &^= Components(ComponentDailyAttacks)
		}
		if effectiveWrites.Has(ComponentAttackDialog) && reflect.DeepEqual(candidate.AttackDialog, current.state.AttackDialog) {
			effectiveWrites &^= Components(ComponentAttackDialog)
		}
		if effectiveWrites.Has(ComponentAttackPresets) && reflect.DeepEqual(candidate.AttackPresets, current.state.AttackPresets) {
			effectiveWrites &^= Components(ComponentAttackPresets)
		}
		if effectiveWrites.Has(ComponentCommandContext) && reflect.DeepEqual(candidate.CommandContext, current.state.CommandContext) {
			effectiveWrites &^= Components(ComponentCommandContext)
		}
		if effectiveWrites.Has(ComponentAutomations) && reflect.DeepEqual(candidate.Automations, current.state.Automations) {
			effectiveWrites &^= Components(ComponentAutomations)
		}
		if effectiveWrites.Has(ComponentObservations) && reflect.DeepEqual(candidate.Observations, current.state.Observations) {
			effectiveWrites &^= Components(ComponentObservations)
		}
	}
	// A few official envelopes update several small logical projections from
	// one payload. Their reducers retain a conservative COW union, then this
	// bounded equality pass removes unchanged projections before publication
	// and persistence. High-cardinality domains above use keyed mutation logs
	// and never pay for a whole-map comparison here.
	if effectiveWrites.Has(ComponentCommanders) && reflect.DeepEqual(candidate.Commanders, current.state.Commanders) {
		effectiveWrites &^= Components(ComponentCommanders)
	}
	if effectiveWrites.Has(ComponentKhan) && reflect.DeepEqual(candidate.Khan, current.state.Khan) {
		effectiveWrites &^= Components(ComponentKhan)
	}
	if effectiveWrites.Has(ComponentAdvisor) && reflect.DeepEqual(candidate.Advisor, current.state.Advisor) {
		effectiveWrites &^= Components(ComponentAdvisor)
	}
	if effectiveWrites.Has(ComponentBeri) && reflect.DeepEqual(candidate.Beri, current.state.Beri) {
		effectiveWrites &^= Components(ComponentBeri)
	}
	if effectiveWrites.Has(ComponentNomadCamps) && reflect.DeepEqual(candidate.NomadCamps, current.state.NomadCamps) {
		effectiveWrites &^= Components(ComponentNomadCamps)
	}
	if effectiveWrites.Has(ComponentStorm) && effectiveWrites != Components(ComponentStorm) &&
		!replaceStorm && len(stormTargetKeys) == 0 &&
		reflect.DeepEqual(candidate.Storm, current.state.Storm) {
		effectiveWrites &^= Components(ComponentStorm)
	}
	if effectiveWrites == 0 {
		return Event{}, fmt.Errorf("changed state mutation produced no tracked component changes")
	}
	candidate.compactMapOverlay()
	candidate.pendingMapChanges = nil
	candidate.replaceMap = false
	candidate.mapMutationCOW = false
	candidate.mutableMapRegions = nil
	candidate.movementMutationCOW = false
	candidate.mutableMovementShards = [4]uint64{}
	candidate.pendingMovementChanges = nil
	candidate.replaceMovements = false
	candidate.castleMutationCOW = false
	candidate.mutableCastles = nil
	candidate.pendingCastleChanges = nil
	candidate.replaceCastles = false
	candidate.inventoryMutationCOW = false
	candidate.mutableInventoryParts = 0
	candidate.pendingEquipmentChanges = nil
	candidate.replaceInventoryEquipment = false
	candidate.pendingGemChanges = nil
	candidate.replaceInventoryGems = false
	candidate.pendingInventoryItemChanges = nil
	candidate.replaceInventoryItems = false
	candidate.stormMutationCOW = false
	candidate.mutableStormParts = 0
	candidate.mutableStormTargetShards = [4]uint64{}
	candidate.pendingStormTargetChanges = nil
	candidate.replaceStormTargets = false
	candidate.towerCooldownMutationCOW = false
	candidate.mutableTowerCooldownShards = [4]uint64{}
	candidate.pendingTowerCooldownChanges = nil
	candidate.replaceTowerCooldowns = false
	candidate.towerQueueMutationCOW = false
	candidate.mutableTowerQueueEntries = nil
	candidate.pendingTowerQueueCastles = nil
	candidate.replaceTowerQueue = false
	candidate.reportMutationCOW = false
	candidate.mutableReportShards = [4]uint64{}
	candidate.pendingReportMessages = nil
	candidate.replaceReports = false
	candidate.eventScoreMutationCOW = false
	candidate.mutableEventScoreShards = [4]uint64{}
	candidate.pendingEventScoreIDs = nil
	candidate.eventScoreMetadataDirty = false
	candidate.eventScoreShopDirty = false
	candidate.replaceEventScores = false
	candidate.attackAnalyticsMutationCOW = false
	candidate.mutableAttackAnalyticsParts = 0
	candidate.mutationWrites = 0
	candidate.Revision++
	candidate.UpdatedAt = time.Now().UTC()
	domains := normalizeDomains(change.Domains)
	partitions := append(defaultPartitionKeys(*candidate, domains), change.Partitions...)
	partitionSnapshot, changedPartitions := advancePartitionVersions(
		current.versions, partitions, candidate.Revision, candidate.UpdatedAt,
	)
	protocol := nextProtocolContext(
		current.protocol, *candidate, domains, change.Partitions, change.FocusSubcontext, candidate.UpdatedAt,
	)
	next := &storeGeneration{state: candidate, versions: partitionSnapshot, protocol: protocol}
	store.generation.Store(next)
	event := Event{
		Sequence: candidate.Revision, Revision: candidate.Revision, Domains: domains,
		Components: effectiveWrites.List(),
		Partitions: changedPartitions, OccurredAt: candidate.UpdatedAt,
		generation: next, mapChanges: mapChanges, replaceMap: replaceMap,
		castleIDs: castleIDs, replaceCastles: replaceCastles,
		castleParts:    castleParts,
		inventoryParts: inventoryParts, replaceInventory: replaceInventory,
		equipmentIDs: equipmentIDs, replaceEquipment: replaceEquipment,
		gemIDs: gemIDs, replaceGems: replaceGems,
		itemKeys: itemKeys, replaceItems: replaceItems,
		stormTargetKeys: stormTargetKeys, replaceStorm: replaceStorm,
		towerCooldownKeys: towerCooldownKeys, replaceCooldowns: replaceCooldowns,
		towerQueueCastles: towerQueueCastles, replaceTowerQueue: replaceTowerQueue,
		reportMessageIDs: reportMessageIDs, replaceReports: replaceReports,
		eventScoreIDs: eventScoreIDs, eventScoreMeta: eventScoreMeta,
		eventScoreShop: eventScoreShop, replaceEventScores: replaceEventScores,
		movementIDs: movementIDs, replaceMovements: replaceMovements,
	}
	event.Patch = componentPatch(next, effectiveWrites, componentChanges{
		mapChanges: mapChanges, replaceMap: replaceMap,
		castleIDs: castleIDs, replaceCastles: replaceCastles,
		castleParts:    castleParts,
		inventoryParts: inventoryParts, replaceInventory: replaceInventory,
		equipmentIDs: equipmentIDs, replaceEquipment: replaceEquipment,
		gemIDs: gemIDs, replaceGems: replaceGems,
		itemKeys: itemKeys, replaceItems: replaceItems,
		stormTargetKeys: stormTargetKeys, replaceStorm: replaceStorm,
		towerCooldownKeys: towerCooldownKeys, replaceCooldowns: replaceCooldowns,
		towerQueueCastles: towerQueueCastles, replaceTowerQueue: replaceTowerQueue,
		reportMessageIDs: reportMessageIDs, replaceReports: replaceReports,
		eventScoreIDs: eventScoreIDs, eventScoreMeta: eventScoreMeta,
		eventScoreShop: eventScoreShop, replaceEventScores: replaceEventScores,
		movementIDs: movementIDs, replaceMovements: replaceMovements,
	})
	event.clientEncoding = &clientEventEncoding{}
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
			state.SetCastleParts(castleID, castle, CastlePartIdentity)
		}
	}
}

func cloneSessionState(session SessionState) SessionState {
	session.CooldownUntil = cloneTimePointer(session.CooldownUntil)
	session.RetryAt = cloneTimePointer(session.RetryAt)
	session.LoginFailure = cloneLoginFailure(session.LoginFailure)
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
		Components: normalizeComponents(
			append(append([]Component(nil), left.Components...), right.Components...),
		),
		Partitions: mergePartitionVersions(
			append(append([]PartitionVersion(nil), left.Partitions...), right.Partitions...),
		),
		OccurredAt:         left.OccurredAt,
		generation:         right.generation,
		mapChanges:         normalizeMapChanges(append(append([]MapChange(nil), left.mapChanges...), right.mapChanges...)),
		replaceMap:         left.replaceMap || right.replaceMap,
		castleIDs:          mergeCastleChangeIDs(left.castleIDs, right.castleIDs),
		castleParts:        mergeCastleChangeParts(left.castleParts, right.castleParts),
		replaceCastles:     left.replaceCastles || right.replaceCastles,
		inventoryParts:     left.inventoryParts | right.inventoryParts,
		replaceInventory:   left.replaceInventory || right.replaceInventory,
		equipmentIDs:       mergeNumericChangeIDs(left.equipmentIDs, right.equipmentIDs),
		replaceEquipment:   left.replaceEquipment || right.replaceEquipment,
		gemIDs:             mergeNumericChangeIDs(left.gemIDs, right.gemIDs),
		replaceGems:        left.replaceGems || right.replaceGems,
		itemKeys:           mergeStringChangeKeys(left.itemKeys, right.itemKeys),
		replaceItems:       left.replaceItems || right.replaceItems,
		stormTargetKeys:    mergeStringChangeKeys(left.stormTargetKeys, right.stormTargetKeys),
		replaceStorm:       left.replaceStorm || right.replaceStorm,
		towerCooldownKeys:  mergeStringChangeKeys(left.towerCooldownKeys, right.towerCooldownKeys),
		replaceCooldowns:   left.replaceCooldowns || right.replaceCooldowns,
		towerQueueCastles:  mergeCastleChangeIDs(left.towerQueueCastles, right.towerQueueCastles),
		replaceTowerQueue:  left.replaceTowerQueue || right.replaceTowerQueue,
		reportMessageIDs:   mergeNumericChangeIDs(left.reportMessageIDs, right.reportMessageIDs),
		replaceReports:     left.replaceReports || right.replaceReports,
		eventScoreIDs:      mergeNumericChangeIDs(left.eventScoreIDs, right.eventScoreIDs),
		eventScoreMeta:     left.eventScoreMeta || right.eventScoreMeta,
		eventScoreShop:     left.eventScoreShop || right.eventScoreShop,
		replaceEventScores: left.replaceEventScores || right.replaceEventScores,
		movementIDs:        mergeNumericChangeIDs(left.movementIDs, right.movementIDs),
		replaceMovements:   left.replaceMovements || right.replaceMovements,
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
	if merged.generation == nil {
		merged.generation = left.generation
	}
	merged.Patch = componentPatch(merged.generation, Components(merged.Components...), componentChanges{
		mapChanges: merged.mapChanges, replaceMap: merged.replaceMap,
		castleIDs: merged.castleIDs, replaceCastles: merged.replaceCastles,
		castleParts:    merged.castleParts,
		inventoryParts: merged.inventoryParts, replaceInventory: merged.replaceInventory,
		equipmentIDs: merged.equipmentIDs, replaceEquipment: merged.replaceEquipment,
		gemIDs: merged.gemIDs, replaceGems: merged.replaceGems,
		itemKeys: merged.itemKeys, replaceItems: merged.replaceItems,
		stormTargetKeys: merged.stormTargetKeys, replaceStorm: merged.replaceStorm,
		towerCooldownKeys: merged.towerCooldownKeys, replaceCooldowns: merged.replaceCooldowns,
		towerQueueCastles: merged.towerQueueCastles, replaceTowerQueue: merged.replaceTowerQueue,
		reportMessageIDs: merged.reportMessageIDs, replaceReports: merged.replaceReports,
		eventScoreIDs: merged.eventScoreIDs, eventScoreMeta: merged.eventScoreMeta,
		eventScoreShop: merged.eventScoreShop, replaceEventScores: merged.replaceEventScores,
		movementIDs: merged.movementIDs, replaceMovements: merged.replaceMovements,
	})
	merged.clientEncoding = &clientEventEncoding{}
	return merged
}

func mergeCastleChangeIDs(left []CastleID, right []CastleID) []CastleID {
	unique := make(map[CastleID]struct{}, len(left)+len(right))
	for _, id := range left {
		unique[id] = struct{}{}
	}
	for _, id := range right {
		unique[id] = struct{}{}
	}
	ids := make([]CastleID, 0, len(unique))
	for id := range unique {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	return ids
}

func mergeCastleChangeParts(
	left map[CastleID]CastleMutationPart,
	right map[CastleID]CastleMutationPart,
) map[CastleID]CastleMutationPart {
	merged := make(map[CastleID]CastleMutationPart, len(left)+len(right))
	for id, parts := range left {
		merged[id] |= parts
	}
	for id, parts := range right {
		merged[id] |= parts
	}
	return merged
}

func mergeNumericChangeIDs[T ~int64](left []T, right []T) []T {
	unique := make(map[T]struct{}, len(left)+len(right))
	for _, id := range left {
		unique[id] = struct{}{}
	}
	for _, id := range right {
		unique[id] = struct{}{}
	}
	ids := make([]T, 0, len(unique))
	for id := range unique {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	return ids
}

func mergeStringChangeKeys(left []string, right []string) []string {
	unique := make(map[string]struct{}, len(left)+len(right))
	for _, key := range left {
		unique[key] = struct{}{}
	}
	for _, key := range right {
		unique[key] = struct{}{}
	}
	keys := make([]string, 0, len(unique))
	for key := range unique {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
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
	out := make([]string, 0, len(domains))
	for _, domain := range domains {
		if domain == "" {
			continue
		}
		duplicate := false
		for _, existing := range out {
			if existing == domain {
				duplicate = true
				break
			}
		}
		if !duplicate {
			out = append(out, domain)
		}
	}
	sort.Strings(out)
	return out
}

func cloneGameState(source GameState) GameState {
	return cloneGameStateComponents(source, AllComponents)
}

func cloneGameStateForMutation(source GameState, components ComponentSet) *GameState {
	// The broad compatibility APIs permit arbitrary nested writes and therefore
	// retain their defensive deep-copy behavior. Production reducers use an
	// explicit component set and receive the coordinate-level map COW path.
	if components == AllComponents {
		clone := cloneGameStateComponents(source, components)
		return &clone
	}
	withoutKeyed := components &^ Components(
		ComponentWorldMap, ComponentCastles, ComponentInventory, ComponentStorm, ComponentTowerCooldowns,
		ComponentTowerQueue,
		ComponentReports,
		ComponentEventScores,
		ComponentAttackAnalytics,
		ComponentPlayer, ComponentCommanders, ComponentGenerals, ComponentCastellans, ComponentMovements,
		ComponentStationing, ComponentMarket, ComponentAttackDialog, ComponentAttackPresets,
		ComponentAutomations, ComponentObservations,
	)
	clone := cloneGameStateComponents(source, withoutKeyed)
	if components.Has(ComponentWorldMap) {
		clone.prepareMapMutation(source)
	}
	if components.Has(ComponentCastles) {
		clone.prepareCastleMutation(source)
	}
	if components.Has(ComponentInventory) {
		clone.prepareInventoryMutation(source)
	}
	if components.Has(ComponentStorm) {
		clone.prepareStormMutation(source)
	}
	if components.Has(ComponentTowerCooldowns) {
		clone.prepareTowerCooldownMutation(source)
	}
	if components.Has(ComponentTowerQueue) {
		clone.prepareTowerQueueMutation(source)
	}
	if components.Has(ComponentReports) {
		clone.prepareReportMutation(source)
	}
	if components.Has(ComponentEventScores) {
		clone.prepareEventScoreMutation(source)
	}
	if components.Has(ComponentAttackAnalytics) {
		clone.prepareAttackAnalyticsMutation(source)
	}
	if components.Has(ComponentPlayer) {
		clone.preparePlayerMutation(source)
	}
	if components.Has(ComponentCommanders) {
		clone.prepareCommanderMutation(source)
	}
	if components.Has(ComponentGenerals) {
		clone.prepareGeneralMutation(source)
	}
	if components.Has(ComponentCastellans) {
		clone.prepareCastellanMutation(source)
	}
	if components.Has(ComponentMovements) {
		clone.prepareMovementMutation(source)
	}
	if components.Has(ComponentStationing) {
		clone.prepareStationingMutation(source)
	}
	if components.Has(ComponentMarket) {
		clone.prepareMarketMutation(source)
	}
	if components.Has(ComponentAttackDialog) {
		clone.prepareAttackDialogMutation(source)
	}
	if components.Has(ComponentAttackPresets) {
		clone.prepareAttackPresetMutation(source)
	}
	if components.Has(ComponentAutomations) {
		clone.prepareAutomationMutation(source)
	}
	if components.Has(ComponentObservations) {
		clone.prepareObservationMutation(source)
	}
	return &clone
}

func cloneGameStateWithMap(source GameState, cloneWorldMap bool) GameState {
	components := AllComponents
	if !cloneWorldMap {
		components &^= Components(ComponentWorldMap)
	}
	return cloneGameStateComponents(source, components)
}

func cloneGameStateComponents(source GameState, components ComponentSet) GameState {
	clone := source
	clone.pendingMapChanges = nil
	clone.replaceMap = false
	clone.mapMutationCOW = false
	clone.mutableMapRegions = nil
	clone.movementMutationCOW = false
	clone.mutableMovementShards = [4]uint64{}
	clone.pendingMovementChanges = nil
	clone.replaceMovements = false
	clone.castleMutationCOW = false
	clone.mutableCastles = nil
	clone.pendingCastleChanges = nil
	clone.replaceCastles = false
	clone.inventoryMutationCOW = false
	clone.mutableInventoryParts = 0
	clone.pendingEquipmentChanges = nil
	clone.replaceInventoryEquipment = false
	clone.pendingGemChanges = nil
	clone.replaceInventoryGems = false
	clone.pendingInventoryItemChanges = nil
	clone.replaceInventoryItems = false
	clone.stormMutationCOW = false
	clone.mutableStormParts = 0
	clone.mutableStormTargetShards = [4]uint64{}
	clone.pendingStormTargetChanges = nil
	clone.replaceStormTargets = false
	clone.towerCooldownMutationCOW = false
	clone.mutableTowerCooldownShards = [4]uint64{}
	clone.pendingTowerCooldownChanges = nil
	clone.replaceTowerCooldowns = false
	clone.towerQueueMutationCOW = false
	clone.mutableTowerQueueEntries = nil
	clone.pendingTowerQueueCastles = nil
	clone.replaceTowerQueue = false
	clone.reportMutationCOW = false
	clone.mutableReportShards = [4]uint64{}
	clone.pendingReportMessages = nil
	clone.replaceReports = false
	clone.eventScoreMutationCOW = false
	clone.mutableEventScoreShards = [4]uint64{}
	clone.pendingEventScoreIDs = nil
	clone.eventScoreMetadataDirty = false
	clone.eventScoreShopDirty = false
	clone.replaceEventScores = false
	clone.attackAnalyticsMutationCOW = false
	clone.mutableAttackAnalyticsParts = 0
	clone.mutationWrites = 0
	if components.Has(ComponentSession) {
		clone.Session = cloneSessionState(source.Session)
	}
	if components.Has(ComponentPlayer) {
		clone.Player.Resources = cloneMap(source.Player.Resources)
		clone.Player.Currencies = cloneMap(source.Player.Currencies)
		clone.Player.Achievements.Completed = cloneMap(source.Player.Achievements.Completed)
		clone.Player.Achievements.Progress = make(map[int64][]int64, len(source.Player.Achievements.Progress))
		for id, progress := range source.Player.Achievements.Progress {
			clone.Player.Achievements.Progress[id] = append([]int64(nil), progress...)
		}
		clone.Player.LegendSkills.ActiveIDs = append([]int64(nil), source.Player.LegendSkills.ActiveIDs...)
		clone.Player.LegendSkills.SceatSkillIDs = append([]int64(nil), source.Player.LegendSkills.SceatSkillIDs...)
		clone.Player.LegendSkills.SceatActivations = append([]SceatSkillActivation(nil), source.Player.LegendSkills.SceatActivations...)
	}
	if components.Has(ComponentCastles) {
		clone.Castles = make(map[CastleID]CastleState, len(source.Castles))
		for id, castle := range source.Castles {
			clone.Castles[id] = cloneCastleState(castle)
		}
	}
	if components.Has(ComponentCommanders) {
		clone.Commanders = make(map[CommanderID]CommanderState, len(source.Commanders))
		for id, commander := range source.Commanders {
			commander.Equipment = cloneMap(commander.Equipment)
			commander.Gems = cloneMap(commander.Gems)
			clone.Commanders[id] = commander
		}
	}
	if components.Has(ComponentGenerals) {
		clone.Generals = make(map[int64]GeneralState, len(source.Generals))
		for id, general := range source.Generals {
			general.ActiveSkillIDs = append([]int64(nil), general.ActiveSkillIDs...)
			clone.Generals[id] = general
		}
	}
	if components.Has(ComponentCastellans) {
		clone.Castellans = make(map[CastellanID]CastellanState, len(source.Castellans))
		for id, castellan := range source.Castellans {
			castellan.Equipment = cloneMap(castellan.Equipment)
			castellan.Gems = cloneMap(castellan.Gems)
			clone.Castellans[id] = castellan
		}
	}
	if components.Has(ComponentMovements) {
		clone.Movements = source.materializedMovements()
		clone.movementRecords = nil
	}
	if components.Has(ComponentStationing) {
		clone.Stationing = make(map[string]StationingOperation, len(source.Stationing))
		for id, operation := range source.Stationing {
			operation.Units = cloneMap(operation.Units)
			operation.DispatchedAt = cloneTimePointer(operation.DispatchedAt)
			operation.ExpectedReturnAt = cloneTimePointer(operation.ExpectedReturnAt)
			operation.NextAttemptAt = cloneTimePointer(operation.NextAttemptAt)
			operation.SafeAfter = cloneTimePointer(operation.SafeAfter)
			operation.SuccessCooldownUntil = cloneTimePointer(operation.SuccessCooldownUntil)
			clone.Stationing[id] = operation
		}
	}
	if components.Has(ComponentScheduled) {
		clone.Scheduled = make(map[string]ScheduledOperation, len(source.Scheduled))
		for id, operation := range source.Scheduled {
			operation.Arguments = append([]byte(nil), operation.Arguments...)
			clone.Scheduled[id] = operation
		}
	}
	if components.Has(ComponentRift) {
		clone.Rift.Launches = make(map[string]RiftLaunch, len(source.Rift.Launches))
		for id, launch := range source.Rift.Launches {
			launch.Body = append([]byte(nil), launch.Body...)
			clone.Rift.Launches[id] = launch
		}
		clone.Rift.DeletedLaunchIDs = cloneMap(source.Rift.DeletedLaunchIDs)
		if source.Rift.MaidenRun != nil {
			run := *source.Rift.MaidenRun
			run.CommanderIDs = append([]CommanderID(nil), source.Rift.MaidenRun.CommanderIDs...)
			run.LaunchIDs = append([]MovementID(nil), source.Rift.MaidenRun.LaunchIDs...)
			clone.Rift.MaidenRun = &run
		}
	}
	if components.Has(ComponentInventory) {
		clone.Inventory.ConstructionItems = cloneMap(source.Inventory.ConstructionItems)
		clone.Inventory.ConstructionOffers = cloneMap(source.Inventory.ConstructionOffers)
		clone.Inventory.ConstructionOffersByCastle = cloneConstructionOfferSnapshots(source.Inventory.ConstructionOffersByCastle)
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
	}
	if components.Has(ComponentSubscriptions) {
		clone.Subscriptions = cloneMap(source.Subscriptions)
	}
	if components.Has(ComponentMarket) {
		clone.Market.Castles = make(map[CastleID]MarketCastleState, len(source.Market.Castles))
		for id, castle := range source.Market.Castles {
			castle.Resources = cloneMap(castle.Resources)
			castle.AreaEffects = append([]MarketAreaEffect(nil), castle.AreaEffects...)
			for index := range castle.AreaEffects {
				castle.AreaEffects[index].Values = append([]float64(nil), castle.AreaEffects[index].Values...)
			}
			clone.Market.Castles[id] = castle
		}
		clone.Market.Boosters = cloneMap(source.Market.Boosters)
	}
	if components.Has(ComponentKingdomTransport) {
		clone.KingdomTransport.Unlocks = cloneMap(source.KingdomTransport.Unlocks)
		clone.KingdomTransport.Pending = append([]KingdomResourceTransport(nil), source.KingdomTransport.Pending...)
		for index := range clone.KingdomTransport.Pending {
			clone.KingdomTransport.Pending[index].Goods = append([]KingdomTransportGood(nil), source.KingdomTransport.Pending[index].Goods...)
		}
		clone.KingdomTransport.PendingUnits = append([]KingdomUnitTransport(nil), source.KingdomTransport.PendingUnits...)
		for index := range clone.KingdomTransport.PendingUnits {
			clone.KingdomTransport.PendingUnits[index].Units = append([]KingdomTransportUnit(nil), source.KingdomTransport.PendingUnits[index].Units...)
		}
		clone.KingdomTransport.ResourceWorkflows = cloneMap(source.KingdomTransport.ResourceWorkflows)
		for kingdomID, workflow := range clone.KingdomTransport.ResourceWorkflows {
			workflow.Goods = append([]KingdomTransportGood(nil), workflow.Goods...)
			clone.KingdomTransport.ResourceWorkflows[kingdomID] = workflow
		}
	}
	if components.Has(ComponentBeri) {
		clone.Beri.TroopsByUnit = cloneMap(source.Beri.TroopsByUnit)
	}
	if components.Has(ComponentAlliance) {
		clone.Alliance.Members = append([]AllianceMember{}, source.Alliance.Members...)
		clone.Alliance.Holdings = append([]AllianceHolding{}, source.Alliance.Holdings...)
	}
	if components.Has(ComponentAllianceHelp) {
		clone.AllianceHelpRequests.HospitalProductionIDs = append(
			[]int64(nil), source.AllianceHelpRequests.HospitalProductionIDs...,
		)
		clone.AllianceHelpRequests.RecruitmentCastleIDs = append(
			[]CastleID(nil), source.AllianceHelpRequests.RecruitmentCastleIDs...,
		)
		clone.AllianceHelpRequests.OwnRecruitmentRequests = append(
			[]RecruitmentAllianceHelpRequest(nil), source.AllianceHelpRequests.OwnRecruitmentRequests...,
		)
		clone.AllianceHelpRequests.PendingOtherListIDs = append(
			[]int64{}, source.AllianceHelpRequests.PendingOtherListIDs...,
		)
	}
	if components.Has(ComponentAlliances) {
		clone.Alliances = make(map[AllianceID]AllianceState, len(source.Alliances))
		for id, alliance := range source.Alliances {
			alliance.Members = append([]AllianceMember{}, alliance.Members...)
			alliance.Holdings = append([]AllianceHolding{}, alliance.Holdings...)
			clone.Alliances[id] = alliance
		}
	}
	if components.Has(ComponentWorldMap) {
		clone.Map = source.materializedMap()
		clone.mapOverlay = nil
		clone.sharedMap = nil
		clone.worldSharing = false
	}
	if components.Has(ComponentTowerCooldowns) {
		clone.TowerCooldowns = source.materializedTowerCooldowns()
		clone.towerCooldowns = nil
	}
	if components.Has(ComponentTowerQueue) {
		clone.TowerQueue.EntriesByCastle = make(map[CastleID][]TowerQueueEntry, len(source.TowerQueue.EntriesByCastle))
		for castleID, entries := range source.TowerQueue.EntriesByCastle {
			clonedEntries := append([]TowerQueueEntry(nil), entries...)
			for index := range clonedEntries {
				clonedEntries[index].DeferredUntil = cloneTimePointer(clonedEntries[index].DeferredUntil)
			}
			clone.TowerQueue.EntriesByCastle[castleID] = clonedEntries
		}
		clone.TowerQueue.LastScannedAt = cloneMap(source.TowerQueue.LastScannedAt)
		clone.TowerQueue.LastAttemptedAt = cloneMap(source.TowerQueue.LastAttemptedAt)
		clone.TowerQueue.ConfirmedLaunchesByCastle = cloneMap(source.TowerQueue.ConfirmedLaunchesByCastle)
		clone.TowerQueue.CapacityByCastle = cloneMap(source.TowerQueue.CapacityByCastle)
	}
	if components.Has(ComponentInvasion) {
		clone.Invasion.LastScannedAt = cloneMap(source.Invasion.LastScannedAt)
		clone.Invasion.FortifiedTargets = cloneMap(source.Invasion.FortifiedTargets)
		clone.Invasion.FortifyCurrencies = append([]string(nil), source.Invasion.FortifyCurrencies...)
	}
	if components.Has(ComponentStorm) {
		clone.Storm.LastScannedAt = cloneMap(source.Storm.LastScannedAt)
		clone.Storm.Map.Targets = source.materializedStormTargets()
		clone.Storm.IslandReturns = make(map[string]StormIslandReturnState, len(source.Storm.IslandReturns))
		for key, operation := range source.Storm.IslandReturns {
			operation.Survivors = cloneMap(operation.Survivors)
			clone.Storm.IslandReturns[key] = operation
		}
		clone.stormTargets = nil
	}
	if components.Has(ComponentNomadCamps) {
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
	}
	if components.Has(ComponentAdvisor) {
		if source.Advisor.Run != nil {
			run := *source.Advisor.Run
			clone.Advisor.Run = &run
		}
		clone.Advisor.Summary.Gains = cloneMap(source.Advisor.Summary.Gains)
		clone.Advisor.Summary.Costs = cloneMap(source.Advisor.Summary.Costs)
	}
	if components.Has(ComponentKhan) {
		clone.Khan.Launches = append([]KhanLaunchState(nil), source.Khan.Launches...)
		clone.Khan.Taunts = cloneMap(source.Khan.Taunts)
		clone.Khan.ResolvedTaunts = append([]KhanTauntState(nil), source.Khan.ResolvedTaunts...)
		clone.Khan.CooldownReports = cloneMap(source.Khan.CooldownReports)
		for reportID, report := range clone.Khan.CooldownReports {
			report.MSDs = append([]KhanCooldownMSDState(nil), report.MSDs...)
			clone.Khan.CooldownReports[reportID] = report
		}
	}
	if components.Has(ComponentAttackDialog) {
		clone.AttackDialog.ActiveEffects = append([]AttackDialogEffect(nil), source.AttackDialog.ActiveEffects...)
		for index := range clone.AttackDialog.ActiveEffects {
			clone.AttackDialog.ActiveEffects[index].Values = append([]float64(nil), source.AttackDialog.ActiveEffects[index].Values...)
		}
	}
	if components.Has(ComponentAttackPresets) {
		clone.AttackPresets = append([]AttackPreset{}, source.AttackPresets...)
		for presetIndex := range clone.AttackPresets {
			for lane := range clone.AttackPresets[presetIndex].Units {
				clone.AttackPresets[presetIndex].Units[lane] = append([]AttackPresetStack(nil), source.AttackPresets[presetIndex].Units[lane]...)
				clone.AttackPresets[presetIndex].Tools[lane] = append([]AttackPresetStack(nil), source.AttackPresets[presetIndex].Tools[lane]...)
			}
		}
	}
	if components.Has(ComponentAttackAnalytics) {
		clone.AttackAnalytics.LaunchIDs = append([]MovementID(nil), source.AttackAnalytics.LaunchIDs...)
		clone.AttackAnalytics.PendingAttacks = append([]AttackFeatureLaunch(nil), source.AttackAnalytics.PendingAttacks...)
		clone.AttackAnalytics.RecentAutoStormLaunches = append(
			[]AttackFeatureLaunch(nil), source.AttackAnalytics.RecentAutoStormLaunches...,
		)
	}
	if components.Has(ComponentEventScores) {
		clone.EventScores = source.materializedEventScores()
		clone.eventScoreRecords = nil
	}
	if components.Has(ComponentCommandContext) {
		clone.CommandContext.ProductionObservedAt = cloneTimePointer(source.CommandContext.ProductionObservedAt)
	}
	if components.Has(ComponentAutomations) {
		clone.Automations = make(map[string]AutomationState, len(source.Automations))
		for id, automation := range source.Automations {
			automation.NextCheckAt = cloneTimePointer(automation.NextCheckAt)
			automation.LastRunAt = cloneTimePointer(automation.LastRunAt)
			automation.Metrics = cloneMap(automation.Metrics)
			automation.OperationalCursors = cloneMap(automation.OperationalCursors)
			clone.Automations[id] = automation
		}
	}
	if components.Has(ComponentReports) {
		clone.Reports = source.materializedReports()
		clone.reportRecords = nil
	}
	if components.Has(ComponentObservations) {
		clone.Observations = make(map[string]ProtocolObservation, len(source.Observations))
		for opcode, observation := range source.Observations {
			observation.LastCode = cloneIntPointer(observation.LastCode)
			clone.Observations[opcode] = observation
		}
	}
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
	clone := make(EquipmentEffects, len(source))
	copy(clone, source)
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
