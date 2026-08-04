package Reports

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"CitadelDesktop/Server/History"
	"CitadelDesktop/Server/State"

	_ "modernc.org/sqlite"
)

const battleReportAnalyticsLimit = 100_000

const targetTypeStorageVersion = "summary.ai.at.v1"

type BattleReportQuery struct {
	AccountUID int64
	WorldID    string
	PlayerID   int64
	FeatureID  string
	EventID    int64
	Since      time.Time
	Limit      int
}

type SQLiteStore struct {
	db *sql.DB

	errorMu sync.RWMutex
	lastErr error
}

func OpenSQLiteStore(dataDir string) (*SQLiteStore, error) {
	dataDir = strings.TrimSpace(dataDir)
	if dataDir == "" {
		return nil, fmt.Errorf("report analytics data directory is required")
	}
	directory := filepath.Join(dataDir, "Runtime")
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return nil, fmt.Errorf("create report analytics directory: %w", err)
	}
	db, err := sql.Open("sqlite", filepath.Join(directory, "Operations.sqlite"))
	if err != nil {
		return nil, fmt.Errorf("open report analytics database: %w", err)
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	store := &SQLiteStore{db: db}
	if err := store.initialize(context.Background()); err != nil {
		_ = db.Close()
		return nil, err
	}
	return store, nil
}

func (store *SQLiteStore) initialize(ctx context.Context) error {
	if store == nil || store.db == nil {
		return fmt.Errorf("report analytics database is unavailable")
	}
	for _, statement := range []string{
		"PRAGMA journal_mode=WAL",
		"PRAGMA synchronous=FULL",
		"PRAGMA busy_timeout=5000",
		"PRAGMA foreign_keys=ON",
		"PRAGMA wal_autocheckpoint=1000",
	} {
		if _, err := store.db.ExecContext(ctx, statement); err != nil {
			return fmt.Errorf("configure report analytics database: %w", err)
		}
	}
	if _, err := store.db.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS battle_report_cloud_outbox (
			lid INTEGER PRIMARY KEY,
			mid INTEGER NOT NULL,
			richness_score INTEGER NOT NULL,
			envelope_json BLOB NOT NULL,
			updated_at TEXT NOT NULL
		);
		CREATE INDEX IF NOT EXISTS battle_report_cloud_outbox_updated
			ON battle_report_cloud_outbox(updated_at, lid);
		CREATE TABLE IF NOT EXISTS battle_report_storage_metadata (
			key TEXT PRIMARY KEY,
			value TEXT NOT NULL
		);
	`); err != nil {
		return fmt.Errorf("initialize cloud battle report outbox: %w", err)
	}
	if err := createCompactBattleAnalyticsTable(ctx, store.db, "battle_report_analytics"); err != nil {
		return err
	}
	columns, err := battleAnalyticsColumns(ctx, store.db)
	if err != nil {
		return err
	}
	if columns["report_json"] {
		if err := store.migrateCanonicalBattleAnalytics(ctx); err != nil {
			return err
		}
		columns, err = battleAnalyticsColumns(ctx, store.db)
		if err != nil {
			return err
		}
	}
	for _, migration := range []struct {
		column      string
		definition  string
		description string
	}{
		{column: "gallantry_points", definition: "INTEGER NOT NULL DEFAULT 0", description: "gallantry"},
		{column: "troops_sent", definition: "INTEGER NOT NULL DEFAULT 0", description: "troops sent"},
	} {
		if columns[migration.column] {
			continue
		}
		if _, err := store.db.ExecContext(ctx, `
			ALTER TABLE battle_report_analytics
			ADD COLUMN `+migration.column+` `+migration.definition+`
		`); err != nil {
			return fmt.Errorf("add battle report %s analytics: %w", migration.description, err)
		}
		columns[migration.column] = true
	}
	for _, required := range []string{
		"gallantry_points", "troops_sent",
		"target_player_id", "target_name", "target_type_id", "target_type",
		"kingdom_id", "target_x", "target_y",
	} {
		if !columns[required] {
			return fmt.Errorf("battle report analytics schema is missing %s", required)
		}
	}
	if err := store.normalizeLegacyTargetTypes(ctx); err != nil {
		return err
	}
	if err := createBattleAnalyticsIndexes(ctx, store.db); err != nil {
		return err
	}
	return nil
}

type compactBattleAnalyticsRecord struct {
	AccountKey  string
	AccountUID  int64
	WorldID     string
	PlayerID    int64
	Analytics   BattleAnalyticsReport
	LootPayload []byte
	UpdatedAt   string
}

func createCompactBattleAnalyticsTable(
	ctx context.Context,
	execer sqlReportExecer,
	tableName string,
) error {
	switch tableName {
	case "battle_report_analytics", "battle_report_analytics_compact":
	default:
		return fmt.Errorf("invalid battle report analytics table %q", tableName)
	}
	_, err := execer.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS `+tableName+` (
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
			troops_sent INTEGER NOT NULL DEFAULT 0,
			own_troop_losses INTEGER NOT NULL DEFAULT 0,
			tools_used INTEGER NOT NULL DEFAULT 0,
			gallantry_points INTEGER NOT NULL DEFAULT 0,
			loot_total INTEGER NOT NULL DEFAULT 0,
			loot_json BLOB NOT NULL,
			target_player_id INTEGER NOT NULL DEFAULT 0,
			target_name TEXT NOT NULL DEFAULT '',
			target_type_id INTEGER NOT NULL DEFAULT 0,
			target_type TEXT NOT NULL DEFAULT '',
			kingdom_id INTEGER NOT NULL DEFAULT 0,
			target_x INTEGER NOT NULL DEFAULT 0,
			target_y INTEGER NOT NULL DEFAULT 0,
			updated_at TEXT NOT NULL,
			PRIMARY KEY (account_key, report_key)
		)
	`)
	if err != nil {
		return fmt.Errorf("create compact battle report analytics table: %w", err)
	}
	return nil
}

func createBattleAnalyticsIndexes(ctx context.Context, execer sqlReportExecer) error {
	if _, err := execer.ExecContext(ctx, `
		CREATE INDEX IF NOT EXISTS battle_report_analytics_feature
			ON battle_report_analytics(account_key, automation_feature, occurred_at);
		CREATE INDEX IF NOT EXISTS battle_report_analytics_event
			ON battle_report_analytics(account_key, event_id, event_occurrence_ends_at, event_activity, occurred_at);
		CREATE INDEX IF NOT EXISTS battle_report_analytics_movement
			ON battle_report_analytics(account_key, movement_id);
		CREATE INDEX IF NOT EXISTS battle_report_analytics_account_binding
			ON battle_report_analytics(world_id, player_id, occurred_at);
		CREATE INDEX IF NOT EXISTS battle_report_analytics_target
			ON battle_report_analytics(account_key, kingdom_id, target_x, target_y, target_type_id, occurred_at);
	`); err != nil {
		return fmt.Errorf("create battle report analytics indexes: %w", err)
	}
	return nil
}

func battleAnalyticsColumns(ctx context.Context, db *sql.DB) (map[string]bool, error) {
	rows, err := db.QueryContext(ctx, `PRAGMA table_info(battle_report_analytics)`)
	if err != nil {
		return nil, fmt.Errorf("inspect battle report analytics schema: %w", err)
	}
	defer rows.Close()
	columns := map[string]bool{}
	for rows.Next() {
		var sequence int
		var name string
		var dataType string
		var notNull int
		var defaultValue any
		var primaryKey int
		if err := rows.Scan(&sequence, &name, &dataType, &notNull, &defaultValue, &primaryKey); err != nil {
			return nil, fmt.Errorf("scan battle report analytics schema: %w", err)
		}
		columns[name] = true
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("inspect battle report analytics schema: %w", err)
	}
	return columns, nil
}

func (store *SQLiteStore) normalizeLegacyTargetTypes(ctx context.Context) error {
	tx, err := store.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin target type storage migration: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	var version string
	err = tx.QueryRowContext(ctx, `
		SELECT value
		FROM battle_report_storage_metadata
		WHERE key = 'target_type_source'
	`).Scan(&version)
	if err == nil && version == targetTypeStorageVersion {
		return nil
	}
	if err != nil && err != sql.ErrNoRows {
		return fmt.Errorf("read target type storage version: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE battle_report_analytics
		SET target_type_id = 0, target_type = ''
		WHERE target_type_id = 6 AND target_type = 'Type 6'
	`); err != nil {
		return fmt.Errorf("clear legacy battle-message types from target analytics: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO battle_report_storage_metadata (key, value)
		VALUES ('target_type_source', ?)
		ON CONFLICT(key) DO UPDATE SET value = excluded.value
	`, targetTypeStorageVersion); err != nil {
		return fmt.Errorf("record target type storage version: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit target type storage migration: %w", err)
	}
	return nil
}

func (store *SQLiteStore) migrateCanonicalBattleAnalytics(ctx context.Context) error {
	records, pvpReports, err := store.readCanonicalBattleAnalytics(ctx)
	if err != nil {
		return err
	}
	tx, err := store.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin compact battle report analytics migration: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, `DROP TABLE IF EXISTS battle_report_analytics_compact`); err != nil {
		return fmt.Errorf("reset compact battle report analytics migration: %w", err)
	}
	if err := createCompactBattleAnalyticsTable(ctx, tx, "battle_report_analytics_compact"); err != nil {
		return err
	}
	statement, err := tx.PrepareContext(ctx, compactBattleAnalyticsInsertSQL("battle_report_analytics_compact"))
	if err != nil {
		return fmt.Errorf("prepare compact battle report analytics migration: %w", err)
	}
	for _, record := range records {
		if _, err := statement.ExecContext(ctx, compactBattleAnalyticsArguments(record)...); err != nil {
			statement.Close()
			return fmt.Errorf("migrate compact battle report %q: %w", record.Analytics.ID, err)
		}
	}
	if err := statement.Close(); err != nil {
		return fmt.Errorf("close compact battle report analytics migration: %w", err)
	}
	for _, report := range pvpReports {
		envelope, eligible, err := buildCloudEnvelopeFromReport(report)
		if err != nil {
			return fmt.Errorf("preserve migrated PvP report LID %d for cloud: %w", report.LID, err)
		}
		if eligible {
			if err := queueCloudReport(ctx, tx, envelope); err != nil {
				return err
			}
		}
	}
	if _, err := tx.ExecContext(ctx, `
		DROP TABLE battle_report_analytics;
		ALTER TABLE battle_report_analytics_compact RENAME TO battle_report_analytics;
	`); err != nil {
		return fmt.Errorf("replace canonical battle report analytics table: %w", err)
	}
	if err := createBattleAnalyticsIndexes(ctx, tx); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit compact battle report analytics migration: %w", err)
	}
	return nil
}

func (store *SQLiteStore) readCanonicalBattleAnalytics(
	ctx context.Context,
) ([]compactBattleAnalyticsRecord, []BattleReport, error) {
	rows, err := store.db.QueryContext(ctx, `
		SELECT account_key, report_key, account_uid, world_id, player_id,
			message_id, battle_report_id, movement_id, automation_feature,
			event_id, event_activity, event_occurrence_ends_at, occurred_at,
			result, role, own_troop_losses, tools_used, loot_total,
			loot_json, report_json, updated_at
		FROM battle_report_analytics
	`)
	if err != nil {
		return nil, nil, fmt.Errorf("read canonical battle report analytics migration: %w", err)
	}
	defer rows.Close()
	records := make([]compactBattleAnalyticsRecord, 0, 1024)
	pvpReports := make([]BattleReport, 0, 128)
	for rows.Next() {
		var record compactBattleAnalyticsRecord
		var analytics BattleAnalyticsReport
		var lootPayload []byte
		var reportPayload []byte
		if err := rows.Scan(
			&record.AccountKey, &analytics.ID, &record.AccountUID, &record.WorldID, &record.PlayerID,
			&analytics.MID, &analytics.LID, &analytics.MovementID, &analytics.AutomationFeature,
			&analytics.EventID, &analytics.EventActivity, &analytics.EventOccurrenceEndsAt, &analytics.OccurredAt,
			&analytics.Result, &analytics.Role, &analytics.OwnTroopLosses, &analytics.ToolsUsed, &analytics.LootTotal,
			&lootPayload, &reportPayload, &record.UpdatedAt,
		); err != nil {
			return nil, nil, fmt.Errorf("scan canonical battle report analytics migration: %w", err)
		}
		var report BattleReport
		if err := json.Unmarshal(reportPayload, &report); err != nil {
			return nil, nil, fmt.Errorf("decode canonical battle report %q for migration: %w", analytics.ID, err)
		}
		report = normalizeStoredBattleReport(report)
		if IsPvPBattleReport(report) {
			pvpReports = append(pvpReports, report)
			continue
		}
		if len(lootPayload) > 0 {
			if err := json.Unmarshal(lootPayload, &analytics.Loot); err != nil {
				return nil, nil, fmt.Errorf("decode battle report loot %q for migration: %w", analytics.ID, err)
			}
		}
		record.LootPayload = append([]byte(nil), lootPayload...)
		target := analyticsTarget(report)
		analytics.DateMs = report.DateMs
		analytics.GallantryPoints = report.GallantryPoints
		if report.Role == "attacker" {
			analytics.TroopsSent = max(int64(0), report.Metrics.AttackerSent)
		}
		analytics.TargetPlayerID = target.playerID
		analytics.TargetName = target.name
		analytics.TargetTypeID = target.typeID
		analytics.TargetType = target.typeName
		analytics.KingdomID = report.KingdomID
		analytics.TargetX = report.TargetX
		analytics.TargetY = report.TargetY
		record.Analytics = analytics
		records = append(records, record)
	}
	if err := rows.Err(); err != nil {
		return nil, nil, fmt.Errorf("read canonical battle report analytics migration: %w", err)
	}
	return records, pvpReports, nil
}

func (store *SQLiteStore) Save(ctx context.Context, report BattleReport) error {
	if store == nil || store.db == nil {
		return fmt.Errorf("report analytics database is unavailable")
	}
	report = normalizeStoredBattleReport(report)
	if strings.TrimSpace(report.ID) == "" {
		return fmt.Errorf("battle report key is required")
	}
	if IsPvPBattleReport(report) {
		return nil
	}
	err := upsertBattleReport(ctx, store.db, report)
	store.setLastError(err)
	return err
}

func (store *SQLiteStore) SaveMany(ctx context.Context, reports []BattleReport) error {
	if store == nil || store.db == nil {
		return fmt.Errorf("report analytics database is unavailable")
	}
	if len(reports) == 0 {
		return nil
	}
	tx, err := store.db.BeginTx(ctx, nil)
	if err != nil {
		store.setLastError(err)
		return fmt.Errorf("begin report analytics backfill: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	for _, report := range reports {
		report = normalizeStoredBattleReport(report)
		if strings.TrimSpace(report.ID) == "" || IsPvPBattleReport(report) {
			continue
		}
		if err := upsertBattleReport(ctx, tx, report); err != nil {
			store.setLastError(err)
			return err
		}
	}
	if err := tx.Commit(); err != nil {
		store.setLastError(err)
		return fmt.Errorf("commit report analytics backfill: %w", err)
	}
	store.setLastError(nil)
	return nil
}

func (store *SQLiteStore) QueueCloudReport(ctx context.Context, report cloudBattleReportEnvelope) error {
	if store == nil || store.db == nil {
		return fmt.Errorf("report analytics database is unavailable")
	}
	return queueCloudReport(ctx, store.db, report)
}

func queueCloudReport(
	ctx context.Context,
	execer sqlReportExecer,
	report cloudBattleReportEnvelope,
) error {
	if report.LID <= 0 || report.MID <= 0 {
		return fmt.Errorf("cloud battle report outbox requires positive MID and LID")
	}
	payload, err := json.Marshal(report)
	if err != nil {
		return fmt.Errorf("encode cloud battle report outbox entry: %w", err)
	}
	_, err = execer.ExecContext(ctx, `
		INSERT INTO battle_report_cloud_outbox (
			lid, mid, richness_score, envelope_json, updated_at
		) VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(lid) DO UPDATE SET
			mid = excluded.mid,
			richness_score = excluded.richness_score,
			envelope_json = excluded.envelope_json,
			updated_at = excluded.updated_at
		WHERE excluded.richness_score >= battle_report_cloud_outbox.richness_score
	`, report.LID, report.MID, report.RichnessScore, payload, time.Now().UTC().Format(time.RFC3339Nano))
	if err != nil {
		return fmt.Errorf("queue cloud battle report: %w", err)
	}
	return nil
}

func (store *SQLiteStore) PendingCloudReports(ctx context.Context, limit int) ([]cloudBattleReportEnvelope, error) {
	if store == nil || store.db == nil {
		return nil, fmt.Errorf("report analytics database is unavailable")
	}
	if limit <= 0 || limit > cloudReportBatchMax {
		limit = cloudReportBatchMax
	}
	rows, err := store.db.QueryContext(ctx, `
		SELECT envelope_json
		FROM battle_report_cloud_outbox
		ORDER BY updated_at ASC, lid ASC
		LIMIT ?
	`, limit)
	if err != nil {
		return nil, fmt.Errorf("list cloud battle report outbox: %w", err)
	}
	defer rows.Close()
	reports := make([]cloudBattleReportEnvelope, 0, limit)
	for rows.Next() {
		var payload []byte
		if err := rows.Scan(&payload); err != nil {
			return nil, fmt.Errorf("scan cloud battle report outbox: %w", err)
		}
		var report cloudBattleReportEnvelope
		if err := json.Unmarshal(payload, &report); err != nil {
			return nil, fmt.Errorf("decode cloud battle report outbox: %w", err)
		}
		reports = append(reports, report)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list cloud battle report outbox: %w", err)
	}
	return reports, nil
}

func (store *SQLiteStore) DeleteCloudReports(ctx context.Context, lids []int64) error {
	if store == nil || store.db == nil {
		return fmt.Errorf("report analytics database is unavailable")
	}
	placeholders := make([]string, 0, len(lids))
	arguments := make([]any, 0, len(lids))
	seen := make(map[int64]struct{}, len(lids))
	for _, lid := range lids {
		if lid <= 0 {
			continue
		}
		if _, exists := seen[lid]; exists {
			continue
		}
		seen[lid] = struct{}{}
		placeholders = append(placeholders, "?")
		arguments = append(arguments, lid)
	}
	if len(arguments) == 0 {
		return nil
	}
	if _, err := store.db.ExecContext(ctx, `
		DELETE FROM battle_report_cloud_outbox
		WHERE lid IN (`+strings.Join(placeholders, ",")+`)
	`, arguments...); err != nil {
		return fmt.Errorf("delete confirmed cloud battle reports: %w", err)
	}
	return nil
}

func (store *SQLiteStore) ArchivedMessageIDs(ctx context.Context, query BattleReportQuery) ([]int64, error) {
	if store == nil || store.db == nil {
		return nil, fmt.Errorf("report analytics database is unavailable")
	}
	accountKey := reportAccountKey(query.AccountUID, query.WorldID, query.PlayerID)
	worldID := strings.TrimSpace(query.WorldID)
	if accountKey == "" && (worldID == "" || query.PlayerID <= 0) {
		return []int64{}, nil
	}
	clauses := make([]string, 0, 2)
	arguments := make([]any, 0, 3)
	if accountKey != "" && worldID != "" && query.PlayerID > 0 {
		clauses = append(clauses, "(account_key = ? OR (world_id = ? AND player_id = ?))")
		arguments = append(arguments, accountKey, worldID, query.PlayerID)
	} else if accountKey != "" {
		clauses = append(clauses, "account_key = ?")
		arguments = append(arguments, accountKey)
	} else {
		clauses = append(clauses, "world_id = ? AND player_id = ?")
		arguments = append(arguments, worldID, query.PlayerID)
	}
	rows, err := store.db.QueryContext(ctx, `
		SELECT message_id
		FROM (
			SELECT DISTINCT message_id
			FROM battle_report_analytics
			WHERE message_id > 0 AND `+strings.Join(clauses, " AND ")+`
			UNION
			SELECT mid AS message_id
			FROM battle_report_cloud_outbox
			WHERE mid > 0
		)
	`, arguments...)
	if err != nil {
		return nil, fmt.Errorf("list archived battle report messages: %w", err)
	}
	defer rows.Close()
	messageIDs := make([]int64, 0, 1024)
	for rows.Next() {
		var messageID int64
		if err := rows.Scan(&messageID); err != nil {
			return nil, fmt.Errorf("scan archived battle report message: %w", err)
		}
		messageIDs = append(messageIDs, messageID)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list archived battle report messages: %w", err)
	}
	return messageIDs, nil
}

func (store *SQLiteStore) Recent(ctx context.Context, query BattleReportQuery) ([]BattleAnalyticsReport, error) {
	if store == nil || store.db == nil {
		return nil, fmt.Errorf("report analytics database is unavailable")
	}
	accountKey := reportAccountKey(query.AccountUID, query.WorldID, query.PlayerID)
	worldID := strings.TrimSpace(query.WorldID)
	if accountKey == "" && (worldID == "" || query.PlayerID <= 0) {
		return []BattleAnalyticsReport{}, nil
	}
	limit := query.Limit
	if limit <= 0 || limit > battleReportAnalyticsLimit {
		limit = battleReportAnalyticsLimit
	}
	clauses := make([]string, 0, 4)
	arguments := make([]any, 0, 8)
	if accountKey != "" && worldID != "" && query.PlayerID > 0 {
		clauses = append(clauses, "(account_key = ? OR (world_id = ? AND player_id = ?))")
		arguments = append(arguments, accountKey, worldID, query.PlayerID)
	} else if accountKey != "" {
		clauses = append(clauses, "account_key = ?")
		arguments = append(arguments, accountKey)
	} else {
		clauses = append(clauses, "world_id = ? AND player_id = ?")
		arguments = append(arguments, worldID, query.PlayerID)
	}
	if featureID := strings.TrimSpace(query.FeatureID); featureID != "" {
		clauses = append(clauses, "automation_feature = ?")
		arguments = append(arguments, featureID)
	}
	if query.EventID > 0 {
		clauses = append(clauses, "event_id = ?")
		arguments = append(arguments, query.EventID)
	}
	if !query.Since.IsZero() {
		clauses = append(clauses, "occurred_at >= ?")
		arguments = append(arguments, query.Since.UTC().Format(time.RFC3339Nano))
	}
	preference := "0"
	queryArguments := make([]any, 0, len(arguments)+2)
	if accountKey != "" {
		preference = "CASE WHEN account_key = ? THEN 0 ELSE 1 END"
		queryArguments = append(queryArguments, accountKey)
	}
	queryArguments = append(queryArguments, arguments...)
	queryArguments = append(queryArguments, limit)
	rows, err := store.db.QueryContext(ctx, `
		SELECT report_key, message_id, battle_report_id, movement_id, automation_feature,
			event_id, event_activity, event_occurrence_ends_at, occurred_at,
			result, role, troops_sent, own_troop_losses, tools_used, gallantry_points, loot_total, loot_json,
			target_player_id, target_name, target_type_id, target_type,
			kingdom_id, target_x, target_y
		FROM (
			SELECT report_key, message_id, battle_report_id, movement_id, automation_feature,
				event_id, event_activity, event_occurrence_ends_at, occurred_at,
				result, role, troops_sent, own_troop_losses, tools_used, gallantry_points, loot_total, loot_json,
				target_player_id, target_name, target_type_id, target_type,
				kingdom_id, target_x, target_y,
				ROW_NUMBER() OVER (
					PARTITION BY report_key
					ORDER BY `+preference+` ASC, updated_at DESC
				) AS binding_rank
			FROM battle_report_analytics
			WHERE `+strings.Join(clauses, " AND ")+`
		)
		WHERE binding_rank = 1
		ORDER BY occurred_at DESC, report_key DESC
		LIMIT ?
	`, queryArguments...)
	if err != nil {
		store.setLastError(err)
		return nil, fmt.Errorf("list report analytics: %w", err)
	}
	defer rows.Close()
	reports := make([]BattleAnalyticsReport, 0)
	for rows.Next() {
		var report BattleAnalyticsReport
		var lootPayload []byte
		if err := rows.Scan(
			&report.ID, &report.MID, &report.LID, &report.MovementID, &report.AutomationFeature,
			&report.EventID, &report.EventActivity, &report.EventOccurrenceEndsAt, &report.OccurredAt,
			&report.Result, &report.Role, &report.TroopsSent, &report.OwnTroopLosses, &report.ToolsUsed, &report.GallantryPoints,
			&report.LootTotal, &lootPayload,
			&report.TargetPlayerID, &report.TargetName, &report.TargetTypeID, &report.TargetType,
			&report.KingdomID, &report.TargetX, &report.TargetY,
		); err != nil {
			store.setLastError(err)
			return nil, fmt.Errorf("scan report analytics: %w", err)
		}
		if len(lootPayload) > 0 {
			if err := json.Unmarshal(lootPayload, &report.Loot); err != nil {
				store.setLastError(err)
				return nil, fmt.Errorf("decode report analytics loot: %w", err)
			}
		}
		if occurredAt, err := time.Parse(time.RFC3339Nano, report.OccurredAt); err == nil {
			report.DateMs = occurredAt.UnixMilli()
		} else {
			store.setLastError(err)
			return nil, fmt.Errorf("decode report analytics timestamp: %w", err)
		}
		reports = append(reports, report)
	}
	if err := rows.Err(); err != nil {
		store.setLastError(err)
		return nil, fmt.Errorf("list report analytics: %w", err)
	}
	store.setLastError(nil)
	return reports, nil
}

func BackfillBattleHistory(ctx context.Context, history *History.Store, store *SQLiteStore, snapshot State.GameState) error {
	if history == nil || store == nil {
		return nil
	}
	rows, err := history.Read(History.CollectionBattleReports, time.Time{}, battleReportAnalyticsLimit)
	if err != nil {
		return fmt.Errorf("read battle history for analytics backfill: %w", err)
	}
	safeInvasionReports := currentInvasionReportAttribution(snapshot)
	reports := make([]BattleReport, 0, len(rows))
	for _, row := range rows {
		var report BattleReport
		if json.Unmarshal(row, &report) != nil || strings.TrimSpace(report.ID) == "" {
			continue
		}
		report = normalizeStoredBattleReport(report)
		if attribution, found := safeInvasionReports[report.LID]; found && report.PlayerID == int64(snapshot.Player.ID) {
			report.AccountUID = snapshot.Account.UID
			report.WorldID = snapshot.Account.WorldID
			report.AutomationFeature = string(State.AttackFeatureAutoInvasion)
			report.EventID = attribution.eventID
			report.EventActivity = string(State.EventActivityInvasion)
			report.EventOccurrenceEndsAt = attribution.occurrenceEndsAt
		}
		if reportAccountKey(report.AccountUID, report.WorldID, report.PlayerID) == "" {
			continue
		}
		reports = append(reports, report)
	}
	return store.SaveMany(ctx, reports)
}

func (store *SQLiteStore) LastError() error {
	if store == nil {
		return nil
	}
	store.errorMu.RLock()
	defer store.errorMu.RUnlock()
	return store.lastErr
}

func (store *SQLiteStore) Close() error {
	if store == nil || store.db == nil {
		return nil
	}
	return store.db.Close()
}

type sqlReportExecer interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
}

func upsertBattleReport(ctx context.Context, execer sqlReportExecer, report BattleReport) error {
	if IsPvPBattleReport(report) {
		return nil
	}
	lootPayload, err := json.Marshal(report.Loot)
	if err != nil {
		return fmt.Errorf("encode battle report loot: %w", err)
	}
	accountKey := reportAccountKey(report.AccountUID, report.WorldID, report.PlayerID)
	if accountKey == "" {
		return fmt.Errorf("battle report %q has no account identity", report.ID)
	}
	if worldID := strings.TrimSpace(report.WorldID); worldID != "" && report.PlayerID > 0 {
		if _, err := execer.ExecContext(ctx, `
			DELETE FROM battle_report_analytics
			WHERE report_key = ? AND world_id = ? AND player_id = ? AND account_key != ?
		`, report.ID, worldID, report.PlayerID, accountKey); err != nil {
			return fmt.Errorf("consolidate battle report account binding: %w", err)
		}
	}
	analytics := analyticsReportFromBattle(report)
	record := compactBattleAnalyticsRecord{
		AccountKey:  accountKey,
		AccountUID:  report.AccountUID,
		WorldID:     strings.TrimSpace(report.WorldID),
		PlayerID:    report.PlayerID,
		Analytics:   analytics,
		LootPayload: lootPayload,
		UpdatedAt:   time.Now().UTC().Format(time.RFC3339Nano),
	}
	_, err = execer.ExecContext(
		ctx,
		compactBattleAnalyticsInsertSQL("battle_report_analytics"),
		compactBattleAnalyticsArguments(record)...,
	)
	if err != nil {
		return fmt.Errorf("save compact battle report analytics: %w", err)
	}
	return nil
}

func compactBattleAnalyticsInsertSQL(tableName string) string {
	switch tableName {
	case "battle_report_analytics", "battle_report_analytics_compact":
	default:
		panic("invalid compact battle report analytics table")
	}
	return `
		INSERT INTO ` + tableName + ` (
			account_key, report_key, account_uid, world_id, player_id,
			message_id, battle_report_id, movement_id, automation_feature,
			event_id, event_activity, event_occurrence_ends_at, occurred_at,
			result, role, troops_sent, own_troop_losses, tools_used, gallantry_points, loot_total, loot_json,
			target_player_id, target_name, target_type_id, target_type,
			kingdom_id, target_x, target_y, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(account_key, report_key) DO UPDATE SET
			account_uid = excluded.account_uid,
			world_id = excluded.world_id,
			player_id = excluded.player_id,
			message_id = excluded.message_id,
			battle_report_id = excluded.battle_report_id,
			movement_id = excluded.movement_id,
			automation_feature = excluded.automation_feature,
			event_id = excluded.event_id,
			event_activity = excluded.event_activity,
			event_occurrence_ends_at = excluded.event_occurrence_ends_at,
			occurred_at = excluded.occurred_at,
			result = excluded.result,
			role = excluded.role,
			troops_sent = excluded.troops_sent,
			own_troop_losses = excluded.own_troop_losses,
			tools_used = excluded.tools_used,
			gallantry_points = excluded.gallantry_points,
			loot_total = excluded.loot_total,
			loot_json = excluded.loot_json,
			target_player_id = excluded.target_player_id,
			target_name = excluded.target_name,
			target_type_id = excluded.target_type_id,
			target_type = excluded.target_type,
			kingdom_id = excluded.kingdom_id,
			target_x = excluded.target_x,
			target_y = excluded.target_y,
			updated_at = excluded.updated_at
	`
}

func compactBattleAnalyticsArguments(record compactBattleAnalyticsRecord) []any {
	analytics := record.Analytics
	lootPayload := record.LootPayload
	if len(lootPayload) == 0 {
		lootPayload = []byte("{}")
	}
	return []any{
		record.AccountKey, analytics.ID, record.AccountUID, record.WorldID, record.PlayerID,
		analytics.MID, analytics.LID, analytics.MovementID, strings.TrimSpace(analytics.AutomationFeature),
		analytics.EventID, strings.TrimSpace(analytics.EventActivity), strings.TrimSpace(analytics.EventOccurrenceEndsAt),
		analytics.OccurredAt, analytics.Result, analytics.Role, analytics.TroopsSent, analytics.OwnTroopLosses, analytics.ToolsUsed,
		analytics.GallantryPoints, analytics.LootTotal, lootPayload, analytics.TargetPlayerID, analytics.TargetName,
		analytics.TargetTypeID, analytics.TargetType, analytics.KingdomID, analytics.TargetX, analytics.TargetY,
		record.UpdatedAt,
	}
}

type battleAnalyticsTarget struct {
	playerID int64
	name     string
	typeID   int
	typeName string
}

func analyticsTarget(report BattleReport) battleAnalyticsTarget {
	target := battleAnalyticsTarget{
		typeID: report.TargetTypeID, typeName: strings.TrimSpace(report.TargetType),
	}
	if report.Defender != nil {
		target.playerID = report.Defender.PlayerID
	}
	target.name = strings.TrimSpace(report.TargetName)
	if target.name == "" {
		target.name = strings.TrimSpace(report.CastleName)
	}
	if target.name == "" && report.Defender != nil {
		target.name = strings.TrimSpace(report.Defender.Name)
	}
	if target.typeID <= 0 {
		_, _ = fmt.Sscanf(target.typeName, "Type %d", &target.typeID)
	}
	if target.typeName == "" && target.typeID > 0 {
		target.typeName = fmt.Sprintf("Type %d", target.typeID)
	}
	return target
}

func analyticsReportFromBattle(report BattleReport) BattleAnalyticsReport {
	target := analyticsTarget(report)
	return BattleAnalyticsReport{
		ID: report.ID, MID: report.MID, LID: report.LID,
		MovementID: report.MovementID, AutomationFeature: strings.TrimSpace(report.AutomationFeature),
		EventID: report.EventID, EventActivity: strings.TrimSpace(report.EventActivity),
		EventOccurrenceEndsAt: strings.TrimSpace(report.EventOccurrenceEndsAt),
		OccurredAt:            report.OccurredAt, DateMs: report.DateMs, Result: report.Result, Role: report.Role,
		TroopsSent: ownTroopsSent(report), OwnTroopLosses: ownTroopLosses(report), ToolsUsed: report.ToolsUsed,
		GallantryPoints: report.GallantryPoints, LootTotal: reportLootTotal(report), Loot: report.Loot,
		TargetPlayerID: target.playerID, TargetName: target.name,
		TargetTypeID: target.typeID, TargetType: target.typeName,
		KingdomID: report.KingdomID, TargetX: report.TargetX, TargetY: report.TargetY,
	}
}

func ownTroopsSent(report BattleReport) int64 {
	if report.Role != "attacker" {
		return 0
	}
	return max(int64(0), report.Metrics.AttackerSent)
}

func normalizeStoredBattleReport(report BattleReport) BattleReport {
	if report.ID == "" {
		report.ID = report.ReportID
	}
	if report.ReportID == "" {
		report.ReportID = report.ID
	}
	if report.PlayerID <= 0 {
		switch report.Role {
		case "attacker":
			if report.Attacker != nil {
				report.PlayerID = report.Attacker.PlayerID
			}
		case "defender":
			if report.Defender != nil {
				report.PlayerID = report.Defender.PlayerID
			}
		}
	}
	if strings.TrimSpace(report.OccurredAt) == "" {
		if report.DateMs > 0 {
			report.OccurredAt = time.UnixMilli(report.DateMs).UTC().Format(time.RFC3339Nano)
		} else {
			report.OccurredAt = time.Unix(0, 0).UTC().Format(time.RFC3339Nano)
		}
	} else if report.DateMs <= 0 {
		if occurredAt, err := time.Parse(time.RFC3339Nano, report.OccurredAt); err == nil {
			report.DateMs = occurredAt.UnixMilli()
		}
	}
	if report.BattleTypeID <= 0 {
		_, _ = fmt.Sscanf(strings.TrimSpace(report.BattleType), "Type %d", &report.BattleTypeID)
	}
	if report.TargetTypeID <= 0 {
		_, _ = fmt.Sscanf(strings.TrimSpace(report.TargetType), "Type %d", &report.TargetTypeID)
	}
	if strings.TrimSpace(report.TargetType) == "" && report.TargetTypeID > 0 {
		report.TargetType = fmt.Sprintf("Type %d", report.TargetTypeID)
	}
	return report
}

func reportAccountKey(uid int64, worldID string, playerID int64) string {
	if uid > 0 {
		return "uid:" + strconv.FormatInt(uid, 10)
	}
	worldID = strings.TrimSpace(worldID)
	if worldID != "" && playerID > 0 {
		return "world:" + worldID + ":player:" + strconv.FormatInt(playerID, 10)
	}
	if playerID > 0 {
		return "legacy-player:" + strconv.FormatInt(playerID, 10)
	}
	return ""
}

func ownTroopLosses(report BattleReport) int64 {
	if report.Role == "defender" {
		return max(int64(0), report.Metrics.DefenderLost)
	}
	return max(int64(0), report.Metrics.AttackerLost)
}

func reportLootTotal(report BattleReport) int64 {
	var total int64
	for _, amount := range report.Loot {
		if amount > 0 {
			total += amount
		}
	}
	return total
}

type invasionReportAttribution struct {
	eventID          int64
	occurrenceEndsAt string
}

func currentInvasionReportAttribution(snapshot State.GameState) map[int64]invasionReportAttribution {
	result := map[int64]invasionReportAttribution{}
	for eventID, activity := range snapshot.EventScores.ActivityByEvent {
		if eventID != 71 && eventID != 103 {
			continue
		}
		occurrenceEndsAt := ""
		if !activity.OccurrenceEndsAt.IsZero() {
			occurrenceEndsAt = activity.OccurrenceEndsAt.UTC().Format(time.RFC3339Nano)
		}
		for _, reportID := range activity.ProcessedReportIDs {
			if reportID > 0 {
				result[reportID] = invasionReportAttribution{eventID: eventID, occurrenceEndsAt: occurrenceEndsAt}
			}
		}
	}
	return result
}

func (store *SQLiteStore) setLastError(err error) {
	if store == nil {
		return
	}
	store.errorMu.Lock()
	store.lastErr = err
	store.errorMu.Unlock()
}
