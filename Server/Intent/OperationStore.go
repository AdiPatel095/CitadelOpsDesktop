package Intent

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	RuntimeKernel "CitadelDesktop/Server/Runtime"

	_ "modernc.org/sqlite"
)

var ErrIdempotencyConflict = errors.New("operation id was already used for a different request")

const operationHistoryLimit = 10_000

type StoredOperation struct {
	Receipt     Receipt
	RequestHash string
}

type OperationStore interface {
	Reserve(ctx context.Context, requestHash string, receipt Receipt) (Receipt, bool, error)
	Save(ctx context.Context, receipt Receipt) error
	Get(ctx context.Context, id string) (StoredOperation, bool, error)
	Recent(ctx context.Context, limit int) ([]StoredOperation, error)
	Recover(ctx context.Context) ([]StoredOperation, error)
	Close() error
}

type SQLiteOperationStore struct {
	db *sql.DB
}

func OpenOperationStore(dataDir string) (*SQLiteOperationStore, error) {
	dataDir = strings.TrimSpace(dataDir)
	if dataDir == "" {
		return nil, fmt.Errorf("operation data directory is required")
	}
	directory := filepath.Join(dataDir, "Runtime")
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return nil, fmt.Errorf("create operation directory: %w", err)
	}
	databaseURL, err := RuntimeKernel.SQLiteFileDSN(
		filepath.Join(directory, "Operations.sqlite"),
		"busy_timeout(5000)",
		"foreign_keys(1)",
	)
	if err != nil {
		return nil, fmt.Errorf("configure operation database path: %w", err)
	}
	db, err := sql.Open("sqlite", databaseURL)
	if err != nil {
		return nil, fmt.Errorf("open operation database: %w", err)
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	store := &SQLiteOperationStore{db: db}
	if err := store.initialize(context.Background()); err != nil {
		_ = db.Close()
		return nil, err
	}
	return store, nil
}

func (store *SQLiteOperationStore) initialize(ctx context.Context) error {
	if store == nil || store.db == nil {
		return fmt.Errorf("operation database is unavailable")
	}
	for _, statement := range []string{
		"PRAGMA journal_mode=WAL",
		"PRAGMA synchronous=FULL",
		"PRAGMA busy_timeout=5000",
		"PRAGMA foreign_keys=ON",
		"PRAGMA wal_autocheckpoint=1000",
	} {
		if _, err := store.db.ExecContext(ctx, statement); err != nil {
			return fmt.Errorf("configure operation database: %w", err)
		}
	}
	if _, err := store.db.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS intent_operations (
			operation_id TEXT PRIMARY KEY,
			request_hash TEXT NOT NULL,
			receipt_json BLOB NOT NULL,
			status TEXT NOT NULL,
			phase TEXT NOT NULL,
			submitted_at TEXT NOT NULL,
			updated_at TEXT NOT NULL
		);
		CREATE INDEX IF NOT EXISTS intent_operations_status
			ON intent_operations(status, updated_at);
	`); err != nil {
		return fmt.Errorf("initialize operation database: %w", err)
	}
	return store.pruneTerminalHistory(ctx)
}

func (store *SQLiteOperationStore) pruneTerminalHistory(ctx context.Context) error {
	result, err := store.db.ExecContext(ctx, `
		DELETE FROM intent_operations
		WHERE operation_id IN (
			SELECT operation_id
			FROM intent_operations
			WHERE status NOT IN (?, ?, ?, ?, ?)
			ORDER BY updated_at DESC
			LIMIT -1 OFFSET ?
		)
	`, StatusPlanning, StatusQueued, StatusRunning, StatusPaused, StatusReconciling, operationHistoryLimit)
	if err != nil {
		return fmt.Errorf("prune terminal operation history: %w", err)
	}
	if _, err := result.RowsAffected(); err != nil {
		return fmt.Errorf("inspect pruned operation history: %w", err)
	}
	return nil
}

func (store *SQLiteOperationStore) Reserve(
	ctx context.Context,
	requestHash string,
	receipt Receipt,
) (Receipt, bool, error) {
	if store == nil || store.db == nil {
		return Receipt{}, false, fmt.Errorf("operation database is unavailable")
	}
	requestHash = strings.TrimSpace(requestHash)
	if requestHash == "" || strings.TrimSpace(receipt.ID) == "" {
		return Receipt{}, false, fmt.Errorf("operation id and request hash are required")
	}
	payload, err := json.Marshal(receipt)
	if err != nil {
		return Receipt{}, false, fmt.Errorf("encode operation receipt: %w", err)
	}
	tx, err := store.db.BeginTx(ctx, nil)
	if err != nil {
		return Receipt{}, false, fmt.Errorf("begin operation reservation: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	result, err := tx.ExecContext(ctx, `
		INSERT INTO intent_operations (
			operation_id, request_hash, receipt_json, status, phase, submitted_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(operation_id) DO NOTHING
	`, receipt.ID, requestHash, payload, receipt.Status, receipt.Phase, receipt.SubmittedAt.Format(time.RFC3339Nano), now)
	if err != nil {
		return Receipt{}, false, fmt.Errorf("reserve operation: %w", err)
	}
	inserted, err := result.RowsAffected()
	if err != nil {
		return Receipt{}, false, fmt.Errorf("inspect operation reservation: %w", err)
	}
	if inserted == 1 {
		if err := tx.Commit(); err != nil {
			return Receipt{}, false, fmt.Errorf("commit operation reservation: %w", err)
		}
		return receipt, true, nil
	}
	var existingHash string
	var existingPayload []byte
	if err := tx.QueryRowContext(ctx, `
		SELECT request_hash, receipt_json FROM intent_operations WHERE operation_id = ?
	`, receipt.ID).Scan(&existingHash, &existingPayload); err != nil {
		return Receipt{}, false, fmt.Errorf("read existing operation: %w", err)
	}
	if existingHash != requestHash {
		return Receipt{}, false, fmt.Errorf("%w: %s", ErrIdempotencyConflict, receipt.ID)
	}
	var existing Receipt
	if err := json.Unmarshal(existingPayload, &existing); err != nil {
		return Receipt{}, false, fmt.Errorf("decode existing operation: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return Receipt{}, false, fmt.Errorf("finish operation lookup: %w", err)
	}
	return existing, false, nil
}

func (store *SQLiteOperationStore) Save(ctx context.Context, receipt Receipt) error {
	if store == nil || store.db == nil {
		return fmt.Errorf("operation database is unavailable")
	}
	payload, err := json.Marshal(receipt)
	if err != nil {
		return fmt.Errorf("encode operation receipt: %w", err)
	}
	result, err := store.db.ExecContext(ctx, `
		UPDATE intent_operations
		SET receipt_json = ?, status = ?, phase = ?, updated_at = ?
		WHERE operation_id = ?
	`, payload, receipt.Status, receipt.Phase, time.Now().UTC().Format(time.RFC3339Nano), receipt.ID)
	if err != nil {
		return fmt.Errorf("save operation receipt: %w", err)
	}
	updated, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("inspect operation receipt update: %w", err)
	}
	if updated != 1 {
		return fmt.Errorf("operation %q was not reserved", receipt.ID)
	}
	return nil
}

func (store *SQLiteOperationStore) Get(ctx context.Context, id string) (StoredOperation, bool, error) {
	if store == nil || store.db == nil {
		return StoredOperation{}, false, fmt.Errorf("operation database is unavailable")
	}
	var requestHash string
	var payload []byte
	err := store.db.QueryRowContext(ctx, `
		SELECT request_hash, receipt_json FROM intent_operations WHERE operation_id = ?
	`, strings.TrimSpace(id)).Scan(&requestHash, &payload)
	if errors.Is(err, sql.ErrNoRows) {
		return StoredOperation{}, false, nil
	}
	if err != nil {
		return StoredOperation{}, false, fmt.Errorf("read operation: %w", err)
	}
	operation, err := decodeStoredOperation(requestHash, payload)
	if err != nil {
		return StoredOperation{}, false, err
	}
	return operation, true, nil
}

func (store *SQLiteOperationStore) Recent(ctx context.Context, limit int) ([]StoredOperation, error) {
	if store == nil || store.db == nil {
		return nil, fmt.Errorf("operation database is unavailable")
	}
	if limit <= 0 || limit > operationHistoryLimit {
		limit = operationHistoryLimit
	}
	rows, err := store.db.QueryContext(ctx, `
		SELECT request_hash, receipt_json
		FROM intent_operations
		ORDER BY updated_at DESC
		LIMIT ?
	`, limit)
	if err != nil {
		return nil, fmt.Errorf("list recent operations: %w", err)
	}
	defer rows.Close()
	operations := make([]StoredOperation, 0)
	for rows.Next() {
		var requestHash string
		var payload []byte
		if err := rows.Scan(&requestHash, &payload); err != nil {
			return nil, fmt.Errorf("scan recent operation: %w", err)
		}
		operation, err := decodeStoredOperation(requestHash, payload)
		if err != nil {
			return nil, err
		}
		operations = append(operations, operation)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list recent operations: %w", err)
	}
	return operations, nil
}

func (store *SQLiteOperationStore) Recover(ctx context.Context) ([]StoredOperation, error) {
	if store == nil || store.db == nil {
		return nil, fmt.Errorf("operation database is unavailable")
	}
	tx, err := store.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin operation recovery: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	rows, err := tx.QueryContext(ctx, `
		SELECT request_hash, receipt_json FROM intent_operations
		WHERE status IN (?, ?, ?, ?, ?)
	`, StatusPlanning, StatusQueued, StatusRunning, StatusPaused, StatusReconciling)
	if err != nil {
		return nil, fmt.Errorf("read recoverable operations: %w", err)
	}
	operations := make([]StoredOperation, 0)
	for rows.Next() {
		var requestHash string
		var payload []byte
		if err := rows.Scan(&requestHash, &payload); err != nil {
			_ = rows.Close()
			return nil, fmt.Errorf("scan recoverable operation: %w", err)
		}
		operation, err := decodeStoredOperation(requestHash, payload)
		if err != nil {
			_ = rows.Close()
			return nil, err
		}
		operations = append(operations, operation)
	}
	if err := rows.Close(); err != nil {
		return nil, fmt.Errorf("close recoverable operations: %w", err)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read recoverable operations: %w", err)
	}
	now := time.Now().UTC()
	for index := range operations {
		receipt := operations[index].Receipt
		possiblyDispatched := receipt.Phase == EffectPhaseDispatching ||
			receipt.Phase == EffectPhaseSent || receipt.Phase == EffectPhaseAwaitingResponse ||
			receipt.Status == StatusRunning || receipt.Status == StatusPaused || receipt.Status == StatusReconciling
		mutating := receipt.Plan == nil || receipt.Plan.Effect != EffectRead
		if possiblyDispatched && mutating {
			receipt.Status = StatusIndeterminate
			receipt.Phase = EffectPhaseReconciliationRequired
			receipt = (&Engine{}).withFailure(receipt, errors.New(
				"application restarted after this operation may have dispatched an effect; automatic replay is prohibited",
			))
		} else {
			receipt.Status = StatusFailed
			receipt.Phase = EffectPhaseCompleted
			receipt = (&Engine{}).withFailure(receipt, errors.New("application restarted before this operation completed"))
		}
		receipt.CompletedAt = &now
		payload, err := json.Marshal(receipt)
		if err != nil {
			return nil, fmt.Errorf("encode recovered operation: %w", err)
		}
		if _, err := tx.ExecContext(ctx, `
			UPDATE intent_operations
			SET receipt_json = ?, status = ?, phase = ?, updated_at = ?
			WHERE operation_id = ?
		`, payload, receipt.Status, receipt.Phase, now.Format(time.RFC3339Nano), receipt.ID); err != nil {
			return nil, fmt.Errorf("recover operation %q: %w", receipt.ID, err)
		}
		operations[index].Receipt = receipt
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit operation recovery: %w", err)
	}
	return operations, nil
}

func (store *SQLiteOperationStore) Close() error {
	if store == nil || store.db == nil {
		return nil
	}
	return store.db.Close()
}

func decodeStoredOperation(requestHash string, payload []byte) (StoredOperation, error) {
	var receipt Receipt
	if err := json.Unmarshal(payload, &receipt); err != nil {
		return StoredOperation{}, fmt.Errorf("decode operation receipt: %w", err)
	}
	return StoredOperation{Receipt: receipt, RequestHash: requestHash}, nil
}

func requestFingerprint(request Request) string {
	arguments := bytes.TrimSpace(request.Arguments)
	if compact := new(bytes.Buffer); json.Compact(compact, arguments) == nil {
		arguments = compact.Bytes()
	}
	expectedRevision := ""
	if request.ExpectedRevision != nil {
		expectedRevision = fmt.Sprint(*request.ExpectedRevision)
	}
	payload, _ := json.Marshal(struct {
		Name             string `json:"name"`
		Actor            string `json:"actor"`
		Priority         int    `json:"priority"`
		Arguments        string `json:"arguments"`
		ExpectedRevision string `json:"expectedRevision"`
		DryRun           bool   `json:"dryRun"`
	}{
		Name: request.Name, Actor: request.Actor, Priority: int(request.Priority), Arguments: string(arguments),
		ExpectedRevision: expectedRevision, DryRun: request.DryRun,
	})
	hash := sha256.Sum256(payload)
	return hex.EncodeToString(hash[:])
}
