package API

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"CitadelDesktop/Server/State"
	"CitadelDesktop/Server/Telemetry"
)

type stubAttackLaunches struct {
	hourly, daily map[string]int
	hourlyOK      bool
	dailyOK       bool
	windows       []time.Duration
}

func (stub *stubAttackLaunches) AttackLaunchCountsSince(since time.Time, now time.Time) (map[string]int, bool) {
	stub.windows = append(stub.windows, now.Sub(since))
	if now.Sub(since) <= time.Hour+time.Second {
		return stub.hourly, stub.hourlyOK
	}
	return stub.daily, stub.dailyOK
}

func hostedRatesServer(source *stubAttackLaunches) *Server {
	gameState := State.NewGameState()
	gameState.DailyAttacks.SessionStartedAt = time.Now().Add(-6 * time.Hour).UTC()
	config := Config{BackgroundOnly: true, State: State.NewStore(&gameState)}
	if source != nil {
		config.AttackLaunches = source
	}
	return NewServer(config)
}

func getJSON(t *testing.T, handler http.Handler, path string) (int, map[string]json.RawMessage) {
	t.Helper()
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, path, nil))
	var body map[string]json.RawMessage
	_ = json.Unmarshal(recorder.Body.Bytes(), &body)
	return recorder.Code, body
}

func TestHostedWorkerServesNoTelemetryRoutes(t *testing.T) {
	handler := hostedRatesServer(&stubAttackLaunches{}).Handler()
	for _, path := range []string{"/api/v2/telemetry/channels", "/api/v2/telemetry/attack-rates", "/api/v2/telemetry/autotowers", "/api/v2/telemetry/websocket_game"} {
		if code, _ := getJSON(t, handler, path); code != http.StatusNotFound {
			t.Errorf("hosted GET %s = %d, want 404", path, code)
		}
	}
}

func TestHostedAttackRatesKeepTheV2ShapeForEveryOffenseFeature(t *testing.T) {
	counts := map[string]int{}
	for index, channel := range Telemetry.AttackFeatureChannels() {
		counts[channel] = index + 1
	}
	source := &stubAttackLaunches{hourly: counts, hourlyOK: true, daily: counts, dailyOK: true}
	code, body := getJSON(t, hostedRatesServer(source).Handler(), "/api/v2/automations/attack-rates")
	if code != http.StatusOK {
		t.Fatalf("status %d", code)
	}
	var window int
	var hourly map[string]int
	var daily struct {
		StartedAt         time.Time      `json:"startedAt"`
		LaunchesByFeature map[string]int `json:"launchesByFeature"`
	}
	if json.Unmarshal(body["windowMinutes"], &window) != nil || json.Unmarshal(body["launchesByFeature"], &hourly) != nil ||
		json.Unmarshal(body["dailySession"], &daily) != nil || body["observedAt"] == nil {
		t.Fatalf("response = %v", body)
	}
	features := []string{string(State.AttackFeatureAutoTowers), string(State.AttackFeatureAutoFortress), string(State.AttackFeatureAutoInvasion),
		string(State.AttackFeatureAutoNomad), string(State.AttackFeatureAutoAdvisor), string(State.AttackFeatureAutoKhan),
		string(State.AttackFeatureAutoBeriWorld), string(State.AttackFeatureAutoStorm)}
	if window != 60 || len(hourly) != len(features) || len(daily.LaunchesByFeature) != len(features) || daily.StartedAt.IsZero() {
		t.Fatalf("shape: window=%d hourly=%v daily=%+v", window, hourly, daily)
	}
	for _, feature := range features {
		if hourly[feature] == 0 || daily.LaunchesByFeature[feature] != hourly[feature] {
			t.Errorf("%s hourly %d daily %d", feature, hourly[feature], daily.LaunchesByFeature[feature])
		}
	}
}

// A window the source cannot cover is null (the badge shows "unavailable"),
// never a smaller number, and each window degrades on its own.
func TestHostedAttackRatesReportUncoveredWindowsAsNull(t *testing.T) {
	counts := map[string]int{Telemetry.ChannelAutoTowers: 4}
	cases := map[string]struct {
		source                 *stubAttackLaunches
		hourlyNull, dailyIsNil bool
	}{
		"hourly uncovered": {&stubAttackLaunches{hourlyOK: false, daily: counts, dailyOK: true}, true, false},
		"daily uncovered":  {&stubAttackLaunches{hourly: counts, hourlyOK: true, dailyOK: false}, false, true},
		"both uncovered":   {&stubAttackLaunches{}, true, true},
		"no source":        {nil, true, true},
	}
	for name, testCase := range cases {
		code, body := getJSON(t, hostedRatesServer(testCase.source).Handler(), "/api/v2/automations/attack-rates")
		if code != http.StatusOK {
			t.Fatalf("%s: status %d", name, code)
		}
		if got := string(body["launchesByFeature"]) == "null"; got != testCase.hourlyNull {
			t.Errorf("%s: hourly null = %v (%s)", name, got, body["launchesByFeature"])
		}
		if got := string(body["dailySession"]) == "null"; got != testCase.dailyIsNil {
			t.Errorf("%s: daily null = %v (%s)", name, got, body["dailySession"])
		}
	}
}

func TestDesktopKeepsTelemetryRoutesAndDoesNotServeTheHostedRoute(t *testing.T) {
	telemetry := Telemetry.NewStore(100)
	defer telemetry.Close()
	telemetry.RecordFeatureActivity("automation:autoTowers", "tower.attack", "INFO", "ATTACK", "Launched test attack")
	accessorState1 := State.NewGameState()
	handler := NewServer(Config{Telemetry: telemetry, State: State.NewStore(&accessorState1)}).Handler()
	for _, path := range []string{"/api/v2/telemetry/channels", "/api/v2/telemetry/attack-rates", "/api/v2/telemetry/autotowers"} {
		if code, _ := getJSON(t, handler, path); code != http.StatusOK {
			t.Errorf("desktop GET %s = %d, want 200", path, code)
		}
	}
	if code, _ := getJSON(t, handler, "/api/v2/automations/attack-rates"); code != http.StatusNotFound {
		t.Errorf("desktop serves the hosted attack-rates route: %d", code)
	}
	_, body := getJSON(t, handler, "/api/v2/telemetry/attack-rates")
	var hourly map[string]int
	if json.Unmarshal(body["launchesByFeature"], &hourly) != nil || hourly[string(State.AttackFeatureAutoTowers)] != 1 {
		t.Fatalf("desktop hourly counts = %s", body["launchesByFeature"])
	}
}

func TestAttackLaunchSourceIsNilWithoutEitherStore(t *testing.T) {
	if source := (&Server{config: Config{}}).attackLaunchSource(); source != nil {
		t.Fatalf("source = %#v, want a true nil interface (a typed-nil store would defeat the nil check)", source)
	}
}
