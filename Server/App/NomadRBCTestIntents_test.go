package App

import (
	"encoding/json"
	"testing"
	"time"

	"CitadelDesktop/Server/AttackPresets"
	"CitadelDesktop/Server/GameData"
	"CitadelDesktop/Server/Intent"
	"CitadelDesktop/Server/State"
)

func TestNomadRBCTestBuildsImmediateResponseGatedChain(t *testing.T) {
	gameData, err := GameData.DecodeStore([]byte(`{
		"versionInfo":[],"buildings":[],"effects":[],"effectCaps":[],"equipment_sets":[],"generalSkills":[],
		"units":[{"wodID":77}]
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
	gameState.Commanders[1] = State.CommanderState{ID: 1, Available: true}
	gameState.Commanders[2] = State.CommanderState{ID: 2, Available: true}
	gameState.Map[0] = map[string]State.MapObservation{
		"101:102": {
			KingdomID: 0, TypeID: kingdomTowerMapTypeID, X: 101, Y: 102, Level: 81,
			TowerVictoryCount: 845, ObservedAt: now,
		},
	}
	unitID := int64(77)
	request := nomadRBCTestAttackRequest{
		RunID: "trial-1", BatchID: "batch-1", SourceCastleID: 1, KingdomID: 0, TargetX: 101, TargetY: 102,
		VictoryCount: 845, ExpectedAttacks: 2, CommanderIDs: []State.CommanderID{1, 2},
		Preset: AttackPresets.Preset{ID: "camp", Name: "Camp", Waves: []AttackPresets.Wave{{
			Middle: AttackPresets.Lane{Troops: []AttackPresets.Slot{{ItemID: &unitID, Quantity: 100}}},
		}}},
	}
	arguments, _ := json.Marshal(request)
	plan, err := planNomadRBCTestAttack(t.Context(), Intent.PlanningContext{State: gameState, GameData: gameData}, arguments)
	if err != nil {
		t.Fatal(err)
	}
	var launches []Intent.Step
	var delays []Intent.Step
	topLevelSetup := 0
	for _, step := range plan.Steps {
		if step.Resolver == "nomad.rbc_test.attack.build" {
			launches = append(launches, step)
		}
		if step.DelayMillis > 0 {
			delays = append(delays, step)
		}
		if step.Opcode == "gbl" || step.Opcode == "adi" || step.Opcode == "gas" {
			topLevelSetup++
		}
	}
	if len(launches) != 2 {
		t.Fatalf("unexpected RBC trial chain: %#v", launches)
	}
	if len(delays) != 0 {
		t.Fatalf("RBC trial added an artificial chain delay: %#v", delays)
	}
	if topLevelSetup != 0 {
		t.Fatalf("RBC trial still owns %d CRA setup command(s)", topLevelSetup)
	}
	for _, launch := range launches {
		if launch.CommandDependencies == nil || launch.CommandDependencies.Opcode != "cra" {
			t.Fatalf("RBC CRA does not declare sender-owned dependencies: %#v", launch)
		}
	}
	gameState.TowerCooldowns["0:101:102"] = State.TowerCooldownState{
		KingdomID: 0, X: 101, Y: 102, LastSuccessfulBattleAt: now, PendingCooldownRefresh: true,
	}
	if _, err := planNomadRBCTestAttack(t.Context(), Intent.PlanningContext{State: gameState, GameData: gameData}, arguments); err == nil {
		t.Fatal("RBC trial planned while the target was awaiting a post-victory cooldown refresh")
	}
	delete(gameState.TowerCooldowns, "0:101:102")
	gameState.NomadCamps.RBCTest = &State.NomadRBCTestState{
		RunID: "trial-1", SourceCastleID: 1, KingdomID: 0, TargetX: 101, TargetY: 102,
		ExpectedAttacks: 2, AttacksLaunched: 2, VictoriesConfirmed: 1, CooldownsSkipped: 1,
	}
	request.BatchID = "batch-2"
	request.ExpectedAttacks = 1
	request.CommanderIDs = []State.CommanderID{1}
	arguments, _ = json.Marshal(request)
	if _, err := planNomadRBCTestAttack(t.Context(), Intent.PlanningContext{State: gameState, GameData: gameData}, arguments); err != nil {
		t.Fatalf("RBC trial rejected an opportunistic refill batch: %v", err)
	}
}

func TestCaptureNomadRBCTestLaunchAllowsCommanderReuseInANewBatch(t *testing.T) {
	now := time.Now().UTC()
	commanderID := State.CommanderID(1)
	gameState := State.NewGameState()
	gameState.NomadCamps.RBCTest = &State.NomadRBCTestState{
		RunID: "trial-1", SourceCastleID: 1, KingdomID: 0, TargetX: 101, TargetY: 102,
		Launches: []State.NomadRBCTestLaunch{}, StartedAt: now,
	}
	gameState.Movements[1] = State.MovementState{
		ID: 1, Direction: 0, SourceCastleID: 1, KingdomID: 0, TargetX: 101, TargetY: 102,
		CommanderID: &commanderID, ArrivesAt: timePointer(now.Add(time.Minute)),
	}
	application := &Application{State: State.NewStore(gameState)}
	first, _ := json.Marshal(nomadRBCTestLaunchCapture{RunID: "trial-1", BatchID: "batch-1", CommanderID: commanderID})
	if err := application.captureNomadRBCTestLaunch(t.Context(), first); err != nil {
		t.Fatal(err)
	}
	_, _ = application.State.Apply(func(state *State.GameState) ([]string, bool, error) {
		delete(state.Movements, 1)
		state.Movements[2] = State.MovementState{
			ID: 2, Direction: 0, SourceCastleID: 1, KingdomID: 0, TargetX: 101, TargetY: 102,
			CommanderID: &commanderID, ArrivesAt: timePointer(now.Add(2 * time.Minute)),
		}
		return []string{"movements"}, true, nil
	})
	second, _ := json.Marshal(nomadRBCTestLaunchCapture{RunID: "trial-1", BatchID: "batch-2", CommanderID: commanderID})
	if err := application.captureNomadRBCTestLaunch(t.Context(), second); err != nil {
		t.Fatal(err)
	}
	test := application.State.Snapshot().NomadCamps.RBCTest
	if test.AttacksLaunched != 2 || test.ExpectedAttacks != 2 || len(test.Launches) != 2 || test.Launches[1].BatchID != "batch-2" {
		t.Fatalf("reused commander launch was not captured cumulatively: %#v", test)
	}
}

func timePointer(value time.Time) *time.Time { return &value }
