package API

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"strings"
	"testing"
	"time"

	"CitadelDesktop/Server/Intent"
	"CitadelDesktop/Server/Outbound"
	"CitadelDesktop/Server/Session"
	"CitadelDesktop/Server/State"
)

type healthLatencySession struct{}

type failedPersistenceHealth struct{}

func (failedPersistenceHealth) PersistenceError() error { return errors.New("disk full") }

func (healthLatencySession) Status() Session.Status {
	return Session.Status{State: "connected", LoggedIn: true, SocketReady: true}
}

type operationsWalkStore struct {
	Intent.OperationStore
	loaded, recovered int
}

func (store *operationsWalkStore) Recent(ctx context.Context, limit int) ([]Intent.StoredOperation, error) {
	operations, err := store.OperationStore.Recent(ctx, limit)
	store.loaded = len(operations)
	return operations, err
}

func (store *operationsWalkStore) Recover(ctx context.Context) ([]Intent.StoredOperation, error) {
	operations, err := store.OperationStore.Recover(ctx)
	store.recovered = len(operations)
	return operations, err
}

// Fixed startup receipts let this same default-response subtest run on develop
// and on the candidate with byte-identical inputs, without wall-clock ordering.
type operationsSnapshotStore struct {
	Intent.OperationStore
	operations []Intent.StoredOperation
}

func (store operationsSnapshotStore) Recent(_ context.Context, limit int) ([]Intent.StoredOperation, error) {
	return store.operations[:min(limit, len(store.operations))], nil
}

func (store operationsSnapshotStore) Recover(context.Context) ([]Intent.StoredOperation, error) {
	return nil, nil
}

func TestOperationsWalkFullHistoryWithCursor(t *testing.T) {
	t.Run("default matches develop", func(t *testing.T) {
		operations := make([]Intent.StoredOperation, 1200)
		for index := range operations {
			operations[index] = Intent.StoredOperation{RequestHash: fmt.Sprintf("hash-%04d", index), Receipt: Intent.Receipt{
				ID: fmt.Sprintf("snapshot-%04d", index), Intent: "test.write", Status: Intent.StatusSucceeded,
				SubmittedAt: time.Unix(int64(1200-index), 0).UTC(),
			}}
		}
		engine := Intent.NewEngine(nil, nil, nil, nil, nil)
		if err := engine.SetOperationStore(t.Context(), operationsSnapshotStore{operations: operations}); err != nil {
			t.Fatal(err)
		}
		recorder := httptest.NewRecorder()
		NewServer(Config{Intents: engine}).Handler().ServeHTTP(recorder,
			httptest.NewRequest(http.MethodGet, "/api/v2/operations?limit=100", nil))
		want := make([]Intent.Receipt, 100)
		for index := range want {
			want[index] = operations[index].Receipt
		}
		var expected bytes.Buffer
		if err := json.NewEncoder(&expected).Encode(want); err != nil {
			t.Fatal(err)
		}
		if recorder.Code != http.StatusOK || !bytes.Equal(recorder.Body.Bytes(), expected.Bytes()) {
			t.Fatalf("default response differs: HTTP %d, got %s, want %s", recorder.Code, recorder.Body.Bytes(), expected.Bytes())
		}
		t.Logf("default response SHA256: %x", sha256.Sum256(recorder.Body.Bytes()))
	})

	dir := t.TempDir()
	store, err := Intent.OpenOperationStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	for index := 0; index < 1200; index++ {
		id := fmt.Sprintf("stored-%04d", index)
		receipt := Intent.Receipt{ID: id, Intent: "test.write", Status: Intent.StatusSucceeded,
			SubmittedAt: time.Unix(int64(1200-index), 0).UTC()}
		if _, created, err := store.Reserve(t.Context(), "hash-"+id, receipt); err != nil || !created {
			t.Fatalf("reserve %s: created=%t err=%v", id, created, err)
		}
	}
	tracked := &operationsWalkStore{OperationStore: store}
	engine := Intent.NewEngine(nil, nil, nil, nil, nil)
	if err := engine.SetOperationStore(t.Context(), tracked); err != nil {
		t.Fatal(err)
	}
	if tracked.loaded != 500 || tracked.recovered != 0 {
		t.Fatalf("startup loaded=%d recovered=%d", tracked.loaded, tracked.recovered)
	}
	handler := NewServer(Config{Intents: engine}).Handler()
	get := func(query string) ([]Intent.Receipt, []byte) {
		t.Helper()
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/v2/operations?"+query, nil))
		if recorder.Code != http.StatusOK {
			t.Fatalf("GET %s: HTTP %d %s", query, recorder.Code, recorder.Body.Bytes())
		}
		var receipts []Intent.Receipt
		if err := json.Unmarshal(recorder.Body.Bytes(), &receipts); err != nil || receipts == nil {
			t.Fatalf("GET %s: decode=%v body=%s", query, err, recorder.Body.Bytes())
		}
		return receipts, recorder.Body.Bytes()
	}
	walk := func() ([]string, []byte) {
		t.Helper()
		query := "history=stored&limit=100"
		seen := make(map[string]bool)
		ids := make([]string, 0, 1200)
		var bodies [][]byte
		for page := 0; page <= 12; page++ {
			receipts, body := get(query)
			bodies = append(bodies, body)
			if len(receipts) == 0 {
				if len(ids) != 1200 {
					t.Fatalf("walk returned %d receipts, want 1200", len(ids))
				}
				return ids, bytes.Join(bodies, nil)
			}
			if len(receipts) != 100 {
				t.Fatalf("page %d has %d receipts, want 100", page, len(receipts))
			}
			for _, receipt := range receipts {
				want := fmt.Sprintf("stored-%04d", 1199-len(ids))
				if seen[receipt.ID] || receipt.ID != want {
					t.Fatalf("walk position %d: got %s, want %s, duplicate=%t", len(ids), receipt.ID, want, seen[receipt.ID])
				}
				seen[receipt.ID] = true
				ids = append(ids, receipt.ID)
			}
			query = "before=" + url.QueryEscape(receipts[len(receipts)-1].ID) + "&limit=100"
		}
		t.Fatal("walk did not terminate")
		return nil, nil
	}
	_, recentBefore := get("limit=100")
	wantIDs, _ := walk()
	_, recentAfter := get("limit=100")
	if !bytes.Equal(recentBefore, recentAfter) {
		t.Fatal("stored walk changed the recent cache")
	}
	// Save an old, evicted receipt, then load it through the operation endpoint.
	// The store lookup re-caches it as recent activity, without moving its rowid.
	old, found, err := store.Get(t.Context(), "stored-0010")
	if err != nil || !found {
		t.Fatalf("old receipt: found=%t err=%v", found, err)
	}
	old.Receipt.Error = "late update"
	if err := store.Save(t.Context(), old.Receipt); err != nil {
		t.Fatal(err)
	}
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/v2/operations/stored-0010", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("operation lookup: HTTP %d %s", recorder.Code, recorder.Body.Bytes())
	}
	recent, _ := get("limit=100")
	if len(recent) != 100 || recent[0].ID != old.Receipt.ID || recent[0].Error != "late update" {
		t.Fatalf("updated receipt was not re-cached: %+v", recent)
	}
	if ids, _ := walk(); !reflect.DeepEqual(ids, wantIDs) {
		t.Fatal("late update changed stored walk")
	}

	// Crash/restart recovery changes statuses and recent order for 300 rows.
	for index := 0; index < 300; index++ {
		operation, found, err := store.Get(t.Context(), fmt.Sprintf("stored-%04d", index))
		if err != nil || !found {
			t.Fatalf("recovery fixture: found=%t err=%v", found, err)
		}
		operation.Receipt.Status = Intent.StatusRunning
		operation.Receipt.Phase = Intent.EffectPhaseDispatching
		if err := store.Save(t.Context(), operation.Receipt); err != nil {
			t.Fatal(err)
		}
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	store, err = Intent.OpenOperationStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	tracked = &operationsWalkStore{OperationStore: store}
	engine = Intent.NewEngine(nil, nil, nil, nil, nil)
	if err := engine.SetOperationStore(t.Context(), tracked); err != nil {
		t.Fatal(err)
	}
	if tracked.loaded != 500 || tracked.recovered != 300 {
		t.Fatalf("restart loaded=%d recovered=%d", tracked.loaded, tracked.recovered)
	}
	handler = NewServer(Config{Intents: engine}).Handler()
	if ids, _ := walk(); !reflect.DeepEqual(ids, wantIDs) {
		t.Fatal("restart recovery changed stored walk")
	}
	_, firstRecoveredWalk := walk()
	_, secondRecoveredWalk := walk()
	if !bytes.Equal(firstRecoveredWalk, secondRecoveredWalk) {
		t.Fatal("two walks after recovery have different receipt bodies")
	}
	for index := 0; index < 300; index++ {
		operation, found, err := store.Get(t.Context(), fmt.Sprintf("stored-%04d", index))
		if err != nil || !found || operation.Receipt.Status != Intent.StatusIndeterminate {
			t.Fatalf("recovered receipt %d: %+v, found=%t err=%v", index, operation.Receipt, found, err)
		}
	}
}

func (healthLatencySession) DispatchLatency() Outbound.DispatchLatencyStats {
	return Outbound.DispatchLatencyStats{TargetMilliseconds: 25, FastPathTargetMet: true}
}

func TestOperationCancellationEndpointStopsIntent(t *testing.T) {
	registry := Intent.NewRegistry()
	if err := registry.Register(Intent.Definition{
		Name: "test.block", Effect: Intent.EffectWrite,
		Planner: func(context.Context, Intent.PlanningContext, json.RawMessage) (Intent.Plan, error) {
			return Intent.Plan{Steps: []Intent.Step{{Action: "test.block"}}}, nil
		},
	}); err != nil {
		t.Fatal(err)
	}
	state := State.NewStore(State.NewGameState())
	engine := Intent.NewEngine(registry, state, nil, nil, nil)
	started := make(chan struct{})
	if err := engine.RegisterAction("test.block", func(ctx context.Context, _ json.RawMessage) error {
		close(started)
		<-ctx.Done()
		return ctx.Err()
	}); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(NewServer(Config{State: state, Intents: engine}).Handler())
	defer server.Close()
	result := make(chan Intent.Receipt, 1)
	go func() {
		response, err := http.Post(
			server.URL+"/api/v2/intents/test.block?wait=true", "application/json",
			strings.NewReader(`{"id":"cancel-api","arguments":{}}`),
		)
		if err != nil {
			result <- Intent.Receipt{Status: Intent.StatusFailed, Error: err.Error()}
			return
		}
		defer response.Body.Close()
		var receipt Intent.Receipt
		_ = json.NewDecoder(response.Body).Decode(&receipt)
		result <- receipt
	}()
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("intent did not start")
	}
	response, err := http.Post(server.URL+"/api/v2/operations/cancel-api/cancel", "application/json", nil)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusAccepted {
		t.Fatalf("cancel endpoint returned HTTP %d", response.StatusCode)
	}
	select {
	case receipt := <-result:
		if receipt.Status != Intent.StatusCancelled {
			t.Fatalf("intent status = %q, error = %q", receipt.Status, receipt.Error)
		}
	case <-time.After(time.Second):
		t.Fatal("cancelled request did not return")
	}
}

func TestIntentSubmissionOutlivesTheDashboardConnection(t *testing.T) {
	registry := Intent.NewRegistry()
	if err := registry.Register(Intent.Definition{
		Name: "test.block", Effect: Intent.EffectWrite,
		Planner: func(context.Context, Intent.PlanningContext, json.RawMessage) (Intent.Plan, error) {
			return Intent.Plan{Steps: []Intent.Step{{Action: "test.block"}}}, nil
		},
	}); err != nil {
		t.Fatal(err)
	}
	state := State.NewStore(State.NewGameState())
	engine := Intent.NewEngine(registry, state, nil, nil, nil)
	runtimeContext, stopRuntime := context.WithCancel(context.Background())
	defer stopRuntime()
	engine.SetRuntimeContext(runtimeContext)
	started := make(chan struct{}, 4)
	actionDone := make(chan error, 4)
	if err := engine.RegisterAction("test.block", func(ctx context.Context, _ json.RawMessage) error {
		started <- struct{}{}
		<-ctx.Done()
		actionDone <- ctx.Err()
		return ctx.Err()
	}); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(NewServer(Config{State: state, Intents: engine}).Handler())
	defer server.Close()

	// Default submission answers immediately with the accepted receipt.
	response, err := http.Post(
		server.URL+"/api/v2/intents/test.block", "application/json",
		strings.NewReader(`{"id":"detached-api","arguments":{}}`),
	)
	if err != nil {
		t.Fatal(err)
	}
	var accepted Intent.Receipt
	_ = json.NewDecoder(response.Body).Decode(&accepted)
	response.Body.Close()
	if response.StatusCode != http.StatusAccepted || accepted.ID != "detached-api" || accepted.Terminal() {
		t.Fatalf("detached submission = HTTP %d %+v", response.StatusCode, accepted)
	}
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("detached intent did not start")
	}

	// A waiting client that goes away must not cancel the operation either.
	requestContext, abandon := context.WithCancel(context.Background())
	waitRequest, _ := http.NewRequestWithContext(requestContext, http.MethodPost,
		server.URL+"/api/v2/intents/test.block?wait=true", strings.NewReader(`{"id":"abandoned-api","arguments":{}}`))
	waitRequest.Header.Set("Content-Type", "application/json")
	waitErrors := make(chan error, 1)
	go func() {
		_, err := http.DefaultClient.Do(waitRequest)
		waitErrors <- err
	}()
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("waited intent did not start")
	}
	abandon()
	if err := <-waitErrors; err == nil {
		t.Fatal("abandoned wait request unexpectedly completed")
	}
	select {
	case err := <-actionDone:
		t.Fatalf("closing the client connection cancelled the operation: %v", err)
	case <-time.After(150 * time.Millisecond):
	}
	for _, id := range []string{"detached-api", "abandoned-api"} {
		receipt, ok := engine.Operation(id)
		if !ok || receipt.Terminal() {
			t.Fatalf("operation %s after client disconnect = %+v", id, receipt)
		}
	}

	// Explicit cancellation and application shutdown remain the only ways to
	// stop a running operation.
	response, err = http.Post(server.URL+"/api/v2/operations/detached-api/cancel", "application/json", nil)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	awaitContext, cancelAwait := context.WithTimeout(context.Background(), time.Second)
	defer cancelAwait()
	if receipt, err := engine.Await(awaitContext, "detached-api"); err != nil || receipt.Status != Intent.StatusCancelled {
		t.Fatalf("Await after cancel = %+v, %v", receipt, err)
	}
	stopRuntime()
	if receipt, err := engine.Await(awaitContext, "abandoned-api"); err != nil || receipt.Status != Intent.StatusCancelled {
		t.Fatalf("Await after runtime stop = %+v, %v", receipt, err)
	}
	if err := engine.WaitIdle(awaitContext); err != nil {
		t.Fatalf("WaitIdle = %v", err)
	}
	if _, err := engine.Await(awaitContext, "missing"); !errors.Is(err, Intent.ErrOperationNotFound) {
		t.Fatalf("Await(missing) = %v", err)
	}
}

func TestRecentOperationsEndpointReturnsLatestReceipts(t *testing.T) {
	registry := Intent.NewRegistry()
	if err := registry.Register(Intent.Definition{
		Name: "test.read", Effect: Intent.EffectRead,
		Planner: func(context.Context, Intent.PlanningContext, json.RawMessage) (Intent.Plan, error) {
			return Intent.Plan{}, nil
		},
	}); err != nil {
		t.Fatal(err)
	}
	state := State.NewStore(State.NewGameState())
	engine := Intent.NewEngine(registry, state, nil, nil, nil)
	for _, id := range []string{"first", "second"} {
		receipt := engine.Submit(context.Background(), Intent.Request{ID: id, Name: "test.read"})
		if receipt.Status != Intent.StatusSucceeded {
			t.Fatalf("submit %s = %#v", id, receipt)
		}
	}
	server := httptest.NewServer(NewServer(Config{State: state, Intents: engine}).Handler())
	defer server.Close()
	response, err := http.Get(server.URL + "/api/v2/operations?limit=1")
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("recent operations returned HTTP %d", response.StatusCode)
	}
	var receipts []Intent.Receipt
	if err := json.NewDecoder(response.Body).Decode(&receipts); err != nil {
		t.Fatal(err)
	}
	if len(receipts) != 1 || receipts[0].ID != "second" {
		t.Fatalf("recent operations = %#v", receipts)
	}
}

func TestHealthIncludesDispatchLatencyWhenSessionProvidesIt(t *testing.T) {
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/v2/health", nil)
	NewServer(Config{Session: healthLatencySession{}}).Handler().ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("health returned HTTP %d", recorder.Code)
	}
	var response struct {
		DispatchLatency *Outbound.DispatchLatencyStats `json:"dispatchLatency"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if response.DispatchLatency == nil || response.DispatchLatency.TargetMilliseconds != 25 ||
		!response.DispatchLatency.FastPathTargetMet {
		t.Fatalf("dispatch latency health = %#v", response.DispatchLatency)
	}
}

func TestHealthIncludesBuildProvenance(t *testing.T) {
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/v2/health", nil)
	NewServer(Config{
		Version: "2.3.0-test", BuildRevision: "0123456789abcdef", BuildID: "build-123",
	}).Handler().ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("health returned HTTP %d", recorder.Code)
	}
	var response struct {
		Version       string `json:"version"`
		BuildRevision string `json:"buildRevision"`
		BuildID       string `json:"buildId"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if response.Version != "2.3.0-test" || response.BuildRevision != "0123456789abcdef" || response.BuildID != "build-123" {
		t.Fatalf("health provenance = %#v", response)
	}
}

func TestHealthReportsPersistenceFailureAsDegraded(t *testing.T) {
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/v2/health", nil)
	NewServer(Config{Persistence: failedPersistenceHealth{}}).Handler().ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("health returned HTTP %d", recorder.Code)
	}
	var response struct {
		Status           string `json:"status"`
		PersistenceError string `json:"persistenceError"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if response.Status != "degraded" || response.PersistenceError != "disk full" {
		t.Fatalf("persistence health = %#v", response)
	}
}

func TestIntentEndpointOwnsActorAndPriorityClassification(t *testing.T) {
	registry := Intent.NewRegistry()
	if err := registry.Register(Intent.Definition{
		Name: "test.identity", Effect: Intent.EffectRead,
		Planner: func(context.Context, Intent.PlanningContext, json.RawMessage) (Intent.Plan, error) {
			return Intent.Plan{}, nil
		},
	}); err != nil {
		t.Fatal(err)
	}
	state := State.NewStore(State.NewGameState())
	engine := Intent.NewEngine(registry, state, nil, nil, nil)
	server := httptest.NewServer(NewServer(Config{State: state, Intents: engine}).Handler())
	defer server.Close()
	response, err := http.Post(
		server.URL+"/api/v2/intents/test.identity", "application/json",
		strings.NewReader(`{"actor":"automation:autoStation","priority":1}`),
	)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	var receipt Intent.Receipt
	if err := json.NewDecoder(response.Body).Decode(&receipt); err != nil {
		t.Fatal(err)
	}
	if receipt.Actor != "ui" || receipt.Priority != Outbound.PriorityInteractive {
		t.Fatalf("server-owned identity = actor %q priority %d", receipt.Actor, receipt.Priority)
	}
}

func TestOperationsEndpointPagesWithBefore(t *testing.T) {
	store, err := Intent.OpenOperationStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	for _, id := range []string{"first", "second", "third"} {
		if _, _, err := store.Reserve(t.Context(), "hash-"+id, Intent.Receipt{ID: id, Status: Intent.StatusSucceeded}); err != nil {
			t.Fatal(err)
		}
	}
	engine := Intent.NewEngine(nil, nil, nil, nil, nil)
	if err := engine.SetOperationStore(t.Context(), store); err != nil {
		t.Fatal(err)
	}
	handler := NewServer(Config{Intents: engine}).Handler()
	for _, test := range []struct {
		name, query, code string
		status            int
		ids               []string
	}{
		{name: "page", query: "before=third", status: 200, ids: []string{"second", "first"}},
		{name: "stored start", query: "history=stored&limit=2", status: 200, ids: []string{"third", "second"}},
		{name: "stored continuation", query: "history=stored&before=third", status: 200, ids: []string{"second", "first"}},
		{name: "recent is invalid", query: "history=recent&limit=1", status: 400, code: "invalid_history"},
		{name: "invalid history", query: "history=invalid", status: 400, code: "invalid_history"},
		{name: "invalid history with cursor", query: "history=invalid&before=third", status: 400, code: "invalid_history"},
		{name: "trim cursor", query: "before=" + url.QueryEscape("  third  ") + "&limit=1", status: 200, ids: []string{"second"}},
		{name: "empty page", query: "before=first", status: 200, ids: []string{}},
		{name: "unknown", query: "before=missing", status: 400, code: "invalid_cursor"},
		{name: "oversized", query: "before=" + strings.Repeat("x", 257), status: 400, code: "invalid_cursor"},
		{name: "oversized UTF8", query: "before=" + url.QueryEscape(strings.Repeat("é", 129)), status: 400, code: "invalid_cursor"},
		{name: "bad limit", query: "before=third&limit=bad", status: 400, code: "invalid_limit"},
		{name: "zero limit", query: "before=third&limit=0", status: 400, code: "invalid_limit"},
		{name: "large limit", query: "before=third&limit=1001", status: 400, code: "invalid_limit"},
		{name: "blank cursor", query: "before=" + url.QueryEscape(" ") + "&limit=1", status: 200, ids: []string{"third"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/v2/operations?"+test.query, nil))
			if recorder.Code != test.status {
				t.Fatalf("HTTP %d: %s", recorder.Code, recorder.Body.String())
			}
			if test.status != http.StatusOK {
				var response struct{ Error struct{ Code string } }
				if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil || response.Error.Code != test.code {
					t.Fatalf("error response = %s, decode=%v", recorder.Body.String(), err)
				}
				return
			}
			var receipts []Intent.Receipt
			if err := json.Unmarshal(recorder.Body.Bytes(), &receipts); err != nil || receipts == nil || len(receipts) != len(test.ids) {
				t.Fatalf("page = %s, decode=%v", recorder.Body.String(), err)
			}
			for index, id := range test.ids {
				if receipts[index].ID != id {
					t.Fatalf("page[%d] = %s, want %s", index, receipts[index].ID, id)
				}
			}
		})
	}
	t.Run("no store", func(t *testing.T) {
		recorder := httptest.NewRecorder()
		for _, query := range []string{"before=first", "history=stored"} {
			recorder = httptest.NewRecorder()
			NewServer(Config{Intents: Intent.NewEngine(nil, nil, nil, nil, nil)}).Handler().ServeHTTP(recorder,
				httptest.NewRequest(http.MethodGet, "/api/v2/operations?"+query, nil))
			if recorder.Code != http.StatusServiceUnavailable || !strings.Contains(recorder.Body.String(), "operations_unavailable") {
				t.Fatalf("no store response = %d %s", recorder.Code, recorder.Body.String())
			}
		}
	})
	t.Run("store error", func(t *testing.T) {
		if err := store.Close(); err != nil {
			t.Fatal(err)
		}
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/v2/operations?before=first", nil))
		if recorder.Code != http.StatusServiceUnavailable || !strings.Contains(recorder.Body.String(), "operations_unavailable") {
			t.Fatalf("store error response = %d %s", recorder.Code, recorder.Body.String())
		}
	})
}
