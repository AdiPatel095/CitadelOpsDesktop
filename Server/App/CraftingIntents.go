package App

import (
	"CitadelDesktop/Server/Localization"
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
		return Intent.Plan{}, Localization.WithError(fmt.Errorf("castle %d is not in the current player state", request.CastleID), Localization.New("server.app.castle_p_is_not.47524bcb", "castle {p0} is not in the current player state", Localization.Params{"p0": fmt.Sprintf("%d", request.CastleID)}))
	}
	if !castle.SupportsSovereignCrafting() {
		return Intent.Plan{}, Localization.WithError(fmt.Errorf("castle %d is a sovereign-resource storage node, not a crafting castle", request.CastleID), Localization.New("server.app.castle_p_is_a.916acd03", "castle {p0} is a sovereign-resource storage node, not a crafting castle", Localization.Params{"p0": fmt.Sprintf("%d", request.CastleID)}))
	}
	building, exists := castle.Crafting.Buildings[request.BuildingInstanceID]
	if !exists || request.BuildingInstanceID <= 0 {
		return Intent.Plan{}, Localization.WithError(fmt.Errorf("crafting building %d is not in castle %d", request.BuildingInstanceID, request.CastleID), Localization.New("server.app.crafting_building_p_is.12059e46", "crafting building {p0} is not in castle {p1}", Localization.Params{"p0": fmt.Sprintf("%d", request.BuildingInstanceID), "p1": fmt.Sprintf("%d", request.CastleID)}))
	}
	if request.Slot < 0 || request.Slot >= len(building.Active) {
		return Intent.Plan{}, Localization.WithError(fmt.Errorf("production slot %d is not active", request.Slot), Localization.New("server.app.production_slot_p_is.46838465", "production slot {p0} is not active", Localization.Params{"p0": request.Slot}))
	}
	active := building.Active[request.Slot]
	if active.RemainingSec == nil || *active.RemainingSec <= 0 || input.GameData == nil {
		return Intent.Plan{}, Localization.WithError(fmt.Errorf("production slot %d has no observed remaining runtime", request.Slot), Localization.New("server.app.production_slot_p_has.f2650517", "production slot {p0} has no observed remaining runtime", Localization.Params{"p0": request.Slot}))
	}
	catalog, err := input.GameData.Catalog("craftingRecipes")
	if err != nil {
		return Intent.Plan{}, err
	}
	raw, exists := catalog.Find(strconv.FormatInt(active.RecipeID, 10))
	if !exists {
		return Intent.Plan{}, Localization.WithError(fmt.Errorf("crafting recipe %d is not in the current official catalog", active.RecipeID), Localization.New("server.app.crafting_recipe_p_is.c49a5825", "crafting recipe {p0} is not in the current official catalog", Localization.Params{"p0": fmt.Sprintf("%d", active.RecipeID)}))
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
		return Intent.Plan{}, Localization.WithError(fmt.Errorf("production slot %d has no official remaining-time ruby price", request.Slot), Localization.New("server.app.production_slot_p_has.4a639bdb", "production slot {p0} has no official remaining-time ruby price", Localization.Params{"p0": request.Slot}))
	}
	if rubies := playerResourceByOfficialKey(input.State, input.GameData, "C2"); rubies < float64(expectedPrice) {
		return Intent.Plan{}, Localization.WithError(fmt.Errorf("crafting skip needs %d rubies; %.0f are observed", expectedPrice, rubies), Localization.New("server.app.crafting_skip_needs_p.f9e27747", "crafting skip needs {p0} rubies; {p1} are observed", Localization.Params{"p0": expectedPrice, "p1": rubies}))
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
		Summary: fmt.Sprintf("Complete crafting recipe %d at %s for %d rubies", active.RecipeID, castleLabel(castle), expectedPrice), SummaryDescriptor: Localization.New("server.app.complete_crafting_recipe_p.f1d4ae9a", "Complete crafting recipe {p0} at {p1} for {p2} rubies", Localization.Params{"p0": fmt.Sprintf("%d", active.RecipeID), "p1": fmt.Sprintf("%s", castleLabel(castle)), "p2": expectedPrice}),
		Steps: []Intent.Step{commandStep("Complete crafting slot with rubies", "crsk", payload, "crsk", Localization.New("server.app.complete_crafting_slot_with.5cff232e", "Complete crafting slot with rubies", nil))},
	}, nil
}

func planCraftingSlotRental(_ context.Context, input Intent.PlanningContext, arguments json.RawMessage) (Intent.Plan, error) {
	var request struct {
		CastleID           State.CastleID           `json:"castleId"`
		BuildingInstanceID State.BuildingInstanceID `json:"buildingInstanceId"`
		SlotType           string                   `json:"slotType"`
		Slot               int                      `json:"slot"`
		MinimumCoinReserve int64                    `json:"minimumCoinReserve,omitempty"`
	}
	if err := decodeIntentArguments(arguments, &request); err != nil {
		return Intent.Plan{}, err
	}
	castle, exists := input.State.Castles[request.CastleID]
	if !exists || request.CastleID <= 0 {
		return Intent.Plan{}, Localization.WithError(fmt.Errorf("castle %d is not in the current player state", request.CastleID), Localization.New("server.app.castle_p_is_not.47524bcb", "castle {p0} is not in the current player state", Localization.Params{"p0": fmt.Sprintf("%d", request.CastleID)}))
	}
	if !castle.SupportsSovereignCrafting() {
		return Intent.Plan{}, Localization.WithError(fmt.Errorf("castle %d is a sovereign-resource storage node, not a crafting castle", request.CastleID), Localization.New("server.app.castle_p_is_a.916acd03", "castle {p0} is a sovereign-resource storage node, not a crafting castle", Localization.Params{"p0": fmt.Sprintf("%d", request.CastleID)}))
	}
	building, exists := castle.Crafting.Buildings[request.BuildingInstanceID]
	if !exists || request.BuildingInstanceID <= 0 {
		return Intent.Plan{}, Localization.WithError(fmt.Errorf("crafting building %d is not in castle %d", request.BuildingInstanceID, request.CastleID), Localization.New("server.app.crafting_building_p_is.12059e46", "crafting building {p0} is not in castle {p1}", Localization.Params{"p0": fmt.Sprintf("%d", request.BuildingInstanceID), "p1": fmt.Sprintf("%d", request.CastleID)}))
	}
	request.SlotType = strings.ToLower(strings.TrimSpace(request.SlotType))
	cost := float64(0)
	switch request.SlotType {
	case "production":
		if request.Slot != len(building.ActiveSlotRentals)+1 || request.Slot != 1 {
			return Intent.Plan{}, Localization.WithError(fmt.Errorf("production slot %d is not the next rentable slot", request.Slot), Localization.New("server.app.production_slot_p_is.db777cac", "production slot {p0} is not the next rentable slot", Localization.Params{"p0": request.Slot}))
		}
		cost = 5_000_000
	case "queue":
		if request.Slot != len(building.QueueSlotRentals)+1 || request.Slot < 1 || request.Slot > 3 {
			return Intent.Plan{}, Localization.WithError(fmt.Errorf("queue slot %d is not the next rentable slot", request.Slot), Localization.New("server.app.queue_slot_p_is.e1ff98e1", "queue slot {p0} is not the next rentable slot", Localization.Params{"p0": request.Slot}))
		}
		cost = map[int]float64{1: 500_000, 2: 3_000_000, 3: 6_500_000}[request.Slot]
	default:
		return Intent.Plan{}, Localization.WithError(fmt.Errorf("slotType must be production or queue"), Localization.New("server.app.slottype_must_be_production.c0106909", "slotType must be production or queue", nil))
	}
	coins := playerResourceByOfficialKey(input.State, input.GameData, "C1")
	if request.MinimumCoinReserve < 0 {
		return Intent.Plan{}, Localization.WithError(fmt.Errorf("minimumCoinReserve must not be negative"), Localization.New("server.app.minimumcoinreserve_must_not_be.5e3434b3", "minimumCoinReserve must not be negative", nil))
	}
	if coins < cost+float64(request.MinimumCoinReserve) {
		return Intent.Plan{}, &Intent.CoinUnavailableError{Required: int64(cost), Reserve: request.MinimumCoinReserve, Observed: int64(math.Floor(coins)), Source: "official crafting slot rental price"}
	}
	payload, _ := json.Marshal(struct {
		KingdomID State.KingdomID          `json:"KID"`
		CastleID  State.CastleID           `json:"AID"`
		Building  State.BuildingInstanceID `json:"OID"`
		Slots     []int                    `json:"S"`
		SlotType  string                   `json:"ST"`
	}{castle.KingdomID, castle.ID, building.InstanceID, []int{request.Slot}, request.SlotType})
	rentalStep := commandStep("Rent crafting slot", "crun", payload, "crun", Localization.New("server.app.rent_crafting_slot.99a45a37", "Rent crafting slot", nil))
	rentalStep.CoinCost = &Intent.CoinCostRequirement{Amount: int64(cost), Reserve: request.MinimumCoinReserve, Source: "official crafting slot rental price"}
	return Intent.Plan{
		Claims: []string{
			"castle:" + strconv.FormatInt(int64(castle.ID), 10),
			"crafting-building:" + strconv.FormatInt(int64(building.InstanceID), 10), "account-resources",
		},
		Summary: fmt.Sprintf("Rent %s crafting slot %d at %s", request.SlotType, request.Slot, castleLabel(castle)), SummaryDescriptor: Localization.New("server.app.rent_p_crafting_slot.6b1f6293", "Rent {p0} crafting slot {p1} at {p2}", Localization.Params{"p0": fmt.Sprintf("%s", request.SlotType), "p1": request.Slot, "p2": fmt.Sprintf("%s", castleLabel(castle))}),
		Steps: []Intent.Step{rentalStep},
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
	minimumCoinReserve int64,
) error {
	capacity := 2 + len(building.ActiveSlotRentals) + len(building.QueueSlotRentals)
	occupied := len(building.Active) + len(building.Queued)
	if capacity <= 0 || occupied >= capacity {
		return Localization.WithError(fmt.Errorf("crafting building %d is full", building.InstanceID), Localization.New("server.app.crafting_building_p_is.1d7dc479", "crafting building {p0} is full", Localization.Params{"p0": fmt.Sprintf("%d", building.InstanceID)}))
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
		reserve := float64(0)
		if strings.EqualFold(cost.JSONKey, "C1") {
			reserve = float64(minimumCoinReserve)
		}
		if available-reserve < cost.Amount {
			if strings.EqualFold(cost.JSONKey, "C1") {
				return &Intent.CoinUnavailableError{Required: int64(math.Ceil(cost.Amount)), Reserve: minimumCoinReserve, Observed: int64(math.Floor(available)), Source: "official crafting recipe cost"}
			}
			label := strings.TrimSpace(cost.JSONKey)
			if label == "" {
				label = strings.TrimPrefix(cost.Field, "cost")
			}
			return Localization.WithError(fmt.Errorf(
				"crafting recipe %d needs %.0f %s; %.0f are observed",
				recipeID, cost.Amount, label, available,
			), Localization.New("server.app.crafting_recipe_p_needs.8097c399", "crafting recipe {p0} needs {p1} {p2}; {p3} are observed", Localization.Params{"p0": fmt.Sprintf("%d", recipeID), "p1": cost.Amount, "p2": fmt.Sprintf("%s", label), "p3": available}))
		}
	}
	return nil
}
