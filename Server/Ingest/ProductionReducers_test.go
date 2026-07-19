package Ingest

import (
	"context"
	"encoding/json"
	"testing"
	"time"

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
		Payload: json.RawMessage(`{"AHL":[
			{"PID":999,"TID":2,"OP":{"AID":77,"RID":22}},
			{"PID":501,"TID":2,"OP":{"AID":77,"RID":21}}
		]}`),
	}
	if _, changed, err := reduceAllianceHelpRequest(context.Background(), frame, &gameState, nil); err != nil || !changed {
		t.Fatalf("reduce alliance help list changed=%t err=%v", changed, err)
	}
	queued := gameState.Castles[77].Production[2].Queued
	if !queued[0].AllianceHelpRequested || queued[1].AllianceHelpRequested {
		t.Fatalf("unexpected hospital alliance-help state: %#v", queued)
	}
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
