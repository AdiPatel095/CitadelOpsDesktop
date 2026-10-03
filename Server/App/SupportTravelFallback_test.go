package App

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"CitadelDesktop/Server/GameData"
	"CitadelDesktop/Server/Intent"
	"CitadelDesktop/Server/State"
)

func coinHorseTravelData(t *testing.T) *GameData.Store {
	t.Helper()
	data, err := GameData.DecodeStore([]byte(`{"versionInfo":[],"resources":[{"resourceID":1,"JSONKey":"C1"}],"currencies":[{"currencyID":22,"JSONKey":"PTT"}],"buildings":[{"wodID":46,"name":"Stable","level":1,"unlockHorses":"1007,1008,1009"}],"horses":[{"wodID":1007,"group":"Travelbooster","costFactorC1":2,"costFactorC2":0},{"wodID":1008,"group":"Travelbooster","costFactorC1":0,"costFactorC2":2},{"wodID":1009,"group":"Travelbooster","costFactorC1":0,"costFactorC2":3}],"units":[{"wodID":1,"type":"Soldier","travelSpeed":20}]}`), GameData.SourceMetadata{ItemVersion: "coin-horse-replay"})
	if err != nil {
		t.Fatal(err)
	}
	return data
}
func coinHorseTravelCastle() State.CastleState {
	return State.CastleState{ID: 10, X: 100, Y: 100, Focused: true, Buildings: map[State.BuildingInstanceID]State.Building{1: {InstanceID: 1, DefinitionID: 46, Placed: true}}}
}
func supportTravelPayload(t *testing.T, step Intent.Step) map[string]json.RawMessage {
	t.Helper()
	payload := step.Command.Payload
	if len(payload) == 0 {
		payload = step.Payload
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(payload, &fields); err != nil {
		t.Fatal(err)
	}
	return fields
}

func TestCoinHorseFallbackFeatureScopeAndFreshness(t *testing.T) {
	for _, feature := range []struct {
		lane, name string
		eligible   bool
	}{
		{"autoBird", "auto_bird.dispatch", true}, {"autoStation", "troops.station", true}, {"", "auto_bird.resend", true},
		{"", "troops.station", false}, {"autoStorm", "storm.island.return", false}, {"other", "support.other", false},
	} {
		for _, rows := range []string{"", `[["PTT",0]]`, `[["PTT",5]]`} {
			t.Run(feature.lane+feature.name+rows, func(t *testing.T) {
				input, _ := ticketReplayInput(t, rows)
				input.GameData = coinHorseTravelData(t)
				input = supportCommanderTestInput(t, input)
				input.State.Player.ResourceObservations[1] = State.PlayerResourceObservation{ObservedAt: time.Now().UTC(), ConnectionGeneration: input.State.Session.ConnectionGeneration}
				input.AutomationLane, input.IntentName = feature.lane, feature.name
				step, err := supportDispatchStep(input, "Support", coinHorseTravelCastle(), State.AllianceHolding{X: 101, Y: 101}, 0, map[State.UnitID]int64{1: 10}, Intent.Step{}, supportCoinHorseEligible(input))
				blocked := rows == "" || rows == `[["PTT",0]]` && !feature.eligible
				if blocked {
					if !errors.Is(err, Intent.ErrCurrencyUnavailable) {
						t.Fatalf("expected gate: %v", err)
					}
					return
				}
				if err != nil {
					t.Fatal(err)
				}
				fields := supportTravelPayload(t, step)
				if rows == `[["PTT",5]]` {
					if string(fields["HBW"]) != "-1" || string(fields["PTT"]) != "1" || step.CoinCost != nil {
						t.Fatal("funded support changed")
					}
				} else {
					if string(fields["HBW"]) != "1007" || string(fields["PTT"]) != "0" || string(fields["BPC"]) != "1" || string(fields["LID"]) != "-14" {
						t.Fatal("wrong horse or changed commander")
					}
					if step.CoinCost == nil || step.CoinCost.Reserve != 0 || step.NameDescriptor.Key != "server.support.coin_horse" {
						t.Fatal("missing reserve/notice")
					}
				}
				if rows == `[["PTT",0]]` {
					input.State.Session.ConnectionGeneration++
					if _, _, _, err := supportTravelChoice(input, coinHorseTravelCastle(), 1, true); !errors.Is(err, Intent.ErrCurrencyUnavailable) {
						t.Fatal("stale fallback allowed")
					}
				}
			})
		}
	}
}

func TestCoinHorseFallbackSwitchesEveryBatchAndUsesSharedCoinGate(t *testing.T) {
	input, _ := ticketReplayInput(t, `[["PTT",2]]`)
	input.GameData = coinHorseTravelData(t)
	input = supportCommanderTestInput(t, input)
	input.State.Player.ResourceObservations[1] = State.PlayerResourceObservation{ObservedAt: time.Now().UTC(), ConnectionGeneration: input.State.Session.ConnectionGeneration}
	source := coinHorseTravelCastle()
	input.State.Castles[10] = source
	amounts := map[State.UnitID]int64{}
	for i := 1; i <= 21; i++ {
		amounts[State.UnitID(i)] = 10
	}
	coins := newCoinDispatchGate()
	gate := newFinalDispatchGates(coins, newTravelTicketDispatchGate())
	input.SupportCommanders = gate
	step, err := supportDispatchStep(input, "Support", source, State.AllianceHolding{X: 101, Y: 101}, 0, amounts, Intent.Step{}, true)
	if err != nil || len(step.Batch) != 3 {
		t.Fatalf("batch=%d err=%v", len(step.Batch), err)
	}
	var total int64
	for _, child := range step.Batch {
		fields := supportTravelPayload(t, child)
		if string(fields["HBW"]) != "1007" || string(fields["PTT"]) != "0" {
			t.Fatal("mixed batch travel")
		}
		child.Opcode = "cds"
		child.Payload = child.Command.Payload
		cost, err := supportCoinCost(input, child.Payload)
		if err != nil {
			t.Fatal(err)
		}
		if err := gate.Validate(coinGateContext(input.OperationID), input, child); err != nil {
			t.Fatal(err)
		}
		total += cost.amount
	}
	var pending int64
	for _, debit := range coins.pending {
		pending += debit.amount
	}
	if pending != total || len(coins.pending) != 3 {
		t.Fatalf("pending=%d wanted=%d", pending, total)
	}
}

func TestCoinHorseFallbackCoinShortageAndUnknownWaitWithoutLock(t *testing.T) {
	for _, balance := range []string{"short", "unknown", "stale", "funded"} {
		t.Run(balance, func(t *testing.T) {
			input, _ := ticketReplayInput(t, `[["PTT",0]]`)
			input.GameData = coinHorseTravelData(t)
			input = supportCommanderTestInput(t, input)
			input.State.Player.ResourceObservations[1] = State.PlayerResourceObservation{ObservedAt: time.Now().UTC(), ConnectionGeneration: input.State.Session.ConnectionGeneration}
			input.State.Castles[10] = coinHorseTravelCastle()
			if balance == "short" {
				input.State.Player.Resources[1] = 1
			}
			if balance == "unknown" {
				delete(input.State.Player.ResourceObservations, 1)
			}
			if balance == "stale" {
				obs := input.State.Player.ResourceObservations[1]
				obs.ConnectionGeneration++
				input.State.Player.ResourceObservations[1] = obs
			}
			store := State.NewStore(&input.State)
			// Store initialization intentionally drops authority; install only the same current observations.
			_, err := store.ApplyComponents(State.Components(State.ComponentPlayer), func(s *State.GameState) ([]string, bool, error) {
				s.Player.CurrencyObservations = input.State.Player.CurrencyObservations
				s.Player.ResourceObservations = input.State.Player.ResourceObservations
				return []string{"currencies", "resources"}, true, nil
			})
			if err != nil {
				t.Fatal(err)
			}
			registry := Intent.NewRegistry()
			err = registry.Register(Intent.Definition{Name: "auto_bird.travel_test", Effect: Intent.EffectWrite, Planner: func(_ context.Context, in Intent.PlanningContext, _ json.RawMessage) (Intent.Plan, error) {
				step, err := supportDispatchStep(in, "Support", in.State.Castles[10], State.AllianceHolding{X: 101, Y: 101}, 0, map[State.UnitID]int64{1: 10}, Intent.Step{}, supportCoinHorseEligible(in))
				return Intent.Plan{Steps: []Intent.Step{step}}, err
			}})
			if err != nil {
				t.Fatal(err)
			}
			observer := newCoinGateEngineObserver()
			sender := &replayTravelSender{store: store, observer: observer, gameData: input.GameData}
			engine := Intent.NewEngine(registry, store, coinGateStoreProvider{input.GameData}, sender, observer)
			engine.SetFinalDispatchProvider(newFinalDispatchGates(newCoinDispatchGate(), newTravelTicketDispatchGate()))
			receipt := engine.Submit(t.Context(), Intent.Request{Name: "auto_bird.travel_test", Actor: "automation:autoBird", AutomationLane: "autoBird"})
			if balance != "funded" {
				if len(sender.payloads) != 0 || receipt.Failure == nil || receipt.Failure.Kind != Intent.FailureAvailability || receipt.Failure.ExplanationDescriptor == nil || receipt.Failure.Explanation == "" || !strings.HasPrefix(receipt.Failure.ExplanationDescriptor.Key, "server.support.coin_horse_") || !store.Snapshot().Automations["autoBird"].SafetyLock.ObservedAt.IsZero() {
					t.Fatalf("sent=%d status=%s failure=%+v error=%s", len(sender.payloads), receipt.Status, receipt.Failure, receipt.DiagnosticError())
				}
			} else {
				if receipt.Status != Intent.StatusSucceeded || len(sender.payloads) != 1 {
					t.Fatalf("status=%s error=%s", receipt.Status, receipt.DiagnosticError())
				}
				activity, ok := coinHorseSupportTravelActivity(receipt)
				if !ok || !strings.Contains(activity.detail, "1 needed, 0 available") {
					t.Fatalf("missing fallback status: %+v", activity)
				}
				receipt.CompletedStepIndexes = nil
				if _, ok := coinHorseSupportTravelActivity(receipt); ok {
					t.Fatal("claimed unsent fallback")
				}
			}
		})
	}
}

func TestCapturedCDS327CoinHorseGolden(t *testing.T) {
	raw, err := os.ReadFile("testdata/travel-ticket-327.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixtures map[string]struct {
		Payload json.RawMessage `json:"payload"`
	}
	if err := json.Unmarshal(raw, &fixtures); err != nil {
		t.Fatal(err)
	}
	var golden map[string]json.RawMessage
	if err := json.Unmarshal(fixtures["cds"].Payload, &golden); err != nil {
		t.Fatal(err)
	}
	input, _ := ticketReplayInput(t, `[["PTT",0]]`)
	input.GameData = coinHorseTravelData(t)
	input = supportCommanderTestInput(t, input)
	input.State.Player.ResourceObservations[1] = State.PlayerResourceObservation{ObservedAt: time.Now().UTC(), ConnectionGeneration: input.State.Session.ConnectionGeneration}
	var troops [][2]int64
	if err := json.Unmarshal(golden["A"], &troops); err != nil {
		t.Fatal(err)
	}
	amounts := map[State.UnitID]int64{}
	for _, troop := range troops {
		amounts[State.UnitID(troop[0])] = troop[1]
	}
	var target State.AllianceHolding
	json.Unmarshal(golden["TX"], &target.X)
	json.Unmarshal(golden["TY"], &target.Y)
	var wait int
	json.Unmarshal(golden["WT"], &wait)
	input.State.Castles[10] = coinHorseTravelCastle()
	store := State.NewStore(&input.State)
	_, err = store.ApplyComponents(State.Components(State.ComponentPlayer), func(state *State.GameState) ([]string, bool, error) {
		state.Player.CurrencyObservations = input.State.Player.CurrencyObservations
		state.Player.ResourceObservations = input.State.Player.ResourceObservations
		return []string{"currencies", "resources"}, true, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	registry := Intent.NewRegistry()
	if err := registry.Register(Intent.Definition{Name: "auto_bird.captured", Effect: Intent.EffectWrite, Planner: func(_ context.Context, in Intent.PlanningContext, _ json.RawMessage) (Intent.Plan, error) {
		step, err := supportDispatchStep(in, "Support", in.State.Castles[10], target, wait, amounts, Intent.Step{}, supportCoinHorseEligible(in))
		return Intent.Plan{Steps: []Intent.Step{step}}, err
	}}); err != nil {
		t.Fatal(err)
	}
	observer := newCoinGateEngineObserver()
	sender := &replayTravelSender{store: store, observer: observer, gameData: input.GameData}
	engine := Intent.NewEngine(registry, store, coinGateStoreProvider{input.GameData}, sender, observer)
	engine.SetFinalDispatchProvider(newFinalDispatchGates(newCoinDispatchGate(), newTravelTicketDispatchGate()))
	receipt := engine.Submit(t.Context(), Intent.Request{Name: "auto_bird.captured", AutomationLane: "autoBird", Actor: "automation:autoBird"})
	if receipt.Status != Intent.StatusSucceeded || len(sender.payloads) != 1 {
		t.Fatalf("captured fallback status=%s error=%s sends=%d", receipt.Status, receipt.DiagnosticError(), len(sender.payloads))
	}
	step := Intent.Step{Payload: sender.payloads[0]}
	if _, ok := coinHorseSupportTravelActivity(receipt); !ok {
		t.Fatal("captured fallback missing notice")
	}

	golden["HBW"], golden["PTT"] = json.RawMessage(`1007`), json.RawMessage(`0`)
	// The fixture sanitizes commander identities to 3. Keep the builder's existing
	// premium commander sentinel; commander selection belongs to CIT-133.
	golden["LID"] = json.RawMessage(`-14`)
	for key, raw := range golden {
		var value any
		if err := json.Unmarshal(raw, &value); err != nil {
			t.Fatal(err)
		}
		golden[key], _ = json.Marshal(value)
	}
	if got := supportTravelPayload(t, step); !reflect.DeepEqual(golden, got) {
		t.Fatalf("CDS constructor drift: wanted=%s got=%s", golden, got)
	}
}

func TestTravelGateNeverRewritesPayload(t *testing.T) {
	input, _ := ticketReplayInput(t, `[["PTT",0]]`)
	original := json.RawMessage(`{"SID":10,"HBW":-1,"PTT":1,"BPC":1,"LID":-14}`)
	step := Intent.Step{Opcode: "cds", Payload: append(json.RawMessage(nil), original...)}
	if err := newTravelTicketDispatchGate().Validate(coinGateContext("unchanged"), input, step); !errors.Is(err, Intent.ErrCurrencyUnavailable) {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(original, step.Payload) {
		t.Fatal("gate rewrote payload")
	}
}

func TestStationResolverUsesRuntimeFeatureForCoinHorseFallback(t *testing.T) {
	for _, lane := range []string{"autoStation", "autoBird", ""} {
		t.Run(lane, func(t *testing.T) {
			input, _ := ticketReplayInput(t, `[["PTT",0]]`)
			input.GameData = coinHorseTravelData(t)
			input = supportCommanderTestInput(t, input)
			input.State.Player.ResourceObservations[1] = State.PlayerResourceObservation{ObservedAt: time.Now().UTC(), ConnectionGeneration: input.State.Session.ConnectionGeneration}
			source := coinHorseTravelCastle()
			source.Units = State.CastleUnits{Stationed: map[State.UnitID]int64{1: 10}}
			input.State.Castles[10] = source
			input.State.Alliance.Holdings = []State.AllianceHolding{{CastleID: 20, KingdomID: 0, X: 101, Y: 101, SlotType: 1}}
			seedStationAuthority(&input.State, time.Now())
			input.AutomationLane = lane
			step, err := resolveTroopsStationStep(t.Context(), input, stationFreshTestArguments(json.RawMessage(`{"sourceCastleId":10,"targetCastleId":20,"purpose":"autoStation","units":[{"unitId":1,"amount":10}]}`)))
			if lane == "" {
				if !errors.Is(err, Intent.ErrCurrencyUnavailable) {
					t.Fatalf("manual purpose bypassed gate: %v", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if string(supportTravelPayload(t, step)["HBW"]) != "1007" {
				t.Fatal("station did not use coin horse")
			}
		})
	}
}

func TestAutoBirdDispatchResolverUsesCoinHorseFallback(t *testing.T) {
	for _, identity := range []struct{ lane, name string }{{"autoBird", "auto_bird.dispatch"}, {"", "auto_bird.resend"}} {
		now := time.Now().UTC()
		state, _ := autoBirdIntentTestState(t, now)
		state.Player.Currencies[22] = 0
		source := coinHorseTravelCastle()
		source.UnitsObservedAt = now
		source.Units = State.CastleUnits{Stationed: map[State.UnitID]int64{1: 10}}
		state.Castles[10] = source
		state.Stationing["autoBird:10"] = State.StationingOperation{ID: "autoBird:10", Purpose: "autoBird", Phase: State.StationingPhaseDispatchReady, SourceCastleID: 10, TargetCastleID: 20, DelayHours: 8, UnitsObservedAt: now, UpdatedAt: now}
		args, _ := json.Marshal(autoBirdCycleRequest{SourceCastleID: 10, TrackingID: "autoBird:10", MinimumDelayHours: 6, MaximumDelayHours: 12, MinimumSend: 1, DispatchStartedAt: now.Add(-time.Second), ExpectedTargetCastle: 20})
		app := &Application{State: travelTicketTestStore(&state)}
		step, err := app.resolveAutoBirdDispatchStep(t.Context(), supportCommanderTestInput(t, Intent.PlanningContext{State: state, GameData: coinHorseTravelData(t), AutomationLane: identity.lane, IntentName: identity.name}), args)
		if err != nil {
			t.Fatal(err)
		}
		if string(supportTravelPayload(t, step)["HBW"]) != "1007" {
			t.Fatal("bird did not use coin horse")
		}
	}
}
