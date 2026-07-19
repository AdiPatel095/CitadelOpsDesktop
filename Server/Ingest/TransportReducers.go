package Ingest

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"

	"CitadelDesktop/Server/GameData"
	"CitadelDesktop/Server/Protocol"
	"CitadelDesktop/Server/State"
)

const caravanOverloaderBoosterID = 11

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
			if castle, exists := gameState.Castles[castleID]; exists {
				ensureCastleMaps(&castle)
				balance := castle.Resources[resourceID]
				if balance.Amount != amount {
					balance.Amount = amount
					castle.Resources[resourceID] = balance
					gameState.Castles[castleID] = castle
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
		ID    wireInt64 `json:"ID"`
		Level int       `json:"L"`
	}
	if err := json.Unmarshal(root["BO"], &rows); err != nil {
		return nil, false, fmt.Errorf("decode market booster rows: %w", err)
	}
	level := 0
	for _, row := range rows {
		if row.ID == caravanOverloaderBoosterID {
			level = row.Level
			break
		}
	}
	if gameState.Market.CaravanLevelLoaded && gameState.Market.CaravanLevel == level {
		return nil, false, nil
	}
	gameState.Market.CaravanLevel = level
	gameState.Market.CaravanLevelLoaded = true
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
		ObservedAt: frame.ReceivedAt,
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
	var payload struct {
		Subscriptions []struct {
			TypeID         int `json:"STID"`
			RemainingSec   int `json:"RS"`
			GracePeriodSec int `json:"RSGP"`
		} `json:"SP"`
	}
	if err := json.Unmarshal(frame.Payload, &payload); err != nil {
		return nil, false, fmt.Errorf("decode subscriptions: %w", err)
	}
	next := make(map[int]State.SubscriptionState, len(payload.Subscriptions))
	for _, subscription := range payload.Subscriptions {
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
