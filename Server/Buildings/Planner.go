package Buildings

import (
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"

	"CitadelDesktop/Server/GameData"
	"CitadelDesktop/Server/State"
)

const (
	ActionConstruct = "construct"
	ActionUpgrade   = "upgrade"
)

type Objective struct {
	Metric string  `json:"metric"`
	Weight float64 `json:"weight"`
}

type Constraints struct {
	AllowPremium      bool               `json:"allowPremium"`
	AllowUnaffordable bool               `json:"allowUnaffordable"`
	MinimumValues     map[string]float64 `json:"minimumValues,omitempty"`
	ResourceReserves  map[string]float64 `json:"resourceReserves,omitempty"`
}

type PreviewRequest struct {
	ExpectedRevision       *uint64        `json:"expectedRevision,omitempty"`
	CastleID               State.CastleID `json:"castleId,omitempty"`
	Profile                string         `json:"profile,omitempty"`
	EventID                *int64         `json:"eventId,omitempty"`
	MapID                  *int64         `json:"mapId,omitempty"`
	Objectives             []Objective    `json:"objectives,omitempty"`
	ActiveQuestIDs         []int64        `json:"activeQuestIds,omitempty"`
	Constraints            Constraints    `json:"constraints"`
	CandidateDefinitionIDs []int64        `json:"candidateDefinitionIds,omitempty"`
	IncludeConstruct       *bool          `json:"includeConstruct,omitempty"`
	IncludeUpgrades        *bool          `json:"includeUpgrades,omitempty"`
	IncludeBlocked         bool           `json:"includeBlocked"`
	MaxCandidates          int            `json:"maxCandidates,omitempty"`
}

type Blocker struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type CostStatus struct {
	Field        string  `json:"field"`
	Key          string  `json:"key"`
	Scope        string  `json:"scope"`
	DefinitionID int64   `json:"definitionId,omitempty"`
	Required     float64 `json:"required"`
	Available    float64 `json:"available"`
	Reserve      float64 `json:"reserve"`
	Shortfall    float64 `json:"shortfall"`
	Known        bool    `json:"known"`
	Sufficient   bool    `json:"sufficient"`
	Premium      bool    `json:"premium"`
}

type Candidate struct {
	Kind                   string                      `json:"kind"`
	BuildingID             State.BuildingInstanceID    `json:"buildingId,omitempty"`
	FromDefinitionID       int64                       `json:"fromDefinitionId,omitempty"`
	Definition             GameData.BuildingDefinition `json:"definition"`
	Placement              *Placement                  `json:"placement,omitempty"`
	DurationSec            int64                       `json:"durationSec"`
	Costs                  []CostStatus                `json:"costs"`
	Affordable             bool                        `json:"affordable"`
	Eligible               bool                        `json:"eligible"`
	RequiresLiveValidation bool                        `json:"requiresLiveValidation"`
	Score                  float64                     `json:"score"`
	DeltaValues            map[string]float64          `json:"deltaValues"`
	ProjectedValues        map[string]float64          `json:"projectedValues"`
	Blockers               []Blocker                   `json:"blockers"`
}

type PreviewResult struct {
	Revision       uint64                          `json:"revision"`
	CatalogVersion string                          `json:"catalogVersion,omitempty"`
	CastleID       State.CastleID                  `json:"castleId"`
	Profile        string                          `json:"profile"`
	EventID        *int64                          `json:"eventId,omitempty"`
	MapID          *int64                          `json:"mapId,omitempty"`
	Objectives     []Objective                     `json:"objectives"`
	Constraints    Constraints                     `json:"constraints"`
	CurrentValues  map[string]float64              `json:"currentValues"`
	Layout         LayoutAnalysis                  `json:"layout"`
	BuildingQueue  State.BuildingConstructionQueue `json:"buildingQueue"`
	Candidates     []Candidate                     `json:"candidates"`
	Recommended    *Candidate                      `json:"recommended,omitempty"`
	RejectedCount  int                             `json:"rejectedCount"`
	Warnings       []string                        `json:"warnings"`
}

type RevisionMismatchError struct {
	Expected uint64
	Actual   uint64
}

func (err RevisionMismatchError) Error() string {
	return fmt.Sprintf("state revision %d does not match expected revision %d", err.Actual, err.Expected)
}

func Preview(state State.GameState, gameData *GameData.Store, request PreviewRequest) (PreviewResult, error) {
	if request.ExpectedRevision != nil && *request.ExpectedRevision != state.Revision {
		return PreviewResult{}, RevisionMismatchError{Expected: *request.ExpectedRevision, Actual: state.Revision}
	}
	if gameData == nil {
		return PreviewResult{}, fmt.Errorf("official game data is unavailable")
	}
	catalog, err := gameData.BuildingCatalog()
	if err != nil {
		return PreviewResult{}, err
	}
	castleID, castle, found := previewCastle(state, request.CastleID)
	if !found {
		if request.CastleID > 0 {
			return PreviewResult{}, fmt.Errorf("castle %d was not found", request.CastleID)
		}
		return PreviewResult{}, fmt.Errorf("no focused castle is available")
	}
	castle = normalizePreviewLayout(castle, catalog)
	profile, eventID, objectives, err := resolveProfile(request.Profile, request.EventID, request.Objectives)
	if err != nil {
		return PreviewResult{}, err
	}
	request.Constraints.MinimumValues = cloneFloatMap(request.Constraints.MinimumValues)
	request.Constraints.ResourceReserves = cloneFloatMap(request.Constraints.ResourceReserves)
	currentValues := currentBuildingValues(castle, catalog)
	result := PreviewResult{
		Revision: state.Revision, CatalogVersion: gameData.Metadata().ItemVersion, CastleID: castleID,
		Profile: profile, EventID: eventID, MapID: cloneInt64(request.MapID), Objectives: objectives, Constraints: request.Constraints,
		CurrentValues: currentValues, Layout: AnalyzeLayout(castle, catalog), BuildingQueue: castle.BuildingQueue,
		Candidates: []Candidate{}, Warnings: []string{
			"This preview uses official static data; typed building intents repeat validation against a fresh castle snapshot before execution.",
		},
	}
	if castle.Layout.ObservedAt.IsZero() {
		result.Warnings = append(result.Warnings, "The castle layout has not been observed in the current normalized format.")
	}
	if castle.BuildingQueue.ObservedAt.IsZero() && len(castle.BuildingQueue.Slots) == 0 {
		result.Warnings = append(result.Warnings, "The building construction queue has not been observed; queue availability is unverified.")
	}
	if profile == "berimond" {
		result.Warnings = append(result.Warnings, "layoutMoralePercent is calculated as 100 plus the official Moral values of placed objects; temporary live bonuses are not included.")
	}

	includeConstruct := request.IncludeConstruct == nil || *request.IncludeConstruct
	includeUpgrades := request.IncludeUpgrades == nil || *request.IncludeUpgrades
	activeQuestIDs := int64Set(request.ActiveQuestIDs)
	allCandidates := make([]Candidate, 0)
	if includeUpgrades {
		allCandidates = append(allCandidates, upgradeCandidates(
			state, castle, catalog, eventID, request.MapID, objectives, request.Constraints, currentValues, activeQuestIDs,
		)...)
	}
	if includeConstruct {
		allCandidates = append(allCandidates, constructionCandidates(
			state, castle, catalog, eventID, request.MapID, objectives, request.Constraints, request.CandidateDefinitionIDs,
			currentValues, activeQuestIDs,
		)...)
	}
	sortCandidates(allCandidates)
	for index := range allCandidates {
		if allCandidates[index].Eligible && result.Recommended == nil {
			copy := cloneCandidate(allCandidates[index])
			result.Recommended = &copy
		}
	}
	result.RejectedCount = countRejected(allCandidates)
	if !request.IncludeBlocked {
		eligible := allCandidates[:0]
		for _, candidate := range allCandidates {
			if candidate.Eligible {
				eligible = append(eligible, candidate)
			}
		}
		allCandidates = eligible
	}
	limit := request.MaxCandidates
	if limit <= 0 {
		limit = 100
	}
	if limit > 500 {
		limit = 500
	}
	if len(allCandidates) > limit {
		allCandidates = allCandidates[:limit]
	}
	result.Candidates = allCandidates
	return result, nil
}

func constructionCandidates(
	state State.GameState,
	castle State.CastleState,
	catalog *GameData.BuildingCatalog,
	eventID *int64,
	mapID *int64,
	objectives []Objective,
	constraints Constraints,
	explicitIDs []int64,
	currentValues map[string]float64,
	activeQuestIDs map[int64]struct{},
) []Candidate {
	definitions := make([]GameData.BuildingDefinition, 0)
	explicit := len(explicitIDs) > 0
	if explicit {
		seen := map[int64]struct{}{}
		for _, id := range explicitIDs {
			if _, duplicate := seen[id]; duplicate {
				continue
			}
			seen[id] = struct{}{}
			if definition, found := catalog.Definition(id); found {
				definitions = append(definitions, definition)
			}
		}
	} else {
		for _, definition := range catalog.Definitions() {
			if definition.DowngradeDefinitionID != 0 || !strings.EqualFold(definition.Group, "Building") ||
				definition.ShopCategory == "" || definition.Width <= 0 || definition.Height <= 0 {
				continue
			}
			delta := actionDelta(definition, nil, activeQuestIDs)
			if scoreValues(delta, objectives) == 0 {
				continue
			}
			definitions = append(definitions, definition)
		}
	}
	sort.Slice(definitions, func(left, right int) bool { return definitions[left].ID < definitions[right].ID })
	counts := buildingCountsByName(castle, catalog)
	result := make([]Candidate, 0, len(definitions))
	for _, definition := range definitions {
		candidate := Candidate{
			Kind: ActionConstruct, Definition: definition, DurationSec: definition.DurationSec,
			RequiresLiveValidation: true, DeltaValues: actionDelta(definition, nil, activeQuestIDs),
		}
		candidate.Blockers = append(candidate.Blockers, definitionBlockers(state.Player, castle, definition, eventID, mapID)...)
		if definition.DowngradeDefinitionID != 0 {
			addBlocker(&candidate, "not_construction_root", "The definition is an upgrade level, not a construction root")
		}
		if !strings.EqualFold(definition.Group, "Building") || definition.ShopCategory == "" {
			addBlocker(&candidate, "not_static_shop_candidate", "The definition is not a statically discoverable shop construction")
		}
		if definition.ForcedPosition != "" {
			addBlocker(&candidate, "forced_position", "The definition requires a server-defined position")
		}
		if definition.MaximumCount != nil && *definition.MaximumCount >= 0 &&
			counts[strings.ToLower(definition.InternalName)] >= *definition.MaximumCount {
			addBlocker(&candidate, "maximum_count", "The official maximum count is already reached")
		}
		if len(definition.Costs) == 0 {
			addBlocker(&candidate, "availability_unknown", "The static definition has no resource cost; inventory or live-offer availability is unknown")
		}
		finalizeCandidate(&candidate, state, castle, objectives, constraints, currentValues)
		if len(candidate.Blockers) == 0 || explicit {
			if placement, ok := FindPlacement(castle, definition, catalog, 0); ok {
				candidate.Placement = placement
			} else {
				addBlocker(&candidate, "no_space", "No collision-free placement fits on currently observed ground")
			}
		}
		candidate.Eligible = len(candidate.Blockers) == 0
		result = append(result, candidate)
	}
	return result
}

func upgradeCandidates(
	state State.GameState,
	castle State.CastleState,
	catalog *GameData.BuildingCatalog,
	eventID *int64,
	mapID *int64,
	objectives []Objective,
	constraints Constraints,
	currentValues map[string]float64,
	activeQuestIDs map[int64]struct{},
) []Candidate {
	result := make([]Candidate, 0)
	for _, source := range previewUpgradeBuildings(castle) {
		buildingID := source.building.InstanceID
		building := source.building
		current, found := catalog.Definition(int64(building.DefinitionID))
		if !found || current.UpgradeDefinitionID <= 0 || strings.EqualFold(current.Group, "Ground") {
			continue
		}
		next, found := catalog.Definition(current.UpgradeDefinitionID)
		if !found {
			continue
		}
		candidate := Candidate{
			Kind: ActionUpgrade, BuildingID: buildingID, FromDefinitionID: current.ID, Definition: next,
			DurationSec: next.DurationSec, RequiresLiveValidation: true,
			DeltaValues: actionDelta(next, &current, activeQuestIDs),
		}
		candidate.Blockers = append(candidate.Blockers, definitionBlockers(state.Player, castle, next, eventID, mapID)...)
		finalizeCandidate(&candidate, state, castle, objectives, constraints, currentValues)
		if source.fixed {
			candidate.Eligible = len(candidate.Blockers) == 0
			result = append(result, candidate)
			continue
		}
		if !building.Placed {
			addBlocker(&candidate, "building_not_placed", "The current object is not placed in the castle")
		} else if len(candidate.Blockers) == 0 {
			width, height := rotatedDimensions(next.Width, next.Height, building.Rotation)
			placement := Placement{
				GridX: building.GridX, GridY: building.GridY, Rotation: building.Rotation, Width: width, Height: height,
			}
			issues := ValidatePlacement(castle, next, placement, catalog, buildingID)
			if len(issues) > 0 {
				addBlocker(&candidate, "upgrade_footprint_invalid", issues[0].Message)
			} else {
				candidate.Placement = &placement
			}
		}
		candidate.Eligible = len(candidate.Blockers) == 0
		result = append(result, candidate)
	}
	return result
}

func finalizeCandidate(
	candidate *Candidate,
	state State.GameState,
	castle State.CastleState,
	objectives []Objective,
	constraints Constraints,
	currentValues map[string]float64,
) {
	candidate.Score = scoreValues(candidate.DeltaValues, objectives)
	if candidate.Score <= 0 {
		addBlocker(candidate, "no_objective_gain", "The action does not improve the selected weighted objectives")
	}
	candidate.ProjectedValues = cloneFloatMap(currentValues)
	if len(candidate.ProjectedValues) == 0 {
		candidate.ProjectedValues = map[string]float64{}
	}
	for key, value := range candidate.DeltaValues {
		addMetric(candidate.ProjectedValues, key, value)
	}
	for metric, minimum := range constraints.MinimumValues {
		current := metricValue(currentValues, metric)
		projected := metricValue(candidate.ProjectedValues, metric)
		if current >= minimum && projected < minimum {
			addBlocker(candidate, "minimum_value", fmt.Sprintf("%s would fall below the required minimum %.2f", metric, minimum))
		} else if current < minimum && projected <= current {
			addBlocker(candidate, "minimum_value_progress", fmt.Sprintf("%s must increase toward the required minimum %.2f first", metric, minimum))
		}
	}
	candidate.Costs, candidate.Affordable = buildingCostStatus(state, castle, candidate.Definition.Costs, constraints.ResourceReserves)
	for _, cost := range candidate.Costs {
		if cost.Premium && !constraints.AllowPremium {
			addBlocker(candidate, "premium_disallowed", "The action spends premium currency and premium spending is disabled")
			break
		}
	}
	if !candidate.Affordable && !constraints.AllowUnaffordable {
		addBlocker(candidate, "insufficient_resources", "Observed balances do not cover the cost and configured reserves")
	}
	if buildingQueueObserved(castle.BuildingQueue) && !buildingQueueAvailable(castle.BuildingQueue) {
		addBlocker(candidate, "construction_queue_full", "No available building construction slot was observed")
	}
}

func definitionBlockers(
	player State.PlayerState,
	castle State.CastleState,
	definition GameData.BuildingDefinition,
	eventID *int64,
	mapID *int64,
) []Blocker {
	result := []Blocker{}
	if definition.RequiredLevel != nil && int64(player.Level) < *definition.RequiredLevel {
		result = append(result, Blocker{Code: "player_level", Message: fmt.Sprintf("Player level %d is below required level %d", player.Level, *definition.RequiredLevel)})
	}
	if definition.RequiredLegendLevel != nil && int64(player.LegendLevel) < *definition.RequiredLegendLevel {
		result = append(result, Blocker{Code: "legend_level", Message: fmt.Sprintf("Legend level %d is below required level %d", player.LegendLevel, *definition.RequiredLegendLevel)})
	}
	if len(definition.KingdomIDs) > 0 && !containsInt64(definition.KingdomIDs, int64(castle.KingdomID)) {
		result = append(result, Blocker{Code: "kingdom", Message: fmt.Sprintf("The definition is not available in kingdom %d", castle.KingdomID)})
	}
	if len(definition.AreaTypeIDs) > 0 && !containsInt64(definition.AreaTypeIDs, int64(castle.SlotType)) {
		result = append(result, Blocker{Code: "area_type", Message: fmt.Sprintf("The definition is not available for castle area type %d", castle.SlotType)})
	}
	if len(definition.EventIDs) > 0 {
		if eventID == nil {
			result = append(result, Blocker{Code: "event_context", Message: "The definition requires a matching active event context"})
		} else if !containsInt64(definition.EventIDs, *eventID) {
			result = append(result, Blocker{Code: "event", Message: fmt.Sprintf("The definition is not available in event %d", *eventID)})
		}
	}
	if len(definition.MapIDs) > 0 {
		if mapID == nil {
			result = append(result, Blocker{Code: "map_context", Message: "The definition requires a matching map context"})
		} else if !containsInt64(definition.MapIDs, *mapID) {
			result = append(result, Blocker{Code: "map", Message: fmt.Sprintf("The definition is not available on map %d", *mapID)})
		}
	}
	return result
}

func actionDelta(
	target GameData.BuildingDefinition,
	current *GameData.BuildingDefinition,
	activeQuestIDs map[int64]struct{},
) map[string]float64 {
	delta := cloneFloatMap(target.Values)
	if current != nil {
		for key, value := range current.Values {
			addMetric(delta, key, -value)
		}
	}
	levelDelta := float64(target.Level)
	if current != nil {
		levelDelta -= float64(current.Level)
	}
	delta["buildingLevel"] = levelDelta
	delta["unlockCount"] = float64(len(target.UnlockIDs))
	area := float64(target.Width * target.Height)
	if area > 0 {
		delta["unitSizePerCell"] = metricValue(delta, "unitSize") / area
		delta["MoralPerCell"] = metricValue(delta, "Moral") / area
		delta["islandAlliancePointsPerCell"] = metricValue(delta, "islandAlliancePoints") / area
	}
	delta["layoutMorale"] = metricValue(delta, "Moral")
	delta["layoutMoralePercent"] = metricValue(delta, "Moral")
	for _, objective := range target.QuestObjectives {
		if _, active := activeQuestIDs[objective.QuestID]; active {
			delta["activeQuestCount"]++
			delta["activeQuestXP"] += objective.XP
		}
	}
	return delta
}

func currentBuildingValues(castle State.CastleState, catalog *GameData.BuildingCatalog) map[string]float64 {
	result := map[string]float64{}
	for _, building := range previewValueBuildings(castle) {
		if !building.Placed || !buildingValuesActive(building) {
			continue
		}
		definition, found := catalog.Definition(int64(building.DefinitionID))
		if !found {
			continue
		}
		for key, value := range definition.Values {
			addMetric(result, key, value)
		}
	}
	if castle.KingdomID == 10 || metricExists(result, "Moral") {
		moral := metricValue(result, "Moral")
		result["layoutMorale"] = moral
		result["layoutMoralePercent"] = 100 + moral
	}
	return result
}

func buildingValuesActive(building State.Building) bool {
	switch building.ConstructionState {
	case State.BuildingStateDisassembleStopped, State.BuildingStateDisassembleInProgress,
		State.BuildingStateDisassembledCompleted:
		return false
	case State.BuildingStateBuildStopped, State.BuildingStateBuildInProgress:
		return building.Level != 1
	default:
		return true
	}
}

func resolveProfile(raw string, eventID *int64, custom []Objective) (string, *int64, []Objective, error) {
	profile := strings.ToLower(strings.TrimSpace(raw))
	if profile == "" {
		profile = "progression"
	}
	if profile == "balanced" {
		profile = "progression"
	}
	if len(custom) > 0 {
		objectives, err := validateObjectives(custom)
		if err != nil {
			return "", nil, nil, err
		}
		return profile, cloneInt64(eventID), objectives, nil
	}
	var objectives []Objective
	switch profile {
	case "progression":
		objectives = []Objective{
			{Metric: "activeQuestCount", Weight: 200}, {Metric: "activeQuestXP", Weight: .1},
			{Metric: "unlockCount", Weight: 50}, {Metric: "Foodproduction", Weight: 10},
			{Metric: "Woodproduction", Weight: 4}, {Metric: "Stoneproduction", Weight: 4},
			{Metric: "foodStorage", Weight: .02}, {Metric: "woodStorage", Weight: .01},
			{Metric: "stoneStorage", Weight: .01}, {Metric: "Population", Weight: .5},
			{Metric: "unitSize", Weight: 2}, {Metric: "auxiliaryCapacity", Weight: 2},
			{Metric: "xp", Weight: .2}, {Metric: "buildingLevel", Weight: 1},
		}
	case "berimond":
		if eventID == nil {
			value := int64(3)
			eventID = &value
		}
		objectives = []Objective{
			{Metric: "unitSizePerCell", Weight: 100}, {Metric: "MoralPerCell", Weight: 10},
			{Metric: "unitSize", Weight: 5}, {Metric: "Moral", Weight: 1},
		}
	case "storm":
		objectives = []Objective{
			{Metric: "islandAlliancePointsPerCell", Weight: 100}, {Metric: "islandAlliancePoints", Weight: 1},
			{Metric: "aquamarineStorage", Weight: .01},
		}
	case "custom":
		return "", nil, nil, fmt.Errorf("custom profile requires at least one objective")
	default:
		return "", nil, nil, fmt.Errorf("unknown building profile %q", raw)
	}
	return profile, cloneInt64(eventID), objectives, nil
}

func validateObjectives(source []Objective) ([]Objective, error) {
	result := make([]Objective, 0, len(source))
	for _, objective := range source {
		objective.Metric = strings.TrimSpace(objective.Metric)
		if objective.Metric == "" {
			return nil, fmt.Errorf("building objective metric is required")
		}
		if math.IsNaN(objective.Weight) || math.IsInf(objective.Weight, 0) {
			return nil, fmt.Errorf("building objective %s has an invalid weight", objective.Metric)
		}
		result = append(result, objective)
	}
	return result, nil
}

func buildingCostStatus(
	state State.GameState,
	castle State.CastleState,
	costs []GameData.BuildingCost,
	reserves map[string]float64,
) ([]CostStatus, bool) {
	result := make([]CostStatus, 0, len(costs))
	affordable := true
	for _, cost := range costs {
		status := CostStatus{
			Field: cost.Field, Key: cost.Key, Scope: cost.Scope, DefinitionID: cost.DefinitionID,
			Required: cost.Amount, Reserve: reserveValue(reserves, cost), Premium: cost.Premium,
		}
		switch cost.Scope {
		case GameData.BuildingCostCastleResource:
			balance, found := castle.Resources[State.ResourceID(cost.DefinitionID)]
			status.Known, status.Available = found, balance.Amount
		case GameData.BuildingCostPlayerResource:
			status.Available, status.Known = state.Player.Resources[State.ResourceID(cost.DefinitionID)]
		case GameData.BuildingCostCurrency:
			status.Available, status.Known = state.Player.Currencies[State.CurrencyID(cost.DefinitionID)]
		}
		status.Shortfall = cost.Amount + status.Reserve - status.Available
		if status.Shortfall < 0 {
			status.Shortfall = 0
		}
		status.Sufficient = status.Known && status.Shortfall == 0
		if !status.Sufficient {
			affordable = false
		}
		result = append(result, status)
	}
	return result, affordable
}

func normalizePreviewLayout(castle State.CastleState, catalog *GameData.BuildingCatalog) State.CastleState {
	if len(castle.Layout.Ground)+len(castle.Layout.Objects)+len(castle.Layout.Fixed) > 0 {
		return castle
	}
	// Preview may receive an immutable planning generation. Build derived legacy
	// layout maps locally instead of writing through the maps owned by that view.
	castle.Layout.Ground = map[State.BuildingInstanceID]State.Building{}
	castle.Layout.Objects = map[State.BuildingInstanceID]State.Building{}
	castle.Layout.Fixed = map[State.BuildingInstanceID]State.Building{}
	for id, building := range castle.Buildings {
		if building.Layer == "" {
			building.Placed = building.GridX >= 0 && building.GridY >= 0
		}
		definition, found := catalog.Definition(int64(building.DefinitionID))
		if found && strings.EqualFold(definition.Group, "Ground") {
			castle.Layout.Ground[id] = building
		} else if found && fixedBuildingGroup(definition.Group) {
			castle.Layout.Fixed[id] = building
		} else {
			castle.Layout.Objects[id] = building
		}
	}
	return castle
}

func previewCastle(state State.GameState, requested State.CastleID) (State.CastleID, State.CastleState, bool) {
	if requested > 0 {
		castle, found := state.Castles[requested]
		return requested, castle, found
	}
	ids := make([]State.CastleID, 0, len(state.Castles))
	for id, castle := range state.Castles {
		if castle.Focused {
			ids = append(ids, id)
		}
	}
	if len(ids) == 0 {
		return 0, State.CastleState{}, false
	}
	sort.Slice(ids, func(left, right int) bool { return ids[left] < ids[right] })
	return ids[0], state.Castles[ids[0]], true
}

func buildingCountsByName(castle State.CastleState, catalog *GameData.BuildingCatalog) map[string]int64 {
	result := map[string]int64{}
	for _, building := range previewValueBuildings(castle) {
		definition, found := catalog.Definition(int64(building.DefinitionID))
		if found && definition.InternalName != "" {
			result[strings.ToLower(definition.InternalName)]++
		}
	}
	return result
}

type previewUpgradeBuilding struct {
	building State.Building
	fixed    bool
}

func previewUpgradeBuildings(castle State.CastleState) []previewUpgradeBuilding {
	result := make([]previewUpgradeBuilding, 0, len(castle.Layout.Objects)+len(castle.Layout.Fixed))
	for _, building := range castle.Layout.Objects {
		result = append(result, previewUpgradeBuilding{building: building})
	}
	for _, building := range castle.Layout.Fixed {
		result = append(result, previewUpgradeBuilding{building: building, fixed: true})
	}
	sort.Slice(result, func(left, right int) bool {
		return result[left].building.InstanceID < result[right].building.InstanceID
	})
	return result
}

func previewValueBuildings(castle State.CastleState) []State.Building {
	result := make([]State.Building, 0, len(castle.Layout.Objects)+len(castle.Layout.Fixed))
	for _, building := range castle.Layout.Objects {
		result = append(result, building)
	}
	for _, building := range castle.Layout.Fixed {
		result = append(result, building)
	}
	sort.Slice(result, func(left, right int) bool { return result[left].InstanceID < result[right].InstanceID })
	return result
}

func fixedBuildingGroup(group string) bool {
	switch strings.ToLower(strings.TrimSpace(group)) {
	case "tower", "gate", "defence", "moat":
		return true
	default:
		return false
	}
}

func sortCandidates(candidates []Candidate) {
	sort.SliceStable(candidates, func(left, right int) bool {
		if candidates[left].Eligible != candidates[right].Eligible {
			return candidates[left].Eligible
		}
		if candidates[left].Score != candidates[right].Score {
			return candidates[left].Score > candidates[right].Score
		}
		if candidates[left].Affordable != candidates[right].Affordable {
			return candidates[left].Affordable
		}
		if candidates[left].DurationSec != candidates[right].DurationSec {
			return candidates[left].DurationSec < candidates[right].DurationSec
		}
		if candidates[left].Kind != candidates[right].Kind {
			return candidates[left].Kind < candidates[right].Kind
		}
		if candidates[left].BuildingID != candidates[right].BuildingID {
			return candidates[left].BuildingID < candidates[right].BuildingID
		}
		return candidates[left].Definition.ID < candidates[right].Definition.ID
	})
}

func scoreValues(values map[string]float64, objectives []Objective) float64 {
	result := 0.0
	for _, objective := range objectives {
		result += metricValue(values, objective.Metric) * objective.Weight
	}
	return result
}

func metricValue(values map[string]float64, metric string) float64 {
	if value, found := values[metric]; found {
		return value
	}
	for key, value := range values {
		if strings.EqualFold(key, metric) {
			return value
		}
	}
	return 0
}

func metricExists(values map[string]float64, metric string) bool {
	for key := range values {
		if strings.EqualFold(key, metric) {
			return true
		}
	}
	return false
}

func addMetric(values map[string]float64, metric string, amount float64) {
	for key := range values {
		if strings.EqualFold(key, metric) {
			values[key] += amount
			return
		}
	}
	values[metric] += amount
}

func addBlocker(candidate *Candidate, code, message string) {
	for _, blocker := range candidate.Blockers {
		if blocker.Code == code {
			return
		}
	}
	candidate.Blockers = append(candidate.Blockers, Blocker{Code: code, Message: message})
}

func reserveValue(reserves map[string]float64, cost GameData.BuildingCost) float64 {
	for key, value := range reserves {
		if strings.EqualFold(key, cost.Key) || strings.EqualFold(key, cost.Field) || key == strconv.FormatInt(cost.DefinitionID, 10) {
			return value
		}
	}
	return 0
}

func buildingQueueObserved(queue State.BuildingConstructionQueue) bool {
	return !queue.ObservedAt.IsZero() || len(queue.Slots) > 0 || queue.SlotCount > 0
}

func buildingQueueAvailable(queue State.BuildingConstructionQueue) bool {
	if len(queue.Slots) == 0 {
		return true
	}
	for _, slot := range queue.Slots {
		if slot.Status == State.BuildingQueueSlotAvailable {
			return true
		}
	}
	return false
}

func containsInt64(values []int64, target int64) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func int64Set(values []int64) map[int64]struct{} {
	result := make(map[int64]struct{}, len(values))
	for _, value := range values {
		if value > 0 {
			result[value] = struct{}{}
		}
	}
	return result
}

func cloneFloatMap(source map[string]float64) map[string]float64 {
	result := make(map[string]float64, len(source))
	for key, value := range source {
		result[key] = value
	}
	return result
}

func cloneInt64(value *int64) *int64 {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}

func cloneCandidate(source Candidate) Candidate {
	clone := source
	clone.Costs = append([]CostStatus(nil), source.Costs...)
	clone.DeltaValues = cloneFloatMap(source.DeltaValues)
	clone.ProjectedValues = cloneFloatMap(source.ProjectedValues)
	clone.Blockers = append([]Blocker(nil), source.Blockers...)
	if source.Placement != nil {
		placement := *source.Placement
		clone.Placement = &placement
	}
	return clone
}

func countRejected(candidates []Candidate) int {
	count := 0
	for _, candidate := range candidates {
		if !candidate.Eligible {
			count++
		}
	}
	return count
}
