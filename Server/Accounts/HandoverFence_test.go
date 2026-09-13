package Accounts

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"CitadelDesktop/Server/Configuration"
	"CitadelDesktop/Server/Runtime"
)

func handoverSourceFixture(t *testing.T) (*Supervisor, *TenantAuthenticator, *Orchestrator, Runtime.ProfileTransferIdentity, RuntimeAssignment) {
	t.Helper()
	supervisor, auth, orchestrator, now := newTestOrchestrator(t)
	snapshot := Configuration.Snapshot{SchemaVersion: 1, Revision: 39, UpdatedAt: now, Sections: map[string]json.RawMessage{"scheduler": json.RawMessage(`{"minAttackDelay":9}`)}}
	_, digest, err := canonicalConfigurationDigest(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	assignment := testAssignment("alpha", "tenant-one", 4, now.Add(10*time.Minute))
	assignment.DesiredConfigurationRevision = snapshot.Revision
	assignment.DesiredConfigurationDigest = digest
	if _, err := orchestrator.Reconcile(t.Context(), ReconcileRequest{SchemaVersion: 1, Revision: 1, Runtimes: []RuntimeAssignment{assignment}}); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(orchestrator.Handler())
	defer server.Close()
	response, body := controlRequest(t, http.MethodPut, server.URL+"/orchestrator/v1/runtimes/alpha/configuration", ConfigurationSyncRequest{SchemaVersion: 1, PlacementEpoch: 4, Digest: digest, Configuration: snapshot})
	if response.StatusCode != http.StatusOK {
		t.Fatalf("sync failed: %d %s", response.StatusCode, body)
	}
	identity := Runtime.ProfileTransferIdentity{OperationID: "operation-one", AccountID: "account-one", RuntimeID: "alpha", TenantID: "tenant-one", SourceCellID: "cell-one", SourceEpoch: 4, ConfigurationRevision: 39, ConfigurationDigest: digest}
	return supervisor, auth, orchestrator, identity, assignment
}

func TestSourceHandoverFencesBeforeExportAndSurvivesRestart(t *testing.T) {
	supervisor, auth, orchestrator, identity, assignment := handoverSourceFixture(t)
	token := strings.Repeat("d", 48)
	if err := auth.SetDashboardGrant("alpha", token, time.Now().Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	application, _ := supervisor.Application("alpha")
	originalDir := application.DataDir
	fence, err := orchestrator.PrepareSourceHandover(t.Context(), identity)
	if err != nil {
		t.Fatal(err)
	}
	if !fence.SourceStopped || fence.ProfileID == "" || fence.Identity != identity {
		t.Fatalf("invalid fence: %+v", fence)
	}
	if _, running := supervisor.Application("alpha"); running {
		t.Fatal("source still running")
	}
	if _, err := os.Stat(originalDir); err != nil {
		t.Fatal("source backup removed")
	}
	request := httptest.NewRequest(http.MethodGet, "/accounts/alpha/api/v2/state", nil)
	request.Header.Set("Authorization", "Bearer "+token)
	if _, ok := auth.Authenticate(request); ok {
		t.Fatal("old dashboard grant survived fence")
	}
	replayed, err := orchestrator.PrepareSourceHandover(t.Context(), identity)
	if err != nil || replayed != fence {
		t.Fatalf("retry changed fence: %v", err)
	}
	assignment.PlacementEpoch += 100
	if _, err := orchestrator.Reconcile(t.Context(), ReconcileRequest{SchemaVersion: 1, Revision: 100, Runtimes: []RuntimeAssignment{assignment}}); err == nil {
		t.Fatal("higher epoch resurrected retired source")
	}
	var archive bytes.Buffer
	receipt, err := Runtime.WriteProfileArchive(t.Context(), originalDir, &archive, identity)
	if err != nil || receipt.ProfileID != fence.ProfileID {
		t.Fatalf("stopped profile cannot be exported: %v", err)
	}
	if err := supervisor.Close(t.Context()); err != nil {
		t.Fatal(err)
	}
	config := supervisor.config
	config.RuntimeContext = context.Background()
	restarted, err := New(context.Background(), config)
	if err != nil {
		t.Fatal(err)
	}
	defer restarted.Close(t.Context())
	if _, err := restarted.AddAccount(t.Context(), AccountConfig{ID: "alpha"}); err == nil {
		t.Fatal("process restart lost durable fence")
	}
	loaded, ok := restarted.sourceFence("alpha")
	if !ok || loaded != fence {
		t.Fatal("fence changed on restart")
	}
}

func TestSourceHandoverRejectsWrongIdentityBeforeStopping(t *testing.T) {
	for _, field := range []string{"tenant", "runtime", "cell", "epoch", "configuration", "digest", "path"} {
		t.Run(field, func(t *testing.T) {
			supervisor, _, orchestrator, identity, _ := handoverSourceFixture(t)
			before, _ := supervisor.Application("alpha")
			switch field {
			case "tenant":
				identity.TenantID = "other"
			case "runtime":
				identity.RuntimeID = "other"
			case "cell":
				identity.SourceCellID = "other"
			case "epoch":
				identity.SourceEpoch++
			case "configuration":
				identity.ConfigurationRevision++
			case "digest":
				identity.ConfigurationDigest = strings.Repeat("f", 64)
			case "path":
				identity.OperationID = "../escape"
			}
			if _, err := orchestrator.PrepareSourceHandover(t.Context(), identity); err == nil {
				t.Fatal("invalid source accepted")
			}
			after, running := supervisor.Application("alpha")
			if !running || before != after {
				t.Fatal("invalid request stopped source")
			}
			if _, exists := supervisor.sourceFence("alpha"); exists {
				t.Fatal("invalid request fenced source")
			}
		})
	}
}

func TestSourceHandoverBlocksStaleControlRequestsAndDifferentOperations(t *testing.T) {
	_, _, orchestrator, identity, _ := handoverSourceFixture(t)
	if _, err := orchestrator.PrepareSourceHandover(t.Context(), identity); err != nil {
		t.Fatal(err)
	}
	identity.OperationID = "other-operation"
	if _, err := orchestrator.PrepareSourceHandover(t.Context(), identity); err == nil {
		t.Fatal("different operation reused retired source")
	}
	for _, route := range []struct{ method, path string }{{http.MethodPost, "reconnect"}, {http.MethodPut, "login"}, {http.MethodPut, "configuration"}, {http.MethodPut, "dashboard-grant"}, {http.MethodPut, "dashboard-bootstrap"}} {
		request := httptest.NewRequest(route.method, "/orchestrator/v1/runtimes/alpha/"+route.path, strings.NewReader(`{}`))
		request.Header.Set("Authorization", "Bearer "+testControlToken)
		response := httptest.NewRecorder()
		orchestrator.Handler().ServeHTTP(response, request)
		if response.Code != http.StatusLocked {
			t.Fatalf("stale %s returned %d", route.path, response.Code)
		}
	}
}

func TestSourceHandoverKeepsSiblingRunning(t *testing.T) {
	supervisor, _, orchestrator, identity, alpha := handoverSourceFixture(t)
	bravo := testAssignment("bravo", "tenant-two", 1, alpha.LeaseExpiresAt)
	if _, err := orchestrator.Reconcile(t.Context(), ReconcileRequest{SchemaVersion: 1, Revision: 2, Runtimes: []RuntimeAssignment{alpha, bravo}}); err != nil {
		t.Fatal(err)
	}
	before, _ := supervisor.Application("bravo")
	if _, err := orchestrator.PrepareSourceHandover(t.Context(), identity); err != nil {
		t.Fatal(err)
	}
	after, running := supervisor.Application("bravo")
	if !running || before != after {
		t.Fatal("handover disturbed sibling")
	}
	if _, err := orchestrator.Reconcile(t.Context(), ReconcileRequest{SchemaVersion: 1, Revision: 3, Runtimes: []RuntimeAssignment{bravo}}); err != nil {
		t.Fatal(err)
	}
}

func TestSourceFenceBlocksAliasOfPlayerProfile(t *testing.T) {
	supervisor, _, orchestrator, identity, _ := handoverSourceFixture(t)
	if _, err := orchestrator.PrepareSourceHandover(t.Context(), identity); err != nil {
		t.Fatal(err)
	}
	supervisor.mu.Lock()
	fence := supervisor.sourceFences["alpha"]
	fence.ProfileDirectory = "Players/shared-player"
	if err := supervisor.saveSourceFenceLocked("alpha", fence); err != nil {
		supervisor.mu.Unlock()
		t.Fatal(err)
	}
	supervisor.playerBindings["alias"] = "shared-player"
	supervisor.mu.Unlock()
	if _, err := supervisor.AddAccount(t.Context(), AccountConfig{ID: "alias"}); err == nil {
		t.Fatal("runtime alias reopened retired player profile")
	}
}

func TestCorruptSourceFenceJournalPreventsStartup(t *testing.T) {
	for _, raw := range []string{`{`, `{"schemaVersion":2,"fences":{}}`, `{"schemaVersion":1,"fences":null}`, `{"schemaVersion":1,"fences":{}} trailing`} {
		t.Run(raw, func(t *testing.T) {
			directory := t.TempDir()
			if err := os.Mkdir(filepath.Join(directory, "Accounts"), 0700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(directory, "Accounts", sourceFenceFile), []byte(raw), 0600); err != nil {
				t.Fatal(err)
			}
			if _, err := New(t.Context(), Config{DataRoot: directory, Offline: true}); err == nil {
				t.Fatal("corrupt fences were ignored")
			}
		})
	}
}
