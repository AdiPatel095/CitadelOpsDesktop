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

func TestDeletedReportMessagesBecomeUnavailableBeforeFetch(t *testing.T) {
	gameState := State.NewGameState()
	gameState.Reports.Notices[101] = State.ReportNotice{MessageID: 101, TypeID: 6, Status: "pending"}
	gameState.Reports.Notices[102] = State.ReportNotice{MessageID: 102, TypeID: 6, Status: "archived"}
	gameState.Reports.SpyCaptures[103] = State.SpyReportCapture{MessageID: 103}
	gameState.Reports.BattleCaptures[101] = State.BattleReportCapture{MessageID: 101, ReportID: 202}
	gameState.Reports.ActiveBattleReport = 202
	code := 0
	frame := Protocol.Frame{
		Direction: Protocol.DirectionInbound, Opcode: "dms", ResponseCode: &code,
		ReceivedAt: time.Date(2026, 7, 21, 19, 24, 53, 0, time.UTC),
		Payload:    json.RawMessage(`{"MID":[101,102,103,104]}`),
	}
	domains, changed, err := reduceDeletedReportMessages(t.Context(), frame, &gameState, nil)
	if err != nil || !changed || len(domains) != 1 || domains[0] != "reports" {
		t.Fatalf("deleted reports reduction: changed=%t domains=%v err=%v", changed, domains, err)
	}
	if gameState.Reports.Notices[101].Status != "unavailable" ||
		gameState.Reports.Notices[102].Status != "archived" ||
		gameState.Reports.Notices[103].Status != "unavailable" ||
		gameState.Reports.Notices[104].Status != "unavailable" {
		t.Fatalf("deleted report statuses = %#v", gameState.Reports.Notices)
	}
	if len(gameState.Reports.SpyCaptures) != 0 || len(gameState.Reports.BattleCaptures) != 0 || gameState.Reports.ActiveBattleReport != 0 {
		t.Fatalf("deleted report captures remain: %#v", gameState.Reports)
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

func TestBattleDetailsReadyReportConfirmedStormIslandSurvivorsForReturn(t *testing.T) {
	launchedAt := time.Date(2026, 7, 15, 18, 0, 0, 0, time.UTC)
	gameState := State.NewGameState()
	gameState.Player.ID = 1
	key := State.StormIslandReturnKey(4, 101, 102)
	gameState.Storm.IslandReturns[key] = State.StormIslandReturnState{
		KingdomID: 4, SourceCastleID: 40, TargetX: 101, TargetY: 102, IslandObjectID: 777,
		Status: State.StormIslandReturnAwaitingReport, LeaveBehind: 1,
		Survivors: map[State.UnitID]int64{}, LaunchedAt: launchedAt,
	}
	gameState.Reports.Notices[101] = State.ReportNotice{MessageID: 101, TypeID: 6, ObservedAt: launchedAt.Add(time.Minute)}
	code := 0
	summary := Protocol.Frame{
		Direction: Protocol.DirectionInbound, Opcode: "bls", ResponseCode: &code, ReceivedAt: launchedAt.Add(time.Minute),
		Payload: json.RawMessage(`{
			"MID":101,"LID":202,"PBI":[[1,0,12,-3],[-403,1,10,-10]],
			"AI":{"AT":24,"K":4,"X":101,"Y":102}
		}`),
	}
	if _, changed, err := reduceBattleSummaryCapture(context.Background(), summary, &gameState, nil); err != nil || !changed {
		t.Fatalf("reduce island summary: changed=%t err=%v", changed, err)
	}
	details := Protocol.Frame{
		Direction: Protocol.DirectionInbound, Opcode: "bld", ResponseCode: &code, ReceivedAt: launchedAt.Add(time.Minute + time.Second),
		Payload: json.RawMessage(`{"LID":202,"Y":[[1,[10,5,-1],[12,7,-2]],[-403,[20,10,-10]]]}`),
	}
	domains, changed, err := reduceBattleDetailCapture(context.Background(), details, &gameState, nil)
	if err != nil || !changed {
		t.Fatalf("reduce island details: changed=%t err=%v", changed, err)
	}
	if len(domains) != 2 || domains[0] != "reports" || domains[1] != "storm" {
		t.Fatalf("island report domains = %#v", domains)
	}
	operation := gameState.Storm.IslandReturns[key]
	if operation.Status != State.StormIslandReturnReady || operation.ReportID != 202 ||
		operation.Survivors[10] != 4 || operation.Survivors[12] != 5 {
		t.Fatalf("report-confirmed island return = %#v", operation)
	}
	returnUnits := operation.UnitsToReturn()
	if returnUnits[10] != 4 || returnUnits[12] != 4 {
		t.Fatalf("island return units = %#v", returnUnits)
	}
}

func TestSuccessfulTowerBattleRefreshesCooldownFromFollowUpMapSnapshot(t *testing.T) {
	gameState := State.NewGameState()
	gameState.Player.ID = 1
	gameState.NomadCamps.RBCTest = &State.NomadRBCTestState{
		RunID: "trial", KingdomID: 3, TargetX: 484, TargetY: 490, ExpectedAttacks: 2, AttacksLaunched: 2,
	}
	code := 0
	battleAt := time.Date(2026, 7, 12, 16, 0, 0, 0, time.UTC)
	summary := Protocol.Frame{
		Direction: Protocol.DirectionInbound, Opcode: "bls", ResponseCode: &code, ReceivedAt: battleAt,
		Payload: json.RawMessage(`{
			"MID":101,"LID":202,"AHP":0,"DHP":0,
			"PBI":[[1,0,1700,-10],[-220,1,135,-135]],
			"AI":{"AT":2,"K":3,"X":484,"Y":490}
		}`),
	}
	if _, changed, err := reduceSuccessfulTowerBattle(context.Background(), summary, &gameState, nil); err != nil || !changed {
		t.Fatalf("tower battle: changed=%t err=%v", changed, err)
	}
	cooldown := gameState.TowerCooldowns["3:484:490"]
	if !cooldown.PendingCooldownRefresh || cooldown.ReportID != 202 || !cooldown.LastSuccessfulBattleAt.Equal(battleAt) {
		t.Fatalf("unexpected pending cooldown: %#v", cooldown)
	}
	if gameState.NomadCamps.RBCTest.VictoriesConfirmed != 1 || gameState.NomadCamps.RBCTest.LastReportID != 202 {
		t.Fatalf("RBC trial did not record its confirmed victory: %#v", gameState.NomadCamps.RBCTest)
	}

	observedAt := battleAt.Add(2 * time.Second)
	mapFrame := Protocol.Frame{
		Direction: Protocol.DirectionInbound, Opcode: "gaa", ResponseCode: &code, ReceivedAt: observedAt,
		Payload: json.RawMessage(`{"KID":3,"AI":[[2,484,490,-1,845,10705,0]]}`),
	}
	if _, changed, err := reduceMapSnapshot(context.Background(), mapFrame, &gameState, nil); err != nil || !changed {
		t.Fatalf("tower cooldown map refresh: changed=%t err=%v", changed, err)
	}
	cooldown = gameState.TowerCooldowns["3:484:490"]
	if cooldown.PendingCooldownRefresh || cooldown.CooldownRemaining != 10705 || !cooldown.CooldownObservedAt.Equal(observedAt) {
		t.Fatalf("unexpected refreshed cooldown: %#v", cooldown)
	}
}

func TestSuccessfulType35KhanBattleCreatesLandingCooldownBoundary(t *testing.T) {
	gameState := State.NewGameState()
	gameState.Player.ID = 1
	gameState.Khan = State.KhanState{
		RunID: "khan", KingdomID: 0, TargetX: 939, TargetY: 1123,
		AttacksLaunched: 1, Taunts: map[State.MovementID]State.KhanTauntState{},
	}
	code := 0
	battleAt := time.Date(2026, 7, 25, 15, 30, 41, 0, time.UTC)
	summary := Protocol.Frame{
		Direction: Protocol.DirectionInbound, Opcode: "bls", ResponseCode: &code, ReceivedAt: battleAt,
		Payload: json.RawMessage(`{
			"MID":2194729220,"LID":711545217,
			"PBI":[[1,0,4850,-331],[-801,1,15812,-15812]],
			"AI":{"AT":35,"K":0,"X":939,"Y":1123,"DP":-801}
		}`),
	}
	domains, changed, err := reduceSuccessfulTowerBattle(t.Context(), summary, &gameState, nil)
	if err != nil || !changed {
		t.Fatalf("Khan battle: domains=%v changed=%t err=%v", domains, changed, err)
	}
	if gameState.Khan.VictoriesConfirmed != 1 || gameState.Khan.LastReportID != 711545217 {
		t.Fatalf("confirmed Khan landing = %#v", gameState.Khan)
	}
	report, exists := gameState.Khan.CooldownReports[711545217]
	if !exists || report.ReportID != 711545217 || report.X != 939 || report.Y != 1123 ||
		!report.LandedAt.Equal(battleAt) || !report.CooldownObservedAt.IsZero() {
		t.Fatalf("report-linked Khan cooldown = %#v", report)
	}
	cooldown := gameState.NomadCamps.Cooldowns["0:939:1123"]
	if !cooldown.PendingCooldownRefresh || cooldown.ReportID != 711545217 ||
		!cooldown.LastSuccessfulBattleAt.Equal(battleAt) {
		t.Fatalf("pending Khan cooldown = %#v", cooldown)
	}
	if _, exists := gameState.TowerCooldowns["0:939:1123"]; exists {
		t.Fatal("type-35 Khan landing was stored as a kingdom-tower cooldown")
	}

	observedAt := battleAt.Add(2 * time.Second)
	mapFrame := Protocol.Frame{
		Direction: Protocol.DirectionInbound, Opcode: "gaa", ResponseCode: &code, ReceivedAt: observedAt,
		Payload: json.RawMessage(`{
			"KID":0,
			"AI":[[35,939,1123,6760,-1,2246,2250,5665,24,1146,200,200,85]]
		}`),
	}
	if _, changed, err := reduceMapSnapshot(t.Context(), mapFrame, &gameState, nil); err != nil || !changed {
		t.Fatalf("Khan cooldown map refresh: changed=%t err=%v", changed, err)
	}
	cooldown = gameState.NomadCamps.Cooldowns["0:939:1123"]
	if cooldown.PendingCooldownRefresh || cooldown.CooldownRemaining != 2246 ||
		!cooldown.CooldownObservedAt.Equal(observedAt) {
		t.Fatalf("refreshed Khan cooldown = %#v", cooldown)
	}
	report = gameState.Khan.CooldownReports[711545217]
	if report.CooldownRemaining != 2246 || !report.CooldownObservedAt.Equal(observedAt) {
		t.Fatalf("report did not receive the target re-ping cooldown = %#v", report)
	}
}
