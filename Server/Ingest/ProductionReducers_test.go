package Ingest

import (
	"context"
	"encoding/json"
	"fmt"
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

func TestProductionSnapshotPreservesCurrentRequestedHelpWhenWireOmitsRAH(t *testing.T) {
	gameState := State.NewGameState()
	gameState.Session.Generation = 7
	gameState.Session.ChangedAt = time.Date(2026, 7, 11, 11, 0, 0, 0, time.UTC)
	gameState.AllianceHelpRequests.OwnObservedGeneration = 7
	gameState.AllianceHelpRequests.RecruitmentCastleIDs = []State.CastleID{77}
	castle := newCastleState(77)
	castle.Focused = true
	castle.Production[0] = State.ProductionQueue{
		LineID: 0, ObservedAt: gameState.Session.ChangedAt.Add(time.Minute),
		Active: &State.QueueItem{ProductionID: 11, AllianceHelpRequested: true},
	}
	gameState.Castles[castle.ID] = castle
	responseCode := 0
	frame := Protocol.Frame{
		Direction: Protocol.DirectionInbound, Opcode: "spl", ResponseCode: &responseCode,
		ReceivedAt: time.Date(2026, 7, 11, 12, 0, 0, 0, time.UTC),
		Payload:    json.RawMessage(`{"PS":{"WID":489,"TUA":6,"PID":11},"LID":0}`),
	}
	if _, changed, err := reduceProductionSnapshot(context.Background(), frame, &gameState, nil); err != nil || !changed {
		t.Fatalf("reduce production snapshot changed=%t err=%v", changed, err)
	}
	if !gameState.Castles[77].Production[0].Active.AllianceHelpRequested {
		t.Fatal("requested alliance help was not preserved")
	}
}

func TestProductionSnapshotDoesNotCarryRequestedHelpAcrossSessionGeneration(t *testing.T) {
	gameState := State.NewGameState()
	gameState.Session.Generation = 8
	gameState.AllianceHelpRequests.OwnObservedGeneration = 7
	gameState.AllianceHelpRequests.RecruitmentCastleIDs = []State.CastleID{77}
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
		ReceivedAt: time.Date(2026, 8, 26, 12, 0, 0, 0, time.UTC),
		Payload:    json.RawMessage(`{"PS":{"WID":489,"TUA":6,"PID":11},"LID":0}`),
	}
	if _, changed, err := reduceProductionSnapshot(t.Context(), frame, &gameState, nil); err != nil || !changed {
		t.Fatalf("reduce production snapshot changed=%t err=%v", changed, err)
	}
	if gameState.Castles[77].Production[0].Active.AllianceHelpRequested {
		t.Fatal("prior-session item flag survived a current production snapshot")
	}
	if State.HasOutstandingRecruitmentAllianceHelpRequest(gameState, 77) {
		t.Fatal("prior-session recruitment castle guard remained active")
	}
}

func TestProductionSnapshotExplicitFalseClearsCurrentRequestedRecruitmentHelp(t *testing.T) {
	gameState := State.NewGameState()
	gameState.Session.Generation = 7
	gameState.Session.ChangedAt = time.Date(2026, 8, 28, 11, 0, 0, 0, time.UTC)
	gameState.AllianceHelpRequests = State.AllianceHelpRequestState{
		RecruitmentCastleIDs: []State.CastleID{77}, OwnObservedGeneration: 7,
	}
	castle := newCastleState(77)
	castle.Focused = true
	castle.Production[0] = State.ProductionQueue{
		LineID: 0, ObservedAt: gameState.Session.ChangedAt.Add(time.Minute),
		Active: &State.QueueItem{ProductionID: 11, AllianceHelpRequested: true},
	}
	gameState.Castles[castle.ID] = castle
	responseCode := 0
	frame := Protocol.Frame{
		Direction: Protocol.DirectionInbound, Opcode: "spl", ResponseCode: &responseCode,
		ReceivedAt: gameState.Session.ChangedAt.Add(2 * time.Minute),
		Payload:    json.RawMessage(`{"PS":{"WID":489,"TUA":6,"PID":11,"RAH":false},"LID":0}`),
	}
	if _, changed, err := reduceProductionSnapshot(t.Context(), frame, &gameState, nil); err != nil || !changed {
		t.Fatalf("reduce production snapshot changed=%t err=%v", changed, err)
	}
	if gameState.Castles[77].Production[0].Active.AllianceHelpRequested ||
		State.HasOutstandingRecruitmentAllianceHelpRequest(gameState, 77) {
		t.Fatalf("explicit RAH=false did not clear recruitment help: queue=%#v state=%#v",
			gameState.Castles[77].Production[0], gameState.AllianceHelpRequests)
	}
}

func TestProductionSnapshotPartialTrueReconcilesCastleGuard(t *testing.T) {
	gameState := State.NewGameState()
	gameState.Player.ID = 501
	gameState.Session.Generation = 7
	gameState.Session.ChangedAt = time.Date(2026, 8, 28, 11, 0, 0, 0, time.UTC)
	gameState.AllianceHelpRequests = State.AllianceHelpRequestState{
		OwnObservedGeneration: 7,
		ObservedAt:            gameState.Session.ChangedAt.Add(time.Minute),
	}
	castle := newCastleState(77)
	castle.Focused = true
	gameState.Castles[castle.ID] = castle
	responseCode := 0
	frame := Protocol.Frame{
		Direction: Protocol.DirectionInbound, Opcode: "spl", ResponseCode: &responseCode,
		ReceivedAt: gameState.Session.ChangedAt.Add(2 * time.Minute),
		Payload: json.RawMessage(`{
			"PS":{"WID":489,"TUA":6,"PID":11,"RAH":true},
			"QS":[{"P":{"WID":489,"TUA":6,"PID":12}}],
			"LID":0
		}`),
	}
	if _, changed, err := reduceProductionSnapshot(t.Context(), frame, &gameState, nil); err != nil || !changed {
		t.Fatalf("reduce partial RAH snapshot changed=%t err=%v", changed, err)
	}
	queue := gameState.Castles[77].Production[0]
	if queue.Active == nil || !queue.Active.AllianceHelpRequested ||
		len(queue.Queued) != 1 || queue.Queued[0].AllianceHelpRequested ||
		!State.HasOutstandingRecruitmentAllianceHelpRequest(gameState, 77) {
		t.Fatalf("partial RAH=true did not preserve per-item state and guard the castle: queue=%#v state=%#v",
			queue, gameState.AllianceHelpRequests)
	}
}

func TestProductionSnapshotDoesNotCarryPriorSessionRequestedHelp(t *testing.T) {
	gameState := State.NewGameState()
	gameState.Session.Generation = 8
	gameState.Session.ChangedAt = time.Date(2026, 8, 28, 12, 0, 0, 0, time.UTC)
	gameState.AllianceHelpRequests = State.AllianceHelpRequestState{
		RecruitmentCastleIDs: []State.CastleID{77}, OwnObservedGeneration: 7,
	}
	castle := newCastleState(77)
	castle.Focused = true
	castle.Production[0] = State.ProductionQueue{
		LineID: 0, ObservedAt: gameState.Session.ChangedAt.Add(-time.Second),
		Active: &State.QueueItem{ProductionID: 11, AllianceHelpRequested: true},
	}
	gameState.Castles[castle.ID] = castle
	responseCode := 0
	frame := Protocol.Frame{
		Direction: Protocol.DirectionInbound, Opcode: "spl", ResponseCode: &responseCode,
		ReceivedAt: gameState.Session.ChangedAt.Add(time.Minute),
		Payload:    json.RawMessage(`{"PS":{"WID":489,"TUA":6,"PID":11},"LID":0}`),
	}
	if _, changed, err := reduceProductionSnapshot(t.Context(), frame, &gameState, nil); err != nil || !changed {
		t.Fatalf("reduce production snapshot changed=%t err=%v", changed, err)
	}
	if gameState.Castles[77].Production[0].Active.AllianceHelpRequested ||
		State.HasOutstandingRecruitmentAllianceHelpRequest(gameState, 77) {
		t.Fatalf("prior-session recruitment help survived a fresh snapshot: queue=%#v state=%#v",
			gameState.Castles[77].Production[0], gameState.AllianceHelpRequests)
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
	if item.ProductionID != 1374447446 || !item.AllianceHelpAvailable || item.AllianceHelpRequested {
		t.Fatalf("unexpected compact hospital alliance-help state: %#v", item)
	}
}

func TestHospitalCompactZeroReductionPreservesConfirmedAHR(t *testing.T) {
	now := time.Date(2026, 9, 1, 13, 2, 15, 0, time.UTC)
	gameState := State.NewGameState()
	gameState.Player.ID = 501
	gameState.Session.Generation = 7
	gameState.Session.ChangedAt = now.Add(-time.Minute)
	gameState.AllianceHelpRequests = State.AllianceHelpRequestState{
		HospitalProductionIDs: []int64{}, ObservedAt: now.Add(-time.Second), OwnObservedGeneration: 7,
	}
	castle := newCastleState(77)
	castle.Focused = true
	castle.Production[2] = State.ProductionQueue{
		LineID: 2, ObservedAt: now.Add(-time.Second),
		Queued: []State.QueueItem{{ProductionID: 1374447446, AllianceHelpAvailable: true}},
	}
	gameState.Castles[castle.ID] = castle
	responseCode := 0
	confirmed := Protocol.Frame{
		Direction: Protocol.DirectionInbound, Opcode: "ahh", ResponseCode: &responseCode, ReceivedAt: now,
		Payload: json.RawMessage(`{"LID":5526,"P":0,"PID":501,"TID":2,"AC":0,"OP":{"RID":1374447446,"AID":77,"RLID":2}}`),
	}
	if _, changed, err := reduceAllianceHelpRequest(t.Context(), confirmed, &gameState, nil); err != nil || !changed {
		t.Fatalf("reduce confirmed hospital AHR changed=%t err=%v", changed, err)
	}
	compact := Protocol.Frame{
		Direction: Protocol.DirectionInbound, Opcode: "spl", ResponseCode: &responseCode,
		ReceivedAt: now.Add(time.Second),
		Payload:    json.RawMessage(`{"LID":2,"PIDL":[[238,5,323,1378,0,1374447446,0,-1],[-1,0,0,0,0,0,0,-1]]}`),
	}
	if _, changed, err := reduceProductionSnapshot(t.Context(), compact, &gameState, nil); err != nil || !changed {
		t.Fatalf("reduce compact hospital snapshot changed=%t err=%v", changed, err)
	}
	queue := gameState.Castles[77].Production[2]
	if len(queue.Queued) != 1 || !queue.Queued[0].AllianceHelpRequested ||
		!State.HasOutstandingHospitalAllianceHelpRequest(gameState, 1374447446) {
		t.Fatalf("zero reduction erased confirmed hospital AHR: queue=%#v help=%#v",
			queue, gameState.AllianceHelpRequests)
	}
}

func TestRecruitmentAllianceHelpEventMarksWholeCastleQueue(t *testing.T) {
	gameState := State.NewGameState()
	gameState.Player.ID = 501
	gameState.Session.Generation = 3
	observedAt := time.Now().UTC()
	gameState.Session.ChangedAt = observedAt.Add(-time.Minute)
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
		ReceivedAt: observedAt,
		Payload:    json.RawMessage(`{"PID":501,"TID":6,"OP":{"AID":77,"SID":0,"RLID":0}}`),
	}
	if _, changed, err := reduceAllianceHelpRequest(
		context.Background(), frame, &gameState, allianceHelpReducerTestGameData(t),
	); err != nil || !changed {
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
	if gameState.AllianceHelpRequests.OwnObservedGeneration != 3 {
		t.Fatalf("own help generation = %d, want 3", gameState.AllianceHelpRequests.OwnObservedGeneration)
	}
	if _, changed, err := reduceAllianceHelpRequest(context.Background(), frame, &gameState, nil); err != nil || changed {
		t.Fatalf("duplicate confirmed help changed=%t err=%v", changed, err)
	}
}

func TestCompletedRecruitmentHelpAHLRemovalRetainsBoundedGrace(t *testing.T) {
	gameState := State.NewGameState()
	gameState.Player.ID = 501
	gameState.Session.Generation = 7
	responseCode := 0
	completedAt := time.Now().UTC()
	gameState.Session.ChangedAt = completedAt.Add(-time.Minute)
	gameData := allianceHelpReducerTestGameData(t)
	completed := Protocol.Frame{
		Direction: Protocol.DirectionInbound, Opcode: "ahl", ResponseCode: &responseCode,
		ReceivedAt: completedAt,
		Payload: json.RawMessage(`{"AHL":[
			{"LID":91,"PID":501,"TID":6,"P":3,"AC":0,"OP":{"AID":77,"RLID":0}}
		]}`),
	}
	if _, changed, err := reduceAllianceHelpRequest(t.Context(), completed, &gameState, gameData); err != nil || !changed {
		t.Fatalf("retain completed recruitment help changed=%t err=%v", changed, err)
	}
	if !State.HasOutstandingRecruitmentAllianceHelpRequest(gameState, 77) {
		t.Fatal("server-retained P=3 recruitment request did not preserve lifecycle coverage")
	}

	removed := completed
	removed.ReceivedAt = completedAt.Add(time.Second)
	removed.Payload = json.RawMessage(`{"AHL":[]}`)
	if _, changed, err := reduceAllianceHelpRequest(t.Context(), removed, &gameState, gameData); err != nil || !changed {
		t.Fatalf("remove completed recruitment help changed=%t err=%v", changed, err)
	}
	if !State.RecruitmentAllianceHelpCovers(gameState, 77, removed.ReceivedAt, 10*time.Second) {
		t.Fatal("P=3/AHD completion grace was not retained")
	}
}

func TestRecruitmentHelpAHHAndAHDTrackExactOwnLifecycle(t *testing.T) {
	gameState := State.NewGameState()
	gameState.Player.ID = 501
	gameState.Session.Generation = 7
	gameData := allianceHelpReducerTestGameData(t)
	responseCode := 0
	startedAt := time.Date(2026, 8, 29, 12, 0, 0, 0, time.UTC)
	gameState.Session.ChangedAt = startedAt.Add(-time.Minute)
	for progress := 0; progress <= 3; progress++ {
		frame := Protocol.Frame{
			Direction: Protocol.DirectionInbound, Opcode: "ahh", ResponseCode: &responseCode,
			ReceivedAt: startedAt.Add(time.Duration(progress) * time.Second),
			Payload: json.RawMessage(fmt.Sprintf(
				`{"LID":91,"PID":501,"TID":6,"P":%d,"AC":0,"OP":{"AID":77,"RLID":0}}`,
				progress,
			)),
		}
		if _, changed, err := reduceAllianceHelpRequest(t.Context(), frame, &gameState, gameData); err != nil || !changed {
			t.Fatalf("reduce own recruitment P=%d changed=%t err=%v", progress, changed, err)
		}
		requests := gameState.AllianceHelpRequests.OwnRecruitmentRequests
		if len(requests) != 1 || requests[0].ListID != 91 || requests[0].CastleID != 77 ||
			requests[0].Progress != progress || requests[0].MaximumHelpers != 3 {
			t.Fatalf("own recruitment P=%d lifecycle=%#v", progress, requests)
		}
		if progress < 3 && !State.RecruitmentAllianceHelpCovers(gameState, 77, frame.ReceivedAt, time.Minute) {
			t.Fatalf("pending own recruitment P=%d did not suppress duplicate AHR", progress)
		}
	}
	completed := gameState.AllianceHelpRequests.OwnRecruitmentRequests[0]
	if !completed.CompletedAt.Equal(startedAt.Add(3*time.Second)) || !completed.RemovedAt.IsZero() {
		t.Fatalf("completed own recruitment lifecycle=%#v", completed)
	}
	deleteFrame := Protocol.Frame{
		Direction: Protocol.DirectionInbound, Opcode: "ahd", ResponseCode: &responseCode,
		ReceivedAt: completed.CompletedAt.Add(time.Millisecond), Payload: json.RawMessage(`{"LID":91}`),
	}
	if _, changed, err := reduceAllianceHelpDelete(t.Context(), deleteFrame, &gameState, gameData); err != nil || !changed {
		t.Fatalf("reduce own recruitment AHD changed=%t err=%v", changed, err)
	}
	removed := gameState.AllianceHelpRequests.OwnRecruitmentRequests
	if len(removed) != 1 || !removed[0].RemovedAt.Equal(deleteFrame.ReceivedAt) {
		t.Fatalf("post-AHD own recruitment lifecycle=%#v", removed)
	}
	if !State.RecruitmentAllianceHelpCovers(gameState, 77, deleteFrame.ReceivedAt, 10*time.Second) {
		t.Fatal("P=3/AHD request did not retain bounded grace")
	}
}

func TestAllianceHelpListKeepsNewRecruitQueueGuardedByCastle(t *testing.T) {
	gameState := State.NewGameState()
	gameState.Player.ID = 501
	gameState.Session.Generation = 4
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
			{"LID":101,"PID":501,"TID":6,"P":0,"AC":0,"OP":{"AID":77,"RLID":0}},
			{"LID":102,"PID":501,"TID":6,"P":0,"AC":0,"OP":{"AID":77,"RLID":0}}
		]}`),
	}
	gameState.Session.ChangedAt = listFrame.ReceivedAt.Add(-time.Minute)
	if _, changed, err := reduceAllianceHelpRequest(
		t.Context(), listFrame, &gameState, allianceHelpReducerTestGameData(t),
	); err != nil || !changed {
		t.Fatalf("reduce alliance help list changed=%t err=%v", changed, err)
	}
	if got := gameState.AllianceHelpRequests.RecruitmentCastleIDs; len(got) != 1 || got[0] != 77 {
		t.Fatalf("authoritative recruitment-help castles = %#v", got)
	}
	if gameState.AllianceHelpRequests.OwnObservedGeneration != 4 {
		t.Fatalf("own help generation = %d, want 4", gameState.AllianceHelpRequests.OwnObservedGeneration)
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

func TestCompleteHospitalSnapshotPrunesOnlyVanishedJobsFromRefreshedCastle(t *testing.T) {
	gameState := State.NewGameState()
	gameState.Session.Generation = 7
	gameState.AllianceHelpRequests = State.AllianceHelpRequestState{
		HospitalProductionIDs: []int64{201, 301},
		ObservedAt:            time.Date(2026, 8, 26, 11, 55, 0, 0, time.UTC),
		OwnObservedGeneration: 7,
	}
	refreshed := newCastleState(77)
	refreshed.Focused = true
	refreshed.Production[2] = State.ProductionQueue{
		LineID: 2,
		Queued: []State.QueueItem{{ProductionID: 201, AllianceHelpRequested: true}},
	}
	other := newCastleState(88)
	other.Production[2] = State.ProductionQueue{
		LineID: 2,
		Queued: []State.QueueItem{{ProductionID: 301, AllianceHelpRequested: true}},
	}
	gameState.Castles[refreshed.ID] = refreshed
	gameState.Castles[other.ID] = other

	responseCode := 0
	frame := Protocol.Frame{
		Direction: Protocol.DirectionInbound, Opcode: "spl", ResponseCode: &responseCode,
		ReceivedAt: time.Date(2026, 8, 26, 12, 0, 0, 0, time.UTC),
		Payload:    json.RawMessage(`{"LID":2,"PIDL":[[-1,0,0,0,0,0,0,-1]]}`),
	}
	if _, changed, err := reduceProductionSnapshot(t.Context(), frame, &gameState, nil); err != nil || !changed {
		t.Fatalf("reduce hospital snapshot changed=%t err=%v", changed, err)
	}
	if State.HasOutstandingHospitalAllianceHelpRequest(gameState, 201) {
		t.Fatal("vanished hospital job still consumes an alliance-help slot")
	}
	if !State.HasOutstandingHospitalAllianceHelpRequest(gameState, 301) {
		t.Fatal("another castle's hospital request was pruned by the local snapshot")
	}
	if got := State.OutstandingHospitalAllianceHelpRequests(gameState); got != 1 {
		t.Fatalf("outstanding hospital alliance-help requests = %d, want 1", got)
	}
}

func TestOutboundAllianceHelpDoesNotMarkBeforeSuccessfulResponse(t *testing.T) {
	gameState := State.NewGameState()
	gameState.Player.ID = 501
	gameState.Session.Generation = 5
	castle := newCastleState(77)
	castle.Focused = true
	castle.Production[2] = State.ProductionQueue{
		LineID: 2, Queued: []State.QueueItem{{ProductionID: 201}, {ProductionID: 202}},
	}
	gameState.Castles[castle.ID] = castle
	store := State.NewStore(gameState)
	registry := NewRegistry()
	if err := RegisterCoreReducers(registry); err != nil {
		t.Fatal(err)
	}
	if registry.HasOutbound("ahr") {
		t.Fatal("outbound AHR still has an optimistic state reducer")
	}
	pipeline := NewPipeline(store, nil, registry)
	if _, err := pipeline.HandleFrame(t.Context(), Protocol.Frame{
		Direction: Protocol.DirectionOutbound, Opcode: "ahr",
		Payload: json.RawMessage(`{"ID":201,"T":2}`),
	}); err != nil {
		t.Fatal(err)
	}
	nonzero := 327
	failed := Protocol.Frame{
		Direction: Protocol.DirectionInbound, Opcode: "ahh", ResponseCode: &nonzero,
		Payload: json.RawMessage(`{"PID":501,"TID":2,"OP":{"AID":77,"RID":201,"RLID":2}}`),
	}
	if _, changed, err := reduceAllianceHelpRequest(t.Context(), failed, &gameState, nil); err != nil || changed {
		t.Fatalf("failed alliance help changed=%t err=%v", changed, err)
	}
	queued := store.Snapshot().Castles[77].Production[2].Queued
	if queued[0].AllianceHelpRequested || queued[1].AllianceHelpRequested {
		t.Fatalf("outbound/failed hospital help poisoned state = %#v", queued)
	}
}

func TestOutboundOrRejectedRecruitmentAllianceHelpDoesNotPoisonState(t *testing.T) {
	gameState := State.NewGameState()
	gameState.Player.ID = 501
	gameState.Session.Generation = 5
	castle := newCastleState(77)
	castle.Focused = true
	castle.Production[0] = State.ProductionQueue{
		LineID: 0, Active: &State.QueueItem{ProductionID: 201},
		Queued: []State.QueueItem{{ProductionID: 202}},
	}
	gameState.Castles[castle.ID] = castle
	registry := NewRegistry()
	if err := RegisterCoreReducers(registry); err != nil {
		t.Fatal(err)
	}
	if registry.HasOutbound("ahr") {
		t.Fatal("outbound AHR still has an optimistic state reducer")
	}
	rejected := 175
	frame := Protocol.Frame{
		Direction: Protocol.DirectionInbound, Opcode: "ahh", ResponseCode: &rejected,
		Payload: json.RawMessage(`{"PID":501,"TID":6,"OP":{"AID":77,"RLID":0}}`),
	}
	if _, changed, err := reduceAllianceHelpRequest(t.Context(), frame, &gameState, nil); err != nil || changed {
		t.Fatalf("rejected recruitment help changed=%t err=%v", changed, err)
	}
	queue := gameState.Castles[77].Production[0]
	if queue.Active.AllianceHelpRequested || queue.Queued[0].AllianceHelpRequested ||
		State.HasOutstandingRecruitmentAllianceHelpRequest(gameState, 77) {
		t.Fatalf("outbound/rejected recruitment help poisoned state: queue=%#v help=%#v", queue, gameState.AllianceHelpRequests)
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

func TestProductionLearnedStackHighWaterAndSubscriptionScopeReset(t *testing.T) {
	gameState := State.NewGameState()
	castle := newCastleState(77)
	castle.Focused = true
	gameState.Castles[castle.ID] = castle
	gameState.Subscriptions = map[int]State.SubscriptionState{
		7: {TypeID: 7, RemainingSec: 86400},
	}
	responseCode := 0
	snapshot := func(amount int64) Protocol.Frame {
		payload, _ := json.Marshal(map[string]any{
			"PS":  map[string]any{"WID": 489, "TUA": amount, "RCT": 75, "PID": 11},
			"QS":  []any{map[string]any{"P": map[string]any{"WID": 489, "TUA": amount, "PID": 12}}},
			"LID": 0,
		})
		return Protocol.Frame{
			Direction: Protocol.DirectionInbound, Opcode: "spl", ResponseCode: &responseCode,
			ReceivedAt: time.Date(2026, 8, 17, 12, 0, 0, 0, time.UTC), Payload: payload,
		}
	}

	// A subscription-sized stack teaches the high-water mark for unit 489.
	if _, _, err := reduceProductionSnapshot(context.Background(), snapshot(260), &gameState, nil); err != nil {
		t.Fatalf("reduce first snapshot: %v", err)
	}
	queue := gameState.Castles[77].Production[0]
	if queue.LearnedStacks[489] != 260 || queue.LearnedStackScope != "sub:7" {
		t.Fatalf("learned = %v scope %q, want 489→260 under sub:7", queue.LearnedStacks, queue.LearnedStackScope)
	}

	// Smaller stacks must NOT ratchet the learned size down while the
	// subscription set is unchanged.
	if _, _, err := reduceProductionSnapshot(context.Background(), snapshot(220), &gameState, nil); err != nil {
		t.Fatalf("reduce second snapshot: %v", err)
	}
	queue = gameState.Castles[77].Production[0]
	if queue.LearnedStacks[489] != 260 {
		t.Fatalf("learned after smaller stacks = %v, want 489 held at 260", queue.LearnedStacks)
	}
	if len(queue.Queued) != 1 || queue.Queued[0].Amount != 220 {
		t.Fatalf("live queue should reflect the observed 220 stack: %#v", queue.Queued)
	}

	// A lapsed subscription changes the scope: stale learned values are
	// discarded and the line re-learns from the live (smaller) stacks.
	gameState.Subscriptions = nil
	if _, _, err := reduceProductionSnapshot(context.Background(), snapshot(220), &gameState, nil); err != nil {
		t.Fatalf("reduce post-lapse snapshot: %v", err)
	}
	queue = gameState.Castles[77].Production[0]
	if queue.LearnedStacks[489] != 220 || queue.LearnedStackScope != "" {
		t.Fatalf("post-lapse learned = %v scope %q, want re-learned 489→220 under empty scope", queue.LearnedStacks, queue.LearnedStackScope)
	}
}
