package Buildings

import (
	"testing"

	"CitadelDesktop/Server/GameData"
	"CitadelDesktop/Server/State"
)

func TestStorageDependencyUsesOnlyTargetSafeStorageUpgrade(t *testing.T) {
	gameData := expansionTestGameData(t)
	state := expansionTestState(8, 6800)
	result, err := PreviewStorageDependency(state, gameData, StorageDependencyRequest{
		CastleID: state.Castles[10].ID,
		Costs: []CostStatus{{
			Key: "W", Scope: GameData.BuildingCostCastleResource, DefinitionID: 3,
			Required: 7183, Known: true,
		}},
		AllowedBuildingDefinitionIDs: []State.BuildingID{134},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.Required || result.CapacityNeeds["woodStorage"] != 383 {
		t.Fatalf("storage dependency = %#v", result)
	}
	if result.RecommendedAction == nil || result.RecommendedAction.Intent != "building.upgrade" ||
		result.RecommendedAction.Arguments["buildingInstanceId"] != State.BuildingInstanceID(28) {
		t.Fatalf("recommended action = %#v", result.RecommendedAction)
	}

	blocked, err := PreviewStorageDependency(state, gameData, StorageDependencyRequest{
		CastleID: state.Castles[10].ID,
		Costs: []CostStatus{{
			Key: "W", Scope: GameData.BuildingCostCastleResource, DefinitionID: 3,
			Required: 7183, Known: true,
		}},
		AllowedBuildingDefinitionIDs: []State.BuildingID{999},
	})
	if err != nil {
		t.Fatal(err)
	}
	if blocked.RecommendedAction != nil || len(blocked.StorageBuildingCandidates) != 0 {
		t.Fatalf("unsafe storage dependency = %#v", blocked)
	}
}

func TestAquamarineStorageMetric(t *testing.T) {
	if metric := expansionStorageMetric("A"); metric != "aquamarineStorage" {
		t.Fatalf("Aquamarine storage metric = %q", metric)
	}
}
