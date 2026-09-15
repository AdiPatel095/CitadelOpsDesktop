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

func TestHostedRuntimeRejectsPortableAndRetiredConfigurationWrites(t *testing.T) {
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
	retired := httptest.NewRequest(
		http.MethodPut,
		"/api/v2/config/"+Reports.BattleResearchConfigurationSection,
		strings.NewReader(`{"value":{"enabled":true}}`),
	)
	retiredResult := httptest.NewRecorder()
	handler.ServeHTTP(retiredResult, retired)
	if retiredResult.Code != http.StatusGone || !strings.Contains(retiredResult.Body.String(), `"configuration_section_retired"`) {
		t.Fatalf("retired configuration update = %d %s", retiredResult.Code, retiredResult.Body.String())
	}
}

func TestRetiredBattleResearchStatusRouteIsNotRegistered(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "/api/v2/battle-research", nil)
	recorder := httptest.NewRecorder()
	NewServer(Config{}).Handler().ServeHTTP(recorder, request)
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("retired battle research route = %d %s", recorder.Code, recorder.Body.String())
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

func TestConfigurationUpdateCanPreserveDisableOrCorrectInvalidLegacyFeast(t *testing.T) {
	legacy := json.RawMessage(`{"version":1,"checkIntervalSec":1800,"feast":{"enabled":true,"minimumRemainingHours":0}}`)
	newHandler := func(t *testing.T) (http.Handler, *Configuration.Store) {
		t.Helper()
		store, err := Configuration.Open(t.TempDir(), map[string]json.RawMessage{"automation.autoBuyer": legacy})
		if err != nil {
			t.Fatal(err)
		}
		return NewServer(Config{Configuration: store}).Handler(), store
	}
	request := func(t *testing.T, handler http.Handler, value json.RawMessage, expected any) *httptest.ResponseRecorder {
		t.Helper()
		payload, err := json.Marshal(map[string]any{"value": value, "expectedValue": expected})
		if err != nil {
			t.Fatal(err)
		}
		result := httptest.NewRecorder()
		handler.ServeHTTP(result, httptest.NewRequest(http.MethodPut, "/api/v2/config/automation.autoBuyer", strings.NewReader(string(payload))))
		return result
	}

	handler, _ := newHandler(t)
	preserved := request(t, handler, json.RawMessage(`{
		"version":1,"checkIntervalSec":3600,"feast":{"enabled":true,"minimumRemainingHours":0}
	}`), legacy)
	if preserved.Code != http.StatusOK {
		t.Fatalf("unrelated legacy edit = %d %s", preserved.Code, preserved.Body.String())
	}

	handler, _ = newHandler(t)
	disabled := request(t, handler, json.RawMessage(`{
		"version":1,"checkIntervalSec":1800,"feast":{"enabled":false,"minimumRemainingHours":0}
	}`), legacy)
	if disabled.Code != http.StatusOK {
		t.Fatalf("legacy feast disable = %d %s", disabled.Code, disabled.Body.String())
	}

	handler, _ = newHandler(t)
	corrected := request(t, handler, json.RawMessage(`{
		"version":1,"checkIntervalSec":1800,"feast":{"enabled":true,"minimumRemainingHours":12}
	}`), legacy)
	if corrected.Code != http.StatusOK {
		t.Fatalf("legacy feast correction = %d %s", corrected.Code, corrected.Body.String())
	}

	handler, _ = newHandler(t)
	invalid := request(t, handler, json.RawMessage(`{
		"version":1,"checkIntervalSec":1800,"feast":{"enabled":true,"minimumRemainingHours":721}
	}`), nil)
	if invalid.Code != http.StatusUnprocessableEntity || !strings.Contains(invalid.Body.String(), "whole number from 1 to 720") {
		t.Fatalf("changed invalid feast = %d %s", invalid.Code, invalid.Body.String())
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
