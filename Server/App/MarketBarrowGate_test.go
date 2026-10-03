package App

import (
	"CitadelDesktop/Server/GameData"
	"CitadelDesktop/Server/Intent"
	"CitadelDesktop/Server/Protocol"
	"CitadelDesktop/Server/State"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"sync"
	"testing"
	"time"
)

func marketBarrowTestInput(t *testing.T) Intent.PlanningContext {
	t.Helper()
	data, err := GameData.DecodeStore([]byte(`{"versionInfo":[],"units":[],"constructionItems":[],"effects":[],"levelBoosters":[],"horses":[],"buildings":[{"wodID":137,"name":"Market","marketCarriages":5}],"resources":[{"resourceID":12,"JSONKey":"F"},{"resourceID":18,"JSONKey":"MEAD"}]}`), GameData.SourceMetadata{ItemVersion: "synthetic-cit127"})
	if err != nil {
		t.Fatal(err)
	}
	gs := State.NewGameState()
	gs.Session.ChangedAt = time.Now().UTC().Add(-time.Hour)
	gs.Player.ID = 1
	gs.Market.CaravanLevelLoaded = true
	for _, id := range []State.CastleID{10, 20, 30} {
		castle := resourceIntentCastle(id, 0, int(id), int(id))
		castle.Buildings[1] = State.Building{InstanceID: 1, DefinitionID: 137}
		castle.Resources[12] = State.ResourceBalance{Amount: 50000}
		castle.Resources[18] = State.ResourceBalance{Amount: 50000}
		gs.Castles[id] = castle
	}
	return Intent.PlanningContext{State: gs, GameData: data}
}

func TestMarketBarrowStaleSourceReplay(t *testing.T) {
	// R6 shape: two independent CRM goods shipments, 3905 food and 156 mead.
	for _, good := range []struct {
		resource int
		amount   int64
	}{{12, 3905}, {18, 156}} {
		t.Run(fmt.Sprint(good.resource), func(t *testing.T) {
			input := marketBarrowTestInput(t)
			now := time.Now().UTC()
			r := now.Add(time.Minute)
			input.State.Market.Castles[10] = State.MarketCastleState{CastleID: 10, TotalBarrows: 125, AvailableBarrows: 125, ObservedAt: now.Add(-3 * time.Minute)}
			input.State.Market.BarrowLeases = map[State.MovementID]State.MarketBarrowLeaseRecord{50: {HomeCastleID: 30, Barrows: 125, ReleasesAt: r}}
			args := json.RawMessage(fmt.Sprintf(`{"sourceCastleId":10,"targetCastleId":20,"resourceId":%d,"amount":%d}`, good.resource, good.amount))
			plan, err := planMarketResourceShipment(t.Context(), input, args)
			if err != nil {
				t.Fatal(err)
			}
			cmi, crm := -1, -1
			for i, step := range plan.Steps {
				if step.Opcode == "cmi" {
					cmi = i
				}
				if step.Opcode == "crm" {
					crm = i
				}
			}
			if cmi < 0 || crm <= cmi || !slices.Contains(plan.Claims, "castle-focus") {
				t.Fatalf("refresh not before shipment: %+v", plan)
			}
			payload := string(plan.Steps[crm].Command.Payload)
			row := input.State.Market.Castles[10]
			row.ObservedAt = time.Now().UTC()
			row.AvailableBarrows = 0
			input.State.Market.Castles[10] = row
			if _, err := planMarketResourceShipment(t.Context(), input, args); err == nil {
				t.Fatal("fresh shortage accepted")
			}
			required, err := marketBarrowsRequired(input, 10, good.amount)
			if err != nil || required <= 0 {
				t.Fatalf("required=%d err=%v", required, err)
			}
			row.AvailableBarrows = required
			input.State.Market.Castles[10] = row
			fresh, err := planMarketResourceShipment(t.Context(), input, args)
			if err != nil {
				t.Fatal(err)
			}
			if len(fresh.Steps) != 1 || fresh.Steps[0].Opcode != "crm" || string(fresh.Steps[0].Command.Payload) != payload {
				t.Fatalf("fresh shipment changed: %+v", fresh)
			}
		})
	}
}

func TestMarketBarrowRequiredRoundsUpAndFailsClosed(t *testing.T) {
	input := marketBarrowTestInput(t)
	input.State.Market.Castles[10] = State.MarketCastleState{CastleID: 10, ObservedAt: time.Now().UTC()}
	capacity, err := input.GameData.MarketCapacity(0, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		amount int64
		want   int
	}{{1, 1}, {int64(capacity.CapacityPerBarrow), 1}, {int64(capacity.CapacityPerBarrow) + 1, 2}} {
		got, err := marketBarrowsRequired(input, 10, tc.amount)
		if err != nil || got != tc.want {
			t.Fatalf("amount=%d got=%d err=%v", tc.amount, got, err)
		}
	}
	input.State.Market.CaravanLevelLoaded = false
	if _, err := marketBarrowsRequired(input, 10, 1); err == nil {
		t.Fatal("unobserved capacity accepted")
	}
}

func TestMarketBarrowRecordsDoNotScheduleMovementClock(t *testing.T) {
	input := marketBarrowTestInput(t)
	input.State.Market.BarrowLeases = map[State.MovementID]State.MarketBarrowLeaseRecord{50: {HomeCastleID: 10, Barrows: 125, ReleasesAt: time.Now().Add(-time.Minute)}}
	if got := nextMovementCompletion(input.State); !got.IsZero() {
		t.Fatalf("record scheduled past completion: %s", got)
	}
}

func TestMarketBarrowDispatchEvidenceCountsAndPrivacy(t *testing.T) {
	input := marketBarrowTestInput(t)
	now := time.Now().UTC()
	r := now.Add(-time.Second)
	input.State.Market.Castles[10] = State.MarketCastleState{CastleID: 10, ObservedAt: now.Add(-time.Minute), TotalBarrows: 125, AvailableBarrows: 125}
	input.State.Market.ObservedAt = now.Add(-time.Minute)
	input.State.Market.BarrowLeases = map[State.MovementID]State.MarketBarrowLeaseRecord{50: {HomeCastleID: 10, Barrows: 125, ReleasesAt: r}}
	step := commandStep("Synthetic market shipment", "crm", json.RawMessage(`{"SID":10,"TX":87654323,"TY":87654324,"G":[["F",3905]],"PTT":0}`), "crm")
	raw, err := json.Marshal(captureDispatchBoundaryEvidence(t.Context(), input, step, "", nil))
	if err != nil {
		t.Fatal(err)
	}
	var value struct {
		Market struct {
			SourceOrdinal, RequiredBarrows, ComputedAvailable int
			Lease                                             State.MarketBarrowLease
			Ready                                             bool
		}
	}
	if err := json.Unmarshal(raw, &value); err != nil {
		t.Fatal(err)
	}
	required, _ := marketBarrowsRequired(input, 10, 3905)
	if value.Market.SourceOrdinal != 1 || value.Market.RequiredBarrows != required || value.Market.ComputedAvailable != 0 || value.Market.Lease.AwaitingConfirmation != 125 || !value.Market.Ready {
		t.Fatalf("evidence=%s", raw)
	}
}

func TestMarketBarrowCRM109StillLocksLane(t *testing.T) {
	step := boundaryTestStep("crm")
	engine, _, _ := boundaryTestEngine(t, boundaryTestState(), step, "", 109)
	receipt := engine.Submit(t.Context(), Intent.Request{Name: "test.dispatch", Actor: "automation:autoFoodBalance", AutomationLane: "economy"})
	if len(receipt.Evidence) != 1 || !engine.AutomationLaneLock("economy").Active(time.Now()) {
		t.Fatalf("receipt=%+v", receipt)
	}
}

func marketBarrowGateInput(t *testing.T) (Intent.PlanningContext, Intent.Step) {
	input := marketBarrowTestInput(t)
	now := time.Now().UTC()
	for _, source := range []State.CastleID{10, 30} {
		input.State.Market.Castles[source] = State.MarketCastleState{CastleID: source, TotalBarrows: 1, AvailableBarrows: 1, ObservedAt: now}
	}
	step := commandStep("Synthetic market shipment", "crm", json.RawMessage(`{"SID":10,"TX":20,"TY":20,"G":[["F",1]],"PTT":0}`), "crm")
	step.Payload = append(json.RawMessage(nil), step.Command.Payload...)
	return input, step
}

func TestMarketBarrowDispatchGateReservations(t *testing.T) {
	input, step := marketBarrowGateInput(t)
	original := string(step.Payload)
	gate := newMarketBarrowDispatchGate()
	first, second := coinGateContext("market-first"), coinGateContext("market-second")
	if err := gate.Validate(first, input, step); err != nil {
		t.Fatal(err)
	}
	if err := gate.Validate(first, input, step); err != nil || len(gate.pending) != 1 {
		t.Fatalf("idempotent: %v", err)
	}
	var shortage *Intent.BalanceUnavailableError
	if err := gate.Validate(second, input, step); !errors.As(err, &shortage) || !shortage.Known || shortage.Pending != 1 || shortage.Key != Intent.MarketBarrowBalanceKey(10) {
		t.Fatalf("double booked: %v", err)
	}
	other := step
	other.Payload = json.RawMessage(`{"SID":30,"G":[["F",1]],"PTT":0}`)
	if err := gate.Validate(coinGateContext("market-other-source"), input, other); err != nil {
		t.Fatalf("other castle blocked: %v", err)
	}
	gate.DefinitiveFailure(first, step)
	if err := gate.Validate(second, input, step); err != nil {
		t.Fatalf("failure retained: %v", err)
	}
	if string(step.Payload) != original {
		t.Fatal("gate changed payload bytes")
	}
}

func TestMarketBarrowDispatchGateSerializesConcurrentShipments(t *testing.T) {
	input, step := marketBarrowGateInput(t)
	gate := newMarketBarrowDispatchGate()
	results := make(chan error, 2)
	var wg sync.WaitGroup
	for _, token := range []string{"concurrent-one", "concurrent-two"} {
		wg.Add(1)
		go func() { defer wg.Done(); results <- gate.Validate(coinGateContext(token), input, step) }()
	}
	wg.Wait()
	close(results)
	passed, blocked := 0, 0
	for err := range results {
		if err == nil {
			passed++
		} else if errors.Is(err, Intent.ErrBalanceUnavailable) {
			blocked++
		} else {
			t.Fatal(err)
		}
	}
	if passed != 1 || blocked != 1 {
		t.Fatalf("pass=%d blocked=%d", passed, blocked)
	}
}

func TestMarketBarrowDispatchGateUncertainAndUncorrelatedCompletion(t *testing.T) {
	for _, outcome := range []string{"indeterminate", "uncorrelated completion"} {
		t.Run(outcome, func(t *testing.T) {
			input, step := marketBarrowGateInput(t)
			gate := newMarketBarrowDispatchGate()
			first := coinGateContext("uncertain-first")
			if err := gate.Validate(first, input, step); err != nil {
				t.Fatal(err)
			}
			if outcome == "indeterminate" {
				gate.Indeterminate(first, step)
			} else if gate.Completed(first, input, step, Protocol.CommittedFrame{}) {
				t.Fatal("requested gbd")
			}
			key := coinDispatchKey(first, step)
			debit := gate.pending[key]
			if debit.outcomeAt.IsZero() {
				t.Fatal("missing outcome clock")
			}
			// A newer CMI on another source cannot release this debit.
			other := input.State.Market.Castles[30]
			other.ObservedAt = debit.outcomeAt.Add(time.Nanosecond)
			input.State.Market.Castles[30] = other
			if err := gate.Validate(coinGateContext("uncertain-second"), input, step); !errors.Is(err, Intent.ErrBalanceUnavailable) {
				t.Fatalf("released before source refresh: %v", err)
			}
			row := input.State.Market.Castles[10]
			row.ObservedAt = debit.outcomeAt
			input.State.Market.Castles[10] = row
			if err := gate.Validate(coinGateContext("uncertain-at-outcome"), input, step); !errors.Is(err, Intent.ErrBalanceUnavailable) {
				t.Fatalf("equal timestamp released: %v", err)
			}
			row.ObservedAt = debit.outcomeAt.Add(time.Nanosecond)
			input.State.Market.Castles[10] = row
			if err := gate.Validate(coinGateContext("uncertain-after-refresh"), input, step); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestMarketBarrowDispatchGateInFlightObservationAndRetention(t *testing.T) {
	input, step := marketBarrowGateInput(t)
	gate := newMarketBarrowDispatchGate()
	first := coinGateContext("in-flight")
	if err := gate.Validate(first, input, step); err != nil {
		t.Fatal(err)
	}
	key := coinDispatchKey(first, step)
	debit := gate.pending[key]
	row := input.State.Market.Castles[10]
	row.ObservedAt = debit.reservedAt.Add(time.Nanosecond)
	input.State.Market.Castles[10] = row
	if err := gate.Validate(coinGateContext("after-in-flight-cmi"), input, step); !errors.Is(err, Intent.ErrBalanceUnavailable) {
		t.Fatalf("in-flight debit released: %v", err)
	}
	gate.reconcile(input.State, debit.reservedAt.Add(State.MarketBarrowLeaseRetention-time.Nanosecond))
	if len(gate.pending) != 1 {
		t.Fatal("retention ended early")
	}
	gate.reconcile(input.State, debit.reservedAt.Add(State.MarketBarrowLeaseRetention))
	if len(gate.pending) != 0 {
		t.Fatal("retention unbounded")
	}
}

func TestMarketBarrowDispatchGateCorrelatedCompletion(t *testing.T) {
	for _, test := range []struct {
		name                               string
		owned, sameSource, sameObservation bool
		reduceError                        string
		released                           bool
	}{
		{"correlated", true, true, true, "", true}, {"foreign movement", false, true, true, "", false},
		{"other source", true, false, true, "", false}, {"old observation", true, true, false, "", false}, {"failed reducer", true, true, true, "synthetic", false},
	} {
		t.Run(test.name, func(t *testing.T) {
			input, step := marketBarrowGateInput(t)
			gate := newMarketBarrowDispatchGate()
			ctx := coinGateContext("completion")
			if err := gate.Validate(ctx, input, step); err != nil {
				t.Fatal(err)
			}
			received := time.Now().UTC()
			observed := received
			source := State.CastleID(10)
			owner := State.PlayerID(1)
			if !test.owned {
				owner = 2
			}
			if !test.sameSource {
				source = 30
			}
			if !test.sameObservation {
				observed = received.Add(-time.Second)
			}
			r := received.Add(time.Minute)
			input.State.Movements[50] = State.MovementState{ID: 50, SourceCastleID: source, OwnerPlayerID: owner, MarketBarrows: 1, ObservedAt: observed, ReturnsAt: &r}
			if gate.Completed(ctx, input, step, Protocol.CommittedFrame{Frame: Protocol.Frame{ReceivedAt: received}, ReduceError: test.reduceError}) {
				t.Fatal("barrow gate requested GBD")
			}
			if (len(gate.pending) == 0) != test.released {
				t.Fatalf("pending=%+v", gate.pending)
			}
		})
	}
}

func TestMarketBarrowDispatchGateFailsClosed(t *testing.T) {
	for _, kind := range []string{"stale", "unobserved", "capacity unavailable", "malformed goods"} {
		t.Run(kind, func(t *testing.T) {
			input, step := marketBarrowGateInput(t)
			row := input.State.Market.Castles[10]
			switch kind {
			case "stale":
				row.ObservedAt = time.Now().Add(-State.MarketBarrowFreshness)
				input.State.Market.Castles[10] = row
			case "unobserved":
				delete(input.State.Market.Castles, 10)
			case "capacity unavailable":
				input.State.Market.CaravanLevelLoaded = false
			case "malformed goods":
				step.Payload = json.RawMessage(`{"SID":10,"G":[["F",-1]]}`)
			}
			var shortage *Intent.BalanceUnavailableError
			gate := newMarketBarrowDispatchGate()
			if err := gate.Validate(coinGateContext("stale"), input, step); !errors.As(err, &shortage) || shortage.Known || shortage.Key.String() != "market_barrows:10" {
				t.Fatalf("error=%v", err)
			}
			if len(gate.pending) != 0 {
				t.Fatal("reserved a blocked send")
			}
		})
	}
}

func TestMarketBarrowWaitBlocksSendWithoutLaneLock(t *testing.T) {
	step := boundaryTestStep("crm")
	engine, _, sender := boundaryTestEngine(t, boundaryTestState(), step, "", 0)
	engine.SetFinalDispatchProvider(newMarketBarrowDispatchGate())
	receipt := engine.Submit(t.Context(), Intent.Request{Name: "test.dispatch", Actor: "automation:autoFoodBalance", AutomationLane: "economy"})
	if receipt.Failure == nil || receipt.Failure.Kind != Intent.FailureAvailability || receipt.Failure.Toast || len(sender.sends) != 0 || engine.AutomationLaneLock("economy").Active(time.Now()) {
		t.Fatalf("receipt=%+v sends=%v", receipt, sender.sends)
	}
}

func TestMarketBarrowCompositePreservesEveryEpicGate(t *testing.T) {
	barrows := newMarketBarrowDispatchGate()
	special := newSpecialCostDispatchGate()
	composite := newFinalDispatchGates(newCoinDispatchGate(), newTravelTicketDispatchGate(), special, barrows)
	if len(composite.gates) != 5 || composite.gates[0] != composite.commanders || composite.gates[2] != composite.tickets || composite.gates[3] != special || composite.gates[4] != barrows {
		t.Fatalf("gates=%+v", composite.gates)
	}
	if _, ok := composite.gates[1].(*coinDispatchGate); !ok {
		t.Fatal("coin gate missing")
	}
}
