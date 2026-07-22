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

func TestAutoNomadPolicyLevelsFourThenLocksWeakestAndChainsCommanders(t *testing.T) {
	now := time.Date(2026, 7, 14, 13, 0, 0, 0, time.UTC)
	snapshot := autoNomadPolicySnapshot(t, now)
	policy := NewAutoNomadPolicy()

	decision, err := policy.Evaluate(t.Context(), snapshot)
	if err != nil || decision.Request == nil || decision.Request.Name != "nomad.camp.attack" {
		t.Fatalf("leveling decision: %#v err=%v", decision, err)
	}
	if decision.Metrics["maximumVictoryCount"] != 9 || !strings.Contains(decision.Detail, "8/9") {
		t.Fatalf("leveling decision did not expose the difficulty ceiling: %#v", decision)
	}
	var leveling struct {
		Mode         string              `json:"mode"`
		TargetX      int                 `json:"targetX"`
		TargetY      int                 `json:"targetY"`
		CommanderIDs []State.CommanderID `json:"commanderIds"`
	}
	if err := json.Unmarshal(decision.Request.Arguments, &leveling); err != nil {
		t.Fatal(err)
	}
	if leveling.Mode != "level" || leveling.TargetX != 99 || leveling.TargetY != 100 ||
		len(leveling.CommanderIDs) != 2 || leveling.CommanderIDs[0] != 1 || leveling.CommanderIDs[1] != 2 {
		t.Fatalf("leveling did not preserve the eligible commander pool: %#v", leveling)
	}

	maxed := snapshot.State.Map[0]["99:100"]
	maxed.EventCampID = 5001
	maxed.ObjectID = 5001
	maxed.EventCampVictoryCount = 9
	snapshot.State.Map[0]["99:100"] = maxed
	decision, err = policy.Evaluate(t.Context(), snapshot)
	if err != nil || decision.Request == nil || decision.Request.Name != "nomad.target.lock" {
		t.Fatalf("target lock decision: %#v err=%v", decision, err)
	}
	var lock struct {
		SourceCastleID State.CastleID  `json:"sourceCastleId"`
		EventID        int64           `json:"eventId"`
		DifficultyID   int64           `json:"difficultyId"`
		KingdomID      State.KingdomID `json:"kingdomId"`
		TargetTypeID   int             `json:"targetTypeId"`
		TargetX        int             `json:"targetX"`
		TargetY        int             `json:"targetY"`
		EventCampID    int64           `json:"eventCampId"`
		VictoryCount   int64           `json:"victoryCount"`
		DefenseScore   int64           `json:"defenseScore"`
		EventEndsAt    time.Time       `json:"eventEndsAt"`
	}
	if err := json.Unmarshal(decision.Request.Arguments, &lock); err != nil {
		t.Fatal(err)
	}
	if lock.TargetX != 101 || lock.TargetY != 100 || lock.EventCampID != 5001 || lock.VictoryCount != 9 {
		t.Fatalf("wrong weakest maxed camp selected: %#v", lock)
	}
	snapshot.State.NomadCamps.LockedTarget = &State.NomadCampTargetState{
		SourceCastleID: lock.SourceCastleID, EventID: lock.EventID, DifficultyID: lock.DifficultyID,
		KingdomID: lock.KingdomID, TypeID: lock.TargetTypeID, X: lock.TargetX, Y: lock.TargetY,
		EventCampID: lock.EventCampID, VictoryCount: lock.VictoryCount, DefenseScore: lock.DefenseScore,
		EventEndsAt: lock.EventEndsAt, LockedAt: now,
	}
	for commanderID := State.CommanderID(3); commanderID <= 25; commanderID++ {
		snapshot.State.Commanders[commanderID] = State.CommanderState{ID: commanderID, Available: true}
	}
	source := snapshot.State.Castles[1]
	source.Units.Stationed[77] = 3_000
	snapshot.State.Castles[1] = source
	snapshot.State.Player.Currencies[1005] = 30
	activeCommander := State.CommanderID(25)
	arrivesAt := now.Add(10 * time.Minute)
	snapshot.State.Commanders[activeCommander] = State.CommanderState{ID: activeCommander, Available: false}
	snapshot.State.Movements[1] = State.MovementState{
		ID: 1, Direction: 0, SourceCastleID: 1, KingdomID: 0, TargetX: 101, TargetY: 100,
		CommanderID: &activeCommander, ArrivesAt: &arrivesAt,
	}
	decision, err = policy.Evaluate(t.Context(), snapshot)
	if err != nil || decision.Request == nil || decision.Request.Name != "nomad.camp.attack" {
		t.Fatalf("chain decision: %#v err=%v", decision, err)
	}
	var chain struct {
		Mode         string              `json:"mode"`
		TargetX      int                 `json:"targetX"`
		TargetY      int                 `json:"targetY"`
		CommanderIDs []State.CommanderID `json:"commanderIds"`
	}
	if err := json.Unmarshal(decision.Request.Arguments, &chain); err != nil {
		t.Fatal(err)
	}
	if chain.Mode != "chain" || chain.TargetX != 101 || chain.TargetY != 100 || len(chain.CommanderIDs) != 24 {
		t.Fatalf("chain waited for an older hit instead of using all 24 currently available commanders: %#v", chain)
	}
	if decision.Metrics["committedCooldownSkips"] != 1 || decision.Metrics["usableCooldownSkips"] != 29 {
		t.Fatalf("chain did not reserve a skip for the older in-flight hit: %#v", decision.Metrics)
	}
}

func TestAutoNomadRBCTestSizesToPresetCopiesThenRefreshesAndSkipsEveryHit(t *testing.T) {
	now := time.Date(2026, 7, 14, 13, 0, 0, 0, time.UTC)
	snapshot := autoNomadPolicySnapshot(t, now)
	for commanderID := State.CommanderID(3); commanderID <= 10; commanderID++ {
		snapshot.State.Commanders[commanderID] = State.CommanderState{ID: commanderID, Available: true}
	}
	source := snapshot.State.Castles[1]
	source.Units.Stationed[77] = 450
	snapshot.State.Castles[1] = source
	snapshot.State.Player.Currencies[1006] = 20
	snapshot.State.Map[0]["101:102"] = State.MapObservation{
		KingdomID: 0, TypeID: kingdomTowerMapTypeID, X: 101, Y: 102,
		TowerVictoryCount: 845, ObservedAt: now,
	}
	snapshot.Configuration.Sections["automation.autoNomad"] = json.RawMessage(`{
		"version":4,"sourceCastleId":1,"presetId":"camp","skipCooldowns":true,
		"timeSkipReserve":{"MS6":1},
		"rbcTest":{"enabled":true,"runId":"trial-1","targetX":101,"targetY":102,"attackCount":10}
	}`)
	policy := NewAutoNomadPolicy()

	decision, err := policy.Evaluate(t.Context(), snapshot)
	if err != nil || decision.Request == nil || decision.Request.Name != "nomad.rbc_test.attack" {
		t.Fatalf("RBC trial launch decision: %#v err=%v", decision, err)
	}
	var launch struct {
		ExpectedAttacks int                 `json:"expectedAttacks"`
		CommanderIDs    []State.CommanderID `json:"commanderIds"`
	}
	if err := json.Unmarshal(decision.Request.Arguments, &launch); err != nil {
		t.Fatal(err)
	}
	if launch.ExpectedAttacks != 4 || len(launch.CommanderIDs) != 4 {
		t.Fatalf("RBC trial did not size itself to four complete preset copies: %#v", launch)
	}

	snapshot.State.NomadCamps.RBCTest = &State.NomadRBCTestState{
		RunID: "trial-1", SourceCastleID: 1, KingdomID: 0, TargetX: 101, TargetY: 102,
		ExpectedAttacks: 4, AttacksLaunched: 4, VictoriesConfirmed: 1, StartedAt: now,
	}
	snapshot.State.TowerCooldowns["0:101:102"] = State.TowerCooldownState{
		KingdomID: 0, X: 101, Y: 102, LastSuccessfulBattleAt: now.Add(time.Minute), PendingCooldownRefresh: true,
	}
	decision, err = policy.Evaluate(t.Context(), snapshot)
	if err != nil || decision.Request == nil || decision.Request.Name != "map.query" {
		t.Fatalf("RBC post-hit refresh decision: %#v err=%v", decision, err)
	}

	cooldown := snapshot.State.TowerCooldowns["0:101:102"]
	cooldown.PendingCooldownRefresh = false
	cooldown.CooldownRemaining = 10_800
	cooldown.CooldownObservedAt = now.Add(time.Minute)
	snapshot.State.TowerCooldowns["0:101:102"] = cooldown
	target := snapshot.State.Map[0]["101:102"]
	target.TowerCooldownRemaining = 10_800
	target.ObservedAt = now.Add(time.Minute)
	snapshot.State.Map[0]["101:102"] = target
	snapshot.Now = now.Add(time.Minute)
	decision, err = policy.Evaluate(t.Context(), snapshot)
	if err != nil || decision.Request == nil || decision.Request.Name != "nomad.cooldown.minute_skip" {
		t.Fatalf("RBC minute-skip decision: %#v err=%v", decision, err)
	}

	target.TowerCooldownRemaining = 0
	target.ObservedAt = snapshot.Now.Add(time.Second)
	snapshot.State.Map[0]["101:102"] = target
	snapshot.State.NomadCamps.RBCTest.CooldownsSkipped = 1
	for commanderID, commander := range snapshot.State.Commanders {
		commander.Available = false
		snapshot.State.Commanders[commanderID] = commander
	}
	commander := snapshot.State.Commanders[1]
	commander.Available = true
	snapshot.State.Commanders[1] = commander
	source = snapshot.State.Castles[1]
	source.Units.Stationed[77] = 100
	snapshot.State.Castles[1] = source
	snapshot.Now = snapshot.Now.Add(time.Second)
	decision, err = policy.Evaluate(t.Context(), snapshot)
	if err != nil || decision.Request == nil || decision.Request.Name != "nomad.rbc_test.attack" {
		t.Fatalf("RBC trial did not opportunistically refill while older hits remained in flight: %#v err=%v", decision, err)
	}
	if err := json.Unmarshal(decision.Request.Arguments, &launch); err != nil {
		t.Fatal(err)
	}
	if launch.ExpectedAttacks != 1 || len(launch.CommanderIDs) != 1 {
		t.Fatalf("RBC refill did not use the one newly available commander and preset copy: %#v", launch)
	}
}

func TestAutoNomadPolicyClearsCooldownWhileLeveling(t *testing.T) {
	now := time.Date(2026, 7, 14, 13, 0, 0, 0, time.UTC)
	snapshot := autoNomadPolicySnapshot(t, now)
	target := snapshot.State.Map[0]["99:100"]
	target.EventCampCooldownRemaining = 3600
	snapshot.State.Map[0]["99:100"] = target

	decision, err := NewAutoNomadPolicy().Evaluate(t.Context(), snapshot)
	if err != nil || decision.Request == nil || decision.Request.Name != "nomad.cooldown.minute_skip" {
		t.Fatalf("leveling cooldown decision: %#v err=%v", decision, err)
	}
}

func TestAutoNomadRBCTestSizesChainToOneCommandSkips(t *testing.T) {
	now := time.Date(2026, 7, 14, 13, 0, 0, 0, time.UTC)
	snapshot := autoNomadPolicySnapshot(t, now)
	snapshot.State.Commanders[3] = State.CommanderState{ID: 3, Available: true}
	snapshot.State.Player.Currencies[1006] = 3
	snapshot.State.Map[0]["101:102"] = State.MapObservation{
		KingdomID: 0, TypeID: kingdomTowerMapTypeID, X: 101, Y: 102,
		TowerVictoryCount: 845, ObservedAt: now,
	}
	snapshot.Configuration.Sections["automation.autoNomad"] = json.RawMessage(`{
		"version":4,"sourceCastleId":1,"presetId":"camp","skipCooldowns":true,
		"timeSkipReserve":{"MS6":1},
		"rbcTest":{"enabled":true,"runId":"trial-budget","targetX":101,"targetY":102,"attackCount":10}
	}`)

	decision, err := NewAutoNomadPolicy().Evaluate(t.Context(), snapshot)
	if err != nil || decision.Request == nil || decision.Request.Name != "nomad.rbc_test.attack" {
		t.Fatalf("RBC skip-budget decision: %#v err=%v", decision, err)
	}
	var launch struct {
		ExpectedAttacks int                 `json:"expectedAttacks"`
		CommanderIDs    []State.CommanderID `json:"commanderIds"`
	}
	if err := json.Unmarshal(decision.Request.Arguments, &launch); err != nil {
		t.Fatal(err)
	}
	if launch.ExpectedAttacks != 2 || len(launch.CommanderIDs) != 2 {
		t.Fatalf("RBC trial did not size itself to two usable skips: %#v", launch)
	}
	snapshot.State.NomadCamps.RBCTest = &State.NomadRBCTestState{
		RunID: "trial-budget", SourceCastleID: 1, KingdomID: 0, TargetX: 101, TargetY: 102,
		ExpectedAttacks: 2, AttacksLaunched: 2, StartedAt: now,
	}
	decision, err = NewAutoNomadPolicy().Evaluate(t.Context(), snapshot)
	if err != nil || decision.Request != nil || decision.Status != "waiting" || decision.Metrics["committedCooldownSkips"] != 2 {
		t.Fatalf("RBC trial overcommitted skips already reserved for launched hits: %#v err=%v", decision, err)
	}
}

func autoNomadPolicySnapshot(t *testing.T, now time.Time) Snapshot {
	t.Helper()
	gameData, err := GameData.DecodeStore([]byte(`{
		"versionInfo":[],
		"buildings":[],
		"units":[],
		"eventAutoScalingDifficulties":[{"difficultyID":201,"eventID":80,"difficultyTypeID":1,"isLocked":0}],
		"eventAutoScalingCamps":[
			{"eventAutoScalingCampID":5000,"eventID":80,"difficultyID":201,"areaType":29,"camplevel":80,"countVictory":8,"coolDown":0,"skipCosts":0,"maxTroopCapacityDefense":500},
			{"eventAutoScalingCampID":5001,"eventID":80,"difficultyID":201,"areaType":29,"camplevel":90,"countVictory":9,"coolDown":3600,"skipCosts":9950,"maxTroopCapacityDefense":600}
		]
	}`), GameData.SourceMetadata{ItemVersion: "test"})
	if err != nil {
		t.Fatal(err)
	}
	gameState := State.NewGameState()
	gameState.Castles[1] = State.CastleState{
		ID: 1, Name: "Main", KingdomID: 0, X: 100, Y: 100,
		Units: State.CastleUnits{Stationed: map[State.UnitID]int64{77: 1_000}, Traveling: map[State.UnitID]int64{}},
	}
	gameState.Commanders[1] = State.CommanderState{ID: 1, Available: true}
	gameState.Commanders[2] = State.CommanderState{ID: 2, Available: true}
	gameState.Player.Currencies[1005] = 10
	gameState.EventScores.ActiveEventID = 80
	gameState.EventScores.ByEvent[80] = State.ScalableEventScore{
		EventID: 80, DifficultyID: 201, PlayerScore: 100, RemainingSec: 7_200, ObservedAt: now,
	}
	gameState.NomadCamps.LastScannedAt[1] = now
	gameState.Map[0] = map[string]State.MapObservation{
		"99:100":  nomadPolicyObservation(99, 100, 5000, 8, 30, now),
		"100:99":  nomadPolicyObservation(100, 99, 5001, 9, 20, now),
		"100:101": nomadPolicyObservation(100, 101, 5001, 9, 10, now),
		"101:100": nomadPolicyObservation(101, 100, 5001, 9, 1, now),
	}
	return Snapshot{
		State: gameState, GameData: gameData, Now: now,
		Configuration: Configuration.Snapshot{Sections: map[string]json.RawMessage{
			"automation.autoNomad": json.RawMessage(`{"version":4,"sourceCastleId":1,"presetId":"camp","nomadDifficultyId":301,"samuraiDifficultyId":201,"scoreTarget":100000,"minimumRemainingSec":1800,"checkIntervalSec":30,"mapRefreshIntervalSec":300,"skipCooldowns":true,"timeSkipReserve":{}}`),
			"attacks.presets":      json.RawMessage(`{"version":1,"presets":[{"id":"camp","name":"Camp","waves":[{"L":{"troops":[],"tools":[]},"M":{"troops":[{"itemId":77,"quantity":100}],"tools":[]},"R":{"troops":[],"tools":[]}}]}]}`),
		}},
	}
}

func nomadPolicyObservation(x, y int, campID, victories, defenseBonus int64, observedAt time.Time) State.MapObservation {
	return State.MapObservation{
		KingdomID: 0, X: x, Y: y, TypeID: samuraiCampTypeID, ObjectID: campID,
		EventCampID: campID, EventCampVictoryCount: victories, EventCampBaseWallBonus: defenseBonus,
		ObservedAt: observedAt,
	}
}
