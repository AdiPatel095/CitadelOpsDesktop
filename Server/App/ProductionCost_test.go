package App

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"CitadelDesktop/Server/Automation"
	"CitadelDesktop/Server/Configuration"
	"CitadelDesktop/Server/GameData"
	"CitadelDesktop/Server/Ingest"
	"CitadelDesktop/Server/Intent"
	"CitadelDesktop/Server/Localization"
	"CitadelDesktop/Server/Protocol"
	"CitadelDesktop/Server/State"
)

func specialCostInput(t *testing.T, extra string, rows string) Intent.PlanningContext {
	t.Helper()
	unit, err := os.ReadFile("testdata/cit118-unit-513.json")
	if err != nil {
		t.Fatal(err)
	}
	currency, err := os.ReadFile("testdata/cit118-currency-dga.json")
	if err != nil {
		t.Fatal(err)
	}
	units := string(unit)
	if extra != "" {
		units += "," + extra
	}
	raw := fmt.Sprintf(`{"versionInfo":[],"buildings":[],"constructionItems":[],"units":[%s],"resources":[{"resourceID":1,"JSONKey":"C1","name":"coins"},{"resourceID":2,"JSONKey":"C2","name":"rubies"},{"resourceID":3,"JSONKey":"W","name":"wood"},{"resourceID":4,"JSONKey":"S","name":"stone"}],"currencies":[%s]}`, units, currency)
	data, err := GameData.DecodeStore([]byte(raw), GameData.SourceMetadata{ItemVersion: "CIT-118-replay-786.03"})
	if err != nil {
		t.Fatal(err)
	}
	language, err := GameData.DecodeLanguage([]byte(`{"GlassWingArcher_name":"Glass-wing archer","currency_name_DragonGlassArrows":"Dragon-glass arrows","currency_name_wood":"Wood","currency_name_stone":"Stone","SampleUnit_name":"Sample unit","currency_name_coins":"Coins"}`), GameData.LanguageMetadata{Language: "en"})
	if err != nil {
		t.Fatal(err)
	}
	state := State.NewGameState()
	now := time.Now().UTC()
	state.Session.ConnectionGeneration = 7
	state.Session.ChangedAt = now.Add(-time.Minute)
	state.Player.Resources[1] = 1000000
	state.Player.Resources[2] = 1000000
	state.Player.ResourceObservations[1] = State.PlayerResourceObservation{ObservedAt: now, ConnectionGeneration: 7}
	state.Player.ResourceObservations[2] = State.PlayerResourceObservation{ObservedAt: now, ConnectionGeneration: 7}
	state.Castles[77118] = State.CastleState{ID: 77118, Name: "Sample keep", Focused: true, ContextSnapshotObservedAt: now, Resources: map[State.ResourceID]State.ResourceBalance{3: {Amount: 300}, 4: {Amount: 120}}, Production: map[int]State.ProductionQueue{0: {LineID: 0, Capacity: 5, Slots: permanentProductionSlots(5, 0), ObservedAt: now}, 1: {LineID: 1, Capacity: 5, Slots: permanentProductionSlots(5, 0), ObservedAt: now}}}
	store := State.NewStore(&state)
	registry := Ingest.NewRegistry()
	if err := Ingest.RegisterCoreReducers(registry); err != nil {
		t.Fatal(err)
	}
	pipeline := Ingest.NewPipeline(store, coinGateStoreProvider{data}, registry)
	if rows != "" {
		code := 0
		if _, err := pipeline.HandleFrame(t.Context(), Protocol.Frame{Direction: Protocol.DirectionInbound, Opcode: "sce", ResponseCode: &code, Payload: json.RawMessage(rows), ReceivedAt: time.Now().UTC()}); err != nil {
			t.Fatal(err)
		}
	}
	code := 0
	if _, err := pipeline.HandleFrame(t.Context(), Protocol.Frame{Direction: Protocol.DirectionInbound, Opcode: "gcu", ResponseCode: &code, Payload: json.RawMessage(`{"C1":1000000,"C2":1000000}`), ReceivedAt: time.Now().UTC()}); err != nil {
		t.Fatal(err)
	}

	return Intent.PlanningContext{State: store.Snapshot(), GameData: data, Language: language}
}
func costStep(unit, amount int64) Intent.Step {
	return Intent.Step{Opcode: "bup", Payload: json.RawMessage(fmt.Sprintf(`{"LID":0,"WID":%d,"AMT":%d,"PO":-1,"PWR":0,"SK":73,"SID":0,"AID":77118}`, unit, amount))}
}
func productionBUPAmounts(plan Intent.Plan) []int64 {
	var amounts []int64
	for _, step := range plan.Steps {
		if step.Opcode == "bup" {
			var fields struct{ AMT int64 }
			_ = json.Unmarshal(step.Command.Payload, &fields)
			amounts = append(amounts, fields.AMT)
		}
	}
	return amounts
}
func TestProductionCostCatalog513RawReplay(t *testing.T) {
	for _, rows := range []string{`[["DGA",300]]`, `[["DGA",0]]`} {
		input := specialCostInput(t, "", rows)
		config := Configuration.Snapshot{Sections: map[string]json.RawMessage{"automation.recruitTroops": json.RawMessage(`{"mode":"global","globalItems":[{"id":513,"amount":280}],"castles":{"77118":{"enabled":true}}}`)}}
		policy := Automation.NewRecruitPolicy(resolveProductionCosts)
		decision, err := policy.Evaluate(t.Context(), Automation.Snapshot{State: input.State, GameData: input.GameData, Language: input.Language, Configuration: config, Now: time.Now().UTC()})
		if err != nil {
			t.Fatal(err)
		}
		if rows == `[["DGA",0]]` {
			if decision.Request != nil || decision.Status != "waiting" || decision.DetailDescriptor == nil || decision.DetailDescriptor.Key != "server.production.cost_short" || !strings.Contains(decision.Detail, "2 needed, 0 available") {
				t.Fatalf("zero balance %+v", decision)
			}
			continue
		}
		if decision.Request == nil {
			t.Fatalf("no request: %+v", decision)
		}
		var args struct{ Amount int64 }
		_ = json.Unmarshal(decision.Request.Arguments, &args)
		if args.Amount != 150 {
			t.Fatalf("280 -> %d", args.Amount)
		}
		plan, err := planProductionEnqueue(t.Context(), input, decision.Request.Arguments)
		if err != nil || !reflect.DeepEqual(productionBUPAmounts(plan), []int64{150}) {
			t.Fatalf("replay amounts %v %v", productionBUPAmounts(plan), err)
		}
		for _, step := range plan.Steps {
			if step.Opcode == "bup" {
				var fields map[string]json.RawMessage
				json.Unmarshal(step.Command.Payload, &fields)
				if len(fields) != 8 {
					t.Fatalf("wire field count %d", len(fields))
				}
				for _, key := range []string{"LID", "WID", "AMT", "PO", "PWR", "SK", "SID", "AID"} {
					if fields[key] == nil {
						t.Fatalf("missing wire key %s", key)
					}
				}
				step.Payload = step.Command.Payload
				if err := newSpecialCostDispatchGate().Validate(coinGateContext("replay"), input, step); err != nil {
					t.Fatal(err)
				}
			}
		}
	}
}
func TestProductionCostFillAvailableAllocatesEachStack(t *testing.T) {
	input := specialCostInput(t, "", `[["DGA",700]]`)
	plan, err := planProductionEnqueue(t.Context(), input, json.RawMessage(`{"castleId":77118,"lineId":0,"definitionId":513,"amount":280,"fillAvailable":true}`))
	if err != nil || !reflect.DeepEqual(productionBUPAmounts(plan), []int64{280, 70}) {
		t.Fatalf("amounts=%v err=%v", productionBUPAmounts(plan), err)
	}
	if !strings.Contains(plan.Summary, "350") {
		t.Fatalf("partial fill summary %q", plan.Summary)
	}
	for _, step := range plan.Steps {
		if step.Action == "production.enqueue.verify_capacity" {
			var guard productionQueueCapacityGuard
			json.Unmarshal(step.ActionArguments, &guard)
			if guard.ExpectedFreeSlots != 5 && guard.ExpectedFreeSlots != 4 {
				t.Fatalf("partial fill queue guard %+v", guard)
			}
		}
	}
}
func TestProductionCostResolutionFailClosedAndOtherUnitsContinue(t *testing.T) {
	cases := []struct {
		unit string
		want string
	}{
		{`{"wodID":88118,"type":"SampleUnit","costWood":"2","costStone":"3","healingCostC1":"999","skipCostC2":"999"}`, ""},
		{`{"wodID":88118,"type":"SampleUnit","costC2":"1"}`, "server.production.needs_rubies"},
		{`{"wodID":88118,"type":"SampleUnit","costMystery":"1"}`, "server.production.cost_unknown"},
		{`{"wodID":88118,"type":"SampleUnit","costDragonGlassArrows":null}`, "server.production.cost_unknown"},
		{`{"wodID":88118,"type":"SampleUnit","costDragonGlassArrows":"bad"}`, "server.production.cost_unknown"},
		{`{"wodID":88118,"type":"SampleUnit","costDragonGlassArrows":"NaN"}`, "server.production.cost_unknown"},
		{`{"wodID":88118,"type":"SampleUnit","costDragonGlassArrows":-1}`, "server.production.cost_unknown"},
	}
	for _, c := range cases {
		input := specialCostInput(t, c.unit, `[["DGA",300]]`)
		costs, err := resolveProductionCosts(input.GameData, input.Language, 0, 88118, 77118)
		if c.want == "" {
			amounts, block := Automation.ProductionStackAmounts(input.State, costs, 100, 1)
			if err != nil || block != nil || !reflect.DeepEqual(amounts, []int64{40}) {
				t.Fatalf("wood/stone amounts %v %v %v", amounts, block, err)
			}
			continue
		}
		localized := productionCostError(input, 88118, err)
		if err == nil || Localization.FromError(localized).Key != c.want {
			t.Fatalf("terminal err %v", localized)
		}
		gateErr := newSpecialCostDispatchGate().Validate(coinGateContext("terminal"), input, costStep(88118, 1))
		if errors.Is(gateErr, Intent.ErrBalanceUnavailable) || gateErr == nil {
			t.Fatalf("terminal gate error %v", gateErr)
		}
	}
	input := specialCostInput(t, `{"wodID":88118,"type":"SampleUnit","costC1":"1"}`, "")
	config := Configuration.Snapshot{Sections: map[string]json.RawMessage{"automation.recruitTroops": json.RawMessage(`{"mode":"perCastle","castles":{"77118":{"enabled":true,"items":[{"id":513,"amount":280},{"id":88118,"amount":10}]}}}`)}}
	decision, err := Automation.NewRecruitPolicy(resolveProductionCosts).Evaluate(t.Context(), Automation.Snapshot{State: input.State, GameData: input.GameData, Language: input.Language, Configuration: config, Now: time.Now().UTC()})
	if err != nil || decision.Request == nil {
		t.Fatalf("unobserved currency blocked other unit: %+v %v", decision, err)
	}
	var body struct {
		DefinitionID int64 `json:"definitionId"`
	}
	json.Unmarshal(decision.Request.Arguments, &body)
	if body.DefinitionID != 88118 {
		t.Fatalf("wrong fallback %d", body.DefinitionID)
	}
	config.Sections["automation.recruitTroops"] = json.RawMessage(`{"mode":"global","globalItems":[{"id":513,"amount":280}],"castles":{"77118":{"enabled":true}}}`)
	decision, err = Automation.NewRecruitPolicy(resolveProductionCosts).Evaluate(t.Context(), Automation.Snapshot{State: input.State, GameData: input.GameData, Language: input.Language, Configuration: config, Now: time.Now().UTC()})
	if err != nil || decision.Request != nil || decision.DetailDescriptor.Key != "server.production.cost_unavailable" {
		t.Fatalf("unobserved %+v %v", decision, err)
	}
}
func TestSpecialCostGateAllOrNothingReservationsAndConcurrentPlans(t *testing.T) {
	input := specialCostInput(t, `{"wodID":88118,"type":"SampleUnit","costDragonGlassArrows":"2","costWood":"4"}`, `[["DGA",0]]`)
	castle := input.State.Castles[77118]
	castle.Resources[3] = State.ResourceBalance{Amount: 300}
	input.State.Castles[77118] = castle
	gate := newSpecialCostDispatchGate()
	step := costStep(88118, 1)
	if err := gate.Validate(coinGateContext("all"), input, step); !errors.Is(err, Intent.ErrBalanceUnavailable) {
		t.Fatal(err)
	}
	for _, debits := range gate.pending {
		if len(debits) != 0 {
			t.Fatal("partial reservation")
		}
	}
	input.State.Player.Currencies[69] = 10
	observation := input.State.Player.CurrencyObservations[69]
	observation.ObservedAt = observation.ObservedAt.Add(time.Second)
	input.State.Player.CurrencyObservations[69] = observation
	if err := gate.Validate(coinGateContext("all"), input, step); err != nil {
		t.Fatal(err)
	}
	if len(gate.pending[Intent.CurrencyBalanceKey(69)]) != 1 || len(gate.pending[Intent.CastleResourceBalanceKey(77118, 3)]) != 1 {
		t.Fatal("balance keys", gate.pending)
	}
	gate.DefinitiveFailure(coinGateContext("all"), step)
	input = specialCostInput(t, "", `[["DGA",300]]`)
	gate = newSpecialCostDispatchGate()
	step = costStep(513, 150)
	var group sync.WaitGroup
	results := make(chan error, 2)
	for i := 0; i < 2; i++ {
		group.Add(1)
		go func(i int) { defer group.Done(); results <- gate.Validate(coinGateContext(fmt.Sprint(i)), input, step) }(i)
	}
	group.Wait()
	close(results)
	allowed := 0
	for err := range results {
		if err == nil {
			allowed++
		} else if !errors.Is(err, Intent.ErrBalanceUnavailable) {
			t.Fatal(err)
		}
	}
	if allowed != 1 {
		t.Fatalf("concurrent sends allowed=%d", allowed)
	}
	// Release whichever operation reserved, then another full stack must pass.
	gate.DefinitiveFailure(coinGateContext("0"), step)
	gate.DefinitiveFailure(coinGateContext("1"), step)
	if err := gate.Validate(coinGateContext("next"), input, step); err != nil {
		t.Fatal("failed release", err)
	}
	if err := gate.Validate(coinGateContext("next"), input, step); err != nil {
		t.Fatal("idempotency", err)
	}
}
func TestSpecialCostGateCorrelationsAndIndeterminateHold(t *testing.T) {
	for _, castleCost := range []bool{false, true} {
		t.Run(fmt.Sprint(castleCost), func(t *testing.T) {
			input := specialCostInput(t, `{"wodID":88118,"costWood":"2"}`, `[["DGA",300]]`)
			unit := int64(513)
			key := Intent.CurrencyBalanceKey(69)
			if castleCost {
				unit = 88118
				key = Intent.CastleResourceBalanceKey(77118, 3)
			}
			step := costStep(unit, 150)
			ctx := coinGateContext("first")
			gate := newSpecialCostDispatchGate()
			if err := gate.Validate(ctx, input, step); err != nil {
				t.Fatal(err)
			}
			gate.Indeterminate(ctx, step)
			if err := gate.Validate(coinGateContext("second"), input, step); !errors.Is(err, Intent.ErrBalanceUnavailable) {
				t.Fatal("uncertain spend released", err)
			}
			now := time.Now().UTC().Add(time.Second)
			if castleCost {
				castle := input.State.Castles[77118]
				castle.ContextSnapshotObservedAt = now
				input.State.Castles[77118] = castle
			} else {
				input.State.Player.CurrencyObservations[69] = State.PlayerResourceObservation{ObservedAt: now, ConnectionGeneration: 7}
			}
			if err := gate.Validate(coinGateContext("second"), input, step); err != nil {
				t.Fatal("later observation did not release", err)
			}
			if len(gate.pending[key]) != 1 {
				t.Fatal("reservation count")
			}
			gate = newSpecialCostDispatchGate()
			if err := gate.Validate(ctx, input, step); err != nil {
				t.Fatal(err)
			}
			response := Protocol.CommittedFrame{Frame: Protocol.Frame{Opcode: "bup", ReceivedAt: now, Payload: json.RawMessage(`{"grc":{"AID":77118,"W":0},"sce":[["DGA",0]],"gcu":{"C1":100}}`)}}
			if castleCost {
				castle := input.State.Castles[77118]
				castle.Resources[3] = State.ResourceBalance{Amount: 0}
				input.State.Castles[77118] = castle
			} else {
				input.State.Player.Currencies[69] = 0
			}
			if refresh := gate.Completed(ctx, input, step, response); refresh || len(gate.pending[key]) != 0 {
				t.Fatalf("correlated completion refresh=%v pending=%v", refresh, gate.pending)
			}
			// An unrelated completion requests GBD only for a currency.
			input = specialCostInput(t, `{"wodID":88118,"costWood":"2"}`, `[["DGA",300]]`)
			gate = newSpecialCostDispatchGate()
			gate.Validate(ctx, input, step)
			response.Frame.Payload = json.RawMessage(`{}`)
			response.Frame.ReceivedAt = time.Now().UTC().Add(time.Second)
			if refresh := gate.Completed(ctx, input, step, response); refresh == castleCost || len(gate.pending[key]) != 1 {
				t.Fatal("uncorrelated outcome", refresh, gate.pending)
			}
			if castleCost {
				// GRC and DCL can change amounts without advancing the JAA snapshot stamp.
				castle := input.State.Castles[77118]
				castle.Resources[3] = State.ResourceBalance{Amount: 600}
				input.State.Castles[77118] = castle
				if err := gate.Validate(coinGateContext("same-stamp"), input, step); err != nil {
					t.Fatal("same-stamp GRC denied", err)
				}
			}
		})
	}
}
func TestSpecialCostGateCompositeUnwindsCoinReservation(t *testing.T) {
	input := specialCostInput(t, `{"wodID":88118,"costC1":"3","costDragonGlassArrows":"2"}`, `[["DGA",0]]`)
	coins := newCoinDispatchGate()
	special := newSpecialCostDispatchGate()
	gates := newFinalDispatchGates(coins, newTravelTicketDispatchGate(), special)
	err := gates.Validate(coinGateContext("mixed"), input, costStep(88118, 1))
	if !errors.Is(err, Intent.ErrBalanceUnavailable) || len(coins.pending) != 0 {
		t.Fatalf("failed gate retained coins %v %v", err, coins.pending)
	}
}
func TestCIT119FeatherWaitUnchanged(t *testing.T) {
	input := coinGateInput(coinGateGameData(t), 1000000)
	input.State.Player.Currencies[22] = 0
	input.State.Player.CurrencyObservations[22] = State.PlayerResourceObservation{ObservedAt: time.Now().UTC(), ConnectionGeneration: 1}
	gate := newFinalDispatchGates(newCoinDispatchGate(), newTravelTicketDispatchGate(), newSpecialCostDispatchGate())
	err := gate.Validate(coinGateContext("feather-regression"), input, Intent.Step{Opcode: "cds", Payload: json.RawMessage(`{"PTT":1,"SID":10,"TX":50,"TY":0,"HBW":-1,"A":[[1,1]]}`)})
	if !errors.Is(err, Intent.ErrCurrencyUnavailable) || errors.Is(err, Intent.ErrBalanceUnavailable) {
		t.Fatalf("ticket sentinel changed: %v", err)
	}
}

func TestSpecialCostGateKeepsUncorrelatedDebitsUntilPostCompletionSnapshot(t *testing.T) {
	input := specialCostInput(t, `{"wodID":88118,"costWood":"2"}`, `[["DGA",300]]`)
	step := costStep(88118, 150)
	ctx := coinGateContext("wood")
	gate := newSpecialCostDispatchGate()
	if err := gate.Validate(ctx, input, step); err != nil {
		t.Fatal(err)
	}
	input.State.Castles[77119] = State.CastleState{ID: 77119}
	frame := Protocol.CommittedFrame{Frame: Protocol.Frame{Opcode: "bup", ReceivedAt: time.Now().UTC(), Payload: json.RawMessage(`{"grc":{"AID":77119,"W":0}}`)}}
	if gate.Completed(ctx, input, step, frame) {
		t.Fatal("castle cost requested GBD")
	}
	if err := gate.Validate(coinGateContext("next"), input, step); !errors.Is(err, Intent.ErrBalanceUnavailable) {
		t.Fatal("wrong-castle response released debit", err)
	}
	castle := input.State.Castles[77118]
	castle.ContextSnapshotObservedAt = time.Now().UTC().Add(time.Second)
	input.State.Castles[77118] = castle
	if err := gate.Validate(coinGateContext("next"), input, step); err != nil {
		t.Fatal("post-completion snapshot did not release debit", err)
	}
	input = specialCostInput(t, "", `[["DGA",300]]`)
	gate = newSpecialCostDispatchGate()
	step = costStep(513, 150)
	gate.Validate(ctx, input, step)
	// A newer currency update received before completion cannot settle a write.
	before := input.State.Player.CurrencyObservations[69]
	stamp := time.Now().UTC().Add(-time.Millisecond)
	before.ObservedAt = stamp
	input.State.Player.CurrencyObservations[69] = before
	gate.Completed(ctx, input, step, Protocol.CommittedFrame{Frame: Protocol.Frame{ReceivedAt: stamp.Add(time.Second), Payload: json.RawMessage(`{}`)}})
	if err := gate.Validate(coinGateContext("next"), input, step); !errors.Is(err, Intent.ErrBalanceUnavailable) {
		t.Fatal("inflight refresh released debit", err)
	}
	before.ObservedAt = time.Now().UTC().Add(time.Second)
	input.State.Player.CurrencyObservations[69] = before
	if err := gate.Validate(coinGateContext("next"), input, step); err != nil {
		t.Fatal("post-completion currency snapshot did not release", err)
	}
}
func TestProductionCostCoinsReservedOnlyOnce(t *testing.T) {
	input := specialCostInput(t, `{"wodID":88118,"costC1":"3","costDragonGlassArrows":"2"}`, `[["DGA",300]]`)
	coins := newCoinDispatchGate()
	special := newSpecialCostDispatchGate()
	gates := newFinalDispatchGates(coins, newTravelTicketDispatchGate(), special)
	step := costStep(88118, 10)
	if err := gates.Validate(coinGateContext("mixed-funded"), input, step); err != nil {
		t.Fatal(err)
	}
	if len(coins.pending) != 1 || len(special.pending) != 1 || len(special.pending[Intent.CurrencyBalanceKey(69)]) != 1 {
		t.Fatal("duplicate/missing reservations", coins.pending, special.pending)
	}
	if _, found := special.pending[Intent.PlayerResourceBalanceKey(1)]; found {
		t.Fatal("C1 reserved by special gate")
	}
}

func TestProductionCostCurrencyNamesAreGenericAndCaseInsensitive(t *testing.T) {
	data, err := GameData.DecodeStore([]byte(`{"versionInfo":[],"buildings":[],"units":[{"wodID":88118,"costComponent8":"1.5","costSceatToken":"3","costLegendaryToken":"4","costDragonScaleArmor":"5","costDragonScaleArrows":"6","costTwinFlameAxes":"7"}],"currencies":[{"currencyID":88001,"Name":"component8"},{"currencyID":88002,"Name":"SceatToken"},{"currencyID":88003,"Name":"LegendaryToken"},{"currencyID":88004,"Name":"DragonScaleArmor"},{"currencyID":88005,"Name":"DragonScaleArrows"},{"currencyID":88006,"Name":"TwinFlameAxes"}]}`), GameData.SourceMetadata{ItemVersion: "synthetic"})
	if err != nil {
		t.Fatal(err)
	}
	for _, line := range []int{0, 1} {
		costs, err := resolveProductionCosts(data, nil, line, 88118, 77118)
		if err != nil || len(costs) != 6 {
			t.Fatalf("generic unit/tool costs %v %v", costs, err)
		}
		state := State.NewGameState()
		state.Session.ConnectionGeneration = 7
		for _, cost := range costs {
			id := State.CurrencyID(cost.Key.ID)
			state.Player.Currencies[id] = cost.PerUnit * 9
			state.Player.CurrencyObservations[id] = State.PlayerResourceObservation{ConnectionGeneration: 7, ObservedAt: time.Now().UTC()}
		}
		amounts, block := Automation.ProductionStackAmounts(state, costs, 280, 1)
		// Fractional balances are floored before division, so 13.5 / 1.5 affords 8.
		if block != nil || !reflect.DeepEqual(amounts, []int64{8}) {
			t.Fatalf("multi-cost sizing %v %v", amounts, block)
		}
	}
}

func TestCIT118MergedCompositePreservesAllDispatchGates(t *testing.T) {
	for _, name := range []string{"premium commander", "coins", "feathers", "production costs"} {
		t.Run(name, func(t *testing.T) {
			input := specialCostInput(t, `{"wodID":88118,"costC1":"1"}`, `[["DGA",0]]`)
			gates := newFinalDispatchGates(newCoinDispatchGate(), newTravelTicketDispatchGate(), newSpecialCostDispatchGate())
			var step Intent.Step
			switch name {
			case "premium commander":
				step = Intent.Step{Opcode: "cra", Payload: json.RawMessage(`{"LID":-14,"BPC":1}`)}
			case "coins":
				input.State.Player.Resources[1] = 0
				step = costStep(88118, 1)
			case "feathers":
				input.State.Player.Currencies[22] = 0
				input.State.Player.CurrencyObservations[22] = State.PlayerResourceObservation{ConnectionGeneration: 7, ObservedAt: time.Now().UTC()}
				step = Intent.Step{Opcode: "cds", Payload: json.RawMessage(`{"SID":77118,"TX":50,"TY":0,"HBW":-1,"PTT":1,"LID":1,"BPC":0,"A":[[88118,1]]}`)}
			case "production costs":
				step = costStep(513, 1)
			}
			err := gates.Validate(coinGateContext("merged-"+name), input, step)
			if err == nil {
				t.Fatal("dispatch gate missing")
			}
			switch name {
			case "premium commander":
				var blocked *Intent.SupportCommanderUnavailableError
				if !errors.As(err, &blocked) {
					t.Fatalf("premium backstop missing: %v", err)
				}
			case "coins":
				if !errors.Is(err, Intent.ErrCoinUnavailable) {
					t.Fatalf("coin gate missing: %v", err)
				}
			case "feathers":
				if !errors.Is(err, Intent.ErrCurrencyUnavailable) {
					t.Fatalf("feather gate missing: %v", err)
				}
			case "production costs":
				if !errors.Is(err, Intent.ErrBalanceUnavailable) {
					t.Fatalf("production gate missing: %v", err)
				}
			}
		})
	}
}
