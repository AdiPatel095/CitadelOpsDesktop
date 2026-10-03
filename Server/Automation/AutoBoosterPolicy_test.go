package Automation

import (
	"encoding/json"
	"strings"
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
	gameState.Session = State.SessionState{Generation: 1, BaselineGeneration: 1, LoggedIn: true, SocketReady: true}
	gameState.Player.RubyConfirmation = State.RubyConfirmationState{Known: true, Amount: -1, Generation: 1}
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

func TestAutoBoosterReportsDistinctAvailabilityAndPurchaseBlockers(t *testing.T) {
	now := time.Date(2026, time.September, 16, 17, 0, 0, 0, time.UTC)
	endsAt := now.Add(time.Hour)
	for _, test := range []struct {
		name          string
		configuration string
		mutate        func(*State.GameState)
		wantDetail    string
	}{
		{name: "invalid snapshot", wantDetail: "valid trigger-event", mutate: func(state *State.GameState) {
			state.EventScores.Inventory.GlobalEffectBaselineObservedAt = time.Time{}
			state.EventScores.Inventory.GlobalEffectBaselineGeneration = 0
		}},
		{name: "unavailable", wantDetail: "not currently available", mutate: func(state *State.GameState) {
			delete(state.EventScores.Inventory.GlobalEffects, 2)
		}},
		{name: "no offer", wantDetail: "not offered", mutate: func(state *State.GameState) {
			delete(state.EventScores.Inventory.GlobalEffectBoosterOffers, 2)
		}},
		{name: "already active", wantDetail: "already active", mutate: func(state *State.GameState) {
			boost := state.EventScores.Inventory.GlobalEffectBoosts[2]
			boost.Boosted = true
			state.EventScores.Inventory.GlobalEffectBoosts[2] = boost
		}},
		{name: "price rejected", wantDetail: "not the approved 2,500-ruby", mutate: func(state *State.GameState) {
			offer := state.EventScores.Inventory.GlobalEffectBoosterOffers[2]
			offer.RubyCost = 2600
			state.EventScores.Inventory.GlobalEffectBoosterOffers[2] = offer
		}},
		{name: "reserve rejected", configuration: `{"version":1,"checkIntervalSec":60,"rubyCostCeiling":2500,"minimumRubyReserve":9000}`, wantDetail: "configured reserve", mutate: func(*State.GameState) {}},
		{name: "unresolved", wantDetail: "outcome is unresolved", mutate: func(state *State.GameState) {
			state.EventScores.Inventory.GlobalEffectPurchases[2] = State.GlobalEffectPurchaseRecord{
				GlobalEffectID: 2, OccurrenceEndsAt: endsAt, Outcome: State.GlobalEffectPurchaseUnresolved,
			}
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			gameState := autoBoosterPolicyState(now, endsAt, 2)
			test.mutate(&gameState)
			configuration := test.configuration
			if configuration == "" {
				configuration = `{"version":1,"checkIntervalSec":60,"rubyCostCeiling":2500,"minimumRubyReserve":0}`
			}
			decision, err := NewAutoBoosterPolicy().Evaluate(t.Context(), Snapshot{
				State: gameState,
				Configuration: Configuration.Snapshot{Sections: map[string]json.RawMessage{
					autoBoosterSection: json.RawMessage(configuration),
				}},
				GameData: autoFortressTestGameData(t), Now: now,
			})
			if err != nil || decision.Request != nil || !strings.Contains(decision.Detail, test.wantDetail) {
				t.Fatalf("blocker detail=%q request=%+v err=%v", decision.Detail, decision.Request, err)
			}
		})
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
	gameState.Session = State.SessionState{Generation: 1, BaselineGeneration: 1, LoggedIn: true, SocketReady: true}
	gameState.Player.RubyConfirmation = State.RubyConfirmationState{Known: true, Amount: -1, Generation: 1}
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
	gameState.Session = State.SessionState{Generation: 1, BaselineGeneration: 1, LoggedIn: true, SocketReady: true}
	gameState.Player.RubyConfirmation = State.RubyConfirmationState{Known: true, Amount: -1, Generation: 1}
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

func TestAutoBoosterRubyConfirmationPolicyMatrix(t *testing.T) {
	now := time.Date(2026, 9, 23, 12, 0, 0, 0, time.UTC)
	configuration := Configuration.Snapshot{Sections: map[string]json.RawMessage{autoBoosterSection: json.RawMessage(`{"version":1,"checkIntervalSec":60,"rubyCostCeiling":2500,"minimumRubyReserve":0}`)}}
	for _, tc := range []struct {
		name       string
		amount     int64
		known      bool
		generation uint64
		want       bool
	}{
		{"250", 250, true, 1, false}, {"2500", 2500, true, 1, false}, {"2501", 2501, true, 1, true}, {"disabled", -1, true, 1, true},
		{"missing", 0, false, 1, false}, {"wrong session", -1, true, 2, false}, {"invalid", 1000001, true, 1, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			state := autoBoosterPolicyState(now, now.Add(time.Hour), 3)
			state.Player.RubyConfirmation = State.RubyConfirmationState{Amount: tc.amount, Known: tc.known, Generation: tc.generation, ObservedAt: now}
			snapshot := Snapshot{State: state, Configuration: configuration, GameData: autoFortressTestGameData(t), Now: now}
			first, err := NewAutoBoosterPolicy().Evaluate(t.Context(), snapshot)
			if err != nil || (first.Request != nil) != tc.want {
				t.Fatalf("decision=%+v err=%v", first, err)
			}
			if !tc.want && (first.DetailDescriptor == nil || !strings.Contains(first.Detail, "You can buy it in the game")) {
				t.Fatalf("notice=%+v", first)
			}
			second, _ := NewAutoBoosterPolicy().Evaluate(t.Context(), snapshot)
			if first.Detail != second.Detail {
				t.Fatal("unchanged notice is not stable")
			}
		})
	}
}

func TestAutoBoosterConfirmationNoticeDeduplicatesAndRechecksChangedThreshold(t *testing.T) {
	now := time.Date(2026, 9, 23, 12, 0, 0, 0, time.UTC)
	gameState := autoBoosterPolicyState(now, now.Add(time.Hour), 3)
	configuration := Configuration.Snapshot{Sections: map[string]json.RawMessage{autoBoosterSection: json.RawMessage(`{"version":1,"checkIntervalSec":60,"rubyCostCeiling":2500,"minimumRubyReserve":0}`)}}
	gameState.Player.RubyConfirmation.Amount = 250
	snapshot := Snapshot{State: gameState, Configuration: configuration, GameData: autoFortressTestGameData(t), Now: now}
	policy := NewAutoBoosterPolicy()
	first, err := policy.Evaluate(t.Context(), snapshot)
	if err != nil {
		t.Fatal(err)
	}
	store := State.NewStore(&gameState)
	coordinator := NewCoordinator(store, nil, nil, nil)
	coordinator.recordDecision("autoBooster", true, first)
	revision := store.Revision()
	coordinator.recordDecision("autoBooster", true, first)
	if store.Revision() != revision {
		t.Fatal("unchanged notice caused another state update")
	}
	snapshot.State.Player.RubyConfirmation.Amount = 2500
	second, _ := policy.Evaluate(t.Context(), snapshot)
	coordinator.recordDecision("autoBooster", true, second)
	if store.Revision() == revision || first.Detail == second.Detail || second.DetailDescriptor.Params["threshold"] != int64(2500) {
		t.Fatal("changed threshold was deduplicated away")
	}
	snapshot.State.Player.RubyConfirmation.Amount = 2501
	allowed, _ := policy.Evaluate(t.Context(), snapshot)
	if allowed.Request == nil {
		t.Fatal("notice permanently suppressed purchase after setting changed")
	}
}

func TestAutoBoosterConfirmationHoldIsScopedToOccurrence(t *testing.T) {
	now := time.Date(2026, 9, 23, 12, 0, 0, 0, time.UTC)
	state := autoBoosterPolicyState(now, now.Add(time.Hour), 3)
	state.Player.RubyConfirmation.ObservedAt = now.Add(-time.Second)
	state.EventScores.Inventory.GlobalEffectPurchases[2] = State.GlobalEffectPurchaseRecord{GlobalEffectID: 2, OccurrenceEndsAt: now.Add(time.Hour), Outcome: State.GlobalEffectPurchaseConfirmationRequired, ResultObservedAt: now, QuotedC2: 2500}
	configuration := Configuration.Snapshot{Sections: map[string]json.RawMessage{autoBoosterSection: json.RawMessage(`{"version":1,"checkIntervalSec":60,"rubyCostCeiling":2500,"minimumRubyReserve":0}`)}}
	snapshot := Snapshot{State: state, Configuration: configuration, GameData: autoFortressTestGameData(t), Now: now}
	policy := NewAutoBoosterPolicy()
	blocked, _ := policy.Evaluate(t.Context(), snapshot)
	if blocked.Request != nil {
		t.Fatal("old observation lifted current occurrence hold")
	}
	// Move to a genuinely different daily occurrence with a current, permissive setting.
	snapshot.State = autoBoosterPolicyState(now, now.Add(25*time.Hour), 3)
	snapshot.State.EventScores.Inventory.GlobalEffectPurchases[2] = state.EventScores.Inventory.GlobalEffectPurchases[2]
	next, _ := policy.Evaluate(t.Context(), snapshot)
	if next.Request == nil || next.Request.Name != "autoBooster.purchase" {
		t.Fatalf("old occurrence blocked new one: %+v", next)
	}
}
