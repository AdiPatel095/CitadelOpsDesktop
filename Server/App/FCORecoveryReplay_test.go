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

func TestQA128UnknownRefreshMustNotReplayFCO(t *testing.T) {
	state := boundaryTestState()
	step := boundaryTestStep("fco")
	args, _ := json.Marshal(buildingFinishFreeReconciliation{CastleID: dispatchTestCastle, BuildingInstanceID: dispatchTestObject, InitialConstructionState: State.BuildingStateUpgradeInProgress})
	step.RejectionReconciliation = &Intent.RejectionReconciliation{Code: 5, Refresh: castleFocusStep(state.Castles[dispatchTestCastle]), Action: "test.reconcile", Arguments: args}
	engine, store, sender := boundaryTestEngine(t, state, step, "", 5)
	app := &Application{State: store}
	if err := engine.RegisterAction("test.reconcile", app.reconcileBuildingFinishFree); err != nil {
		t.Fatal(err)
	}
	receipt := engine.Submit(t.Context(), Intent.Request{Name: "test.dispatch", Actor: "test"})
	t.Logf("unchanged upgrading state, empty refresh: sends=%v status=%s evidence=%d", sender.sends, receipt.Status, len(receipt.Evidence))
	if receipt.Status != Intent.StatusFailed || len(receipt.Evidence) != 1 {
		t.Fatalf("unknown recovery must fail with one rejection: status=%s evidence=%d", receipt.Status, len(receipt.Evidence))
	}
	if strings.Join(sender.sends, ",") != "fco,jaa" {
		t.Fatalf("unknown refresh blindly replayed FCO: %v; expected exactly fco,jaa", sender.sends)
	}
}
func TestQA128RealFinishPlannerUnknownRefreshMustNotReplay(t *testing.T) {
	state := buildingIntentState()
	castle := state.Castles[10]
	now := time.Now().UTC()
	castle.ContextSnapshotObservedAt = now
	castle.Layout.ObservedAt = now
	castle.BuildingQueue.ObservedAt = now
	building := castle.Buildings[42]
	building.ConstructionState = State.BuildingStateUpgradeInProgress
	castle.Buildings[42] = building
	castle.Layout.Objects[42] = building
	castle.BuildingQueue.Slots = []State.BuildingConstructionQueueSlot{{Status: State.BuildingQueueSlotOccupied, BuildingID: 42}}
	state.Castles[10] = castle
	store := State.NewStore(&state)
	store.ObserveProtocolFocus(State.FocusSubcontextCastle, now)
	reducers := Ingest.NewRegistry()
	if err := Ingest.RegisterCoreReducers(reducers); err != nil {
		t.Fatal(err)
	}
	pipeline := Ingest.NewPipeline(store, nil, reducers)
	sender := &qa128SparseSnapshotSender{pipeline: pipeline}
	registry := Intent.NewRegistry()
	if err := registry.Register(Intent.Definition{Name: "test.finish", Effect: Intent.EffectWrite, Planner: planBuildingFinishFree}); err != nil {
		t.Fatal(err)
	}
	data := buildingIntentGameData(t)
	engine := Intent.NewEngine(registry, store, coinGateStoreProvider{store: data}, sender, pipeline)
	engine.SetDispatchEvidenceCollector(captureDispatchBoundaryEvidence)
	if err := engine.RegisterStepResolver("building.finish_free.build", resolveBuildingFinishFreeStep); err != nil {
		t.Fatal(err)
	}
	// This closure calls the same validator as guardBuildingFinishFree with current
	// committed state and official synthetic catalog, rather than a permissive guard.
	if err := engine.RegisterAction("building.finish_free.guard", func(ctx context.Context, args json.RawMessage) error {
		var request buildingInstanceIntentRequest
		if err := decodeIntentArguments(args, &request); err != nil {
			return err
		}
		_, _, _, err := validatedBuildingFinishFree(Intent.PlanningContext{State: store.ReadOnlyView(), GameData: data}, request, true)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	app := &Application{State: store}
	if err := engine.RegisterAction("building.finish_free.reconcile", app.reconcileBuildingFinishFree); err != nil {
		t.Fatal(err)
	}
	receipt := engine.Submit(t.Context(), Intent.Request{Name: "test.finish", Actor: "test", Arguments: json.RawMessage(`{"castleId":10,"buildingInstanceId":42}`)})
	t.Logf("real planner/resolver/validation: sends=%v status=%s evidence=%d error=%s", sender.sends, receipt.Status, len(receipt.Evidence), receipt.DiagnosticError())
	if receipt.Status != Intent.StatusFailed || len(receipt.Evidence) != 1 {
		t.Fatalf("unknown recovery must fail with one rejection: status=%s evidence=%d", receipt.Status, len(receipt.Evidence))
	}
	if strings.Join(sender.sends, ",") != "fco,jaa" {
		t.Fatalf("real planner blindly replayed FCO after unknown refresh: %v; expected exactly fco,jaa", sender.sends)
	}
}

// Use production reducers and a valid sparse JAA response with castle identity,
// but no building layout or construction queue observation.
type qa128SparseSnapshotSender struct {
	pipeline *Ingest.Pipeline
	sends    []string
}

func (*qa128SparseSnapshotSender) Ready() bool       { return true }
func (*qa128SparseSnapshotSender) Namespace() string { return "EmpireEx_21" }
func (s *qa128SparseSnapshotSender) Send(ctx context.Context, payload []byte) error {
	if err := Outbound.ValidateFinalDispatch(ctx); err != nil {
		return err
	}
	command, err := Protocol.Decode(string(payload), Protocol.DirectionOutbound, time.Now().UTC())
	if err != nil {
		return err
	}
	s.sends = append(s.sends, command.Opcode)
	code, body := 5, `{}`
	if command.Opcode == "jaa" {
		code = 0
		body = `{"gca":{"A":[0,0,0,10]}}`
	}
	frame, err := Protocol.Decode(fmt.Sprintf("%%xt%%%s%%1%%%d%%%s%%", command.Opcode, code, body), Protocol.DirectionInbound, time.Now().UTC())
	if err != nil {
		return err
	}
	observed := s.pipeline.ObserveFrame(frame)
	go func() { _, _ = s.pipeline.CommitFrame(context.Background(), observed) }()
	return nil
}
