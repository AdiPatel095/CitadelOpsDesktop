package Reports

import (
	"context"
	"testing"
	"time"

	"CitadelDesktop/Server/History"
	"CitadelDesktop/Server/State"
)

func TestSQLiteStorePersistsScopedFeatureAndEventReport(t *testing.T) {
	store, err := OpenSQLiteStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	report := BattleReport{
		ID: "10-20", ReportID: "10-20", AccountUID: 44, WorldID: "world", PlayerID: 7,
		MID: 10, LID: 20, MovementID: 30, AutomationFeature: string(State.AttackFeatureAutoInvasion),
		EventID: 103, EventActivity: string(State.EventActivityInvasion),
		EventOccurrenceEndsAt: time.Date(2026, 7, 24, 8, 0, 0, 0, time.UTC).Format(time.RFC3339Nano),
		OccurredAt:            time.Date(2026, 7, 22, 17, 0, 0, 0, time.UTC).Format(time.RFC3339),
		DateMs:                time.Date(2026, 7, 22, 17, 0, 0, 0, time.UTC).UnixMilli(),
		Result:                "Victory", Role: "attacker", Metrics: BattleMetrics{AttackerLost: 12}, ToolsUsed: 9,
		Loot: map[string]int64{"W": 100, "S": 200, "C1": 30},
	}
	if err := store.Save(context.Background(), report); err != nil {
		t.Fatal(err)
	}
	reports, err := store.Recent(context.Background(), BattleReportQuery{
		AccountUID: 44, FeatureID: string(State.AttackFeatureAutoInvasion), EventID: 103, Limit: 10,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(reports) != 1 || reports[0].MovementID != 30 || reports[0].ToolsUsed != 9 || reports[0].Loot["S"] != 200 {
		t.Fatalf("stored reports = %#v", reports)
	}
	otherAccount, err := store.Recent(context.Background(), BattleReportQuery{AccountUID: 45, Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(otherAccount) != 0 {
		t.Fatalf("other account received reports = %#v", otherAccount)
	}
	refreshedUID, err := store.Recent(context.Background(), BattleReportQuery{
		AccountUID: 45, WorldID: "world", PlayerID: 7, Limit: 10,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(refreshedUID) != 1 {
		t.Fatalf("refreshed UID did not retain the verified world/player history = %#v", refreshedUID)
	}
	report.AccountUID = 45
	if err := store.Save(context.Background(), report); err != nil {
		t.Fatal(err)
	}
	refreshedUID, err = store.Recent(context.Background(), BattleReportQuery{
		AccountUID: 45, WorldID: "world", PlayerID: 7, Limit: 10,
	})
	if err != nil || len(refreshedUID) != 1 {
		t.Fatalf("consolidated UID reports = %#v, err=%v", refreshedUID, err)
	}
}

func TestBackfillBattleHistorySafelyAttributesCurrentInvasion(t *testing.T) {
	dataDir := t.TempDir()
	history, err := History.Open(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	report := BattleReport{
		ID: "100-200", ReportID: "100-200", MID: 100, LID: 200, PlayerID: 7,
		OccurredAt: time.Date(2026, 7, 22, 17, 0, 0, 0, time.UTC).Format(time.RFC3339),
		Result:     "Victory", Role: "attacker", Attacker: &BattleCombatant{PlayerID: 7},
		Loot: map[string]int64{"W": 100},
	}
	if err := history.Append(History.CollectionBattleReports, report); err != nil {
		t.Fatal(err)
	}
	store, err := OpenSQLiteStore(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	snapshot := State.NewGameState()
	snapshot.Account = State.AccountBindingState{UID: 55, WorldID: "world", PlayerID: 7}
	snapshot.Player.ID = 7
	snapshot.EventScores.ActivityByEvent[103] = State.EventActivityState{
		EventID:            103,
		OccurrenceEndsAt:   time.Date(2026, 7, 24, 8, 0, 0, 0, time.UTC),
		ProcessedReportIDs: []int64{200},
	}
	if err := BackfillBattleHistory(context.Background(), history, store, snapshot); err != nil {
		t.Fatal(err)
	}
	reports, err := store.Recent(context.Background(), BattleReportQuery{AccountUID: 55, Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(reports) != 1 || reports[0].AutomationFeature != string(State.AttackFeatureAutoInvasion) ||
		reports[0].EventActivity != string(State.EventActivityInvasion) {
		t.Fatalf("backfilled reports = %#v", reports)
	}
}
