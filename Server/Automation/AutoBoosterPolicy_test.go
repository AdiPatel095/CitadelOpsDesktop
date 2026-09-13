package Automation

import (
	"encoding/json"
	"testing"
	"time"

	"CitadelDesktop/Server/Configuration"
	"CitadelDesktop/Server/GameData"
	"CitadelDesktop/Server/State"
)

func TestAutoBoosterPurchasesOnlyFreshUnboostedExactQuote(t *testing.T) {
	now := time.Date(2026, time.September, 2, 17, 0, 0, 0, time.UTC)
	endsAt := now.Add(24 * time.Hour).Truncate(time.Minute)
	gameState := State.NewGameState()
	gameState.Player.Resources[2] = 10_000
	gameState.EventScores.Inventory = State.EventInventoryState{
		ObservedAt: now, ActiveByEvent: map[int64]State.EventAvailability{},
		GlobalEffectsObservedAt: now,
		GlobalEffects: map[int64]State.GlobalEffectAvailability{
			2: {GlobalEffectID: 2, Strength: 60, EndsAt: endsAt},
		},
		GlobalEffectBoosterOffers: map[int64]State.GlobalEffectBoosterOffer{
			2: {GlobalEffectID: 2, RubyCost: 2500, BonusValue: 60},
		},
		GlobalEffectBoostsObservedAt: now,
		GlobalEffectBoosts: map[int64]State.GlobalEffectBoostState{
			2: {GlobalEffectID: 2, Boosted: false, OccurrenceEndsAt: endsAt, ObservedAt: now},
		},
	}
	configuration := Configuration.Snapshot{Sections: map[string]json.RawMessage{
		autoBoosterSection: json.RawMessage(`{"version":1,"checkIntervalSec":60,"rubyCostCeiling":2500,"minimumRubyReserve":5000}`),
	}}
	decision, err := NewAutoBoosterPolicy().Evaluate(t.Context(), Snapshot{
		State: gameState, Configuration: configuration, GameData: autoFortressTestGameData(t), Now: now,
	})
	if err != nil || decision.Request == nil || decision.Request.Name != "autoBooster.purchase" ||
		decision.OperationalCursor == nil || decision.OperationalCursor.Key != autoBoosterCursorKey ||
		int64(decision.OperationalCursor.Value) != endsAt.Unix() {
		t.Fatalf("Auto Booster purchase decision = %#v err=%v", decision, err)
	}
	var request struct {
		GlobalEffectID      int64 `json:"globalEffectId"`
		ExpectedRubyCost    int64 `json:"expectedRubyCost"`
		MinimumRubyReserve  int64 `json:"minimumRubyReserve"`
		ExpectedRubyBalance int64 `json:"expectedRubyBalance"`
	}
	if err := json.Unmarshal(decision.Request.Arguments, &request); err != nil {
		t.Fatal(err)
	}
	if request.GlobalEffectID != GameData.FortressDailyGlobalEffectID || request.ExpectedRubyCost != 2500 ||
		request.MinimumRubyReserve != 5000 || request.ExpectedRubyBalance != 10_000 {
		t.Fatalf("Auto Booster purchase request = %+v", request)
	}
}

func TestAutoBoosterDoesNotRepurchaseCurrentWindowOrChangedPrice(t *testing.T) {
	now := time.Date(2026, time.September, 2, 17, 0, 0, 0, time.UTC)
	endsAt := now.Add(24 * time.Hour).Truncate(time.Minute)
	gameState := State.NewGameState()
	gameState.Player.Resources[2] = 10_000
	gameState.EventScores.Inventory = State.EventInventoryState{
		ObservedAt: now, ActiveByEvent: map[int64]State.EventAvailability{}, GlobalEffectsObservedAt: now,
		GlobalEffects:                map[int64]State.GlobalEffectAvailability{2: {GlobalEffectID: 2, Strength: 60, EndsAt: endsAt}},
		GlobalEffectBoosterOffers:    map[int64]State.GlobalEffectBoosterOffer{2: {GlobalEffectID: 2, RubyCost: 2500, BonusValue: 60}},
		GlobalEffectBoostsObservedAt: now,
		GlobalEffectBoosts:           map[int64]State.GlobalEffectBoostState{2: {GlobalEffectID: 2, OccurrenceEndsAt: endsAt, ObservedAt: now}},
	}
	gameState.Automations["autoBooster"] = State.AutomationState{
		ID: "autoBooster", OperationalCursors: map[string]int{autoBoosterCursorKey: int(endsAt.Unix())},
	}
	configuration := Configuration.Snapshot{Sections: map[string]json.RawMessage{
		autoBoosterSection: json.RawMessage(`{"version":1,"checkIntervalSec":60,"rubyCostCeiling":2500,"minimumRubyReserve":0}`),
	}}
	policy := NewAutoBoosterPolicy()
	decision, err := policy.Evaluate(t.Context(), Snapshot{
		State: gameState, Configuration: configuration, GameData: autoFortressTestGameData(t), Now: now,
	})
	if err != nil || decision.Request != nil || decision.Status != "idle" {
		t.Fatalf("accepted current window was repurchased: %#v err=%v", decision, err)
	}

	delete(gameState.Automations, "autoBooster")
	offer := gameState.EventScores.Inventory.GlobalEffectBoosterOffers[2]
	offer.RubyCost = 2600
	gameState.EventScores.Inventory.GlobalEffectBoosterOffers[2] = offer
	decision, err = policy.Evaluate(t.Context(), Snapshot{
		State: gameState, Configuration: configuration, GameData: autoFortressTestGameData(t), Now: now,
	})
	if err != nil || decision.Request != nil || decision.Status != "waiting" {
		t.Fatalf("changed server price was accepted: %#v err=%v", decision, err)
	}
}
