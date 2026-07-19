package Ingest

import (
	"encoding/json"
	"testing"
	"time"

	"CitadelDesktop/Server/GameData"
	"CitadelDesktop/Server/Protocol"
	"CitadelDesktop/Server/State"
)

func TestScalableEventScoresTrackSnapshotsAndPointUpdates(t *testing.T) {
	gameData := scalableEventTestGameData(t)
	gameState := State.NewGameState()
	observedAt := time.Date(2026, 7, 13, 20, 23, 4, 0, time.UTC)
	code := 0

	_, changed, err := reduceScalableEventSnapshot(t.Context(), Protocol.Frame{
		Opcode: "sei", Direction: Protocol.DirectionInbound, ResponseCode: &code, ReceivedAt: observedAt,
		Payload: json.RawMessage(`{"E":[
			{"EID":72,"RS":25616,"SP":{"OP":1500,"OR":25},"A":{"OP":184426,"OR":150},"EASE":1,"EDID":308,"PIDS":"10, 11,bad"},
			{"EID":68,"RS":500,"PID":[12,13]}
		]}`),
	}, &gameState, gameData)
	if err != nil || !changed {
		t.Fatalf("event snapshot: changed=%t err=%v", changed, err)
	}
	score, found := gameState.ActiveScalableEventScore()
	if !found || score.EventID != 72 || score.DifficultyTypeName != "expertPlus" || score.PlayerScore != 1500 || score.AllianceScore != 184426 {
		t.Fatalf("unexpected event score: %#v found=%t", score, found)
	}
	if !gameState.ScalableEventScoreReached(72, 1500) || gameState.ScalableEventScoreReached(72, 1501) || !gameState.ActiveScalableEventScoreReached(1500) {
		t.Fatalf("threshold helper did not use the player score: %#v", gameState.EventScores)
	}
	for packageID, eventID := range map[State.PackageID]int64{10: 72, 11: 72, 12: 68, 13: 68} {
		route, active := gameState.ActiveShopForPackage(packageID, observedAt.Add(time.Second))
		if !active || route.EventID != eventID {
			t.Fatalf("package %d route = %#v active=%t", packageID, route, active)
		}
	}

	_, changed, err = reduceEventPoints(t.Context(), Protocol.Frame{
		Opcode: "pep", Direction: Protocol.DirectionInbound, ResponseCode: &code, ReceivedAt: observedAt.Add(time.Minute),
		Payload: json.RawMessage(`{"OP":[2200,190000],"OR":[18,140],"PT":[0],"EID":72}`),
	}, &gameState, gameData)
	if err != nil || !changed {
		t.Fatalf("event points: changed=%t err=%v", changed, err)
	}
	score, _ = gameState.ActiveScalableEventScore()
	if score.PlayerScore != 2200 || score.AllianceScore != 190000 || score.PlayerRank != 18 || score.AllianceRank != 140 || score.RemainingSec != 25556 {
		t.Fatalf("point update was not applied: %#v", score)
	}
}

func scalableEventTestGameData(t *testing.T) *GameData.Store {
	t.Helper()
	store, err := GameData.DecodeStore([]byte(`{
		"versionInfo":[],
		"buildings":[{"wodID":1}],
		"units":[{"wodID":1}],
		"events":[{"eventID":"72","comment1":"AllianceNomad Invasion","eventType":"AllianceNomadInvasion"}],
		"eventAutoScalingDifficulties":[{"difficultyID":"308","eventID":"72","difficultyTypeID":"8"}],
		"eventAutoScalingDifficultyTypes":[{"difficultyTypeID":"8","name":"expertPlus","sortOrder":"8"}]
	}`), GameData.SourceMetadata{ItemVersion: "test"})
	if err != nil {
		t.Fatal(err)
	}
	return store
}
