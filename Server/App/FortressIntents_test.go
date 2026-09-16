package App

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
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

func TestFortressAttackBuildsOneFullDirewolfFlankWaveWithoutPremiumBooster(t *testing.T) {
	gameData := fortressIntentGameData(t)
	now := time.Now().UTC()
	gameState := fortressIntentState(now)
	arguments := json.RawMessage(`{
		"sourceCastleId":10,"kingdomId":1,"targetX":101,"targetY":100,
		"commanderIds":[5],"horseTravelBoostId":-1,"minimumCommanderSpeedBonus":100
	}`)
	plan, err := planFortressAttack(t.Context(), Intent.PlanningContext{State: gameState, GameData: gameData}, arguments)
	if err != nil {
		t.Fatal(err)
	}
	if plan.Admission == nil || plan.Admission.Module != "autoFortress" || plan.Admission.Class != Intent.AdmissionAttackLaunch {
		t.Fatalf("fortress admission = %#v", plan.Admission)
	}
	var deferred Intent.Step
	gaaCount, gaaIndex, guardIndex, launchIndex := 0, -1, -1, -1
	for index, step := range plan.Steps {
		if step.Opcode == "gaa" {
			gaaCount++
			gaaIndex = index
			if step.ResponseBarrier != Intent.ResponseBarrierCommitted {
				t.Fatalf("fortress target refresh is not committed: %#v", step)
			}
			if step.FinalDispatchAction != "fortress.target.verification.arm" {
				t.Fatalf("fortress target refresh does not arm exact-response proof: %#v", step)
			}
			var payload struct {
				X1 int `json:"AX1"`
				Y1 int `json:"AY1"`
				X2 int `json:"AX2"`
				Y2 int `json:"AY2"`
			}
			if err := json.Unmarshal(step.Command.Payload, &payload); err != nil {
				t.Fatal(err)
			}
			if payload.X1 != 101 || payload.Y1 != 100 || payload.X2 != 101 || payload.Y2 != 100 {
				t.Fatalf("fortress pre-CRA refresh is not 1x1: %#v", payload)
			}
		}
		if step.Action == "fortress.target.verification.guard" {
			guardIndex = index
		}
		if step.Resolver == "fortress.attack.build" {
			deferred = step
			launchIndex = index
		}
	}
	if gaaCount != 1 || gaaIndex < 0 || guardIndex <= gaaIndex || launchIndex <= guardIndex {
		t.Fatalf("fortress plan must commit one 1x1 GAA before CRA: %#v", plan.Steps)
	}
	if deferred.Resolver == "" || deferred.CommandDependencies == nil || deferred.CommandDependencies.Opcode != "cra" {
		t.Fatalf("fortress plan has no guarded CRA resolver: %#v", plan.Steps)
	}
	gameState.AttackDialog = State.AttackDialogState{
		SourceCastleID: 10, KingdomID: 1, ObservedAt: time.Now().UTC(),
		Target: State.AttackDialogTarget{TypeID: State.MapTypeKingdomFortress, X: 101, Y: 100, Level: 45},
	}
	resolved, err := (&Application{}).resolveFortressAttackStep(t.Context(), Intent.PlanningContext{State: gameState, GameData: gameData}, deferred.ResolverArguments)
	if err != nil {
		t.Fatal(err)
	}
	var body struct {
		Leader State.CommanderID `json:"LID"`
		Waves  []attackWave      `json:"A"`
	}
	if err := json.Unmarshal(resolved.Command.Payload, &body); err != nil {
		t.Fatal(err)
	}
	if resolved.Opcode != "cra" || body.Leader != 5 || len(body.Waves) != 1 {
		t.Fatalf("fortress CRA body = %#v", body)
	}
	if resolved.FinalDispatchAction != "fortress.target.verification.guard" {
		t.Fatalf("fortress CRA has no final exact-target guard: %#v", resolved)
	}
	wave := body.Waves[0]
	if wave.Left.Units[0][0] != GameData.DirewolfUnitID || wave.Left.Units[0][1] <= 0 ||
		wave.Right.Units[0][0] != GameData.DirewolfUnitID || wave.Right.Units[0][1] <= 0 ||
		wave.Middle.Units[0] != (attackPair{-1, 0}) {
		t.Fatalf("fortress formation is not one full Direwolf flank wave: %#v", wave)
	}
}

func TestFortressAttackEngineFinalizesOnlyWithAuthoritativeMovement(t *testing.T) {
	for _, projectMovement := range []bool{true, false} {
		t.Run(map[bool]string{true: "confirmed", false: "missing-movement"}[projectMovement], func(t *testing.T) {
			now := time.Now().UTC()
			gameData := fortressIntentGameData(t)
			gameState := fortressIntentState(now)
			gameState.Player.ID = 42
			gameState.Session.ConnectionGeneration = 1
			stateStore := State.NewStore(gameState)
			ingestRegistry := Ingest.NewRegistry()
			if err := Ingest.RegisterCoreReducers(ingestRegistry); err != nil {
				t.Fatal(err)
			}
			pipeline := Ingest.NewPipeline(stateStore, beriIntentGameDataProvider{store: gameData}, ingestRegistry)
			sender := &fortressEngineSender{
				pipeline: pipeline, state: stateStore, projectMovement: projectMovement, connectionGeneration: 1,
			}
			intentRegistry := Intent.NewRegistry()
			intentRegistry.EnforceResourceDeclarations()
			engine := Intent.NewEngine(intentRegistry, stateStore, beriIntentGameDataProvider{store: gameData}, sender, pipeline)
			application := &Application{State: stateStore, Intents: engine, Ingest: pipeline}
			if err := intentRegistry.Register(Intent.Definition{Name: "fortress.attack", Effect: Intent.EffectLaunch, Planner: planFortressAttack}); err != nil {
				t.Fatal(err)
			}
			if err := engine.RegisterStepResolver("fortress.attack.build", application.resolveFortressAttackStep); err != nil {
				t.Fatal(err)
			}
			if err := engine.RegisterCommandDependencies("cra", application.resolveCRACommandDependencies); err != nil {
				t.Fatal(err)
			}
			fortressGuard := func(ctx context.Context, arguments json.RawMessage) error {
				if err := application.guardFortressTargetVerification(ctx, arguments); err != nil {
					return err
				}
				sender.events = append(sender.events, "fortress:guard")
				return nil
			}
			for name, action := range map[string]Intent.Action{
				"game.ui.close":                      func(context.Context, json.RawMessage) error { return nil },
				"attack.cra.send.guard":              application.guardCRASend,
				"attack.analytics.capture":           application.captureAttackFeatureLaunch,
				"fortress.target.verification.arm":   application.armFortressTargetVerification,
				"fortress.target.verification.guard": fortressGuard,
			} {
				if err := engine.RegisterAction(name, action); err != nil {
					t.Fatal(err)
				}
			}
			receipt := engine.Submit(t.Context(), Intent.Request{
				ID: "fortress-engine", Name: "fortress.attack", Actor: "automation:autoFortress", AutomationLane: "autoFortress",
				Arguments: json.RawMessage(`{"sourceCastleId":10,"kingdomId":1,"targetX":101,"targetY":100,"commanderIds":[5],"horseTravelBoostId":-1,"minimumCommanderSpeedBonus":100}`),
			})
			committedIndex := slices.Index(sender.events, "gaa:committed")
			guardIndex := slices.Index(sender.events, "fortress:guard")
			adiIndex := slices.Index(sender.events, "adi")
			craIndex := slices.Index(sender.events, "cra")
			if committedIndex < 0 || guardIndex <= committedIndex || adiIndex <= guardIndex || craIndex <= adiIndex {
				t.Fatalf("fortress dispatch order = %v", sender.events)
			}
			if projectMovement {
				if receipt.Status != Intent.StatusSucceeded || sender.craSends != 1 {
					t.Fatalf("confirmed engine receipt=%+v opcodes=%v", receipt, sender.opcodes)
				}
				launches := stateStore.ReadOnlyView().AttackAnalytics.PendingAttacks
				if len(launches) != 1 || launches[0].FeatureID != State.AttackFeatureAutoFortress || launches[0].MovementID != 99 {
					t.Fatalf("authoritative fortress launch=%#v", launches)
				}
				projected := stateStore.ReadOnlyView()
				if movement, found := projected.LookupMovement(99); !found || movement.Units[GameData.DirewolfUnitID] != 100 {
					t.Fatalf("production CRA movement=%#v found=%t", movement, found)
				}
				if donor := projected.Castles[10].Units.Stationed[GameData.DirewolfUnitID]; donor != 10_000 {
					t.Fatalf("CRA launch rewrote authoritative donor stock: got=%d want=10000", donor)
				}
				if !projected.Castles[10].UnitsObservedAt.IsZero() {
					t.Fatal("confirmed launch left pre-launch donor stock authoritative")
				}
				if _, found := projected.LookupTowerCooldown("1:101:100"); found {
					t.Fatal("CRA launch created a fortress victory cooldown before a battle report")
				}
				code := 0
				for _, frame := range []Protocol.Frame{
					{Opcode: "jaa", Direction: Protocol.DirectionInbound, ResponseCode: &code, ReceivedAt: time.Now().UTC(), Payload: json.RawMessage(`{"KID":1,"gca":{"A":[12,100,100,10,42,0,0,0,0,0,"Winter Keep"]},"gui":{"I":[[277,9900]],"TU":[],"HI":[],"SHI":[]}}`)},
					{Opcode: "bls", Direction: Protocol.DirectionInbound, ResponseCode: &code, ReceivedAt: time.Now().UTC(), Payload: json.RawMessage(`{"MID":101,"LID":202,"PBI":[[42,0,1700,-10],[-220,1,135,-135]],"AI":{"AT":11,"K":1,"X":101,"Y":100}}`)},
					{Opcode: "gaa", Direction: Protocol.DirectionInbound, ResponseCode: &code, ReceivedAt: time.Now().UTC(), Payload: json.RawMessage(`{"KID":1,"AI":[[11,101,100,0,45,431998,42,1]]}`)},
				} {
					if _, err := pipeline.HandleFrame(t.Context(), frame); err != nil {
						t.Fatal(err)
					}
				}
				final := stateStore.ReadOnlyView()
				if donor := final.Castles[10].Units.Stationed[GameData.DirewolfUnitID]; donor != 9_900 || final.Castles[10].UnitsObservedAt.IsZero() {
					t.Fatalf("authoritative post-launch donor stock=%d observed=%v", donor, final.Castles[10].UnitsObservedAt)
				}
				cooldown, found := final.LookupTowerCooldown("1:101:100")
				if !found || cooldown.PendingCooldownRefresh || cooldown.CooldownRemaining != 431_998 || cooldown.LastSuccessfulBattleAt.IsZero() {
					t.Fatalf("authoritative fortress victory cooldown=%#v found=%t", cooldown, found)
				}
			} else {
				if receipt.Status == Intent.StatusSucceeded || sender.craSends != 1 || !strings.Contains(receipt.Error, "did not return") {
					t.Fatalf("missing-movement receipt=%+v opcodes=%v", receipt, sender.opcodes)
				}
			}
		})
	}
}

type fortressEngineSender struct {
	pipeline                *Ingest.Pipeline
	state                   *State.Store
	opcodes                 []string
	events                  []string
	craSends                int
	projectMovement         bool
	gaaPayload              json.RawMessage
	gaaReceivedAt           time.Time
	gaaCausationOperationID string
	reconnectAfterGAA       bool
	connectionGeneration    uint64
	beforeFinalDispatch     func(string) error
}

func (*fortressEngineSender) Ready() bool               { return true }
func (*fortressEngineSender) Namespace() string         { return "EmpireEx_21" }
func (*fortressEngineSender) CorrelatesResponses() bool { return true }
func (sender *fortressEngineSender) ConnectionGeneration() uint64 {
	if sender.connectionGeneration == 0 {
		return 1
	}
	return sender.connectionGeneration
}

func (sender *fortressEngineSender) Send(ctx context.Context, payload []byte) error {
	command, err := Protocol.Decode(string(payload), Protocol.DirectionOutbound, time.Now().UTC())
	if err != nil {
		return err
	}
	if sender.beforeFinalDispatch != nil {
		if err := sender.beforeFinalDispatch(command.Opcode); err != nil {
			return err
		}
	}
	if err := Outbound.ValidateFinalDispatch(ctx); err != nil {
		return err
	}
	sender.opcodes = append(sender.opcodes, command.Opcode)
	sender.events = append(sender.events, command.Opcode)
	responseOpcode := command.Opcode
	if command.Opcode == "jca" {
		responseOpcode = "jaa"
	}
	if command.Opcode == "cra" {
		sender.craSends++
	}
	responsePayload := json.RawMessage(`{}`)
	switch command.Opcode {
	case "jaa", "jca":
		responsePayload = json.RawMessage(`{"KID":1,"gca":{"A":[12,100,100,10,42,0,0,0,0,0,"Winter Keep"]},"gui":{"I":[[277,10000]],"TU":[],"HI":[],"SHI":[]}}`)
	case "gaa":
		responsePayload = sender.gaaPayload
		if len(responsePayload) == 0 {
			responsePayload = json.RawMessage(`{"KID":1,"AI":[[11,101,100,-1,45,0,-1,0]]}`)
		}
	case "gam":
		responsePayload = json.RawMessage(`{"M":[],"O":[]}`)
	case "adi":
		responsePayload = json.RawMessage(`{"KID":1,"SCID":10,"gaa":{"AI":[11,101,100,-1,45,0,-1,0]},"AE":[]}`)
	case "gas":
		responsePayload = json.RawMessage(`{"S":[]}`)
	case "cra":
		if sender.projectMovement {
			responsePayload = json.RawMessage(`{"M":{"MID":99,"PT":0,"TT":60,"D":0,"T":0,"KID":1,"OID":42,"TID":-1,"SA":[12,100,100,10,42],"TA":[11,101,100,-1,-1]},"UM":{"L":{"ID":5}},"A":[[277,100]]}`)
		}
	}
	metadata := Outbound.MetadataFromContext(ctx)
	code := 0
	receivedAt := time.Now().UTC()
	if !sender.gaaReceivedAt.IsZero() && command.Opcode == "gaa" {
		receivedAt = sender.gaaReceivedAt
	}
	causation := ""
	if sender.gaaCausationOperationID != "" && command.Opcode == "gaa" {
		causation = sender.gaaCausationOperationID
	}
	_, err = sender.pipeline.HandleFrame(ctx, Protocol.Frame{
		Opcode: responseOpcode, Direction: Protocol.DirectionInbound, ResponseCode: &code,
		ReceivedAt: receivedAt, Payload: responsePayload, ResponseToken: metadata.ResponseToken,
		CausationOperationID: causation,
	})
	if err == nil && command.Opcode == "gaa" {
		sender.events = append(sender.events, "gaa:committed")
		if sender.reconnectAfterGAA && sender.state != nil {
			_, err = sender.state.ApplyComponents(State.Components(State.ComponentSession), func(gameState *State.GameState) ([]string, bool, error) {
				gameState.Session.ConnectionGeneration++
				return []string{"session"}, true, nil
			})
			sender.connectionGeneration++
		}
	}
	return err
}

func runFortressEngineScenario(
	t *testing.T,
	configure func(*fortressEngineSender, *State.Store),
) (Intent.Receipt, *fortressEngineSender, *State.Store) {
	t.Helper()
	now := time.Now().UTC()
	gameData := fortressIntentGameData(t)
	gameState := fortressIntentState(now)
	gameState.Player.ID = 42
	gameState.Session.ConnectionGeneration = 1
	stateStore := State.NewStore(gameState)
	ingestRegistry := Ingest.NewRegistry()
	if err := Ingest.RegisterCoreReducers(ingestRegistry); err != nil {
		t.Fatal(err)
	}
	pipeline := Ingest.NewPipeline(stateStore, beriIntentGameDataProvider{store: gameData}, ingestRegistry)
	sender := &fortressEngineSender{
		pipeline: pipeline, state: stateStore, projectMovement: true, connectionGeneration: 1,
	}
	if configure != nil {
		configure(sender, stateStore)
	}
	intentRegistry := Intent.NewRegistry()
	intentRegistry.EnforceResourceDeclarations()
	engine := Intent.NewEngine(intentRegistry, stateStore, beriIntentGameDataProvider{store: gameData}, sender, pipeline)
	application := &Application{State: stateStore, Intents: engine, Ingest: pipeline}
	if err := intentRegistry.Register(Intent.Definition{Name: "fortress.attack", Effect: Intent.EffectLaunch, Planner: planFortressAttack}); err != nil {
		t.Fatal(err)
	}
	if err := engine.RegisterStepResolver("fortress.attack.build", application.resolveFortressAttackStep); err != nil {
		t.Fatal(err)
	}
	if err := engine.RegisterCommandDependencies("cra", application.resolveCRACommandDependencies); err != nil {
		t.Fatal(err)
	}
	for name, action := range map[string]Intent.Action{
		"game.ui.close":                      func(context.Context, json.RawMessage) error { return nil },
		"attack.cra.send.guard":              application.guardCRASend,
		"attack.analytics.capture":           application.captureAttackFeatureLaunch,
		"fortress.target.verification.arm":   application.armFortressTargetVerification,
		"fortress.target.verification.guard": application.guardFortressTargetVerification,
	} {
		if err := engine.RegisterAction(name, action); err != nil {
			t.Fatal(err)
		}
	}
	receipt := engine.Submit(t.Context(), Intent.Request{
		ID: "fortress-engine-scenario", Name: "fortress.attack", Actor: "automation:autoFortress", AutomationLane: "autoFortress",
		Arguments: json.RawMessage(`{"sourceCastleId":10,"kingdomId":1,"targetX":101,"targetY":100,"commanderIds":[5],"horseTravelBoostId":-1,"minimumCommanderSpeedBonus":100}`),
	})
	return receipt, sender, stateStore
}

func TestFortressAttackRejectsNonAuthoritativeExactTargetResponsesBeforeADI(t *testing.T) {
	tests := []struct {
		name      string
		payload   json.RawMessage
		configure func(*fortressEngineSender, *State.Store)
	}{
		{name: "captured cooldown 84430", payload: json.RawMessage(`{"KID":1,"AI":[[11,101,100,-1,55,84430,14722196,3]]}`)},
		{name: "explicit empty", payload: json.RawMessage(`{"KID":1,"AI":[]}`)},
		{name: "missing rows", payload: json.RawMessage(`{"KID":1}`)},
		{name: "malformed row", payload: json.RawMessage(`{"KID":1,"AI":[[11,101,100]]}`)},
		{name: "malformed cooldown", payload: json.RawMessage(`{"KID":1,"AI":[[11,101,100,-1,45,"bad",-1,0]]}`)},
		{name: "wrong target", payload: json.RawMessage(`{"KID":1,"AI":[[11,102,100,-1,45,0,-1,0]]}`)},
		{name: "changed target type", payload: json.RawMessage(`{"KID":1,"AI":[[2,101,100,-1,45,0,0]]}`)},
		{name: "wrong kingdom", payload: json.RawMessage(`{"KID":2,"AI":[[11,101,100,-1,45,0,-1,0]]}`)},
		{name: "conflicting nonempty causation", payload: json.RawMessage(`{"KID":1,"AI":[[11,101,100,-1,45,0,-1,0]]}`), configure: func(sender *fortressEngineSender, _ *State.Store) {
			sender.gaaCausationOperationID = "previous-operation"
		}},
		{name: "stale response", payload: json.RawMessage(`{"KID":1,"AI":[[11,101,100,-1,45,0,-1,0]]}`), configure: func(sender *fortressEngineSender, _ *State.Store) {
			sender.gaaReceivedAt = time.Now().UTC().Add(-time.Minute)
		}},
		{name: "reconnect", payload: json.RawMessage(`{"KID":1,"AI":[[11,101,100,-1,45,0,-1,0]]}`), configure: func(sender *fortressEngineSender, _ *State.Store) {
			sender.reconnectAfterGAA = true
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			receipt, sender, stateStore := runFortressEngineScenario(t, func(sender *fortressEngineSender, store *State.Store) {
				sender.gaaPayload = test.payload
				if test.configure != nil {
					test.configure(sender, store)
				}
			})
			if receipt.Status == Intent.StatusSucceeded || slices.Contains(sender.opcodes, "adi") || slices.Contains(sender.opcodes, "cra") {
				t.Fatalf("unsafe response launched attack: receipt=%+v opcodes=%v", receipt, sender.opcodes)
			}
			if test.name == "captured cooldown 84430" {
				target, found := stateStore.ReadOnlyView().LookupMapObservation(1, "101:100")
				if !found || target.TowerCooldownRemaining != 84_430 {
					t.Fatalf("captured cooldown projection=%#v found=%t", target, found)
				}
			}
			if test.name == "explicit empty" || test.name == "wrong target" {
				if _, found := stateStore.ReadOnlyView().LookupMapObservation(1, "101:100"); found {
					t.Fatal("authoritatively absent fortress remained eligible for another request")
				}
			}
		})
	}
}

func TestFortressAttackRechecksProofAtQueuedADIAndFinalCRA(t *testing.T) {
	t.Run("queued ADI projection changed", func(t *testing.T) {
		mutated := false
		receipt, sender, _ := runFortressEngineScenario(t, func(sender *fortressEngineSender, store *State.Store) {
			sender.beforeFinalDispatch = func(opcode string) error {
				if opcode != "adi" || mutated {
					return nil
				}
				mutated = true
				_, err := store.ApplyComponents(State.Components(State.ComponentWorldMap), func(gameState *State.GameState) ([]string, bool, error) {
					target, _ := gameState.LookupMapObservation(1, "101:100")
					target.ObservedAt = time.Now().UTC().Add(time.Second)
					gameState.SetMapObservation(target)
					return []string{"map-fortress"}, true, nil
				})
				return err
			}
		})
		if receipt.Status == Intent.StatusSucceeded || slices.Contains(sender.opcodes, "adi") || slices.Contains(sender.opcodes, "cra") {
			t.Fatalf("queued ADI used invalidated proof: receipt=%+v opcodes=%v", receipt, sender.opcodes)
		}
	})

	t.Run("queued ADI focus changed", func(t *testing.T) {
		mutated := false
		receipt, sender, _ := runFortressEngineScenario(t, func(sender *fortressEngineSender, store *State.Store) {
			_, err := store.ApplyComponents(State.Components(State.ComponentCastles), func(gameState *State.GameState) ([]string, bool, error) {
				gameState.SetCastle(20, State.CastleState{ID: 20, KingdomID: 1, SlotType: 12, X: 200, Y: 200})
				return []string{"castles"}, true, nil
			})
			if err != nil {
				t.Fatal(err)
			}
			sender.beforeFinalDispatch = func(opcode string) error {
				if opcode != "adi" || mutated {
					return nil
				}
				mutated = true
				code := 0
				_, err := sender.pipeline.HandleFrame(t.Context(), Protocol.Frame{
					Opcode: "jaa", Direction: Protocol.DirectionInbound, ResponseCode: &code, ReceivedAt: time.Now().UTC(),
					Payload: json.RawMessage(`{"KID":1,"gca":{"A":[12,200,200,20,42,0,0,0,0,0,"Other Keep"]},"gui":{"I":[],"TU":[],"HI":[],"SHI":[]}}`),
				})
				return err
			}
		})
		if receipt.Status == Intent.StatusSucceeded || slices.Contains(sender.opcodes, "adi") || slices.Contains(sender.opcodes, "cra") {
			t.Fatalf("queued ADI crossed focus change: receipt=%+v opcodes=%v", receipt, sender.opcodes)
		}
	})

	t.Run("target unavailable at CRA transport", func(t *testing.T) {
		mutated := false
		receipt, sender, _ := runFortressEngineScenario(t, func(sender *fortressEngineSender, store *State.Store) {
			sender.beforeFinalDispatch = func(opcode string) error {
				if opcode != "cra" || mutated {
					return nil
				}
				mutated = true
				_, err := store.ApplyComponents(State.Components(State.ComponentAttackDialog), func(gameState *State.GameState) ([]string, bool, error) {
					gameState.AttackDialog.Target.TowerCooldownRemaining = 90
					return []string{"attack-dialog"}, true, nil
				})
				return err
			}
		})
		if receipt.Status == Intent.StatusSucceeded || !slices.Contains(sender.opcodes, "adi") || slices.Contains(sender.opcodes, "cra") {
			t.Fatalf("final CRA guard did not stop unavailable target: receipt=%+v opcodes=%v", receipt, sender.opcodes)
		}
	})
}

func TestFortressTargetVerificationUsesTransportResponseCorrelation(t *testing.T) {
	state := fortressIntentState(time.Now().UTC())
	source := state.Castles[10]
	source.Focused = true
	state.Castles[10] = source
	state.Session.ConnectionGeneration = 1
	store := State.NewStore(state)
	registry := Ingest.NewRegistry()
	if err := Ingest.RegisterCoreReducers(registry); err != nil {
		t.Fatal(err)
	}
	pipeline := Ingest.NewPipeline(store, nil, registry)
	application := &Application{State: store, Ingest: pipeline}
	arguments := json.RawMessage(`{"sourceCastleId":10,"kingdomId":1,"targetX":101,"targetY":100}`)
	arm := func(token string) context.Context {
		ctx := Outbound.WithMetadata(t.Context(), Outbound.Metadata{
			OperationID: "same-operation", ResponseToken: token, ConnectionGeneration: 1,
		})
		if err := application.armFortressTargetVerification(ctx, arguments); err != nil {
			t.Fatal(err)
		}
		return ctx
	}
	commit := func(token, causationOperationID string) {
		observed, err := pipeline.DecodeTransportFrameAt(
			`%xt%gaa%1%0%{"KID":1,"AI":[[11,101,100,-1,45,0,-1,0]]}%`,
			Protocol.DirectionInbound,
			time.Now().UTC(),
			token,
			causationOperationID,
		)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := pipeline.CommitFrame(t.Context(), observed); err != nil {
			t.Fatal(err)
		}
	}
	oldContext := arm("old-token")
	commit("old-token", "")
	if err := application.guardFortressTargetVerification(oldContext, arguments); err != nil {
		t.Fatalf("transport-realistic response did not authorize attempt: %v", err)
	}
	newContext := arm("new-token")
	commit("old-token", "")
	if err := application.guardFortressTargetVerification(newContext, arguments); !errors.Is(err, Intent.ErrPlanStale) {
		t.Fatalf("previous response token authorized retry: %v", err)
	}
	commit("", "")
	if err := application.guardFortressTargetVerification(newContext, arguments); !errors.Is(err, Intent.ErrPlanStale) {
		t.Fatalf("missing response token authorized retry: %v", err)
	}
	commit("new-token", "different-operation")
	if err := application.guardFortressTargetVerification(newContext, arguments); !errors.Is(err, Intent.ErrPlanStale) {
		t.Fatalf("conflicting inbound causation authorized retry: %v", err)
	}
	commit("new-token", "")
	if err := application.guardFortressTargetVerification(newContext, arguments); err != nil {
		t.Fatalf("current response token with empty inbound causation did not authorize retry: %v", err)
	}
}

func TestFortressPersonalCooldownSurvivesReadyGlobalMapRow(t *testing.T) {
	gameData := fortressIntentGameData(t)
	now := time.Now().UTC()
	gameState := fortressIntentState(now)
	gameState.TowerCooldowns["1:101:100"] = State.TowerCooldownState{
		KingdomID: 1, TargetTypeID: State.MapTypeKingdomFortress, X: 101, Y: 100,
		LastSuccessfulBattleAt: now.Add(-24 * time.Hour), CooldownObservedAt: now, CooldownRemaining: 0,
	}
	_, _, _, _, err := fortressAttackContext(Intent.PlanningContext{State: gameState, GameData: gameData}, json.RawMessage(`{
		"sourceCastleId":10,"kingdomId":1,"targetX":101,"targetY":100,
		"commanderIds":[5],"horseTravelBoostId":-1,"minimumCommanderSpeedBonus":100
	}`), now, true)
	if !errors.Is(err, Intent.ErrPlanStale) {
		t.Fatalf("personal five-day cooldown was not enforced: %v", err)
	}
}

func TestPlanFortressMapScanUsesOneAdaptiveFullMapAction(t *testing.T) {
	state := fortressIntentState(time.Now().UTC())
	source := state.Castles[10]
	source.Focused = true
	state.Castles[10] = source
	plan, err := planFortressMapScan(t.Context(), Intent.PlanningContext{State: state}, json.RawMessage(`{
		"sourceCastleId":10,"kingdomId":1
	}`))
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Steps) != 1 || plan.Steps[0].Action != "fortress.scan.full" || plan.Steps[0].Opcode != "" {
		t.Fatalf("Fortress full-map plan = %#v", plan.Steps)
	}
	var request map[string]any
	if err := json.Unmarshal(plan.Steps[0].ActionArguments, &request); err != nil {
		t.Fatal(err)
	}
	if _, retained := request["radius"]; retained {
		t.Fatalf("legacy Fortress radius leaked into the normalized request: %#v", request)
	}
	if request["scanStartedAt"] == nil || !strings.Contains(plan.Summary, "every fortress") {
		t.Fatalf("Fortress full-map metadata = %#v summary=%q", request, plan.Summary)
	}
}

func TestDiscoverFullFortressMapExpandsFarBeyondTheOldRadius(t *testing.T) {
	source := State.CastleState{X: 135, Y: 135}
	visited := map[fortressMapChunk]bool{}
	result, err := discoverFullFortressMap(t.Context(), source, func(_ context.Context, window towerMapWindow) (bool, error) {
		chunk := fortressMapChunk{X: window.X1 / fortressMapChunkSize, Y: window.Y1 / fortressMapChunkSize}
		visited[chunk] = true
		return chunk.Y == 1 && chunk.X >= 0 && chunk.X <= 6, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if !visited[fortressMapChunk{X: 6, Y: 1}] {
		t.Fatalf("full-map discovery never reached a connected chunk 405+ tiles from the source: %#v", visited)
	}
	if len(result.ContentWindows) != 7 || len(result.FailedChunks) != 0 {
		t.Fatalf("Fortress discovery result = %#v", result)
	}
	for _, window := range result.Windows {
		if width, height := window.X2-window.X1+1, window.Y2-window.Y1+1; width != fortressMapChunkSize || height != fortressMapChunkSize {
			t.Fatalf("Fortress GAA window is not %dx%d: %#v", fortressMapChunkSize, fortressMapChunkSize, window)
		}
	}
}

func TestDiscoverFullFortressMapReportsFailedChunkAsIncomplete(t *testing.T) {
	source := State.CastleState{X: 135, Y: 135}
	failed := fortressMapChunk{X: 2, Y: 1}
	result, err := discoverFullFortressMap(t.Context(), source, func(_ context.Context, window towerMapWindow) (bool, error) {
		chunk := fortressMapChunk{X: window.X1 / fortressMapChunkSize, Y: window.Y1 / fortressMapChunkSize}
		if chunk == failed {
			return false, errors.New("test timeout")
		}
		return chunk == (fortressMapChunk{X: 1, Y: 1}), nil
	})
	if err == nil || !strings.Contains(err.Error(), "incomplete") || len(result.FailedChunks) != 1 || result.FailedChunks[0] != failed {
		t.Fatalf("failed Fortress chunk result = %#v err=%v", result, err)
	}
}

func TestFortressMapWindowWaitsForCommittedCorrelatedResponse(t *testing.T) {
	observer := newStormMapBurstTestObserver()
	sender := &stormMapBurstTestSender{
		observer: observer, expectedSends: 1,
		responsePayload: json.RawMessage(`{"KID":1,"AI":[[11,45,46,0,45,0,0,1]]}`),
	}
	hasContent, err := runFortressMapGAAWindow(
		Outbound.WithMetadata(t.Context(), Outbound.Metadata{OperationID: "fortress-map-test"}),
		sender, observer, nil, 1,
		towerMapWindow{X1: 0, Y1: 0, X2: 89, Y2: 89},
		"fortress-map-test/fortress-gaa/1",
	)
	if err != nil {
		t.Fatal(err)
	}
	if !hasContent || len(sender.frames) != 1 || sender.frames[0].Opcode != "gaa" ||
		string(sender.frames[0].Payload) != `{"KID":1,"AX1":0,"AY1":0,"AX2":89,"AY2":89}` {
		t.Fatalf("Fortress GAA send = content:%t frames:%#v", hasContent, sender.frames)
	}
	if len(sender.metadata) != 1 || sender.metadata[0].ResponseToken != "fortress-map-test/fortress-gaa/1" ||
		sender.metadata[0].ResponseTimeoutMillis != int(fortressMapResponseTimeout/time.Millisecond) {
		t.Fatalf("Fortress GAA response metadata = %#v", sender.metadata)
	}
	observer.mu.Lock()
	waited := append([]uint64(nil), observer.waited...)
	observer.mu.Unlock()
	if len(waited) != 1 || waited[0] != 1 {
		t.Fatalf("Fortress GAA committed responses = %v", waited)
	}
}

func TestCaptureFullFortressMapRemovesOnlyStaleTargetsInsideScannedWindows(t *testing.T) {
	startedAt := time.Now().UTC()
	state := State.NewGameState()
	state.Castles[10] = State.CastleState{ID: 10, KingdomID: 1, SlotType: 12}
	state.Map[1] = map[string]State.MapObservation{
		"10:10":   {KingdomID: 1, X: 10, Y: 10, TypeID: State.MapTypeKingdomFortress, ObservedAt: startedAt.Add(-time.Hour)},
		"20:20":   {KingdomID: 1, X: 20, Y: 20, TypeID: State.MapTypeKingdomFortress, ObservedAt: startedAt.Add(time.Second)},
		"200:200": {KingdomID: 1, X: 200, Y: 200, TypeID: State.MapTypeKingdomFortress, ObservedAt: startedAt.Add(-time.Hour)},
	}
	application := &Application{State: State.NewStore(state)}
	if err := application.captureFullFortressMap(
		fortressMapRequest{SourceCastleID: 10, KingdomID: 1, ScanStartedAt: startedAt},
		[]towerMapWindow{{X1: 0, Y1: 0, X2: 89, Y2: 89}},
	); err != nil {
		t.Fatal(err)
	}
	view := application.State.ReadOnlyView()
	if _, exists := view.LookupMapObservation(1, "10:10"); exists {
		t.Fatal("stale Fortress survived a completed scan of its window")
	}
	if _, exists := view.LookupMapObservation(1, "20:20"); !exists {
		t.Fatal("fresh Fortress was removed from a completed scan window")
	}
	if _, exists := view.LookupMapObservation(1, "200:200"); !exists {
		t.Fatal("Fortress outside completed scan windows was removed")
	}
}

func fortressIntentGameData(t *testing.T) *GameData.Store {
	t.Helper()
	store, err := GameData.DecodeStore([]byte(`{
		"versionInfo":[],"buildings":[],
		"units":[{"wodID":277,"type":"Elitetinoswolves","name":"Eventunit","comment1":"Nomad Shop"}],
		"effects":[
			{"effectID":2106,"name":"relicSpeedBonus","effectTypeID":15,"capID":1006},
			{"effectID":426,"name":"speedBonus","effectTypeID":15,"capID":99}
		],
		"effectCaps":[{"capID":1006,"maxTotalBonus":100}],
		"globalEffects":[{"ID":10,"globalEffectID":2,"name":"SpeedBoost","effects":"426&60","boostValue":60}],
		"resources":[{"resourceID":2,"JSONKey":"C2","name":"Rubies"}]
	}`), GameData.SourceMetadata{ItemVersion: "test"})
	if err != nil {
		t.Fatal(err)
	}
	return store
}

func fortressIntentState(now time.Time) State.GameState {
	gameState := State.NewGameState()
	gameState.Castles[10] = State.CastleState{
		ID: 10, KingdomID: 1, SlotType: 12, X: 100, Y: 100,
		Units: State.CastleUnits{Stationed: map[State.UnitID]int64{GameData.DirewolfUnitID: 10_000}},
	}
	gameState.Commanders[5] = State.CommanderState{ID: 5, Available: true, Equipment: map[string]State.EquipmentInstanceID{"1": 5001}}
	gameState.Inventory.Equipment[5001] = State.EquipmentInstance{
		ID: 5001, Slot: 1, RarityID: 5, Effects: State.EquipmentEffects{
			{DefinitionID: 2106, Values: []float64{100}},
			{DefinitionID: 1, Values: []float64{1}},
			{DefinitionID: 2, Values: []float64{1}},
			{DefinitionID: 3, Values: []float64{1}},
		},
	}
	gameState.Map[1] = map[string]State.MapObservation{
		"101:100": {KingdomID: 1, X: 101, Y: 100, TypeID: State.MapTypeKingdomFortress, Level: 45, ObservedAt: now},
	}
	return gameState
}
