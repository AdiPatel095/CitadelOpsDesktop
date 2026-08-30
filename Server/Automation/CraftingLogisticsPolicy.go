package Automation

import (
	"context"
	"encoding/json"
	"strconv"
	"time"

	"CitadelDesktop/Server/Intent"
	"CitadelDesktop/Server/State"
)

const autoSceatLogisticsPolicyID = "autoSceatResLogistics"

type CraftingLogisticsPolicy struct{}

func NewCraftingLogisticsPolicy() *CraftingLogisticsPolicy {
	return &CraftingLogisticsPolicy{}
}

func (*CraftingLogisticsPolicy) ID() string { return autoSceatLogisticsPolicyID }

func (*CraftingLogisticsPolicy) ActorID() string { return autoSceatTransportOwner }

func (*CraftingLogisticsPolicy) EnabledKey() string { return "auto_sceat_resources" }

func (*CraftingLogisticsPolicy) ScheduleKey() string { return "autoSceatRes" }

func (*CraftingLogisticsPolicy) WakeDomains() []string {
	return []string{"crafting", "currencies", "kingdom-transport", "market", "movements", "resources"}
}

func (*CraftingLogisticsPolicy) WakeSections() []string {
	return []string{"automation.autoSceatResources"}
}

func (*CraftingLogisticsPolicy) Evaluate(_ context.Context, snapshot Snapshot) (Decision, error) {
	settings, _, configured := craftingSettingsFromSnapshot(snapshot)
	interval := policyInterval(settings.CheckIntervalSec, 300)
	if !configured {
		return Decision{
			Status: "waiting", Detail: "No sovereign crafting logistics plan is configured",
			NextCheckAt: snapshot.Now.Add(interval),
		}, nil
	}
	if settings.AutoKingdomTransport {
		logisticsStale, marketLeaseUntil, err := craftingLogisticsStale(snapshot, interval)
		if err != nil {
			return Decision{}, err
		}
		if !marketLeaseUntil.IsZero() {
			return Decision{
				Status: "waiting", Detail: "Waiting for leased market barrows to return before refreshing logistics",
				NextCheckAt: marketLeaseUntil.Add(time.Second),
			}, nil
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
	if !settings.AutoKingdomTransport {
		return Decision{
			Status: "idle", Detail: "Automatic resource logistics are disabled in Auto Sceat settings",
			NextCheckAt: snapshot.Now.Add(interval),
		}, nil
	}

	var deferredTransportWait *Decision
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
			if len(building.Active)+len(building.Queued) >= capacity {
				continue
			}
			cursor := plan.Cursor % len(cycle)
			if cursor < 0 {
				cursor = 0
			}
			selection, missingSelection, _, err := selectCraftingRecipe(
				snapshot, settings, castle, building, cycle, cursor,
			)
			if err != nil {
				return Decision{}, err
			}
			if selection.RecipeID > 0 || missingSelection == nil {
				continue
			}
			decision, handled := craftingTransportDecision(
				settings, snapshot, castle, missingSelection.Costs.Missing, interval,
			)
			if !handled {
				continue
			}
			if decision.Request != nil {
				return decision, nil
			}
			if deferredTransportWait == nil {
				deferredTransportWait = &decision
			}
		}
	}
	if decision, ready := craftingLootDrainDecision(settings, snapshot); ready {
		return decision, nil
	}
	if decision, ready := craftingOverflowRedistributionDecision(settings, snapshot); ready {
		return decision, nil
	}
	if decision, ready := marketOverflowDecision(settings, snapshot, interval); ready {
		return decision, nil
	}
	if decision, ready := stormOverflowDecision(settings, snapshot, interval); ready {
		return decision, nil
	}
	if deferredTransportWait != nil {
		return *deferredTransportWait, nil
	}
	return Decision{
		Status: "idle", Detail: "No Auto Sceat resource move is currently needed",
		NextCheckAt: snapshot.Now.Add(interval),
	}, nil
}
