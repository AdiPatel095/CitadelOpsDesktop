package API

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"CitadelDesktop/Server/Configuration"
	"CitadelDesktop/Server/History"
	"CitadelDesktop/Server/State"
)

func TestPlayerTrackerRetentionEndpointReportsConfiguredAndEffectivePolicy(t *testing.T) {
	configuration, err := Configuration.Open(t.TempDir(), map[string]json.RawMessage{
		History.PlayerSamplesConfigurationSection: json.RawMessage(`{"version":1,"retention":"unlimited"}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name      string
		hosted    bool
		effective History.PlayerSamplesRetention
		maximum   History.PlayerSamplesRetention
	}{
		{"local", false, History.PlayerSamplesRetentionUnlimited, History.PlayerSamplesRetentionUnlimited},
		{"hosted", true, History.PlayerSamplesRetention30Days, History.PlayerSamplesRetention30Days},
	} {
		t.Run(test.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			request := httptest.NewRequest(http.MethodGet, "/api/v2/history/player-tracker/retention", nil)
			NewServer(Config{Configuration: configuration, BackgroundOnly: test.hosted}).Handler().ServeHTTP(recorder, request)
			if recorder.Code != http.StatusOK {
				t.Fatalf("retention returned HTTP %d: %s", recorder.Code, recorder.Body.String())
			}
			if cacheControl := recorder.Header().Get("Cache-Control"); cacheControl != "no-store" {
				t.Fatalf("Cache-Control = %q, want no-store", cacheControl)
			}
			var policy History.PlayerSamplesRetentionPolicy
			if err := json.NewDecoder(recorder.Body).Decode(&policy); err != nil {
				t.Fatal(err)
			}
			if policy.Configured != History.PlayerSamplesRetentionUnlimited || policy.Effective != test.effective ||
				policy.Hosted != test.hosted || policy.Maximum != test.maximum || len(policy.Options) != 7 ||
				policy.Revision != configuration.Revision() {
				t.Fatalf("retention policy = %+v", policy)
			}
			for _, option := range policy.Options {
				if option.Value == "" || option.Label == "" || option.Description == "" {
					t.Fatalf("incomplete option: %+v", option)
				}
			}
		})
	}
}

func TestPlayerTrackerRetentionApplyCompactsBeforeAcknowledging(t *testing.T) {
	dataDir := t.TempDir()
	configuration, err := Configuration.Open(dataDir, map[string]json.RawMessage{
		History.PlayerSamplesConfigurationSection: json.RawMessage(`{"version":1,"retention":"30d"}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	history, err := History.Open(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	if err := history.Append(History.CollectionPlayerSamples, History.PlayerSample{
		TimestampUnix: time.Now().UTC().Unix(), PlayerID: 42, Might: 123,
	}); err != nil {
		t.Fatal(err)
	}
	recorder := httptest.NewRecorder()
	request := playerTrackerRetentionApplyRequest(t, History.PlayerSamplesRetentionNone, configuration.Revision())
	NewServer(Config{Configuration: configuration, History: history}).Handler().ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("apply returned HTTP %d: %s", recorder.Code, recorder.Body.String())
	}
	if cacheControl := recorder.Header().Get("Cache-Control"); cacheControl != "no-store" {
		t.Fatalf("Cache-Control = %q, want no-store", cacheControl)
	}
	var response playerSamplesRetentionApplyResponse
	if err := json.NewDecoder(recorder.Body).Decode(&response); err != nil {
		t.Fatal(err)
	}
	if response.Policy.Configured != History.PlayerSamplesRetentionNone ||
		response.Policy.Effective != History.PlayerSamplesRetentionNone ||
		response.Policy.Revision == 0 ||
		!response.Report.Complete || response.Report.Retention != History.PlayerSamplesRetentionNone ||
		response.Report.DeletedRows != 1 {
		t.Fatalf("apply response = %+v", response)
	}
	samples, err := history.PlayerSamplesForPlayer(time.Time{}, 100, 42)
	if err != nil {
		t.Fatal(err)
	}
	if len(samples) != 0 {
		t.Fatalf("apply acknowledged before clearing history: %+v", samples)
	}
}

func TestPlayerTrackerRetentionApplyRemainsInstallationLocalOnHostedRuntime(t *testing.T) {
	dataDir := t.TempDir()
	configuration, err := Configuration.Open(dataDir, map[string]json.RawMessage{
		History.PlayerSamplesConfigurationSection: json.RawMessage(`{"version":1,"retention":"30d"}`),
		"scheduler": json.RawMessage(`{"botLocked":false}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	configuration.SetExternalAuthority(true, History.PlayerSamplesConfigurationSection)
	history, err := History.Open(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	server := NewServer(Config{
		Configuration:  configuration,
		History:        history,
		BackgroundOnly: true,
	})
	server.SetExternalConfigurationAuthority(true)

	recorder := httptest.NewRecorder()
	request := playerTrackerRetentionApplyRequest(t, History.PlayerSamplesRetention1Year, configuration.Revision())
	server.Handler().ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("hosted apply returned HTTP %d: %s", recorder.Code, recorder.Body.String())
	}
	var response playerSamplesRetentionApplyResponse
	if err := json.NewDecoder(recorder.Body).Decode(&response); err != nil {
		t.Fatal(err)
	}
	if response.Policy.Configured != History.PlayerSamplesRetention1Year ||
		response.Policy.Effective != History.PlayerSamplesRetention30Days ||
		!response.Policy.Hosted || response.Policy.Maximum != History.PlayerSamplesRetention30Days {
		t.Fatalf("hosted retention policy = %+v", response.Policy)
	}
	if _, err := configuration.Update("scheduler", json.RawMessage(`{"botLocked":true}`)); !errors.Is(err, Configuration.ErrExternalAuthority) {
		t.Fatalf("portable hosted update error = %v, want external authority", err)
	}
}

func TestPlayerTrackerRetentionApplyReturnsFailureWhenCompactionFails(t *testing.T) {
	dataDir := t.TempDir()
	configuration, err := Configuration.Open(dataDir, map[string]json.RawMessage{
		History.PlayerSamplesConfigurationSection: json.RawMessage(`{"version":1,"retention":"30d"}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	history, err := History.Open(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	historyDirectory := filepath.Join(dataDir, "History")
	if err := os.RemoveAll(historyDirectory); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(historyDirectory, []byte("not a directory"), 0o600); err != nil {
		t.Fatal(err)
	}

	recorder := httptest.NewRecorder()
	request := playerTrackerRetentionApplyRequest(t, History.PlayerSamplesRetentionNone, configuration.Revision())
	NewServer(Config{Configuration: configuration, History: history}).Handler().ServeHTTP(recorder, request)
	if recorder.Code != http.StatusInternalServerError ||
		!strings.Contains(recorder.Body.String(), `"history_retention_apply_failed"`) {
		t.Fatalf("failed apply returned HTTP %d: %s", recorder.Code, recorder.Body.String())
	}
}

func TestPlayerTrackerRetentionApplyRejectsStaleRevisionWithoutChangingHistory(t *testing.T) {
	dataDir := t.TempDir()
	configuration, err := Configuration.Open(dataDir, map[string]json.RawMessage{
		History.PlayerSamplesConfigurationSection: json.RawMessage(`{"version":1,"retention":"30d"}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	history, err := History.Open(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	if err := history.Append(History.CollectionPlayerSamples, History.PlayerSample{
		TimestampUnix: time.Now().UTC().Unix(), PlayerID: 42, Might: 123,
	}); err != nil {
		t.Fatal(err)
	}
	staleRevision := configuration.Revision()
	if _, err := configuration.Update("scheduler", json.RawMessage(`{"minAttackDelay":9}`)); err != nil {
		t.Fatal(err)
	}

	recorder := httptest.NewRecorder()
	request := playerTrackerRetentionApplyRequest(t, History.PlayerSamplesRetentionNone, staleRevision)
	NewServer(Config{Configuration: configuration, History: history}).Handler().ServeHTTP(recorder, request)
	if recorder.Code != http.StatusConflict ||
		!strings.Contains(recorder.Body.String(), `"history_retention_conflict"`) {
		t.Fatalf("stale apply returned HTTP %d: %s", recorder.Code, recorder.Body.String())
	}
	policy := History.ResolvePlayerSamplesRetention(
		configuration.Snapshot().Sections[History.PlayerSamplesConfigurationSection],
		false,
	)
	if policy.Configured != History.PlayerSamplesRetention30Days {
		t.Fatalf("stale apply changed configured retention: %+v", policy)
	}
	samples, err := history.PlayerSamplesForPlayer(time.Time{}, 100, 42)
	if err != nil {
		t.Fatal(err)
	}
	if len(samples) != 1 {
		t.Fatalf("stale apply changed history: %+v", samples)
	}
}

func playerTrackerRetentionApplyRequest(
	t *testing.T,
	retention History.PlayerSamplesRetention,
	expectedRevision uint64,
) *http.Request {
	t.Helper()
	body := strings.NewReader(fmt.Sprintf(
		`{"retention":%q,"expectedRevision":%d}`,
		retention,
		expectedRevision,
	))
	return httptest.NewRequest(http.MethodPost, "/api/v2/history/player-tracker/retention/apply", body)
}

func TestPlayerTrackerHistorySinceHonorsAllAndCapsNumericRanges(t *testing.T) {
	now := time.Date(2026, 8, 29, 12, 0, 0, 0, time.UTC)
	tests := []struct {
		name      string
		raw       string
		retention History.PlayerSamplesRetention
		want      time.Time
	}{
		{"all year", "all", History.PlayerSamplesRetention1Year, now.Add(-365 * 24 * time.Hour)},
		{"all unlimited", " ALL ", History.PlayerSamplesRetentionUnlimited, time.Time{}},
		{"numeric capped", "2592000", History.PlayerSamplesRetention24Hours, now.Add(-24 * time.Hour)},
		{"numeric unlimited", "7776000", History.PlayerSamplesRetentionUnlimited, now.Add(-90 * 24 * time.Hour)},
		{"none", "all", History.PlayerSamplesRetentionNone, now},
		{"invalid defaults capped", "invalid", History.PlayerSamplesRetention7Days, now.Add(-7 * 24 * time.Hour)},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := playerTrackerHistorySince(now, test.raw, test.retention); !got.Equal(test.want) {
				t.Fatalf("since = %s, want %s", got, test.want)
			}
		})
	}
}

func TestPlayerTrackerHistoryNoneStillReturnsCurrentValues(t *testing.T) {
	dataDir := t.TempDir()
	configuration, err := Configuration.Open(dataDir, map[string]json.RawMessage{
		History.PlayerSamplesConfigurationSection: json.RawMessage(`{"version":1,"retention":"none"}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	history, err := History.Open(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	state := State.NewGameState()
	state.Player.ID = 42
	state.Player.Might = 12345
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/v2/history/player-tracker?rangeSeconds=all", nil)
	NewServer(Config{
		Configuration: configuration,
		History:       history,
		State:         State.NewStore(state),
	}).Handler().ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("history returned HTTP %d: %s", recorder.Code, recorder.Body.String())
	}
	var response struct {
		Current *History.PlayerSample  `json:"current"`
		Samples []History.PlayerSample `json:"samples"`
	}
	if err := json.NewDecoder(recorder.Body).Decode(&response); err != nil {
		t.Fatal(err)
	}
	if response.Current == nil || response.Current.PlayerID != 42 || response.Current.Might != 12345 {
		t.Fatalf("current values = %+v", response.Current)
	}
	if len(response.Samples) != 0 {
		t.Fatalf("none retention returned historical samples: %+v", response.Samples)
	}
}
