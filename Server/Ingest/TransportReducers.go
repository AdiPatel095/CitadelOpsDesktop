package Ingest

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"math"
	"math/big"
	"reflect"
	"strconv"
	"strings"
	"time"

	"CitadelDesktop/Server/GameData"
	"CitadelDesktop/Server/Protocol"
	"CitadelDesktop/Server/State"
)

const caravanOverloaderBoosterID = 11

const permanentBoosterRemainingSec = math.MaxInt32

func reduceMarketInfo(
	_ context.Context,
	frame Protocol.Frame,
	gameState *State.GameState,
	gameData *GameData.Store,
) ([]string, bool, error) {
	if !frameSucceeded(frame) || len(frame.Payload) == 0 {
		return nil, false, nil
	}
	var root map[string]json.RawMessage
	if err := json.Unmarshal(frame.Payload, &root); err != nil {
		return nil, false, fmt.Errorf("decode market info: %w", err)
	}
	if nested := root["cmi"]; len(nested) > 0 {
		if err := json.Unmarshal(nested, &root); err != nil {
			return nil, false, fmt.Errorf("decode nested market info: %w", err)
		}
	}
	var rows []map[string]json.RawMessage
	if err := json.Unmarshal(root["C"], &rows); err != nil {
		return nil, false, fmt.Errorf("decode market castle rows: %w", err)
	}
	next := make(map[State.CastleID]State.MarketCastleState, len(rows))
	castleResourcesChanged := false
	for _, row := range rows {
		castleIDValue, ok := rawInt64(row["CID"])
		if !ok || castleIDValue <= 0 {
			continue
		}
		castleID := State.CastleID(castleIDValue)
		marketCastle := State.MarketCastleState{
			CastleID:         castleID,
			KingdomID:        State.KingdomID(rawInteger(row["KID"])),
			TotalBarrows:     int(rawInteger(row["TC"])),
			AvailableBarrows: int(rawInteger(row["AC"])),
			Resources:        map[State.ResourceID]float64{},
			AreaEffects:      []State.MarketAreaEffect{},
		}
		for jsonKey, rawValue := range row {
			definitionID, found := officialDefinitionID(gameData, "resources", "resourceID", jsonKey)
			amount, number := rawFloat64(rawValue)
			if !found || !number {
				continue
			}
			resourceID := State.ResourceID(definitionID)
			marketCastle.Resources[resourceID] = amount
			if castle, exists := gameState.MutableCastleParts(castleID, State.CastlePartResources); exists {
				ensureCastleMaps(&castle)
				balance := castle.Resources[resourceID]
				if balance.Amount != amount {
					balance.Amount = amount
					castle.Resources[resourceID] = balance
					gameState.SetCastleParts(castleID, castle, State.CastlePartResources)
					castleResourcesChanged = true
				}
			}
		}
		var effects [][]json.RawMessage
		_ = json.Unmarshal(row["AE"], &effects)
		for _, effect := range effects {
			if len(effect) < 2 {
				continue
			}
			parsed := State.MarketAreaEffect{EffectID: rowInt(effect, 0), Values: []float64{}}
			var values []json.RawMessage
			if json.Unmarshal(effect[1], &values) == nil {
				for _, rawValue := range values {
					if value, exists := rawFloat64(rawValue); exists {
						parsed.Values = append(parsed.Values, value)
					}
				}
			}
			if len(effect) > 2 {
				_ = json.Unmarshal(effect[2], &parsed.Source)
			}
			if parsed.EffectID > 0 {
				marketCastle.AreaEffects = append(marketCastle.AreaEffects, parsed)
			}
		}
		next[castleID] = marketCastle
	}
	marketChanged := !reflect.DeepEqual(gameState.Market.Castles, next) || !gameState.Market.ObservedAt.Equal(frame.ReceivedAt)
	if marketChanged {
		gameState.Market.Castles = next
		gameState.Market.ObservedAt = frame.ReceivedAt
	}
	return []string{"market", "castles", "resources"}, marketChanged || castleResourcesChanged, nil
}

func reduceMarketBooster(
	_ context.Context,
	frame Protocol.Frame,
	gameState *State.GameState,
	_ *GameData.Store,
) ([]string, bool, error) {
	if frame.ResponseCode != nil && *frame.ResponseCode != 0 {
		market := &gameState.Market
		if market.SpecialistPurchasePending && !frame.ReceivedAt.Before(market.SpecialistPurchasePendingSince) &&
			pendingSpecialistResponseMatches(*market, frame) && specialistResponseOpcodeMatches(market.SpecialistPurchaseExpectedID, frame.Opcode) {
			market.LatestSpecialistPurchase.Outcome = "rejected"
			market.LatestSpecialistPurchase.UpdatedAt = frame.ReceivedAt
			market.LatestSpecialistPurchase.Detail = fmt.Sprintf("The game rejected the specialist purchase with response code %d", *frame.ResponseCode)
			clearPendingSpecialistResponse(market)
			return []string{"boosters", "market"}, true, nil
		}
		return nil, false, nil
	}
	if frame.ResponseCode == nil || len(frame.Payload) == 0 {
		return nil, false, nil
	}
	var outerRoot map[string]json.RawMessage
	var root map[string]json.RawMessage
	if err := json.Unmarshal(frame.Payload, &root); err != nil {
		return nil, false, fmt.Errorf("decode market boosters: %w", err)
	}
	outerRoot = root
	if nested := root["boi"]; len(nested) > 0 {
		if err := json.Unmarshal(nested, &root); err != nil {
			return nil, false, fmt.Errorf("decode nested market boosters: %w", err)
		}
	}
	rawRows, present := root["BO"]
	if !present || len(rawRows) == 0 || rawJSONNull(rawRows) {
		return nil, false, nil
	}
	var rows []map[string]json.RawMessage
	if err := json.Unmarshal(rawRows, &rows); err != nil || rows == nil {
		return nil, false, fmt.Errorf("decode market booster rows: %w", err)
	}
	if frame.ReceivedAt.IsZero() || frame.ReceivedAt.After(time.Now().UTC().Add(5*time.Second)) ||
		(!gameState.Market.BoostersObservedAt.IsZero() && frame.ReceivedAt.Before(gameState.Market.BoostersObservedAt)) {
		return nil, false, nil
	}
	level := 0
	boosters := make(map[int]State.MarketBoosterState, len(rows))
	for _, row := range rows {
		rawID, hasID := row["ID"]
		rawRemaining, hasRemaining := row["RT"]
		idValue, idValid := rawJSONInt64(rawID)
		remainingValue, remainingValid := rawJSONInt64(rawRemaining)
		if !hasID || !hasRemaining || !idValid || !remainingValid ||
			idValue < 0 || remainingValue < 0 || remainingValue > int64(math.MaxInt64)/int64(time.Second) {
			return nil, false, fmt.Errorf("decode market booster rows: invalid ID or RT")
		}
		id := int(idValue)
		if _, duplicate := boosters[id]; duplicate {
			return nil, false, fmt.Errorf("decode market booster rows: duplicate ID %d", id)
		}
		var levelValue int
		var bonusValue, purchaseCountValue int64
		if raw := row["L"]; len(raw) > 0 {
			parsed, valid := rawJSONInt64(raw)
			levelValue = int(parsed)
			if !valid || int64(levelValue) != parsed {
				return nil, false, fmt.Errorf("decode market booster rows: invalid L for ID %d", id)
			}
		}
		if raw := row["B"]; len(raw) > 0 {
			var valid bool
			bonusValue, valid = rawJSONInt64(raw)
			if !valid {
				return nil, false, fmt.Errorf("decode market booster rows: invalid B for ID %d", id)
			}
		}
		if raw := row["PC"]; len(raw) > 0 {
			var valid bool
			purchaseCountValue, valid = rawJSONInt64(raw)
			if !valid || purchaseCountValue < 0 {
				return nil, false, fmt.Errorf("decode market booster rows: invalid PC for ID %d", id)
			}
		}
		remainingSec := int64(remainingValue)
		booster := State.MarketBoosterState{
			ID: id, Level: levelValue, BonusPercent: int(bonusValue), RemainingSec: remainingSec,
			ContinuousPurchaseCount: int(purchaseCountValue),
		}
		if remainingSec == permanentBoosterRemainingSec {
			booster.Permanent = true
		} else if remainingSec > 0 {
			booster.ExpiresAt = frame.ReceivedAt.Add(time.Duration(remainingSec) * time.Second)
		}
		boosters[id] = booster
		if idValue == int64(caravanOverloaderBoosterID) {
			level = levelValue
		}
	}
	feast := gameState.Market.Feast
	feastPresent := false
	if rawFeast, ok := root["bfs"]; ok && !rawJSONNull(rawFeast) {
		var err error
		feast, err = marketFeastFromRaw(rawFeast, frame.ReceivedAt)
		if err != nil {
			return nil, false, fmt.Errorf("decode market feast from boosters: %w", err)
		}
		feastPresent = true
	}
	pendingChanged := feastPresent && reconcilePendingFeastSnapshot(gameState, feast, frame)
	pendingChanged = reconcilePendingSpecialistResponse(gameState, boosters, outerRoot, frame) || pendingChanged
	if gameState.Market.CaravanLevelLoaded && gameState.Market.CaravanLevel == level &&
		reflect.DeepEqual(gameState.Market.Boosters, boosters) &&
		reflect.DeepEqual(gameState.Market.Feast, feast) &&
		gameState.Market.BoostersObservedAt.Equal(frame.ReceivedAt) && !pendingChanged {
		return nil, false, nil
	}
	gameState.Market.CaravanLevel = level
	gameState.Market.CaravanLevelLoaded = true
	gameState.Market.Boosters = boosters
	gameState.Market.Feast = feast
	gameState.Market.BoostersObservedAt = frame.ReceivedAt
	gameState.Market.BoostersObservedGeneration = gameState.Session.ConnectionGeneration
	return []string{"boosters", "market"}, true, nil
}

func reconcilePendingSpecialistResponse(
	gameState *State.GameState,
	boosters map[int]State.MarketBoosterState,
	root map[string]json.RawMessage,
	frame Protocol.Frame,
) bool {
	market := &gameState.Market
	if !market.SpecialistPurchasePending || frame.ResponseCode == nil || *frame.ResponseCode != 0 ||
		frame.ReceivedAt.Before(market.SpecialistPurchasePendingSince) ||
		!pendingSpecialistResponseMatches(*market, frame) {
		return false
	}
	if !specialistResponseOpcodeMatches(market.SpecialistPurchaseExpectedID, frame.Opcode) {
		return false
	}
	booster, present := boosters[market.SpecialistPurchaseExpectedID]
	baseline := market.SpecialistPurchasePreviousExpiry
	if baseline.Before(market.SpecialistPurchasePendingSince) {
		baseline = market.SpecialistPurchasePendingSince
	}
	if !present || booster.Permanent || booster.ExpiresAt.Before(baseline.Add(7*24*time.Hour-time.Second)) {
		return false
	}
	rawGCU, present := root["gcu"]
	if !present || rawJSONNull(rawGCU) {
		return false
	}
	var resources map[string]json.RawMessage
	if json.Unmarshal(rawGCU, &resources) != nil || resources == nil {
		return false
	}
	ruby, valid := rawJSONInt64(resources["C2"])
	if !valid || ruby < 0 {
		return false
	}
	observation := gameState.Player.ResourceObservations[market.SpecialistPurchaseRubyResourceID]
	if market.SpecialistPurchaseRubyResourceID <= 0 || !observation.ObservedAt.Equal(frame.ReceivedAt) ||
		observation.ConnectionGeneration == 0 || observation.ConnectionGeneration != gameState.Session.ConnectionGeneration {
		return false
	}
	market.SpecialistPurchaseResponseConfirmedAt = frame.ReceivedAt
	market.SpecialistPurchaseResponseExpiresAt = booster.ExpiresAt
	market.SpecialistPurchaseResponseRuby = ruby
	market.SpecialistPurchaseResponseRubyAt = observation.ObservedAt
	market.LatestSpecialistPurchase.Outcome = "verifying"
	market.LatestSpecialistPurchase.UpdatedAt = frame.ReceivedAt
	market.LatestSpecialistPurchase.TimerAfter = booster.ExpiresAt
	market.LatestSpecialistPurchase.TimerAfterObservedAt = frame.ReceivedAt
	market.LatestSpecialistPurchase.RubyAfter = ruby
	market.LatestSpecialistPurchase.RubyAfterKnown = true
	market.LatestSpecialistPurchase.RubyAfterObservedAt = observation.ObservedAt
	market.LatestSpecialistPurchase.Detail = "Correlated purchase response contained an increased timer and command-local ruby balance; waiting for a read-only timer refresh"
	return true
}

func pendingSpecialistResponseMatches(market State.MarketState, frame Protocol.Frame) bool {
	return market.SpecialistPurchaseResponseToken != "" && frame.ResponseToken == market.SpecialistPurchaseResponseToken ||
		market.SpecialistPurchaseOperationID != "" && frame.CausationOperationID == market.SpecialistPurchaseOperationID
}

func specialistResponseOpcodeMatches(specialistID int, opcode string) bool {
	expected := "ovs"
	if specialistID == 6 {
		expected = "bms"
	} else if specialistID == 8 {
		expected = "btx"
	} else if specialistID == 10 {
		expected = "bis"
	}
	return strings.EqualFold(opcode, expected)
}

func clearPendingSpecialistResponse(market *State.MarketState) {
	market.SpecialistPurchasePending = false
	market.SpecialistPurchaseOperationID = ""
	market.SpecialistPurchaseResponseToken = ""
	market.SpecialistPurchaseResponseConfirmedAt = time.Time{}
	market.SpecialistPurchaseResponseExpiresAt = time.Time{}
	market.SpecialistPurchaseRubyResourceID = 0
	market.SpecialistPurchaseResponseRuby = 0
	market.SpecialistPurchaseResponseRubyAt = time.Time{}
}

func reduceMarketFeast(
	_ context.Context,
	frame Protocol.Frame,
	gameState *State.GameState,
	_ *GameData.Store,
) ([]string, bool, error) {
	if frame.ResponseCode == nil {
		return nil, false, fmt.Errorf("decode purchased feast: response result code is missing")
	}
	if *frame.ResponseCode != 0 {
		if gameState.Market.FeastPurchasePending &&
			!frame.ReceivedAt.Before(gameState.Market.FeastPurchasePendingSince) &&
			pendingFeastResponseMatches(gameState.Market, frame) {
			gameState.Market.LatestFeastPurchase.Outcome = "rejected"
			gameState.Market.LatestFeastPurchase.UpdatedAt = frame.ReceivedAt
			gameState.Market.LatestFeastPurchase.Detail = fmt.Sprintf("The game rejected the feast purchase with response code %d", *frame.ResponseCode)
			clearPendingFeastPurchase(&gameState.Market)
			return []string{"boosters", "market"}, true, nil
		}
		return nil, false, nil
	}
	feast, err := marketFeastFromRaw(frame.Payload, frame.ReceivedAt)
	if err != nil {
		return nil, false, fmt.Errorf("decode purchased feast: %w", err)
	}
	if !feast.ActiveAt(frame.ReceivedAt) {
		return nil, false, fmt.Errorf("decode purchased feast: successful response did not contain an active feast")
	}
	if gameState.Market.FeastPurchasePending {
		if !pendingFeastResponseMatches(gameState.Market, frame) ||
			feast.ID != gameState.Market.FeastPurchaseExpectedID ||
			frame.ReceivedAt.Before(gameState.Market.FeastPurchasePendingSince) ||
			!pendingFeastTimerIncreased(gameState.Market, feast.ExpiresAt) {
			return nil, false, fmt.Errorf("decode purchased feast: correlated response did not confirm an increased expected feast timer")
		}
		gameState.Market.Feast = feast
		gameState.Market.FeastLastPurchaseAt = frame.ReceivedAt
		gameState.Market.FeastPurchaseResponseConfirmedAt = frame.ReceivedAt
		gameState.Market.FeastPurchaseResponseExpiresAt = feast.ExpiresAt
		gameState.Market.LatestFeastPurchase.Outcome = "verifying"
		gameState.Market.LatestFeastPurchase.UpdatedAt = frame.ReceivedAt
		gameState.Market.LatestFeastPurchase.ConfirmedRemainingSec = feast.RemainingSec
		gameState.Market.LatestFeastPurchase.ConfirmedExpiresAt = feast.ExpiresAt
		gameState.Market.LatestFeastPurchase.Detail = "Correlated purchase response received; waiting for an authoritative timer refresh"
		return []string{"boosters", "market"}, true, nil
	}
	lastPurchaseAt := frame.ReceivedAt
	if reflect.DeepEqual(gameState.Market.Feast, feast) &&
		gameState.Market.FeastLastPurchaseAt.Equal(lastPurchaseAt) {
		return nil, false, nil
	}
	gameState.Market.Feast = feast
	gameState.Market.FeastLastPurchaseAt = lastPurchaseAt
	return []string{"boosters", "market"}, true, nil
}

func reconcilePendingFeastSnapshot(
	gameState *State.GameState,
	feast State.MarketFeastState,
	frame Protocol.Frame,
) bool {
	observedAt := frame.ReceivedAt
	market := &gameState.Market
	if !market.FeastPurchasePending || market.FeastPurchasePendingSince.IsZero() ||
		observedAt.Before(market.FeastPurchasePendingSince) {
		return false
	}
	if !market.FeastPurchaseResponseConfirmedAt.IsZero() &&
		!observedAt.Before(market.FeastPurchaseResponseConfirmedAt) &&
		feast.ActiveAt(observedAt) && feast.ID == market.FeastPurchaseExpectedID &&
		pendingFeastTimerIncreased(*market, feast.ExpiresAt) &&
		!feast.ExpiresAt.Before(market.FeastPurchaseResponseExpiresAt.Add(-5*time.Second)) {
		market.LatestFeastPurchase.Outcome = "confirmed"
		market.LatestFeastPurchase.UpdatedAt = observedAt
		market.LatestFeastPurchase.ActivationConfirmed = true
		market.LatestFeastPurchase.ConfirmedRemainingSec = feast.RemainingSec
		market.LatestFeastPurchase.ConfirmedExpiresAt = feast.ExpiresAt
		market.LatestFeastPurchase.ActivationConfirmedAt = observedAt
		market.LatestFeastPurchase.Detail = "Activation confirmed by the correlated purchase response and refreshed feast timer"
		clearPendingFeastPurchase(market)
		return true
	}
	// Never infer failure from a timeout, an omitted BFS, or one early reply.
	// Two distinct matched BOI responses in the same live connection must
	// explicitly report inactivity, at least 30 seconds apart and after the
	// dispatch settling period. Active/conflicting evidence starts over.
	if feast.ActiveAt(observedAt) {
		changed := !market.FeastPurchaseInactiveObservedAt.IsZero()
		market.FeastPurchaseInactiveObservedAt = time.Time{}
		market.FeastPurchaseInactiveResponseToken = ""
		market.FeastPurchaseInactiveGeneration = 0
		return changed
	}
	generation := gameState.Session.ConnectionGeneration
	if generation > 0 && frame.ResponseToken != "" &&
		!observedAt.Before(gameState.Session.ChangedAt) &&
		!observedAt.Before(market.FeastPurchasePendingSince.Add(30*time.Second)) {
		first := market.FeastPurchaseInactiveObservedAt
		if first.IsZero() || market.FeastPurchaseInactiveGeneration != generation || first.Before(gameState.Session.ChangedAt) {
			market.FeastPurchaseInactiveObservedAt = observedAt
			market.FeastPurchaseInactiveResponseToken = frame.ResponseToken
			market.FeastPurchaseInactiveGeneration = generation
			return true
		}
		if frame.ResponseToken != market.FeastPurchaseInactiveResponseToken && !observedAt.Before(first.Add(30*time.Second)) {
			market.LatestFeastPurchase.Outcome = "not-confirmed"
			market.LatestFeastPurchase.UpdatedAt = observedAt
			market.LatestFeastPurchase.Detail = "Two authoritative reconciliation checks reported no active feast"
			clearPendingFeastPurchase(market)
			return true
		}
	}
	if feast.ActiveAt(observedAt) || market.FeastPurchaseExpectedExpiresAt.IsZero() ||
		observedAt.Before(market.FeastPurchaseExpectedExpiresAt) {
		return false
	}
	market.LatestFeastPurchase.Outcome = "not-confirmed"
	market.LatestFeastPurchase.UpdatedAt = observedAt
	market.LatestFeastPurchase.Detail = "The bounded reconciliation window ended without a confirmed activation"
	clearPendingFeastPurchase(market)
	return true
}

func pendingFeastTimerIncreased(market State.MarketState, expiresAt time.Time) bool {
	baseline := market.FeastPurchasePreviousExpiresAt
	if baseline.IsZero() {
		baseline = market.FeastPurchasePendingSince
	}
	return State.FeastTimerProgressed(baseline, expiresAt)
}

func pendingFeastResponseMatches(market State.MarketState, frame Protocol.Frame) bool {
	return market.FeastPurchaseResponseToken != "" && frame.ResponseToken == market.FeastPurchaseResponseToken ||
		market.FeastPurchaseOperationID != "" && frame.CausationOperationID == market.FeastPurchaseOperationID
}

func clearPendingFeastPurchase(market *State.MarketState) {
	market.FeastPurchasePending = false
	market.FeastPurchaseExpectedID = 0
	market.FeastPurchasePendingSince = time.Time{}
	market.FeastPurchaseExpectedExpiresAt = time.Time{}
	market.FeastPurchasePreviousExpiresAt = time.Time{}
	market.FeastPurchaseOperationID = ""
	market.FeastPurchaseResponseToken = ""
	market.FeastPurchaseResponseConfirmedAt = time.Time{}
	market.FeastPurchaseResponseExpiresAt = time.Time{}
	market.FeastPurchaseInactiveObservedAt = time.Time{}
	market.FeastPurchaseInactiveResponseToken = ""
	market.FeastPurchaseInactiveGeneration = 0
}

func marketFeastFromRaw(raw json.RawMessage, observedAt time.Time) (State.MarketFeastState, error) {
	if observedAt.IsZero() {
		return State.MarketFeastState{}, fmt.Errorf("observation timestamp is required")
	}
	if rawJSONNull(raw) {
		return State.MarketFeastState{}, fmt.Errorf("payload must be an object")
	}
	var values map[string]json.RawMessage
	if err := json.Unmarshal(raw, &values); err != nil {
		return State.MarketFeastState{}, err
	}
	if values == nil {
		return State.MarketFeastState{}, fmt.Errorf("payload must be an object")
	}
	if nested, found := values["bfs"]; found {
		if rawJSONNull(nested) {
			return State.MarketFeastState{}, fmt.Errorf("nested bfs payload must be an object")
		}
		if err := json.Unmarshal(nested, &values); err != nil {
			return State.MarketFeastState{}, fmt.Errorf("decode nested bfs payload: %w", err)
		}
		if values == nil {
			return State.MarketFeastState{}, fmt.Errorf("nested bfs payload must be an object")
		}
	}
	id, err := marketFeastInteger(values["T"], "T")
	if err != nil {
		return State.MarketFeastState{}, err
	}
	remainingSec, err := marketFeastInteger(values["RT"], "RT")
	if err != nil {
		return State.MarketFeastState{}, err
	}
	if id == -1 && remainingSec == 0 {
		return State.MarketFeastState{ObservedAt: observedAt}, nil
	}
	if id < 0 || remainingSec <= 0 {
		return State.MarketFeastState{}, fmt.Errorf("T and RT are incoherent: want T=-1/RT=0 or T>=0/RT>0")
	}
	if remainingSec > int64(math.MaxInt) || remainingSec > int64(math.MaxInt64)/int64(time.Second) {
		return State.MarketFeastState{}, fmt.Errorf("RT is outside the supported duration range")
	}
	duration := time.Duration(remainingSec) * time.Second
	expiresAt := observedAt.Add(duration)
	if !expiresAt.After(observedAt) {
		return State.MarketFeastState{}, fmt.Errorf("RT does not produce a future expiry")
	}
	return State.MarketFeastState{
		ID: id, RemainingSec: int(remainingSec), ExpiresAt: expiresAt, ObservedAt: observedAt,
	}, nil
}

func marketFeastInteger(raw json.RawMessage, field string) (int64, error) {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || bytes.Equal(raw, []byte("null")) {
		return 0, fmt.Errorf("%s is required", field)
	}
	text := string(raw)
	if raw[0] == '"' {
		if err := json.Unmarshal(raw, &text); err != nil {
			return 0, fmt.Errorf("%s must be an integer", field)
		}
		text = strings.TrimSpace(text)
	}
	if _, err := strconv.ParseFloat(text, 64); err != nil {
		return 0, fmt.Errorf("%s must be an integer", field)
	}
	rational, ok := new(big.Rat).SetString(text)
	if !ok || !rational.IsInt() || !rational.Num().IsInt64() {
		return 0, fmt.Errorf("%s must be an integer", field)
	}
	return rational.Num().Int64(), nil
}

func rawJSONNull(raw json.RawMessage) bool {
	raw = bytes.TrimSpace(raw)
	return len(raw) == 0 || bytes.Equal(raw, []byte("null"))
}

func reduceFeastCostReduction(
	_ context.Context,
	frame Protocol.Frame,
	gameState *State.GameState,
	_ *GameData.Store,
) ([]string, bool, error) {
	if !frameSucceeded(frame) {
		return nil, false, nil
	}
	if len(frame.Payload) == 0 {
		return nil, false, fmt.Errorf("decode feast cost reduction: response payload is empty")
	}
	var root map[string]json.RawMessage
	if err := json.Unmarshal(frame.Payload, &root); err != nil {
		return nil, false, fmt.Errorf("decode feast cost reduction: %w", err)
	}
	if nested := root["fce"]; len(nested) > 0 {
		if err := json.Unmarshal(nested, &root); err != nil {
			return nil, false, fmt.Errorf("decode nested feast cost reduction: %w", err)
		}
	}
	rawPercent, found := root["FRM"]
	percent, number := rawFloat64(rawPercent)
	if !found || !number || math.IsNaN(percent) || math.IsInf(percent, 0) || percent != math.Trunc(percent) {
		return nil, false, fmt.Errorf("decode feast cost reduction: FRM must be an integer percentage")
	}
	if percent < 0 || percent > 100 {
		return nil, false, fmt.Errorf("decode feast cost reduction: FRM percentage %.0f is outside 0..100", percent)
	}
	reduction := int(percent)
	if gameState.Market.FeastCostReductionPercent == reduction &&
		gameState.Market.FeastCostReductionObservedAt.Equal(frame.ReceivedAt) {
		return nil, false, nil
	}
	gameState.Market.FeastCostReductionPercent = reduction
	gameState.Market.FeastCostReductionObservedAt = frame.ReceivedAt
	return []string{"market"}, true, nil
}

func reduceKingdomTransport(
	_ context.Context,
	frame Protocol.Frame,
	gameState *State.GameState,
	gameData *GameData.Store,
) ([]string, bool, error) {
	if !frameSucceeded(frame) || len(frame.Payload) == 0 {
		return nil, false, nil
	}
	var root map[string]json.RawMessage
	if err := json.Unmarshal(frame.Payload, &root); err != nil {
		return nil, false, fmt.Errorf("decode kingdom transport: %w", err)
	}
	if nested := root["kpi"]; len(nested) > 0 {
		if err := json.Unmarshal(nested, &root); err != nil {
			return nil, false, fmt.Errorf("decode nested kingdom transport: %w", err)
		}
	}
	rawUnlocks, hasUnlocks := root["UL"]
	rawResources, hasResources := root["RT"]
	rawUnits, hasUnits := root["UT"]
	if !hasUnlocks && !hasResources && !hasUnits {
		return nil, false, nil
	}
	if frame.ReceivedAt.IsZero() || frame.ReceivedAt.After(time.Now().UTC().Add(5*time.Second)) ||
		(!gameState.KingdomTransport.ObservedAt.IsZero() && frame.ReceivedAt.Before(gameState.KingdomTransport.ObservedAt)) {
		return nil, false, nil
	}
	// UL is present on a validated full KPI snapshot even when no transport is
	// active. KUT acknowledgements can instead carry only UT. Missing RT/UT on
	// a full snapshot is authoritative empty; missing fields on a partial reply
	// preserve the last observation.
	fullSnapshot := hasUnlocks
	next := cloneKingdomTransportState(gameState.KingdomTransport)
	if fullSnapshot {
		next.Unlocks = map[State.KingdomID]State.KingdomTransportUnlock{}
		next.Pending = []State.KingdomResourceTransport{}
		next.PendingUnits = []State.KingdomUnitTransport{}
	}
	next.ObservedAt = frame.ReceivedAt
	if next.ResourceWorkflows == nil {
		next.ResourceWorkflows = map[State.KingdomID]State.KingdomResourceTransportWorkflow{}
	}
	if next.TroopWorkflows == nil {
		next.TroopWorkflows = map[State.KingdomID]State.KingdomTroopTransportWorkflow{}
	}
	for kingdomID, workflow := range gameState.KingdomTransport.ResourceWorkflows {
		workflow.Goods = append([]State.KingdomTransportGood(nil), workflow.Goods...)
		next.ResourceWorkflows[kingdomID] = workflow
	}
	if hasUnlocks {
		var unlocks []map[string]json.RawMessage
		if rawJSONNull(rawUnlocks) || json.Unmarshal(rawUnlocks, &unlocks) != nil {
			return nil, false, fmt.Errorf("decode kingdom transport UL: expected an array")
		}
		for _, row := range unlocks {
			kingdomID := State.KingdomID(rawInteger(row["KID"]))
			if kingdomID < 0 {
				continue
			}
			next.Unlocks[kingdomID] = State.KingdomTransportUnlock{
				KingdomID: kingdomID, Unlocked: rawInteger(row["U"]) != 0,
				Created: rawInteger(row["C"]) != 0, Stage: int(rawInteger(row["SL"])),
			}
		}
	}
	if hasResources {
		var pending []map[string]json.RawMessage
		if rawJSONNull(rawResources) || json.Unmarshal(rawResources, &pending) != nil {
			return nil, false, fmt.Errorf("decode kingdom transport RT: expected an array")
		}
		next.Pending = []State.KingdomResourceTransport{}
		for _, row := range pending {
			transport := State.KingdomResourceTransport{
				KingdomID:    State.KingdomID(rawInteger(row["KID"])),
				RemainingSec: int(rawInteger(row["RS"])), Goods: []State.KingdomTransportGood{},
			}
			var goods [][]json.RawMessage
			if raw := row["G"]; len(raw) > 0 && !rawJSONNull(raw) && json.Unmarshal(raw, &goods) != nil {
				return nil, false, fmt.Errorf("decode kingdom transport RT goods: expected an array")
			}
			for _, good := range goods {
				jsonKey := rowString(good, 0)
				definitionID, found := officialDefinitionID(gameData, "resources", "resourceID", jsonKey)
				amount, exists := rawFloat64(rawAt(good, 1))
				if !found || !exists || amount <= 0 {
					continue
				}
				transport.Goods = append(transport.Goods, State.KingdomTransportGood{
					ResourceID: State.ResourceID(definitionID), Amount: amount,
				})
			}
			next.Pending = append(next.Pending, transport)
		}
	}
	if hasUnits {
		var pendingUnits []map[string]json.RawMessage
		if rawJSONNull(rawUnits) || json.Unmarshal(rawUnits, &pendingUnits) != nil {
			return nil, false, fmt.Errorf("decode kingdom transport UT: expected an array")
		}
		next.PendingUnits = []State.KingdomUnitTransport{}
		for _, row := range pendingUnits {
			transport := State.KingdomUnitTransport{
				KingdomID:    State.KingdomID(rawInteger(row["KID"])),
				RemainingSec: int(rawInteger(row["RS"])), Units: []State.KingdomTransportUnit{},
			}
			var units [][]json.RawMessage
			if raw := row["I"]; len(raw) == 0 || rawJSONNull(raw) || json.Unmarshal(raw, &units) != nil {
				return nil, false, fmt.Errorf("decode kingdom transport UT units: expected an array")
			}
			for _, unit := range units {
				unitID, amount := State.UnitID(rowInt(unit, 0)), rowInt(unit, 1)
				if unitID <= 0 || amount <= 0 {
					continue
				}
				transport.Units = append(transport.Units, State.KingdomTransportUnit{UnitID: unitID, Amount: amount})
			}
			next.PendingUnits = append(next.PendingUnits, transport)
		}
	}
	if fullSnapshot || hasUnits {
		reconcileTroopTransportWorkflows(&next, frame.ReceivedAt, gameState.Session.ConnectionGeneration)
	}
	if reflect.DeepEqual(gameState.KingdomTransport, next) {
		return nil, false, nil
	}
	gameState.KingdomTransport = next
	return []string{"kingdom-transport"}, true, nil
}

func cloneKingdomTransportState(source State.KingdomTransportState) State.KingdomTransportState {
	clone := source
	clone.Unlocks = make(map[State.KingdomID]State.KingdomTransportUnlock, len(source.Unlocks))
	for key, value := range source.Unlocks {
		clone.Unlocks[key] = value
	}
	clone.Pending = append([]State.KingdomResourceTransport(nil), source.Pending...)
	for index := range clone.Pending {
		clone.Pending[index].Goods = append([]State.KingdomTransportGood(nil), source.Pending[index].Goods...)
	}
	clone.PendingUnits = append([]State.KingdomUnitTransport(nil), source.PendingUnits...)
	for index := range clone.PendingUnits {
		clone.PendingUnits[index].Units = append([]State.KingdomTransportUnit(nil), source.PendingUnits[index].Units...)
	}
	clone.ResourceWorkflows = make(map[State.KingdomID]State.KingdomResourceTransportWorkflow, len(source.ResourceWorkflows))
	for key, value := range source.ResourceWorkflows {
		value.Goods = append([]State.KingdomTransportGood(nil), value.Goods...)
		clone.ResourceWorkflows[key] = value
	}
	clone.TroopWorkflows = make(map[State.KingdomID]State.KingdomTroopTransportWorkflow, len(source.TroopWorkflows))
	for key, value := range source.TroopWorkflows {
		value.Units = append([]State.KingdomTransportUnit(nil), value.Units...)
		clone.TroopWorkflows[key] = value
	}
	return clone
}

func reconcileTroopTransportWorkflows(state *State.KingdomTransportState, observedAt time.Time, connectionGeneration uint64) {
	for kingdomID, workflow := range state.TroopWorkflows {
		if !workflow.ArmedAt.IsZero() && observedAt.Before(workflow.ArmedAt) {
			continue
		}
		pending := false
		remaining := 0
		for _, transport := range state.PendingUnits {
			if transport.KingdomID == kingdomID && kingdomTransportUnitsEqual(transport.Units, workflow.Units) {
				if workflow.SkipRequestedAt.IsZero() && (workflow.Status == "awaiting_destination_refresh" || workflow.Status == "ownership_uncertain") ||
					!kingdomTransportTimerContinuous(workflow, transport.RemainingSec, observedAt) {
					continue
				}
				pending = true
				remaining = transport.RemainingSec
				break
			}
		}
		workflow.TransportObservedAt = observedAt
		if workflow.Status == "ownership_uncertain" {
			state.TroopWorkflows[kingdomID] = workflow
			continue
		}
		if !workflow.SkipRequestedAt.IsZero() {
			workflow.RemainingSec = remaining
			state.TroopWorkflows[kingdomID] = workflow
			continue
		}
		if pending {
			if workflow.Status == "armed" && (workflow.SessionGeneration == 0 || workflow.SessionGeneration != connectionGeneration) {
				workflow.Status = "ownership_uncertain"
				state.TroopWorkflows[kingdomID] = workflow
				continue
			}
			workflow.Status = "pending"
			workflow.RemainingSec = remaining
			if workflow.LaunchedAt.IsZero() {
				workflow.LaunchedAt = observedAt
			}
		} else {
			workflow.Status = "awaiting_destination_refresh"
		}
		state.TroopWorkflows[kingdomID] = workflow
	}
}

func kingdomTransportTimerContinuous(workflow State.KingdomTroopTransportWorkflow, remainingSec int, observedAt time.Time) bool {
	if workflow.RemainingSec <= 0 {
		return true
	}
	maximum := workflow.RemainingSec
	if !workflow.TransportObservedAt.IsZero() && observedAt.After(workflow.TransportObservedAt) {
		elapsed := int(observedAt.Sub(workflow.TransportObservedAt) / time.Second)
		maximum = max(0, maximum-elapsed+5)
	}
	return remainingSec <= maximum
}

func kingdomTransportUnitsEqual(observed, expected []State.KingdomTransportUnit) bool {
	if len(observed) != len(expected) || len(expected) == 0 {
		return false
	}
	amounts := make(map[State.UnitID]int64, len(observed))
	for _, unit := range observed {
		amounts[unit.UnitID] += unit.Amount
	}
	for _, unit := range expected {
		if unit.Amount <= 0 || amounts[unit.UnitID] != unit.Amount {
			return false
		}
	}
	return len(amounts) == len(expected)
}

func reduceSubscriptions(
	_ context.Context,
	frame Protocol.Frame,
	gameState *State.GameState,
	_ *GameData.Store,
) ([]string, bool, error) {
	if !frameSucceeded(frame) || len(frame.Payload) == 0 {
		return nil, false, nil
	}
	var payload map[string]json.RawMessage
	if err := json.Unmarshal(frame.Payload, &payload); err != nil {
		return nil, false, fmt.Errorf("decode subscriptions: %w", err)
	}
	rawSubscriptions, exists := payload["SP"]
	if !exists {
		return nil, false, nil
	}
	var subscriptions []struct {
		TypeID         int `json:"STID"`
		RemainingSec   int `json:"RS"`
		GracePeriodSec int `json:"RSGP"`
	}
	if err := json.Unmarshal(rawSubscriptions, &subscriptions); err != nil {
		return nil, false, fmt.Errorf("decode subscriptions: %w", err)
	}
	next := make(map[int]State.SubscriptionState, len(subscriptions))
	for _, subscription := range subscriptions {
		if subscription.TypeID <= 0 {
			continue
		}
		next[subscription.TypeID] = State.SubscriptionState{
			TypeID: subscription.TypeID, RemainingSec: subscription.RemainingSec,
			GracePeriodSec: subscription.GracePeriodSec,
		}
	}
	if reflect.DeepEqual(gameState.Subscriptions, next) {
		return nil, false, nil
	}
	gameState.Subscriptions = next
	return []string{"subscriptions"}, true, nil
}

func rawAt(row []json.RawMessage, index int) json.RawMessage {
	if index < 0 || index >= len(row) {
		return nil
	}
	return row[index]
}
