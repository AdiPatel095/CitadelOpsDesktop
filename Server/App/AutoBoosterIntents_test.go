package App

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"CitadelDesktop/Server/Configuration"
	"CitadelDesktop/Server/GameData"
	"CitadelDesktop/Server/Ingest"
	"CitadelDesktop/Server/Intent"
	"CitadelDesktop/Server/Outbound"
	"CitadelDesktop/Server/Protocol"
	"CitadelDesktop/Server/State"
)

func TestAutoBoosterPurchaseRefreshesGuardsAndSendsAuthoritativeAGBShape(t *testing.T) {
	now := time.Now().UTC()
	endsAt := now.Add(time.Hour).Truncate(time.Minute)
	gameState := State.NewGameState()
	gameState.Session.ChangedAt = now.Add(-time.Minute)
	gameState.Player.Resources[2] = 10_000
	gameState.Player.ResourceObservations[2] = State.PlayerResourceObservation{ObservedAt: now}
	gameState.EventScores.Inventory = autoBoosterIntentInventory(now, endsAt, 2500, false)
	arguments, _ := json.Marshal(autoBoosterPurchaseRequest{
		GlobalEffectID: 2, ExpectedEndsAtUnix: endsAt.Unix(), ExpectedRubyCost: 2500,
		ExpectedBonusValue: 60, MinimumRubyReserve: 5000, ExpectedCheckIntervalSec: 60, ExpectedRubyBalance: 10_000,
	})
	plan, err := planAutoBoosterPurchase(t.Context(), Intent.PlanningContext{
		State: gameState, GameData: fortressIntentGameData(t),
	}, arguments)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Steps) != 4 || plan.Steps[0].Opcode != "gbd" || !plan.Steps[0].Command.Bare || plan.Steps[0].CaptureResponse ||
		plan.Steps[0].ResponseBarrier != Intent.ResponseBarrierCommitted ||
		plan.Steps[1].Opcode != "agb" || plan.Steps[1].ResponseBarrier != Intent.ResponseBarrierCommitted ||
		plan.Steps[1].PreDispatchAction != "auto_booster.purchase.arm" || plan.Steps[1].FinalDispatchAction != "auto_booster.purchase.dispatch" ||
		plan.Steps[2].Opcode != "gbd" || plan.Steps[3].Action != "auto_booster.purchase.reconcile" {
		t.Fatalf("Auto Booster steps = %#v", plan.Steps)
	}
	if !slices.Equal(plan.Claims, []string{"shop", "events", "global-effect:2", "account-resources"}) {
		t.Fatalf("Auto Booster claims=%v", plan.Claims)
	}
	gbd := plan.Steps[0].Command
	gbd.Namespace, gbd.Sequence = "EmpireEx_21", "1"
	wire, err := Protocol.Encode(gbd)
	if err != nil || string(wire) != "%xt%EmpireEx_21%gbd%1%" {
		t.Fatalf("GBD wire=%q err=%v", wire, err)
	}
	var payload map[string]int64
	if err := json.Unmarshal(plan.Steps[1].Command.Payload, &payload); err != nil {
		t.Fatal(err)
	}
	if len(payload) != 1 || payload["GEID"] != 2 {
		t.Fatalf("AGB payload = %#v", payload)
	}
}

func TestAutoBoosterFinalControlsRejectQueuedSettingChanges(t *testing.T) {
	gameState := State.NewGameState()
	gameState.Session.LoggedIn = true
	gameState.Session.SocketReady = true
	configuration, err := Configuration.Open(t.TempDir(), map[string]json.RawMessage{
		"automation.enabled":     json.RawMessage(`{"auto_booster":true}`),
		"automation.autoBooster": json.RawMessage(`{"version":1,"checkIntervalSec":60,"rubyCostCeiling":2500,"minimumRubyReserve":1000}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	application := &Application{State: State.NewStore(gameState), Configuration: configuration}
	request := autoBoosterPurchaseRequest{MinimumRubyReserve: 1000, ExpectedCheckIntervalSec: 60}
	if err := application.validateAutoBoosterControls(time.Now().UTC(), request); err != nil {
		t.Fatalf("valid controls rejected: %v", err)
	}
	if _, err := configuration.Update("automation.autoBooster", json.RawMessage(`{"version":1,"checkIntervalSec":60,"rubyCostCeiling":2500,"minimumRubyReserve":2000}`)); err != nil {
		t.Fatal(err)
	}
	if err := application.validateAutoBoosterControls(time.Now().UTC(), request); !errors.Is(err, Intent.ErrPlanStale) {
		t.Fatalf("raised reserve did not invalidate queued purchase: %v", err)
	}
	if _, err := configuration.Update("automation.autoBooster", json.RawMessage(`{"version":1,"checkIntervalSec":90,"rubyCostCeiling":2500,"minimumRubyReserve":1000}`)); err != nil {
		t.Fatal(err)
	}
	if err := application.validateAutoBoosterControls(time.Now().UTC(), request); !errors.Is(err, Intent.ErrPlanStale) {
		t.Fatalf("changed check interval did not invalidate queued purchase: %v", err)
	}
	if _, err := configuration.Update("automation.autoBooster", json.RawMessage(`{"version":1,"checkIntervalSec":10,"rubyCostCeiling":2500,"minimumRubyReserve":1000}`)); err != nil {
		t.Fatal(err)
	}
	if err := application.validateAutoBoosterControls(time.Now().UTC(), request); !errors.Is(err, Intent.ErrPlanStale) {
		t.Fatalf("invalid check interval did not invalidate queued purchase: %v", err)
	}
}

func TestAutoBoosterReconcileRecoversAcceptedAGBFromDurableReceipt(t *testing.T) {
	dataDir := t.TempDir()
	now := time.Now().UTC().Truncate(time.Second)
	endsAt := now.Add(time.Hour).Truncate(time.Minute)
	gameState := State.NewGameState()
	gameState.Session.ConnectionGeneration = 5
	gameState.EventScores.Inventory = autoBoosterIntentInventory(now, endsAt, 2500, false)
	gameState.EventScores.Inventory.GlobalEffectBaselineGeneration = 5
	gameState.EventScores.Inventory.GlobalEffectBoosts[2] = State.GlobalEffectBoostState{
		GlobalEffectID: 2, OccurrenceEndsAt: endsAt, ObservedAt: now, ConnectionGeneration: 5,
	}
	gameState.EventScores.Inventory.GlobalEffectPurchases[2] = State.GlobalEffectPurchaseRecord{
		GlobalEffectID: 2, OccurrenceEndsAt: endsAt, ExpiresAt: endsAt,
		RubyBefore: 10000, RubyBeforeObservedAt: now.Add(-2 * time.Minute),
		RequestedAt: now.Add(-2 * time.Minute), DispatchedAt: now.Add(-90 * time.Second),
		RequestOpcode: "agb", OperationID: "accepted-then-post-read-failed",
		ConnectionGeneration: 5, DebitUnverified: true, Outcome: State.GlobalEffectPurchaseUnresolved,
	}
	if err := State.SaveSnapshot(dataDir, gameState); err != nil {
		t.Fatal(err)
	}
	loaded, err := State.LoadSnapshot(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	stateStore := State.NewStore(loaded)
	operationStore, err := Intent.OpenOperationStore(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = operationStore.Close() })
	code := 0
	completedAt := now
	receipt := Intent.Receipt{
		ID: "accepted-then-post-read-failed", Intent: "autoBooster.purchase", Actor: "automation:autoBooster",
		Status: Intent.StatusPartiallySucceeded, SubmittedAt: now.Add(-2 * time.Minute), CompletedAt: &completedAt,
		Exchanges: []Intent.CommandExchange{{
			Step:     "Activate daily fortress-speed boost",
			Command:  Protocol.Command{Opcode: "agb", Payload: json.RawMessage(`{"GEID":2}`)},
			Response: &Protocol.Frame{Opcode: "agb", ResponseCode: &code, ReceivedAt: now.Add(-time.Minute)},
		}},
	}
	if _, created, err := operationStore.Reserve(t.Context(), "accepted-request", receipt); err != nil || !created {
		t.Fatalf("reserve receipt: created=%t err=%v", created, err)
	}
	if err := operationStore.Save(t.Context(), receipt); err != nil {
		t.Fatal(err)
	}
	engine := Intent.NewEngine(Intent.NewRegistry(), stateStore, autoBoosterIntentGameDataManager(t), nil, nil)
	if err := engine.SetOperationStore(t.Context(), operationStore); err != nil {
		t.Fatal(err)
	}
	application := &Application{
		DataDir: dataDir, State: stateStore, GameData: autoBoosterIntentGameDataManager(t), Intents: engine,
	}
	if err := application.reconcileAutoBoosterPurchase(t.Context(), nil); err != nil {
		t.Fatal(err)
	}
	record := stateStore.ReadOnlyView().EventScores.Inventory.GlobalEffectPurchases[2]
	if record.Outcome != State.GlobalEffectPurchaseAccepted || record.ResultCode == nil || *record.ResultCode != 0 || !record.DebitUnverified {
		t.Fatalf("recovered purchase record=%+v", record)
	}
	persisted, err := State.LoadSnapshot(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	if got := persisted.EventScores.Inventory.GlobalEffectPurchases[2].Outcome; got != State.GlobalEffectPurchaseAccepted {
		t.Fatalf("durable recovered outcome=%q", got)
	}
}

func TestAutoBoosterEnginePipelinePurchasesOnceForBothResponseOrderings(t *testing.T) {
	for _, bieFirst := range []bool{false, true} {
		t.Run(fmt.Sprintf("bie-first-%t", bieFirst), func(t *testing.T) {
			now := time.Now().UTC().Add(-time.Second).Truncate(time.Second)
			endsAt := now.Add(time.Hour).Truncate(time.Minute)
			gameState := State.NewGameState()
			gameState.Session.LoggedIn = true
			gameState.Session.SocketReady = true
			gameState.Session.ConnectionGeneration = 8
			gameState.Session.ChangedAt = now.Add(-time.Minute)
			stateStore := State.NewStore(gameState)
			manager := autoBoosterIntentGameDataManager(t)
			registry := Ingest.NewRegistry()
			if err := Ingest.RegisterCoreReducers(registry); err != nil {
				t.Fatal(err)
			}
			pipeline := Ingest.NewPipeline(stateStore, manager, registry)
			code := 0
			if _, err := pipeline.HandleFrame(t.Context(), Protocol.Frame{
				Opcode: "gbd", Direction: Protocol.DirectionInbound, ResponseCode: &code,
				ReceivedAt: now, Payload: autoBoosterGBDFixture(t, now, endsAt, false, 10000),
			}); err != nil {
				t.Fatal(err)
			}
			configuration, err := Configuration.Open(t.TempDir(), map[string]json.RawMessage{
				"automation.enabled":     json.RawMessage(`{"auto_booster":true}`),
				"automation.autoBooster": json.RawMessage(`{"version":1,"checkIntervalSec":60,"rubyCostCeiling":2500,"minimumRubyReserve":0}`),
			})
			if err != nil {
				t.Fatal(err)
			}
			dataDir := t.TempDir()
			application := &Application{DataDir: dataDir, State: stateStore, GameData: manager, Configuration: configuration, Ingest: pipeline}
			pipeline.SetDurabilityFence(application.saveStateEvent)
			sender := &autoBoosterIntegrationSender{application: application, pipeline: pipeline, endsAt: endsAt, bieFirst: bieFirst}
			engine := Intent.NewEngine(Intent.NewRegistry(), stateStore, manager, sender, pipeline)
			application.Intents = engine
			if err := application.registerAutoBoosterIntents(); err != nil {
				t.Fatal(err)
			}
			arguments, _ := json.Marshal(autoBoosterPurchaseRequest{
				GlobalEffectID: 2, ExpectedEndsAtUnix: endsAt.Unix(), ExpectedRubyCost: 2500,
				ExpectedBonusValue: 50, MinimumRubyReserve: 0, ExpectedCheckIntervalSec: 60, ExpectedRubyBalance: 10000,
			})
			receipt := engine.Submit(t.Context(), Intent.Request{
				ID: "auto-booster-engine-" + fmt.Sprint(bieFirst), Name: "autoBooster.purchase",
				Actor: "automation:autoBooster", AutomationLane: "autoBooster", Arguments: arguments,
			})
			if receipt.Status != Intent.StatusSucceeded {
				t.Fatalf("purchase receipt=%+v", receipt)
			}
			if got := fmt.Sprint(sender.opcodes); got != "[gbd agb gbd]" || sender.agbSends != 1 || !sender.persistedMarkerBeforeAGB {
				t.Fatalf("wire path opcodes=%s agb=%d marker=%t err=%v", got, sender.agbSends, sender.persistedMarkerBeforeAGB, sender.markerErr)
			}
			if sender.markerErr != nil {
				t.Fatal(sender.markerErr)
			}
			record := stateStore.ReadOnlyView().EventScores.Inventory.GlobalEffectPurchases[2]
			if record.Outcome != State.GlobalEffectPurchaseConfirmed || record.ResultCode == nil || *record.ResultCode != 0 ||
				!record.RubyAfterKnown || record.RubyAfter != 7500 || record.ObservedRubyChange != 2500 || !record.DebitUnverified {
				t.Fatalf("final purchase record=%+v", record)
			}
			persisted, err := State.LoadSnapshot(dataDir)
			if err != nil {
				t.Fatal(err)
			}
			if got := persisted.EventScores.Inventory.GlobalEffectPurchases[2].Outcome; got != State.GlobalEffectPurchaseConfirmed {
				t.Fatalf("persisted purchase outcome=%q", got)
			}

			second := engine.Submit(t.Context(), Intent.Request{
				ID: "auto-booster-recheck-" + fmt.Sprint(bieFirst), Name: "autoBooster.purchase",
				Actor: "automation:autoBooster", AutomationLane: "autoBooster", Arguments: arguments,
			})
			if second.Status == Intent.StatusSucceeded || sender.agbSends != 1 {
				t.Fatalf("re-evaluation replayed AGB: receipt=%+v sends=%d", second, sender.agbSends)
			}

			persisted.Session.LoggedIn = true
			persisted.Session.SocketReady = true
			persisted.Session.ConnectionGeneration = 9
			persisted.Session.ChangedAt = time.Now().UTC().Add(-time.Second)
			restartedStore := State.NewStore(persisted)
			restartedRegistry := Ingest.NewRegistry()
			if err := Ingest.RegisterCoreReducers(restartedRegistry); err != nil {
				t.Fatal(err)
			}
			restartedPipeline := Ingest.NewPipeline(restartedStore, manager, restartedRegistry)
			refreshAt := time.Now().UTC()
			if _, err := restartedPipeline.HandleFrame(t.Context(), Protocol.Frame{
				Opcode: "gbd", Direction: Protocol.DirectionInbound, ResponseCode: &code,
				ReceivedAt: refreshAt, Payload: autoBoosterGBDFixture(t, refreshAt, endsAt, false, 7500),
			}); err != nil {
				t.Fatal(err)
			}
			restartedApplication := &Application{DataDir: dataDir, State: restartedStore, GameData: manager, Configuration: configuration, Ingest: restartedPipeline}
			restartedSender := &autoBoosterIntegrationSender{application: restartedApplication, pipeline: restartedPipeline, endsAt: endsAt}
			restartedEngine := Intent.NewEngine(Intent.NewRegistry(), restartedStore, manager, restartedSender, restartedPipeline)
			restartedApplication.Intents = restartedEngine
			if err := restartedApplication.registerAutoBoosterIntents(); err != nil {
				t.Fatal(err)
			}
			restartedReceipt := restartedEngine.Submit(t.Context(), Intent.Request{
				ID: "auto-booster-restart-" + fmt.Sprint(bieFirst), Name: "autoBooster.purchase",
				Actor: "automation:autoBooster", AutomationLane: "autoBooster", Arguments: arguments,
			})
			if restartedReceipt.Status == Intent.StatusSucceeded || restartedSender.agbSends != 0 || len(restartedSender.opcodes) != 0 {
				t.Fatalf("restart replayed purchase: receipt=%+v opcodes=%v", restartedReceipt, restartedSender.opcodes)
			}
		})
	}
}

func TestAutoBoosterIndeterminateDispatchStaysBlockedUntilTerminalGBDReconciliation(t *testing.T) {
	now := time.Now().UTC().Add(-time.Second).Truncate(time.Second)
	endsAt := now.Add(time.Hour).Truncate(time.Minute)
	gameState := State.NewGameState()
	gameState.Session.LoggedIn = true
	gameState.Session.SocketReady = true
	gameState.Session.ConnectionGeneration = 12
	gameState.Session.ChangedAt = now.Add(-time.Minute)
	stateStore := State.NewStore(gameState)
	manager := autoBoosterIntentGameDataManager(t)
	registry := Ingest.NewRegistry()
	if err := Ingest.RegisterCoreReducers(registry); err != nil {
		t.Fatal(err)
	}
	pipeline := Ingest.NewPipeline(stateStore, manager, registry)
	code := 0
	if _, err := pipeline.HandleFrame(t.Context(), Protocol.Frame{
		Opcode: "gbd", Direction: Protocol.DirectionInbound, ResponseCode: &code,
		ReceivedAt: now, Payload: autoBoosterGBDFixture(t, now, endsAt, false, 10000),
	}); err != nil {
		t.Fatal(err)
	}
	configuration, err := Configuration.Open(t.TempDir(), map[string]json.RawMessage{
		"automation.enabled":     json.RawMessage(`{"auto_booster":true}`),
		"automation.autoBooster": json.RawMessage(`{"version":1,"checkIntervalSec":60,"rubyCostCeiling":2500,"minimumRubyReserve":0}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	application := &Application{DataDir: t.TempDir(), State: stateStore, GameData: manager, Configuration: configuration, Ingest: pipeline}
	sender := &autoBoosterIndeterminateSender{pipeline: pipeline, endsAt: endsAt}
	engine := Intent.NewEngine(Intent.NewRegistry(), stateStore, manager, sender, pipeline)
	application.Intents = engine
	if err := application.registerAutoBoosterIntents(); err != nil {
		t.Fatal(err)
	}
	arguments, _ := json.Marshal(autoBoosterPurchaseRequest{
		GlobalEffectID: 2, ExpectedEndsAtUnix: endsAt.Unix(), ExpectedRubyCost: 2500,
		ExpectedBonusValue: 50, ExpectedCheckIntervalSec: 60, ExpectedRubyBalance: 10000,
	})
	receipt := engine.Submit(t.Context(), Intent.Request{
		ID: "auto-booster-indeterminate", Name: "autoBooster.purchase", Actor: "automation:autoBooster",
		AutomationLane: "autoBooster", Arguments: arguments,
	})
	if receipt.Status != Intent.StatusIndeterminate || sender.agbSends != 1 {
		t.Fatalf("ambiguous dispatch receipt=%+v sends=%d", receipt, sender.agbSends)
	}
	record := stateStore.ReadOnlyView().EventScores.Inventory.GlobalEffectPurchases[2]
	if record.Outcome != State.GlobalEffectPurchaseUnresolved || record.DispatchedAt.IsZero() {
		t.Fatalf("ambiguous dispatch did not preserve unresolved marker: %+v", record)
	}

	refresh := engine.Submit(t.Context(), Intent.Request{
		ID: "auto-booster-indeterminate-refresh", Name: "autoBooster.refresh", Actor: "automation:autoBooster",
		AutomationLane: "autoBooster", Arguments: json.RawMessage(`{}`),
	})
	if refresh.Status != Intent.StatusSucceeded {
		t.Fatalf("reconciliation refresh=%+v", refresh)
	}
	record = stateStore.ReadOnlyView().EventScores.Inventory.GlobalEffectPurchases[2]
	if record.Outcome != State.GlobalEffectPurchaseRejected || record.ResultObservedAt.IsZero() || sender.agbSends != 1 {
		t.Fatalf("terminal inactive GBD did not resolve ambiguity: record=%+v sends=%d", record, sender.agbSends)
	}
}

type autoBoosterIntegrationSender struct {
	application              *Application
	pipeline                 *Ingest.Pipeline
	endsAt                   time.Time
	bieFirst                 bool
	purchased                bool
	agbSends                 int
	opcodes                  []string
	persistedMarkerBeforeAGB bool
	markerErr                error
	lastResponseAt           time.Time
}

type autoBoosterIndeterminateSender struct {
	pipeline       *Ingest.Pipeline
	endsAt         time.Time
	agbSends       int
	lastResponseAt time.Time
}

func (*autoBoosterIndeterminateSender) Ready() bool                  { return true }
func (*autoBoosterIndeterminateSender) Namespace() string            { return "EmpireEx_21" }
func (*autoBoosterIndeterminateSender) CorrelatesResponses() bool    { return true }
func (*autoBoosterIndeterminateSender) ConnectionGeneration() uint64 { return 12 }
func (sender *autoBoosterIndeterminateSender) Send(ctx context.Context, payload []byte) error {
	if err := Outbound.ValidateFinalDispatch(ctx); err != nil {
		return err
	}
	command, err := Protocol.Decode(string(payload), Protocol.DirectionOutbound, time.Now().UTC())
	if err != nil {
		return err
	}
	if command.Opcode == "agb" {
		sender.agbSends++
		return Outbound.MarkIndeterminate(context.DeadlineExceeded)
	}
	if command.Opcode != "gbd" {
		return fmt.Errorf("unexpected Auto Booster command %q", command.Opcode)
	}
	receivedAt := time.Now().UTC()
	if !receivedAt.After(sender.lastResponseAt) {
		receivedAt = sender.lastResponseAt.Add(time.Microsecond)
	}
	sender.lastResponseAt = receivedAt
	code := 0
	metadata := Outbound.MetadataFromContext(ctx)
	_, err = sender.pipeline.HandleFrame(ctx, Protocol.Frame{
		Opcode: "gbd", Direction: Protocol.DirectionInbound, ResponseCode: &code,
		ReceivedAt: receivedAt, Payload: autoBoosterGBDFixture(nil, receivedAt, sender.endsAt, false, 10000),
		ResponseToken: metadata.ResponseToken, CausationOperationID: metadata.OperationID,
	})
	return err
}

func (*autoBoosterIntegrationSender) Ready() bool                  { return true }
func (*autoBoosterIntegrationSender) Namespace() string            { return "EmpireEx_21" }
func (*autoBoosterIntegrationSender) CorrelatesResponses() bool    { return true }
func (*autoBoosterIntegrationSender) ConnectionGeneration() uint64 { return 8 }

func (sender *autoBoosterIntegrationSender) Send(ctx context.Context, payload []byte) error {
	if err := Outbound.ValidateFinalDispatch(ctx); err != nil {
		return err
	}
	command, err := Protocol.Decode(string(payload), Protocol.DirectionOutbound, time.Now().UTC())
	if err != nil {
		return err
	}
	sender.opcodes = append(sender.opcodes, command.Opcode)
	metadata := Outbound.MetadataFromContext(ctx)
	receivedAt := time.Now().UTC()
	if !receivedAt.After(sender.lastResponseAt) {
		receivedAt = sender.lastResponseAt.Add(time.Microsecond)
	}
	sender.lastResponseAt = receivedAt
	code := 0
	commit := func(opcode string, responsePayload json.RawMessage) error {
		_, commitErr := sender.pipeline.HandleFrame(ctx, Protocol.Frame{
			Opcode: opcode, Direction: Protocol.DirectionInbound, ResponseCode: &code,
			ReceivedAt: receivedAt, Payload: responsePayload, ResponseToken: metadata.ResponseToken,
			CausationOperationID: metadata.OperationID,
		})
		receivedAt = receivedAt.Add(time.Microsecond)
		sender.lastResponseAt = receivedAt
		return commitErr
	}
	switch command.Opcode {
	case "gbd":
		balance := int64(10000)
		if sender.purchased {
			balance = 7500
		}
		return commit("gbd", autoBoosterGBDFixture(nil, receivedAt, sender.endsAt, sender.purchased, balance))
	case "agb":
		sender.agbSends++
		persisted, loadErr := State.LoadSnapshot(sender.application.DataDir)
		if loadErr != nil {
			sender.markerErr = loadErr
		} else {
			record := persisted.EventScores.Inventory.GlobalEffectPurchases[2]
			sender.persistedMarkerBeforeAGB = record.Outcome == State.GlobalEffectPurchaseUnresolved &&
				record.OperationID == metadata.OperationID && !record.DispatchedAt.IsZero()
		}
		if sender.bieFirst {
			if err := commit("bie", json.RawMessage(`{"GE":[2]}`)); err != nil {
				return err
			}
		}
		if err := commit("agb", nil); err != nil {
			return err
		}
		if !sender.bieFirst {
			if err := commit("bie", json.RawMessage(`{"GE":[2]}`)); err != nil {
				return err
			}
		}
		sender.purchased = true
		return nil
	default:
		return fmt.Errorf("unexpected Auto Booster command %q", command.Opcode)
	}
}

func autoBoosterGBDFixture(t *testing.T, observedAt, endsAt time.Time, boosted bool, rubyBalance int64) json.RawMessage {
	if t != nil {
		t.Helper()
	}
	remaining := int64(math.Ceil(endsAt.Sub(observedAt).Seconds()))
	if remaining < 1 {
		remaining = 1
	}
	boostedIDs := "[]"
	if boosted {
		boostedIDs = "[2]"
	}
	return json.RawMessage(fmt.Sprintf(`{
		"gpi":{"UID":456,"PID":123,"PN":"Fixture Player"},
		"sei":{"E":[
			{"EID":610,"RS":%d,"GE":[[2,%d,10]]},
			{"EID":612,"RS":%d,"GEB":[{"GEID":2,"C2":2500,"BV":50}]}
		]},
		"bie":{"GE":%s},"gcu":{"C2":%d}
	}`, remaining, remaining, remaining, boostedIDs, rubyBalance))
}

func TestAutoBoosterPurchaseGuardRejectsChangedQuoteBalanceAndBoostState(t *testing.T) {
	now := time.Now().UTC()
	endsAt := now.Add(time.Hour).Truncate(time.Minute)
	gameState := State.NewGameState()
	gameState.Session.ChangedAt = now.Add(-time.Minute)
	gameState.Player.Resources[2] = 10_000
	gameState.Player.ResourceObservations[2] = State.PlayerResourceObservation{ObservedAt: now}
	gameState.EventScores.Inventory = autoBoosterIntentInventory(now, endsAt, 2500, false)
	request := autoBoosterPurchaseRequest{
		GlobalEffectID: 2, ExpectedEndsAtUnix: endsAt.Unix(), ExpectedRubyCost: 2500,
		ExpectedBonusValue: 60, MinimumRubyReserve: 5000, ExpectedCheckIntervalSec: 60, ExpectedRubyBalance: 10_000,
	}
	arguments, _ := json.Marshal(request)
	input := Intent.PlanningContext{State: gameState, GameData: fortressIntentGameData(t)}
	if _, err := autoBoosterPurchaseContext(input, arguments, now, true); err != nil {
		t.Fatalf("valid guarded purchase: %v", err)
	}

	offer := gameState.EventScores.Inventory.GlobalEffectBoosterOffers[2]
	offer.RubyCost = 2600
	gameState.EventScores.Inventory.GlobalEffectBoosterOffers[2] = offer
	input.State = gameState
	if _, err := autoBoosterPurchaseContext(input, arguments, now, true); !errors.Is(err, Intent.ErrPlanStale) {
		t.Fatalf("changed quote was accepted: %v", err)
	}

	offer.RubyCost = 2500
	gameState.EventScores.Inventory.GlobalEffectBoosterOffers[2] = offer
	gameState.Player.Resources[2] = 9_999
	input.State = gameState
	if _, err := autoBoosterPurchaseContext(input, arguments, now, true); !errors.Is(err, Intent.ErrPlanStale) {
		t.Fatalf("changed balance was accepted: %v", err)
	}

	gameState.Player.Resources[2] = 10_000
	status := gameState.EventScores.Inventory.GlobalEffectBoosts[2]
	status.Boosted = true
	gameState.EventScores.Inventory.GlobalEffectBoosts[2] = status
	input.State = gameState
	if _, err := autoBoosterPurchaseContext(input, arguments, now, true); !errors.Is(err, Intent.ErrPlanStale) {
		t.Fatalf("already active boost was repurchased: %v", err)
	}

	status.Boosted = false
	gameState.EventScores.Inventory.GlobalEffectBoosts[2] = status
	gameState.EventScores.Inventory.GlobalEffectPurchases[2] = State.GlobalEffectPurchaseRecord{
		GlobalEffectID: 2, OccurrenceEndsAt: endsAt, OperationID: "manual-op",
		Outcome: State.GlobalEffectPurchaseUnresolved,
	}
	input.State = gameState
	if _, err := autoBoosterPurchaseContextForOperation(input, arguments, now, true, "queued-auto-op"); !errors.Is(err, Intent.ErrPlanStale) {
		t.Fatalf("queued manual purchase was not detected at final boundary: %v", err)
	}
}

func autoBoosterIntentInventory(now, endsAt time.Time, rubyCost int64, boosted bool) State.EventInventoryState {
	return State.EventInventoryState{
		ObservedAt: now, ActiveByEvent: map[int64]State.EventAvailability{}, GlobalEffectsObservedAt: now, GlobalEffectBaselineObservedAt: now,
		GlobalEffects: map[int64]State.GlobalEffectAvailability{
			2: {GlobalEffectID: 2, Strength: 60, EndsAt: endsAt},
		},
		GlobalEffectBoosterOffers: map[int64]State.GlobalEffectBoosterOffer{
			2: {GlobalEffectID: 2, RubyCost: rubyCost, BonusValue: 60},
		},
		GlobalEffectBoostsObservedAt: now,
		GlobalEffectBoosts: map[int64]State.GlobalEffectBoostState{
			2: {GlobalEffectID: 2, Boosted: boosted, OccurrenceEndsAt: endsAt, ObservedAt: now},
		},
		GlobalEffectPurchases: map[int64]State.GlobalEffectPurchaseRecord{},
	}
}

func autoBoosterIntentGameDataManager(t *testing.T) *GameData.Manager {
	t.Helper()
	cacheDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(cacheDir, "Items-vtest.json"), []byte(`{
		"versionInfo":{"version":{"@value":"test"}},"buildings":[],"units":[],
		"effects":[
			{"effectID":2106,"name":"relicSpeedBonus","effectTypeID":15,"capID":1006},
			{"effectID":426,"name":"speedBonus","effectTypeID":15,"capID":99}
		],
		"effectCaps":[{"capID":1006,"maxTotalBonus":100}],
		"globalEffects":[{"ID":10,"globalEffectID":2,"name":"SpeedBoost","effects":"426&60","boostValue":60}],
		"resources":[{"resourceID":2,"JSONKey":"C2","name":"Rubies"}]
	}`), 0o600); err != nil {
		t.Fatal(err)
	}
	manager := GameData.NewManager(GameData.UpdaterConfig{CacheDir: cacheDir, VersionURL: "offline://items-version"})
	if err := manager.Initialize(t.Context()); err != nil {
		t.Fatal(err)
	}
	return manager
}
