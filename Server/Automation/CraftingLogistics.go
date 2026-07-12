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

const kingdomResourceDeliveryRatio = 0.90

const craftingActiveRentalCost = 5_000_000

var craftingQueueRentalCosts = map[int]float64{1: 500_000, 2: 3_000_000, 3: 6_500_000}

var kingdomTimeSkipSeconds = map[string]int{
	"MS1": 60, "MS2": 300, "MS3": 600, "MS4": 1_800,
	"MS5": 3_600, "MS6": 18_000, "MS7": 86_400,
}

type craftingCostDefinition struct {
	ResourceID State.ResourceID
	CurrencyID State.CurrencyID
	JSONKey    string
}

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
		Status: "ready", Detail: fmt.Sprintf("Rent %s crafting slot %d at %s", slotType, slot, castleName(castle)),
		NextCheckAt: snapshot.Now.Add(2 * time.Second), Metrics: map[string]float64{"coinCost": cost},
		Request: &Intent.Request{Name: "crafting.rent_slot", Arguments: arguments},
	}, true
}

func craftingLogisticsStale(snapshot Snapshot, interval time.Duration) bool {
	if snapshot.State.Market.ObservedAt.IsZero() || snapshot.State.KingdomTransport.ObservedAt.IsZero() || !snapshot.State.Market.CaravanLevelLoaded {
		return true
	}
	oldest := snapshot.State.Market.ObservedAt
	if snapshot.State.KingdomTransport.ObservedAt.Before(oldest) {
		oldest = snapshot.State.KingdomTransport.ObservedAt
	}
	return snapshot.Now.Sub(oldest) >= interval
}

func pendingKingdomSkipDecision(settings craftingSettings, snapshot Snapshot, interval time.Duration) (Decision, bool) {
	if !settings.UseKingdomTimeSkips || len(settings.AllowedTimeSkips) == 0 {
		return Decision{}, false
	}
	pending := append([]State.KingdomResourceTransport(nil), snapshot.State.KingdomTransport.Pending...)
	sort.Slice(pending, func(left, right int) bool { return pending[left].RemainingSec < pending[right].RemainingSec })
	for _, transport := range pending {
		skipID := chooseKingdomTimeSkip(settings, snapshot, transport.RemainingSec)
		if skipID == "" {
			continue
		}
		arguments, _ := json.Marshal(map[string]any{"targetKingdomId": transport.KingdomID, "timeSkipId": skipID})
		return Decision{
			Status: "ready", Detail: fmt.Sprintf("Apply %s to the kingdom %d resource shipment", skipID, transport.KingdomID),
			NextCheckAt: snapshot.Now.Add(2 * time.Second),
			Request:     &Intent.Request{Name: "resource.kingdom.skip", Arguments: arguments},
		}, true
	}
	return Decision{NextCheckAt: snapshot.Now.Add(interval)}, false
}

func chooseKingdomTimeSkip(settings craftingSettings, snapshot Snapshot, remainingSec int) string {
	if remainingSec <= 0 || snapshot.GameData == nil {
		return ""
	}
	type candidate struct {
		id      string
		seconds int
	}
	covering := []candidate{}
	partial := []candidate{}
	seen := map[string]struct{}{}
	for _, rawID := range settings.AllowedTimeSkips {
		id := strings.ToUpper(strings.TrimSpace(rawID))
		seconds := kingdomTimeSkipSeconds[id]
		if seconds <= 0 {
			continue
		}
		if _, duplicate := seen[id]; duplicate {
			continue
		}
		seen[id] = struct{}{}
		currencyID := currencyIDForJSONKey(snapshot.GameData, id)
		if currencyID <= 0 || snapshot.State.Player.Currencies[currencyID] <= float64(settings.TimeSkipReserve[id]) {
			continue
		}
		entry := candidate{id: id, seconds: seconds}
		if seconds >= remainingSec {
			covering = append(covering, entry)
		} else {
			partial = append(partial, entry)
		}
	}
	if len(covering) > 0 {
		sort.Slice(covering, func(left, right int) bool { return covering[left].seconds < covering[right].seconds })
		return covering[0].id
	}
	if len(partial) > 0 {
		sort.Slice(partial, func(left, right int) bool { return partial[left].seconds > partial[right].seconds })
		return partial[0].id
	}
	return ""
}

func craftingRecipeCostState(
	snapshot Snapshot,
	castle State.CastleState,
	recipeID int64,
	settings craftingSettings,
) (craftingCostEvaluation, error) {
	result := craftingCostEvaluation{Missing: map[State.ResourceID]float64{}}
	definitions, err := craftingCostDefinitions(snapshot.GameData)
	if err != nil {
		return result, err
	}
	catalog, err := snapshot.GameData.Catalog("craftingRecipes")
	if err != nil {
		return result, err
	}
	raw, exists := catalog.Find(strconv.FormatInt(recipeID, 10))
	if !exists {
		return result, fmt.Errorf("crafting recipe %d is not in the official catalog", recipeID)
	}
	record, err := GameData.DecodeRecord(raw)
	if err != nil {
		return result, err
	}
	for field := range record {
		if !strings.HasPrefix(field, "cost") || field == "cost" {
			continue
		}
		amount, valid := record.Float64(field)
		if !valid || amount <= 0 {
			continue
		}
		definition := definitions[strings.ToLower(strings.TrimPrefix(field, "cost"))]
		if definition.ResourceID > 0 {
			available := castle.Resources[definition.ResourceID].Amount
			reserve := float64(0)
			switch strings.ToUpper(definition.JSONKey) {
			case "C1":
				available = snapshot.State.Player.Resources[definition.ResourceID]
				reserve = settings.MinimumCoinReserve
			case "C2":
				if !settings.AllowRubyRecipes {
					result.Blocked = "Ruby recipes are disabled"
					continue
				}
				available = snapshot.State.Player.Resources[definition.ResourceID]
				reserve = settings.MinimumRubyReserve
			}
			if available-reserve < amount && transportableResource(definition.JSONKey) {
				result.Missing[definition.ResourceID] = amount - math.Max(0, available-reserve)
			} else if available-reserve < amount {
				result.Blocked = fmt.Sprintf("Insufficient %s above its configured reserve", definition.JSONKey)
			}
			continue
		}
		if definition.CurrencyID > 0 && snapshot.State.Player.Currencies[definition.CurrencyID] < amount {
			result.Blocked = fmt.Sprintf("Insufficient %s currency", definition.JSONKey)
		}
	}
	return result, nil
}

func craftingCostDefinitions(store *GameData.Store) (map[string]craftingCostDefinition, error) {
	if store == nil {
		return nil, fmt.Errorf("official game data is unavailable")
	}
	result := map[string]craftingCostDefinition{}
	for _, collection := range []string{"resources", "currencies"} {
		catalog, err := store.Catalog(collection)
		if err != nil {
			return nil, err
		}
		for _, raw := range catalog.Rows() {
			record, decodeErr := GameData.DecodeRecord(raw)
			if decodeErr != nil {
				continue
			}
			jsonKey, _ := record.String("JSONKey")
			nameField := "name"
			idField := "resourceID"
			definition := craftingCostDefinition{JSONKey: jsonKey}
			if collection == "resources" {
				id, _ := record.Int64(idField)
				definition.ResourceID = State.ResourceID(id)
			} else {
				nameField, idField = "Name", "currencyID"
				id, _ := record.Int64(idField)
				definition.CurrencyID = State.CurrencyID(id)
			}
			name, _ := record.String(nameField)
			assetName, _ := record.String("assetName")
			for _, alias := range []string{jsonKey, name, assetName} {
				alias = strings.ToLower(strings.TrimSpace(alias))
				if alias != "" {
					result[alias] = definition
				}
			}
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
	bestCapacityPerBarrow := 0
	for _, sourceID := range sortedCastleIDs(snapshot.State.Castles) {
		source := snapshot.State.Castles[sourceID]
		if source.ID == target.ID || source.KingdomID != target.KingdomID {
			continue
		}
		market, observed := snapshot.State.Market.Castles[source.ID]
		if !observed || market.AvailableBarrows <= 0 {
			continue
		}
		capacityPerBarrow := marketCapacityPerBarrow(snapshot, market)
		if capacityPerBarrow <= 0 {
			continue
		}
		available := sourceAvailableResource(settings, snapshot, source, resourceID)
		shipmentCapacity := float64(market.AvailableBarrows * capacityPerBarrow)
		if candidate := math.Min(available, shipmentCapacity); candidate > bestAvailable {
			best, bestAvailable, bestCapacityPerBarrow = source, candidate, capacityPerBarrow
		}
	}
	if best.ID <= 0 || bestAvailable <= 0 {
		return Decision{}, false
	}
	amount := math.Max(shortfall, float64(settings.MinimumShipmentSize))
	amount = math.Min(amount, bestAvailable)
	if balance := target.Resources[resourceID]; balance.Capacity != nil {
		amount = math.Min(amount, math.Max(0, *balance.Capacity-balance.Amount))
	}
	amount = math.Floor(amount)
	if amount <= 0 || settings.MinimumShipmentSize > 0 && amount < float64(settings.MinimumShipmentSize) {
		return Decision{}, false
	}
	barrows := int(math.Ceil(amount / float64(bestCapacityPerBarrow)))
	coinCost := marketShipmentCoinCost(best, target, barrows)
	if playerResourceAmount(snapshot, "C1")-settings.MinimumCoinReserve < coinCost {
		return Decision{}, false
	}
	arguments, _ := json.Marshal(map[string]any{
		"sourceCastleId": best.ID, "targetCastleId": target.ID,
		"resourceId": resourceID, "amount": int64(amount),
	})
	return Decision{
		Status: "ready", Detail: fmt.Sprintf("Ship %.0f resource %d from %s to %s", amount, resourceID, castleName(best), castleName(target)),
		NextCheckAt: snapshot.Now.Add(2 * time.Second), Metrics: map[string]float64{"shipmentAmount": amount, "coinCost": coinCost},
		Request: &Intent.Request{Name: "resource.market.ship", Arguments: arguments},
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
	for _, pending := range snapshot.State.KingdomTransport.Pending {
		if pending.KingdomID != target.KingdomID || pending.RemainingSec <= 0 {
			continue
		}
		return Decision{
			Status: "waiting", Detail: fmt.Sprintf("Kingdom %d already has a resource shipment in flight", target.KingdomID),
			NextCheckAt: snapshot.Now.Add(time.Duration(pending.RemainingSec) * time.Second),
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
	arguments, _ := json.Marshal(map[string]any{
		"sourceCastleId": best.ID, "targetKingdomId": target.KingdomID,
		"resourceId": resourceID, "amount": int64(amount),
	})
	return Decision{
		Status: "ready", Detail: fmt.Sprintf("Ship %.0f resource %d from kingdom %d to %d", amount, resourceID, best.KingdomID, target.KingdomID),
		NextCheckAt: snapshot.Now.Add(2 * time.Second), Metrics: map[string]float64{"shipmentAmount": amount},
		Request: &Intent.Request{Name: "resource.kingdom.ship", Arguments: arguments},
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
		if !craftingRecipeMatches(snapshot, cycle[cursor], queueType) {
			continue
		}
		if amount := craftingRecipeResourceCost(snapshot.GameData, cycle[cursor], resourceID); amount > 0 {
			protected += amount
		}
	}
	return protected
}

func craftingRecipeResourceCost(store *GameData.Store, recipeID int64, resourceID State.ResourceID) float64 {
	definitions, err := craftingCostDefinitions(store)
	if err != nil {
		return 0
	}
	catalog, err := store.Catalog("craftingRecipes")
	if err != nil {
		return 0
	}
	raw, exists := catalog.Find(strconv.FormatInt(recipeID, 10))
	if !exists {
		return 0
	}
	record, err := GameData.DecodeRecord(raw)
	if err != nil {
		return 0
	}
	for field := range record {
		definition := definitions[strings.ToLower(strings.TrimPrefix(field, "cost"))]
		if strings.HasPrefix(field, "cost") && definition.ResourceID == resourceID {
			amount, _ := record.Float64(field)
			return amount
		}
	}
	return 0
}

func incomingMarketResource(snapshot Snapshot, target State.CastleState, resourceID State.ResourceID) float64 {
	amount := float64(0)
	for _, movement := range snapshot.State.Movements {
		if movement.Direction != 0 || movement.KingdomID != target.KingdomID || movement.TargetX != target.X || movement.TargetY != target.Y {
			continue
		}
		if movement.ArrivesAt != nil && !movement.ArrivesAt.After(snapshot.Now) {
			continue
		}
		for _, good := range movement.MarketGoods {
			if good.ResourceID == resourceID {
				amount += good.Amount
			}
		}
	}
	return amount
}

func nextMarketArrival(snapshot Snapshot, target State.CastleState, resourceID State.ResourceID, fallback time.Duration) time.Time {
	next := snapshot.Now.Add(fallback)
	for _, movement := range snapshot.State.Movements {
		if movement.Direction != 0 || movement.KingdomID != target.KingdomID || movement.TargetX != target.X || movement.TargetY != target.Y || movement.ArrivesAt == nil {
			continue
		}
		for _, good := range movement.MarketGoods {
			if good.ResourceID == resourceID && movement.ArrivesAt.Before(next) && movement.ArrivesAt.After(snapshot.Now) {
				next = *movement.ArrivesAt
			}
		}
	}
	return next
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
	catalog, err := snapshot.GameData.Catalog("resources")
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
		return snapshot.State.Player.Resources[State.ResourceID(id)]
	}
	return 0
}

func currencyIDForJSONKey(store *GameData.Store, jsonKey string) State.CurrencyID {
	if store == nil {
		return 0
	}
	catalog, err := store.Catalog("currencies")
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
		id, _ := record.Int64("currencyID")
		return State.CurrencyID(id)
	}
	return 0
}

func resourceJSONKey(store *GameData.Store, resourceID State.ResourceID) string {
	if store == nil || resourceID <= 0 {
		return ""
	}
	catalog, err := store.Catalog("resources")
	if err != nil {
		return ""
	}
	raw, exists := catalog.Find(strconv.FormatInt(int64(resourceID), 10))
	if !exists {
		return ""
	}
	record, err := GameData.DecodeRecord(raw)
	if err != nil {
		return ""
	}
	value, _ := record.String("JSONKey")
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
