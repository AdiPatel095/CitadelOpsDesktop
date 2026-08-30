package Automation

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"CitadelDesktop/Server/Intent"
	"CitadelDesktop/Server/State"
)

const (
	kingdomResourceDeliveryRatio                = 0.90
	kingdomResourceTransportInitialRemainingSec = 3_600
)

const (
	autoFoodBalanceTransportOwner = "autoFoodBalance"
	autoSceatTransportOwner       = "autoSceatRes"
)

var kingdomTimeSkipSeconds = map[string]int{
	"MS1": 60, "MS2": 300, "MS3": 600, "MS4": 1_800,
	"MS5": 3_600, "MS6": 18_000, "MS7": 86_400,
}

func ownedKingdomTransportDecision(
	owner string,
	displayName string,
	useTimeSkips bool,
	allowedTimeSkips []string,
	timeSkipReserve map[string]int64,
	snapshot Snapshot,
) (Decision, bool) {
	workflows := ownedKingdomResourceWorkflows(snapshot.State, owner)
	for _, workflow := range workflows {
		pending, found := pendingKingdomResourceTransport(snapshot.State, workflow.KingdomID)
		if !found {
			arguments, _ := json.Marshal(map[string]any{
				"owner": owner, "targetKingdomId": workflow.KingdomID,
			})
			return Decision{
				Status:              "ready",
				Detail:              fmt.Sprintf("Refresh the destination of %s's completed kingdom %d resource shipment", displayName, workflow.KingdomID),
				NextCheckAt:         snapshot.Now.Add(2 * time.Second),
				Request:             &Intent.Request{Name: "resource.kingdom.settle", Arguments: arguments},
				ReevaluateOnSuccess: true,
			}, true
		}
		if !useTimeSkips || len(allowedTimeSkips) == 0 || pending.RemainingSec <= 0 {
			continue
		}
		skipID := chooseKingdomTimeSkip(allowedTimeSkips, timeSkipReserve, snapshot, pending.RemainingSec)
		if skipID == "" {
			continue
		}
		reserve := max(int64(0), automationTimeSkipReserve(timeSkipReserve, skipID))
		arguments, _ := json.Marshal(map[string]any{
			"targetKingdomId":  pending.KingdomID,
			"timeSkipId":       skipID,
			"minimumRemaining": reserve,
		})
		return Decision{
			Status:              "ready",
			Detail:              fmt.Sprintf("Apply %s to %s's kingdom %d resource shipment", skipID, displayName, pending.KingdomID),
			NextCheckAt:         snapshot.Now.Add(2 * time.Second),
			Request:             &Intent.Request{Name: "resource.kingdom.skip", Arguments: arguments},
			FailureFallback:     &Intent.Request{Name: "resource.logistics.refresh", Arguments: json.RawMessage(`{}`)},
			FailureDetail:       fmt.Sprintf("Refreshed kingdom transport state after %s's skip did not complete", displayName),
			ReevaluateOnSuccess: true,
			ReevaluateOnStale:   true,
		}, true
	}
	return Decision{}, false
}

func ownedKingdomResourceWorkflows(gameState State.GameState, owner string) []State.KingdomResourceTransportWorkflow {
	result := make([]State.KingdomResourceTransportWorkflow, 0, len(gameState.KingdomTransport.ResourceWorkflows))
	for _, workflow := range gameState.KingdomTransport.ResourceWorkflows {
		if workflow.Owner == owner {
			result = append(result, workflow)
		}
	}
	sort.Slice(result, func(left, right int) bool {
		if result[left].LaunchedAt.Equal(result[right].LaunchedAt) {
			return result[left].KingdomID < result[right].KingdomID
		}
		return result[left].LaunchedAt.Before(result[right].LaunchedAt)
	})
	return result
}

func chooseKingdomTimeSkip(
	allowedTimeSkips []string,
	timeSkipReserve map[string]int64,
	snapshot Snapshot,
	remainingSec int,
) string {
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
	for _, rawID := range allowedTimeSkips {
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
		reserve := max(int64(0), automationTimeSkipReserve(timeSkipReserve, id))
		if currencyID <= 0 || snapshot.State.Player.Currencies[currencyID] <= float64(reserve) {
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
