package PrivateMetrics

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"CitadelDesktop/Server/Configuration"
	"CitadelDesktop/Server/State"
)

// countingBackend counts actual uploads on both endpoints.
type countingBackend struct {
	client      *http.Client
	metrics     atomic.Int64
	checkpoints atomic.Int64
}

type countingRoundTripper func(*http.Request) (*http.Response, error)

func (transport countingRoundTripper) RoundTrip(request *http.Request) (*http.Response, error) {
	return transport(request)
}

func newCountingBackend(t *testing.T) *countingBackend {
	t.Helper()
	backend := &countingBackend{}
	backend.client = &http.Client{Transport: countingRoundTripper(func(request *http.Request) (*http.Response, error) {
		_, _ = readAll(request)
		status := http.StatusNoContent
		switch request.URL.Path {
		case "/metrics":
			backend.metrics.Add(1)
		case "/checkpoints":
			backend.checkpoints.Add(1)
		default:
			status = http.StatusNotFound
		}
		return &http.Response{StatusCode: status, Header: make(http.Header), Body: io.NopCloser(strings.NewReader("")), Request: request}, nil
	})}
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
// settle cadence every 25 ms stand for one minute each; callers scale the
// three-minute window. The heartbeats and steady checkpoint interval are an hour.
func startSettleRig(t *testing.T, window time.Duration) *settleRig {
	t.Helper()
	backend := newCountingBackend(t)
	client, err := NewClient(ClientConfig{
		Client: backend.client, Endpoint: "https://backend.example/metrics",
		CheckpointEndpoint: "https://backend.example/checkpoints", ClientVersion: "test",
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
	settle.RequestSample()
	if !settle.TakeSampleRequest() || settle.TakeSampleRequest() {
		t.Fatal("a sample request must be taken exactly once")
	}
	nilSettle.RequestSample()
	if nilSettle.TakeSampleRequest() {
		t.Fatal("a nil window returned a sample request")
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

func changeSession(t *testing.T, store *State.Store, change func(*State.GameState)) {
	t.Helper()
	if _, err := store.ApplyComponents(State.Components(State.ComponentSession), func(state *State.GameState) ([]string, bool, error) {
		change(state)
		return []string{"session"}, true, nil
	}); err != nil {
		t.Fatal(err)
	}
}

func waitForSteady(t *testing.T, rig *settleRig, window time.Duration) {
	t.Helper()
	waitUntil(t, 3*time.Second, "a quiet steady state", func() bool {
		return !rig.settle.Active(time.Now()) && rig.quiet(window)
	})
}

func TestSettleTriggerFollowsTheSampleGate(t *testing.T) {
	view := readyPrivateMetricsState(t, time.Now().UTC()).ReadOnlyView()
	want := settleKey{4, view.Session.ConnectionGeneration, view.Session.Generation}
	if got := settleTrigger(4, view); got != want {
		t.Fatalf("ready trigger = %+v, want %+v", got, want)
	}
	for _, test := range []struct {
		name   string
		epoch  uint64
		mutate func(*State.GameState)
	}{
		{"no placement", 0, nil},
		{"login failure", 4, func(v *State.GameState) { v.Session.LoginFailure = &State.LoginFailure{} }},
		{"unbound identity", 4, func(v *State.GameState) { v.Account.BoundAt = time.Time{} }},
		{"world mismatch", 4, func(v *State.GameState) { v.Account.WorldID = "other.example" }},
		{"not logged in", 4, func(v *State.GameState) { v.Session.LoggedIn = false }},
		{"socket not ready", 4, func(v *State.GameState) { v.Session.SocketReady = false }},
		{"generation zero", 4, func(v *State.GameState) { v.Session.Generation = 0 }},
		{"baseline mismatch", 4, func(v *State.GameState) { v.Session.BaselineGeneration++ }},
		{"connection zero", 4, func(v *State.GameState) { v.Session.ConnectionGeneration = 0 }},
	} {
		t.Run(test.name, func(t *testing.T) {
			changed := view
			if test.mutate != nil {
				test.mutate(&changed)
			}
			if got := settleTrigger(test.epoch, changed); got != (settleKey{}) {
				t.Fatalf("trigger = %+v, want zero", got)
			}
		})
	}
	for _, test := range []struct {
		name   string
		epoch  uint64
		mutate func(*State.GameState)
	}{
		{"epoch", 5, nil},
		{"connection", 4, func(v *State.GameState) { v.Session.ConnectionGeneration++ }},
		{"generation", 4, func(v *State.GameState) { v.Session.Generation++; v.Session.BaselineGeneration++ }},
	} {
		t.Run(test.name, func(t *testing.T) {
			changed := view
			if test.mutate != nil {
				test.mutate(&changed)
			}
			if got := settleTrigger(test.epoch, changed); got == want || got == (settleKey{}) {
				t.Fatalf("changed trigger = %+v", got)
			}
		})
	}
}

func TestDelayedLoginAndSlowBaselineOpenTheWindowAtReadiness(t *testing.T) {
	const window = 150 * time.Millisecond
	const fresh = 100 * time.Millisecond // the two-minute freshness bound, scaled
	rig := startSettleRig(t, window)
	waitUntil(t, 3*time.Second, "first uploads", func() bool { return rig.backend.metrics.Load() >= 1 && rig.backend.checkpoints.Load() >= 1 })
	waitForSteady(t, rig, window)
	metricsBefore, checkpointsBefore := rig.backend.metrics.Load(), rig.backend.checkpoints.Load()
	changeSession(t, rig.store, func(v *State.GameState) {
		v.Session.Status = "released"
		v.Session.LoggedIn = false
		v.Session.SocketReady = false
	})
	waitUntil(t, time.Second, "drop checkpoint", func() bool { return rig.backend.checkpoints.Load() == checkpointsBefore+1 })
	time.Sleep(2*window + 20*time.Millisecond)
	if rig.backend.metrics.Load() != metricsBefore || rig.settle.Active(time.Now()) || rig.backend.checkpoints.Load() != checkpointsBefore+1 {
		t.Fatalf("drop opened settling or uploaded samples: metrics=%d checkpoints=%d", rig.backend.metrics.Load()-metricsBefore, rig.backend.checkpoints.Load()-checkpointsBefore)
	}
	changeSession(t, rig.store, func(v *State.GameState) {
		v.Session.Status = "connected"
		v.Session.LoggedIn = true
		v.Session.SocketReady = true
		v.Session.Generation++
		v.Session.ConnectionGeneration++
	})
	waitUntil(t, time.Second, "slow-baseline session checkpoint", func() bool { return rig.backend.checkpoints.Load() == checkpointsBefore+2 })
	time.Sleep(2*window + 20*time.Millisecond)
	if rig.backend.metrics.Load() != metricsBefore || rig.settle.Active(time.Now()) {
		t.Fatal("generation ahead of baseline opened settling or uploaded a sample")
	}
	readyAt := time.Now()
	changeSession(t, rig.store, func(v *State.GameState) { v.Session.BaselineGeneration = v.Session.Generation })
	waitUntil(t, 3*time.Second, "a handover-ready moment after baseline", func() bool { return handoverReady(rig, readyAt, fresh) })
	if !rig.settle.Active(time.Now()) {
		t.Fatal("sample readiness did not open the window")
	}
	end := time.Now().Add(fresh * 9 / 5)
	for time.Now().Before(end) {
		if !handoverReady(rig, readyAt, fresh) {
			t.Fatalf("readiness lapsed inside the settle window: metrics %+v checkpoint %+v", rig.metrics.Status(), rig.checkpoints.Status())
		}
		time.Sleep(5 * time.Millisecond)
	}

	// And the window closes again afterwards: steady-state savings come back.
	waitForSteady(t, rig, window)
}

func TestFailedLoginCyclesNeverOpenTheWindow(t *testing.T) {
	const window = 100 * time.Millisecond
	rig := startSettleRig(t, window)
	waitUntil(t, 3*time.Second, "first uploads", func() bool { return rig.backend.metrics.Load() >= 1 && rig.backend.checkpoints.Load() >= 1 })
	waitForSteady(t, rig, window)
	changeSession(t, rig.store, func(v *State.GameState) { v.Session.LoggedIn = false; v.Session.SocketReady = false })
	waitUntil(t, time.Second, "drop checkpoint", func() bool { return rig.checkpoints.Status().LastReason == CheckpointReasonSession })
	metricsBefore := rig.backend.metrics.Load()
	for cycle := 0; cycle < 5; cycle++ {
		before := rig.backend.checkpoints.Load()
		changeSession(t, rig.store, func(v *State.GameState) {
			v.Session.ConnectionGeneration++
			v.Session.Status = "login-failed"
			v.Session.LoginFailure = &State.LoginFailure{}
		})
		time.Sleep(3 * 25 * time.Millisecond)
		if got := rig.backend.checkpoints.Load() - before; got > 1 {
			t.Fatalf("cycle %d uploaded %d checkpoints", cycle, got)
		}
		if rig.backend.metrics.Load() != metricsBefore || rig.settle.Active(time.Now()) {
			t.Fatalf("failed login cycle %d opened window or uploaded sample", cycle)
		}
	}
}

func TestSuccessfulReconnectOpensOneShortWindow(t *testing.T) {
	const window = 125 * time.Millisecond
	rig := startSettleRig(t, window)
	waitUntil(t, 3*time.Second, "first uploads", func() bool { return rig.backend.metrics.Load() >= 1 && rig.backend.checkpoints.Load() >= 1 })
	waitForSteady(t, rig, window)
	changeSession(t, rig.store, func(v *State.GameState) {
		v.Session.LoggedIn = false
		v.Session.SocketReady = false
		v.Session.Status = "released"
	})
	waitUntil(t, time.Second, "drop checkpoint", func() bool { return rig.checkpoints.Status().LastReason == CheckpointReasonSession })
	before := rig.backend.checkpoints.Load()
	readyAt := time.Now()
	changeSession(t, rig.store, func(v *State.GameState) {
		v.Session.LoggedIn = true
		v.Session.SocketReady = true
		v.Session.Status = "connected"
		v.Session.ConnectionGeneration++
	})
	waitUntil(t, time.Second, "reconnect readiness", func() bool { return handoverReady(rig, readyAt, 100*time.Millisecond) })
	if !rig.settle.Active(time.Now()) {
		t.Fatal("reconnect did not open settle window")
	}
	waitForSteady(t, rig, window)
	if got := rig.backend.checkpoints.Load() - before; got > int64(window/(25*time.Millisecond))+2 {
		t.Fatalf("reconnect uploaded %d checkpoints", got)
	}
}

func TestConfigurationApplyDoesNotOpenTheWindow(t *testing.T) {
	const window = 100 * time.Millisecond
	rig := startSettleRig(t, window)
	waitUntil(t, 3*time.Second, "first uploads", func() bool { return rig.backend.metrics.Load() >= 1 && rig.backend.checkpoints.Load() >= 1 })
	waitForSteady(t, rig, window)

	metricsBefore, checkpointsBefore := rig.backend.metrics.Load(), rig.backend.checkpoints.Load()
	if _, err := rig.configStore.Update("scheduler", json.RawMessage(`{"minAttackDelay":9}`)); err != nil {
		t.Fatal(err)
	}
	waitUntil(t, time.Second, "configuration checkpoint and paired sample", func() bool {
		return rig.backend.metrics.Load() == metricsBefore+1 && rig.backend.checkpoints.Load() == checkpointsBefore+1
	})
	if rig.settle.Active(time.Now()) || !rig.quiet(3*window) {
		t.Fatal("configuration apply opened a window or repeated uploads")
	}
}

func TestNewEpochOpensTheWindowOnlyForAReadySession(t *testing.T) {
	const window = 100 * time.Millisecond
	rig := startSettleRig(t, window)
	waitUntil(t, 3*time.Second, "first uploads", func() bool { return rig.backend.metrics.Load() >= 1 && rig.backend.checkpoints.Load() >= 1 })
	waitForSteady(t, rig, window)
	changeSession(t, rig.store, func(v *State.GameState) {
		v.Session.LoggedIn = false
		v.Session.SocketReady = false
		v.Session.Status = "released"
	})
	waitUntil(t, time.Second, "drop checkpoint", func() bool { return rig.checkpoints.Status().LastReason == CheckpointReasonSession })
	metricsBefore, checkpointsBefore := rig.backend.metrics.Load(), rig.backend.checkpoints.Load()
	placement := testPlacement(time.Now().UTC(), 5, 20, strings.Repeat("e", 48))
	if err := rig.metrics.SetPlacement(placement); err != nil {
		t.Fatal(err)
	}
	if err := rig.checkpoints.SetPlacement(placement); err != nil {
		t.Fatal(err)
	}
	waitUntil(t, time.Second, "new-epoch checkpoint while not ready", func() bool { return rig.backend.checkpoints.Load() == checkpointsBefore+1 })
	time.Sleep(2*window + 20*time.Millisecond)
	if rig.backend.metrics.Load() != metricsBefore || rig.settle.Active(time.Now()) {
		t.Fatal("bare new epoch opened the window or uploaded a sample")
	}
	changeSession(t, rig.store, func(v *State.GameState) {
		v.Session.LoggedIn = true
		v.Session.SocketReady = true
		v.Session.Status = "connected"
		v.Session.ConnectionGeneration++
	})
	waitUntil(t, time.Second, "ready new-epoch window", func() bool { return rig.settle.Active(time.Now()) })
	waitForSteady(t, rig, window)
	metricsBefore, checkpointsBefore = rig.backend.metrics.Load(), rig.backend.checkpoints.Load()
	renewal := testPlacement(time.Now().UTC(), 5, 21, strings.Repeat("f", 48))
	if err := rig.metrics.SetPlacement(renewal); err != nil {
		t.Fatal(err)
	}
	if err := rig.checkpoints.SetPlacement(renewal); err != nil {
		t.Fatal(err)
	}
	if rig.settle.Active(time.Now()) || !rig.quiet(2*window) ||
		rig.backend.metrics.Load() != metricsBefore || rig.backend.checkpoints.Load() != checkpointsBefore {
		t.Fatal("same-epoch renewal reopened the window or uploaded")
	}
	readyEpoch := testPlacement(time.Now().UTC(), 6, 30, strings.Repeat("g", 48))
	if err := rig.metrics.SetPlacement(readyEpoch); err != nil {
		t.Fatal(err)
	}
	if err := rig.checkpoints.SetPlacement(readyEpoch); err != nil {
		t.Fatal(err)
	}
	waitUntil(t, time.Second, "new epoch arriving while ready", func() bool { return rig.settle.Active(time.Now()) })
}

func TestEveryCheckpointUploadRequestsOneSample(t *testing.T) {
	rig := startSettleRig(t, time.Nanosecond)
	waitUntil(t, 3*time.Second, "first uploads", func() bool { return rig.backend.metrics.Load() >= 1 && rig.backend.checkpoints.Load() >= 1 })
	waitForSteady(t, rig, 5*15*time.Millisecond)
	before := rig.backend.metrics.Load()
	if err := rig.checkpoints.Checkpoint(context.Background(), CheckpointReasonSession); err != nil {
		t.Fatal(err)
	}
	waitUntil(t, 2*15*time.Millisecond+time.Second/10, "paired sample", func() bool { return rig.backend.metrics.Load() == before+1 })
	if !rig.quiet(5*15*time.Millisecond) || rig.backend.metrics.Load() != before+1 {
		t.Fatal("one checkpoint caused repeated samples")
	}
	// Isolate the pending-request case from the checkpoint publisher's own
	// mandatory session checkpoint on recovery.
	backend := newCountingBackend(t)
	client, err := NewClient(ClientConfig{Client: backend.client, Endpoint: "https://backend.example/metrics", ClientVersion: "test"})
	if err != nil {
		t.Fatal(err)
	}
	store := readyPrivateMetricsState(t, time.Now().UTC())
	settle := NewSettle(time.Nanosecond)
	metrics, err := NewPublisher(PublisherConfig{
		RuntimeID: "runtime-one", State: store, Client: client,
		Placement: testPlacement(time.Now().UTC(), 4, 10, strings.Repeat("p", 48)),
		Interval:  15 * time.Millisecond, Heartbeat: time.Hour, Settle: settle, Debounce: time.Millisecond,
		Jitter: func() float64 { return 0 },
	})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go metrics.Run(ctx)
	waitUntil(t, time.Second, "metrics-only first sample", func() bool { return backend.metrics.Load() == 1 })
	time.Sleep(5 * 15 * time.Millisecond)
	changeSession(t, store, func(v *State.GameState) {
		v.Session.LoggedIn = false
		v.Session.SocketReady = false
		v.Session.Status = "released"
	})
	waitUntil(t, time.Second, "metrics waiting on dropped session", func() bool {
		return metrics.Status().State == StateWaitingForRuntime
	})
	settle.RequestSample()
	before = backend.metrics.Load()
	time.Sleep(5 * 15 * time.Millisecond)
	settle.mu.Lock()
	requested := settle.sampleRequested
	settle.mu.Unlock()
	if backend.metrics.Load() != before || !requested {
		t.Fatal("unready session consumed a sample request")
	}
	changeSession(t, store, func(v *State.GameState) {
		v.Session.LoggedIn = true
		v.Session.SocketReady = true
		v.Session.Status = "connected"
	})
	waitUntil(t, time.Second, "sample after readiness", func() bool { return backend.metrics.Load() == before+1 })
	time.Sleep(5 * 15 * time.Millisecond)
	if settle.TakeSampleRequest() || backend.metrics.Load() != before+1 {
		t.Fatal("ready session did not consume exactly one sample request")
	}
}

func TestSameEpochRenewalKeepsPublishedStates(t *testing.T) {
	rig := startSettleRig(t, time.Nanosecond)
	waitUntil(t, 3*time.Second, "published states", func() bool {
		return rig.metrics.Status().State == StatePublished && rig.checkpoints.Status().State == StatePublished
	})
	waitForSteady(t, rig, 5*15*time.Millisecond)
	metricsBefore, checkpointsBefore := rig.backend.metrics.Load(), rig.backend.checkpoints.Load()
	renewal := testPlacement(time.Now().UTC(), 4, 11, strings.Repeat("r", 48))
	if err := rig.metrics.SetPlacement(renewal); err != nil {
		t.Fatal(err)
	}
	if err := rig.checkpoints.SetPlacement(renewal); err != nil {
		t.Fatal(err)
	}
	if rig.metrics.Status().State != StatePublished || rig.checkpoints.Status().State != StatePublished {
		t.Fatal("same-epoch renewal withdrew a published state immediately")
	}
	time.Sleep(3*2*time.Millisecond + 20*time.Millisecond)
	if rig.metrics.Status().State != StatePublished || rig.checkpoints.Status().State != StatePublished ||
		rig.backend.metrics.Load() != metricsBefore || rig.backend.checkpoints.Load() != checkpointsBefore {
		t.Fatal("same-epoch renewal withdrew a published state or uploaded")
	}
	newEpoch := testPlacement(time.Now().UTC(), 5, 20, strings.Repeat("n", 48))
	if err := rig.metrics.SetPlacement(newEpoch); err != nil {
		t.Fatal(err)
	}
	if err := rig.checkpoints.SetPlacement(newEpoch); err != nil {
		t.Fatal(err)
	}
	if rig.metrics.Status().State != StateWaitingForRuntime || rig.checkpoints.Status().State != StateWaitingForRuntime {
		t.Fatal("new epoch did not reset both states to waiting-for-runtime")
	}
	waitUntil(t, time.Second, "new-epoch published states", func() bool {
		return rig.metrics.Status().State == StatePublished && rig.checkpoints.Status().State == StatePublished &&
			rig.backend.metrics.Load() > metricsBefore && rig.backend.checkpoints.Load() > checkpointsBefore
	})
}
