package Automation

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"time"

	"CitadelDesktop/Server/GameData"
	"CitadelDesktop/Server/Intent"
	"CitadelDesktop/Server/State"
)

const foodBalanceKingdomDeliveryRatio = 0.90

type FoodBalancePolicy struct{}

type foodBalanceSettings struct {
	CheckIntervalSec         int     `json:"checkIntervalSec"`
	StateRefreshIntervalSec  int     `json:"stateRefreshIntervalSec"`
	LogisticsRefreshInterval int     `json:"logisticsRefreshIntervalSec"`
	SafetyHours              float64 `json:"safetyHours"`
	SourceSafetyHours        float64 `json:"sourceSafetyHours"`
	MinimumShipmentSize      int64   `json:"minimumShipmentSize"`
	MinimumSourceReserve     float64 `json:"minimumSourceReserve"`
	MinimumCoinReserve       float64 `json:"minimumCoinReserve"`
	AutoKingdomTransport     bool    `json:"autoKingdomTransport"`
}

type foodBalanceProjection struct {
	castle   State.CastleState
	consumed GameData.CastleFoodConsumption
}

type foodBalanceRisk struct {
	target       foodBalanceProjection
	resourceID   State.ResourceID
	rate         GameData.FoodConsumptionRate
	shortfall    float64
	urgencyHours float64
}

func NewFoodBalancePolicy() *FoodBalancePolicy { return &FoodBalancePolicy{} }

func (*FoodBalancePolicy) ID() string { return "autoFoodBalance" }

func (*FoodBalancePolicy) EnabledKey() string { return "auto_food_balance" }

func (*FoodBalancePolicy) WakeDomains() []string {
	return []string{"kingdom-transport", "market", "movements", "resources", "units"}
}

func (*FoodBalancePolicy) WakeSections() []string {
	return []string{"automation.autoFoodBalance"}
}

func (*FoodBalancePolicy) Evaluate(_ context.Context, snapshot Snapshot) (Decision, error) {
	settings := foodBalanceSettings{
		CheckIntervalSec: 60, StateRefreshIntervalSec: 900, LogisticsRefreshInterval: 300,
		SafetyHours: 8, SourceSafetyHours: 24, MinimumShipmentSize: 1_000,
		MinimumSourceReserve: 1_000, AutoKingdomTransport: true,
	}
	if !decodeSection(snapshot.Configuration, "automation.autoFoodBalance", &settings) {
		return Decision{
			Status: "waiting", Detail: "No automatic food-balancing settings are configured",
			NextCheckAt: snapshot.Now.Add(policyInterval(settings.CheckIntervalSec, 60)),
		}, nil
	}
	interval := policyInterval(settings.CheckIntervalSec, 60)
	stateRefreshInterval := policyInterval(settings.StateRefreshIntervalSec, 900)
	logisticsRefreshInterval := policyInterval(settings.LogisticsRefreshInterval, 300)
	if snapshot.GameData == nil {
		return Decision{Status: "waiting", Detail: "Waiting for official game data", NextCheckAt: snapshot.Now.Add(interval)}, nil
	}
	if decision, refresh := foodBalanceStateRefreshDecision(snapshot, stateRefreshInterval); refresh {
		return decision, nil
	}
	if foodBalanceLogisticsStale(snapshot, logisticsRefreshInterval, settings.AutoKingdomTransport) {
		return Decision{
			Status:              "ready",
			Detail:              "Refresh market and kingdom-resource logistics",
			NextCheckAt:         snapshot.Now.Add(2 * time.Second),
			Request:             &Intent.Request{Name: "resource.logistics.refresh", Arguments: json.RawMessage(`{}`)},
			ReevaluateOnSuccess: true,
		}, nil
	}
	projections, err := foodBalanceProjections(snapshot)
	if err != nil {
		return Decision{}, err
	}
	for _, castleID := range sortedCastleIDsFromFoodProjections(projections) {
		projection := projections[castleID]
		if foodBalanceEconomyComplete(projection) {
			continue
		}
		arguments, _ := json.Marshal(map[string]any{"castleId": projection.castle.ID})
		return Decision{
			Status:              "waiting",
			Detail:              fmt.Sprintf("Waiting for current food production at %s", castleName(projection.castle)),
			NextCheckAt:         snapshot.Now.Add(2 * time.Second),
			Request:             &Intent.Request{Name: "game.focus_castle", Arguments: arguments},
			ReevaluateOnSuccess: true,
		}, nil
	}
	risks := foodBalanceRisks(projections, settings)
	if len(risks) == 0 {
		return Decision{
			Status: "idle", Detail: "All observed castles have their configured food reserve",
			NextCheckAt: snapshot.Now.Add(interval), Metrics: map[string]float64{"castles": float64(len(projections))},
		}, nil
	}
	for _, risk := range risks {
		incoming := incomingMarketResource(snapshot, risk.target.castle, risk.resourceID)
		if incoming >= risk.shortfall {
			return Decision{
				Status:      "waiting",
				Detail:      fmt.Sprintf("An in-flight %s shipment protects %s", risk.rate.ResourceJSONKey, castleName(risk.target.castle)),
				NextCheckAt: nextMarketArrival(snapshot, risk.target.castle, risk.resourceID, interval),
			}, nil
		}
		shortfall := risk.shortfall - incoming
		decision, ready, marketErr := foodBalanceMarketShipment(settings, snapshot, projections, risk, shortfall)
		if marketErr != nil {
			return Decision{}, marketErr
		}
		if ready {
			return decision, nil
		}
		if settings.AutoKingdomTransport {
			if decision, handled := foodBalanceKingdomShipment(settings, snapshot, projections, risk, shortfall, interval); handled {
				return decision, nil
			}
		}
	}
	mostUrgent := risks[0]
	return Decision{
		Status:      "waiting",
		Detail:      fmt.Sprintf("No safe %s source is currently available for %s", mostUrgent.rate.ResourceJSONKey, castleName(mostUrgent.target.castle)),
		NextCheckAt: snapshot.Now.Add(interval),
		Metrics: map[string]float64{
			"shortfall": mostUrgent.shortfall, "hoursUntilDepleted": mostUrgent.urgencyHours,
		},
	}, nil
}

func foodBalanceStateRefreshDecision(snapshot Snapshot, interval time.Duration) (Decision, bool) {
	for _, castleID := range sortedCastleIDs(snapshot.State.Castles) {
		castle := snapshot.State.Castles[castleID]
		if !castle.FoodStateObservedAt.IsZero() && snapshot.Now.Sub(castle.FoodStateObservedAt) < interval {
			continue
		}
		arguments, _ := json.Marshal(map[string]any{"castleId": castle.ID})
		return Decision{
			Status:              "ready",
			Detail:              fmt.Sprintf("Refresh food state at %s", castleName(castle)),
			NextCheckAt:         snapshot.Now.Add(2 * time.Second),
			Request:             &Intent.Request{Name: "game.focus_castle", Arguments: arguments},
			ReevaluateOnSuccess: true,
		}, true
	}
	return Decision{}, false
}

func foodBalanceLogisticsStale(snapshot Snapshot, interval time.Duration, includeKingdomTransport bool) bool {
	if snapshot.State.Market.ObservedAt.IsZero() || !snapshot.State.Market.CaravanLevelLoaded {
		return true
	}
	oldest := snapshot.State.Market.ObservedAt
	if includeKingdomTransport {
		if snapshot.State.KingdomTransport.ObservedAt.IsZero() {
			return true
		}
		if snapshot.State.KingdomTransport.ObservedAt.Before(oldest) {
			oldest = snapshot.State.KingdomTransport.ObservedAt
		}
	}
	return snapshot.Now.Sub(oldest) >= interval
}

func foodBalanceProjections(snapshot Snapshot) (map[State.CastleID]foodBalanceProjection, error) {
	result := make(map[State.CastleID]foodBalanceProjection, len(snapshot.State.Castles))
	for _, castleID := range sortedCastleIDs(snapshot.State.Castles) {
		castle := snapshot.State.Castles[castleID]
		consumed, err := snapshot.GameData.EstimateFoodConsumption(castle)
		if err != nil {
			return nil, fmt.Errorf("estimate food at %s: %w", castleName(castle), err)
		}
		result[castleID] = foodBalanceProjection{castle: castle, consumed: consumed}
	}
	return result, nil
}

func foodBalanceEconomyComplete(projection foodBalanceProjection) bool {
	for _, rate := range projection.consumed.ByResource {
		if rate.TotalConsumptionPerHour > 0 && rate.NetPerHour == nil {
			return false
		}
	}
	return true
}

func foodBalanceRisks(projections map[State.CastleID]foodBalanceProjection, settings foodBalanceSettings) []foodBalanceRisk {
	safetyHours := settings.SafetyHours
	if safetyHours < 1 {
		safetyHours = 1
	}
	result := make([]foodBalanceRisk, 0)
	for _, castleID := range sortedCastleIDsFromFoodProjections(projections) {
		projection := projections[castleID]
		resourceIDs := make([]State.ResourceID, 0, len(projection.consumed.ByResource))
		for resourceID := range projection.consumed.ByResource {
			resourceIDs = append(resourceIDs, resourceID)
		}
		sort.Slice(resourceIDs, func(left, right int) bool { return resourceIDs[left] < resourceIDs[right] })
		for _, resourceID := range resourceIDs {
			rate := projection.consumed.ByResource[resourceID]
			if rate.TotalConsumptionPerHour <= 0 || rate.NetPerHour == nil {
				continue
			}
			balance := projection.castle.Resources[resourceID]
			required := rate.TotalConsumptionPerHour * safetyHours
			if balance.Capacity != nil {
				required = math.Min(required, math.Max(0, *balance.Capacity))
			}
			if required <= 0 {
				continue
			}
			shortfall := float64(0)
			if *rate.NetPerHour < 0 {
				shortfall = required - (balance.Amount + *rate.NetPerHour*safetyHours)
			} else if balance.Amount <= 0 {
				shortfall = required
			} else if *rate.NetPerHour == 0 {
				shortfall = required - balance.Amount
			}
			if shortfall <= 0 {
				continue
			}
			urgency := math.Inf(1)
			if *rate.NetPerHour < 0 {
				urgency = math.Max(0, balance.Amount) / -*rate.NetPerHour
			}
			result = append(result, foodBalanceRisk{
				target: projection, resourceID: resourceID, rate: rate, shortfall: shortfall, urgencyHours: urgency,
			})
		}
	}
	sort.Slice(result, func(left, right int) bool {
		if result[left].urgencyHours != result[right].urgencyHours {
			return result[left].urgencyHours < result[right].urgencyHours
		}
		if result[left].shortfall != result[right].shortfall {
			return result[left].shortfall > result[right].shortfall
		}
		if result[left].target.castle.ID != result[right].target.castle.ID {
			return result[left].target.castle.ID < result[right].target.castle.ID
		}
		return result[left].resourceID < result[right].resourceID
	})
	return result
}

func foodBalanceMarketShipment(
	settings foodBalanceSettings,
	snapshot Snapshot,
	projections map[State.CastleID]foodBalanceProjection,
	risk foodBalanceRisk,
	shortfall float64,
) (Decision, bool, error) {
	best := foodBalanceProjection{}
	bestAvailable := float64(0)
	bestCapacityPerBarrow := 0
	for _, castleID := range sortedCastleIDsFromFoodProjections(projections) {
		candidate := projections[castleID]
		if candidate.castle.ID == risk.target.castle.ID || candidate.castle.KingdomID != risk.target.castle.KingdomID {
			continue
		}
		hasMarketplace, err := snapshot.GameData.CastleHasMarketplace(candidate.castle)
		if err != nil {
			return Decision{}, false, fmt.Errorf("check marketplace at %s: %w", castleName(candidate.castle), err)
		}
		if !hasMarketplace {
			continue
		}
		market, observed := snapshot.State.Market.Castles[candidate.castle.ID]
		if !observed || market.AvailableBarrows <= 0 {
			continue
		}
		capacityPerBarrow := marketCapacityPerBarrow(snapshot, market)
		if capacityPerBarrow <= 0 {
			continue
		}
		available := foodBalanceSourceAvailable(settings, candidate, risk.resourceID)
		capacity := float64(market.AvailableBarrows * capacityPerBarrow)
		if shipment := math.Min(available, capacity); shipment > bestAvailable {
			best, bestAvailable, bestCapacityPerBarrow = candidate, shipment, capacityPerBarrow
		}
	}
	if best.castle.ID <= 0 || bestAvailable <= 0 {
		return Decision{}, false, nil
	}
	amount := math.Max(shortfall, float64(maxFoodBalanceShipment(settings.MinimumShipmentSize)))
	amount = math.Min(amount, bestAvailable)
	amount = math.Min(amount, foodBalanceTargetFreeCapacity(risk.target.castle, risk.resourceID))
	amount = math.Floor(amount)
	if amount <= 0 {
		return Decision{}, false, nil
	}
	barrows := int(math.Ceil(amount / float64(bestCapacityPerBarrow)))
	coinCost := marketShipmentCoinCost(best.castle, risk.target.castle, barrows)
	if playerResourceAmount(snapshot, "C1")-settings.MinimumCoinReserve < coinCost {
		return Decision{}, false, nil
	}
	arguments, _ := json.Marshal(map[string]any{
		"sourceCastleId": best.castle.ID, "targetCastleId": risk.target.castle.ID,
		"resourceId": risk.resourceID, "amount": int64(amount),
	})
	return Decision{
		Status:              "ready",
		Detail:              fmt.Sprintf("Send %.0f %s from %s to %s", amount, risk.rate.ResourceJSONKey, castleName(best.castle), castleName(risk.target.castle)),
		NextCheckAt:         snapshot.Now.Add(2 * time.Second),
		Metrics:             map[string]float64{"shipmentAmount": amount, "coinCost": coinCost, "hoursUntilDepleted": risk.urgencyHours},
		Request:             &Intent.Request{Name: "resource.market.ship", Arguments: arguments},
		ReevaluateOnSuccess: true,
	}, true, nil
}

func foodBalanceKingdomShipment(
	settings foodBalanceSettings,
	snapshot Snapshot,
	projections map[State.CastleID]foodBalanceProjection,
	risk foodBalanceRisk,
	shortfall float64,
	interval time.Duration,
) (Decision, bool) {
	for _, pending := range snapshot.State.KingdomTransport.Pending {
		if pending.KingdomID == risk.target.castle.KingdomID && pending.RemainingSec > 0 {
			return Decision{
				Status: "waiting", Detail: fmt.Sprintf("Kingdom %d already has a resource shipment in flight", pending.KingdomID),
				NextCheckAt: snapshot.Now.Add(time.Duration(pending.RemainingSec) * time.Second),
			}, true
		}
	}
	unlock, observed := snapshot.State.KingdomTransport.Unlocks[risk.target.castle.KingdomID]
	if !observed || !unlock.Unlocked {
		return Decision{}, false
	}
	best := foodBalanceProjection{}
	bestAvailable := float64(0)
	for _, castleID := range sortedCastleIDsFromFoodProjections(projections) {
		candidate := projections[castleID]
		if candidate.castle.ID == risk.target.castle.ID || candidate.castle.KingdomID == risk.target.castle.KingdomID {
			continue
		}
		if available := foodBalanceSourceAvailable(settings, candidate, risk.resourceID); available > bestAvailable {
			best, bestAvailable = candidate, available
		}
	}
	if best.castle.ID <= 0 || bestAvailable <= 0 {
		return Decision{}, false
	}
	amount := math.Max(shortfall, float64(maxFoodBalanceShipment(settings.MinimumShipmentSize))) / foodBalanceKingdomDeliveryRatio
	amount = math.Min(amount, bestAvailable)
	free := foodBalanceTargetFreeCapacity(risk.target.castle, risk.resourceID)
	amount = math.Min(amount, free/foodBalanceKingdomDeliveryRatio)
	amount = math.Floor(amount)
	if amount <= 0 {
		return Decision{}, false
	}
	arguments, _ := json.Marshal(map[string]any{
		"sourceCastleId": best.castle.ID, "targetCastleId": risk.target.castle.ID, "targetKingdomId": risk.target.castle.KingdomID,
		"resourceId": risk.resourceID, "amount": int64(amount),
	})
	return Decision{
		Status:      "ready",
		Detail:      fmt.Sprintf("Send %.0f %s from %s to %s by kingdom transport", amount, risk.rate.ResourceJSONKey, castleName(best.castle), castleName(risk.target.castle)),
		NextCheckAt: snapshot.Now.Add(interval),
		Metrics:     map[string]float64{"shipmentAmount": amount, "hoursUntilDepleted": risk.urgencyHours},
		Request:     &Intent.Request{Name: "resource.kingdom.ship", Arguments: arguments},
	}, true
}

func foodBalanceSourceAvailable(settings foodBalanceSettings, projection foodBalanceProjection, resourceID State.ResourceID) float64 {
	rate, exists := projection.consumed.ByResource[resourceID]
	if !exists {
		return 0
	}
	safetyHours := settings.SourceSafetyHours
	if safetyHours < 1 {
		safetyHours = 1
	}
	floor := math.Max(settings.MinimumSourceReserve, rate.TotalConsumptionPerHour*safetyHours)
	return math.Max(0, projection.castle.Resources[resourceID].Amount-floor)
}

func foodBalanceTargetFreeCapacity(castle State.CastleState, resourceID State.ResourceID) float64 {
	balance := castle.Resources[resourceID]
	if balance.Capacity == nil {
		return math.Inf(1)
	}
	return math.Max(0, *balance.Capacity-balance.Amount)
}

func maxFoodBalanceShipment(value int64) int64 {
	if value < 1 {
		return 1
	}
	return value
}

func sortedCastleIDsFromFoodProjections(values map[State.CastleID]foodBalanceProjection) []State.CastleID {
	result := make([]State.CastleID, 0, len(values))
	for castleID := range values {
		result = append(result, castleID)
	}
	sort.Slice(result, func(left, right int) bool { return result[left] < result[right] })
	return result
}
