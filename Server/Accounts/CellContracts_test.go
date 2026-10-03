package Accounts

import (
	"bytes"
	"flag"
	"net/http/httptest"
	"testing"

	"CitadelDesktop/internal/contractfill"
)

var updateCellContracts = flag.Bool("update", false, "regenerate sender cell contract fixtures")

func TestGenerateCellContracts(t *testing.T) {
	defer contractfill.Inventory(t, "testdata/contracts/worker", []string{"status", "reconcile-response"}, "configuration-sync-ack.first", "configuration-sync-ack.repeat", "control-fence")
	for _, name := range []string{"status", "reconcile-response"} {
		t.Run(name, func(t *testing.T) {
			var status CellStatus
			if err := contractfill.Fill(&status, map[string]any{
				"cpuWindowSeconds": 20.030486354,
				"memoryLimitBytes": uint64(9223372036854775807),
				"postGcHeapBytes":  uint64(9223372036854775806),
			}); err != nil {
				t.Fatal(err)
			}
			contractfill.Golden(t, "testdata/contracts/worker", name, &status, *updateCellContracts)
		})
	}
	t.Run("configuration-sync-ack", func(t *testing.T) { configurationResponseGoldens(t, *updateCellContracts) })
	t.Run("control-fence", func(t *testing.T) { controlFenceGolden(t, *updateCellContracts) })
}

func TestReceiveCellContracts(t *testing.T) {
	contractfill.Inventory(t, "testdata/contracts/backend", []string{"reconcile", "configuration-sync", "dashboard-grant", "dashboard-bootstrap", "login-credential"})
	for name, target := range map[string]any{
		"reconcile":           &ReconcileRequest{},
		"configuration-sync":  &ConfigurationSyncRequest{},
		"dashboard-grant":     &DashboardGrantRequest{},
		"dashboard-bootstrap": &DashboardBootstrapRequest{},
		"login-credential":    &LoginCredentialRequest{},
	} {
		t.Run(name, func(t *testing.T) {
			raw, keys, optional := contractfill.ReadReceiver(t, "testdata/contracts/backend", name)
			request := httptest.NewRequest("POST", "/orchestrator/v1", bytes.NewReader(raw))
			request.Header.Set("Content-Type", "application/json")
			if err := decodeControlJSON(httptest.NewRecorder(), request, target); err != nil {
				t.Fatalf("production worker decode: %v", err)
			}
			if err := contractfill.Verify(raw, target, keys, optional); err != nil {
				t.Fatal(err)
			}
		})
	}
}
