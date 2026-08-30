package State

import (
	"encoding/json"
	"sync"
	"time"
)

type clientEventEncoding struct {
	once sync.Once
	raw  json.RawMessage
	err  error
}

// ClientStateSnapshot preserves the official GameState contract while
// projecting large backend-only collections for the dashboard consumer.
// It retains an immutable generation by value and does not clone account state.
type ClientStateSnapshot struct {
	state GameState
}

type ClientStateEvent struct {
	Sequence   uint64                `json:"sequence"`
	Gap        bool                  `json:"gap,omitempty"`
	Revision   uint64                `json:"revision"`
	Domains    []string              `json:"domains"`
	Components []Component           `json:"components,omitempty"`
	Partitions []PartitionVersion    `json:"partitions,omitempty"`
	OccurredAt time.Time             `json:"occurredAt"`
	Patch      *ClientComponentPatch `json:"patch,omitempty"`
}

// ClientComponentPatch embeds the ordinary official-domain patch and
// overrides only edge projections that are derived for the dashboard.
type ClientComponentPatch struct {
	*ComponentPatch
	Storm   *ClientStormState `json:"storm,omitempty"`
	Reports *ReportState      `json:"reports,omitempty"`
}

type ClientStormState struct {
	Map ClientStormMapState `json:"map"`
}

type ClientStormMapState struct {
	StormMapState
	Targets           map[string]MapObservation `json:"targets"`
	TargetCount       int                       `json:"targetCount"`
	ReadyTargetCount  int                       `json:"readyTargetCount"`
	NextTargetReadyAt *time.Time                `json:"nextTargetReadyAt,omitempty"`
}

func NewClientStateSnapshot(state GameState) ClientStateSnapshot {
	return ClientStateSnapshot{state: state}
}

func (snapshot ClientStateSnapshot) MarshalJSON() ([]byte, error) {
	type wireState GameState
	projected := snapshot.state.clientStateProjection()
	storm := newClientStormState(snapshot.state, time.Now().UTC())
	return json.Marshal(struct {
		wireState
		Map   WorldMap         `json:"map"`
		Storm ClientStormState `json:"storm"`
	}{wireState: wireState(projected), Map: snapshot.state.clientMapProjection(), Storm: storm})
}

// ClientEvent filters component deltas with the same consumer policy used by
// the initial snapshot. The retained generation and durable event remain
// untouched for backend readers and persistence.
func ClientEvent(source Event) ClientStateEvent {
	result := ClientStateEvent{
		Sequence: source.Sequence, Gap: source.Gap, Revision: source.Revision,
		Domains: source.Domains, Partitions: source.Partitions, OccurredAt: source.OccurredAt,
	}
	if source.Patch == nil {
		return result
	}
	var storm *ClientStormState
	if clientEventRefreshesStorm(source) && source.generation != nil {
		value := newClientStormState(*source.generation.state, time.Now().UTC())
		storm = &value
	}
	var reports *ReportState
	if source.Patch.Reports != nil && source.generation != nil {
		value := clientReports(*source.generation.state)
		reports = &value
	}
	patch := *source.Patch
	projectClientComponentPatch(&patch)
	if patch.Map != nil {
		projected := clientMapProjection(*patch.Map)
		patch.Map = &projected
	}
	if patch.MapChanges != nil {
		projected := clientMapChanges(*patch.MapChanges)
		patch.MapChanges = &projected
	}
	components := make([]Component, 0, len(source.Components))
	for _, component := range source.Components {
		if clientComponentVisible(component) {
			components = append(components, component)
		}
	}
	result.Components = components
	result.Patch = &ClientComponentPatch{ComponentPatch: &patch, Storm: storm, Reports: reports}
	return result
}

func clientEventRefreshesStorm(source Event) bool {
	// Cooperative coverage commits each returned tile into the shared map so
	// backend readers see it immediately. Rebuilding the complete dashboard
	// Storm projection for every tile would turn that targeted update back into
	// an O(all Storm targets) scan. The completion event refreshes it once.
	if eventHasDomain(source.Domains, "storm-scan-progress") {
		return false
	}
	return eventHasDomain(source.Domains, "storm-scan") ||
		source.Patch != nil && (source.Patch.Storm != nil || patchContainsStormMapChange(source.Patch))
}

func eventHasDomain(domains []string, wanted string) bool {
	for _, domain := range domains {
		if domain == wanted {
			return true
		}
	}
	return false
}

// ClientEventPayload projects and encodes a state event at most once, even
// when several dashboard websocket subscribers receive the same immutable
// event. The returned bytes are immutable and must not be modified.
func ClientEventPayload(source Event) (json.RawMessage, error) {
	cache := source.clientEncoding
	if cache == nil {
		raw, err := json.Marshal(ClientEvent(source))
		return json.RawMessage(raw), err
	}
	cache.once.Do(func() {
		raw, err := json.Marshal(ClientEvent(source))
		cache.raw = json.RawMessage(raw)
		cache.err = err
	})
	return cache.raw, cache.err
}

func patchContainsStormMapChange(patch *ComponentPatch) bool {
	if patch == nil {
		return false
	}
	if patch.Map != nil {
		for _, observations := range *patch.Map {
			for _, observation := range observations {
				if observation.TypeID == MapTypeStormIsland || observation.TypeID == MapTypeStormFort {
					return true
				}
			}
		}
	}
	if patch.MapChanges != nil {
		for _, change := range *patch.MapChanges {
			typeID := change.TypeID
			if change.Observation != nil {
				typeID = change.Observation.TypeID
			}
			if typeID == MapTypeStormIsland || typeID == MapTypeStormFort {
				return true
			}
		}
	}
	return false
}

func (state GameState) clientStateProjection() GameState {
	projected := state
	projected.CatalogVersion = ""
	projected.LanguageVersion = ""
	projected.Castles = clientCastles(state.Castles)
	projected.Movements = state.materializedMovements()
	projected.Generals = map[int64]GeneralState{}
	projected.Inventory = clientInventory(state.Inventory)
	projected.Subscriptions = map[int]SubscriptionState{}
	projected.Market = clientMarket(state.Market)
	projected.KingdomTransport = clientKingdomTransport(state.KingdomTransport)
	projected.Beri = BeriState{TroopsByUnit: map[UnitID]int64{}}
	projected.Alliance = clientAlliance(state.Alliance)
	projected.Alliances = map[AllianceID]AllianceState{}
	projected.AllianceHelpRequests = AllianceHelpRequestState{
		HospitalProductionIDs: []int64{}, RecruitmentCastleIDs: []CastleID{},
		OwnRecruitmentRequests: []RecruitmentAllianceHelpRequest{}, PendingOtherListIDs: []int64{},
	}
	projected.TowerCooldowns = map[string]TowerCooldownState{}
	projected.TowerQueue = TowerQueueState{
		EntriesByCastle: map[CastleID][]TowerQueueEntry{}, LastScannedAt: map[CastleID]time.Time{},
		LastAttemptedAt: map[CastleID]time.Time{}, ConfirmedLaunchesByCastle: map[CastleID]int64{},
		CapacityByCastle: map[CastleID]TowerCapacityObservation{},
	}
	projected.Invasion = clientInvasion(state.Invasion)
	projected.Storm = StormState{
		LastScannedAt: map[CastleID]time.Time{},
		Map:           StormMapState{Targets: map[string]MapObservation{}},
		IslandReturns: map[string]StormIslandReturnState{},
	}
	projected.NomadCamps = NomadCampState{
		LastScannedAt: map[CastleID]time.Time{}, Cooldowns: map[string]NomadCampCooldownState{},
	}
	projected.Khan = clientKhan(state.Khan)
	projected.AttackDialog = AttackDialogState{ActiveEffects: []AttackDialogEffect{}}
	projected.AttackPresets = []AttackPreset{}
	projected.AttackAnalytics = AttackAnalyticsState{
		LaunchIDs: []MovementID{}, PendingAttacks: []AttackFeatureLaunch{}, RecentAutoStormLaunches: []AttackFeatureLaunch{},
	}
	projected.EventScores = clientEventScores(state)
	projected.CommandContext = CommandContextState{}
	projected.Reports = clientReports(state)
	projected.Observations = clientObservations(state.Observations)
	return projected
}

func projectClientComponentPatch(patch *ComponentPatch) {
	if patch == nil {
		return
	}
	// Official catalog data has its own versioned endpoint; these state domains
	// have no dashboard reader and remain backend-only.
	patch.CatalogVersion = nil
	patch.LanguageVersion = nil
	patch.Generals = nil
	patch.Subscriptions = nil
	patch.Beri = nil
	patch.Alliances = nil
	patch.AllianceHelpRequests = nil
	patch.TowerCooldowns = nil
	patch.TowerQueue = nil
	patch.NomadCamps = nil
	patch.AttackDialog = nil
	patch.AttackPresets = nil
	patch.AttackAnalytics = nil
	patch.CommandContext = nil
	if patch.Castles != nil {
		value := clientCastles(*patch.Castles)
		patch.Castles = &value
	}
	if patch.CastleChanges != nil {
		value := clientCastleChanges(*patch.CastleChanges)
		patch.CastleChanges = &value
	}
	if patch.Inventory != nil {
		value := clientInventory(*patch.Inventory)
		patch.Inventory = &value
	}
	if patch.InventoryChanges != nil {
		value := *patch.InventoryChanges
		value.ConstructionItems = nil
		value.ConstructionItemsObservedAt = nil
		value.Items = nil
		value.ItemChanges = nil
		patch.InventoryChanges = &value
	}
	if patch.Market != nil {
		value := clientMarket(*patch.Market)
		patch.Market = &value
	}
	if patch.KingdomTransport != nil {
		value := clientKingdomTransport(*patch.KingdomTransport)
		patch.KingdomTransport = &value
	}
	if patch.Alliance != nil {
		value := clientAlliance(*patch.Alliance)
		patch.Alliance = &value
	}
	if patch.Invasion != nil {
		value := clientInvasion(*patch.Invasion)
		patch.Invasion = &value
	}
	if patch.Storm != nil {
		patch.Storm = nil
	}
	if patch.Khan != nil {
		value := clientKhan(*patch.Khan)
		patch.Khan = &value
	}
	if patch.EventScores != nil {
		value := clientEventScoreState(*patch.EventScores)
		patch.EventScores = &value
	}
	if patch.EventScoreChanges != nil {
		value := *patch.EventScoreChanges
		value.ShopByPackage = nil
		value.Changes = append([]EventScoreChange(nil), value.Changes...)
		for index := range value.Changes {
			if value.Changes[index].Activity != nil {
				activity := clientEventActivity(*value.Changes[index].Activity)
				value.Changes[index].Activity = &activity
			}
		}
		patch.EventScoreChanges = &value
	}
	if patch.Reports != nil {
		patch.Reports = nil
	}
	if patch.Observations != nil {
		value := clientObservations(*patch.Observations)
		patch.Observations = &value
	}
}

func clientComponentVisible(component Component) bool {
	switch component {
	case ComponentSession, ComponentAccount, ComponentPlayer, ComponentCastles,
		ComponentCommanders, ComponentCastellans, ComponentMovements, ComponentMovementSnapshot, ComponentStationing,
		ComponentScheduled, ComponentRift, ComponentInventory, ComponentMarket,
		ComponentKingdomTransport, ComponentAlliance, ComponentWorldMap, ComponentInvasion,
		ComponentStorm, ComponentAdvisor, ComponentKhan, ComponentDailyAttacks,
		ComponentEventScores, ComponentAutomations, ComponentReports, ComponentObservations:
		return true
	default:
		return false
	}
}

func clientCastles(source map[CastleID]CastleState) map[CastleID]CastleState {
	result := make(map[CastleID]CastleState, len(source))
	for id, castle := range source {
		result[id] = clientCastle(castle)
	}
	return result
}

func clientCastle(source CastleState) CastleState {
	projected := source
	projected.ContextSnapshotObservedAt = time.Time{}
	projected.FoodStateObservedAt = time.Time{}
	projected.UnitsObservedAt = time.Time{}
	projected.BuildingProduction = map[BuildingInstanceID]BuildingProduction{}
	projected.Layout = CastleLayout{}
	projected.BuildingQueue = BuildingConstructionQueue{}
	projected.ConstructionSlots = map[BuildingInstanceID][]ConstructionSlot{}
	projected.ConstructionSlotsObservedAt = time.Time{}
	projected.QueueableObservedAt = time.Time{}
	return projected
}

func clientCastleChanges(source []CastleChange) []CastleChange {
	result := make([]CastleChange, 0, len(source))
	for _, change := range source {
		projected := change
		if change.Castle != nil {
			value := clientCastle(*change.Castle)
			projected.Castle = &value
		}
		if change.Patch != nil {
			value := *change.Patch
			value.ContextSnapshotObservedAt = nil
			value.FoodStateObservedAt = nil
			value.UnitsObservedAt = nil
			value.BuildingProduction = nil
			value.Layout = nil
			value.BuildingQueue = nil
			value.ConstructionSlots = nil
			value.ConstructionSlotsObservedAt = nil
			value.QueueableObservedAt = nil
			projected.Patch = &value
		}
		result = append(result, projected)
	}
	return result
}

func clientInventory(source InventoryState) InventoryState {
	projected := source
	projected.ConstructionItems = map[ConstructionItemID]int64{}
	projected.ConstructionItemsObservedAt = time.Time{}
	projected.Items = map[string]map[int64]int64{}
	return projected
}

func clientMarket(source MarketState) MarketState {
	return MarketState{
		Castles: map[CastleID]MarketCastleState{}, Boosters: source.Boosters,
		Feast: source.Feast, BoostersObservedAt: source.BoostersObservedAt,
	}
}

func clientKingdomTransport(source KingdomTransportState) KingdomTransportState {
	unlocks := map[KingdomID]KingdomTransportUnlock{}
	if storm, found := source.Unlocks[KingdomID(4)]; found {
		unlocks[KingdomID(4)] = storm
	}
	return KingdomTransportState{
		Unlocks: unlocks, Pending: []KingdomResourceTransport{}, PendingUnits: []KingdomUnitTransport{},
		ResourceWorkflows: map[KingdomID]KingdomResourceTransportWorkflow{},
	}
}

func clientAlliance(source AllianceState) AllianceState {
	return AllianceState{
		ID: source.ID, Name: source.Name, Members: source.Members,
		Holdings: []AllianceHolding{}, ObservedAt: source.ObservedAt,
	}
}

func clientInvasion(source InvasionState) InvasionState {
	return InvasionState{
		LastScannedAt: map[CastleID]time.Time{}, FortifiedTargets: map[string]string{},
		FortifyCurrencies: append([]string(nil), source.FortifyCurrencies...),
	}
}

func newClientStormState(state GameState, now time.Time) ClientStormState {
	source := state.Storm
	mapState := ClientStormMapState{
		StormMapState: source.Map,
		Targets:       map[string]MapObservation{},
	}
	mapState.StormMapState.Targets = nil
	var nextReady time.Time
	state.RangeStormTargets(func(_ string, target MapObservation) bool {
		if target.ObservedAt.IsZero() {
			return true
		}
		readyAt := target.ObservedAt
		if target.StormCooldownRemaining > 0 &&
			(target.TypeID == MapTypeStormFort || target.TypeID == MapTypeStormIsland && target.OwnerID > 0) {
			readyAt = readyAt.Add(time.Duration(target.StormCooldownRemaining) * time.Second)
		}
		if target.TypeID == MapTypeStormIsland && target.OwnerID <= 0 && target.StormCooldownRemaining > 0 &&
			!now.Before(target.ObservedAt.Add(time.Duration(target.StormCooldownRemaining)*time.Second)) {
			return true
		}
		mapState.TargetCount++
		if !readyAt.After(now) {
			mapState.ReadyTargetCount++
		} else if nextReady.IsZero() || readyAt.Before(nextReady) {
			nextReady = readyAt
		}
		return true
	})
	if !nextReady.IsZero() {
		nextReady = nextReady.UTC()
		mapState.NextTargetReadyAt = &nextReady
	}
	return ClientStormState{Map: mapState}
}

func clientKhan(source KhanState) KhanState {
	return KhanState{
		Launches: []KhanLaunchState{}, Taunts: map[MovementID]KhanTauntState{},
		ResolvedTaunts: []KhanTauntState{}, Protection: source.Protection,
		CooldownReports: map[int64]KhanCooldownReportState{},
	}
}

func clientEventScores(state GameState) EventScoreState {
	result := EventScoreState{
		ActiveEventID: state.EventScores.ActiveEventID,
		ByEvent:       map[int64]ScalableEventScore{}, ShopByPackage: map[PackageID]EventShopRoute{},
		ActivityByEvent: map[int64]EventActivityState{}, RankingByEvent: map[int64]EventRankingState{},
		Inventory: clientEventInventory(state.EventScores.Inventory),
	}
	state.RangeScalableEventScores(func(eventID int64, score ScalableEventScore) bool {
		result.ByEvent[eventID] = score
		return true
	})
	state.RangeEventActivities(func(eventID int64, activity EventActivityState) bool {
		result.ActivityByEvent[eventID] = clientEventActivity(activity)
		return true
	})
	state.RangeEventRankings(func(eventID int64, ranking EventRankingState) bool {
		result.RankingByEvent[eventID] = ranking
		return true
	})
	return result
}

func clientEventScoreState(source EventScoreState) EventScoreState {
	activities := make(map[int64]EventActivityState, len(source.ActivityByEvent))
	for eventID, activity := range source.ActivityByEvent {
		activities[eventID] = clientEventActivity(activity)
	}
	return EventScoreState{
		ActiveEventID: source.ActiveEventID, ByEvent: source.ByEvent,
		ShopByPackage: map[PackageID]EventShopRoute{}, ActivityByEvent: activities,
		RankingByEvent: source.RankingByEvent, Inventory: clientEventInventory(source.Inventory),
	}
}

func clientEventInventory(source EventInventoryState) EventInventoryState {
	return EventInventoryState{
		ObservedAt: source.ObservedAt, ActiveByEvent: cloneMap(source.ActiveByEvent),
	}
}

func clientEventActivity(source EventActivityState) EventActivityState {
	source.LaunchIDs = nil
	source.PendingAttacks = nil
	source.ProcessedReportIDs = nil
	return source
}

func clientReports(state GameState) ReportState {
	spies := map[int64]SpyReportCapture{}
	state.RangeSpyReportCaptures(func(id int64, capture SpyReportCapture) bool {
		spies[id] = SpyReportCapture{
			MessageID: capture.MessageID, Payload: json.RawMessage(`{}`), CapturedAt: capture.CapturedAt,
		}
		return true
	})
	return ReportState{
		Notices: map[int64]ReportNotice{}, SpyCaptures: spies,
		BattleCaptures: map[int64]BattleReportCapture{},
	}
}

func clientObservations(source map[string]ProtocolObservation) map[string]ProtocolObservation {
	result := map[string]ProtocolObservation{}
	for _, opcode := range []string{"gam", "crin"} {
		if observation, found := source[opcode]; found {
			result[opcode] = observation
		}
	}
	return result
}

func (state GameState) clientMapProjection() WorldMap {
	result := WorldMap{}
	for _, kingdomID := range state.MapKingdomIDs() {
		state.RangeMapObservationsByKind(kingdomID, MapProjectionRift, func(key string, observation MapObservation) bool {
			if mapVisibleToDashboard(observation.TypeID) {
				if result[kingdomID] == nil {
					result[kingdomID] = map[string]MapObservation{}
				}
				result[kingdomID][key] = observation
			}
			return true
		})
	}
	return result
}

func clientMapProjection(source WorldMap) WorldMap {
	result := WorldMap{}
	for kingdomID, observations := range source {
		for key, observation := range observations {
			if !mapVisibleToDashboard(observation.TypeID) {
				continue
			}
			if result[kingdomID] == nil {
				result[kingdomID] = map[string]MapObservation{}
			}
			result[kingdomID][key] = observation
		}
	}
	return result
}

func clientMapChanges(source []MapChange) []MapChange {
	result := make([]MapChange, 0, len(source))
	for _, change := range source {
		typeID := change.TypeID
		if change.Observation != nil {
			typeID = change.Observation.TypeID
		}
		// An old/unknown delete is retained fail-closed so an already visible
		// coordinate cannot remain stale on a dashboard after migration.
		if typeID == 0 && change.Deleted || mapVisibleToDashboard(typeID) {
			result = append(result, change)
		}
	}
	return result
}

func mapVisibleToDashboard(typeID int) bool {
	policy, retained := MapProjectionFor(typeID)
	return retained && policy.DashboardMap
}
