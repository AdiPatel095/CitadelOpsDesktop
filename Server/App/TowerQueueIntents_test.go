package App

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"CitadelDesktop/Server/Intent"
	"CitadelDesktop/Server/State"
)

func TestTowerQueueScanFocusesOnceThenRefreshesAndCaptures(t *testing.T) {
	gameState := State.NewGameState()
	gameState.Castles[1] = State.CastleState{ID: 1, KingdomID: 0, X: 100, Y: 100}
	plan, err := planTowerQueueScan(context.Background(), Intent.PlanningContext{State: gameState}, json.RawMessage(`{"sourceCastleId":1,"radius":10}`))
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Steps) != 3 || plan.Steps[0].Opcode != "jaa" || plan.Steps[1].Opcode != "gaa" || plan.Steps[2].Action != "tower.queue.capture" {
		t.Fatalf("unexpected tower queue scan: %#v", plan.Steps)
	}
}

func TestTowerQueueScanAcceptsRadiusFifty(t *testing.T) {
	gameState := State.NewGameState()
	gameState.Castles[1] = State.CastleState{ID: 1, KingdomID: 0, X: 100, Y: 100}
	plan, err := planTowerQueueScan(context.Background(), Intent.PlanningContext{State: gameState}, json.RawMessage(`{"sourceCastleId":1,"radius":50}`))
	if err != nil {
		t.Fatalf("radius 50 was rejected: %v", err)
	}
	if len(plan.Steps) != 7 || plan.Steps[0].Opcode != "jaa" || plan.Steps[5].Opcode != "gaa" || plan.Steps[6].Action != "tower.queue.capture" {
		t.Fatalf("radius 50 scan did not use five GAA windows: %#v", plan.Steps)
	}
	if _, err := planTowerQueueScan(context.Background(), Intent.PlanningContext{State: gameState}, json.RawMessage(`{"sourceCastleId":1,"radius":51}`)); err == nil {
		t.Fatal("radius 51 was accepted")
	}
}

func TestTowerMapScanWindowsCoverConfiguredSquareWithoutGaps(t *testing.T) {
	source := State.CastleState{X: 500, Y: 600}
	for _, test := range []struct {
		radius int
		count  int
	}{{20, 1}, {25, 2}, {40, 3}, {49, 4}, {50, 5}} {
		windows := towerMapScanWindows(source, test.radius)
		if len(windows) != test.count {
			t.Fatalf("radius %d window count = %d, want %d", test.radius, len(windows), test.count)
		}
		nextX := source.X - test.radius
		for _, window := range windows {
			if window.X1 != nextX || window.Y1 != source.Y-test.radius || window.Y2 != source.Y+test.radius {
				t.Fatalf("radius %d has a gap or wrong Y coverage: %#v", test.radius, windows)
			}
			nextX = window.X2 + 1
		}
		if nextX != source.X+test.radius+1 {
			t.Fatalf("radius %d ended at X %d", test.radius, nextX-1)
		}
	}
}

func TestTowerQueueCaptureStoresEveryFreshTowerAndConsumeRemovesOne(t *testing.T) {
	now := time.Now().UTC()
	gameState := State.NewGameState()
	gameState.Castles[1] = State.CastleState{ID: 1, KingdomID: 0, X: 100, Y: 100, Focused: true}
	gameState.Map[0] = map[string]State.MapObservation{
		"100:99":  {KingdomID: 0, X: 100, Y: 99, TypeID: kingdomTowerMapTypeID, TowerVictoryCount: 845, Level: 81, ObservedAt: now.Add(-time.Minute)},
		"99:100":  {KingdomID: 0, X: 99, Y: 100, TypeID: kingdomTowerMapTypeID, TowerVictoryCount: 845, Level: 81, ObservedAt: now},
		"100:101": {KingdomID: 0, X: 100, Y: 101, TypeID: kingdomTowerMapTypeID, TowerVictoryCount: 845, Level: 81, ObservedAt: now},
		"101:100": {KingdomID: 0, X: 101, Y: 100, TypeID: kingdomTowerMapTypeID, TowerVictoryCount: 845, Level: 81, ObservedAt: now},
		"101:101": {KingdomID: 0, X: 101, Y: 101, TypeID: kingdomTowerMapTypeID, TowerVictoryCount: 845, Level: 81, TowerCooldownRemaining: 300, ObservedAt: now},
	}
	application := &Application{State: State.NewStore(gameState)}
	arguments, err := json.Marshal(towerQueueScanRequest{SourceCastleID: 1, Radius: 2, ScanStartedAt: now.Add(-time.Second)})
	if err != nil {
		t.Fatal(err)
	}
	if err := application.captureTowerQueue(context.Background(), arguments); err != nil {
		t.Fatal(err)
	}
	entries := application.State.Snapshot().TowerQueue.EntriesByCastle[1]
	if len(entries) != 4 || entries[0].TargetX != 99 || entries[1].TargetX != 101 || entries[2].TargetY != 101 || entries[3].TargetX != 101 || entries[3].TargetY != 101 {
		t.Fatalf("tower queue order = %#v", entries)
	}
	consume, _ := json.Marshal(towerQueueEntryRequest{SourceCastleID: 1, KingdomID: 0, TargetX: 101, TargetY: 100})
	if err := application.consumeTowerQueueEntry(context.Background(), consume); err != nil {
		t.Fatal(err)
	}
	entries = application.State.Snapshot().TowerQueue.EntriesByCastle[1]
	if len(entries) != 3 || entries[0].TargetX != 99 || entries[1].TargetY != 101 {
		t.Fatalf("tower queue after consume = %#v", entries)
	}
}
