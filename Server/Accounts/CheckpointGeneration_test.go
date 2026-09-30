package Accounts

import (
	"CitadelDesktop/Server/GameData"
	"CitadelDesktop/Server/PrivateMetrics"
	"CitadelDesktop/Server/Session"
	"compress/gzip"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestDelayedDrainCheckpointCannotUnregisterReplacement(t *testing.T) {
	entered, release := make(chan struct{}), make(chan struct{})
	var enteredOnce, releaseOnce sync.Once
	unblock := func() { releaseOnce.Do(func() { close(release) }) }
	defer unblock()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reader := io.Reader(r.Body)
		if r.Header.Get("Content-Encoding") == "gzip" {
			gz, err := gzip.NewReader(r.Body)
			if err != nil {
				w.WriteHeader(400)
				return
			}
			defer gz.Close()
			reader = gz
		}
		data, _ := io.ReadAll(reader)
		if strings.Contains(string(data), `"reason":"drain"`) {
			enteredOnce.Do(func() { close(entered) })
			select {
			case <-release:
			case <-r.Context().Done():
				return
			}
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{}`))
	}))
	defer server.Close()
	client, err := PrivateMetrics.NewClient(PrivateMetrics.ClientConfig{Client: server.Client(), Endpoint: server.URL + "/metrics", CheckpointEndpoint: server.URL + "/checkpoint"})
	if err != nil {
		t.Fatal(err)
	}
	parent, cancelParent := context.WithCancel(t.Context())
	defer cancelParent()
	manager := GameData.NewManager(GameData.UpdaterConfig{CacheDir: t.TempDir()})
	supervisor, err := New(t.Context(), Config{DataRoot: t.TempDir(), Offline: true, RuntimeContext: parent, GameData: manager, PrivateMetricsClient: client})
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		unblock()
		if err := supervisor.Close(context.Background()); err != nil {
			t.Errorf("close test supervisor: %v", err)
		}
	}()
	now := time.Now()
	placement := &PrivateMetrics.Placement{CellID: "cell", TenantID: "tenant", RuntimeID: "alpha", PlacementEpoch: 1, DesiredRevision: 1, LeaseExpiresAt: now.Add(time.Minute), Grant: PrivateMetrics.Grant{Token: strings.Repeat("x", 48), ExpiresAt: now.Add(time.Minute)}}
	old, err := supervisor.AddAccount(t.Context(), AccountConfig{ID: "alpha", BackgroundOnly: true, Transport: Session.NewUnavailableTransport(), PrivateMetricsPlacement: placement})
	if err != nil {
		t.Fatal(err)
	}
	originalDone := make(chan error, 1)
	go func() { originalDone <- supervisor.RemoveAccount(t.Context(), "alpha") }()
	select {
	case <-entered:
	case <-time.After(2 * time.Second):
		t.Fatal("original drain checkpoint did not enter")
	}
	// Supported parent cancellation can finish old application teardown while
	// RemoveAccount's independent checkpoint request remains blocked.
	cancelParent()
	// Cancellation starts the final flush immediately; wait for it to finish
	// before reusing the profile rather than imposing a wall-clock I/O budget.
	if err := old.Wait(t.Context()); err != nil {
		t.Fatal(err)
	}
	if err := supervisor.RemoveAccount(t.Context(), "alpha"); err != nil {
		t.Fatal(err)
	}
	replacement, err := supervisor.AddAccount(t.Context(), AccountConfig{ID: "alpha", BackgroundOnly: true, Transport: Session.NewUnavailableTransport()})
	if err != nil {
		t.Fatal(err)
	}
	if replacement == old {
		t.Fatal("application generation did not change")
	}
	// AddAccount currently permits a canceled parent. The replacement inherits
	// cancellation; scanner registration here tests ownership, not live gameplay.
	supervisor.worldMaps.AcquireStormScan("alpha", "world-one", 4, now)
	unblock()
	if err := <-originalDone; err != nil {
		t.Fatal(err)
	}
	if assignment := supervisor.worldMaps.AcquireStormScan("bravo", "world-one", 4, now.Add(time.Millisecond)); assignment.ParticipantCount != 2 {
		t.Fatal("delayed old checkpoint cleanup unregistered replacement scanner")
	}
}
