package Intent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"testing"
	"time"

	"CitadelDesktop/Server/Protocol"
	"CitadelDesktop/Server/State"
)

// The caller owns the engine lock, or no engine execution is running.
func assertReceiptCacheConsistent(t *testing.T, engine *Engine) {
	t.Helper()
	count := len(engine.operations)
	if len(engine.requestHashes) != count || len(engine.durableOperations) != count ||
		len(engine.operationIndex) != count || len(engine.operationSizes) != count || len(engine.operationOrder) != count {
		t.Fatalf("inconsistent cache: receipts=%d hashes=%d durable=%d index=%d sizes=%d order=%d", count,
			len(engine.requestHashes), len(engine.durableOperations), len(engine.operationIndex), len(engine.operationSizes), len(engine.operationOrder))
	}
	seen := make(map[string]bool, count)
	bytes := 0
	for _, id := range engine.operationOrder {
		receipt, present := engine.operations[id]
		_, durable := engine.durableOperations[id]
		_, indexed := engine.operationIndex[id]
		if seen[id] || !present || !durable || !indexed || engine.requestHashes[id] != "hash-"+id ||
			engine.operationSizes[id] != estimatedReceiptBytes(receipt) {
			t.Fatalf("inconsistent cache entry %s", id)
		}
		seen[id] = true
		bytes += engine.operationSizes[id]
	}
	if bytes != engine.cachedOperationBytes {
		t.Fatalf("cached bytes = %d, recomputed %d", engine.cachedOperationBytes, bytes)
	}
}

func openReceiptHistoryStore(t *testing.T, count int) *SQLiteOperationStore {
	t.Helper()
	store, err := OpenOperationStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	for index := 0; index < count; index++ {
		id := fmt.Sprintf("stored-%04d", index)
		receipt := Receipt{ID: id, Intent: "test.write", Status: StatusSucceeded, Phase: EffectPhaseCompleted,
			SubmittedAt: time.Unix(int64(count-index), 0).UTC()}
		if _, created, err := store.Reserve(t.Context(), "hash-"+id, receipt); err != nil || !created {
			t.Fatalf("reserve %s: created=%t err=%v", id, created, err)
		}
	}
	return store
}

type startupWindowStore struct {
	OperationStore
	recentLimit int
}

func (store *startupWindowStore) Recent(ctx context.Context, limit int) ([]StoredOperation, error) {
	store.recentLimit = limit
	return store.OperationStore.Recent(ctx, limit)
}

func TestEngineLoadsOnlyTheMemoryWindowAtStart(t *testing.T) {
	store := &startupWindowStore{OperationStore: openReceiptHistoryStore(t, 2000)}
	recovered := map[string]Status{"recover-planning": StatusFailed, "recover-running": StatusIndeterminate, "recover-read": StatusFailed}
	for id := range recovered {
		receipt := Receipt{ID: id, Status: StatusRunning, Phase: EffectPhaseDispatching, Plan: &Plan{Effect: EffectWrite}}
		if id == "recover-planning" {
			receipt.Status, receipt.Phase = StatusPlanning, EffectPhaseAccepted
		} else if id == "recover-read" {
			receipt.Plan.Effect = EffectRead
		}
		if _, _, err := store.Reserve(t.Context(), "hash-"+id, receipt); err != nil {
			t.Fatal(err)
		}
	}
	engine := NewEngine(nil, nil, nil, nil, nil)
	if err := engine.SetOperationStore(t.Context(), store); err != nil {
		t.Fatal(err)
	}
	if store.recentLimit != operationMemoryLimit || len(engine.operations) != operationMemoryLimit || engine.cachedOperationBytes > operationMemoryBytes {
		t.Fatalf("startup window: requested=%d cached=%d bytes=%d", store.recentLimit, len(engine.operations), engine.cachedOperationBytes)
	}
	for id, status := range recovered {
		if receipt, present := engine.operations[id]; !present || receipt.Status != status {
			t.Fatalf("recovered receipt %s = %+v, present=%t", id, receipt, present)
		}
		if _, durable := engine.durableOperations[id]; !durable {
			t.Fatalf("recovered receipt %s is not durable", id)
		}
		if _, active := engine.active[id]; active {
			t.Fatalf("recovered receipt %s is active", id)
		}
	}
	assertReceiptCacheConsistent(t, engine)
}

func TestRecentOperationsFillsFromTheStoreBeyondMemory(t *testing.T) {
	store := openReceiptHistoryStore(t, 1200)
	engine := NewEngine(nil, nil, nil, nil, nil)
	if err := engine.SetOperationStore(t.Context(), store); err != nil {
		t.Fatal(err)
	}
	memory, err := engine.RecentOperations(t.Context(), 500)
	if err != nil || len(memory) != 500 {
		t.Fatalf("memory = %d receipts, %v", len(memory), err)
	}
	receipts, err := engine.RecentOperations(t.Context(), 1000)
	if err != nil || len(receipts) != 1000 {
		t.Fatalf("recent = %d receipts, %v", len(receipts), err)
	}
	if !reflect.DeepEqual(receipts[:500], memory) {
		t.Fatal("store fill changed the first 500 memory receipts")
	}
	seen := make(map[string]bool)
	for _, receipt := range receipts {
		if seen[receipt.ID] {
			t.Fatalf("duplicate receipt %s", receipt.ID)
		}
		seen[receipt.ID] = true
	}
	for index, receipt := range receipts[500:] {
		if want := fmt.Sprintf("stored-%04d", 699-index); receipt.ID != want {
			t.Fatalf("stored history[%d] = %s, want %s", index, receipt.ID, want)
		}
	}
	if len(engine.operations) != 500 {
		t.Fatal("history fill expanded the cache")
	}
}

func TestOperationLookupFromStoreKeepsDurability(t *testing.T) {
	store := &countingOperationStore{OperationStore: openReceiptHistoryStore(t, 600)}
	engine := NewEngine(nil, nil, nil, nil, nil)
	if err := engine.SetOperationStore(t.Context(), store); err != nil {
		t.Fatal(err)
	}
	if _, cached := engine.operations["stored-0000"]; cached {
		t.Fatal("old receipt was not evicted")
	}
	receipt, found := engine.Operation("stored-0000")
	if !found {
		t.Fatal("store lookup did not find evicted receipt")
	}
	if _, durable := engine.durableOperations[receipt.ID]; !durable {
		t.Fatal("store lookup lost durability")
	}
	receipt.Status = StatusIndeterminate
	receipt.Phase = EffectPhaseReconciliationRequired
	if err := engine.update(receipt); err != nil {
		t.Fatal(err)
	}
	persisted, found, err := store.Get(t.Context(), receipt.ID)
	if store.saves.Load() != 1 || err != nil || !found || persisted.Receipt.Status != StatusIndeterminate {
		t.Fatalf("updated receipt: saves=%d stored=%+v found=%t err=%v", store.saves.Load(), persisted, found, err)
	}
	assertReceiptCacheConsistent(t, engine)
}

func TestOperationsBeforePagesStoredHistory(t *testing.T) {
	store := openReceiptHistoryStore(t, 1200)
	registry := NewRegistry()
	if err := registry.Register(Definition{Name: "test.read", Effect: EffectRead,
		Planner: func(context.Context, PlanningContext, json.RawMessage) (Plan, error) { return Plan{}, nil },
	}); err != nil {
		t.Fatal(err)
	}
	engine := NewEngine(registry, State.NewStore(State.NewGameState()), nil, nil, nil)
	if err := engine.SetOperationStore(t.Context(), store); err != nil {
		t.Fatal(err)
	}
	read := engine.Submit(t.Context(), Request{ID: "read-only", Name: "test.read"})
	if read.Status != StatusSucceeded {
		t.Fatalf("read receipt = %+v", read)
	}
	order := append([]string(nil), engine.operationOrder...)
	bytes := engine.cachedOperationBytes
	seen := make(map[string]bool)
	cursor := ""
	count := 0
	for {
		page, err := engine.OperationsBefore(t.Context(), cursor, 100)
		if err != nil {
			t.Fatal(err)
		}
		if len(page) == 0 {
			break
		}
		if len(page) != 100 {
			t.Fatalf("page length = %d", len(page))
		}
		for _, receipt := range page {
			want := fmt.Sprintf("stored-%04d", 1199-count)
			if seen[receipt.ID] || receipt.ID != want {
				t.Fatalf("receipt %d = %s, want %s, duplicate=%t", count, receipt.ID, want, seen[receipt.ID])
			}
			seen[receipt.ID] = true
			count++
		}
		cursor = page[len(page)-1].ID
	}
	if count != 1200 || !reflect.DeepEqual(order, engine.operationOrder) || bytes != engine.cachedOperationBytes {
		t.Fatalf("paging: count=%d or cache changed", count)
	}
	for _, cursor := range []string{"unknown", read.ID} {
		if _, err := engine.OperationsBefore(t.Context(), cursor, 100); !errors.Is(err, ErrUnknownOperationCursor) {
			t.Fatalf("cursor %q: %v", cursor, err)
		}
	}
	if _, err := NewEngine(nil, nil, nil, nil, nil).OperationsBefore(t.Context(), "anything", 100); !errors.Is(err, ErrOperationHistoryUnavailable) {
		t.Fatalf("no store: %v", err)
	}
	for _, limit := range []int{-1, 0, 1, operationHistoryLimit + 1} {
		page, err := store.Page(t.Context(), "", limit)
		want := min(max(limit, 1), 1200)
		if err != nil || len(page) != want {
			t.Fatalf("Page limit %d = %d receipts, %v, want %d", limit, len(page), err, want)
		}
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := engine.OperationsBefore(ctx, "", 100); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled page: %v", err)
	}
}

func TestCapturedResponsesDropTheRawFrame(t *testing.T) {
	for _, barrier := range []ResponseBarrier{"", ResponseBarrierWire} {
		t.Run(string(barrier), func(t *testing.T) {
			code := 0
			frame := Protocol.Frame{Opcode: "test", Raw: `%xt%test%1%0%{"value":1}%`,
				Payload: json.RawMessage(`{"value":1}`), PayloadText: `{"value":1}`, ResponseCode: &code}
			observer := &wireCleanupObserver{frames: make(chan Protocol.CommittedFrame, 1), forgot: make(map[uint64]bool)}
			observer.frames <- Protocol.CommittedFrame{Frame: frame}
			engine := NewEngine(nil, State.NewStore(State.NewGameState()), nil, &performanceSender{}, observer)
			exchange, err := engine.executeStep(t.Context(), 0, Step{Opcode: "test", AwaitOpcode: "test",
				Payload: json.RawMessage(`{}`), CaptureResponse: true, ResponseBarrier: barrier, TimeoutMillis: 1000})
			if err != nil || exchange == nil || exchange.Response == nil {
				t.Fatalf("captured exchange = %+v, %v", exchange, err)
			}
			if exchange.Response.Raw != "" || string(exchange.Response.Payload) != string(frame.Payload) || exchange.Response.PayloadText != frame.PayloadText {
				t.Fatalf("captured response = %+v", exchange.Response)
			}
			if frame.Raw == "" {
				t.Fatal("capture changed the authoritative frame")
			}
		})
	}
}
