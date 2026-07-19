package Automation

import (
	"encoding/json"
	"testing"
	"time"

	"CitadelDesktop/Server/Configuration"
	"CitadelDesktop/Server/GameData"
	"CitadelDesktop/Server/State"
)

func TestCraftingPolicyRepairsUnavailableRecipeLevel(t *testing.T) {
	store, err := GameData.DecodeStore([]byte(`{
		"versionInfo":[],"buildings":[{"wodID":1}],"units":[{"wodID":1}],
		"resources":[{"resourceID":6,"JSONKey":"C","name":"coal"}],
		"currencies":[],
		"craftingRecipes":[
			{"craftingRecipeId":102,"queueTypeId":1,"recipeGroupID":4,"researchGroupID":9,"level":2,"type":"Short","costC":1000},
			{"craftingRecipeId":103,"queueTypeId":1,"recipeGroupID":4,"researchGroupID":9,"level":3,"type":"Short","costC":2000}
		]
	}`), GameData.SourceMetadata{ItemVersion: "test"})
	if err != nil {
		t.Fatal(err)
	}

	now := time.Date(2026, 7, 15, 12, 0, 0, 0, time.UTC)
	gameState := State.NewGameState()
	castle := State.CastleState{
		ID: 20, Name: "Main",
		Resources: map[State.ResourceID]State.ResourceBalance{6: {Amount: 5_000}},
		Crafting: State.CraftingState{
			Buildings: map[State.BuildingInstanceID]State.CraftingBuilding{
				200: {
					CastleID: 20, InstanceID: 200, DefinitionID: 1, QueueTypeID: 1,
					Active: []State.CraftingQueueItem{}, Queued: []State.CraftingQueueItem{}, ObservedAt: now,
				},
			},
			EnabledRecipeIDs:       []int64{102},
			EnabledRecipeGroupIDs:  []int64{4},
			OutputBoostByQueueType: map[int]float64{},
		},
	}
	gameState.Castles[castle.ID] = castle
	rawSettings := json.RawMessage(`{
		"castles":{"20":{"buildings":{"1":{
			"enabled":true,"steps":[{"recipeID":103,"repeat":1}],"cursor":0
		}}}}
	}`)
	decision, err := NewCraftingPolicy().Evaluate(t.Context(), Snapshot{
		State: gameState,
		Configuration: Configuration.Snapshot{Sections: map[string]json.RawMessage{
			"automation.autoSceatResources": rawSettings,
		}},
		GameData: store,
		Now:      now,
	})
	if err != nil {
		t.Fatal(err)
	}
	if decision.Request == nil || decision.Request.Name != "crafting.start" {
		t.Fatalf("unexpected decision: %+v", decision)
	}
	var start struct {
		RecipeID int64 `json:"recipeId"`
	}
	if err := json.Unmarshal(decision.Request.Arguments, &start); err != nil {
		t.Fatal(err)
	}
	if start.RecipeID != 102 {
		t.Fatalf("resolved recipe = %d, want 102", start.RecipeID)
	}
	if decision.FollowUp == nil || decision.FollowUp.Name != "config.update" {
		t.Fatalf("missing repair follow-up: %+v", decision)
	}
	var followUp struct {
		Value json.RawMessage `json:"value"`
	}
	if err := json.Unmarshal(decision.FollowUp.Arguments, &followUp); err != nil {
		t.Fatal(err)
	}
	var updated craftingSettings
	if err := json.Unmarshal(followUp.Value, &updated); err != nil {
		t.Fatal(err)
	}
	if got := updated.Castles["20"].Buildings["1"].Steps[0].RecipeID; got != 102 {
		t.Fatalf("persisted recipe = %d, want 102", got)
	}
}
