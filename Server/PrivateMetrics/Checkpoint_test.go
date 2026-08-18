package PrivateMetrics

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"CitadelDesktop/Server/State"
)

type receivedCheckpoint struct {
	authorization  string
	idempotencyKey string
	encoding       string
	body           []byte
	request        CheckpointRequest
}

func checkpointServer(t *testing.T) (*httptest.Server, <-chan receivedCheckpoint) {
	t.Helper()
	received := make(chan receivedCheckpoint, 16)
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/checkpoints" {
			writer.WriteHeader(http.StatusNotFound)
			return
		}
		reader := io.Reader(request.Body)
		if request.Header.Get("Content-Encoding") == "gzip" {
			gzipReader, err := gzip.NewReader(request.Body)
			if err != nil {
				t.Errorf("gzip checkpoint: %v", err)
				writer.WriteHeader(http.StatusBadRequest)
				return
			}
			reader = gzipReader
		}
		body, _ := io.ReadAll(io.LimitReader(reader, 16<<20))
		var payload CheckpointRequest
		if err := json.Unmarshal(body, &payload); err != nil {
			t.Errorf("decode checkpoint: %v", err)
			writer.WriteHeader(http.StatusBadRequest)
			return
		}
		received <- receivedCheckpoint{
			authorization: request.Header.Get("Authorization"), idempotencyKey: request.Header.Get("Idempotency-Key"),
			encoding: request.Header.Get("Content-Encoding"), body: body, request: payload,
		}
		writer.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(server.Close)
	return server, received
}

func awaitCheckpoint(t *testing.T, received <-chan receivedCheckpoint) receivedCheckpoint {
	t.Helper()
	select {
	case checkpoint := <-received:
		return checkpoint
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for a dashboard checkpoint")
		return receivedCheckpoint{}
	}
}

func TestCheckpointPublisherPublishesTheDashboardReadModel(t *testing.T) {
	server, received := checkpointServer(t)
	client, err := NewClient(ClientConfig{
		Client: server.Client(), Endpoint: server.URL + "/metrics", CheckpointEndpoint: server.URL + "/checkpoints", ClientVersion: "test",
	})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	store := readyPrivateMetricsState(t, now)
	token := strings.Repeat("c", 48)
	publisher, err := NewCheckpointPublisher(CheckpointPublisherConfig{
		RuntimeID: "runtime-one", State: store, Client: client,
		Placement: testPlacement(now, 4, 10, token),
		Interval:  time.Hour, Debounce: 5 * time.Millisecond, Jitter: func() float64 { return 0 },
	})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go publisher.Run(ctx)

	first := awaitCheckpoint(t, received)
	if first.authorization != "Bearer "+token || first.encoding != "gzip" || first.idempotencyKey == "" ||
		first.idempotencyKey != first.request.Checkpoint.CheckpointID || first.request.PlacementEpoch != 4 {
		t.Fatalf("first checkpoint = %+v", first)
	}
	if bytes.Contains(first.body, []byte(token)) {
		t.Fatal("checkpoint body exposed the grant")
	}
	checkpoint := first.request.Checkpoint
	if checkpoint.Reason != CheckpointReasonSession && checkpoint.Reason != CheckpointReasonCadence {
		t.Fatalf("initial checkpoint reason = %q", checkpoint.Reason)
	}
	if checkpoint.Account == nil || checkpoint.Account.WorldID != "world.example" || checkpoint.Account.PlayerID != 42 ||
		checkpoint.Session.State != "" && !checkpoint.Session.LoggedIn {
		t.Fatalf("checkpoint identity/session = %+v %+v", checkpoint.Account, checkpoint.Session)
	}
	var state struct {
		Player struct {
			Name  string  `json:"name"`
			Might float64 `json:"might"`
		} `json:"player"`
		Castles map[string]any `json:"castles"`
	}
	if err := json.Unmarshal(checkpoint.State, &state); err != nil {
		t.Fatalf("checkpoint state is not the dashboard document: %v", err)
	}
	if state.Player.Name != "Player Forty Two" || state.Player.Might != 12345 || len(state.Castles) != 1 {
		t.Fatalf("checkpoint state = %+v", state)
	}
	if status := publisher.Status(); status.State != StatePublished || status.LastRevision != checkpoint.StateRevision {
		t.Fatalf("publisher status = %+v", status)
	}

	// A session transition (here: the game session dropping) is checkpointed
	// promptly so the stale dashboard shows the last true situation.
	if _, err := store.ApplyComponents(State.Components(State.ComponentSession), func(gameState *State.GameState) ([]string, bool, error) {
		gameState.Session.Status = "released"
		gameState.Session.LoggedIn = false
		gameState.Session.SocketReady = false
		return []string{"session"}, true, nil
	}); err != nil {
		t.Fatal(err)
	}
	released := awaitCheckpoint(t, received)
	if released.request.Checkpoint.Reason != CheckpointReasonSession || released.request.Checkpoint.Session.State != "released" ||
		released.request.Checkpoint.CheckpointID == first.request.Checkpoint.CheckpointID {
		t.Fatalf("session checkpoint = %+v", released.request.Checkpoint.Session)
	}

	// The drain checkpoint is synchronous and bounded by its context.
	drainContext, cancelDrain := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancelDrain()
	if err := publisher.Checkpoint(drainContext, CheckpointReasonDrain); err != nil {
		t.Fatal(err)
	}
	drain := awaitCheckpoint(t, received)
	if drain.request.Checkpoint.Reason != CheckpointReasonDrain {
		t.Fatalf("drain checkpoint reason = %q", drain.request.Checkpoint.Reason)
	}
}

func TestBuildCheckpointHasNoReadinessGateAndOmitsUnboundIdentity(t *testing.T) {
	store := State.NewStore(State.NewGameState())
	checkpoint, err := BuildCheckpoint(context.Background(), store, nil, nil, CheckpointReasonCadence, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	if checkpoint.Account != nil || len(checkpoint.State) == 0 || checkpoint.Reason != CheckpointReasonCadence {
		t.Fatalf("empty-state checkpoint = %+v", checkpoint)
	}
	if !json.Valid(checkpoint.State) {
		t.Fatal("checkpoint state is not valid JSON")
	}
}

func TestCheckpointPublisherRequiresCheckpointEndpoint(t *testing.T) {
	client, err := NewClient(ClientConfig{Endpoint: "https://backend.example/private-metrics"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := NewCheckpointPublisher(CheckpointPublisherConfig{
		RuntimeID: "runtime-one", State: State.NewStore(State.NewGameState()), Client: client,
	}); err == nil {
		t.Fatal("checkpoint publisher was created without a checkpoint endpoint")
	}
	if _, err := NewClient(ClientConfig{Endpoint: "https://backend.example/private-metrics", CheckpointEndpoint: "not a url"}); err == nil {
		t.Fatal("invalid checkpoint endpoint was accepted")
	}
}
