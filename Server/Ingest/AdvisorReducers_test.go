package Ingest

import (
	"encoding/json"
	"testing"
	"time"

	"CitadelDesktop/Server/Protocol"
	"CitadelDesktop/Server/State"
)

func TestAdvisorReducersTrackActivationRunOverviewAndCancellation(t *testing.T) {
	gameData := scalableEventTestGameData(t)
	gameState := State.NewGameState()
	observedAt := time.Date(2026, 7, 19, 16, 11, 27, 0, time.UTC)
	code := 0

	_, changed, err := reduceScalableEventSnapshot(t.Context(), Protocol.Frame{
		Opcode: "sei", Direction: Protocol.DirectionInbound, ResponseCode: &code, ReceivedAt: observedAt,
		Payload: json.RawMessage(`{"E":[{"EID":72,"RS":259200,"EDID":308,"EASE":1,"ACI":77,"AAA":0,"AAF":0}]}`),
	}, &gameState, gameData)
	if err != nil || !changed {
		t.Fatalf("advisor event snapshot: changed=%t err=%v", changed, err)
	}
	score := gameState.EventScores.ByEvent[72]
	if score.AdvisorActive || score.AdvisorCurrencyID != 77 || score.AdvisorFree {
		t.Fatalf("unexpected advisor event state: %#v", score)
	}

	_, changed, err = reduceAdvisorActivation(t.Context(), Protocol.Frame{
		Opcode: "aa", Direction: Protocol.DirectionInbound, ResponseCode: &code, ReceivedAt: observedAt.Add(time.Second),
		Payload: json.RawMessage(`{"AAT":1}`),
	}, &gameState, gameData)
	if err != nil || !changed || !gameState.EventScores.ByEvent[72].AdvisorActive {
		t.Fatalf("advisor activation: changed=%t err=%v score=%#v", changed, err, gameState.EventScores.ByEvent[72])
	}

	_, changed, err = reduceAdvisorMovement(t.Context(), Protocol.Frame{
		Opcode: "cra", Direction: Protocol.DirectionInbound, ResponseCode: &code, ReceivedAt: observedAt.Add(2 * time.Second),
		Payload: json.RawMessage(`{"AAM":{"M":{"MID":27264414,"D":0,"KID":0,"SA":[1,100,100,1],"TA":[27,101,100,-1]},"UM":{"AAT":1,"AAN":1,"AAC":1241,"AAL":0,"L":{"ID":3}}}}`),
	}, &gameState, gameData)
	if err != nil || !changed {
		t.Fatalf("advisor launch: changed=%t err=%v", changed, err)
	}
	run := gameState.Advisor.Run
	if run == nil || run.EventID != 72 || run.SourceCastleID != 1 || run.TargetTypeID != 27 ||
		run.CurrentAttack != 1 || run.RequestedAttacks != 1241 || run.Status != "running" {
		t.Fatalf("unexpected advisor launch state: %#v", run)
	}

	_, changed, err = reduceAdvisorMovement(t.Context(), Protocol.Frame{
		Opcode: "cra", Direction: Protocol.DirectionInbound, ResponseCode: &code, ReceivedAt: observedAt.Add(3 * time.Minute),
		Payload: json.RawMessage(`{"AAM":{"M":{"MID":27264420,"D":0,"KID":0,"SA":[1,100,100,1],"TA":[27,101,100,-1]},"UM":{"AAT":1,"AAN":7,"AAC":1241,"AAL":0,"L":{"ID":3}}}}`),
	}, &gameState, gameData)
	if err != nil || !changed || gameState.Advisor.Run.CurrentAttack != 7 || gameState.Advisor.Run.Status != "running" {
		t.Fatalf("advisor progression: changed=%t err=%v run=%#v", changed, err, gameState.Advisor.Run)
	}

	_, changed, err = reduceAdvisorOverview(t.Context(), Protocol.Frame{
		Opcode: "aao", Direction: Protocol.DirectionInbound, ResponseCode: &code, ReceivedAt: observedAt.Add(4 * time.Minute),
		Payload: json.RawMessage(`{"AAT":1,"C":1,"G":[["C1",189613],["KT",2588]],"GP":0,"L":[["C1",2863],["C2",0],["PTT",7]],"LU":2291,"LT":252,"W":6,"D":0,"P":1235}`),
	}, &gameState, gameData)
	if err != nil || !changed {
		t.Fatalf("advisor overview: changed=%t err=%v", changed, err)
	}
	summary := gameState.Advisor.Summary
	if summary.Gains["C1"] != 189613 || summary.Costs["PTT"] != 7 || summary.UnitsLost != 2291 ||
		summary.ToolsLost != 252 || summary.Wins != 6 || summary.PendingAttacks != 1235 {
		t.Fatalf("unexpected advisor overview: %#v", summary)
	}

	_, changed, err = reduceAdvisorMovement(t.Context(), Protocol.Frame{
		Opcode: "mcm", Direction: Protocol.DirectionInbound, ResponseCode: &code, ReceivedAt: observedAt.Add(5 * time.Minute),
		Payload: json.RawMessage(`{"A":{"M":{"MID":27264414,"D":1,"KID":0,"SA":[27,1162,1166,-1,6,-55,0,-601,997,160,160,75],"TA":[1,1164,1167,16326717,17334928,8,7,7,6,7,"GhostTown",0,0,-1,-1,-1,0,0,[],0]},"UM":{"AAT":1,"AAN":7,"AAC":1241,"AAL":1,"L":{"ID":0}}}}`),
	}, &gameState, gameData)
	if err != nil || !changed {
		t.Fatalf("advisor cancellation: changed=%t err=%v", changed, err)
	}
	run = gameState.Advisor.Run
	if run == nil || run.Status != "cancelled" || run.LaunchState != 1 || run.CurrentAttack != 7 || run.RequestedAttacks != 1241 ||
		run.SourceCastleID != 16326717 || run.TargetTypeID != 27 || run.TargetX != 1162 || run.TargetY != 1166 {
		t.Fatalf("mcm did not terminate the advisor chain: %#v", run)
	}
}

func TestAdvisorReducerDoesNotReviveCancelledRunFromStaleMovementSnapshot(t *testing.T) {
	gameState := State.NewGameState()
	observedAt := time.Date(2026, 7, 19, 16, 20, 0, 0, time.UTC)
	gameState.EventScores.ActiveEventID = 72
	gameState.EventScores.ByEvent[72] = State.ScalableEventScore{EventID: 72, RemainingSec: 10_000, ObservedAt: observedAt}
	gameState.Advisor.Run = &State.AdvisorRunState{
		EventID: 72, SourceCastleID: 1, RequestedAttacks: 1241, CurrentAttack: 7,
		LaunchState: 1, Status: "cancelled", StartedAt: observedAt.Add(-time.Minute), LastAttackAt: observedAt,
	}
	code := 0

	_, _, err := reduceAdvisorMovement(t.Context(), Protocol.Frame{
		Opcode: "gam", Direction: Protocol.DirectionInbound, ResponseCode: &code, ReceivedAt: observedAt.Add(time.Second),
		Payload: json.RawMessage(`{"M":[{"M":{"MID":27264414,"D":0,"KID":0,"SA":[1,100,100,1],"TA":[27,101,100,-1]},"UM":{"AAT":1,"AAN":7,"AAC":1241,"AAL":0,"L":{"ID":3}}}]}`),
	}, &gameState, nil)
	if err != nil {
		t.Fatal(err)
	}
	if gameState.Advisor.Run == nil || gameState.Advisor.Run.Status != "cancelled" || gameState.Advisor.Run.LaunchState != 1 {
		t.Fatalf("stale gam snapshot revived a cancelled advisor run: %#v", gameState.Advisor.Run)
	}
}
