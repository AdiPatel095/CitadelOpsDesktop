package Reports

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"CitadelDesktop/Server/History"
	"CitadelDesktop/Server/Intent"
	"CitadelDesktop/Server/State"
)

type managerTestIntents struct {
	submissions int
}

func TestManagerArchivesBattleReportsWithoutBlockingIngest(t *testing.T) {
	dataDir := t.TempDir()
	history, err := History.Open(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	analytics, err := OpenSQLiteStore(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = analytics.Close() })
	gameState := State.NewGameState()
	gameState.Account = State.AccountBindingState{UID: 44, WorldID: "world", PlayerID: 1}
	gameState.Player.ID = 1
	gameState.Reports.Notices[101] = State.ReportNotice{
		MessageID: 101, TypeID: 6, BattleKey: "battle#key", Status: "pending",
	}
	gameState.Reports.BattleCaptures[101] = State.BattleReportCapture{
		MessageID: 101, ReportID: 202, BattleKey: "battle#key",
		CapturedAt: time.Date(2026, 7, 11, 12, 0, 0, 0, time.UTC),
		Summary: json.RawMessage(`{
			"MID":101,"LID":202,"MT":6,"AHP":1,"DHP":0,
			"PI":[{"OID":1,"N":"Attacker"},{"OID":-2,"DUM":true,"N":"Target"}],
			"PBI":[[1,0,1000,-100],[-2,1,900,-900]],
			"AI":{"N":"Target","DP":-2,"AT":24,"K":4,"X":10,"Y":20}
		}`),
		Waves:   json.RawMessage(`{"LID":202,"W":[]}`),
		Details: json.RawMessage(`{"LID":202,"Y":[]}`),
	}
	state := State.NewStore(gameState)
	manager := NewManager(state, history, &managerTestIntents{}, analytics)
	started := make(chan struct{})
	release := make(chan struct{})
	manager.persistArchive = func(context.Context, battleArchiveTask) error {
		close(started)
		<-release
		return nil
	}
	ctx, cancel := context.WithCancel(t.Context())
	go manager.Run(ctx)
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("background report writer did not start")
	}
	snapshot := state.Snapshot()
	if snapshot.Reports.Notices[101].Status != "archiving" {
		t.Fatalf("queued report status = %q", snapshot.Reports.Notices[101].Status)
	}
	if _, exists := snapshot.Reports.BattleCaptures[101]; !exists {
		t.Fatal("capture was discarded before durable save completed")
	}
	close(release)
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		snapshot = state.Snapshot()
		if snapshot.Reports.Notices[101].Status == "archived" {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	if snapshot.Reports.Notices[101].Status != "archived" {
		t.Fatalf("completed report status = %q", snapshot.Reports.Notices[101].Status)
	}
	if _, exists := snapshot.Reports.BattleCaptures[101]; exists {
		t.Fatal("capture remained after durable save completed")
	}
	cancel()
	manager.Wait()
}

func TestManagerHoldsPossibleInvasionReportUntilRecoveryExhausts(t *testing.T) {
	history, err := History.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	reservedAt := now.Add(-time.Minute)
	gameState := State.NewGameState()
	gameState.Player.ID = 1
	for _, candidate := range []struct {
		messageID int64
		occurred  time.Time
	}{
		// This report is closest to dispatch, while the second is closest to
		// the eventual movement impact. Both must survive until exact recovery.
		{messageID: 101, occurred: reservedAt.Add(time.Second)},
		{messageID: 102, occurred: reservedAt.Add(55 * time.Second)},
	} {
		reportID := candidate.messageID + 101
		gameState.Reports.Notices[candidate.messageID] = State.ReportNotice{
			MessageID: candidate.messageID, TypeID: 6, BattleKey: "battle#invasion", Status: "pending", ObservedAt: now,
		}
		gameState.Reports.BattleCaptures[candidate.messageID] = State.BattleReportCapture{
			MessageID: candidate.messageID, ReportID: reportID, BattleKey: "battle#invasion",
			OccurredAt: candidate.occurred, CapturedAt: now,
			Summary: json.RawMessage(fmt.Sprintf(`{
				"MID":%d,"LID":%d,"MT":6,"AHP":1,"DHP":0,
				"PI":[{"OID":1,"N":"Attacker"},{"OID":-2,"DUM":true,"N":"Target"}],
				"PBI":[[1,0,1000,-100],[-2,1,900,-900]],
				"AI":{"N":"Target","DP":-2,"AT":34,"K":0,"X":120,"Y":121}
			}`, candidate.messageID, reportID)),
			Waves:   json.RawMessage(fmt.Sprintf(`{"LID":%d,"W":[]}`, reportID)),
			Details: json.RawMessage(fmt.Sprintf(`{"LID":%d,"Y":[]}`, reportID)),
		}
	}
	gameState.Invasion.ReserveTarget(State.InvasionTargetReservation{
		KingdomID: 0, EventID: 103, OccurrenceEndsAt: now.Add(time.Hour),
		TargetTypeID: 34, X: 120, Y: 121,
		SourceCastleID: 1, SourceX: 100, SourceY: 101, SourceKnown: true,
		CommanderID: 7, CommanderKnown: true,
		OperationID: "unresolved-cra", ReservedAt: reservedAt,
	})
	state := State.NewStore(gameState)
	manager := NewManager(state, history, &managerTestIntents{})

	next := manager.processNext(t.Context())
	held := state.ReadOnlyView()
	if next.IsZero() {
		t.Fatal("possible invasion reports did not schedule a hold deadline")
	}
	for _, messageID := range []int64{101, 102} {
		heldNotice, heldNoticeFound := held.LookupReportNotice(messageID)
		if !heldNoticeFound || heldNotice.Status != "pending" {
			t.Fatalf("possible invasion report %d was not held: notice=%#v found=%t", messageID, heldNotice, heldNoticeFound)
		}
		if _, exists := held.LookupBattleReportCapture(messageID); !exists {
			t.Fatalf("possible invasion report %d was archived before movement reconciliation", messageID)
		}
	}

	if _, err := state.ApplyComponents(State.Components(State.ComponentInvasion), func(current *State.GameState) ([]string, bool, error) {
		key := State.InvasionTargetKey(0, 120, 121)
		reservation, exists := current.Invasion.TargetReservations[key]
		if !exists {
			return nil, false, fmt.Errorf("invasion reservation disappeared before exhaustion")
		}
		reservation.ReconcileAttempts = State.InvasionTargetReservationMaxReconcileAttempts
		reservation.RecoveryExhaustedAt = now
		current.Invasion.TargetReservations[key] = reservation
		return []string{"invasion"}, true, nil
	}); err != nil {
		t.Fatal(err)
	}
	delete(manager.nextAttempt, 101)
	delete(manager.nextAttempt, 102)
	manager.processNext(t.Context())
	archived := state.ReadOnlyView()
	for _, messageID := range []int64{101, 102} {
		archivedNotice, archivedNoticeFound := archived.LookupReportNotice(messageID)
		if !archivedNoticeFound || archivedNotice.Status != "archived" {
			t.Fatalf("exhausted invasion report %d = %#v found=%t", messageID, archivedNotice, archivedNoticeFound)
		}
		if _, exists := archived.LookupBattleReportCapture(messageID); exists {
			t.Fatalf("exhausted invasion report %d was not archived", messageID)
		}
	}
	if _, reserved := archived.Invasion.TargetReservation(0, 120, 121); !reserved {
		t.Fatal("recovery exhaustion released the separate no-replay reservation")
	}
}

func (intents *managerTestIntents) Submit(context.Context, Intent.Request) Intent.Receipt {
	intents.submissions++
	return Intent.Receipt{Status: Intent.StatusSucceeded}
}

func TestManagerUsesHistoryToCompleteStalePersistedNotice(t *testing.T) {
	history, err := History.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := history.Append(History.CollectionBattleReports, BattleReport{MID: 99, LID: 100}); err != nil {
		t.Fatal(err)
	}
	gameState := State.NewGameState()
	gameState.Reports.Notices[99] = State.ReportNotice{MessageID: 99, TypeID: 6, Status: "error"}
	state := State.NewStore(gameState)
	intents := &managerTestIntents{}
	manager := NewManager(state, history, intents)
	manager.loadArchivedMessages()
	manager.processNext(t.Context())
	if intents.submissions != 0 {
		t.Fatalf("archived report caused %d fetches", intents.submissions)
	}
	if status := state.Snapshot().Reports.Notices[99].Status; status != "archived" {
		t.Fatalf("persisted notice status = %q", status)
	}
}

func TestManagerSuccessfulFetchUsesOneSecondSettleTimer(t *testing.T) {
	history, err := History.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	gameState := State.NewGameState()
	gameState.Session.LoggedIn = true
	gameState.Session.SocketReady = true
	gameState.Reports.Notices[7] = State.ReportNotice{MessageID: 7, TypeID: 3, Status: "pending"}
	intents := &managerTestIntents{}
	manager := NewManager(State.NewStore(gameState), history, intents)

	before := time.Now()
	next := manager.processNext(t.Context())
	if intents.submissions != 1 || next.Before(before.Add(900*time.Millisecond)) || next.After(before.Add(2*time.Second)) {
		t.Fatalf("first fetch = submissions %d next %s", intents.submissions, next)
	}
	manager.processNext(t.Context())
	if intents.submissions != 1 {
		t.Fatalf("settle guard allowed %d immediate fetches", intents.submissions)
	}
}

func TestManagerBoundsOnlyTerminalReportNotices(t *testing.T) {
	history, err := History.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	gameState := State.NewGameState()
	base := time.Date(2026, 8, 14, 12, 0, 0, 0, time.UTC)
	for index := 1; index <= reportNoticeRetention+88; index++ {
		gameState.Reports.Notices[int64(index)] = State.ReportNotice{
			MessageID: int64(index), TypeID: 6, Status: "archived", ObservedAt: base.Add(time.Duration(index) * time.Second),
		}
	}
	activeID := int64(10_000)
	gameState.Reports.Notices[activeID] = State.ReportNotice{
		MessageID: activeID, TypeID: 3, Status: "pending", ObservedAt: base.Add(-time.Hour),
	}
	state := State.NewStore(gameState)
	manager := NewManager(state, history, &managerTestIntents{})
	manager.processNext(t.Context())

	terminal := 0
	activeRetained := false
	state.ReadOnlyView().RangeReportNotices(func(messageID int64, notice State.ReportNotice) bool {
		if reportNoticeTerminal(notice.Status) {
			terminal++
		}
		if messageID == activeID {
			activeRetained = true
		}
		return true
	})
	if terminal != reportNoticeRetention || !activeRetained {
		t.Fatalf("retained terminal=%d active=%t", terminal, activeRetained)
	}
}

func TestReportManagerWakesOnlyForReportSessionOrGapEvents(t *testing.T) {
	for _, event := range []State.Event{
		{Domains: []string{"reports"}},
		{Domains: []string{"session"}},
		{Gap: true},
	} {
		if !reportStateEventRelevant(event) {
			t.Fatalf("relevant event was ignored: %#v", event)
		}
	}
	if reportStateEventRelevant(State.Event{Domains: []string{"resources", "movements"}}) {
		t.Fatal("unrelated state churn woke the report manager")
	}
}
