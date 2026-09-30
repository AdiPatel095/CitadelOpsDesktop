package Accounts

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

	"CitadelDesktop/Server/API"
	"CitadelDesktop/Server/PrivateMetrics"
	"github.com/gorilla/websocket"
)

func TestDashboardOriginPolicyValidatesAndNormalizes(t *testing.T) {
	policy, err := NewDashboardOriginPolicy([]string{" https://App.CitadelOps.app , http://localhost:5173/"})
	if err != nil {
		t.Fatal(err)
	}
	for _, origin := range []string{"https://app.citadelops.app", "http://localhost:5173"} {
		if _, ok := policy.allowed[origin]; !ok {
			t.Fatalf("policy did not keep %q: %v", origin, policy.allowed)
		}
	}
	for _, invalid := range []string{"app.citadelops.app", "https://app.citadelops.app/dashboard", "ftp://x", "https://user@host"} {
		if _, err := NewDashboardOriginPolicy([]string{invalid}); err == nil {
			t.Fatalf("invalid origin %q was accepted", invalid)
		}
	}
	// A nil policy keeps the same-host rule and nothing else.
	var none *DashboardOriginPolicy
	request := httptest.NewRequest(http.MethodGet, "https://cell.internal/accounts/alpha/api/v2/state", nil)
	request.Host = "cell.internal"
	request.Header.Set("Origin", "https://cell.internal")
	if allowed, cross := none.Allowed(request); !allowed || cross {
		t.Fatalf("same-host under nil policy = %t %t", allowed, cross)
	}
	request.Header.Set("Origin", "https://app.citadelops.app")
	if allowed, _ := none.Allowed(request); allowed {
		t.Fatal("nil policy allowed a foreign origin")
	}
}

func TestShardRouterAcceptsAllowlistedFrontendOriginWithCORS(t *testing.T) {
	supervisor, dashboardAuth, orchestrator, now := newTestOrchestrator(t)
	alpha := testAssignment("alpha", "tenant-one", 1, now.Add(10*time.Minute))
	if _, err := orchestrator.Reconcile(t.Context(), ReconcileRequest{SchemaVersion: 1, Revision: 1, Runtimes: []RuntimeAssignment{alpha}}); err != nil {
		t.Fatal(err)
	}
	application, _ := supervisor.Application("alpha")
	setTestPlayer(t, application.State, 101, "Alpha")
	token := strings.Repeat("a", 48)
	if err := dashboardAuth.SetDashboardGrant("alpha", token, now.Add(5*time.Minute)); err != nil {
		t.Fatal(err)
	}
	policy, err := NewDashboardOriginPolicy([]string{"https://app.citadelops.app"})
	if err != nil {
		t.Fatal(err)
	}
	dashboardAuth.SetDashboardOrigins(policy)
	runtimeServer := httptest.NewServer(supervisor.HandlerWithOrigins(dashboardAuth, nil, policy))
	defer runtimeServer.Close()

	// Preflight from the frontend origin is answered before authentication.
	preflight, _ := http.NewRequest(http.MethodOptions, runtimeServer.URL+"/accounts/alpha/api/v2/state", nil)
	preflight.Header.Set("Origin", "https://app.citadelops.app")
	preflight.Header.Set("Access-Control-Request-Method", "GET")
	preflight.Header.Set("Access-Control-Request-Headers", "authorization")
	response, err := http.DefaultClient.Do(preflight)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusNoContent || response.Header.Get("Access-Control-Allow-Origin") != "https://app.citadelops.app" ||
		response.Header.Get("Access-Control-Allow-Credentials") != "true" ||
		!strings.Contains(response.Header.Get("Access-Control-Allow-Headers"), "Authorization") {
		t.Fatalf("preflight = %d %v", response.StatusCode, response.Header)
	}

	// The real request from the frontend origin is served with CORS headers.
	request, _ := http.NewRequest(http.MethodGet, runtimeServer.URL+"/accounts/alpha/api/v2/state", nil)
	request.Header.Set("Origin", "https://app.citadelops.app")
	request.Header.Set("Authorization", "Bearer "+token)
	response, err = http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(response.Body)
	response.Body.Close()
	if response.StatusCode != http.StatusOK || response.Header.Get("Access-Control-Allow-Origin") != "https://app.citadelops.app" ||
		!bytes.Contains(body, []byte(`"name":"Alpha"`)) {
		t.Fatalf("frontend-origin state = %d %v %s", response.StatusCode, response.Header, body)
	}

	// Any other origin is still rejected, and preflights for it are not answered.
	other, _ := http.NewRequest(http.MethodGet, runtimeServer.URL+"/accounts/alpha/api/v2/state", nil)
	other.Header.Set("Origin", "https://attacker.example")
	other.Header.Set("Authorization", "Bearer "+token)
	response, err = http.DefaultClient.Do(other)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusForbidden || response.Header.Get("Access-Control-Allow-Origin") != "" {
		t.Fatalf("foreign-origin state = %d %v", response.StatusCode, response.Header)
	}
	otherPreflight, _ := http.NewRequest(http.MethodOptions, runtimeServer.URL+"/accounts/alpha/api/v2/state", nil)
	otherPreflight.Header.Set("Origin", "https://attacker.example")
	otherPreflight.Header.Set("Access-Control-Request-Method", "GET")
	response, err = http.DefaultClient.Do(otherPreflight)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode == http.StatusNoContent || response.Header.Get("Access-Control-Allow-Origin") != "" {
		t.Fatalf("foreign preflight = %d %v", response.StatusCode, response.Header)
	}

	// The tenant login accepts the same allowlisted origin.
	loginServer := httptest.NewServer(dashboardAuth.LoginHandler())
	defer loginServer.Close()
	login, _ := http.NewRequest(http.MethodPost, loginServer.URL+"/tenant/login",
		strings.NewReader(`{"accountId":"alpha","token":"`+token+`"}`))
	login.Header.Set("Origin", "https://app.citadelops.app")
	login.Header.Set("Content-Type", "application/json")
	response, err = http.DefaultClient.Do(login)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode >= http.StatusBadRequest || response.Header.Get("Access-Control-Allow-Origin") != "https://app.citadelops.app" {
		t.Fatalf("frontend-origin login = %d %v", response.StatusCode, response.Header)
	}
}

func TestDrainPublishesAFinalDashboardCheckpoint(t *testing.T) {
	received := make(chan PrivateMetrics.CheckpointRequest, 16)
	backend := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path == "/checkpoints" {
			reader, err := gzip.NewReader(request.Body)
			if err != nil {
				writer.WriteHeader(http.StatusBadRequest)
				return
			}
			var payload PrivateMetrics.CheckpointRequest
			if err := json.NewDecoder(reader).Decode(&payload); err != nil {
				writer.WriteHeader(http.StatusBadRequest)
				return
			}
			received <- payload
		}
		writer.WriteHeader(http.StatusNoContent)
	}))
	defer backend.Close()
	client, err := PrivateMetrics.NewClient(PrivateMetrics.ClientConfig{
		Client: backend.Client(), Endpoint: backend.URL + "/metrics", CheckpointEndpoint: backend.URL + "/checkpoints", ClientVersion: "test",
	})
	if err != nil {
		t.Fatal(err)
	}
	supervisor, _, orchestrator, now := newTestOrchestrator(t)
	supervisor.privateMetrics = client
	alpha := testAssignment("alpha", "tenant-one", 2, now.Add(10*time.Minute))
	alpha.PrivateMetrics = &PrivateMetrics.Grant{Token: strings.Repeat("m", 48), ExpiresAt: now.Add(8 * time.Minute)}
	if _, err := orchestrator.Reconcile(t.Context(), ReconcileRequest{SchemaVersion: 1, Revision: 1, Runtimes: []RuntimeAssignment{alpha}}); err != nil {
		t.Fatal(err)
	}
	// The runtime checkpoints on its own once it has a placement…
	select {
	case first := <-received:
		if first.RuntimeID != "alpha" || first.PlacementEpoch != 2 || len(first.Checkpoint.State) == 0 {
			t.Fatalf("initial checkpoint = %+v", first)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("no initial dashboard checkpoint")
	}
	// …and once more, synchronously, when the account is drained.
	if _, err := orchestrator.Reconcile(t.Context(), ReconcileRequest{SchemaVersion: 1, Revision: 2, Runtimes: nil}); err != nil {
		t.Fatal(err)
	}
	waitForRuntime(t, supervisor, "alpha", false)
	deadline := time.After(5 * time.Second)
	for {
		select {
		case checkpoint := <-received:
			if checkpoint.Checkpoint.Reason == PrivateMetrics.CheckpointReasonDrain {
				if status := orchestrator.Status(); len(status.Runtimes) != 0 {
					t.Fatalf("drained runtime still listed: %+v", status.Runtimes)
				}
				return
			}
		case <-deadline:
			t.Fatal("drain did not publish a final dashboard checkpoint")
		}
	}
}

var _ = context.Background

func TestDirectDashboardBearerSessionOverTLS(t *testing.T) {
	supervisor, auth, orchestrator, now := newTestOrchestrator(t)
	assignments := []RuntimeAssignment{testAssignment("alpha", "tenant-one", 1, now.Add(10*time.Minute)), testAssignment("bravo", "tenant-two", 1, now.Add(10*time.Minute))}
	if _, err := orchestrator.Reconcile(t.Context(), ReconcileRequest{SchemaVersion: 1, Revision: 1, Runtimes: assignments}); err != nil {
		t.Fatal(err)
	}
	application, _ := supervisor.Application("alpha")
	setTestPlayer(t, application.State, 101, "Alpha")
	const origin = "https://app.citadelops.app"
	policy, err := NewDashboardOriginPolicy([]string{origin})
	if err != nil {
		t.Fatal(err)
	}
	auth.SetDashboardOrigins(policy)
	mux := http.NewServeMux()
	mux.Handle("/tenant/login", auth.LoginHandler())
	mux.Handle("/accounts/", supervisor.HandlerWithOrigins(auth, nil, policy))
	server := httptest.NewTLSServer(mux)
	defer server.Close()
	bootstrap := strings.Repeat("x", 48)
	if err := auth.SetDashboardBootstrap("alpha", bootstrap, time.Now().Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	send := func(method, path, body, token string, preflight bool) (int, http.Header, []byte) {
		t.Helper()
		request, err := http.NewRequest(method, server.URL+path, strings.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		request.Header.Set("Origin", origin)
		if body != "" {
			request.Header.Set("Content-Type", "application/json")
		}
		if token != "" {
			request.Header.Set("Authorization", "Bearer "+token)
		}
		if preflight {
			request.Header.Set("Access-Control-Request-Method", "POST")
			request.Header.Set("Access-Control-Request-Headers", "content-type")
		}
		response, err := server.Client().Do(request)
		if err != nil {
			t.Fatal(err)
		}
		defer response.Body.Close()
		data, err := io.ReadAll(response.Body)
		if err != nil {
			t.Fatal(err)
		}
		return response.StatusCode, response.Header, data
	}
	status, headers, _ := send(http.MethodOptions, "/tenant/login", "", "", true)
	if status != http.StatusNoContent || headers.Get("Access-Control-Allow-Origin") != origin {
		t.Fatalf("login preflight = %d, %v", status, headers)
	}
	loginBody := `{"accountId":"alpha","token":"` + bootstrap + `","credential":"bearer"}`
	status, headers, body := send(http.MethodPost, "/tenant/login", loginBody, "", false)
	if status != http.StatusOK || headers.Get("Access-Control-Allow-Origin") != origin || headers.Get("Set-Cookie") != "" || headers.Get("Cache-Control") != "no-store" {
		t.Fatalf("bearer login status = %d", status)
	}
	var login struct {
		SessionToken string `json:"sessionToken"`
	}
	if err := json.Unmarshal(body, &login); err != nil {
		t.Fatal(err)
	}
	if login.SessionToken == "" {
		t.Fatal("missing session token")
	}
	if status, _, _ := send(http.MethodPost, "/tenant/login", loginBody, "", false); status != http.StatusUnauthorized {
		t.Fatalf("bootstrap replay status = %d", status)
	}
	status, headers, body = send(http.MethodGet, "/accounts/alpha/api/v2/state", "", login.SessionToken, false)
	if status != http.StatusOK || headers.Get("Access-Control-Allow-Origin") != origin || !bytes.Contains(body, []byte(`"name":"Alpha"`)) {
		t.Fatalf("state status = %d; player present = %t", status, bytes.Contains(body, []byte(`"name":"Alpha"`)))
	}
	if status, _, _ := send(http.MethodGet, "/accounts/bravo/api/v2/state", "", login.SessionToken, false); status != http.StatusNotFound {
		t.Fatalf("wrong runtime status = %d", status)
	}
	transport := server.Client().Transport.(*http.Transport)
	dialer := websocket.Dialer{TLSClientConfig: transport.TLSClientConfig, Subprotocols: []string{API.EventsSubprotocol, sessionSubprotocolPrefix + login.SessionToken}}
	socket, response, err := dialer.Dial("wss"+strings.TrimPrefix(server.URL, "https")+"/accounts/alpha/api/v2/events", http.Header{"Origin": []string{origin}})
	if err != nil {
		t.Fatalf("authenticated socket failed: %v", err)
	}
	defer socket.Close()
	if response.StatusCode != http.StatusSwitchingProtocols || response.Header.Get("Sec-WebSocket-Protocol") != API.EventsSubprotocol || socket.Subprotocol() != API.EventsSubprotocol {
		t.Fatal("socket did not echo the contract subprotocol")
	}
	if err := socket.SetReadDeadline(time.Now().Add(5 * time.Second)); err != nil {
		t.Fatal(err)
	}
	var frame API.Envelope
	if err := socket.ReadJSON(&frame); err != nil {
		t.Fatal(err)
	}
	if frame.Type != "state.snapshot" {
		t.Fatalf("first frame = %q", frame.Type)
	}
	for _, test := range []struct {
		name, origin string
		protocols    []string
		want         int
	}{
		{"no credential", origin, []string{API.EventsSubprotocol}, http.StatusUnauthorized},
		{"rejected origin", "https://attacker.example", []string{API.EventsSubprotocol, sessionSubprotocolPrefix + login.SessionToken}, http.StatusForbidden},
	} {
		t.Run(test.name, func(t *testing.T) {
			rejectedDialer := dialer
			rejectedDialer.Subprotocols = test.protocols
			connection, response, err := rejectedDialer.Dial("wss"+strings.TrimPrefix(server.URL, "https")+"/accounts/alpha/api/v2/events", http.Header{"Origin": []string{test.origin}})
			if connection != nil {
				connection.Close()
			}
			if response != nil {
				defer response.Body.Close()
			}
			if err == nil || response == nil || response.StatusCode != test.want {
				t.Fatalf("socket rejection = %v, response %v, want %d", err, response, test.want)
			}
		})
	}
	auth.RevokeRuntime("alpha")
	if status, _, _ := send(http.MethodGet, "/accounts/alpha/api/v2/state", "", login.SessionToken, false); status != http.StatusUnauthorized {
		t.Fatalf("revoked session status = %d", status)
	}
}
