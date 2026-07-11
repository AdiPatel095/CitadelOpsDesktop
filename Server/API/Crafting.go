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
	Buildings   []craftingBuilding `json:"buildings"`
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

func (server *Server) handleCraftingProjection(writer http.ResponseWriter, _ *http.Request) {
	if server.config.GameData == nil || server.config.State == nil {
		writeError(writer, http.StatusServiceUnavailable, "crafting_unavailable", "Crafting data is unavailable")
		return
	}
	catalog, err := server.config.GameData.CraftingCatalog()
	if err != nil {
		writeError(writer, http.StatusServiceUnavailable, "crafting_unavailable", err.Error())
		return
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
		sort.Slice(node.Buildings, func(left, right int) bool {
			return node.Buildings[left].QueueTypeID < node.Buildings[right].QueueTypeID
		})
		node.CanCraft = len(node.Buildings) > 0
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

func availableCraftingRecipes(recipes []GameData.CraftingRecipe, building State.CraftingBuilding, crafting State.CraftingState) []int64 {
	enabledRecipes := make(map[int64]struct{}, len(crafting.EnabledRecipeIDs))
	for _, id := range crafting.EnabledRecipeIDs {
		enabledRecipes[id] = struct{}{}
	}
	enabledGroups := make(map[int64]struct{}, len(crafting.EnabledRecipeGroupIDs))
	for _, id := range crafting.EnabledRecipeGroupIDs {
		enabledGroups[id] = struct{}{}
	}
	result := make([]int64, 0)
	for _, recipe := range recipes {
		if recipe.QueueTypeID != building.QueueTypeID || !buildingAllowed(recipe.RequiredBuildingWIDs, int64(building.DefinitionID)) {
			continue
		}
		if recipe.ResearchGroupID != 0 {
			_, recipeEnabled := enabledRecipes[recipe.RecipeID]
			_, groupEnabled := enabledGroups[recipe.RecipeGroupID]
			if !recipeEnabled && !groupEnabled {
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
