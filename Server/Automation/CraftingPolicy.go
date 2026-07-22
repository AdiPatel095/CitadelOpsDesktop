package Automation

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"CitadelDesktop/Server/GameData"
	"CitadelDesktop/Server/Intent"
	"CitadelDesktop/Server/State"
)

type CraftingPolicy struct{}

type craftingSettings struct {
	CheckIntervalSec         int                           `json:"checkIntervalSec"`
	MinimumShipmentSize      int64                         `json:"minimumShipmentSize"`
	SourceReservePercent     int                           `json:"sourceReservePercent"`
	OverflowThresholdPercent int                           `json:"overflowThresholdPercent"`
	AutoKingdomTransport     bool                          `json:"autoKingdomTransport"`
	UseKingdomTimeSkips      bool                          `json:"useKingdomTimeSkips"`
	AllowedTimeSkips         []string                      `json:"allowedTimeSkips"`
	TimeSkipReserve          map[string]int64              `json:"timeSkipReserve"`
	UseStormBuffer           bool                          `json:"useStormBuffer"`
	AllowRubyRecipes         bool                          `json:"allowRubyRecipes"`
	UseRubyOverflowSkip      bool                          `json:"useRubyOverflowSkip"`
	MinimumCoinReserve       float64                       `json:"minimumCoinReserve"`
	MinimumRubyReserve       float64                       `json:"minimumRubyReserve"`
	Castles                  map[string]craftingCastlePlan `json:"castles"`
}

type craftingCastlePlan struct {
	Buildings map[string]craftingBuildingPlan `json:"buildings"`
}

type craftingBuildingPlan struct {
	Enabled            bool                 `json:"enabled"`
	Steps              []craftingRecipeStep `json:"steps"`
	Cursor             int                  `json:"cursor"`
	AutoRentActiveSlot bool                 `json:"autoRentActiveSlot"`
	AutoRentQueueSlots int                  `json:"autoRentQueueSlots"`
}

type craftingRecipeStep struct {
	RecipeID int64 `json:"recipeID"`
	Repeat   int   `json:"repeat"`
}

type craftingRecipeSelection struct {
	Cursor             int
	ConfiguredRecipeID int64
	RecipeID           int64
	Costs              craftingCostEvaluation
}

func NewCraftingPolicy() *CraftingPolicy { return &CraftingPolicy{} }

func (*CraftingPolicy) ID() string { return "autoSceatRes" }

func (*CraftingPolicy) EnabledKey() string { return "auto_sceat_resources" }

func (*CraftingPolicy) WakeDomains() []string {
	return []string{"crafting", "currencies", "kingdom-transport", "market", "movements", "resources"}
}

func (*CraftingPolicy) WakeSections() []string {
	return []string{"automation.autoSceatResources"}
}

func (*CraftingPolicy) Evaluate(_ context.Context, snapshot Snapshot) (Decision, error) {
	settings := craftingSettings{
		CheckIntervalSec: 300, MinimumShipmentSize: 10_000, SourceReservePercent: 10,
		OverflowThresholdPercent: 90, UseStormBuffer: true,
		TimeSkipReserve: map[string]int64{}, Castles: map[string]craftingCastlePlan{},
	}
	raw := snapshot.Configuration.Sections["automation.autoSceatResources"]
	if len(raw) == 0 || json.Unmarshal(raw, &settings) != nil {
		return Decision{
			Status: "waiting", Detail: "No sovereign crafting plan is configured",
			NextCheckAt: snapshot.Now.Add(policyInterval(settings.CheckIntervalSec, 300)),
		}, nil
	}
	interval := policyInterval(settings.CheckIntervalSec, 300)
	if settings.AutoKingdomTransport {
		logisticsStale, logisticsErr := craftingLogisticsStale(snapshot, interval)
		if logisticsErr != nil {
			return Decision{}, logisticsErr
		}
		if logisticsStale {
			return Decision{
				Status:              "ready",
				Detail:              "Refresh market and kingdom-resource logistics",
				NextCheckAt:         snapshot.Now.Add(2 * time.Second),
				Request:             &Intent.Request{Name: "resource.logistics.refresh", Arguments: json.RawMessage(`{}`)},
				ReevaluateOnSuccess: true,
			}, nil
		}
	}
	if decision, ready := ownedKingdomTransportDecision(
		autoSceatTransportOwner, "Auto Sceat Resources",
		settings.AutoKingdomTransport && settings.UseKingdomTimeSkips,
		settings.AllowedTimeSkips, settings.TimeSkipReserve, snapshot,
	); ready {
		return decision, nil
	}
	if craftingSnapshotStale(settings, snapshot, interval) {
		return Decision{
			Status:              "ready",
			Detail:              "Refresh sovereign crafting queues",
			NextCheckAt:         snapshot.Now.Add(2 * time.Second),
			Request:             &Intent.Request{Name: "crafting.refresh", Arguments: json.RawMessage(`{}`)},
			ReevaluateOnSuccess: true,
		}, nil
	}
	plans := 0
	observed := 0
	full := 0
	waitingForResources := 0
	for _, castleKey := range sortedNumericKeys(settings.Castles) {
		castleIDValue, _ := strconv.ParseInt(castleKey, 10, 64)
		castleID := State.CastleID(castleIDValue)
		castle, exists := snapshot.State.Castles[castleID]
		if !exists || !castle.SupportsSovereignCrafting() {
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
			cursor := plan.Cursor % len(cycle)
			if cursor < 0 {
				cursor = 0
			}
			selection, missingSelection, waiting, costErr := selectCraftingRecipe(
				snapshot, settings, castle, building, cycle, cursor,
			)
			if costErr != nil {
				return Decision{}, costErr
			}
			occupied := len(building.Active) + len(building.Queued)
			capacity := 2 + len(building.ActiveSlotRentals) + len(building.QueueSlotRentals)
			if occupied >= capacity {
				full++
				if selection.RecipeID > 0 {
					if decision, ready := craftingRentalDecision(settings, snapshot, castle, building, plan); ready {
						return decision, nil
					}
				}
				if waiting {
					waitingForResources++
				}
				continue
			}
			if selection.RecipeID <= 0 {
				if waiting {
					waitingForResources++
				}
				if settings.AutoKingdomTransport && missingSelection != nil {
					if decision, handled := craftingTransportDecision(settings, snapshot, castle, missingSelection.Costs.Missing, interval); handled {
						return decision, nil
					}
				}
				continue
			}
			configuredRecipeID := selection.ConfiguredRecipeID
			recipeID := selection.RecipeID
			arguments, _ := json.Marshal(map[string]any{
				"castleId": castleID, "buildingInstanceId": building.InstanceID,
				"recipeId": recipeID, "power": 0,
			})
			nextCursor := (selection.Cursor + 1) % len(cycle)
			updated, updateErr := advanceCraftingCursor(raw, castleKey, queueKey, nextCursor)
			if updateErr != nil {
				return Decision{}, updateErr
			}
			if recipeID != configuredRecipeID {
				if updateErr = replaceCraftingRecipeID(updated, castleKey, queueKey, configuredRecipeID, recipeID); updateErr != nil {
					return Decision{}, updateErr
				}
			}
			followUpArguments, _ := json.Marshal(map[string]any{
				"section": "automation.autoSceatResources", "value": updated,
				"expectedValue": json.RawMessage(raw),
			})
			detail := fmt.Sprintf("Queue crafting recipe %d at %s", recipeID, castleName(castle))
			metrics := map[string]float64(nil)
			if selection.Cursor != cursor {
				detail = fmt.Sprintf(
					"Queue available crafting recipe %d at %s while earlier cycle recipe %d waits",
					recipeID, castleName(castle), cycle[cursor],
				)
				metrics = map[string]float64{
					"waitingRecipeId":  float64(cycle[cursor]),
					"selectedRecipeId": float64(recipeID),
				}
			}
			if recipeID != configuredRecipeID {
				detail = fmt.Sprintf(
					"Queue highest unlocked recipe %d at %s and replace unavailable recipe %d",
					recipeID, castleName(castle), configuredRecipeID,
				)
				metrics = map[string]float64{
					"configuredRecipeId": float64(configuredRecipeID),
					"resolvedRecipeId":   float64(recipeID),
				}
			}
			return Decision{
				Status:              "ready",
				Detail:              detail,
				NextCheckAt:         snapshot.Now.Add(interval),
				Metrics:             metrics,
				Request:             &Intent.Request{Name: "crafting.start", Arguments: arguments},
				FollowUp:            &Intent.Request{Name: "config.update", Arguments: followUpArguments},
				ReevaluateOnSuccess: true,
			}, nil
		}
	}
	if settings.AutoKingdomTransport {
		if decision, ready := craftingLootDrainDecision(settings, snapshot); ready {
			return decision, nil
		}
		if decision, ready := marketOverflowDecision(settings, snapshot, interval); ready {
			return decision, nil
		}
		if decision, ready := stormOverflowDecision(settings, snapshot, interval); ready {
			return decision, nil
		}
	}
	if decision, ready := rubyOverflowSkipDecision(settings, snapshot); ready {
		return decision, nil
	}
	detail := "No enabled crafting sequence is configured"
	if plans > 0 && observed == 0 {
		detail = "Waiting for configured crafting buildings to be observed"
	} else if plans > 0 && observed == full {
		detail = "All configured crafting queues are full"
	} else if plans > 0 && waitingForResources > 0 {
		detail = "Configured crafting recipes are waiting for resources or configured reserves"
	} else if plans > 0 {
		detail = "Configured recipes are not available for the observed crafting queues"
	}
	return Decision{Status: "idle", Detail: detail, NextCheckAt: snapshot.Now.Add(interval)}, nil
}

func selectCraftingRecipe(
	snapshot Snapshot,
	settings craftingSettings,
	castle State.CastleState,
	building State.CraftingBuilding,
	cycle []int64,
	cursor int,
) (craftingRecipeSelection, *craftingRecipeSelection, bool, error) {
	var firstMissing *craftingRecipeSelection
	waiting := false
	for offset := 0; offset < len(cycle); offset++ {
		candidateCursor := (cursor + offset) % len(cycle)
		configuredRecipeID := cycle[candidateCursor]
		recipeID := resolveCraftingRecipe(snapshot, configuredRecipeID, building, castle.Crafting)
		if recipeID <= 0 {
			continue
		}
		costs, err := craftingRecipeCostState(snapshot, castle, recipeID, settings)
		if err != nil {
			return craftingRecipeSelection{}, nil, waiting, err
		}
		candidate := craftingRecipeSelection{
			Cursor: candidateCursor, ConfiguredRecipeID: configuredRecipeID, RecipeID: recipeID, Costs: costs,
		}
		if costs.Blocked == "" && len(costs.Missing) == 0 {
			return candidate, firstMissing, waiting, nil
		}
		waiting = true
		if costs.Blocked == "" && len(costs.Missing) > 0 && firstMissing == nil {
			copy := candidate
			firstMissing = &copy
		}
	}
	return craftingRecipeSelection{}, firstMissing, waiting, nil
}

func craftingSnapshotStale(settings craftingSettings, snapshot Snapshot, interval time.Duration) bool {
	configured := false
	oldest := time.Time{}
	missing := false
	for castleKey, castlePlan := range settings.Castles {
		castleIDValue, _ := strconv.ParseInt(castleKey, 10, 64)
		castle, castleExists := snapshot.State.Castles[State.CastleID(castleIDValue)]
		if castleExists && !castle.SupportsSovereignCrafting() {
			continue
		}
		for queueKey, plan := range castlePlan.Buildings {
			if !plan.Enabled || len(craftingCycle(plan.Steps)) == 0 {
				continue
			}
			configured = true
			queueType, _ := strconv.Atoi(queueKey)
			building, buildingExists := craftingBuildingForQueue(castle, queueType)
			if !castleExists || !buildingExists || building.ObservedAt.IsZero() {
				missing = true
				continue
			}
			if oldest.IsZero() || building.ObservedAt.Before(oldest) {
				oldest = building.ObservedAt
			}
		}
	}
	if !configured {
		return false
	}
	if observation, found := snapshot.State.Observations["crin"]; found {
		observedAt := observation.SuccessfulInboundAt()
		if !observedAt.IsZero() {
			return (!snapshot.State.Session.ChangedAt.IsZero() && observedAt.Before(snapshot.State.Session.ChangedAt)) ||
				snapshot.Now.Sub(observedAt) >= interval
		}
	}
	return missing || oldest.IsZero() || snapshot.Now.Sub(oldest) >= interval
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

func craftingRecipeMatches(
	snapshot Snapshot,
	recipeID int64,
	building State.CraftingBuilding,
	crafting State.CraftingState,
) bool {
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
	return craftingRecipeAvailable(record, recipeID, building, crafting)
}

// resolveCraftingRecipe keeps plans saved by the former group-level entitlement
// logic usable. That logic exposed every level in an unlocked recipe group, so
// some plans contain a level the account has never unlocked. Preserve the
// selected output and duration type while choosing the highest unlocked level
// at or below the configured level.
func resolveCraftingRecipe(
	snapshot Snapshot,
	configuredID int64,
	building State.CraftingBuilding,
	crafting State.CraftingState,
) int64 {
	if craftingRecipeMatches(snapshot, configuredID, building, crafting) {
		return configuredID
	}
	if snapshot.GameData == nil {
		return 0
	}
	catalog, err := snapshot.GameData.Catalog("craftingRecipes")
	if err != nil {
		return 0
	}
	targetRaw, exists := catalog.Find(strconv.FormatInt(configuredID, 10))
	if !exists {
		return 0
	}
	target, err := GameData.DecodeRecord(targetRaw)
	if err != nil {
		return 0
	}
	targetQueue, _ := target.Int64("queueTypeId")
	targetGroup, _ := target.Int64("recipeGroupID")
	targetLevel, _ := target.Int64("level")
	targetType, _ := target.String("type")
	if int(targetQueue) != building.QueueTypeID || targetGroup <= 0 || targetLevel <= 0 || strings.TrimSpace(targetType) == "" {
		return 0
	}

	bestID := int64(0)
	bestLevel := int64(0)
	for _, raw := range catalog.Rows() {
		record, decodeErr := GameData.DecodeRecord(raw)
		if decodeErr != nil {
			continue
		}
		queueType, _ := record.Int64("queueTypeId")
		groupID, _ := record.Int64("recipeGroupID")
		level, _ := record.Int64("level")
		recipeType, _ := record.String("type")
		if queueType != targetQueue || groupID != targetGroup || level <= 0 || level > targetLevel ||
			!strings.EqualFold(strings.TrimSpace(recipeType), strings.TrimSpace(targetType)) {
			continue
		}
		candidateID, _ := record.Int64("craftingRecipeId")
		if candidateID <= 0 || !craftingRecipeAvailable(record, candidateID, building, crafting) {
			continue
		}
		if level > bestLevel || level == bestLevel && candidateID > bestID {
			bestID = candidateID
			bestLevel = level
		}
	}
	return bestID
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

func replaceCraftingRecipeID(
	document map[string]any,
	castleKey string,
	queueKey string,
	configuredID int64,
	resolvedID int64,
) error {
	castles, ok := document["castles"].(map[string]any)
	if !ok {
		return fmt.Errorf("crafting configuration has no castles object")
	}
	castle, ok := castles[castleKey].(map[string]any)
	if !ok {
		return fmt.Errorf("crafting configuration has no castle %s", castleKey)
	}
	buildings, ok := castle["buildings"].(map[string]any)
	if !ok {
		return fmt.Errorf("crafting configuration has no buildings for castle %s", castleKey)
	}
	building, ok := buildings[queueKey].(map[string]any)
	if !ok {
		return fmt.Errorf("crafting configuration has no queue %s for castle %s", queueKey, castleKey)
	}
	steps, ok := building["steps"].([]any)
	if !ok {
		return fmt.Errorf("crafting configuration has no recipe steps for queue %s at castle %s", queueKey, castleKey)
	}
	replaced := false
	for _, rawStep := range steps {
		step, stepOK := rawStep.(map[string]any)
		if !stepOK {
			continue
		}
		value, valueOK := step["recipeID"].(float64)
		if valueOK && int64(value) == configuredID {
			step["recipeID"] = resolvedID
			replaced = true
		}
	}
	if !replaced {
		return fmt.Errorf("crafting configuration recipe %d was not found for queue %s at castle %s", configuredID, queueKey, castleKey)
	}
	return nil
}
