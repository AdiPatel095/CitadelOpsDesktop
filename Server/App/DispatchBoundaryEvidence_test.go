package App

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"CitadelDesktop/Server/Ingest"
	"CitadelDesktop/Server/Intent"
	"CitadelDesktop/Server/Outbound"
	"CitadelDesktop/Server/Protocol"
	"CitadelDesktop/Server/State"
)

// All identities in these fixtures are synthetic. Real payloads are not retained.
const dispatchTestCastle State.CastleID = 90123451
const dispatchTestObject State.BuildingInstanceID = 90123452
const dispatchTestJob int64 = 90123453

type boundaryTestSender struct {
	pipeline       *Ingest.Pipeline
	code           int
	sends          []string
	beforeResponse func(string) error
}

func (*boundaryTestSender) Ready() bool       { return true }
func (*boundaryTestSender) Namespace() string { return "EmpireEx_21" }
func (sender *boundaryTestSender) Send(ctx context.Context, payload []byte) error {
	if err := Outbound.ValidateFinalDispatch(ctx); err != nil {
		return err
	}
	command, err := Protocol.Decode(string(payload), Protocol.DirectionOutbound, time.Now().UTC())
	if err != nil {
		return err
	}
	sender.sends = append(sender.sends, command.Opcode)
	if sender.beforeResponse != nil {
		if err := sender.beforeResponse(command.Opcode); err != nil {
			return err
		}
	}
	code := sender.code
	if command.Opcode == "jaa" {
		code = 0
	}
	// Decode an actual server-shaped response, including the response-code slot.
	frame, err := Protocol.Decode(fmt.Sprintf("%%xt%%%s%%1%%%d%%{}%%", command.Opcode, code), Protocol.DirectionInbound, time.Now().UTC())
	if err != nil {
		return err
	}
	observed := sender.pipeline.ObserveFrame(frame)
	go func() { _, _ = sender.pipeline.CommitFrame(context.Background(), observed) }()
	return nil
}

func boundaryTestState() State.GameState {
	state := State.NewGameState()
	state.Player.ID = 90123454
	state.Player.Name = "synthetic-private-player"
	now := time.Now().UTC()
	job := State.QueueItem{Definition: State.DefinitionRef{Collection: "units", ID: 701}, Amount: 5, ProductionID: dispatchTestJob}
	building := State.Building{InstanceID: dispatchTestObject, DefinitionID: 301, GridX: 87654321, GridY: 87654322,
		ConstructionState: State.BuildingStateUpgradeInProgress,
		CompletionEvents:  []State.BuildingCompletionEvent{{Opcode: "fco", ConstructionState: State.BuildingStateUpgradeCompleted, ObservedAt: now.Add(-time.Minute)}}}
	state.Castles[dispatchTestCastle] = State.CastleState{ID: dispatchTestCastle, Name: "synthetic-private-castle", X: 87654323, Y: 87654324,
		Focused: true, ContextSnapshotObservedAt: now, Production: map[int]State.ProductionQueue{2: {LineID: 2, Active: &job, Queued: []State.QueueItem{}, Capacity: 3, ObservedAt: now}},
		Buildings: map[State.BuildingInstanceID]State.Building{dispatchTestObject: building},
		Layout:    State.CastleLayout{ObservedAt: now}, BuildingQueue: State.BuildingConstructionQueue{ObservedAt: now, Slots: []State.BuildingConstructionQueueSlot{{Status: State.BuildingQueueSlotOccupied, BuildingID: dispatchTestObject}}}}
	state.AllianceHelpRequests.HospitalProductionIDs = []int64{dispatchTestJob}
	state.AllianceHelpRequests.ObservedAt = now
	return state
}

func boundaryTestEngine(t *testing.T, state State.GameState, step Intent.Step, source string, code int, following ...Intent.Step) (*Intent.Engine, *State.Store, *boundaryTestSender) {
	t.Helper()
	store := State.NewStore(&state)
	store.ObserveProtocolFocus(State.FocusSubcontextCastle, time.Now().UTC())
	pipeline := Ingest.NewPipeline(store, nil, Ingest.NewRegistry())
	sender := &boundaryTestSender{pipeline: pipeline, code: code}
	registry := Intent.NewRegistry()
	planned := step
	if source != "" {
		planned = Intent.Step{Resolver: source, AwaitOpcode: step.AwaitOpcode, AwaitOpcodes: step.AwaitOpcodes, SuccessCodes: []int{0}}
	}
	if err := registry.Register(Intent.Definition{Name: "test.dispatch", Effect: Intent.EffectWrite, Planner: func(context.Context, Intent.PlanningContext, json.RawMessage) (Intent.Plan, error) {
		return Intent.Plan{Steps: append([]Intent.Step{planned}, following...)}, nil
	}}); err != nil {
		t.Fatal(err)
	}
	engine := Intent.NewEngine(registry, store, nil, sender, pipeline)
	engine.SetDispatchEvidenceCollector(captureDispatchBoundaryEvidence)
	if source != "" {
		if err := engine.RegisterStepResolver(source, func(context.Context, Intent.PlanningContext, json.RawMessage) (Intent.Step, error) { return step, nil }); err != nil {
			t.Fatal(err)
		}
	}
	if step.FinalDispatchAction != "" && step.FinalDispatchAction != "test.blocked" {
		if err := engine.RegisterAction(step.FinalDispatchAction, func(context.Context, json.RawMessage) error { return nil }); err != nil {
			t.Fatal(err)
		}
	}
	return engine, store, sender
}

func boundaryTestStep(opcode string) Intent.Step {
	payload := json.RawMessage(`{"U":701,"A":5}`)
	if opcode == "ahr" {
		payload = json.RawMessage(fmt.Sprintf(`{"ID":%d,"T":%d}`, dispatchTestJob, allianceHelpHospitalType))
	}
	if opcode == "fco" {
		payload = json.RawMessage(fmt.Sprintf(`{"OID":%d,"FS":1}`, dispatchTestObject))
	}
	step := commandStep("Synthetic dispatch", opcode, payload, opcode)
	step.FinalDispatchAction = "test.guard"
	step.FinalDispatchArguments = json.RawMessage(fmt.Sprintf(`{"castleId":%d}`, dispatchTestCastle))
	return step
}

func TestDispatchBoundaryRejectionReceipts(t *testing.T) {
	for _, pair := range []struct {
		opcode string
		code   int
		source string
		fields []string
	}{
		{"hru", 63, "", []string{"hospitalQueue", "capacity", "occupancy", "focus", "finalGuard", "source"}},
		{"hru", 63, "hospital.heal.build", []string{"hospitalQueue", "capacity", "occupancy", "focus", "finalGuard", "source"}},
		{"ahr", 2, "alliance.help.build", []string{"hospitalJob", "jobIdentity", "listIdentity", "helpRequests", "helpObservedAt", "helpGeneration", "castle", "session"}},
		{"fco", 5, "building.finish_free.build", []string{"target", "targetVersion", "completionEvents", "constructionIdentity"}},
	} {
		t.Run(pair.opcode, func(t *testing.T) {
			step := boundaryTestStep(pair.opcode)
			// Unknown command fields fail closed, including names and coordinates.
			var payload map[string]any
			_ = json.Unmarshal(step.Command.Payload, &payload)
			payload["playerId"] = 90123454
			payload["castleName"] = "synthetic-private-castle"
			payload["PX"] = 87654323
			step.Command.Payload, _ = json.Marshal(payload)
			step.Payload = step.Command.Payload
			engine, store, sender := boundaryTestEngine(t, boundaryTestState(), step, pair.source, pair.code)
			// A concurrent commit after dispatch must not change the retained snapshot.
			sender.beforeResponse = func(string) error {
				_, err := store.Apply(func(state *State.GameState) ([]string, bool, error) {
					castle := state.Castles[dispatchTestCastle]
					q := castle.Production[2]
					q.Capacity = 9
					castle.Production[2] = q
					state.SetCastle(dispatchTestCastle, castle)
					return []string{"castles"}, true, nil
				})
				return err
			}
			receipt := engine.Submit(t.Context(), Intent.Request{Name: "test.dispatch", Actor: "test"})
			if receipt.Status != Intent.StatusFailed || len(receipt.Evidence) != 1 || receipt.Evidence[0].Kind != "dispatch_boundary" {
				t.Fatalf("receipt: %+v", receipt)
			}
			raw := string(receipt.Evidence[0].Data)
			for _, forbidden := range []string{"90123451", "90123452", "90123453", "90123454", "87654321", "87654322", "87654323", "87654324", "synthetic-private-player", "synthetic-private-castle"} {
				if strings.Contains(raw, forbidden) {
					t.Fatalf("private fixture value retained: %s", forbidden)
				}
			}
			var evidence map[string]json.RawMessage
			if err := json.Unmarshal(receipt.Evidence[0].Data, &evidence); err != nil {
				t.Fatal(err)
			}
			for _, field := range append(pair.fields, "command", "emitter", "dispatchAt", "castleObservedAt", "stateRevision", "correlationIdentity") {
				if len(evidence[field]) == 0 || string(evidence[field]) == "null" {
					t.Fatalf("missing %s: %s", field, raw)
				}
			}
			if pair.opcode == "hru" && string(evidence["capacity"]) != "3" {
				t.Fatalf("snapshot changed after dispatch: %s", raw)
			}
			var command string
			_ = json.Unmarshal(evidence["command"], &command)
			decoded, err := Protocol.Decode(command, Protocol.DirectionOutbound, time.Now())
			if err != nil || decoded.Opcode != pair.opcode {
				t.Fatalf("final serialized command: %s %v", command, err)
			}
		})
	}
}

func TestDispatchBoundaryOnlyTargetedRejections(t *testing.T) {
	for _, pair := range []struct {
		opcode string
		code   int
	}{{"hru", 0}, {"ahr", 0}, {"fco", 0}, {"hru", 88}, {"ahr", 114}, {"fco", 4}} {
		t.Run(fmt.Sprintf("%s/%d", pair.opcode, pair.code), func(t *testing.T) {
			engine, _, _ := boundaryTestEngine(t, boundaryTestState(), boundaryTestStep(pair.opcode), "", pair.code)
			receipt := engine.Submit(t.Context(), Intent.Request{Name: "test.dispatch", Actor: "test"})
			if len(receipt.Evidence) != 0 {
				t.Fatalf("non-targeted outcome retained evidence: %+v", receipt.Evidence)
			}
			if pair.code == 0 && receipt.Status != Intent.StatusSucceeded {
				t.Fatalf("success failed: %+v", receipt)
			}
		})
	}
	// The existing recruitment AHR records are not dispatch hospital evidence.
	step := boundaryTestStep("ahr")
	step.Command.Payload = json.RawMessage(`{"ID":-1,"T":0}`)
	step.Payload = step.Command.Payload
	engine, _, _ := boundaryTestEngine(t, boundaryTestState(), step, "", 2)
	if receipt := engine.Submit(t.Context(), Intent.Request{Name: "test.dispatch", Actor: "test"}); len(receipt.Evidence) != 0 {
		t.Fatal("recruitment retained hospital evidence")
	}
}

func TestFCORejectionRefreshesWithoutBlindResend(t *testing.T) {
	for _, outcome := range []string{"upgrading", "completed", "unknown", "stale", "refresh_rejected", "missing_object", "session_changed"} {
		t.Run(outcome, func(t *testing.T) {
			state := boundaryTestState()
			step := boundaryTestStep("fco")
			args, _ := json.Marshal(buildingFinishFreeReconciliation{CastleID: dispatchTestCastle, BuildingInstanceID: dispatchTestObject, SnapshotAfter: time.Now().UTC(), InitialConstructionState: State.BuildingStateUpgradeInProgress})
			step.RejectionReconciliation = &Intent.RejectionReconciliation{Code: 5, Refresh: castleFocusStep(state.Castles[dispatchTestCastle]), Action: "test.reconcile", Arguments: args}
			engine, store, sender := boundaryTestEngine(t, state, step, "", 5, Intent.Step{Action: "test.after"})
			if err := engine.RegisterAction("test.after", func(context.Context, json.RawMessage) error { return fmt.Errorf("retired operation continued") }); err != nil {
				t.Fatal(err)
			}
			app := &Application{State: store}
			if err := engine.RegisterAction("test.reconcile", app.reconcileBuildingFinishFree); err != nil {
				t.Fatal(err)
			}
			sender.beforeResponse = func(opcode string) error {
				if opcode != "jaa" {
					return nil
				}
				if outcome == "refresh_rejected" {
					return fmt.Errorf("synthetic refresh transport failure")
				}
				_, err := store.Apply(func(state *State.GameState) ([]string, bool, error) {
					castle := state.Castles[dispatchTestCastle]
					if outcome != "stale" {
						now := time.Now().UTC()
						castle.ContextSnapshotObservedAt = now
						castle.Layout.ObservedAt = now
						castle.BuildingQueue.ObservedAt = now
					}
					if outcome == "completed" || outcome == "unknown" {
						building := castle.Buildings[dispatchTestObject]
						building.ConstructionState = State.BuildingStateUpgradeCompleted
						if outcome == "unknown" {
							building.ConstructionState = State.BuildingStateWaitingForServer
						}
						castle.Buildings[dispatchTestObject] = building
						castle.BuildingQueue.Slots = nil
					}
					if outcome == "missing_object" {
						delete(castle.Buildings, dispatchTestObject)
						castle.BuildingQueue.Slots = nil
					}
					if outcome == "session_changed" {
						state.Session.Generation++
					}
					state.SetCastle(dispatchTestCastle, castle)
					return []string{"castles"}, true, nil
				})
				return err
			}
			receipt := engine.Submit(t.Context(), Intent.Request{Name: "test.dispatch", Actor: "automation:build", AutomationLane: "build"})
			want := Intent.StatusFailed
			if outcome == "completed" {
				want = Intent.StatusSucceeded
			}
			if receipt.Status != want {
				t.Fatalf("status %s, want %s: %+v", receipt.Status, want, receipt)
			}
			if strings.Join(sender.sends, ",") != "fco,jaa" {
				t.Fatalf("blind resend or missing refresh: %v", sender.sends)
			}
			if len(receipt.Evidence) != 1 {
				t.Fatalf("missing FCO evidence: %+v", receipt)
			}
			if lock := engine.AutomationLaneLock("build"); !lock.Active(time.Now()) || lock.Code != 5 {
				t.Fatalf("lane backstop changed: %+v", lock)
			}
		})
	}
}

func TestFCOResolverSchedulesCommittedReconciliation(t *testing.T) {
	data := buildingIntentGameData(t)
	state := buildingIntentState()
	castle := state.Castles[10]
	building := castle.Buildings[42]
	building.ConstructionState = State.BuildingStateUpgradeInProgress
	castle.Buildings[42] = building
	castle.Layout.Objects[42] = building
	castle.BuildingQueue.Slots = []State.BuildingConstructionQueueSlot{{Status: State.BuildingQueueSlotOccupied, BuildingID: 42}}
	state.Castles[10] = castle
	input := Intent.PlanningContext{State: state, GameData: data}
	args := json.RawMessage(`{"castleId":10,"buildingInstanceId":42}`)
	plan, err := planBuildingFinishFree(t.Context(), input, args)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Steps) == 0 || plan.Steps[0].Opcode != "jaa" || plan.Steps[0].ResponseBarrier != Intent.ResponseBarrierCommitted {
		t.Fatalf("FCO plan does not refresh first: %+v", plan)
	}
	step, err := resolveBuildingFinishFreeStep(t.Context(), input, args)
	if err != nil {
		t.Fatal(err)
	}
	recovery := step.RejectionReconciliation
	if recovery == nil || recovery.Code != 5 || recovery.Refresh.Opcode != "jaa" || recovery.Refresh.ResponseBarrier != Intent.ResponseBarrierCommitted || recovery.Action != "building.finish_free.reconcile" {
		t.Fatalf("missing FCO rejection refresh: %+v", step)
	}
	if step.ResponseRetry != nil || step.FinalDispatchAction != "building.finish_free.guard" {
		t.Fatal("blind retry or changed final guard")
	}
}

func TestDispatchBoundaryBlockedFinalGuardCapturesNothing(t *testing.T) {
	step := boundaryTestStep("hru")
	step.FinalDispatchAction = "test.blocked"
	engine, _, sender := boundaryTestEngine(t, boundaryTestState(), step, "", 63)
	// A locally blocked final guard never reaches the transport or a rejection.
	if err := engine.RegisterAction("test.blocked", func(context.Context, json.RawMessage) error { return Intent.ErrPlanStale }); err != nil {
		t.Fatal(err)
	}
	receipt := engine.Submit(t.Context(), Intent.Request{Name: "test.dispatch", Actor: "test"})
	if receipt.Status != Intent.StatusFailed || len(receipt.Evidence) != 0 || len(sender.sends) != 0 {
		t.Fatalf("blocked send captured rejection evidence: %+v sends %v", receipt, sender.sends)
	}
}
