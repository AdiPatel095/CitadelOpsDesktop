package App

import (
	"context"
	"errors"
	"testing"
	"time"

	"CitadelDesktop/Server/Intent"
	"CitadelDesktop/Server/Telemetry"
)

var ledgerAttackFeatures = []struct {
	actor   string
	channel string
	intent  string
}{
	{"automation:autoTowers", Telemetry.ChannelAutoTowers, "tower.attack"},
	{"automation:autoFortress", Telemetry.ChannelAutoFortress, "fortress.attack"},
	{"automation:autoInvasion", Telemetry.ChannelAutoInvasion, "invasion.attack"},
	{"automation:autoNomad", Telemetry.ChannelAutoNomad, "nomad.camp.attack"},
	{"automation:autoAdvisor", Telemetry.ChannelAutoAdvisor, "advisor.attack"},
	{"automation:autoKhan", Telemetry.ChannelAutoKhan, "khan.attack"},
	{"automation:autoBeriWorld", Telemetry.ChannelAutoBeriWorld, "beri.attack"},
	{"automation:autoStorm", Telemetry.ChannelAutoStorm, "storm.attack"},
}

func launchStep(name string) Intent.Step {
	return Intent.Step{Name: name, CommandDependencies: &Intent.CommandDependencyRequest{Opcode: "cra"}}
}

func chainPlan(launches int) *Intent.Plan {
	steps := make([]Intent.Step, 0, launches*2)
	for index := 0; index < launches; index++ {
		steps = append(steps, launchStep("Build and launch attack with commander 4"), Intent.Step{Name: "Capture launch", Action: "attack.capture"})
	}
	return &Intent.Plan{Effect: Intent.EffectLaunch, Summary: "Chain attacks", Steps: steps}
}

// fixtureReceipts covers every branch of the launch definition: succeeded
// single and chained attacks, failed and partial receipts with confirmed launch
// steps, failed receipts with none, non-attack intents, supporting work, and
// actors without an attack feature.
func fixtureReceipts() []Intent.Receipt {
	var receipts []Intent.Receipt
	for index, feature := range ledgerAttackFeatures {
		id := feature.channel
		receipts = append(receipts,
			Intent.Receipt{ID: id + "-single", Intent: feature.intent, Actor: feature.actor, Status: Intent.StatusSucceeded,
				Plan: &Intent.Plan{Effect: Intent.EffectLaunch, Summary: "Attack", Steps: []Intent.Step{launchStep("Launch attack")}}},
			Intent.Receipt{ID: id + "-chain", Intent: feature.intent, Actor: feature.actor, Status: Intent.StatusSucceeded, Plan: chainPlan(2 + index%2)},
			Intent.Receipt{ID: id + "-partial", Intent: feature.intent, Actor: feature.actor, Status: Intent.StatusPartiallySucceeded,
				CompletedStepIndexes: []int{0, 1, 2}, Error: "the game rejected the action", Plan: chainPlan(3)},
			Intent.Receipt{ID: id + "-failed-with-launch", Intent: feature.intent, Actor: feature.actor, Status: Intent.StatusFailed,
				CompletedStepIndexes: []int{0}, Error: "timed out waiting for the game", Plan: chainPlan(2)},
			Intent.Receipt{ID: id + "-failed", Intent: feature.intent, Actor: feature.actor, Status: Intent.StatusFailed,
				Error: "the game rejected the action", Plan: chainPlan(2)},
			Intent.Receipt{ID: id + "-running", Intent: feature.intent, Actor: feature.actor, Status: Intent.StatusRunning, Plan: chainPlan(1)},
			Intent.Receipt{ID: id + "-skipped", Intent: feature.intent, Actor: feature.actor, Status: Intent.StatusSucceeded,
				Plan: &Intent.Plan{Effect: Intent.EffectLaunch, Summary: "Skip attack: no commander is available"}},
			Intent.Receipt{ID: id + "-scan", Intent: "tower.queue.scan", Actor: feature.actor, Status: Intent.StatusSucceeded,
				Plan: &Intent.Plan{Effect: Intent.EffectRead, Summary: "Refresh map", Steps: []Intent.Step{{Opcode: "gaa"}}}},
			Intent.Receipt{ID: id + "-queue", Intent: "production.enqueue", Actor: feature.actor, Status: Intent.StatusSucceeded,
				Plan: &Intent.Plan{Effect: Intent.EffectWrite, Summary: "Queue tools", Steps: []Intent.Step{{Opcode: "bup"}}}},
		)
	}
	receipts = append(receipts,
		Intent.Receipt{ID: "ui-attack", Intent: "nomad.camp.attack", Actor: "ui", Status: Intent.StatusSucceeded, Plan: chainPlan(2)},
		Intent.Receipt{ID: "bird-attack", Intent: "nomad.camp.attack", Actor: "automation:autoBird", Status: Intent.StatusSucceeded, Plan: chainPlan(2)},
		Intent.Receipt{ID: "rift-launch", Intent: "rift.maiden_wave.launch", Actor: "automation:riftMaidenRun", Status: Intent.StatusSucceeded, Plan: chainPlan(1)},
		Intent.Receipt{ID: "khan-cooldown", Intent: "khan.taunt", Actor: "automation:autoKhan:cooldown", Status: Intent.StatusSucceeded,
			Plan: &Intent.Plan{Effect: Intent.EffectLaunch, Summary: "Taunt", Steps: []Intent.Step{launchStep("Taunt")}}},
	)
	return receipts
}

type memoryLedgerStore struct {
	state       Intent.AttackLaunchLedgerState
	history     Intent.TerminalHistory
	appendErr   error
	appended    [][]Intent.AttackLaunchRow
	gaps        []time.Time
	historyRead int
}

func (store *memoryLedgerStore) LoadAttackLaunchLedger(_ context.Context, _ time.Time) (Intent.AttackLaunchLedgerState, error) {
	return store.state, nil
}

func (store *memoryLedgerStore) InitializeAttackLaunchLedger(_ context.Context, coveredFrom time.Time, seed []Intent.AttackLaunchRow) error {
	store.state = Intent.AttackLaunchLedgerState{Initialized: true, CoveredFrom: coveredFrom, Rows: seed}
	return nil
}

func (store *memoryLedgerStore) AppendAttackLaunches(_ context.Context, rows []Intent.AttackLaunchRow) error {
	if store.appendErr != nil {
		return store.appendErr
	}
	store.appended = append(store.appended, rows)
	store.state.Rows = append(store.state.Rows, rows...)
	return nil
}

func (store *memoryLedgerStore) MarkAttackLaunchGap(_ context.Context, at time.Time) error {
	store.gaps = append(store.gaps, at)
	return nil
}

func (store *memoryLedgerStore) PruneAttackLaunches(context.Context, time.Time) error { return nil }

func (store *memoryLedgerStore) TerminalOperationHistory(context.Context) (Intent.TerminalHistory, error) {
	store.historyRead++
	return store.history, nil
}

// The receipt-derived counters must agree exactly with the telemetry path for
// the same receipt sequence, for both windows and every feature.
func TestAttackLaunchLedgerMatchesTelemetryCounts(t *testing.T) {
	telemetry := Telemetry.NewStore(100)
	defer telemetry.Close()
	ledger, err := OpenAttackLaunchLedger(context.Background(), &memoryLedgerStore{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	application := &Application{Telemetry: telemetry, AttackLaunches: ledger}
	total := 0
	for _, receipt := range fixtureReceipts() {
		application.recordIntentLog(receipt)
	}
	now := time.Now()
	for _, window := range []time.Duration{time.Hour, 6 * time.Hour, 24 * time.Hour, 47 * time.Hour} {
		want, wantOK := telemetry.AttackLaunchCountsSince(now.Add(-window), now)
		got, gotOK := ledger.AttackLaunchCountsSince(now.Add(-window), now)
		if !wantOK || !gotOK {
			t.Fatalf("window %v availability telemetry=%v ledger=%v", window, wantOK, gotOK)
		}
		for _, channel := range Telemetry.AttackFeatureChannels() {
			if got[channel] != want[channel] {
				t.Errorf("window %v %s: ledger %d, telemetry %d", window, channel, got[channel], want[channel])
			}
			total += got[channel]
		}
	}
	if total == 0 {
		t.Fatal("fixture produced no launches; the comparison proved nothing")
	}
	// The per-feature expectation is also pinned, so both sources cannot drift together.
	counts, _ := ledger.AttackLaunchCountsSince(now.Add(-time.Hour), now)
	for index, feature := range ledgerAttackFeatures {
		chain := 2 + index%2
		// single(1) + chain + partial(steps 0 and 2 confirmed) + failed-with-launch(1);
		// Khan also has its cooldown lane's taunt (one more confirmed launch).
		want := 1 + chain + 2 + 1
		if feature.channel == Telemetry.ChannelAutoKhan {
			want++
		}
		if counts[feature.channel] != want {
			t.Errorf("%s hourly launches = %d, want %d", feature.channel, counts[feature.channel], want)
		}
	}
}

func TestAttackLaunchLedgerRecordsEachReceiptOnceAndKeepsTheLargestCount(t *testing.T) {
	ledger, err := OpenAttackLaunchLedger(context.Background(), &memoryLedgerStore{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	receipt := Intent.Receipt{ID: "op", Intent: "nomad.camp.attack", Actor: "automation:autoNomad", Status: Intent.StatusIndeterminate,
		CompletedStepIndexes: []int{0, 1}, Plan: chainPlan(3), Error: "the game did not confirm whether the action completed"}
	count := func() int {
		counts, ok := ledger.AttackLaunchCountsSince(time.Now().Add(-time.Hour), time.Now())
		if !ok {
			t.Fatal("window unavailable")
		}
		return counts[Telemetry.ChannelAutoNomad]
	}
	ledger.RecordReceipt(context.Background(), receipt)
	ledger.RecordReceipt(context.Background(), receipt)
	if count() != 1 {
		t.Fatalf("a receipt delivered twice counted %d times", count())
	}
	receipt.Status, receipt.CompletedStepIndexes = Intent.StatusSucceeded, []int{0, 1, 2, 3, 4, 5}
	ledger.RecordReceipt(context.Background(), receipt)
	if count() != 3 {
		t.Fatalf("reconciled receipt counted %d launches, want the 3 it confirmed", count())
	}
}

func TestAttackLaunchLedgerCoverageNeverReturnsATruncatedWindow(t *testing.T) {
	now := time.Now().UTC()
	// The journal was at its row limit, so it can only vouch from 30 minutes ago.
	store := &memoryLedgerStore{history: Intent.TerminalHistory{PossiblyPruned: true, Oldest: now.Add(-30 * time.Minute)}}
	ledger, err := OpenAttackLaunchLedger(context.Background(), store, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := ledger.AttackLaunchCountsSince(now.Add(-time.Hour), now); ok {
		t.Fatal("an hourly window reaching before the journal's coverage was reported as a number")
	}
	if _, ok := ledger.AttackLaunchCountsSince(now.Add(-10*time.Minute), now); !ok {
		t.Fatal("a window inside the ledger's coverage was reported unavailable")
	}
	if _, ok := ledger.AttackLaunchCountsSince(now.Add(-24*time.Hour), now); ok {
		t.Fatal("a daily window reaching before the journal's coverage was reported as a number")
	}
	// A journal that never pruned is complete since the beginning.
	complete, err := OpenAttackLaunchLedger(context.Background(), &memoryLedgerStore{history: Intent.TerminalHistory{Oldest: now.Add(-30 * time.Minute)}}, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := complete.AttackLaunchCountsSince(now.Add(-47*time.Hour), now); !ok {
		t.Fatal("an unpruned journal's ledger reported a covered window unavailable")
	}
	// Beyond the retention horizon or with a missing/future boundary: unavailable.
	for name, since := range map[string]time.Time{"missing": {}, "future": now.Add(time.Minute), "beyond retention": now.Add(-49 * time.Hour)} {
		if _, ok := complete.AttackLaunchCountsSince(since, now); ok {
			t.Errorf("%s boundary was reported available", name)
		}
	}
}

func TestAttackLaunchLedgerStreamGapWithdrawsEarlierWindows(t *testing.T) {
	now := time.Now().UTC()
	store := &memoryLedgerStore{}
	ledger, err := OpenAttackLaunchLedger(context.Background(), store, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := ledger.AttackLaunchCountsSince(now.Add(-time.Hour), now); !ok {
		t.Fatal("fresh ledger should cover the hour")
	}
	gapped := Intent.Receipt{ID: "after-gap", Intent: "nomad.camp.attack", Actor: "automation:autoNomad", Status: Intent.StatusSucceeded, Plan: chainPlan(1), StreamGap: true}
	ledger.RecordReceipt(context.Background(), gapped)
	if len(store.gaps) != 1 {
		t.Fatalf("gap was not persisted: %v", store.gaps)
	}
	if _, ok := ledger.AttackLaunchCountsSince(now.Add(-time.Hour), now); ok {
		t.Fatal("a window spanning dropped receipts was reported as a number")
	}
	later := now.Add(time.Minute)
	if counts, ok := ledger.AttackLaunchCountsSince(now.Add(time.Second), later); !ok || counts[Telemetry.ChannelAutoNomad] != 0 {
		t.Fatalf("window after the gap = %v %v", counts, ok)
	}
}

func TestAttackLaunchLedgerRetriesLaunchesThatFailedToPersist(t *testing.T) {
	store := &memoryLedgerStore{appendErr: errors.New("disk full")}
	ledger, err := OpenAttackLaunchLedger(context.Background(), store, nil)
	if err != nil {
		t.Fatal(err)
	}
	first := Intent.Receipt{ID: "one", Intent: "tower.attack", Actor: "automation:autoTowers", Status: Intent.StatusSucceeded, Plan: chainPlan(1)}
	ledger.RecordReceipt(context.Background(), first)
	if counts, _ := ledger.AttackLaunchCountsSince(time.Now().Add(-time.Hour), time.Now()); counts[Telemetry.ChannelAutoTowers] != 1 {
		t.Fatal("a launch that could not be persisted was not counted in memory")
	}
	store.appendErr = nil
	ledger.RecordReceipt(context.Background(), Intent.Receipt{ID: "two", Intent: "tower.attack", Actor: "automation:autoTowers", Status: Intent.StatusSucceeded, Plan: chainPlan(1)})
	persisted := 0
	for _, batch := range store.appended {
		persisted += len(batch)
	}
	if persisted != 2 {
		t.Fatalf("persisted %d launches after the store recovered, want both", persisted)
	}
}

func TestAttackLaunchLedgerSeedsOnceFromTheJournalAndThenIsAuthoritative(t *testing.T) {
	now := time.Now().UTC()
	completed := now.Add(-20 * time.Minute)
	old := now.Add(-72 * time.Hour)
	seedReceipt := Intent.Receipt{ID: "seed", Intent: "tower.attack", Actor: "automation:autoTowers", Status: Intent.StatusSucceeded, Plan: chainPlan(2), CompletedAt: &completed}
	expired := Intent.Receipt{ID: "expired", Intent: "tower.attack", Actor: "automation:autoTowers", Status: Intent.StatusSucceeded, Plan: chainPlan(5), CompletedAt: &old}
	store := &memoryLedgerStore{history: Intent.TerminalHistory{Receipts: []Intent.Receipt{seedReceipt, expired}}}
	ledger, err := OpenAttackLaunchLedger(context.Background(), store, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	if counts, ok := ledger.AttackLaunchCountsSince(now.Add(-time.Hour), now); !ok || counts[Telemetry.ChannelAutoTowers] != 2 {
		t.Fatalf("seeded counts = %v %v, want the two launches from 20 minutes ago and none from 72 hours ago", counts, ok)
	}
	// Opening again reads the ledger, not the journal.
	store.history = Intent.TerminalHistory{}
	reopened, err := OpenAttackLaunchLedger(context.Background(), store, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	if store.historyRead != 1 {
		t.Fatalf("journal read %d times, want once", store.historyRead)
	}
	if counts, _ := reopened.AttackLaunchCountsSince(now.Add(-time.Hour), now); counts[Telemetry.ChannelAutoTowers] != 2 {
		t.Fatalf("reopened counts = %v", counts)
	}
}
