package Reports

import (
	"context"
	"testing"

	"CitadelDesktop/Server/History"
	"CitadelDesktop/Server/Intent"
	"CitadelDesktop/Server/State"
)

type managerTestIntents struct {
	submissions int
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
