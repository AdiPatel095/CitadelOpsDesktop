package API

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"CitadelDesktop/Server/Configuration"
	"CitadelDesktop/Server/History"
	"CitadelDesktop/Server/Reports"
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
				policy.Hosted != test.hosted || policy.Maximum != test.maximum || len(policy.Options) != 8 ||
				policy.Revision != configuration.Revision() || policy.RecordingIntervalSeconds != 3600 ||
				len(policy.RecordingIntervalOptions) != 6 {
				t.Fatalf("retention policy = %+v", policy)
			}
			if policy.Storage.Format != "jsonl" || policy.Storage.EstimatedBytesPerRecording <= 0 {
				t.Fatalf("storage estimate = %+v", policy.Storage)
			}
			for _, option := range policy.Options {
				if option.Value == "" || option.Label == "" || option.Description == "" {
					t.Fatalf("incomplete option: %+v", option)
				}
			}
			required := map[History.PlayerSamplesRetention]bool{
				History.PlayerSamplesRetentionNone:    false,
				History.PlayerSamplesRetention24Hours: false,
				History.PlayerSamplesRetention100Days: false,
				History.PlayerSamplesRetention1Year:   false,
			}
			for _, option := range policy.Options {
				if _, ok := required[option.Value]; ok {
					required[option.Value] = true
				}
			}
			for value, present := range required {
				if !present {
					t.Fatalf("required retention option %q is missing from %+v", value, policy.Options)
				}
			}
		})
	}
}

func TestPlayerTrackerRetentionApplyAcceptsCustomWholeDays(t *testing.T) {
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
	custom := History.PlayerSamplesRetention("137d")
	recorder := httptest.NewRecorder()
	request := playerTrackerRetentionApplyRequest(t, custom, configuration.Revision())
	NewServer(Config{Configuration: configuration, History: history}).Handler().ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("custom apply returned HTTP %d: %s", recorder.Code, recorder.Body.String())
	}
	var response playerSamplesRetentionApplyResponse
	if err := json.NewDecoder(recorder.Body).Decode(&response); err != nil {
		t.Fatal(err)
	}
	if response.Policy.Configured != custom || response.Policy.Effective != custom ||
		response.Policy.ConfiguredDays == nil || *response.Policy.ConfiguredDays != 137 ||
		response.Policy.EffectiveDays == nil || *response.Policy.EffectiveDays != 137 {
		t.Fatalf("custom policy = %+v", response.Policy)
	}
}

func TestPlayerTrackerRetentionApplyPatchesCadenceWithoutResettingRetention(t *testing.T) {
	dataDir := t.TempDir()
	configuration, err := Configuration.Open(dataDir, map[string]json.RawMessage{
		History.PlayerSamplesConfigurationSection: json.RawMessage(`{"version":1,"retention":"137d","recordingIntervalSeconds":3600}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	history, err := History.Open(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	recorder := httptest.NewRecorder()
	body := strings.NewReader(fmt.Sprintf(
		`{"recordingIntervalSeconds":300,"expectedRevision":%d}`,
		configuration.Revision(),
	))
	request := httptest.NewRequest(http.MethodPost, "/api/v2/history/player-tracker/retention/apply", body)
	NewServer(Config{Configuration: configuration, History: history}).Handler().ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("cadence apply returned HTTP %d: %s", recorder.Code, recorder.Body.String())
	}
	var response playerSamplesRetentionApplyResponse
	if err := json.NewDecoder(recorder.Body).Decode(&response); err != nil {
		t.Fatal(err)
	}
	if response.Policy.Configured != "137d" || response.Policy.RecordingIntervalSeconds != 300 ||
		response.Report.Retention != "137d" || response.Report.RecordingIntervalSeconds != 300 {
		t.Fatalf("cadence apply response = %+v", response)
	}
}

func TestPlayerTrackerRetentionLegacyApplyPreservesSavedCadence(t *testing.T) {
	dataDir := t.TempDir()
	configuration, err := Configuration.Open(dataDir, map[string]json.RawMessage{
		History.PlayerSamplesConfigurationSection: json.RawMessage(`{"version":1,"retention":"30d","recordingIntervalSeconds":600}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	history, err := History.Open(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	recorder := httptest.NewRecorder()
	request := playerTrackerRetentionApplyRequest(t, History.PlayerSamplesRetention100Days, configuration.Revision())
	NewServer(Config{Configuration: configuration, History: history}).Handler().ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("legacy apply returned HTTP %d: %s", recorder.Code, recorder.Body.String())
	}
	var response playerSamplesRetentionApplyResponse
	if err := json.NewDecoder(recorder.Body).Decode(&response); err != nil {
		t.Fatal(err)
	}
	if response.Policy.Configured != History.PlayerSamplesRetention100Days || response.Policy.RecordingIntervalSeconds != 600 {
		t.Fatalf("legacy apply reset saved cadence: %+v", response.Policy)
	}
}

func TestPlayerTrackerRetentionApplyRejectsUnknownCadence(t *testing.T) {
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
	recorder := httptest.NewRecorder()
	body := strings.NewReader(fmt.Sprintf(
		`{"recordingIntervalSeconds":120,"expectedRevision":%d}`,
		configuration.Revision(),
	))
	request := httptest.NewRequest(http.MethodPost, "/api/v2/history/player-tracker/retention/apply", body)
	NewServer(Config{Configuration: configuration, History: history}).Handler().ServeHTTP(recorder, request)
	if recorder.Code != http.StatusUnprocessableEntity ||
		!strings.Contains(recorder.Body.String(), `"history_recording_interval_invalid"`) {
		t.Fatalf("invalid cadence returned HTTP %d: %s", recorder.Code, recorder.Body.String())
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
		History.PlayerSamplesConfigurationSection: json.RawMessage(`{"version":1,"retention":"none","recordingIntervalSeconds":300}`),
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
		Current                  *History.PlayerSample  `json:"current"`
		Samples                  []History.PlayerSample `json:"samples"`
		IntervalSeconds          int                    `json:"intervalSeconds"`
		RecordingIntervalSeconds int                    `json:"recordingIntervalSeconds"`
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
	if response.IntervalSeconds != 300 || response.RecordingIntervalSeconds != 300 {
		t.Fatalf("history intervals = %+v, want 300", response)
	}
}

func TestResourceAggregatesFilterByViewAndPageOnMinuteBoundary(t *testing.T) {
	reportStore, err := Reports.OpenSQLiteStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = reportStore.Close() })
	now := time.Date(2026, 9, 1, 19, 0, 0, 0, time.UTC)
	for index, id := range []string{"oldest", "middle", "newest"} {
		report := Reports.BattleReport{
			ID: id, AccountUID: 77, WorldID: "world.example", PlayerID: 42,
			AutomationFeature: string(State.AttackFeatureAutoTowers),
			OccurredAt:        now.Add(time.Duration(index) * time.Minute).Add(15 * time.Second).Format(time.RFC3339Nano),
			Role:              "attacker", Result: "Victory", Loot: map[string]int64{"W": int64(index + 1)},
			Defender: &Reports.BattleCombatant{PlayerID: int64(-index - 1), Dummy: true},
		}
		if err := reportStore.Save(t.Context(), report); err != nil {
			t.Fatal(err)
		}
	}
	storm := Reports.BattleReport{
		ID: "storm", AccountUID: 77, WorldID: "world.example", PlayerID: 42,
		AutomationFeature: string(State.AttackFeatureAutoStorm),
		OccurredAt:        now.Add(3 * time.Minute).Format(time.RFC3339Nano), Role: "attacker",
		Loot: map[string]int64{"IAP": 50}, Defender: &Reports.BattleCombatant{PlayerID: -5, Dummy: true},
	}
	if err := reportStore.Save(t.Context(), storm); err != nil {
		t.Fatal(err)
	}
	state := State.NewGameState()
	state.Account.UID = 77
	state.Account.WorldID = "world.example"
	state.Player.ID = 42
	server := NewServer(Config{ReportAnalytics: reportStore, State: State.NewStore(state)})

	firstRecorder := httptest.NewRecorder()
	server.Handler().ServeHTTP(firstRecorder, httptest.NewRequest(
		http.MethodGet,
		"/api/v2/analytics/resource-aggregates?view=tower&limit=2&since="+url.QueryEscape(now.Add(-time.Minute).Format(time.RFC3339Nano)),
		nil,
	))
	if firstRecorder.Code != http.StatusOK {
		t.Fatalf("first resource page returned HTTP %d: %s", firstRecorder.Code, firstRecorder.Body.String())
	}
	var first resourceAggregateResponse
	if err := json.NewDecoder(firstRecorder.Body).Decode(&first); err != nil {
		t.Fatal(err)
	}
	if len(first.Aggregates) != 2 || first.Aggregates[0].Resources["W"] != 3 ||
		first.Aggregates[1].Resources["W"] != 2 || first.NextBefore == nil || first.SourceBucketSeconds != 60 {
		t.Fatalf("first resource page = %+v", first)
	}

	secondRecorder := httptest.NewRecorder()
	server.Handler().ServeHTTP(secondRecorder, httptest.NewRequest(
		http.MethodGet,
		"/api/v2/analytics/resource-aggregates?view=tower&limit=2&since="+url.QueryEscape(now.Add(-time.Minute).Format(time.RFC3339Nano))+"&before="+url.QueryEscape(first.NextBefore.Format(time.RFC3339Nano)),
		nil,
	))
	if secondRecorder.Code != http.StatusOK {
		t.Fatalf("second resource page returned HTTP %d: %s", secondRecorder.Code, secondRecorder.Body.String())
	}
	var second resourceAggregateResponse
	if err := json.NewDecoder(secondRecorder.Body).Decode(&second); err != nil {
		t.Fatal(err)
	}
	if len(second.Aggregates) != 1 || second.Aggregates[0].Resources["W"] != 1 || second.NextBefore != nil {
		t.Fatalf("second resource page = %+v", second)
	}

	invalidRecorder := httptest.NewRecorder()
	server.Handler().ServeHTTP(invalidRecorder, httptest.NewRequest(http.MethodGet, "/api/v2/analytics/resource-aggregates?view=unknown", nil))
	if invalidRecorder.Code != http.StatusUnprocessableEntity || !strings.Contains(invalidRecorder.Body.String(), `"resource_view_invalid"`) {
		t.Fatalf("invalid view returned HTTP %d: %s", invalidRecorder.Code, invalidRecorder.Body.String())
	}
}
