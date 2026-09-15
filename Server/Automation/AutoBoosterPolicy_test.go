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
	gameState.Session.ChangedAt = now.Add(-time.Minute)
	gameState.Player.Resources[2] = 10_000
	gameState.Player.ResourceObservations[2] = State.PlayerResourceObservation{ObservedAt: now}
	gameState.EventScores.Inventory = State.EventInventoryState{
		ObservedAt: now, ActiveByEvent: map[int64]State.EventAvailability{},
		GlobalEffectsObservedAt: now, GlobalEffectReadObservedAt: now, GlobalEffectBaselineObservedAt: now,
		GlobalEffects: map[int64]State.GlobalEffectAvailability{
			2: {GlobalEffectID: 2, Strength: 10, EndsAt: endsAt},
		},
		GlobalEffectBoosterOffers: map[int64]State.GlobalEffectBoosterOffer{
			2: {GlobalEffectID: 2, RubyCost: 2500, BonusValue: 50},
		},
		GlobalEffectBoostsObservedAt: now,
		GlobalEffectBoosts: map[int64]State.GlobalEffectBoostState{
			2: {GlobalEffectID: 2, Boosted: false, OccurrenceEndsAt: endsAt, ObservedAt: now},
		},
		GlobalEffectPurchases: map[int64]State.GlobalEffectPurchaseRecord{},
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

func TestAutoBoosterDurablePurchaseRecordSuppressesReplayAcrossReconnect(t *testing.T) {
	now := time.Date(2026, time.September, 15, 12, 0, 0, 0, time.UTC)
	endsAt := now.Add(time.Hour)
	configuration := Configuration.Snapshot{Sections: map[string]json.RawMessage{
		autoBoosterSection: json.RawMessage(`{"version":1,"checkIntervalSec":60,"rubyCostCeiling":2500,"minimumRubyReserve":0}`),
	}}
	for _, outcome := range []string{State.GlobalEffectPurchaseUnresolved, State.GlobalEffectPurchaseAccepted, State.GlobalEffectPurchaseConfirmed} {
		t.Run(outcome, func(t *testing.T) {
			gameState := autoBoosterPolicyState(now, endsAt, 3)
			gameState.EventScores.Inventory.GlobalEffectPurchases[2] = State.GlobalEffectPurchaseRecord{
				GlobalEffectID: 2, OccurrenceEndsAt: endsAt.Add(-time.Minute), ExpiresAt: endsAt.Add(-time.Minute),
				Outcome: outcome, OperationID: "persisted-op", DebitUnverified: true,
			}
			decision, err := NewAutoBoosterPolicy().Evaluate(t.Context(), Snapshot{
				State: gameState, Configuration: configuration, GameData: autoFortressTestGameData(t), Now: now,
			})
			if err != nil || decision.Request != nil {
				t.Fatalf("persisted %s record replayed: decision=%+v err=%v", outcome, decision, err)
			}

			gameState.Session.ConnectionGeneration = 4
			gameState.Session.ChangedAt = now.Add(time.Second)
			decision, err = NewAutoBoosterPolicy().Evaluate(t.Context(), Snapshot{
				State: gameState, Configuration: configuration, GameData: autoFortressTestGameData(t), Now: now.Add(2 * time.Second),
			})
			if err != nil || decision.Request == nil || decision.Request.Name != "autoBooster.refresh" {
				t.Fatalf("reconnect did not require GBD refresh: decision=%+v err=%v", decision, err)
			}
			refreshedAt := now.Add(3 * time.Second)
			gameState.EventScores.Inventory.GlobalEffectReadObservedAt = refreshedAt
			gameState.EventScores.Inventory.GlobalEffectReadGeneration = 4
			gameState.EventScores.Inventory.GlobalEffectBaselineObservedAt = refreshedAt
			gameState.EventScores.Inventory.GlobalEffectBaselineGeneration = 4
			status := gameState.EventScores.Inventory.GlobalEffectBoosts[2]
			status.ObservedAt, status.ConnectionGeneration = refreshedAt, 4
			gameState.EventScores.Inventory.GlobalEffectBoosts[2] = status
			gameState.Player.ResourceObservations[2] = State.PlayerResourceObservation{ObservedAt: refreshedAt, ConnectionGeneration: 4}
			decision, err = NewAutoBoosterPolicy().Evaluate(t.Context(), Snapshot{
				State: gameState, Configuration: configuration, GameData: autoFortressTestGameData(t), Now: refreshedAt,
			})
			if err != nil || decision.Request != nil {
				t.Fatalf("refreshed persisted %s record replayed: decision=%+v err=%v", outcome, decision, err)
			}
		})
	}
}

func TestAutoBoosterWaitsConfiguredIntervalAfterIncompleteSuccessfulRead(t *testing.T) {
	now := time.Date(2026, time.September, 15, 12, 0, 0, 0, time.UTC)
	gameState := autoBoosterPolicyState(now, now.Add(time.Hour), 2)
	gameState.EventScores.Inventory.GlobalEffectBaselineObservedAt = time.Time{}
	gameState.EventScores.Inventory.GlobalEffectBaselineGeneration = 0
	configuration := Configuration.Snapshot{Sections: map[string]json.RawMessage{
		autoBoosterSection: json.RawMessage(`{"version":1,"checkIntervalSec":90,"rubyCostCeiling":2500,"minimumRubyReserve":0}`),
	}}
	decision, err := NewAutoBoosterPolicy().Evaluate(t.Context(), Snapshot{
		State: gameState, Configuration: configuration, GameData: autoFortressTestGameData(t), Now: now.Add(time.Second),
	})
	if err != nil || decision.Request != nil || decision.Status != "waiting" || !decision.NextCheckAt.Equal(now.Add(91*time.Second)) {
		t.Fatalf("incomplete successful GBD retried immediately: decision=%+v err=%v", decision, err)
	}

	gameState = autoBoosterPolicyState(now, now.Add(time.Hour), 2)
	delete(gameState.EventScores.Inventory.GlobalEffects, 2)
	decision, err = NewAutoBoosterPolicy().Evaluate(t.Context(), Snapshot{
		State: gameState, Configuration: configuration, GameData: autoFortressTestGameData(t), Now: now.Add(time.Second),
	})
	if err != nil || decision.Request != nil || decision.Status != "waiting" {
		t.Fatalf("complete GBD without effect started a refresh loop: decision=%+v err=%v", decision, err)
	}
	gameState = autoBoosterPolicyState(now, now.Add(time.Hour), 2)
	delete(gameState.EventScores.Inventory.GlobalEffectBoosterOffers, 2)
	decision, err = NewAutoBoosterPolicy().Evaluate(t.Context(), Snapshot{
		State: gameState, Configuration: configuration, GameData: autoFortressTestGameData(t), Now: now.Add(time.Second),
	})
	if err != nil || decision.Request != nil || decision.Status != "waiting" {
		t.Fatalf("complete GBD without offer started a refresh loop: decision=%+v err=%v", decision, err)
	}
}

func TestAutoBoosterRejectsPurchaseAtThirtySecondExpiryBoundary(t *testing.T) {
	now := time.Date(2026, time.September, 15, 12, 0, 0, 0, time.UTC)
	gameState := autoBoosterPolicyState(now, now.Add(30*time.Second), 2)
	configuration := Configuration.Snapshot{Sections: map[string]json.RawMessage{
		autoBoosterSection: json.RawMessage(`{"version":1,"checkIntervalSec":60,"rubyCostCeiling":2500,"minimumRubyReserve":0}`),
	}}
	decision, err := NewAutoBoosterPolicy().Evaluate(t.Context(), Snapshot{
		State: gameState, Configuration: configuration, GameData: autoFortressTestGameData(t), Now: now,
	})
	if err != nil || decision.Request != nil || decision.Status != "idle" || !decision.NextCheckAt.Equal(now.Add(31*time.Second)) {
		t.Fatalf("near-expiry effect authorized purchase: decision=%+v err=%v", decision, err)
	}
}

func autoBoosterPolicyState(observedAt, endsAt time.Time, generation uint64) State.GameState {
	gameState := State.NewGameState()
	gameState.Session.ConnectionGeneration = generation
	gameState.Session.ChangedAt = observedAt.Add(-time.Minute)
	gameState.Player.Resources[2] = 10000
	gameState.Player.ResourceObservations[2] = State.PlayerResourceObservation{ObservedAt: observedAt, ConnectionGeneration: generation}
	gameState.EventScores.Inventory = State.EventInventoryState{
		ObservedAt: observedAt, ActiveByEvent: map[int64]State.EventAvailability{},
		GlobalEffectsObservedAt: observedAt, GlobalEffectReadObservedAt: observedAt, GlobalEffectReadGeneration: generation,
		GlobalEffectBaselineObservedAt: observedAt, GlobalEffectBaselineGeneration: generation,
		GlobalEffects:                map[int64]State.GlobalEffectAvailability{2: {GlobalEffectID: 2, Strength: 10, EndsAt: endsAt}},
		GlobalEffectBoosterOffers:    map[int64]State.GlobalEffectBoosterOffer{2: {GlobalEffectID: 2, RubyCost: 2500, BonusValue: 50}},
		GlobalEffectBoostsObservedAt: observedAt,
		GlobalEffectBoosts: map[int64]State.GlobalEffectBoostState{2: {
			GlobalEffectID: 2, OccurrenceEndsAt: endsAt, ObservedAt: observedAt, ConnectionGeneration: generation,
		}},
		GlobalEffectPurchases: map[int64]State.GlobalEffectPurchaseRecord{},
	}
	return gameState
}

func TestAutoBoosterDoesNotRepurchaseCurrentWindowOrChangedPrice(t *testing.T) {
	now := time.Date(2026, time.September, 2, 17, 0, 0, 0, time.UTC)
	endsAt := now.Add(24 * time.Hour).Truncate(time.Minute)
	gameState := State.NewGameState()
	gameState.Session.ChangedAt = now.Add(-time.Minute)
	gameState.Player.Resources[2] = 10_000
	gameState.Player.ResourceObservations[2] = State.PlayerResourceObservation{ObservedAt: now}
	gameState.EventScores.Inventory = State.EventInventoryState{
		ObservedAt: now, ActiveByEvent: map[int64]State.EventAvailability{}, GlobalEffectsObservedAt: now, GlobalEffectReadObservedAt: now, GlobalEffectBaselineObservedAt: now,
		GlobalEffects:                map[int64]State.GlobalEffectAvailability{2: {GlobalEffectID: 2, Strength: 60, EndsAt: endsAt}},
		GlobalEffectBoosterOffers:    map[int64]State.GlobalEffectBoosterOffer{2: {GlobalEffectID: 2, RubyCost: 2500, BonusValue: 60}},
		GlobalEffectBoostsObservedAt: now,
		GlobalEffectBoosts:           map[int64]State.GlobalEffectBoostState{2: {GlobalEffectID: 2, OccurrenceEndsAt: endsAt, ObservedAt: now}},
		GlobalEffectPurchases:        map[int64]State.GlobalEffectPurchaseRecord{},
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
	if err != nil || decision.Request != nil || decision.Status != "waiting" {
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
