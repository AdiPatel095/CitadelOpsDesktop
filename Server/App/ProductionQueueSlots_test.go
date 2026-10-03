package App

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"CitadelDesktop/Server/Automation"
	"CitadelDesktop/Server/Configuration"
	"CitadelDesktop/Server/GameData"
	"CitadelDesktop/Server/Ingest"
	"CitadelDesktop/Server/Intent"
	"CitadelDesktop/Server/Protocol"
	"CitadelDesktop/Server/State"
)

// Replay the real QS sequence through raw-frame decoding and registered ingest.
// Use the test clock with the captured 234 ms gap so the planner's wall clock
// doesn't mistake this historical capture for a completed production stack.
func TestProductionQueueCapturedBUPReplay(t *testing.T) {
	now := time.Now().UTC().Add(-time.Second)
	state := State.NewGameState()
	state.Castles[1] = State.CastleState{ID: 1, Focused: true, Production: map[int]State.ProductionQueue{0: {LineID: 0, Capacity: 5, ObservedAt: now.Add(-time.Minute)}}}
	store := State.NewStore(&state)
	registry := Ingest.NewRegistry()
	if err := Ingest.RegisterCoreReducers(registry); err != nil {
		t.Fatal(err)
	}
	pipeline := Ingest.NewPipeline(store, nil, registry)
	data := queueSlotsGameData(t)
	configuration := Configuration.Snapshot{Sections: map[string]json.RawMessage{
		"automation.recruitTroops": json.RawMessage(`{"mode":"global","globalItems":[{"id":607,"amount":110}],"castles":{"1":{"enabled":true}}}`),
	}}
	args := json.RawMessage(`{"castleId":1,"lineId":0,"definitionId":607,"amount":110,"fillAvailable":true}`)
	for index, filename := range []string{"testdata/cit-117/bup-success-1.xt", "testdata/cit-117/bup-success-2.xt"} {
		raw, err := os.ReadFile(filename)
		if err != nil {
			t.Fatal(err)
		}
		observed := now.Add(time.Duration(index) * 234 * time.Millisecond)
		if _, err := pipeline.HandleRawAt(t.Context(), strings.TrimSpace(string(raw)), Protocol.DirectionInbound, observed); err != nil {
			t.Fatal(err)
		}
		snapshot := store.Snapshot()
		queue := snapshot.Castles[1].Production[0]
		if queue.Capacity != 4 || len(queue.Slots) != 5 || len(queue.Queued) != 3+index || State.ProductionQueueFreeSlots(queue, observed) != 1-index {
			t.Fatalf("frame %d: capacity=%d slots=%d occupied=%d free=%d", index+1, queue.Capacity, len(queue.Slots), len(queue.Queued), State.ProductionQueueFreeSlots(queue, observed))
		}
		if !queue.Slots[2].ExpiresAt.Equal(observed.Add(time.Duration(5273268-index) * time.Second)) {
			t.Fatal("captured VIP expiry not preserved")
		}
		decision, err := Automation.NewRecruitPolicy().Evaluate(t.Context(), Automation.Snapshot{State: snapshot, Configuration: configuration, GameData: data, Now: observed})
		if err != nil {
			t.Fatal(err)
		}
		plan, err := planProductionEnqueue(t.Context(), Intent.PlanningContext{State: snapshot, GameData: data}, args)
		if err != nil {
			t.Fatal(err)
		}
		if index == 0 {
			if decision.Request == nil || decision.Request.Name != "production.enqueue" || countQueueSlotBUPs(plan) != 1 {
				t.Fatal("first frame should allow exactly one BUP")
			}
		} else {
			if decision.Status != "waiting" || decision.Request != nil || !strings.Contains(decision.Detail, "full at 4 of 4") || decision.DetailDescriptor == nil || decision.DetailDescriptor.Params["usable"] != 4 || len(plan.Steps) != 0 {
				t.Fatalf("full captured queue: status=%s detail=%q request=%v steps=%d", decision.Status, decision.Detail, decision.Request, len(plan.Steps))
			}
			sender := &queueSlotsSender{}
			receipt := executeQueueSlotPlan(t, store, data, sender, plan, nil)
			if receipt.Status != Intent.StatusSucceeded || sender.sent != 0 || receipt.Failure != nil && receipt.Failure.SafetyLock != nil {
				t.Fatalf("full replay status=%s sends=%d", receipt.Status, sender.sent)
			}
		}
		if !store.Snapshot().Automations["autoRecruit"].SafetyLock.Until.IsZero() {
			t.Fatal("local waiting took a lane safety lock")
		}
	}
	if State.AutomationRejectionWhitelisted("bup", 63) || State.AutomationSafetyLockDuration != 30*time.Minute {
		t.Fatal("BUP/63 safety backstop changed")
	}
	// The next snapshot unlocks the fifth slot: policy resumes without a permanent skip.
	raw, err := os.ReadFile("testdata/cit-117/bup-success-2.xt")
	if err != nil {
		t.Fatal(err)
	}
	unlocked := strings.Replace(string(raw), `"RUT":0,"VIP":0`, `"RUT":60,"VIP":0`, 1)
	if _, err := pipeline.HandleRawAt(t.Context(), strings.TrimSpace(unlocked), Protocol.DirectionInbound, now.Add(500*time.Millisecond)); err != nil {
		t.Fatal(err)
	}
	decision, err := Automation.NewRecruitPolicy().Evaluate(t.Context(), Automation.Snapshot{State: store.Snapshot(), Configuration: configuration, GameData: data, Now: now.Add(500 * time.Millisecond)})
	if err != nil || decision.Request == nil || decision.Request.Name != "production.enqueue" {
		t.Fatal("newly unlocked slot did not resume recruitment")
	}
}

func TestProductionQueueExpiryBetweenPlanningAndDispatchPreventsSend(t *testing.T) {
	now := time.Now().UTC()
	data := queueSlotsGameData(t)
	for _, test := range []struct {
		name     string
		slots    []State.QueueSlot
		capacity int
	}{
		{"VIP", []State.QueueSlot{{ExpiresAt: now.Add(time.Hour)}}, 1},
		{"effect beyond VIP capacity", []State.QueueSlot{{Permanent: true, Occupied: true}, {Permanent: true, Occupied: true}, {ExpiresAt: now.Add(time.Hour)}}, 3},
	} {
		t.Run(test.name, func(t *testing.T) {
			state := State.NewGameState()
			state.Castles[1] = State.CastleState{ID: 1, Focused: true, Production: map[int]State.ProductionQueue{0: {LineID: 0, Capacity: test.capacity, ObservedAt: now, Slots: test.slots}}}
			store := State.NewStore(&state)
			args := json.RawMessage(`{"castleId":1,"lineId":0,"definitionId":607,"amount":110,"fillAvailable":true}`)
			plan, err := planProductionEnqueue(t.Context(), Intent.PlanningContext{State: state, GameData: data}, args)
			if err != nil || countQueueSlotBUPs(plan) != 1 {
				t.Fatalf("expected one planned BUP: count=%d err=%v", countQueueSlotBUPs(plan), err)
			}
			app := &Application{State: store}
			if err := app.verifyProductionQueueCapacityAt(plan.Steps[0].ActionArguments, now); err != nil {
				t.Fatal(err)
			}
			sender := &queueSlotsSender{}
			receipt := executeQueueSlotPlan(t, store, data, sender, plan, func(ctx context.Context, args json.RawMessage) error {
				return app.verifyProductionQueueCapacityAt(args, now.Add(2*time.Hour))
			})
			if sender.sent != 0 || receipt.Status == Intent.StatusSucceeded || receipt.Failure != nil && receipt.Failure.SafetyLock != nil {
				t.Fatalf("expired slot: sends=%d status=%s", sender.sent, receipt.Status)
			}
			if err := app.verifyProductionQueueCapacityAt(plan.Steps[0].ActionArguments, now.Add(time.Hour)); !errors.Is(err, Intent.ErrPlanStale) {
				t.Fatal("expiry boundary did not invalidate plan")
			}
			if !store.Snapshot().Automations["autoRecruit"].SafetyLock.Until.IsZero() {
				t.Fatal("expiry guard took a lane safety lock")
			}
		})
	}
}

func TestProductionQueueLegacySnapshotBlocksPlanningAndDispatch(t *testing.T) {
	now := time.Now().UTC()
	state := State.NewGameState()
	state.Castles[1] = State.CastleState{ID: 1, Focused: true, Production: map[int]State.ProductionQueue{0: {LineID: 0, Capacity: 5, ObservedAt: now}}}
	args := json.RawMessage(`{"castleId":1,"lineId":0,"definitionId":607,"amount":110}`)
	if _, err := planProductionEnqueue(t.Context(), Intent.PlanningContext{State: state, GameData: queueSlotsGameData(t)}, args); !errors.Is(err, Intent.ErrPlanStale) {
		t.Fatalf("legacy planning error=%v", err)
	}
	guard, _ := json.Marshal(productionQueueCapacityGuard{CastleID: 1, LineID: 0, ExpectedFreeSlots: 1})
	app := &Application{State: State.NewStore(&state)}
	if err := app.verifyProductionQueueCapacityAt(guard, now); !errors.Is(err, Intent.ErrPlanStale) {
		t.Fatalf("legacy dispatch error=%v", err)
	}
}

func countQueueSlotBUPs(plan Intent.Plan) int {
	count := 0
	for _, step := range plan.Steps {
		if step.Command.Opcode == "bup" {
			count++
		}
	}
	return count
}
func queueSlotsGameData(t *testing.T) *GameData.Store {
	t.Helper()
	data, err := GameData.DecodeStore([]byte(`{"versionInfo":[],"buildings":[],"units":[{"wodID":607}],"constructionItems":[],"viplevels":[]}`), GameData.SourceMetadata{ItemVersion: "test"})
	if err != nil {
		t.Fatal(err)
	}
	return data
}

type queueSlotsData struct{ data *GameData.Store }

func (p queueSlotsData) Current() (*GameData.Store, bool) { return p.data, true }

type queueSlotsSender struct{ sent int }

func (*queueSlotsSender) Ready() bool       { return true }
func (*queueSlotsSender) Namespace() string { return "EmpireEx_1" }
func (s *queueSlotsSender) Send(context.Context, []byte) error {
	s.sent++
	return errors.New("unexpected send")
}
func executeQueueSlotPlan(t *testing.T, store *State.Store, data *GameData.Store, sender *queueSlotsSender, plan Intent.Plan, guard Intent.Action) Intent.Receipt {
	t.Helper()
	registry := Intent.NewRegistry()
	if err := registry.Register(Intent.Definition{Name: "production.enqueue", Planner: func(context.Context, Intent.PlanningContext, json.RawMessage) (Intent.Plan, error) { return plan, nil }}); err != nil {
		t.Fatal(err)
	}
	engine := Intent.NewEngine(registry, store, queueSlotsData{data}, sender, nil)
	if guard != nil {
		if err := engine.RegisterAction("production.enqueue.verify_capacity", guard); err != nil {
			t.Fatal(err)
		}
	}
	receipt := engine.Submit(t.Context(), Intent.Request{Name: "production.enqueue", Actor: "automation:autoRecruit", AutomationLane: "autoRecruit"})
	completed, err := engine.Await(t.Context(), receipt.ID)
	if err != nil {
		t.Fatal(err)
	}
	return completed
}
