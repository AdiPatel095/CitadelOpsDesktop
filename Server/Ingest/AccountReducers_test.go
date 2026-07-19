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

func TestInitialStateReducerHydratesOnlyCurrentReadySession(t *testing.T) {
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
	domains, changed, err := reduceInitialState(t.Context(), frame, &gameState, nil)
	if err != nil || !changed || gameState.Session.BaselineGeneration != 7 {
		t.Fatalf("current baseline reduction: changed=%t domains=%v session=%+v err=%v", changed, domains, gameState.Session, err)
	}
	if !slices.Contains(domains, "session") {
		t.Fatalf("baseline reduction domains = %v", domains)
	}

	gameState.Session.BaselineGeneration = 0
	gameState.Session.ChangedAt = now.Add(time.Second)
	frame.ReceivedAt = now
	_, changed, err = reduceInitialState(t.Context(), frame, &gameState, nil)
	if err != nil || changed || gameState.Session.BaselineGeneration != 0 {
		t.Fatalf("stale baseline hydrated newer session: changed=%t session=%+v err=%v", changed, gameState.Session, err)
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
