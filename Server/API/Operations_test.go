package API

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"CitadelDesktop/Server/Intent"
	"CitadelDesktop/Server/State"
)

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
