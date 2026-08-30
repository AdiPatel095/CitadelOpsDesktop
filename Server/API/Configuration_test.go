package API

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"CitadelDesktop/Server/Configuration"
	"CitadelDesktop/Server/History"
	"CitadelDesktop/Server/Reports"
)

func TestConfigurationUpdatePersistsWithoutGameSession(t *testing.T) {
	dataDir := t.TempDir()
	store, err := Configuration.Open(dataDir, map[string]json.RawMessage{
		"scheduler": json.RawMessage(`{"minAttackDelay":4}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	initialRevision := store.Snapshot().Revision
	handler := NewServer(Config{Configuration: store}).Handler()
	request := httptest.NewRequest(
		http.MethodPut,
		"/api/v2/config/scheduler",
		strings.NewReader(`{"value":{"minAttackDelay":9},"expectedRevision":`+jsonUint(initialRevision)+`}`),
	)
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("offline configuration update = %d %s", recorder.Code, recorder.Body.String())
	}
	var snapshot Configuration.Snapshot
	if err := json.NewDecoder(recorder.Body).Decode(&snapshot); err != nil {
		t.Fatal(err)
	}
	if snapshot.Revision != initialRevision+1 || string(snapshot.Sections["scheduler"]) != `{"minAttackDelay":9}` {
		t.Fatalf("updated snapshot = %+v", snapshot)
	}

	// Reopening the store proves this was a durable settings write rather than
	// an in-memory side effect of a live game session or intent operation.
	reopened, err := Configuration.Open(dataDir, nil)
	if err != nil {
		t.Fatal(err)
	}
	if value, ok := reopened.Section("scheduler"); !ok || string(value) != `{"minAttackDelay":9}` {
		t.Fatalf("persisted scheduler = %s, found = %t", value, ok)
	}

	stale := httptest.NewRequest(
		http.MethodPut,
		"/api/v2/config/scheduler",
		strings.NewReader(`{"value":{"minAttackDelay":12},"expectedRevision":`+jsonUint(initialRevision)+`}`),
	)
	staleResult := httptest.NewRecorder()
	handler.ServeHTTP(staleResult, stale)
	if staleResult.Code != http.StatusConflict || !strings.Contains(staleResult.Body.String(), `"configuration_conflict"`) {
		t.Fatalf("stale offline update = %d %s", staleResult.Code, staleResult.Body.String())
	}
}

func TestHostedRuntimeRejectsPortableConfigurationWritesButKeepsLocalConsent(t *testing.T) {
	store, err := Configuration.Open(t.TempDir(), map[string]json.RawMessage{
		"scheduler": json.RawMessage(`{"botLocked":false}`),
		Reports.BattleResearchConfigurationSection: json.RawMessage(`{"enabled":false}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	server := NewServer(Config{Configuration: store, BackgroundOnly: true})
	handler := server.Handler()

	portable := httptest.NewRequest(http.MethodPut, "/api/v2/config/scheduler", strings.NewReader(`{"value":{"botLocked":true}}`))
	portableResult := httptest.NewRecorder()
	handler.ServeHTTP(portableResult, portable)
	if portableResult.Code != http.StatusConflict || !strings.Contains(portableResult.Body.String(), `"configuration_control_plane_owned"`) {
		t.Fatalf("hosted portable update = %d %s", portableResult.Code, portableResult.Body.String())
	}
	consent := httptest.NewRequest(
		http.MethodPut,
		"/api/v2/config/"+Reports.BattleResearchConfigurationSection,
		strings.NewReader(`{"value":{"enabled":true}}`),
	)
	consentResult := httptest.NewRecorder()
	handler.ServeHTTP(consentResult, consent)
	if consentResult.Code != http.StatusOK {
		t.Fatalf("hosted installation consent = %d %s", consentResult.Code, consentResult.Body.String())
	}
}

func TestConfigurationUpdateExpectedValueIgnoresUnrelatedRevisionButRejectsSameSectionConflict(t *testing.T) {
	store, err := Configuration.Open(t.TempDir(), map[string]json.RawMessage{
		"attacks.presets": json.RawMessage(`{"version":1,"presets":[]}`),
		"scheduler":       json.RawMessage(`{"botLocked":false}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	expected, _ := store.Section("attacks.presets")
	if _, err := store.Update("scheduler", json.RawMessage(`{"botLocked":true}`)); err != nil {
		t.Fatal(err)
	}
	handler := NewServer(Config{Configuration: store}).Handler()
	payload, _ := json.Marshal(map[string]any{
		"value":         map[string]any{"version": 1, "presets": []any{map[string]any{"id": "one"}}},
		"expectedValue": json.RawMessage(expected),
	})
	request := httptest.NewRequest(http.MethodPut, "/api/v2/config/attacks.presets", strings.NewReader(string(payload)))
	result := httptest.NewRecorder()
	handler.ServeHTTP(result, request)
	if result.Code != http.StatusOK {
		t.Fatalf("section-scoped update after unrelated revision = %d %s", result.Code, result.Body.String())
	}

	stale := httptest.NewRequest(http.MethodPut, "/api/v2/config/attacks.presets", strings.NewReader(string(payload)))
	staleResult := httptest.NewRecorder()
	handler.ServeHTTP(staleResult, stale)
	if staleResult.Code != http.StatusConflict || !strings.Contains(staleResult.Body.String(), `"configuration_conflict"`) {
		t.Fatalf("same-section conflict = %d %s", staleResult.Code, staleResult.Body.String())
	}
}

func TestConfigurationUpdateRoutesPlayerHistoryRetentionThroughDurableEndpoint(t *testing.T) {
	store, err := Configuration.Open(t.TempDir(), map[string]json.RawMessage{
		History.PlayerSamplesConfigurationSection: json.RawMessage(`{"version":1,"retention":"30d"}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(
		http.MethodPut,
		"/api/v2/config/"+History.PlayerSamplesConfigurationSection,
		strings.NewReader(`{"value":{"version":1,"retention":"none"},"expectedRevision":`+jsonUint(store.Revision())+`}`),
	)
	recorder := httptest.NewRecorder()
	NewServer(Config{Configuration: store}).Handler().ServeHTTP(recorder, request)
	if recorder.Code != http.StatusConflict ||
		!strings.Contains(recorder.Body.String(), `"configuration_requires_retention_apply"`) {
		t.Fatalf("retention configuration update = %d %s", recorder.Code, recorder.Body.String())
	}
	policy := History.ResolvePlayerSamplesRetention(
		store.Snapshot().Sections[History.PlayerSamplesConfigurationSection],
		false,
	)
	if policy.Configured != History.PlayerSamplesRetention30Days {
		t.Fatalf("generic endpoint changed retention: %+v", policy)
	}
}

func jsonUint(value uint64) string {
	encoded, _ := json.Marshal(value)
	return string(encoded)
}
