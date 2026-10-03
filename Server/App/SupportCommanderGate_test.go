package App

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"CitadelDesktop/Server/CommanderFeatures"
	"CitadelDesktop/Server/GameData"
	"CitadelDesktop/Server/Intent"
	"CitadelDesktop/Server/Protocol"
	"CitadelDesktop/Server/State"
	"CitadelDesktop/Server/Telemetry"
)

func quotaInput(t *testing.T, used int) Intent.PlanningContext {
	input := supportCommanderTestInput(t, Intent.PlanningContext{State: State.NewGameState()})
	input.State.Player.VIP.UsedPremiumCommanders = used
	input.State.Session.LoggedIn = true
	input.State.Session.SocketReady = true
	input.AutomationLane = "autoBird"
	input.SupportSendKey = "invented-send"
	return input
}
func quotaStep(token string) Intent.Step {
	return Intent.Step{Opcode: "cds", Payload: json.RawMessage(`{"LID":-14,"BPC":1}`), SupportCommanderReservation: token}
}
func assertSupportKey(t *testing.T, err error, key string) {
	t.Helper()
	var unavailable *Intent.SupportCommanderUnavailableError
	if !errors.As(err, &unavailable) || unavailable.LocalizationMessage().Key != key {
		t.Fatalf("error = %v; want %s", err, key)
	}
}

func TestSupportPremiumQuotaCurrentVIPAndNoPremiumAccountRoute(t *testing.T) {
	for _, kind := range []string{"available", "used", "unknown", "old-session", "old-connection", "expired", "inactive", "future", "missing-catalog"} {
		t.Run(kind, func(t *testing.T) {
			input := quotaInput(t, 0)
			switch kind {
			case "used":
				input.State.Player.VIP.UsedPremiumCommanders = 100
			case "unknown":
				input.State.Player.VIP.ObservedAt = time.Time{}
			case "old-session":
				input.State.Player.VIP.Generation++
			case "old-connection":
				input.State.Player.VIP.ConnectionGeneration++
			case "expired":
				input.State.Player.VIP.ObservedAt = time.Now().Add(-2 * time.Hour)
			case "inactive":
				input.State.Player.VIP.RemainingSec = 0
			case "future":
				input.State.Player.VIP.ObservedAt = time.Now().Add(time.Hour)
			case "missing-catalog":
				input.GameData = nil
			}
			gate := newPremiumCommanderDispatchGate()
			token, ok := gate.ReservePremiumCommander(input)
			if ok != (kind == "available") || ok && token == "" {
				t.Fatalf("token=%q available=%v", token, ok)
			}
		})
	}
}

func TestSupportSharedQuotaConcurrentAllSenders(t *testing.T) {
	input := quotaInput(t, 98)
	gate := newPremiumCommanderDispatchGate()
	var group sync.WaitGroup
	results := make(chan string, 20)
	for i := 0; i < 20; i++ {
		group.Add(1)
		go func(i int) {
			defer group.Done()
			in := input
			in.OperationID = fmt.Sprintf("invented-op-%d", i)
			in.SupportSendKey = in.OperationID
			if i%4 < 2 {
				_, token, err := gate.SelectSupportCommander(in, []string{"autoBird", "autoStation"}[i%2], State.CastleState{})
				if err == nil {
					results <- token
				}
			} else if token, ok := gate.ReservePremiumCommander(in); ok {
				results <- token
			}
		}(i)
	}
	group.Wait()
	close(results)
	if len(results) != 2 || len(gate.pending) != 2 {
		t.Fatalf("reserved %d movements, pending %d", len(results), len(gate.pending))
	}
}

func TestSupportPremiumReservationLifecycleAndDispatchRecheck(t *testing.T) {
	input := quotaInput(t, 99)
	gate := newPremiumCommanderDispatchGate()
	ctx := coinGateContext(input.OperationID)
	token, ok := gate.ReservePremiumCommander(input)
	if !ok {
		t.Fatal("no quota")
	}
	step := quotaStep(token)
	if err := gate.Validate(coinGateContext("invented-other-op"), input, step); err == nil {
		t.Fatal("stolen reservation")
	}
	if err := gate.Validate(ctx, input, step); err != nil {
		t.Fatal(err)
	}
	gate.Indeterminate(ctx, step)
	gate.OperationFinished(ctx)
	if _, ok := gate.ReservePremiumCommander(input); ok {
		t.Fatal("uncertain send released quota")
	}
	// An unchanged UPG does not prove that the pending movement was accounted for.
	input.State.Player.VIP.ObservedAt = nextSupportVIPObservation(input.State.Player.VIP.ObservedAt)
	if _, ok := gate.ReservePremiumCommander(input); ok {
		t.Fatal("unchanged UPG released quota")
	}
	input.State.Player.VIP.UsedPremiumCommanders = 100
	input.State.Player.VIP.ObservedAt = nextSupportVIPObservation(input.State.Player.VIP.ObservedAt)
	gate.Completed(ctx, input, step, Protocol.CommittedFrame{})
	if len(gate.pending) != 0 {
		t.Fatal("UPG increase did not settle send")
	}
	input.State.Player.VIP.UsedPremiumCommanders = 0
	input.State.Player.VIP.ObservedAt = nextSupportVIPObservation(input.State.Player.VIP.ObservedAt)
	input.SupportSendKey = "invented-after-reset"
	token, ok = gate.ReservePremiumCommander(input)
	if !ok {
		allowance, known := supportPremiumAllowance(input)
		t.Fatalf("daily reset did not resume: allowance=%d known=%v vip=%+v watermark=%+v pending=%+v", allowance, known, input.State.Player.VIP, gate.watermark, gate.pending)
	}
	input.State.Player.VIP.UsedPremiumCommanders = 100
	input.State.Player.VIP.ObservedAt = nextSupportVIPObservation(input.State.Player.VIP.ObservedAt)
	if err := gate.Validate(ctx, input, quotaStep(token)); err == nil {
		t.Fatal("quota changed before dispatch")
	}
	gate.DefinitiveFailure(ctx, quotaStep(token))
	if len(gate.pending) != 0 {
		t.Fatal("failure leaked reservation")
	}
}

func TestSupportDispatchPremiumBackstopEveryBuilder(t *testing.T) {
	for _, opcode := range []string{"cra", "cds"} {
		for _, payload := range []string{`{"LID":5,"BPC":1}`, `{"LID":-14,"BPC":0}`, `{"LID":-14,"BPC":1}`} {
			input := quotaInput(t, 0)
			gate := newPremiumCommanderDispatchGate()
			err := gate.Validate(coinGateContext(input.OperationID), input, Intent.Step{Opcode: opcode, Payload: json.RawMessage(payload)})
			assertSupportKey(t, err, "server.support.premium_blocked")
		}
	}
	input := quotaInput(t, 0)
	gate := newPremiumCommanderDispatchGate()
	token, _ := gate.ReservePremiumCommander(input)
	step := quotaStep(token)
	step.Opcode = "cra"
	if err := gate.Validate(coinGateContext(input.OperationID), input, step); err == nil {
		t.Fatal("CRA accepted even with reservation")
	}
}

func addSupportCommander(input *Intent.PlanningContext, id State.CommanderID, speed, cost float64) {
	equipmentID := State.EquipmentInstanceID(id + 1000)
	input.State.Commanders[id] = State.CommanderState{ID: id, Available: true, Equipment: map[string]State.EquipmentInstanceID{"weapon": equipmentID}}
	input.State.Inventory.Equipment[equipmentID] = State.EquipmentInstance{ID: equipmentID, Effects: State.EquipmentEffects{{DefinitionID: 263, Values: []float64{speed}}, {DefinitionID: 265, Values: []float64{cost}}, {DefinitionID: 264, Values: []float64{10000}}, {DefinitionID: 266, Values: []float64{10000}}}}
}

func TestSupportCommanderRankingEligibilityAndHolds(t *testing.T) {
	for _, test := range []struct {
		name        string
		speed, cost float64
		want        State.CommanderID
	}{
		{"speed", 60, 0, 9}, {"cost", 50, 40, 9}, {"id", 50, 30, 5}, {"capped", 200, 30, 5},
	} {
		t.Run(test.name, func(t *testing.T) {
			input := quotaInput(t, 100)
			gate := newPremiumCommanderDispatchGate()
			baseSpeed := 50.0
			if test.name == "capped" {
				baseSpeed = 110
			}
			addSupportCommander(&input, 5, baseSpeed, 30)
			addSupportCommander(&input, 9, test.speed, test.cost)
			addSupportCommander(&input, 3, 99, 99)
			commander := input.State.Commanders[3]
			commander.Available = false
			input.State.Commanders[3] = commander
			addSupportCommander(&input, 4, 99, 99)
			arrives := time.Now().Add(time.Hour)
			input.State.Player.ID = 99
			input.State.Movements[44] = State.MovementState{ID: 44, OwnerPlayerID: 99, CommanderID: ptrCommander(4), ArrivesAt: &arrives}
			addSupportCommander(&input, 6, 99, 99)
			input.CommanderHolds.HoldCommanders([]State.CommanderID{6}, time.Now().Add(time.Hour))
			addSupportCommander(&input, 7, 99, 99)
			gate.assignments = func() (CommanderFeatures.Configuration, error) {
				return CommanderFeatures.Configuration{Assignments: map[string][]State.CommanderID{"autoBird": {3, 4, 5, 6, 9}}}, nil
			}
			id, token, err := gate.SelectSupportCommander(input, "autoBird", State.CastleState{})
			if err != nil || id != test.want {
				t.Fatalf("id=%d err=%v", id, err)
			}
			if !input.CommanderHolds.CommanderHeldAt(id, time.Now()) {
				t.Fatal("selected commander not held")
			}
			same, again, err := gate.SelectSupportCommander(input, "autoBird", State.CastleState{})
			if err != nil || same != id || again != token {
				t.Fatal("replan could not use its own hold")
			}
			gate.ReleaseSupportCommander(token)
			if input.CommanderHolds.CommanderHeldAt(id, time.Now()) {
				t.Fatal("failure leaked hold")
			}
		})
	}
}
func ptrCommander(id State.CommanderID) *State.CommanderID { return &id }

func TestSupportNoCommanderWaitAndFeatherIndependentChoice(t *testing.T) {
	input := quotaInput(t, 100)
	gate := newPremiumCommanderDispatchGate()
	_, _, err := gate.SelectSupportCommander(input, "autoBird", State.CastleState{})
	assertSupportKey(t, err, "server.support.commander_wait")
	for _, tickets := range []float64{0, 100} {
		input := quotaInput(t, 100)
		input.State.Player.Currencies[22] = tickets
		gate := newPremiumCommanderDispatchGate()
		addSupportCommander(&input, 8, 50, 40)
		id, _, err := gate.SelectSupportCommander(input, "autoStation", State.CastleState{})
		if err != nil || id != 8 {
			t.Fatalf("tickets=%v id=%d err=%v", tickets, id, err)
		}
	}
}

func TestSupportBatchesReserveEachMovementAndGoldenRealCommander(t *testing.T) {
	input := quotaInput(t, 99)
	input.AutomationLane = "autoStation"
	input.State.Player.CurrencyObservations[22] = State.PlayerResourceObservation{ObservedAt: time.Now(), ConnectionGeneration: 1}
	input.State.Player.Currencies[22] = 100
	addSupportCommander(&input, 8, 50, 40)
	gate := newPremiumCommanderDispatchGate()
	input.SupportCommanders = gate
	amounts := map[State.UnitID]int64{}
	for i := 1; i <= 11; i++ {
		amounts[State.UnitID(i)] = 10
	}
	step, err := supportDispatchStep(input, "invented-support", State.CastleState{ID: 10}, State.AllianceHolding{X: 20, Y: 30}, 6, amounts, Intent.Step{}, false)
	if err != nil || len(step.Batch) != 2 {
		t.Fatalf("batches=%d err=%v", len(step.Batch), err)
	}
	premium := supportTravelPayload(t, step.Batch[0])
	owned := supportTravelPayload(t, step.Batch[1])
	if string(premium["LID"]) != "-14" || string(premium["BPC"]) != "1" || string(owned["LID"]) != "8" || string(owned["BPC"]) != "0" {
		t.Fatal("selection per movement incorrect")
	}
	// Captured CDS constructor key set. All route/travel fields stay unchanged.
	golden := json.RawMessage(`{"SID":10,"TX":20,"TY":30,"LID":8,"WT":6,"HBW":-1,"BPC":0,"PTT":1,"SD":0,"A":[[11,10]]}`)
	var expected map[string]json.RawMessage
	json.Unmarshal(golden, &expected)
	if !reflect.DeepEqual(owned, expected) {
		t.Fatalf("owned golden differs: %s", step.Batch[1].Command.Payload)
	}
	if len(gate.pending) != 2 || step.Batch[0].SupportCommanderReservation == step.Batch[1].SupportCommanderReservation {
		t.Fatal("batch reservation reused")
	}
	gate.OperationFinished(coinGateContext(input.OperationID))
	if len(gate.pending) != 0 || input.CommanderHolds.CommanderHeldAt(8, time.Now()) {
		t.Fatal("unexecuted plan leaked")
	}
}

func TestSupportManualUnavailableAndStormWaitThenVIPResume(t *testing.T) {
	for _, unknown := range []bool{false, true} {
		input := quotaInput(t, 100)
		if unknown {
			input.State.Player.VIP.ObservedAt = time.Time{}
		}
		for _, storm := range []bool{false, true} {
			_, err := reservePremiumCommander(input, storm)
			key := "server.support.manual_quota"
			if storm {
				key = "server.support.storm_quota"
			}
			if unknown {
				key = strings.Replace(key, "quota", "vip_unknown", 1)
			}
			assertSupportKey(t, err, key)
		}
		input.State.Player.VIP.ObservedAt = time.Now()
		input.State.Player.VIP.UsedPremiumCommanders = 0
		if token, err := reservePremiumCommander(input, true); err != nil || token == "" {
			t.Fatalf("fresh VIP did not resume: %v", err)
		}
	}
}

func TestRiftPremiumCapturedTemplateBlockedBeforeSubstitutionAndUnchanged(t *testing.T) {
	for _, premium := range []string{`"LID":5,"BPC":1`, `"LID":-14,"BPC":0`, `"LID":5,"BPC":0`} {
		input := quotaInput(t, 0)
		addSupportCommander(&input, 5, 0, 0)
		input.State.Castles[10] = State.CastleState{ID: 10, Focused: true, X: 1, Y: 2}
		body := json.RawMessage(`{` + premium + `,"SX":1,"SY":2,"TX":30,"TY":40,"KID":0,"A":[{}]}`)
		input.State.Rift.Launches["invented-launch"] = State.RiftLaunch{ID: "invented-launch", DisplayName: "Invented launch", CommanderID: 5, Body: body}
		before := append([]byte(nil), body...)
		app := &Application{}
		plan, err := app.planRiftReplay(context.Background(), input, json.RawMessage(`{"launchId":"invented-launch","commanderId":5,"sourceCastleId":10}`))
		if premiumCommanderPayload(body) {
			assertSupportKey(t, err, "server.rift.premium_capture")
			if len(plan.Steps) != 0 {
				t.Fatal("premium template planned commands")
			}
		} else if err != nil {
			t.Fatal(err)
		}
		if string(body) != string(before) || string(input.State.Rift.Launches["invented-launch"].Body) != string(before) {
			t.Fatal("template rewritten")
		}
	}
}

func TestSupportPremiumBuildersOnlyUseReservationBackstop(t *testing.T) {
	// Any builder using the premium constant must attach the shared reservation.
	files := []string{}
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if strings.HasSuffix(entry.Name(), ".go") && !strings.HasSuffix(entry.Name(), "_test.go") {
			files = append(files, entry.Name())
		}
	}
	for _, name := range files {
		source, err := os.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(source), `"cds"`) {
			continue
		}
		tree, err := parser.ParseFile(token.NewFileSet(), filepath.Clean(name), source, 0)
		if err != nil {
			t.Fatal(err)
		}
		ast.Inspect(tree, func(node ast.Node) bool {
			assignment, ok := node.(*ast.AssignStmt)
			if !ok {
				return true
			}
			for _, rhs := range assignment.Rhs {
				if literal, ok := rhs.(*ast.UnaryExpr); ok {
					if integer, ok := literal.X.(*ast.BasicLit); ok && literal.Op == token.SUB && integer.Value == "14" {
						t.Errorf("%s hardcodes premium lord in CDS builder", name)
					}
				}
			}
			return true
		})
		if strings.Contains(string(source), "premiumSupportCommander") && (!strings.Contains(string(source), "SupportCommanderReservation") || !strings.Contains(string(source), "reservePremiumCommander") && !strings.Contains(string(source), "selectSupportCommander")) {
			t.Errorf("%s has unreserved premium CDS builder", name)
		}
	}
}

func TestSupportManualPlannerFailsBeforeCommandsAndUsesQuota(t *testing.T) {
	state, data := autoBirdIntentTestState(t, time.Now().UTC())
	state.Castles[10] = State.CastleState{ID: 10, Focused: true, Units: State.CastleUnits{Stationed: map[State.UnitID]int64{215: 10}}}
	args := json.RawMessage(`{"sourceCastleId":10,"targetCastleId":20,"delayHours":6,"units":[{"unitId":215,"amount":10}]}`)
	for _, test := range []struct {
		used    int
		unknown bool
		key     string
	}{{0, false, ""}, {100, false, "server.support.manual_quota"}, {0, true, "server.support.manual_vip_unknown"}} {
		input := supportCommanderTestInput(t, Intent.PlanningContext{State: state, GameData: data})
		input.State.Player.VIP.UsedPremiumCommanders = test.used
		if test.unknown {
			input.State.Player.VIP.ObservedAt = time.Time{}
		}
		plan, err := planTroopsStation(t.Context(), input, args)
		if test.key != "" {
			assertSupportKey(t, err, test.key)
			if len(plan.Steps) != 0 {
				t.Fatal("unavailable manual request planned context commands")
			}
			continue
		}
		if err != nil || len(plan.Steps) == 0 {
			t.Fatalf("manual plan=%+v err=%v", plan, err)
		}
		step, err := resolveTroopsStationStep(t.Context(), input, args)
		if err != nil {
			t.Fatal(err)
		}
		fields := supportTravelPayload(t, step)
		if string(fields["LID"]) != "-14" || string(fields["BPC"]) != "1" || step.SupportCommanderReservation == "" {
			t.Fatal("manual request did not reserve premium")
		}
	}
}

func TestSupportStormPlannerWaitsUntilVIPQuotaResets(t *testing.T) {
	input := quotaInput(t, 100)
	data, err := GameData.DecodeStore([]byte(`{"versionInfo":[],"buildings":[],"units":[{"wodID":10}]}`), GameData.SourceMetadata{ItemVersion: "invented-storm-fixture"})
	if err != nil {
		t.Fatal(err)
	}
	input = supportCommanderTestInput(t, Intent.PlanningContext{State: input.State, GameData: data})
	input.State.Player.VIP.UsedPremiumCommanders = 100
	input.AutomationLane = "autoStorm"
	input.State.Player.Currencies[22] = 100
	input.State.Player.CurrencyObservations[22] = State.PlayerResourceObservation{ObservedAt: time.Now(), ConnectionGeneration: 1}
	input.State.Castles[40] = State.CastleState{ID: 40, KingdomID: 4, X: 200, Y: 300, Focused: true}
	key := State.StormIslandReturnKey(4, 101, 102)
	input.State.Storm.IslandReturns[key] = State.StormIslandReturnState{KingdomID: 4, SourceCastleID: 40, TargetX: 101, TargetY: 102, IslandObjectID: 777, ReportID: 202, Status: State.StormIslandReturnReady, LeaveBehind: 1, Survivors: map[State.UnitID]int64{10: 4}, ReportedAt: time.Now()}
	args := json.RawMessage(`{"sourceCastleId":40,"kingdomId":4,"islandX":101,"islandY":102,"islandObjectId":777,"reportId":202,"units":[{"unitId":10,"amount":3}]}`)
	plan, err := planStormIslandReturn(t.Context(), input, args)
	assertSupportKey(t, err, "server.support.storm_quota")
	if len(plan.Steps) != 0 {
		t.Fatal("waiting storm return planned commands")
	}
	input.State.Player.VIP.ObservedAt = time.Now()
	input.State.Player.VIP.UsedPremiumCommanders = 0
	plan, err = planStormIslandReturn(t.Context(), input, args)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, step := range plan.Steps {
		if step.Opcode == "cds" {
			found = true
			fields := supportTravelPayload(t, step)
			if string(fields["LID"]) != "-14" || string(fields["BPC"]) != "1" || step.SupportCommanderReservation == "" {
				t.Fatal("storm did not reserve premium")
			}
		}
	}
	if !found {
		t.Fatal("storm did not resume after VIP reset")
	}
}

func TestSupportBackstopRuntimeSendsNothingAndNeverLocksLane(t *testing.T) {
	for _, opcode := range []string{"cds", "cra"} {
		input := quotaInput(t, 0)
		store := State.NewStore(&input.State)
		registry := Intent.NewRegistry()
		err := registry.Register(Intent.Definition{Name: "invented.premium.backstop", Effect: Intent.EffectWrite, Planner: func(context.Context, Intent.PlanningContext, json.RawMessage) (Intent.Plan, error) {
			step := quotaStep("")
			step.Opcode = opcode
			return Intent.Plan{Steps: []Intent.Step{step}}, nil
		}})
		if err != nil {
			t.Fatal(err)
		}
		observer := newCoinGateEngineObserver()
		sender := &replayTravelSender{store: store, observer: observer, gameData: input.GameData}
		engine := Intent.NewEngine(registry, store, coinGateStoreProvider{input.GameData}, sender, observer)
		engine.SetFinalDispatchProvider(newFinalDispatchGates(newCoinDispatchGate(), newTravelTicketDispatchGate()))
		receipt := engine.Submit(t.Context(), Intent.Request{Name: "invented.premium.backstop", Actor: "automation:autoBird", AutomationLane: "autoBird"})
		if len(sender.payloads) != 0 || receipt.Status != Intent.StatusFailed || receipt.Failure == nil || receipt.Failure.Kind != Intent.FailureAvailability || receipt.Failure.Toast || !store.Snapshot().Automations["autoBird"].SafetyLock.ObservedAt.IsZero() {
			t.Fatalf("opcode=%s sends=%d receipt=%+v", opcode, len(sender.payloads), receipt)
		}
	}
}

func TestSupportDryRunNeverDebitsQuotaOrHoldsCommander(t *testing.T) {
	input := quotaInput(t, 99)
	input.DryRun = true
	gate := newPremiumCommanderDispatchGate()
	for i := 0; i < 3; i++ {
		if _, ok := gate.ReservePremiumCommander(input); !ok {
			t.Fatal("dry run depleted quota")
		}
	}
	if len(gate.pending) != 0 {
		t.Fatal("dry run debited premium quota")
	}
	input.State.Player.VIP.UsedPremiumCommanders = 100
	addSupportCommander(&input, 8, 10, 10)
	for i := 0; i < 3; i++ {
		if _, _, err := gate.SelectSupportCommander(input, "autoBird", State.CastleState{}); err != nil {
			t.Fatal(err)
		}
	}
	if len(gate.pending) != 0 || input.CommanderHolds.CommanderHeldAt(8, time.Now()) {
		t.Fatal("dry run held commander")
	}
}

func TestSupportOfficialUnboundedCap99RanksWithoutInventedLimit(t *testing.T) {
	input := quotaInput(t, 100)
	collections := map[string]json.RawMessage{}
	for _, name := range input.GameData.CatalogNames() {
		collections[name], _ = input.GameData.RawCollection(name)
	}
	collections["versionInfo"] = json.RawMessage(`[]`)
	collections["effectCaps"] = json.RawMessage(`[{"capID":"99"}]`)
	raw, _ := json.Marshal(collections)
	data, err := GameData.DecodeStore(raw, GameData.SourceMetadata{ItemVersion: "invented-unbounded-cap-fixture"})
	if err != nil {
		t.Fatal(err)
	}
	input.GameData = data
	addSupportCommander(&input, 5, 120, 0)
	addSupportCommander(&input, 9, 160, 0)
	gate := newPremiumCommanderDispatchGate()
	id, _, err := gate.SelectSupportCommander(input, "autoBird", State.CastleState{})
	if err != nil || id != 9 {
		t.Fatalf("id=%d err=%v", id, err)
	}
}

func TestRiftPremiumCaptureWarningDeduplicatedAndNamesLaunch(t *testing.T) {
	app := &Application{Telemetry: Telemetry.NewStore(20)}
	defer app.Telemetry.Close()
	body := json.RawMessage(`{"LID":-14,"BPC":1}`)
	for i := 0; i < 3; i++ {
		app.recordPremiumRiftCaptureWarning("invented-launch", "Invented launch", body)
	}
	lines := app.Telemetry.Tail(Telemetry.ChannelRift, 20)
	if len(lines) != 1 || !strings.Contains(lines[0], "Invented launch") {
		t.Fatalf("warnings=%+v", lines)
	}
}

func nextSupportVIPObservation(previous time.Time) time.Time {
	for {
		now := time.Now().UTC()
		if now.After(previous) {
			return now
		}
	}
}
