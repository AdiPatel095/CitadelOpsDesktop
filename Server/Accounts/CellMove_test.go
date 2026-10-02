package Accounts

import (
	"CitadelDesktop/Server/App"
	"CitadelDesktop/Server/Runtime"
	"CitadelDesktop/Server/State"
	"CitadelDesktop/Server/Telemetry"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"CitadelDesktop/Server/Intent"
	"CitadelDesktop/Server/Reports"
)

func TestCellMoveFinishOpenTimeoutReturns409WithoutFencingOrStopping(t *testing.T) {
	s, _, o, identity, _ := handoverSourceFixture(t)
	o.handoverTransport = true
	app, _ := s.Application("alpha")
	// This synthetic action exercises drain coordination without requiring a
	// connected game session. Engine drain behavior has its own race tests.
	app.Intents.SetExecutionGate(nil)
	started, finish := make(chan struct{}), make(chan struct{})
	defer close(finish)
	if err := app.Intents.Registry().Register(Intent.Definition{Name: "test.cell.move", Effect: Intent.EffectWrite, Planner: func(context.Context, Intent.PlanningContext, json.RawMessage) (Intent.Plan, error) {
		return Intent.Plan{Claims: []string{"configuration:test.cell.move"}, Steps: []Intent.Step{{Action: "test.cell.move"}}}, nil
	}}); err != nil {
		t.Fatal(err)
	}
	if err := app.Intents.RegisterAction("test.cell.move", func(ctx context.Context, _ json.RawMessage) error {
		close(started)
		select {
		case <-finish:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	}); err != nil {
		t.Fatal(err)
	}
	app.Intents.SubmitDetached(Intent.Request{ID: "mutation", Name: "test.cell.move"})
	select {
	case <-started:
	case <-time.After(time.Second):
		receipt, _ := app.Intents.Operation("mutation")
		t.Fatalf("synthetic operation did not start: %+v", receipt)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Millisecond)
	defer cancel()
	raw, _ := json.Marshal(ProfileExportRequest{identity, true})
	r := httptest.NewRequest(http.MethodPost, "/orchestrator/v1/handovers/export", bytes.NewReader(raw)).WithContext(ctx)
	r.Header.Set("Authorization", "Bearer "+testControlToken)
	r.Header.Set(controlEpochHeader, "1")
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	o.Handler().ServeHTTP(w, r)
	if w.Code != http.StatusConflict {
		t.Fatalf("timeout status %d: %s", w.Code, w.Body.String())
	}
	if _, ok := s.sourceFence("alpha"); ok {
		t.Fatal("timeout persisted a source fence")
	}
	if got, running := s.Application("alpha"); !running || got != app {
		t.Fatal("timeout stopped or replaced source")
	}
	if receipt, _ := app.Intents.Operation("mutation"); receipt.Terminal() {
		t.Fatalf("timeout terminated mutation: %+v", receipt)
	}
	if finishOpenDrainTimeout != 3*time.Minute {
		t.Fatal("drain bound differs from addendum")
	}
}

func TestCellMoveSameBuildMatchesPlainRestartAndKeepsChannel(t *testing.T) {
	s, _, o, identity, _ := handoverSourceFixture(t)
	app, _ := s.Application("alpha")
	sourceBuild := o.Status()
	// Seed actual durable journal, ledger and state records before departure.
	now := time.Now().UTC()
	operation := Intent.Receipt{ID: "saved-operation", Intent: "test.completed", Status: Intent.StatusSucceeded, Phase: Intent.EffectPhaseCompleted, SubmittedAt: now, CompletedAt: &now}
	if _, inserted, err := app.OperationStore.Reserve(t.Context(), "fixture-hash", operation); err != nil || !inserted {
		t.Fatal("seed operation", err)
	}
	rows := []Intent.AttackLaunchRow{{OperationID: operation.ID, Ordinal: 0, Feature: Telemetry.ChannelAutoFortress, LaunchedAt: now}, {OperationID: operation.ID, Ordinal: 1, Feature: Telemetry.ChannelAutoFortress, LaunchedAt: now}}
	if err := app.OperationStore.AppendAttackLaunches(t.Context(), rows); err != nil {
		t.Fatal(err)
	}
	report := Reports.BattleReport{ID: "10-20", ReportID: "10-20", MID: 10, LID: 20, AccountUID: 44, WorldID: "world", PlayerID: 7, OccurredAt: now.Format(time.RFC3339Nano), DateMs: now.UnixMilli(), Result: "Victory", Role: "attacker", TargetTypeID: 24, BattleTypeID: 6, Defender: &Reports.BattleCombatant{PlayerID: -50, Dummy: true}, AutomationFeature: string(State.AttackFeatureAutoFortress)}
	if err := app.ReportStore.Save(t.Context(), report); err != nil {
		t.Fatal(err)
	}
	reportQuery := Reports.BattleReportQuery{AccountUID: 44, Limit: 10}
	savedReports, err := app.ReportStore.Recent(t.Context(), reportQuery)
	if err != nil || len(savedReports) != 1 {
		t.Fatal("seed durable battle report", err, savedReports)
	}
	presets := []State.AttackPreset{{Slot: 1, Name: "saved formation", Units: [3][]State.AttackPresetStack{{{DefinitionID: 21, Amount: 100}}, nil, nil}}}
	scheduled := map[string]State.ScheduledOperation{"timed-run": {ID: "timed-run", Version: 1, Intent: "tower.attack", Actor: "scheduler:tower.attack", Arguments: json.RawMessage(`{"targetId":7}`), ExecuteAt: now.Add(24 * time.Hour), CreatedAt: now, UpdatedAt: now, Status: "scheduled"}}
	if _, err := app.State.ApplyComponents(State.Components(State.ComponentAttackPresets, State.ComponentScheduled), func(state *State.GameState) ([]string, bool, error) {
		state.AttackPresets = presets
		state.Scheduled = scheduled
		return []string{"attack-presets", "scheduled-operations"}, true, nil
	}); err != nil {
		t.Fatal(err)
	}
	snapshot := app.Configuration.Snapshot()
	beta := json.RawMessage(`{"enabled":true,"consentVersion":1,"spyCount":2}`)
	snapshot.Sections[Reports.BattleResearchConfigurationSection] = beta
	if _, _, err := app.Configuration.ReplaceAllAuthoritative(snapshot.Sections); err != nil {
		t.Fatal(err)
	}
	export, err := o.exportSourceProfile(t.Context(), identity, true)
	if err != nil {
		t.Fatal(err)
	}
	_, file, err := o.openProfileExport(t.Context(), identity)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	archive, err := io.ReadAll(file)
	if err != nil {
		t.Fatal(err)
	}
	restartDir := filepath.Join(t.TempDir(), "profile")
	if _, err := Runtime.RestoreProfileArchive(t.Context(), bytes.NewReader(archive), restartDir, identity, export.Receipt.SHA256); err != nil {
		t.Fatal(err)
	}
	// A plain restart and a move must apply the same startup migrations.
	plain, err := App.New(t.Context(), App.Config{DataDir: restartDir, Offline: true, GameData: s.gameData, BackgroundOnly: true})
	if err != nil {
		t.Fatal(err)
	}
	// No session or application goroutines are started in this offline comparison.
	defer func() {
		_ = plain.OperationStore.Close()
		_ = plain.ReportStore.Close()
		_ = plain.ProfileLease.Close()
	}()
	target, targetOrchestrator := adoptionTarget(t)
	profile, err := targetOrchestrator.RestoreTargetProfile(t.Context(), export.Receipt, identity.SourceEpoch+1, adoptionSnapshot(), bytes.NewReader(archive))
	if err != nil {
		t.Fatal(err)
	}
	startAdoptionParked(t, targetOrchestrator, profile, 1)
	targetApp, ok := target.Application("alpha")
	if !ok {
		t.Fatal("target did not start")
	}

	if _, ok := targetApp.Configuration.Section(Reports.BattleResearchConfigurationSection); ok {
		t.Fatal("move revived retired beta section")
	}
	if _, ok := plain.Configuration.Section(Reports.BattleResearchConfigurationSection); ok {
		t.Fatal("plain restart revived retired beta section")
	}
	before, after := plain.Configuration.Snapshot().Sections, targetApp.Configuration.Snapshot().Sections
	if len(before) != len(after) {
		t.Fatalf("configuration section count changed: %d != %d", len(before), len(after))
	}
	for section, value := range before {
		if !bytes.Equal(value, after[section]) {
			t.Fatalf("restart parity differs for %s: %s != %s", section, value, after[section])
		}
	}
	targetBuild := targetOrchestrator.Status()
	if sourceBuild.Version != targetBuild.Version || sourceBuild.BuildRevision != targetBuild.BuildRevision || sourceBuild.BuildID != targetBuild.BuildID {
		t.Fatal("same-channel move changed build/channel")
	}
	stored, exists, err := targetApp.OperationStore.Get(t.Context(), operation.ID)
	if err != nil || !exists || !reflect.DeepEqual(stored.Receipt, operation) {
		t.Fatal("operation history changed", err, stored)
	}
	ledger, err := targetApp.OperationStore.LoadAttackLaunchLedger(t.Context(), now.Add(-time.Hour))
	if err != nil || !reflect.DeepEqual(ledger.Rows, rows) {
		t.Fatal("attack ledger changed", err, ledger.Rows)
	}
	counts, _ := targetApp.AttackLaunches.AttackLaunchCountsSince(now.Add(-time.Minute), now.Add(time.Second))
	if counts[Telemetry.ChannelAutoFortress] != 2 {
		t.Fatal("attack counts changed", counts)
	}
	targetState := targetApp.State.Snapshot()
	if !reflect.DeepEqual(targetState.AttackPresets, presets) || !reflect.DeepEqual(targetState.Scheduled, scheduled) {
		t.Fatal("presets, schedules or timed runs changed")
	}
	restoredReports, err := targetApp.ReportStore.Recent(t.Context(), reportQuery)
	if err != nil || !reflect.DeepEqual(savedReports, restoredReports) {
		t.Fatal("durable battle history changed", err, restoredReports)
	}
	// Activation must keep the complete profile rather than resetting histories.
	for _, relative := range []string{"Runtime/ProfileID", "Config/Settings.json"} {
		if _, err := os.Stat(filepath.Join(targetApp.DataDir, filepath.FromSlash(relative))); err != nil {
			t.Fatal(err)
		}
	}
}
