package App

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"testing"
	"time"

	"CitadelDesktop/Server/Automation"
	"CitadelDesktop/Server/Configuration"
	"CitadelDesktop/Server/Ingest"
	"CitadelDesktop/Server/Intent"
	"CitadelDesktop/Server/Outbound"
	"CitadelDesktop/Server/Protocol"
	"CitadelDesktop/Server/State"
)

type autoBoosterRubyQuoteSender struct {
	*autoBoosterIntegrationSender
	threshold int64
}

func (sender *autoBoosterRubyQuoteSender) Send(ctx context.Context, raw []byte) error {
	if err := Outbound.ValidateFinalDispatch(ctx); err != nil {
		return err
	}
	command, err := Protocol.Decode(string(raw), Protocol.DirectionOutbound, time.Now().UTC())
	if err != nil {
		return err
	}
	metadata := Outbound.MetadataFromContext(ctx)
	if command.Opcode == "gbd" {
		observedAt := time.Now().UTC()
		var payload map[string]json.RawMessage
		if err := json.Unmarshal(autoBoosterGBDFixture(nil, observedAt, sender.endsAt, false, 10000), &payload); err != nil {
			return err
		}
		payload["opt"] = json.RawMessage(fmt.Sprintf(`{"CC2T":%d}`, sender.threshold))
		encoded, _ := json.Marshal(payload)
		code := 0
		_, err := sender.pipeline.HandleFrame(ctx, Protocol.Frame{Opcode: "gbd", Direction: Protocol.DirectionInbound, ResponseCode: &code, ReceivedAt: observedAt, Payload: encoded, ResponseToken: metadata.ResponseToken, CausationOperationID: metadata.OperationID})
		return err
	}
	if command.Opcode != "agb" || string(command.Payload) != `{"GEID":2}` {
		return fmt.Errorf("unexpected boost command: %s %s", command.Opcode, command.Payload)
	}
	sender.agbSends++
	// Sanitized retained September 23 shape, decoded through the actual transport parser.
	frame, err := sender.pipeline.DecodeTransportFrameAt(`%xt%agb%1%440%{"GEID":2,"CC2T":2500}%`, Protocol.DirectionInbound, time.Now().UTC(), metadata.ResponseToken, metadata.OperationID)
	if err != nil {
		return err
	}
	_, err = sender.pipeline.CommitFrame(ctx, frame)
	return err
}

func TestAutoBoosterCapturedAGB440PersistsHoldAndRequiresFreshSetting(t *testing.T) {
	now := time.Now().UTC()
	endsAt := now.Add(time.Hour).Truncate(time.Minute)
	state := State.NewGameState()
	state.Session = State.SessionState{Generation: 1, BaselineGeneration: 1, ConnectionGeneration: 8, LoggedIn: true, SocketReady: true, ChangedAt: now.Add(-time.Minute)}
	store := State.NewStore(&state)
	manager := autoBoosterIntentGameDataManager(t)
	reducers := Ingest.NewRegistry()
	if err := Ingest.RegisterCoreReducers(reducers); err != nil {
		t.Fatal(err)
	}
	pipeline := Ingest.NewPipeline(store, manager, reducers)
	code := 0
	if _, err := pipeline.HandleFrame(t.Context(), Protocol.Frame{Opcode: "gbd", Direction: Protocol.DirectionInbound, ResponseCode: &code, ReceivedAt: now, Payload: autoBoosterGBDFixture(t, now, endsAt, false, 10000)}); err != nil {
		t.Fatal(err)
	}
	configuration, err := Configuration.Open(t.TempDir(), map[string]json.RawMessage{
		"automation.enabled":     json.RawMessage(`{"auto_booster":true}`),
		"automation.autoBooster": json.RawMessage(`{"version":1,"checkIntervalSec":60,"rubyCostCeiling":2500,"minimumRubyReserve":0}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	app := &Application{DataDir: t.TempDir(), State: store, GameData: manager, Configuration: configuration, Ingest: pipeline}
	pipeline.SetDurabilityFence(app.saveStateEvent)
	sender := &autoBoosterRubyQuoteSender{autoBoosterIntegrationSender: &autoBoosterIntegrationSender{application: app, pipeline: pipeline, endsAt: endsAt}, threshold: -1}
	engine := Intent.NewEngine(Intent.NewRegistry(), store, manager, sender, pipeline)
	app.Intents = engine
	if err := app.registerAutoBoosterIntents(); err != nil {
		t.Fatal(err)
	}
	data, _ := manager.Current()
	evaluate := func() Automation.Decision {
		t.Helper()
		result, err := Automation.NewAutoBoosterPolicy().Evaluate(t.Context(), Automation.Snapshot{State: store.ReadOnlyView(), GameData: data, Configuration: configuration.Snapshot(), Now: time.Now().UTC()})
		if err != nil {
			t.Fatal(err)
		}
		return result
	}
	decision := evaluate()
	if decision.Request == nil || decision.Request.Name != "autoBooster.purchase" {
		t.Fatalf("decision=%+v", decision)
	}
	request := *decision.Request
	request.ID, request.Actor, request.AutomationLane = "sanitized-booster-quote", "automation:autoBooster", "autoBooster"
	receipt := engine.Submit(t.Context(), request)
	if receipt.Status != Intent.StatusFailed || receipt.Failure == nil || receipt.Failure.Toast || sender.agbSends != 1 {
		t.Fatalf("receipt=%+v sends=%d", receipt, sender.agbSends)
	}
	record := store.ReadOnlyView().EventScores.Inventory.GlobalEffectPurchases[2]
	if record.Outcome != State.GlobalEffectPurchaseConfirmationRequired || record.QuotedC2 != 2500 || record.ResultCode == nil || *record.ResultCode != 440 || record.ResultObservedAt.IsZero() {
		t.Fatalf("record=%+v", record)
	}
	if store.ReadOnlyView().Automations["autoBooster"].SafetyLock.OperationID != "" || len(receipt.Evidence) != 1 {
		t.Fatal("quote locked or evidence missing")
	}
	if next := evaluate(); next.Request != nil {
		t.Fatalf("held purchase replayed: %+v", next)
	}
	replay := engine.Submit(t.Context(), request)
	if replay.Status == Intent.StatusSucceeded || sender.agbSends != 1 {
		t.Fatal("planner resent held occurrence")
	}
	// An unchanged permissive value from before the response is insufficient.
	if _, err := pipeline.HandleRawAt(t.Context(), `%xt%opt%1%0%{"CC2T":-1}%`, Protocol.DirectionInbound, record.ResultObservedAt.Add(-time.Nanosecond)); err != nil {
		t.Fatal(err)
	}
	if evaluate().Request != nil {
		t.Fatal("older setting observation lifted hold")
	}
	// The same setting observed after the quote must wake and release the hold.
	freshAt := record.ResultObservedAt.Add(time.Nanosecond)
	committed, err := pipeline.HandleRawAt(t.Context(), `%xt%opt%1%0%{"CC2T":-1}%`, Protocol.DirectionInbound, freshAt)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(committed.Domains, "ruby-confirmation") {
		t.Fatal("fresh unchanged setting did not commit")
	}
	if next := evaluate(); next.Request == nil || next.Request.Name != "autoBooster.purchase" {
		t.Fatalf("fresh setting did not resume: %+v", next)
	}
	loaded, err := State.LoadSnapshot(app.DataDir)
	if err != nil {
		t.Fatal(err)
	}
	persisted := loaded.EventScores.Inventory.GlobalEffectPurchases[2]
	if persisted.Outcome != State.GlobalEffectPurchaseConfirmationRequired || persisted.QuotedC2 != 2500 || loaded.Player.RubyConfirmation.Known {
		t.Fatalf("restart persistence=%+v setting=%+v", persisted, loaded.Player.RubyConfirmation)
	}
	// Reconnect invalidates even a previously permissive observation.
	session := store.Session()
	session.Generation++
	if !State.RubyConfirmationPurchaseHeld(store.ReadOnlyView().Player.RubyConfirmation, session, record, 2500) {
		t.Fatal("reconnect lifted hold")
	}
	setting := State.RubyConfirmationState{Known: true, Amount: -1, Generation: session.Generation, ObservedAt: record.ResultObservedAt.Add(time.Second)}
	if State.RubyConfirmationPurchaseHeld(setting, session, record, 2500) {
		t.Fatal("fresh reconnected setting did not lift hold")
	}
	// A setting change in the pre-purchase GBD must stop AGB at the final guard.
	sender.threshold = 2500
	next := evaluate()
	if next.Request == nil {
		t.Fatal("missing resumed plan")
	}
	changed := *next.Request
	changed.ID, changed.Actor, changed.AutomationLane = "setting-changed-before-send", "automation:autoBooster", "autoBooster"
	result := engine.Submit(t.Context(), changed)
	if result.Status == Intent.StatusSucceeded || sender.agbSends != 1 {
		t.Fatalf("changed setting dispatched: %+v sends=%d", result, sender.agbSends)
	}
	if store.ReadOnlyView().Automations["autoBooster"].SafetyLock.OperationID != "" {
		t.Fatal("local prevention created a lock")
	}
}
