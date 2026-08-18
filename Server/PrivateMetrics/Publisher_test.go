package PrivateMetrics

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"CitadelDesktop/Server/State"
)

type receivedPublication struct {
	authorization  string
	idempotencyKey string
	body           []byte
	request        PublishRequest
	receivedAt     time.Time
}

func TestPrivateMetricsClientIsDisabledWithoutExplicitEndpoint(t *testing.T) {
	client, err := NewClient(ClientConfig{})
	if err != nil {
		t.Fatal(err)
	}
	if client != nil {
		t.Fatal("private metrics client enabled without an explicit endpoint")
	}
}

// publicationServer records every publication and answers with the status
// returned by respond, which sees the 1-based request ordinal.
func publicationServer(t *testing.T, respond func(ordinal int, request *http.Request, writer http.ResponseWriter)) (*httptest.Server, <-chan receivedPublication) {
	t.Helper()
	received := make(chan receivedPublication, 32)
	var ordinal atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		body, _ := io.ReadAll(io.LimitReader(request.Body, 2<<20))
		var payload PublishRequest
		if err := json.Unmarshal(body, &payload); err != nil {
			t.Errorf("decode publication: %v", err)
			writer.WriteHeader(http.StatusBadRequest)
			return
		}
		received <- receivedPublication{
			authorization:  request.Header.Get("Authorization"),
			idempotencyKey: request.Header.Get("Idempotency-Key"), body: body, request: payload,
			receivedAt: time.Now(),
		}
		respond(int(ordinal.Add(1)), request, writer)
	}))
	t.Cleanup(server.Close)
	return server, received
}

func testPlacement(now time.Time, epoch uint64, revision uint64, token string) *Placement {
	return &Placement{
		CellID: "cell-one", TenantID: "tenant-one", RuntimeID: "runtime-one",
		PlacementEpoch: epoch, DesiredRevision: revision, LeaseExpiresAt: now.Add(10 * time.Minute),
		Grant: Grant{Token: token, ExpiresAt: now.Add(9 * time.Minute)},
	}
}

func startPublisher(t *testing.T, server *httptest.Server, config PublisherConfig) *Publisher {
	t.Helper()
	client, err := NewClient(ClientConfig{Client: server.Client(), Endpoint: server.URL, ClientVersion: "test"})
	if err != nil {
		t.Fatal(err)
	}
	config.Client = client
	if config.RuntimeID == "" {
		config.RuntimeID = "runtime-one"
	}
	if config.State == nil {
		config.State = readyPrivateMetricsState(t, time.Now().UTC())
	}
	if config.Jitter == nil {
		config.Jitter = func() float64 { return 0 }
	}
	publisher, err := NewPublisher(config)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go publisher.Run(ctx)
	return publisher
}

func TestPublisherScopesCredentialsAndRotatesPlacementEpoch(t *testing.T) {
	server, received := publicationServer(t, func(_ int, _ *http.Request, writer http.ResponseWriter) {
		writer.WriteHeader(http.StatusNoContent)
	})
	now := time.Now().UTC()
	firstToken := strings.Repeat("a", 48)
	publisher := startPublisher(t, server, PublisherConfig{
		Placement: testPlacement(now, 4, 10, firstToken),
		Interval:  100 * time.Millisecond, Debounce: time.Millisecond,
	})

	first := awaitPublication(t, received)
	if first.authorization != "Bearer "+firstToken || first.request.PlacementEpoch != 4 ||
		first.request.RuntimeID != "runtime-one" || first.request.TenantID != "tenant-one" {
		t.Fatalf("first publication = %+v", first)
	}
	if first.idempotencyKey == "" || first.idempotencyKey != first.request.Sample.SampleID {
		t.Fatalf("idempotency key = %q, sample = %q", first.idempotencyKey, first.request.Sample.SampleID)
	}
	if bytes.Contains(first.body, []byte(firstToken)) || bytes.Contains(bytes.ToLower(first.body), []byte("authorization")) {
		t.Fatalf("publication body exposed its grant: %s", first.body)
	}

	secondToken := strings.Repeat("b", 48)
	if err := publisher.SetPlacement(testPlacement(now, 5, 11, secondToken)); err != nil {
		t.Fatal(err)
	}
	// Every publication after rotation must carry the new grant and fencing
	// labels; nothing may still be labeled with the previous placement.
	rotated := awaitPublicationMatching(t, received, func(publication receivedPublication) bool {
		return publication.request.PlacementEpoch == 5
	})
	if rotated.authorization != "Bearer "+secondToken || rotated.request.DesiredRevision != 11 ||
		rotated.request.Sample.SampleID == first.request.Sample.SampleID {
		t.Fatalf("rotated publication = %+v", rotated)
	}
	if bytes.Contains(rotated.body, []byte(firstToken)) || bytes.Contains(rotated.body, []byte(secondToken)) {
		t.Fatal("rotated publication body exposed a private metrics grant")
	}
	for _, later := range drainPublications(received, 250*time.Millisecond) {
		if later.authorization != "Bearer "+secondToken || later.request.PlacementEpoch != 5 {
			t.Fatalf("publication after rotation still used the old placement: %+v", later)
		}
	}
}

func TestPublisherRejectsCrossRuntimeAndExpiredPlacement(t *testing.T) {
	client, err := NewClient(ClientConfig{Endpoint: "https://backend.example/internal/private-metrics"})
	if err != nil {
		t.Fatal(err)
	}
	publisher, err := NewPublisher(PublisherConfig{
		RuntimeID: "runtime-one", State: readyPrivateMetricsState(t, time.Now().UTC()), Client: client,
	})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	for _, placement := range []Placement{
		{
			CellID: "cell-one", TenantID: "tenant-one", RuntimeID: "runtime-two",
			PlacementEpoch: 1, DesiredRevision: 1, LeaseExpiresAt: now.Add(time.Minute),
			Grant: Grant{Token: strings.Repeat("a", 48), ExpiresAt: now.Add(time.Minute)},
		},
		{
			CellID: "cell-one", TenantID: "tenant-one", RuntimeID: "runtime-one",
			PlacementEpoch: 1, DesiredRevision: 1, LeaseExpiresAt: now.Add(-time.Minute),
			Grant: Grant{Token: strings.Repeat("a", 48), ExpiresAt: now.Add(-time.Minute)},
		},
	} {
		if err := publisher.SetPlacement(&placement); err == nil {
			t.Fatalf("SetPlacement(%+v) succeeded", placement)
		}
	}
}

func TestClientDoesNotExposeBackendErrorBody(t *testing.T) {
	secretBody := "private backend detail should not escape"
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Retry-After", "120")
		writer.WriteHeader(http.StatusInternalServerError)
		_, _ = writer.Write([]byte(secretBody))
	}))
	defer server.Close()
	client, err := NewClient(ClientConfig{Client: server.Client(), Endpoint: server.URL})
	if err != nil {
		t.Fatal(err)
	}
	err = client.Upload(t.Context(), Placement{Grant: Grant{Token: strings.Repeat("a", 48)}}, Sample{SampleID: "sample"})
	if err == nil || strings.Contains(err.Error(), secretBody) {
		t.Fatalf("Upload error = %v", err)
	}
	var publishErr *PublishError
	if !errors.As(err, &publishErr) || publishErr.Outcome != OutcomeTransient ||
		publishErr.StatusCode != http.StatusInternalServerError || publishErr.RetryAfter != 2*time.Minute {
		t.Fatalf("publish error = %+v", publishErr)
	}
}

func TestClientClassifiesBackendOutcomesWithoutTrustingErrorText(t *testing.T) {
	tests := []struct {
		status  int
		code    string
		outcome UploadOutcome
		want    string
	}{
		{status: http.StatusUnauthorized, code: "grant_expired", outcome: OutcomeUnauthorized, want: "grant_expired"},
		{status: http.StatusForbidden, code: "placement_fenced", outcome: OutcomeUnauthorized, want: "placement_fenced"},
		{status: http.StatusConflict, code: "stale_placement_epoch", outcome: OutcomeRejected, want: "stale_placement_epoch"},
		{status: http.StatusUnprocessableEntity, code: "<script>alert(1)</script>", outcome: OutcomeRejected, want: ""},
		{status: http.StatusTooManyRequests, code: "slow_down", outcome: OutcomeTransient, want: "slow_down"},
		{status: http.StatusBadGateway, code: strings.Repeat("x", 80), outcome: OutcomeTransient, want: ""},
	}
	for _, test := range tests {
		t.Run(http.StatusText(test.status), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
				writer.Header().Set("Content-Type", "application/json")
				writer.WriteHeader(test.status)
				_ = json.NewEncoder(writer).Encode(map[string]any{"error": map[string]string{"code": test.code}})
			}))
			defer server.Close()
			client, err := NewClient(ClientConfig{Client: server.Client(), Endpoint: server.URL})
			if err != nil {
				t.Fatal(err)
			}
			err = client.Upload(t.Context(), Placement{Grant: Grant{Token: strings.Repeat("a", 48)}}, Sample{SampleID: "sample"})
			var publishErr *PublishError
			if !errors.As(err, &publishErr) || publishErr.Outcome != test.outcome || publishErr.Code != test.want ||
				OutcomeOf(err) != test.outcome {
				t.Fatalf("Upload error = %v (%+v)", err, publishErr)
			}
		})
	}
	if OutcomeOf(errors.New("local failure")) != OutcomeTransient {
		t.Fatal("unknown errors must stay transient so valid samples are not discarded")
	}
}

func TestPublisherReplaysIdenticalSampleAfterTransientFailureThenResumesCadence(t *testing.T) {
	server, received := publicationServer(t, func(ordinal int, _ *http.Request, writer http.ResponseWriter) {
		if ordinal <= 2 {
			writer.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		writer.WriteHeader(http.StatusNoContent)
	})
	publisher := startPublisher(t, server, PublisherConfig{
		Placement: testPlacement(time.Now().UTC(), 4, 10, strings.Repeat("a", 48)),
		Interval:  40 * time.Millisecond, Debounce: time.Millisecond,
	})
	first := awaitPublication(t, received)
	if status := publisher.Status(); status.State != StateRetrying && status.State != StatePublishing {
		// The status may still read "publishing" if the goroutine has not
		// processed the response yet; give it a moment.
		waitForStatus(t, publisher, StateRetrying)
	}
	second := awaitPublication(t, received)
	third := awaitPublication(t, received)
	for index, retry := range []receivedPublication{second, third} {
		if retry.idempotencyKey != first.idempotencyKey || !bytes.Equal(retry.body, first.body) {
			t.Fatalf("retry %d changed the sample: key %q vs %q", index+2, retry.idempotencyKey, first.idempotencyKey)
		}
	}
	waitForStatus(t, publisher, StatePublished)
	if status := publisher.Status(); status.ConsecutiveFailures != 0 || status.LastError != "" || status.LastPublishedAt.IsZero() {
		t.Fatalf("recovered status = %+v", status)
	}
	fourth := awaitPublication(t, received)
	if fourth.idempotencyKey == first.idempotencyKey {
		t.Fatal("cadence publication after recovery reused the delivered sample id")
	}
	if !fourth.request.Sample.ObservedAt.After(first.request.Sample.ObservedAt) {
		t.Fatalf("fresh sample was not newer: %v vs %v", fourth.request.Sample.ObservedAt, first.request.Sample.ObservedAt)
	}
}

func TestPublisherDropsDurablyRejectedSampleAndPublishesFreshOne(t *testing.T) {
	server, received := publicationServer(t, func(ordinal int, _ *http.Request, writer http.ResponseWriter) {
		if ordinal == 1 {
			writer.Header().Set("Content-Type", "application/json")
			writer.WriteHeader(http.StatusUnprocessableEntity)
			_, _ = writer.Write([]byte(`{"error":{"code":"sample_invalid"}}`))
			return
		}
		writer.WriteHeader(http.StatusNoContent)
	})
	publisher := startPublisher(t, server, PublisherConfig{
		Placement: testPlacement(time.Now().UTC(), 4, 10, strings.Repeat("a", 48)),
		Interval:  40 * time.Millisecond, Debounce: time.Millisecond,
	})
	rejected := awaitPublication(t, received)
	waitForStatus(t, publisher, StateRejected)
	if status := publisher.Status(); status.ConsecutiveFailures != 1 || !strings.Contains(status.LastError, "sample_invalid") ||
		strings.Contains(status.LastError, "Bearer") {
		t.Fatalf("rejected status = %+v", status)
	}
	fresh := awaitPublication(t, received)
	if fresh.idempotencyKey == rejected.idempotencyKey || bytes.Equal(fresh.body, rejected.body) {
		t.Fatal("publisher replayed a durably rejected sample instead of building a fresh one")
	}
	waitForStatus(t, publisher, StatePublished)
}

func TestPublisherStopsSpendingRefusedGrantUntilPlacementRotates(t *testing.T) {
	refusedToken := strings.Repeat("a", 48)
	server, received := publicationServer(t, func(_ int, request *http.Request, writer http.ResponseWriter) {
		if request.Header.Get("Authorization") == "Bearer "+refusedToken {
			writer.WriteHeader(http.StatusUnauthorized)
			return
		}
		writer.WriteHeader(http.StatusNoContent)
	})
	now := time.Now().UTC()
	interval := 40 * time.Millisecond
	publisher := startPublisher(t, server, PublisherConfig{
		Placement: testPlacement(now, 4, 10, refusedToken),
		Interval:  interval, Debounce: time.Millisecond,
	})
	refused := awaitPublication(t, received)
	if refused.authorization != "Bearer "+refusedToken {
		t.Fatalf("first publication = %+v", refused)
	}
	waitForStatus(t, publisher, StateGrantRejected)
	if status := publisher.Status(); status.NextAttemptAt.Before(status.LastAttemptAt.Add(interval * maximumBackoffFactor)) {
		t.Fatalf("refused grant was rescheduled too soon: %+v", status)
	}
	if extra := drainPublications(received, 4*interval); len(extra) != 0 {
		t.Fatalf("publisher kept spending a refused grant: %d extra publications", len(extra))
	}
	// State changes must not bypass the refused-grant hold either.
	if _, err := publisher.state.ApplyComponents(State.Components(State.ComponentPlayer), func(state *State.GameState) ([]string, bool, error) {
		state.Player.Might++
		return []string{"player"}, true, nil
	}); err != nil {
		t.Fatal(err)
	}
	if extra := drainPublications(received, 3*interval); len(extra) != 0 {
		t.Fatalf("state change bypassed the refused-grant hold: %d publications", len(extra))
	}
	rotatedToken := strings.Repeat("b", 48)
	if err := publisher.SetPlacement(testPlacement(now, 5, 11, rotatedToken)); err != nil {
		t.Fatal(err)
	}
	rotated := awaitPublication(t, received)
	if rotated.authorization != "Bearer "+rotatedToken || rotated.request.PlacementEpoch != 5 {
		t.Fatalf("rotated publication = %+v", rotated)
	}
	waitForStatus(t, publisher, StatePublished)
	if status := publisher.Status(); status.ConsecutiveFailures != 0 || status.LastError != "" {
		t.Fatalf("status after rotation = %+v", status)
	}
}

func TestPlacementRotationFollowsCadenceInsteadOfBursting(t *testing.T) {
	server, received := publicationServer(t, func(_ int, _ *http.Request, writer http.ResponseWriter) {
		writer.WriteHeader(http.StatusNoContent)
	})
	now := time.Now().UTC()
	interval := 400 * time.Millisecond
	publisher := startPublisher(t, server, PublisherConfig{
		Placement: testPlacement(now, 4, 10, strings.Repeat("a", 48)),
		Interval:  interval, Debounce: time.Millisecond,
	})
	first := awaitPublication(t, received)
	rotatedToken := strings.Repeat("b", 48)
	if err := publisher.SetPlacement(testPlacement(now, 5, 11, rotatedToken)); err != nil {
		t.Fatal(err)
	}
	if burst := drainPublications(received, interval/3); len(burst) != 0 {
		t.Fatalf("placement rotation burst %d publications outside the cadence", len(burst))
	}
	next := awaitPublication(t, received)
	if next.authorization != "Bearer "+rotatedToken || next.request.PlacementEpoch != 5 {
		t.Fatalf("cadence publication after rotation = %+v", next)
	}
	if elapsed := next.receivedAt.Sub(first.receivedAt); elapsed < interval-interval/10 {
		t.Fatalf("publication after rotation arrived after %s, before the %s cadence", elapsed, interval)
	}
}

func TestPublisherPausesOnLapsedLeaseAndResumesOnRenewal(t *testing.T) {
	server, received := publicationServer(t, func(_ int, _ *http.Request, writer http.ResponseWriter) {
		writer.WriteHeader(http.StatusNoContent)
	})
	now := time.Now().UTC()
	firstToken := strings.Repeat("a", 48)
	interval := 40 * time.Millisecond
	publisher := startPublisher(t, server, PublisherConfig{
		Placement: &Placement{
			CellID: "cell-one", TenantID: "tenant-one", RuntimeID: "runtime-one",
			PlacementEpoch: 4, DesiredRevision: 10, LeaseExpiresAt: now.Add(300 * time.Millisecond),
			Grant: Grant{Token: firstToken, ExpiresAt: now.Add(250 * time.Millisecond)},
		},
		Interval: interval, Debounce: time.Millisecond,
	})
	first := awaitPublication(t, received)
	if first.authorization != "Bearer "+firstToken {
		t.Fatalf("first publication = %+v", first)
	}
	// Once the lease and grant lapse the publisher stops spending the grant and
	// simply waits; the runtime itself is untouched (it is not this package's
	// concern) and local history keeps accumulating.
	waitForStatus(t, publisher, StateWaitingForPlacement)
	for _, earlier := range drainPublications(received, 20*time.Millisecond) {
		if earlier.authorization != "Bearer "+firstToken {
			t.Fatalf("publication before the lapse used an unexpected grant: %+v", earlier)
		}
	}
	if late := drainPublications(received, 5*interval); len(late) != 0 {
		t.Fatalf("publisher kept publishing with a lapsed placement: %d publications", len(late))
	}
	renewedToken := strings.Repeat("b", 48)
	if err := publisher.SetPlacement(testPlacement(time.Now().UTC(), 4, 11, renewedToken)); err != nil {
		t.Fatal(err)
	}
	renewed := awaitPublication(t, received)
	if renewed.authorization != "Bearer "+renewedToken || renewed.request.PlacementEpoch != 4 || renewed.request.DesiredRevision != 11 {
		t.Fatalf("renewed publication = %+v", renewed)
	}
	waitForStatus(t, publisher, StatePublished)
}

func awaitPublication(t *testing.T, received <-chan receivedPublication) receivedPublication {
	t.Helper()
	select {
	case publication := <-received:
		return publication
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for private metrics publication")
		return receivedPublication{}
	}
}

func awaitPublicationMatching(t *testing.T, received <-chan receivedPublication, matches func(receivedPublication) bool) receivedPublication {
	t.Helper()
	deadline := time.After(5 * time.Second)
	for {
		select {
		case publication := <-received:
			if matches(publication) {
				return publication
			}
		case <-deadline:
			t.Fatal("timed out waiting for a matching private metrics publication")
			return receivedPublication{}
		}
	}
}

func drainPublications(received <-chan receivedPublication, window time.Duration) []receivedPublication {
	deadline := time.After(window)
	var publications []receivedPublication
	for {
		select {
		case publication := <-received:
			publications = append(publications, publication)
		case <-deadline:
			return publications
		}
	}
}

func waitForStatus(t *testing.T, publisher *Publisher, state string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if publisher.Status().State == state {
			return
		}
		time.Sleep(2 * time.Millisecond)
	}
	t.Fatalf("publisher never reached %q: %+v", state, publisher.Status())
}
