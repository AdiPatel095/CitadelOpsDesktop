package API

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
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
			server.URL+"/api/v2/intents/test.block", "application/json",
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
