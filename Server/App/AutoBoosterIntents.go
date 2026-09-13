package App

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"time"

	"CitadelDesktop/Server/GameData"
	"CitadelDesktop/Server/Intent"
	"CitadelDesktop/Server/State"
)

const (
	autoBoosterPurchaseCursorKey   = "global-effect/2"
	autoBoosterPurchaseFreshness   = 2 * time.Minute
	autoBoosterPurchaseWindowGuard = 30 * time.Second
)

type autoBoosterPurchaseRequest struct {
	GlobalEffectID      int64 `json:"globalEffectId"`
	ExpectedEndsAtUnix  int64 `json:"expectedEndsAtUnix"`
	ExpectedRubyCost    int64 `json:"expectedRubyCost"`
	ExpectedBonusValue  int64 `json:"expectedBonusValue"`
	MinimumRubyReserve  int64 `json:"minimumRubyReserve"`
	ExpectedRubyBalance int64 `json:"expectedRubyBalance"`
}

func (application *Application) registerAutoBoosterIntents() error {
	definitions := []Intent.Definition{
		{
			Name: "autoBooster.refresh", Description: "Refresh daily global-effect windows and account offers",
			Effect: Intent.EffectRead, ArgumentsExample: json.RawMessage(`{}`), Planner: planAutoBoosterRefresh,
		},
		{
			Name: "autoBooster.purchase", Description: "Purchase only the live server-quoted daily fortress-speed global boost",
			Effect:           Intent.EffectWrite,
			ArgumentsExample: json.RawMessage(`{"globalEffectId":2,"expectedEndsAtUnix":1788382800,"expectedRubyCost":2500,"expectedBonusValue":60,"minimumRubyReserve":0,"expectedRubyBalance":10000}`),
			Planner:          planAutoBoosterPurchase,
		},
	}
	for _, definition := range definitions {
		if err := application.Intents.Registry().Register(definition); err != nil {
			return err
		}
	}
	return application.Intents.RegisterAction("auto_booster.purchase.guard", application.guardAutoBoosterPurchase)
}

func planAutoBoosterRefresh(_ context.Context, _ Intent.PlanningContext, arguments json.RawMessage) (Intent.Plan, error) {
	var request struct{}
	if err := decodeIntentArguments(arguments, &request); err != nil {
		return Intent.Plan{}, err
	}
	refresh := shopCommandStep("Refresh daily global-effect offers", "sei", json.RawMessage(`{}`), 0)
	refresh.ResponseBarrier = Intent.ResponseBarrierCommitted
	return Intent.Plan{
		Claims:  []string{"shop", "events", "global-effect:2"},
		Summary: "Refresh the daily fortress-speed global-effect offer", Steps: []Intent.Step{refresh},
	}, nil
}

func planAutoBoosterPurchase(_ context.Context, input Intent.PlanningContext, arguments json.RawMessage) (Intent.Plan, error) {
	request, err := autoBoosterPurchaseContext(input, arguments, time.Now().UTC(), false)
	if err != nil {
		return Intent.Plan{}, err
	}
	resolved, _ := json.Marshal(request)
	refresh := shopCommandStep("Refresh global-effect offer before purchase", "sei", json.RawMessage(`{}`), 0)
	refresh.ResponseBarrier = Intent.ResponseBarrierCommitted
	payload, _ := json.Marshal(map[string]any{"GEID": request.GlobalEffectID})
	purchase := shopCommandStep("Activate daily fortress-speed boost", "agb", payload, 0)
	purchase.ResponseBarrier = Intent.ResponseBarrierCommitted
	return Intent.Plan{
		Claims:  []string{"shop", "events", "global-effect:" + strconv.FormatInt(request.GlobalEffectID, 10), "account-resources"},
		Summary: fmt.Sprintf("Activate the daily fortress-speed boost for %d rubies", request.ExpectedRubyCost),
		Steps: []Intent.Step{
			refresh,
			Intent.RebuildOnResume(Intent.Step{
				Name: "Recheck daily fortress-speed boost purchase", Action: "auto_booster.purchase.guard", ActionArguments: resolved,
			}),
			purchase,
		},
	}, nil
}

func (application *Application) guardAutoBoosterPurchase(_ context.Context, arguments json.RawMessage) error {
	input, err := application.autoBoosterPlanningContext()
	if err != nil {
		return err
	}
	_, err = autoBoosterPurchaseContext(input, arguments, time.Now().UTC(), true)
	return err
}

func (application *Application) autoBoosterPlanningContext() (Intent.PlanningContext, error) {
	if application == nil || application.State == nil || application.GameData == nil {
		return Intent.PlanningContext{}, fmt.Errorf("Auto Booster state is unavailable")
	}
	gameData, ready := application.GameData.Current()
	if !ready {
		return Intent.PlanningContext{}, fmt.Errorf("official game data is unavailable")
	}
	return Intent.PlanningContext{State: application.State.ReadOnlyView(), GameData: gameData}, nil
}

func autoBoosterPurchaseContext(
	input Intent.PlanningContext,
	arguments json.RawMessage,
	now time.Time,
	requireFresh bool,
) (autoBoosterPurchaseRequest, error) {
	var request autoBoosterPurchaseRequest
	if err := decodeIntentArguments(arguments, &request); err != nil {
		return request, err
	}
	if request.GlobalEffectID != GameData.FortressDailyGlobalEffectID ||
		request.ExpectedRubyCost != GameData.FortressDailyBoosterRubyCost || request.ExpectedBonusValue <= 0 ||
		request.ExpectedEndsAtUnix <= 0 || request.MinimumRubyReserve < 0 || request.ExpectedRubyBalance < 0 {
		return request, fmt.Errorf("Auto Booster request does not match the approved 2,500-ruby fortress-speed purchase")
	}
	if input.GameData == nil {
		return request, fmt.Errorf("official game data is unavailable")
	}
	contract, err := input.GameData.FortressSpeed()
	if err != nil {
		return request, err
	}
	if contract.DailyGlobalEffectID != request.GlobalEffectID || contract.DailyBoostPercent <= 0 {
		return request, fmt.Errorf("official fortress-speed global effect changed; refusing purchase")
	}
	inventory := input.State.EventScores.Inventory
	if requireFresh && (inventory.GlobalEffectsObservedAt.IsZero() || now.Before(inventory.GlobalEffectsObservedAt) ||
		now.Sub(inventory.GlobalEffectsObservedAt) >= autoBoosterPurchaseFreshness) {
		return request, fmt.Errorf("%w: daily global-effect offer is stale", Intent.ErrPlanStale)
	}
	effect, found := inventory.GlobalEffects[request.GlobalEffectID]
	if !found || !effect.ActiveAt(now) || effect.EndsAt.Sub(now) <= autoBoosterPurchaseWindowGuard ||
		effect.EndsAt.Unix() != request.ExpectedEndsAtUnix {
		return request, fmt.Errorf("%w: daily fortress-speed effect window changed", Intent.ErrPlanStale)
	}
	if automation, found := input.State.Automations["autoBooster"]; found &&
		int64(automation.OperationalCursors[autoBoosterPurchaseCursorKey]) == request.ExpectedEndsAtUnix {
		return request, fmt.Errorf("%w: daily fortress-speed boost was already accepted for this window", Intent.ErrPlanStale)
	}
	status, found := inventory.GlobalEffectBoosts[request.GlobalEffectID]
	if !found || status.GlobalEffectID != request.GlobalEffectID || !status.OccurrenceEndsAt.Equal(effect.EndsAt) || status.ObservedAt.IsZero() {
		return request, fmt.Errorf("%w: current boosted-effect status is unavailable", Intent.ErrPlanStale)
	}
	if status.Boosted {
		return request, fmt.Errorf("%w: daily fortress-speed boost is already active", Intent.ErrPlanStale)
	}
	offer, found := inventory.GlobalEffectBoosterOffers[request.GlobalEffectID]
	if !found || offer.GlobalEffectID != request.GlobalEffectID || offer.RubyCost != request.ExpectedRubyCost ||
		offer.RubyCost != GameData.FortressDailyBoosterRubyCost || offer.BonusValue != request.ExpectedBonusValue || offer.BonusValue <= 0 {
		return request, fmt.Errorf("%w: live fortress-speed offer or price changed", Intent.ErrPlanStale)
	}
	resourceID, found := input.GameData.ResourceIDForJSONKey("C2")
	if !found || resourceID <= 0 {
		return request, fmt.Errorf("ruby balance is unavailable")
	}
	rubyValue, found := input.State.Player.Resources[State.ResourceID(resourceID)]
	if !found {
		return request, fmt.Errorf("ruby balance is unavailable")
	}
	rubies := int64(math.Floor(rubyValue))
	if requireFresh && rubies != request.ExpectedRubyBalance {
		return request, fmt.Errorf("%w: ruby balance changed before purchase", Intent.ErrPlanStale)
	}
	if rubies-request.MinimumRubyReserve < offer.RubyCost {
		return request, fmt.Errorf("%w: purchase would cross the configured ruby reserve", Intent.ErrPlanStale)
	}
	return request, nil
}
