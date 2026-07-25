package API

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"CitadelDesktop/Server/Configuration"
)

func TestConfigurationSettingsBundleRoundTrip(t *testing.T) {
	store, err := Configuration.Open(t.TempDir(), map[string]json.RawMessage{
		"automation.enabled": json.RawMessage(`{"autoStorm":false}`),
		"scheduler":          json.RawMessage(`{"minAttackDelay":4}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(NewServer(Config{
		Version:       "2.0.0-alpha",
		Configuration: store,
	}).Handler())
	defer server.Close()

	response, err := http.Get(server.URL + "/api/v2/config/export")
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("export returned HTTP %d", response.StatusCode)
	}
	if disposition := response.Header.Get("Content-Disposition"); !strings.Contains(disposition, "CitadelOps-Settings-") {
		t.Fatalf("export content disposition = %q", disposition)
	}
	var bundle settingsBundle
	if err := json.NewDecoder(response.Body).Decode(&bundle); err != nil {
		t.Fatal(err)
	}
	if bundle.Format != settingsBundleFormat || bundle.FormatVersion != settingsBundleVersion ||
		bundle.AppVersion != "2.0.0-alpha" {
		t.Fatalf("unexpected export metadata: %+v", bundle)
	}
	bundle.Configuration.Sections["automation.enabled"] = json.RawMessage(`{"autoStorm":true}`)
	bundle.Configuration.Sections["scheduler"] = json.RawMessage(`{"minAttackDelay":6}`)
	bundle.ClientPreferences = map[string]string{"theme": "light"}
	contents, err := json.Marshal(bundle)
	if err != nil {
		t.Fatal(err)
	}
	response, err = http.Post(server.URL+"/api/v2/config/import", "application/json", bytes.NewReader(contents))
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("import returned HTTP %d", response.StatusCode)
	}
	var result settingsImportResult
	if err := json.NewDecoder(response.Body).Decode(&result); err != nil {
		t.Fatal(err)
	}
	if result.ImportedSections != 2 || result.ChangedSections != 2 ||
		result.IncludedClientPreferences != 1 || result.Revision != 1 {
		t.Fatalf("unexpected import result: %+v", result)
	}
	if value, ok := store.Section("automation.enabled"); !ok || string(value) != `{"autoStorm":true}` {
		t.Fatalf("imported automation section = %s, found = %t", value, ok)
	}
	if value, ok := store.Section("scheduler"); !ok || string(value) != `{"minAttackDelay":6}` {
		t.Fatalf("imported scheduler section = %s, found = %t", value, ok)
	}
}

func TestConfigurationSettingsBundleRejectsUnsupportedFormat(t *testing.T) {
	store, err := Configuration.Open(t.TempDir(), map[string]json.RawMessage{
		"scheduler": json.RawMessage(`{"minAttackDelay":4}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/v2/config/import", strings.NewReader(`{
		"format":"other-app",
		"formatVersion":1,
		"exportedAt":"2026-07-24T04:00:00Z",
		"configuration":{"schemaVersion":1,"sections":{"scheduler":{"minAttackDelay":9}}}
	}`))
	NewServer(Config{Configuration: store}).Handler().ServeHTTP(recorder, request)
	if recorder.Code != http.StatusUnprocessableEntity {
		t.Fatalf("unsupported import returned HTTP %d", recorder.Code)
	}
	if value, _ := store.Section("scheduler"); string(value) != `{"minAttackDelay":4}` {
		t.Fatalf("unsupported import changed settings: %s", value)
	}
}
