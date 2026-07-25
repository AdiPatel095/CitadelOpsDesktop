package App

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"CitadelDesktop/Server/AttackPresets"
	"CitadelDesktop/Server/GameData"
	"CitadelDesktop/Server/Intent"
	"CitadelDesktop/Server/State"
)

func TestKhanPresetShortageCancelsTheCRAAsStale(t *testing.T) {
	itemID := int64(215)
	preset := AttackPresets.Preset{
		Waves: []AttackPresets.Wave{{
			Middle: AttackPresets.Lane{
				Troops: []AttackPresets.Slot{{ItemID: &itemID, Quantity: 100}},
			},
		}},
	}
	source := State.CastleState{
		ID:    2,
		Units: State.CastleUnits{Stationed: map[State.UnitID]int64{215: 50}},
	}
	err := khanAttackPresetAvailability(preset, source)
	if !errors.Is(err, Intent.ErrPlanStale) ||
		!strings.Contains(err.Error(), "CRA launch cursor paused") {
		t.Fatalf("Khan preset shortage error = %v", err)
	}
}

func TestPlanKhanMapJumpUsesCapturedFNMCommand(t *testing.T) {
	now := time.Now().UTC()
	gameState := State.NewGameState()
	gameState.EventScores.ByEvent[khanEventID] = State.ScalableEventScore{
		EventID: khanEventID, RemainingSec: 7_200, ObservedAt: now,
	}
	plan, err := planKhanMapJump(t.Context(), Intent.PlanningContext{State: gameState}, json.RawMessage(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Claims) != 2 || plan.Claims[0] != "castle-focus" || plan.Claims[1] != "map:0" ||
		len(plan.Steps) != 1 || plan.Steps[0].Opcode != "fnm" || plan.Steps[0].AwaitOpcode != "fnm" {
		t.Fatalf("Khan map jump plan = %#v", plan)
	}
	var payload struct {
		TargetTypeID int             `json:"T"`
		KingdomID    State.KingdomID `json:"KID"`
		MinimumLevel int             `json:"LMIN"`
		MaximumLevel int             `json:"LMAX"`
		NPCID        int             `json:"NID"`
	}
	if err := json.Unmarshal(plan.Steps[0].Command.Payload, &payload); err != nil {
		t.Fatal(err)
	}
	if payload.TargetTypeID != 35 || payload.KingdomID != 0 ||
		payload.MinimumLevel != -1 || payload.MaximumLevel != -1 || payload.NPCID != -801 {
		t.Fatalf("Khan map jump payload = %#v", payload)
	}
}

func TestPlanKhanTauntUsesLTAAndAllowsParallelRetaliations(t *testing.T) {
	now := time.Now().UTC()
	gameData, err := GameData.DecodeStore([]byte(`{
		"versionInfo":[],
		"buildings":[{"wodID":1}],
		"units":[{"wodID":1}],
		"eventAutoScalingCamps":[{
			"eventAutoScalingCampID":"1147","eventID":"72","difficultyID":"310",
			"areaType":"35","camplevel":"107","playerRageCap":"1740"
		}]
	}`), GameData.SourceMetadata{ItemVersion: "test"})
	if err != nil {
		t.Fatal(err)
	}
	gameState := State.NewGameState()
	gameState.Castles[1] = State.CastleState{ID: 1, KingdomID: 0, SlotType: 1, X: 212, Y: 941}
	gameState.Map[0] = map[string]State.MapObservation{
		"939:1123": {KingdomID: 0, TypeID: khanCampTypeID, X: 939, Y: 1123, EventCampID: 1147},
	}
	gameState.EventScores.ByEvent[khanEventID] = State.ScalableEventScore{
		EventID: khanEventID, RemainingSec: 7_200, ObservedAt: now,
	}
	gameState.Khan = State.KhanState{
		RageCampID: 1147, PlayerRage: 1740, PlayerRageCap: 1740, PlayerTotalRage: 52140,
		RageObservedAt: now, TauntsTriggered: 1, LastTauntTriggeredAt: now.Add(-time.Minute),
		LastTauntTriggeredRage: 50400,
		Taunts: map[State.MovementID]State.KhanTauntState{
			90: {MovementID: 90, ObservedAt: now.Add(-time.Minute), ImpactAt: now.Add(time.Minute)},
		},
		Launches: []State.KhanLaunchState{},
	}
	arguments, _ := json.Marshal(khanTauntRequest{
		EventID: khanEventID, MainCastleID: 1, TargetX: 939, TargetY: 1123,
		RageCampID: 1147, PlayerTotalRage: 52140, RageObservedAt: now,
		KhanGuard: khanLaneGuardRequest{MainCastleID: 1},
	})
	plan, err := planKhanTaunt(t.Context(), Intent.PlanningContext{
		State: gameState, GameData: gameData,
	}, arguments)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Steps) != 3 || plan.Steps[0].Action != "khan.lane.guard" ||
		plan.Steps[1].Opcode != "lta" || plan.Steps[1].AwaitOpcode != "" ||
		string(plan.Steps[1].Command.Payload) != `{"AV":0,"EID":72}` ||
		plan.Steps[2].Action != "khan.taunt.dispatched" {
		t.Fatalf("Khan LTA plan = %#v", plan)
	}
	var guard khanLaneGuardActionRequest
	if err := decodeIntentArguments(plan.Steps[0].ActionArguments, &guard); err != nil ||
		guard.KhanGuard.MainCastleID != 1 {
		t.Fatalf("Khan LTA guard arguments = %#v, err=%v", guard, err)
	}

	gameState.Khan.LastTauntTriggeredRage = gameState.Khan.PlayerTotalRage
	_, err = planKhanTaunt(t.Context(), Intent.PlanningContext{
		State: gameState, GameData: gameData,
	}, arguments)
	if err == nil || !strings.Contains(err.Error(), "rage bar is no longer ready") {
		t.Fatalf("same rage fill plan error = %v", err)
	}
}

func TestCaptureKhanLaunchRecordsAndBlocksOvertakingArrival(t *testing.T) {
	now := time.Date(2026, 7, 14, 15, 0, 0, 0, time.UTC)
	previousArrival := now.Add(2 * time.Minute)
	currentArrival := now.Add(time.Minute)
	commanderID := State.CommanderID(2)
	gameState := State.NewGameState()
	gameState.Khan = State.KhanState{
		RunID: "run", SourceCastleID: 2, MainCastleID: 1, KingdomID: 0, TargetX: 210, TargetY: 942,
		AttacksLaunched: 1,
		Launches:        []State.KhanLaunchState{{CommanderID: 1, MovementID: 10, ArrivesAt: previousArrival}},
		Taunts:          map[State.MovementID]State.KhanTauntState{},
	}
	gameState.Movements[11] = State.MovementState{
		ID: 11, Direction: 0, SourceCastleID: 2, KingdomID: 0, TargetX: 210, TargetY: 942,
		CommanderID: &commanderID, ArrivesAt: &currentArrival, ObservedAt: now,
	}
	application := &Application{State: State.NewStore(gameState)}
	arguments, _ := json.Marshal(khanLaunchCapture{
		RunID: "run", SourceCastleID: 2, MainCastleID: 1, KingdomID: 0,
		TargetX: 210, TargetY: 942, CommanderID: commanderID,
	})
	err := application.captureKhanLaunch(t.Context(), arguments)
	if err == nil || !strings.Contains(err.Error(), "unsafe Khan chain arrival order") {
		t.Fatalf("capture error = %v", err)
	}
	khan := application.State.Snapshot().Khan
	if khan.SafetyError == "" || khan.AttacksLaunched != 2 || len(khan.Launches) != 2 {
		t.Fatalf("captured Khan state = %#v", khan)
	}
}

func TestPlanKhanPointLimitProtectionRecallsKhanLaunchAndOpensGates(t *testing.T) {
	now := time.Now().UTC()
	arrival := now.Add(time.Minute)
	gameState := State.NewGameState()
	gameState.Player.ID = 158
	gameState.Castles[1] = State.CastleState{ID: 1, KingdomID: 0, SlotType: 1}
	gameState.Movements[50] = State.MovementState{
		ID: 50, Direction: 0, OwnerPlayerID: 158, SourceCastleID: 1, ArrivesAt: &arrival,
	}
	gameState.Khan.Launches = []State.KhanLaunchState{{MovementID: 50}}
	gameState.EventScores.ByEvent[khanEventID] = State.ScalableEventScore{EventID: khanEventID, PlayerScore: 10_000}
	arguments := json.RawMessage(`{"castleId":1,"pointThreshold":10000}`)
	plan, err := planKhanPointLimitProtection(t.Context(), Intent.PlanningContext{State: gameState}, arguments)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Steps) != 2 || plan.Steps[0].Opcode != "mcm" || plan.Steps[1].Opcode != "mos" {
		t.Fatalf("point-limit steps = %#v", plan.Steps)
	}
}
