package App

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"CitadelDesktop/Server/AttackPresets"
	"CitadelDesktop/Server/GameData"
	"CitadelDesktop/Server/Intent"
	"CitadelDesktop/Server/Outbound"
	"CitadelDesktop/Server/Protocol"
	"CitadelDesktop/Server/State"
)

const nomadCooldownSkipCatalog = `{
		"versionInfo":[],
		"buildings":[],
		"units":[],
		"resources":[{"resourceID":2,"JSONKey":"C2"}],
		"eventAutoScalingCamps":[{
			"eventAutoScalingCampID":5001,"eventID":80,"difficultyID":201,"areaType":29,
			"camplevel":90,"countVictory":9,"coolDown":3600,"skipCosts":9950,"maxTroopCapacityDefense":620
		}]
	}`

func TestNomadCooldownSkipUsesLockedTargetAndOfficialRubyCeiling(t *testing.T) {
	gameData, err := GameData.DecodeStore([]byte(nomadCooldownSkipCatalog), GameData.SourceMetadata{ItemVersion: "test"})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	gameState := State.NewGameState()
	gameState.Player.Resources[2] = 20_000
	gameState.Map[0] = map[string]State.MapObservation{
		"101:102": {
			KingdomID: 0, X: 101, Y: 102, TypeID: 29, EventCampID: 5001,
			EventCampVictoryCount: 9, EventCampCooldownRemaining: 3_600, ObservedAt: now,
		},
	}
	gameState.NomadCamps.LockedTarget = &State.NomadCampTargetState{
		SourceCastleID: 1, EventID: 80, DifficultyID: 201, KingdomID: 0,
		TypeID: 29, X: 101, Y: 102, EventCampID: 5001,
	}
	request := nomadCooldownSkipRequest{
		nomadTargetRequest: nomadTargetRequest{
			SourceCastleID: 1, EventID: 80, DifficultyID: 201, KingdomID: 0,
			TargetTypeID: 29, TargetX: 101, TargetY: 102, EventCampID: 5001,
		},
		MaximumRubyCost: 10_000, MinimumRubyReserve: 5_000,
	}
	arguments, _ := json.Marshal(request)
	plan, err := planNomadCooldownSkip(t.Context(), Intent.PlanningContext{State: gameState, GameData: gameData}, arguments)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Steps) != 3 || plan.Steps[0].Action != "nomad.cooldown.guard" ||
		plan.Steps[1].Opcode != "sdc" || plan.Steps[1].AwaitOpcode != "sdc" ||
		plan.Steps[1].FinalDispatchAction != "nomad.cooldown.guard" ||
		plan.Steps[2].Action != "nomad.cooldown.verify" {
		t.Fatalf("unexpected guarded cooldown plan: %#v", plan.Steps)
	}
	var payload struct {
		KingdomID State.KingdomID `json:"KID"`
		X         int             `json:"X"`
		Y         int             `json:"Y"`
		MapID     int             `json:"MID"`
		NodeID    int             `json:"NID"`
	}
	if err := json.Unmarshal(plan.Steps[1].Command.Payload, &payload); err != nil {
		t.Fatal(err)
	}
	if payload.KingdomID != 0 || payload.X != 101 || payload.Y != 102 || payload.MapID != -1 || payload.NodeID != -1 {
		t.Fatalf("unexpected sdc payload: %#v", payload)
	}
	ceilingRequest := request
	ceilingRequest.MaximumRubyCost = 9_000
	ceilingArguments, _ := json.Marshal(ceilingRequest)
	if _, err := planNomadCooldownSkip(
		t.Context(), Intent.PlanningContext{State: gameState, GameData: gameData}, ceilingArguments,
	); err == nil || !strings.Contains(err.Error(), "above configured cap") {
		t.Fatalf("cooldown reset ignored the configured ruby ceiling: %v", err)
	}
	application := &Application{
		State: State.NewStore(gameState), GameData: appTestGameDataManagerFromCatalog(t, nomadCooldownSkipCatalog),
	}
	pending := gameState
	pending.NomadCamps.Cooldowns["0:101:102"] = State.NomadCampCooldownState{
		KingdomID: 0, X: 101, Y: 102, LastSuccessfulBattleAt: now.Add(time.Second), PendingCooldownRefresh: true,
	}
	application.State = State.NewStore(pending)
	if err := application.guardNomadCooldownSkip(t.Context(), plan.Steps[1].FinalDispatchArguments); !errors.Is(err, Intent.ErrPlanStale) {
		t.Fatalf("pending post-victory cooldown reached SDC dispatch: %v", err)
	}
	cleared := gameState
	observation := cleared.Map[0]["101:102"]
	observation.EventCampCooldownRemaining = 0
	observation.ObservedAt = now.Add(2 * time.Second)
	cleared.Map[0]["101:102"] = observation
	cleared.NomadCamps.Cooldowns["0:101:102"] = State.NomadCampCooldownState{
		KingdomID: 0, X: 101, Y: 102, LastSuccessfulBattleAt: now.Add(time.Second),
		CooldownObservedAt: observation.ObservedAt,
	}
	application.State = State.NewStore(cleared)
	if err := application.guardNomadCooldownSkip(t.Context(), plan.Steps[1].FinalDispatchArguments); !errors.Is(err, Intent.ErrPlanStale) {
		t.Fatalf("clear camp reached duplicate SDC dispatch: %v", err)
	}

}

func TestNomadChainLaunchesClearedCampWithoutSpeculativeCooldownSkips(t *testing.T) {
	gameData, err := GameData.DecodeStore([]byte(`{
		"versionInfo":[],
		"buildings":[],
		"units":[
			{"wodID":77},
			{"wodID":244,"name":"Eventtool","type":"EmperorKhanChest","comment2":"EmperorKhanChest","slotTypes":"1","allowedToAttack":"0+27#0+35","usageEventID":"5,72"}
		],
		"effects":[],
		"currencies":[
			{"currencyID":1001,"JSONKey":"MS1"},{"currencyID":1002,"JSONKey":"MS2"},
			{"currencyID":1003,"JSONKey":"MS3"},{"currencyID":1004,"JSONKey":"MS4"},
			{"currencyID":1005,"JSONKey":"MS5"},{"currencyID":1006,"JSONKey":"MS6"},
			{"currencyID":1007,"JSONKey":"MS7"}
		],
		"currencyMinutesSkipValues":[
			{"currencyID":"1001","MinutesSkipValue":"1"},{"currencyID":"1002","MinutesSkipValue":"5"},
			{"currencyID":"1003","MinutesSkipValue":"10"},{"currencyID":"1004","MinutesSkipValue":"30"},
			{"currencyID":"1005","MinutesSkipValue":"60"},{"currencyID":"1006","MinutesSkipValue":"300"},
			{"currencyID":"1007","MinutesSkipValue":"1440"}
		],
		"eventAutoScalingCamps":[{
			"eventAutoScalingCampID":5001,"eventID":80,"difficultyID":201,"areaType":29,
			"camplevel":90,"countVictory":9,"coolDown":3600,"skipCosts":9950,"maxTroopCapacityDefense":620
		}]
	}`), GameData.SourceMetadata{ItemVersion: "test"})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	gameState := State.NewGameState()
	gameState.Castles[1] = State.CastleState{
		ID: 1, KingdomID: 0, X: 100, Y: 100, Focused: true,
		Units: State.CastleUnits{Stationed: map[State.UnitID]int64{77: 1_000, 244: 100}},
	}
	for commanderID := State.CommanderID(1); commanderID <= 3; commanderID++ {
		gameState.Commanders[commanderID] = State.CommanderState{ID: commanderID, Available: true}
	}
	gameState.Player.LegendSkills.ObservedAt = now
	gameState.Player.Currencies[1005] = 3
	gameState.DailyAttacks = State.DailyAttackState{Count: 0, ObservedAt: now}
	gameState.EventScores.ByEvent[80] = State.ScalableEventScore{
		EventID: 80, DifficultyID: 201, PlayerScore: 100, RemainingSec: 7_200, ObservedAt: now,
	}
	gameState.NomadCamps.LastScannedAt[1] = now
	gameState.Map[0] = map[string]State.MapObservation{}
	for _, coordinate := range [][2]int{{99, 100}, {100, 99}, {100, 101}, {101, 100}} {
		observation := State.MapObservation{
			KingdomID: 0, X: coordinate[0], Y: coordinate[1], TypeID: 29,
			ObjectID: 5001, EventCampID: 5001, EventCampVictoryCount: 9, ObservedAt: now,
		}
		gameState.Map[0][fmt.Sprintf("%d:%d", coordinate[0], coordinate[1])] = observation
	}
	gameState.NomadCamps.LockedTarget = &State.NomadCampTargetState{
		SourceCastleID: 1, EventID: 80, DifficultyID: 201, KingdomID: 0,
		TypeID: 29, X: 101, Y: 100, EventCampID: 5001,
	}
	unitID := int64(77)
	request := nomadCampAttackRequest{
		nomadTargetRequest: nomadTargetRequest{
			SourceCastleID: 1, EventID: 80, DifficultyID: 201, KingdomID: 0,
			TargetTypeID: 29, TargetX: 101, TargetY: 100, EventCampID: 5001,
		},
		Mode: "chain", ScoreTarget: 100_000, MinimumRemainingSec: 1_800, VictoryCount: 9,
		SkipCooldowns: true, TimeSkipReserve: map[string]int64{}, DailyAttackLimit: 100,
		CommanderIDs: []State.CommanderID{1, 2, 3},
		Preset: AttackPresets.Preset{ID: "camp", Name: "Camp", Waves: []AttackPresets.Wave{{
			Middle: AttackPresets.Lane{Troops: []AttackPresets.Slot{{ItemID: &unitID, Quantity: 100}}},
		}}},
	}
	arguments, _ := json.Marshal(request)
	plan, err := planNomadCampAttack(t.Context(), Intent.PlanningContext{State: gameState, GameData: gameData}, arguments)
	if err != nil {
		t.Fatal(err)
	}
	var launches []Intent.Step
	var delays []Intent.Step
	var arrivalGuards []Intent.Step
	consumeSteps := 0
	topLevelSetup := 0
	for _, step := range plan.Steps {
		if step.Resolver == "nomad.attack.build" {
			launches = append(launches, step)
		}
		if step.Opcode == "msd" {
			t.Fatalf("cleared-camp attack chain emitted a speculative MSD: %#v", step)
		}
		if step.Action == timeSkipConsumeAction {
			consumeSteps++
		}
		if step.DelayMillis > 0 {
			delays = append(delays, step)
		}
		if step.Action == "nomad.attack.arrival.guard" {
			arrivalGuards = append(arrivalGuards, step)
		}
		if step.Opcode == "gbl" || step.Opcode == "adi" || step.Opcode == "gas" {
			topLevelSetup++
		}
	}
	if len(launches) != 3 {
		t.Fatalf("unexpected response-gated chain: %#v", launches)
	}
	if consumeSteps != 0 {
		t.Fatalf("cleared-camp attack chain speculatively debited %d time skips", consumeSteps)
	}
	if len(delays) != 0 {
		t.Fatalf("chain added an artificial send delay: %#v", delays)
	}
	if len(arrivalGuards) != 2 {
		t.Fatalf("expected every later launch to verify its authoritative arrival: %#v", arrivalGuards)
	}
	if topLevelSetup != 0 {
		t.Fatalf("Auto Nomad still owns %d CRA setup command(s)", topLevelSetup)
	}
	gameState.AttackDialog = State.AttackDialogState{
		SourceCastleID: 1, KingdomID: 0, ObservedAt: now,
		Target: State.AttackDialogTarget{
			TypeID: 29, X: 101, Y: 100, ObjectID: 5001, EventCampID: 5001, EventCampVictoryCount: 9,
		},
	}
	for _, launch := range launches {
		if launch.CommandDependencies == nil || launch.CommandDependencies.Opcode != "cra" {
			t.Fatalf("Nomad CRA does not declare sender-owned dependencies: %#v", launch)
		}
		concrete, err := (&Application{}).resolveNomadCampAttackStep(
			t.Context(), Intent.PlanningContext{State: gameState, GameData: gameData}, launch.ResolverArguments,
		)
		if err != nil {
			t.Fatalf("resolve concrete Nomad CRA: %v", err)
		}
		if concrete.PreDispatchAction != "nomad.attack.guard" || string(concrete.PreDispatchArguments) != string(launch.ResolverArguments) ||
			concrete.FinalDispatchAction != "nomad.attack.guard" || string(concrete.FinalDispatchArguments) != string(launch.ResolverArguments) {
			t.Fatalf("concrete Nomad CRA is missing its dispatch-time daily-limit guard: %#v", concrete)
		}
		dependencies, err := (&Application{}).resolveCRACommandDependencies(
			t.Context(), Intent.PlanningContext{State: gameState}, Intent.Step{
				Command: Protocol.Command{Opcode: "cra", Payload: launch.CommandDependencies.Payload},
			},
		)
		if err != nil {
			t.Fatalf("resolve Nomad CRA dependencies: %v", err)
		}
		adiCount, gaaCount, msdCount := 0, 0, 0
		for _, dependency := range dependencies.Steps {
			switch dependency.Opcode {
			case "adi":
				adiCount++
				if dependency.FinalDispatchAction != "nomad.attack.sequential_arrival.guard" {
					t.Fatalf("ADI lacks sequential-arrival final guard: %#v", dependency)
				}
			case "gaa":
				gaaCount++
			case "msd":
				msdCount++
			}
		}
		if adiCount != 1 || gaaCount != 0 || msdCount != 0 {
			t.Fatalf("safe CRA dependency wire counts: ADI=%d GAA=%d MSD=%d", adiCount, gaaCount, msdCount)
		}
		blockedState := gameState
		blockedState.DailyAttacks.Count = 100
		if err := (&Application{State: State.NewStore(blockedState)}).guardNomadCampAttack(
			t.Context(), concrete.PreDispatchArguments,
		); !errors.Is(err, Intent.ErrPlanStale) || !strings.Contains(err.Error(), "100 / 100") {
			t.Fatalf("concrete Nomad CRA guard accepted reached daily limit: %v", err)
		}
	}
	finalStep, err := (&Application{}).resolveNomadCampAttackStep(
		t.Context(), Intent.PlanningContext{State: gameState, GameData: gameData}, launches[0].ResolverArguments,
	)
	if err != nil {
		t.Fatal(err)
	}
	finalStep.AwaitOpcode = ""
	finalStep.SuccessCodes = nil
	stateStore := State.NewStore(gameState)
	manager := appTestGameDataManagerFromCatalog(t, nomadCooldownSkipCatalog)
	sender := &nomadFinalDispatchSender{}
	registry := Intent.NewRegistry()
	if err := registry.Register(Intent.Definition{
		Name: "test.nomad.final_dispatch", Effect: Intent.EffectLaunch,
		Planner: func(context.Context, Intent.PlanningContext, json.RawMessage) (Intent.Plan, error) {
			return Intent.Plan{Summary: "test Nomad final dispatch", Steps: []Intent.Step{finalStep}}, nil
		},
	}); err != nil {
		t.Fatal(err)
	}
	engine := Intent.NewEngine(registry, stateStore, manager, sender, nil)
	applicationAtDispatch := &Application{State: stateStore, GameData: manager, Intents: engine}
	if err := engine.RegisterAction("nomad.attack.guard", applicationAtDispatch.guardNomadCampAttack); err != nil {
		t.Fatal(err)
	}
	if err := applicationAtDispatch.guardNomadCampAttack(t.Context(), finalStep.PreDispatchArguments); err != nil {
		t.Fatalf("initial Nomad dispatch guard did not pass before queued mutation: %v", err)
	}
	finalHooks := 0
	sender.beforeFinal = func() {
		finalHooks++
		arrival := time.Now().UTC().Add(time.Second)
		_, applyErr := stateStore.ApplyComponents(
			State.Components(State.ComponentEventScores, State.ComponentMovements),
			func(current *State.GameState) ([]string, bool, error) {
				current.SetMovement(999, State.MovementState{
					ID: 999, Direction: 0, SourceCastleID: 1, KingdomID: 0, TargetTypeID: samuraiIntentCampTypeID,
					TargetX: 101, TargetY: 100, ArrivesAt: &arrival, ObservedAt: time.Now().UTC(),
				})
				changed := State.RecordEventAttackLaunch(current, samuraiIntentEventID, State.EventAttackRecord{
					MovementID: 999, Kind: State.EventActivityCamp, KingdomID: 0, TargetTypeID: samuraiIntentCampTypeID,
					TargetX: 101, TargetY: 100, LaunchedAt: time.Now().UTC(), ArrivesAt: arrival,
				})
				return []string{"event-scores", "movements"}, changed, nil
			},
		)
		if applyErr != nil {
			t.Errorf("stage final-dispatch race: %v", applyErr)
		}
	}
	receipt := engine.Submit(t.Context(), Intent.Request{Name: "test.nomad.final_dispatch", Arguments: json.RawMessage(`{}`)})
	if receipt.Status == Intent.StatusSucceeded || finalHooks != 1 || sender.sends != 0 ||
		!strings.Contains(receipt.Error, Intent.ErrPlanStale.Error()) {
		t.Fatalf("final CRA guard missed newly pending arrival: receipt=%#v hooks=%d sends=%d", receipt, finalHooks, sender.sends)
	}
	var first, second, third resolvedNomadCampAttackRequest
	if err := json.Unmarshal(launches[0].ResolverArguments, &first); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(launches[1].ResolverArguments, &second); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(launches[2].ResolverArguments, &third); err != nil {
		t.Fatal(err)
	}
	arrival := now.Add(62 * time.Second)
	gameState.Movements[86115613] = State.MovementState{
		ID: 86115613, Direction: 0, SourceCastleID: 1, KingdomID: 0, TargetX: 101, TargetY: 100,
		CommanderID: &first.CommanderID, ArrivesAt: &arrival, ObservedAt: now.Add(time.Second),
	}
	application := &Application{State: State.NewStore(gameState)}
	if err := application.captureNomadCampLaunch(t.Context(), launches[0].ResolverArguments); err != nil {
		t.Fatalf("capture accepted 62-second camp movement: %v", err)
	}
	gameState = application.State.Snapshot()
	concrete, err := application.resolveNomadCampAttackStep(
		t.Context(), Intent.PlanningContext{State: gameState, GameData: gameData}, launches[1].ResolverArguments,
	)
	if err != nil || concrete.Opcode != "cra" || second.CommanderID == first.CommanderID {
		t.Fatalf("clear camp did not advance to the next distinct CRA after accepted movement: step=%#v err=%v", concrete, err)
	}
	gameState.NomadCamps.Cooldowns["0:101:100"] = State.NomadCampCooldownState{
		KingdomID: 0, X: 101, Y: 100, LastSuccessfulBattleAt: now.Add(63 * time.Second), PendingCooldownRefresh: true,
	}
	if _, err := application.resolveNomadCampAttackStep(
		t.Context(), Intent.PlanningContext{State: gameState, GameData: gameData}, launches[2].ResolverArguments,
	); !errors.Is(err, Intent.ErrPlanStale) || third.CommanderID == first.CommanderID {
		t.Fatalf("pending post-victory cooldown did not yield before the next distinct CRA: %v", err)
	}
	gameState.NomadCamps.Cooldowns["0:101:100"] = State.NomadCampCooldownState{
		KingdomID: 0, X: 101, Y: 100, LastSuccessfulBattleAt: now.Add(63 * time.Second),
		CooldownRemaining: 3_600, CooldownObservedAt: time.Now().UTC().Add(time.Second),
	}
	if _, err := application.resolveNomadCampAttackStep(
		t.Context(), Intent.PlanningContext{State: gameState, GameData: gameData}, launches[2].ResolverArguments,
	); !errors.Is(err, Intent.ErrPlanStale) || !strings.Contains(err.Error(), "on cooldown") {
		t.Fatalf("positive refreshed cooldown did not yield before the next CRA: %v", err)
	}
	delete(gameState.NomadCamps.Cooldowns, "0:101:100")
	delete(gameState.Movements, 86115613)
	for commanderID := State.CommanderID(1); commanderID <= 3; commanderID++ {
		commander := gameState.Commanders[commanderID]
		commander.Available = true
		gameState.Commanders[commanderID] = commander
	}
	withoutSkips := request
	withoutSkips.SkipCooldowns = false
	withoutSkipArguments, _ := json.Marshal(withoutSkips)
	if _, err := planNomadCampAttack(
		t.Context(), Intent.PlanningContext{State: gameState, GameData: gameData}, withoutSkipArguments,
	); err == nil || !strings.Contains(err.Error(), "require cooldown time skips") {
		t.Fatalf("unsafe no-skip chain error = %v", err)
	}
	insufficientCapacity := request
	gameState.Player.Currencies[1005] = 2
	insufficientArguments, _ := json.Marshal(insufficientCapacity)
	if _, err := planNomadCampAttack(
		t.Context(), Intent.PlanningContext{State: gameState, GameData: gameData}, insufficientArguments,
	); err == nil || !strings.Contains(err.Error(), "cannot cover committed attack 3 of 3") {
		t.Fatalf("direct chain caller bypassed committed cooldown capacity: %v", err)
	}
	gameState.Player.Currencies[1005] = 3
	khanChestID := int64(244)
	incompatible := request
	incompatible.Preset = AttackPresets.Preset{ID: "sami", Name: "Sami's", Waves: []AttackPresets.Wave{{
		Middle: AttackPresets.Lane{
			Troops: []AttackPresets.Slot{{ItemID: &unitID, Quantity: 100}},
			Tools:  []AttackPresets.Slot{{ItemID: &khanChestID, Quantity: 10}},
		},
	}}}
	incompatibleArguments, _ := json.Marshal(incompatible)
	if _, err := planNomadCampAttack(
		t.Context(), Intent.PlanningContext{State: gameState, GameData: gameData}, incompatibleArguments,
	); err == nil || !strings.Contains(err.Error(), "Emperor Khan Chest") || !strings.Contains(err.Error(), "Samurai camps") {
		t.Fatalf("authoritative incompatible-tool error = %v", err)
	}
	gameState.NomadCamps.Cooldowns["0:101:100"] = State.NomadCampCooldownState{
		KingdomID: 0, X: 101, Y: 100, LastSuccessfulBattleAt: now, PendingCooldownRefresh: true,
	}
	stalePlan, err := planNomadCampAttack(t.Context(), Intent.PlanningContext{State: gameState, GameData: gameData}, arguments)
	if err != nil || len(stalePlan.Steps) != 0 {
		t.Fatalf("pending post-victory refresh did not yield for a safe replan: plan=%#v err=%v", stalePlan, err)
	}
}

type nomadFinalDispatchSender struct {
	beforeFinal func()
	sends       int
}

func (*nomadFinalDispatchSender) Ready() bool       { return true }
func (*nomadFinalDispatchSender) Namespace() string { return "EmpireEx_21" }
func (sender *nomadFinalDispatchSender) Send(ctx context.Context, _ []byte) error {
	if sender.beforeFinal != nil {
		sender.beforeFinal()
	}
	if err := Outbound.ValidateFinalDispatch(ctx); err != nil {
		return err
	}
	sender.sends++
	return nil
}

func TestNomadLevelSelectsOneAvailableCommanderFromCandidatePool(t *testing.T) {
	gameData, err := GameData.DecodeStore([]byte(`{
		"versionInfo":[],
		"buildings":[],
		"units":[{"wodID":77}],
		"effects":[],
		"eventAutoScalingCamps":[
			{"eventAutoScalingCampID":5000,"eventID":80,"difficultyID":201,"areaType":29,
			 "camplevel":80,"countVictory":8,"coolDown":0,"skipCosts":0,"maxTroopCapacityDefense":500},
			{"eventAutoScalingCampID":5001,"eventID":80,"difficultyID":201,"areaType":29,
			 "camplevel":90,"countVictory":9,"coolDown":3600,"skipCosts":9950,"maxTroopCapacityDefense":620}
		]
	}`), GameData.SourceMetadata{ItemVersion: "test"})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	gameState := State.NewGameState()
	gameState.Castles[1] = State.CastleState{
		ID: 1, KingdomID: 0, X: 100, Y: 100, Focused: true,
		Units: State.CastleUnits{Stationed: map[State.UnitID]int64{77: 200}},
	}
	gameState.Commanders[1] = State.CommanderState{ID: 1, Available: false}
	gameState.Commanders[2] = State.CommanderState{ID: 2, Available: true}
	gameState.Commanders[3] = State.CommanderState{ID: 3, Available: true}
	gameState.Player.LegendSkills.ObservedAt = now
	gameState.EventScores.ByEvent[80] = State.ScalableEventScore{
		EventID: 80, DifficultyID: 201, PlayerScore: 100, RemainingSec: 7_200, ObservedAt: now,
	}
	gameState.NomadCamps.LastScannedAt[1] = now
	gameState.Map[0] = map[string]State.MapObservation{}
	for _, coordinate := range [][2]int{{99, 100}, {100, 99}, {100, 101}, {101, 100}} {
		observation := State.MapObservation{
			KingdomID: 0, X: coordinate[0], Y: coordinate[1], TypeID: 29,
			ObjectID: 5000, EventCampID: 5000, EventCampVictoryCount: 8, ObservedAt: now,
		}
		gameState.Map[0][fmt.Sprintf("%d:%d", coordinate[0], coordinate[1])] = observation
	}
	unitID := int64(77)
	request := nomadCampAttackRequest{
		nomadTargetRequest: nomadTargetRequest{
			SourceCastleID: 1, EventID: 80, DifficultyID: 201, KingdomID: 0,
			TargetTypeID: 29, TargetX: 99, TargetY: 100, EventCampID: 5000,
		},
		Mode: "level", ScoreTarget: 100_000, MinimumRemainingSec: 1_800, VictoryCount: 8,
		CommanderIDs: []State.CommanderID{1, 2, 3},
		Preset: AttackPresets.Preset{ID: "camp", Name: "Camp", Waves: []AttackPresets.Wave{{
			Middle: AttackPresets.Lane{Troops: []AttackPresets.Slot{{ItemID: &unitID, Quantity: 300}}},
		}}},
	}
	arguments, _ := json.Marshal(request)
	plan, err := planNomadCampAttack(t.Context(), Intent.PlanningContext{State: gameState, GameData: gameData}, arguments)
	if err != nil {
		t.Fatal(err)
	}
	launchCount := 0
	for _, step := range plan.Steps {
		if step.Action == "nomad.attack.inventory.guard" {
			var guarded nomadCampAttackRequest
			if err := json.Unmarshal(step.ActionArguments, &guarded); err != nil {
				t.Fatal(err)
			}
			if len(guarded.CommanderIDs) != 1 || guarded.CommanderIDs[0] != 2 {
				t.Fatalf("guarded commanders = %v, want [2]", guarded.CommanderIDs)
			}
		}
		if step.Resolver == "nomad.attack.build" {
			launchCount++
			var resolved resolvedNomadCampAttackRequest
			if err := json.Unmarshal(step.ResolverArguments, &resolved); err != nil {
				t.Fatal(err)
			}
			if resolved.CommanderID != 2 {
				t.Fatalf("resolved commander = %d, want 2", resolved.CommanderID)
			}
		}
	}
	if launchCount != 1 {
		t.Fatalf("level plan launch count = %d, want 1", launchCount)
	}
	advanced := gameState.Map[0]["99:100"]
	advanced.ObjectID = 5001
	advanced.EventCampID = 5001
	advanced.EventCampVictoryCount = 9
	gameState.Map[0]["99:100"] = advanced
	stalePlan, err := planNomadCampAttack(t.Context(), Intent.PlanningContext{State: gameState, GameData: gameData}, arguments)
	if err != nil {
		t.Fatalf("stale level decision returned an error: %v", err)
	}
	if len(stalePlan.Steps) != 0 || stalePlan.Summary != "Nomad/Samurai camp progression changed; reevaluate the current camp state" {
		t.Fatalf("stale level decision plan = %#v", stalePlan)
	}
}

func TestNomadChainArrivalGuardRejectsOvertaking(t *testing.T) {
	previousCommander, currentCommander := State.CommanderID(1), State.CommanderID(2)
	previousArrival := time.Date(2026, 7, 14, 15, 0, 0, 0, time.UTC)
	currentArrival := previousArrival.Add(-time.Second)
	gameState := State.NewGameState()
	gameState.Movements[10] = State.MovementState{
		ID: 10, Direction: 0, SourceCastleID: 1, KingdomID: 0, TargetX: 101, TargetY: 100,
		CommanderID: &previousCommander, ArrivesAt: &previousArrival,
	}
	gameState.Movements[11] = State.MovementState{
		ID: 11, Direction: 0, SourceCastleID: 1, KingdomID: 0, TargetX: 101, TargetY: 100,
		CommanderID: &currentCommander, ArrivesAt: &currentArrival,
	}
	application := &Application{State: State.NewStore(gameState)}
	arguments, _ := json.Marshal(nomadChainArrivalGuard{
		SourceCastleID: 1, KingdomID: 0, TargetX: 101, TargetY: 100,
		PreviousCommander: previousCommander, CurrentCommander: currentCommander,
	})
	if err := application.guardNomadChainArrival(t.Context(), arguments); err == nil {
		t.Fatal("server-returned arrival overtake was accepted")
	}
}
