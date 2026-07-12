package Automation

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"

	"CitadelDesktop/Server/GameData"
	"CitadelDesktop/Server/Intent"
	"CitadelDesktop/Server/State"
)

type CraftingPolicy struct{}

type craftingSettings struct {
	CheckIntervalSec int                           `json:"checkIntervalSec"`
	Castles          map[string]craftingCastlePlan `json:"castles"`
}

type craftingCastlePlan struct {
	Buildings map[string]craftingBuildingPlan `json:"buildings"`
}

type craftingBuildingPlan struct {
	Enabled bool                 `json:"enabled"`
	Steps   []craftingRecipeStep `json:"steps"`
	Cursor  int                  `json:"cursor"`
}

type craftingRecipeStep struct {
	RecipeID int64 `json:"recipeID"`
	Repeat   int   `json:"repeat"`
}

func NewCraftingPolicy() *CraftingPolicy { return &CraftingPolicy{} }

func (*CraftingPolicy) ID() string { return "autoSceatRes" }

func (*CraftingPolicy) EnabledKey() string { return "auto_sceat_resources" }

func (*CraftingPolicy) Evaluate(_ context.Context, snapshot Snapshot) (Decision, error) {
	settings := craftingSettings{CheckIntervalSec: 300, Castles: map[string]craftingCastlePlan{}}
	raw := snapshot.Configuration.Sections["automation.autoSceatResources"]
	if len(raw) == 0 || json.Unmarshal(raw, &settings) != nil {
		return Decision{
			Status: "waiting", Detail: "No sovereign crafting plan is configured",
			NextCheckAt: snapshot.Now.Add(policyInterval(settings.CheckIntervalSec, 300)),
		}, nil
	}
	interval := policyInterval(settings.CheckIntervalSec, 300)
	plans := 0
	observed := 0
	full := 0
	for _, castleKey := range sortedNumericKeys(settings.Castles) {
		castleIDValue, _ := strconv.ParseInt(castleKey, 10, 64)
		castleID := State.CastleID(castleIDValue)
		castle, exists := snapshot.State.Castles[castleID]
		if !exists {
			continue
		}
		castlePlan := settings.Castles[castleKey]
		for _, queueKey := range sortedNumericKeys(castlePlan.Buildings) {
			plan := castlePlan.Buildings[queueKey]
			cycle := craftingCycle(plan.Steps)
			if !plan.Enabled || len(cycle) == 0 {
				continue
			}
			plans++
			queueType, _ := strconv.Atoi(queueKey)
			building, buildingExists := craftingBuildingForQueue(castle, queueType)
			if !buildingExists {
				continue
			}
			observed++
			capacity := building.SlotCount
			if capacity <= 0 {
				capacity = 1
			}
			if len(building.Active)+len(building.Queued) >= capacity {
				full++
				continue
			}
			cursor := plan.Cursor % len(cycle)
			if cursor < 0 {
				cursor = 0
			}
			recipeID := cycle[cursor]
			if !craftingRecipeMatches(snapshot, recipeID, queueType) {
				continue
			}
			arguments, _ := json.Marshal(map[string]any{
				"castleId": castleID, "buildingInstanceId": building.InstanceID,
				"recipeId": recipeID, "power": 0,
			})
			nextCursor := (cursor + 1) % len(cycle)
			updated, updateErr := advanceCraftingCursor(raw, castleKey, queueKey, nextCursor)
			if updateErr != nil {
				return Decision{}, updateErr
			}
			followUpArguments, _ := json.Marshal(map[string]any{
				"section": "automation.autoSceatResources", "value": updated,
				"expectedRevision": snapshot.Configuration.Revision,
			})
			return Decision{
				Status:      "ready",
				Detail:      fmt.Sprintf("Queue crafting recipe %d at %s", recipeID, castleName(castle)),
				NextCheckAt: snapshot.Now.Add(interval),
				Request:     &Intent.Request{Name: "crafting.start", Arguments: arguments},
				FollowUp:    &Intent.Request{Name: "config.update", Arguments: followUpArguments},
			}, nil
		}
	}
	detail := "No enabled crafting sequence is configured"
	if plans > 0 && observed == 0 {
		detail = "Waiting for configured crafting buildings to be observed"
	} else if plans > 0 && observed == full {
		detail = "All configured crafting queues are full"
	} else if plans > 0 {
		detail = "Configured recipes are not available for the observed crafting queues"
	}
	return Decision{Status: "idle", Detail: detail, NextCheckAt: snapshot.Now.Add(interval)}, nil
}

func craftingCycle(steps []craftingRecipeStep) []int64 {
	result := make([]int64, 0)
	for _, step := range steps {
		if step.RecipeID <= 0 {
			continue
		}
		repeat := step.Repeat
		if repeat < 1 {
			repeat = 1
		}
		if repeat > 100 {
			repeat = 100
		}
		for index := 0; index < repeat; index++ {
			result = append(result, step.RecipeID)
		}
	}
	return result
}

func craftingBuildingForQueue(castle State.CastleState, queueType int) (State.CraftingBuilding, bool) {
	ids := make([]State.BuildingInstanceID, 0, len(castle.Crafting.Buildings))
	for id := range castle.Crafting.Buildings {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(left, right int) bool { return ids[left] < ids[right] })
	for _, id := range ids {
		building := castle.Crafting.Buildings[id]
		if building.QueueTypeID == queueType {
			return building, true
		}
	}
	return State.CraftingBuilding{}, false
}

func craftingRecipeMatches(snapshot Snapshot, recipeID int64, queueType int) bool {
	if snapshot.GameData == nil {
		return false
	}
	catalog, err := snapshot.GameData.Catalog("craftingRecipes")
	if err != nil {
		return false
	}
	raw, exists := catalog.Find(strconv.FormatInt(recipeID, 10))
	if !exists {
		return false
	}
	record, err := GameData.DecodeRecord(raw)
	if err != nil {
		return false
	}
	value, _ := record.Int64("queueTypeId")
	return int(value) == queueType
}

func advanceCraftingCursor(raw json.RawMessage, castleKey string, queueKey string, cursor int) (map[string]any, error) {
	var document map[string]any
	if err := json.Unmarshal(raw, &document); err != nil {
		return nil, fmt.Errorf("decode crafting configuration: %w", err)
	}
	castles, ok := document["castles"].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("crafting configuration has no castles object")
	}
	castle, ok := castles[castleKey].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("crafting configuration has no castle %s", castleKey)
	}
	buildings, ok := castle["buildings"].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("crafting configuration has no buildings for castle %s", castleKey)
	}
	building, ok := buildings[queueKey].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("crafting configuration has no queue %s for castle %s", queueKey, castleKey)
	}
	building["cursor"] = cursor
	return document, nil
}
