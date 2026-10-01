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

type windowAttackLaunches struct {
	coveredFrom time.Time
	launches    []time.Time
}

func (source *windowAttackLaunches) AttackLaunchCountsSince(start, end time.Time) (map[string]int, bool) {
	if start.IsZero() || start.After(end) || (!source.coveredFrom.IsZero() && !start.After(source.coveredFrom)) {
		return nil, false
	}
	counts := map[string]int{}
	for _, launchedAt := range source.launches {
		if !launchedAt.Before(start) && !launchedAt.After(end) {
			counts[Telemetry.ChannelAutoTowers]++
		}
	}
	return counts, true
}

func TestAttackLaunchDisplayWindows(t *testing.T) {
	now := time.Now().UTC()
	since := now.Add(-20 * time.Minute)
	reset := now.Add(-6 * time.Hour)
	cases := []struct {
		name                  string
		daily                 State.DailyAttackState
		source                *windowAttackLaunches
		wantWindow            string
		wantDaily, wantHourly int
		dailyNull, hourlyNull bool
		restore               bool
	}{
		{name: "fresh profile", daily: State.DailyAttackState{CountingStartedAt: since, ObservedAt: since}, source: &windowAttackLaunches{}, wantWindow: "since"},
		{name: "new day", daily: State.DailyAttackState{SessionStartedAt: reset, ObservedAt: reset}, source: &windowAttackLaunches{}, wantWindow: "day"},
		{name: "mid-day start", daily: State.DailyAttackState{CountingStartedAt: since, ObservedAt: since}, source: &windowAttackLaunches{launches: []time.Time{now.Add(-10 * time.Minute)}}, wantWindow: "since", wantDaily: 1, wantHourly: 1},
		{name: "mid-day then reset", daily: State.DailyAttackState{SessionStartedAt: since, ObservedAt: since}, source: &windowAttackLaunches{launches: []time.Time{now.Add(-30 * time.Minute), now.Add(-10 * time.Minute)}}, wantWindow: "day", wantDaily: 1, wantHourly: 2},
		{name: "restored restart", daily: State.DailyAttackState{CountingStartedAt: since, ObservedAt: since}, source: &windowAttackLaunches{launches: []time.Time{now.Add(-10 * time.Minute)}}, wantWindow: "since", wantDaily: 1, wantHourly: 1, restore: true},
		{name: "restart nothing recorded", daily: State.DailyAttackState{CountingStartedAt: since, ObservedAt: since}, source: &windowAttackLaunches{}, wantWindow: "since", restore: true},
		{name: "pruned window", daily: State.DailyAttackState{SessionStartedAt: reset, ObservedAt: reset}, source: &windowAttackLaunches{coveredFrom: now.Add(-30 * time.Minute)}, dailyNull: true, hourlyNull: true},
		{name: "coverage starts at boundary", daily: State.DailyAttackState{CountingStartedAt: since, ObservedAt: since}, source: &windowAttackLaunches{coveredFrom: since}, dailyNull: true, hourlyNull: true},
		{name: "daily pruned hour retained", daily: State.DailyAttackState{SessionStartedAt: reset, ObservedAt: reset}, source: &windowAttackLaunches{coveredFrom: now.Add(-2 * time.Hour)}, dailyNull: true},
		{name: "no source", daily: State.DailyAttackState{CountingStartedAt: since, ObservedAt: since}, dailyNull: true, hourlyNull: true},
		{name: "no observation", source: &windowAttackLaunches{}, dailyNull: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			state := State.NewGameState()
			state.DailyAttacks = tc.daily
			if tc.restore {
				data, err := json.Marshal(state)
				if err != nil {
					t.Fatal(err)
				}
				if err := json.Unmarshal(data, &state); err != nil {
					t.Fatal(err)
				}
				// The fake persisted ledger is restored independently of the state.
				data, err = json.Marshal(tc.source.launches)
				if err != nil {
					t.Fatal(err)
				}
				restored := &windowAttackLaunches{coveredFrom: tc.source.coveredFrom}
				if err := json.Unmarshal(data, &restored.launches); err != nil {
					t.Fatal(err)
				}
				tc.source = restored
			}
			config := Config{BackgroundOnly: true, State: State.NewStore(&state)}
			if tc.source != nil {
				config.AttackLaunches = tc.source
			}
			server := &Server{config: config}
			recorder := httptest.NewRecorder()
			server.handleAttackLaunchRates(recorder, httptest.NewRequest(http.MethodGet, "/api/v2/automations/attack-rates", nil))
			var got struct {
				ObservedAt        time.Time                 `json:"observedAt"`
				WindowStartedAt   time.Time                 `json:"windowStartedAt"`
				LaunchesByFeature map[string]int            `json:"launchesByFeature"`
				Daily             *attackLaunchDailySession `json:"dailySession"`
			}
			if recorder.Code != http.StatusOK {
				t.Fatalf("status/body %d/%s", recorder.Code, recorder.Body.String())
			}
			if err := json.Unmarshal(recorder.Body.Bytes(), &got); err != nil {
				t.Fatal(err)
			}
			start := got.ObservedAt.Add(-time.Hour)
			if tc.daily.CountingStartedAt.After(start) {
				start = tc.daily.CountingStartedAt
			}
			if !got.WindowStartedAt.Equal(start) {
				t.Fatalf("hour start = %s, want %s", got.WindowStartedAt, start)
			}
			if (got.LaunchesByFeature == nil) != tc.hourlyNull || (got.Daily == nil) != tc.dailyNull {
				t.Fatalf("null windows: %s", recorder.Body.String())
			}
			if !tc.hourlyNull && (len(got.LaunchesByFeature) != 8 || got.LaunchesByFeature["autoTowers"] != tc.wantHourly) {
				t.Fatalf("hour counts = %v", got.LaunchesByFeature)
			}
			if !tc.dailyNull {
				dailyStart := tc.daily.SessionStartedAt
				if dailyStart.IsZero() {
					dailyStart = tc.daily.CountingStartedAt
				}
				if got.Daily.Window != tc.wantWindow || !got.Daily.StartedAt.Equal(dailyStart) || len(got.Daily.LaunchesByFeature) != 8 || got.Daily.LaunchesByFeature["autoTowers"] != tc.wantDaily {
					t.Fatalf("daily = %#v", got.Daily)
				}
			}
			if server.config.State.ReadOnlyView().DailyAttacks != tc.daily {
				t.Fatal("display handler changed reset state")
			}
		})
	}
}
