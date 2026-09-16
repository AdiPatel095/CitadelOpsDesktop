package Ingest

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"math"
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
	previousInventory := gameState.EventScores.Inventory
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
				endsAt := observedAt.Add(time.Duration(effectRemainingSec) * time.Second).UTC().Truncate(time.Minute)
				if previous, found := previousInventory.GlobalEffects[globalEffectID]; found &&
					State.SameEventOccurrence(previous.EndsAt, endsAt) {
					endsAt = previous.EndsAt
				}
				globalEffects[globalEffectID] = State.GlobalEffectAvailability{
					GlobalEffectID: globalEffectID, Strength: strength,
					EndsAt: endsAt,
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
	if gameState.ReplaceEventInventory(State.EventInventoryState{
		ObservedAt: observedAt, ActiveByEvent: activeByEvent,
		GlobalEffectsObservedAt: observedAt.UTC(), GlobalEffects: globalEffects,
		GlobalEffectBoosterOffers:      globalEffectBoosterOffers,
		GlobalEffectBoostsObservedAt:   previousInventory.GlobalEffectBoostsObservedAt,
		GlobalEffectBoosts:             previousInventory.GlobalEffectBoosts,
		GlobalEffectReadObservedAt:     previousInventory.GlobalEffectReadObservedAt,
		GlobalEffectReadGeneration:     previousInventory.GlobalEffectReadGeneration,
		GlobalEffectBaselineObservedAt: previousInventory.GlobalEffectBaselineObservedAt,
		GlobalEffectBaselineGeneration: previousInventory.GlobalEffectBaselineGeneration,
		GlobalEffectPurchases:          previousInventory.GlobalEffectPurchases,
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
	gameData *GameData.Store,
) ([]string, bool, error) {
	if frame.ResponseCode == nil || *frame.ResponseCode != 0 || len(frame.Payload) == 0 {
		return nil, false, nil
	}
	if !gameState.EventScores.Inventory.GlobalEffectsObservedAt.IsZero() &&
		frame.ReceivedAt.Before(gameState.EventScores.Inventory.GlobalEffectsObservedAt) {
		return nil, false, nil
	}
	boosted, err := decodeGlobalEffectBoosterIDs(frame.Payload)
	if err != nil {
		return nil, false, err
	}
	if !gameState.EventScores.Inventory.GlobalEffectBoostsObservedAt.IsZero() &&
		frame.ReceivedAt.Before(gameState.EventScores.Inventory.GlobalEffectBoostsObservedAt) {
		return nil, false, nil
	}
	statuses := make(map[int64]State.GlobalEffectBoostState, len(gameState.EventScores.Inventory.GlobalEffects))
	for globalEffectID, effect := range gameState.EventScores.Inventory.GlobalEffects {
		if !effect.ActiveAt(frame.ReceivedAt) {
			continue
		}
		_, active := boosted[globalEffectID]
		if previous, found := gameState.EventScores.Inventory.GlobalEffectBoosts[globalEffectID]; found &&
			previous.Boosted && State.SameEventOccurrence(previous.OccurrenceEndsAt, effect.EndsAt) {
			active = true
		}
		statuses[globalEffectID] = State.GlobalEffectBoostState{
			GlobalEffectID: globalEffectID, Boosted: active,
			OccurrenceEndsAt: effect.EndsAt, ObservedAt: frame.ReceivedAt.UTC(),
			ConnectionGeneration: gameState.Session.ConnectionGeneration,
		}
	}
	inventory := gameState.EventScores.Inventory
	inventory.GlobalEffectPurchases = cloneGlobalEffectPurchaseRecords(inventory.GlobalEffectPurchases)
	inventory.GlobalEffectBoostsObservedAt = frame.ReceivedAt.UTC()
	inventory.GlobalEffectBoosts = statuses
	purchaseEvidence := false
	for globalEffectID, status := range statuses {
		if globalEffectID != GameData.FortressDailyGlobalEffectID || !status.Boosted {
			continue
		}
		_, explicitlyBoosted := boosted[globalEffectID]
		previousStatus := gameState.EventScores.Inventory.GlobalEffectBoosts[globalEffectID]
		record, found := inventory.GlobalEffectPurchases[globalEffectID]
		if !found || !State.SameEventOccurrence(record.OccurrenceEndsAt, status.OccurrenceEndsAt) {
			record = State.GlobalEffectPurchaseRecord{
				GlobalEffectID: globalEffectID, OccurrenceEndsAt: status.OccurrenceEndsAt,
				ExpiresAt: status.OccurrenceEndsAt, RequestOpcode: "agb",
			}
		}
		record.Outcome = State.GlobalEffectPurchaseConfirmed
		if explicitlyBoosted {
			record.ActivationObservedAt = frame.ReceivedAt.UTC()
		} else if record.ActivationObservedAt.IsZero() && previousStatus.Boosted {
			record.ActivationObservedAt = previousStatus.ObservedAt
		}
		record.DebitUnverified = !updateGlobalEffectPurchaseRubyEvidence(&record, gameState, gameData)
		if explicitlyBoosted {
			record.Detail = "The server confirmed that the daily fortress-speed boost is active"
		} else if record.Detail == "" {
			record.Detail = "An earlier server confirmation remains authoritative for this daily fortress-speed occurrence"
		}
		if inventory.GlobalEffectPurchases == nil {
			inventory.GlobalEffectPurchases = map[int64]State.GlobalEffectPurchaseRecord{}
		}
		inventory.GlobalEffectPurchases[globalEffectID] = record
		purchaseEvidence = true
	}
	changed := gameState.ReplaceEventInventory(inventory)
	domains := []string{"events", "event-scores", "global-effects"}
	if purchaseEvidence {
		domains = append(domains, globalEffectPurchaseDurabilityDomain)
	}
	return domains, changed, nil
}

func decodeGlobalEffectBoosterIDs(raw json.RawMessage) (map[int64]struct{}, error) {
	var root map[string]json.RawMessage
	if err := json.Unmarshal(raw, &root); err != nil {
		return nil, fmt.Errorf("decode global-effect booster status: %w", err)
	}
	if nested := root["bie"]; len(nested) > 0 {
		if err := json.Unmarshal(nested, &root); err != nil {
			return nil, fmt.Errorf("decode nested global-effect booster status: %w", err)
		}
	}
	rawIDs, present := root["GE"]
	trimmed := bytes.TrimSpace(rawIDs)
	if !present || len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) || trimmed[0] != '[' {
		return nil, fmt.Errorf("decode boosted global-effect ids: GE must be an explicit array")
	}
	var boostedIDs []json.RawMessage
	if err := json.Unmarshal(trimmed, &boostedIDs); err != nil {
		return nil, fmt.Errorf("decode boosted global-effect ids: %w", err)
	}
	boosted := make(map[int64]struct{}, len(boostedIDs))
	for _, rawID := range boostedIDs {
		id, valid := rawJSONInt64(rawID)
		if !valid || id <= 0 {
			return nil, fmt.Errorf("decode boosted global-effect ids: GE contains a non-positive integer")
		}
		boosted[id] = struct{}{}
	}
	return boosted, nil
}

func reduceGlobalEffectPurchaseAcknowledgement(
	_ context.Context,
	frame Protocol.Frame,
	gameState *State.GameState,
	gameData *GameData.Store,
) ([]string, bool, error) {
	if gameState == nil || frame.ResponseCode == nil {
		return nil, false, nil
	}
	inventory := gameState.EventScores.Inventory
	inventory.GlobalEffectPurchases = cloneGlobalEffectPurchaseRecords(inventory.GlobalEffectPurchases)
	record, found := inventory.GlobalEffectPurchases[GameData.FortressDailyGlobalEffectID]
	if !found || !globalEffectPurchaseResponseMatches(record, frame) {
		return nil, false, nil
	}
	code := *frame.ResponseCode
	record.ResultCode = &code
	record.ResultObservedAt = frame.ReceivedAt.UTC()
	verifiedDebit := updateGlobalEffectPurchaseRubyEvidence(&record, gameState, gameData)
	record.DebitUnverified = !verifiedDebit
	if code == 0 {
		if record.Outcome != State.GlobalEffectPurchaseConfirmed {
			record.Outcome = State.GlobalEffectPurchaseAccepted
			record.Detail = "The game accepted the boost purchase; awaiting current boosted-state confirmation"
		}
	} else {
		record.Outcome = State.GlobalEffectPurchaseRejected
		record.Detail = fmt.Sprintf("The game rejected the boost purchase with result code %d", code)
	}
	inventory.GlobalEffectPurchases[record.GlobalEffectID] = record
	changed := gameState.ReplaceEventInventory(inventory)
	return []string{"events", "event-scores", "global-effects", globalEffectPurchaseDurabilityDomain}, changed, nil
}

func cloneGlobalEffectPurchaseRecords(input map[int64]State.GlobalEffectPurchaseRecord) map[int64]State.GlobalEffectPurchaseRecord {
	result := make(map[int64]State.GlobalEffectPurchaseRecord, len(input))
	for id, record := range input {
		if record.ResultCode != nil {
			code := *record.ResultCode
			record.ResultCode = &code
		}
		result[id] = record
	}
	return result
}

func globalEffectPurchaseResponseMatches(record State.GlobalEffectPurchaseRecord, frame Protocol.Frame) bool {
	if record.ResponseToken != "" && frame.ResponseToken == record.ResponseToken {
		return true
	}
	return record.OperationID != "" && frame.CausationOperationID == record.OperationID
}

func updateGlobalEffectPurchaseRubyEvidence(record *State.GlobalEffectPurchaseRecord, gameState *State.GameState, gameData *GameData.Store) bool {
	if record == nil || gameState == nil || gameData == nil || record.RubyBeforeObservedAt.IsZero() {
		return false
	}
	resourceID, found := gameData.ResourceIDForJSONKey("C2")
	if !found || resourceID <= 0 {
		return false
	}
	observation := gameState.Player.ResourceObservations[State.ResourceID(resourceID)]
	value, found := gameState.Player.Resources[State.ResourceID(resourceID)]
	minimumObservedAt := record.RubyBeforeObservedAt
	if record.DispatchedAt.After(minimumObservedAt) {
		minimumObservedAt = record.DispatchedAt
	}
	if !found || observation.ObservedAt.IsZero() || !observation.ObservedAt.After(minimumObservedAt) ||
		observation.ConnectionGeneration != record.ConnectionGeneration {
		return false
	}
	record.RubyAfter = int64(math.Floor(value))
	record.RubyAfterKnown = true
	record.RubyAfterObservedAt = observation.ObservedAt
	record.ObservedRubyChange = record.RubyBefore - record.RubyAfter
	// A matching aggregate balance change is useful evidence but cannot prove
	// this command caused the debit; manual spending can occur between reads.
	return false
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
