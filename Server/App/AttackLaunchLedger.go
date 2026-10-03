package App

import (
	"context"
	"sort"
	"strings"
	"sync"
	"time"

	"CitadelDesktop/Server/Intent"
	"CitadelDesktop/Server/Telemetry"
)

// attackLaunchStore is the durable side of the ledger. SQLiteOperationStore
// provides it inside the runtime's existing operation database.
type attackLaunchStore interface {
	LoadAttackLaunchLedger(ctx context.Context, since time.Time) (Intent.AttackLaunchLedgerState, error)
	InitializeAttackLaunchLedger(ctx context.Context, coveredFrom time.Time, seed []Intent.AttackLaunchRow) error
	AppendAttackLaunches(ctx context.Context, rows []Intent.AttackLaunchRow) error
	MarkAttackLaunchGap(ctx context.Context, at time.Time) error
	PruneAttackLaunches(ctx context.Context, before time.Time) error
	TerminalOperationHistory(ctx context.Context) (Intent.TerminalHistory, error)
}

const attackLaunchPruneInterval = time.Hour

type ledgerKey struct {
	operationID string
	ordinal     int
}

// AttackLaunchLedger counts confirmed feature attack launches for the hosted
// worker, which composes no telemetry store. A launch is exactly what the
// telemetry path counted (an INFO ATTACK activity from featureActivities on a
// feature channel), taken from the same intent receipts. Counts live in memory
// for O(log n) reads, are persisted in the operation database so a restart
// restores both windows, and never depend on the journal's row limit.
//
// A window that reaches back beyond what the ledger can vouch for is reported
// unavailable (ok == false) rather than as a smaller number.
type AttackLaunchLedger struct {
	store attackLaunchStore
	now   func() time.Time

	mu          sync.Mutex
	launches    map[string][]time.Time  // feature channel -> ascending launch times
	seen        map[ledgerKey]time.Time // identity -> launch time, so a receipt is never counted twice
	coveredFrom time.Time
	pending     []Intent.AttackLaunchRow // rows that failed to persist, retried on the next record
	lastPrune   time.Time
}

// OpenAttackLaunchLedger loads the persisted ledger, seeding it once from the
// operation journal (so deploying this change does not reset either badge).
func OpenAttackLaunchLedger(ctx context.Context, store attackLaunchStore, now func() time.Time) (*AttackLaunchLedger, error) {
	if now == nil {
		now = func() time.Time { return time.Now().UTC() }
	}
	ledger := &AttackLaunchLedger{
		store: store, now: now,
		launches: make(map[string][]time.Time, len(Telemetry.AttackFeatureChannels())),
		seen:     map[ledgerKey]time.Time{},
	}
	current := now().UTC()
	horizon := current.Add(-Telemetry.AttackLaunchRetention)
	state, err := store.LoadAttackLaunchLedger(ctx, horizon)
	if err != nil {
		return nil, err
	}
	if !state.Initialized {
		history, err := store.TerminalOperationHistory(ctx)
		if err != nil {
			return nil, err
		}
		var coveredFrom time.Time
		if history.PossiblyPruned {
			coveredFrom = history.Oldest
		}
		var seed []Intent.AttackLaunchRow
		for _, receipt := range history.Receipts {
			seed = append(seed, attackLaunchRows(receipt, receiptLaunchTime(receipt, current), horizon)...)
		}
		if err := store.InitializeAttackLaunchLedger(ctx, coveredFrom, seed); err != nil {
			return nil, err
		}
		if state, err = store.LoadAttackLaunchLedger(ctx, horizon); err != nil {
			return nil, err
		}
	}
	ledger.coveredFrom = state.CoveredFrom
	for _, row := range state.Rows {
		ledger.insertLocked(row)
	}
	ledger.lastPrune = current
	return ledger, nil
}

// receiptLaunchTime is when a receipt's launches happened: its completion, or
// the observation time for a receipt that carries none.
func receiptLaunchTime(receipt Intent.Receipt, fallback time.Time) time.Time {
	if receipt.CompletedAt != nil && !receipt.CompletedAt.IsZero() {
		return receipt.CompletedAt.UTC()
	}
	return fallback.UTC()
}

// attackLaunchRows derives the receipt's confirmed launches with the same
// definition the telemetry path used: featureActivities, keeping INFO ATTACK
// entries, attributed to the actor's feature channel. Non-attack intents can
// never produce one, so they are skipped before any activity text is built.
func attackLaunchRows(receipt Intent.Receipt, launchedAt time.Time, horizon time.Time) []Intent.AttackLaunchRow {
	if launchedAt.Before(horizon) || featureActivityEvent(receipt.Intent) != "ATTACK" {
		return nil
	}
	channel := Telemetry.FeatureChannelForActor(receipt.Actor)
	if !isAttackFeatureChannel(channel) || strings.TrimSpace(receipt.ID) == "" {
		return nil
	}
	var rows []Intent.AttackLaunchRow
	for _, activity := range featureActivities(receipt) {
		if activity.severity == "INFO" && activity.event == "ATTACK" {
			rows = append(rows, Intent.AttackLaunchRow{
				OperationID: receipt.ID, Ordinal: len(rows), Feature: channel, LaunchedAt: launchedAt,
			})
		}
	}
	return rows
}

func isAttackFeatureChannel(channel string) bool {
	for _, candidate := range Telemetry.AttackFeatureChannels() {
		if candidate == channel {
			return true
		}
	}
	return false
}

// RecordReceipt counts the launches of one intent receipt. It ignores receipts
// that are not finished, and one that was already recorded.
func (ledger *AttackLaunchLedger) RecordReceipt(ctx context.Context, receipt Intent.Receipt) {
	if ledger == nil {
		return
	}
	current := ledger.now().UTC()
	if receipt.StreamGap {
		// The subscription dropped receipts: some launches may be missing, so
		// nothing before now can be vouched for any more.
		ledger.markGap(ctx, current)
	}
	rows := attackLaunchRows(receipt, receiptLaunchTime(receipt, current), current.Add(-Telemetry.AttackLaunchRetention))
	ledger.mu.Lock()
	var fresh []Intent.AttackLaunchRow
	for _, row := range rows {
		if ledger.insertLocked(row) {
			fresh = append(fresh, row)
		}
	}
	batch := append(ledger.pending, fresh...)
	ledger.pending = nil
	pruneDue := current.Sub(ledger.lastPrune) >= attackLaunchPruneInterval
	if pruneDue {
		ledger.lastPrune = current
		ledger.pruneLocked(current.Add(-Telemetry.AttackLaunchRetention))
	}
	ledger.mu.Unlock()
	if len(batch) > 0 {
		if err := ledger.store.AppendAttackLaunches(ctx, batch); err != nil {
			ledger.mu.Lock()
			ledger.pending = append(batch, ledger.pending...)
			ledger.mu.Unlock()
		}
	}
	if pruneDue {
		_ = ledger.store.PruneAttackLaunches(ctx, current.Add(-Telemetry.AttackLaunchRetention))
	}
}

func (ledger *AttackLaunchLedger) markGap(ctx context.Context, at time.Time) {
	ledger.mu.Lock()
	if at.After(ledger.coveredFrom) {
		ledger.coveredFrom = at
	}
	ledger.mu.Unlock()
	_ = ledger.store.MarkAttackLaunchGap(ctx, at)
}

// insertLocked adds one launch in time order and reports whether it was new.
func (ledger *AttackLaunchLedger) insertLocked(row Intent.AttackLaunchRow) bool {
	key := ledgerKey{operationID: row.OperationID, ordinal: row.Ordinal}
	if _, exists := ledger.seen[key]; exists {
		return false
	}
	ledger.seen[key] = row.LaunchedAt
	times := ledger.launches[row.Feature]
	position := sort.Search(len(times), func(index int) bool { return times[index].After(row.LaunchedAt) })
	times = append(times, time.Time{})
	copy(times[position+1:], times[position:])
	times[position] = row.LaunchedAt
	ledger.launches[row.Feature] = times
	return true
}

func (ledger *AttackLaunchLedger) pruneLocked(before time.Time) {
	for feature, times := range ledger.launches {
		first := sort.Search(len(times), func(index int) bool { return !times[index].Before(before) })
		if first > 0 {
			ledger.launches[feature] = append([]time.Time(nil), times[first:]...)
		}
	}
	for key, launchedAt := range ledger.seen {
		if launchedAt.Before(before) {
			delete(ledger.seen, key)
		}
	}
}

// AttackLaunchCountsSince returns the launches per feature channel at or after
// since. ok is false when since is missing, in the future, beyond the retention
// horizon, or before the ledger can vouch for its own completeness.
func (ledger *AttackLaunchLedger) AttackLaunchCountsSince(since time.Time, now time.Time) (map[string]int, bool) {
	channels := Telemetry.AttackFeatureChannels()
	counts := make(map[string]int, len(channels))
	if ledger == nil {
		return counts, false
	}
	if now.IsZero() {
		now = ledger.now()
	}
	if since.IsZero() || since.After(now) || since.Before(now.Add(-Telemetry.AttackLaunchRetention)) {
		return counts, false
	}
	ledger.mu.Lock()
	defer ledger.mu.Unlock()
	if !ledger.coveredFrom.IsZero() && !since.After(ledger.coveredFrom) {
		return counts, false
	}
	for _, channel := range channels {
		times := ledger.launches[channel]
		first := sort.Search(len(times), func(index int) bool { return !times[index].Before(since) })
		counts[channel] = len(times) - first
	}
	return counts, true
}
