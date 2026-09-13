package Ingest

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
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
	EventID           wireInt64           `json:"EID"`
	RemainingSec      wireInt64           `json:"RS"`
	DifficultyID      wireInt64           `json:"EDID"`
	AutoScaling       wireInt64           `json:"EASE"`
	PlayerProgress    eventPointScore     `json:"SP"`
	AllianceProgress  eventPointScore     `json:"A"`
	PackageIDs        string              `json:"PIDS"`
	Packages          json.RawMessage     `json:"PID"`
	AdvisorCurrency   wireInt64           `json:"ACI"`
	AdvisorActive     wireInt64           `json:"AAA"`
	AdvisorFree       wireInt64           `json:"AAF"`
	FortifyCurrencies []string            `json:"RCKS"`
	AllianceCamp      *khanRageSnapshot   `json:"AC"`
	GlobalEffects     [][]json.RawMessage `json:"GE"`
	GlobalBoosters    []struct {
		GlobalEffectID wireInt64 `json:"GEID"`
		RubyCost       wireInt64 `json:"C2"`
		BonusValue     wireInt64 `json:"BV"`
	} `json:"GEB"`
}

const (
	globalEffectsEventID        = 610
	globalEffectBoostersEventID = 612
)

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
	return []string{"events", "event-scores", "global-effects", "khan"}, changed, err
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
	changed := false
	activeByEvent := make(map[int64]State.EventAvailability, len(payload.Events))
	globalEffects := map[int64]State.GlobalEffectAvailability{}
	globalEffectBoosterOffers := map[int64]State.GlobalEffectBoosterOffer{}
	for _, event := range payload.Events {
		eventID := int64(event.EventID)
		remainingSec := int64(event.RemainingSec)
		if eventID <= 0 || remainingSec <= 0 {
			continue
		}
		activeByEvent[eventID] = State.EventAvailability{
			EventID: eventID,
			EndsAt:  observedAt.Add(time.Duration(remainingSec) * time.Second).UTC().Truncate(time.Minute),
		}
		switch eventID {
		case globalEffectsEventID:
			for _, row := range event.GlobalEffects {
				globalEffectID, effectRemainingSec, strength := rowInt(row, 0), rowInt(row, 1), rowInt(row, 2)
				if globalEffectID <= 0 || effectRemainingSec <= 0 {
					continue
				}
				globalEffects[globalEffectID] = State.GlobalEffectAvailability{
					GlobalEffectID: globalEffectID, Strength: strength,
					EndsAt: observedAt.Add(time.Duration(effectRemainingSec) * time.Second).UTC().Truncate(time.Minute),
				}
			}
		case globalEffectBoostersEventID:
			for _, offer := range event.GlobalBoosters {
				globalEffectID := int64(offer.GlobalEffectID)
				if globalEffectID <= 0 || int64(offer.RubyCost) <= 0 || int64(offer.BonusValue) <= 0 {
					continue
				}
				globalEffectBoosterOffers[globalEffectID] = State.GlobalEffectBoosterOffer{
					GlobalEffectID: globalEffectID, RubyCost: int64(offer.RubyCost), BonusValue: int64(offer.BonusValue),
				}
			}
		}
	}
	previousInventory := gameState.EventScores.Inventory
	if gameState.ReplaceEventInventory(State.EventInventoryState{
		ObservedAt: observedAt, ActiveByEvent: activeByEvent,
		GlobalEffectsObservedAt: observedAt.UTC(), GlobalEffects: globalEffects,
		GlobalEffectBoosterOffers:    globalEffectBoosterOffers,
		GlobalEffectBoostsObservedAt: previousInventory.GlobalEffectBoostsObservedAt,
		GlobalEffectBoosts:           previousInventory.GlobalEffectBoosts,
	}) {
		changed = true
	}
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
	if gameState.ReplaceEventShopRoutes(shopByPackage) {
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
		if previous, found := gameState.LookupScalableEventScore(eventID); !found || previous != score {
			gameState.SetScalableEventScore(eventID, score)
			changed = true
		}
		activity, activityAvailable := State.EnsureEventActivity(gameState, eventID, observedAt)
		if activeEventID == 0 {
			activeEventID = eventID
			offered := normalizedInvasionFortifyCurrencies(event.FortifyCurrencies)
			if activityAvailable {
				if len(activity.FortifyCurrencies) == 0 && len(offered) > 0 {
					activity.FortifyCurrencies = append([]string(nil), offered...)
					activity.FortifyCurrenciesObservedAt = observedAt.UTC()
					gameState.SetEventActivity(eventID, activity)
					changed = true
				}
				fortifyCurrencies = append([]string(nil), activity.FortifyCurrencies...)
			} else {
				fortifyCurrencies = offered
			}
		}
	}
	if gameState.SetActiveEventID(activeEventID) {
		changed = true
	}
	if !slices.Equal(gameState.Invasion.FortifyCurrencies, fortifyCurrencies) {
		gameState.Invasion.FortifyCurrencies = append([]string(nil), fortifyCurrencies...)
		changed = true
	}
	return changed, nil
}

func reduceGlobalEffectBoosterInfo(
	_ context.Context,
	frame Protocol.Frame,
	gameState *State.GameState,
	_ *GameData.Store,
) ([]string, bool, error) {
	if !frameSucceeded(frame) || len(frame.Payload) == 0 {
		return nil, false, nil
	}
	var root map[string]json.RawMessage
	if err := json.Unmarshal(frame.Payload, &root); err != nil {
		return nil, false, fmt.Errorf("decode global-effect booster status: %w", err)
	}
	if nested := root["bie"]; len(nested) > 0 {
		if err := json.Unmarshal(nested, &root); err != nil {
			return nil, false, fmt.Errorf("decode nested global-effect booster status: %w", err)
		}
	}
	var boostedIDs []wireInt64
	if err := json.Unmarshal(root["GE"], &boostedIDs); err != nil {
		return nil, false, fmt.Errorf("decode boosted global-effect ids: %w", err)
	}
	boosted := make(map[int64]struct{}, len(boostedIDs))
	for _, id := range boostedIDs {
		if id > 0 {
			boosted[int64(id)] = struct{}{}
		}
	}
	statuses := make(map[int64]State.GlobalEffectBoostState, len(gameState.EventScores.Inventory.GlobalEffects))
	for globalEffectID, effect := range gameState.EventScores.Inventory.GlobalEffects {
		if !effect.ActiveAt(frame.ReceivedAt) {
			continue
		}
		_, active := boosted[globalEffectID]
		statuses[globalEffectID] = State.GlobalEffectBoostState{
			GlobalEffectID: globalEffectID, Boosted: active,
			OccurrenceEndsAt: effect.EndsAt, ObservedAt: frame.ReceivedAt.UTC(),
		}
	}
	inventory := gameState.EventScores.Inventory
	inventory.GlobalEffectBoostsObservedAt = frame.ReceivedAt.UTC()
	inventory.GlobalEffectBoosts = statuses
	changed := gameState.ReplaceEventInventory(inventory)
	return []string{"events", "event-scores", "global-effects"}, changed, nil
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
	score, tracked := gameState.LookupScalableEventScore(eventID)
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
	previous, _ := gameState.LookupScalableEventScore(eventID)
	gameState.SetScalableEventScore(eventID, score)
	changed := !tracked || previous != score
	if scalable && gameState.EventScores.ActiveEventID == 0 {
		gameState.SetActiveEventID(eventID)
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
