package App

import (
	"encoding/json"
	"errors"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"CitadelDesktop/Server/Configuration"
	"CitadelDesktop/Server/GameData"
	"CitadelDesktop/Server/Ingest"
	"CitadelDesktop/Server/Intent"
	"CitadelDesktop/Server/Protocol"
	"CitadelDesktop/Server/State"
)

func TestRegisteredKingdomTroopReconciliationIntentsRetainIdentityArguments(t *testing.T) {
	for _, intentName := range []string{
		"troops.kingdom.settle",
		"troops.kingdom.reconcile_donor",
		"troops.kingdom.skip.reconcile_timer",
		"troops.kingdom.skip.reconcile_inventory",
	} {
		t.Run(intentName, func(t *testing.T) {
			state, now := kingdomTroopReconciliationState(intentName)
			if intentName == "troops.kingdom.settle" {
				dataDir := t.TempDir()
				if err := State.SaveSnapshot(dataDir, state); err != nil {
					t.Fatal(err)
				}
				var err error
				state, err = State.LoadSnapshot(dataDir)
				if err != nil {
					t.Fatal(err)
				}
			}

			application, engine := newRegisteredKingdomTroopReconciliationEngine(t, state)
			arguments := json.RawMessage(`{"owner":"autoFortress","workflowId":"owned-transport","targetKingdomId":2}`)
			receipt := engine.Submit(t.Context(), Intent.Request{
				ID: "valid-" + strings.ReplaceAll(intentName, ".", "-"), Name: intentName,
				Actor: "automation:autoFortress", AutomationLane: "autoFortress", Arguments: arguments,
			})
			if receipt.Status != Intent.StatusSucceeded || receipt.Plan == nil {
				view := application.State.ReadOnlyView()
				t.Fatalf("registered %s receipt=%+v workflow=%#v observation=%#v session=%#v", intentName, receipt,
					view.KingdomTransport.TroopWorkflows[2], view.Player.CurrencyObservations[1005], view.Session)
			}
			if len(receipt.Plan.Steps) != 1 || string(receipt.Plan.Steps[0].ActionArguments) != string(arguments) {
				t.Fatalf("registered %s action arguments=%s plan=%#v", intentName, receipt.Plan.Steps[0].ActionArguments, receipt.Plan)
			}
			if len(receipt.Exchanges) != 0 {
				t.Fatalf("registered %s sent game commands: %#v", intentName, receipt.Exchanges)
			}

			workflow, exists := application.State.ReadOnlyView().KingdomTransport.TroopWorkflows[2]
			switch intentName {
			case "troops.kingdom.settle":
				if exists {
					t.Fatalf("persisted completed workflow was not retired: %#v", workflow)
				}
			case "troops.kingdom.reconcile_donor":
				if !exists || !workflow.SourceReconciledAt.Equal(now) {
					t.Fatalf("donor reconciliation result=%#v exists=%t", workflow, exists)
				}
			case "troops.kingdom.skip.reconcile_timer":
				if !exists || workflow.Status != "skip_inventory_pending" || workflow.RemainingSec != 0 {
					t.Fatalf("timer reconciliation result=%#v exists=%t", workflow, exists)
				}
			case "troops.kingdom.skip.reconcile_inventory":
				if !exists || workflow.Status != "awaiting_destination_refresh" || !workflow.SkipRequestedAt.IsZero() {
					t.Fatalf("inventory reconciliation result=%#v exists=%t", workflow, exists)
				}
			}
		})
	}
}

func TestRegisteredKingdomTroopReconciliationIntentsRejectMissingOrWrongIdentity(t *testing.T) {
	for _, intentName := range []string{
		"troops.kingdom.settle",
		"troops.kingdom.reconcile_donor",
		"troops.kingdom.skip.reconcile_timer",
		"troops.kingdom.skip.reconcile_inventory",
	} {
		for _, identity := range []struct {
			name      string
			arguments json.RawMessage
		}{
			{name: "missing", arguments: json.RawMessage(`{}`)},
			{name: "wrong-owner", arguments: json.RawMessage(`{"owner":"anotherAutomation","workflowId":"owned-transport","targetKingdomId":2}`)},
			{name: "wrong-workflow", arguments: json.RawMessage(`{"owner":"autoFortress","workflowId":"replacement","targetKingdomId":2}`)},
		} {
			t.Run(intentName+"/"+identity.name, func(t *testing.T) {
				state, _ := kingdomTroopReconciliationState(intentName)
				before := state.KingdomTransport.TroopWorkflows[2]
				application, engine := newRegisteredKingdomTroopReconciliationEngine(t, state)
				receipt := engine.Submit(t.Context(), Intent.Request{
					ID: "invalid-" + identity.name + "-" + strings.ReplaceAll(intentName, ".", "-"), Name: intentName,
					Actor: "automation:autoFortress", AutomationLane: "autoFortress", Arguments: identity.arguments,
				})
				if receipt.Status == Intent.StatusSucceeded {
					t.Fatalf("registered %s accepted %s identity: %+v", intentName, identity.name, receipt)
				}
				after, exists := application.State.ReadOnlyView().KingdomTransport.TroopWorkflows[2]
				if !exists || !reflect.DeepEqual(after, before) {
					t.Fatalf("registered %s changed another workflow for %s identity: before=%#v after=%#v exists=%t", intentName, identity.name, before, after, exists)
				}
				if len(receipt.Exchanges) != 0 {
					t.Fatalf("registered %s sent game commands for %s identity: %#v", intentName, identity.name, receipt.Exchanges)
				}
			})
		}
	}
}

func TestActionPlannerOwnsForwardedArguments(t *testing.T) {
	arguments := json.RawMessage(`{"owner":"autoFortress"}`)
	planner := actionPlanner("test.action", "troop-transport", "Test action")
	plan, err := planner(t.Context(), Intent.PlanningContext{}, arguments)
	if err != nil {
		t.Fatal(err)
	}
	arguments[10] = 'X'
	if got := string(plan.Steps[0].ActionArguments); got != `{"owner":"autoFortress"}` {
		t.Fatalf("action planner retained caller-owned arguments: %s", got)
	}
	empty, err := planner(t.Context(), Intent.PlanningContext{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(empty.Steps[0].ActionArguments) != 0 {
		t.Fatalf("empty action arguments = %q", empty.Steps[0].ActionArguments)
	}
}

func newRegisteredKingdomTroopReconciliationEngine(t *testing.T, state State.GameState) (*Application, *Intent.Engine) {
	t.Helper()
	stateStore := State.NewStore(state)
	if workflow := state.KingdomTransport.TroopWorkflows[2]; workflow.Status == "skip_inventory_pending" {
		if _, err := stateStore.ApplyComponents(State.Components(State.ComponentPlayer), func(current *State.GameState) ([]string, bool, error) {
			current.Player.Currencies[1005] = state.Player.Currencies[1005]
			current.Player.CurrencyObservations[1005] = state.Player.CurrencyObservations[1005]
			return []string{"currencies"}, true, nil
		}); err != nil {
			t.Fatal(err)
		}
	}
	registry := Intent.NewRegistry()
	registry.EnforceResourceDeclarations()
	engine := Intent.NewEngine(registry, stateStore, nil, nil, nil)
	application := &Application{State: stateStore, Intents: engine}
	if err := application.registerGameIntents(); err != nil {
		t.Fatal(err)
	}
	return application, engine
}

func kingdomTroopReconciliationState(intentName string) (State.GameState, time.Time) {
	now := time.Date(2026, time.September, 16, 12, 0, 0, 0, time.UTC)
	state := State.NewGameState()
	state.Session.ConnectionGeneration = 7
	donor := kingdomTroopIntentCastle(10, 0, "Donor")
	donor.UnitsObservedAt = now
	target := kingdomTroopIntentCastle(20, 2, "Destination")
	target.UnitsObservedAt = now
	state.Castles[donor.ID] = donor
	state.Castles[target.ID] = target
	state.Player.Currencies[1005] = 1
	state.Player.CurrencyObservations[1005] = State.PlayerResourceObservation{
		ObservedAt: now, ConnectionGeneration: state.Session.ConnectionGeneration,
	}
	workflow := State.KingdomTroopTransportWorkflow{
		ID: "owned-transport", Owner: "autoFortress", Status: "awaiting_destination_refresh", KingdomID: 2,
		SourceCastleID: donor.ID, TargetCastleID: target.ID,
		Units:               []State.KingdomTransportUnit{{UnitID: 10, Amount: 5}},
		TransportObservedAt: now.Add(-time.Minute), SourceDebitedLocally: true, SessionGeneration: 7,
	}
	switch intentName {
	case "troops.kingdom.reconcile_donor":
		workflow.Status = "ownership_absent"
		workflow.SourceDebitedLocally = false
	case "troops.kingdom.skip.reconcile_timer":
		workflow.Status = "pending"
		workflow.TransportObservedAt = now
		workflow.SkipCurrencyID = 1005
		workflow.SkipWireKey = "MS5"
		workflow.SkipBalanceBefore = 2
		workflow.SkipRemainingBefore = 3600
		workflow.SkipDurationSec = 3600
		workflow.SkipRequestedAt = now.Add(-10 * time.Second)
	case "troops.kingdom.skip.reconcile_inventory":
		workflow.Status = "skip_inventory_pending"
		workflow.SkipCurrencyID = 1005
		workflow.SkipWireKey = "MS5"
		workflow.SkipBalanceBefore = 2
		workflow.SkipRemainingBefore = 3600
		workflow.SkipDurationSec = 3600
		workflow.SkipRequestedAt = now.Add(-10 * time.Second)
	}
	state.KingdomTransport.TroopWorkflows[2] = workflow
	return state, now
}

func TestKingdomTroopShipmentUsesCapturedKutShape(t *testing.T) {
	gameData := kingdomTroopIntentGameData(t)
	gameState := State.NewGameState()
	donor := kingdomTroopIntentCastle(10, 0, "Donor")
	donor.Focused = true
	donor.Units.Stationed[10] = 20
	donor.Units.Stationed[20] = 30
	target := kingdomTroopIntentCastle(40, 4, "Storm")
	gameState.Castles[donor.ID] = donor
	gameState.Castles[target.ID] = target
	gameState.KingdomTransport.ObservedAt = time.Now().UTC()
	gameState.KingdomTransport.Unlocks[4] = State.KingdomTransportUnlock{KingdomID: 4, Unlocked: true}

	plan, err := planKingdomTroopShipment(t.Context(), Intent.PlanningContext{State: gameState, GameData: gameData}, json.RawMessage(`{
		"sourceCastleId":10,"targetCastleId":40,"targetKingdomId":4,
		"units":[{"unitId":20,"amount":7},{"unitId":10,"amount":5}]
	}`))
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Steps) != 4 || plan.Steps[0].Opcode != "kpi" ||
		plan.Steps[1].Action != "kingdom.transport.verify_available" || plan.Steps[2].Opcode != "kut" ||
		plan.Steps[3].Action != "troops.kingdom.consume_source" {
		t.Fatalf("unexpected troop transfer steps: %#v", plan.Steps)
	}
	if got := string(plan.Steps[2].Command.Payload); got != `{"SCID":10,"SKID":0,"TKID":4,"CID":-1,"A":[[10,5],[20,7]]}` {
		t.Fatalf("KUT payload = %s", got)
	}
}

func TestOwnedKingdomTroopPlansArmBeforeDispatchAndUseGBDConfirmation(t *testing.T) {
	gameData := kingdomTroopIntentGameData(t)
	gameState := State.NewGameState()
	donor := kingdomTroopIntentCastle(10, 0, "Donor")
	donor.Focused = true
	donor.Units.Stationed[10] = 20
	target := kingdomTroopIntentCastle(40, 4, "Storm")
	gameState.Castles[donor.ID] = donor
	gameState.Castles[target.ID] = target
	gameState.KingdomTransport.ObservedAt = time.Now().UTC()
	gameState.KingdomTransport.Unlocks[4] = State.KingdomTransportUnlock{KingdomID: 4, Unlocked: true}

	plan, err := planKingdomTroopShipment(t.Context(), Intent.PlanningContext{State: gameState, GameData: gameData}, json.RawMessage(`{
		"sourceCastleId":10,"targetCastleId":40,"targetKingdomId":4,"owner":"autoFortress","workflowId":"owned-1",
		"units":[{"unitId":10,"amount":5}]
	}`))
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Steps) != 5 || plan.Steps[2].Opcode != "kut" ||
		plan.Steps[2].PreDispatchAction != "troops.kingdom.workflow.arm" ||
		plan.Steps[2].FinalDispatchAction != "troops.kingdom.workflow.dispatch" ||
		plan.Steps[2].DefinitiveSendFailureAction != "troops.kingdom.workflow.disarm" ||
		!plan.Steps[2].ResponseProjectionFailureIndeterminate ||
		plan.Steps[3].Action != "troops.kingdom.workflow.confirm" ||
		plan.Steps[4].Action != "troops.kingdom.consume_source" {
		t.Fatalf("owned troop workflow steps=%#v", plan.Steps)
	}

	gameState.KingdomTransport.TroopWorkflows[4] = State.KingdomTroopTransportWorkflow{
		ID: "owned-1", Owner: "autoFortress", Status: "pending", KingdomID: 4,
		SourceCastleID: donor.ID, TargetCastleID: target.ID,
		Units: []State.KingdomTransportUnit{{UnitID: 10, Amount: 5}},
	}
	gameState.KingdomTransport.PendingUnits = []State.KingdomUnitTransport{{
		KingdomID: 4, RemainingSec: 3600, Units: []State.KingdomTransportUnit{{UnitID: 10, Amount: 5}},
	}}
	gameState.Player.Currencies[1005] = 2
	skipPlan, err := planKingdomTroopSkip(t.Context(), Intent.PlanningContext{State: gameState, GameData: gameData}, json.RawMessage(`{
		"targetKingdomId":4,"timeSkipId":"MS5","owner":"autoFortress","workflowId":"owned-1","expectedRemaining":3600
	}`))
	if err != nil {
		t.Fatal(err)
	}
	if len(skipPlan.Steps) != 4 || skipPlan.Steps[0].PreDispatchAction != "troops.kingdom.skip.arm" ||
		skipPlan.Steps[0].FinalDispatchAction != "troops.kingdom.skip.dispatch" ||
		skipPlan.Steps[1].Action != "troops.kingdom.skip.verify_timer" || skipPlan.Steps[2].Opcode != "gbd" ||
		!skipPlan.Steps[2].Command.Bare || skipPlan.Steps[3].Action != "troops.kingdom.skip.verify_inventory" {
		t.Fatalf("owned troop skip steps=%#v", skipPlan.Steps)
	}
	for _, step := range skipPlan.Steps {
		if step.Action == timeSkipConsumeAction {
			t.Fatalf("owned troop skip used local currency decrement: %#v", skipPlan.Steps)
		}
	}
}

func TestOwnedKingdomTroopDispatchStopsWhenDestinationIsDisabled(t *testing.T) {
	now := time.Now().UTC()
	gameState := State.NewGameState()
	gameState.Session.ConnectionGeneration = 3
	gameState.KingdomTransport.Unlocks[2] = State.KingdomTransportUnlock{KingdomID: 2, Unlocked: true}
	donor := kingdomTroopIntentCastle(10, 0, "Donor")
	donor.UnitsObservedAt = now
	donor.Units.Stationed[10] = 20
	target := kingdomTroopIntentCastle(20, 2, "Sands")
	gameState.Castles[donor.ID] = donor
	gameState.Castles[target.ID] = target
	gameState.KingdomTransport.TroopWorkflows[2] = State.KingdomTroopTransportWorkflow{
		ID: "owned", Owner: "autoFortress", Status: "armed", KingdomID: 2,
		SourceCastleID: donor.ID, TargetCastleID: target.ID, SessionGeneration: 3,
		Units: []State.KingdomTransportUnit{{UnitID: 10, Amount: 5}},
	}
	configuration, err := Configuration.Open(t.TempDir(), map[string]json.RawMessage{
		"automation.autoFortress": json.RawMessage(`{"kingdoms":{"2":{"enabled":false}}}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	application := &Application{State: State.NewStore(gameState), Configuration: configuration}
	err = application.guardKingdomTroopWorkflowDispatch(t.Context(), json.RawMessage(`{
		"sourceCastleId":10,"targetCastleId":20,"targetKingdomId":2,"owner":"autoFortress","workflowId":"owned",
		"units":[{"unitId":10,"amount":5}]
	}`))
	if !errors.Is(err, Intent.ErrPlanStale) || !strings.Contains(err.Error(), "disabled") {
		t.Fatalf("disabled final-dispatch guard=%v", err)
	}
}

func TestOwnedKingdomTroopArmIsDurableBeforeDispatch(t *testing.T) {
	state := State.NewGameState()
	state.Session.ConnectionGeneration = 9
	dataDir := t.TempDir()
	application := &Application{DataDir: dataDir, State: State.NewStore(state)}
	arguments := json.RawMessage(`{
		"sourceCastleId":10,"targetCastleId":20,"targetKingdomId":2,"owner":"autoFortress","workflowId":"owned",
		"units":[{"unitId":10,"amount":5}]
	}`)
	if err := application.armKingdomTroopWorkflow(t.Context(), arguments); err != nil {
		t.Fatal(err)
	}
	persisted, err := State.LoadSnapshot(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	workflow, found := persisted.KingdomTransport.TroopWorkflows[2]
	if !found || workflow.ID != "owned" || workflow.Status != "ownership_uncertain" || workflow.SessionGeneration != 0 {
		t.Fatalf("durable owned workflow=%#v found=%t", workflow, found)
	}
}

func TestOwnedKingdomTroopDonorConsumptionAvoidsAuthoritativeDoubleDebit(t *testing.T) {
	now := time.Now().UTC()
	for _, test := range []struct {
		name             string
		sourceObservedAt time.Time
		wantUnits        int64
		wantLocal        bool
	}{
		{name: "post-response-authority", sourceObservedAt: now, wantUnits: 90, wantLocal: false},
		{name: "cached-source", sourceObservedAt: now.Add(-2 * time.Minute), wantUnits: 80, wantLocal: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			state := State.NewGameState()
			source := kingdomTroopIntentCastle(10, 0, "Donor")
			source.Units.Stationed[10], source.Units.Total[10], source.UnitsObservedAt = 90, 90, test.sourceObservedAt
			state.Castles[10] = source
			state.KingdomTransport.TroopWorkflows[2] = State.KingdomTroopTransportWorkflow{
				ID: "owned", Owner: "autoFortress", Status: "pending", KingdomID: 2, SourceCastleID: 10,
				TransportObservedAt: now.Add(-time.Minute), Units: []State.KingdomTransportUnit{{UnitID: 10, Amount: 10}},
			}
			application := &Application{State: State.NewStore(state)}
			if err := application.consumeKingdomTroopSource(t.Context(), json.RawMessage(`{"sourceCastleId":10,"targetKingdomId":2,"owner":"autoFortress","workflowId":"owned","units":[{"unitId":10,"amount":10}]}`)); err != nil {
				t.Fatal(err)
			}
			view := application.State.ReadOnlyView()
			if got := view.Castles[10].Units.Stationed[10]; got != test.wantUnits {
				t.Fatalf("donor units=%d want=%d", got, test.wantUnits)
			}
			workflow := view.KingdomTransport.TroopWorkflows[2]
			if workflow.SourceDebitedLocally != test.wantLocal || (!test.wantLocal && workflow.SourceReconciledAt.IsZero()) {
				t.Fatalf("donor evidence=%#v", workflow)
			}
			if test.wantLocal && !view.Castles[10].UnitsObservedAt.Equal(test.sourceObservedAt) {
				t.Fatalf("local debit manufactured authority: %v", view.Castles[10].UnitsObservedAt)
			}
			if err := application.consumeKingdomTroopSource(t.Context(), json.RawMessage(`{"sourceCastleId":10,"targetKingdomId":2,"owner":"autoFortress","workflowId":"owned","units":[{"unitId":10,"amount":10}]}`)); err != nil {
				t.Fatal(err)
			}
			if got := application.State.ReadOnlyView().Castles[10].Units.Stationed[10]; got != test.wantUnits {
				t.Fatalf("repeated consume debited donor twice: got=%d want=%d", got, test.wantUnits)
			}
		})
	}
}

func TestOwnedKingdomTroopReconnectEvidenceRestoresDonorReconciliation(t *testing.T) {
	frameAt := time.Now().UTC().Add(-time.Second)
	for _, test := range []struct {
		name       string
		workflow   State.KingdomTroopTransportWorkflow
		payload    json.RawMessage
		wantStatus string
	}{
		{
			name: "lost-consume-pending-restart",
			workflow: State.KingdomTroopTransportWorkflow{
				ID: "pending", Owner: "autoFortress", Status: "pending", KingdomID: 2,
				SourceCastleID: 10, TargetCastleID: 20, ArmedAt: frameAt.Add(-time.Hour),
				TransportObservedAt: frameAt.Add(-10 * time.Second), RemainingSec: 3600,
				Units: []State.KingdomTransportUnit{{UnitID: 10, Amount: 5}},
			},
			payload:    json.RawMessage(`{"UL":[{"KID":2,"U":1}],"UT":[{"KID":2,"RS":3590,"I":[[10,5]]}]}`),
			wantStatus: "pending",
		},
		{
			name: "armed-empty-reconnect",
			workflow: State.KingdomTroopTransportWorkflow{
				ID: "armed", Owner: "autoFortress", Status: "armed", KingdomID: 2,
				SourceCastleID: 10, TargetCastleID: 20, ArmedAt: frameAt.Add(-time.Hour),
				Units: []State.KingdomTransportUnit{{UnitID: 10, Amount: 5}},
			},
			payload:    json.RawMessage(`{"UL":[{"KID":2,"U":1}]}`),
			wantStatus: "ownership_absent",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			state := State.NewGameState()
			state.Session.ConnectionGeneration = 11
			donor := kingdomTroopIntentCastle(10, 0, "Donor")
			donor.Units.Stationed[10], donor.Units.Total[10] = 95, 95
			state.Castles[10] = donor
			state.Castles[20] = kingdomTroopIntentCastle(20, 2, "Sands")
			state.KingdomTransport.TroopWorkflows[2] = test.workflow

			store := State.NewStore(state)
			registry := Ingest.NewRegistry()
			if err := Ingest.RegisterCoreReducers(registry); err != nil {
				t.Fatal(err)
			}
			pipeline := Ingest.NewPipeline(store, beriIntentGameDataProvider{store: kingdomTroopIntentGameData(t)}, registry)
			code := 0
			if _, err := pipeline.HandleFrame(t.Context(), Protocol.Frame{
				Opcode: "kpi", Direction: Protocol.DirectionInbound, ResponseCode: &code,
				ReceivedAt: frameAt, Payload: test.payload,
			}); err != nil {
				t.Fatal(err)
			}
			rebound := store.ReadOnlyView().KingdomTransport.TroopWorkflows[2]
			if rebound.Status != test.wantStatus || rebound.SessionGeneration != 11 || !rebound.TransportObservedAt.Equal(frameAt) {
				t.Fatalf("rebound workflow=%#v", rebound)
			}

			freshAt := time.Now().UTC()
			if _, err := store.ApplyComponents(State.Components(State.ComponentCastles), func(current *State.GameState) ([]string, bool, error) {
				donor, found := current.MutableCastleParts(10, State.CastlePartUnits)
				if !found {
					return nil, false, errors.New("donor missing")
				}
				donor.UnitsObservedAt = freshAt
				current.SetCastleParts(10, donor, State.CastlePartUnits)
				return []string{"castles", "units"}, true, nil
			}); err != nil {
				t.Fatal(err)
			}
			application := &Application{State: store}
			if err := application.reconcileKingdomTroopDonor(t.Context(), json.RawMessage(`{"owner":"autoFortress","workflowId":"`+test.workflow.ID+`","targetKingdomId":2}`)); err != nil {
				t.Fatal(err)
			}
			reconciled := store.ReadOnlyView().KingdomTransport.TroopWorkflows[2]
			if !reconciled.SourceReconciledAt.Equal(freshAt) || reconciled.SourceDebitedLocally {
				t.Fatalf("reconciled workflow=%#v", reconciled)
			}
		})
	}
}

func TestOwnedKingdomTroopSkipRequiresDurationProgressAndInventoryConsumption(t *testing.T) {
	now := time.Now().UTC()
	state := State.NewGameState()
	state.Session.ConnectionGeneration = 5
	state.Player.Currencies[1005] = 2
	state.KingdomTransport.PendingUnits = []State.KingdomUnitTransport{{KingdomID: 2, RemainingSec: 3590, Units: []State.KingdomTransportUnit{{UnitID: 10, Amount: 5}}}}
	state.KingdomTransport.TroopWorkflows[2] = State.KingdomTroopTransportWorkflow{
		ID: "owned", Owner: "autoFortress", Status: "pending", KingdomID: 2,
		Units: []State.KingdomTransportUnit{{UnitID: 10, Amount: 5}}, TransportObservedAt: now,
		SkipCurrencyID: 1005, SkipWireKey: "MS5", SkipBalanceBefore: 2, SkipRemainingBefore: 3600,
		SkipDurationSec: 3600, SkipRequestedAt: now.Add(-10 * time.Second),
	}
	application := &Application{State: State.NewStore(state)}
	arguments := json.RawMessage(`{"targetKingdomId":2,"owner":"autoFortress","workflowId":"owned"}`)
	if err := application.verifyKingdomTroopSkipTimer(t.Context(), arguments); err == nil || !strings.Contains(err.Error(), "natural countdown") {
		t.Fatalf("natural countdown confirmed skip progress: %v", err)
	}
	if observed := application.State.ReadOnlyView().KingdomTransport.TroopWorkflows[2].SkipTimerObservedAt; !observed.Equal(now) {
		t.Fatalf("natural-countdown failure marker was not committed: %v", observed)
	}

	_, err := application.State.ApplyComponents(State.Components(State.ComponentKingdomTransport), func(current *State.GameState) ([]string, bool, error) {
		current.KingdomTransport.PendingUnits = nil
		return []string{"kingdom-transport"}, true, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := application.verifyKingdomTroopSkipTimer(t.Context(), arguments); err != nil {
		t.Fatal(err)
	}
	_, err = application.State.ApplyComponents(State.Components(State.ComponentPlayer), func(current *State.GameState) ([]string, bool, error) {
		current.Player.CurrencyObservations[1005] = State.PlayerResourceObservation{ObservedAt: now.Add(time.Second), ConnectionGeneration: 5}
		return []string{"currencies"}, true, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := application.verifyKingdomTroopSkipInventory(t.Context(), arguments); err == nil {
		t.Fatal("unchanged authoritative inventory confirmed consumption")
	}
	uncertain := application.State.ReadOnlyView().KingdomTransport.TroopWorkflows[2]
	if uncertain.Status != "skip_uncertain" || uncertain.SkipRequestedAt.IsZero() || uncertain.SkipInventoryObservedAt.IsZero() {
		t.Fatalf("uncertain spend marker=%#v", uncertain)
	}
	_, err = application.State.ApplyComponents(State.Components(State.ComponentPlayer), func(current *State.GameState) ([]string, bool, error) {
		current.Player.Currencies[1005] = 1
		current.Player.CurrencyObservations[1005] = State.PlayerResourceObservation{ObservedAt: now.Add(2 * time.Second), ConnectionGeneration: 5}
		return []string{"currencies"}, true, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := application.verifyKingdomTroopSkipInventory(t.Context(), arguments); err != nil {
		t.Fatal(err)
	}
	confirmed := application.State.ReadOnlyView().KingdomTransport.TroopWorkflows[2]
	if confirmed.Status != "awaiting_destination_refresh" || !confirmed.SkipRequestedAt.IsZero() {
		t.Fatalf("confirmed spend=%#v", confirmed)
	}
}

func TestKingdomTroopShipmentRejectsToolsAndSkipUsesTroopTransportType(t *testing.T) {
	gameData := kingdomTroopIntentGameData(t)
	gameState := State.NewGameState()
	donor := kingdomTroopIntentCastle(10, 0, "Donor")
	donor.Focused = true
	donor.Units.Stationed[30] = 10
	target := kingdomTroopIntentCastle(40, 4, "Storm")
	gameState.Castles[donor.ID] = donor
	gameState.Castles[target.ID] = target
	gameState.KingdomTransport.ObservedAt = time.Now().UTC()
	gameState.KingdomTransport.Unlocks[4] = State.KingdomTransportUnlock{KingdomID: 4, Unlocked: true}

	_, err := planKingdomTroopShipment(t.Context(), Intent.PlanningContext{State: gameState, GameData: gameData}, json.RawMessage(`{
		"sourceCastleId":10,"targetCastleId":40,"targetKingdomId":4,"units":[{"unitId":30,"amount":1}]
	}`))
	if err == nil || !strings.Contains(err.Error(), "tool") {
		t.Fatalf("tool transfer error = %v", err)
	}

	gameState.KingdomTransport.PendingUnits = []State.KingdomUnitTransport{{KingdomID: 4, RemainingSec: 3600}}
	gameState.Player.Currencies[1005] = 2
	plan, err := planKingdomTroopSkip(t.Context(), Intent.PlanningContext{State: gameState, GameData: gameData}, json.RawMessage(`{
		"targetKingdomId":4,"timeSkipId":"MS5"
	}`))
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Steps) != 2 || plan.Steps[1].Action != timeSkipConsumeAction {
		t.Fatalf("troop time-skip steps = %#v", plan.Steps)
	}
	if got := string(plan.Steps[0].Command.Payload); got != `{"KID":"4","MST":"MS5","TT":"1"}` {
		t.Fatalf("troop time-skip payload = %s", got)
	}
	if plan.Summary != "Apply a 1-hour time skip to kingdom 4 troop transport" {
		t.Fatalf("troop time-skip summary = %q", plan.Summary)
	}
	if !slices.Contains(plan.Claims, "currency:1005") {
		t.Fatalf("troop time-skip plan is missing its currency claim: %#v", plan.Claims)
	}

	application := &Application{State: State.NewStore(gameState)}
	if err := application.consumeTimeSkip(t.Context(), plan.Steps[1].ActionArguments); err != nil {
		t.Fatal(err)
	}
	if got := application.State.Snapshot().Player.Currencies[1005]; got != 1 {
		t.Fatalf("reconciled troop time-skip inventory = %v, want 1", got)
	}
	if err := application.consumeTimeSkip(t.Context(), plan.Steps[1].ActionArguments); err != nil {
		t.Fatal(err)
	}
	if got := application.State.Snapshot().Player.Currencies[1005]; got != 1 {
		t.Fatalf("replayed reconciliation consumed another skip: %v", got)
	}
}

func TestKingdomTroopShipmentRejectsStormBelowMeadFloor(t *testing.T) {
	gameData := kingdomTroopIntentGameData(t)
	gameState := State.NewGameState()
	donor := kingdomTroopIntentCastle(10, 0, "Donor")
	donor.Units.Stationed[10] = 20
	target := kingdomTroopIntentCastle(40, 4, "Storm")
	target.Resources[12] = State.ResourceBalance{Amount: GameData.StormTroopSupportMead - 1}
	gameState.Castles[donor.ID] = donor
	gameState.Castles[target.ID] = target
	gameState.KingdomTransport.ObservedAt = time.Now().UTC()
	gameState.KingdomTransport.Unlocks[4] = State.KingdomTransportUnlock{KingdomID: 4, Unlocked: true}

	_, err := planKingdomTroopShipment(t.Context(), Intent.PlanningContext{
		State: gameState, GameData: gameData,
	}, json.RawMessage(`{
		"sourceCastleId":10,"targetCastleId":40,"targetKingdomId":4,"units":[{"unitId":10,"amount":1}]
	}`))
	if err == nil || !strings.Contains(err.Error(), "at least 50000 Mead") {
		t.Fatalf("low Storm Mead error = %v", err)
	}
}

func TestKingdomTroopShipmentCapCountsAwayTargetTroops(t *testing.T) {
	gameData := kingdomTroopIntentGameData(t)
	gameState := State.NewGameState()
	donor := kingdomTroopIntentCastle(10, 0, "Donor")
	donor.Units.Stationed[10] = 20
	target := kingdomTroopIntentCastle(40, 4, "Storm")
	target.Units.Stationed[10] = 2
	target.Units.Traveling[10] = 6
	gameState.Castles[donor.ID] = donor
	gameState.Castles[target.ID] = target
	gameState.Movements[99] = State.MovementState{
		ID: 99, SourceCastleID: target.ID, KingdomID: target.KingdomID,
		Units: map[State.UnitID]int64{10: 6},
	}
	gameState.KingdomTransport.ObservedAt = time.Now().UTC()
	gameState.KingdomTransport.Unlocks[4] = State.KingdomTransportUnlock{KingdomID: 4, Unlocked: true}

	_, err := planKingdomTroopShipment(t.Context(), Intent.PlanningContext{
		State: gameState, GameData: gameData,
	}, json.RawMessage(`{
		"sourceCastleId":10,"targetCastleId":40,"targetKingdomId":4,
		"maximumTargetTroops":10,"units":[{"unitId":10,"amount":3}]
	}`))
	if err == nil || !strings.Contains(err.Error(), "above its 10-troop import cap") ||
		!strings.Contains(err.Error(), "8 committed, 3 incoming") {
		t.Fatalf("away-target troop cap error = %v", err)
	}
}

func TestKingdomTroopShipmentAddsExecutionTimeCapGuard(t *testing.T) {
	gameData := kingdomTroopIntentGameData(t)
	gameState := State.NewGameState()
	donor := kingdomTroopIntentCastle(10, 0, "Donor")
	donor.Focused = true
	donor.Units.Stationed[10] = 20
	target := kingdomTroopIntentCastle(40, 4, "Storm")
	target.Units.Stationed[10] = 2
	gameState.Castles[donor.ID] = donor
	gameState.Castles[target.ID] = target
	gameState.KingdomTransport.ObservedAt = time.Now().UTC()
	gameState.KingdomTransport.Unlocks[4] = State.KingdomTransportUnlock{KingdomID: 4, Unlocked: true}

	plan, err := planKingdomTroopShipment(t.Context(), Intent.PlanningContext{
		State: gameState, GameData: gameData,
	}, json.RawMessage(`{
		"sourceCastleId":10,"targetCastleId":40,"targetKingdomId":4,
		"maximumTargetTroops":10,"units":[{"unitId":10,"amount":3}]
	}`))
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Steps) != 5 || plan.Steps[2].Action != "troops.kingdom.guard_target_cap" ||
		plan.Steps[3].Opcode != "kut" {
		t.Fatalf("capped troop transfer steps = %#v", plan.Steps)
	}
}

func TestKingdomTroopShipmentRejectsChangedExpectedAttackReset(t *testing.T) {
	gameData := kingdomTroopIntentGameData(t)
	gameState := State.NewGameState()
	donor := kingdomTroopIntentCastle(10, 0, "Donor")
	donor.Units.Stationed[10] = 20
	target := kingdomTroopIntentCastle(40, 4, "Storm")
	gameState.Castles[donor.ID] = donor
	gameState.Castles[target.ID] = target
	gameState.KingdomTransport.ObservedAt = time.Now().UTC()
	gameState.KingdomTransport.Unlocks[4] = State.KingdomTransportUnlock{KingdomID: 4, Unlocked: true}
	gameState.DailyAttacks.SessionStartedAt = time.Date(2026, time.September, 9, 0, 0, 0, 0, time.UTC)
	expected := gameState.DailyAttacks.SessionStartedAt.Add(-24 * time.Hour)
	arguments, err := json.Marshal(kingdomTroopShipmentRequest{
		SourceCastleID: 10, TargetCastleID: 40, TargetKingdomID: 4,
		MaximumTargetTroops: 5_000, ExpectedDailyAttackSessionStartedAt: &expected,
		Units: []kingdomTroopShipmentUnit{{UnitID: 10, Amount: 3}},
	})
	if err != nil {
		t.Fatal(err)
	}

	_, err = planKingdomTroopShipment(t.Context(), Intent.PlanningContext{
		State: gameState, GameData: gameData,
	}, arguments)
	if !errors.Is(err, Intent.ErrPlanStale) || !strings.Contains(err.Error(), "reset changed") {
		t.Fatalf("changed attack-reset guard error = %v", err)
	}
}

func kingdomTroopIntentGameData(t *testing.T) *GameData.Store {
	t.Helper()
	store, err := GameData.DecodeStore([]byte(`{
		"versionInfo":[],"buildings":[],
		"units":[{"wodID":10},{"wodID":20},{"wodID":30,"slotTypes":"1,2"}],
		"resources":[{"resourceID":12,"JSONKey":"MEAD"}],
		"currencies":[{"currencyID":1005,"JSONKey":"MS5"}],
		"currencyMinutesSkipValues":[{"currencyID":"1005","MinutesSkipValue":"60"}]
	}`), GameData.SourceMetadata{ItemVersion: "test"})
	if err != nil {
		t.Fatal(err)
	}
	return store
}

func kingdomTroopIntentCastle(id State.CastleID, kingdom State.KingdomID, name string) State.CastleState {
	return State.CastleState{
		ID: id, KingdomID: kingdom, Name: name,
		Resources:           map[State.ResourceID]State.ResourceBalance{12: {Amount: GameData.StormTroopSupportMead}},
		FoodStateObservedAt: time.Now().UTC(),
		Units: State.CastleUnits{
			Stationed: map[State.UnitID]int64{}, Traveling: map[State.UnitID]int64{},
			Hospital: map[State.UnitID]int64{}, SpecialHospital: map[State.UnitID]int64{}, Total: map[State.UnitID]int64{},
		},
	}
}
