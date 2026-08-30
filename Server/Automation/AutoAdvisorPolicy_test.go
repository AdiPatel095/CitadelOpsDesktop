package Automation

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"CitadelDesktop/Server/Configuration"
	"CitadelDesktop/Server/GameData"
	"CitadelDesktop/Server/State"
)

func TestAutoAdvisorPolicySizesOneRunToCompletePresetInventory(t *testing.T) {
	now := time.Date(2026, 7, 19, 17, 0, 0, 0, time.UTC)
	snapshot := autoAdvisorPolicySnapshot(t, now)

	decision, err := NewAutoAdvisorPolicy().Evaluate(t.Context(), snapshot)
	if err != nil || decision.Request == nil || decision.Request.Name != "advisor.run.launch" {
		t.Fatalf("advisor launch decision: %#v err=%v", decision, err)
	}
	var launch struct {
		AttackCount int               `json:"attackCount"`
		CommanderID State.CommanderID `json:"commanderId"`
		TargetX     int               `json:"targetX"`
		TargetY     int               `json:"targetY"`
	}
	if err := json.Unmarshal(decision.Request.Arguments, &launch); err != nil {
		t.Fatal(err)
	}
	if launch.AttackCount != 3 || launch.CommanderID != 1 || launch.TargetX != 101 || launch.TargetY != 100 {
		t.Fatalf("advisor run was not conservatively sized to three complete preset copies: %#v", launch)
	}
	if decision.Metrics["inventoryCapacity"] != 3 || decision.Metrics["plannedAttacks"] != 3 {
		t.Fatalf("advisor inventory limit is not visible in policy metrics: %#v", decision.Metrics)
	}
	var launchArguments map[string]json.RawMessage
	if err := json.Unmarshal(decision.Request.Arguments, &launchArguments); err != nil {
		t.Fatal(err)
	}
	if _, configurable := launchArguments["cycleSeconds"]; configurable {
		t.Fatalf("advisor launch exposed an internal cycle estimate: %s", decision.Request.Arguments)
	}
}

func TestAutoAdvisorPolicyDoesNotRelaunchCancelledRun(t *testing.T) {
	now := time.Date(2026, 7, 19, 17, 0, 0, 0, time.UTC)
	snapshot := autoAdvisorPolicySnapshot(t, now)
	snapshot.State.Advisor.Run = &State.AdvisorRunState{
		EventID: 80, EventEndsAt: now.Add(7_200 * time.Second), RequestedAttacks: 12, CurrentAttack: 7, LaunchState: 1, Status: "cancelled",
		StartedAt: now.Add(-10 * time.Minute), LastAttackAt: now.Add(-time.Minute), UpdatedAt: now,
	}

	decision, err := NewAutoAdvisorPolicy().Evaluate(t.Context(), snapshot)
	if err != nil || decision.Request != nil || decision.Status != "idle" || !strings.Contains(decision.Detail, "will not be restarted") {
		t.Fatalf("cancelled advisor run was eligible for replacement: %#v err=%v", decision, err)
	}
}

func TestAutoAdvisorPolicyAllowsSameEventIDInANewOccurrence(t *testing.T) {
	now := time.Date(2026, 7, 19, 17, 0, 0, 0, time.UTC)
	snapshot := autoAdvisorPolicySnapshot(t, now)
	snapshot.State.Advisor.Run = &State.AdvisorRunState{
		EventID: 80, EventEndsAt: now.Add(-24 * time.Hour), RequestedAttacks: 12, CurrentAttack: 7,
		LaunchState: 1, Status: "cancelled", StartedAt: now.Add(-27 * time.Hour), UpdatedAt: now.Add(-24 * time.Hour),
	}

	decision, err := NewAutoAdvisorPolicy().Evaluate(t.Context(), snapshot)
	if err != nil || decision.Request == nil || decision.Request.Name != "advisor.run.launch" {
		t.Fatalf("an old occurrence blocked the new Samurai event: %#v err=%v", decision, err)
	}
}

func TestAutoAdvisorPolicyCapsRubyHorseRunByConfiguredRubyBudget(t *testing.T) {
	now := time.Date(2026, 7, 19, 17, 0, 0, 0, time.UTC)
	snapshot := autoAdvisorPolicySnapshot(t, now)
	snapshot.State.Player.Resources[2] = 250
	snapshot.Configuration.Sections["automation.autoAdvisor"] = json.RawMessage(`{
		"version":1,"sourceCastleId":1,"presetId":"camp","nomadDifficultyId":301,"samuraiDifficultyId":201,
		"maxAttackCount":10,"minimumRemainingSec":1800,"coinCostPerAttack":500,
		"minimumCoinReserve":0,"rubyCostPerAttack":100,"minimumRubyReserve":0,"minimumFeatherReserve":0,
		"timeSkipReserve":{},"checkIntervalSec":30,"mapRefreshIntervalSec":300,"horseTravelBoostId":1008
	}`)

	decision, err := NewAutoAdvisorPolicy().Evaluate(t.Context(), snapshot)
	if err != nil || decision.Request == nil || decision.Metrics["rubyCapacity"] != 2 || decision.Metrics["plannedAttacks"] != 2 {
		t.Fatalf("ruby horse run ignored its configured ruby ceiling: %#v err=%v", decision, err)
	}
}

func TestAutoAdvisorPolicyNeverConsumesActivationToken(t *testing.T) {
	now := time.Date(2026, 7, 19, 17, 0, 0, 0, time.UTC)
	snapshot := autoAdvisorPolicySnapshot(t, now)
	score := snapshot.State.EventScores.ByEvent[80]
	score.AdvisorActive = false
	snapshot.State.EventScores.ByEvent[80] = score
	snapshot.State.Player.Currencies[78] = 1

	decision, err := NewAutoAdvisorPolicy().Evaluate(t.Context(), snapshot)
	if err != nil || decision.Request != nil || decision.Status != "waiting" || !strings.Contains(decision.Detail, "explicit token confirmation") {
		t.Fatalf("locked advisor did not stop at the paid-token gate: %#v err=%v", decision, err)
	}
}

func autoAdvisorPolicySnapshot(t *testing.T, now time.Time) Snapshot {
	t.Helper()
	gameData, err := GameData.DecodeStore([]byte(`{
		"versionInfo":[],
		"buildings":[],
		"units":[{"wodID":77},{"wodID":88}],
		"resources":[{"resourceID":1,"JSONKey":"C1"},{"resourceID":2,"JSONKey":"C2"}],
		"currencies":[{"currencyID":22,"JSONKey":"PTT"}],
		"eventAutoScalingDifficulties":[{"difficultyID":201,"eventID":80,"difficultyTypeID":1,"isLocked":0}],
		"eventAutoScalingCamps":[
			{"eventAutoScalingCampID":5001,"eventID":80,"difficultyID":201,"areaType":29,"camplevel":90,"countVictory":9,"coolDown":3600,"skipCosts":9950,"maxTroopCapacityDefense":600}
		]
	}`), GameData.SourceMetadata{ItemVersion: "test"})
	if err != nil {
		t.Fatal(err)
	}
	gameState := State.NewGameState()
	gameState.Castles[1] = State.CastleState{
		ID: 1, Name: "Main", KingdomID: 0, X: 100, Y: 100,
		Units: State.CastleUnits{
			Stationed: map[State.UnitID]int64{77: 500, 88: 30}, Traveling: map[State.UnitID]int64{},
		},
	}
	gameState.Commanders[1] = State.CommanderState{ID: 1, Available: true}
	gameState.Player.Resources[1] = 100_000
	gameState.Player.Currencies[22] = 10
	gameState.Player.Currencies[1005] = 10
	gameState.EventScores.ActiveEventID = 80
	gameState.EventScores.ByEvent[80] = State.ScalableEventScore{
		EventID: 80, DifficultyID: 201, PlayerScore: 100, RemainingSec: 7_200,
		AdvisorActive: true, AdvisorCurrencyID: 78, ObservedAt: now,
	}
	gameState.NomadCamps.LastScannedAt[1] = now
	gameState.Map[0] = map[string]State.MapObservation{
		"99:100":  advisorPolicyObservation(99, 100, 40, now),
		"100:99":  advisorPolicyObservation(100, 99, 30, now),
		"100:101": advisorPolicyObservation(100, 101, 20, now),
		"101:100": advisorPolicyObservation(101, 100, 10, now),
	}
	return Snapshot{
		State: gameState, GameData: gameData, Now: now,
		Configuration: Configuration.Snapshot{Sections: map[string]json.RawMessage{
			"automation.autoAdvisor": json.RawMessage(`{
				"version":1,"sourceCastleId":1,"presetId":"camp","nomadDifficultyId":301,"samuraiDifficultyId":201,
				"maxAttackCount":10,"minimumRemainingSec":1800,"coinCostPerAttack":500,
				"minimumCoinReserve":0,"minimumFeatherReserve":0,"timeSkipReserve":{},
				"checkIntervalSec":30,"mapRefreshIntervalSec":300,"horseTravelBoostId":-1
			}`),
			"attacks.presets": json.RawMessage(`{"version":1,"presets":[{"id":"camp","name":"Camp","waves":[{"L":{"troops":[],"tools":[]},"M":{"troops":[{"itemId":77,"quantity":100}],"tools":[{"itemId":88,"quantity":10}]},"R":{"troops":[],"tools":[]}}]}]}`),
		}},
	}
}

func advisorPolicyObservation(x, y int, defenseBonus int64, observedAt time.Time) State.MapObservation {
	return State.MapObservation{
		KingdomID: 0, X: x, Y: y, TypeID: samuraiCampTypeID, ObjectID: 5001,
		EventCampID: 5001, EventCampVictoryCount: 9, EventCampBaseWallBonus: defenseBonus,
		ObservedAt: observedAt,
	}
}
