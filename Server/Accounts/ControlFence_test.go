package Accounts

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func fencedControlRequest(orchestrator *Orchestrator, epoch, token string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodPost, "/orchestrator/v1/reconcile", strings.NewReader(`{"schemaVersion":1,"revision":1,"runtimes":[]}`))
	request.Header.Set("Authorization", "Bearer "+token)
	request.Header.Set("Content-Type", "application/json")
	if epoch != "" {
		request.Header.Set(controlEpochHeader, epoch)
	}
	response := httptest.NewRecorder()
	orchestrator.Handler().ServeHTTP(response, request)
	return response
}

func TestControlFenceSurvivesRestartAndRejectsLegacyWriters(t *testing.T) {
	supervisor, auth, orchestrator, _ := newTestOrchestrator(t)
	if response := fencedControlRequest(orchestrator, "99", "wrong-token"); response.Code != http.StatusUnauthorized {
		t.Fatal(response.Code)
	}
	if orchestrator.controlEpoch.Load() != 0 {
		t.Fatal("unauthenticated request advanced fence")
	}
	if response := fencedControlRequest(orchestrator, "", testControlToken); response.Code != http.StatusOK {
		t.Fatal(response.Body.String())
	}
	if response := fencedControlRequest(orchestrator, "8", testControlToken); response.Code != http.StatusOK {
		t.Fatal(response.Body.String())
	}
	if status := orchestrator.Status(); status.ControlEpoch != 8 || status.ControlFenceSchema != 1 {
		t.Fatal("missing fence status")
	}
	for _, epoch := range []string{"", "7"} {
		response := fencedControlRequest(orchestrator, epoch, testControlToken)
		if response.Code != http.StatusConflict || !strings.Contains(response.Body.String(), "stale_control_epoch") {
			t.Fatalf("%q = %d %s", epoch, response.Code, response.Body.String())
		}
	}
	restarted, err := NewOrchestrator(OrchestratorConfig{CellID: "cell-one", Token: testControlToken, Supervisor: supervisor, DashboardAuth: auth})
	if err != nil {
		t.Fatal(err)
	}
	if response := fencedControlRequest(restarted, "7", testControlToken); response.Code != http.StatusConflict {
		t.Fatal("restart lost fence")
	}
	if response := fencedControlRequest(restarted, "9", testControlToken); response.Code != http.StatusOK {
		t.Fatal(response.Body.String())
	}
	// A second still-running orchestrator must re-read the durable high water.
	if response := fencedControlRequest(orchestrator, "8", testControlToken); response.Code != http.StatusConflict {
		t.Fatal("other instance ignored newer fence")
	}
}

func TestControlFenceRejectsMalformedAndCorruptState(t *testing.T) {
	supervisor, auth, orchestrator, _ := newTestOrchestrator(t)
	for _, epoch := range []string{"0", "01", "+1", "-1", " 1", "9223372036854775808", "1,2"} {
		if response := fencedControlRequest(orchestrator, epoch, testControlToken); response.Code != http.StatusBadRequest {
			t.Fatalf("accepted epoch %q", epoch)
		}
	}
	if response := fencedControlRequest(orchestrator, "2", testControlToken); response.Code != http.StatusOK {
		t.Fatal(response.Body.String())
	}
	path := filepath.Join(supervisor.config.DataRoot, "Accounts", controlFenceFile)
	if err := os.WriteFile(path, []byte(`{"schemaVersion":1,"cellId":"cell-one","epoch":2,"epoch":1}`), 0600); err != nil {
		t.Fatal(err)
	}
	if response := fencedControlRequest(orchestrator, "3", testControlToken); response.Code != http.StatusServiceUnavailable {
		t.Fatal("accepted corrupt journal")
	}
	if _, err := NewOrchestrator(OrchestratorConfig{CellID: "cell-one", Token: testControlToken, Supervisor: supervisor, DashboardAuth: auth}); err == nil {
		t.Fatal("startup ignored corrupt journal")
	}
}

func TestControlFenceSerializesWholeMutation(t *testing.T) {
	supervisor, auth, first, _ := newTestOrchestrator(t)
	second, err := NewOrchestrator(OrchestratorConfig{CellID: "cell-one", Token: testControlToken, Supervisor: supervisor, DashboardAuth: auth})
	if err != nil {
		t.Fatal(err)
	}
	entered := make(chan struct{})
	release := make(chan struct{})
	finished := make(chan struct{})
	go func() {
		defer close(finished)
		request := httptest.NewRequest(http.MethodPost, "/", nil)
		request.Header.Set(controlEpochHeader, "3")
		first.withControlFence(httptest.NewRecorder(), request, http.HandlerFunc(func(http.ResponseWriter, *http.Request) { close(entered); <-release }))
	}()
	<-entered
	newDone := make(chan *httptest.ResponseRecorder, 1)
	go func() { newDone <- fencedControlRequest(second, "4", testControlToken) }()
	select {
	case <-newDone:
		close(release)
		t.Fatal("new owner overlapped old mutation")
	case <-time.After(50 * time.Millisecond):
	}
	close(release)
	<-finished
	if response := <-newDone; response.Code != http.StatusOK {
		t.Fatal(response.Body.String())
	}
	if response := fencedControlRequest(first, "3", testControlToken); response.Code != http.StatusConflict {
		t.Fatal("stale mutation accepted after takeover")
	}
}
