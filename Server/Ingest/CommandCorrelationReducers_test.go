package Ingest

import (
	"encoding/json"
	"testing"
	"time"

	"CitadelDesktop/Server/Protocol"
	"CitadelDesktop/Server/State"
)

func commandCorrelationPipeline(t *testing.T, gameState State.GameState) (*State.Store, *Pipeline) {
	t.Helper()
	store := State.NewStore(&gameState)
	registry := NewRegistry()
	if err := RegisterCoreReducers(registry); err != nil {
		t.Fatal(err)
	}
	return store, NewPipeline(store, nil, registry)
}

func handleCorrelationFrame(t *testing.T, pipeline *Pipeline, frame Protocol.Frame) {
	t.Helper()
	if _, err := pipeline.HandleFrame(t.Context(), frame); err != nil {
		t.Fatalf("handle %s %s: %v", frame.Direction, frame.Opcode, err)
	}
}

// Sanitized CIT-13 capture shape: EID 6558434871 sold, then a stale SEQ 214.
func TestEquipmentSaleResponsesReconcileOnlyTheCorrelatedInstance(t *testing.T) {
	base := time.Date(2026, 9, 17, 10, 0, 0, 0, time.UTC)
	gameState := State.NewGameState()
	gameState.Session.Generation = 3
	for _, id := range []State.EquipmentInstanceID{6558434871, 6558434872, 6558434873} {
		gameState.Inventory.Equipment[id] = State.EquipmentInstance{ID: id, DefinitionID: 100, Slot: 1, RarityID: 2}
	}
	store, pipeline := commandCorrelationPipeline(t, gameState)
	sell := func(id int64, at time.Time) {
		payload, _ := json.Marshal(map[string]int64{"EID": id, "LID": -1, "EX": 0, "LFID": -1})
		handleCorrelationFrame(t, pipeline, Protocol.Frame{
			Direction: Protocol.DirectionOutbound, Opcode: "seq", Payload: payload, ReceivedAt: at,
			CausationOperationID: "sale-op",
		})
	}
	reply := func(code int, at time.Time) {
		handleCorrelationFrame(t, pipeline, Protocol.Frame{
			Direction: Protocol.DirectionInbound, Opcode: "seq", ResponseCode: &code, ReceivedAt: at,
			Payload: json.RawMessage(`{"gcu":{"C1":1000},"esl":{"E":10,"TE":100}}`),
		})
	}

	sell(6558434871, base)
	if got := store.ReadOnlyView().Inventory.EquipmentMutatedAt; !got.Equal(base) {
		t.Fatalf("outbound sale did not mark storage stale: %v", got)
	}
	reply(0, base.Add(100*time.Millisecond))
	inventory := store.ReadOnlyView().Inventory.Equipment
	if _, sold := inventory[6558434871]; sold || len(inventory) != 2 {
		t.Fatalf("SEQ 0 did not remove exactly the sold instance: %#v", inventory)
	}

	sell(6558434872, base.Add(time.Second))
	reply(214, base.Add(time.Second+100*time.Millisecond))
	view := store.ReadOnlyView()
	if _, kept := view.Inventory.Equipment[6558434872]; !kept || len(view.Inventory.Equipment) != 2 {
		t.Fatalf("SEQ 214 was treated as a sale: %#v", view.Inventory.Equipment)
	}
	if !view.Inventory.EquipmentMutatedAt.Equal(base.Add(time.Second)) || len(State.PendingCommandRequests(&view, "seq")) != 0 {
		t.Fatalf("SEQ 214 state = mutated %v pending %#v", view.Inventory.EquipmentMutatedAt, view.CommandContext.PendingRequests)
	}

	// No reply: nothing is inferred and the unresolved identity dies with the session.
	sell(6558434873, base.Add(2*time.Second))
	accessorState1 := store.ReadOnlyView()
	if got := State.PendingCommandRequests(&accessorState1, "seq"); len(got) != 1 || got[0].EquipmentID != 6558434873 {
		t.Fatalf("unanswered sale identity = %#v", got)
	}
	if _, err := store.ApplyComponents(State.Components(State.ComponentSession), func(state *State.GameState) ([]string, bool, error) {
		state.Session.Generation++
		return []string{"session"}, true, nil
	}); err != nil {
		t.Fatal(err)
	}
	reply(0, base.Add(3*time.Second))
	view = store.ReadOnlyView()
	if _, kept := view.Inventory.Equipment[6558434873]; !kept || len(view.CommandContext.PendingRequests) != 0 {
		t.Fatalf("new-session reply consumed an old sale: inventory=%#v pending=%#v", view.Inventory.Equipment, view.CommandContext.PendingRequests)
	}
}

func TestGemSaleResponsesOnlyResolvePendingIdentity(t *testing.T) {
	base := time.Date(2026, 9, 17, 10, 0, 0, 0, time.UTC)
	gameState := State.NewGameState()
	gameState.Session.Generation = 1
	gameState.Inventory.GemStacks[20] = 2
	store, pipeline := commandCorrelationPipeline(t, gameState)
	handleCorrelationFrame(t, pipeline, Protocol.Frame{
		Direction: Protocol.DirectionOutbound, Opcode: "sge", ReceivedAt: base,
		Payload: json.RawMessage(`{"GID":20,"RGEM":0,"LFID":-1}`),
	})
	accessorState2 := store.ReadOnlyView()
	pending := State.PendingCommandRequests(&accessorState2, "sge")
	if len(pending) != 1 || pending[0].GemID != 20 || pending[0].RelicGem {
		t.Fatalf("pending gem sale = %#v", pending)
	}
	code := 0
	handleCorrelationFrame(t, pipeline, Protocol.Frame{
		Direction: Protocol.DirectionInbound, Opcode: "sge", ResponseCode: &code, ReceivedAt: base.Add(time.Second),
		Payload: json.RawMessage(`{}`),
	})
	view := store.ReadOnlyView()
	if len(view.CommandContext.PendingRequests) != 0 || view.Inventory.GemStacks[20] != 2 ||
		!view.Inventory.EquipmentMutatedAt.Equal(base) {
		t.Fatalf("gem sale response state = pending %#v stacks %#v mutated %v", view.CommandContext.PendingRequests, view.Inventory.GemStacks, view.Inventory.EquipmentMutatedAt)
	}
}

func TestAllianceHelpRejection269MarksOnlyTheRequestedRecruitmentList(t *testing.T) {
	base := time.Date(2026, 9, 14, 12, 0, 0, 0, time.UTC)
	gameState := State.NewGameState()
	gameState.Player.ID = 501
	gameState.Session.Generation = 5
	castle := newCastleState(77)
	castle.Focused = true
	castle.Production[0] = State.ProductionQueue{
		LineID: 0, ObservedAt: base, Active: &State.QueueItem{ProductionID: 201, Amount: 8},
		Queued: []State.QueueItem{{ProductionID: 202, Amount: 8}},
	}
	castle.Production[2] = State.ProductionQueue{LineID: 2, ObservedAt: base, Queued: []State.QueueItem{{ProductionID: 401}}}
	gameState.Castles[castle.ID] = castle
	other := newCastleState(88)
	other.Production[0] = State.ProductionQueue{LineID: 0, ObservedAt: base, Queued: []State.QueueItem{{ProductionID: 301, Amount: 8}}}
	gameState.Castles[other.ID] = other
	store, pipeline := commandCorrelationPipeline(t, gameState)

	request := func(payload string, at time.Time) {
		handleCorrelationFrame(t, pipeline, Protocol.Frame{
			Direction: Protocol.DirectionOutbound, Opcode: "ahr", Payload: json.RawMessage(payload), ReceivedAt: at,
		})
	}
	reject := func(code int, at time.Time) {
		handleCorrelationFrame(t, pipeline, Protocol.Frame{
			Direction: Protocol.DirectionInbound, Opcode: "ahr", ResponseCode: &code, ReceivedAt: at,
		})
	}

	// Duplicate 273 and hospital rejections keep their existing handling.
	request(`{"ID":0,"T":6}`, base)
	reject(273, base.Add(50*time.Millisecond))
	request(`{"ID":401,"T":2}`, base.Add(time.Second))
	reject(269, base.Add(time.Second+50*time.Millisecond))
	view := store.ReadOnlyView()
	if len(view.AllianceHelpRequests.IneligibleRecruitment) != 0 || len(view.CommandContext.PendingRequests) != 0 {
		t.Fatalf("273/hospital replies changed recruitment eligibility: %#v pending=%#v", view.AllianceHelpRequests.IneligibleRecruitment, view.CommandContext.PendingRequests)
	}

	request(`{"ID":0,"T":6}`, base.Add(2*time.Second))
	reject(269, base.Add(2*time.Second+50*time.Millisecond))
	view = store.ReadOnlyView()
	record, found := view.AllianceHelpRequests.IneligibleRecruitment[77]
	if !found || len(record.ProductionIDs) != 2 || record.ProductionIDs[0] != 201 || record.ProductionIDs[1] != 202 ||
		!record.Until.Equal(base.Add(2*time.Second+50*time.Millisecond).Add(State.RecruitmentHelpIneligibilityFallback)) {
		t.Fatalf("AHR 269 record = %#v found=%t", record, found)
	}
	if _, otherMarked := view.AllianceHelpRequests.IneligibleRecruitment[88]; otherMarked {
		t.Fatal("another castle was marked ineligible")
	}
	queue := view.Castles[77].Production[0]
	if queue.Active.AllianceHelpRequested || queue.Queued[0].AllianceHelpRequested {
		t.Fatalf("AHR 269 poisoned the RAH projection: %#v", queue)
	}

	// Recruitment snapshot without the rejected jobs clears the record.
	ok := 0
	handleCorrelationFrame(t, pipeline, Protocol.Frame{
		Direction: Protocol.DirectionInbound, Opcode: "spl", ResponseCode: &ok, ReceivedAt: base.Add(3 * time.Second),
		Payload: json.RawMessage(`{"LID":0,"PS":{"WID":1,"TUA":8,"RCT":60,"PID":203,"RAH":false},"QS":[]}`),
	})
	if _, still := store.ReadOnlyView().AllianceHelpRequests.IneligibleRecruitment[77]; still {
		t.Fatalf("record survived its jobs leaving the queue: %#v", store.ReadOnlyView().AllianceHelpRequests.IneligibleRecruitment)
	}
}

// Daniel review r4135053984: a lost SEQ reply must not shift the next reply
// onto the wrong item once a storage refresh has been requested after it.
func TestStorageRefreshResolvesLostSaleRepliesBeforeTheNextSale(t *testing.T) {
	base := time.Date(2026, 9, 17, 10, 0, 0, 0, time.UTC)
	gameState := State.NewGameState()
	gameState.Session.Generation = 2
	for _, id := range []State.EquipmentInstanceID{6558434871, 6558434872} {
		gameState.Inventory.Equipment[id] = State.EquipmentInstance{ID: id, DefinitionID: 100, Slot: 1, RarityID: 2}
	}
	store, pipeline := commandCorrelationPipeline(t, gameState)
	outbound := func(opcode string, payload string, at time.Time) {
		handleCorrelationFrame(t, pipeline, Protocol.Frame{
			Direction: Protocol.DirectionOutbound, Opcode: opcode, Payload: json.RawMessage(payload), ReceivedAt: at,
		})
	}
	inbound := func(opcode string, payload string, at time.Time) {
		code := 0
		handleCorrelationFrame(t, pipeline, Protocol.Frame{
			Direction: Protocol.DirectionInbound, Opcode: opcode, ResponseCode: &code, Payload: json.RawMessage(payload), ReceivedAt: at,
		})
	}

	outbound("seq", `{"EID":6558434871,"LID":-1,"EX":0,"LFID":-1}`, base) // reply lost
	outbound("gei", `{}`, base.Add(time.Second))
	inbound("gei", `{"I":[[6558434871,1,0,2,100,[],100],[6558434872,1,0,2,100,[],100]]}`, base.Add(2*time.Second))
	accessorState3 := store.ReadOnlyView()
	if pending := State.PendingCommandRequests(&accessorState3, "seq"); len(pending) != 0 {
		t.Fatalf("storage refresh left the lost sale pending: %#v", pending)
	}
	outbound("seq", `{"EID":6558434872,"LID":-1,"EX":0,"LFID":-1}`, base.Add(3*time.Second))
	inbound("seq", `{}`, base.Add(3*time.Second+100*time.Millisecond))
	inventory := store.ReadOnlyView().Inventory.Equipment
	if _, kept := inventory[6558434871]; !kept {
		t.Fatal("the next sale reply was attributed to the lost sale")
	}
	if _, sold := inventory[6558434872]; sold {
		t.Fatal("the answered sale was not reconciled")
	}
}
