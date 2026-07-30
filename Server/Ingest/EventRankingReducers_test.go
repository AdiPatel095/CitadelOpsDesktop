package Ingest

import (
	"encoding/json"
	"testing"
	"time"

	"CitadelDesktop/Server/Protocol"
	"CitadelDesktop/Server/State"
)

func TestEventRankingReducerStoresAllPublicNomadAllianceFields(t *testing.T) {
	gameState := State.NewGameState()
	gameState.Player.AllianceID = 182288
	gameState.EventScores.ActiveEventID = 72
	gameState.EventScores.ByEvent[72] = State.ScalableEventScore{EventID: 72, LeagueID: 5, AllianceLeagueID: 1}
	gameState.EventScores.RankingByEvent[72] = State.EventRankingState{
		EventID: 72, Scope: "alliance", LeagueID: 1, ListType: 47, Pending: true, Entries: []State.EventRankingEntry{},
	}
	now := time.Date(2026, 7, 20, 20, 0, 0, 0, time.UTC)
	code := 0
	frame := Protocol.Frame{
		Opcode: "hgh", Direction: Protocol.DirectionInbound, ResponseCode: &code, ReceivedAt: now,
		Payload: json.RawMessage(`{"LT":47,"LID":1,"L":[[140,280000,[190452,"Other",20,950130065]],[141,284713,[182288,"Sharks",33,965245397]]],"LR":221,"SV":"-1","FR":140,"IGH":0}`),
	}
	if _, changed, err := reduceEventRanking(t.Context(), frame, &gameState, nil); err != nil || !changed {
		t.Fatalf("changed=%t err=%v", changed, err)
	}
	ranking := gameState.EventScores.RankingByEvent[72]
	if ranking.Pending || ranking.Scope != "alliance" || ranking.TotalAlliances != 221 || ranking.FirstRank != 140 ||
		ranking.SearchValue != "-1" || ranking.GlobalFlag != 0 || ranking.OwnAllianceID != 182288 || len(ranking.Entries) != 2 {
		t.Fatalf("ranking = %#v", ranking)
	}
	entry := ranking.Entries[1]
	if entry.AllianceID != 182288 || entry.Alliance != "Sharks" || entry.MemberCount != 33 ||
		entry.FamePoints != 965245397 || entry.Rank != 141 || entry.Score != 284713 {
		t.Fatalf("own alliance entry = %#v", entry)
	}
}

func TestEventRankingReducerClearsPendingRefreshAfterGameRejection(t *testing.T) {
	gameState := State.NewGameState()
	gameState.EventScores.RankingByEvent[72] = State.EventRankingState{
		EventID: 72, Scope: "alliance", LeagueID: 1, ListType: 47, Pending: true, Entries: []State.EventRankingEntry{},
	}
	code := 1
	frame := Protocol.Frame{Opcode: "hgh", Direction: Protocol.DirectionInbound, ResponseCode: &code}
	if _, changed, err := reduceEventRanking(t.Context(), frame, &gameState, nil); err != nil || !changed {
		t.Fatalf("failed ranking frame: changed=%t err=%v", changed, err)
	}
	if gameState.EventScores.RankingByEvent[72].Pending {
		t.Fatal("ranking remained pending after the game rejected the refresh")
	}
}
