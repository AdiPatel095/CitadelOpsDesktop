package Buildings

import (
	"fmt"
	"math"

	"CitadelDesktop/Server/GameData"
	"CitadelDesktop/Server/State"
)

type StorageDependencyRequest struct {
	ExpectedRevision             *uint64            `json:"expectedRevision,omitempty"`
	CastleID                     State.CastleID     `json:"castleId"`
	Costs                        []CostStatus       `json:"costs"`
	ResourceReserves             map[string]float64 `json:"resourceReserves,omitempty"`
	AllowPremium                 bool               `json:"allowPremium,omitempty"`
	AllowResourceTransport       bool               `json:"allowResourceTransport,omitempty"`
	AllowTimeSkips               bool               `json:"allowTimeSkips,omitempty"`
	AllowedBuildingDefinitionIDs []State.BuildingID `json:"allowedBuildingDefinitionIds,omitempty"`
}

type StorageDependencyResult struct {
	Revision                  uint64                               `json:"revision"`
	CastleID                  State.CastleID                       `json:"castleId"`
	Required                  bool                                 `json:"required"`
	CapacityNeeds             map[string]float64                   `json:"capacityNeeds"`
	StorageBuildingCandidates []Candidate                          `json:"storageBuildingCandidates"`
	StorageItemCandidates     []ExpansionConstructionItemCandidate `json:"storageItemCandidates"`
	TransportCandidates       []ExpansionTransportCandidate        `json:"transportCandidates"`
	PendingTransport          *State.KingdomResourceTransport      `json:"pendingTransport,omitempty"`
	PendingStorageBuild       *ExpansionPendingStorageBuild        `json:"pendingStorageBuild,omitempty"`
	TimeSkipOptions           []ExpansionTimeSkipOption            `json:"timeSkipOptions"`
	RecommendedAction         *ExpansionAction                     `json:"recommendedAction,omitempty"`
	Blockers                  []Blocker                            `json:"blockers"`
}

func PreviewStorageDependency(
	state State.GameState,
	gameData *GameData.Store,
	request StorageDependencyRequest,
) (StorageDependencyResult, error) {
	if request.ExpectedRevision != nil && *request.ExpectedRevision != state.Revision {
		return StorageDependencyResult{}, RevisionMismatchError{Expected: *request.ExpectedRevision, Actual: state.Revision}
	}
	if gameData == nil {
		return StorageDependencyResult{}, fmt.Errorf("official game data is unavailable")
	}
	castle, found := state.Castles[request.CastleID]
	if !found || request.CastleID <= 0 {
		return StorageDependencyResult{}, fmt.Errorf("castle %d was not found", request.CastleID)
	}
	result := StorageDependencyResult{
		Revision: state.Revision, CastleID: castle.ID, CapacityNeeds: map[string]float64{},
		StorageBuildingCandidates: []Candidate{}, StorageItemCandidates: []ExpansionConstructionItemCandidate{},
		TransportCandidates: []ExpansionTransportCandidate{}, TimeSkipOptions: []ExpansionTimeSkipOption{},
		Blockers: []Blocker{},
	}
	needsRefresh := false
	for _, cost := range request.Costs {
		if cost.Scope != GameData.BuildingCostCastleResource || cost.DefinitionID <= 0 || cost.Required <= 0 {
			continue
		}
		required := cost.Required + cost.Reserve
		balance, observed := castle.Resources[State.ResourceID(cost.DefinitionID)]
		if !observed || balance.Capacity == nil {
			result.Required = true
			needsRefresh = true
			addStorageDependencyBlocker(&result, "capacity_unobserved", fmt.Sprintf("Storage capacity for %s has not been observed", cost.Key))
			continue
		}
		if *balance.Capacity >= required {
			continue
		}
		result.Required = true
		metric := expansionStorageMetric(cost.Key)
		if metric == "" {
			addStorageDependencyBlocker(&result, "storage_metric_unavailable", fmt.Sprintf(
				"No storage dependency metric is known for %s, whose capacity is %.0f below the required %.0f",
				cost.Key, required-*balance.Capacity, required,
			))
			continue
		}
		shortfall := required - *balance.Capacity
		result.CapacityNeeds[metric] = math.Max(result.CapacityNeeds[metric], shortfall)
		addStorageDependencyBlocker(&result, "storage_capacity", fmt.Sprintf(
			"%s storage is %.0f below the %.0f required for the next target action", cost.Key, shortfall, required,
		))
	}
	if !result.Required {
		return result, nil
	}
	if needsRefresh || !castle.Focused || castle.Layout.ObservedAt.IsZero() {
		result.RecommendedAction = expansionRefreshAction(castle.ID, "Refresh the castle layout, resource balances, capacities, and construction slots")
		return result, nil
	}
	if len(result.CapacityNeeds) == 0 {
		return result, nil
	}

	expansionRequest := ExpansionPreviewRequest{
		CastleID: castle.ID, Payment: ExpansionPaymentResources,
		ResourceReserves: request.ResourceReserves, AllowPremium: request.AllowPremium,
		AllowTimeSkips: request.AllowTimeSkips,
	}
	var err error
	result.PendingStorageBuild, err = expansionPendingStorageBuild(castle, gameData, result.CapacityNeeds)
	if err != nil {
		return StorageDependencyResult{}, err
	}
	result.StorageBuildingCandidates, err = expansionStorageBuildingCandidates(state, gameData, castle, expansionRequest, result.CapacityNeeds)
	if err != nil {
		return StorageDependencyResult{}, err
	}
	result.StorageBuildingCandidates = filterStorageDependencyBuildings(
		result.StorageBuildingCandidates, request.AllowedBuildingDefinitionIDs,
	)
	result.StorageItemCandidates, err = expansionStorageItemCandidates(state, gameData, castle, result.CapacityNeeds)
	if err != nil {
		return StorageDependencyResult{}, err
	}
	result.RecommendedAction = recommendExpansionCapacityAction(
		castle.ID, expansionRequest, result.StorageBuildingCandidates, result.StorageItemCandidates, result.CapacityNeeds,
	)
	if result.PendingStorageBuild != nil {
		result.TimeSkipOptions = expansionTimeSkipOptionsForSeconds(
			state, gameData, int(result.PendingStorageBuild.EstimatedRemainingSec),
		)
		result.RecommendedAction = recommendPendingStorageBuildAction(
			castle.ID, expansionRequest, result.PendingStorageBuild, result.TimeSkipOptions,
		)
		return result, nil
	}
	if result.RecommendedAction != nil {
		return result, nil
	}

	fundingNeeds := expansionStorageFundingNeeds(result.StorageBuildingCandidates, result.CapacityNeeds)
	if len(fundingNeeds) > 0 && request.AllowResourceTransport {
		result.PendingTransport = expansionPendingTransport(state, castle.KingdomID)
		result.TimeSkipOptions = expansionTimeSkipOptions(state, gameData, result.PendingTransport)
		result.TransportCandidates = expansionTransportCandidates(state, castle, fundingNeeds, request.ResourceReserves)
		preview := ExpansionPreviewResult{
			TransportCandidates: result.TransportCandidates,
			PendingTransport:    result.PendingTransport,
			TimeSkipOptions:     result.TimeSkipOptions,
		}
		result.RecommendedAction = recommendExpansionTransportAction(state, castle, expansionRequest, preview)
	}
	if len(result.StorageBuildingCandidates) == 0 && len(result.StorageItemCandidates) == 0 {
		addStorageDependencyBlocker(&result, "target_storage_path_unavailable", "The captured target has no remaining storage-building step that can satisfy this capacity dependency")
	} else if len(fundingNeeds) > 0 && !request.AllowResourceTransport {
		addStorageDependencyBlocker(&result, "storage_resources_pending", "The required target-safe storage action is waiting for resources and resource transport is disabled")
	}
	return result, nil
}

func filterStorageDependencyBuildings(candidates []Candidate, allowed []State.BuildingID) []Candidate {
	if allowed == nil {
		return candidates
	}
	allowedSet := make(map[int64]struct{}, len(allowed))
	for _, definitionID := range allowed {
		if definitionID > 0 {
			allowedSet[int64(definitionID)] = struct{}{}
		}
	}
	result := make([]Candidate, 0, len(candidates))
	for _, candidate := range candidates {
		if _, found := allowedSet[candidate.Definition.ID]; found {
			result = append(result, candidate)
		}
	}
	return result
}

func addStorageDependencyBlocker(result *StorageDependencyResult, code string, message string) {
	for _, blocker := range result.Blockers {
		if blocker.Code == code && blocker.Message == message {
			return
		}
	}
	result.Blockers = append(result.Blockers, Blocker{Code: code, Message: message})
}
