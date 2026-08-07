package WorldIntel

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	RuntimeKernel "CitadelDesktop/Server/Runtime"
	_ "modernc.org/sqlite"
)

const (
	maximumOutboxRows = 2_000
	desktopBatchLimit = 20
)

type InstallationCredentials struct {
	InstallationID string
	Secret         string
}

type QueuedBatch struct {
	Batch    ObservationBatch
	Attempts int
}

type StoreStatus struct {
	Pending        int
	LastCapturedAt *time.Time
	LastUploadAt   *time.Time
	LastScanAt     *time.Time
	LastError      string
	LastScanError  string
}

type DesktopStore struct {
	db *sql.DB

	errorMu sync.RWMutex
	lastErr error
}

func OpenDesktopStore(dataDir string) (*DesktopStore, error) {
	dataDir = strings.TrimSpace(dataDir)
	if dataDir == "" {
		return nil, fmt.Errorf("world intelligence data directory is required")
	}
	directory := filepath.Join(dataDir, "Runtime")
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return nil, fmt.Errorf("create world intelligence directory: %w", err)
	}
	databasePath := filepath.Join(directory, "WorldIntelligence.sqlite")
	databaseURL, err := RuntimeKernel.SQLiteFileDSN(
		databasePath,
		"busy_timeout(5000)", "foreign_keys(1)",
	)
	if err != nil {
		return nil, fmt.Errorf("configure world intelligence database: %w", err)
	}
	db, err := sql.Open("sqlite", databaseURL)
	if err != nil {
		return nil, fmt.Errorf("open world intelligence database: %w", err)
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	store := &DesktopStore{db: db}
	if err := store.initialize(context.Background()); err != nil {
		_ = db.Close()
		return nil, err
	}
	for _, path := range []string{databasePath, databasePath + "-wal", databasePath + "-shm"} {
		if err := os.Chmod(path, 0o600); err != nil && !os.IsNotExist(err) {
			_ = db.Close()
			return nil, fmt.Errorf("protect world intelligence database: %w", err)
		}
	}
	return store, nil
}

func (store *DesktopStore) initialize(ctx context.Context) error {
	for _, statement := range []string{
		"PRAGMA journal_mode=WAL",
		"PRAGMA synchronous=FULL",
		"PRAGMA busy_timeout=5000",
	} {
		if _, err := store.db.ExecContext(ctx, statement); err != nil {
			return fmt.Errorf("configure world intelligence database: %w", err)
		}
	}
	if _, err := store.db.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS world_intel_outbox (
			batch_id TEXT PRIMARY KEY,
			payload_json BLOB NOT NULL,
			created_at TEXT NOT NULL,
			next_attempt_at TEXT NOT NULL,
			attempt_count INTEGER NOT NULL DEFAULT 0,
			last_error TEXT NOT NULL DEFAULT ''
		);
		CREATE INDEX IF NOT EXISTS world_intel_outbox_pending
			ON world_intel_outbox(next_attempt_at, created_at);
		CREATE TABLE IF NOT EXISTS world_intel_metadata (
			key TEXT PRIMARY KEY,
			value TEXT NOT NULL
		);
	`); err != nil {
		return fmt.Errorf("initialize world intelligence database: %w", err)
	}
	return nil
}

func (store *DesktopStore) Credentials(ctx context.Context) (InstallationCredentials, error) {
	if store == nil || store.db == nil {
		return InstallationCredentials{}, fmt.Errorf("world intelligence store is unavailable")
	}
	transaction, err := store.db.BeginTx(ctx, nil)
	if err != nil {
		return InstallationCredentials{}, fmt.Errorf("begin installation credential transaction: %w", err)
	}
	defer transaction.Rollback()
	id, idFound, err := metadataValue(ctx, transaction, "installation_id")
	if err != nil {
		return InstallationCredentials{}, err
	}
	secret, secretFound, err := metadataValue(ctx, transaction, "installation_secret")
	if err != nil {
		return InstallationCredentials{}, err
	}
	if !idFound || !secretFound || id == "" || secret == "" {
		id, err = randomHex(16)
		if err != nil {
			return InstallationCredentials{}, err
		}
		secret, err = randomHex(32)
		if err != nil {
			return InstallationCredentials{}, err
		}
		for key, value := range map[string]string{"installation_id": id, "installation_secret": secret} {
			if _, err := transaction.ExecContext(ctx, `
				INSERT INTO world_intel_metadata(key, value) VALUES (?, ?)
				ON CONFLICT(key) DO UPDATE SET value = excluded.value
			`, key, value); err != nil {
				return InstallationCredentials{}, fmt.Errorf("store installation credentials: %w", err)
			}
		}
	}
	if err := transaction.Commit(); err != nil {
		return InstallationCredentials{}, fmt.Errorf("commit installation credentials: %w", err)
	}
	return InstallationCredentials{InstallationID: id, Secret: secret}, nil
}

func (store *DesktopStore) Enqueue(ctx context.Context, batch ObservationBatch) (bool, error) {
	if store == nil || store.db == nil {
		return false, fmt.Errorf("world intelligence store is unavailable")
	}
	if err := ValidateFinalizedBatch(batch); err != nil {
		return false, err
	}
	payload, err := json.Marshal(batch)
	if err != nil {
		return false, fmt.Errorf("encode world intelligence outbox batch: %w", err)
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	result, err := store.db.ExecContext(ctx, `
		INSERT OR IGNORE INTO world_intel_outbox (
			batch_id, payload_json, created_at, next_attempt_at
		) VALUES (?, ?, ?, ?)
	`, batch.BatchID, payload, now, now)
	if err != nil {
		store.recordError(err)
		return false, fmt.Errorf("queue world intelligence batch: %w", err)
	}
	if _, err := store.db.ExecContext(ctx, `
		DELETE FROM world_intel_outbox
		WHERE batch_id IN (
			SELECT batch_id FROM world_intel_outbox
			ORDER BY created_at DESC
			LIMIT -1 OFFSET ?
		)
	`, maximumOutboxRows); err != nil {
		store.recordError(err)
		return false, fmt.Errorf("bound world intelligence outbox: %w", err)
	}
	store.setMetadata(ctx, "last_captured_at", batch.CapturedAt.Format(time.RFC3339Nano))
	rows, _ := result.RowsAffected()
	return rows > 0, nil
}

func (store *DesktopStore) Pending(ctx context.Context, now time.Time, limit int) ([]QueuedBatch, error) {
	if store == nil || store.db == nil {
		return nil, fmt.Errorf("world intelligence store is unavailable")
	}
	if limit <= 0 || limit > desktopBatchLimit {
		limit = desktopBatchLimit
	}
	rows, err := store.db.QueryContext(ctx, `
		SELECT payload_json, attempt_count
		FROM world_intel_outbox
		WHERE next_attempt_at <= ?
		ORDER BY created_at ASC
		LIMIT ?
	`, now.UTC().Format(time.RFC3339Nano), limit)
	if err != nil {
		store.recordError(err)
		return nil, fmt.Errorf("list world intelligence outbox: %w", err)
	}
	defer rows.Close()
	result := make([]QueuedBatch, 0, limit)
	for rows.Next() {
		var payload []byte
		var attempts int
		if err := rows.Scan(&payload, &attempts); err != nil {
			return nil, fmt.Errorf("scan world intelligence outbox: %w", err)
		}
		var batch ObservationBatch
		if err := json.Unmarshal(payload, &batch); err != nil {
			return nil, fmt.Errorf("decode world intelligence outbox batch: %w", err)
		}
		result = append(result, QueuedBatch{Batch: batch, Attempts: attempts})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read world intelligence outbox: %w", err)
	}
	return result, nil
}

func (store *DesktopStore) Confirm(ctx context.Context, batchID string, uploadedAt time.Time) error {
	if _, err := store.db.ExecContext(ctx, `DELETE FROM world_intel_outbox WHERE batch_id = ?`, batchID); err != nil {
		store.recordError(err)
		return fmt.Errorf("confirm world intelligence upload: %w", err)
	}
	store.setMetadata(ctx, "last_upload_at", uploadedAt.UTC().Format(time.RFC3339Nano))
	store.setMetadata(ctx, "last_upload_error", "")
	return nil
}

func (store *DesktopStore) Fail(ctx context.Context, batchID string, attempts int, message string, now time.Time) error {
	attempts++
	delay := time.Duration(1<<min(attempts, 10)) * time.Second
	if delay > time.Hour {
		delay = time.Hour
	}
	message = strings.TrimSpace(message)
	if len(message) > 1_000 {
		message = message[:1_000]
	}
	if _, err := store.db.ExecContext(ctx, `
		UPDATE world_intel_outbox
		SET attempt_count = ?, next_attempt_at = ?, last_error = ?
		WHERE batch_id = ?
	`, attempts, now.UTC().Add(delay).Format(time.RFC3339Nano), message, batchID); err != nil {
		store.recordError(err)
		return fmt.Errorf("defer world intelligence upload: %w", err)
	}
	store.setMetadata(ctx, "last_upload_error", message)
	return nil
}

func (store *DesktopStore) RecordUploadError(ctx context.Context, message string) {
	message = strings.TrimSpace(message)
	if len(message) > 1_000 {
		message = message[:1_000]
	}
	store.setMetadata(ctx, "last_upload_error", message)
}

func (store *DesktopStore) RecordScanSuccess(ctx context.Context, bucket time.Time, completedAt time.Time) {
	store.setMetadata(ctx, "last_scan_bucket", bucket.UTC().Format(time.RFC3339Nano))
	store.setMetadata(ctx, "last_scan_at", completedAt.UTC().Format(time.RFC3339Nano))
	store.setMetadata(ctx, "last_scan_error", "")
}

func (store *DesktopStore) RecordScanError(ctx context.Context, message string) {
	message = strings.TrimSpace(message)
	if len(message) > 1_000 {
		message = message[:1_000]
	}
	store.setMetadata(ctx, "last_scan_error", message)
}

func (store *DesktopStore) LastScanBucket(ctx context.Context) (time.Time, bool) {
	if store == nil || store.db == nil {
		return time.Time{}, false
	}
	value, found, err := metadataValue(ctx, store.db, "last_scan_bucket")
	if err != nil || !found {
		return time.Time{}, false
	}
	parsed, err := time.Parse(time.RFC3339Nano, value)
	return parsed.UTC(), err == nil
}

func (store *DesktopStore) Status(ctx context.Context) StoreStatus {
	status := StoreStatus{}
	if store == nil || store.db == nil {
		status.LastError = "World intelligence store is unavailable."
		return status
	}
	if err := store.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM world_intel_outbox`).Scan(&status.Pending); err != nil {
		status.LastError = err.Error()
	}
	for key, target := range map[string]**time.Time{
		"last_captured_at": &status.LastCapturedAt,
		"last_upload_at":   &status.LastUploadAt,
		"last_scan_at":     &status.LastScanAt,
	} {
		if value, found, _ := metadataValue(ctx, store.db, key); found {
			if parsed, err := time.Parse(time.RFC3339Nano, value); err == nil {
				parsed = parsed.UTC()
				*target = &parsed
			}
		}
	}
	if value, found, _ := metadataValue(ctx, store.db, "last_upload_error"); found {
		status.LastError = value
	}
	if value, found, _ := metadataValue(ctx, store.db, "last_scan_error"); found {
		status.LastScanError = value
	}
	return status
}

func (store *DesktopStore) PersistenceError() error {
	if store == nil {
		return nil
	}
	store.errorMu.RLock()
	defer store.errorMu.RUnlock()
	return store.lastErr
}

func (store *DesktopStore) Close() error {
	if store == nil || store.db == nil {
		return nil
	}
	return store.db.Close()
}

func (store *DesktopStore) setMetadata(ctx context.Context, key string, value string) {
	if _, err := store.db.ExecContext(ctx, `
		INSERT INTO world_intel_metadata(key, value) VALUES (?, ?)
		ON CONFLICT(key) DO UPDATE SET value = excluded.value
	`, key, value); err != nil {
		store.recordError(err)
	}
}

func (store *DesktopStore) recordError(err error) {
	store.errorMu.Lock()
	store.lastErr = err
	store.errorMu.Unlock()
}

type metadataQuerier interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

func metadataValue(ctx context.Context, query metadataQuerier, key string) (string, bool, error) {
	var value string
	err := query.QueryRowContext(ctx, `SELECT value FROM world_intel_metadata WHERE key = ?`, key).Scan(&value)
	if err == sql.ErrNoRows {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("read world intelligence metadata %q: %w", key, err)
	}
	return value, true, nil
}

func randomHex(bytes int) (string, error) {
	buffer := make([]byte, bytes)
	if _, err := rand.Read(buffer); err != nil {
		return "", fmt.Errorf("generate installation credential: %w", err)
	}
	return hex.EncodeToString(buffer), nil
}
