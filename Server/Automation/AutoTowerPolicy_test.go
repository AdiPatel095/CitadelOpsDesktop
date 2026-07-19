package Automation

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"CitadelDesktop/Server/Configuration"
	"CitadelDesktop/Server/GameData"
	"CitadelDesktop/Server/State"
)

func TestAutoTowerPolicyScansStaleCastlesBeforeLaunchingQueue(t *testing.T) {
	now := time.Date(2026, 7, 12, 16, 0, 0, 0, time.UTC)
	snapshot := autoTowerPolicySnapshot(now)
	policy := NewAutoTowerPolicy()

	decision, err := policy.Evaluate(t.Context(), snapshot)
	if err != nil || decision.Request == nil || decision.Request.Name != "tower.queue.scan" {
		t.Fatalf("queue scan decision: %#v err=%v", decision, err)
	}
	if decision.ScheduleKey != "autoTowers:1" {
		t.Fatalf("tower scan schedule key = %q", decision.ScheduleKey)
	}
	var scanArguments struct {
		ScanStartedAt time.Time `json:"scanStartedAt"`
	}
	if err := json.Unmarshal(decision.Request.Arguments, &scanArguments); err != nil || !scanArguments.ScanStartedAt.Equal(now) {
		t.Fatalf("queue scan freshness boundary = %#v err=%v", scanArguments, err)
	}

	fillTowerCoverage(&snapshot.State, 100, 100, now)
	snapshot.State.Map[0]["101:100"] = State.MapObservation{
		KingdomID: 0, X: 101, Y: 100, TypeID: kingdomTowerMapTypeID,
		TowerVictoryCount: 845, Level: 81, ObservedAt: now,
	}
	snapshot.State.TowerQueue.LastScannedAt[1] = now
	snapshot.State.TowerQueue.EntriesByCastle[1] = []State.TowerQueueEntry{{
		KingdomID: 0, TargetX: 101, TargetY: 100, MapObservedAt: now, QueuedAt: now,
	}}
	decision, err = policy.Evaluate(t.Context(), snapshot)
	if err != nil || decision.Request == nil || decision.Request.Name != "tower.attack" || decision.FollowUp != nil {
		t.Fatalf("tower preparation decision: %#v err=%v", decision, err)
	}
	if decision.ScheduleKey != "autoTowers:1" {
		t.Fatalf("tower attack schedule key = %q", decision.ScheduleKey)
	}
	if !decision.NextCheckAt.Equal(now.Add(2 * time.Second)) {
		t.Fatalf("tower queue next check = %s, want immediate queue cadence", decision.NextCheckAt)
	}
}

func TestAutoTowerPolicyRescansAtThirtyMinuteBoundary(t *testing.T) {
	now := time.Date(2026, 7, 12, 16, 0, 0, 0, time.UTC)
	snapshot := autoTowerPolicySnapshot(now)
	snapshot.State.TowerQueue.LastScannedAt[1] = now.Add(-29 * time.Minute)

	decision, err := NewAutoTowerPolicy().Evaluate(t.Context(), snapshot)
	if err != nil || decision.Request != nil || decision.Status != "idle" {
		t.Fatalf("fresh scan should not repeat early: %#v err=%v", decision, err)
	}

	snapshot.State.TowerQueue.LastScannedAt[1] = now.Add(-30 * time.Minute)
	decision, err = NewAutoTowerPolicy().Evaluate(t.Context(), snapshot)
	if err != nil || decision.Request == nil || decision.Request.Name != "tower.queue.scan" {
		t.Fatalf("30-minute scan decision: %#v err=%v", decision, err)
	}
}

func TestAutoTowerPolicyWakesWhenPerCastleScheduleOpens(t *testing.T) {
	now := time.Date(2026, 7, 13, 12, 0, 0, 0, time.UTC)
	snapshot := autoTowerPolicySnapshot(now)
	snapshot.Configuration.Sections["automation.autoTowers"] = json.RawMessage(`{
		"checkIntervalSec":300,"mapRefreshIntervalSec":1800,
		"castles":{"1":{"enabled":true,"radius":1,"maxActiveTowers":1,"unitId":77,"maidenOnly":false}}
	}`)
	snapshot.Configuration.Sections["scheduler"] = json.RawMessage(`{
		"featureSchedules":{"autoTowers:1":{
			"enabled":true,"timeZone":"UTC","slots":[{"day":1,"startMinute":721,"endMinute":800}]
		}}
	}`)

	decision, err := NewAutoTowerPolicy().Evaluate(t.Context(), snapshot)
	if err != nil || decision.Request != nil {
		t.Fatalf("closed per-castle schedule decision: %#v err=%v", decision, err)
	}
	want := now.Add(time.Minute)
	if !decision.NextCheckAt.Equal(want) {
		t.Fatalf("per-castle schedule wake = %s, want %s", decision.NextCheckAt, want)
	}
}

func TestAutoTowerPolicyLaunchesOpportunisticallyWhileOlderTowerMovementIsActive(t *testing.T) {
	now := time.Date(2026, 7, 12, 16, 0, 0, 0, time.UTC)
	snapshot := autoTowerPolicySnapshot(now)
	fillTowerCoverage(&snapshot.State, 100, 100, now)
	snapshot.State.Map[0]["101:100"] = State.MapObservation{
		KingdomID: 0, X: 101, Y: 100, TypeID: kingdomTowerMapTypeID,
		TowerVictoryCount: 845, Level: 81, ObservedAt: now,
	}
	snapshot.State.TowerQueue.LastScannedAt[1] = now
	snapshot.State.TowerQueue.EntriesByCastle[1] = []State.TowerQueueEntry{{KingdomID: 0, TargetX: 101, TargetY: 100, MapObservedAt: now, QueuedAt: now}}
	snapshot.State.Movements[1] = State.MovementState{
		ID: 1, SourceCastleID: 1, TargetTypeID: kingdomTowerMapTypeID, Direction: 0,
		KingdomID: 0, TargetX: 102, TargetY: 100, Units: map[State.UnitID]int64{77: 100},
	}
	decision, err := NewAutoTowerPolicy().Evaluate(t.Context(), snapshot)
	if err != nil || decision.Request == nil || decision.Request.Name != "tower.attack" {
		t.Fatalf("opportunistic tower decision: %#v err=%v", decision, err)
	}
}

func TestAutoTowerPolicyWaitsForEnoughTroopsBeforeSubmittingAttack(t *testing.T) {
	now := time.Date(2026, 7, 15, 20, 0, 0, 0, time.UTC)
	snapshot := autoTowerPolicySnapshot(now)
	gameData, err := GameData.DecodeStore([]byte(`{
		"versionInfo":[],"units":[],"buildings":[],"effects":[]
	}`), GameData.SourceMetadata{ItemVersion: "test"})
	if err != nil {
		t.Fatal(err)
	}
	snapshot.GameData = gameData
	fillTowerCoverage(&snapshot.State, 100, 100, now)
	snapshot.State.Map[0]["101:100"] = State.MapObservation{
		KingdomID: 0, X: 101, Y: 100, TypeID: kingdomTowerMapTypeID,
		TowerVictoryCount: 845, Level: 81, ObservedAt: now,
	}
	snapshot.State.TowerQueue.LastScannedAt[1] = now
	snapshot.State.TowerQueue.EntriesByCastle[1] = []State.TowerQueueEntry{{
		KingdomID: 0, TargetX: 101, TargetY: 100, MapObservedAt: now, QueuedAt: now,
	}}
	castle := snapshot.State.Castles[1]
	castle.Units.Stationed = map[State.UnitID]int64{77: 1}
	snapshot.State.Castles[1] = castle

	decision, err := NewAutoTowerPolicy().Evaluate(t.Context(), snapshot)
	if err != nil || decision.Request != nil || decision.Status != "waiting" ||
		!strings.Contains(decision.Detail, "Waiting for tower troops") {
		t.Fatalf("tower shortage decision: %#v err=%v", decision, err)
	}

	castle.Units.Stationed[77] = 2_000
	snapshot.State.Castles[1] = castle
	decision, err = NewAutoTowerPolicy().Evaluate(t.Context(), snapshot)
	if err != nil || decision.Request == nil || decision.Request.Name != "tower.attack" {
		t.Fatalf("tower ready decision: %#v err=%v", decision, err)
	}
}

func TestTowerQueueHasNoConfiguredMaximumAndLocksActiveTargets(t *testing.T) {
	now := time.Date(2026, 7, 12, 16, 0, 0, 0, time.UTC)
	snapshot := autoTowerPolicySnapshot(now)
	fillTowerCoverage(&snapshot.State, 100, 100, now)
	for _, target := range []State.MapObservation{
		{KingdomID: 0, X: 99, Y: 100, TypeID: kingdomTowerMapTypeID, TowerVictoryCount: 845, Level: 81, ObservedAt: now},
		{KingdomID: 0, X: 100, Y: 101, TypeID: kingdomTowerMapTypeID, TowerVictoryCount: 845, Level: 81, ObservedAt: now},
		{KingdomID: 0, X: 101, Y: 100, TypeID: kingdomTowerMapTypeID, TowerVictoryCount: 845, Level: 81, ObservedAt: now},
	} {
		snapshot.State.Map[0][mapKey(target.X, target.Y)] = target
	}
	settings := autoTowerSettings{Castles: map[string]autoTowerCastle{
		"1": {Enabled: true, Radius: 1, UnitID: 77},
	}}
	snapshot.State.TowerQueue.LastScannedAt[1] = now
	for _, target := range []State.TowerQueueEntry{
		{KingdomID: 0, TargetX: 99, TargetY: 100, MapObservedAt: now, QueuedAt: now},
		{KingdomID: 0, TargetX: 100, TargetY: 101, MapObservedAt: now, QueuedAt: now},
		{KingdomID: 0, TargetX: 101, TargetY: 100, MapObservedAt: now, QueuedAt: now},
	} {
		snapshot.State.TowerQueue.EntriesByCastle[1] = append(snapshot.State.TowerQueue.EntriesByCastle[1], target)
	}
	candidates, _, _ := queuedTowerCandidates(snapshot, settings)
	if len(candidates) != 3 {
		t.Fatalf("unexpected uncapped queue: %#v", candidates)
	}

	snapshot.State.Movements[1] = State.MovementState{
		ID: 1, Direction: 1, SourceTypeID: kingdomTowerMapTypeID, SourceX: 101, SourceY: 100,
		TargetCastleID: 1, KingdomID: 0,
	}
	candidates, _, _ = queuedTowerCandidates(snapshot, settings)
	if len(candidates) != 2 {
		t.Fatalf("returning tower movement did not lock its target: %#v", candidates)
	}
	returnedAt := now.Add(-time.Second)
	movement := snapshot.State.Movements[1]
	movement.ReturnsAt = &returnedAt
	snapshot.State.Movements[1] = movement
	candidates, active, _ := queuedTowerCandidates(snapshot, settings)
	if len(candidates) != 3 || active != 0 {
		t.Fatalf("completed return still locked tower work: candidates=%#v active=%d", candidates, active)
	}
}

func TestAutoTowerRadiusAllowsFiftyTiles(t *testing.T) {
	if clampTowerRadius(50) != 50 || clampTowerRadius(51) != 50 {
		t.Fatalf("tower radius clamp = %d, %d", clampTowerRadius(50), clampTowerRadius(51))
	}
}

func TestTowerQueuePromotesCoolingTargetsWithoutAnotherScan(t *testing.T) {
	now := time.Date(2026, 7, 12, 16, 0, 0, 0, time.UTC)
	snapshot := autoTowerPolicySnapshot(now)
	fillTowerCoverage(&snapshot.State, 100, 100, now)
	snapshot.State.Map[0]["101:100"] = State.MapObservation{
		KingdomID: 0, X: 101, Y: 100, TypeID: kingdomTowerMapTypeID,
		TowerVictoryCount: 845, Level: 81, TowerCooldownRemaining: 30, ObservedAt: now,
	}
	snapshot.State.TowerQueue.LastScannedAt[1] = now
	snapshot.State.TowerQueue.EntriesByCastle[1] = []State.TowerQueueEntry{{KingdomID: 0, TargetX: 101, TargetY: 100, MapObservedAt: now, QueuedAt: now}}
	settings := autoTowerSettings{Castles: map[string]autoTowerCastle{"1": {Enabled: true, Radius: 1, UnitID: 77}}}

	candidates, _, _ := queuedTowerCandidates(snapshot, settings)
	if len(candidates) != 0 {
		t.Fatalf("cooling target should wait: %#v", candidates)
	}
	snapshot.Now = now.Add(30 * time.Second)
	candidates, _, _ = queuedTowerCandidates(snapshot, settings)
	if len(candidates) != 1 || candidates[0].Entry.TargetX != 101 {
		t.Fatalf("cooling target should become eligible without a rescan: %#v", candidates)
	}
}

func TestTowerQueueAssignsOverlappingTargetsToClosestCastle(t *testing.T) {
	now := time.Date(2026, 7, 12, 16, 0, 0, 0, time.UTC)
	snapshot := autoTowerPolicySnapshot(now)
	snapshot.State.Castles[2] = State.CastleState{ID: 2, Name: "Closer Tower Castle", KingdomID: 0, X: 102, Y: 100}
	fillTowerCoverage(&snapshot.State, 100, 100, now)
	snapshot.State.Map[0]["103:100"] = State.MapObservation{
		KingdomID: 0, X: 103, Y: 100, TypeID: kingdomTowerMapTypeID, TowerVictoryCount: 845, Level: 81, ObservedAt: now,
	}
	entry := State.TowerQueueEntry{KingdomID: 0, TargetX: 103, TargetY: 100, MapObservedAt: now, QueuedAt: now}
	snapshot.State.TowerQueue.EntriesByCastle[1] = []State.TowerQueueEntry{entry}
	snapshot.State.TowerQueue.EntriesByCastle[2] = []State.TowerQueueEntry{entry}
	settings := autoTowerSettings{Castles: map[string]autoTowerCastle{
		"1": {Enabled: true, Radius: 5, UnitID: 77},
		"2": {Enabled: true, Radius: 5, UnitID: 77},
	}}

	candidates, _, _ := queuedTowerCandidates(snapshot, settings)
	if len(candidates) != 1 || candidates[0].Castle.ID != 2 {
		t.Fatalf("overlapping target should use closest castle: %#v", candidates)
	}
}

func TestAutoTowerPolicyRefreshesCooldownAfterConfirmedBattle(t *testing.T) {
	now := time.Date(2026, 7, 12, 16, 0, 0, 0, time.UTC)
	snapshot := autoTowerPolicySnapshot(now)
	snapshot.State.TowerCooldowns["0:101:100"] = State.TowerCooldownState{
		KingdomID: 0, X: 101, Y: 100, LastSuccessfulBattleAt: now, PendingCooldownRefresh: true,
	}
	decision, err := NewAutoTowerPolicy().Evaluate(t.Context(), snapshot)
	if err != nil || decision.Request == nil || decision.Request.Name != "map.query" {
		t.Fatalf("cooldown refresh decision: %#v err=%v", decision, err)
	}
	if string(decision.Request.Arguments) != `{"kingdomId":0,"x1":101,"x2":101,"y1":100,"y2":100}` {
		t.Fatalf("cooldown map query = %s", decision.Request.Arguments)
	}
}

func autoTowerPolicySnapshot(now time.Time) Snapshot {
	gameState := State.NewGameState()
	gameState.Castles[1] = State.CastleState{ID: 1, Name: "Tower Castle", KingdomID: 0, X: 100, Y: 100}
	gameState.Commanders[1] = State.CommanderState{ID: 1, Available: true}
	return Snapshot{
		State: gameState,
		Configuration: Configuration.Snapshot{Sections: map[string]json.RawMessage{
			"automation.autoTowers": json.RawMessage(`{"checkIntervalSec":30,"mapRefreshIntervalSec":1800,"castles":{"1":{"enabled":true,"radius":1,"maxActiveTowers":1,"unitId":77,"maidenOnly":false}}}`),
		}},
		Now: now,
	}
}

func fillTowerCoverage(gameState *State.GameState, centerX int, centerY int, observedAt time.Time) {
	gameState.Map[0] = map[string]State.MapObservation{}
	for y := centerY - 1; y <= centerY+1; y++ {
		for x := centerX - 1; x <= centerX+1; x++ {
			gameState.Map[0][mapKey(x, y)] = State.MapObservation{KingdomID: 0, X: x, Y: y, TypeID: 31, ObservedAt: observedAt}
		}
	}
}

func mapKey(x int, y int) string {
	return fmt.Sprintf("%d:%d", x, y)
}
