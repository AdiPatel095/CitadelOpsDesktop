package Accounts

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"sort"
	"testing"
	"time"

	"CitadelDesktop/Server/Configuration"
	"CitadelDesktop/internal/contractfill"
)

func contractOrchestrator(t *testing.T) (*Orchestrator, time.Time) {
	t.Helper()
	_, _, orchestrator, clock := newTestOrchestratorWithClock(t)
	clock.mu.Lock()
	clock.now = time.Date(2026, 10, 3, 12, 34, 56, 123456789, time.UTC)
	clock.mu.Unlock()
	return orchestrator, clock.Now()
}

// Permanent version of Sophie's overlay: compare the real first-apply and
// idempotent response bodies with their distinct acknowledgement goldens.
func TestQAConfigurationResponseMatchesFixture(t *testing.T) {
	configurationResponseGoldens(t, false)
}

func configurationResponseGoldens(t *testing.T, update bool) {
	t.Helper()
	orchestrator, now := contractOrchestrator(t)
	snapshot := Configuration.Snapshot{SchemaVersion: Configuration.SchemaVersion, Revision: 7, UpdatedAt: now,
		Sections: map[string]json.RawMessage{"scheduler": json.RawMessage(`{"minAttackDelay":9}`)}}
	_, digest, err := canonicalConfigurationDigest(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	assignment := testAssignment("qa-synthetic-runtime", "qa-synthetic-tenant", 3, now.Add(10*time.Minute))
	assignment.StartSession = false
	assignment.DesiredConfigurationRevision = snapshot.Revision
	assignment.DesiredConfigurationDigest = digest
	if _, err := orchestrator.Reconcile(t.Context(), ReconcileRequest{SchemaVersion: 1, Revision: 1, Runtimes: []RuntimeAssignment{assignment}}); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(orchestrator.Handler())
	defer server.Close()
	request := ConfigurationSyncRequest{SchemaVersion: 1, PlacementEpoch: assignment.PlacementEpoch, Digest: digest, Configuration: snapshot}
	for _, mode := range []string{"first", "repeat"} {
		t.Run(mode, func(t *testing.T) {
			response, raw := controlRequest(t, http.MethodPut, server.URL+"/orchestrator/v1/runtimes/qa-synthetic-runtime/configuration", request)
			if response.StatusCode != http.StatusOK {
				t.Fatalf("status %d: %s", response.StatusCode, raw)
			}
			var object map[string]json.RawMessage
			if err := json.Unmarshal(raw, &object); err != nil {
				t.Fatal(err)
			}
			expected := []string{"appliedConfigurationDigest", "appliedConfigurationRevision", "configurationState", "placementEpoch", "runtimeId"}
			if mode == "first" {
				expected = append(expected, "runtimeConfigurationRevision", "changedSections")
			}
			actual := []string{}
			for key := range object {
				actual = append(actual, key)
			}
			sort.Strings(actual)
			sort.Strings(expected)
			if !reflect.DeepEqual(actual, expected) {
				t.Fatalf("%s response keys %v; want %v", mode, actual, expected)
			}
			t.Logf("%s HTTP 200 acknowledgement: %d keys", mode, len(actual))
			contractfill.GoldenBody(t, "testdata/contracts/worker", "configuration-sync-ack."+mode, raw, update)
		})
	}
}

func controlFenceGolden(t *testing.T, update bool) {
	t.Helper()
	orchestrator, _ := contractOrchestrator(t)
	request := httptest.NewRequest(http.MethodPost, "/orchestrator/v1/control-fence", nil)
	request.Header.Set("Authorization", "Bearer "+testControlToken)
	request.Header.Set(controlEpochHeader, "7")
	response := httptest.NewRecorder()
	orchestrator.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status %d: %s", response.Code, response.Body.String())
	}
	var status CellStatus
	if err := json.Unmarshal(response.Body.Bytes(), &status); err != nil {
		t.Fatal(err)
	}
	if status.CellID != "cell-one" || status.ControlFenceSchema != 1 || status.ControlEpoch != 7 {
		t.Fatalf("fence status %+v", status)
	}
	contractfill.GoldenBody(t, "testdata/contracts/worker", "control-fence", response.Body.Bytes(), update)
}
