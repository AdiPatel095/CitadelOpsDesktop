package Reports

import (
	"context"
	"encoding/json"
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
