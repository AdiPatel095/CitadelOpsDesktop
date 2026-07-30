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
	ExpansionPaymentResources = "resources"
	ExpansionPaymentPremium   = "premium"

	kingdomTransportDeliveryFactor = 0.90
)

type ExpansionPreviewRequest struct {
	ExpectedRevision       *uint64            `json:"expectedRevision,omitempty"`
	CastleID               State.CastleID     `json:"castleId"`
	Payment                string             `json:"payment,omitempty"`
	ResourceReserves       map[string]float64 `json:"resourceReserves,omitempty"`
	SourceResourceReserves map[string]float64 `json:"sourceResourceReserves,omitempty"`
	AllowPremium           bool               `json:"allowPremium,omitempty"`
	AllowTimeSkips         bool               `json:"allowTimeSkips,omitempty"`
	X                      *int               `json:"x,omitempty"`
	Y                      *int               `json:"y,omitempty"`
	Direction              *int               `json:"direction,omitempty"`
}

type ExpansionCostStatus struct {
	CostStatus
	Capacity           float64 `json:"capacity,omitempty"`
	CapacityKnown      bool    `json:"capacityKnown"`
	CapacityShortfall  float64 `json:"capacityShortfall"`
	CapacitySufficient bool    `json:"capacitySufficient"`
	StorageMetric      string  `json:"storageMetric,omitempty"`
}

type ExpansionConstructionItemCandidate struct {
	ConstructionItemID   State.ConstructionItemID `json:"constructionItemId"`
	GroupID              int64                    `json:"groupId"`
	Level                int64                    `json:"level"`
	DisplayName          string                   `json:"displayName"`
	BuildingInstanceID   State.BuildingInstanceID `json:"buildingInstanceId,omitempty"`
	BuildingDefinitionID State.BuildingID         `json:"buildingDefinitionId,omitempty"`
	InventoryCount       int64                    `json:"inventoryCount"`
	CapacityGain         map[string]float64       `json:"capacityGain"`
	Eligible             bool                     `json:"eligible"`
	ClosesCapacityGap    bool                     `json:"closesCapacityGap"`
	Blockers             []Blocker                `json:"blockers"`
}

type ExpansionTransportGood struct {
	ResourceID State.ResourceID `json:"resourceId"`
	Key        string           `json:"key"`
	Amount     int64            `json:"amount"`
}

type ExpansionTransportCandidate struct {
	SourceCastleID   State.CastleID           `json:"sourceCastleId"`
	SourceKingdomID  State.KingdomID          `json:"sourceKingdomId"`
	TargetCastleID   State.CastleID           `json:"targetCastleId"`
	TargetKingdomID  State.KingdomID          `json:"targetKingdomId"`
	Goods            []ExpansionTransportGood `json:"goods"`
	Coverage         float64                  `json:"coverage"`
	RemainingSurplus float64                  `json:"remainingSurplus"`
	Complete         bool                     `json:"complete"`
	Eligible         bool                     `json:"eligible"`
	Blockers         []Blocker                `json:"blockers"`
}

type ExpansionTimeSkipOption struct {
	CurrencyID State.CurrencyID `json:"currencyId"`
	TimeSkipID string           `json:"timeSkipId"`
	Minutes    int              `json:"minutes"`
	Balance    float64          `json:"balance"`
}

type ExpansionPendingStorageBuild struct {
	BuildingInstanceID    State.BuildingInstanceID `json:"buildingInstanceId"`
	CurrentDefinitionID   State.BuildingID         `json:"currentDefinitionId"`
	TargetDefinitionID    State.BuildingID         `json:"targetDefinitionId"`
	ProgressSec           int64                    `json:"progressSec"`
	EstimatedRemainingSec int64                    `json:"estimatedRemainingSec"`
	CapacityGain          map[string]float64       `json:"capacityGain"`
	ClosesCapacityGap     bool                     `json:"closesCapacityGap"`
}

type ExpansionAction struct {
	Kind      string         `json:"kind"`
	Intent    string         `json:"intent"`
	Arguments map[string]any `json:"arguments"`
	Reason    string         `json:"reason"`
}

type ExpansionPreviewResult struct {
	Revision                  uint64                               `json:"revision"`
	CatalogVersion            string                               `json:"catalogVersion,omitempty"`
	CastleID                  State.CastleID                       `json:"castleId"`
	KingdomID                 State.KingdomID                      `json:"kingdomId"`
	Payment                   string                               `json:"payment"`
	CurrentExpansionLevel     int64                                `json:"currentExpansionLevel"`
	NextExpansion             *GameData.ExpansionDefinition        `json:"nextExpansion,omitempty"`
	Costs                     []ExpansionCostStatus                `json:"costs"`
	StorageBuildingCandidates []Candidate                          `json:"storageBuildingCandidates"`
	StorageItemCandidates     []ExpansionConstructionItemCandidate `json:"storageItemCandidates"`
	TransportCandidates       []ExpansionTransportCandidate        `json:"transportCandidates"`
	PendingTransport          *State.KingdomResourceTransport      `json:"pendingTransport,omitempty"`
	PendingStorageBuild       *ExpansionPendingStorageBuild        `json:"pendingStorageBuild,omitempty"`
	TimeSkipOptions           []ExpansionTimeSkipOption            `json:"timeSkipOptions"`
	Ready                     bool                                 `json:"ready"`
	RecommendedAction         *ExpansionAction                     `json:"recommendedAction,omitempty"`
	Blockers                  []Blocker                            `json:"blockers"`
	Warnings                  []string                             `json:"warnings"`
}

func PreviewExpansion(state State.GameState, gameData *GameData.Store, request ExpansionPreviewRequest) (ExpansionPreviewResult, error) {
	if request.ExpectedRevision != nil && *request.ExpectedRevision != state.Revision {
		return ExpansionPreviewResult{}, RevisionMismatchError{Expected: *request.ExpectedRevision, Actual: state.Revision}
	}
	if gameData == nil {
		return ExpansionPreviewResult{}, fmt.Errorf("official game data is unavailable")
	}
	castle, found := state.Castles[request.CastleID]
	if !found || request.CastleID <= 0 {
		return ExpansionPreviewResult{}, fmt.Errorf("castle %d was not found", request.CastleID)
	}
	payment, err := normalizeExpansionPayment(request.Payment)
	if err != nil {
		return ExpansionPreviewResult{}, err
	}
	if err := validateExpansionPosition(request); err != nil {
		return ExpansionPreviewResult{}, err
	}
	catalog, err := gameData.ExpansionCatalog()
	if err != nil {
		return ExpansionPreviewResult{}, err
	}
	currentLevel := int64(len(castle.Layout.Ground) - 1)
	if currentLevel < 0 {
		currentLevel = 0
	}
	nextLevel := currentLevel + 1
	result := ExpansionPreviewResult{
		Revision: state.Revision, CatalogVersion: gameData.Metadata().ItemVersion,
		CastleID: castle.ID, KingdomID: castle.KingdomID, Payment: payment,
		CurrentExpansionLevel: currentLevel, Costs: []ExpansionCostStatus{},
		StorageBuildingCandidates: []Candidate{}, StorageItemCandidates: []ExpansionConstructionItemCandidate{},
		TransportCandidates: []ExpansionTransportCandidate{}, TimeSkipOptions: []ExpansionTimeSkipOption{},
		Blockers: []Blocker{}, Warnings: []string{
			"The next expansion level is inferred from the focused castle's observed ground-tile count, matching the official client calculation.",
		},
	}
	if !castle.Focused || castle.Layout.ObservedAt.IsZero() {
		addExpansionBlocker(&result, "layout_unobserved", "The castle needs a fresh focused layout before an expansion can be planned")
	}
	definition, exists := catalog.Definition(int64(castle.KingdomID), nextLevel)
	if !exists {
		addExpansionBlocker(&result, "no_next_expansion", fmt.Sprintf("Official data has no expansion level %d for kingdom %d", nextLevel, castle.KingdomID))
		if !castle.Focused || castle.Layout.ObservedAt.IsZero() {
			result.RecommendedAction = expansionRefreshAction(castle.ID, "Refresh the castle before confirming that no further expansion is available")
		}
		return result, nil
	}
	result.NextExpansion = &definition
	selectedCosts := expansionCostsForPayment(definition.Costs, payment)
	if len(selectedCosts) == 0 {
		addExpansionBlocker(&result, "payment_unavailable", fmt.Sprintf("Expansion level %d has no %s payment path", nextLevel, payment))
	}
	if payment == ExpansionPaymentPremium && !request.AllowPremium {
		addExpansionBlocker(&result, "premium_disallowed", "Premium expansion payment requires allowPremium=true")
	}
	if definition.EffectLocked && definition.SceatSkillLocked > 0 &&
		!containsInt64(state.Player.LegendSkills.SceatSkillIDs, definition.SceatSkillLocked) {
		addExpansionBlocker(&result, "sceat_skill_locked", fmt.Sprintf("Expansion level %d requires Sceat skill %d", nextLevel, definition.SceatSkillLocked))
	}
	baseStatuses, _ := buildingCostStatus(state, castle, selectedCosts, request.ResourceReserves)
	for _, status := range baseStatuses {
		expansionStatus := ExpansionCostStatus{CostStatus: status, CapacitySufficient: true}
		if status.Scope == GameData.BuildingCostCastleResource {
			balance, known := castle.Resources[State.ResourceID(status.DefinitionID)]
			expansionStatus.CapacityKnown = known && balance.Capacity != nil
			if expansionStatus.CapacityKnown {
				expansionStatus.Capacity = *balance.Capacity
				expansionStatus.CapacityShortfall = status.Required + status.Reserve - expansionStatus.Capacity
				if expansionStatus.CapacityShortfall < 0 {
					expansionStatus.CapacityShortfall = 0
				}
			}
			expansionStatus.CapacitySufficient = expansionStatus.CapacityKnown && expansionStatus.CapacityShortfall == 0
			expansionStatus.StorageMetric = expansionStorageMetric(status.Key)
			if !expansionStatus.CapacityKnown {
				addExpansionBlocker(&result, "capacity_unobserved", fmt.Sprintf("Storage capacity for %s has not been observed", status.Key))
			} else if !expansionStatus.CapacitySufficient {
				addExpansionBlocker(&result, "storage_capacity", fmt.Sprintf(
					"%s storage is %.0f below the %.0f required for expansion level %d",
					status.Key, expansionStatus.CapacityShortfall, status.Required+status.Reserve, nextLevel,
				))
			}
		}
		if !status.Sufficient {
			addExpansionBlocker(&result, "insufficient_resources", fmt.Sprintf(
				"%s is %.0f below the expansion cost and configured reserve", status.Key, status.Shortfall,
			))
		}
		result.Costs = append(result.Costs, expansionStatus)
	}

	capacityNeeds := expansionCapacityNeeds(result.Costs)
	capacityBlocked := expansionCapacityBlocked(result.Costs)
	if capacityBlocked && len(capacityNeeds) > 0 {
		result.PendingStorageBuild, err = expansionPendingStorageBuild(castle, gameData, capacityNeeds)
		if err != nil {
			return ExpansionPreviewResult{}, err
		}
		result.StorageBuildingCandidates, err = expansionStorageBuildingCandidates(state, gameData, castle, request, capacityNeeds)
		if err != nil {
			return ExpansionPreviewResult{}, err
		}
		result.StorageItemCandidates, err = expansionStorageItemCandidates(state, gameData, castle, capacityNeeds)
		if err != nil {
			return ExpansionPreviewResult{}, err
		}
		result.RecommendedAction = recommendExpansionCapacityAction(castle.ID, request, result.StorageBuildingCandidates, result.StorageItemCandidates, capacityNeeds)
		if result.PendingStorageBuild != nil {
			result.TimeSkipOptions = expansionTimeSkipOptionsForSeconds(
				state, gameData, int(result.PendingStorageBuild.EstimatedRemainingSec),
			)
			result.RecommendedAction = recommendPendingStorageBuildAction(castle.ID, request, result.PendingStorageBuild, result.TimeSkipOptions)
		} else if result.RecommendedAction == nil {
			fundingNeeds := expansionStorageFundingNeeds(result.StorageBuildingCandidates, capacityNeeds)
			if len(fundingNeeds) > 0 {
				result.TransportCandidates = expansionTransportCandidates(state, castle, fundingNeeds, request.SourceResourceReserves)
				result.PendingTransport = expansionPendingTransport(state, castle.KingdomID)
				result.TimeSkipOptions = expansionTimeSkipOptions(state, gameData, result.PendingTransport)
				result.RecommendedAction = recommendExpansionTransportAction(state, castle, request, result)
			}
		}
	}

	resourceNeeds := expansionResourceNeeds(result.Costs)
	if !capacityBlocked && len(resourceNeeds) > 0 {
		result.TransportCandidates = expansionTransportCandidates(state, castle, resourceNeeds, request.SourceResourceReserves)
		result.PendingTransport = expansionPendingTransport(state, castle.KingdomID)
		result.TimeSkipOptions = expansionTimeSkipOptions(state, gameData, result.PendingTransport)
		result.RecommendedAction = recommendExpansionTransportAction(state, castle, request, result)
	}

	result.Ready = len(result.Blockers) == 0
	if result.Ready && request.X != nil {
		result.RecommendedAction = &ExpansionAction{
			Kind: "expand", Intent: "building.expand", Reason: fmt.Sprintf("Expansion level %d is affordable and fits current storage", nextLevel),
			Arguments: map[string]any{
				"castleId": castle.ID, "x": *request.X, "y": *request.Y, "direction": *request.Direction,
				"payment": payment, "resourceReserves": cloneFloatMap(request.ResourceReserves), "allowPremium": request.AllowPremium,
			},
		}
	}
	if !castle.Focused || castle.Layout.ObservedAt.IsZero() || (capacityBlocked && len(capacityNeeds) == 0) {
		result.RecommendedAction = expansionRefreshAction(castle.ID, "Refresh the castle layout, resource balances, capacities, and construction slots")
	}
	return result, nil
}

func normalizeExpansionPayment(payment string) (string, error) {
	payment = strings.ToLower(strings.TrimSpace(payment))
	if payment == "" {
		payment = ExpansionPaymentResources
	}
	if payment != ExpansionPaymentResources && payment != ExpansionPaymentPremium {
		return "", fmt.Errorf("payment must be %q or %q", ExpansionPaymentResources, ExpansionPaymentPremium)
	}
	return payment, nil
}

func validateExpansionPosition(request ExpansionPreviewRequest) error {
	provided := 0
	for _, value := range []*int{request.X, request.Y, request.Direction} {
		if value != nil {
			provided++
		}
	}
	if provided != 0 && provided != 3 {
		return fmt.Errorf("x, y, and direction must be provided together")
	}
	if provided == 3 && (*request.X < 0 || *request.Y < 0 || *request.Direction < 0 || *request.Direction > 3) {
		return fmt.Errorf("expansion coordinates must be non-negative and direction must be 0 through 3")
	}
	return nil
}

func expansionCostsForPayment(costs []GameData.BuildingCost, payment string) []GameData.BuildingCost {
	result := make([]GameData.BuildingCost, 0, len(costs))
	for _, cost := range costs {
		if (payment == ExpansionPaymentPremium) == cost.Premium {
			result = append(result, cost)
		}
	}
	return result
}

func expansionStorageMetric(key string) string {
	switch strings.ToUpper(strings.TrimSpace(key)) {
	case "W":
		return "woodStorage"
	case "S":
		return "stoneStorage"
	case "F":
		return "foodStorage"
	case "C":
		return "coalStorage"
	case "O":
		return "oilStorage"
	case "G":
		return "glassStorage"
	case "I":
		return "ironStorage"
	case "A":
		return "aquamarineStorage"
	case "HONEY":
		return "honeyStorage"
	case "MEAD":
		return "meadStorage"
	default:
		return ""
	}
}

func expansionCapacityNeeds(costs []ExpansionCostStatus) map[string]float64 {
	needs := map[string]float64{}
	for _, cost := range costs {
		if cost.CapacityShortfall <= 0 || cost.StorageMetric == "" {
			continue
		}
		needs[cost.StorageMetric] = math.Max(needs[cost.StorageMetric], cost.CapacityShortfall)
	}
	return needs
}

func expansionCapacityBlocked(costs []ExpansionCostStatus) bool {
	for _, cost := range costs {
		if cost.Scope == GameData.BuildingCostCastleResource && !cost.CapacitySufficient {
			return true
		}
	}
	return false
}

func expansionResourceNeeds(costs []ExpansionCostStatus) []ExpansionTransportGood {
	needs := make([]ExpansionTransportGood, 0)
	for _, cost := range costs {
		if cost.Scope != GameData.BuildingCostCastleResource || cost.DefinitionID <= 0 || cost.Shortfall <= 0 {
			continue
		}
		needs = append(needs, ExpansionTransportGood{
			ResourceID: State.ResourceID(cost.DefinitionID), Key: cost.Key, Amount: int64(math.Ceil(cost.Shortfall)),
		})
	}
	sort.Slice(needs, func(left, right int) bool { return needs[left].ResourceID < needs[right].ResourceID })
	return needs
}

func expansionStorageBuildingCandidates(
	state State.GameState,
	gameData *GameData.Store,
	castle State.CastleState,
	request ExpansionPreviewRequest,
	needs map[string]float64,
) ([]Candidate, error) {
	objectives := make([]Objective, 0, len(needs))
	minimums := map[string]float64{}
	current := map[string]float64{}
	if catalog, err := gameData.BuildingCatalog(); err == nil {
		current = currentBuildingValues(castle, catalog)
	}
	for metric, shortfall := range needs {
		objectives = append(objectives, Objective{Metric: metric, Weight: 1})
		minimums[metric] = metricValue(current, metric) + shortfall
	}
	sort.Slice(objectives, func(left, right int) bool { return objectives[left].Metric < objectives[right].Metric })
	preview, err := Preview(state, gameData, PreviewRequest{
		CastleID: castle.ID, Profile: "custom", Objectives: objectives,
		Constraints: Constraints{
			AllowPremium: request.AllowPremium, MinimumValues: minimums,
			ResourceReserves: cloneFloatMap(request.ResourceReserves),
		},
		IncludeBlocked: true, MaxCandidates: 500,
	})
	if err != nil {
		return nil, err
	}
	result := make([]Candidate, 0)
	for _, candidate := range preview.Candidates {
		useful := false
		for metric := range needs {
			if metricValue(candidate.DeltaValues, metric) > 0 {
				useful = true
				break
			}
		}
		if useful {
			result = append(result, candidate)
		}
	}
	return result, nil
}

func expansionStorageItemCandidates(
	state State.GameState,
	gameData *GameData.Store,
	castle State.CastleState,
	needs map[string]float64,
) ([]ExpansionConstructionItemCandidate, error) {
	items, err := gameData.Catalog("constructionItems")
	if err != nil {
		return nil, err
	}
	buildings, err := gameData.BuildingCatalog()
	if err != nil {
		return nil, err
	}
	result := make([]ExpansionConstructionItemCandidate, 0)
	for _, raw := range items.Rows() {
		record, decodeErr := GameData.DecodeRecord(raw)
		if decodeErr != nil || GameData.ConstructionItemIsTemporary(record) {
			continue
		}
		id, _ := record.Int64("constructionItemID")
		groupID, _ := record.Int64("constructionItemGroupID")
		level, _ := record.Int64("level")
		wood, _ := record.Float64("woodStorage")
		stone, _ := record.Float64("stoneStorage")
		gain := map[string]float64{}
		if wood > 0 && needs["woodStorage"] > 0 {
			gain["woodStorage"] = wood
		}
		if stone > 0 && needs["stoneStorage"] > 0 {
			gain["stoneStorage"] = stone
		}
		if id <= 0 || groupID <= 0 || len(gain) == 0 {
			continue
		}
		name, _ := record.String("_display_name")
		if name == "" {
			name, _ = record.String("name")
		}
		candidate := ExpansionConstructionItemCandidate{
			ConstructionItemID: State.ConstructionItemID(id), GroupID: groupID, Level: level,
			DisplayName: name, InventoryCount: state.Inventory.ConstructionItems[State.ConstructionItemID(id)],
			CapacityGain: gain, Blockers: []Blocker{},
		}
		if state.Inventory.ConstructionItemsObservedAt.IsZero() {
			candidate.Blockers = append(candidate.Blockers, Blocker{Code: "construction_inventory_unobserved", Message: "Construction-item inventory has not been observed"})
		} else if candidate.InventoryCount <= 0 {
			candidate.Blockers = append(candidate.Blockers, Blocker{Code: "construction_item_unavailable", Message: "The storage item is not in construction-item inventory"})
		}
		compatible, available := expansionConstructionHosts(castle, buildings, groupID)
		if len(available) > 0 {
			candidate.BuildingInstanceID = available[0].InstanceID
			candidate.BuildingDefinitionID = available[0].DefinitionID
		} else if len(compatible) > 0 {
			candidate.Blockers = append(candidate.Blockers, Blocker{Code: "construction_slot_occupied", Message: "Every compatible building already has a construction item equipped"})
		} else {
			candidate.Blockers = append(candidate.Blockers, Blocker{Code: "construction_host_missing", Message: "The castle has no compatible building for this storage item"})
		}
		if castle.ConstructionSlotsObservedAt.IsZero() {
			candidate.Blockers = append(candidate.Blockers, Blocker{Code: "construction_slots_unobserved", Message: "Construction-item slots have not been observed for this castle"})
		}
		candidate.ClosesCapacityGap = expansionGainClosesNeeds(gain, needs)
		candidate.Eligible = len(candidate.Blockers) == 0
		result = append(result, candidate)
	}
	sort.SliceStable(result, func(left, right int) bool {
		if result[left].Eligible != result[right].Eligible {
			return result[left].Eligible
		}
		if result[left].ClosesCapacityGap != result[right].ClosesCapacityGap {
			return result[left].ClosesCapacityGap
		}
		leftGain := expansionCoveredCapacity(result[left].CapacityGain, needs)
		rightGain := expansionCoveredCapacity(result[right].CapacityGain, needs)
		if leftGain != rightGain {
			return leftGain > rightGain
		}
		return result[left].ConstructionItemID < result[right].ConstructionItemID
	})
	return result, nil
}

func expansionPendingStorageBuild(
	castle State.CastleState,
	gameData *GameData.Store,
	needs map[string]float64,
) (*ExpansionPendingStorageBuild, error) {
	catalog, err := gameData.BuildingCatalog()
	if err != nil {
		return nil, err
	}
	var best *ExpansionPendingStorageBuild
	for _, slot := range castle.BuildingQueue.Slots {
		if slot.Status != State.BuildingQueueSlotOccupied || slot.BuildingID <= 0 {
			continue
		}
		building, found := expansionBuildingByID(castle, slot.BuildingID)
		if !found {
			continue
		}
		current, found := catalog.Definition(int64(building.DefinitionID))
		if !found {
			continue
		}
		target := current
		gain := map[string]float64{}
		switch building.ConstructionState {
		case State.BuildingStateBuildStopped, State.BuildingStateBuildInProgress:
			for metric := range needs {
				gain[metric] = metricValue(target.Values, metric)
			}
		case State.BuildingStateUpgradeStopped, State.BuildingStateUpgradeInProgress:
			var targetFound bool
			target, targetFound = catalog.Definition(current.UpgradeDefinitionID)
			if !targetFound || current.UpgradeDefinitionID <= 0 {
				continue
			}
			for metric := range needs {
				gain[metric] = metricValue(target.Values, metric) - metricValue(current.Values, metric)
			}
		default:
			continue
		}
		covered := expansionCoveredCapacity(gain, needs)
		if covered <= 0 {
			continue
		}
		remaining := target.DurationSec - building.ProgressSec
		if remaining < 0 {
			remaining = 0
		}
		candidate := &ExpansionPendingStorageBuild{
			BuildingInstanceID: building.InstanceID, CurrentDefinitionID: building.DefinitionID,
			TargetDefinitionID: State.BuildingID(target.ID), ProgressSec: building.ProgressSec,
			EstimatedRemainingSec: remaining, CapacityGain: gain, ClosesCapacityGap: expansionGainClosesNeeds(gain, needs),
		}
		if best == nil || (candidate.ClosesCapacityGap && !best.ClosesCapacityGap) ||
			(candidate.ClosesCapacityGap == best.ClosesCapacityGap &&
				expansionCoveredCapacity(candidate.CapacityGain, needs) > expansionCoveredCapacity(best.CapacityGain, needs)) {
			best = candidate
		}
	}
	return best, nil
}

func expansionBuildingByID(castle State.CastleState, buildingID State.BuildingInstanceID) (State.Building, bool) {
	for _, buildings := range []map[State.BuildingInstanceID]State.Building{
		castle.Layout.Objects, castle.Layout.Fixed, castle.Layout.Ground, castle.Buildings,
	} {
		if building, found := buildings[buildingID]; found {
			return building, true
		}
	}
	return State.Building{}, false
}

func expansionConstructionHosts(
	castle State.CastleState,
	catalog *GameData.BuildingCatalog,
	groupID int64,
) ([]State.Building, []State.Building) {
	compatible := make([]State.Building, 0)
	available := make([]State.Building, 0)
	for _, building := range castle.Buildings {
		definition, found := catalog.Definition(int64(building.DefinitionID))
		if !found || !containsInt64(definition.ConstructionItemGroupIDs, groupID) {
			continue
		}
		compatible = append(compatible, building)
		if len(castle.ConstructionSlots[building.InstanceID]) == 0 {
			available = append(available, building)
		}
	}
	sort.Slice(compatible, func(left, right int) bool { return compatible[left].InstanceID < compatible[right].InstanceID })
	sort.Slice(available, func(left, right int) bool { return available[left].InstanceID < available[right].InstanceID })
	return compatible, available
}

func expansionGainClosesNeeds(gain map[string]float64, needs map[string]float64) bool {
	for metric, need := range needs {
		if gain[metric] < need {
			return false
		}
	}
	return true
}

func expansionCoveredCapacity(gain map[string]float64, needs map[string]float64) float64 {
	total := 0.0
	for metric, need := range needs {
		total += math.Min(gain[metric], need)
	}
	return total
}

func recommendExpansionCapacityAction(
	castleID State.CastleID,
	request ExpansionPreviewRequest,
	buildings []Candidate,
	items []ExpansionConstructionItemCandidate,
	needs map[string]float64,
) *ExpansionAction {
	type option struct {
		action   ExpansionAction
		closes   bool
		covered  float64
		duration int64
	}
	options := make([]option, 0)
	for _, candidate := range buildings {
		if !candidate.Eligible {
			continue
		}
		gain := map[string]float64{}
		for metric := range needs {
			gain[metric] = metricValue(candidate.DeltaValues, metric)
		}
		arguments := map[string]any{
			"castleId": castleID, "resourceReserves": cloneFloatMap(request.ResourceReserves), "allowPremium": request.AllowPremium,
		}
		intent := "building.upgrade"
		kind := "upgrade_storage"
		if candidate.Kind == ActionUpgrade {
			arguments["buildingInstanceId"] = candidate.BuildingID
		} else if candidate.Kind == ActionConstruct && candidate.Placement != nil {
			intent = "building.construct"
			kind = "construct_storage"
			arguments["definitionId"] = candidate.Definition.ID
			arguments["x"] = candidate.Placement.GridX
			arguments["y"] = candidate.Placement.GridY
			arguments["rotation"] = candidate.Placement.Rotation
		} else {
			continue
		}
		options = append(options, option{
			action: ExpansionAction{Kind: kind, Intent: intent, Arguments: arguments, Reason: fmt.Sprintf("Increase storage with %s", candidate.Definition.DisplayName)},
			closes: expansionGainClosesNeeds(gain, needs), covered: expansionCoveredCapacity(gain, needs), duration: candidate.DurationSec,
		})
	}
	for _, candidate := range items {
		if !candidate.Eligible {
			continue
		}
		options = append(options, option{
			action: ExpansionAction{
				Kind: "equip_storage_item", Intent: "construction.equip",
				Arguments: map[string]any{
					"castleId": castleID, "buildingInstanceId": candidate.BuildingInstanceID,
					"constructionItemId": candidate.ConstructionItemID, "slot": 0, "mode": 0,
				},
				Reason: fmt.Sprintf("Equip %s to increase storage immediately", candidate.DisplayName),
			},
			closes: candidate.ClosesCapacityGap, covered: expansionCoveredCapacity(candidate.CapacityGain, needs),
		})
	}
	if len(options) == 0 {
		return nil
	}
	sort.SliceStable(options, func(left, right int) bool {
		if options[left].closes != options[right].closes {
			return options[left].closes
		}
		if options[left].covered != options[right].covered {
			return options[left].covered > options[right].covered
		}
		if options[left].duration != options[right].duration {
			return options[left].duration < options[right].duration
		}
		return options[left].action.Intent < options[right].action.Intent
	})
	return &options[0].action
}

func expansionStorageFundingNeeds(candidates []Candidate, needs map[string]float64) []ExpansionTransportGood {
	type option struct {
		needs    []ExpansionTransportGood
		closes   bool
		covered  float64
		duration int64
	}
	options := make([]option, 0)
	for _, candidate := range candidates {
		if candidate.Eligible {
			continue
		}
		if candidate.Kind == ActionUpgrade && candidate.BuildingID <= 0 {
			continue
		}
		if candidate.Kind == ActionConstruct && candidate.Placement == nil {
			continue
		}
		actionable := len(candidate.Blockers) > 0
		for _, blocker := range candidate.Blockers {
			if blocker.Code != "insufficient_resources" {
				actionable = false
				break
			}
		}
		if !actionable {
			continue
		}
		funding := make([]ExpansionTransportGood, 0)
		for _, cost := range candidate.Costs {
			if cost.Shortfall <= 0 {
				continue
			}
			if cost.Scope != GameData.BuildingCostCastleResource || cost.DefinitionID <= 0 {
				actionable = false
				break
			}
			funding = append(funding, ExpansionTransportGood{
				ResourceID: State.ResourceID(cost.DefinitionID), Key: cost.Key, Amount: int64(math.Ceil(cost.Shortfall)),
			})
		}
		if !actionable || len(funding) == 0 {
			continue
		}
		sort.Slice(funding, func(left, right int) bool { return funding[left].ResourceID < funding[right].ResourceID })
		gain := map[string]float64{}
		for metric := range needs {
			gain[metric] = metricValue(candidate.DeltaValues, metric)
		}
		options = append(options, option{
			needs: funding, closes: expansionGainClosesNeeds(gain, needs),
			covered: expansionCoveredCapacity(gain, needs), duration: candidate.DurationSec,
		})
	}
	if len(options) == 0 {
		return nil
	}
	sort.SliceStable(options, func(left, right int) bool {
		if options[left].closes != options[right].closes {
			return options[left].closes
		}
		if options[left].covered != options[right].covered {
			return options[left].covered > options[right].covered
		}
		return options[left].duration < options[right].duration
	})
	return options[0].needs
}

func expansionPendingTransport(state State.GameState, kingdomID State.KingdomID) *State.KingdomResourceTransport {
	for _, transport := range state.KingdomTransport.Pending {
		if transport.KingdomID != kingdomID || transport.RemainingSec <= 0 {
			continue
		}
		copy := transport
		copy.Goods = append([]State.KingdomTransportGood(nil), transport.Goods...)
		return &copy
	}
	return nil
}

func expansionTransportCandidates(
	state State.GameState,
	target State.CastleState,
	needs []ExpansionTransportGood,
	reserves map[string]float64,
) []ExpansionTransportCandidate {
	unlock, unlockObserved := state.KingdomTransport.Unlocks[target.KingdomID]
	transportObserved := !state.KingdomTransport.ObservedAt.IsZero() && unlockObserved
	pending := false
	for _, transport := range state.KingdomTransport.Pending {
		if transport.KingdomID == target.KingdomID && transport.RemainingSec > 0 {
			pending = true
			break
		}
	}
	result := make([]ExpansionTransportCandidate, 0)
	for _, source := range state.Castles {
		if source.ID == target.ID || source.KingdomID == target.KingdomID {
			continue
		}
		candidate := ExpansionTransportCandidate{
			SourceCastleID: source.ID, SourceKingdomID: source.KingdomID,
			TargetCastleID: target.ID, TargetKingdomID: target.KingdomID,
			Goods: []ExpansionTransportGood{}, Blockers: []Blocker{},
		}
		for _, need := range needs {
			reserve := reserveValue(reserves, GameData.BuildingCost{
				Key: need.Key, Scope: GameData.BuildingCostCastleResource, DefinitionID: int64(need.ResourceID),
			})
			available := int64(math.Floor(math.Max(0, source.Resources[need.ResourceID].Amount-reserve)))
			requiredShipment := expansionShipmentAmount(need.Amount)
			if capacity := target.Resources[need.ResourceID].Capacity; capacity != nil {
				freeCapacity := math.Max(0, *capacity-target.Resources[need.ResourceID].Amount)
				requiredShipment = min(
					requiredShipment,
					int64(math.Floor(freeCapacity/kingdomTransportDeliveryFactor)),
				)
			}
			amount := min(available, requiredShipment)
			delivered := int64(math.Round(float64(amount) * kingdomTransportDeliveryFactor))
			candidate.RemainingSurplus += float64(max(available-amount, 0))
			if amount <= 0 {
				continue
			}
			good := need
			good.Amount = amount
			candidate.Goods = append(candidate.Goods, good)
			candidate.Coverage += math.Min(float64(delivered), float64(need.Amount)) / float64(need.Amount)
		}
		candidate.Complete = len(candidate.Goods) == len(needs)
		if candidate.Complete {
			for index, good := range candidate.Goods {
				delivered := int64(math.Round(float64(good.Amount) * kingdomTransportDeliveryFactor))
				if delivered < needs[index].Amount {
					candidate.Complete = false
					break
				}
			}
		}
		if len(candidate.Goods) == 0 {
			candidate.Blockers = append(candidate.Blockers, Blocker{Code: "source_resources", Message: "The source has none of the required resources"})
		}
		if !transportObserved {
			candidate.Blockers = append(candidate.Blockers, Blocker{Code: "transport_unobserved", Message: "Kingdom transport state has not been observed"})
		} else if !unlock.Unlocked {
			candidate.Blockers = append(candidate.Blockers, Blocker{Code: "transport_locked", Message: "Transport to the target kingdom is locked"})
		}
		if pending {
			candidate.Blockers = append(candidate.Blockers, Blocker{Code: "transport_pending", Message: "The target kingdom already has a pending resource transport"})
		}
		candidate.Eligible = len(candidate.Blockers) == 0
		result = append(result, candidate)
	}
	sort.SliceStable(result, func(left, right int) bool {
		if result[left].Eligible != result[right].Eligible {
			return result[left].Eligible
		}
		if result[left].Complete != result[right].Complete {
			return result[left].Complete
		}
		if result[left].Coverage != result[right].Coverage {
			return result[left].Coverage > result[right].Coverage
		}
		if result[left].RemainingSurplus != result[right].RemainingSurplus {
			return result[left].RemainingSurplus > result[right].RemainingSurplus
		}
		return result[left].SourceCastleID < result[right].SourceCastleID
	})
	return result
}

func expansionShipmentAmount(deliveryAmount int64) int64 {
	if deliveryAmount <= 0 {
		return 0
	}
	return int64(math.Ceil(float64(deliveryAmount) / kingdomTransportDeliveryFactor))
}

func expansionTimeSkipOptions(
	state State.GameState,
	gameData *GameData.Store,
	pending *State.KingdomResourceTransport,
) []ExpansionTimeSkipOption {
	if pending == nil || gameData == nil {
		return []ExpansionTimeSkipOption{}
	}
	return expansionTimeSkipOptionsForSeconds(state, gameData, pending.RemainingSec)
}

func expansionTimeSkipOptionsForSeconds(
	state State.GameState,
	gameData *GameData.Store,
	remainingSec int,
) []ExpansionTimeSkipOption {
	if gameData == nil || remainingSec <= 0 {
		return []ExpansionTimeSkipOption{}
	}
	values, err := gameData.Catalog("currencyMinutesSkipValues")
	if err != nil {
		return []ExpansionTimeSkipOption{}
	}
	currencies, err := gameData.Catalog("currencies")
	if err != nil {
		return []ExpansionTimeSkipOption{}
	}
	result := make([]ExpansionTimeSkipOption, 0)
	for _, raw := range values.Rows() {
		record, decodeErr := GameData.DecodeRecord(raw)
		if decodeErr != nil {
			continue
		}
		currencyID, _ := record.Int64("currencyID")
		minutes, _ := record.Int64("MinutesSkipValue")
		balance := state.Player.Currencies[State.CurrencyID(currencyID)]
		if currencyID <= 0 || minutes <= 0 || balance < 1 {
			continue
		}
		currencyRaw, found := currencies.Find(strconv.FormatInt(currencyID, 10))
		if !found {
			continue
		}
		currency, decodeErr := GameData.DecodeRecord(currencyRaw)
		if decodeErr != nil {
			continue
		}
		key, _ := currency.String("JSONKey")
		key = strings.ToUpper(key)
		if !supportedExpansionTimeSkip(key, int(minutes)) {
			continue
		}
		result = append(result, ExpansionTimeSkipOption{
			CurrencyID: State.CurrencyID(currencyID), TimeSkipID: strings.ToUpper(key), Minutes: int(minutes), Balance: balance,
		})
	}
	sort.Slice(result, func(left, right int) bool {
		leftFits := result[left].Minutes*60 <= remainingSec
		rightFits := result[right].Minutes*60 <= remainingSec
		if leftFits != rightFits {
			return leftFits
		}
		if leftFits {
			return result[left].Minutes > result[right].Minutes
		}
		return result[left].Minutes < result[right].Minutes
	})
	return result
}

func supportedExpansionTimeSkip(key string, minutes int) bool {
	return map[string]int{
		"MS1": 1, "MS2": 5, "MS3": 10, "MS4": 30, "MS5": 60, "MS6": 300, "MS7": 1440,
	}[key] == minutes
}

func recommendPendingStorageBuildAction(
	castleID State.CastleID,
	request ExpansionPreviewRequest,
	pending *ExpansionPendingStorageBuild,
	options []ExpansionTimeSkipOption,
) *ExpansionAction {
	if pending == nil || !request.AllowTimeSkips || len(options) == 0 {
		return nil
	}
	option := options[0]
	return &ExpansionAction{
		Kind: "skip_storage_build", Intent: "building.skip_time",
		Arguments: map[string]any{
			"castleId": castleID, "buildingInstanceId": pending.BuildingInstanceID,
			"minutes": option.Minutes, "minimumRemaining": 0,
		},
		Reason: fmt.Sprintf("Advance the queued storage upgrade with the best available %d-minute skip", option.Minutes),
	}
}

func recommendExpansionTransportAction(
	state State.GameState,
	castle State.CastleState,
	request ExpansionPreviewRequest,
	result ExpansionPreviewResult,
) *ExpansionAction {
	unlock, observed := state.KingdomTransport.Unlocks[castle.KingdomID]
	if state.KingdomTransport.ObservedAt.IsZero() || !observed {
		return &ExpansionAction{
			Kind: "refresh_transport", Intent: "resource.logistics.refresh", Arguments: map[string]any{},
			Reason: "Refresh kingdom transport unlocks and pending shipments before moving expansion resources",
		}
	}
	if !unlock.Unlocked {
		return nil
	}
	if result.PendingTransport != nil {
		if request.AllowTimeSkips && len(result.TimeSkipOptions) > 0 {
			option := result.TimeSkipOptions[0]
			return &ExpansionAction{
				Kind: "skip_transport", Intent: "resource.kingdom.skip",
				Arguments: map[string]any{"targetKingdomId": castle.KingdomID, "timeSkipId": option.TimeSkipID},
				Reason:    fmt.Sprintf("Apply the best available %d-minute skip to the pending resource transport", option.Minutes),
			}
		}
		return nil
	}
	for _, candidate := range result.TransportCandidates {
		if !candidate.Eligible || len(candidate.Goods) == 0 {
			continue
		}
		goods := make([]map[string]any, 0, len(candidate.Goods))
		for _, good := range candidate.Goods {
			goods = append(goods, map[string]any{"resourceId": good.ResourceID, "amount": good.Amount})
		}
		return &ExpansionAction{
			Kind: "transport_resources", Intent: "resource.ship",
			Arguments: map[string]any{
				"sourceCastleId": candidate.SourceCastleID, "targetCastleId": castle.ID,
				"goods": goods,
			},
			Reason: "Transport the largest useful batch of missing expansion resources from an owned castle",
		}
	}
	return nil
}

func expansionRefreshAction(castleID State.CastleID, reason string) *ExpansionAction {
	return &ExpansionAction{
		Kind: "refresh_castle", Intent: "building.refresh", Arguments: map[string]any{"castleId": castleID}, Reason: reason,
	}
}

func addExpansionBlocker(result *ExpansionPreviewResult, code string, message string) {
	for _, blocker := range result.Blockers {
		if blocker.Code == code && blocker.Message == message {
			return
		}
	}
	result.Blockers = append(result.Blockers, Blocker{Code: code, Message: message})
}
