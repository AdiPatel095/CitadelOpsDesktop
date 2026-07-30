package Automation

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"time"

	"CitadelDesktop/Server/GameData"
	"CitadelDesktop/Server/Intent"
	"CitadelDesktop/Server/State"
)

const (
	beriToolCheckInterval       = 30 * time.Second
	beriToolInventoryRefreshAge = 5 * time.Minute
)

type BeriToolPolicy struct{}

func NewBeriToolPolicy() *BeriToolPolicy { return &BeriToolPolicy{} }

func (*BeriToolPolicy) ID() string         { return "autoBeriWorldTools" }
func (*BeriToolPolicy) EnabledKey() string { return "auto_beri_world" }
func (*BeriToolPolicy) ActorID() string    { return "autoBeriWorld" }
func (*BeriToolPolicy) ScheduleKey() string {
	return "autoBeriWorld"
}

func (*BeriToolPolicy) WakeDomains() []string {
	return []string{"castles", "kingdom-transport", "resources", "units"}
}

func (*BeriToolPolicy) WakeSections() []string {
	return []string{autoBeriWorldSection}
}

func (*BeriToolPolicy) Evaluate(_ context.Context, snapshot Snapshot) (Decision, error) {
	var settings beriSettings
	decodeSection(snapshot.Configuration, autoBeriWorldSection, &settings)
	minimums := beriConfiguredToolMinimums(settings.ToolMinimums)
	if len(minimums) == 0 {
		return Decision{
			Status: "idle", Detail: "No Berimond armorer tool minimums are configured",
			NextCheckAt: snapshot.Now.Add(beriToolCheckInterval),
		}, nil
	}
	if snapshot.GameData == nil {
		return beriToolWaiting(snapshot.Now, "Official game data is unavailable"), nil
	}
	castle, found := beriToolCastle(snapshot.State, settings.BeriCastleID)
	if !found {
		return beriToolWaiting(snapshot.Now, "Waiting for an owned Berimond camp"), nil
	}
	if unlock, observed := snapshot.State.KingdomTransport.Unlocks[State.KingdomID(GameData.BerimondKingdomID)]; observed &&
		!unlock.Unlocked {
		return Decision{
			Status: "complete", Detail: "The Battle for Berimond is not currently unlocked",
			NextCheckAt: snapshot.Now.Add(beriToolCheckInterval),
		}, nil
	}
	if castle.UnitsObservedAt.IsZero() || castle.UnitsObservedAt.After(snapshot.Now) ||
		snapshot.Now.Sub(castle.UnitsObservedAt) >= beriToolInventoryRefreshAge {
		arguments, _ := json.Marshal(map[string]any{"castleId": castle.ID})
		return Decision{
			Status: "ready", Detail: "Refresh Berimond armorer tool inventory",
			NextCheckAt:         snapshot.Now.Add(time.Second),
			Request:             &Intent.Request{Name: "beri.tools.refresh", Arguments: arguments},
			ReevaluateOnSuccess: true, ReevaluateOnStale: true,
		}, nil
	}

	metrics := map[string]float64{}
	for _, rawToolID := range GameData.BerimondCoinAttackToolIDs() {
		toolID := State.UnitID(rawToolID)
		minimum, configured := minimums[toolID]
		if !configured {
			continue
		}
		available := castle.Units.Stationed[toolID]
		metrics[fmt.Sprintf("tool%dAvailable", toolID)] = float64(available)
		metrics[fmt.Sprintf("tool%dMinimum", toolID)] = float64(minimum)
		if available >= minimum {
			continue
		}
		item, supported := snapshot.GameData.BerimondArmorerAttackTool(rawToolID)
		if !supported {
			return beriToolWaiting(
				snapshot.Now,
				fmt.Sprintf("Official Berimond armorer package for tool %d is unavailable", toolID),
			), nil
		}
		if item.MinLevel > snapshot.State.Player.Level {
			return beriToolWaiting(
				snapshot.Now,
				fmt.Sprintf("%s unlocks at level %d", item.Name, item.MinLevel),
			), nil
		}
		deficit := minimum - available
		purchases := deficit / item.ToolAmount
		if deficit%item.ToolAmount != 0 {
			purchases++
		}
		purchases = min(purchases, int64(GameData.BerimondArmorerMaxPurchaseAmount))
		if purchases <= 0 || item.CoinPrice <= 0 || purchases > math.MaxInt64/item.CoinPrice {
			return beriToolWaiting(snapshot.Now, "The configured Berimond tool minimum is too large"), nil
		}
		cost := purchases * item.CoinPrice
		coins := int64(math.Floor(snapshot.State.Player.Resources[State.ResourceID(1)]))
		if coins < cost {
			return Decision{
				Status: "waiting",
				Detail: fmt.Sprintf(
					"Waiting for %d coins to buy %d %s; %d coins available",
					cost, purchases*item.ToolAmount, strings.ToLower(item.Name), coins,
				),
				NextCheckAt: snapshot.Now.Add(beriToolCheckInterval), Metrics: metrics,
			}, nil
		}
		arguments, _ := json.Marshal(map[string]any{
			"castleId": castle.ID, "packageId": item.PackageID, "toolId": toolID,
			"amount": purchases, "minimum": minimum,
		})
		return Decision{
			Status: "ready",
			Detail: fmt.Sprintf(
				"Buy a batch of %d %s for %d coins; %d stocked, minimum %d",
				purchases*item.ToolAmount, strings.ToLower(item.Name), cost, available, minimum,
			),
			NextCheckAt: snapshot.Now.Add(beriToolCheckInterval), Metrics: metrics,
			Request:             &Intent.Request{Name: "beri.tools.purchase", Arguments: arguments},
			ReevaluateOnSuccess: true, ReevaluateOnStale: true,
		}, nil
	}
	return Decision{
		Status: "idle", Detail: "Berimond coin attack tools meet their configured minimums",
		NextCheckAt: snapshot.Now.Add(beriToolCheckInterval), Metrics: metrics,
	}, nil
}

func beriConfiguredToolMinimums(configured map[State.UnitID]int64) map[State.UnitID]int64 {
	result := map[State.UnitID]int64{}
	for _, rawToolID := range GameData.BerimondCoinAttackToolIDs() {
		toolID := State.UnitID(rawToolID)
		if minimum := configured[toolID]; minimum > 0 {
			result[toolID] = minimum
		}
	}
	return result
}

func beriToolCastle(gameState State.GameState, requested State.CastleID) (State.CastleState, bool) {
	if requested > 0 {
		castle, found := gameState.Castles[requested]
		if !found || castle.KingdomID != State.KingdomID(GameData.BerimondKingdomID) {
			return State.CastleState{}, false
		}
		return castle, true
	}
	return beriCastle(gameState)
}

func beriToolWaiting(now time.Time, detail string) Decision {
	return Decision{
		Status: "waiting", Detail: detail,
		NextCheckAt: now.Add(beriToolCheckInterval),
	}
}
