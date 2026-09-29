package State

import (
	"testing"
	"time"
)

func TestPendingCommandRequestsCorrelateFIFOWithinSessionAndWindow(t *testing.T) {
	base := time.Date(2026, 9, 29, 12, 0, 0, 0, time.UTC)
	state := NewGameState()
	state.Session.Generation = 4
	for index, id := range []EquipmentInstanceID{6558434871, 6558434872, 6558434873} {
		state.RecordPendingCommandRequest(PendingCommandRequest{
			Opcode: "SEQ", OperationID: "sale-1", SentAt: base.Add(time.Duration(index) * time.Millisecond), EquipmentID: id,
		})
	}
	state.RecordPendingCommandRequest(PendingCommandRequest{Opcode: "ahr", SentAt: base, HelpType: 6, CastleID: 77})

	taken, found := state.TakePendingCommandRequest("seq", "", base.Add(time.Second))
	if !found || taken.EquipmentID != 6558434871 {
		t.Fatalf("FIFO head = %#v found=%t", taken, found)
	}
	taken, found = state.TakePendingCommandRequest("seq", "sale-1", base.Add(time.Second))
	if !found || taken.EquipmentID != 6558434872 {
		t.Fatalf("causation match = %#v found=%t", taken, found)
	}
	if _, found = state.TakePendingCommandRequest("seq", "other-operation", base.Add(time.Second)); found {
		t.Fatal("reply of another operation consumed a pending sale")
	}
	if got := PendingCommandRequests(state, "seq"); len(got) != 1 || got[0].EquipmentID != 6558434873 {
		t.Fatalf("remaining seq requests = %#v", got)
	}
	if got := PendingCommandRequests(state, "ahr"); len(got) != 1 || got[0].CastleID != 77 {
		t.Fatalf("other opcode requests = %#v", got)
	}

	// A request older than the correlation window makes FIFO order ambiguous:
	// the expired request is discarded and the reply reconciles nothing.
	state.RecordPendingCommandRequest(PendingCommandRequest{Opcode: "seq", SentAt: base.Add(40 * time.Second), EquipmentID: 9})
	if taken, found = state.TakePendingCommandRequest("seq", "", base.Add(41*time.Second)); found {
		t.Fatalf("ambiguous reply was correlated to %#v", taken)
	}
	if got := PendingCommandRequests(state, "seq"); len(got) != 1 || got[0].EquipmentID != 9 {
		t.Fatalf("expired request was not dropped or fresh request was lost: %#v", got)
	}

	// A storage refresh requested after a lost reply resolves that sale: the
	// next reply must not be attributed to it.
	state.RecordPendingCommandRequest(PendingCommandRequest{Opcode: "seq", SentAt: base.Add(50 * time.Second), EquipmentID: 10})
	if !state.DropPendingCommandRequestsBefore("seq", base.Add(45*time.Second)) {
		t.Fatal("sales sent before the storage refresh were retained")
	}
	if got := PendingCommandRequests(state, "seq"); len(got) != 1 || got[0].EquipmentID != 10 {
		t.Fatalf("storage refresh dropped the wrong sales: %#v", got)
	}
	if got := PendingCommandRequests(state, "ahr"); len(got) != 1 {
		t.Fatalf("storage refresh dropped another opcode: %#v", got)
	}
	if taken, found = state.TakePendingCommandRequest("seq", "", base.Add(51*time.Second)); !found || taken.EquipmentID != 10 {
		t.Fatalf("reply after the refresh = %#v found=%t", taken, found)
	}

	state.Session.Generation = 5
	if got := PendingCommandRequests(state, "seq"); len(got) != 0 {
		t.Fatalf("previous session requests are still pending: %#v", got)
	}
	if _, found = state.TakePendingCommandRequest("seq", "", base.Add(42*time.Second)); found {
		t.Fatal("previous session request answered a new session reply")
	}
	if len(state.CommandContext.PendingRequests) != 0 {
		t.Fatalf("session change did not drop requests: %#v", state.CommandContext.PendingRequests)
	}
}

func TestPendingCommandRequestsAreBoundedPerOpcode(t *testing.T) {
	base := time.Date(2026, 9, 29, 12, 0, 0, 0, time.UTC)
	state := NewGameState()
	state.Session.Generation = 1
	state.RecordPendingCommandRequest(PendingCommandRequest{Opcode: "ahr", SentAt: base})
	for index := 0; index < PendingCommandRequestLimit+10; index++ {
		state.RecordPendingCommandRequest(PendingCommandRequest{
			Opcode: "seq", SentAt: base.Add(time.Duration(index) * time.Millisecond), EquipmentID: EquipmentInstanceID(index + 1),
		})
	}
	seq := PendingCommandRequests(state, "seq")
	if len(seq) != PendingCommandRequestLimit || seq[0].EquipmentID != 11 || len(PendingCommandRequests(state, "ahr")) != 1 {
		t.Fatalf("bounded requests seq=%d first=%d ahr=%d", len(seq), seq[0].EquipmentID, len(PendingCommandRequests(state, "ahr")))
	}
}

func TestPendingCommandRequestsNeverMutateAnEarlierStoreGeneration(t *testing.T) {
	base := time.Date(2026, 9, 29, 12, 0, 0, 0, time.UTC)
	initial := NewGameState()
	initial.Session.Generation = 1
	initial.RecordPendingCommandRequest(PendingCommandRequest{Opcode: "seq", SentAt: base, EquipmentID: 1})
	initial.RecordPendingCommandRequest(PendingCommandRequest{Opcode: "seq", SentAt: base, EquipmentID: 2})
	store := NewStore(initial)
	before := store.ReadOnlyView()
	if _, err := store.ApplyComponents(Components(ComponentCommandContext), func(state *GameState) ([]string, bool, error) {
		state.TakePendingCommandRequest("seq", "", base.Add(time.Second))
		return []string{"command-context"}, true, nil
	}); err != nil {
		t.Fatal(err)
	}
	if got := PendingCommandRequests(before, "seq"); len(got) != 2 || got[0].EquipmentID != 1 {
		t.Fatalf("earlier generation changed: %#v", got)
	}
	if got := PendingCommandRequests(store.ReadOnlyView(), "seq"); len(got) != 1 || got[0].EquipmentID != 2 {
		t.Fatalf("current generation = %#v", got)
	}
}

func TestRecruitmentHelpIneligibilityCoversRejectedListUntilItChanges(t *testing.T) {
	now := time.Date(2026, 9, 14, 12, 0, 0, 0, time.UTC)
	completes := now.Add(10 * time.Minute)
	state := NewGameState()
	state.Castles[77] = CastleState{ID: 77, Production: map[int]ProductionQueue{0: {
		LineID: 0, Active: &QueueItem{ProductionID: 201, Amount: 8, CompletesAt: &completes},
		Queued: []QueueItem{{ProductionID: 202, Amount: 8}},
	}}}
	state.Castles[88] = CastleState{ID: 88, Production: map[int]ProductionQueue{0: {
		LineID: 0, Queued: []QueueItem{{ProductionID: 301, Amount: 8}},
	}}}
	if !RecordRecruitmentHelpIneligibility(&state, 77, "ahr-op", now) {
		t.Fatal("AHR 269 record was not stored")
	}
	record := state.AllianceHelpRequests.IneligibleRecruitment[77]
	if len(record.ProductionIDs) != 2 || !record.Until.Equal(completes) || record.OperationID != "ahr-op" {
		t.Fatalf("record = %#v", record)
	}
	queue := state.Castles[77].Production[0]
	for _, item := range []QueueItem{*queue.Active, queue.Queued[0]} {
		if RecruitmentAllianceHelpItemEligible(state, 77, item, now.Add(time.Minute)) {
			t.Fatalf("rejected job %d is still eligible", item.ProductionID)
		}
	}
	if !RecruitmentAllianceHelpItemEligible(state, 88, state.Castles[88].Production[0].Queued[0], now.Add(time.Minute)) {
		t.Fatal("another castle's job was disabled")
	}
	if !RecruitmentAllianceHelpItemEligible(state, 77, QueueItem{ProductionID: 203, Amount: 8}, now.Add(time.Minute)) {
		t.Fatal("a job added after the rejection was disabled")
	}
	if !RecruitmentAllianceHelpItemEligible(state, 77, queue.Queued[0], completes) {
		t.Fatal("rejection outlived its bounded window")
	}

	// Once every rejected job leaves the queue the record is pruned.
	castle := state.Castles[77]
	castle.Production[0] = ProductionQueue{LineID: 0, Active: &QueueItem{ProductionID: 204, Amount: 8}}
	state.Castles[77] = castle
	if !PruneRecruitmentHelpIneligibility(&state, now.Add(time.Minute)) {
		t.Fatal("record for departed jobs was retained")
	}
	if _, found := state.AllianceHelpRequests.IneligibleRecruitment[77]; found {
		t.Fatalf("records = %#v", state.AllianceHelpRequests.IneligibleRecruitment)
	}
}

func TestRecruitmentHelpEligibilityRefusesInferredOrCompletedJobs(t *testing.T) {
	now := time.Date(2026, 9, 14, 12, 0, 0, 0, time.UTC)
	past := now.Add(-time.Second)
	state := NewGameState()
	for name, item := range map[string]QueueItem{
		"inferred RAH":  {ProductionID: 1, Amount: 8, AllianceHelpRequested: true},
		"missing id":    {Amount: 8},
		"completed job": {ProductionID: 2, Amount: 8, CompletesAt: &past},
		"below minimum": {ProductionID: 4, Amount: RecruitmentAllianceHelpMinimumUnits - 1},
	} {
		if RecruitmentAllianceHelpItemEligible(state, 77, item, now) {
			t.Errorf("%s was eligible", name)
		}
	}
	if !RecruitmentAllianceHelpItemEligible(state, 77, QueueItem{ProductionID: 3, Amount: RecruitmentAllianceHelpMinimumUnits}, now) {
		t.Fatal("queued explicit-RAH-false job was refused")
	}
}

func TestInventoryEquipmentMutationPersistsWithGemStacks(t *testing.T) {
	directory := t.TempDir()
	mutatedAt := time.Date(2026, 9, 29, 12, 0, 0, 0, time.UTC)
	store := NewStore(NewGameState())
	event, err := store.ApplyComponents(Components(ComponentInventory), func(state *GameState) ([]string, bool, error) {
		return []string{"inventory"}, state.MarkInventoryEquipmentMutated(mutatedAt), nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := SaveComponentSnapshot(directory, event, Components(event.Components...)); err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadSnapshot(directory)
	if err != nil {
		t.Fatal(err)
	}
	if !loaded.Inventory.EquipmentMutatedAt.Equal(mutatedAt) {
		t.Fatalf("persisted equipment mutation = %v", loaded.Inventory.EquipmentMutatedAt)
	}
	detached := NewGameState()
	detached.Inventory.EquipmentMutatedAt = mutatedAt
	if detached.MarkInventoryEquipmentMutated(mutatedAt.Add(-time.Second)) || !detached.Inventory.EquipmentMutatedAt.Equal(mutatedAt) {
		t.Fatal("an older sale moved the mutation marker backwards")
	}
}
