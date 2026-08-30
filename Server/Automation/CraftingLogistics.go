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

const craftingActiveRentalCost = 5_000_000

var craftingQueueRentalCosts = map[int]float64{1: 500_000, 2: 3_000_000, 3: 6_500_000}

type craftingCostEvaluation struct {
	Missing map[State.ResourceID]float64
	Blocked string
}

func craftingRentalDecision(
	settings craftingSettings,
	snapshot Snapshot,
	castle State.CastleState,
	building State.CraftingBuilding,
	plan craftingBuildingPlan,
) (Decision, bool) {
	slotType := ""
	slot := 0
	cost := float64(0)
	if plan.AutoRentActiveSlot && len(building.ActiveSlotRentals) < 1 {
		slotType, slot, cost = "production", len(building.ActiveSlotRentals)+1, craftingActiveRentalCost
	} else {
		desiredQueueSlots := clampInt(plan.AutoRentQueueSlots, 0, 3)
		if len(building.QueueSlotRentals) < desiredQueueSlots {
			slotType, slot = "queue", len(building.QueueSlotRentals)+1
			cost = craftingQueueRentalCosts[slot]
		}
	}
	if slotType == "" || slot <= 0 || cost <= 0 || playerResourceAmount(snapshot, "C1")-settings.MinimumCoinReserve < cost {
		return Decision{}, false
	}
	arguments, _ := json.Marshal(map[string]any{
		"castleId": castle.ID, "buildingInstanceId": building.InstanceID,
		"slotType": slotType, "slot": slot,
	})
	return Decision{
		Status:              "ready",
		Detail:              fmt.Sprintf("Rent %s crafting slot %d at %s", slotType, slot, castleName(castle)),
		NextCheckAt:         snapshot.Now.Add(2 * time.Second),
		Metrics:             map[string]float64{"coinCost": cost},
		Request:             &Intent.Request{Name: "crafting.rent_slot", Arguments: arguments},
		ReevaluateOnSuccess: true,
	}, true
}

func craftingLogisticsStale(snapshot Snapshot, interval time.Duration) (bool, time.Time, error) {
	marketRequired, err := marketLogisticsRequired(snapshot)
	if err != nil {
		return false, time.Time{}, err
	}
	kingdomRequired := kingdomLogisticsRequired(snapshot.State)
	if !marketRequired && !kingdomRequired {
		return false, time.Time{}, nil
	}
	marketStale := false
	if marketRequired {
		if snapshot.State.Market.ObservedAt.IsZero() || !snapshot.State.Market.CaravanLevelLoaded {
			marketStale = true
		} else {
			marketStale = snapshot.Now.Sub(snapshot.State.Market.ObservedAt) >= interval
		}
	}
	kingdomStale := false
	if kingdomRequired {
		if snapshot.State.KingdomTransport.ObservedAt.IsZero() {
			kingdomStale = true
		} else {
			kingdomStale = snapshot.Now.Sub(snapshot.State.KingdomTransport.ObservedAt) >= interval
		}
	}
	if kingdomStale {
		return true, time.Time{}, nil
	}
	if marketStale {
		if releasesAt := State.NextMarketBarrowLeaseRelease(snapshot.State, snapshot.Now); !releasesAt.IsZero() {
			return false, releasesAt, nil
		}
		return true, time.Time{}, nil
	}
	return false, time.Time{}, nil
}

func craftingRecipeCostState(
	snapshot Snapshot,
	castle State.CastleState,
	recipeID int64,
	settings craftingSettings,
) (craftingCostEvaluation, error) {
	result := craftingCostEvaluation{Missing: map[State.ResourceID]float64{}}
	costs, err := GameData.CraftingRecipeCostsView(snapshot.GameData, recipeID)
	if err != nil {
		return result, err
	}
	for _, cost := range costs {
		resourceID := State.ResourceID(cost.ResourceID)
		if resourceID > 0 {
			available := castle.Resources[resourceID].Amount
			reserve := float64(0)
			switch strings.ToUpper(cost.JSONKey) {
			case "C1":
				available = snapshot.State.Player.Resources[resourceID]
				reserve = settings.MinimumCoinReserve
			case "C2":
				if !settings.AllowRubyRecipes {
					result.Blocked = "Ruby recipes are disabled"
					continue
				}
				available = snapshot.State.Player.Resources[resourceID]
				reserve = settings.MinimumRubyReserve
			}
			if available-reserve < cost.Amount && transportableResource(cost.JSONKey) {
				result.Missing[resourceID] = cost.Amount - math.Max(0, available-reserve)
			} else if available-reserve < cost.Amount {
				result.Blocked = fmt.Sprintf("Insufficient %s above its configured reserve", cost.JSONKey)
			}
			continue
		}
		currencyID := State.CurrencyID(cost.CurrencyID)
		if currencyID > 0 && snapshot.State.Player.Currencies[currencyID] < cost.Amount {
			result.Blocked = fmt.Sprintf("Insufficient %s currency", cost.JSONKey)
		}
	}
	return result, nil
}

func craftingTransportDecision(
	settings craftingSettings,
	snapshot Snapshot,
	target State.CastleState,
	missing map[State.ResourceID]float64,
	interval time.Duration,
) (Decision, bool) {
	resourceIDs := make([]State.ResourceID, 0, len(missing))
	for id := range missing {
		resourceIDs = append(resourceIDs, id)
	}
	sort.Slice(resourceIDs, func(left, right int) bool { return resourceIDs[left] < resourceIDs[right] })
	for _, resourceID := range resourceIDs {
		shortfall := missing[resourceID]
		incoming := incomingMarketResource(snapshot, target, resourceID)
		if incoming >= shortfall {
			return Decision{
				Status: "waiting", Detail: fmt.Sprintf("An in-flight market shipment covers resource %d at %s", resourceID, castleName(target)),
				NextCheckAt: nextMarketArrival(snapshot, target, resourceID, interval),
			}, true
		}
		shortfall -= incoming
		if decision, ready := sameKingdomShipmentDecision(settings, snapshot, target, resourceID, shortfall, interval); ready {
			return decision, true
		}
		if decision, handled := crossKingdomShipmentDecision(settings, snapshot, target, resourceID, shortfall, interval); handled {
			return decision, true
		}
	}
	return Decision{}, false
}

type craftingLootDrainCandidate struct {
	source          State.CastleState
	target          State.CastleState
	resource        State.ResourceID
	amount          float64
	delivered       float64
	targetDemand    float64
	targetBalance   float64
	targetCoverage  float64
	preferredSource bool
}

// craftingLootDrainDecision keeps logistics independent from queue admission.
// Queue starts still require a free slot, while sovereign resources that arrive
// at donor castles can fund one complete future refill of every configured
// crafting queue even when those queues are currently full.
func craftingLootDrainDecision(settings craftingSettings, snapshot Snapshot) (Decision, bool) {
	demands := craftingRefillDemands(settings, snapshot)
	if len(demands) == 0 {
		return Decision{}, false
	}
	best := craftingLootDrainCandidate{}
	for _, resourceID := range sovereignResourceIDs(snapshot.GameData) {
		// State.Castles is the player's owned-castle roster. Keep every slot type
		// eligible: the four crafting castles are donors/storage too, alongside
		// outposts, capitals, metropolises, and Storm.
		for _, sourceID := range sortedCastleIDs(snapshot.State.Castles) {
			source := snapshot.State.Castles[sourceID]
			available := craftingLootDrainAvailable(settings, source, resourceID, demands[source.ID][resourceID])
			if available <= 0 {
				continue
			}
			for _, targetID := range sortedCastleIDs(snapshot.State.Castles) {
				target := snapshot.State.Castles[targetID]
				targetDemand := demands[target.ID][resourceID]
				if target.ID == source.ID || targetDemand <= 0 {
					continue
				}
				balance := target.Resources[resourceID]
				if balance.Capacity == nil || *balance.Capacity <= 0 {
					continue
				}
				incoming := incomingMarketResource(snapshot, target, resourceID)
				targetGoal := math.Min(targetDemand, *balance.Capacity)
				targetBalance := balance.Amount + incoming
				shortfall := targetGoal - targetBalance
				if shortfall <= 0 {
					continue
				}
				candidate, ready := craftingLootDrainRoute(
					settings, snapshot, source, target, resourceID, available, shortfall, targetDemand, targetBalance,
				)
				candidate.preferredSource = craftingLootDrainPreferredSource(snapshot.GameData, resourceID, source.KingdomID)
				if ready && betterCraftingLootDrainCandidate(candidate, best) {
					best = candidate
				}
			}
		}
	}
	if best.amount <= 0 {
		return Decision{}, false
	}
	shipmentArguments := map[string]any{
		"sourceCastleId": best.source.ID, "targetCastleId": best.target.ID,
		"resourceId": best.resource, "amount": int64(best.amount),
	}
	if best.source.KingdomID != best.target.KingdomID {
		shipmentArguments["workflowOwner"] = autoSceatTransportOwner
	}
	arguments, _ := json.Marshal(shipmentArguments)
	return Decision{
		Status: "ready",
		Detail: fmt.Sprintf(
			"Drain %.0f resource %d from %s into %s's configured crafting buffer",
			best.amount, best.resource, castleName(best.source), castleName(best.target),
		),
		NextCheckAt: snapshot.Now.Add(2 * time.Second),
		Metrics: map[string]float64{
			"shipmentAmount": best.amount, "projectedDelivery": best.delivered,
			"targetBufferDemand": best.targetDemand, "targetBalance": best.targetBalance,
		},
		Request:             &Intent.Request{Name: "resource.ship", Arguments: arguments},
		ReevaluateOnSuccess: true,
	}, true
}

func craftingRefillDemands(settings craftingSettings, snapshot Snapshot) map[State.CastleID]map[State.ResourceID]float64 {
	result := map[State.CastleID]map[State.ResourceID]float64{}
	resourceIDs := sovereignResourceIDs(snapshot.GameData)
	costsByRecipe := map[int64]map[State.ResourceID]float64{}
	recipeCosts := func(recipeID int64) map[State.ResourceID]float64 {
		if costs, cached := costsByRecipe[recipeID]; cached {
			return costs
		}
		costs := map[State.ResourceID]float64{}
		for _, resourceID := range resourceIDs {
			if amount := craftingRecipeResourceCost(snapshot.GameData, recipeID, resourceID); amount > 0 {
				costs[resourceID] = amount
			}
		}
		costsByRecipe[recipeID] = costs
		return costs
	}
	for _, castleKey := range sortedNumericKeys(settings.Castles) {
		castleIDValue, _ := strconv.ParseInt(castleKey, 10, 64)
		castle, exists := snapshot.State.Castles[State.CastleID(castleIDValue)]
		if !exists || !castle.SupportsSovereignCrafting() {
			continue
		}
		for _, queueKey := range sortedNumericKeys(settings.Castles[castleKey].Buildings) {
			plan := settings.Castles[castleKey].Buildings[queueKey]
			cycle := craftingCycle(plan.Steps)
			if !plan.Enabled || len(cycle) == 0 {
				continue
			}
			queueType, _ := strconv.Atoi(queueKey)
			building, found := craftingBuildingForQueue(castle, queueType)
			if !found {
				continue
			}
			capacity := 2 + len(building.ActiveSlotRentals) + len(building.QueueSlotRentals)
			cursor := plan.Cursor % len(cycle)
			if cursor < 0 {
				cursor = 0
			}
			for offset := 0; offset < capacity; offset++ {
				configuredID := cycle[(cursor+offset)%len(cycle)]
				recipeID := resolveCraftingRecipe(snapshot, configuredID, building, castle.Crafting)
				if recipeID <= 0 {
					continue
				}
				for resourceID, amount := range recipeCosts(recipeID) {
					if result[castle.ID] == nil {
						result[castle.ID] = map[State.ResourceID]float64{}
					}
					result[castle.ID][resourceID] += amount
				}
			}
		}
	}
	return result
}

func craftingLootDrainAvailable(
	settings craftingSettings,
	source State.CastleState,
	resourceID State.ResourceID,
	localDemand float64,
) float64 {
	balance := source.Resources[resourceID]
	if balance.Capacity == nil || *balance.Capacity <= 0 {
		return 0
	}
	reserve := *balance.Capacity * float64(clampInt(settings.SourceReservePercent, 0, 95)) / 100
	return math.Floor(math.Max(0, balance.Amount-math.Max(reserve, localDemand)))
}

func craftingLootDrainRoute(
	settings craftingSettings,
	snapshot Snapshot,
	source State.CastleState,
	target State.CastleState,
	resourceID State.ResourceID,
	available float64,
	shortfall float64,
	targetDemand float64,
	targetBalance float64,
) (craftingLootDrainCandidate, bool) {
	minimum := math.Max(0, float64(settings.MinimumShipmentSize))
	targetCapacity := *target.Resources[resourceID].Capacity
	targetFree := math.Max(0, targetCapacity-targetBalance)
	requested := shortfall
	deliveredRatio := 1.0
	capacityPerBarrow := 0
	if source.KingdomID == target.KingdomID {
		if !craftingHasMarketplace(snapshot.GameData, source) {
			return craftingLootDrainCandidate{}, false
		}
		market, observed := snapshot.State.Market.Castles[source.ID]
		availableBarrows := State.AvailableMarketBarrowsAt(snapshot.State, market, snapshot.Now)
		if !observed || availableBarrows <= 0 {
			return craftingLootDrainCandidate{}, false
		}
		capacityPerBarrow = marketCapacityPerBarrow(snapshot, market)
		if capacityPerBarrow <= 0 {
			return craftingLootDrainCandidate{}, false
		}
		shipmentCapacity := float64(availableBarrows * capacityPerBarrow)
		requested = math.Min(requested, shipmentCapacity)
	} else {
		if source.KingdomID == 4 && !settings.UseStormBuffer {
			return craftingLootDrainCandidate{}, false
		}
		if _, pending := pendingKingdomResourceTransport(snapshot.State, target.KingdomID); pending {
			return craftingLootDrainCandidate{}, false
		}
		if _, settling := kingdomResourceTransportWorkflow(snapshot.State, target.KingdomID); settling {
			return craftingLootDrainCandidate{}, false
		}
		unlock, observed := snapshot.State.KingdomTransport.Unlocks[target.KingdomID]
		if !observed || !unlock.Unlocked {
			return craftingLootDrainCandidate{}, false
		}
		deliveredRatio = kingdomResourceDeliveryRatio
		requested = math.Ceil(shortfall / deliveredRatio)
		if requested < minimum {
			requested = minimum
		}
		targetFree /= deliveredRatio
	}
	amount := math.Floor(math.Min(requested, math.Min(available, targetFree)))
	if amount <= 0 || source.KingdomID != target.KingdomID &&
		!craftingShipmentMeetsMinimum(amount, settings.MinimumShipmentSize) {
		return craftingLootDrainCandidate{}, false
	}
	coverage := targetBalance / targetDemand
	return craftingLootDrainCandidate{
		source: source, target: target, resource: resourceID,
		amount: amount, delivered: amount * deliveredRatio,
		targetDemand: targetDemand, targetBalance: targetBalance, targetCoverage: coverage,
	}, true
}

func betterCraftingLootDrainCandidate(candidate craftingLootDrainCandidate, current craftingLootDrainCandidate) bool {
	if current.amount <= 0 {
		return true
	}
	if candidate.preferredSource != current.preferredSource {
		return candidate.preferredSource
	}
	if candidate.targetCoverage != current.targetCoverage {
		return candidate.targetCoverage < current.targetCoverage
	}
	if candidate.delivered != current.delivered {
		return candidate.delivered > current.delivered
	}
	if candidate.resource != current.resource {
		return candidate.resource < current.resource
	}
	if candidate.source.ID != current.source.ID {
		return candidate.source.ID < current.source.ID
	}
	return candidate.target.ID < current.target.ID
}

func craftingLootDrainPreferredSource(store *GameData.Store, resourceID State.ResourceID, kingdomID State.KingdomID) bool {
	switch strings.ToUpper(resourceJSONKey(store, resourceID)) {
	case "C":
		return kingdomID == 2
	case "O":
		return kingdomID == 1
	case "G":
		return kingdomID == 3
	default:
		return false
	}
}

func sameKingdomShipmentDecision(
	settings craftingSettings,
	snapshot Snapshot,
	target State.CastleState,
	resourceID State.ResourceID,
	shortfall float64,
	interval time.Duration,
) (Decision, bool) {
	best := State.CastleState{}
	bestAvailable := float64(0)
	for _, sourceID := range sortedCastleIDs(snapshot.State.Castles) {
		source := snapshot.State.Castles[sourceID]
		if source.ID == target.ID || source.KingdomID != target.KingdomID {
			continue
		}
		if !craftingHasMarketplace(snapshot.GameData, source) {
			continue
		}
		market, observed := snapshot.State.Market.Castles[source.ID]
		availableBarrows := State.AvailableMarketBarrowsAt(snapshot.State, market, snapshot.Now)
		if !observed || availableBarrows <= 0 {
			continue
		}
		capacityPerBarrow := marketCapacityPerBarrow(snapshot, market)
		if capacityPerBarrow <= 0 {
			continue
		}
		available := sourceAvailableResource(settings, snapshot, source, resourceID)
		shipmentCapacity := float64(availableBarrows * capacityPerBarrow)
		if candidate := math.Min(available, shipmentCapacity); candidate > bestAvailable {
			best, bestAvailable = source, candidate
		}
	}
	if best.ID <= 0 || bestAvailable <= 0 {
		return Decision{}, false
	}
	amount := math.Min(shortfall, bestAvailable)
	if balance := target.Resources[resourceID]; balance.Capacity != nil {
		amount = math.Min(amount, math.Max(0, *balance.Capacity-balance.Amount))
	}
	amount = math.Floor(amount)
	if amount <= 0 {
		return Decision{}, false
	}
	arguments, _ := json.Marshal(map[string]any{
		"sourceCastleId": best.ID, "targetCastleId": target.ID,
		"resourceId": resourceID, "amount": int64(amount),
	})
	return Decision{
		Status:              "ready",
		Detail:              fmt.Sprintf("Ship %.0f resource %d from %s to %s", amount, resourceID, castleName(best), castleName(target)),
		NextCheckAt:         snapshot.Now.Add(2 * time.Second),
		Metrics:             map[string]float64{"shipmentAmount": amount},
		Request:             &Intent.Request{Name: "resource.ship", Arguments: arguments},
		ReevaluateOnSuccess: true,
	}, true
}

func crossKingdomShipmentDecision(
	settings craftingSettings,
	snapshot Snapshot,
	target State.CastleState,
	resourceID State.ResourceID,
	shortfall float64,
	interval time.Duration,
) (Decision, bool) {
	if pending, found := pendingKingdomResourceTransport(snapshot.State, target.KingdomID); found {
		detail := fmt.Sprintf("Kingdom %d already has a resource shipment in flight", target.KingdomID)
		nextCheck := snapshot.Now.Add(coordinatorTick)
		if pending.RemainingSec > 0 {
			nextCheck = snapshot.Now.Add(time.Duration(pending.RemainingSec) * time.Second)
		} else {
			detail = fmt.Sprintf("Waiting for the kingdom %d resource shipment to settle", target.KingdomID)
		}
		return Decision{Status: "waiting", Detail: detail, NextCheckAt: nextCheck}, true
	}
	if workflow, found := kingdomResourceTransportWorkflow(snapshot.State, target.KingdomID); found {
		return Decision{
			Status:      "waiting",
			Detail:      fmt.Sprintf("Waiting for %s to refresh the kingdom %d resource destination", workflow.Owner, workflow.KingdomID),
			NextCheckAt: snapshot.Now.Add(coordinatorTick),
		}, true
	}
	unlock, observed := snapshot.State.KingdomTransport.Unlocks[target.KingdomID]
	if !observed || !unlock.Unlocked {
		return Decision{}, false
	}
	best := State.CastleState{}
	bestAvailable := float64(0)
	resourceKey := resourceJSONKey(snapshot.GameData, resourceID)
	preferredKingdom := map[string]State.KingdomID{"C": 2, "O": 1, "G": 3}[strings.ToUpper(resourceKey)]
	preferredFound := false
	for _, sourceID := range sortedCastleIDs(snapshot.State.Castles) {
		source := snapshot.State.Castles[sourceID]
		if source.KingdomID == target.KingdomID || source.ID == target.ID || source.KingdomID == 4 && !settings.UseStormBuffer {
			continue
		}
		available := sourceAvailableResource(settings, snapshot, source, resourceID)
		preferred := preferredKingdom > 0 && source.KingdomID == preferredKingdom
		if preferred && !preferredFound && available > 0 {
			preferredFound, best, bestAvailable = true, source, available
			continue
		}
		if preferredFound && !preferred {
			continue
		}
		if available > bestAvailable {
			best, bestAvailable = source, available
		}
	}
	if best.ID <= 0 || bestAvailable <= 0 {
		return Decision{}, false
	}
	amount := math.Ceil(shortfall / kingdomResourceDeliveryRatio)
	if amount < float64(settings.MinimumShipmentSize) && bestAvailable >= float64(settings.MinimumShipmentSize) {
		amount = float64(settings.MinimumShipmentSize)
	}
	if balance := target.Resources[resourceID]; balance.Capacity != nil {
		amount = math.Min(amount, math.Max(0, (*balance.Capacity-balance.Amount)/kingdomResourceDeliveryRatio))
	}
	amount = math.Floor(math.Min(amount, bestAvailable))
	if amount <= 0 || settings.MinimumShipmentSize > 0 && amount < float64(settings.MinimumShipmentSize) {
		return Decision{}, false
	}
	shipmentArguments := map[string]any{
		"sourceCastleId": best.ID, "targetCastleId": target.ID,
		"resourceId": resourceID, "amount": int64(amount), "workflowOwner": autoSceatTransportOwner,
	}
	arguments, _ := json.Marshal(shipmentArguments)
	return Decision{
		Status:              "ready",
		Detail:              fmt.Sprintf("Ship %.0f resource %d from kingdom %d to %d", amount, resourceID, best.KingdomID, target.KingdomID),
		NextCheckAt:         snapshot.Now.Add(2 * time.Second),
		Metrics:             map[string]float64{"shipmentAmount": amount},
		Request:             &Intent.Request{Name: "resource.ship", Arguments: arguments},
		ReevaluateOnSuccess: true,
	}, true
}

func sourceAvailableResource(settings craftingSettings, snapshot Snapshot, source State.CastleState, resourceID State.ResourceID) float64 {
	balance := source.Resources[resourceID]
	reserve := float64(0)
	if balance.Capacity != nil {
		reserve = *balance.Capacity * float64(clampInt(settings.SourceReservePercent, 0, 95)) / 100
	}
	protected := protectedCraftingDemand(settings, snapshot, source.ID, resourceID)
	return math.Max(0, balance.Amount-math.Max(reserve, protected))
}

func protectedCraftingDemand(settings craftingSettings, snapshot Snapshot, castleID State.CastleID, resourceID State.ResourceID) float64 {
	castlePlan, exists := settings.Castles[strconv.FormatInt(int64(castleID), 10)]
	if !exists {
		return 0
	}
	castle, exists := snapshot.State.Castles[castleID]
	if !exists {
		return 0
	}
	protected := float64(0)
	for queueKey, plan := range castlePlan.Buildings {
		if !plan.Enabled {
			continue
		}
		cycle := craftingCycle(plan.Steps)
		if len(cycle) == 0 {
			continue
		}
		cursor := plan.Cursor % len(cycle)
		if cursor < 0 {
			cursor = 0
		}
		queueType, _ := strconv.Atoi(queueKey)
		building, buildingExists := craftingBuildingForQueue(castle, queueType)
		if !buildingExists || !craftingRecipeMatches(snapshot, cycle[cursor], building, castle.Crafting) {
			continue
		}
		if amount := craftingRecipeResourceCost(snapshot.GameData, cycle[cursor], resourceID); amount > 0 {
			protected += amount
		}
	}
	return protected
}

func craftingRecipeResourceCost(store *GameData.Store, recipeID int64, resourceID State.ResourceID) float64 {
	costs, err := GameData.CraftingRecipeCostsView(store, recipeID)
	if err != nil {
		return 0
	}
	for _, cost := range costs {
		if State.ResourceID(cost.ResourceID) == resourceID {
			return cost.Amount
		}
	}
	return 0
}

func incomingMarketResource(snapshot Snapshot, target State.CastleState, resourceID State.ResourceID) float64 {
	amount := float64(0)
	snapshot.State.RangeMovements(func(_ State.MovementID, movement State.MovementState) bool {
		if movement.Direction != 0 || movement.KingdomID != target.KingdomID || movement.TargetX != target.X || movement.TargetY != target.Y {
			return true
		}
		if movement.ArrivesAt != nil && !movement.ArrivesAt.After(snapshot.Now) {
			return true
		}
		for _, good := range movement.MarketGoods {
			if good.ResourceID == resourceID {
				amount += good.Amount
			}
		}
		return true
	})
	return amount
}

func nextMarketArrival(snapshot Snapshot, target State.CastleState, resourceID State.ResourceID, fallback time.Duration) time.Time {
	next := snapshot.Now.Add(fallback)
	snapshot.State.RangeMovements(func(_ State.MovementID, movement State.MovementState) bool {
		if movement.Direction != 0 || movement.KingdomID != target.KingdomID || movement.TargetX != target.X || movement.TargetY != target.Y || movement.ArrivesAt == nil {
			return true
		}
		for _, good := range movement.MarketGoods {
			if good.ResourceID == resourceID && movement.ArrivesAt.Before(next) && movement.ArrivesAt.After(snapshot.Now) {
				next = *movement.ArrivesAt
			}
		}
		return true
	})
	return next
}

func craftingHasMarketplace(store *GameData.Store, castle State.CastleState) bool {
	for _, building := range castle.Buildings {
		barrows, err := store.MarketBarrowsForBuilding(int64(building.DefinitionID))
		if err == nil && barrows > 0 {
			return true
		}
	}
	return false
}

func marketCapacityPerBarrow(snapshot Snapshot, market State.MarketCastleState) int {
	if snapshot.GameData == nil || !snapshot.State.Market.CaravanLevelLoaded {
		return 0
	}
	effects := make([]GameData.MarketEffect, 0, len(market.AreaEffects))
	for _, effect := range market.AreaEffects {
		effects = append(effects, GameData.MarketEffect{EffectID: effect.EffectID, Values: effect.Values})
	}
	capacity, err := snapshot.GameData.MarketCapacity(snapshot.State.Market.CaravanLevel, effects)
	if err != nil {
		return 0
	}
	return capacity.CapacityPerBarrow
}

func marketShipmentCoinCost(source State.CastleState, target State.CastleState, barrows int) float64 {
	if barrows <= 0 {
		return 0
	}
	distance := math.Hypot(float64(source.X-target.X), float64(source.Y-target.Y))
	if distance <= 0 {
		return 0
	}
	return math.Ceil(3 * float64(barrows) * math.Log(distance+1) / math.Log(2.3))
}

func playerResourceAmount(snapshot Snapshot, jsonKey string) float64 {
	if snapshot.GameData == nil {
		return 0
	}
	id, found := snapshot.GameData.ResourceIDForJSONKey(jsonKey)
	if !found {
		return 0
	}
	return snapshot.State.Player.Resources[State.ResourceID(id)]
}

func currencyIDForJSONKey(store *GameData.Store, jsonKey string) State.CurrencyID {
	if store == nil {
		return 0
	}
	id, found := store.CurrencyIDForJSONKey(jsonKey)
	if !found {
		return 0
	}
	return State.CurrencyID(id)
}

func resourceJSONKey(store *GameData.Store, resourceID State.ResourceID) string {
	if store == nil || resourceID <= 0 {
		return ""
	}
	value, _ := store.ResourceJSONKey(int64(resourceID))
	return value
}

func transportableResource(jsonKey string) bool {
	switch strings.ToUpper(strings.TrimSpace(jsonKey)) {
	case "W", "S", "C", "O", "G", "I":
		return true
	default:
		return false
	}
}
