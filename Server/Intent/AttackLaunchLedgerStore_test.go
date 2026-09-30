package Intent

import (
	"testing"
	"time"
)

func TestAttackLaunchLedgerStoreRoundTripsAndSeedsOnce(t *testing.T) {
	store, err := OpenOperationStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	ctx := t.Context()
	base := time.Date(2026, 9, 30, 12, 0, 0, 123456789, time.UTC)

	state, err := store.LoadAttackLaunchLedger(ctx, base.Add(-48*time.Hour))
	if err != nil || state.Initialized || len(state.Rows) != 0 {
		t.Fatalf("empty ledger = %+v, %v", state, err)
	}
	coveredFrom := base.Add(-time.Hour)
	seed := []AttackLaunchRow{
		{OperationID: "b", Ordinal: 0, Feature: "autotowers", LaunchedAt: base.Add(-2 * time.Minute)},
		{OperationID: "a", Ordinal: 0, Feature: "autonomad", LaunchedAt: base.Add(-3 * time.Minute)},
		{OperationID: "a", Ordinal: 1, Feature: "autonomad", LaunchedAt: base.Add(-3 * time.Minute)},
	}
	if err := store.InitializeAttackLaunchLedger(ctx, coveredFrom, seed); err != nil {
		t.Fatal(err)
	}
	// A second seed is ignored entirely: the ledger, not the journal, is authoritative.
	if err := store.InitializeAttackLaunchLedger(ctx, time.Time{}, []AttackLaunchRow{{OperationID: "late", Feature: "autokhan", LaunchedAt: base}}); err != nil {
		t.Fatal(err)
	}
	state, err = store.LoadAttackLaunchLedger(ctx, base.Add(-48*time.Hour))
	if err != nil || !state.Initialized || !state.CoveredFrom.Equal(coveredFrom) || len(state.Rows) != 3 {
		t.Fatalf("seeded ledger = %+v, %v", state, err)
	}
	if !state.Rows[0].LaunchedAt.Equal(seed[1].LaunchedAt) || !state.Rows[2].LaunchedAt.Equal(seed[0].LaunchedAt) {
		t.Fatalf("rows are not in time order with nanosecond precision: %+v", state.Rows)
	}

	// Recording is idempotent per (operation, ordinal).
	more := []AttackLaunchRow{
		{OperationID: "a", Ordinal: 1, Feature: "autonomad", LaunchedAt: base},
		{OperationID: "a", Ordinal: 2, Feature: "autonomad", LaunchedAt: base},
	}
	if err := store.AppendAttackLaunches(ctx, more); err != nil {
		t.Fatal(err)
	}
	if err := store.AppendAttackLaunches(ctx, more); err != nil {
		t.Fatal(err)
	}
	state, _ = store.LoadAttackLaunchLedger(ctx, base.Add(-48*time.Hour))
	if len(state.Rows) != 4 {
		t.Fatalf("rows after duplicate appends = %d, want 4", len(state.Rows))
	}

	// A gap only ever moves the boundary forward.
	if err := store.MarkAttackLaunchGap(ctx, base); err != nil {
		t.Fatal(err)
	}
	if err := store.MarkAttackLaunchGap(ctx, base.Add(-time.Hour)); err != nil {
		t.Fatal(err)
	}
	state, _ = store.LoadAttackLaunchLedger(ctx, base.Add(-48*time.Hour))
	if !state.CoveredFrom.Equal(base) {
		t.Fatalf("coveredFrom = %v, want %v", state.CoveredFrom, base)
	}

	if err := store.PruneAttackLaunches(ctx, base.Add(-150*time.Second)); err != nil {
		t.Fatal(err)
	}
	state, _ = store.LoadAttackLaunchLedger(ctx, base.Add(-48*time.Hour))
	// a/0 and a/1 (the duplicate append at base was ignored) fall before the
	// cutoff; b/0 at -2m and the new a/2 at base remain.
	if len(state.Rows) != 2 {
		t.Fatalf("rows after prune = %d: %+v", len(state.Rows), state.Rows)
	}
}

func TestTerminalOperationHistoryReportsWhetherTheJournalMayHavePruned(t *testing.T) {
	seed := func(t *testing.T, store *SQLiteOperationStore, rows int) {
		t.Helper()
		if _, err := store.db.Exec(`
			WITH RECURSIVE sequence(value) AS (
				SELECT 1 UNION ALL SELECT value + 1 FROM sequence WHERE value < ?
			)
			INSERT INTO intent_operations (operation_id, request_hash, receipt_json, status, phase, submitted_at, updated_at)
			SELECT printf('terminal-%d', value), printf('hash-%d', value), printf('{"id":"terminal-%d"}', value), ?, ?,
				strftime('%Y-%m-%dT%H:%M:%SZ', 1767225600 + value, 'unixepoch'), strftime('%Y-%m-%dT%H:%M:%SZ', 1767225600 + value, 'unixepoch')
			FROM sequence`, rows, StatusSucceeded, EffectPhaseCompleted); err != nil {
			t.Fatal(err)
		}
		if _, err := store.db.Exec(`
			INSERT INTO intent_operations (operation_id, request_hash, receipt_json, status, phase, submitted_at, updated_at)
			VALUES ('active', 'active', '{"id":"active"}', ?, ?, '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z')`,
			StatusRunning, EffectPhaseDispatching); err != nil {
			t.Fatal(err)
		}
	}
	few, err := OpenOperationStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = few.Close() })
	seed(t, few, 25)
	history, err := few.TerminalOperationHistory(t.Context())
	if err != nil || len(history.Receipts) != 25 || history.PossiblyPruned {
		t.Fatalf("small journal: %d receipts, pruned=%v, %v", len(history.Receipts), history.PossiblyPruned, err)
	}
	if want := time.Unix(1767225601, 0).UTC(); !history.Oldest.Equal(want) {
		t.Fatalf("oldest = %v, want %v", history.Oldest, want)
	}
	if history.Receipts[0].ID != "terminal-25" {
		t.Fatalf("newest first expected, got %q", history.Receipts[0].ID)
	}

	full, err := OpenOperationStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = full.Close() })
	seed(t, full, operationHistoryLimit)
	history, err = full.TerminalOperationHistory(t.Context())
	if err != nil || len(history.Receipts) != operationHistoryLimit || !history.PossiblyPruned {
		t.Fatalf("journal at its limit: %d receipts, pruned=%v, %v", len(history.Receipts), history.PossiblyPruned, err)
	}
}
