package Ingest

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"strconv"
	"strings"
	"time"

	"CitadelDesktop/Server/GameData"
	"CitadelDesktop/Server/Protocol"
	"CitadelDesktop/Server/State"
)

type eventPointScore struct {
	Points wireInt64 `json:"OP"`
	Rank   wireInt64 `json:"OR"`
}

type scalableEventSnapshot struct {
	EventID          wireInt64       `json:"EID"`
	RemainingSec     wireInt64       `json:"RS"`
	DifficultyID     wireInt64       `json:"EDID"`
	AutoScaling      wireInt64       `json:"EASE"`
	PlayerProgress   eventPointScore `json:"SP"`
	AllianceProgress eventPointScore `json:"A"`
	PackageIDs       string          `json:"PIDS"`
	Packages         []wireInt64     `json:"PID"`
}

func reduceScalableEventSnapshot(
	_ context.Context,
	frame Protocol.Frame,
	gameState *State.GameState,
	gameData *GameData.Store,
) ([]string, bool, error) {
	if !frameSucceeded(frame) || len(frame.Payload) == 0 {
		return nil, false, nil
	}
	changed, err := applyScalableEventSnapshot(frame.Payload, frame.ReceivedAt, gameState, gameData)
	return []string{"events", "event-scores"}, changed, err
}

func applyScalableEventSnapshot(
	raw json.RawMessage,
	observedAt time.Time,
	gameState *State.GameState,
	gameData *GameData.Store,
) (bool, error) {
	var payload struct {
		Events []scalableEventSnapshot `json:"E"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return false, fmt.Errorf("decode event snapshot: %w", err)
	}
	ensureEventScoreMap(gameState)
	changed := false
	shopByPackage := make(map[State.PackageID]State.EventShopRoute)
	for _, event := range payload.Events {
		eventID := int64(event.EventID)
		remainingSec := int64(event.RemainingSec)
		if eventID <= 0 || remainingSec <= 0 {
			continue
		}
		route := State.EventShopRoute{EventID: eventID, RemainingSec: remainingSec, ObservedAt: observedAt.UTC()}
		for _, packageID := range eventShopPackageIDs(event) {
			shopByPackage[State.PackageID(packageID)] = route
		}
	}
	if !maps.Equal(gameState.EventScores.ShopByPackage, shopByPackage) {
		gameState.EventScores.ShopByPackage = shopByPackage
		changed = true
	}
	activeEventID := int64(0)
	for _, event := range payload.Events {
		if int64(event.AutoScaling) != 1 || int64(event.EventID) <= 0 {
			continue
		}
		eventID := int64(event.EventID)
		difficultyID := int64(event.DifficultyID)
		definition, _ := gameData.ScalableEvent(eventID, difficultyID)
		score := State.ScalableEventScore{
			EventID:            eventID,
			EventType:          definition.EventType,
			Name:               definition.Name,
			LocalizationKey:    definition.LocalizationKey,
			DifficultyID:       difficultyID,
			DifficultyTypeID:   definition.DifficultyTypeID,
			DifficultyTypeName: definition.DifficultyTypeName,
			PlayerScore:        int64(event.PlayerProgress.Points),
			AllianceScore:      int64(event.AllianceProgress.Points),
			PlayerRank:         int64(event.PlayerProgress.Rank),
			AllianceRank:       int64(event.AllianceProgress.Rank),
			RemainingSec:       int64(event.RemainingSec),
			ObservedAt:         observedAt.UTC(),
		}
		if score.LocalizationKey == "" {
			score.LocalizationKey = fmt.Sprintf("event_title_%d", eventID)
		}
		if previous, found := gameState.EventScores.ByEvent[eventID]; !found || previous != score {
			gameState.EventScores.ByEvent[eventID] = score
			changed = true
		}
		if activeEventID == 0 {
			activeEventID = eventID
		}
	}
	if gameState.EventScores.ActiveEventID != activeEventID {
		gameState.EventScores.ActiveEventID = activeEventID
		changed = true
	}
	return changed, nil
}

func eventShopPackageIDs(event scalableEventSnapshot) []int64 {
	result := make([]int64, 0, len(event.Packages)+8)
	seen := map[int64]struct{}{}
	appendID := func(packageID int64) {
		if packageID <= 0 {
			return
		}
		if _, exists := seen[packageID]; exists {
			return
		}
		seen[packageID] = struct{}{}
		result = append(result, packageID)
	}
	for _, packageID := range event.Packages {
		appendID(int64(packageID))
	}
	for _, raw := range strings.Split(event.PackageIDs, ",") {
		packageID, err := strconv.ParseInt(strings.TrimSpace(raw), 10, 64)
		if err == nil {
			appendID(packageID)
		}
	}
	return result
}

func reduceEventPoints(
	_ context.Context,
	frame Protocol.Frame,
	gameState *State.GameState,
	gameData *GameData.Store,
) ([]string, bool, error) {
	if !frameSucceeded(frame) || len(frame.Payload) == 0 {
		return nil, false, nil
	}
	var payload struct {
		EventID wireInt64   `json:"EID"`
		Points  []wireInt64 `json:"OP"`
		Ranks   []wireInt64 `json:"OR"`
	}
	if err := json.Unmarshal(frame.Payload, &payload); err != nil {
		return nil, false, fmt.Errorf("decode event points: %w", err)
	}
	eventID := int64(payload.EventID)
	if eventID <= 0 {
		return nil, false, nil
	}
	ensureEventScoreMap(gameState)
	score, tracked := gameState.EventScores.ByEvent[eventID]
	definition, scalable := gameData.ScalableEvent(eventID, score.DifficultyID)
	if !tracked && !scalable {
		return nil, false, nil
	}
	if !tracked {
		score = State.ScalableEventScore{EventID: eventID}
	}
	if scalable {
		score.EventType = definition.EventType
		score.Name = definition.Name
		score.LocalizationKey = definition.LocalizationKey
		score.DifficultyTypeID = definition.DifficultyTypeID
		score.DifficultyTypeName = definition.DifficultyTypeName
	}
	if score.LocalizationKey == "" {
		score.LocalizationKey = fmt.Sprintf("event_title_%d", eventID)
	}
	if value, found := eventPointValue(payload.Points, 0); found {
		score.PlayerScore = value
	}
	if value, found := eventPointValue(payload.Points, 1); found {
		score.AllianceScore = value
	}
	if value, found := eventPointValue(payload.Ranks, 0); found {
		score.PlayerRank = value
	}
	if value, found := eventPointValue(payload.Ranks, 1); found {
		score.AllianceRank = value
	}
	if score.RemainingSec > 0 && !score.ObservedAt.IsZero() && frame.ReceivedAt.After(score.ObservedAt) {
		score.RemainingSec = max(0, score.RemainingSec-int64(frame.ReceivedAt.Sub(score.ObservedAt).Seconds()))
	}
	score.ObservedAt = frame.ReceivedAt.UTC()
	previous := gameState.EventScores.ByEvent[eventID]
	gameState.EventScores.ByEvent[eventID] = score
	changed := !tracked || previous != score
	if scalable && gameState.EventScores.ActiveEventID == 0 {
		gameState.EventScores.ActiveEventID = eventID
		changed = true
	}
	return []string{"events", "event-scores"}, changed, nil
}

func eventPointValue(values []wireInt64, index int) (int64, bool) {
	if index < 0 || index >= len(values) {
		return 0, false
	}
	return int64(values[index]), true
}

func ensureEventScoreMap(gameState *State.GameState) {
	if gameState.EventScores.ByEvent == nil {
		gameState.EventScores.ByEvent = map[int64]State.ScalableEventScore{}
	}
	if gameState.EventScores.ShopByPackage == nil {
		gameState.EventScores.ShopByPackage = map[State.PackageID]State.EventShopRoute{}
	}
}
