package Ingest

import (
	"encoding/json"
	"testing"
	"time"

	"CitadelDesktop/Server/State"
)

func TestReconcileAttackFeatureBattleReportAttributesConfirmedLaunch(t *testing.T) {
	now := time.Date(2026, 7, 21, 12, 0, 0, 0, time.UTC)
	gameState := State.NewGameState()
	gameState.Player.ID = 11
	State.RecordAttackFeatureLaunch(&gameState, State.AttackFeatureLaunch{
		MovementID: 77, FeatureID: State.AttackFeatureAutoTowers, KingdomID: 0,
		TargetTypeID: 2, TargetX: 101, TargetY: 102,
		LaunchedAt: now.Add(-2 * time.Minute), ArrivesAt: now,
	})
	capture := State.BattleReportCapture{
		MessageID: 10, ReportID: 20, CapturedAt: now.Add(time.Minute),
		Summary: json.RawMessage(`{
			"MID":10,"LID":20,"PBI":[[11,0,100,-1],[22,1,100,-100]],
			"AI":{"AT":2,"K":0,"X":101,"Y":102}
		}`),
		Details: json.RawMessage(`{"LID":20,"Y":[]}`),
	}
	changed, err := reconcileAttackFeatureBattleReport(&gameState, &capture)
	if err != nil {
		t.Fatal(err)
	}
	if !changed || capture.AutomationFeature != State.AttackFeatureAutoTowers || capture.MovementID != 77 {
		t.Fatalf("capture was not attributed: changed=%t feature=%q movement=%d", changed, capture.AutomationFeature, capture.MovementID)
	}
	if len(gameState.AttackAnalytics.PendingAttacks) != 0 {
		t.Fatalf("matched launch was not consumed: %#v", gameState.AttackAnalytics.PendingAttacks)
	}
}

func TestReconcileAttackFeatureBattleReportIgnoresUnattributedAndEventBattles(t *testing.T) {
	now := time.Date(2026, 7, 21, 12, 0, 0, 0, time.UTC)
	gameState := State.NewGameState()
	gameState.Player.ID = 11
	capture := State.BattleReportCapture{
		MessageID: 10, CapturedAt: now,
		Summary: json.RawMessage(`{"MID":10,"PBI":[[11,0,100,-1]],"AI":{"AT":53,"K":0,"X":5,"Y":6}}`),
		Details: json.RawMessage(`{"Y":[]}`),
	}
	changed, err := reconcileAttackFeatureBattleReport(&gameState, &capture)
	if err != nil || changed || capture.AutomationFeature != "" {
		t.Fatalf("unattributed battle changed analytics: changed=%t feature=%q err=%v", changed, capture.AutomationFeature, err)
	}
}
