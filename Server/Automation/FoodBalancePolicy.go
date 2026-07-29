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
	CheckIntervalSec         int              `json:"checkIntervalSec"`
	StateRefreshIntervalSec  int              `json:"stateRefreshIntervalSec"`
	LogisticsRefreshInterval int              `json:"logisticsRefreshIntervalSec"`
	SafetyHours              float64          `json:"safetyHours"`
	SourceSafetyHours        float64          `json:"sourceSafetyHours"`
	MinimumShipmentSize      int64            `json:"minimumShipmentSize"`
	MinimumSourceReserve     float64          `json:"minimumSourceReserve"`
	MinimumCoinReserve       float64          `json:"minimumCoinReserve"`
	AutoKingdomTransport     bool             `json:"autoKingdomTransport"`
	UseKingdomTimeSkips      bool             `json:"useKingdomTimeSkips"`
	AllowedTimeSkips         []string         `json:"allowedTimeSkips"`
	TimeSkipReserve          map[string]int64 `json:"timeSkipReserve"`
	HorseTravelBoostID       int              `json:"horseTravelBoostId"`
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

type foodBalanceDonor struct {
	projection foodBalanceProjection
	available  float64
	netPerHour float64
}

func NewFoodBalancePolicy() *FoodBalancePolicy { return &FoodBalancePolicy{} }

func (*FoodBalancePolicy) ID() string { return "autoFoodBalance" }

func (*FoodBalancePolicy) EnabledKey() string { return "auto_food_balance" }

func (*FoodBalancePolicy) WakeDomains() []string {
	return []string{"currencies", "kingdom-transport", "market", "movements", "resources", "units"}
}

func (*FoodBalancePolicy) WakeSections() []string {
	return []string{"automation.autoFoodBalance"}
}

func (*FoodBalancePolicy) Evaluate(_ context.Context, snapshot Snapshot) (Decision, error) {
	settings := foodBalanceSettings{
		CheckIntervalSec: 60, StateRefreshIntervalSec: 900, LogisticsRefreshInterval: 300,
		SafetyHours: 8, SourceSafetyHours: 24, MinimumShipmentSize: 1_000,
		MinimumSourceReserve: 1_000, AutoKingdomTransport: true, TimeSkipReserve: map[string]int64{},
		HorseTravelBoostID: -1,
	}
	if !decodeSection(snapshot.Configuration, "automation.autoFoodBalance", &settings) {
		return Decision{
			Status: "waiting", Detail: "No automatic food-balancing settings are configured",
			NextCheckAt: snapshot.Now.Add(policyInterval(settings.CheckIntervalSec, 60)),
		}, nil
	}
	if !validHorseTravelBoostID(settings.HorseTravelBoostID) {
		return Decision{
			Status: "waiting", Detail: "Choose a supported market-barrow horse travel boost",
			NextCheckAt: snapshot.Now.Add(policyInterval(settings.CheckIntervalSec, 60)),
		}, nil
	}
	interval := policyInterval(settings.CheckIntervalSec, 60)
	stateRefreshInterval := policyInterval(settings.StateRefreshIntervalSec, 900)
	logisticsRefreshInterval := policyInterval(settings.LogisticsRefreshInterval, 300)
	if snapshot.GameData == nil {
		return Decision{Status: "waiting", Detail: "Waiting for official game data", NextCheckAt: snapshot.Now.Add(interval)}, nil
	}
	waitingDecision := Decision{}
	waitingDecisionFound := false
	rememberWaiting := func(decision Decision) {
		if decision.Status == "" {
			return
		}
		if !waitingDecisionFound || waitingDecision.NextCheckAt.IsZero() ||
			(!decision.NextCheckAt.IsZero() && decision.NextCheckAt.Before(waitingDecision.NextCheckAt)) {
			waitingDecision = decision
			waitingDecisionFound = true
		}
	}
	if decision, refresh := foodBalanceStateRefreshDecision(snapshot, stateRefreshInterval); refresh {
		return decision, nil
	}
	logisticsStale, marketLeaseUntil, logisticsErr := foodBalanceLogisticsStale(snapshot, logisticsRefreshInterval, settings.AutoKingdomTransport)
	if logisticsErr != nil {
		return Decision{}, logisticsErr
	}
	if !marketLeaseUntil.IsZero() {
		rememberWaiting(Decision{
			Status: "waiting", Detail: "Waiting for leased market barrows to return before refreshing logistics",
			NextCheckAt: marketLeaseUntil.Add(time.Second),
		})
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
	if decision, ready := ownedKingdomTransportDecision(
		autoFoodBalanceTransportOwner, "Auto Food",
		settings.AutoKingdomTransport && settings.UseKingdomTimeSkips,
		settings.AllowedTimeSkips, settings.TimeSkipReserve, snapshot,
	); ready {
		return decision, nil
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
			Detail:              fmt.Sprintf("Waiting for current food production and storage at %s", castleName(projection.castle)),
			NextCheckAt:         snapshot.Now.Add(2 * time.Second),
			Request:             &Intent.Request{Name: "game.focus_castle", Arguments: arguments},
			ReevaluateOnSuccess: true,
		}, nil
	}
	risks := foodBalanceRisks(projections, settings)
	if len(risks) == 0 {
		if waitingDecisionFound {
			return waitingDecision, nil
		}
		return Decision{
			Status: "idle", Detail: "All observed castles have their configured food reserve",
			NextCheckAt: snapshot.Now.Add(interval), Metrics: map[string]float64{"castles": float64(len(projections))},
		}, nil
	}
	for _, risk := range risks {
		incoming := incomingMarketResource(snapshot, risk.target.castle, risk.resourceID)
		if incoming >= risk.shortfall {
			rememberWaiting(Decision{
				Status:      "waiting",
				Detail:      fmt.Sprintf("An in-flight %s shipment protects %s", risk.rate.ResourceJSONKey, castleName(risk.target.castle)),
				NextCheckAt: nextMarketArrival(snapshot, risk.target.castle, risk.resourceID, interval),
			})
			continue
		}
		targetNeed, storageKnown := foodBalanceTargetFreeCapacity(risk.target.castle, risk.resourceID)
		if !storageKnown {
			arguments, _ := json.Marshal(map[string]any{"castleId": risk.target.castle.ID})
			return Decision{
				Status:              "waiting",
				Detail:              fmt.Sprintf("Waiting for current %s storage at %s", risk.rate.ResourceJSONKey, castleName(risk.target.castle)),
				NextCheckAt:         snapshot.Now.Add(2 * time.Second),
				Request:             &Intent.Request{Name: "game.focus_castle", Arguments: arguments},
				ReevaluateOnSuccess: true,
			}, nil
		}
		targetNeed = math.Max(0, targetNeed-incoming)
		if targetNeed <= 0 {
			continue
		}
		if decision, ready, shipmentErr := foodBalanceShipment(
			settings, snapshot, projections, risk, targetNeed, interval,
		); shipmentErr != nil {
			return Decision{}, shipmentErr
		} else if ready {
			return decision, nil
		} else {
			rememberWaiting(decision)
		}
	}
	if waitingDecisionFound {
		return waitingDecision, nil
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

func foodBalanceLogisticsStale(snapshot Snapshot, interval time.Duration, includeKingdomTransport bool) (bool, time.Time, error) {
	marketRequired, err := marketLogisticsRequired(snapshot)
	if err != nil {
		return false, time.Time{}, err
	}
	kingdomRequired := includeKingdomTransport && kingdomLogisticsRequired(snapshot.State)
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
	for resourceID, rate := range projection.consumed.ByResource {
		if rate.TotalConsumptionPerHour <= 0 {
			continue
		}
		balance, exists := projection.castle.Resources[resourceID]
		if !exists || balance.Capacity == nil || rate.NetPerHour == nil {
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

func foodBalanceShipment(
	settings foodBalanceSettings,
	snapshot Snapshot,
	projections map[State.CastleID]foodBalanceProjection,
	risk foodBalanceRisk,
	targetNeed float64,
	interval time.Duration,
) (Decision, bool, error) {
	marketAmount := math.Floor(targetNeed)
	kingdomAmount, kingdomAmountReady := foodBalanceKingdomDispatchAmount(settings, targetNeed)
	blocked := Decision{}
	blockedFound := false
	for _, donor := range foodBalanceDonors(settings, projections, risk) {
		if donor.projection.castle.KingdomID == risk.target.castle.KingdomID {
			if marketAmount <= 0 || donor.available < marketAmount {
				continue
			}
			decision, ready, err := foodBalanceMarketShipmentFromDonor(
				settings, snapshot, risk, targetNeed, donor,
			)
			if err != nil {
				return Decision{}, false, err
			}
			if ready {
				return decision, true, nil
			}
			continue
		}
		if !settings.AutoKingdomTransport || !kingdomAmountReady || donor.available < kingdomAmount {
			continue
		}
		decision, handled := foodBalanceKingdomShipmentFromDonor(
			settings, snapshot, risk, kingdomAmount, interval, donor,
		)
		if !handled {
			continue
		}
		if decision.Request != nil {
			return decision, true, nil
		}
		if !blockedFound {
			blocked = decision
			blockedFound = true
		}
	}
	return blocked, false, nil
}

func foodBalanceMarketShipment(
	settings foodBalanceSettings,
	snapshot Snapshot,
	projections map[State.CastleID]foodBalanceProjection,
	risk foodBalanceRisk,
	targetNeed float64,
) (Decision, bool, error) {
	neededAmount := math.Floor(targetNeed)
	if neededAmount <= 0 {
		return Decision{}, false, nil
	}
	for _, donor := range foodBalanceDonors(settings, projections, risk) {
		if donor.projection.castle.KingdomID != risk.target.castle.KingdomID ||
			donor.available < neededAmount {
			continue
		}
		decision, ready, err := foodBalanceMarketShipmentFromDonor(settings, snapshot, risk, targetNeed, donor)
		if err != nil || ready {
			return decision, ready, err
		}
	}
	return Decision{}, false, nil
}

func foodBalanceMarketShipmentFromDonor(
	settings foodBalanceSettings,
	snapshot Snapshot,
	risk foodBalanceRisk,
	targetNeed float64,
	donor foodBalanceDonor,
) (Decision, bool, error) {
	hasMarketplace, err := snapshot.GameData.CastleHasMarketplace(donor.projection.castle)
	if err != nil {
		return Decision{}, false, fmt.Errorf("check marketplace at %s: %w", castleName(donor.projection.castle), err)
	}
	if !hasMarketplace {
		return Decision{}, false, nil
	}
	market, observed := snapshot.State.Market.Castles[donor.projection.castle.ID]
	availableBarrows := State.AvailableMarketBarrowsAt(snapshot.State, market, snapshot.Now)
	if !observed || availableBarrows <= 0 {
		return Decision{}, false, nil
	}
	capacityPerBarrow := marketCapacityPerBarrow(snapshot, market)
	if capacityPerBarrow <= 0 {
		return Decision{}, false, nil
	}
	capacity := float64(availableBarrows * capacityPerBarrow)
	amount := math.Floor(math.Min(targetNeed, capacity))
	if amount <= 0 {
		return Decision{}, false, nil
	}
	barrows := int(math.Ceil(amount / float64(capacityPerBarrow)))
	coinCost := marketShipmentCoinCost(donor.projection.castle, risk.target.castle, barrows)
	if playerResourceAmount(snapshot, "C1")-settings.MinimumCoinReserve < coinCost {
		return Decision{}, false, nil
	}
	arguments, _ := json.Marshal(map[string]any{
		"sourceCastleId": donor.projection.castle.ID, "targetCastleId": risk.target.castle.ID,
		"resourceId": risk.resourceID, "amount": int64(amount),
		"workflowOwner": autoFoodBalanceTransportOwner, "enforceTargetCapacity": true,
		"horseTravelBoostId": settings.HorseTravelBoostID,
	})
	return Decision{
		Status:              "ready",
		Detail:              fmt.Sprintf("Send %.0f %s from %s to %s", amount, risk.rate.ResourceJSONKey, castleName(donor.projection.castle), castleName(risk.target.castle)),
		NextCheckAt:         snapshot.Now.Add(2 * time.Second),
		Metrics:             map[string]float64{"shipmentAmount": amount, "targetNeed": targetNeed, "coinCost": coinCost, "hoursUntilDepleted": risk.urgencyHours},
		Request:             &Intent.Request{Name: "resource.ship", Arguments: arguments},
		ReevaluateOnSuccess: true,
	}, true, nil
}

func foodBalanceKingdomShipment(
	settings foodBalanceSettings,
	snapshot Snapshot,
	projections map[State.CastleID]foodBalanceProjection,
	risk foodBalanceRisk,
	targetNeed float64,
	interval time.Duration,
) (Decision, bool) {
	amount, ready := foodBalanceKingdomDispatchAmount(settings, targetNeed)
	if !ready {
		return Decision{}, false
	}
	for _, donor := range foodBalanceDonors(settings, projections, risk) {
		if donor.projection.castle.KingdomID == risk.target.castle.KingdomID ||
			donor.available < amount {
			continue
		}
		if decision, handled := foodBalanceKingdomShipmentFromDonor(
			settings, snapshot, risk, amount, interval, donor,
		); handled {
			return decision, true
		}
	}
	return Decision{}, false
}

func foodBalanceKingdomDispatchAmount(settings foodBalanceSettings, targetNeed float64) (float64, bool) {
	minimumDelivery := float64(maxFoodBalanceShipment(settings.MinimumShipmentSize))
	minimumDispatch := math.Ceil(minimumDelivery / foodBalanceKingdomDeliveryRatio)
	amount := math.Floor(targetNeed / foodBalanceKingdomDeliveryRatio)
	return amount, amount >= minimumDispatch
}

func foodBalanceKingdomShipmentFromDonor(
	settings foodBalanceSettings,
	snapshot Snapshot,
	risk foodBalanceRisk,
	amount float64,
	interval time.Duration,
	donor foodBalanceDonor,
) (Decision, bool) {
	if pending, found := pendingKingdomResourceTransport(snapshot.State, risk.target.castle.KingdomID); found {
		detail := fmt.Sprintf("Kingdom %d already has a resource shipment in flight", pending.KingdomID)
		nextCheck := snapshot.Now.Add(coordinatorTick)
		if pending.RemainingSec > 0 {
			nextCheck = snapshot.Now.Add(time.Duration(pending.RemainingSec) * time.Second)
		} else {
			detail = fmt.Sprintf("Waiting for the kingdom %d resource shipment to settle", pending.KingdomID)
		}
		return Decision{Status: "waiting", Detail: detail, NextCheckAt: nextCheck}, true
	}
	if workflow, found := kingdomResourceTransportWorkflow(snapshot.State, risk.target.castle.KingdomID); found {
		return Decision{
			Status:      "waiting",
			Detail:      fmt.Sprintf("Waiting for %s to refresh the kingdom %d resource destination", workflow.Owner, workflow.KingdomID),
			NextCheckAt: snapshot.Now.Add(coordinatorTick),
		}, true
	}
	unlock, observed := snapshot.State.KingdomTransport.Unlocks[risk.target.castle.KingdomID]
	if !observed || !unlock.Unlocked {
		return Decision{}, false
	}
	shipmentArguments := map[string]any{
		"sourceCastleId": donor.projection.castle.ID, "targetCastleId": risk.target.castle.ID,
		"resourceId": risk.resourceID, "amount": int64(amount),
		"workflowOwner": autoFoodBalanceTransportOwner, "enforceTargetCapacity": true,
	}
	if skipID, reserve, found := foodBalanceImmediateKingdomTimeSkip(settings, snapshot); found {
		shipmentArguments["timeSkipId"] = skipID
		shipmentArguments["minimumRemaining"] = reserve
		shipmentArguments["settleAfterTimeSkip"] = true
	}
	arguments, _ := json.Marshal(shipmentArguments)
	return Decision{
		Status:              "ready",
		Detail:              fmt.Sprintf("Send %.0f %s from %s to %s by kingdom transport", amount, risk.rate.ResourceJSONKey, castleName(donor.projection.castle), castleName(risk.target.castle)),
		NextCheckAt:         snapshot.Now.Add(interval),
		Metrics:             map[string]float64{"shipmentAmount": amount, "hoursUntilDepleted": risk.urgencyHours},
		Request:             &Intent.Request{Name: "resource.ship", Arguments: arguments},
		ReevaluateOnSuccess: true,
	}, true
}

func foodBalanceImmediateKingdomTimeSkip(
	settings foodBalanceSettings,
	snapshot Snapshot,
) (string, int64, bool) {
	if !settings.UseKingdomTimeSkips {
		return "", 0, false
	}
	skipID := chooseKingdomTimeSkip(
		settings.AllowedTimeSkips,
		settings.TimeSkipReserve,
		snapshot,
		kingdomResourceTransportInitialRemainingSec,
	)
	if kingdomTimeSkipSeconds[skipID] < kingdomResourceTransportInitialRemainingSec {
		return "", 0, false
	}
	return skipID, max(int64(0), automationTimeSkipReserve(settings.TimeSkipReserve, skipID)), true
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

func foodBalanceDonors(
	settings foodBalanceSettings,
	projections map[State.CastleID]foodBalanceProjection,
	risk foodBalanceRisk,
) []foodBalanceDonor {
	result := make([]foodBalanceDonor, 0, len(projections))
	for _, castleID := range sortedCastleIDsFromFoodProjections(projections) {
		candidate := projections[castleID]
		if candidate.castle.ID == risk.target.castle.ID {
			continue
		}
		rate, rateKnown := candidate.consumed.ByResource[risk.resourceID]
		if !rateKnown || rate.NetPerHour == nil {
			continue
		}
		available := foodBalanceSourceAvailable(settings, candidate, risk.resourceID)
		if available > 0 {
			result = append(result, foodBalanceDonor{
				projection: candidate, available: available, netPerHour: *rate.NetPerHour,
			})
		}
	}
	sort.Slice(result, func(left, right int) bool {
		if result[left].netPerHour != result[right].netPerHour {
			return result[left].netPerHour > result[right].netPerHour
		}
		if result[left].available != result[right].available {
			return result[left].available > result[right].available
		}
		return result[left].projection.castle.ID < result[right].projection.castle.ID
	})
	return result
}

func foodBalanceTargetFreeCapacity(castle State.CastleState, resourceID State.ResourceID) (float64, bool) {
	balance, exists := castle.Resources[resourceID]
	if !exists || balance.Capacity == nil {
		return 0, false
	}
	return math.Max(0, *balance.Capacity-balance.Amount), true
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
