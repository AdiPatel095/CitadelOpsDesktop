package App

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"CitadelDesktop/Server/AttackPresets"
	"CitadelDesktop/Server/GameData"
	"CitadelDesktop/Server/Intent"
	"CitadelDesktop/Server/State"
)

func TestNomadCooldownSkipUsesLockedTargetAndOfficialRubyCeiling(t *testing.T) {
	gameData, err := GameData.DecodeStore([]byte(`{
		"versionInfo":[],
		"buildings":[],
		"units":[],
		"resources":[{"resourceID":2,"JSONKey":"C2"}],
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

	request.MaximumRubyCost = 9_000
	arguments, _ = json.Marshal(request)
	if _, err := planNomadCooldownSkip(t.Context(), Intent.PlanningContext{State: gameState, GameData: gameData}, arguments); err == nil {
		t.Fatal("cooldown reset ignored the configured ruby ceiling")
	}
}

func TestNomadChainDeclaresSendLevelCooldownDependencies(t *testing.T) {
	gameData, err := GameData.DecodeStore([]byte(`{
		"versionInfo":[],
		"buildings":[],
		"units":[{"wodID":77}],
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
		Units: State.CastleUnits{Stationed: map[State.UnitID]int64{77: 1_000}},
	}
	for commanderID := State.CommanderID(1); commanderID <= 3; commanderID++ {
		gameState.Commanders[commanderID] = State.CommanderState{ID: commanderID, Available: true}
	}
	gameState.Player.LegendSkills.ObservedAt = now
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
	topLevelSetup := 0
	for _, step := range plan.Steps {
		if step.Resolver == "nomad.attack.build" {
			launches = append(launches, step)
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
	if len(delays) != 0 {
		t.Fatalf("chain added an artificial send delay: %#v", delays)
	}
	if len(arrivalGuards) != 2 {
		t.Fatalf("expected every later launch to verify its authoritative arrival: %#v", arrivalGuards)
	}
	if topLevelSetup != 0 {
		t.Fatalf("Auto Nomad still owns %d CRA setup command(s)", topLevelSetup)
	}
	for _, launch := range launches {
		if launch.CommandDependencies == nil || launch.CommandDependencies.Opcode != "cra" {
			t.Fatalf("Nomad CRA does not declare sender-owned dependencies: %#v", launch)
		}
	}
	gameState.NomadCamps.Cooldowns["0:101:100"] = State.NomadCampCooldownState{
		KingdomID: 0, X: 101, Y: 100, LastSuccessfulBattleAt: now, PendingCooldownRefresh: true,
	}
	if _, err := planNomadCampAttack(t.Context(), Intent.PlanningContext{State: gameState, GameData: gameData}, arguments); err == nil {
		t.Fatal("chain planned while the camp was awaiting a post-victory cooldown refresh")
	}
}

func TestNomadLevelSelectsOneAvailableCommanderFromCandidatePool(t *testing.T) {
	gameData, err := GameData.DecodeStore([]byte(`{
		"versionInfo":[],
		"buildings":[],
		"units":[{"wodID":77}],
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
		Units: State.CastleUnits{Stationed: map[State.UnitID]int64{77: 1_000}},
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
			Middle: AttackPresets.Lane{Troops: []AttackPresets.Slot{{ItemID: &unitID, Quantity: 100}}},
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
