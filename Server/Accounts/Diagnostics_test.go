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
	// CIT-47: a hosted runtime has no telemetry store; recording is a no-op.
	if app.Telemetry != nil {
		t.Fatal("hosted runtime created a telemetry store")
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
			Process                  struct{ Scope string }
			Telemetry                map[string]any
			TelemetryStores          int
			StatePersistence         map[string]any
			StatePersistenceRuntimes int
		}
		if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
		if body.Process.Scope != "process" || body.TelemetryStores != 0 {
			t.Fatal("wrong aggregation scope")
		}
		if body.StatePersistenceRuntimes != 1 || body.StatePersistence == nil {
			t.Fatal("missing state persistence aggregation")
		}
		for _, name := range []string{"flushes", "fileSyncs", "directorySyncs", "skippedVolatileWrites"} {
			if _, ok := body.StatePersistence[name].(float64); !ok {
				t.Fatalf("statePersistence.%s is not numeric", name)
			}
		}
		// The fields stay in the response (rollout gates read them) and are zero.
		if body.Telemetry == nil {
			t.Fatal("diagnostics dropped the telemetry object")
		}
		for name, value := range body.Telemetry {
			switch typed := value.(type) {
			case float64:
				if typed != 0 {
					t.Fatalf("telemetry.%s = %v on a hosted cell, want 0", name, typed)
				}
			case string:
				if typed != "" && !strings.HasPrefix(typed, "0001-01-01") {
					t.Fatalf("telemetry.%s = %q on a hosted cell, want empty", name, typed)
				}
			case bool:
				if typed {
					t.Fatalf("telemetry.%s = true on a hosted cell", name)
				}
			}
		}
	}
}
