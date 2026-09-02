package Reports

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"CitadelDesktop/Server/State"
)

func TestResourceAggregatesSumReportsByViewAndUTCMinute(t *testing.T) {
	store, err := OpenSQLiteStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	minute := time.Date(2026, 9, 1, 18, 42, 0, 0, time.UTC)
	reports := []BattleReport{
		resourceAggregateTestReport("tower-a", State.AttackFeatureAutoTowers, minute.Add(5*time.Second), 41, map[string]int64{"W": 10, "S": 4}),
		resourceAggregateTestReport("tower-b", State.AttackFeatureAutoTowers, minute.Add(55*time.Second), 41, map[string]int64{
			"W": 7, "C1": 3, "HONEY": 11, "EVENT_REWARD": 13,
		}),
		resourceAggregateTestReport("storm-a", State.AttackFeatureAutoStorm, minute.Add(20*time.Second), 41, map[string]int64{"IAP": 8}),
	}
	if err := store.SaveMany(t.Context(), reports); err != nil {
		t.Fatal(err)
	}
	tower, err := store.ResourceAggregates(t.Context(), ResourceAggregateQuery{
		AccountUID: 9001, WorldID: "https://WORLD.example/", PlayerID: 41,
		ViewKey: ResourceViewTower,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(tower) != 1 || tower[0].ReportCount != 2 || tower[0].BucketSeconds != 60 ||
		!tower[0].BucketStart.Equal(minute) || tower[0].Resources["W"] != 17 ||
		tower[0].Resources["S"] != 4 || tower[0].Resources["C1"] != 3 ||
		tower[0].Resources["HONEY"] != 11 || tower[0].Resources["EVENT_REWARD"] != 13 {
		t.Fatalf("tower minute = %+v", tower)
	}
	storm, err := store.ResourceAggregates(t.Context(), ResourceAggregateQuery{
		AccountUID: 9001, WorldID: "world.example", PlayerID: 41,
		ViewKey: ResourceViewStorm,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(storm) != 1 || storm[0].ReportCount != 1 || storm[0].Resources["IAP"] != 8 {
		t.Fatalf("storm minute = %+v", storm)
	}
	pending, err := store.PendingResourceAggregates(t.Context(), ResourceAggregateOutboxQuery{
		AccountUID: 9001, WorldID: "world.example", PlayerID: 41, Limit: 100,
	})
	if err != nil {
		t.Fatal(err)
	}
	foundTowerRollup := false
	for _, item := range pending {
		if item.Aggregate.ViewKey != ResourceViewTower || len(item.Rollups) != 2 {
			continue
		}
		if item.Rollups[0].BucketSeconds == 3600 && item.Rollups[0].ReportCount == 2 &&
			item.Rollups[0].Resources["W"] == 17 && item.Rollups[0].Resources["HONEY"] == 11 &&
			item.Rollups[0].Resources["EVENT_REWARD"] == 13 {
			foundTowerRollup = true
		}
	}
	if !foundTowerRollup {
		t.Fatalf("tower absolute rollup missing from outbox: %+v", pending)
	}
	status, err := store.ResourceAggregateMigrationStatus(t.Context(), ResourceAggregateOutboxQuery{
		AccountUID: 9001, WorldID: "world.example", PlayerID: 41,
	})
	if err != nil {
		t.Fatal(err)
	}
	if status.SourceReports != 3 || status.SourceBuckets != 2 || status.PendingBuckets != 2 ||
		!status.OldestOccurredAt.Equal(minute.Add(5*time.Second)) ||
		!status.NewestOccurredAt.Equal(minute.Add(55*time.Second)) ||
		!status.OldestPendingBucket.Equal(minute) || !status.NewestPendingBucket.Equal(minute) {
		t.Fatalf("migration status before acknowledgement = %+v", status)
	}
	if err := store.AcknowledgeResourceAggregates(t.Context(), pending); err != nil {
		t.Fatal(err)
	}
	status, err = store.ResourceAggregateMigrationStatus(t.Context(), ResourceAggregateOutboxQuery{
		AccountUID: 9001, WorldID: "world.example", PlayerID: 41,
	})
	if err != nil {
		t.Fatal(err)
	}
	if status.SourceReports != 3 || status.SourceBuckets != 2 || status.PendingBuckets != 0 ||
		!status.OldestPendingBucket.IsZero() || !status.NewestPendingBucket.IsZero() {
		t.Fatalf("migration status after acknowledgement = %+v", status)
	}
}

func TestResourceAggregateReprocessingReplacesContributionWithoutDoubleCounting(t *testing.T) {
	store, err := OpenSQLiteStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	minute := time.Date(2026, 9, 1, 19, 12, 0, 0, time.UTC)
	report := resourceAggregateTestReport("corrected", State.AttackFeatureAutoTowers, minute.Add(4*time.Second), 41, map[string]int64{"W": 10})
	if err := store.Save(t.Context(), report); err != nil {
		t.Fatal(err)
	}
	report.Loot = map[string]int64{"W": 25, "S": 2}
	report.Metrics.AttackerLost = 3
	if err := store.Save(t.Context(), report); err != nil {
		t.Fatal(err)
	}
	rows, err := store.ResourceAggregates(t.Context(), ResourceAggregateQuery{
		AccountUID: 9001, WorldID: "world.example", PlayerID: 41, ViewKey: ResourceViewTower,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].ReportCount != 1 || rows[0].Resources["W"] != 25 ||
		rows[0].Resources["S"] != 2 || rows[0].TroopLosses != 3 {
		t.Fatalf("corrected minute = %+v", rows)
	}
}

func TestResourceAggregateMoveQueuesOldMinuteTombstone(t *testing.T) {
	store, err := OpenSQLiteStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	firstMinute := time.Date(2026, 9, 1, 20, 1, 0, 0, time.UTC)
	report := resourceAggregateTestReport("moved", State.AttackFeatureAutoTowers, firstMinute.Add(10*time.Second), 41, map[string]int64{"W": 6})
	if err := store.Save(t.Context(), report); err != nil {
		t.Fatal(err)
	}
	pending, err := store.PendingResourceAggregates(t.Context(), ResourceAggregateOutboxQuery{
		AccountUID: 9001, WorldID: "world.example", PlayerID: 41, Limit: 100,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.AcknowledgeResourceAggregates(t.Context(), pending); err != nil {
		t.Fatal(err)
	}
	report.OccurredAt = firstMinute.Add(time.Hour + 5*time.Second).Format(time.RFC3339Nano)
	report.DateMs = 0
	if err := store.Save(t.Context(), report); err != nil {
		t.Fatal(err)
	}
	rows, err := store.ResourceAggregates(t.Context(), ResourceAggregateQuery{
		AccountUID: 9001, WorldID: "world.example", PlayerID: 41, ViewKey: ResourceViewTower,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || !rows[0].BucketStart.Equal(firstMinute.Add(time.Hour)) {
		t.Fatalf("moved minute rows = %+v", rows)
	}
	pending, err = store.PendingResourceAggregates(t.Context(), ResourceAggregateOutboxQuery{
		AccountUID: 9001, WorldID: "world.example", PlayerID: 41, Limit: 100,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(pending) != 2 {
		t.Fatalf("pending updates = %+v", pending)
	}
	seenDelete := false
	seenInsert := false
	seenEmptyHour := false
	for _, item := range pending {
		if item.Aggregate.Deleted && item.Aggregate.BucketStart.Equal(firstMinute) {
			seenDelete = true
			seenEmptyHour = len(item.Rollups) == 2 && item.Rollups[0].Deleted &&
				item.Rollups[0].BucketStart.Equal(firstMinute.Truncate(time.Hour))
		}
		if !item.Aggregate.Deleted && item.Aggregate.BucketStart.Equal(firstMinute.Add(time.Hour)) {
			seenInsert = true
		}
	}
	if !seenDelete || !seenInsert || !seenEmptyHour {
		t.Fatalf("move outbox = %+v", pending)
	}
}

func TestResourceAggregateRebuildAndWorldPlayerIdentitySurviveUIDChange(t *testing.T) {
	dataDir := t.TempDir()
	store, err := OpenSQLiteStore(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	minute := time.Date(2026, 9, 1, 21, 9, 0, 0, time.UTC)
	report := resourceAggregateTestReport("uid-change", State.AttackFeatureAutoTowers, minute.Add(15*time.Second), 41, map[string]int64{"W": 12})
	if err := store.Save(t.Context(), report); err != nil {
		t.Fatal(err)
	}
	report.AccountUID = 9002
	report.Loot = map[string]int64{"W": 14}
	if err := store.Save(t.Context(), report); err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.ExecContext(t.Context(), `
		DELETE FROM feature_resource_aggregates;
		DELETE FROM feature_resource_aggregate_outbox;
		DELETE FROM battle_report_storage_metadata WHERE key IN (?, ?)
	`, resourceAggregateSchemaKey, resourceAggregateWatermarkKey); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	store, err = OpenSQLiteStore(filepath.Clean(dataDir))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	rows, err := store.ResourceAggregates(context.Background(), ResourceAggregateQuery{
		AccountUID: 9002, WorldID: "WORLD.EXAMPLE", PlayerID: 41, ViewKey: ResourceViewTower,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].ReportCount != 1 || rows[0].Resources["W"] != 14 {
		t.Fatalf("rebuilt UID-bound minute = %+v", rows)
	}
}

func TestResourceAggregatesMergeLegacyUIDAndStableIdentityBeforeCloudDelivery(t *testing.T) {
	store, err := OpenSQLiteStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	minute := time.Date(2026, 9, 1, 21, 12, 0, 0, time.UTC)
	legacy := resourceAggregateTestReport(
		"legacy-uid-only", State.AttackFeatureAutoTowers, minute.Add(5*time.Second), 0,
		map[string]int64{"W": 7},
	)
	legacy.WorldID = ""
	legacy.PlayerID = 0
	if err := store.Save(t.Context(), legacy); err != nil {
		t.Fatal(err)
	}
	stable := resourceAggregateTestReport(
		"stable-world-player", State.AttackFeatureAutoTowers, minute.Add(35*time.Second), 41,
		map[string]int64{"W": 11, "S": 3},
	)
	if err := store.Save(t.Context(), stable); err != nil {
		t.Fatal(err)
	}

	query := ResourceAggregateQuery{
		AccountUID: 9001, WorldID: "world.example", PlayerID: 41, ViewKey: ResourceViewTower,
	}
	rows, err := store.ResourceAggregates(t.Context(), query)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].ReportCount != 2 || rows[0].Resources["W"] != 18 || rows[0].Resources["S"] != 3 {
		t.Fatalf("merged local identity minute = %+v", rows)
	}

	pending, err := store.PendingResourceAggregates(t.Context(), ResourceAggregateOutboxQuery{
		AccountUID: query.AccountUID, WorldID: query.WorldID, PlayerID: query.PlayerID, Limit: 100,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(pending) != 2 {
		t.Fatalf("identity outbox rows = %+v", pending)
	}
	for _, item := range pending {
		if item.Aggregate.ReportCount != 2 || item.Aggregate.Resources["W"] != 18 || item.Aggregate.Resources["S"] != 3 {
			t.Fatalf("merged cloud source minute = %+v", item.Aggregate)
		}
		if len(item.Rollups) != 2 || item.Rollups[0].ReportCount != 2 || item.Rollups[1].ReportCount != 2 {
			t.Fatalf("merged cloud rollups = %+v", item.Rollups)
		}
	}
}

func TestResourceAggregateMigrationAcknowledgementSurvivesReopen(t *testing.T) {
	dataDir := t.TempDir()
	store, err := OpenSQLiteStore(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	minute := time.Date(2026, 8, 22, 4, 18, 0, 0, time.UTC)
	if err := store.Save(t.Context(), resourceAggregateTestReport(
		"durable-ack", State.AttackFeatureAutoTowers, minute.Add(8*time.Second), 41, map[string]int64{"W": 9},
	)); err != nil {
		t.Fatal(err)
	}
	pending, err := store.PendingResourceAggregates(t.Context(), ResourceAggregateOutboxQuery{
		AccountUID: 9001, WorldID: "world.example", PlayerID: 41, Limit: 100,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(pending) != 1 {
		t.Fatalf("pending migration before acknowledgement = %+v", pending)
	}
	if err := store.AcknowledgeResourceAggregates(t.Context(), pending); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	store, err = OpenSQLiteStore(filepath.Clean(dataDir))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	status, err := store.ResourceAggregateMigrationStatus(t.Context(), ResourceAggregateOutboxQuery{
		AccountUID: 9001, WorldID: "world.example", PlayerID: 41,
	})
	if err != nil {
		t.Fatal(err)
	}
	if status.SourceReports != 1 || status.SourceBuckets != 1 || status.PendingBuckets != 0 ||
		!status.OldestOccurredAt.Equal(minute.Add(8*time.Second)) {
		t.Fatalf("migration receipt after reopen = %+v", status)
	}
}

func resourceAggregateTestReport(
	id string,
	feature State.AttackFeatureID,
	occurredAt time.Time,
	playerID int64,
	loot map[string]int64,
) BattleReport {
	return BattleReport{
		ID: id, ReportID: id, AccountUID: 9001, WorldID: "world.example", PlayerID: playerID,
		AutomationFeature: string(feature), OccurredAt: occurredAt.Format(time.RFC3339Nano),
		Result: "Victory", Role: "attacker", Attacker: &BattleCombatant{PlayerID: playerID},
		Defender: &BattleCombatant{PlayerID: -1, Dummy: true},
		Metrics:  BattleMetrics{AttackerSent: 100, AttackerLost: 1}, ToolsUsed: 2,
		GallantryPoints: 3, Loot: loot,
	}
}
