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
		CREATE TABLE IF NOT EXISTS battle_report_analytics (
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
		);
		CREATE INDEX IF NOT EXISTS battle_report_analytics_feature
			ON battle_report_analytics(account_key, automation_feature, occurred_at);
		CREATE INDEX IF NOT EXISTS battle_report_analytics_event
			ON battle_report_analytics(account_key, event_id, event_occurrence_ends_at, event_activity, occurred_at);
		CREATE INDEX IF NOT EXISTS battle_report_analytics_movement
			ON battle_report_analytics(account_key, movement_id);
		CREATE INDEX IF NOT EXISTS battle_report_analytics_account_binding
			ON battle_report_analytics(world_id, player_id, occurred_at);
	`); err != nil {
		return fmt.Errorf("initialize report analytics database: %w", err)
	}
	return nil
}

func (store *SQLiteStore) Save(ctx context.Context, report BattleReport) error {
	if store == nil || store.db == nil {
		return fmt.Errorf("report analytics database is unavailable")
	}
	report = normalizeStoredBattleReport(report)
	if strings.TrimSpace(report.ID) == "" {
		return fmt.Errorf("battle report key is required")
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
		if strings.TrimSpace(report.ID) == "" {
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

func (store *SQLiteStore) Recent(ctx context.Context, query BattleReportQuery) ([]BattleReport, error) {
	if store == nil || store.db == nil {
		return nil, fmt.Errorf("report analytics database is unavailable")
	}
	accountKey := reportAccountKey(query.AccountUID, query.WorldID, query.PlayerID)
	worldID := strings.TrimSpace(query.WorldID)
	if accountKey == "" && (worldID == "" || query.PlayerID <= 0) {
		return []BattleReport{}, nil
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
		SELECT report_json
		FROM (
			SELECT report_json, occurred_at, report_key,
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
	reports := make([]BattleReport, 0)
	for rows.Next() {
		var payload []byte
		if err := rows.Scan(&payload); err != nil {
			store.setLastError(err)
			return nil, fmt.Errorf("scan report analytics: %w", err)
		}
		var report BattleReport
		if err := json.Unmarshal(payload, &report); err != nil {
			store.setLastError(err)
			return nil, fmt.Errorf("decode report analytics: %w", err)
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
	payload, err := json.Marshal(report)
	if err != nil {
		return fmt.Errorf("encode battle report analytics: %w", err)
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
	_, err = execer.ExecContext(ctx, `
		INSERT INTO battle_report_analytics (
			account_key, report_key, account_uid, world_id, player_id,
			message_id, battle_report_id, movement_id, automation_feature,
			event_id, event_activity, event_occurrence_ends_at, occurred_at,
			result, role, own_troop_losses, tools_used, loot_total, loot_json, report_json, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
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
			own_troop_losses = excluded.own_troop_losses,
			tools_used = excluded.tools_used,
			loot_total = excluded.loot_total,
			loot_json = excluded.loot_json,
			report_json = excluded.report_json,
			updated_at = excluded.updated_at
	`,
		accountKey, report.ID, report.AccountUID, strings.TrimSpace(report.WorldID), report.PlayerID,
		report.MID, report.LID, report.MovementID, strings.TrimSpace(report.AutomationFeature),
		report.EventID, strings.TrimSpace(report.EventActivity), strings.TrimSpace(report.EventOccurrenceEndsAt), report.OccurredAt,
		report.Result, report.Role, ownTroopLosses(report), report.ToolsUsed, reportLootTotal(report), lootPayload, payload,
		time.Now().UTC().Format(time.RFC3339Nano),
	)
	if err != nil {
		return fmt.Errorf("save battle report analytics: %w", err)
	}
	return nil
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
