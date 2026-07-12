package Ingest

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"CitadelDesktop/Server/Protocol"
	"CitadelDesktop/Server/State"
)

func TestReportNoticeReducerClassifiesSpyAndBattleNotices(t *testing.T) {
	gameState := State.NewGameState()
	code := 0
	frame := Protocol.Frame{
		Direction: Protocol.DirectionInbound, Opcode: "sne", ResponseCode: &code,
		ReceivedAt: time.Date(2026, 7, 11, 12, 0, 0, 0, time.UTC),
		Payload: json.RawMessage(`{"MSG":[
			[101,3,"spy-key","",-1,0],
			[104,3,"shared-spy","",77,0],
			[102,6,"2+0+0#0+-209","",-1,0],
			[103,3,"expired","",-1,21600]
		]}`),
	}
	_, changed, err := reduceReportNotices(context.Background(), frame, &gameState, nil)
	if err != nil {
		t.Fatalf("reduce report notices: %v", err)
	}
	if !changed || gameState.Reports.Notices[101].TypeID != 3 || gameState.Reports.Notices[102].TypeID != 6 {
		t.Fatalf("unexpected notices: %#v", gameState.Reports.Notices)
	}
	if !gameState.Reports.Notices[101].OwnedByPlayer || gameState.Reports.Notices[104].OwnedByPlayer {
		t.Fatalf("spy ownership = own:%t shared:%t", gameState.Reports.Notices[101].OwnedByPlayer, gameState.Reports.Notices[104].OwnedByPlayer)
	}
	if gameState.Reports.Notices[103].Status != "expired" {
		t.Fatalf("expired notice status = %q", gameState.Reports.Notices[103].Status)
	}
}

func TestBattleResponsesUseSummaryAndOutboundReportContext(t *testing.T) {
	gameState := State.NewGameState()
	code := 0
	summary := Protocol.Frame{
		Direction: Protocol.DirectionInbound, Opcode: "bls", ResponseCode: &code,
		ReceivedAt: time.Now().UTC(), Payload: json.RawMessage(`{"MID":101,"LID":202}`),
	}
	if _, changed, err := reduceBattleSummaryCapture(context.Background(), summary, &gameState, nil); err != nil || !changed {
		t.Fatalf("reduce summary: changed=%t err=%v", changed, err)
	}
	command := Protocol.Frame{
		Direction: Protocol.DirectionOutbound, Opcode: "blm", Payload: json.RawMessage(`{"LID":202}`),
	}
	if _, changed, err := reduceBattleCommandContext(context.Background(), command, &gameState, nil); err != nil || !changed {
		t.Fatalf("reduce command context: changed=%t err=%v", changed, err)
	}
	waves := Protocol.Frame{
		Direction: Protocol.DirectionInbound, Opcode: "blm", ResponseCode: &code,
		ReceivedAt: time.Now().UTC(), Payload: json.RawMessage(`{"W":[]}`),
	}
	if _, changed, err := reduceBattleWaveCapture(context.Background(), waves, &gameState, nil); err != nil || !changed {
		t.Fatalf("reduce waves: changed=%t err=%v", changed, err)
	}
	if len(gameState.Reports.BattleCaptures[101].Waves) == 0 {
		t.Fatal("expected waves to attach to the summary-derived report")
	}
}
