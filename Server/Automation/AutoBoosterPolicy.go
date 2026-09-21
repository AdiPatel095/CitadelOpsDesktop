package Automation

import (
	"CitadelDesktop/Server/Localization"
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
		return autoBoosterWaiting(snapshot.Now, settings.CheckIntervalSec, "Auto Booster settings have not been saved", nil, Localization.New("server.automation.auto_booster_settings_have.0587b25e", "Auto Booster settings have not been saved", nil)), nil
	}
	if settings.Version != 1 || settings.CheckIntervalSec < 30 || settings.CheckIntervalSec > 3600 ||
		settings.RubyCostCeiling != GameData.FortressDailyBoosterRubyCost || settings.MinimumRubyReserve < 0 {
		return autoBoosterWaiting(snapshot.Now, settings.CheckIntervalSec, "Auto Booster requires the exact 2,500-ruby ceiling and a non-negative reserve", nil, Localization.New("server.automation.auto_booster_requires_the.4865e84e", "Auto Booster requires the exact 2,500-ruby ceiling and a non-negative reserve", nil)), nil
	}
	if snapshot.GameData == nil {
		return autoBoosterWaiting(snapshot.Now, settings.CheckIntervalSec, "Official game data is unavailable", nil, Localization.New("server.automation.official_game_data_is.c5e55e7e", "Official game data is unavailable", nil)), nil
	}
	contract, err := snapshot.GameData.FortressSpeed()
	if err != nil {
		return autoBoosterWaiting(snapshot.Now, settings.CheckIntervalSec, err.Error(), nil), nil
	}
	if contract.DailyGlobalEffectID != GameData.FortressDailyGlobalEffectID || contract.DailyBoostPercent <= 0 {
		return autoBoosterWaiting(snapshot.Now, settings.CheckIntervalSec, "Official fortress-speed global effect changed; Auto Booster is paused", nil, Localization.New("server.automation.official_fortress_speed_global.60d2222b", "Official fortress-speed global effect changed; Auto Booster is paused", nil)), nil
	}

	metrics := map[string]float64{
		"globalEffectId":     float64(contract.DailyGlobalEffectID),
		"rubyCostCeiling":    float64(settings.RubyCostCeiling),
		"minimumRubyReserve": float64(settings.MinimumRubyReserve),
	}
	inventory := snapshot.State.EventScores.Inventory
	if inventory.GlobalEffectReadObservedAt.IsZero() || snapshot.Now.Before(inventory.GlobalEffectReadObservedAt) ||
		snapshot.Now.Sub(inventory.GlobalEffectReadObservedAt) >= autoBoosterOfferFreshness ||
		inventory.GlobalEffectReadGeneration != snapshot.State.Session.ConnectionGeneration ||
		(!snapshot.State.Session.ChangedAt.IsZero() && inventory.GlobalEffectReadObservedAt.Before(snapshot.State.Session.ChangedAt)) {
		return autoBoosterRequest(snapshot.Now, settings.CheckIntervalSec, metrics,
			"Refresh the current daily effect, boost status, and ruby balance", "autoBooster.refresh", map[string]any{}), nil
	}
	if !inventory.GlobalEffectBaselineObservedAt.Equal(inventory.GlobalEffectReadObservedAt) ||
		inventory.GlobalEffectBaselineGeneration != snapshot.State.Session.ConnectionGeneration {
		return autoBoosterWaiting(snapshot.Now, settings.CheckIntervalSec,
			"The latest account snapshot did not contain valid trigger-event, boost-status, and ruby-balance data", metrics, Localization.New("server.automation.the_latest_account_snapshot.33bd6d36", "The latest account snapshot did not contain valid trigger-event, boost-status, and ruby-balance data", nil)), nil
	}
	metrics["baselineObservedAtUnix"] = float64(inventory.GlobalEffectBaselineObservedAt.Unix())

	effect, effectAvailable := inventory.GlobalEffects[contract.DailyGlobalEffectID]
	if !effectAvailable || !effect.ActiveAt(snapshot.Now) {
		return autoBoosterWaiting(snapshot.Now, settings.CheckIntervalSec,
			"The daily fortress-speed global effect is not currently available", metrics, Localization.New("server.automation.the_daily_fortress_speed.b8bf090f", "The daily fortress-speed global effect is not currently available", nil)), nil
	}
	metrics["effectEndsAtUnix"] = float64(effect.EndsAt.Unix())
	metrics["effectRemainingSec"] = effect.EndsAt.Sub(snapshot.Now).Seconds()
	if effect.EndsAt.Sub(snapshot.Now) <= autoBoosterMinimumEffectWindow {
		return Decision{
			Status: "idle", Detail: "Waiting for the next daily fortress-speed effect window", DetailDescriptor: Localization.New("server.automation.waiting_for_the_next.d4627489", "Waiting for the next daily fortress-speed effect window", nil),
			NextCheckAt: effect.EndsAt.Add(time.Second), Metrics: metrics,
		}, nil
	}

	if record, found := inventory.GlobalEffectPurchases[contract.DailyGlobalEffectID]; found &&
		State.SameEventOccurrence(record.OccurrenceEndsAt, effect.EndsAt) {
		switch record.Outcome {
		case State.GlobalEffectPurchaseConfirmed:
			metrics["boosted"] = 1
			return Decision{Status: "idle", Detail: "Daily fortress-speed boost is active until the current event ends", DetailDescriptor: Localization.New("server.automation.daily_fortress_speed_boost.ee62ac4e", "Daily fortress-speed boost is active until the current event ends", nil), NextCheckAt: effect.EndsAt.Add(time.Second), Metrics: metrics}, nil
		case State.GlobalEffectPurchaseAccepted:
			return autoBoosterWaiting(snapshot.Now, settings.CheckIntervalSec, "Boost purchase accepted; awaiting current active-state confirmation", metrics, Localization.New("server.automation.boost_purchase_accepted_awaiting.7838db66", "Boost purchase accepted; awaiting current active-state confirmation", nil)), nil
		case State.GlobalEffectPurchaseUnresolved:
			return autoBoosterWaiting(snapshot.Now, settings.CheckIntervalSec, "Boost purchase outcome is unresolved; waiting for an authoritative account snapshot", metrics, Localization.New("server.automation.boost_purchase_outcome_is.e151f4a8", "Boost purchase outcome is unresolved; waiting for an authoritative account snapshot", nil)), nil
		}
	}
	if cursor, found := operationalCursor(snapshot.State, "autoBooster", autoBoosterCursorKey); found &&
		State.SameEventOccurrence(time.Unix(int64(cursor), 0).UTC(), effect.EndsAt) {
		return Decision{
			Status: "waiting", Detail: "Daily fortress-speed boost purchase was accepted; awaiting current active-state confirmation", DetailDescriptor: Localization.New("server.automation.daily_fortress_speed_boost.d49271ad", "Daily fortress-speed boost purchase was accepted; awaiting current active-state confirmation", nil),
			NextCheckAt: snapshot.Now.Add(time.Duration(settings.CheckIntervalSec) * time.Second), Metrics: metrics,
		}, nil
	}
	boost, boostKnown := inventory.GlobalEffectBoosts[contract.DailyGlobalEffectID]
	boostKnown = boostKnown && boost.GlobalEffectID == contract.DailyGlobalEffectID &&
		State.SameEventOccurrence(boost.OccurrenceEndsAt, effect.EndsAt) && !boost.ObservedAt.IsZero() &&
		boost.ObservedAt.Equal(inventory.GlobalEffectBaselineObservedAt) &&
		boost.ConnectionGeneration == snapshot.State.Session.ConnectionGeneration
	if boostKnown && boost.Boosted {
		metrics["boosted"] = 1
		return Decision{
			Status: "idle", Detail: "Daily fortress-speed boost is already active", DetailDescriptor: Localization.New("server.automation.daily_fortress_speed_boost.b20ecc03", "Daily fortress-speed boost is already active", nil),
			NextCheckAt: effect.EndsAt.Add(time.Second), Metrics: metrics,
		}, nil
	}
	if !boostKnown {
		return autoBoosterWaiting(snapshot.Now, settings.CheckIntervalSec,
			"Waiting for the server's current boosted-effect status", metrics, Localization.New("server.automation.waiting_for_the_server.e21d4cb3", "Waiting for the server's current boosted-effect status", nil)), nil
	}

	offer, offered := inventory.GlobalEffectBoosterOffers[contract.DailyGlobalEffectID]
	if !offered {
		return autoBoosterWaiting(snapshot.Now, settings.CheckIntervalSec,
			"The paid fortress-speed boost is not offered in the current daily window", metrics, Localization.New("server.automation.the_paid_fortress_speed.789ad527", "The paid fortress-speed boost is not offered in the current daily window", nil)), nil
	}
	metrics["quotedRubyCost"] = float64(offer.RubyCost)
	metrics["quotedBonusValue"] = float64(offer.BonusValue)
	if offer.GlobalEffectID != contract.DailyGlobalEffectID || offer.RubyCost != settings.RubyCostCeiling || offer.BonusValue <= 0 {
		return autoBoosterWaiting(snapshot.Now, settings.CheckIntervalSec,
			fmt.Sprintf("Server quote is not the approved 2,500-ruby fortress-speed offer (quoted %d); no purchase was sent", offer.RubyCost), metrics, Localization.New("server.automation.server_quote_is_not.8efefa0a", "Server quote is not the approved 2,500-ruby fortress-speed offer (quoted {p0, number}); no purchase was sent", Localization.Params{"p0": offer.RubyCost})), nil
	}
	rubies, balanceAvailable := autoBoosterRubyBalance(snapshot.State, snapshot.GameData)
	resourceID, resourceFound := snapshot.GameData.ResourceIDForJSONKey("C2")
	resourceObservation := snapshot.State.Player.ResourceObservations[State.ResourceID(resourceID)]
	if !balanceAvailable || !resourceFound || resourceID <= 0 || resourceObservation.ObservedAt.IsZero() ||
		!resourceObservation.ObservedAt.Equal(inventory.GlobalEffectBaselineObservedAt) ||
		resourceObservation.ConnectionGeneration != snapshot.State.Session.ConnectionGeneration {
		return autoBoosterWaiting(snapshot.Now, settings.CheckIntervalSec, "A fresh current-session ruby balance is unavailable", metrics, Localization.New("server.automation.a_fresh_current_session.4ebab05c", "A fresh current-session ruby balance is unavailable", nil)), nil
	}
	metrics["rubyBalance"] = float64(rubies)
	if rubies-settings.MinimumRubyReserve < offer.RubyCost {
		return autoBoosterWaiting(snapshot.Now, settings.CheckIntervalSec,
			fmt.Sprintf("Waiting for %d rubies above the configured reserve", offer.RubyCost), metrics, Localization.New("server.automation.waiting_for_p_rubies.5b48f3bd", "Waiting for {p0, number} rubies above the configured reserve", Localization.Params{"p0": offer.RubyCost})), nil
	}

	arguments, _ := json.Marshal(map[string]any{
		"globalEffectId": contract.DailyGlobalEffectID, "expectedEndsAtUnix": effect.EndsAt.Unix(),
		"expectedRubyCost": offer.RubyCost, "expectedBonusValue": offer.BonusValue,
		"minimumRubyReserve": settings.MinimumRubyReserve, "expectedCheckIntervalSec": settings.CheckIntervalSec,
		"expectedRubyBalance":        rubies,
		"expectedBaselineObservedAt": inventory.GlobalEffectBaselineObservedAt,
		"expectedRubyObservedAt":     resourceObservation.ObservedAt,
		"expectedSessionGeneration":  snapshot.State.Session.ConnectionGeneration,
		"expectedRubyResourceId":     resourceID,
	})
	return Decision{
		Status: "ready", Detail: "Purchase the current daily fortress-speed boost for 2,500 rubies", DetailDescriptor: Localization.New("server.automation.purchase_the_current_daily.deac3945", "Purchase the current daily fortress-speed boost for 2,500 rubies", nil),
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

func autoBoosterWaiting(now time.Time, intervalSec int, detail string, metrics map[string]float64, descriptors ...*Localization.Message) Decision {
	if intervalSec < 30 || intervalSec > 3600 {
		intervalSec = autoBoosterDefaultCheckIntervalSec
	}
	return Decision{Status: "waiting", Detail: detail, DetailDescriptor: Localization.First(descriptors), NextCheckAt: now.Add(time.Duration(intervalSec) * time.Second), Metrics: metrics}
}
