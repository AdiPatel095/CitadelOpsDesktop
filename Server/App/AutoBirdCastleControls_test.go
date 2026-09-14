package App

import (
	"encoding/json"
	"errors"
	"testing"
	"time"

	"CitadelDesktop/Server/Intent"
	"CitadelDesktop/Server/State"
)

func TestCastleBirdControlsAreIsolatedAndInvalidateOldCommands(t *testing.T) {
	game := State.NewGameState()
	game.Castles[10] = State.CastleState{ID: 10}
	game.Castles[11] = State.CastleState{ID: 11}
	for _, key := range []string{"autoBird:10", "autoBird:11", "autoStation:10"} {
		game.Stationing[key] = State.StationingOperation{ID: key, Purpose: "autoBird"}
	}
	game.Stationing["autoStation:10"] = State.StationingOperation{ID: "autoStation:10", Purpose: "autoStation"}
	game.Movements[50] = State.MovementState{ID: 50, SourceCastleID: 10}
	app := &Application{State: State.NewStore(game)}
	registry := Intent.NewRegistry()
	registry.EnforceResourceDeclarations()
	if err := registry.Register(Intent.Definition{Name: "auto_bird.castle_control", Effect: Intent.EffectWrite, Planner: planAutoBirdCastleControl}); err != nil {
		t.Fatal(err)
	}
	engine := Intent.NewEngine(registry, app.State, nil, nil, nil)
	if err := engine.RegisterAction("auto_bird.castle.control", app.controlAutoBirdCastle); err != nil {
		t.Fatal(err)
	}
	submit := func(args string) {
		t.Helper()
		receipt := engine.Submit(t.Context(), Intent.Request{Name: "auto_bird.castle_control", Arguments: json.RawMessage(args)})
		if receipt.Status != Intent.StatusSucceeded {
			t.Fatalf("control failed: %#v", receipt)
		}
	}
	submit(`{"sourceCastleId":10,"action":"pause","durationMinutes":30}`)
	paused := app.State.Snapshot()
	control := paused.AutoBirdControl(10)
	if !paused.AutoBirdPaused(10, time.Now()) || control.PausedUntil == nil || paused.AutoBirdPaused(11, time.Now()) {
		t.Fatal("pause not castle-local")
	}
	if paused.AutoBirdPaused(10, control.PausedUntil.Add(time.Second)) {
		t.Fatal("timed pause did not expire")
	}
	// Persistence and cloning must preserve the control independently of cycles.
	raw, err := json.Marshal(paused)
	if err != nil {
		t.Fatal(err)
	}
	var restored State.GameState
	if err := json.Unmarshal(raw, &restored); err != nil {
		t.Fatal(err)
	}
	if !restored.AutoBirdPaused(10, time.Now()) {
		t.Fatal("pause lost after restore")
	}
	if err := app.clearAutoBirdTracking(t.Context(), json.RawMessage(`{}`)); err != nil {
		t.Fatal(err)
	}
	if !app.State.Snapshot().AutoBirdPaused(10, time.Now()) {
		t.Fatal("clear-all erased pause")
	}
	submit(`{"sourceCastleId":10,"action":"resume"}`)
	resumed := app.State.Snapshot()
	if resumed.AutoBirdPaused(10, time.Now()) {
		t.Fatal("resume failed")
	}
	old := autoBirdCycleRequest{SourceCastleID: 10, ControlRevision: control.UpdatedAt}
	if !errors.Is(validateAutoBirdControl(resumed, old, time.Now()), Intent.ErrPlanStale) {
		t.Fatal("old command still valid after resume")
	}
	old.ControlRevision = resumed.AutoBirdControl(10).UpdatedAt
	submit(`{"sourceCastleId":10,"action":"resend"}`)
	rescanning := app.State.Snapshot()
	if !rescanning.AutoBirdControl(10).RescanRequested || rescanning.Movements[50].ID != 50 || rescanning.Stationing["autoStation:10"].ID == "" {
		t.Fatal("resend lost movements or Auto Station")
	}
	if !errors.Is(validateAutoBirdControl(rescanning, old, time.Now()), Intent.ErrPlanStale) {
		t.Fatal("resend did not invalidate old prepared commands")
	}
	// An old AIN response must not recreate tracking after a rescan.
	args, _ := json.Marshal(old)
	if !errors.Is(app.captureAutoBirdTarget(t.Context(), args), Intent.ErrPlanStale) {
		t.Fatal("stale AIN capture was accepted")
	}
	if _, ok := app.State.Snapshot().Stationing["autoBird:10"]; ok {
		t.Fatal("stale response recreated cycle")
	}
}

func TestCastleBirdControlRejectsInvalidRequests(t *testing.T) {
	for _, args := range []string{
		`{"sourceCastleId":0,"action":"pause"}`,
		`{"sourceCastleId":10,"action":"pause","durationMinutes":-1}`,
		`{"sourceCastleId":10,"action":"pause","durationMinutes":10081}`,
		`{"sourceCastleId":10,"action":"resume","durationMinutes":1}`,
		`{"sourceCastleId":10,"action":"launch"}`,
	} {
		if _, err := decodeAutoBirdCastleControl(json.RawMessage(args)); err == nil {
			t.Fatalf("accepted %s", args)
		}
	}
	if _, err := planAutoBirdCastleControl(t.Context(), Intent.PlanningContext{State: State.NewGameState()}, json.RawMessage(`{"sourceCastleId":10,"action":"resend"}`)); err == nil {
		t.Fatal("accepted unowned castle")
	}
}
