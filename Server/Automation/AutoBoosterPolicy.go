package Automation

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"time"

	"CitadelDesktop/Server/GameData"
	"CitadelDesktop/Server/Intent"
	"CitadelDesktop/Server/State"
)

const (
	autoBoosterSection                 = "automation.autoBooster"
	autoBoosterDefaultCheckIntervalSec = 60
	autoBoosterOfferFreshness          = 2 * time.Minute
	autoBoosterMinimumEffectWindow     = 30 * time.Second
	autoBoosterCursorKey               = "global-effect/2"
)

type AutoBoosterPolicy struct{}

type autoBoosterSettings struct {
	Version            int   `json:"version"`
	CheckIntervalSec   int   `json:"checkIntervalSec"`
	RubyCostCeiling    int64 `json:"rubyCostCeiling"`
	MinimumRubyReserve int64 `json:"minimumRubyReserve"`
}

func NewAutoBoosterPolicy() *AutoBoosterPolicy { return &AutoBoosterPolicy{} }

func (*AutoBoosterPolicy) ID() string         { return "autoBooster" }
func (*AutoBoosterPolicy) EnabledKey() string { return "auto_booster" }

func (*AutoBoosterPolicy) WakeDomains() []string {
	return []string{"events", "event-scores", "global-effects", "resources"}
}

func (*AutoBoosterPolicy) WakeSections() []string { return []string{autoBoosterSection} }

func (*AutoBoosterPolicy) Evaluate(_ context.Context, snapshot Snapshot) (Decision, error) {
	settings := defaultAutoBoosterSettings()
	if !decodeSection(snapshot.Configuration, autoBoosterSection, &settings) {
		return autoBoosterWaiting(snapshot.Now, settings.CheckIntervalSec, "Auto Booster settings have not been saved", nil), nil
	}
	if settings.Version != 1 || settings.CheckIntervalSec < 30 || settings.CheckIntervalSec > 3600 ||
		settings.RubyCostCeiling != GameData.FortressDailyBoosterRubyCost || settings.MinimumRubyReserve < 0 {
		return autoBoosterWaiting(snapshot.Now, settings.CheckIntervalSec, "Auto Booster requires the exact 2,500-ruby ceiling and a non-negative reserve", nil), nil
	}
	if snapshot.GameData == nil {
		return autoBoosterWaiting(snapshot.Now, settings.CheckIntervalSec, "Official game data is unavailable", nil), nil
	}
	contract, err := snapshot.GameData.FortressSpeed()
	if err != nil {
		return autoBoosterWaiting(snapshot.Now, settings.CheckIntervalSec, err.Error(), nil), nil
	}
	if contract.DailyGlobalEffectID != GameData.FortressDailyGlobalEffectID || contract.DailyBoostPercent <= 0 {
		return autoBoosterWaiting(snapshot.Now, settings.CheckIntervalSec, "Official fortress-speed global effect changed; Auto Booster is paused", nil), nil
	}

	metrics := map[string]float64{
		"globalEffectId":     float64(contract.DailyGlobalEffectID),
		"rubyCostCeiling":    float64(settings.RubyCostCeiling),
		"minimumRubyReserve": float64(settings.MinimumRubyReserve),
	}
	inventory := snapshot.State.EventScores.Inventory
	if inventory.GlobalEffectsObservedAt.IsZero() || snapshot.Now.Before(inventory.GlobalEffectsObservedAt) ||
		snapshot.Now.Sub(inventory.GlobalEffectsObservedAt) >= autoBoosterOfferFreshness {
		return autoBoosterRequest(snapshot.Now, settings.CheckIntervalSec, metrics,
			"Refresh the current daily global-effect offer", "autoBooster.refresh", map[string]any{}), nil
	}

	effect, effectAvailable := inventory.GlobalEffects[contract.DailyGlobalEffectID]
	if !effectAvailable || !effect.ActiveAt(snapshot.Now) {
		return autoBoosterWaiting(snapshot.Now, settings.CheckIntervalSec,
			"The daily fortress-speed global effect is not currently available", metrics), nil
	}
	metrics["effectEndsAtUnix"] = float64(effect.EndsAt.Unix())
	if effect.EndsAt.Sub(snapshot.Now) <= autoBoosterMinimumEffectWindow {
		return Decision{
			Status: "idle", Detail: "Waiting for the next daily fortress-speed effect window",
			NextCheckAt: effect.EndsAt.Add(time.Second), Metrics: metrics,
		}, nil
	}

	if cursor, found := operationalCursor(snapshot.State, "autoBooster", autoBoosterCursorKey); found && int64(cursor) == effect.EndsAt.Unix() {
		metrics["boosted"] = 1
		return Decision{
			Status: "idle", Detail: "Daily fortress-speed boost was accepted for the current effect window",
			NextCheckAt: effect.EndsAt.Add(time.Second), Metrics: metrics,
		}, nil
	}
	boost, boostKnown := inventory.GlobalEffectBoosts[contract.DailyGlobalEffectID]
	boostKnown = boostKnown && boost.GlobalEffectID == contract.DailyGlobalEffectID &&
		boost.OccurrenceEndsAt.Equal(effect.EndsAt) && !boost.ObservedAt.IsZero()
	if boostKnown && boost.Boosted {
		metrics["boosted"] = 1
		return Decision{
			Status: "idle", Detail: "Daily fortress-speed boost is already active",
			NextCheckAt: effect.EndsAt.Add(time.Second), Metrics: metrics,
		}, nil
	}
	if !boostKnown {
		return autoBoosterWaiting(snapshot.Now, settings.CheckIntervalSec,
			"Waiting for the server's current boosted-effect status", metrics), nil
	}

	offer, offered := inventory.GlobalEffectBoosterOffers[contract.DailyGlobalEffectID]
	if !offered {
		return autoBoosterWaiting(snapshot.Now, settings.CheckIntervalSec,
			"The paid fortress-speed boost is not offered in the current daily window", metrics), nil
	}
	metrics["quotedRubyCost"] = float64(offer.RubyCost)
	metrics["quotedBonusValue"] = float64(offer.BonusValue)
	if offer.GlobalEffectID != contract.DailyGlobalEffectID || offer.RubyCost != settings.RubyCostCeiling || offer.BonusValue <= 0 {
		return autoBoosterWaiting(snapshot.Now, settings.CheckIntervalSec,
			fmt.Sprintf("Server quote is not the approved 2,500-ruby fortress-speed offer (quoted %d); no purchase was sent", offer.RubyCost), metrics), nil
	}
	rubies, balanceAvailable := autoBoosterRubyBalance(snapshot.State, snapshot.GameData)
	if !balanceAvailable {
		return autoBoosterWaiting(snapshot.Now, settings.CheckIntervalSec, "Ruby balance is unavailable", metrics), nil
	}
	metrics["rubyBalance"] = float64(rubies)
	if rubies-settings.MinimumRubyReserve < offer.RubyCost {
		return autoBoosterWaiting(snapshot.Now, settings.CheckIntervalSec,
			fmt.Sprintf("Waiting for %d rubies above the configured reserve", offer.RubyCost), metrics), nil
	}

	arguments, _ := json.Marshal(map[string]any{
		"globalEffectId": contract.DailyGlobalEffectID, "expectedEndsAtUnix": effect.EndsAt.Unix(),
		"expectedRubyCost": offer.RubyCost, "expectedBonusValue": offer.BonusValue,
		"minimumRubyReserve": settings.MinimumRubyReserve, "expectedRubyBalance": rubies,
	})
	return Decision{
		Status: "ready", Detail: "Purchase the current daily fortress-speed boost for 2,500 rubies",
		NextCheckAt: snapshot.Now.Add(time.Duration(settings.CheckIntervalSec) * time.Second), Metrics: metrics,
		Request:             &Intent.Request{Name: "autoBooster.purchase", Arguments: arguments},
		OperationalCursor:   &OperationalCursorUpdate{Key: autoBoosterCursorKey, Value: int(effect.EndsAt.Unix())},
		ReevaluateOnSuccess: true, ReevaluateOnStale: true,
	}, nil
}

func defaultAutoBoosterSettings() autoBoosterSettings {
	return autoBoosterSettings{
		Version: 1, CheckIntervalSec: autoBoosterDefaultCheckIntervalSec,
		RubyCostCeiling: GameData.FortressDailyBoosterRubyCost,
	}
}

func autoBoosterRubyBalance(gameState State.GameState, gameData *GameData.Store) (int64, bool) {
	if gameData == nil {
		return 0, false
	}
	resourceID, found := gameData.ResourceIDForJSONKey("C2")
	if !found || resourceID <= 0 {
		return 0, false
	}
	balance, found := gameState.Player.Resources[State.ResourceID(resourceID)]
	return int64(math.Floor(balance)), found
}

func autoBoosterRequest(now time.Time, intervalSec int, metrics map[string]float64, detail, name string, arguments any) Decision {
	raw, _ := json.Marshal(arguments)
	return Decision{
		Status: "ready", Detail: detail, NextCheckAt: now.Add(time.Duration(intervalSec) * time.Second), Metrics: metrics,
		Request: &Intent.Request{Name: name, Arguments: raw}, ReevaluateOnSuccess: true, ReevaluateOnStale: true,
	}
}

func autoBoosterWaiting(now time.Time, intervalSec int, detail string, metrics map[string]float64) Decision {
	if intervalSec < 30 || intervalSec > 3600 {
		intervalSec = autoBoosterDefaultCheckIntervalSec
	}
	return Decision{Status: "waiting", Detail: detail, NextCheckAt: now.Add(time.Duration(intervalSec) * time.Second), Metrics: metrics}
}
