package App

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"CitadelDesktop/Server/GameData"
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
	reduceRefresh  func(Protocol.Frame, *State.GameState) ([]string, bool, error)
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
	reducers := Ingest.NewRegistry()
	sender := &boundaryTestSender{code: code}
	if err := reducers.Register("jaa", func(_ context.Context, frame Protocol.Frame, state *State.GameState, _ *GameData.Store) ([]string, bool, error) {
		if sender.reduceRefresh != nil {
			return sender.reduceRefresh(frame, state)
		}
		return nil, false, nil
	}); err != nil {
		t.Fatal(err)
	}
	pipeline := Ingest.NewPipeline(store, nil, reducers)
	sender.pipeline = pipeline
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
	engine.SetFinalDispatchProvider(newPremiumCommanderDispatchGate())
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
	for _, lane := range []string{"", "build"} {
		t.Run("lane="+lane, func(t *testing.T) {
			for _, outcome := range []string{"upgrading", "completed", "unknown", "stale", "refresh_rejected", "missing_object", "session_changed", "connection_changed", "missing_layout", "missing_queue", "other_snapshot"} {
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
						if opcode == "jaa" && outcome == "refresh_rejected" {
							return fmt.Errorf("synthetic refresh transport failure")
						}
						return nil
					}
					sender.reduceRefresh = func(frame Protocol.Frame, state *State.GameState) ([]string, bool, error) {
						castle := state.Castles[dispatchTestCastle]
						if outcome != "stale" {
							now := frame.ReceivedAt
							if outcome == "other_snapshot" {
								now = now.Add(time.Nanosecond)
							}
							castle.ContextSnapshotObservedAt = now
							if outcome != "missing_layout" {
								castle.Layout.ObservedAt = now
							}
							if outcome != "missing_queue" {
								castle.BuildingQueue.ObservedAt = now
							}
						}
						if outcome == "completed" || outcome == "unknown" || outcome == "missing_layout" || outcome == "missing_queue" || outcome == "other_snapshot" || outcome == "connection_changed" {
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
						if outcome == "connection_changed" {
							state.Session.ConnectionGeneration++
						}
						state.SetCastle(dispatchTestCastle, castle)
						return []string{"castles"}, true, nil
					}

					actor := "test"
					if lane != "" {
						actor = "automation:" + lane
					}
					receipt := engine.Submit(t.Context(), Intent.Request{Name: "test.dispatch", Actor: actor, AutomationLane: lane})
					want := Intent.StatusFailed
					if outcome == "completed" {
						want = Intent.StatusSucceeded
					}
					if receipt.Status != want {
						t.Fatalf("status %s, want %s: %s", receipt.Status, want, receipt.Error)
					}
					if strings.Join(sender.sends, ",") != "fco,jaa" {
						t.Fatalf("blind resend or missing refresh: %v", sender.sends)
					}
					if len(receipt.Evidence) != 1 {
						t.Fatalf("missing FCO evidence: %+v", receipt)
					}
					if lock := engine.AutomationLaneLock("build"); lane != "" && (!lock.Active(time.Now()) || lock.Code != 5) {
						t.Fatalf("lane backstop changed: %+v", lock)
					}
				})
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
	if len(plan.Steps) == 0 || plan.Steps[0].Resolver != "building.finish_free.build" {
		t.Fatalf("ordinary FCO plan gained a refresh: %+v", plan)
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

func TestQA128NoExtraHappyPathRefresh(t *testing.T) {
	state := buildingIntentState()
	castle := state.Castles[10]
	castle.Focused = true
	building := castle.Buildings[42]
	building.ConstructionState = State.BuildingStateUpgradeInProgress
	castle.Buildings[42] = building
	castle.Layout.Objects[42] = building
	castle.BuildingQueue.Slots = []State.BuildingConstructionQueueSlot{{Status: State.BuildingQueueSlotOccupied, BuildingID: 42}}
	state.Castles[10] = castle
	input := Intent.PlanningContext{State: state, GameData: buildingIntentGameData(t), ProtocolContext: State.ProtocolContextState{FocusedCastleID: 10, FocusSubcontext: State.FocusSubcontextCastle}}
	plan, err := planBuildingFinishFree(t.Context(), input, json.RawMessage(`{"castleId":10,"buildingInstanceId":42}`))
	if err != nil {
		t.Fatal(err)
	}
	if plan.Steps[0].Resolver != "building.finish_free.build" {
		t.Fatalf("already focused, no prior rejection: first step opcode=%q resolver=%q; expected original resolver first, no added JAA", plan.Steps[0].Opcode, plan.Steps[0].Resolver)
	}
}
func TestQA128RefreshMustCommitAfterRejection(t *testing.T) {
	state := boundaryTestState()
	step := boundaryTestStep("fco")
	args, _ := json.Marshal(buildingFinishFreeReconciliation{CastleID: dispatchTestCastle, BuildingInstanceID: dispatchTestObject, SnapshotAfter: time.Now().UTC(), InitialConstructionState: State.BuildingStateUpgradeInProgress})
	step.RejectionReconciliation = &Intent.RejectionReconciliation{Code: 5, Refresh: castleFocusStep(state.Castles[dispatchTestCastle]), Action: "test.reconcile", Arguments: args}
	engine, store, sender := boundaryTestEngine(t, state, step, "", 5)
	app := &Application{State: store}
	if err := engine.RegisterAction("test.reconcile", app.reconcileBuildingFinishFree); err != nil {
		t.Fatal(err)
	}
	// Commit a completion before the FCO rejection. The subsequent JAA is an empty
	// response and does not commit a new castle snapshot (registry has no reducers).
	sender.beforeResponse = func(opcode string) error {
		if opcode != "fco" {
			return nil
		}
		_, err := store.Apply(func(state *State.GameState) ([]string, bool, error) {
			castle := state.Castles[dispatchTestCastle]
			now := time.Now().UTC()
			castle.ContextSnapshotObservedAt = now
			castle.Layout.ObservedAt = now
			castle.BuildingQueue.ObservedAt = now
			building := castle.Buildings[dispatchTestObject]
			building.ConstructionState = State.BuildingStateUpgradeCompleted
			castle.Buildings[dispatchTestObject] = building
			castle.BuildingQueue.Slots = nil
			state.SetCastle(dispatchTestCastle, castle)
			return []string{"castles"}, true, nil
		})
		return err
	}
	receipt := engine.Submit(t.Context(), Intent.Request{Name: "test.dispatch", Actor: "test"})
	t.Logf("sends=%v status=%s evidence=%d", sender.sends, receipt.Status, len(receipt.Evidence))
	if strings.Join(sender.sends, ",") != "fco,jaa" {
		t.Fatalf("blind resend after empty refresh: %v", sender.sends)
	}
	if receipt.Status != Intent.StatusFailed {
		t.Fatalf("retired from pre-rejection snapshot despite empty JAA: actual=%s expected=failed without fresh committed rejection refresh", receipt.Status)
	}
}
func TestQA128EvidenceBoundedSize(t *testing.T) {
	for _, count := range []int{1000, 10000} {
		state := boundaryTestState()
		castle := state.Castles[dispatchTestCastle]
		queue := castle.Production[2]
		queue.Queued = make([]State.QueueItem, count)
		for i := range queue.Queued {
			queue.Queued[i] = State.QueueItem{ProductionID: int64(i + 100000), Amount: 1}
		}
		castle.Production[2] = queue
		state.Castles[dispatchTestCastle] = castle
		engine, _, _ := boundaryTestEngine(t, state, boundaryTestStep("hru"), "", 63)
		receipt := engine.Submit(t.Context(), Intent.Request{Name: "test.dispatch", Actor: "test"})
		if len(receipt.Evidence) != 1 {
			t.Fatal("missing evidence")
		}
		raw := receipt.Evidence[0].Data
		if len(raw) > dispatchEvidenceMaxBytes {
			t.Fatalf("evidence exceeds byte budget: %d", len(raw))
		}
		var fields map[string]json.RawMessage
		_ = json.Unmarshal(raw, &fields)
		var q struct {
			Queued []json.RawMessage `json:"queued"`
		}
		_ = json.Unmarshal(fields["hospitalQueue"], &q)
		t.Logf("input queued=%d retained queued=%d evidence bytes=%d", count, len(q.Queued), len(raw))
		if count == 10000 && len(q.Queued) == count {
			t.Fatalf("all 10000 state entries retained (%d bytes); no diagnostic item/byte bound", len(raw))
		}
	}
}

func TestDispatchBoundaryEvidenceItemBudget(t *testing.T) {
	for _, count := range []int{dispatchEvidenceMaxItems - 1, dispatchEvidenceMaxItems, dispatchEvidenceMaxItems + 1, 10000} {
		for _, opcode := range []string{"hru", "ahr", "fco"} {
			t.Run(fmt.Sprintf("%s/%d", opcode, count), func(t *testing.T) {
				state := boundaryTestState()
				castle := state.Castles[dispatchTestCastle]
				queue := castle.Production[2]
				queue.Queued = make([]State.QueueItem, count)
				state.AllianceHelpRequests.HospitalProductionIDs = make([]int64, count)
				building := castle.Buildings[dispatchTestObject]
				building.CompletionEvents = make([]State.BuildingCompletionEvent, count)
				for i := range count {
					queue.Queued[i] = State.QueueItem{ProductionID: dispatchTestJob, Amount: 1}
					state.AllianceHelpRequests.HospitalProductionIDs[i] = dispatchTestJob
					building.CompletionEvents[i] = State.BuildingCompletionEvent{Opcode: "synthetic-private-player", ObservedAt: time.Now().UTC()}
				}
				castle.Production[2] = queue
				castle.Buildings[dispatchTestObject] = building
				state.Castles[dispatchTestCastle] = castle
				step := boundaryTestStep(opcode)
				var payload map[string]any
				_ = json.Unmarshal(step.Command.Payload, &payload)
				for i := range 1000 {
					payload[fmt.Sprintf("synthetic-private-player-%d", i)] = "synthetic-private-castle"
				}
				step.Command.Payload, _ = json.Marshal(payload)
				step.Payload = step.Command.Payload
				code := map[string]int{"hru": 63, "ahr": 2, "fco": 5}[opcode]
				engine, _, _ := boundaryTestEngine(t, state, step, "", code)
				receipt := engine.Submit(t.Context(), Intent.Request{Name: "test.dispatch", Actor: "test"})
				if len(receipt.Evidence) != 1 {
					t.Fatal("missing bounded evidence")
				}
				raw := receipt.Evidence[0].Data
				if len(raw) > dispatchEvidenceMaxBytes || strings.Contains(string(raw), "synthetic-private") || strings.Contains(string(raw), fmt.Sprint(dispatchTestJob)) {
					t.Fatal("evidence exceeds size or privacy budget")
				}
				var fields map[string]json.RawMessage
				if err := json.Unmarshal(raw, &fields); err != nil {
					t.Fatal(err)
				}
				listField, countField, truncatedField := "helpRequests", "helpRequestCount", "helpRequestsTruncated"
				if opcode == "hru" {
					if err := json.Unmarshal(fields["hospitalQueue"], &fields); err != nil {
						t.Fatal(err)
					}
					listField, countField, truncatedField = "queued", "queuedCount", "queuedTruncated"
				} else if opcode == "fco" {
					listField, countField, truncatedField = "completionEvents", "completionEventCount", "completionEventsTruncated"
				}
				var list []json.RawMessage
				if err := json.Unmarshal(fields[listField], &list); err != nil {
					t.Fatal(err)
				}
				if len(list) != min(count, dispatchEvidenceMaxItems) || string(fields[countField]) != fmt.Sprint(count) || string(fields[truncatedField]) != fmt.Sprint(count > dispatchEvidenceMaxItems) {
					t.Fatal("incorrect retained count or truncation metadata")
				}
			})
		}
	}
}

func TestDispatchBoundaryEvidenceByteBudget(t *testing.T) {
	for _, delta := range []int{-1, 0, 1} {
		t.Run(fmt.Sprint(delta), func(t *testing.T) {
			queue := map[string]any{"queued": []string{""}, "queuedCount": 1, "queuedTruncated": false}
			snapshot := map[string]any{"command": "<redacted>", "hospitalQueue": queue, "maxBytes": dispatchEvidenceMaxBytes}
			base, _ := json.Marshal(snapshot)
			queue["queued"] = []string{strings.Repeat("x", dispatchEvidenceMaxBytes-len(base)+delta)}
			raw, ok := boundedDispatchSnapshot(snapshot).(json.RawMessage)
			if !ok || len(raw) > dispatchEvidenceMaxBytes || !json.Valid(raw) {
				t.Fatal("invalid or oversized bounded evidence")
			}
			if delta <= 0 && len(raw) != dispatchEvidenceMaxBytes+delta {
				t.Fatalf("unexpected truncation at byte boundary: %d", len(raw))
			}
			if delta > 0 && (snapshot["truncated"] != true || queue["queuedTruncated"] != true) {
				t.Fatal("missing byte truncation metadata")
			}
		})
	}
}
