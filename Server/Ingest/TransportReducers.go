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
	if !frameSucceeded(frame) || len(frame.Payload) == 0 {
		return nil, false, nil
	}
	var root map[string]json.RawMessage
	if err := json.Unmarshal(frame.Payload, &root); err != nil {
		return nil, false, fmt.Errorf("decode market boosters: %w", err)
	}
	if nested := root["boi"]; len(nested) > 0 {
		if err := json.Unmarshal(nested, &root); err != nil {
			return nil, false, fmt.Errorf("decode nested market boosters: %w", err)
		}
	}
	var rows []struct {
		ID                      wireInt64 `json:"ID"`
		Level                   int       `json:"L"`
		Bonus                   wireInt64 `json:"B"`
		RemainingSec            wireInt64 `json:"RT"`
		ContinuousPurchaseCount wireInt64 `json:"PC"`
	}
	if err := json.Unmarshal(root["BO"], &rows); err != nil {
		return nil, false, fmt.Errorf("decode market booster rows: %w", err)
	}
	level := 0
	boosters := make(map[int]State.MarketBoosterState, len(rows))
	for _, row := range rows {
		id := int(row.ID)
		if id < 0 {
			continue
		}
		remainingSec := int(row.RemainingSec)
		booster := State.MarketBoosterState{
			ID: id, Level: row.Level, BonusPercent: int(row.Bonus), RemainingSec: remainingSec,
			ContinuousPurchaseCount: int(row.ContinuousPurchaseCount),
		}
		if remainingSec == permanentBoosterRemainingSec {
			booster.Permanent = true
		} else if remainingSec > 0 {
			booster.ExpiresAt = frame.ReceivedAt.Add(time.Duration(remainingSec) * time.Second)
		}
		boosters[id] = booster
		if row.ID == caravanOverloaderBoosterID {
			level = row.Level
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
	pendingChanged := feastPresent && reconcilePendingFeastSnapshot(gameState, feast, frame.ReceivedAt)
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
	return []string{"boosters", "market"}, true, nil
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
	if gameState.Market.FeastPurchasePending &&
		(feast.ID != gameState.Market.FeastPurchaseExpectedID ||
			frame.ReceivedAt.Before(gameState.Market.FeastPurchasePendingSince) ||
			!pendingFeastExpiryConfirmed(gameState.Market, feast.ExpiresAt)) {
		return nil, false, fmt.Errorf("decode purchased feast: response did not confirm the expected feast")
	}
	lastPurchaseAt := frame.ReceivedAt
	pendingChanged := gameState.Market.FeastPurchasePending
	clearPendingFeastPurchase(&gameState.Market)
	if reflect.DeepEqual(gameState.Market.Feast, feast) &&
		gameState.Market.FeastLastPurchaseAt.Equal(lastPurchaseAt) && !pendingChanged {
		return nil, false, nil
	}
	gameState.Market.Feast = feast
	gameState.Market.FeastLastPurchaseAt = lastPurchaseAt
	return []string{"boosters", "market"}, true, nil
}

func reconcilePendingFeastSnapshot(
	gameState *State.GameState,
	feast State.MarketFeastState,
	observedAt time.Time,
) bool {
	market := &gameState.Market
	if !market.FeastPurchasePending || market.FeastPurchasePendingSince.IsZero() ||
		observedAt.Before(market.FeastPurchasePendingSince) {
		return false
	}
	if feast.ActiveAt(observedAt) && feast.ID == market.FeastPurchaseExpectedID &&
		pendingFeastExpiryConfirmed(*market, feast.ExpiresAt) {
		if market.FeastLastPurchaseAt.Before(market.FeastPurchasePendingSince) {
			market.FeastLastPurchaseAt = market.FeastPurchasePendingSince
		}
		clearPendingFeastPurchase(market)
		return true
	}
	if feast.ActiveAt(observedAt) || market.FeastPurchaseExpectedExpiresAt.IsZero() ||
		observedAt.Before(market.FeastPurchaseExpectedExpiresAt) {
		return false
	}
	clearPendingFeastPurchase(market)
	return true
}

func pendingFeastExpiryConfirmed(market State.MarketState, expiresAt time.Time) bool {
	return !market.FeastPurchaseExpectedExpiresAt.IsZero() &&
		!expiresAt.Before(market.FeastPurchaseExpectedExpiresAt.Add(-time.Minute))
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
	market.FeastPurchaseOperationID = ""
	market.FeastPurchaseResponseToken = ""
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
	if len(root["UL"]) == 0 && len(root["RT"]) == 0 && len(root["UT"]) == 0 {
		return nil, false, nil
	}
	next := State.KingdomTransportState{
		Unlocks: map[State.KingdomID]State.KingdomTransportUnlock{},
		Pending: []State.KingdomResourceTransport{}, PendingUnits: []State.KingdomUnitTransport{},
		ResourceWorkflows: map[State.KingdomID]State.KingdomResourceTransportWorkflow{},
		ObservedAt:        frame.ReceivedAt,
	}
	for kingdomID, workflow := range gameState.KingdomTransport.ResourceWorkflows {
		workflow.Goods = append([]State.KingdomTransportGood(nil), workflow.Goods...)
		next.ResourceWorkflows[kingdomID] = workflow
	}
	var unlocks []map[string]json.RawMessage
	_ = json.Unmarshal(root["UL"], &unlocks)
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
	var pending []map[string]json.RawMessage
	_ = json.Unmarshal(root["RT"], &pending)
	for _, row := range pending {
		transport := State.KingdomResourceTransport{
			KingdomID:    State.KingdomID(rawInteger(row["KID"])),
			RemainingSec: int(rawInteger(row["RS"])), Goods: []State.KingdomTransportGood{},
		}
		var goods [][]json.RawMessage
		_ = json.Unmarshal(row["G"], &goods)
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
	var pendingUnits []map[string]json.RawMessage
	_ = json.Unmarshal(root["UT"], &pendingUnits)
	for _, row := range pendingUnits {
		transport := State.KingdomUnitTransport{
			KingdomID:    State.KingdomID(rawInteger(row["KID"])),
			RemainingSec: int(rawInteger(row["RS"])), Units: []State.KingdomTransportUnit{},
		}
		var units [][]json.RawMessage
		_ = json.Unmarshal(row["I"], &units)
		for _, unit := range units {
			unitID, amount := State.UnitID(rowInt(unit, 0)), rowInt(unit, 1)
			if unitID <= 0 || amount <= 0 {
				continue
			}
			transport.Units = append(transport.Units, State.KingdomTransportUnit{UnitID: unitID, Amount: amount})
		}
		next.PendingUnits = append(next.PendingUnits, transport)
	}
	if reflect.DeepEqual(gameState.KingdomTransport, next) {
		return nil, false, nil
	}
	gameState.KingdomTransport = next
	return []string{"kingdom-transport"}, true, nil
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
