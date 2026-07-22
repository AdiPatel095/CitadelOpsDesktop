package Ingest

import (
	"encoding/json"
	"testing"
	"time"

	"CitadelDesktop/Server/State"
)

func TestEventBattleActivitySeparatesCampAndKhanDefenseTotals(t *testing.T) {
	now := time.Date(2026, 7, 20, 20, 0, 0, 0, time.UTC)
	gameState := State.NewGameState()
	gameState.Player.ID = 10
	gameState.EventScores.ByEvent[72] = State.ScalableEventScore{EventID: 72, RemainingSec: 7_200, ObservedAt: now}
	State.RecordEventAttackLaunch(&gameState, 72, State.EventAttackRecord{
		MovementID: 1, Kind: State.EventActivityCamp, KingdomID: 0, TargetTypeID: 27,
		TargetX: 100, TargetY: 101, LaunchedAt: now, ArrivesAt: now.Add(time.Minute),
	})
	camp := State.BattleReportCapture{
		MessageID: 100, ReportID: 200, CapturedAt: now.Add(time.Minute),
		Summary: json.RawMessage(`{"MID":100,"LID":200,"PBI":[[10,0,100,-20],[-601,1,50,-50]],"AI":{"AT":27,"K":0,"X":100,"Y":101}}`),
		Details: json.RawMessage(`{"W":[[[10,[[[1,100,-20]],[[700,10,-10]]] ],[-601,[[],[]]]]],"S":[[10,[403,1,-1]]]}`),
	}
	changed, err := reconcileEventBattleActivity(&gameState, &camp)
	if err != nil || !changed {
		t.Fatalf("camp reconcile: changed=%t err=%v", changed, err)
	}
	if camp.AutomationFeature != State.AttackFeatureAutoNomad || camp.EventID != 72 || camp.EventActivity != State.EventActivityCamp || camp.MovementID != 1 {
		t.Fatalf("camp attribution = %#v", camp)
	}
	activity := gameState.EventScores.ActivityByEvent[72]
	if activity.Camp.Launches != 1 || activity.Camp.Battles != 1 || activity.Camp.Victories != 1 ||
		activity.Camp.TroopLosses != 20 || activity.Camp.ToolsUsed != 11 || len(activity.PendingAttacks) != 0 {
		t.Fatalf("camp activity = %#v", activity.Camp)
	}

	State.RecordEventAttackLaunch(&gameState, 72, State.EventAttackRecord{
		MovementID: 2, Kind: State.EventActivityKhanDefense, KingdomID: 0, TargetTypeID: 1,
		TargetX: 110, TargetY: 111, LaunchedAt: now.Add(2 * time.Minute), ArrivesAt: now.Add(3 * time.Minute),
	})
	defense := State.BattleReportCapture{
		MessageID: 101, ReportID: 201, CapturedAt: now.Add(3 * time.Minute),
		Summary: json.RawMessage(`{"MID":101,"LID":201,"PBI":[[-601,0,50,-50],[10,1,100,-5]],"AI":{"AT":1,"K":0,"X":110,"Y":111}}`),
		Details: json.RawMessage(`{"W":[[[-601,[[],[]]],[10,[[[1,100,-5]],[[701,3,-3]]]]]]}`),
	}
	changed, err = reconcileEventBattleActivity(&gameState, &defense)
	if err != nil || !changed {
		t.Fatalf("defense reconcile: changed=%t err=%v", changed, err)
	}
	if defense.AutomationFeature != State.AttackFeatureAutoKhan || defense.EventID != 72 ||
		defense.EventActivity != State.EventActivityKhanDefense || defense.MovementID != 2 {
		t.Fatalf("Khan defense attribution = %#v", defense)
	}
	activity = gameState.EventScores.ActivityByEvent[72]
	if activity.KhanDefense.Launches != 1 || activity.KhanDefense.Battles != 1 || activity.KhanDefense.Victories != 1 ||
		activity.KhanDefense.TroopLosses != 5 || activity.KhanDefense.ToolsUsed != 3 {
		t.Fatalf("Khan defense activity = %#v", activity.KhanDefense)
	}
}

func TestEventBattleActivityTracksBloodcrowInvasion(t *testing.T) {
	now := time.Date(2026, 7, 21, 14, 0, 0, 0, time.UTC)
	gameState := State.NewGameState()
	gameState.Player.ID = 10
	gameState.EventScores.ByEvent[103] = State.ScalableEventScore{EventID: 103, RemainingSec: 7_200, ObservedAt: now}
	State.RecordEventAttackLaunch(&gameState, 103, State.EventAttackRecord{
		MovementID: 3, Kind: State.EventActivityInvasion, KingdomID: 0, TargetTypeID: 34,
		TargetX: 120, TargetY: 121, LaunchedAt: now, ArrivesAt: now.Add(time.Minute),
	})
	report := State.BattleReportCapture{
		MessageID: 102, ReportID: 202, CapturedAt: now.Add(time.Minute),
		Summary: json.RawMessage(`{"MID":102,"LID":202,"PBI":[[10,0,18560,-765,[["W",184800],["S",184800],["F",171037],["C1",11445],["HONEY",164120],["MEAD",164120]]],[-1002,1,19281,-19281]],"AI":{"AT":34,"K":0,"X":120,"Y":121}}`),
		Details: json.RawMessage(`{"W":[[[10,[[[1,100,-12]],[[702,7,-7]]] ],[-603,[[],[]]]]]}`),
	}
	changed, err := reconcileEventBattleActivity(&gameState, &report)
	if err != nil || !changed {
		t.Fatalf("Bloodcrow reconcile: changed=%t err=%v", changed, err)
	}
	if report.AutomationFeature != State.AttackFeatureAutoInvasion || report.EventID != 103 ||
		report.EventActivity != State.EventActivityInvasion || report.MovementID != 3 {
		t.Fatalf("Bloodcrow attribution = %#v", report)
	}
	totals := gameState.EventScores.ActivityByEvent[103].Invasion
	if totals.Launches != 1 || totals.Battles != 1 || totals.Victories != 1 ||
		totals.TroopLosses != 765 || totals.ToolsUsed != 7 || totals.Loot != 880_322 {
		t.Fatalf("Bloodcrow activity = %#v", totals)
	}
}
