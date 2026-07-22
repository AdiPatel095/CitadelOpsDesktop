package API

import (
	"net/http"
	"sort"

	"CitadelDesktop/Server/GameData"
	"CitadelDesktop/Server/State"
)

type craftingProjection struct {
	Recipes        []GameData.CraftingRecipe            `json:"recipes"`
	Resources      map[string]GameData.CraftingResource `json:"resources"`
	Nodes          []craftingNode                       `json:"nodes"`
	ResearchLoaded bool                                 `json:"researchLoaded"`
}

type craftingNode struct {
	CastleID    State.CastleID     `json:"castleID"`
	Name        string             `json:"name"`
	Role        string             `json:"role"`
	KingdomID   State.KingdomID    `json:"kingdomID"`
	CanCraft    bool               `json:"canCraft"`
	StormBuffer bool               `json:"stormBuffer"`
	Resources   map[string]float64 `json:"resources"`
	Storage     map[string]float64 `json:"storage"`
	Market      *craftingMarket    `json:"market,omitempty"`
	Buildings   []craftingBuilding `json:"buildings"`
}

type craftingMarket struct {
	Loaded                    bool    `json:"loaded"`
	BaseBarrows               int     `json:"baseBarrows"`
	BuildItemBarrows          int     `json:"buildItemBarrows"`
	OtherBarrows              int     `json:"otherBarrows"`
	TotalBarrows              int     `json:"totalBarrows"`
	AvailableBarrows          int     `json:"availableBarrows"`
	BusyBarrows               int     `json:"busyBarrows"`
	CaravanLevel              int     `json:"caravanLevel"`
	CaravanBoostPercent       float64 `json:"caravanBoostPercent"`
	AreaCapacityBoostPercent  float64 `json:"areaCapacityBoostPercent"`
	CapacityBonus             float64 `json:"capacityBonus"`
	CapacityPerBarrow         int     `json:"capacityPerBarrow"`
	AvailableShipmentCapacity int     `json:"availableShipmentCapacity"`
}

type craftingBuilding struct {
	QueueTypeID        int                      `json:"queueTypeID"`
	Name               string                   `json:"name"`
	OID                State.BuildingInstanceID `json:"oid"`
	WID                State.BuildingID         `json:"wid"`
	ActiveCapacity     int                      `json:"activeCapacity"`
	QueueCapacity      int                      `json:"queueCapacity"`
	ActiveRecipes      []int64                  `json:"activeRecipes"`
	QueuedRecipes      []int64                  `json:"queuedRecipes"`
	AvailableRecipeIDs []int64                  `json:"availableRecipeIDs"`
}

func (server *Server) handleCraftingProjection(writer http.ResponseWriter, request *http.Request) {
	if server.config.GameData == nil || server.config.State == nil {
		writeError(writer, http.StatusServiceUnavailable, "crafting_unavailable", "Crafting data is unavailable")
		return
	}
	store, ready := server.config.GameData.Current()
	if !ready {
		writeError(writer, http.StatusServiceUnavailable, "crafting_unavailable", "Official game data is unavailable")
		return
	}
	catalog, err := server.config.GameData.CraftingCatalog()
	if err != nil {
		writeError(writer, http.StatusServiceUnavailable, "crafting_unavailable", err.Error())
		return
	}
	if assets, assetErr := server.config.GameData.CurrencyAssets(request.Context()); assetErr == nil {
		applyCraftingIconURLs(&catalog, assets.Icons)
	}
	snapshot := server.config.State.Snapshot()
	projection := craftingProjection{
		Recipes: catalog.Recipes, Resources: catalog.Resources,
		Nodes: make([]craftingNode, 0, len(snapshot.Castles)),
	}
	for _, castle := range snapshot.Castles {
		node := craftingNode{
			CastleID: castle.ID, Name: castle.Name, Role: craftingCastleRole(castle),
			KingdomID: castle.KingdomID, StormBuffer: castle.KingdomID == 4,
			Resources: map[string]float64{}, Storage: map[string]float64{},
			Buildings: []craftingBuilding{},
		}
		for resourceID, balance := range castle.Resources {
			key := catalog.ResourceByID[int64(resourceID)]
			if key == "" {
				continue
			}
			node.Resources[key] = balance.Amount
			if balance.Capacity != nil {
				node.Storage[key] = *balance.Capacity
			}
		}
		for _, building := range castle.Crafting.Buildings {
			item := craftingBuilding{
				QueueTypeID: building.QueueTypeID, Name: craftingQueueName(building.QueueTypeID),
				OID: building.InstanceID, WID: building.DefinitionID,
				ActiveCapacity:     1 + len(building.ActiveSlotRentals),
				QueueCapacity:      1 + len(building.QueueSlotRentals),
				ActiveRecipes:      craftingRecipeIDs(building.Active),
				QueuedRecipes:      craftingRecipeIDs(building.Queued),
				AvailableRecipeIDs: availableCraftingRecipes(catalog.Recipes, building, castle.Crafting),
			}
			node.Buildings = append(node.Buildings, item)
		}
		node.Market = craftingMarketProjection(store, snapshot, castle)
		sort.Slice(node.Buildings, func(left, right int) bool {
			return node.Buildings[left].QueueTypeID < node.Buildings[right].QueueTypeID
		})
		node.CanCraft = castle.SupportsSovereignCrafting() && len(node.Buildings) > 0
		if len(castle.Crafting.EnabledRecipeIDs) > 0 || len(castle.Crafting.EnabledRecipeGroupIDs) > 0 {
			projection.ResearchLoaded = true
		}
		projection.Nodes = append(projection.Nodes, node)
	}
	sort.Slice(projection.Nodes, func(left, right int) bool {
		if projection.Nodes[left].KingdomID != projection.Nodes[right].KingdomID {
			return projection.Nodes[left].KingdomID < projection.Nodes[right].KingdomID
		}
		return projection.Nodes[left].CastleID < projection.Nodes[right].CastleID
	})
	writeJSON(writer, http.StatusOK, projection)
}

func applyCraftingIconURLs(catalog *GameData.CraftingCatalog, icons map[string]string) {
	if catalog == nil || len(icons) == 0 {
		return
	}
	for key, resource := range catalog.Resources {
		if iconURL := icons[resource.AssetName]; iconURL != "" {
			resource.IconURL = iconURL
			catalog.Resources[key] = resource
		}
	}
	for index := range catalog.Recipes {
		resource, found := catalog.Resources[catalog.Recipes[index].Output.Key]
		if found && resource.IconURL != "" {
			catalog.Recipes[index].Output.IconURL = resource.IconURL
		}
	}
}

func craftingMarketProjection(store *GameData.Store, snapshot State.GameState, castle State.CastleState) *craftingMarket {
	marketBuildingID := State.BuildingInstanceID(0)
	baseBarrows := 0
	for instanceID, building := range castle.Buildings {
		barrows, err := store.MarketBarrowsForBuilding(int64(building.DefinitionID))
		if err == nil && barrows > baseBarrows {
			baseBarrows = barrows
			marketBuildingID = instanceID
		}
	}
	buildItemBarrows := 0
	for _, slot := range castle.ConstructionSlots[marketBuildingID] {
		barrows, err := store.MarketBarrowsForConstructionItem(int64(slot.DefinitionID))
		if err == nil {
			buildItemBarrows += barrows
		}
	}
	marketCastle, loaded := snapshot.Market.Castles[castle.ID]
	totalBarrows := marketCastle.TotalBarrows
	availableBarrows := marketCastle.AvailableBarrows
	if totalBarrows <= 0 {
		totalBarrows = baseBarrows + buildItemBarrows
	}
	if totalBarrows <= 0 {
		return nil
	}
	availableBarrows = max(0, min(availableBarrows, totalBarrows))
	effects := make([]GameData.MarketEffect, 0, len(marketCastle.AreaEffects))
	for _, effect := range marketCastle.AreaEffects {
		effects = append(effects, GameData.MarketEffect{EffectID: effect.EffectID, Values: effect.Values})
	}
	capacity, err := store.MarketCapacity(snapshot.Market.CaravanLevel, effects)
	if err != nil {
		return nil
	}
	otherBarrows := max(0, totalBarrows-baseBarrows-buildItemBarrows)
	return &craftingMarket{
		Loaded:      loaded && snapshot.Market.CaravanLevelLoaded,
		BaseBarrows: baseBarrows, BuildItemBarrows: buildItemBarrows, OtherBarrows: otherBarrows,
		TotalBarrows: totalBarrows, AvailableBarrows: availableBarrows, BusyBarrows: totalBarrows - availableBarrows,
		CaravanLevel: snapshot.Market.CaravanLevel, CaravanBoostPercent: capacity.CaravanBoostPercent,
		AreaCapacityBoostPercent: capacity.AreaCapacityBoostPercent, CapacityBonus: capacity.CapacityBonus,
		CapacityPerBarrow: capacity.CapacityPerBarrow, AvailableShipmentCapacity: availableBarrows * capacity.CapacityPerBarrow,
	}
}

func availableCraftingRecipes(recipes []GameData.CraftingRecipe, building State.CraftingBuilding, crafting State.CraftingState) []int64 {
	enabledRecipes := make(map[int64]struct{}, len(crafting.EnabledRecipeIDs))
	for _, id := range crafting.EnabledRecipeIDs {
		enabledRecipes[id] = struct{}{}
	}
	result := make([]int64, 0)
	for _, recipe := range recipes {
		if recipe.QueueTypeID != building.QueueTypeID || !buildingAllowed(recipe.RequiredBuildingWIDs, int64(building.DefinitionID)) {
			continue
		}
		if recipe.ResearchGroupID != 0 {
			_, recipeEnabled := enabledRecipes[recipe.RecipeID]
			if !recipeEnabled {
				continue
			}
		}
		result = append(result, recipe.RecipeID)
	}
	return result
}

func buildingAllowed(allowed []int64, buildingID int64) bool {
	if len(allowed) == 0 {
		return true
	}
	for _, candidate := range allowed {
		if candidate == buildingID {
			return true
		}
	}
	return false
}

func craftingRecipeIDs(items []State.CraftingQueueItem) []int64 {
	result := make([]int64, 0, len(items))
	for _, item := range items {
		result = append(result, item.RecipeID)
	}
	return result
}

func craftingQueueName(queueTypeID int) string {
	switch queueTypeID {
	case 1:
		return "Refinery"
	case 2:
		return "Toolsmith"
	case 3:
		return "Dragon Hoard"
	case 4:
		return "Dragon Forge"
	default:
		return "Crafting Queue"
	}
}

func craftingCastleRole(castle State.CastleState) string {
	if castle.Name != "" {
		return castle.Name
	}
	if castle.KingdomID == 0 {
		return "Great Empire Castle"
	}
	return "Kingdom Castle"
}
