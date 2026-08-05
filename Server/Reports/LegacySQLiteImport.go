package Reports

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
)

const legacyOperationsImportVersion = "operations.sqlite.v1"

func (store *SQLiteStore) importLegacyOperations(ctx context.Context, legacyPath string) error {
	if store == nil || store.db == nil {
		return fmt.Errorf("report analytics database is unavailable")
	}
	if _, err := os.Stat(legacyPath); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("inspect legacy report analytics: %w", err)
	}
	var imported string
	err := store.db.QueryRowContext(ctx, `
		SELECT value FROM battle_report_storage_metadata WHERE key = 'legacy_operations_import'
	`).Scan(&imported)
	if err == nil && imported == legacyOperationsImportVersion {
		return nil
	}
	if err != nil && err != sql.ErrNoRows {
		return fmt.Errorf("read legacy report import version: %w", err)
	}

	databaseURL := url.URL{Scheme: "file", Path: legacyPath}
	parameters := databaseURL.Query()
	parameters.Add("_pragma", "busy_timeout(5000)")
	databaseURL.RawQuery = parameters.Encode()
	legacyDB, err := sql.Open("sqlite", databaseURL.String())
	if err != nil {
		return fmt.Errorf("open legacy report analytics: %w", err)
	}
	legacyDB.SetMaxOpenConns(1)
	legacyDB.SetMaxIdleConns(1)
	defer legacyDB.Close()

	hasAnalytics, err := sqliteTableExists(ctx, legacyDB, "battle_report_analytics")
	if err != nil {
		return err
	}
	hasOutbox, err := sqliteTableExists(ctx, legacyDB, "battle_report_cloud_outbox")
	if err != nil {
		return err
	}
	if !hasAnalytics && !hasOutbox {
		return nil
	}
	legacyStore := &SQLiteStore{db: legacyDB}
	if err := legacyStore.initialize(ctx); err != nil {
		return fmt.Errorf("prepare legacy report analytics: %w", err)
	}
	records, err := readCompactBattleAnalytics(ctx, legacyDB)
	if err != nil {
		return err
	}
	envelopes, err := readLegacyCloudOutbox(ctx, legacyDB)
	if err != nil {
		return err
	}

	tx, err := store.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin legacy report import: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	statement, err := tx.PrepareContext(ctx, compactBattleAnalyticsInsertSQL("battle_report_analytics"))
	if err != nil {
		return fmt.Errorf("prepare legacy report import: %w", err)
	}
	defer statement.Close()
	for _, record := range records {
		if _, err := statement.ExecContext(ctx, compactBattleAnalyticsArguments(record)...); err != nil {
			return fmt.Errorf("import legacy report analytics: %w", err)
		}
	}
	for _, envelope := range envelopes {
		if err := queueCloudReport(ctx, tx, envelope); err != nil {
			return fmt.Errorf("import legacy cloud report outbox: %w", err)
		}
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO battle_report_storage_metadata (key, value)
		VALUES ('legacy_operations_import', ?)
		ON CONFLICT(key) DO UPDATE SET value = excluded.value
	`, legacyOperationsImportVersion); err != nil {
		return fmt.Errorf("record legacy report import: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit legacy report import: %w", err)
	}
	return nil
}

func sqliteTableExists(ctx context.Context, db *sql.DB, tableName string) (bool, error) {
	var count int
	if err := db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = ?
	`, tableName).Scan(&count); err != nil {
		return false, fmt.Errorf("inspect legacy report table %q: %w", tableName, err)
	}
	return count > 0, nil
}

func readCompactBattleAnalytics(ctx context.Context, db *sql.DB) ([]compactBattleAnalyticsRecord, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT account_key, report_key, account_uid, world_id, player_id,
			message_id, battle_report_id, movement_id, automation_feature,
			event_id, event_activity, event_occurrence_ends_at, occurred_at,
			result, role, troops_sent, own_troop_losses, tools_used, gallantry_points,
			loot_total, loot_json, target_player_id, target_name, target_type_id,
			target_type, kingdom_id, target_x, target_y, updated_at
		FROM battle_report_analytics
	`)
	if err != nil {
		return nil, fmt.Errorf("read legacy report analytics: %w", err)
	}
	defer rows.Close()
	records := make([]compactBattleAnalyticsRecord, 0)
	for rows.Next() {
		var record compactBattleAnalyticsRecord
		analytics := &record.Analytics
		if err := rows.Scan(
			&record.AccountKey, &analytics.ID, &record.AccountUID, &record.WorldID, &record.PlayerID,
			&analytics.MID, &analytics.LID, &analytics.MovementID, &analytics.AutomationFeature,
			&analytics.EventID, &analytics.EventActivity, &analytics.EventOccurrenceEndsAt, &analytics.OccurredAt,
			&analytics.Result, &analytics.Role, &analytics.TroopsSent, &analytics.OwnTroopLosses, &analytics.ToolsUsed,
			&analytics.GallantryPoints, &analytics.LootTotal, &record.LootPayload, &analytics.TargetPlayerID,
			&analytics.TargetName, &analytics.TargetTypeID, &analytics.TargetType, &analytics.KingdomID,
			&analytics.TargetX, &analytics.TargetY, &record.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan legacy report analytics: %w", err)
		}
		records = append(records, record)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read legacy report analytics: %w", err)
	}
	return records, nil
}

func readLegacyCloudOutbox(ctx context.Context, db *sql.DB) ([]cloudBattleReportEnvelope, error) {
	rows, err := db.QueryContext(ctx, `SELECT envelope_json FROM battle_report_cloud_outbox`)
	if err != nil {
		return nil, fmt.Errorf("read legacy cloud report outbox: %w", err)
	}
	defer rows.Close()
	envelopes := make([]cloudBattleReportEnvelope, 0)
	for rows.Next() {
		var payload []byte
		if err := rows.Scan(&payload); err != nil {
			return nil, fmt.Errorf("scan legacy cloud report outbox: %w", err)
		}
		var envelope cloudBattleReportEnvelope
		if err := json.Unmarshal(payload, &envelope); err != nil {
			return nil, fmt.Errorf("decode legacy cloud report outbox: %w", err)
		}
		envelopes = append(envelopes, envelope)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read legacy cloud report outbox: %w", err)
	}
	return envelopes, nil
}
