package PrivateMetrics

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"CitadelDesktop/Server/Configuration"
	"CitadelDesktop/Server/State"
)

// countingBackend counts actual uploads on both endpoints.
type countingBackend struct {
	server      *httptest.Server
	metrics     atomic.Int64
	checkpoints atomic.Int64
	mu          sync.Mutex
	lastMetrics time.Time
}

func newCountingBackend(t *testing.T) *countingBackend {
	t.Helper()
	backend := &countingBackend{}
	backend.server = httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		_, _ = readAll(request)
		switch request.URL.Path {
		case "/metrics":
			backend.metrics.Add(1)
		case "/checkpoints":
			backend.checkpoints.Add(1)
		default:
			writer.WriteHeader(http.StatusNotFound)
			return
		}
		writer.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(backend.server.Close)
	return backend
}

func readAll(request *http.Request) ([]byte, error) {
	buffer := make([]byte, 32<<10)
	var total []byte
	for {
		count, err := request.Body.Read(buffer)
		total = append(total, buffer[:count]...)
		if err != nil {
			return total, err
		}
	}
}

type settleRig struct {
	backend     *countingBackend
	store       *State.Store
	metrics     *Publisher
	checkpoints *CheckpointPublisher
	settle      *Settle
	configStore *Configuration.Store
}

// startSettleRig runs both publishers on one shared settle window with
// intervals scaled down: the metrics evaluation every 15 ms and the checkpoint
// settle cadence every 25 ms stand for one minute each; the settle window stands
// for ten minutes; the heartbeats and steady checkpoint interval are an hour.
func startSettleRig(t *testing.T, window time.Duration) *settleRig {
	t.Helper()
	backend := newCountingBackend(t)
	client, err := NewClient(ClientConfig{
		Client: backend.server.Client(), Endpoint: backend.server.URL + "/metrics",
		CheckpointEndpoint: backend.server.URL + "/checkpoints", ClientVersion: "test",
	})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	store := readyPrivateMetricsState(t, now)
	configuration, err := Configuration.Open(t.TempDir(), map[string]json.RawMessage{"scheduler": json.RawMessage(`{"minAttackDelay":4}`)})
	if err != nil {
		t.Fatal(err)
	}
	settle := NewSettle(window)
	placement := testPlacement(now, 4, 10, strings.Repeat("s", 48))
	metrics, err := NewPublisher(PublisherConfig{
		RuntimeID: "runtime-one", State: store, Client: client, Placement: placement,
		Interval: 15 * time.Millisecond, Heartbeat: time.Hour, Settle: settle, Debounce: time.Millisecond,
		Jitter: func() float64 { return 0 },
	})
	if err != nil {
		t.Fatal(err)
	}
	checkpoints, err := NewCheckpointPublisher(CheckpointPublisherConfig{
		RuntimeID: "runtime-one", State: store, Configuration: configuration, Client: client, Placement: placement,
		Interval: time.Hour, Heartbeat: 2 * time.Hour, SettleInterval: 25 * time.Millisecond, Settle: settle,
		Debounce: 2 * time.Millisecond, Jitter: func() float64 { return 0 },
	})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go metrics.Run(ctx)
	go checkpoints.Run(ctx)
	return &settleRig{backend: backend, store: store, metrics: metrics, checkpoints: checkpoints, settle: settle, configStore: configuration}
}

func waitUntil(t *testing.T, within time.Duration, what string, condition func() bool) {
	t.Helper()
	deadline := time.Now().Add(within)
	for !condition() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(3 * time.Millisecond)
	}
}

// quiet reports whether neither endpoint received an upload during the window.
func (rig *settleRig) quiet(window time.Duration) bool {
	metrics, checkpoints := rig.backend.metrics.Load(), rig.backend.checkpoints.Load()
	time.Sleep(window)
	return rig.backend.metrics.Load() == metrics && rig.backend.checkpoints.Load() == checkpoints
}

func TestSettleWindowIsSharedAndOnlyExtends(t *testing.T) {
	settle := NewSettle(100 * time.Millisecond)
	base := time.Now()
	if settle.Active(base) {
		t.Fatal("a new window starts closed")
	}
	settle.Restart(base)
	if !settle.Active(base.Add(99*time.Millisecond)) || settle.Active(base.Add(100*time.Millisecond)) {
		t.Fatal("the window lasts exactly its length")
	}
	settle.Restart(base.Add(50 * time.Millisecond))
	if !settle.Active(base.Add(149 * time.Millisecond)) {
		t.Fatal("a later trigger extends the window")
	}
	settle.Restart(base) // an older trigger never shortens it
	if !settle.Active(base.Add(149 * time.Millisecond)) {
		t.Fatal("an earlier restart shortened the window")
	}
	var nilSettle *Settle
	nilSettle.Restart(base)
	if nilSettle.Active(base) {
		t.Fatal("a nil window is never active")
	}
	client, _ := NewClient(ClientConfig{Endpoint: "https://backend.example/m", CheckpointEndpoint: "https://backend.example/c"})
	shared := NewSettle(0)
	metrics, _ := NewPublisher(PublisherConfig{RuntimeID: "runtime-one", State: State.NewStore(State.NewGameState()), Client: client, Settle: shared})
	checkpoints, _ := NewCheckpointPublisher(CheckpointPublisherConfig{RuntimeID: "runtime-one", State: State.NewStore(State.NewGameState()), Client: client, Settle: shared})
	if metrics.settle != shared || checkpoints.settle != shared {
		t.Fatal("both publishers must use the one window they were given")
	}
}

// A checkpoint evaluation inside the settle window uploads even though nothing
// visible changed: the handover needs an actual upload, not a confirmed digest.
func TestCheckpointsUploadEveryEvaluationWhileSettlingEvenWhenUnchanged(t *testing.T) {
	rig := startSettleRig(t, time.Hour)
	waitUntil(t, 3*time.Second, "several unchanged checkpoints while settling", func() bool { return rig.backend.checkpoints.Load() >= 4 })
	if status := rig.checkpoints.Status(); status.UnchangedSkips != 0 {
		t.Fatalf("a settling checkpoint publisher skipped %d unchanged evaluations", status.UnchangedSkips)
	}
	waitUntil(t, 3*time.Second, "several unchanged samples while settling", func() bool { return rig.backend.metrics.Load() >= 4 })
	if status := rig.metrics.Status(); status.UnchangedSkips != 0 {
		t.Fatalf("a settling metrics publisher skipped %d unchanged evaluations", status.UnchangedSkips)
	}
}

// The handover readiness predicate (hosted/handover_executor.go), scaled: an
// actual checkpoint upload and an actual metrics upload, both published and both
// no older than the freshness bound, at the same moment.
func handoverReady(rig *settleRig, since time.Time, fresh time.Duration) bool {
	metrics, checkpoint := rig.metrics.Status(), rig.checkpoints.Status()
	within := func(at time.Time) bool { return !at.IsZero() && !at.Before(since) && time.Since(at) <= fresh }
	// An upload in flight ("publishing") does not withdraw the previous publication.
	published := func(state string) bool { return state == StatePublished || state == StatePublishing }
	return published(metrics.State) && published(checkpoint.State) &&
		within(metrics.LastPublishedAt) && within(checkpoint.LastCheckpointAt)
}

func TestLoginAfterTheSettleWindowExpiredStillMakesAHandoverReady(t *testing.T) {
	const window = 600 * time.Millisecond
	const fresh = 250 * time.Millisecond // the two-minute freshness bound, scaled
	rig := startSettleRig(t, window)
	waitUntil(t, 3*time.Second, "first uploads", func() bool { return rig.backend.metrics.Load() >= 1 && rig.backend.checkpoints.Load() >= 1 })

	// The startup window expires and the runtime goes quiet: steady state skips unchanged content.
	waitUntil(t, 3*time.Second, "a quiet steady state", func() bool { return rig.quiet(3 * window / 2) })

	// A login: the session transition restarts the window for both publishers.
	loginAt := time.Now()
	if _, err := rig.store.ApplyComponents(State.Components(State.ComponentSession), func(gameState *State.GameState) ([]string, bool, error) {
		gameState.Session.Status = "connected"
		gameState.Session.ConnectionGeneration++
		return []string{"session"}, true, nil
	}); err != nil {
		t.Fatal(err)
	}
	waitUntil(t, 3*time.Second, "a handover-ready moment after the login", func() bool { return handoverReady(rig, loginAt, fresh) })

	// It stays ready well past one freshness bound, with unchanged content: uploads
	// must keep coming, so every sample of the predicate over 1.8x the bound holds.
	end := time.Now().Add(fresh * 9 / 5)
	for time.Now().Before(end) {
		if !handoverReady(rig, loginAt, fresh) {
			t.Fatalf("readiness lapsed inside the settle window: metrics %+v checkpoint %+v", rig.metrics.Status(), rig.checkpoints.Status())
		}
		time.Sleep(5 * time.Millisecond)
	}

	// And the window closes again afterwards: steady-state savings come back.
	waitUntil(t, 5*time.Second, "steady state after the window", func() bool { return rig.quiet(3 * window / 2) })
}

func TestConfigurationApplyRestartsTheSharedWindow(t *testing.T) {
	const window = 300 * time.Millisecond
	rig := startSettleRig(t, window)
	waitUntil(t, 3*time.Second, "first uploads", func() bool { return rig.backend.metrics.Load() >= 1 && rig.backend.checkpoints.Load() >= 1 })
	waitUntil(t, 3*time.Second, "a quiet steady state", func() bool { return rig.quiet(3 * window / 2) })

	applyAt := time.Now()
	if _, err := rig.configStore.Update("scheduler", json.RawMessage(`{"minAttackDelay":9}`)); err != nil {
		t.Fatal(err)
	}
	// Both publishers upload repeatedly afterwards although the sample content did not change:
	// the window opened by the configuration apply is shared, so the metrics publisher stops skipping too.
	metricsBefore, checkpointsBefore := rig.backend.metrics.Load(), rig.backend.checkpoints.Load()
	waitUntil(t, 3*time.Second, "repeated uploads after a configuration apply", func() bool {
		return rig.backend.metrics.Load() >= metricsBefore+3 && rig.backend.checkpoints.Load() >= checkpointsBefore+3
	})
	if handoverReady(rig, applyAt, 250*time.Millisecond) == false {
		waitUntil(t, 2*time.Second, "readiness after the apply", func() bool { return handoverReady(rig, applyAt, 250*time.Millisecond) })
	}
}
