package Ingest

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"slices"
	"strconv"
	"strings"
	"time"

	"CitadelDesktop/Server/GameData"
	"CitadelDesktop/Server/Protocol"
	"CitadelDesktop/Server/State"
)

type eventPointScore struct {
	Points      wireInt64 `json:"OP"`
	Rank        wireInt64 `json:"OR"`
	LeagueID    wireInt64 `json:"LID"`
	RewardSetID wireInt64 `json:"RSID"`
}

func (score *eventPointScore) UnmarshalJSON(raw []byte) error {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || raw[0] != '{' {
		*score = eventPointScore{}
		return nil
	}
	type scoreAlias eventPointScore
	var decoded scoreAlias
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return err
	}
	*score = eventPointScore(decoded)
	return nil
}

type khanRageSnapshot struct {
	CampID          wireInt64 `json:"ACID"`
	AllianceRage    wireInt64 `json:"AR"`
	PlayerRage      wireInt64 `json:"PCRP"`
	PlayerTotalRage wireInt64 `json:"PTRP"`
	Valid           bool      `json:"-"`
}

func (snapshot *khanRageSnapshot) UnmarshalJSON(raw []byte) error {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || raw[0] != '{' {
		*snapshot = khanRageSnapshot{}
		return nil
	}
	type snapshotAlias khanRageSnapshot
	var decoded snapshotAlias
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return err
	}
	*snapshot = khanRageSnapshot(decoded)
	snapshot.Valid = true
	return nil
}

type scalableEventSnapshot struct {
	EventID           wireInt64         `json:"EID"`
	RemainingSec      wireInt64         `json:"RS"`
	DifficultyID      wireInt64         `json:"EDID"`
	AutoScaling       wireInt64         `json:"EASE"`
	PlayerProgress    eventPointScore   `json:"SP"`
	AllianceProgress  eventPointScore   `json:"A"`
	PackageIDs        string            `json:"PIDS"`
	Packages          json.RawMessage   `json:"PID"`
	AdvisorCurrency   wireInt64         `json:"ACI"`
	AdvisorActive     wireInt64         `json:"AAA"`
	AdvisorFree       wireInt64         `json:"AAF"`
	FortifyCurrencies []string          `json:"RCKS"`
	AllianceCamp      *khanRageSnapshot `json:"AC"`
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
	return []string{"events", "event-scores", "khan"}, changed, err
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
	fortifyCurrencies := []string{}
	for _, event := range payload.Events {
		if int64(event.AutoScaling) != 1 || int64(event.EventID) <= 0 {
			continue
		}
		eventID := int64(event.EventID)
		if eventID == 72 && event.AllianceCamp != nil && event.AllianceCamp.Valid {
			if applyKhanRageSnapshot(gameState, gameData, *event.AllianceCamp, observedAt) {
				changed = true
			}
		}
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
			LeagueID:           int64(event.PlayerProgress.LeagueID),
			AllianceLeagueID:   int64(event.AllianceProgress.LeagueID),
			RewardSetID:        int64(event.PlayerProgress.RewardSetID),
			AdvisorActive:      int64(event.AdvisorActive) == 1,
			AdvisorCurrencyID:  State.CurrencyID(event.AdvisorCurrency),
			AdvisorFree:        int64(event.AdvisorFree) == 1,
			ObservedAt:         observedAt.UTC(),
		}
		if score.LocalizationKey == "" {
			score.LocalizationKey = fmt.Sprintf("event_title_%d", eventID)
		}
		rewards := gameData.ScalableEventRewardProgress(
			eventID, difficultyID, score.LeagueID, score.RewardSetID, score.PlayerScore,
		)
		score.RewardPagesReached = rewards.Reached
		score.RewardPagesTotal = rewards.Total
		score.NextRewardScore = rewards.NextScore
		if previous, found := gameState.EventScores.ByEvent[eventID]; !found || previous != score {
			gameState.EventScores.ByEvent[eventID] = score
			changed = true
		}
		State.EnsureEventActivity(gameState, eventID, observedAt)
		if activeEventID == 0 {
			activeEventID = eventID
			fortifyCurrencies = normalizedInvasionFortifyCurrencies(event.FortifyCurrencies)
		}
	}
	if gameState.EventScores.ActiveEventID != activeEventID {
		gameState.EventScores.ActiveEventID = activeEventID
		changed = true
	}
	if !slices.Equal(gameState.Invasion.FortifyCurrencies, fortifyCurrencies) {
		gameState.Invasion.FortifyCurrencies = fortifyCurrencies
		changed = true
	}
	return changed, nil
}

func applyKhanRageSnapshot(
	gameState *State.GameState,
	gameData *GameData.Store,
	snapshot khanRageSnapshot,
	observedAt time.Time,
) bool {
	campID := int64(snapshot.CampID)
	definition, found := gameData.EventCamp(campID)
	rageCap := int64(0)
	if found && definition.EventID == 72 && definition.AreaTypeID == 35 {
		rageCap = definition.PlayerRageCap
	}
	playerRage := max(int64(0), int64(snapshot.PlayerRage))
	totalRage := max(int64(0), int64(snapshot.PlayerTotalRage))
	observedAt = observedAt.UTC()
	if gameState.Khan.RageCampID == campID &&
		gameState.Khan.PlayerRage == playerRage &&
		gameState.Khan.PlayerRageCap == rageCap &&
		gameState.Khan.PlayerTotalRage == totalRage &&
		!gameState.Khan.RageObservedAt.IsZero() {
		return false
	}
	gameState.Khan.RageCampID = campID
	gameState.Khan.PlayerRage = playerRage
	gameState.Khan.PlayerRageCap = rageCap
	gameState.Khan.PlayerTotalRage = totalRage
	gameState.Khan.RageObservedAt = observedAt
	return true
}

func reduceKhanRagePoints(
	_ context.Context,
	frame Protocol.Frame,
	gameState *State.GameState,
	gameData *GameData.Store,
) ([]string, bool, error) {
	if !frameSucceeded(frame) || len(frame.Payload) == 0 {
		return nil, false, nil
	}
	var payload struct {
		EventID         wireInt64 `json:"EID"`
		PlayerRage      wireInt64 `json:"PCRP"`
		PlayerTotalRage wireInt64 `json:"PTRP"`
	}
	if err := json.Unmarshal(frame.Payload, &payload); err != nil {
		return nil, false, fmt.Errorf("decode Khan rage update: %w", err)
	}
	if int64(payload.EventID) != 72 {
		return nil, false, nil
	}
	rageCap := gameState.Khan.PlayerRageCap
	campID := gameState.Khan.RageCampID
	if definition, found := gameData.EventCamp(campID); !found ||
		definition.EventID != 72 || definition.AreaTypeID != 35 {
		rageCap = 0
	} else {
		rageCap = definition.PlayerRageCap
	}
	playerRage := max(int64(0), int64(payload.PlayerRage))
	totalRage := max(int64(0), int64(payload.PlayerTotalRage))
	if gameState.Khan.PlayerRage == playerRage &&
		gameState.Khan.PlayerRageCap == rageCap &&
		gameState.Khan.PlayerTotalRage == totalRage &&
		!gameState.Khan.RageObservedAt.IsZero() {
		return nil, false, nil
	}
	gameState.Khan.PlayerRage = playerRage
	gameState.Khan.PlayerRageCap = rageCap
	gameState.Khan.PlayerTotalRage = totalRage
	gameState.Khan.RageObservedAt = frame.ReceivedAt.UTC()
	return []string{"khan"}, true, nil
}

func normalizedInvasionFortifyCurrencies(values []string) []string {
	result := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		value = strings.ToUpper(strings.TrimSpace(value))
		if value == "" {
			continue
		}
		if _, duplicate := seen[value]; duplicate {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}

func eventShopPackageIDs(event scalableEventSnapshot) []int64 {
	packageIDs := eventPackageWireIDs(event.Packages)
	result := make([]int64, 0, len(packageIDs)+8)
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
	for _, packageID := range packageIDs {
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

func eventPackageWireIDs(raw json.RawMessage) []wireInt64 {
	var packageIDs []wireInt64
	if err := json.Unmarshal(raw, &packageIDs); err == nil {
		return packageIDs
	}
	var packageID wireInt64
	if err := json.Unmarshal(raw, &packageID); err == nil && packageID > 0 {
		return []wireInt64{packageID}
	}
	return nil
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
	rewards := gameData.ScalableEventRewardProgress(
		eventID, score.DifficultyID, score.LeagueID, score.RewardSetID, score.PlayerScore,
	)
	score.RewardPagesReached = rewards.Reached
	score.RewardPagesTotal = rewards.Total
	score.NextRewardScore = rewards.NextScore
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
	if gameState.EventScores.ActivityByEvent == nil {
		gameState.EventScores.ActivityByEvent = map[int64]State.EventActivityState{}
	}
	if gameState.EventScores.RankingByEvent == nil {
		gameState.EventScores.RankingByEvent = map[int64]State.EventRankingState{}
	}
}
