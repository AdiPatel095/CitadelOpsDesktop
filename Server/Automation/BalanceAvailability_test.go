package Automation

import (
	"fmt"
	"testing"
	"time"

	"CitadelDesktop/Server/Intent"
	"CitadelDesktop/Server/State"
)

func TestBalanceAvailabilityParsesAndWaitsWithoutFailurePause(t *testing.T) {
	now := time.Now().UTC()
	for _, key := range []Intent.BalanceKey{Intent.CurrencyBalanceKey(69), Intent.PlayerResourceBalanceKey(3), Intent.CastleResourceBalanceKey(77118, 1)} {
		shortage := &Intent.BalanceUnavailableError{Key: key, Required: 2, Known: true, Name: "Arrows", NameKey: "currency_name_Arrows"}
		explanation := shortage.Detail()
		descriptor := shortage.LocalizationMessage()
		result := operationResult{policyID: "autoRecruit", receipt: Intent.Receipt{Status: Intent.StatusFailed, RawError: "wrapped: " + shortage.Error(), Failure: &Intent.FailurePresentation{Explanation: explanation, ExplanationDescriptor: descriptor}}}
		gate, ok := operationResultCoinAvailabilityGate(result)
		if !ok || gate.key != key || gate.detail != explanation || gate.detailDescriptor.Key != descriptor.Key {
			t.Fatalf("gate %+v %v", gate, ok)
		}
		current := &policyRuntime{running: true, evaluatedStateRevision: 10}
		_, immediate := completePolicyRun(current, result, now)
		if immediate || current.coinAvailabilityGate == nil || !current.failureBlockedUntil.IsZero() || !current.nextCheck.Equal(now.Add(30*time.Second)) {
			t.Fatalf("wait %+v", current)
		}
		if coinAvailabilityGateWaiting(current, now.Add(30*time.Second)) {
			t.Fatal("bounded recheck did not expire")
		}
		result.receipt.Failure = nil
		fallback, ok := operationResultCoinAvailabilityGate(result)
		if !ok || fallback.detailDescriptor.Key != "server.balance.waiting" {
			t.Fatal("fallback", fallback)
		}
	}
	for _, text := range []string{"not enough balance for dispatch: bad:69 2 needed; 0 available from 0 observed after 0 pending", "not enough balance for dispatch: currency:69 invalid", "not enough balance for dispatch: castle_resource:1@0 2 needed; 0 available from 0 observed after 0 pending"} {
		if _, ok := operationResultCoinAvailabilityGate(operationResult{receipt: Intent.Receipt{Status: Intent.StatusFailed, RawError: text}}); ok {
			t.Fatal("parsed", text)
		}
	}
}
func TestBalanceAvailabilityWakesOnlyForItsBalance(t *testing.T) {
	now := time.Now().UTC()
	keys := []Intent.BalanceKey{{}, Intent.CurrencyBalanceKey(69), Intent.PlayerResourceBalanceKey(3), Intent.CastleResourceBalanceKey(77118, 1)}
	for _, key := range keys {
		t.Run(key.String(), func(t *testing.T) {
			initial := func() State.GameState {
				s := State.NewGameState()
				s.Player.Resources[1] = 0
				s.Player.Resources[3] = 0
				s.Player.Currencies[69] = 0
				s.Castles[77118] = State.CastleState{ID: 77118, ContextSnapshotObservedAt: now, Resources: map[State.ResourceID]State.ResourceBalance{1: {Amount: 0}}}
				return s
			}
			gate := coinAvailabilityGate{key: key, observedAt: now}
			state := initial()
			state.Player.Currencies[22] = 100
			state.Player.Resources[99] = 100
			state.Player.Currencies[88] = 100
			state.Castles[77119] = State.CastleState{ID: 77119, ContextSnapshotObservedAt: now.Add(time.Second)}
			if coinAvailabilityGateChanged(&gate, &state) {
				t.Fatal("unrelated balance woke gate")
			}
			state = initial()
			if key.Kind == Intent.BalanceCastleResource {
				castle := state.Castles[77118]
				castle.Resources[1] = State.ResourceBalance{Amount: 3}
				state.Castles[77118] = castle
				if coinAvailabilityGateChanged(&gate, &state) {
					t.Fatal("GRC tick woke castle gate")
				}
				castle.ContextSnapshotObservedAt = now.Add(time.Second)
				state.Castles[77118] = castle
			} else if key.Kind == Intent.BalanceCurrency {
				state.Player.CurrencyObservations[69] = State.PlayerResourceObservation{ObservedAt: now.Add(time.Second)}
			} else {
				id := State.ResourceID(key.ID)
				if id == 0 {
					id = 1
				}
				state.Player.ResourceObservations[id] = State.PlayerResourceObservation{ObservedAt: now.Add(time.Second)}
			}
			if !coinAvailabilityGateChanged(&gate, &state) {
				t.Fatal("new observation did not wake")
			}
			current := &policyRuntime{coinAvailabilityGate: &gate, evaluatedStateRevision: 10}
			domains := []string{"resources", "currencies"}
			if key.Kind == Intent.BalanceCastleResource {
				domains = []string{"castles"}
			}
			clearCoinAvailabilityGates(map[string]*policyRuntime{"autoRecruit": current}, State.Event{Revision: 11, Domains: domains}, &state)
			if current.coinAvailabilityGate != nil || !current.evaluationPending {
				t.Fatal("observation event did not clear gate")
			}
			if key.Kind == Intent.BalanceCastleResource {
				state = initial()
				delete(state.Castles, 77118)
				if !coinAvailabilityGateChanged(&gate, &state) {
					t.Fatal("missing castle")
				}
			}
		})
	}
}
func TestCIT119FeatherWaitUnchanged(t *testing.T) {
	now := time.Now().UTC()
	for _, known := range []bool{true, false} {
		shortage := &Intent.CurrencyUnavailableError{CurrencyID: 22, Required: 1, Observed: 0, Known: known}
		golden := "not enough travel tickets for dispatch: 1 needed; 0 available from 0 observed after 0 pending (currency 22)"
		key := "server.travel_tickets.short"
		detail := "Not enough travel tickets: 1 needed, 0 available"
		if !known {
			golden += ": current-session balance unavailable"
			key = "server.travel_tickets.unavailable"
			detail = "Travel ticket balance is unavailable; waiting for fresh game data"
		}
		if shortage.Error() != golden {
			t.Fatal(shortage.Error())
		}
		gate, ok := operationResultCoinAvailabilityGate(operationResult{receipt: Intent.Receipt{Status: Intent.StatusFailed, RawError: fmt.Sprintf("outer: %s", golden)}})
		if !ok || gate.key != Intent.CurrencyBalanceKey(22) || gate.detail != detail || gate.detailDescriptor.Key != key {
			t.Fatalf("ticket drift %+v", gate)
		}
		gate.observedAt = now
		state := State.NewGameState()
		state.Player.Resources[1] = 500
		state.Player.Currencies[69] = 300
		state.Castles[77118] = State.CastleState{ContextSnapshotObservedAt: now.Add(time.Second)}
		if coinAvailabilityGateChanged(&gate, &state) {
			t.Fatal("unrelated event woke ticket gate")
		}
		state.Player.CurrencyObservations[22] = State.PlayerResourceObservation{ObservedAt: now.Add(time.Second)}
		if !coinAvailabilityGateChanged(&gate, &state) {
			t.Fatal("ticket observation did not wake")
		}
	}
}

func TestBalanceAvailabilityEventDomains(t *testing.T) {
	now := time.Now().UTC()
	for _, key := range []Intent.BalanceKey{{}, Intent.PlayerResourceBalanceKey(3), Intent.CurrencyBalanceKey(69), Intent.CastleResourceBalanceKey(77118, 1)} {
		for _, domain := range []string{"resources", "currencies", "castles"} {
			state := State.NewGameState()
			state.Player.Resources[1] = 1
			state.Player.Resources[3] = 1
			state.Player.Currencies[69] = 1
			state.Castles[77118] = State.CastleState{ContextSnapshotObservedAt: now.Add(time.Second)}
			current := &policyRuntime{coinAvailabilityGate: &coinAvailabilityGate{key: key, observedAt: now}, evaluatedStateRevision: 10}
			clearCoinAvailabilityGates(map[string]*policyRuntime{"sample": current}, State.Event{Revision: 11, Domains: []string{domain}}, &state)
			expected := "resources"
			if key.Kind == Intent.BalanceCurrency {
				expected = "currencies"
			}
			if key.Kind == Intent.BalanceCastleResource {
				expected = "castles"
			}
			if (current.coinAvailabilityGate == nil) != (domain == expected) {
				t.Fatalf("%v woke on %s", key, domain)
			}
		}
	}
}

func TestMarketBarrowAvailabilityWaitAndSourceWake(t *testing.T) {
	now := time.Now().UTC()
	key := Intent.MarketBarrowBalanceKey(77127)
	shortage := &Intent.BalanceUnavailableError{Key: key, Required: 2, Observed: 1, Known: true, CastleName: "Invented keep"}
	result := operationResult{policyID: "autoFoodBalance", receipt: Intent.Receipt{Status: Intent.StatusFailed, RawError: shortage.Error(), Failure: &Intent.FailurePresentation{Explanation: shortage.Detail(), ExplanationDescriptor: shortage.LocalizationMessage()}}}
	gate, ok := operationResultCoinAvailabilityGate(result)
	if !ok || gate.key != key || gate.detailDescriptor.Key != "server.market_barrows.short" {
		t.Fatalf("gate=%+v", gate)
	}
	current := &policyRuntime{running: true, evaluatedStateRevision: 10}
	_, immediate := completePolicyRun(current, result, now)
	if immediate || current.coinAvailabilityGate == nil || !current.failureBlockedUntil.IsZero() || !current.nextCheck.Equal(now.Add(30*time.Second)) {
		t.Fatalf("runtime=%+v", current)
	}
	for _, event := range []struct {
		name, domain                  string
		sourceObserved, otherObserved time.Time
		missing, wake                 bool
	}{
		{"movement tick", "movements", now, now, false, false},
		{"same row market tick", "market", now, now, false, false},
		{"other castle cmi", "market", now, now.Add(time.Second), false, false},
		{"source cmi in wrong domain", "movements", now.Add(time.Second), now, false, false},
		{"source cmi", "market", now.Add(time.Second), now, false, true},
		{"source omitted", "market", now, now, true, true},
	} {
		t.Run(event.name, func(t *testing.T) {
			state := State.NewGameState()
			if !event.missing {
				state.Market.Castles[77127] = State.MarketCastleState{CastleID: 77127, ObservedAt: event.sourceObserved}
			}
			state.Market.Castles[77128] = State.MarketCastleState{CastleID: 77128, ObservedAt: event.otherObserved}
			gate.observedAt = now
			current := &policyRuntime{coinAvailabilityGate: &gate, evaluatedStateRevision: 10}
			clearCoinAvailabilityGates(map[string]*policyRuntime{"sample": current}, State.Event{Revision: 11, Domains: []string{event.domain}}, &state)
			if (current.coinAvailabilityGate == nil) != event.wake {
				t.Fatalf("wake=%t want=%t", current.coinAvailabilityGate == nil, event.wake)
			}
		})
	}
}
