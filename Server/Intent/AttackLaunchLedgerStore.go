package Intent

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

// AttackLaunchRow is one confirmed attack launch attributed to a feature
// channel. (OperationID, Ordinal) identify the launch, so recording the same
// receipt again can never count it twice.
type AttackLaunchRow struct {
	OperationID string
	Ordinal     int
	Feature     string
	LaunchedAt  time.Time
}

// AttackLaunchLedgerState is the persisted ledger at worker start.
type AttackLaunchLedgerState struct {
	// Initialized is false until the ledger was first seeded from the journal.
	Initialized bool
	// CoveredFrom is the instant from which the ledger is complete. Zero means
	// complete since the beginning; a window that starts at or before it must
	// be reported unavailable rather than as a smaller number.
	CoveredFrom time.Time
	Rows        []AttackLaunchRow
}

// TerminalHistory is what the operation journal still holds of finished
// operations, with the evidence needed to say how far back it is complete.
type TerminalHistory struct {
	Receipts []Receipt
	// PossiblyPruned is true when the journal is at its row limit, so rows older
	// than Oldest may have been removed.
	PossiblyPruned bool
	Oldest         time.Time
}

const attackLaunchLedgerSchema = `
	CREATE TABLE IF NOT EXISTS attack_launch_ledger (
		operation_id TEXT NOT NULL,
		ordinal INTEGER NOT NULL,
		feature TEXT NOT NULL,
		launched_at INTEGER NOT NULL,
		PRIMARY KEY (operation_id, ordinal)
	);
	CREATE INDEX IF NOT EXISTS attack_launch_ledger_time ON attack_launch_ledger(launched_at);
	CREATE TABLE IF NOT EXISTS attack_launch_ledger_meta (
		id INTEGER PRIMARY KEY CHECK (id = 1),
		covered_from INTEGER NOT NULL
	);
`

func (store *SQLiteOperationStore) ensureAttackLaunchLedger(ctx context.Context) error {
	if store == nil || store.db == nil {
		return fmt.Errorf("operation database is unavailable")
	}
	if _, err := store.db.ExecContext(ctx, attackLaunchLedgerSchema); err != nil {
		return fmt.Errorf("initialize attack launch ledger: %w", err)
	}
	return nil
}

func ledgerNanos(value time.Time) int64 {
	if value.IsZero() {
		return 0
	}
	return value.UTC().UnixNano()
}

func ledgerTime(nanos int64) time.Time {
	if nanos == 0 {
		return time.Time{}
	}
	return time.Unix(0, nanos).UTC()
}

// LoadAttackLaunchLedger returns the persisted ledger rows at or after since.
func (store *SQLiteOperationStore) LoadAttackLaunchLedger(ctx context.Context, since time.Time) (AttackLaunchLedgerState, error) {
	if err := store.ensureAttackLaunchLedger(ctx); err != nil {
		return AttackLaunchLedgerState{}, err
	}
	var state AttackLaunchLedgerState
	var coveredFrom int64
	err := store.db.QueryRowContext(ctx, `SELECT covered_from FROM attack_launch_ledger_meta WHERE id = 1`).Scan(&coveredFrom)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return state, nil
	case err != nil:
		return state, fmt.Errorf("read attack launch ledger: %w", err)
	}
	state.Initialized, state.CoveredFrom = true, ledgerTime(coveredFrom)
	rows, err := store.db.QueryContext(ctx, `
		SELECT operation_id, ordinal, feature, launched_at FROM attack_launch_ledger
		WHERE launched_at >= ? ORDER BY launched_at`, ledgerNanos(since))
	if err != nil {
		return state, fmt.Errorf("read attack launch ledger rows: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var row AttackLaunchRow
		var launchedAt int64
		if err := rows.Scan(&row.OperationID, &row.Ordinal, &row.Feature, &launchedAt); err != nil {
			return state, fmt.Errorf("scan attack launch ledger row: %w", err)
		}
		row.LaunchedAt = ledgerTime(launchedAt)
		state.Rows = append(state.Rows, row)
	}
	return state, rows.Err()
}

// InitializeAttackLaunchLedger seeds the ledger once, atomically with its
// coverage boundary. A ledger that is already initialized is left alone.
func (store *SQLiteOperationStore) InitializeAttackLaunchLedger(ctx context.Context, coveredFrom time.Time, seed []AttackLaunchRow) error {
	if err := store.ensureAttackLaunchLedger(ctx); err != nil {
		return err
	}
	tx, err := store.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin attack launch ledger seed: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	result, err := tx.ExecContext(ctx, `INSERT INTO attack_launch_ledger_meta (id, covered_from) VALUES (1, ?) ON CONFLICT(id) DO NOTHING`, ledgerNanos(coveredFrom))
	if err != nil {
		return fmt.Errorf("seed attack launch ledger: %w", err)
	}
	if inserted, _ := result.RowsAffected(); inserted == 1 {
		if err := insertAttackLaunchRows(ctx, tx, seed); err != nil {
			return err
		}
	}
	return tx.Commit()
}

type execer interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

func insertAttackLaunchRows(ctx context.Context, target execer, rows []AttackLaunchRow) error {
	for _, row := range rows {
		if _, err := target.ExecContext(ctx, `
			INSERT INTO attack_launch_ledger (operation_id, ordinal, feature, launched_at) VALUES (?, ?, ?, ?)
			ON CONFLICT(operation_id, ordinal) DO NOTHING`,
			row.OperationID, row.Ordinal, row.Feature, ledgerNanos(row.LaunchedAt)); err != nil {
			return fmt.Errorf("record attack launch: %w", err)
		}
	}
	return nil
}

// AppendAttackLaunches records launches. Rows already present are ignored.
func (store *SQLiteOperationStore) AppendAttackLaunches(ctx context.Context, rows []AttackLaunchRow) error {
	if len(rows) == 0 {
		return nil
	}
	tx, err := store.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin attack launch record: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if err := insertAttackLaunchRows(ctx, tx, rows); err != nil {
		return err
	}
	return tx.Commit()
}

// MarkAttackLaunchGap moves the coverage boundary forward to at, so windows
// that started earlier are reported unavailable instead of undercounted.
func (store *SQLiteOperationStore) MarkAttackLaunchGap(ctx context.Context, at time.Time) error {
	if _, err := store.db.ExecContext(ctx, `UPDATE attack_launch_ledger_meta SET covered_from = MAX(covered_from, ?) WHERE id = 1`, ledgerNanos(at)); err != nil {
		return fmt.Errorf("mark attack launch gap: %w", err)
	}
	return nil
}

// PruneAttackLaunches removes launches older than before.
func (store *SQLiteOperationStore) PruneAttackLaunches(ctx context.Context, before time.Time) error {
	if _, err := store.db.ExecContext(ctx, `DELETE FROM attack_launch_ledger WHERE launched_at < ?`, ledgerNanos(before)); err != nil {
		return fmt.Errorf("prune attack launch ledger: %w", err)
	}
	return nil
}

// TerminalOperationHistory reads every finished operation the journal retains,
// newest first. It is used once, to seed the ledger.
func (store *SQLiteOperationStore) TerminalOperationHistory(ctx context.Context) (TerminalHistory, error) {
	if store == nil || store.db == nil {
		return TerminalHistory{}, fmt.Errorf("operation database is unavailable")
	}
	rows, err := store.db.QueryContext(ctx, `
		SELECT receipt_json, updated_at FROM intent_operations
		WHERE status NOT IN (?, ?, ?, ?, ?)
		ORDER BY updated_at DESC`,
		StatusPlanning, StatusQueued, StatusRunning, StatusPaused, StatusReconciling)
	if err != nil {
		return TerminalHistory{}, fmt.Errorf("read finished operations: %w", err)
	}
	defer rows.Close()
	var history TerminalHistory
	for rows.Next() {
		var payload []byte
		var updatedAt string
		if err := rows.Scan(&payload, &updatedAt); err != nil {
			return history, fmt.Errorf("scan finished operation: %w", err)
		}
		var receipt Receipt
		if err := json.Unmarshal(payload, &receipt); err != nil {
			return history, fmt.Errorf("decode finished operation: %w", err)
		}
		history.Receipts = append(history.Receipts, receipt)
		if parsed, err := time.Parse(time.RFC3339Nano, updatedAt); err == nil &&
			(history.Oldest.IsZero() || parsed.Before(history.Oldest)) {
			history.Oldest = parsed.UTC()
		}
	}
	if err := rows.Err(); err != nil {
		return history, fmt.Errorf("read finished operations: %w", err)
	}
	// Pruning keeps exactly operationHistoryLimit finished rows and nothing else
	// ever removes one, so fewer rows than the limit means nothing was pruned.
	history.PossiblyPruned = len(history.Receipts) >= operationHistoryLimit
	return history, nil
}
