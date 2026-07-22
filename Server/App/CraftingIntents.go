package App

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"CitadelDesktop/Server/GameData"
	"CitadelDesktop/Server/Intent"
	"CitadelDesktop/Server/State"
)

func planCraftingSkip(_ context.Context, input Intent.PlanningContext, arguments json.RawMessage) (Intent.Plan, error) {
	var request struct {
		CastleID           State.CastleID           `json:"castleId"`
		BuildingInstanceID State.BuildingInstanceID `json:"buildingInstanceId"`
		Slot               int                      `json:"slot"`
	}
	if err := decodeIntentArguments(arguments, &request); err != nil {
		return Intent.Plan{}, err
	}
	castle, exists := input.State.Castles[request.CastleID]
	if !exists || request.CastleID <= 0 {
		return Intent.Plan{}, fmt.Errorf("castle %d is not in the current player state", request.CastleID)
	}
	if !castle.SupportsSovereignCrafting() {
		return Intent.Plan{}, fmt.Errorf("castle %d is a sovereign-resource storage node, not a crafting castle", request.CastleID)
	}
	building, exists := castle.Crafting.Buildings[request.BuildingInstanceID]
	if !exists || request.BuildingInstanceID <= 0 {
		return Intent.Plan{}, fmt.Errorf("crafting building %d is not in castle %d", request.BuildingInstanceID, request.CastleID)
	}
	if request.Slot < 0 || request.Slot >= len(building.Active) {
		return Intent.Plan{}, fmt.Errorf("production slot %d is not active", request.Slot)
	}
	active := building.Active[request.Slot]
	if active.RemainingSec == nil || *active.RemainingSec <= 0 || input.GameData == nil {
		return Intent.Plan{}, fmt.Errorf("production slot %d has no observed remaining runtime", request.Slot)
	}
	catalog, err := input.GameData.Catalog("craftingRecipes")
	if err != nil {
		return Intent.Plan{}, err
	}
	raw, exists := catalog.Find(strconv.FormatInt(active.RecipeID, 10))
	if !exists {
		return Intent.Plan{}, fmt.Errorf("crafting recipe %d is not in the current official catalog", active.RecipeID)
	}
	recipe, err := GameData.DecodeRecord(raw)
	if err != nil {
		return Intent.Plan{}, err
	}
	duration, _ := recipe.Int64("craftingDuration")
	fullPrice, _ := recipe.Int64("skipCostC2")
	remaining := *active.RemainingSec
	if !building.ObservedAt.IsZero() && time.Now().After(building.ObservedAt) {
		remaining -= int(time.Since(building.ObservedAt) / time.Second)
	}
	remaining = max(0, remaining)
	expectedPrice := 0
	if duration > 0 && fullPrice > 0 && remaining > 0 {
		expectedPrice = int(math.Ceil(math.Min(float64(remaining), float64(duration)) / float64(duration) * float64(fullPrice)))
	}
	if expectedPrice <= 0 {
		return Intent.Plan{}, fmt.Errorf("production slot %d has no official remaining-time ruby price", request.Slot)
	}
	if rubies := playerResourceByOfficialKey(input.State, input.GameData, "C2"); rubies < float64(expectedPrice) {
		return Intent.Plan{}, fmt.Errorf("crafting skip needs %d rubies; %.0f are observed", expectedPrice, rubies)
	}
	payload, _ := json.Marshal(struct {
		KingdomID State.KingdomID          `json:"KID"`
		CastleID  State.CastleID           `json:"AID"`
		Building  State.BuildingInstanceID `json:"OID"`
		Slot      int                      `json:"S"`
		SlotType  string                   `json:"ST"`
		Price     int                      `json:"PC2"`
	}{castle.KingdomID, castle.ID, building.InstanceID, request.Slot, "production", expectedPrice})
	return Intent.Plan{
		Claims: []string{
			"castle:" + strconv.FormatInt(int64(castle.ID), 10),
			"crafting-building:" + strconv.FormatInt(int64(building.InstanceID), 10), "account-resources",
		},
		Summary: fmt.Sprintf("Complete crafting recipe %d at %s for %d rubies", active.RecipeID, castleLabel(castle), expectedPrice),
		Steps:   []Intent.Step{commandStep("Complete crafting slot with rubies", "crsk", payload, "crsk")},
	}, nil
}

func planCraftingSlotRental(_ context.Context, input Intent.PlanningContext, arguments json.RawMessage) (Intent.Plan, error) {
	var request struct {
		CastleID           State.CastleID           `json:"castleId"`
		BuildingInstanceID State.BuildingInstanceID `json:"buildingInstanceId"`
		SlotType           string                   `json:"slotType"`
		Slot               int                      `json:"slot"`
	}
	if err := decodeIntentArguments(arguments, &request); err != nil {
		return Intent.Plan{}, err
	}
	castle, exists := input.State.Castles[request.CastleID]
	if !exists || request.CastleID <= 0 {
		return Intent.Plan{}, fmt.Errorf("castle %d is not in the current player state", request.CastleID)
	}
	if !castle.SupportsSovereignCrafting() {
		return Intent.Plan{}, fmt.Errorf("castle %d is a sovereign-resource storage node, not a crafting castle", request.CastleID)
	}
	building, exists := castle.Crafting.Buildings[request.BuildingInstanceID]
	if !exists || request.BuildingInstanceID <= 0 {
		return Intent.Plan{}, fmt.Errorf("crafting building %d is not in castle %d", request.BuildingInstanceID, request.CastleID)
	}
	request.SlotType = strings.ToLower(strings.TrimSpace(request.SlotType))
	cost := float64(0)
	switch request.SlotType {
	case "production":
		if request.Slot != len(building.ActiveSlotRentals)+1 || request.Slot != 1 {
			return Intent.Plan{}, fmt.Errorf("production slot %d is not the next rentable slot", request.Slot)
		}
		cost = 5_000_000
	case "queue":
		if request.Slot != len(building.QueueSlotRentals)+1 || request.Slot < 1 || request.Slot > 3 {
			return Intent.Plan{}, fmt.Errorf("queue slot %d is not the next rentable slot", request.Slot)
		}
		cost = map[int]float64{1: 500_000, 2: 3_000_000, 3: 6_500_000}[request.Slot]
	default:
		return Intent.Plan{}, fmt.Errorf("slotType must be production or queue")
	}
	coins := playerResourceByOfficialKey(input.State, input.GameData, "C1")
	if coins < cost {
		return Intent.Plan{}, fmt.Errorf("crafting slot rental needs %.0f coins; %.0f are observed", cost, coins)
	}
	payload, _ := json.Marshal(struct {
		KingdomID State.KingdomID          `json:"KID"`
		CastleID  State.CastleID           `json:"AID"`
		Building  State.BuildingInstanceID `json:"OID"`
		Slots     []int                    `json:"S"`
		SlotType  string                   `json:"ST"`
	}{castle.KingdomID, castle.ID, building.InstanceID, []int{request.Slot}, request.SlotType})
	return Intent.Plan{
		Claims: []string{
			"castle:" + strconv.FormatInt(int64(castle.ID), 10),
			"crafting-building:" + strconv.FormatInt(int64(building.InstanceID), 10), "account-resources",
		},
		Summary: fmt.Sprintf("Rent %s crafting slot %d at %s", request.SlotType, request.Slot, castleLabel(castle)),
		Steps:   []Intent.Step{commandStep("Rent crafting slot", "crun", payload, "crun")},
	}, nil
}

func playerResourceByOfficialKey(state State.GameState, store *GameData.Store, jsonKey string) float64 {
	if store == nil {
		return 0
	}
	catalog, err := store.Catalog("resources")
	if err != nil {
		return 0
	}
	for _, raw := range catalog.Rows() {
		record, decodeErr := GameData.DecodeRecord(raw)
		if decodeErr != nil {
			continue
		}
		candidate, _ := record.String("JSONKey")
		if !strings.EqualFold(candidate, jsonKey) {
			continue
		}
		id, _ := record.Int64("resourceID")
		return state.Player.Resources[State.ResourceID(id)]
	}
	return 0
}

func validateCraftingStartAvailability(
	state State.GameState,
	store *GameData.Store,
	castle State.CastleState,
	building State.CraftingBuilding,
	recipeID int64,
) error {
	capacity := 2 + len(building.ActiveSlotRentals) + len(building.QueueSlotRentals)
	occupied := len(building.Active) + len(building.Queued)
	if capacity <= 0 || occupied >= capacity {
		return fmt.Errorf("crafting building %d is full", building.InstanceID)
	}
	costs, err := GameData.CraftingRecipeCosts(store, recipeID)
	if err != nil {
		return err
	}
	for _, cost := range costs {
		available := float64(0)
		switch {
		case cost.ResourceID > 0:
			available = castle.Resources[State.ResourceID(cost.ResourceID)].Amount
			if strings.EqualFold(cost.JSONKey, "C1") || strings.EqualFold(cost.JSONKey, "C2") {
				available = state.Player.Resources[State.ResourceID(cost.ResourceID)]
			}
		case cost.CurrencyID > 0:
			available = state.Player.Currencies[State.CurrencyID(cost.CurrencyID)]
		}
		if available < cost.Amount {
			label := strings.TrimSpace(cost.JSONKey)
			if label == "" {
				label = strings.TrimPrefix(cost.Field, "cost")
			}
			return fmt.Errorf(
				"crafting recipe %d needs %.0f %s; %.0f are observed",
				recipeID, cost.Amount, label, available,
			)
		}
	}
	return nil
}
