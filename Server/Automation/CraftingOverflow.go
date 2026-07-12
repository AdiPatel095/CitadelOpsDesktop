package Automation

import (
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"

	"CitadelDesktop/Server/GameData"
	"CitadelDesktop/Server/Intent"
	"CitadelDesktop/Server/State"
)

func marketOverflowDecision(settings craftingSettings, snapshot Snapshot, interval time.Duration) (Decision, bool) {
	type candidate struct {
		source   State.CastleState
		target   State.CastleState
		resource State.ResourceID
		amount   float64
		barrows  int
	}
	best := candidate{}
	for _, sourceID := range sortedCastleIDs(snapshot.State.Castles) {
		source := snapshot.State.Castles[sourceID]
		market, observed := snapshot.State.Market.Castles[source.ID]
		if !observed || market.AvailableBarrows <= 0 {
			continue
		}
		capacityPerBarrow := marketCapacityPerBarrow(snapshot, market)
		if capacityPerBarrow <= 0 {
			continue
		}
		shipmentCapacity := float64(market.AvailableBarrows * capacityPerBarrow)
		for _, resourceID := range sovereignResourceIDs(snapshot.GameData) {
			overflow := math.Min(craftingOverflowAmount(settings, snapshot, source, resourceID), shipmentCapacity)
			if overflow <= 0 {
				continue
			}
			for _, targetID := range sortedCastleIDs(snapshot.State.Castles) {
				target := snapshot.State.Castles[targetID]
				if target.ID == source.ID || target.KingdomID != source.KingdomID {
					continue
				}
				balance := target.Resources[resourceID]
				if balance.Capacity == nil {
					continue
				}
				free := *balance.Capacity - balance.Amount - incomingMarketResource(snapshot, target, resourceID)
				amount := math.Floor(math.Min(overflow, free))
				if !craftingShipmentMeetsMinimum(amount, settings.MinimumShipmentSize) || amount <= best.amount {
					continue
				}
				best = candidate{
					source: source, target: target, resource: resourceID, amount: amount,
					barrows: int(math.Ceil(amount / float64(capacityPerBarrow))),
				}
			}
		}
	}
	if best.amount <= 0 {
		return Decision{}, false
	}
	coinCost := marketShipmentCoinCost(best.source, best.target, best.barrows)
	if playerResourceAmount(snapshot, "C1")-settings.MinimumCoinReserve < coinCost {
		return Decision{}, false
	}
	arguments, _ := json.Marshal(map[string]any{
		"sourceCastleId": best.source.ID, "targetCastleId": best.target.ID,
		"resourceId": best.resource, "amount": int64(best.amount),
	})
	return Decision{
		Status:      "ready",
		Detail:      fmt.Sprintf("Move %.0f overflow resource %d from %s to %s", best.amount, best.resource, castleName(best.source), castleName(best.target)),
		NextCheckAt: snapshot.Now.Add(interval),
		Metrics:     map[string]float64{"shipmentAmount": best.amount, "coinCost": coinCost},
		Request:     &Intent.Request{Name: "resource.market.ship", Arguments: arguments},
	}, true
}

func stormOverflowDecision(settings craftingSettings, snapshot Snapshot, interval time.Duration) (Decision, bool) {
	if !settings.UseStormBuffer {
		return Decision{}, false
	}
	var storm State.CastleState
	for _, castleID := range sortedCastleIDs(snapshot.State.Castles) {
		castle := snapshot.State.Castles[castleID]
		if castle.KingdomID == 4 {
			storm = castle
			break
		}
	}
	if storm.ID <= 0 {
		return Decision{}, false
	}
	for _, pending := range snapshot.State.KingdomTransport.Pending {
		if pending.KingdomID == storm.KingdomID && pending.RemainingSec > 0 {
			return Decision{}, false
		}
	}
	unlock, observed := snapshot.State.KingdomTransport.Unlocks[storm.KingdomID]
	if !observed || !unlock.Unlocked {
		return Decision{}, false
	}
	type candidate struct {
		source   State.CastleState
		resource State.ResourceID
		amount   float64
	}
	best := candidate{}
	for _, sourceID := range sortedCastleIDs(snapshot.State.Castles) {
		source := snapshot.State.Castles[sourceID]
		if source.KingdomID == storm.KingdomID {
			continue
		}
		for _, resourceID := range sovereignResourceIDs(snapshot.GameData) {
			targetBalance := storm.Resources[resourceID]
			if targetBalance.Capacity == nil {
				continue
			}
			freeAtTarget := math.Max(0, *targetBalance.Capacity-targetBalance.Amount)
			amount := math.Floor(math.Min(
				craftingOverflowAmount(settings, snapshot, source, resourceID),
				freeAtTarget/kingdomResourceDeliveryRatio,
			))
			if craftingShipmentMeetsMinimum(amount, settings.MinimumShipmentSize) && amount > best.amount {
				best = candidate{source: source, resource: resourceID, amount: amount}
			}
		}
	}
	if best.amount <= 0 {
		return Decision{}, false
	}
	arguments, _ := json.Marshal(map[string]any{
		"sourceCastleId": best.source.ID, "targetKingdomId": storm.KingdomID,
		"resourceId": best.resource, "amount": int64(best.amount),
	})
	return Decision{
		Status:      "ready",
		Detail:      fmt.Sprintf("Move %.0f overflow resource %d from kingdom %d to Storm", best.amount, best.resource, best.source.KingdomID),
		NextCheckAt: snapshot.Now.Add(interval), Metrics: map[string]float64{"shipmentAmount": best.amount},
		Request: &Intent.Request{Name: "resource.kingdom.ship", Arguments: arguments},
	}, true
}

func rubyOverflowSkipDecision(settings craftingSettings, snapshot Snapshot) (Decision, bool) {
	if !settings.AutoKingdomTransport || !settings.UseRubyOverflowSkip {
		return Decision{}, false
	}
	pressured := craftingOverflowPressure(settings, snapshot)
	if len(pressured) == 0 {
		return Decision{}, false
	}
	var main State.CastleState
	for _, castleID := range sortedCastleIDs(snapshot.State.Castles) {
		castle := snapshot.State.Castles[castleID]
		if castle.KingdomID == 0 && castle.SlotType == 1 {
			main = castle
			break
		}
	}
	castleKey := strconv.FormatInt(int64(main.ID), 10)
	castlePlan, planned := settings.Castles[castleKey]
	if main.ID <= 0 || !planned {
		return Decision{}, false
	}
	type candidate struct {
		building  State.CraftingBuilding
		slot      int
		activeID  int64
		nextID    int64
		resource  State.ResourceID
		remaining int
		price     int
	}
	best := candidate{}
	for _, queueKey := range sortedNumericKeys(castlePlan.Buildings) {
		plan := castlePlan.Buildings[queueKey]
		cycle := craftingCycle(plan.Steps)
		if !plan.Enabled || len(cycle) == 0 {
			continue
		}
		queueType, _ := strconv.Atoi(queueKey)
		building, found := craftingBuildingForQueue(main, queueType)
		if !found || len(building.Active)+len(building.Queued) < 2+len(building.ActiveSlotRentals)+len(building.QueueSlotRentals) {
			continue
		}
		cursor := plan.Cursor % len(cycle)
		if cursor < 0 {
			cursor = 0
		}
		nextID := cycle[cursor]
		nextRecipe, found := craftingRecipeRecord(snapshot.GameData, nextID)
		if !found || craftingRecipeIsRuby(nextRecipe) || !craftingRecipeAvailable(nextRecipe, nextID, building, main.Crafting) {
			continue
		}
		pressureResource := craftingPressureResource(snapshot.GameData, nextID, pressured)
		if pressureResource <= 0 {
			continue
		}
		costs, err := craftingRecipeCostState(snapshot, main, nextID, settings)
		if err != nil || costs.Blocked != "" || len(costs.Missing) > 0 {
			continue
		}
		for slot, active := range building.Active {
			activeRecipe, exists := craftingRecipeRecord(snapshot.GameData, active.RecipeID)
			if !exists || craftingRecipeIsRuby(activeRecipe) {
				continue
			}
			remaining := craftingRemainingSeconds(active, building.ObservedAt, snapshot.Now)
			price := craftingRubySkipPrice(activeRecipe, remaining)
			if price <= 0 || best.price > 0 && price >= best.price {
				continue
			}
			best = candidate{
				building: building, slot: slot, activeID: active.RecipeID, nextID: nextID,
				resource: pressureResource, remaining: remaining, price: price,
			}
		}
	}
	if best.price <= 0 || playerResourceAmount(snapshot, "C2")-settings.MinimumRubyReserve < float64(best.price) {
		return Decision{}, false
	}
	arguments, _ := json.Marshal(map[string]any{
		"castleId": main.ID, "buildingInstanceId": best.building.InstanceID,
		"slot": best.slot,
	})
	return Decision{
		Status:      "ready",
		Detail:      fmt.Sprintf("Complete crafting recipe %d for %d rubies so recipe %d can consume overflow resource %d", best.activeID, best.price, best.nextID, best.resource),
		NextCheckAt: snapshot.Now.Add(2 * time.Second),
		Metrics:     map[string]float64{"rubyCost": float64(best.price), "remainingSec": float64(best.remaining)},
		Request:     &Intent.Request{Name: "crafting.skip", Arguments: arguments},
	}, true
}

func craftingOverflowAmount(settings craftingSettings, snapshot Snapshot, castle State.CastleState, resourceID State.ResourceID) float64 {
	balance := castle.Resources[resourceID]
	if balance.Capacity == nil || *balance.Capacity <= 0 {
		return 0
	}
	threshold := *balance.Capacity * float64(clampInt(settings.OverflowThresholdPercent, 50, 100)) / 100
	leaveBehind := math.Max(threshold, protectedCraftingDemand(settings, snapshot, castle.ID, resourceID))
	return math.Max(0, balance.Amount-leaveBehind)
}

func craftingOverflowPressure(settings craftingSettings, snapshot Snapshot) map[State.ResourceID]bool {
	result := map[State.ResourceID]bool{}
	minimum := math.Max(1, float64(settings.MinimumShipmentSize))
	for _, castleID := range sortedCastleIDs(snapshot.State.Castles) {
		castle := snapshot.State.Castles[castleID]
		for _, resourceID := range sovereignResourceIDs(snapshot.GameData) {
			balance := castle.Resources[resourceID]
			if balance.Capacity == nil || *balance.Capacity <= 0 {
				continue
			}
			threshold := *balance.Capacity * float64(clampInt(settings.OverflowThresholdPercent, 50, 100)) / 100
			if balance.Amount-threshold >= minimum {
				result[resourceID] = true
			}
		}
	}
	return result
}

func sovereignResourceIDs(store *GameData.Store) []State.ResourceID {
	if store == nil {
		return nil
	}
	catalog, err := store.Catalog("resources")
	if err != nil {
		return nil
	}
	byKey := map[string]State.ResourceID{}
	for _, raw := range catalog.Rows() {
		record, decodeErr := GameData.DecodeRecord(raw)
		if decodeErr != nil {
			continue
		}
		key, _ := record.String("JSONKey")
		switch strings.ToUpper(strings.TrimSpace(key)) {
		case "C", "O", "G", "I":
			id, _ := record.Int64("resourceID")
			if id > 0 {
				byKey[strings.ToUpper(strings.TrimSpace(key))] = State.ResourceID(id)
			}
		}
	}
	result := make([]State.ResourceID, 0, len(byKey))
	for _, key := range []string{"C", "O", "G", "I"} {
		if id := byKey[key]; id > 0 {
			result = append(result, id)
		}
	}
	return result
}

func craftingShipmentMeetsMinimum(amount float64, minimum int64) bool {
	return amount > 0 && (minimum <= 0 || amount >= float64(minimum))
}

func craftingPressureResource(store *GameData.Store, recipeID int64, pressured map[State.ResourceID]bool) State.ResourceID {
	ids := make([]State.ResourceID, 0, len(pressured))
	for id := range pressured {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(left, right int) bool { return ids[left] < ids[right] })
	for _, id := range ids {
		if craftingRecipeResourceCost(store, recipeID, id) > 0 {
			return id
		}
	}
	return 0
}

func craftingRecipeRecord(store *GameData.Store, recipeID int64) (GameData.Record, bool) {
	if store == nil || recipeID <= 0 {
		return nil, false
	}
	catalog, err := store.Catalog("craftingRecipes")
	if err != nil {
		return nil, false
	}
	raw, exists := catalog.Find(strconv.FormatInt(recipeID, 10))
	if !exists {
		return nil, false
	}
	record, err := GameData.DecodeRecord(raw)
	return record, err == nil
}

func craftingRecipeAvailable(record GameData.Record, recipeID int64, building State.CraftingBuilding, crafting State.CraftingState) bool {
	queueType, _ := record.Int64("queueTypeId")
	if int(queueType) != building.QueueTypeID {
		return false
	}
	if required, _ := record.String("requiredCraftingBuildings"); strings.TrimSpace(required) != "" {
		allowed := false
		for _, value := range strings.FieldsFunc(required, func(character rune) bool { return character == ',' || character == '#' }) {
			id, err := strconv.ParseInt(strings.TrimSpace(value), 10, 64)
			if err == nil && State.BuildingID(id) == building.DefinitionID {
				allowed = true
				break
			}
		}
		if !allowed {
			return false
		}
	}
	researchGroupID, _ := record.Int64("researchGroupID")
	if researchGroupID <= 0 {
		return true
	}
	recipeGroupID, _ := record.Int64("recipeGroupID")
	return containsInt64(crafting.EnabledRecipeIDs, recipeID) || containsInt64(crafting.EnabledRecipeGroupIDs, recipeGroupID)
}

func craftingRecipeIsRuby(record GameData.Record) bool {
	recipeType, _ := record.String("type")
	if strings.EqualFold(strings.TrimSpace(recipeType), "ruby") {
		return true
	}
	for field := range record {
		if strings.EqualFold(field, "costC2") {
			amount, _ := record.Float64(field)
			return amount > 0
		}
	}
	return false
}

func craftingRemainingSeconds(item State.CraftingQueueItem, observedAt time.Time, now time.Time) int {
	if item.RemainingSec == nil {
		return 0
	}
	remaining := *item.RemainingSec
	if !observedAt.IsZero() && now.After(observedAt) {
		remaining -= int(now.Sub(observedAt) / time.Second)
	}
	return max(0, remaining)
}

func craftingRubySkipPrice(record GameData.Record, remainingSec int) int {
	duration, _ := record.Int64("craftingDuration")
	fullPrice, _ := record.Int64("skipCostC2")
	if duration <= 0 || fullPrice <= 0 || remainingSec <= 0 {
		return 0
	}
	remaining := math.Min(float64(remainingSec), float64(duration))
	return int(math.Ceil(remaining / float64(duration) * float64(fullPrice)))
}

func containsInt64(values []int64, target int64) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
