package GameParser

import (
	"encoding/json"
	"reflect"
	"sync/atomic"

	"CitadelDesktop/Server/Models"
	gamestate "CitadelDesktop/Server/Models/GameState"
)

const caravanOverloaderBoosterID = 11

var (
	marketInfoUpdateGeneration    uint64
	marketBoosterUpdateGeneration uint64
)

// MarketInfoUpdateGeneration increments on each valid cmi snapshot.
func MarketInfoUpdateGeneration() uint64 {
	return atomic.LoadUint64(&marketInfoUpdateGeneration)
}

// MarketBoosterUpdateGeneration increments on each valid boi snapshot.
func MarketBoosterUpdateGeneration() uint64 {
	return atomic.LoadUint64(&marketBoosterUpdateGeneration)
}

// ApplyMarketInfoFromJSON applies standalone cmi or nested {"cmi": ...} data.
func ApplyMarketInfoFromJSON(data string) bool {
	var root map[string]interface{}
	if err := json.Unmarshal([]byte(data), &root); err != nil {
		return false
	}
	if nested, ok := root["cmi"].(map[string]interface{}); ok {
		root = nested
	}
	rows, ok := root["C"].([]interface{})
	if !ok {
		return false
	}

	castles := make([]gamestate.MarketCastleState, 0, len(rows))
	for _, raw := range rows {
		row, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}
		castleID := intFromMapAny(row, "CID")
		if castleID <= 0 {
			continue
		}
		state := gamestate.MarketCastleState{
			CastleID:         castleID,
			KingdomID:        intFromMapAny(row, "KID"),
			TotalBarrows:     intFromMapAny(row, "TC"),
			AvailableBarrows: intFromMapAny(row, "AC"),
			Resources:        make(map[string]float64),
		}
		for _, key := range []string{"W", "S", "F", "C", "O", "G", "I", "HONEY", "MEAD", "BEEF"} {
			if value, exists := row[key]; exists {
				state.Resources[key] = floatFromAny(value)
			}
		}
		if effects, ok := row["AE"].([]interface{}); ok {
			for _, effectRaw := range effects {
				effect, ok := effectRaw.([]interface{})
				if !ok || len(effect) < 2 {
					continue
				}
				parsed := gamestate.MarketAreaEffect{EffectID: int(floatFromAny(effect[0]))}
				if values, ok := effect[1].([]interface{}); ok {
					for _, value := range values {
						parsed.Values = append(parsed.Values, floatFromAny(value))
					}
				}
				if len(effect) > 2 {
					parsed.Source, _ = effect[2].(string)
				}
				if parsed.EffectID > 0 {
					state.AreaEffects = append(state.AreaEffects, parsed)
				}
			}
		}
		castles = append(castles, state)
	}

	gs := Models.GetGameState()
	previous := gs.MarketTransportSnapshot().Castles
	gs.SetMarketCastles(castles)
	for _, marketCastle := range castles {
		castle := gs.GetCastleByID(marketCastle.CastleID)
		if castle == nil {
			continue
		}
		castle.MarketBarrowsTotal = marketCastle.TotalBarrows
		castle.MarketBarrowsAvailable = marketCastle.AvailableBarrows
		if value, ok := marketCastle.Resources["W"]; ok {
			castle.Amount.WoodAmount = value
		}
		if value, ok := marketCastle.Resources["S"]; ok {
			castle.Amount.StoneAmount = value
		}
		if value, ok := marketCastle.Resources["F"]; ok {
			castle.Amount.FoodAmount = value
		}
		if value, ok := marketCastle.Resources["C"]; ok {
			castle.Amount.CoalAmount = value
		}
		if value, ok := marketCastle.Resources["O"]; ok {
			castle.Amount.OilAmount = value
		}
		if value, ok := marketCastle.Resources["G"]; ok {
			castle.Amount.GlassAmount = value
		}
		if value, ok := marketCastle.Resources["I"]; ok {
			castle.Amount.IronAmount = value
		}
		if value, ok := marketCastle.Resources["HONEY"]; ok {
			castle.Amount.HoneyAmount = value
		}
		if value, ok := marketCastle.Resources["MEAD"]; ok {
			castle.Amount.MeadAmount = value
		}
		if value, ok := marketCastle.Resources["BEEF"]; ok {
			castle.Amount.BeefAmount = value
		}
	}
	atomic.AddUint64(&marketInfoUpdateGeneration, 1)
	return !reflect.DeepEqual(previous, castles)
}

// ApplyMarketBoosterFromJSON applies standalone boi or nested {"boi": ...} data.
func ApplyMarketBoosterFromJSON(data string) bool {
	var root map[string]interface{}
	if err := json.Unmarshal([]byte(data), &root); err != nil {
		return false
	}
	if nested, ok := root["boi"].(map[string]interface{}); ok {
		root = nested
	}
	return ApplyMarketBoosterMap(root)
}

// ApplyMarketBoosterMap stores the permanent caravan-overloader level from boi.BO ID 11.
func ApplyMarketBoosterMap(root map[string]interface{}) bool {
	rows, ok := root["BO"].([]interface{})
	if !ok {
		return false
	}
	level := 0
	for _, raw := range rows {
		row, ok := raw.(map[string]interface{})
		if !ok || intFromMapAny(row, "ID") != caravanOverloaderBoosterID {
			continue
		}
		level = intFromMapAny(row, "L")
		break
	}
	gs := Models.GetGameState()
	previous := gs.MarketTransportSnapshot()
	gs.SetMarketCaravanLevel(level)
	atomic.AddUint64(&marketBoosterUpdateGeneration, 1)
	return !previous.CaravanLevelLoaded || previous.CaravanLevel != level
}
