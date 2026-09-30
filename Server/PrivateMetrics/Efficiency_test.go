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
	"sync/atomic"
	"testing"
	"time"

	"CitadelDesktop/Server/State"
)

func setAutomation(t *testing.T, store *State.Store, nextCheck time.Time, status string) {
	t.Helper()
	if _, err := store.ApplyComponents(State.Components(State.ComponentAutomations), func(state *State.GameState) ([]string, bool, error) {
		next := nextCheck
		state.Automations["autoTower"] = State.AutomationState{
			ID: "autoTower", Enabled: true, Status: status, NextCheckAt: &next, UpdatedAt: nextCheck,
		}
		return []string{"automation"}, true, nil
	}); err != nil {
		t.Fatal(err)
	}
}

func setMight(t *testing.T, store *State.Store, might float64) {
	t.Helper()
	if _, err := store.ApplyComponents(State.Components(State.ComponentPlayer), func(state *State.GameState) ([]string, bool, error) {
		state.Player.Might = might
		return []string{"player"}, true, nil
	}); err != nil {
		t.Fatal(err)
	}
}

func buildCheckpointFor(t *testing.T, store *State.Store, at time.Time) Checkpoint {
	t.Helper()
	checkpoint, err := BuildCheckpoint(context.Background(), store, nil, nil, CheckpointReasonCadence, at)
	if err != nil {
		t.Fatal(err)
	}
	return checkpoint
}

func TestCheckpointDigestIgnoresVolatileFieldsAndTracksVisibleContent(t *testing.T) {
	now := time.Now().UTC()
	store := readyPrivateMetricsState(t, now)
	setAutomation(t, store, now.Add(time.Minute), "running")
	first := buildCheckpointFor(t, store, now)

	// Same content later: only the observation time differs.
	if checkpointDigest(buildCheckpointFor(t, store, now.Add(time.Hour))) != checkpointDigest(first) {
		t.Fatal("observation time changed the digest")
	}
	// A re-evaluated automation moves its next check and update time and bumps
	// the state revision, but shows the same dashboard content.
	setAutomation(t, store, now.Add(2*time.Minute), "running")
	moved := buildCheckpointFor(t, store, now)
	if moved.StateRevision == first.StateRevision {
		t.Fatal("test premise: the state revision should have moved")
	}
	if checkpointDigest(moved) != checkpointDigest(first) {
		t.Fatal("volatile automation timestamps or the revision changed the digest")
	}
	// Anything the dashboard shows changes it.
	setAutomation(t, store, now.Add(2*time.Minute), "blocked")
	if checkpointDigest(buildCheckpointFor(t, store, now)) == checkpointDigest(first) {
		t.Fatal("an automation status change did not change the digest")
	}
	setAutomation(t, store, now.Add(2*time.Minute), "running")
	setMight(t, store, 99999)
	if checkpointDigest(buildCheckpointFor(t, store, now)) == checkpointDigest(first) {
		t.Fatal("a player change did not change the digest")
	}
	withSession := first
	withSession.Session.State = "released"
	if checkpointDigest(withSession) == checkpointDigest(first) {
		t.Fatal("a session change did not change the digest")
	}
	withOperations := first
	withOperations.Operations = json.RawMessage(`[{"id":"op-1"}]`)
	if checkpointDigest(withOperations) == checkpointDigest(first) {
		t.Fatal("new operation receipts did not change the digest")
	}
}

type efficiencyCheckpointRig struct {
	store     *State.Store
	publisher *CheckpointPublisher
	received  <-chan receivedCheckpoint
	now       time.Time
}

func startEfficiencyCheckpoints(t *testing.T, config CheckpointPublisherConfig) *efficiencyCheckpointRig {
	t.Helper()
	server, received := checkpointServer(t)
	client, err := NewClient(ClientConfig{
		Client: server.Client(), Endpoint: server.URL + "/metrics", CheckpointEndpoint: server.URL + "/checkpoints", ClientVersion: "test",
	})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	store := readyPrivateMetricsState(t, now)
	config.RuntimeID, config.State, config.Client = "runtime-one", store, client
	if config.Placement == nil {
		config.Placement = testPlacement(now, 4, 10, strings.Repeat("c", 48))
	}
	if config.Debounce == 0 {
		config.Debounce = 5 * time.Millisecond
	}
	config.Jitter = func() float64 { return 0 }
	publisher, err := NewCheckpointPublisher(config)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go publisher.Run(ctx)
	return &efficiencyCheckpointRig{store: store, publisher: publisher, received: received, now: now}
}

func noCheckpointWithin(t *testing.T, received <-chan receivedCheckpoint, window time.Duration, why string) {
	t.Helper()
	select {
	case checkpoint := <-received:
		t.Fatalf("unexpected checkpoint (%s): reason %q revision %d", why, checkpoint.request.Checkpoint.Reason, checkpoint.request.Checkpoint.StateRevision)
	case <-time.After(window):
	}
}

func TestCheckpointPublisherSkipsUnchangedContentUntilHeartbeat(t *testing.T) {
	rig := startEfficiencyCheckpoints(t, CheckpointPublisherConfig{
		Interval: 15 * time.Millisecond, Heartbeat: 600 * time.Millisecond, SettleWindow: time.Nanosecond,
	})
	first := awaitCheckpoint(t, rig.received)
	_ = awaitCheckpointPublished(t, rig.publisher, first.request.Checkpoint.StateRevision)

	// Revisions move without visible change: evaluated every interval, never sent.
	for step := 1; step <= 5; step++ {
		setAutomation(t, rig.store, rig.now.Add(time.Duration(step)*time.Minute), "running")
		if step == 1 {
			// The first automation entry is new visible content.
			awaitCheckpoint(t, rig.received)
			continue
		}
		noCheckpointWithin(t, rig.received, 60*time.Millisecond, "volatile automation timestamps")
	}
	if status := rig.publisher.Status(); status.UnchangedSkips == 0 || status.LastUnchangedAt.IsZero() {
		t.Fatalf("status did not record unchanged evaluations: %+v", status)
	}

	// A visible change goes out on the next cadence evaluation.
	setMight(t, rig.store, 4242)
	changed := awaitCheckpoint(t, rig.received)
	if changed.request.Checkpoint.Reason != CheckpointReasonCadence {
		t.Fatalf("content-change checkpoint reason = %q", changed.request.Checkpoint.Reason)
	}

	// Nothing changes any more: the heartbeat still refreshes the backend copy.
	heartbeatFrom := time.Now()
	beat := awaitCheckpoint(t, rig.received)
	if waited := time.Since(heartbeatFrom); waited < 400*time.Millisecond {
		t.Fatalf("heartbeat arrived after %v, before the configured 600ms window", waited)
	}
	if beat.request.Checkpoint.CheckpointID == changed.request.Checkpoint.CheckpointID {
		t.Fatal("heartbeat reused the previous checkpoint id")
	}
}

func TestCheckpointPlacementRenewalDoesNotCheckpointButNewEpochDoes(t *testing.T) {
	rig := startEfficiencyCheckpoints(t, CheckpointPublisherConfig{
		Interval: time.Hour, Heartbeat: 2 * time.Hour, SettleWindow: time.Nanosecond,
	})
	first := awaitCheckpoint(t, rig.received)
	_ = awaitCheckpointPublished(t, rig.publisher, first.request.Checkpoint.StateRevision)

	// The controller renews lease and grant of the same epoch every few
	// minutes; that alone must not rebuild or upload the dashboard.
	for renewal := 0; renewal < 3; renewal++ {
		if err := rig.publisher.SetPlacement(testPlacement(rig.now, 4, uint64(11+renewal), strings.Repeat(string(rune('d'+renewal)), 48))); err != nil {
			t.Fatal(err)
		}
		noCheckpointWithin(t, rig.received, 60*time.Millisecond, "same-epoch placement renewal")
	}
	// A new epoch is a real fence change (a handover waits for a checkpoint
	// under it), so it publishes once.
	if err := rig.publisher.SetPlacement(testPlacement(rig.now, 5, 20, strings.Repeat("z", 48))); err != nil {
		t.Fatal(err)
	}
	moved := awaitCheckpoint(t, rig.received)
	if moved.request.PlacementEpoch != 5 {
		t.Fatalf("checkpoint after epoch change carried epoch %d", moved.request.PlacementEpoch)
	}
	noCheckpointWithin(t, rig.received, 60*time.Millisecond, "second evaluation of the same epoch")
}

func TestCheckpointCadenceFollowsSettleWindowThenSteadyInterval(t *testing.T) {
	settling := startEfficiencyCheckpoints(t, CheckpointPublisherConfig{
		Interval: time.Hour, SettleInterval: 40 * time.Millisecond, SettleWindow: time.Hour,
	})
	first := awaitCheckpoint(t, settling.received)
	status := awaitCheckpointPublished(t, settling.publisher, first.request.Checkpoint.StateRevision)
	if next := status.NextAttemptAt.Sub(status.LastCheckpointAt); next > time.Second {
		t.Fatalf("next attempt while settling is %v away, want the settle interval", next)
	}
	steady := startEfficiencyCheckpoints(t, CheckpointPublisherConfig{
		Interval: time.Hour, SettleInterval: 40 * time.Millisecond, SettleWindow: time.Nanosecond,
	})
	first = awaitCheckpoint(t, steady.received)
	status = awaitCheckpointPublished(t, steady.publisher, first.request.Checkpoint.StateRevision)
	if next := status.NextAttemptAt.Sub(status.LastCheckpointAt); next < 50*time.Minute {
		t.Fatalf("next attempt in steady state is only %v away, want about the steady interval", next)
	}
}

func TestCheckpointDefaultsAreTheAgreedCadence(t *testing.T) {
	client, err := NewClient(ClientConfig{Endpoint: "https://backend.example/m", CheckpointEndpoint: "https://backend.example/c"})
	if err != nil {
		t.Fatal(err)
	}
	publisher, err := NewCheckpointPublisher(CheckpointPublisherConfig{RuntimeID: "runtime-one", State: State.NewStore(State.NewGameState()), Client: client})
	if err != nil {
		t.Fatal(err)
	}
	if publisher.interval != 15*time.Minute || publisher.heartbeat != 30*time.Minute || publisher.retry != 5*time.Minute ||
		publisher.settleEvery != time.Minute || publisher.settle.window != 10*time.Minute {
		t.Fatalf("defaults = interval %v heartbeat %v retry %v settle %v/%v",
			publisher.interval, publisher.heartbeat, publisher.retry, publisher.settleEvery, publisher.settle.window)
	}
	metrics, err := NewPublisher(PublisherConfig{RuntimeID: "runtime-one", State: State.NewStore(State.NewGameState()), Client: client})
	if err != nil {
		t.Fatal(err)
	}
	if metrics.interval != time.Minute || metrics.heartbeat != 5*time.Minute || metrics.settle.window != 10*time.Minute {
		t.Fatalf("metrics defaults = interval %v heartbeat %v settle %v", metrics.interval, metrics.heartbeat, metrics.settle.window)
	}
}

// gzipMembers splits a body into its gzip members without reading past them.
func gzipMembers(t *testing.T, body []byte) [][]byte {
	t.Helper()
	source := bytes.NewReader(body)
	reader, err := gzip.NewReader(source)
	if err != nil {
		t.Fatal(err)
	}
	var members [][]byte
	for {
		reader.Multistream(false)
		plain, err := io.ReadAll(reader)
		if err != nil {
			t.Fatal(err)
		}
		members = append(members, plain)
		if source.Len() == 0 {
			return members
		}
		if err := reader.Reset(source); err != nil {
			t.Fatal(err)
		}
	}
}

func TestCheckpointRequestUsesTheSplitGzipLayout(t *testing.T) {
	var captured []byte
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		captured, _ = io.ReadAll(request.Body)
		writer.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(server.Close)
	client, err := NewClient(ClientConfig{Client: server.Client(), Endpoint: server.URL + "/m", CheckpointEndpoint: server.URL + "/c"})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	checkpoint := buildCheckpointFor(t, readyPrivateMetricsState(t, now), now)
	checkpoint.CheckpointID = "split-check"
	if err := client.UploadCheckpoint(context.Background(), *testPlacement(now, 4, 10, strings.Repeat("c", 48)), checkpoint); err != nil {
		t.Fatal(err)
	}
	members := gzipMembers(t, captured)
	if len(members) != 3 || !bytes.HasSuffix(members[0], []byte(`"checkpoint":`)) || strings.TrimSpace(string(members[2])) != "}" {
		t.Fatalf("layout = %d members; head %q tail %q", len(members), members[0], members[len(members)-1])
	}
	whole := bytes.Join(members, nil)
	var request CheckpointRequest
	if err := json.Unmarshal(whole, &request); err != nil || request.Checkpoint.CheckpointID != "split-check" || request.PlacementEpoch != 4 {
		t.Fatalf("concatenated members are not the checkpoint request: %v %+v", err, request)
	}
	expected, _ := json.Marshal(checkpoint)
	if !bytes.Equal(members[1], expected) {
		t.Fatal("the middle member is not exactly the checkpoint document")
	}
	// A gzip-aware reader that does not know the layout sees ordinary JSON.
	reader, err := gzip.NewReader(bytes.NewReader(captured))
	if err != nil {
		t.Fatal(err)
	}
	plain, _ := io.ReadAll(reader)
	if !bytes.Equal(plain, whole) {
		t.Fatal("default multistream decoding differs from the member concatenation")
	}
}

func TestSampleDigestIgnoresVolatileFieldsOnly(t *testing.T) {
	now := time.Now().UTC()
	sample, err := NewSampleBuilder(readyPrivateMetricsState(t, now), nil, nil).Build(context.Background(), now)
	if err != nil {
		t.Fatal(err)
	}
	base := sampleDigest(sample)
	later := sample
	later.SampleID, later.ObservedAt, later.StateRevision = "other", now.Add(time.Hour), sample.StateRevision+50
	if len(sample.Features.EventScores) == 0 {
		t.Fatal("test premise: the ready state has an event score")
	}
	later.Features.EventScores = append([]EventScoreMetrics(nil), sample.Features.EventScores...)
	later.Features.EventScores[0].ObservedAt = now.Add(time.Hour)
	if sampleDigest(later) != base {
		t.Fatal("id, observation time, revision or score observation time changed the digest")
	}
	if sample.Features.EventScores[0].ObservedAt.Equal(now.Add(time.Hour)) {
		t.Fatal("digest computation mutated the caller's event scores")
	}
	changed := sample
	changed.Player.Might++
	if sampleDigest(changed) == base {
		t.Fatal("a player change did not change the digest")
	}
	rescored := sample
	rescored.Features.EventScores = append([]EventScoreMetrics(nil), sample.Features.EventScores...)
	rescored.Features.EventScores[0].PlayerScore++
	if sampleDigest(rescored) == base {
		t.Fatal("an event score change did not change the digest")
	}
	aligned := sample
	aligned.Features.EventScores = append([]EventScoreMetrics(nil), sample.Features.EventScores...)
	aligned.Features.EventScores[0].OccurrenceEndsAt = now.Truncate(time.Hour).Add(time.Hour + 10*time.Second)
	alignedBase := sampleDigest(aligned)
	jittered := aligned
	jittered.Features.EventScores = append([]EventScoreMetrics(nil), aligned.Features.EventScores...)
	jittered.Features.EventScores[0].OccurrenceEndsAt = aligned.Features.EventScores[0].OccurrenceEndsAt.Add(2 * time.Second)
	if sampleDigest(jittered) != alignedBase {
		t.Fatal("second-level jitter of the derived event end changed the digest")
	}
	jittered.Features.EventScores[0].OccurrenceEndsAt = aligned.Features.EventScores[0].OccurrenceEndsAt.Add(2 * time.Hour)
	if sampleDigest(jittered) == alignedBase {
		t.Fatal("a new event end did not change the digest")
	}
	reconnected := sample
	reconnected.ConnectionGeneration++
	if sampleDigest(reconnected) == base {
		t.Fatal("a new connection generation did not change the digest")
	}
}

func TestPublisherSkipsUnchangedSamplesUntilHeartbeat(t *testing.T) {
	server, received := publicationServer(t, func(_ int, _ *http.Request, writer http.ResponseWriter) {
		writer.WriteHeader(http.StatusNoContent)
	})
	store := readyPrivateMetricsState(t, time.Now().UTC())
	// Align the score's observation to a minute boundary plus ten seconds so a
	// one-second refresh stays inside the same minute of the derived event end.
	if _, err := store.ApplyComponents(State.Components(State.ComponentEventScores), func(state *State.GameState) ([]string, bool, error) {
		score := state.EventScores.ByEvent[71]
		score.ObservedAt = time.Now().UTC().Truncate(time.Hour).Add(10 * time.Second)
		state.SetScalableEventScore(71, score)
		return []string{"scores"}, true, nil
	}); err != nil {
		t.Fatal(err)
	}
	publisher := startPublisher(t, server, PublisherConfig{
		State: store, Placement: testPlacement(time.Now().UTC(), 4, 10, strings.Repeat("a", 48)),
		Interval: 10 * time.Millisecond, Heartbeat: time.Second, SettleWindow: time.Nanosecond, Debounce: time.Millisecond,
	})
	first := awaitPublication(t, received)
	if first.encoding != "gzip" {
		t.Fatalf("sample was not gzip-compressed: %q", first.encoding)
	}
	if late := drainPublications(received, 300*time.Millisecond); len(late) != 0 {
		t.Fatalf("%d unchanged samples were uploaded before the heartbeat", len(late))
	}
	status := publisher.Status()
	if status.UnchangedSkips == 0 || status.LastUnchangedAt.IsZero() || status.State != StatePublished {
		t.Fatalf("status did not record skips: %+v", status)
	}

	// A score's observation time (and the second-level jitter it puts on the
	// derived event end) is volatile; its value is content.
	if _, err := store.ApplyComponents(State.Components(State.ComponentEventScores), func(state *State.GameState) ([]string, bool, error) {
		score := state.EventScores.ByEvent[71]
		score.ObservedAt = score.ObservedAt.Add(time.Second)
		state.SetScalableEventScore(71, score)
		return []string{"scores"}, true, nil
	}); err != nil {
		t.Fatal(err)
	}
	if late := drainPublications(received, 150*time.Millisecond); len(late) != 0 {
		t.Fatalf("a score observation time uploaded %d samples", len(late))
	}
	setMight(t, store, 777)
	changed := awaitPublication(t, received)
	if changed.request.Sample.Player.Might != 777 || changed.request.Sample.SampleID == first.request.Sample.SampleID {
		t.Fatalf("changed sample = %+v", changed.request.Sample.Player)
	}
	// Then quiet again: exactly a heartbeat later, one more sample.
	from := time.Now()
	beat := awaitPublication(t, received)
	if waited := time.Since(from); waited < 700*time.Millisecond {
		t.Fatalf("heartbeat after %v, want about 1s", waited)
	}
	if beat.request.Sample.SampleID == changed.request.Sample.SampleID || beat.request.Sample.Player.Might != 777 {
		t.Fatalf("heartbeat sample = %+v", beat.request.Sample)
	}
}

func TestPublisherUploadsEveryEvaluationWhileSettling(t *testing.T) {
	server, received := publicationServer(t, func(_ int, _ *http.Request, writer http.ResponseWriter) {
		writer.WriteHeader(http.StatusNoContent)
	})
	startPublisher(t, server, PublisherConfig{
		Placement: testPlacement(time.Now().UTC(), 4, 10, strings.Repeat("a", 48)),
		Interval:  20 * time.Millisecond, Heartbeat: time.Hour, SettleWindow: time.Hour, Debounce: time.Millisecond,
	})
	_ = awaitPublication(t, received)
	if later := drainPublications(received, 300*time.Millisecond); len(later) < 3 {
		t.Fatalf("only %d samples while settling; a fresh placement must keep uploading each evaluation", len(later))
	}
}

func TestPublisherLeaseRenewalDoesNotRestartSettleWindow(t *testing.T) {
	server, received := publicationServer(t, func(_ int, _ *http.Request, writer http.ResponseWriter) {
		writer.WriteHeader(http.StatusNoContent)
	})
	now := time.Now().UTC()
	publisher := startPublisher(t, server, PublisherConfig{
		Placement: testPlacement(now, 4, 10, strings.Repeat("a", 48)),
		Interval:  10 * time.Millisecond, Heartbeat: time.Hour, SettleWindow: 80 * time.Millisecond, Debounce: time.Millisecond,
	})
	_ = awaitPublication(t, received)
	time.Sleep(200 * time.Millisecond) // the window ends; unchanged samples stop
	_ = drainPublications(received, 50*time.Millisecond)
	for renewal := 0; renewal < 3; renewal++ {
		if err := publisher.SetPlacement(testPlacement(now, 4, uint64(11+renewal), strings.Repeat("b", 48))); err != nil {
			t.Fatal(err)
		}
	}
	if late := drainPublications(received, 300*time.Millisecond); len(late) > 1 {
		t.Fatalf("lease renewals of the same epoch resumed uploading %d unchanged samples", len(late))
	}
}

func TestClientGzipsSamplesAndFallsBackToPlainForAnOlderBackend(t *testing.T) {
	var gzipAttempts, plainAttempts atomic.Int64
	var keys []string
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		body, _ := io.ReadAll(request.Body)
		keys = append(keys, request.Header.Get("Idempotency-Key"))
		if request.Header.Get("Content-Encoding") == "gzip" {
			gzipAttempts.Add(1)
			// The former backend fails to parse a gzip body as JSON.
			writer.Header().Set("Content-Type", "application/json")
			writer.WriteHeader(http.StatusUnprocessableEntity)
			_, _ = writer.Write([]byte(`{"error":{"code":"unsupported_schema"}}`))
			return
		}
		plainAttempts.Add(1)
		var payload PublishRequest
		if err := json.Unmarshal(body, &payload); err != nil {
			writer.WriteHeader(http.StatusBadRequest)
			return
		}
		writer.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(server.Close)
	client, err := NewClient(ClientConfig{Client: server.Client(), Endpoint: server.URL, ClientVersion: "test"})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	placement := *testPlacement(now, 4, 10, strings.Repeat("a", 48))
	sample, err := NewSampleBuilder(readyPrivateMetricsState(t, now), nil, nil).Build(context.Background(), now)
	if err != nil {
		t.Fatal(err)
	}
	sample.SampleID = "sample-one"
	if err := client.Upload(context.Background(), placement, sample); err != nil {
		t.Fatalf("upload against an older backend = %v", err)
	}
	if gzipAttempts.Load() != 1 || plainAttempts.Load() != 1 || keys[0] != "sample-one" || keys[1] != "sample-one" {
		t.Fatalf("gzip=%d plain=%d keys=%v", gzipAttempts.Load(), plainAttempts.Load(), keys)
	}
	sample.SampleID = "sample-two"
	if err := client.Upload(context.Background(), placement, sample); err != nil || gzipAttempts.Load() != 1 || plainAttempts.Load() != 2 {
		t.Fatalf("second upload retried gzip inside the fallback window: %v gzip=%d plain=%d", err, gzipAttempts.Load(), plainAttempts.Load())
	}
	// After the fallback window the client offers gzip again.
	client.plainUntil.Store(0)
	sample.SampleID = "sample-three"
	if err := client.Upload(context.Background(), placement, sample); err != nil || gzipAttempts.Load() != 2 || plainAttempts.Load() != 3 {
		t.Fatalf("third upload: %v gzip=%d plain=%d", err, gzipAttempts.Load(), plainAttempts.Load())
	}
}

func TestClientDoesNotFallBackToPlainOnAuthorizationFailures(t *testing.T) {
	var attempts atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		attempts.Add(1)
		writer.WriteHeader(http.StatusUnauthorized)
	}))
	t.Cleanup(server.Close)
	client, err := NewClient(ClientConfig{Client: server.Client(), Endpoint: server.URL})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	sample, err := NewSampleBuilder(readyPrivateMetricsState(t, now), nil, nil).Build(context.Background(), now)
	if err != nil {
		t.Fatal(err)
	}
	sample.SampleID = "unauthorized"
	err = client.Upload(context.Background(), *testPlacement(now, 4, 10, strings.Repeat("a", 48)), sample)
	if OutcomeOf(err) != OutcomeUnauthorized || attempts.Load() != 1 {
		t.Fatalf("outcome %v after %d attempts", OutcomeOf(err), attempts.Load())
	}
}
