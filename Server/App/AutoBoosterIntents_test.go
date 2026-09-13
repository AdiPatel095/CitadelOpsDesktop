package App

import (
	"encoding/json"
	"errors"
	"testing"
	"time"

	"CitadelDesktop/Server/Intent"
	"CitadelDesktop/Server/State"
)

func TestAutoBoosterPurchaseRefreshesGuardsAndSendsAuthoritativeAGBShape(t *testing.T) {
	now := time.Now().UTC()
	endsAt := now.Add(time.Hour).Truncate(time.Minute)
	gameState := State.NewGameState()
	gameState.Player.Resources[2] = 10_000
	gameState.EventScores.Inventory = autoBoosterIntentInventory(now, endsAt, 2500, false)
	arguments, _ := json.Marshal(autoBoosterPurchaseRequest{
		GlobalEffectID: 2, ExpectedEndsAtUnix: endsAt.Unix(), ExpectedRubyCost: 2500,
		ExpectedBonusValue: 60, MinimumRubyReserve: 5000, ExpectedRubyBalance: 10_000,
	})
	plan, err := planAutoBoosterPurchase(t.Context(), Intent.PlanningContext{
		State: gameState, GameData: fortressIntentGameData(t),
	}, arguments)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Steps) != 3 || plan.Steps[0].Opcode != "sei" ||
		plan.Steps[0].ResponseBarrier != Intent.ResponseBarrierCommitted ||
		plan.Steps[1].Action != "auto_booster.purchase.guard" || plan.Steps[2].Opcode != "agb" ||
		plan.Steps[2].ResponseBarrier != Intent.ResponseBarrierCommitted {
		t.Fatalf("Auto Booster steps = %#v", plan.Steps)
	}
	var payload map[string]int64
	if err := json.Unmarshal(plan.Steps[2].Command.Payload, &payload); err != nil {
		t.Fatal(err)
	}
	if len(payload) != 1 || payload["GEID"] != 2 {
		t.Fatalf("AGB payload = %#v", payload)
	}
}

func TestAutoBoosterPurchaseGuardRejectsChangedQuoteBalanceAndBoostState(t *testing.T) {
	now := time.Now().UTC()
	endsAt := now.Add(time.Hour).Truncate(time.Minute)
	gameState := State.NewGameState()
	gameState.Player.Resources[2] = 10_000
	gameState.EventScores.Inventory = autoBoosterIntentInventory(now, endsAt, 2500, false)
	request := autoBoosterPurchaseRequest{
		GlobalEffectID: 2, ExpectedEndsAtUnix: endsAt.Unix(), ExpectedRubyCost: 2500,
		ExpectedBonusValue: 60, MinimumRubyReserve: 5000, ExpectedRubyBalance: 10_000,
	}
	arguments, _ := json.Marshal(request)
	input := Intent.PlanningContext{State: gameState, GameData: fortressIntentGameData(t)}
	if _, err := autoBoosterPurchaseContext(input, arguments, now, true); err != nil {
		t.Fatalf("valid guarded purchase: %v", err)
	}

	offer := gameState.EventScores.Inventory.GlobalEffectBoosterOffers[2]
	offer.RubyCost = 2600
	gameState.EventScores.Inventory.GlobalEffectBoosterOffers[2] = offer
	input.State = gameState
	if _, err := autoBoosterPurchaseContext(input, arguments, now, true); !errors.Is(err, Intent.ErrPlanStale) {
		t.Fatalf("changed quote was accepted: %v", err)
	}

	offer.RubyCost = 2500
	gameState.EventScores.Inventory.GlobalEffectBoosterOffers[2] = offer
	gameState.Player.Resources[2] = 9_999
	input.State = gameState
	if _, err := autoBoosterPurchaseContext(input, arguments, now, true); !errors.Is(err, Intent.ErrPlanStale) {
		t.Fatalf("changed balance was accepted: %v", err)
	}

	gameState.Player.Resources[2] = 10_000
	status := gameState.EventScores.Inventory.GlobalEffectBoosts[2]
	status.Boosted = true
	gameState.EventScores.Inventory.GlobalEffectBoosts[2] = status
	input.State = gameState
	if _, err := autoBoosterPurchaseContext(input, arguments, now, true); !errors.Is(err, Intent.ErrPlanStale) {
		t.Fatalf("already active boost was repurchased: %v", err)
	}
}

func autoBoosterIntentInventory(now, endsAt time.Time, rubyCost int64, boosted bool) State.EventInventoryState {
	return State.EventInventoryState{
		ObservedAt: now, ActiveByEvent: map[int64]State.EventAvailability{}, GlobalEffectsObservedAt: now,
		GlobalEffects: map[int64]State.GlobalEffectAvailability{
			2: {GlobalEffectID: 2, Strength: 60, EndsAt: endsAt},
		},
		GlobalEffectBoosterOffers: map[int64]State.GlobalEffectBoosterOffer{
			2: {GlobalEffectID: 2, RubyCost: rubyCost, BonusValue: 60},
		},
		GlobalEffectBoostsObservedAt: now,
		GlobalEffectBoosts: map[int64]State.GlobalEffectBoostState{
			2: {GlobalEffectID: 2, Boosted: boosted, OccurrenceEndsAt: endsAt, ObservedAt: now},
		},
	}
}
