package Intent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"
	"time"

	"CitadelDesktop/Server/Ingest"
	"CitadelDesktop/Server/Protocol"
	"CitadelDesktop/Server/State"
)

func TestLaneSafetyPolicyIsAnExactAllowlist(t *testing.T) {
	for _, test := range []struct {
		opcode  string
		code    int
		allowed bool
	}{
		{"adi", 95, true}, {" ADI ", 95, true}, {"adi", 91, false},
		{"ere", 227, true}, {" EQE ", 227, true}, {"bup", 87, true},
		{"cra", 95, false}, {"cra", 256, false}, {"msd", 311, false},
		{"ere", 226, false}, {"ere", 236, false}, {"eqe", 222, false},
		{"bup", 203, false}, {"bup", 227, false}, {"adi", 87, false},
		{"sbp", 55, false}, {"sbp", 159, false}, {"sbp", 203, false},
		{"msd", 87, false}, {"msd", 95, false}, {"msd", 227, false},
		{"future", 87, false}, {"future", 95, false}, {"future", 227, false}, {"future", 9876, false},
	} {
		if got := rejectionAllowsRecovery(test.opcode, test.code); got != test.allowed {
			t.Errorf("%s %d recovery=%t", test.opcode, test.code, got)
		}
	}
}

func TestLaneSafetyRejectsMissingOriginWithoutGuessingFeatureLock(t *testing.T) {
	store := State.NewStore(State.NewGameState())
	registry := NewRegistry()
	planned := false
	if err := registry.Register(Definition{Name: "test.missing-lane", Effect: EffectWrite,
		Planner: func(context.Context, PlanningContext, json.RawMessage) (Plan, error) {
			planned = true
			return Plan{}, nil
		},
	}); err != nil {
		t.Fatal(err)
	}
	engine := NewEngine(registry, store, nil, nil, nil)
	receipt := engine.Submit(t.Context(), Request{Name: "test.missing-lane", Actor: "automation:autoKhan"})
	if receipt.Status != StatusFailed || planned {
		t.Fatalf("missing lane executed: %#v", receipt)
	}
	if len(store.ReadOnlyView().Automations) != 0 {
		t.Fatal("missing lane created a feature lock")
	}
}

func TestLaneSafetyStopsRetriesCompensationAndChains(t *testing.T) {
	for _, mode := range []string{"retry", "stale", "compensation", "no-success-codes", "response-alias", "no-await-opcode"} {
		t.Run(mode, func(t *testing.T) {
			store := State.NewStore(State.NewGameState())
			pipeline := Ingest.NewPipeline(store, nil, Ingest.NewRegistry())
			sender := &responseSequenceSender{pipeline: pipeline, responseCodes: []int{226, 0}}
			registry := NewRegistry()
			step := Step{Name: "Rejected command", Opcode: "ere", AwaitOpcode: "ere", TimeoutMillis: 1000,
				SuccessCodes: []int{0}, CaptureResponse: true, Command: Protocol.Command{Opcode: "ere", Payload: json.RawMessage(`{}`)}}
			switch mode {
			case "retry":
				step.ResponseRetry = &ResponseRetryPolicy{Codes: []int{226}, GuardAction: "test.recovery", DelayMillis: 1}
			case "stale":
				step.StaleCodes = []int{226}
			case "compensation":
				step.PreDispatchAction = "test.setup"
				step.DefinitiveResponseFailureAction = "test.recovery"
			case "no-success-codes":
				step.SuccessCodes = nil
			case "response-alias":
				step.AwaitOpcode = "other"
			case "no-await-opcode":
				step.AwaitOpcode = ""
			}
			if err := registry.Register(Definition{Name: "test.safety", Effect: EffectWrite,
				Planner: func(context.Context, PlanningContext, json.RawMessage) (Plan, error) {
					return Plan{Steps: []Step{{Name: "Prior progress", Action: "test.setup"}, step, {Name: "Chain continuation", Action: "test.recovery"}}}, nil
				}}); err != nil {
				t.Fatal(err)
			}
			engine := NewEngine(registry, store, nil, sender, pipeline)
			setup, recovery := 0, 0
			if err := engine.RegisterAction("test.setup", func(context.Context, json.RawMessage) error { setup++; return nil }); err != nil {
				t.Fatal(err)
			}
			if err := engine.RegisterAction("test.recovery", func(context.Context, json.RawMessage) error { recovery++; return nil }); err != nil {
				t.Fatal(err)
			}
			receipt := engine.Submit(t.Context(), Request{ID: "incident", Name: "test.safety", Actor: "automation:shared", AutomationLane: "lane-a"})
			if receipt.Status != StatusPartiallySucceeded || receipt.Failure == nil || receipt.Failure.SafetyLock == nil || !receipt.Failure.Toast {
				t.Fatalf("missing safety failure: %#v", receipt)
			}
			wantSetup := 1
			if mode == "compensation" {
				wantSetup = 2
			}
			if sends, _ := sender.snapshot(); sends != 1 || setup != wantSetup || recovery != 0 {
				t.Fatalf("sends=%d setup=%d recovery=%d", sends, setup, recovery)
			}
			if len(receipt.CompletedStepIndexes) != 1 || len(receipt.Exchanges) != 1 {
				t.Fatalf("lost prior progress or evidence: %#v", receipt)
			}
			blocked := engine.Submit(t.Context(), Request{Name: "test.safety", Actor: "automation:shared", AutomationLane: "lane-a"})
			if blocked.Failure == nil || blocked.Failure.SafetyLock == nil || blocked.Failure.Toast {
				t.Fatalf("blocked receipt=%#v", blocked)
			}
			if sends, _ := sender.snapshot(); sends != 1 {
				t.Fatalf("locked lane sent %d commands", sends)
			}
			other := engine.Submit(t.Context(), Request{Name: "test.safety", Actor: "automation:shared", AutomationLane: "lane-b"})
			if mode != "response-alias" && other.Status != StatusSucceeded {
				t.Fatalf("unrelated lane blocked: %#v", other)
			}
			if sends, _ := sender.snapshot(); sends != 2 {
				t.Fatal("unrelated lane did not dispatch")
			}
		})
	}
}

func safetyContext(id, lane string) context.Context {
	return context.WithValue(context.Background(), laneSafetyContextKey{}, Request{ID: id, Name: "test.action", Actor: "automation:shared", AutomationLane: lane})
}

func TestLaneSafetyAllowlistedRejectionsRemainFailuresWithoutLocking(t *testing.T) {
	for _, pair := range []struct {
		opcode string
		code   int
	}{{"adi", 95}, {"ere", 227}, {"eqe", 227}, {"bup", 87}} {
		t.Run(fmt.Sprintf("%s-%d", pair.opcode, pair.code), func(t *testing.T) {
			store := State.NewStore(State.NewGameState())
			pipeline := Ingest.NewPipeline(store, nil, Ingest.NewRegistry())
			sender := &responseSequenceSender{pipeline: pipeline, responseCodes: []int{pair.code, 0}}
			registry := NewRegistry()
			if err := registry.Register(Definition{Name: "test.allowed", Effect: EffectWrite,
				Planner: func(context.Context, PlanningContext, json.RawMessage) (Plan, error) {
					return Plan{Steps: []Step{{Name: "Command", Opcode: pair.opcode, AwaitOpcode: pair.opcode,
						SuccessCodes: []int{0}, TimeoutMillis: 1000, CaptureResponse: true,
						Command: Protocol.Command{Opcode: pair.opcode, Payload: json.RawMessage(`{}`)}}}}, nil
				}}); err != nil {
				t.Fatal(err)
			}
			engine := NewEngine(registry, store, nil, sender, pipeline)
			request := Request{Name: "test.allowed", Actor: "automation:shared", AutomationLane: "lane"}
			receipt := engine.Submit(t.Context(), request)
			if receipt.Status != StatusFailed || receipt.Failure == nil || receipt.Failure.SafetyLock != nil ||
				receipt.Failure.GameCode == nil || *receipt.Failure.GameCode != pair.code {
				t.Fatalf("allowlisted rejection was swallowed or locked: %#v", receipt)
			}
			if engine.AutomationLaneLock("lane").OperationID != "" {
				t.Fatal("allowlisted response recorded a lane lock")
			}
			if sends, _ := sender.snapshot(); sends != 1 {
				t.Fatal("allowlist added an undeclared retry")
			}
			if next := engine.Submit(t.Context(), request); next.Status != StatusSucceeded {
				t.Fatalf("allowlisted rejection blocked next operation: %#v", next)
			}
		})
	}
}

func TestLaneSafetyAllowlistedEnchantmentRetriesStillCheckReserves(t *testing.T) {
	for _, opcode := range []string{"ere", "eqe"} {
		t.Run(opcode, func(t *testing.T) {
			store := State.NewStore(State.NewGameState())
			pipeline := Ingest.NewPipeline(store, nil, Ingest.NewRegistry())
			sender := &responseSequenceSender{pipeline: pipeline, responseCodes: []int{227, 227, 0}}
			registry := NewRegistry()
			if err := registry.Register(Definition{Name: "test.enchant", Effect: EffectWrite,
				Planner: func(context.Context, PlanningContext, json.RawMessage) (Plan, error) {
					return Plan{Steps: []Step{{Name: "Enchant", Opcode: opcode, AwaitOpcode: opcode,
						SuccessCodes: []int{0}, TimeoutMillis: 1000, Command: Protocol.Command{Opcode: opcode, Payload: json.RawMessage(`{}`)},
						ResponseRetry: &ResponseRetryPolicy{Codes: []int{227}, GuardAction: "test.reserve", DelayMillis: 1}}}}, nil
				}}); err != nil {
				t.Fatal(err)
			}
			engine := NewEngine(registry, store, nil, sender, pipeline)
			guards := 0
			if err := engine.RegisterAction("test.reserve", func(context.Context, json.RawMessage) error {
				guards++
				return nil
			}); err != nil {
				t.Fatal(err)
			}
			receipt := engine.Submit(t.Context(), Request{Name: "test.enchant", Actor: "automation:equipment", AutomationLane: "equipment"})
			if receipt.Status != StatusSucceeded || guards != 2 || len(receipt.CompletedStepIndexes) != 1 {
				t.Fatalf("retry lost reserve checks or counted failed rolls: guards=%d receipt=%#v", guards, receipt)
			}
			if sends, _ := sender.snapshot(); sends != 3 {
				t.Fatalf("enchantment sends=%d", sends)
			}
			if engine.AutomationLaneLock("equipment").OperationID != "" {
				t.Fatal("approved failed rolls locked lane")
			}
		})
	}
}

func TestLaneSafetyDurableRestartReviewAndCooldown(t *testing.T) {
	dir := t.TempDir()
	newEngine := func(store *State.Store) *Engine {
		engine := NewEngine(NewRegistry(), store, nil, nil, nil)
		engine.SetLaneSafetyPersistence(func(_ context.Context, event State.Event) error {
			return State.SaveComponentSnapshot(dir, event, State.Components(event.Components...))
		})
		return engine
	}
	engine := newEngine(State.NewStore(State.NewGameState()))
	ctx, cancel := context.WithCancel(safetyContext("unknown-incident", "attack"))
	cancel() // Persistence cannot depend on the canceled operation.
	engine.guardRejection(ctx, NewResponseCodeError(nil, "cra", 256))
	engine.guardRejection(safetyContext("msd-incident", "cooldown"), NewResponseCodeError(nil, "msd", 999))
	before := engine.AutomationLaneLock("cooldown")
	if err := engine.ClearAutomationLaneLock("cooldown", "msd-incident", "try early", "api"); err == nil {
		t.Fatal("manual review bypassed mandatory MSD cooldown")
	}
	if before.Until.Sub(before.ObservedAt) != 30*time.Minute {
		t.Fatalf("MSD TTL=%v", before)
	}
	loaded, err := State.LoadSnapshot(dir)
	if err != nil {
		t.Fatal(err)
	}
	engine = newEngine(State.NewStore(loaded))
	if err := engine.checkLaneSafety(Request{Actor: "automation:attack", AutomationLane: "attack"}); err == nil {
		t.Fatal("restart cleared unknown lock")
	}
	if after := engine.AutomationLaneLock("cooldown"); !after.Until.Equal(before.Until) {
		t.Fatal("restart reset MSD timer")
	}
	if engine.AutomationLaneLock("attack").Active(time.Now().Add(7*24*time.Hour)) != true {
		t.Fatal("unknown lock expired")
	}
	if before.Active(before.Until) {
		t.Fatal("MSD timer did not expire")
	}
	for _, test := range []struct{ operation, review, actor string }{
		{"stale", "Checked game state", "api"}, {"unknown-incident", "", "api"}, {"unknown-incident", "retry", "automation:attack"},
	} {
		if err := engine.ClearAutomationLaneLock("attack", test.operation, test.review, test.actor); err == nil {
			t.Fatalf("invalid review accepted: %+v", test)
		}
	}
	if err := engine.ClearAutomationLaneLock("attack", "unknown-incident", "Game state checked; approve one new attempt", "api"); err != nil {
		t.Fatal(err)
	}
	loaded, err = State.LoadSnapshot(dir)
	if err != nil {
		t.Fatal(err)
	}
	cleared := loaded.Automations["attack"].SafetyLock
	if cleared.Active(time.Now()) || cleared.Review == "" || cleared.ReviewedBy != "api" {
		t.Fatalf("review not durable: %#v", cleared)
	}
	engine.guardRejection(safetyContext("new-incident", "attack"), NewResponseCodeError(nil, "cra", 256))
	if !engine.AutomationLaneLock("attack").Active(time.Now()) {
		t.Fatal("review silently allowlisted rejection")
	}
	if err := engine.ClearAutomationLaneLock("attack", "unknown-incident", "stale screen", "api"); err == nil {
		t.Fatal("stale screen cleared new incident")
	}
}

func TestLaneSafetyActionErrorsAndPersistenceFailureStayClosed(t *testing.T) {
	engine := NewEngine(NewRegistry(), State.NewStore(State.NewGameState()), nil, nil, nil)
	engine.SetLaneSafetyPersistence(func(context.Context, State.Event) error { return errors.New("disk unavailable") })
	err := engine.guardRejection(safetyContext("incident", "lane"), fmt.Errorf("action failed: %w", NewResponseCodeError(nil, "new", 311)))
	var locked *LaneLockedError
	if !errors.As(err, &locked) || engine.checkLaneSafety(Request{Actor: "automation:lane", AutomationLane: "lane"}) == nil {
		t.Fatalf("persistence failure reopened lane: %v", err)
	}
	if err := engine.ClearAutomationLaneLock("lane", "incident", "checked", "api"); err == nil {
		t.Fatal("failed durable clear succeeded")
	}
	if engine.checkLaneSafety(Request{Actor: "automation:lane", AutomationLane: "lane"}) == nil {
		t.Fatal("failed clear left lane open")
	}
	if engine.guardRejection(context.Background(), NewResponseCodeError(nil, "other", 311)) == nil {
		t.Fatal("manual error disappeared")
	}
	if err := engine.checkLaneSafety(Request{Actor: "api", AutomationLane: "lane"}); err != nil {
		t.Fatal("manual request incorrectly treated as automated")
	}
	if got := engine.guardRejection(safetyContext("allowed", "other"), NewResponseCodeError(nil, "adi", 95)); errors.As(got, &locked) {
		t.Fatal("ADI 95 locked")
	}
}
