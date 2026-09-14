package Intent

import (
	"context"
	"encoding/json"
	"fmt"
	"sync/atomic"
	"testing"

	"CitadelDesktop/Server/Ingest"
	"CitadelDesktop/Server/Outbound"
	"CitadelDesktop/Server/Protocol"
	"CitadelDesktop/Server/State"
)

func supportBatchEngine(t *testing.T, codes []int) (*Engine, *responseSequenceSender, *atomic.Int32, *atomic.Int32) {
	t.Helper()
	store := State.NewStore(State.NewGameState())
	pipeline := Ingest.NewPipeline(store, nil, Ingest.NewRegistry())
	sender := &responseSequenceSender{pipeline: pipeline, responseCodes: codes}
	registry := NewRegistry()
	if err := registry.Register(Definition{Name: "test.batches", Effect: EffectWrite, Planner: func(context.Context, PlanningContext, json.RawMessage) (Plan, error) {
		return Plan{Steps: []Step{{Resolver: "test.batch.build", AwaitOpcode: "cds"}}}, nil
	}}); err != nil {
		t.Fatal(err)
	}
	engine := NewEngine(registry, store, nil, sender, pipeline)
	builds := &atomic.Int32{}
	captures := &atomic.Int32{}
	if err := engine.RegisterAction("test.batch.capture", func(context.Context, json.RawMessage) error { captures.Add(1); return nil }); err != nil {
		t.Fatal(err)
	}
	if err := engine.RegisterStepResolver("test.batch.build", func(context.Context, PlanningContext, json.RawMessage) (Step, error) {
		builds.Add(1)
		steps := []Step{}
		for i := 1; i <= 3; i++ {
			payload := json.RawMessage(fmt.Sprintf(`{"A":[[%d,100]]}`, i))
			steps = append(steps, Step{Name: fmt.Sprintf("Batch %d", i), Opcode: "cds", AwaitOpcode: "cds", TimeoutMillis: 1000, SuccessCodes: []int{0}, CaptureResponse: true, ResponseBarrier: ResponseBarrierCommitted, Command: Protocol.Command{Opcode: "cds", Payload: payload}}, Step{Action: "test.batch.capture"})
		}
		return Step{Batch: steps}, nil
	}); err != nil {
		t.Fatal(err)
	}
	return engine, sender, builds, captures
}

func TestResolvedBatchesStopOnRejectedSecondCommand(t *testing.T) {
	engine, sender, builds, captures := supportBatchEngine(t, []int{0, 101})
	receipt := engine.Submit(t.Context(), Request{Name: "test.batches"})
	sends, _ := sender.snapshot()
	if sends != 2 || builds.Load() != 1 || captures.Load() != 1 || receipt.Status == StatusSucceeded || len(receipt.Exchanges) != 2 {
		t.Fatalf("partial batch was replayed or continued: sends=%d captures=%d receipt=%+v", sends, captures.Load(), receipt)
	}
	if receipt.Plan == nil || len(receipt.Plan.Steps) != 6 || len(receipt.CompletedStepIndexes) != 2 {
		t.Fatalf("expanded plan/progress missing: %+v", receipt)
	}
}

func TestResolvedBatchesResumeWithoutRebuildingOrReplaying(t *testing.T) {
	engine, sender, builds, captures := supportBatchEngine(t, []int{0, 0, 0})
	resume := make(chan struct{})
	var paused atomic.Bool
	engine.SetExecutionGate(func(ctx context.Context, _ Request, _ Plan, point ExecutionPoint) error {
		if point == ExecutionBeforeClaims && paused.Load() {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-resume:
				return nil
			}
		}
		sends, _ := sender.snapshot()
		if point == ExecutionBeforeStep && sends == 1 && paused.CompareAndSwap(false, true) {
			return Outbound.ErrAutomationLocked
		}
		return nil
	})
	updates, unsubscribe := engine.Subscribe(64)
	defer unsubscribe()
	done := make(chan Receipt, 1)
	go func() { done <- engine.Submit(t.Context(), Request{Name: "test.batches"}) }()
	waitForIntentStatus(t, updates, StatusPaused)
	close(resume)
	receipt := <-done
	sends, _ := sender.snapshot()
	if receipt.Status != StatusSucceeded || sends != 3 || builds.Load() != 1 || captures.Load() != 3 || len(receipt.Exchanges) != 3 {
		t.Fatalf("resume replay/loss: sends=%d builds=%d captures=%d receipt=%+v", sends, builds.Load(), captures.Load(), receipt)
	}
}

type missingSecondBatchResponseSender struct {
	delegate *responseSequenceSender
	sends    atomic.Int32
}

func (*missingSecondBatchResponseSender) Ready() bool       { return true }
func (*missingSecondBatchResponseSender) Namespace() string { return "EmpireEx_21" }
func (sender *missingSecondBatchResponseSender) Send(ctx context.Context, payload []byte) error {
	if sender.sends.Add(1) == 2 {
		return nil
	} // transported, but no acknowledgement
	return sender.delegate.Send(ctx, payload)
}
func TestResolvedBatchesStopOnUnacknowledgedSecondCommand(t *testing.T) {
	engine, delegate, builds, captures := supportBatchEngine(t, []int{0})
	sender := &missingSecondBatchResponseSender{delegate: delegate}
	engine.sender = sender
	receipt := engine.Submit(t.Context(), Request{Name: "test.batches"})
	if receipt.Status != StatusIndeterminate || sender.sends.Load() != 2 || builds.Load() != 1 || captures.Load() != 1 {
		t.Fatalf("uncertain batch continued/replayed: sends=%d status=%s", sender.sends.Load(), receipt.Status)
	}
}
