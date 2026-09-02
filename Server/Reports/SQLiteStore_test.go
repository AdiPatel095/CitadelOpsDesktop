package Reports

import (
	"context"
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"CitadelDesktop/Server/History"
	"CitadelDesktop/Server/Intent"
	"CitadelDesktop/Server/State"
)

func TestReportWritesAreIsolatedFromOperationJournal(t *testing.T) {
	dataDir := t.TempDir()
	operationStore, err := Intent.OpenOperationStore(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = operationStore.Close() })
	reportStore, err := OpenSQLiteStore(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = reportStore.Close() })
	for _, name := range []string{"Operations.sqlite", "Reports.sqlite"} {
		if _, err := os.Stat(filepath.Join(dataDir, "Runtime", name)); err != nil {
			t.Fatalf("missing %s: %v", name, err)
		}
	}
	reportTx, err := reportStore.db.BeginTx(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer reportTx.Rollback()
	if _, err := reportTx.ExecContext(t.Context(), `
		INSERT INTO battle_report_storage_metadata (key, value) VALUES ('test_lock', 'held')
	`); err != nil {
		t.Fatal(err)
	}
	receipt := Intent.Receipt{
		ID: "isolated-operation", Intent: "test.write", Actor: "test",
		Status: Intent.StatusPlanning, Phase: Intent.EffectPhaseAccepted, SubmittedAt: time.Now().UTC(),
	}
	if _, created, err := operationStore.Reserve(t.Context(), "request-hash", receipt); err != nil || !created {
		t.Fatalf("operation reservation while report writer held lock: created=%t err=%v", created, err)
	}
}

func TestResourceAggregateSchemaAddsOccurrenceBoundsToEarlyDatabase(t *testing.T) {
	dataDir := t.TempDir()
	store, err := OpenSQLiteStore(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", filepath.Join(dataDir, "Runtime", "Reports.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(t.Context(), `
		DROP TABLE feature_resource_aggregates;
		CREATE TABLE feature_resource_aggregates (
			identity_key TEXT NOT NULL, view_key TEXT NOT NULL, bucket_start INTEGER NOT NULL,
			bucket_seconds INTEGER NOT NULL, report_count INTEGER NOT NULL, victories INTEGER NOT NULL,
			defeats INTEGER NOT NULL, troops_sent INTEGER NOT NULL, troop_losses INTEGER NOT NULL,
			tools_used INTEGER NOT NULL, gallantry_points INTEGER NOT NULL, loot_total INTEGER NOT NULL,
			resources_json BLOB NOT NULL, revision INTEGER NOT NULL, updated_at TEXT NOT NULL,
			PRIMARY KEY (identity_key, view_key, bucket_start)
		);
		DELETE FROM battle_report_storage_metadata WHERE key IN (?, ?);
	`, resourceAggregateSchemaKey, resourceAggregateWatermarkKey); err != nil {
		db.Close()
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	store, err = OpenSQLiteStore(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	columns, err := store.db.QueryContext(t.Context(), `PRAGMA table_info(feature_resource_aggregates)`)
	if err != nil {
		t.Fatal(err)
	}
	defer columns.Close()
	found := map[string]bool{}
	for columns.Next() {
		var sequence, notNull, primaryKey int
		var name, columnType string
		var defaultValue any
		if err := columns.Scan(&sequence, &name, &columnType, &notNull, &defaultValue, &primaryKey); err != nil {
			t.Fatal(err)
		}
		found[name] = true
	}
	if !found["first_occurred_at"] || !found["last_occurred_at"] {
		t.Fatalf("migrated feature resource aggregate columns = %+v", found)
	}
}

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
		Result:                "Victory", Role: "attacker",
		Metrics: BattleMetrics{AttackerSent: 1_250, AttackerLost: 12}, ToolsUsed: 9,
		GallantryPoints: 1743, Loot: map[string]int64{"W": 100, "S": 200, "C1": 30},
		KingdomID: 4, TargetX: 100, TargetY: 200, TargetName: "Event target",
		BattleTypeID: 6, BattleType: "Type 6", TargetTypeID: 24, TargetType: "Type 24",
		Defender: &BattleCombatant{PlayerID: -50, Dummy: true, Name: "Event target"},
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
	if len(reports) != 1 || reports[0].MovementID != 30 || reports[0].TroopsSent != 1_250 || reports[0].ToolsUsed != 9 ||
		reports[0].GallantryPoints != 1743 || reports[0].Loot["S"] != 200 ||
		reports[0].TargetPlayerID != -50 || reports[0].TargetName != "Event target" ||
		reports[0].TargetTypeID != 24 || reports[0].TargetType != "Type 24" ||
		reports[0].KingdomID != 4 || reports[0].TargetX != 100 || reports[0].TargetY != 200 {
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

func TestSQLiteStoreExcludesPvPAndHasNoCanonicalReportColumn(t *testing.T) {
	store, err := OpenSQLiteStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	report := BattleReport{
		ID: "10-20", ReportID: "10-20", AccountUID: 44, PlayerID: 1,
		MID: 10, LID: 20, DateMs: time.Now().UnixMilli(), OccurredAt: time.Now().UTC().Format(time.RFC3339),
		Attacker: &BattleCombatant{PlayerID: 1},
		Defender: &BattleCombatant{PlayerID: 2},
	}
	if err := store.Save(context.Background(), report); err != nil {
		t.Fatal(err)
	}
	reports, err := store.Recent(context.Background(), BattleReportQuery{AccountUID: 44, Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(reports) != 0 {
		t.Fatalf("PvP reports stored in analytics = %#v", reports)
	}
	columns, err := battleAnalyticsColumns(context.Background(), store.db)
	if err != nil {
		t.Fatal(err)
	}
	if columns["report_json"] || !columns["gallantry_points"] || !columns["troops_sent"] ||
		!columns["target_name"] || !columns["target_type_id"] {
		t.Fatalf("compact analytics columns = %#v", columns)
	}
}

func TestSQLiteStoreAddsGallantryColumnToExistingCompactSchema(t *testing.T) {
	dataDir := t.TempDir()
	store, err := OpenSQLiteStore(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	report := BattleReport{
		ID: "10-20", ReportID: "10-20", AccountUID: 44, PlayerID: 1,
		MID: 10, LID: 20, DateMs: time.Now().UnixMilli(), OccurredAt: time.Now().UTC().Format(time.RFC3339),
		Attacker: &BattleCombatant{PlayerID: 1},
		Defender: &BattleCombatant{PlayerID: -2, Dummy: true},
		Loot:     map[string]int64{"W": 100},
	}
	if err := store.Save(context.Background(), report); err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.Exec(`ALTER TABLE battle_report_analytics DROP COLUMN gallantry_points`); err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.Exec(`ALTER TABLE battle_report_analytics DROP COLUMN troops_sent`); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	store, err = OpenSQLiteStore(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	columns, err := battleAnalyticsColumns(context.Background(), store.db)
	if err != nil {
		t.Fatal(err)
	}
	reports, err := store.Recent(context.Background(), BattleReportQuery{AccountUID: 44, Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if !columns["gallantry_points"] || !columns["troops_sent"] || len(reports) != 1 ||
		reports[0].GallantryPoints != 0 || reports[0].TroopsSent != 0 || reports[0].Loot["W"] != 100 {
		t.Fatalf("migrated compact analytics: columns=%#v reports=%#v", columns, reports)
	}
}

func TestSQLiteStoreClearsLegacyBattleMessageTargetTypeOnlyOnce(t *testing.T) {
	dataDir := t.TempDir()
	store, err := OpenSQLiteStore(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	report := BattleReport{
		ID: "10-20", ReportID: "10-20", AccountUID: 44, PlayerID: 1,
		MID: 10, LID: 20, DateMs: time.Now().UnixMilli(), OccurredAt: time.Now().UTC().Format(time.RFC3339),
		TargetTypeID: 6, TargetType: "Type 6",
		Attacker: &BattleCombatant{PlayerID: 1},
		Defender: &BattleCombatant{PlayerID: -2, Dummy: true},
	}
	if err := store.Save(context.Background(), report); err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.Exec(`DELETE FROM battle_report_storage_metadata WHERE key = 'target_type_source'`); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	store, err = OpenSQLiteStore(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	reports, err := store.Recent(context.Background(), BattleReportQuery{AccountUID: 44, Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(reports) != 1 || reports[0].TargetTypeID != 0 || reports[0].TargetType != "" {
		t.Fatalf("legacy target type was not cleared = %#v", reports)
	}
	if err := store.Save(context.Background(), report); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	store, err = OpenSQLiteStore(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	reports, err = store.Recent(context.Background(), BattleReportQuery{AccountUID: 44, Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(reports) != 1 || reports[0].TargetTypeID != 6 || reports[0].TargetType != "Type 6" {
		t.Fatalf("verified target type did not survive reopen = %#v", reports)
	}
}

func TestSQLiteStoreMigratesCanonicalRowsToPvEAndCloudOutbox(t *testing.T) {
	dataDir := t.TempDir()
	runtimeDir := filepath.Join(dataDir, "Runtime")
	if err := os.MkdirAll(runtimeDir, 0o700); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", filepath.Join(runtimeDir, "Operations.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`
		CREATE TABLE battle_report_analytics (
			account_key TEXT NOT NULL,
			report_key TEXT NOT NULL,
			account_uid INTEGER NOT NULL DEFAULT 0,
			world_id TEXT NOT NULL DEFAULT '',
			player_id INTEGER NOT NULL DEFAULT 0,
			message_id INTEGER NOT NULL DEFAULT 0,
			battle_report_id INTEGER NOT NULL DEFAULT 0,
			movement_id INTEGER NOT NULL DEFAULT 0,
			automation_feature TEXT NOT NULL DEFAULT '',
			event_id INTEGER NOT NULL DEFAULT 0,
			event_activity TEXT NOT NULL DEFAULT '',
			event_occurrence_ends_at TEXT NOT NULL DEFAULT '',
			occurred_at TEXT NOT NULL,
			result TEXT NOT NULL DEFAULT '',
			role TEXT NOT NULL DEFAULT '',
			own_troop_losses INTEGER NOT NULL DEFAULT 0,
			tools_used INTEGER NOT NULL DEFAULT 0,
			loot_total INTEGER NOT NULL DEFAULT 0,
			loot_json BLOB NOT NULL,
			report_json BLOB NOT NULL,
			updated_at TEXT NOT NULL,
			PRIMARY KEY (account_key, report_key)
		)
	`); err != nil {
		t.Fatal(err)
	}
	battleTime := time.Date(2026, 7, 25, 12, 0, 0, 0, time.UTC)
	pve := BattleReport{
		ID: "10-20", ReportID: "10-20", AccountUID: 44, PlayerID: 1,
		MID: 10, LID: 20, DateMs: battleTime.UnixMilli(), OccurredAt: battleTime.Format(time.RFC3339),
		Result: "Victory", Role: "attacker", AutomationFeature: string(State.AttackFeatureAutoTowers),
		KingdomID: 4, TargetX: 10, TargetY: 20, TargetName: "Tower",
		BattleType: "Type 6", Defender: &BattleCombatant{PlayerID: -2, Dummy: true, Name: "Tower"},
		GallantryPoints: 1743, Metrics: BattleMetrics{AttackerSent: 975}, Loot: map[string]int64{"W": 100},
	}
	pvp := BattleReport{
		ID: "11-21", ReportID: "11-21", AccountUID: 44, PlayerID: 1,
		MID: 11, LID: 21, DateMs: battleTime.Add(time.Minute).UnixMilli(),
		OccurredAt: battleTime.Add(time.Minute).Format(time.RFC3339), Result: "Victory", Role: "attacker",
		Attacker: &BattleCombatant{PlayerID: 1},
		Defender: &BattleCombatant{PlayerID: 2},
	}
	insertLegacy := func(report BattleReport) {
		reportPayload, marshalErr := json.Marshal(report)
		if marshalErr != nil {
			t.Fatal(marshalErr)
		}
		lootPayload, marshalErr := json.Marshal(report.Loot)
		if marshalErr != nil {
			t.Fatal(marshalErr)
		}
		_, insertErr := db.Exec(`
			INSERT INTO battle_report_analytics (
				account_key, report_key, account_uid, world_id, player_id,
				message_id, battle_report_id, movement_id, automation_feature,
				event_id, event_activity, event_occurrence_ends_at, occurred_at,
				result, role, own_troop_losses, tools_used, loot_total,
				loot_json, report_json, updated_at
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		`, "uid:44", report.ID, int64(44), "", int64(1), report.MID, report.LID,
			report.MovementID, report.AutomationFeature, report.EventID, report.EventActivity,
			report.EventOccurrenceEndsAt, report.OccurredAt, report.Result, report.Role,
			ownTroopLosses(report), report.ToolsUsed, reportLootTotal(report),
			lootPayload, reportPayload, battleTime.Format(time.RFC3339Nano))
		if insertErr != nil {
			t.Fatal(insertErr)
		}
	}
	insertLegacy(pve)
	insertLegacy(pvp)
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	store, err := OpenSQLiteStore(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	reports, err := store.Recent(context.Background(), BattleReportQuery{AccountUID: 44, Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(reports) != 1 || reports[0].LID != 20 || reports[0].GallantryPoints != 1743 ||
		reports[0].TroopsSent != 975 ||
		reports[0].TargetName != "Tower" ||
		reports[0].TargetTypeID != 0 || reports[0].KingdomID != 4 ||
		reports[0].TargetX != 10 || reports[0].TargetY != 20 {
		t.Fatalf("migrated PvE analytics = %#v", reports)
	}
	aggregates, err := store.ResourceAggregates(context.Background(), ResourceAggregateQuery{
		AccountUID: 44, PlayerID: 1, ViewKey: ResourceViewTower,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(aggregates) != 1 || aggregates[0].ReportCount != 1 || aggregates[0].Resources["W"] != 100 ||
		aggregates[0].TroopsSent != 975 || aggregates[0].GallantryPoints != 1743 ||
		!aggregates[0].BucketStart.Equal(battleTime.Truncate(time.Minute)) {
		t.Fatalf("legacy PvE minute migration = %+v", aggregates)
	}
	migration, err := store.ResourceAggregateMigrationStatus(context.Background(), ResourceAggregateOutboxQuery{
		AccountUID: 44, PlayerID: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if migration.SourceReports != 1 || migration.SourceBuckets != 1 || migration.PendingBuckets != 1 ||
		!migration.OldestOccurredAt.Equal(battleTime) || !migration.NewestOccurredAt.Equal(battleTime) {
		t.Fatalf("legacy PvE migration receipt = %+v", migration)
	}
	pending, err := store.PendingCloudReports(context.Background(), 25)
	if err != nil {
		t.Fatal(err)
	}
	if len(pending) != 1 || pending[0].LID != 21 {
		t.Fatalf("migrated PvP outbox = %#v", pending)
	}
	columns, err := battleAnalyticsColumns(context.Background(), store.db)
	if err != nil {
		t.Fatal(err)
	}
	if columns["report_json"] || !columns["gallantry_points"] || !columns["troops_sent"] {
		t.Fatalf("unexpected compact migration columns: %#v", columns)
	}
}

func TestAnalyticsGenerationAdvancesOnlyOnCommittedAnalyticsWrites(t *testing.T) {
	store, err := OpenSQLiteStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	initial := store.AnalyticsGeneration()
	report := BattleReport{
		ID: "generation-1", AccountUID: 7001, WorldID: "world.example", PlayerID: 42,
		AutomationFeature: "autoTowers", OccurredAt: time.Now().UTC().Format(time.RFC3339Nano),
		Result: "Victory", Role: "attacker", Attacker: &BattleCombatant{PlayerID: 42},
	}
	if err := store.Save(context.Background(), report); err != nil {
		t.Fatal(err)
	}
	afterSave := store.AnalyticsGeneration()
	if afterSave <= initial {
		t.Fatalf("generation did not advance after Save: %d -> %d", initial, afterSave)
	}
	if err := store.Save(context.Background(), BattleReport{}); err == nil {
		t.Fatal("empty report was accepted")
	}
	pvp := report
	pvp.ID = "generation-pvp"
	pvp.Defender = &BattleCombatant{PlayerID: 77}
	if err := store.Save(context.Background(), pvp); err != nil {
		t.Fatal(err)
	}
	if store.AnalyticsGeneration() != afterSave {
		t.Fatalf("generation advanced without an analytics write: %d -> %d", afterSave, store.AnalyticsGeneration())
	}
	second := report
	second.ID = "generation-2"
	if err := store.SaveMany(context.Background(), []BattleReport{report, second}); err != nil {
		t.Fatal(err)
	}
	if store.AnalyticsGeneration() != afterSave+1 {
		t.Fatalf("SaveMany generation = %d, want %d", store.AnalyticsGeneration(), afterSave+1)
	}
	if err := store.SaveMany(context.Background(), nil); err != nil {
		t.Fatal(err)
	}
	if store.AnalyticsGeneration() != afterSave+1 {
		t.Fatal("empty SaveMany advanced the analytics generation")
	}
}

func TestRecentFeatureAnalyticsMatchesRecentAttackerRows(t *testing.T) {
	store, err := OpenSQLiteStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	now := time.Now().UTC()
	reports := []BattleReport{
		{
			ID: "feature-1", AccountUID: 7001, WorldID: "world.example", PlayerID: 42,
			AutomationFeature: "autoTowers", OccurredAt: now.Add(-time.Hour).Format(time.RFC3339Nano),
			Result: "Victory", Role: "attacker", Attacker: &BattleCombatant{PlayerID: 42},
			Metrics: BattleMetrics{AttackerSent: 300, AttackerLost: 5}, ToolsUsed: 2, GallantryPoints: 1,
			Loot: map[string]int64{"resource:1": 40, "resource:2": 2},
		},
		{
			// A defender report is never part of the feature projection.
			ID: "feature-2", AccountUID: 7001, WorldID: "world.example", PlayerID: 42,
			AutomationFeature: "autoTowers", OccurredAt: now.Add(-30 * time.Minute).Format(time.RFC3339Nano),
			Result: "Defeat", Role: "defender", Defender: &BattleCombatant{PlayerID: 42},
		},
		{
			// Bound only by world and player (legacy row without an account UID)
			// must still be included through the account binding fallback.
			ID: "feature-3", WorldID: "world.example", PlayerID: 42,
			AutomationFeature: "autoStorm", OccurredAt: now.Add(-2 * time.Hour).Format(time.RFC3339Nano),
			Result: "Defeat", Role: "attacker", Attacker: &BattleCombatant{PlayerID: 42},
			Metrics: BattleMetrics{AttackerSent: 120, AttackerLost: 120}, Loot: map[string]int64{},
		},
	}
	if err := store.SaveMany(context.Background(), reports); err != nil {
		t.Fatal(err)
	}
	// Re-saving feature-3 under the account UID must consolidate the binding
	// so the report appears exactly once with the account-bound values.
	rebound := reports[2]
	rebound.AccountUID = 7001
	rebound.Metrics.AttackerLost = 100
	if err := store.Save(context.Background(), rebound); err != nil {
		t.Fatal(err)
	}
	query := BattleReportQuery{AccountUID: 7001, WorldID: "world.example", PlayerID: 42, Limit: 10}
	complete, err := store.Recent(context.Background(), query)
	if err != nil {
		t.Fatal(err)
	}
	lean, err := store.RecentFeatureAnalytics(context.Background(), query)
	if err != nil {
		t.Fatal(err)
	}
	expected := make(map[string]BattleAnalyticsReport)
	for _, report := range complete {
		if strings.EqualFold(report.Role, "attacker") {
			expected[report.ID] = report
		}
	}
	if len(lean) != len(expected) || len(lean) != 2 {
		t.Fatalf("lean rows = %d, attacker rows in Recent = %d (%+v)", len(lean), len(expected), lean)
	}
	for index, row := range lean {
		want, found := expected[row.ReportKey]
		if !found {
			t.Fatalf("lean row %q missing from Recent", row.ReportKey)
		}
		lootPayload, _ := json.Marshal(want.Loot)
		if row.AutomationFeature != want.AutomationFeature || row.OccurredAt != want.OccurredAt || row.Result != want.Result ||
			row.TroopsSent != want.TroopsSent || row.OwnTroopLosses != want.OwnTroopLosses || row.ToolsUsed != want.ToolsUsed ||
			row.GallantryPoints != want.GallantryPoints || row.LootTotal != want.LootTotal ||
			(len(want.Loot) > 0 && string(row.LootJSON) != string(lootPayload)) {
			t.Fatalf("lean row %d = %+v, want %+v", index, row, want)
		}
		if index > 0 && lean[index-1].OccurredAt < row.OccurredAt {
			t.Fatalf("lean rows are not newest first: %+v", lean)
		}
	}
	if lean[len(lean)-1].ReportKey != "feature-3" || lean[len(lean)-1].OwnTroopLosses != 100 {
		t.Fatalf("consolidated binding did not win: %+v", lean[len(lean)-1])
	}
	limited, err := store.RecentFeatureAnalytics(context.Background(), BattleReportQuery{
		AccountUID: 7001, WorldID: "world.example", PlayerID: 42, Limit: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(limited) != 1 || limited[0].ReportKey != "feature-1" {
		t.Fatalf("limited lean rows = %+v", limited)
	}
}
