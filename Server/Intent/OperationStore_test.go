package Intent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"CitadelDesktop/Server/Protocol"
	"CitadelDesktop/Server/State"
)

type operationStoreSender struct {
	sends atomic.Int32
}

func (*operationStoreSender) Ready() bool       { return true }
func (*operationStoreSender) Namespace() string { return "EmpireEx_21" }
func (sender *operationStoreSender) Send(context.Context, []byte) error {
	sender.sends.Add(1)
	return nil
}

func TestOperationStoreRecoversPossiblyDispatchedWriteWithoutReplay(t *testing.T) {
	dataDir := t.TempDir()
	store, err := OpenOperationStore(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	receipt := Receipt{
		ID: "recover-write", Intent: "test.write", Actor: "api", Status: StatusRunning,
		Phase: EffectPhaseDispatching, SubmittedAt: time.Now().UTC(),
		Plan: &Plan{Intent: "test.write", Effect: EffectWrite},
	}
	if _, created, err := store.Reserve(t.Context(), "same-request", receipt); err != nil || !created {
		t.Fatalf("reserve operation: created=%t err=%v", created, err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	store, err = OpenOperationStore(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	recovered, err := store.Recover(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if len(recovered) != 1 {
		t.Fatalf("recovered operations = %d, want 1", len(recovered))
	}
	if recovered[0].Receipt.Status != StatusIndeterminate ||
		recovered[0].Receipt.Phase != EffectPhaseReconciliationRequired {
		t.Fatalf("recovered receipt = %#v", recovered[0].Receipt)
	}
	existing, created, err := store.Reserve(t.Context(), "same-request", receipt)
	if err != nil || created {
		t.Fatalf("reserve recovered operation: created=%t err=%v", created, err)
	}
	if existing.Status != StatusIndeterminate {
		t.Fatalf("idempotent replay returned status %q", existing.Status)
	}
	if _, _, err := store.Reserve(t.Context(), "different-request", receipt); !errors.Is(err, ErrIdempotencyConflict) {
		t.Fatalf("different request error = %v", err)
	}
}

func TestOperationStoreRecoversUndispatchedPlanningAsFailed(t *testing.T) {
	store, err := OpenOperationStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	receipt := Receipt{
		ID: "recover-planning", Intent: "test.write", Actor: "api", Status: StatusPlanning,
		Phase: EffectPhaseAccepted, SubmittedAt: time.Now().UTC(),
	}
	if _, _, err := store.Reserve(t.Context(), "request", receipt); err != nil {
		t.Fatal(err)
	}
	recovered, err := store.Recover(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if len(recovered) != 1 || recovered[0].Receipt.Status != StatusFailed {
		t.Fatalf("recovered receipt = %#v", recovered)
	}
}

func TestEngineUsesDurableOperationIDAsIdempotencyKey(t *testing.T) {
	store, err := OpenOperationStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	registry := NewRegistry()
	if err := registry.Register(Definition{
		Name: "test.idempotent", Effect: EffectWrite,
		Planner: func(context.Context, PlanningContext, json.RawMessage) (Plan, error) {
			return Plan{Steps: []Step{{
				Opcode: "ain", Command: Protocol.Command{Opcode: "ain", Payload: json.RawMessage(`{}`)},
			}}}, nil
		},
	}); err != nil {
		t.Fatal(err)
	}
	sender := &operationStoreSender{}
	engine := NewEngine(registry, State.NewStore(State.NewGameState()), nil, sender, nil)
	if err := engine.SetOperationStore(t.Context(), store); err != nil {
		t.Fatal(err)
	}
	request := Request{ID: "stable-operation", Name: "test.idempotent", Arguments: json.RawMessage(`{"value":1}`)}
	first := engine.Submit(t.Context(), request)
	if first.Status != StatusSucceeded || first.Phase != EffectPhaseCompleted || first.Attempt != 1 {
		t.Fatalf("first receipt = %#v", first)
	}
	second := engine.Submit(t.Context(), request)
	if second.Status != StatusSucceeded || sender.sends.Load() != 1 {
		t.Fatalf("idempotent receipt = %#v, sends = %d", second, sender.sends.Load())
	}
	request.Arguments = json.RawMessage(`{"value":2}`)
	conflict := engine.Submit(t.Context(), request)
	if conflict.Status != StatusFailed || !strings.Contains(conflict.Error, ErrIdempotencyConflict.Error()) {
		t.Fatalf("idempotency conflict = %#v", conflict)
	}
	if sender.sends.Load() != 1 {
		t.Fatalf("physical sends after conflict = %d", sender.sends.Load())
	}
}

func BenchmarkOperationStoreReserveAndDispatch(b *testing.B) {
	store, err := OpenOperationStore(b.TempDir())
	if err != nil {
		b.Fatal(err)
	}
	defer store.Close()
	b.ReportAllocs()
	b.ResetTimer()
	for index := 0; b.Loop(); index++ {
		receipt := Receipt{
			ID: fmt.Sprintf("benchmark-%d", index), Intent: "benchmark.write", Actor: "api",
			Status: StatusPlanning, Phase: EffectPhaseAccepted, SubmittedAt: time.Now().UTC(),
		}
		if _, _, err := store.Reserve(context.Background(), fmt.Sprintf("request-%d", index), receipt); err != nil {
			b.Fatal(err)
		}
		receipt.Status = StatusRunning
		receipt.Phase = EffectPhaseDispatching
		if err := store.Save(context.Background(), receipt); err != nil {
			b.Fatal(err)
		}
	}
}
