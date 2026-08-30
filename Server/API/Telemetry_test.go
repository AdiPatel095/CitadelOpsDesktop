package API

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"CitadelDesktop/Server/State"
	"CitadelDesktop/Server/Telemetry"
)

func TestAttackLaunchRatesExposeHourlyAndDailyCountsForEveryFeature(t *testing.T) {
	telemetry := Telemetry.NewStore(100)
	defer telemetry.Close()
	features := []struct {
		actor     string
		featureID State.AttackFeatureID
	}{
		{actor: "automation:autoTowers", featureID: State.AttackFeatureAutoTowers},
		{actor: "automation:autoInvasion", featureID: State.AttackFeatureAutoInvasion},
		{actor: "automation:autoNomad", featureID: State.AttackFeatureAutoNomad},
		{actor: "automation:autoAdvisor", featureID: State.AttackFeatureAutoAdvisor},
		{actor: "automation:autoKhan", featureID: State.AttackFeatureAutoKhan},
		{actor: "automation:autoBeriWorld", featureID: State.AttackFeatureAutoBeriWorld},
		{actor: "automation:autoStorm", featureID: State.AttackFeatureAutoStorm},
	}
	for _, feature := range features {
		telemetry.RecordFeatureActivity(feature.actor, "test.attack", "INFO", "ATTACK", "Launched test attack")
	}

	server := &Server{config: Config{Telemetry: telemetry}}
	recorder := httptest.NewRecorder()
	server.handleAttackLaunchRates(recorder, httptest.NewRequest(http.MethodGet, "/api/v2/telemetry/attack-rates", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", recorder.Code, recorder.Body.String())
	}
	var response struct {
		WindowMinutes          int            `json:"windowMinutes"`
		DailyWindowMinutes     int            `json:"dailyWindowMinutes"`
		LaunchesByFeature      map[string]int `json:"launchesByFeature"`
		DailyLaunchesByFeature map[string]int `json:"dailyLaunchesByFeature"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if response.WindowMinutes != 60 || response.DailyWindowMinutes != 1440 {
		t.Fatalf("attack-rate response = %#v", response)
	}
	if len(response.LaunchesByFeature) != len(features) || len(response.DailyLaunchesByFeature) != len(features) {
		t.Fatalf("feature coverage = hourly %d daily %d, want %d/%d", len(response.LaunchesByFeature), len(response.DailyLaunchesByFeature), len(features), len(features))
	}
	for _, feature := range features {
		featureID := string(feature.featureID)
		if response.LaunchesByFeature[featureID] != 1 || response.DailyLaunchesByFeature[featureID] != 1 {
			t.Errorf("%s hourly/daily counts = %d/%d, want 1/1", featureID, response.LaunchesByFeature[featureID], response.DailyLaunchesByFeature[featureID])
		}
	}
}

func TestTelemetryTailRejectsPrivateDiagnosticChannels(t *testing.T) {
	telemetry := Telemetry.NewStore(100)
	defer telemetry.Close()
	telemetry.RecordFeatureActivity("automation:autoBird", "troops.station", "INFO", "TRANSPORT", "Stationed troops")
	server := &Server{config: Config{Telemetry: telemetry}}

	activityRequest := httptest.NewRequest(http.MethodGet, "/api/v2/telemetry/activity", nil)
	activityRequest.SetPathValue("channel", Telemetry.ChannelActivity)
	activityResponse := httptest.NewRecorder()
	server.handleTelemetryTail(activityResponse, activityRequest)
	if activityResponse.Code != http.StatusOK || !strings.Contains(activityResponse.Body.String(), "Stationed troops") {
		t.Fatalf("activity response = %d %s", activityResponse.Code, activityResponse.Body.String())
	}

	diagnosticRequest := httptest.NewRequest(http.MethodGet, "/api/v2/telemetry/websocket_game", nil)
	diagnosticRequest.SetPathValue("channel", Telemetry.ChannelWebSocketGame)
	diagnosticResponse := httptest.NewRecorder()
	server.handleTelemetryTail(diagnosticResponse, diagnosticRequest)
	if diagnosticResponse.Code != http.StatusNotFound {
		t.Fatalf("diagnostic response = %d %s", diagnosticResponse.Code, diagnosticResponse.Body.String())
	}
}
