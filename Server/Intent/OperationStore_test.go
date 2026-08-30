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

type countingOperationStore struct {
	OperationStore
	reserves    atomic.Int32
	saves       atomic.Int32
	lastReserve atomic.Pointer[Receipt]
}

func (store *countingOperationStore) Reserve(
	ctx context.Context,
	requestHash string,
	receipt Receipt,
) (Receipt, bool, error) {
	store.reserves.Add(1)
	copy := receipt
	store.lastReserve.Store(&copy)
	return store.OperationStore.Reserve(ctx, requestHash, receipt)
}

func (store *countingOperationStore) Save(ctx context.Context, receipt Receipt) error {
	store.saves.Add(1)
	return store.OperationStore.Save(ctx, receipt)
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

func TestEngineKeepsReadsInMemoryAndCheckpointsWrites(t *testing.T) {
	sqliteStore, err := OpenOperationStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sqliteStore.Close() })
	store := &countingOperationStore{OperationStore: sqliteStore}
	registry := NewRegistry()
	for _, definition := range []Definition{
		{
			Name: "test.read", Effect: EffectRead,
			Planner: func(context.Context, PlanningContext, json.RawMessage) (Plan, error) {
				return Plan{}, nil
			},
		},
		{
			Name: "test.write", Effect: EffectWrite,
			Planner: func(context.Context, PlanningContext, json.RawMessage) (Plan, error) {
				return Plan{Steps: []Step{{
					Opcode: "ain", Command: Protocol.Command{Opcode: "ain", Payload: json.RawMessage(`{}`)},
				}}}, nil
			},
		},
		{
			Name: "test.action", Effect: EffectWrite,
			Planner: func(context.Context, PlanningContext, json.RawMessage) (Plan, error) {
				return Plan{Steps: []Step{{Action: "test.action"}}}, nil
			},
		},
	} {
		if err := registry.Register(definition); err != nil {
			t.Fatal(err)
		}
	}
	engine := NewEngine(registry, State.NewStore(State.NewGameState()), nil, &operationStoreSender{}, nil)
	if err := engine.RegisterAction("test.action", func(context.Context, json.RawMessage) error { return nil }); err != nil {
		t.Fatal(err)
	}
	if err := engine.SetOperationStore(t.Context(), store); err != nil {
		t.Fatal(err)
	}
	read := engine.Submit(t.Context(), Request{ID: "read-operation", Name: "test.read"})
	if read.Status != StatusSucceeded {
		t.Fatalf("read receipt = %#v", read)
	}
	if _, found, err := sqliteStore.Get(t.Context(), read.ID); err != nil || found {
		t.Fatalf("read durable lookup: found=%t err=%v", found, err)
	}
	if store.reserves.Load() != 0 || store.saves.Load() != 0 {
		t.Fatalf("read persistence calls: reserves=%d saves=%d", store.reserves.Load(), store.saves.Load())
	}
	recent, err := engine.RecentOperations(t.Context(), 1)
	if err != nil || len(recent) != 1 || recent[0].ID != read.ID {
		t.Fatalf("in-memory recent read operation = %#v, err=%v", recent, err)
	}

	write := engine.Submit(t.Context(), Request{ID: "write-operation", Name: "test.write"})
	if write.Status != StatusSucceeded {
		t.Fatalf("write receipt = %#v", write)
	}
	if store.reserves.Load() != 1 || store.saves.Load() != 1 {
		t.Fatalf("write persistence calls: reserves=%d saves=%d", store.reserves.Load(), store.saves.Load())
	}
	checkpoint := store.lastReserve.Load()
	if checkpoint == nil || checkpoint.Plan == nil || checkpoint.Phase != EffectPhaseDispatching {
		t.Fatalf("pre-dispatch checkpoint = %#v", checkpoint)
	}
	stored, found, err := sqliteStore.Get(t.Context(), write.ID)
	if err != nil || !found || stored.Receipt.Plan == nil || stored.Receipt.Status != StatusSucceeded {
		t.Fatalf("stored write: found=%t operation=%#v err=%v", found, stored, err)
	}
	action := engine.Submit(t.Context(), Request{ID: "action-operation", Name: "test.action"})
	if action.Status != StatusSucceeded || store.reserves.Load() != 2 || store.saves.Load() != 2 {
		t.Fatalf(
			"action persistence: receipt=%#v reserves=%d saves=%d",
			action, store.reserves.Load(), store.saves.Load(),
		)
	}
}

func TestOperationStorePrunesTerminalRowsButKeepsActiveRows(t *testing.T) {
	store, err := OpenOperationStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	if _, err := store.db.Exec(`
		WITH RECURSIVE sequence(value) AS (
			SELECT 1
			UNION ALL
			SELECT value + 1 FROM sequence WHERE value < ?
		)
		INSERT INTO intent_operations (
			operation_id, request_hash, receipt_json, status, phase, submitted_at, updated_at
		)
		SELECT printf('terminal-%d', value), printf('hash-%d', value), '{}', ?, ?,
			printf('2026-01-01T00:00:%06dZ', value), printf('2026-01-01T00:00:%06dZ', value)
		FROM sequence
	`, operationHistoryLimit+5, StatusSucceeded, EffectPhaseCompleted); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"active-running", "active-paused"} {
		if _, err := store.db.Exec(`
			INSERT INTO intent_operations (
				operation_id, request_hash, receipt_json, status, phase, submitted_at, updated_at
			) VALUES (?, ?, '{}', ?, ?, '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z')
		`, id, id, StatusRunning, EffectPhaseDispatching); err != nil {
			t.Fatal(err)
		}
	}
	if err := store.pruneTerminalHistory(t.Context()); err != nil {
		t.Fatal(err)
	}
	var terminalCount int
	if err := store.db.QueryRow(`SELECT COUNT(*) FROM intent_operations WHERE status = ?`, StatusSucceeded).Scan(&terminalCount); err != nil {
		t.Fatal(err)
	}
	var activeCount int
	if err := store.db.QueryRow(`SELECT COUNT(*) FROM intent_operations WHERE status IN (?, ?)`, StatusRunning, StatusPaused).Scan(&activeCount); err != nil {
		t.Fatal(err)
	}
	if terminalCount != operationHistoryLimit || activeCount != 2 {
		t.Fatalf("pruned counts: terminal=%d active=%d", terminalCount, activeCount)
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
