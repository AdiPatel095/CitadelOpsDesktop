package Ingest

import (
	"context"
	"encoding/json"
	"slices"
	"testing"
	"time"

	"CitadelDesktop/Server/GameData"
	"CitadelDesktop/Server/Protocol"
	"CitadelDesktop/Server/State"
)

func TestAllianceReducerCapturesProtectedHoldings(t *testing.T) {
	gameState := State.NewGameState()
	gameState.Player.ID = 42
	code := 0
	frame := Protocol.Frame{
		Opcode: "ain", Direction: Protocol.DirectionInbound, ResponseCode: &code,
		ReceivedAt: time.Now().UTC(),
		Payload:    json.RawMessage(`{"A":{"AID":9,"N":"Test","M":[{"OID":42,"N":"Player","RPT":259200,"AP":[[0,100,12,34,1],[2,200,45,67,12]]}]}}`),
	}
	domains, changed, err := reduceAllianceInfo(context.Background(), frame, &gameState, nil)
	if err != nil || !changed {
		t.Fatalf("reduce alliance: changed=%v domains=%v err=%v", changed, domains, err)
	}
	if len(gameState.Alliance.Members) != 1 || gameState.Alliance.Members[0].ReturnProtectionSec != 259200 {
		t.Fatalf("unexpected alliance member: %#v", gameState.Alliance.Members)
	}
	if len(gameState.Alliance.Holdings) != 2 {
		t.Fatalf("unexpected holdings: %#v", gameState.Alliance.Holdings)
	}
	if gameState.Alliances[9].ID != 9 {
		t.Fatalf("alliance directory = %#v", gameState.Alliances)
	}
	holding := gameState.Alliance.Holdings[1]
	if holding.PlayerID != 42 || holding.CastleID != 200 || holding.KingdomID != 2 || holding.X != 45 || holding.Y != 67 || holding.SlotType != 12 {
		t.Fatalf("unexpected holding: %#v", holding)
	}
}

func TestAllianceReducerDoesNotReplaceOwnAllianceDuringInspection(t *testing.T) {
	gameState := State.NewGameState()
	gameState.Player.ID = 42
	gameState.Player.AllianceID = 9
	gameState.Alliance = State.AllianceState{ID: 9, Name: "Own", Members: []State.AllianceMember{}, Holdings: []State.AllianceHolding{}}
	code := 0
	frame := Protocol.Frame{
		Opcode: "ain", Direction: Protocol.DirectionInbound, ResponseCode: &code,
		ReceivedAt: time.Now().UTC(),
		Payload:    json.RawMessage(`{"A":{"AID":10,"N":"Target","M":[{"OID":77,"N":"Other","RPT":0,"AP":[[0,200,45,67,1]]}]}}`),
	}
	if _, changed, err := reduceAllianceInfo(t.Context(), frame, &gameState, nil); err != nil || !changed {
		t.Fatalf("reduce inspected alliance: changed=%t err=%v", changed, err)
	}
	if gameState.Alliance.ID != 9 || gameState.Alliance.Name != "Own" {
		t.Fatalf("own alliance was replaced: %+v", gameState.Alliance)
	}
	if gameState.Alliances[10].Name != "Target" || len(gameState.Alliances[10].Holdings) != 1 {
		t.Fatalf("inspected alliance missing: %+v", gameState.Alliances[10])
	}
}

func TestAllianceReducerClearsStaleOwnAllianceWhenAINRosterExcludesCurrentPlayer(t *testing.T) {
	gameState := State.NewGameState()
	gameState.Player.ID = 42
	gameState.Player.AllianceID = 9
	gameState.Alliance = State.AllianceState{
		ID: 9, Name: "Former", Members: []State.AllianceMember{{PlayerID: 42}}, Holdings: []State.AllianceHolding{},
	}
	code := 0
	frame := Protocol.Frame{
		Opcode: "ain", Direction: Protocol.DirectionInbound, ResponseCode: &code,
		ReceivedAt: time.Now().UTC(),
		Payload:    json.RawMessage(`{"A":{"AID":9,"N":"Former","M":[{"OID":77,"N":"Other","RPT":0,"AP":[[0,200,45,67,1]]}]}}`),
	}
	if _, changed, err := reduceAllianceInfo(t.Context(), frame, &gameState, nil); err != nil || !changed {
		t.Fatalf("reduce stale alliance: changed=%t err=%v", changed, err)
	}
	if gameState.Player.AllianceID != 0 || gameState.Alliance.ID != 0 || len(gameState.Alliance.Members) != 0 || len(gameState.Alliance.Holdings) != 0 {
		t.Fatalf("stale membership was retained: player=%+v alliance=%+v", gameState.Player, gameState.Alliance)
	}
	if gameState.Alliances[9].Name != "Former" || len(gameState.Alliances[9].Members) != 1 {
		t.Fatalf("alliance directory did not retain response: %+v", gameState.Alliances[9])
	}
}

func TestInitialStateReducerWaitsForCastleSnapshotBeforeCompletingSessionBaseline(t *testing.T) {
	now := time.Date(2026, 7, 13, 12, 0, 0, 0, time.UTC)
	gameState := State.NewGameState()
	gameState.Session.LoggedIn = true
	gameState.Session.SocketReady = true
	gameState.Session.Generation = 7
	gameState.Session.ChangedAt = now.Add(-time.Second)
	code := 0
	frame := Protocol.Frame{
		Opcode: "gbd", Direction: Protocol.DirectionInbound, ResponseCode: &code,
		ReceivedAt: now, Payload: json.RawMessage(`{}`),
	}
	domains, _, err := reduceInitialState(t.Context(), frame, &gameState, nil)
	if err != nil || gameState.Session.BaselineGeneration != 0 {
		t.Fatalf("account snapshot completed command baseline: domains=%v session=%+v err=%v", domains, gameState.Session, err)
	}

	gameState.Session.ChangedAt = now.Add(time.Second)
	frame.ReceivedAt = now
	_, _, err = reduceInitialState(t.Context(), frame, &gameState, nil)
	if err != nil || gameState.Session.BaselineGeneration != 0 {
		t.Fatalf("stale account snapshot completed command baseline: session=%+v err=%v", gameState.Session, err)
	}
}

func TestInitialStateReducerRefreshesSessionUIDWithoutReset(t *testing.T) {
	gameState := State.NewGameState()
	gameState.Session.ServerURL = "wss://world.example"
	gameState.Account = State.AccountBindingState{UID: 111, WorldID: gameState.Session.ServerURL, PlayerID: 42}
	gameState.Player.ID = 42
	gameState.Castles[9] = newCastleState(9)
	gameState.EventScores.ActivityByEvent[103] = State.EventActivityState{
		EventID: 103,
		PendingAttacks: []State.EventAttackRecord{{
			MovementID: 88,
			Kind:       State.EventActivityInvasion,
			TargetX:    206,
			TargetY:    937,
		}},
	}
	code := 0
	frame := Protocol.Frame{
		Opcode: "gbd", Direction: Protocol.DirectionInbound, ResponseCode: &code,
		ReceivedAt: time.Now().UTC(),
		Payload:    json.RawMessage(`{"gpi":{"UID":222,"PID":42,"PN":"Same account"}}`),
	}

	_, changed, err := reduceInitialState(t.Context(), frame, &gameState, nil)
	if err != nil || !changed {
		t.Fatalf("reduce UID refresh: changed=%t err=%v", changed, err)
	}
	if gameState.Account.UID != 222 || gameState.Account.PlayerID != 42 {
		t.Fatalf("account binding = %+v", gameState.Account)
	}
	if _, found := gameState.Castles[9]; !found {
		t.Fatalf("same-account castle was cleared on UID refresh: %+v", gameState.Castles)
	}
	activity := gameState.EventScores.ActivityByEvent[103]
	if len(activity.PendingAttacks) != 1 || activity.PendingAttacks[0].MovementID != 88 {
		t.Fatalf("same-account pending event attack was cleared on UID refresh: %+v", activity)
	}
}

func TestPlayerProtectionModeUsesRPTInsteadOfPMTCooldown(t *testing.T) {
	now := time.Date(2026, 7, 22, 8, 8, 0, 0, time.UTC)
	gameState := State.NewGameState()
	code := 0
	frame := Protocol.Frame{
		Opcode: "gbd", Direction: Protocol.DirectionInbound, ResponseCode: &code, ReceivedAt: now,
		Payload: json.RawMessage(`{"gpi":{"PID":42},"uap":{"KID":0,"NS":-1,"PMS":1,"PMT":320207},"ain":{"A":{"M":[{"OID":42,"RPT":320207}]}}}`),
	}
	domains, changed, err := reduceInitialState(t.Context(), frame, &gameState, nil)
	if err != nil || !changed {
		t.Fatalf("reduce purchased Protection Mode: changed=%t domains=%v err=%v", changed, domains, err)
	}
	protection := gameState.Player.ProtectionMode
	if protection.ModeState != 1 || protection.ModeTimerSec != 320207 || protection.RemainingSec != 320207 ||
		!protection.ModeObservedAt.Equal(now) || !protection.ObservedAt.Equal(now) ||
		!protection.PreparingOrActive(now) || !slices.Contains(domains, "player-protection") {
		t.Fatalf("purchased Protection Mode = %+v domains=%v", protection, domains)
	}

	frame = Protocol.Frame{
		Opcode: "jaa", Direction: Protocol.DirectionInbound, ResponseCode: &code, ReceivedAt: now.Add(time.Minute),
		Payload: json.RawMessage(`{"uap":{"KID":0,"NS":-1,"PMS":2,"PMT":171741},"gca":{"O":{"OID":42,"RPT":0}}}`),
	}
	domains, changed, err = reducePlayerProtectionMode(t.Context(), frame, &gameState, nil)
	if err != nil || !changed || gameState.Player.ProtectionMode.PreparingOrActive(frame.ReceivedAt) ||
		!slices.Contains(domains, "player-protection") {
		t.Fatalf("cancel Protection Mode: changed=%t state=%+v domains=%v err=%v", changed, gameState.Player.ProtectionMode, domains, err)
	}
	protection = gameState.Player.ProtectionMode
	if protection.ModeState != 2 || protection.ModeTimerSec != 171741 || protection.RemainingSec != 0 ||
		!protection.ObservedAt.Equal(frame.ReceivedAt) {
		t.Fatalf("post-cancel cooldown state = %+v", protection)
	}
}

func TestPlayerProtectionModeRefreshUsesOnlyCurrentPlayersRPT(t *testing.T) {
	now := time.Date(2026, 7, 22, 9, 46, 21, 0, time.UTC)
	gameState := State.NewGameState()
	gameState.Player.ID = 42
	gameState.Player.ProtectionMode = State.PlayerProtectionModeState{
		ModeState: 1, ModeTimerSec: 200000, ModeObservedAt: now.Add(-time.Hour),
		RemainingSec: 200000, ObservedAt: now.Add(-time.Hour),
	}
	code := 0
	frame := Protocol.Frame{
		Opcode: "gaa", Direction: Protocol.DirectionInbound, ResponseCode: &code, ReceivedAt: now,
		Payload: json.RawMessage(`{"uap":{"PMS":2,"PMT":171741},"OI":[{"OID":99,"RPT":999999},{"OID":42,"RPT":0}]}`),
	}
	domains, changed, err := reducePlayerProtectionMode(t.Context(), frame, &gameState, nil)
	if err != nil || !changed {
		t.Fatalf("refresh Protection Mode: changed=%t domains=%v err=%v", changed, domains, err)
	}
	protection := gameState.Player.ProtectionMode
	if protection.RemainingSec != 0 || !protection.ObservedAt.Equal(now) || protection.PreparingOrActive(now) ||
		!slices.Contains(domains, "player-protection") {
		t.Fatalf("refreshed Protection Mode = %+v domains=%v", protection, domains)
	}
}

func TestPlayerProtectionModeDoesNotRefreshRPTFromUAPAlone(t *testing.T) {
	now := time.Date(2026, 7, 22, 9, 46, 21, 0, time.UTC)
	gameState := State.NewGameState()
	gameState.Player.ID = 42
	code := 0
	frame := Protocol.Frame{
		Opcode: "gaa", Direction: Protocol.DirectionInbound, ResponseCode: &code, ReceivedAt: now,
		Payload: json.RawMessage(`{"uap":{"PMS":2,"PMT":171741},"OI":[{"OID":99,"RPT":999999}]}`),
	}
	_, changed, err := reducePlayerProtectionMode(t.Context(), frame, &gameState, nil)
	if err != nil || !changed {
		t.Fatalf("reduce mode cooldown: changed=%t err=%v", changed, err)
	}
	protection := gameState.Player.ProtectionMode
	if protection.ModeTimerSec != 171741 || protection.RemainingSec != 0 || !protection.ObservedAt.IsZero() ||
		protection.PreparingOrActive(now) {
		t.Fatalf("mode cooldown was treated as protection = %+v", protection)
	}
}

func TestInitialStateReducerHydratesGameReportedOpenGateDuration(t *testing.T) {
	now := time.Date(2026, 7, 22, 9, 0, 0, 0, time.UTC)
	gameState := State.NewGameState()
	code := 0
	frame := Protocol.Frame{
		Opcode: "gbd", Direction: Protocol.DirectionInbound, ResponseCode: &code, ReceivedAt: now,
		Payload: json.RawMessage(`{"dcl":{"C":[{"KID":0,"AI":[{"AID":100,"OGT":90}]}]}}`),
	}
	_, changed, err := reduceInitialState(t.Context(), frame, &gameState, nil)
	if err != nil || !changed {
		t.Fatalf("hydrate game-reported Open Gate: changed=%t err=%v", changed, err)
	}
	openUntil := gameState.Castles[100].Defense.OpenGateUntil
	if openUntil == nil || !openUntil.Equal(now.Add(90*time.Second)) {
		t.Fatalf("game-reported Open Gate until = %v", openUntil)
	}

	frame.ReceivedAt = now.Add(10 * time.Second)
	frame.Payload = json.RawMessage(`{"dcl":{"C":[{"KID":0,"AI":[{"AID":100,"OGT":0}]}]}}`)
	_, changed, err = reduceInitialState(t.Context(), frame, &gameState, nil)
	if err != nil || !changed || gameState.Castles[100].Defense.OpenGateUntil != nil {
		t.Fatalf("clear game-reported Open Gate: changed=%t until=%v err=%v", changed, gameState.Castles[100].Defense.OpenGateUntil, err)
	}
}

func TestInitialStateReducerHydratesAchievementUnlockProgress(t *testing.T) {
	now := time.Date(2026, 7, 14, 12, 0, 0, 0, time.UTC)
	gameState := State.NewGameState()
	code := 0
	frame := Protocol.Frame{
		Opcode: "gbd", Direction: Protocol.DirectionInbound, ResponseCode: &code, ReceivedAt: now,
		Payload: json.RawMessage(`{"vli":{"AVP":78280,"RA":[{"AID":1088,"P":[40]},{"AID":1094,"P":[0]}],"FA":[1084,1085,1086,1087,1090,1091,1092,1093]}}`),
	}
	_, changed, err := reduceInitialState(t.Context(), frame, &gameState, nil)
	if err != nil || !changed {
		t.Fatalf("reduce achievement snapshot: changed=%t err=%v", changed, err)
	}
	if gameState.Player.Achievements.Points != 78280 || !gameState.Player.Achievements.Completed[1087] || !gameState.Player.Achievements.Completed[1093] {
		t.Fatalf("unexpected completed achievements: %+v", gameState.Player.Achievements)
	}
	if progress := gameState.Player.Achievements.Progress[1088]; len(progress) != 1 || progress[0] != 40 {
		t.Fatalf("unexpected running achievement progress: %v", progress)
	}
	if !gameState.Player.Achievements.ObservedAt.Equal(now) {
		t.Fatalf("achievement observation time = %v", gameState.Player.Achievements.ObservedAt)
	}
}

func TestLegendSkillReducerCapturesActiveHallOfLegendsSkills(t *testing.T) {
	now := time.Date(2026, 7, 14, 14, 0, 0, 0, time.UTC)
	gameState := State.NewGameState()
	code := 0
	frame := Protocol.Frame{
		Opcode: "skl", Direction: Protocol.DirectionInbound, ResponseCode: &code, ReceivedAt: now,
		Payload: json.RawMessage(`{"SID":[106,251,193],"RS":1296000,"SP":"550","RC":1,"SIDS":[363,365],"SSA":[{"ID":263,"RS":18665},{"ID":"366","RS":"1910845"}]}`),
	}
	_, changed, err := reduceLegendSkills(t.Context(), frame, &gameState, nil)
	if err != nil || !changed {
		t.Fatalf("reduce Hall of Legends skills: changed=%t err=%v", changed, err)
	}
	if !slices.Equal(gameState.Player.LegendSkills.ActiveIDs, []int64{251, 193, 106}) ||
		gameState.Player.LegendSkills.SkillPoints != 550 ||
		gameState.Player.LegendSkills.ResetRemainingSec != 1296000 ||
		gameState.Player.LegendSkills.ResetCount != 1 ||
		!slices.Equal(gameState.Player.LegendSkills.SceatSkillIDs, []int64{365, 363}) ||
		!slices.Equal(gameState.Player.LegendSkills.SceatActivations, []State.SceatSkillActivation{{ID: 366, RemainingSec: 1910845}, {ID: 263, RemainingSec: 18665}}) ||
		!gameState.Player.LegendSkills.ObservedAt.Equal(now) {
		t.Fatalf("unexpected Hall of Legends state: %+v", gameState.Player.LegendSkills)
	}
}

func TestLegendSkillPurchaseReducerReplacesSkillGroupAndAddsAutomaticUnlock(t *testing.T) {
	gameData, err := GameData.DecodeStore([]byte(`{
		"versionInfo":{},
		"buildings":[{"wodID":1}],
		"units":[{"wodID":1}],
		"legendskills":[
			{"skillID":71,"skillTreeID":0,"skillGroupID":7},
			{"skillID":72,"skillTreeID":0,"skillGroupID":7},
			{"skillID":101,"skillTreeID":0,"skillGroupID":10,"specialType":"special"}
		]
	}`), GameData.SourceMetadata{ItemVersion: "test"})
	if err != nil {
		t.Fatal(err)
	}
	previous := time.Date(2026, 7, 14, 14, 0, 0, 0, time.UTC)
	now := previous.Add(time.Minute)
	gameState := State.NewGameState()
	gameState.Player.LegendSkills = State.LegendSkillState{
		ActiveIDs: []int64{71, 20}, SkillPoints: 550, ResetRemainingSec: 3600,
		SceatSkillIDs: []int64{365}, SceatActivations: []State.SceatSkillActivation{{ID: 366, RemainingSec: 120}}, ObservedAt: previous,
	}
	code := 0
	frame := Protocol.Frame{
		Opcode: "skp", Direction: Protocol.DirectionInbound, ResponseCode: &code, ReceivedAt: now,
		Payload: json.RawMessage(`{"IDS":["72","101"]}`),
	}
	_, changed, err := reduceLegendSkillPurchase(t.Context(), frame, &gameState, gameData)
	if err != nil || !changed {
		t.Fatalf("reduce Hall of Legends purchase: changed=%t err=%v", changed, err)
	}
	if !slices.Equal(gameState.Player.LegendSkills.ActiveIDs, []int64{101, 72, 20}) ||
		!slices.Equal(gameState.Player.LegendSkills.SceatSkillIDs, []int64{365}) ||
		gameState.Player.LegendSkills.ResetRemainingSec != 3540 ||
		!slices.Equal(gameState.Player.LegendSkills.SceatActivations, []State.SceatSkillActivation{{ID: 366, RemainingSec: 60}}) ||
		!gameState.Player.LegendSkills.ObservedAt.Equal(now) {
		t.Fatalf("unexpected Hall of Legends purchase state: %+v", gameState.Player.LegendSkills)
	}
}

func TestLegendSkillResetReducerInvalidatesSnapshotUntilSKLRefresh(t *testing.T) {
	gameState := State.NewGameState()
	gameState.Player.LegendSkills = State.LegendSkillState{
		ActiveIDs: []int64{251, 193}, SkillPoints: 550, ResetCount: 1,
		SceatSkillIDs: []int64{365}, ObservedAt: time.Now().UTC(),
	}
	code := 0
	frame := Protocol.Frame{Opcode: "skr", Direction: Protocol.DirectionInbound, ResponseCode: &code}
	_, changed, err := reduceLegendSkillReset(t.Context(), frame, &gameState, nil)
	if err != nil || !changed {
		t.Fatalf("reduce Hall of Legends reset: changed=%t err=%v", changed, err)
	}
	if len(gameState.Player.LegendSkills.ActiveIDs) != 0 ||
		!gameState.Player.LegendSkills.ObservedAt.IsZero() ||
		!slices.Equal(gameState.Player.LegendSkills.SceatSkillIDs, []int64{365}) {
		t.Fatalf("unexpected Hall of Legends reset state: %+v", gameState.Player.LegendSkills)
	}
}
