package Ingest

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"

	"CitadelDesktop/Server/GameData"
	"CitadelDesktop/Server/Protocol"
	"CitadelDesktop/Server/State"
)

func reduceEventRanking(
	_ context.Context,
	frame Protocol.Frame,
	gameState *State.GameState,
	_ *GameData.Store,
) ([]string, bool, error) {
	if !frameSucceeded(frame) {
		if frame.Opcode == "hgh" && clearPendingEventRankings(gameState) {
			return []string{"events", "event-scores"}, true, nil
		}
		return nil, false, nil
	}
	if frame.Opcode != "hgh" || len(frame.Payload) == 0 {
		return nil, false, nil
	}
	var payload struct {
		Entries     []json.RawMessage `json:"L"`
		LeagueID    *wireInt64        `json:"LID"`
		ListType    wireInt64         `json:"LT"`
		Total       wireInt64         `json:"LR"`
		SearchValue string            `json:"SV"`
		FirstRank   wireInt64         `json:"FR"`
		GlobalFlag  wireInt64         `json:"IGH"`
	}
	if err := json.Unmarshal(frame.Payload, &payload); err != nil {
		return nil, false, fmt.Errorf("decode event ranking: %w", err)
	}
	if payload.LeagueID == nil {
		return nil, false, nil
	}
	leagueID := int64(*payload.LeagueID)
	listType := int64(payload.ListType)
	eventID := rankingEventID(*gameState, leagueID, listType)
	if eventID <= 0 {
		return nil, false, nil
	}
	ranking, _ := gameState.MutableEventRanking(eventID)
	if ranking.EventID != eventID || ranking.LeagueID != leagueID || ranking.ListType != listType {
		ranking = State.EventRankingState{
			EventID: eventID, Scope: "alliance", LeagueID: leagueID, ListType: listType,
			OwnAllianceID: gameState.Player.AllianceID, Entries: []State.EventRankingEntry{},
		}
	}
	entries := make(map[string]State.EventRankingEntry, len(payload.Entries))
	for index, rawRow := range payload.Entries {
		entry, err := decodeEventAllianceRankingEntry(rawRow)
		if err != nil {
			return nil, false, fmt.Errorf("decode event ranking row %d: %w", index, err)
		}
		entries[eventRankingEntryKey(entry)] = entry
	}
	ranking.Entries = ranking.Entries[:0]
	for _, entry := range entries {
		ranking.Entries = append(ranking.Entries, entry)
	}
	sort.Slice(ranking.Entries, func(left, right int) bool {
		if ranking.Entries[left].Rank != ranking.Entries[right].Rank {
			return ranking.Entries[left].Rank < ranking.Entries[right].Rank
		}
		return ranking.Entries[left].Alliance < ranking.Entries[right].Alliance
	})
	ranking.Scope = "alliance"
	ranking.TotalAlliances = int64(payload.Total)
	ranking.OwnAllianceID = gameState.Player.AllianceID
	ranking.SearchValue = payload.SearchValue
	ranking.FirstRank = int64(payload.FirstRank)
	ranking.GlobalFlag = int64(payload.GlobalFlag)
	ranking.Pending = false
	ranking.ObservedAt = frame.ReceivedAt.UTC()
	gameState.SetEventRanking(eventID, ranking)
	return []string{"events", "event-scores"}, true, nil
}

func rankingEventID(gameState State.GameState, leagueID int64, listType int64) int64 {
	var selected int64
	gameState.RangeEventRankings(func(eventID int64, ranking State.EventRankingState) bool {
		if ranking.Pending && ranking.LeagueID == leagueID && ranking.ListType == listType {
			selected = eventID
			return false
		}
		return true
	})
	return selected
}

func clearPendingEventRankings(gameState *State.GameState) bool {
	if gameState == nil {
		return false
	}
	changed := false
	gameState.RangeEventRankings(func(eventID int64, ranking State.EventRankingState) bool {
		if !ranking.Pending {
			return true
		}
		ranking.Pending = false
		gameState.SetEventRanking(eventID, ranking)
		changed = true
		return true
	})
	return changed
}

func eventRankingEntryKey(entry State.EventRankingEntry) string {
	if entry.AllianceID > 0 {
		return "alliance:" + strconv.FormatInt(int64(entry.AllianceID), 10)
	}
	return strconv.FormatInt(entry.Rank, 10) + ":" + entry.Alliance
}

func decodeEventAllianceRankingEntry(raw json.RawMessage) (State.EventRankingEntry, error) {
	var row []json.RawMessage
	if err := json.Unmarshal(raw, &row); err != nil {
		return State.EventRankingEntry{}, err
	}
	if len(row) < 3 {
		return State.EventRankingEntry{}, fmt.Errorf("expected rank, score, and alliance details")
	}
	var rank wireInt64
	var score wireInt64
	var alliance []json.RawMessage
	if err := json.Unmarshal(row[0], &rank); err != nil {
		return State.EventRankingEntry{}, fmt.Errorf("rank: %w", err)
	}
	if err := json.Unmarshal(row[1], &score); err != nil {
		return State.EventRankingEntry{}, fmt.Errorf("score: %w", err)
	}
	if err := json.Unmarshal(row[2], &alliance); err != nil {
		return State.EventRankingEntry{}, fmt.Errorf("alliance: %w", err)
	}
	if len(alliance) < 4 {
		return State.EventRankingEntry{}, fmt.Errorf("expected alliance id, name, members, and fame")
	}
	var allianceID wireInt64
	var name string
	var memberCount wireInt64
	var famePoints wireInt64
	if err := json.Unmarshal(alliance[0], &allianceID); err != nil {
		return State.EventRankingEntry{}, fmt.Errorf("alliance id: %w", err)
	}
	if err := json.Unmarshal(alliance[1], &name); err != nil {
		return State.EventRankingEntry{}, fmt.Errorf("alliance name: %w", err)
	}
	if err := json.Unmarshal(alliance[2], &memberCount); err != nil {
		return State.EventRankingEntry{}, fmt.Errorf("member count: %w", err)
	}
	if err := json.Unmarshal(alliance[3], &famePoints); err != nil {
		return State.EventRankingEntry{}, fmt.Errorf("fame points: %w", err)
	}
	return State.EventRankingEntry{
		Alliance: name, AllianceID: State.AllianceID(allianceID), MemberCount: int64(memberCount),
		FamePoints: int64(famePoints), Rank: int64(rank), Score: int64(score),
	}, nil
}
