package App

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"CitadelDesktop/Server/GameData"
	"CitadelDesktop/Server/Ingest"
	"CitadelDesktop/Server/Intent"
	"CitadelDesktop/Server/Outbound"
	"CitadelDesktop/Server/Protocol"
	"CitadelDesktop/Server/State"
)

// CIT-13 ABI/95 replay: GAA exact row cooldown 0 → guard passes → ABI 95.
// The lane locks, no CRA is sent, nothing is accounted as launched, and the
// fortress is deferred.
func TestFortressABICoolingDownDefersTargetWithoutLaunch(t *testing.T) {
	for _, test := range []struct {
		name       string
		defeaterID int
		deferral   time.Duration
	}{
		{"unknown defeater", -1, State.AttackTargetRejectionBase},
		{"own defeat unseen", 42, State.AttackTargetRejectionPersonalBase},
	} {
		t.Run(test.name, func(t *testing.T) {
			before := time.Now().UTC()
			receipt, sender, stateStore := runFortressEngineScenario(t, func(sender *fortressEngineSender, _ *State.Store) {
				sender.feedOutbound = true
				sender.responseCodes = map[string]int{"abi": 95}
				sender.gaaPayload = json.RawMessage(`{"KID":1,"AI":[[11,101,100,-1,45,0,` + itoa(test.defeaterID) + `,0]]}`)
			})
			committed, abi := indexOf(sender.events, "gaa:committed"), indexOf(sender.events, "abi")
			if committed < 0 || abi <= committed {
				t.Fatalf("CIT-10 order changed: %v", sender.events)
			}
			// Earlier context reads complete, so the launch ends failed or
			// partially_succeeded; either way the lane lock carries ABI 95.
			if receipt.Status != Intent.StatusFailed && receipt.Status != Intent.StatusPartiallySucceeded ||
				receipt.Failure == nil || receipt.Failure.SafetyLock == nil ||
				receipt.Failure.SafetyLock.Opcode != "abi" || receipt.Failure.SafetyLock.Code != 95 {
				t.Fatalf("ABI 95 receipt = %s %#v", receipt.Status, receipt.Failure)
			}
			if sender.craSends != 0 || indexOf(sender.events, "cra") >= 0 {
				t.Fatalf("CRA was sent after ABI 95: %v", sender.events)
			}
			view := stateStore.ReadOnlyView()
			if len(view.AttackAnalytics.PendingAttacks) != 0 {
				t.Fatalf("ABI 95 recorded a launch: %#v", view.AttackAnalytics.PendingAttacks)
			}
			rejection, rejected := State.AttackTargetRejectedAt(&view, 1, State.MapTypeKingdomFortress, 101, 100, time.Now().UTC())
			if !rejected || rejection.Opcode != "abi" || rejection.Code != 95 || rejection.Count != 1 ||
				rejection.Until.Sub(rejection.ObservedAt) != test.deferral || rejection.ObservedAt.Before(before) {
				t.Fatalf("fortress rejection = %#v found=%t", rejection, rejected)
			}
			// The planner, the exact-target guard and the CRA guard all refuse it.
			input := Intent.PlanningContext{State: view, GameData: fortressIntentGameData(t)}
			arguments := json.RawMessage(`{"sourceCastleId":10,"kingdomId":1,"targetX":101,"targetY":100,"commanderIds":[5],"horseTravelBoostId":-1}`)
			if _, _, _, _, err := fortressAttackContext(input, arguments, time.Now().UTC(), false); err == nil ||
				errors.Is(err, Intent.ErrPlanStale) || !strings.Contains(err.Error(), "Deferred: target 101:100 rejected by ABI 95") {
				t.Fatalf("planner accepted the rejected fortress: %v", err)
			}
		})
	}
}

func TestAttackGuardsRefuseRejectedTargets(t *testing.T) {
	now := time.Now().UTC()
	gameState := fortressIntentState(now)
	gameState.AttackDialog = State.AttackDialogState{
		SourceCastleID: 10, KingdomID: 1, ObservedAt: now,
		Target: State.AttackDialogTarget{TypeID: State.MapTypeKingdomFortress, X: 101, Y: 100},
	}
	State.RecordAttackTargetRejection(&gameState, State.AttackTargetRejection{
		KingdomID: 1, TargetTypeID: State.MapTypeKingdomFortress, X: 101, Y: 100, Opcode: "abi", Code: 95, ObservedAt: now,
	})
	application := &Application{State: State.NewStore(&gameState)}
	guardArguments, _ := json.Marshal(craSendGuardRequest{
		SourceX: 100, SourceY: 100, TargetX: 101, TargetY: 100, KingdomID: 1,
		DialogObservedAt: now.Add(-time.Second),
	})
	if err := application.guardCRASend(t.Context(), guardArguments); err == nil || !strings.Contains(err.Error(), "Deferred") {
		t.Fatalf("CRA guard accepted a rejected fortress: %v", err)
	}
}

// CIT-13 CRA/95 replay of the Storm plan tail: the CRA rejection stops the
// operation before attack.analytics.capture and storm.target.consume.
func TestStormCRACoolingDownRecordsNoLaunchAndDefersTarget(t *testing.T) {
	now := time.Now().UTC()
	gameState := State.NewGameState()
	gameState.Session = State.SessionState{
		Generation: 1, BaselineGeneration: 1, ConnectionGeneration: 1,
		Status: "connected", LoggedIn: true, SocketReady: true, Namespace: "EmpireEx_21",
	}
	gameState.Map[stormIntentKingdomID] = map[string]State.MapObservation{
		"101:102": {KingdomID: stormIntentKingdomID, X: 101, Y: 102, TypeID: stormIntentFortMapTypeID, StormIsleID: 7, ObservedAt: now},
		"110:110": {KingdomID: stormIntentKingdomID, X: 110, Y: 110, TypeID: stormIntentFortMapTypeID, StormIsleID: 7, ObservedAt: now},
	}
	stateStore := State.NewStore(&gameState)
	registry := Ingest.NewRegistry()
	if err := Ingest.RegisterCoreReducers(registry); err != nil {
		t.Fatal(err)
	}
	pipeline := Ingest.NewPipeline(stateStore, nil, registry)
	sender := &equipmentSaleEngineSender{pipeline: pipeline}
	intents := Intent.NewRegistry()
	craPayload := json.RawMessage(`{"SX":100,"SY":100,"TX":101,"TY":102,"KID":4,"LID":43,"A":[]}`)
	if err := intents.Register(Intent.Definition{Name: "storm.attack.test", Effect: Intent.EffectLaunch,
		Planner: func(context.Context, Intent.PlanningContext, json.RawMessage) (Intent.Plan, error) {
			return Intent.Plan{Steps: []Intent.Step{
				commandStep("Attack Storm fort", "cra", craPayload, "cra"),
				{Name: "Capture launch", Action: "attack.analytics.capture"},
				{Name: "Consume Storm target", Action: "storm.target.consume"},
			}}, nil
		}}); err != nil {
		t.Fatal(err)
	}
	engine := Intent.NewEngine(intents, stateStore, nil, &craRejectingSender{equipmentSaleEngineSender: sender}, pipeline)
	accounted := 0
	for _, name := range []string{"attack.analytics.capture", "storm.target.consume"} {
		if err := engine.RegisterAction(name, func(context.Context, json.RawMessage) error { accounted++; return nil }); err != nil {
			t.Fatal(err)
		}
	}
	receipt := engine.Submit(t.Context(), Intent.Request{
		ID: "storm-cra-95", Name: "storm.attack.test", Actor: "automation:autoStorm", AutomationLane: "autoStorm",
	})
	if receipt.Status != Intent.StatusFailed || receipt.Failure == nil || receipt.Failure.SafetyLock == nil ||
		receipt.Failure.SafetyLock.Opcode != "cra" || accounted != 0 {
		t.Fatalf("CRA 95 receipt = %s accounted=%d failure=%#v", receipt.Status, accounted, receipt.Failure)
	}
	view := stateStore.ReadOnlyView()
	if len(view.AttackAnalytics.PendingAttacks) != 0 || len(view.AttackAnalytics.RecentAutoStormLaunches) != 0 {
		t.Fatalf("CRA 95 was accounted as a launch: %#v", view.AttackAnalytics)
	}
	if _, stillMapped := view.LookupMapObservation(stormIntentKingdomID, "101:102"); !stillMapped {
		t.Fatal("Storm target left map state after a rejected launch")
	}
	if _, rejected := State.AttackTargetRejectedAt(&view, stormIntentKingdomID, stormIntentFortMapTypeID, 101, 102, now.Add(time.Second)); !rejected {
		t.Fatal("CRA 95 did not defer the Storm fort")
	}
	if _, rejected := State.AttackTargetRejectedAt(&view, stormIntentKingdomID, stormIntentFortMapTypeID, 110, 110, now.Add(time.Second)); rejected {
		t.Fatal("another Storm fort was deferred")
	}
}

// craRejectingSender answers one opcode with a rejection code and delegates
// every other command to the equipment test sender.
type craRejectingSender struct {
	*equipmentSaleEngineSender
	opcode string
	code   int
}

func (sender *craRejectingSender) Send(ctx context.Context, payload []byte) error {
	command, err := Protocol.Decode(string(payload), Protocol.DirectionOutbound, time.Now().UTC())
	if err != nil {
		return err
	}
	opcode, code := sender.opcode, sender.code
	if opcode == "" {
		opcode, code = "cra", 95
	}
	if command.Opcode != opcode {
		return sender.equipmentSaleEngineSender.Send(ctx, payload)
	}
	if _, err := sender.pipeline.HandleFrame(ctx, Protocol.Frame{
		Direction: Protocol.DirectionOutbound, Opcode: opcode, Payload: command.Payload, ReceivedAt: time.Now().UTC(),
	}); err != nil {
		return err
	}
	_, err = sender.pipeline.HandleFrame(ctx, Protocol.Frame{
		Direction: Protocol.DirectionInbound, Opcode: opcode, ResponseCode: &code, ReceivedAt: time.Now().UTC(),
		ResponseToken: Outbound.MetadataFromContext(ctx).ResponseToken,
	})
	return err
}

func TestStormAttackContextRefusesRejectedTargetWithoutStale(t *testing.T) {
	now := time.Now().UTC()
	state := State.NewGameState()
	state.Castles[40] = State.CastleState{ID: 40, KingdomID: stormIntentKingdomID, X: 100, Y: 100, Focused: true}
	state.Commanders[43] = State.CommanderState{ID: 43, Available: true}
	state.Map[stormIntentKingdomID] = map[string]State.MapObservation{
		"101:102": {KingdomID: stormIntentKingdomID, X: 101, Y: 102, TypeID: stormIntentFortMapTypeID, StormIsleID: 7, StormVictoryCount: 5, ObservedAt: now},
	}
	State.RecordAttackTargetRejection(&state, State.AttackTargetRejection{
		KingdomID: stormIntentKingdomID, TargetTypeID: stormIntentFortMapTypeID, X: 101, Y: 102, Opcode: "cra", Code: 95, ObservedAt: now,
	})
	gameData := stormAttackTestGameData(t)
	_, err := planStormAttack(t.Context(), Intent.PlanningContext{State: state, GameData: gameData}, json.RawMessage(`{
		"sourceCastleId":40,"kingdomId":4,"targetTypeId":25,"targetX":101,"targetY":102,
		"stormIsleId":7,"minimumVictoryCount":4,"commanderIds":[43],
		"preset":{"id":"fort","name":"Fort","waves":[]}
	}`))
	if err == nil || errors.Is(err, Intent.ErrPlanStale) || !strings.Contains(err.Error(), "rejected by CRA 95") {
		t.Fatalf("Storm planner accepted a rejected target: %v", err)
	}
}

func stormAttackTestGameData(t *testing.T) *GameData.Store {
	t.Helper()
	gameData, err := GameData.DecodeStore([]byte(`{
		"versionInfo":[],"buildings":[],"units":[],
		"isles":[{"IsleID":7,"type":"DUNGEON","dungeonlevel":40,"maxCountVictories":10,"countVictories":"0#1#2#3#4#5#6#7#8#9"}]
	}`), GameData.SourceMetadata{ItemVersion: "test"})
	if err != nil {
		t.Fatal(err)
	}
	return gameData
}

func itoa(value int) string {
	raw, _ := json.Marshal(value)
	return string(raw)
}

func indexOf(values []string, wanted string) int {
	for index, value := range values {
		if value == wanted {
			return index
		}
	}
	return -1
}

// The replay above uses the real plan tail order: launch accounting and target
// consumption only run after the CRA step succeeds.
func TestStormPlanAccountsLaunchOnlyAfterCRA(t *testing.T) {
	now := time.Now().UTC()
	state := State.NewGameState()
	state.Castles[40] = State.CastleState{ID: 40, KingdomID: stormIntentKingdomID, X: 100, Y: 100, Focused: true}
	state.Commanders[43] = State.CommanderState{ID: 43, Available: true}
	state.Map[stormIntentKingdomID] = map[string]State.MapObservation{
		"101:102": {KingdomID: stormIntentKingdomID, X: 101, Y: 102, TypeID: stormIntentFortMapTypeID, StormIsleID: 7, StormVictoryCount: 5, ObservedAt: now},
	}
	plan, err := planStormAttack(t.Context(), Intent.PlanningContext{State: state, GameData: stormAttackTestGameData(t)}, json.RawMessage(`{
		"sourceCastleId":40,"kingdomId":4,"targetTypeId":25,"targetX":101,"targetY":102,
		"stormIsleId":7,"minimumVictoryCount":4,"commanderIds":[43],
		"preset":{"id":"fort","name":"Fort","waves":[]}
	}`))
	if err != nil {
		t.Fatal(err)
	}
	cra, capture, consume := -1, -1, -1
	for index, step := range plan.Steps {
		switch {
		case step.Resolver == "storm.attack.build":
			cra = index
		case step.Action == "attack.analytics.capture":
			capture = index
		case step.Action == "storm.target.consume":
			consume = index
		}
	}
	if cra < 0 || capture <= cra || consume <= cra {
		t.Fatalf("Storm plan order cra=%d capture=%d consume=%d: %#v", cra, capture, consume, plan.Steps)
	}
}
