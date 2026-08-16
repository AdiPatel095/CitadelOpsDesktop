package State

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

const (
	sharedStormScanRefreshInterval = 2 * time.Hour
	sharedStormScannerHeartbeatTTL = 2 * time.Minute
	sharedStormRosterSettleDelay   = 2 * time.Second
	sharedStormLeaseTTL            = 2 * time.Minute
	sharedStormHeartbeatInterval   = time.Minute
	sharedStormWindowSize          = 101
	sharedStormInitialMin          = 400
	sharedStormInitialMax          = 900
	sharedStormEdgeBuffer          = 25
	sharedStormExpansionStride     = sharedStormWindowSize - 1
	sharedStormMaximumCoordinate   = 40_399
)

// StormScanWindowState is anonymous process-shared coverage metadata. It
// contains no account, player, castle, configuration, or operation identity.
type StormScanWindowState struct {
	Bounds      StormMapBounds `json:"bounds"`
	CompletedAt time.Time      `json:"completedAt,omitempty"`
}

type StormScanCoverage struct {
	Available        bool           `json:"available"`
	Bounds           StormMapBounds `json:"bounds"`
	WindowCount      int            `json:"windowCount"`
	FreshWindowCount int            `json:"freshWindowCount"`
	ParticipantCount int            `json:"participantCount"`
	LastCompletedAt  time.Time      `json:"lastCompletedAt,omitempty"`
	NextRefreshAt    time.Time      `json:"nextRefreshAt,omitempty"`
}

// stormScanPlan is immutable geometry cached with a shared-world generation.
// Acquiring a lease or reading coverage is therefore O(window count) and never
// rescans world facts. The plan expands incrementally on Storm upserts and is
// rebuilt from the Storm partition only after a complete coverage cycle.
type stormScanPlan struct {
	Bounds  StormMapBounds
	Windows []StormMapBounds
	Keys    []string
}

func (coverage StormScanCoverage) Complete() bool {
	return coverage.Available && coverage.WindowCount > 0 && coverage.FreshWindowCount == coverage.WindowCount
}

// StormScanAssignment is returned only to the account that owns the lease.
// Slot and participant count are anonymous operational metrics; no peer
// account identifiers leave the coordinator.
type StormScanAssignment struct {
	LeaseID          string
	Windows          []StormMapBounds
	Slot             int
	ParticipantCount int
	NextCheckAt      time.Time
	Coverage         StormScanCoverage
}

type stormScanRoster struct {
	signature string
	changedAt time.Time
}

type stormScanLease struct {
	id         string
	account    string
	worldID    string
	kingdomID  KingdomID
	windows    []StormMapBounds
	windowKeys []string
	expiresAt  time.Time
}

type persistedStormScanWindow struct {
	WorldID     string
	KingdomID   KingdomID
	Key         string
	Bounds      StormMapBounds
	CompletedAt time.Time
}

// AcquireStormScan registers a capable account heartbeat and leases only its
// deterministic portion of stale windows. Membership is memory-only and
// anonymous outside this method. When a scanner disappears, its heartbeat and
// leases expire and the remaining scanners automatically repartition.
func (store *WorldMapStore) AcquireStormScan(
	accountKey string,
	worldID string,
	kingdomID KingdomID,
	now time.Time,
) StormScanAssignment {
	accountKey = strings.TrimSpace(accountKey)
	worldID = CanonicalWorldID(worldID)
	if store == nil || accountKey == "" || worldID == "" || kingdomID < 0 || now.IsZero() {
		return StormScanAssignment{NextCheckAt: now.Add(sharedStormHeartbeatInterval)}
	}
	store.mu.Lock()
	scope := stormScanScopeKey(worldID, kingdomID)
	participants := store.stormParticipants[scope]
	if participants == nil {
		participants = map[string]time.Time{}
		store.stormParticipants[scope] = participants
	}
	for participant, heartbeat := range participants {
		if now.Sub(heartbeat) > sharedStormScannerHeartbeatTTL {
			delete(participants, participant)
		}
	}
	participants[accountKey] = now

	ids := make([]string, 0, len(participants))
	for participant := range participants {
		ids = append(ids, participant)
	}
	sort.Strings(ids)
	signature := strings.Join(ids, "\x00")
	roster := store.stormRosters[scope]
	if roster.signature != signature {
		roster = stormScanRoster{signature: signature, changedAt: now}
		store.stormRosters[scope] = roster
	}
	for leaseID, lease := range store.stormLeases {
		if !lease.expiresAt.After(now) {
			delete(store.stormLeases, leaseID)
		}
	}

	generation := store.worlds[worldID]
	plan := sharedStormScanPlan(generation, kingdomID)
	candidates := plan.Windows
	coverage := stormScanCoverageWithPlan(generation, kingdomID, plan, len(ids), now)
	slot := sort.SearchStrings(ids, accountKey)
	assignment := StormScanAssignment{
		Slot: slot, ParticipantCount: len(ids), Coverage: coverage,
		NextCheckAt: now.Add(sharedStormHeartbeatInterval),
	}
	if now.Before(roster.changedAt.Add(sharedStormRosterSettleDelay)) {
		assignment.NextCheckAt = roster.changedAt.Add(sharedStormRosterSettleDelay)
		store.mu.Unlock()
		return assignment
	}

	leasedWindows := map[string]struct{}{}
	for _, lease := range store.stormLeases {
		if lease.worldID != worldID || lease.kingdomID != kingdomID || !lease.expiresAt.After(now) {
			continue
		}
		for index, bounds := range lease.windows {
			key := stormScanWindowKey(bounds)
			if index < len(lease.windowKeys) {
				key = lease.windowKeys[index]
			}
			leasedWindows[key] = struct{}{}
		}
	}
	windows := make([]StormMapBounds, 0, (len(candidates)+len(ids)-1)/max(1, len(ids)))
	windowKeys := make([]string, 0, cap(windows))
	for index, bounds := range candidates {
		if len(ids) == 0 || index%len(ids) != slot {
			continue
		}
		key := stormScanPlanWindowKey(plan, index, bounds)
		if _, leased := leasedWindows[key]; leased {
			continue
		}
		completedAt := stormScanWindowCompletedAt(generation, kingdomID, key)
		if !completedAt.IsZero() && now.Before(completedAt.Add(sharedStormScanRefreshInterval)) {
			if due := completedAt.Add(sharedStormScanRefreshInterval); due.Before(assignment.NextCheckAt) {
				assignment.NextCheckAt = due
			}
			continue
		}
		windows = append(windows, bounds)
		windowKeys = append(windowKeys, key)
	}
	if len(windows) == 0 {
		store.mu.Unlock()
		return assignment
	}
	store.nextStormLease++
	leaseID := fmt.Sprintf("storm-%016x", store.nextStormLease)
	store.stormLeases[leaseID] = stormScanLease{
		id: leaseID, account: accountKey, worldID: worldID, kingdomID: kingdomID,
		windows: append([]StormMapBounds(nil), windows...), windowKeys: append([]string(nil), windowKeys...),
		expiresAt: now.Add(sharedStormLeaseTTL),
	}
	assignment.LeaseID = leaseID
	assignment.Windows = windows
	assignment.NextCheckAt = now.Add(sharedStormLeaseTTL)
	store.mu.Unlock()
	return assignment
}

// CompleteStormScan atomically marks leased windows fresh and removes only
// Storm rows that an authoritative response omitted from those exact windows.
// The scan contributor is deliberately absent from the shared generation and
// durable database.
func (store *WorldMapStore) CompleteStormScan(
	accountKey string,
	worldID string,
	kingdomID KingdomID,
	leaseID string,
	windows []StormMapBounds,
	startedAt time.Time,
	completedAt time.Time,
) (WorldMapEvent, error) {
	accountKey = strings.TrimSpace(accountKey)
	worldID = CanonicalWorldID(worldID)
	leaseID = strings.TrimSpace(leaseID)
	windows = normalizeStormScanWindows(windows)
	if store == nil || accountKey == "" || worldID == "" || leaseID == "" || len(windows) == 0 ||
		startedAt.IsZero() || completedAt.Before(startedAt) {
		return WorldMapEvent{}, fmt.Errorf("invalid shared Storm scan completion")
	}

	store.mu.Lock()
	lease, found := store.stormLeases[leaseID]
	if !found || lease.account != accountKey || lease.worldID != worldID || lease.kingdomID != kingdomID ||
		!sameStormScanWindows(lease.windows, windows) {
		store.mu.Unlock()
		return WorldMapEvent{}, fmt.Errorf("shared Storm scan lease is unavailable or does not match its account")
	}
	delete(store.stormLeases, leaseID)
	current := store.worlds[worldID]
	if current == nil {
		current = &worldMapGeneration{
			values: worldFactMap{}, stormWindows: map[KingdomID]map[string]StormScanWindowState{},
			stormPlans: initialStormScanPlans(),
		}
	}

	changes := make([]MapChange, 0)
	rangeWorldStormFacts(current.values, kingdomID, func(key string, fact WorldMapFact) bool {
		if !fact.ObservedAt.Before(startedAt) || !stormBoundsContainAny(windows, fact.X, fact.Y) {
			return true
		}
		changes = append(changes, MapChange{
			KingdomID: kingdomID, Key: key, TypeID: fact.TypeID, Deleted: true,
			expectedObservedAt: fact.ObservedAt,
		})
		return true
	})
	nextValues := applySharedWorldDeletes(current.values, changes)
	nextScanWindows := cloneMap(current.stormWindows)
	kingdomWindows := cloneMap(nextScanWindows[kingdomID])
	if kingdomWindows == nil {
		kingdomWindows = map[string]StormScanWindowState{}
	}
	for _, bounds := range windows {
		kingdomWindows[stormScanWindowKey(bounds)] = StormScanWindowState{Bounds: bounds, CompletedAt: completedAt.UTC()}
	}
	nextScanWindows[kingdomID] = kingdomWindows
	nextPlans := current.stormPlans
	currentPlan := sharedStormScanPlan(current, kingdomID)
	if stormScanCoverageForPlan(currentPlan, kingdomID, kingdomWindows, 0, completedAt).Complete() {
		rebuilt := buildStormScanPlan(nextValues, kingdomID)
		// A rebuilt plan can shift its tile grid while shrinking. The just-finished
		// outer coverage contains every rebuilt window, so carry the cycle's
		// completion forward instead of scheduling an immediate duplicate scan.
		for _, bounds := range rebuilt.Windows {
			kingdomWindows[stormScanWindowKey(bounds)] = StormScanWindowState{
				Bounds: bounds, CompletedAt: completedAt.UTC(),
			}
		}
		nextPlans = cloneMap(current.stormPlans)
		if nextPlans == nil {
			nextPlans = map[KingdomID]stormScanPlan{}
		}
		nextPlans[kingdomID] = rebuilt
	}
	next := &worldMapGeneration{
		version: current.version + 1, updatedAt: completedAt.UTC(), values: nextValues,
		stormWindows: nextScanWindows, stormPlans: nextPlans,
	}
	store.worlds[worldID] = next
	event := WorldMapEvent{
		WorldID: worldID, Version: next.version, UpdatedAt: next.updatedAt,
		Changes: normalizeMapChanges(changes), Domains: []string{"storm-scan"}, KingdomIDs: []KingdomID{kingdomID}, generation: next,
	}
	subscribers := make([]chan WorldMapEvent, 0, len(store.subscribers))
	for _, channel := range store.subscribers {
		subscribers = append(subscribers, channel)
	}
	store.mu.Unlock()

	store.queuePersistence(event)
	store.queueStormScanPersistence(worldID, kingdomID, windows, completedAt.UTC())
	for _, channel := range subscribers {
		channel <- event
	}
	return event, nil
}

func (store *WorldMapStore) ReleaseStormScan(accountKey string, leaseID string) {
	if store == nil {
		return
	}
	store.mu.Lock()
	if lease, found := store.stormLeases[strings.TrimSpace(leaseID)]; found && lease.account == strings.TrimSpace(accountKey) {
		delete(store.stormLeases, lease.id)
	}
	store.mu.Unlock()
}

func (store *WorldMapStore) UnregisterStormScanner(accountKey string) {
	if store == nil || strings.TrimSpace(accountKey) == "" {
		return
	}
	accountKey = strings.TrimSpace(accountKey)
	store.mu.Lock()
	for scope, participants := range store.stormParticipants {
		delete(participants, accountKey)
		if len(participants) == 0 {
			delete(store.stormParticipants, scope)
		}
	}
	for leaseID, lease := range store.stormLeases {
		if lease.account == accountKey {
			delete(store.stormLeases, leaseID)
		}
	}
	store.mu.Unlock()
}

func (state GameState) SharedStormScanCoverage(kingdomID KingdomID, now time.Time) StormScanCoverage {
	if state.sharedMap == nil || !accountCanAccessSharedKingdom(state, kingdomID) {
		return StormScanCoverage{}
	}
	plan := sharedStormScanPlan(state.sharedMap, kingdomID)
	return stormScanCoverageWithPlan(state.sharedMap, kingdomID, plan, 0, now)
}

// sharedStormCandidateWindows reads the immutable plan cached on the world
// generation. The fallback exists for legacy/test generations and traverses
// only the Storm partition, never the complete world map.
func sharedStormCandidateWindows(generation *worldMapGeneration, kingdomID KingdomID) []StormMapBounds {
	return sharedStormScanPlan(generation, kingdomID).Windows
}

func sharedStormScanPlan(generation *worldMapGeneration, kingdomID KingdomID) stormScanPlan {
	if generation != nil && generation.stormPlans != nil {
		if plan, found := generation.stormPlans[kingdomID]; found && len(plan.Windows) > 0 {
			return plan
		}
	}
	if generation == nil {
		return stormScanPlanForBounds(initialStormScanBounds())
	}
	return buildStormScanPlan(generation.values, kingdomID)
}

func initialStormScanBounds() StormMapBounds {
	return StormMapBounds{
		X1: sharedStormInitialMin, Y1: sharedStormInitialMin,
		X2: sharedStormInitialMax, Y2: sharedStormInitialMax,
	}
}

func initialStormScanPlans() map[KingdomID]stormScanPlan {
	return map[KingdomID]stormScanPlan{stormKingdomID: stormScanPlanForBounds(initialStormScanBounds())}
}

func buildWorldStormScanPlans(values worldFactMap) map[KingdomID]stormScanPlan {
	plans := initialStormScanPlans()
	for kingdomID := range values {
		plan := buildStormScanPlan(values, kingdomID)
		if plan.Bounds != initialStormScanBounds() || kingdomID == stormKingdomID {
			plans[kingdomID] = plan
		}
	}
	return plans
}

func buildStormScanPlan(values worldFactMap, kingdomID KingdomID) stormScanPlan {
	bounds := initialStormScanBounds()
	rangeWorldStormFacts(values, kingdomID, func(_ string, fact WorldMapFact) bool {
		bounds = expandStormScanBounds(bounds, fact.X, fact.Y)
		return true
	})
	return stormScanPlanForBounds(bounds)
}

func expandStormScanBounds(bounds StormMapBounds, x int, y int) StormMapBounds {
	if !bounds.IsValid() {
		bounds = initialStormScanBounds()
	}
	for expansion := 0; expansion < 32; expansion++ {
		next := bounds
		if x <= bounds.X1+sharedStormEdgeBuffer {
			next.X1 = max(0, bounds.X1-sharedStormExpansionStride)
		}
		if y <= bounds.Y1+sharedStormEdgeBuffer {
			next.Y1 = max(0, bounds.Y1-sharedStormExpansionStride)
		}
		if x >= bounds.X2-sharedStormEdgeBuffer {
			next.X2 = min(sharedStormMaximumCoordinate, bounds.X2+sharedStormExpansionStride)
		}
		if y >= bounds.Y2-sharedStormEdgeBuffer {
			next.Y2 = min(sharedStormMaximumCoordinate, bounds.Y2+sharedStormExpansionStride)
		}
		if next == bounds {
			break
		}
		bounds = next
	}
	return bounds
}

func stormScanPlanForBounds(bounds StormMapBounds) stormScanPlan {
	if !bounds.IsValid() {
		bounds = initialStormScanBounds()
	}
	windows := make([]StormMapBounds, 0)
	for y := bounds.Y1; y <= bounds.Y2; y += sharedStormWindowSize {
		for x := bounds.X1; x <= bounds.X2; x += sharedStormWindowSize {
			windows = append(windows, StormMapBounds{
				X1: x, Y1: y,
				X2: min(bounds.X2, x+sharedStormWindowSize-1),
				Y2: min(bounds.Y2, y+sharedStormWindowSize-1),
			})
		}
	}
	windows = normalizeStormScanWindows(windows)
	keys := make([]string, len(windows))
	for index, window := range windows {
		keys[index] = stormScanWindowKey(window)
	}
	return stormScanPlan{Bounds: bounds, Windows: windows, Keys: keys}
}

func stormScanPlanWindowKey(plan stormScanPlan, index int, bounds StormMapBounds) string {
	if index >= 0 && index < len(plan.Keys) && plan.Keys[index] != "" {
		return plan.Keys[index]
	}
	return stormScanWindowKey(bounds)
}

func updateStormScanPlans(
	current map[KingdomID]stormScanPlan,
	values worldFactMap,
	changes []MapChange,
) map[KingdomID]stormScanPlan {
	plans := current
	if plans == nil {
		plans = buildWorldStormScanPlans(values)
	}
	cloned := false
	for _, change := range changes {
		if change.Deleted || change.Observation == nil || !isStormMapType(change.Observation.TypeID) {
			continue
		}
		plan, found := plans[change.KingdomID]
		if !found || len(plan.Windows) == 0 {
			plan = stormScanPlanForBounds(initialStormScanBounds())
		}
		nextBounds := expandStormScanBounds(plan.Bounds, change.Observation.X, change.Observation.Y)
		if nextBounds == plan.Bounds {
			continue
		}
		if !cloned {
			plans = cloneMap(plans)
			cloned = true
		}
		plans[change.KingdomID] = stormScanPlanForBounds(nextBounds)
	}
	return plans
}

func stormScanCoverageForPlan(
	plan stormScanPlan,
	kingdomID KingdomID,
	windows map[string]StormScanWindowState,
	participants int,
	now time.Time,
) StormScanCoverage {
	generation := &worldMapGeneration{stormWindows: map[KingdomID]map[string]StormScanWindowState{kingdomID: windows}}
	return stormScanCoverageWithPlan(generation, kingdomID, plan, participants, now)
}

func stormScanCoverage(
	generation *worldMapGeneration,
	kingdomID KingdomID,
	candidates []StormMapBounds,
	participants int,
	now time.Time,
) StormScanCoverage {
	keys := make([]string, len(candidates))
	for index, bounds := range candidates {
		keys[index] = stormScanWindowKey(bounds)
	}
	return stormScanCoverageWithPlan(generation, kingdomID, stormScanPlan{Windows: candidates, Keys: keys}, participants, now)
}

func stormScanCoverageWithPlan(
	generation *worldMapGeneration,
	kingdomID KingdomID,
	plan stormScanPlan,
	participants int,
	now time.Time,
) StormScanCoverage {
	candidates := plan.Windows
	coverage := StormScanCoverage{Available: generation != nil, ParticipantCount: participants, WindowCount: len(candidates)}
	for index, bounds := range candidates {
		if index == 0 {
			coverage.Bounds = bounds
		} else {
			coverage.Bounds.X1 = min(coverage.Bounds.X1, bounds.X1)
			coverage.Bounds.Y1 = min(coverage.Bounds.Y1, bounds.Y1)
			coverage.Bounds.X2 = max(coverage.Bounds.X2, bounds.X2)
			coverage.Bounds.Y2 = max(coverage.Bounds.Y2, bounds.Y2)
		}
		key := stormScanPlanWindowKey(plan, index, bounds)
		completedAt := stormScanWindowCompletedAt(generation, kingdomID, key)
		if completedAt.IsZero() {
			continue
		}
		if coverage.LastCompletedAt.IsZero() || completedAt.After(coverage.LastCompletedAt) {
			coverage.LastCompletedAt = completedAt
		}
		due := completedAt.Add(sharedStormScanRefreshInterval)
		if coverage.NextRefreshAt.IsZero() || due.Before(coverage.NextRefreshAt) {
			coverage.NextRefreshAt = due
		}
		if now.IsZero() || now.Before(due) {
			coverage.FreshWindowCount++
		}
	}
	return coverage
}

func stormScanWindowCompletedAt(generation *worldMapGeneration, kingdomID KingdomID, key string) time.Time {
	if generation == nil || generation.stormWindows == nil {
		return time.Time{}
	}
	return generation.stormWindows[kingdomID][key].CompletedAt
}

func normalizeStormScanWindows(values []StormMapBounds) []StormMapBounds {
	unique := map[string]StormMapBounds{}
	for _, bounds := range values {
		if bounds.IsValid() {
			unique[stormScanWindowKey(bounds)] = bounds
		}
	}
	keys := make([]string, 0, len(unique))
	for key := range unique {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	result := make([]StormMapBounds, 0, len(keys))
	for _, key := range keys {
		result = append(result, unique[key])
	}
	return result
}

func stormScanWindowKey(bounds StormMapBounds) string {
	return fmt.Sprintf("%010d:%010d:%010d:%010d", bounds.Y1, bounds.X1, bounds.Y2, bounds.X2)
}

func stormScanScopeKey(worldID string, kingdomID KingdomID) string {
	return CanonicalWorldID(worldID) + "\x00" + fmt.Sprintf("%d", kingdomID)
}

func sameStormScanWindows(left []StormMapBounds, right []StormMapBounds) bool {
	left = normalizeStormScanWindows(left)
	right = normalizeStormScanWindows(right)
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func stormBoundsContainAny(bounds []StormMapBounds, x int, y int) bool {
	for _, candidate := range bounds {
		if candidate.Contains(x, y) {
			return true
		}
	}
	return false
}

func applySharedWorldDeletes(values worldFactMap, changes []MapChange) worldFactMap {
	if len(changes) == 0 {
		return values
	}
	next := cloneMap(values)
	regions := map[KingdomID]*mutableWorldFactRegion{}
	for _, change := range changes {
		if !change.Deleted {
			continue
		}
		mutable := regions[change.KingdomID]
		if mutable == nil {
			var region worldFactRegion
			if current := values[change.KingdomID]; current != nil {
				region = *current
			}
			mutable = &mutableWorldFactRegion{region: &region}
			regions[change.KingdomID] = mutable
			next[change.KingdomID] = mutable.region
		}
		delete(mutable.mutableWorldFactShard(MapProjectionStorm, change.Key), change.Key)
	}
	return next
}

func (store *WorldMapStore) queueStormScanPersistence(
	worldID string,
	kingdomID KingdomID,
	windows []StormMapBounds,
	completedAt time.Time,
) {
	if store == nil || store.db == nil {
		return
	}
	store.persistMu.Lock()
	for _, bounds := range windows {
		key := stormScanWindowKey(bounds)
		persistenceKey := persistedWorldMapKey(worldID, kingdomID, "scan:"+key)
		store.dirtyStormScans[persistenceKey] = persistedStormScanWindow{
			WorldID: worldID, KingdomID: kingdomID, Key: key, Bounds: bounds, CompletedAt: completedAt,
		}
	}
	store.persistMu.Unlock()
	select {
	case store.persistWake <- struct{}{}:
	default:
	}
}
