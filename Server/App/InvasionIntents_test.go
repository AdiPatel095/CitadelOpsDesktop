package App

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"CitadelDesktop/Server/AttackPresets"
	"CitadelDesktop/Server/GameData"
	"CitadelDesktop/Server/Intent"
	"CitadelDesktop/Server/State"
)

func TestInvasionAttackResolvesFreshLaneCapacityAndAcceptsCommanderZero(t *testing.T) {
	gameData, err := GameData.DecodeStore([]byte(`{
		"versionInfo":[],
		"units":[{"wodID":216},{"wodID":217}],
		"buildings":[],
		"effects":[
			{"effectID":2110,"name":"relicAttackUnitAmountFront","effectTypeID":34,"capID":1010},
			{"effectID":2807,"name":"relicAttackUnitAmountFrontPVP","effectTypeID":34,"capID":1707,"areaTypeID":"1,3,4,10,12,15,21"},
			{"effectID":700,"name":"attackUnitAmountReinforcementBonus","effectTypeID":179,"capID":99}
		],
		"legendskills":[{"skillID":106,"effectType":"additionalUnitAmountOnFront","totalEffectValue":25}],
		"effectCaps":[{"capID":99}]
	}`), GameData.SourceMetadata{ItemVersion: "test"})
	if err != nil {
		t.Fatal(err)
	}
	unitID, supportUnitID := int64(216), int64(217)
	now := time.Now().UTC()
	gameState := State.NewGameState()
	gameState.Castles[1] = State.CastleState{
		ID: 1, KingdomID: 0, X: 1164, Y: 1167,
		Units: State.CastleUnits{Stationed: map[State.UnitID]int64{216: 1_000, 217: 240}},
	}
	gameState.Commanders[0] = State.CommanderState{
		ID: 0, Available: true, Equipment: map[string]State.EquipmentInstanceID{"1": 1001, "2": 1003, "6": 1002},
	}
	gameState.Inventory.Equipment[1001] = State.EquipmentInstance{
		ID: 1001, Effects: State.EquipmentEffects{{DefinitionID: 2110, Values: []float64{45}}},
	}
	gameState.Inventory.Equipment[1002] = State.EquipmentInstance{
		ID: 1002, Effects: State.EquipmentEffects{{DefinitionID: 2807, Values: []float64{25.1}}},
	}
	gameState.Inventory.Equipment[1003] = State.EquipmentInstance{
		ID: 1003, Effects: State.EquipmentEffects{{DefinitionID: 700, Values: []float64{240}}},
	}
	gameState.Player.LegendSkills = State.LegendSkillState{ActiveIDs: []int64{106}, ObservedAt: now}
	gameState.Map[0] = map[string]State.MapObservation{
		"1165:1166": {KingdomID: 0, TypeID: 21, X: 1165, Y: 1166, ObjectID: 70, ObservedAt: now},
	}
	gameState.AttackDialog = State.AttackDialogState{
		SourceCastleID: 1, KingdomID: 0,
		Target:        State.AttackDialogTarget{TypeID: 21, X: 1165, Y: 1166, ObjectID: 70},
		ActiveEffects: []State.AttackDialogEffect{},
		ObservedAt:    now,
	}
	request := invasionAttackRequest{
		SourceCastleID: 1, EventID: 71, ScoreTarget: 5_000_000, MinimumRemainingSec: 1_800,
		TargetTypeID: 21, KingdomID: 0, TargetX: 1165, TargetY: 1166, TargetObjectID: 70,
		Preset: AttackPresets.Preset{
			Name: "Trial",
			Waves: []AttackPresets.Wave{{
				Middle: AttackPresets.Lane{Troops: []AttackPresets.Slot{{ItemID: &unitID, Quantity: 1_000}}},
			}},
			CourtyardSupport: AttackPresets.CourtyardSupport{
				Troops: []AttackPresets.Slot{{ItemID: &supportUnitID, Quantity: 1_000}},
			},
		},
	}
	arguments, _ := json.Marshal(request)
	plan, err := planInvasionAttack(context.Background(), Intent.PlanningContext{State: gameState, GameData: gameData}, arguments)
	if err != nil {
		t.Fatal(err)
	}
	var deferred *Intent.Step
	foundRefresh, foundTargetGuard, foundCapture := false, false, false
	for index := range plan.Steps {
		step := &plan.Steps[index]
		if step.Opcode == "gaa" {
			foundRefresh = true
		}
		if step.Action == "invasion.target.guard" {
			foundTargetGuard = true
		}
		if step.Action == "invasion.attack.capture" {
			foundCapture = true
		}
		if step.Resolver == "invasion.attack.build" {
			deferred = step
		}
	}
	if deferred == nil || deferred.CommandDependencies == nil || deferred.CommandDependencies.Opcode != "cra" {
		t.Fatalf("invasion attack did not defer formation building behind CRA send dependencies: %#v", plan.Steps)
	}
	if !foundRefresh || !foundTargetGuard || !foundCapture {
		t.Fatalf("invasion attack does not refresh, verify, and capture its target: %#v", plan.Steps)
	}
	resolved, err := (&Application{}).resolveInvasionAttackStep(
		context.Background(), Intent.PlanningContext{State: gameState, GameData: gameData}, deferred.ResolverArguments,
	)
	if err != nil {
		t.Fatal(err)
	}
	var body attackBody
	if err := json.Unmarshal(resolved.Command.Payload, &body); err != nil {
		t.Fatal(err)
	}
	if body.Leader != 0 || len(body.Waves) != 1 {
		t.Fatalf("unexpected invasion attack body: %#v", body)
	}
	if body.Waves[0].Middle.Units[0] != (attackPair{216, 375}) {
		t.Fatalf("front was not capped to the freshly resolved capacity: %#v", body.Waves[0].Middle.Units)
	}
	if len(body.SupportTroops) != 8 || body.SupportTroops[0] != (attackPair{217, 240}) {
		t.Fatalf("courtyard support was not capped to the freshly resolved capacity: %#v", body.SupportTroops)
	}
}

func TestInvasionAttackDoesNotPlanDuringPurchasedProtectionMode(t *testing.T) {
	now := time.Now().UTC()
	gameState := State.NewGameState()
	gameState.Player.ProtectionMode = State.PlayerProtectionModeState{
		ModeState: 1, RemainingSec: 3_600, ObservedAt: now,
	}
	_, err := planInvasionAttack(t.Context(), Intent.PlanningContext{State: gameState}, json.RawMessage(`{}`))
	if err == nil || err.Error() != "invasion attacks are disabled while Protection Mode is preparing or active" {
		t.Fatalf("protected invasion plan error = %v", err)
	}
}

func TestInvasionAttackUsesLiveEventFortificationCurrencies(t *testing.T) {
	gameState := State.NewGameState()
	gameState.Invasion.FortifyCurrencies = []string{"GTO", "STO", "ST"}
	gameState.Castles[1] = State.CastleState{ID: 1, KingdomID: 0}
	gameState.Map[0] = map[string]State.MapObservation{
		"206:937": {KingdomID: 0, TypeID: 34, X: 206, Y: 937},
	}
	request := invasionAttackRequest{
		SourceCastleID: 1, EventID: 103, ScoreTarget: 1, TargetTypeID: 34,
		KingdomID: 0, TargetX: 206, TargetY: 937, FortifyCurrency: "KM", HorseTravelBoostID: -1,
	}
	arguments, _ := json.Marshal(request)
	_, _, _, err := invasionAttackContext(Intent.PlanningContext{State: gameState}, arguments)
	if err == nil || err.Error() != "fortification currency KM is unavailable for event 103" {
		t.Fatalf("unsupported Bloodcrow fortification error = %v", err)
	}

	request.FortifyCurrency = "ST"
	arguments, _ = json.Marshal(request)
	resolved, _, _, err := invasionAttackContext(Intent.PlanningContext{State: gameState}, arguments)
	if err != nil || resolved.FortifyCurrency != "ST" {
		t.Fatalf("Samurai-token Bloodcrow fortification = %+v err=%v", resolved, err)
	}
}

func TestGuardInvasionTargetRequiresLaunchTimeMapObservation(t *testing.T) {
	now := time.Now().UTC()
	gameState := State.NewGameState()
	gameState.Castles[1] = State.CastleState{ID: 1, KingdomID: 0, X: 100, Y: 100}
	gameState.Commanders[7] = State.CommanderState{ID: 7, Available: true}
	gameState.EventScores.ActiveEventID = 71
	gameState.EventScores.ByEvent[71] = State.ScalableEventScore{
		EventID: 71, PlayerScore: 0, RemainingSec: 3_600, ObservedAt: now,
	}
	gameState.Map[0] = map[string]State.MapObservation{
		"101:100": {KingdomID: 0, TypeID: 21, X: 101, Y: 100, ObjectID: 70, ObservedAt: now},
	}
	request := resolvedInvasionAttackRequest{
		invasionAttackRequest: invasionAttackRequest{
			SourceCastleID: 1, EventID: 71, ScoreTarget: 1_000, MinimumRemainingSec: 60,
			TargetTypeID: 21, KingdomID: 0, TargetX: 101, TargetY: 100, TargetObjectID: 70,
		},
		CommanderID: 7,
	}
	arguments, _ := json.Marshal(invasionTargetVerificationRequest{Request: request, RefreshStartedAt: now.Add(time.Second)})
	application := &Application{State: State.NewStore(gameState)}
	if err := application.guardInvasionTarget(t.Context(), arguments); err == nil {
		t.Fatal("stale invasion target passed launch-time refresh guard")
	}

	target := gameState.Map[0]["101:100"]
	target.ObservedAt = now.Add(2 * time.Second)
	gameState.Map[0]["101:100"] = target
	application.State = State.NewStore(gameState)
	if err := application.guardInvasionTarget(t.Context(), arguments); err != nil {
		t.Fatalf("fresh invasion target failed launch-time refresh guard: %v", err)
	}
}

func TestCaptureInvasionLaunchTracksActiveEvent(t *testing.T) {
	now := time.Date(2026, 7, 21, 13, 45, 0, 0, time.UTC)
	arrivesAt := now.Add(90 * time.Second)
	commanderID := State.CommanderID(4)
	gameState := State.NewGameState()
	gameState.EventScores.ActiveEventID = 103
	gameState.EventScores.ByEvent[103] = State.ScalableEventScore{
		EventID: 103, RemainingSec: 7_200, ObservedAt: now,
	}
	gameState.Movements[88] = State.MovementState{
		ID: 88, Direction: 0, SourceCastleID: 1, CommanderID: &commanderID, KingdomID: 0,
		TargetTypeID: 34, TargetX: 120, TargetY: 121, ArrivesAt: &arrivesAt, ObservedAt: now,
	}
	application := &Application{State: State.NewStore(gameState)}
	arguments, _ := json.Marshal(resolvedInvasionAttackRequest{
		invasionAttackRequest: invasionAttackRequest{
			SourceCastleID: 1, EventID: 103, TargetTypeID: 34, KingdomID: 0, TargetX: 120, TargetY: 121,
		},
		CommanderID: commanderID,
	})
	if err := application.captureInvasionLaunch(t.Context(), arguments); err != nil {
		t.Fatal(err)
	}
	if err := application.captureInvasionLaunch(t.Context(), arguments); err != nil {
		t.Fatal(err)
	}
	activity := application.State.Snapshot().EventScores.ActivityByEvent[103]
	if activity.Invasion.Launches != 1 || len(activity.PendingAttacks) != 1 ||
		activity.PendingAttacks[0].Kind != State.EventActivityInvasion {
		t.Fatalf("unexpected Bloodcrow activity: %#v", activity)
	}
}
