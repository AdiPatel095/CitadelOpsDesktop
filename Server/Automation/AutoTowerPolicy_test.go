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
	if !decision.ReevaluateOnStale {
		t.Fatal("tower attack did not opt into immediate stale re-evaluation")
	}
	if decision.ScheduleKey != "autoTowers:1" {
		t.Fatalf("tower attack schedule key = %q", decision.ScheduleKey)
	}
	if !decision.NextCheckAt.Equal(now.Add(2 * time.Second)) {
		t.Fatalf("tower queue next check = %s, want immediate queue cadence", decision.NextCheckAt)
	}
	var attackArguments struct {
		CommanderIDs []State.CommanderID `json:"commanderIds"`
	}
	if err := json.Unmarshal(decision.Request.Arguments, &attackArguments); err != nil {
		t.Fatal(err)
	}
	if len(attackArguments.CommanderIDs) != 1 || attackArguments.CommanderIDs[0] != 1 {
		t.Fatalf("tower commander reservation = %#v", attackArguments.CommanderIDs)
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

func TestAutoTowerPolicyRefreshesStaleQueuedTargetBeforeAttack(t *testing.T) {
	now := time.Date(2026, 7, 12, 16, 0, 0, 0, time.UTC)
	snapshot := autoTowerPolicySnapshot(now)
	observedAt := now.Add(-autoTowerTargetVerificationAge)
	fillTowerCoverage(&snapshot.State, 100, 100, observedAt)
	snapshot.State.Map[0]["101:100"] = State.MapObservation{
		KingdomID: 0, X: 101, Y: 100, TypeID: kingdomTowerMapTypeID,
		TowerVictoryCount: 845, Level: 81, ObservedAt: observedAt,
	}
	snapshot.State.TowerQueue.LastScannedAt[1] = now
	snapshot.State.TowerQueue.EntriesByCastle[1] = []State.TowerQueueEntry{{
		KingdomID: 0, TargetX: 101, TargetY: 100, MapObservedAt: observedAt, QueuedAt: observedAt,
	}}

	decision, err := NewAutoTowerPolicy().Evaluate(t.Context(), snapshot)
	if err != nil || decision.Request == nil || decision.Request.Name != "tower.queue.target.refresh" {
		t.Fatalf("stale target refresh decision: %#v err=%v", decision, err)
	}
	var refreshArguments struct {
		SourceCastleID   State.CastleID `json:"sourceCastleId"`
		TargetX          int            `json:"targetX"`
		TargetY          int            `json:"targetY"`
		RefreshStartedAt time.Time      `json:"refreshStartedAt"`
	}
	if err := json.Unmarshal(decision.Request.Arguments, &refreshArguments); err != nil ||
		refreshArguments.SourceCastleID != 1 || refreshArguments.TargetX != 101 || refreshArguments.TargetY != 100 ||
		!refreshArguments.RefreshStartedAt.Equal(now) {
		t.Fatalf("stale target refresh arguments = %#v err=%v", refreshArguments, err)
	}
}

func TestAutoTowerPolicySkipsDeferredTowerAndContinuesQueue(t *testing.T) {
	now := time.Date(2026, 7, 12, 16, 0, 0, 0, time.UTC)
	snapshot := autoTowerPolicySnapshot(now)
	observedAt := now.Add(-autoTowerTargetVerificationAge)
	fillTowerCoverage(&snapshot.State, 100, 100, observedAt)
	for _, targetX := range []int{101, 102} {
		snapshot.State.Map[0][fmt.Sprintf("%d:100", targetX)] = State.MapObservation{
			KingdomID: 0, X: targetX, Y: 100, TypeID: kingdomTowerMapTypeID,
			TowerVictoryCount: 845, Level: 81, ObservedAt: observedAt,
		}
	}
	snapshot.State.TowerQueue.LastScannedAt[1] = now
	deferredUntil := now.Add(time.Minute)
	snapshot.State.TowerQueue.EntriesByCastle[1] = []State.TowerQueueEntry{
		{KingdomID: 0, TargetX: 101, TargetY: 100, MapObservedAt: observedAt, QueuedAt: observedAt, DeferredUntil: &deferredUntil},
		{KingdomID: 0, TargetX: 102, TargetY: 100, MapObservedAt: observedAt, QueuedAt: observedAt},
	}

	decision, err := NewAutoTowerPolicy().Evaluate(t.Context(), snapshot)
	if err != nil || decision.Request == nil || decision.Request.Name != "tower.queue.target.refresh" {
		t.Fatalf("deferred target decision: %#v err=%v", decision, err)
	}
	var arguments struct {
		TargetX int `json:"targetX"`
	}
	if err := json.Unmarshal(decision.Request.Arguments, &arguments); err != nil || arguments.TargetX != 102 {
		t.Fatalf("deferred queue chose target %#v err=%v", arguments, err)
	}
}

func TestTowerQueueKeepsRotatedTargetBehindOlderReadyTargets(t *testing.T) {
	now := time.Date(2026, 7, 12, 16, 0, 0, 0, time.UTC)
	snapshot := autoTowerPolicySnapshot(now)
	fillTowerCoverage(&snapshot.State, 100, 100, now)
	for _, targetX := range []int{101, 102} {
		snapshot.State.Map[0][fmt.Sprintf("%d:100", targetX)] = State.MapObservation{
			KingdomID: 0, X: targetX, Y: 100, TypeID: kingdomTowerMapTypeID,
			TowerVictoryCount: 845, Level: 81, ObservedAt: now,
		}
	}
	snapshot.State.TowerQueue.EntriesByCastle[1] = []State.TowerQueueEntry{
		{KingdomID: 0, TargetX: 101, TargetY: 100, MapObservedAt: now, QueuedAt: now},
		{KingdomID: 0, TargetX: 102, TargetY: 100, MapObservedAt: now, QueuedAt: now.Add(-time.Minute)},
	}
	settings := autoTowerSettings{Castles: map[string]autoTowerCastle{
		"1": {Enabled: true, Radius: 2, UnitID: 77},
	}}

	candidates, _, _ := queuedTowerCandidates(snapshot, settings)
	if len(candidates) != 2 || candidates[0].Entry.TargetX != 102 || candidates[1].Entry.TargetX != 101 {
		t.Fatalf("rotated queue order = %#v", candidates)
	}
}

func TestTowerQueueRotatesAcrossCastlesBeforeDrainingOneBatch(t *testing.T) {
	now := time.Date(2026, 7, 22, 18, 45, 0, 0, time.UTC)
	snapshot := autoTowerPolicySnapshot(now)
	snapshot.State.Castles[2] = State.CastleState{ID: 2, KingdomID: 0, X: 200, Y: 200}
	snapshot.State.Map[0] = map[string]State.MapObservation{
		"101:100": {KingdomID: 0, X: 101, Y: 100, TypeID: kingdomTowerMapTypeID, ObservedAt: now},
		"102:100": {KingdomID: 0, X: 102, Y: 100, TypeID: kingdomTowerMapTypeID, ObservedAt: now},
		"201:200": {KingdomID: 0, X: 201, Y: 200, TypeID: kingdomTowerMapTypeID, ObservedAt: now},
	}
	snapshot.State.TowerQueue.EntriesByCastle[1] = []State.TowerQueueEntry{
		{KingdomID: 0, TargetX: 101, TargetY: 100, QueuedAt: now.Add(-2 * time.Hour)},
		{KingdomID: 0, TargetX: 102, TargetY: 100, QueuedAt: now.Add(-2 * time.Hour)},
	}
	snapshot.State.TowerQueue.EntriesByCastle[2] = []State.TowerQueueEntry{
		{KingdomID: 0, TargetX: 201, TargetY: 200, QueuedAt: now.Add(-time.Hour)},
	}
	snapshot.State.TowerQueue.LastAttemptedAt[1] = now
	snapshot.State.TowerQueue.LastAttemptedAt[2] = now.Add(-time.Minute)
	settings := autoTowerSettings{Castles: map[string]autoTowerCastle{
		"1": {Enabled: true, Radius: 2, UnitID: 77},
		"2": {Enabled: true, Radius: 2, UnitID: 77},
	}}

	candidates, _, _ := queuedTowerCandidates(snapshot, settings)
	if len(candidates) != 3 || candidates[0].Castle.ID != 2 {
		t.Fatalf("castle round-robin order = %#v", candidates)
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

func TestAutoTowerCommanderRejectsActiveMovementEvenWhenRosterSaysAvailable(t *testing.T) {
	now := time.Date(2026, 7, 22, 18, 50, 0, 0, time.UTC)
	gameState := State.NewGameState()
	gameState.Player.ID = 1
	gameState.Castles[100] = State.CastleState{ID: 100}
	gameState.Commanders[1] = State.CommanderState{ID: 1, Available: true}
	gameState.Commanders[2] = State.CommanderState{ID: 2, Available: true}
	arrivesAt := now.Add(time.Minute)
	commanderID := State.CommanderID(1)
	gameState.Movements[10] = State.MovementState{
		ID: 10, Direction: 0, OwnerPlayerID: 1, SourceCastleID: 100,
		CommanderID: &commanderID, ArrivesAt: &arrivesAt,
	}

	selected, found := nextAutoTowerCommander(gameState, []State.CommanderID{1, 2}, true, false, now)
	if !found || selected != 2 {
		t.Fatalf("active movement commander was selected: commander=%d found=%t", selected, found)
	}
}

func TestAutoTowerCommanderIgnoresForeignMovementWithSameLeaderID(t *testing.T) {
	now := time.Date(2026, 7, 22, 18, 50, 0, 0, time.UTC)
	gameState := State.NewGameState()
	gameState.Player.ID = 1
	gameState.Commanders[1] = State.CommanderState{ID: 1, Available: true}
	arrivesAt := now.Add(time.Minute)
	commanderID := State.CommanderID(1)
	gameState.Movements[10] = State.MovementState{
		ID: 10, Direction: 0, OwnerPlayerID: 2, CommanderID: &commanderID, ArrivesAt: &arrivesAt,
	}

	selected, found := nextAutoTowerCommander(gameState, []State.CommanderID{1}, true, false, now)
	if !found || selected != 1 {
		t.Fatalf("foreign movement blocked own commander: commander=%d found=%t", selected, found)
	}
}

func TestTowerQueueKeepsSettlingAttackTargetReservedAfterMovementExpires(t *testing.T) {
	now := time.Date(2026, 7, 21, 20, 43, 56, 0, time.UTC)
	snapshot := autoTowerPolicySnapshot(now)
	fillTowerCoverage(&snapshot.State, 100, 100, now)
	snapshot.State.Map[0]["101:100"] = State.MapObservation{
		KingdomID: 0, X: 101, Y: 100, TypeID: kingdomTowerMapTypeID,
		TowerVictoryCount: 845, Level: 81, ObservedAt: now,
	}
	snapshot.State.TowerQueue.LastScannedAt[1] = now
	snapshot.State.TowerQueue.EntriesByCastle[1] = []State.TowerQueueEntry{{
		KingdomID: 0, TargetX: 101, TargetY: 100, MapObservedAt: now, QueuedAt: now,
	}}
	snapshot.State.AttackAnalytics.PendingAttacks = []State.AttackFeatureLaunch{{
		MovementID: 10, FeatureID: State.AttackFeatureAutoTowers, KingdomID: 0,
		TargetTypeID: kingdomTowerMapTypeID, TargetX: 101, TargetY: 100,
		LaunchedAt: now.Add(-5 * time.Minute), ArrivesAt: now.Add(-time.Second),
	}}
	settings := autoTowerSettings{Castles: map[string]autoTowerCastle{
		"1": {Enabled: true, Radius: 1, UnitID: 77},
	}}

	candidates, _, _ := queuedTowerCandidates(snapshot, settings)
	if len(candidates) != 0 {
		t.Fatalf("settling attack target was reselected: %#v", candidates)
	}
	snapshot.Now = now.Add(State.AttackFeatureTargetSettlementGrace)
	candidates, _, _ = queuedTowerCandidates(snapshot, settings)
	if len(candidates) != 1 {
		t.Fatalf("expired settlement reservation still blocked target: %#v", candidates)
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

func TestAutoTowerPolicyBypassesCapacityCorrectedCastleForReadyQueue(t *testing.T) {
	now := time.Date(2026, 7, 22, 18, 30, 0, 0, time.UTC)
	snapshot := autoTowerPolicySnapshot(now)
	gameData, err := GameData.DecodeStore([]byte(`{
		"versionInfo":[],"units":[],"buildings":[],"effects":[]
	}`), GameData.SourceMetadata{ItemVersion: "test"})
	if err != nil {
		t.Fatal(err)
	}
	snapshot.GameData = gameData
	snapshot.Configuration.Sections["automation.autoTowers"] = json.RawMessage(`{
		"checkIntervalSec":30,"mapRefreshIntervalSec":1800,
		"castles":{
			"1":{"enabled":true,"radius":1,"unitId":77,"maidenOnly":false},
			"2":{"enabled":true,"radius":1,"unitId":77,"maidenOnly":false}
		}
	}`)
	fillTowerCoverage(&snapshot.State, 100, 100, now)
	snapshot.State.Castles[2] = State.CastleState{
		ID: 2, Name: "Ready Tower Castle", KingdomID: 0, X: 200, Y: 200,
		Units: State.CastleUnits{Stationed: map[State.UnitID]int64{77: 2_000}},
	}
	snapshot.State.Map[0]["101:100"] = State.MapObservation{
		KingdomID: 0, X: 101, Y: 100, TypeID: kingdomTowerMapTypeID,
		TowerVictoryCount: 845, Level: 81, ObservedAt: now,
	}
	snapshot.State.Map[0]["201:200"] = State.MapObservation{
		KingdomID: 0, X: 201, Y: 200, TypeID: kingdomTowerMapTypeID,
		TowerVictoryCount: 845, Level: 81, ObservedAt: now,
	}
	shortCastle := snapshot.State.Castles[1]
	firstEntry := State.TowerQueueEntry{
		KingdomID: 0, TargetX: 101, TargetY: 100, MapObservedAt: now, QueuedAt: now.Add(-time.Minute),
	}
	baseline, err := autoTowerCapacityRequirement(snapshot, towerQueueCandidate{
		Castle: shortCastle, Plan: autoTowerCastle{Enabled: true, Radius: 1, UnitID: 77}, Entry: firstEntry,
	}, 1)
	if err != nil {
		t.Fatal(err)
	}
	shortCastle.Units.Stationed = map[State.UnitID]int64{77: baseline}
	snapshot.State.Castles[1] = shortCastle
	snapshot.State.TowerQueue.CapacityByCastle[1] = State.TowerCapacityObservation{
		AdditionalUnits: 1, FullFlankUnits: baseline + 1, ObservedAt: now,
	}
	snapshot.State.TowerQueue.LastScannedAt[1] = now
	snapshot.State.TowerQueue.LastScannedAt[2] = now
	snapshot.State.TowerQueue.EntriesByCastle[1] = []State.TowerQueueEntry{firstEntry}
	snapshot.State.TowerQueue.EntriesByCastle[2] = []State.TowerQueueEntry{{
		KingdomID: 0, TargetX: 201, TargetY: 200, MapObservedAt: now, QueuedAt: now,
	}}

	decision, err := NewAutoTowerPolicy().Evaluate(t.Context(), snapshot)
	if err != nil || decision.Request == nil || decision.Request.Name != "tower.attack" {
		t.Fatalf("multi-castle tower decision = %#v err=%v", decision, err)
	}
	var arguments struct {
		SourceCastleID State.CastleID `json:"sourceCastleId"`
		TargetX        int            `json:"targetX"`
		TargetY        int            `json:"targetY"`
	}
	if err := json.Unmarshal(decision.Request.Arguments, &arguments); err != nil {
		t.Fatal(err)
	}
	if arguments.SourceCastleID != 2 || arguments.TargetX != 201 || arguments.TargetY != 200 {
		t.Fatalf("tower attack did not bypass capacity-corrected castle: %#v", arguments)
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
