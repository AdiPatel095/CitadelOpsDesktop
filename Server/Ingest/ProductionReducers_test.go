package Ingest

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"CitadelDesktop/Server/GameData"
	"CitadelDesktop/Server/Protocol"
	"CitadelDesktop/Server/State"
)

func TestProductionSnapshotUsesFocusedCastleAndPreservesCapacity(t *testing.T) {
	gameState := State.NewGameState()
	castle := newCastleState(77)
	castle.Focused = true
	gameState.Castles[castle.ID] = castle
	responseCode := 0
	frame := Protocol.Frame{
		Direction: Protocol.DirectionInbound, Opcode: "spl", ResponseCode: &responseCode,
		ReceivedAt: time.Date(2026, 7, 11, 12, 0, 0, 0, time.UTC),
		Payload: json.RawMessage(`{
			"PS":{"WID":489,"TUA":6,"RCT":75,"PID":11,"SPID":12,"RAH":true},
			"QS":[{"P":{"WID":489,"TUA":324,"PID":12,"RAH":true}},{"P":{"WID":489,"TUA":444,"PID":13}},{"SI":{"RUT":-1}}],
			"LID":0
		}`),
	}
	domains, changed, err := reduceProductionSnapshot(context.Background(), frame, &gameState, nil)
	if err != nil {
		t.Fatalf("reduce production snapshot: %v", err)
	}
	if !changed || len(domains) == 0 {
		t.Fatal("expected production state change")
	}
	queue := gameState.Castles[77].Production[0]
	if queue.Capacity != 3 {
		t.Fatalf("capacity = %d, want 3", queue.Capacity)
	}
	if queue.Active == nil || queue.Active.Definition.ID != 489 || queue.Active.Amount != 6 {
		t.Fatalf("unexpected active item: %#v", queue.Active)
	}
	if queue.Active.ProductionID != 12 || !queue.Active.AllianceHelpAvailable || !queue.Active.AllianceHelpRequested {
		t.Fatalf("unexpected active alliance-help state: %#v", queue.Active)
	}
	if len(queue.Queued) != 2 || queue.Queued[1].Amount != 444 {
		t.Fatalf("unexpected queued items: %#v", queue.Queued)
	}
	if queue.Queued[0].ProductionID != 12 || !queue.Queued[0].AllianceHelpAvailable || !queue.Queued[0].AllianceHelpRequested {
		t.Fatalf("unexpected queued alliance-help state: %#v", queue.Queued)
	}
	if queue.Queued[1].ProductionID != 13 || !queue.Queued[1].AllianceHelpAvailable || queue.Queued[1].AllianceHelpRequested {
		t.Fatalf("unexpected queued alliance-help state: %#v", queue.Queued)
	}
}

func TestProductionSnapshotCountsEmptyEffectSlotsAsCapacity(t *testing.T) {
	// Capacity effects (event boosters, premium slot purchases, castellan
	// bonuses) grant slots that arrive in QS as entries with no product and
	// no rental or VIP flag. Every owned slot is capacity, occupied or not.
	gameState := State.NewGameState()
	castle := newCastleState(88)
	castle.Focused = true
	gameState.Castles[castle.ID] = castle
	responseCode := 0
	frame := Protocol.Frame{
		Direction: Protocol.DirectionInbound, Opcode: "spl", ResponseCode: &responseCode,
		ReceivedAt: time.Date(2026, 8, 17, 12, 0, 0, 0, time.UTC),
		Payload: json.RawMessage(`{
			"PS":{"WID":489,"TUA":6,"RCT":75,"PID":11},
			"QS":[{"P":{"WID":489,"TUA":100,"PID":12}},{},{"SI":{"VIP":1}},{"SI":{"RUT":900}},{}],
			"LID":0
		}`),
	}
	if _, changed, err := reduceProductionSnapshot(context.Background(), frame, &gameState, nil); err != nil || !changed {
		t.Fatalf("reduce production snapshot changed=%t err=%v", changed, err)
	}
	queue := gameState.Castles[88].Production[0]
	if queue.Capacity != 5 {
		t.Fatalf("capacity = %d, want 5 (one occupied + two plain empty + one empty VIP + one empty rented)", queue.Capacity)
	}
	if len(queue.Queued) != 1 {
		t.Fatalf("queued = %d, want 1", len(queue.Queued))
	}
}

func TestProductionSnapshotPreservesRequestedAllianceHelpByProductionID(t *testing.T) {
	gameState := State.NewGameState()
	castle := newCastleState(77)
	castle.Focused = true
	castle.Production[0] = State.ProductionQueue{
		LineID: 0,
		Active: &State.QueueItem{ProductionID: 11, AllianceHelpRequested: true},
	}
	gameState.Castles[castle.ID] = castle
	responseCode := 0
	frame := Protocol.Frame{
		Direction: Protocol.DirectionInbound, Opcode: "spl", ResponseCode: &responseCode,
		ReceivedAt: time.Date(2026, 7, 11, 12, 0, 0, 0, time.UTC),
		Payload:    json.RawMessage(`{"PS":{"WID":489,"TUA":6,"PID":11,"RAH":false},"LID":0}`),
	}
	if _, changed, err := reduceProductionSnapshot(context.Background(), frame, &gameState, nil); err != nil || !changed {
		t.Fatalf("reduce production snapshot changed=%t err=%v", changed, err)
	}
	if !gameState.Castles[77].Production[0].Active.AllianceHelpRequested {
		t.Fatal("requested alliance help was not preserved")
	}
}

func TestHospitalCompactProductionSnapshotParsesAllianceHelpJob(t *testing.T) {
	gameState := State.NewGameState()
	castle := newCastleState(77)
	castle.Focused = true
	castle.Production[2] = State.ProductionQueue{
		LineID: 2,
		Queued: []State.QueueItem{{ProductionID: 1374447446, AllianceHelpRequested: true}},
	}
	gameState.Castles[castle.ID] = castle
	responseCode := 0
	frame := Protocol.Frame{
		Direction: Protocol.DirectionInbound, Opcode: "spl", ResponseCode: &responseCode,
		ReceivedAt: time.Date(2026, 7, 11, 12, 0, 0, 0, time.UTC),
		Payload:    json.RawMessage(`{"LID":2,"PIDL":[[238,5,323,1378,0,1374447446,0,-1],[-1,0,0,0,0,0,0,-1]]}`),
	}
	if _, changed, err := reduceProductionSnapshot(context.Background(), frame, &gameState, nil); err != nil || !changed {
		t.Fatalf("reduce production snapshot changed=%t err=%v", changed, err)
	}
	queue := gameState.Castles[77].Production[2]
	if queue.Capacity != 2 || len(queue.Queued) != 1 {
		t.Fatalf("unexpected compact hospital queue: %#v", queue)
	}
	item := queue.Queued[0]
	if item.ProductionID != 1374447446 || !item.AllianceHelpAvailable || !item.AllianceHelpRequested {
		t.Fatalf("unexpected compact hospital alliance-help state: %#v", item)
	}
}

func TestRecruitmentAllianceHelpEventMarksWholeCastleQueue(t *testing.T) {
	gameState := State.NewGameState()
	gameState.Player.ID = 501
	castle := newCastleState(77)
	castle.Production[0] = State.ProductionQueue{
		LineID: 0,
		Active: &State.QueueItem{ProductionID: 11},
		Queued: []State.QueueItem{{ProductionID: 11}, {ProductionID: 12}},
	}
	gameState.Castles[castle.ID] = castle
	responseCode := 0
	frame := Protocol.Frame{
		Direction: Protocol.DirectionInbound, Opcode: "ahh", ResponseCode: &responseCode,
		Payload: json.RawMessage(`{"PID":501,"TID":6,"OP":{"AID":77,"SID":0,"RLID":0}}`),
	}
	if _, changed, err := reduceAllianceHelpRequest(context.Background(), frame, &gameState, nil); err != nil || !changed {
		t.Fatalf("reduce alliance help changed=%t err=%v", changed, err)
	}
	queue := gameState.Castles[77].Production[0]
	if queue.Active == nil || !queue.Active.AllianceHelpRequested {
		t.Fatalf("active recruitment item was not marked: %#v", queue.Active)
	}
	for index, item := range queue.Queued {
		if !item.AllianceHelpRequested {
			t.Fatalf("queued recruitment item %d was not marked: %#v", index, item)
		}
	}
	if !State.HasOutstandingRecruitmentAllianceHelpRequest(gameState, 77) {
		t.Fatal("successful recruitment help was not retained for the castle")
	}
}

func TestAllianceHelpListKeepsNewRecruitQueueGuardedByCastle(t *testing.T) {
	gameState := State.NewGameState()
	gameState.Player.ID = 501
	castle := newCastleState(77)
	castle.Focused = true
	castle.Production[0] = State.ProductionQueue{
		LineID: 0,
		Active: &State.QueueItem{ProductionID: 11},
	}
	gameState.Castles[castle.ID] = castle
	responseCode := 0
	listFrame := Protocol.Frame{
		Direction: Protocol.DirectionInbound, Opcode: "ahl", ResponseCode: &responseCode,
		ReceivedAt: time.Date(2026, 7, 28, 20, 5, 41, 0, time.UTC),
		Payload: json.RawMessage(`{"AHL":[
			{"PID":999,"TID":6,"OP":{"AID":88,"RLID":0}},
			{"PID":501,"TID":6,"OP":{"AID":77,"RLID":0}},
			{"PID":501,"TID":6,"OP":{"AID":77,"RLID":0}}
		]}`),
	}
	if _, changed, err := reduceAllianceHelpRequest(t.Context(), listFrame, &gameState, nil); err != nil || !changed {
		t.Fatalf("reduce alliance help list changed=%t err=%v", changed, err)
	}
	if got := gameState.AllianceHelpRequests.RecruitmentCastleIDs; len(got) != 1 || got[0] != 77 {
		t.Fatalf("authoritative recruitment-help castles = %#v", got)
	}
	snapshotFrame := Protocol.Frame{
		Direction: Protocol.DirectionInbound, Opcode: "spl", ResponseCode: &responseCode,
		ReceivedAt: listFrame.ReceivedAt.Add(time.Minute),
		Payload: json.RawMessage(`{
			"PS":{"WID":489,"TUA":6,"PID":21,"SPID":22},
			"QS":[{"P":{"WID":489,"TUA":444,"PID":22}},{"P":{"WID":489,"TUA":444,"PID":23}}],
			"LID":0
		}`),
	}
	if _, changed, err := reduceProductionSnapshot(t.Context(), snapshotFrame, &gameState, nil); err != nil || !changed {
		t.Fatalf("reduce replacement production snapshot changed=%t err=%v", changed, err)
	}
	queue := gameState.Castles[77].Production[0]
	if queue.Active == nil || !queue.Active.AllianceHelpRequested {
		t.Fatalf("replacement active recruitment item was not guarded: %#v", queue.Active)
	}
	for index, item := range queue.Queued {
		if !item.AllianceHelpRequested {
			t.Fatalf("replacement queued recruitment item %d was not guarded: %#v", index, item)
		}
	}
}

func TestAllianceHelpListMarksOnlyOwnHospitalJob(t *testing.T) {
	gameState := State.NewGameState()
	gameState.Player.ID = 501
	castle := newCastleState(77)
	castle.Production[2] = State.ProductionQueue{
		LineID: 2,
		Queued: []State.QueueItem{{ProductionID: 21}, {ProductionID: 22}},
	}
	gameState.Castles[castle.ID] = castle
	responseCode := 0
	frame := Protocol.Frame{
		Direction: Protocol.DirectionInbound, Opcode: "ahl", ResponseCode: &responseCode,
		ReceivedAt: time.Date(2026, 7, 21, 20, 2, 56, 0, time.UTC),
		Payload: json.RawMessage(`{"AHL":[
			{"PID":999,"TID":2,"OP":{"AID":77,"RID":22}},
			{"PID":501,"TID":2,"OP":{"AID":77,"RID":21}},
			{"PID":501,"TID":2,"OP":{"AID":77,"RID":23}}
		]}`),
	}
	if _, changed, err := reduceAllianceHelpRequest(context.Background(), frame, &gameState, nil); err != nil || !changed {
		t.Fatalf("reduce alliance help list changed=%t err=%v", changed, err)
	}
	queued := gameState.Castles[77].Production[2].Queued
	if !queued[0].AllianceHelpRequested || queued[1].AllianceHelpRequested {
		t.Fatalf("unexpected hospital alliance-help state: %#v", queued)
	}
	if got := State.OutstandingHospitalAllianceHelpRequests(gameState); got != 2 {
		t.Fatalf("authoritative hospital alliance-help count = %d, want 2", got)
	}
	if !State.HasOutstandingHospitalAllianceHelpRequest(gameState, 23) {
		t.Fatal("completed server-side hospital help request was not retained")
	}
}

func TestHospitalCompactSnapshotTreatsAppliedHelpReductionAsRequested(t *testing.T) {
	gameState := State.NewGameState()
	castle := newCastleState(77)
	castle.Focused = true
	gameState.Castles[castle.ID] = castle
	responseCode := 0
	frame := Protocol.Frame{
		Direction: Protocol.DirectionInbound, Opcode: "hru", ResponseCode: &responseCode,
		ReceivedAt: time.Date(2026, 7, 22, 0, 21, 47, 0, time.UTC),
		Payload: json.RawMessage(`{"spl":{"LID":2,"PIDL":[
			[216,2,98,1237,33,1708231396,0,-1],
			[216,4,331,1237,0,454401568,0,-1]
		]}}`),
	}
	if _, changed, err := reduceProductionSnapshot(t.Context(), frame, &gameState, nil); err != nil || !changed {
		t.Fatalf("reduce compact hospital snapshot changed=%t err=%v", changed, err)
	}
	queued := gameState.Castles[77].Production[2].Queued
	if len(queued) != 2 || !queued[0].AllianceHelpRequested || queued[1].AllianceHelpRequested {
		t.Fatalf("compact hospital help state = %#v", queued)
	}
}

func TestOutboundAllianceHelpMarksHospitalJobBeforeResponse(t *testing.T) {
	gameState := State.NewGameState()
	castle := newCastleState(77)
	castle.Focused = true
	castle.Production[2] = State.ProductionQueue{
		LineID: 2, Queued: []State.QueueItem{{ProductionID: 201}, {ProductionID: 202}},
	}
	gameState.Castles[castle.ID] = castle
	frame := Protocol.Frame{
		Direction: Protocol.DirectionOutbound, Opcode: "ahr",
		Payload: json.RawMessage(`{"ID":201,"T":2}`),
	}
	if _, changed, err := reduceAllianceHelpCommand(t.Context(), frame, &gameState, nil); err != nil || !changed {
		t.Fatalf("reduce outbound alliance help changed=%t err=%v", changed, err)
	}
	queued := gameState.Castles[77].Production[2].Queued
	if !queued[0].AllianceHelpRequested || queued[1].AllianceHelpRequested {
		t.Fatalf("outbound hospital help state = %#v", queued)
	}
}

func TestAllianceHelpListTracksOnlyActionableRequestsFromOtherMembers(t *testing.T) {
	gameState := State.NewGameState()
	gameState.Player.ID = 501
	gameState.Session.Generation = 7
	gameState.AllianceHelpRequests.LastHelpAllGeneration = 7
	gameState.AllianceHelpRequests.LastHelpAllAt = time.Date(2026, 8, 5, 11, 59, 0, 0, time.UTC)
	responseCode := 0
	frame := Protocol.Frame{
		Direction: Protocol.DirectionInbound, Opcode: "ahl", ResponseCode: &responseCode,
		ReceivedAt: time.Date(2026, 8, 5, 12, 0, 0, 0, time.UTC),
		Payload: json.RawMessage(`{"AHL":[
			{"LID":10,"PID":999,"TID":2,"P":4,"AC":0},
			{"LID":11,"PID":999,"TID":2,"P":4,"AC":1},
			{"LID":12,"PID":999,"TID":2,"P":5,"AC":0},
			{"LID":13,"PID":999,"TID":6,"P":2,"AC":0},
			{"LID":14,"PID":999,"TID":999,"P":0,"AC":0},
			{"LID":15,"PID":501,"TID":2,"P":0,"AC":0,"OP":{"RID":321}}
		]}`),
	}
	if _, changed, err := reduceAllianceHelpRequest(
		t.Context(), frame, &gameState, allianceHelpReducerTestGameData(t),
	); err != nil || !changed {
		t.Fatalf("reduce alliance help list changed=%t err=%v", changed, err)
	}
	pending := gameState.AllianceHelpRequests.PendingOtherListIDs
	if len(pending) != 2 || pending[0] != 10 || pending[1] != 13 {
		t.Fatalf("actionable other-member requests = %#v, want [10 13]", pending)
	}
	if gameState.AllianceHelpRequests.OthersObservedGeneration != 7 ||
		!gameState.AllianceHelpRequests.OthersObservedAt.Equal(frame.ReceivedAt) {
		t.Fatalf("unexpected external request observation: %#v", gameState.AllianceHelpRequests)
	}
	if gameState.AllianceHelpRequests.LastHelpAllGeneration != 7 ||
		gameState.AllianceHelpRequests.LastHelpAllAt.IsZero() {
		t.Fatalf("full list discarded the help-all bootstrap marker: %#v", gameState.AllianceHelpRequests)
	}
}

func TestAllianceHelpDeltasAddConfirmAndDeleteOtherRequests(t *testing.T) {
	gameState := State.NewGameState()
	gameState.Player.ID = 501
	gameState.Session.Generation = 7
	gameState.AllianceHelpRequests = State.AllianceHelpRequestState{
		PendingOtherListIDs: []int64{90}, OthersObservedGeneration: 6,
	}
	responseCode := 0
	gameData := allianceHelpReducerTestGameData(t)
	addFrame := Protocol.Frame{
		Direction: Protocol.DirectionInbound, Opcode: "ahh", ResponseCode: &responseCode,
		ReceivedAt: time.Date(2026, 8, 5, 12, 0, 0, 0, time.UTC),
		Payload:    json.RawMessage(`{"LID":101,"PID":999,"TID":2,"P":0,"AC":0}`),
	}
	if _, changed, err := reduceAllianceHelpRequest(t.Context(), addFrame, &gameState, gameData); err != nil || !changed {
		t.Fatalf("add alliance help changed=%t err=%v", changed, err)
	}
	if pending := gameState.AllianceHelpRequests.PendingOtherListIDs; len(pending) != 1 || pending[0] != 101 {
		t.Fatalf("new-session pending requests = %#v, want [101]", pending)
	}

	confirmedFrame := addFrame
	confirmedFrame.ReceivedAt = addFrame.ReceivedAt.Add(time.Second)
	confirmedFrame.Payload = json.RawMessage(`{"LID":101,"PID":999,"TID":2,"P":1,"AC":1}`)
	if _, changed, err := reduceAllianceHelpRequest(t.Context(), confirmedFrame, &gameState, gameData); err != nil || !changed {
		t.Fatalf("confirm alliance help changed=%t err=%v", changed, err)
	}
	if pending := gameState.AllianceHelpRequests.PendingOtherListIDs; len(pending) != 0 {
		t.Fatalf("confirmed request remained pending: %#v", pending)
	}

	gameState.AllianceHelpRequests.PendingOtherListIDs = []int64{102, 103}
	deleteFrame := Protocol.Frame{
		Direction: Protocol.DirectionInbound, Opcode: "ahd", ResponseCode: &responseCode,
		ReceivedAt: confirmedFrame.ReceivedAt.Add(time.Second), Payload: json.RawMessage(`{"LID":102}`),
	}
	if _, changed, err := reduceAllianceHelpDelete(t.Context(), deleteFrame, &gameState, gameData); err != nil || !changed {
		t.Fatalf("delete alliance help changed=%t err=%v", changed, err)
	}
	if pending := gameState.AllianceHelpRequests.PendingOtherListIDs; len(pending) != 1 || pending[0] != 103 {
		t.Fatalf("remaining pending requests = %#v, want [103]", pending)
	}
}

func allianceHelpReducerTestGameData(t *testing.T) *GameData.Store {
	t.Helper()
	store, err := GameData.DecodeStore([]byte(`{
		"versionInfo":[],"buildings":[],"units":[],
		"alliancehelprequests":[
			{"allianceHelpRequestID":"2","maxHelpersCount":"5"},
			{"allianceHelpRequestID":"6","maxHelpersCount":"3"}
		]
	}`), GameData.SourceMetadata{ItemVersion: "test"})
	if err != nil {
		t.Fatal(err)
	}
	return store
}

func TestProductionCommandLearnsSessionKeyFromOutboundFrame(t *testing.T) {
	gameState := State.NewGameState()
	frame := Protocol.Frame{
		Direction: Protocol.DirectionOutbound, Opcode: "bup",
		ReceivedAt: time.Date(2026, 7, 11, 12, 0, 0, 0, time.UTC),
		Payload:    json.RawMessage(`{"LID":0,"WID":489,"AMT":444,"SK":91,"AID":77}`),
	}
	_, changed, err := reduceProductionCommandContext(context.Background(), frame, &gameState, nil)
	if err != nil {
		t.Fatalf("reduce production context: %v", err)
	}
	if !changed || gameState.CommandContext.ProductionSessionKey != 91 {
		t.Fatalf("production session key = %d, want 91", gameState.CommandContext.ProductionSessionKey)
	}
	if gameState.CommandContext.ProductionObservedAt == nil || !gameState.CommandContext.ProductionObservedAt.Equal(frame.ReceivedAt) {
		t.Fatalf("unexpected observed time: %v", gameState.CommandContext.ProductionObservedAt)
	}
}
