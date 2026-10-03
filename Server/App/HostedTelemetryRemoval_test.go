package App

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"CitadelDesktop/Server/Intent"
	"CitadelDesktop/Server/Session"
	"CitadelDesktop/Server/State"
	"CitadelDesktop/Server/Telemetry"
)

func startHostedApplication(t *testing.T, dataDir string) (*Application, func()) {
	t.Helper()
	runtimeContext, cancelRuntime := context.WithCancel(context.Background())
	application, err := New(runtimeContext, Config{
		DataDir: dataDir, Offline: true, BackgroundOnly: true, Transport: Session.NewUnavailableTransport(),
	})
	if err != nil {
		cancelRuntime()
		t.Fatal(err)
	}
	application.Start(runtimeContext)
	stop := func() {
		cancelRuntime()
		waitContext, cancelWait := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancelWait()
		if err := application.Wait(waitContext); err != nil {
			t.Errorf("wait for hosted application shutdown: %v", err)
		}
	}
	return application, stop
}

func hostedRates(t *testing.T, application *Application) (hourly map[string]int, daily map[string]int, dailyPresent bool, hourlyPresent bool) {
	t.Helper()
	recorder := httptest.NewRecorder()
	application.API.Handler().ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/v2/automations/attack-rates", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("attack-rates = %d %s", recorder.Code, recorder.Body.String())
	}
	var body struct {
		LaunchesByFeature map[string]int `json:"launchesByFeature"`
		DailySession      *struct {
			LaunchesByFeature map[string]int `json:"launchesByFeature"`
		} `json:"dailySession"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.DailySession != nil {
		daily, dailyPresent = body.DailySession.LaunchesByFeature, true
	}
	return body.LaunchesByFeature, daily, dailyPresent, body.LaunchesByFeature != nil
}

func setDailyAttackSession(t *testing.T, application *Application, startedAt time.Time) {
	t.Helper()
	if _, err := application.State.ApplyComponents(State.Components(State.ComponentDailyAttacks), func(state *State.GameState) ([]string, bool, error) {
		state.DailyAttacks = State.DailyAttackState{Count: 1, ServerThreshold: 3500, SessionStartedAt: startedAt, ObservedAt: time.Now().UTC()}
		return []string{"daily-attacks"}, true, nil
	}); err != nil {
		t.Fatal(err)
	}
}

func TestHostedApplicationCreatesNoTelemetryAndServesNoTelemetryRoutes(t *testing.T) {
	dataDir := t.TempDir()
	application, stop := startHostedApplication(t, dataDir)
	if application.Telemetry != nil || application.AttackLaunches == nil {
		t.Fatalf("hosted composition: telemetry=%v ledger=%v", application.Telemetry, application.AttackLaunches)
	}
	// Activity from receipts still flows without anything being written as a log.
	for _, receipt := range fixtureReceipts() {
		application.recordIntentLog(receipt)
	}
	for _, path := range []string{"/api/v2/telemetry/channels", "/api/v2/telemetry/attack-rates", "/api/v2/telemetry/activity"} {
		recorder := httptest.NewRecorder()
		application.API.Handler().ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, path, nil))
		if recorder.Code != http.StatusNotFound {
			t.Errorf("hosted GET %s = %d, want 404", path, recorder.Code)
		}
	}
	stop()
	if _, err := os.Stat(filepath.Join(dataDir, "Logs")); !os.IsNotExist(err) {
		t.Fatalf("hosted runtime created a Logs directory: %v", err)
	}
	var written []string
	_ = filepath.WalkDir(dataDir, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		relative, _ := filepath.Rel(dataDir, path)
		lower := strings.ToLower(relative)
		if strings.Contains(lower, "telemetry") || strings.Contains(lower, "channels") || strings.HasSuffix(lower, ".log") {
			written = append(written, relative)
		}
		return nil
	})
	if len(written) != 0 {
		t.Fatalf("telemetry-looking files under the data dir: %v", written)
	}
}

func TestDesktopApplicationStillCreatesTelemetry(t *testing.T) {
	dataDir := t.TempDir()
	runtimeContext, cancel := context.WithCancel(context.Background())
	application, err := New(runtimeContext, Config{DataDir: dataDir, Offline: true, Transport: Session.NewUnavailableTransport()})
	if err != nil {
		cancel()
		t.Fatal(err)
	}
	application.Start(runtimeContext)
	defer func() {
		cancel()
		waitContext, cancelWait := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancelWait()
		_ = application.Wait(waitContext)
	}()
	if application.Telemetry == nil || application.AttackLaunches != nil {
		t.Fatalf("desktop composition changed: telemetry=%v ledger=%v", application.Telemetry, application.AttackLaunches)
	}
	recorder := httptest.NewRecorder()
	application.API.Handler().ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/v2/telemetry/channels", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("desktop telemetry channels = %d", recorder.Code)
	}
}

// Both windows survive a worker restart with identical counts, served from the
// persisted ledger and not from log files.
func TestHostedAttackRatesSurviveARestartExactly(t *testing.T) {
	dataDir := t.TempDir()
	first, stop := startHostedApplication(t, dataDir)
	setDailyAttackSession(t, first, time.Now().Add(-3*time.Hour).UTC())
	for _, receipt := range fixtureReceipts() {
		first.recordIntentLog(receipt)
	}
	hourlyBefore, dailyBefore, dailyPresent, hourlyPresent := hostedRates(t, first)
	if !dailyPresent || !hourlyPresent {
		t.Fatalf("windows unavailable before restart: hourly=%v daily=%v", hourlyPresent, dailyPresent)
	}
	towers := string(State.AttackFeatureAutoTowers)
	if hourlyBefore[towers] == 0 || dailyBefore[towers] != hourlyBefore[towers] {
		t.Fatalf("counts before restart: hourly=%v daily=%v", hourlyBefore, dailyBefore)
	}
	stop()

	second, stopSecond := startHostedApplication(t, dataDir)
	defer stopSecond()
	// The daily boundary is state, restored by state persistence in production;
	// here it is set again so the comparison is about launch counts.
	setDailyAttackSession(t, second, time.Now().Add(-3*time.Hour).UTC())
	hourlyAfter, dailyAfter, dailyPresent, hourlyPresent := hostedRates(t, second)
	if !dailyPresent || !hourlyPresent {
		t.Fatalf("windows unavailable after restart: hourly=%v daily=%v", hourlyPresent, dailyPresent)
	}
	for _, channel := range []string{
		"autoTowers", "autoFortress", "autoInvasion", "autoNomad", "autoAdvisor", "autoKhan", "autoBeriWorld", "autoStorm",
	} {
		if hourlyAfter[channel] != hourlyBefore[channel] || dailyAfter[channel] != dailyBefore[channel] {
			t.Errorf("%s changed across restart: hourly %d->%d daily %d->%d", channel,
				hourlyBefore[channel], hourlyAfter[channel], dailyBefore[channel], dailyAfter[channel])
		}
	}
}

// A runtime upgraded onto this change already has a journal of finished
// receipts: the first start seeds the ledger from it so neither badge resets.
func TestHostedLedgerSeedsFromExistingJournalOnFirstStart(t *testing.T) {
	dataDir := t.TempDir()
	store, err := Intent.OpenOperationStore(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	completed := time.Now().Add(-10 * time.Minute).UTC()
	for _, receipt := range []Intent.Receipt{
		{ID: "journal-1", Intent: "tower.attack", Actor: "automation:autoTowers", Status: Intent.StatusSucceeded, Plan: chainPlan(2), CompletedAt: &completed, SubmittedAt: completed},
		{ID: "journal-2", Intent: "nomad.camp.attack", Actor: "automation:autoNomad", Status: Intent.StatusSucceeded, Plan: chainPlan(1), CompletedAt: &completed, SubmittedAt: completed},
	} {
		if _, reserved, err := store.Reserve(t.Context(), receipt.ID+"-hash", receipt); err != nil || !reserved {
			t.Fatalf("reserve %s: %v %v", receipt.ID, reserved, err)
		}
		receipt.Status = Intent.StatusSucceeded
		if err := store.Save(t.Context(), receipt); err != nil {
			t.Fatal(err)
		}
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	application, stop := startHostedApplication(t, dataDir)
	defer stop()
	counts, ok := application.AttackLaunches.AttackLaunchCountsSince(time.Now().Add(-time.Hour), time.Now())
	if !ok || counts[Telemetry.ChannelAutoTowers] != 2 || counts[Telemetry.ChannelAutoNomad] != 1 {
		t.Fatalf("seeded counts = %v %v", counts, ok)
	}
}
