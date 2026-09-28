package Accounts

import (
	"CitadelDesktop/Server/Protocol"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestProtectedProcessDiagnosticsPrivacy(t *testing.T) {
	supervisor, _, orchestrator, now := newTestOrchestrator(t)
	_, err := orchestrator.Reconcile(t.Context(), ReconcileRequest{SchemaVersion: 1, Revision: 1, Runtimes: []RuntimeAssignment{testAssignment("private-runtime-label", "private-tenant-label", 1, now.Add(time.Minute))}})
	if err != nil {
		t.Fatal(err)
	}
	app, ok := supervisor.Application("private-runtime-label")
	if !ok {
		t.Fatal("missing runtime")
	}
	app.Telemetry.RecordRaw("private-payload-secret", Protocol.DirectionInbound, time.Now(), nil)
	for _, token := range []string{"", "wrong", testControlToken} {
		req := httptest.NewRequest(http.MethodGet, "/orchestrator/v1/diagnostics", nil)
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		w := httptest.NewRecorder()
		orchestrator.Handler().ServeHTTP(w, req)
		if token != testControlToken {
			if w.Code != http.StatusUnauthorized {
				t.Fatalf("unauthorized=%d", w.Code)
			}
			continue
		}
		if w.Code != http.StatusOK {
			t.Fatalf("diagnostics=%d", w.Code)
		}
		for _, secret := range []string{"private-runtime-label", "private-tenant-label", "private-payload-secret", testControlToken, "error", "path"} {
			if strings.Contains(w.Body.String(), secret) {
				t.Fatalf("private content: %s", secret)
			}
		}
		var body struct {
			Process         struct{ Scope string }
			TelemetryStores int
		}
		if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
		if body.Process.Scope != "process" || body.TelemetryStores != 1 {
			t.Fatal("wrong aggregation scope")
		}
	}
}
