package Accounts

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"CitadelDesktop/Server/App"
	"CitadelDesktop/Server/PrivateMetrics"
	"CitadelDesktop/Server/State"
)

const testControlToken = "orchestrator-control-token-00000000000000000000000000000000"

func TestOrchestratorAddsRuntimeWithoutRestartingSiblings(t *testing.T) {
	supervisor, _, orchestrator, now := newTestOrchestrator(t)
	supervisor.Start()
	alphaAssignment := testAssignment("alpha", "tenant-one", 1, now.Add(10*time.Minute))
	if _, err := orchestrator.Reconcile(t.Context(), ReconcileRequest{
		SchemaVersion: 1, Revision: 1, Runtimes: []RuntimeAssignment{alphaAssignment},
	}); err != nil {
		t.Fatal(err)
	}
	alpha, exists := supervisor.Application("alpha")
	if !exists {
		t.Fatal("alpha runtime was not added")
	}
	setTestPlayer(t, alpha.State, 101, "Alpha")

	alphaAssignment.LeaseExpiresAt = now.Add(11 * time.Minute)
	bravoAssignment := testAssignment("bravo", "tenant-two", 7, now.Add(11*time.Minute))
	status, err := orchestrator.Reconcile(t.Context(), ReconcileRequest{
		SchemaVersion: 1, Revision: 2,
		Runtimes: []RuntimeAssignment{alphaAssignment, bravoAssignment},
	})
	if err != nil {
		t.Fatal(err)
	}
	alphaAfter, exists := supervisor.Application("alpha")
	if !exists || alphaAfter != alpha {
		t.Fatal("adding bravo replaced or interrupted alpha")
	}
	if player := alphaAfter.State.Snapshot().Player; player.ID != 101 || player.Name != "Alpha" {
		t.Fatalf("alpha private state changed while adding sibling: %+v", player)
	}
	bravo, exists := supervisor.Application("bravo")
	if !exists || bravo == nil {
		t.Fatal("bravo runtime was not added")
	}
	if bravo.State == alpha.State || bravo.Session == alpha.Session || bravo.API == alpha.API {
		t.Fatal("dynamic runtimes share an account-private component")
	}
	if bravo.GameData != alpha.GameData || bravo.Updates != alpha.Updates ||
		bravo.WorldIntelClient != alpha.WorldIntelClient || bravo.Ingest.Registry() != alpha.Ingest.Registry() {
		t.Fatal("dynamic runtimes did not join the process-level shared services")
	}
	if status.Capacity.Active != 2 || len(status.Runtimes) != 2 {
		t.Fatalf("dynamic status = %+v", status)
	}

	bravoAssignment.LeaseExpiresAt = now.Add(12 * time.Minute)
	if _, err := orchestrator.Reconcile(t.Context(), ReconcileRequest{
		SchemaVersion: 1, Revision: 3, Runtimes: []RuntimeAssignment{bravoAssignment},
	}); err != nil {
		t.Fatal(err)
	}
	bravoAfter, exists := supervisor.Application("bravo")
	if !exists || bravoAfter != bravo {
		t.Fatal("draining alpha interrupted bravo")
	}
	waitForRuntime(t, supervisor, "alpha", false)
}

func TestDashboardGrantCannotCrossRuntimeOrEnterControlPlane(t *testing.T) {
	supervisor, dashboardAuth, orchestrator, now := newTestOrchestrator(t)
	assignments := []RuntimeAssignment{
		testAssignment("alpha", "tenant-one", 1, now.Add(10*time.Minute)),
		testAssignment("bravo", "tenant-two", 1, now.Add(10*time.Minute)),
	}
	if _, err := orchestrator.Reconcile(t.Context(), ReconcileRequest{SchemaVersion: 1, Revision: 1, Runtimes: assignments}); err != nil {
		t.Fatal(err)
	}
	alpha, _ := supervisor.Application("alpha")
	bravo, _ := supervisor.Application("bravo")
	setTestPlayer(t, alpha.State, 101, "Alpha")
	setTestPlayer(t, bravo.State, 202, "Bravo")

	controlServer := httptest.NewServer(orchestrator.Handler())
	defer controlServer.Close()
	alphaToken := strings.Repeat("a", 48)
	putDashboardGrant(t, controlServer.URL, "alpha", 1, alphaToken, now.Add(5*time.Minute), http.StatusOK)

	runtimeServer := httptest.NewServer(supervisor.Handler(dashboardAuth, nil))
	defer runtimeServer.Close()
	state := getRuntimeState(t, runtimeServer.URL+"/accounts/alpha/api/v2/state", alphaToken, http.StatusOK)
	if state.Player.ID != 101 {
		t.Fatalf("alpha dashboard returned player %d", state.Player.ID)
	}
	_ = getRuntimeState(t, runtimeServer.URL+"/accounts/bravo/api/v2/state", alphaToken, http.StatusNotFound)
	_ = getRuntimeState(t, runtimeServer.URL+"/accounts/alpha/api/v2/state", testControlToken, http.StatusUnauthorized)

	request, _ := http.NewRequest(http.MethodGet, controlServer.URL+"/orchestrator/v1/status", nil)
	request.Header.Set("Authorization", "Bearer "+alphaToken)
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != http.StatusUnauthorized {
		t.Fatalf("dashboard token entered control plane with status %d", response.StatusCode)
	}
	response.Body.Close()

	rotated := strings.Repeat("z", 48)
	putDashboardGrant(t, controlServer.URL, "alpha", 1, rotated, now.Add(5*time.Minute), http.StatusOK)
	_ = getRuntimeState(t, runtimeServer.URL+"/accounts/alpha/api/v2/state", alphaToken, http.StatusUnauthorized)
	_ = getRuntimeState(t, runtimeServer.URL+"/accounts/alpha/api/v2/state", rotated, http.StatusOK)
	putDashboardGrant(t, controlServer.URL, "bravo", 1, rotated, now.Add(5*time.Minute), http.StatusBadRequest)
	putDashboardGrant(t, controlServer.URL, "bravo", 1, testControlToken, now.Add(5*time.Minute), http.StatusBadRequest)

	statusRequest, _ := http.NewRequest(http.MethodGet, controlServer.URL+"/orchestrator/v1/status", nil)
	statusRequest.Header.Set("Authorization", "Bearer "+testControlToken)
	statusResponse, err := http.DefaultClient.Do(statusRequest)
	if err != nil {
		t.Fatal(err)
	}
	defer statusResponse.Body.Close()
	var raw bytes.Buffer
	_, _ = raw.ReadFrom(statusResponse.Body)
	if bytes.Contains(raw.Bytes(), []byte(alphaToken)) || bytes.Contains(raw.Bytes(), []byte(rotated)) {
		t.Fatal("orchestrator status exposed a dashboard token")
	}
}

func TestOrchestratorFencesStaleRevisionsAndKeepsRuntimesThroughLeaseLapse(t *testing.T) {
	supervisor, dashboardAuth, orchestrator, clock := newTestOrchestratorWithClock(t)
	now := clock.Now()
	alpha := testAssignment("alpha", "tenant-one", 5, now.Add(5*time.Minute))
	bravo := testAssignment("bravo", "tenant-two", 2, now.Add(10*time.Minute))
	if _, err := orchestrator.Reconcile(t.Context(), ReconcileRequest{
		SchemaVersion: 1, Revision: 5, Runtimes: []RuntimeAssignment{alpha, bravo},
	}); err != nil {
		t.Fatal(err)
	}
	alphaApp, _ := supervisor.Application("alpha")
	bravoApp, _ := supervisor.Application("bravo")
	if _, err := orchestrator.Reconcile(t.Context(), ReconcileRequest{
		SchemaVersion: 1, Revision: 4, Runtimes: []RuntimeAssignment{alpha, bravo},
	}); orchestratorErrorCode(err) != "stale_desired_revision" {
		t.Fatalf("stale desired revision error = %v", err)
	}
	alpha.PlacementEpoch = 4
	if _, err := orchestrator.Reconcile(t.Context(), ReconcileRequest{
		SchemaVersion: 1, Revision: 6, Runtimes: []RuntimeAssignment{alpha, bravo},
	}); orchestratorErrorCode(err) != "stale_placement_epoch" {
		t.Fatalf("stale placement epoch error = %v", err)
	}
	alpha.PlacementEpoch = 5

	// A runtime lives for as long as the account stays in the desired set. A
	// lapsed placement lease pauses credential-bearing side channels and is
	// reported, but it never drains the runtime or interrupts its session.
	if err := dashboardAuth.SetDashboardGrant("alpha", strings.Repeat("a", 48), now.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	clock.Advance(6 * time.Minute)
	status := orchestrator.Status()
	if len(status.Runtimes) != 2 || status.Runtimes[0].RuntimeID != "alpha" ||
		status.Runtimes[0].PlacementLease != PlacementLeaseLapsed || status.Runtimes[0].Lifecycle != "running" ||
		status.Runtimes[1].PlacementLease != PlacementLeaseActive {
		t.Fatalf("status after lease lapse = %+v", status.Runtimes)
	}
	time.Sleep(20 * time.Millisecond)
	alphaAfter, exists := supervisor.Application("alpha")
	if !exists || alphaAfter != alphaApp {
		t.Fatal("lapsed lease drained or restarted the alpha runtime")
	}
	if _, valid := dashboardAuth.authenticateToken(strings.Repeat("a", 48)); !valid {
		t.Fatal("lapsed placement lease revoked a dashboard grant that has not reached its own expiry")
	}

	// Renewing the lease through an ordinary reconcile clears the lapse and
	// reuses the same application; nothing is reconstructed.
	renewed := clock.Now()
	alpha.LeaseExpiresAt = renewed.Add(5 * time.Minute)
	bravo.LeaseExpiresAt = renewed.Add(10 * time.Minute)
	if _, err := orchestrator.Reconcile(t.Context(), ReconcileRequest{
		SchemaVersion: 1, Revision: 7, Runtimes: []RuntimeAssignment{alpha, bravo},
	}); err != nil {
		t.Fatal(err)
	}
	status = orchestrator.Status()
	if status.Runtimes[0].PlacementLease != PlacementLeaseActive {
		t.Fatalf("renewed lease still lapsed: %+v", status.Runtimes[0])
	}
	if current, _ := supervisor.Application("alpha"); current != alphaApp {
		t.Fatal("lease renewal reconstructed the alpha runtime")
	}

	// Only omission from the desired set drains a runtime, and only that one.
	if _, err := orchestrator.Reconcile(t.Context(), ReconcileRequest{
		SchemaVersion: 1, Revision: 8, Runtimes: []RuntimeAssignment{bravo},
	}); err != nil {
		t.Fatal(err)
	}
	waitForRuntime(t, supervisor, "alpha", false)
	bravoAfter, exists := supervisor.Application("bravo")
	if !exists || bravoAfter != bravoApp {
		t.Fatal("draining alpha interrupted bravo")
	}
	if _, valid := dashboardAuth.authenticateToken(strings.Repeat("a", 48)); valid {
		t.Fatal("drained runtime dashboard grant remained valid")
	}
}

func TestOrchestratorStreamsSanitizedLiveCellStatus(t *testing.T) {
	_, _, orchestrator, now := newTestOrchestrator(t)
	if _, err := orchestrator.Reconcile(t.Context(), ReconcileRequest{
		SchemaVersion: 1, Revision: 1,
		Runtimes: []RuntimeAssignment{testAssignment("alpha", "tenant-one", 1, now.Add(10*time.Minute))},
	}); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(orchestrator.Handler())
	defer server.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	request, _ := http.NewRequestWithContext(ctx, http.MethodGet, server.URL+"/orchestrator/v1/events", nil)
	request.Header.Set("Authorization", "Bearer "+testControlToken)
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.Header.Get("Content-Type") != "text/event-stream" {
		t.Fatalf("event stream content type = %q", response.Header.Get("Content-Type"))
	}
	scanner := bufio.NewScanner(response.Body)
	var event string
	var status CellStatus
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "event: ") {
			event = strings.TrimPrefix(line, "event: ")
		}
		if strings.HasPrefix(line, "data: ") {
			if err := json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &status); err != nil {
				t.Fatal(err)
			}
			break
		}
	}
	if event != "orchestrator.ready" || status.CellID != "cell-one" || status.DesiredRevision != 1 || len(status.Runtimes) != 1 {
		t.Fatalf("initial orchestrator event = %q %+v", event, status)
	}
	payload, _ := json.Marshal(status)
	for _, forbidden := range []string{"token", "password", "resources", "inventory", "playerName"} {
		if bytes.Contains(bytes.ToLower(payload), []byte(strings.ToLower(forbidden))) {
			t.Fatalf("sanitized status contains %q: %s", forbidden, payload)
		}
	}
}

func TestOrchestratorScopesPrivateMetricsGrantToOneRuntimePlacement(t *testing.T) {
	metricsServer := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.WriteHeader(http.StatusNoContent)
	}))
	defer metricsServer.Close()
	metricsClient, err := PrivateMetrics.NewClient(PrivateMetrics.ClientConfig{
		Client: metricsServer.Client(), Endpoint: metricsServer.URL, ClientVersion: "test",
	})
	if err != nil {
		t.Fatal(err)
	}
	supervisor, dashboardAuth, orchestrator, now := newTestOrchestrator(t)
	supervisor.privateMetrics = metricsClient
	alpha := testAssignment("alpha", "tenant-one", 3, now.Add(10*time.Minute))
	if _, err := orchestrator.Reconcile(t.Context(), ReconcileRequest{
		SchemaVersion: 1, Revision: 1, Runtimes: []RuntimeAssignment{alpha},
	}); orchestratorErrorCode(err) != "private_metrics_grant_required" {
		t.Fatalf("missing private metrics grant error = %v", err)
	}
	metricsToken := strings.Repeat("m", 48)
	alpha.PrivateMetrics = &PrivateMetrics.Grant{Token: metricsToken, ExpiresAt: now.Add(8 * time.Minute)}
	status, err := orchestrator.Reconcile(t.Context(), ReconcileRequest{
		SchemaVersion: 1, Revision: 2, Runtimes: []RuntimeAssignment{alpha},
	})
	if err != nil {
		t.Fatal(err)
	}
	application, exists := supervisor.Application("alpha")
	if !exists || application.PrivateMetrics == nil {
		t.Fatal("alpha did not receive its private metrics publisher")
	}
	payload, _ := json.Marshal(status)
	if bytes.Contains(payload, []byte(metricsToken)) || bytes.Contains(bytes.ToLower(payload), []byte("authorization")) {
		t.Fatalf("runtime status exposed private metrics grant material: %s", payload)
	}
	if len(status.Runtimes) != 1 || status.Runtimes[0].PrivateMetricsState == "" {
		t.Fatalf("runtime status omitted sanitized private metrics state: %+v", status)
	}
	dashboardToken := strings.Repeat("d", 48)
	if err := dashboardAuth.SetDashboardGrant("alpha", dashboardToken, now.Add(5*time.Minute)); err != nil {
		t.Fatal(err)
	}
	alpha.PrivateMetrics = &PrivateMetrics.Grant{Token: dashboardToken, ExpiresAt: now.Add(8 * time.Minute)}
	if _, err := orchestrator.Reconcile(t.Context(), ReconcileRequest{
		SchemaVersion: 1, Revision: 3, Runtimes: []RuntimeAssignment{alpha},
	}); orchestratorErrorCode(err) != "invalid_private_metrics_grant" {
		t.Fatalf("dashboard token reused as private metrics grant error = %v", err)
	}
	alpha.PrivateMetrics = &PrivateMetrics.Grant{Token: metricsToken, ExpiresAt: now.Add(8 * time.Minute)}

	bravo := testAssignment("bravo", "tenant-two", 1, now.Add(10*time.Minute))
	bravo.PrivateMetrics = &PrivateMetrics.Grant{Token: metricsToken, ExpiresAt: now.Add(8 * time.Minute)}
	if _, err := orchestrator.Reconcile(t.Context(), ReconcileRequest{
		SchemaVersion: 1, Revision: 4, Runtimes: []RuntimeAssignment{alpha, bravo},
	}); orchestratorErrorCode(err) != "duplicate_private_metrics_grant" {
		t.Fatalf("duplicate private metrics grant error = %v", err)
	}
	if _, exists := supervisor.Application("bravo"); exists {
		t.Fatal("bravo started with alpha's private metrics grant")
	}
	if _, err := orchestrator.Reconcile(t.Context(), ReconcileRequest{
		SchemaVersion: 1, Revision: 4, Runtimes: []RuntimeAssignment{bravo},
	}); orchestratorErrorCode(err) != "private_metrics_grant_reassigned" {
		t.Fatalf("private metrics grant reassignment error = %v", err)
	}

	controlServer := httptest.NewServer(orchestrator.Handler())
	defer controlServer.Close()
	putDashboardGrant(t, controlServer.URL, "alpha", 3, metricsToken, now.Add(5*time.Minute), http.StatusBadRequest)
}

// testClock is the orchestrator's injected clock; tests advance it to observe
// lease lapses without waiting.
type testClock struct {
	mu  sync.Mutex
	now time.Time
}

func (clock *testClock) Now() time.Time {
	clock.mu.Lock()
	defer clock.mu.Unlock()
	return clock.now
}

func (clock *testClock) Advance(delta time.Duration) time.Time {
	clock.mu.Lock()
	defer clock.mu.Unlock()
	clock.now = clock.now.Add(delta)
	return clock.now
}

func newTestOrchestrator(t *testing.T) (*Supervisor, *TenantAuthenticator, *Orchestrator, time.Time) {
	t.Helper()
	supervisor, auth, orchestrator, clock := newTestOrchestratorWithClock(t)
	return supervisor, auth, orchestrator, clock.Now()
}

func newTestOrchestratorWithClock(t *testing.T) (*Supervisor, *TenantAuthenticator, *Orchestrator, *testClock) {
	t.Helper()
	supervisor := newTestSupervisor(t)
	auth, err := NewDynamicTenantAuthenticator([]byte(strings.Repeat("s", 32)), true)
	if err != nil {
		t.Fatal(err)
	}
	clock := &testClock{now: time.Now().UTC().Truncate(time.Second)}
	orchestrator, err := NewOrchestrator(OrchestratorConfig{
		CellID: "cell-one", Token: testControlToken,
		Supervisor: supervisor, DashboardAuth: auth, Now: clock.Now,
		DrainTimeout: time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	return supervisor, auth, orchestrator, clock
}

func testAssignment(runtimeID, tenantID string, epoch uint64, lease time.Time) RuntimeAssignment {
	return RuntimeAssignment{
		RuntimeID: runtimeID, TenantID: tenantID, PlacementEpoch: epoch,
		LeaseExpiresAt: lease, StartSession: false,
	}
}

func putDashboardGrant(t *testing.T, baseURL, runtimeID string, epoch uint64, token string, expiresAt time.Time, want int) {
	t.Helper()
	body, _ := json.Marshal(DashboardGrantRequest{PlacementEpoch: epoch, Token: token, ExpiresAt: expiresAt})
	request, _ := http.NewRequest(http.MethodPut, baseURL+"/orchestrator/v1/runtimes/"+runtimeID+"/dashboard-grant", bytes.NewReader(body))
	request.Header.Set("Authorization", "Bearer "+testControlToken)
	request.Header.Set("Content-Type", "application/json")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != want {
		t.Fatalf("dashboard grant for %s status = %d, want %d", runtimeID, response.StatusCode, want)
	}
}

func getRuntimeState(t *testing.T, target, token string, want int) State.GameState {
	t.Helper()
	request, _ := http.NewRequest(http.MethodGet, target, nil)
	request.Header.Set("Authorization", "Bearer "+token)
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != want {
		t.Fatalf("GET %s status = %d, want %d", target, response.StatusCode, want)
	}
	var state State.GameState
	if want == http.StatusOK {
		if err := json.NewDecoder(response.Body).Decode(&state); err != nil {
			t.Fatal(err)
		}
	}
	return state
}

func waitForRuntime(t *testing.T, supervisor *Supervisor, id AccountID, exists bool) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for {
		_, found := supervisor.Application(id)
		if found == exists {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("runtime %q existence = %t, want %t", id, found, exists)
		}
		time.Sleep(time.Millisecond)
	}
}

func orchestratorErrorCode(err error) string {
	var controlErr *orchestratorError
	if errors.As(err, &controlErr) {
		return controlErr.code
	}
	return ""
}

func controlRequest(t *testing.T, method, target string, body any) (*http.Response, []byte) {
	t.Helper()
	var reader io.Reader
	if body != nil {
		payload, _ := json.Marshal(body)
		reader = bytes.NewReader(payload)
	}
	request, _ := http.NewRequest(method, target, reader)
	request.Header.Set("Authorization", "Bearer "+testControlToken)
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	var raw bytes.Buffer
	_, _ = raw.ReadFrom(response.Body)
	return response, raw.Bytes()
}

func TestOrchestratorInstallsAndRevokesRuntimeLoginWithoutEchoingIt(t *testing.T) {
	supervisor, _, orchestrator, now := newTestOrchestrator(t)
	alpha := testAssignment("alpha", "tenant-one", 3, now.Add(10*time.Minute))
	if _, err := orchestrator.Reconcile(t.Context(), ReconcileRequest{SchemaVersion: 1, Revision: 1, Runtimes: []RuntimeAssignment{alpha}}); err != nil {
		t.Fatal(err)
	}
	controlServer := httptest.NewServer(orchestrator.Handler())
	defer controlServer.Close()
	loginURL := controlServer.URL + "/orchestrator/v1/runtimes/alpha/login"
	const password = "correct horse battery staple"

	// A stale epoch, an unknown runtime, and an invalid server selection are
	// all refused before anything is written.
	response, _ := controlRequest(t, http.MethodPut, loginURL, LoginCredentialRequest{
		PlacementEpoch: 2, Username: "james", Password: password, Server: "US1",
	})
	if response.StatusCode != http.StatusConflict {
		t.Fatalf("stale epoch status = %d", response.StatusCode)
	}
	response, _ = controlRequest(t, http.MethodPut, controlServer.URL+"/orchestrator/v1/runtimes/zulu/login", LoginCredentialRequest{
		PlacementEpoch: 3, Username: "james", Password: password, Server: "US1",
	})
	if response.StatusCode != http.StatusNotFound {
		t.Fatalf("unknown runtime status = %d", response.StatusCode)
	}
	response, body := controlRequest(t, http.MethodPut, loginURL, LoginCredentialRequest{
		PlacementEpoch: 3, Username: "james", Password: password, Server: "wss://evil.example",
	})
	if response.StatusCode != http.StatusBadRequest || bytes.Contains(body, []byte(password)) {
		t.Fatalf("invalid server status = %d body = %s", response.StatusCode, body)
	}
	application, _ := supervisor.Application("alpha")
	if login, err := application.BackgroundLogin.Status(); err != nil || login.Configured {
		t.Fatalf("login installed despite refusals: %+v %v", login, err)
	}

	response, body = controlRequest(t, http.MethodPut, loginURL, LoginCredentialRequest{
		PlacementEpoch: 3, Username: "james", Password: password, Server: "us1", Language: "en",
	})
	if response.StatusCode != http.StatusOK {
		t.Fatalf("install status = %d body = %s", response.StatusCode, body)
	}
	if bytes.Contains(body, []byte(password)) || bytes.Contains(body, []byte("james")) {
		t.Fatalf("install response echoed the credential: %s", body)
	}
	if !bytes.Contains(body, []byte(`"server":"US1"`)) || !bytes.Contains(body, []byte(`"configured":true`)) {
		t.Fatalf("install response = %s", body)
	}
	login, err := application.BackgroundLogin.Status()
	if err != nil || !login.Configured || login.Server != "US1" || login.ServerURL != "wss://ep-live-us1-game.goodgamestudios.com:443" {
		t.Fatalf("installed login status = %+v %v", login, err)
	}
	credentialPath := filepath.Join(application.DataDir, "Session", "BackgroundLogin.json")
	info, err := os.Stat(credentialPath)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("saved login mode = %o", info.Mode().Perm())
	}
	saved, _ := os.ReadFile(credentialPath)
	if !bytes.Contains(saved, []byte(password)) || !bytes.Contains(saved, []byte(`"username":"james"`)) {
		t.Fatalf("saved login = %s", saved)
	}
	if strings.HasPrefix(application.DataDir, supervisor.config.DataRoot) == false {
		t.Fatalf("runtime data dir %q escaped the cell data root", application.DataDir)
	}

	// Status and events carry only the sanitized summary.
	response, body = controlRequest(t, http.MethodGet, controlServer.URL+"/orchestrator/v1/status", nil)
	if response.StatusCode != http.StatusOK || bytes.Contains(body, []byte(password)) || bytes.Contains(body, []byte("james")) {
		t.Fatalf("status leaked credential material: %d %s", response.StatusCode, body)
	}
	var status CellStatus
	if err := json.Unmarshal(body, &status); err != nil {
		t.Fatal(err)
	}
	if len(status.Runtimes) != 1 || status.Runtimes[0].BackgroundLogin == nil ||
		!status.Runtimes[0].BackgroundLogin.Configured || status.Runtimes[0].BackgroundLogin.Server != "US1" {
		t.Fatalf("status background login = %+v", status.Runtimes)
	}

	// The control plane may pin the directory-resolved endpoint and zone;
	// both must round-trip into the saved credential so multi-zone worlds
	// (GB1 shares its host with INT1 and SK1) connect to the right zone.
	response, body = controlRequest(t, http.MethodPut, loginURL, LoginCredentialRequest{
		PlacementEpoch: 3, Username: "albion", Password: password, Server: "GB1",
		ServerURL: "wss://ep-live-mz-int1-sk1-gb1-game.goodgamestudios.com:443", Zone: "EmpireEx_19", Language: "en",
	})
	if response.StatusCode != http.StatusOK {
		t.Fatalf("resolved install status = %d body = %s", response.StatusCode, body)
	}
	login, err = application.BackgroundLogin.Status()
	if err != nil || login.Server != "GB1" || login.ServerURL != "wss://ep-live-mz-int1-sk1-gb1-game.goodgamestudios.com:443" {
		t.Fatalf("resolved login status = %+v %v", login, err)
	}
	saved, _ = os.ReadFile(credentialPath)
	if !bytes.Contains(saved, []byte(`"zone":"EmpireEx_19"`)) {
		t.Fatalf("saved login lost the pinned zone: %s", saved)
	}
	response, body = controlRequest(t, http.MethodPut, loginURL, LoginCredentialRequest{
		PlacementEpoch: 3, Username: "albion", Password: password, Server: "GB1",
		ServerURL: "wss://evil.example.com:443", Zone: "EmpireEx_19",
	})
	if response.StatusCode != http.StatusBadRequest {
		t.Fatalf("unofficial pinned URL status = %d body = %s", response.StatusCode, body)
	}

	// Revocation scrubs the saved login from the runtime and reports it.
	response, body = controlRequest(t, http.MethodDelete, loginURL, nil)
	if response.StatusCode != http.StatusOK || !bytes.Contains(body, []byte(`"configured":false`)) {
		t.Fatalf("revoke status = %d body = %s", response.StatusCode, body)
	}
	if _, err := os.Stat(credentialPath); !os.IsNotExist(err) {
		t.Fatalf("saved login survived revocation: %v", err)
	}
	if login, err := application.BackgroundLogin.Status(); err != nil || login.Configured {
		t.Fatalf("login status after revocation = %+v %v", login, err)
	}

	// Revocation also scrubs a drained runtime's profile that remains on disk.
	if response, _ = controlRequest(t, http.MethodPut, loginURL, LoginCredentialRequest{
		PlacementEpoch: 3, Username: "james", Password: password, Server: "US1",
	}); response.StatusCode != http.StatusOK {
		t.Fatalf("reinstall status = %d", response.StatusCode)
	}
	if _, err := orchestrator.Reconcile(t.Context(), ReconcileRequest{SchemaVersion: 1, Revision: 2, Runtimes: nil}); err != nil {
		t.Fatal(err)
	}
	waitForRuntime(t, supervisor, "alpha", false)
	if _, err := os.Stat(credentialPath); err != nil {
		t.Fatalf("drained runtime lost its saved login before revocation: %v", err)
	}
	response, _ = controlRequest(t, http.MethodDelete, loginURL, nil)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("drained revoke status = %d", response.StatusCode)
	}
	if _, err := os.Stat(credentialPath); !os.IsNotExist(err) {
		t.Fatalf("drained runtime saved login survived revocation: %v", err)
	}
	response, _ = controlRequest(t, http.MethodDelete, controlServer.URL+"/orchestrator/v1/runtimes/never/login", nil)
	if response.StatusCode != http.StatusNotFound {
		t.Fatalf("unknown drained revoke status = %d", response.StatusCode)
	}
}

func waitForSessionState(t *testing.T, application *App.Application, states ...string) string {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for {
		current := application.State.Session().Status
		for _, state := range states {
			if current == state {
				return current
			}
		}
		if time.Now().After(deadline) {
			t.Fatalf("session state = %q, want one of %v", current, states)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func TestOrchestratorStopsAndRestartsRuntimeWithDesiredState(t *testing.T) {
	supervisor, _, orchestrator, now := newTestOrchestrator(t)
	alpha := testAssignment("alpha", "tenant-one", 1, now.Add(10*time.Minute))
	alpha.StartSession = true

	// Enable: the runtime exists and its session is asked to start. Without a
	// saved login the Background transport reports "unavailable" — the runtime
	// itself stays up and waits for the credential.
	if _, err := orchestrator.Reconcile(t.Context(), ReconcileRequest{SchemaVersion: 1, Revision: 1, Runtimes: []RuntimeAssignment{alpha}}); err != nil {
		t.Fatal(err)
	}
	first, exists := supervisor.Application("alpha")
	if !exists {
		t.Fatal("alpha runtime was not created")
	}
	waitForSessionState(t, first, "unavailable", "starting", "reconnecting", "error")
	dataDir := first.DataDir

	// The user stops the account: the runtime is drained and its slot freed.
	if _, err := orchestrator.Reconcile(t.Context(), ReconcileRequest{SchemaVersion: 1, Revision: 2, Runtimes: nil}); err != nil {
		t.Fatal(err)
	}
	waitForRuntime(t, supervisor, "alpha", false)
	if capacity := supervisor.Capacity(); capacity.Active != 0 {
		t.Fatalf("drained runtime still counted as active: %+v", capacity)
	}
	if _, err := os.Stat(dataDir); err != nil {
		t.Fatalf("stopping the account destroyed its data: %v", err)
	}

	// The user re-enables it: a fresh runtime comes back on the same profile
	// and the session is started again.
	alpha.PlacementEpoch = 2
	deadline := time.Now().Add(5 * time.Second)
	for {
		_, err := orchestrator.Reconcile(t.Context(), ReconcileRequest{SchemaVersion: 1, Revision: 3, Runtimes: []RuntimeAssignment{alpha}})
		if err == nil {
			break
		}
		if orchestratorErrorCode(err) != "runtime_start_failed" || time.Now().After(deadline) {
			t.Fatalf("re-enable reconcile = %v", err)
		}
		time.Sleep(10 * time.Millisecond)
	}
	second, exists := supervisor.Application("alpha")
	if !exists || second == first || second.DataDir != dataDir {
		t.Fatalf("re-enabled runtime = %v (exists %t, same profile %t)", second, exists, second != nil && second.DataDir == dataDir)
	}
	waitForSessionState(t, second, "unavailable", "starting", "reconnecting", "error")

	// Suspending keeps the runtime and its dashboard shard but parks the game
	// session; enabling again restarts it in place.
	alpha.StartSession = false
	if _, err := orchestrator.Reconcile(t.Context(), ReconcileRequest{SchemaVersion: 1, Revision: 4, Runtimes: []RuntimeAssignment{alpha}}); err != nil {
		t.Fatal(err)
	}
	if current, _ := supervisor.Application("alpha"); current != second {
		t.Fatal("suspending the session reconstructed the runtime")
	}
	waitForSessionState(t, second, "stopped")
	status := orchestrator.Status()
	if len(status.Runtimes) != 1 || status.Runtimes[0].DesiredSession || status.Runtimes[0].SessionState != "stopped" {
		t.Fatalf("suspended status = %+v", status.Runtimes)
	}
	alpha.StartSession = true
	if _, err := orchestrator.Reconcile(t.Context(), ReconcileRequest{SchemaVersion: 1, Revision: 5, Runtimes: []RuntimeAssignment{alpha}}); err != nil {
		t.Fatal(err)
	}
	waitForSessionState(t, second, "unavailable", "starting", "reconnecting", "error")
}

func TestOrchestratorReleasePolicyAndReconnectControl(t *testing.T) {
	supervisor, _, orchestrator, now := newTestOrchestrator(t)
	alpha := testAssignment("alpha", "tenant-one", 1, now.Add(10*time.Minute))
	alpha.OnDisconnect = "hibernate"
	if _, err := orchestrator.Reconcile(t.Context(), ReconcileRequest{SchemaVersion: 1, Revision: 1, Runtimes: []RuntimeAssignment{alpha}}); orchestratorErrorCode(err) != "invalid_disconnect_policy" {
		t.Fatalf("invalid disconnect policy error = %v", err)
	}
	alpha.OnDisconnect = "release"
	if _, err := orchestrator.Reconcile(t.Context(), ReconcileRequest{SchemaVersion: 1, Revision: 1, Runtimes: []RuntimeAssignment{alpha}}); err != nil {
		t.Fatal(err)
	}
	status := orchestrator.Status()
	if len(status.Runtimes) != 1 || status.Runtimes[0].OnDisconnect != "release" {
		t.Fatalf("status did not echo the disconnect policy: %+v", status.Runtimes)
	}
	// Omitting the field means release (hosted runtimes live only while
	// connected); hold must be requested explicitly, and a changed policy is a
	// real change under the same revision rules.
	bravo := testAssignment("bravo", "tenant-two", 1, now.Add(10*time.Minute))
	charlie := testAssignment("charlie", "tenant-three", 1, now.Add(10*time.Minute))
	charlie.OnDisconnect = "hold"
	if _, err := orchestrator.Reconcile(t.Context(), ReconcileRequest{SchemaVersion: 1, Revision: 2, Runtimes: []RuntimeAssignment{alpha, bravo, charlie}}); err != nil {
		t.Fatal(err)
	}
	for _, runtime := range orchestrator.Status().Runtimes {
		want := map[string]string{"alpha": "release", "bravo": "release", "charlie": "hold"}[runtime.RuntimeID]
		if runtime.OnDisconnect != want {
			t.Fatalf("runtime %s policy = %q, want %q", runtime.RuntimeID, runtime.OnDisconnect, want)
		}
	}
	alpha.OnDisconnect = "hold"
	if _, err := orchestrator.Reconcile(t.Context(), ReconcileRequest{SchemaVersion: 1, Revision: 2, Runtimes: []RuntimeAssignment{alpha, bravo, charlie}}); orchestratorErrorCode(err) != "desired_revision_conflict" {
		t.Fatalf("policy change under a reused revision = %v", err)
	}

	controlServer := httptest.NewServer(orchestrator.Handler())
	defer controlServer.Close()
	// Reconnect is refused with a stable reason when the runtime cannot start
	// a session (no saved login yet), and 404 for unknown runtimes; the
	// runtime itself is untouched either way.
	response, body := controlRequest(t, http.MethodPost, controlServer.URL+"/orchestrator/v1/runtimes/alpha/reconnect", nil)
	if response.StatusCode != http.StatusConflict || !bytes.Contains(body, []byte(`"reconnect_refused"`)) ||
		!bytes.Contains(body, []byte(`"detail":"session_start_failed"`)) {
		t.Fatalf("reconnect without a login = %d %s", response.StatusCode, body)
	}
	if _, exists := supervisor.Application("alpha"); !exists {
		t.Fatal("refused reconnect removed the runtime")
	}
	response, _ = controlRequest(t, http.MethodPost, controlServer.URL+"/orchestrator/v1/runtimes/zulu/reconnect", nil)
	if response.StatusCode != http.StatusNotFound {
		t.Fatalf("reconnect for unknown runtime = %d", response.StatusCode)
	}
	if got := bytes.Count(body, []byte("password")); got != 0 {
		t.Fatalf("reconnect response mentioned credentials: %s", body)
	}
}
