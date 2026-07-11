package GameParser

import (
	"encoding/json"
	"reflect"
	"sync/atomic"

	"CitadelDesktop/Server/Models"
	gamestate "CitadelDesktop/Server/Models/GameState"
)

var kingdomTransportUpdateGeneration uint64

// KingdomTransportUpdateGeneration increments on each valid standalone or nested **kpi** snapshot.
func KingdomTransportUpdateGeneration() uint64 {
	return atomic.LoadUint64(&kingdomTransportUpdateGeneration)
}

// ApplyKingdomTransportFromJSON applies standalone kpi or nested {"kpi": ...} responses.
func ApplyKingdomTransportFromJSON(data string) bool {
	var root map[string]interface{}
	if err := json.Unmarshal([]byte(data), &root); err != nil {
		return false
	}
	if nested, ok := root["kpi"].(map[string]interface{}); ok {
		root = nested
	}
	return ApplyKingdomTransportMap(root)
}

// ApplyKingdomTransportMap replaces the latest complete kingdom transport snapshot.
func ApplyKingdomTransportMap(root map[string]interface{}) bool {
	if root == nil {
		return false
	}
	if nested, ok := root["kpi"].(map[string]interface{}); ok {
		root = nested
	}
	if _, hasUnlocks := root["UL"]; !hasUnlocks {
		if _, hasPending := root["RT"]; !hasPending {
			return false
		}
	}

	state := gamestate.KingdomTransportState{}
	if rows, ok := root["UL"].([]interface{}); ok {
		state.Unlocks = make([]gamestate.KingdomTransportUnlock, 0, len(rows))
		for _, rowRaw := range rows {
			row, ok := rowRaw.(map[string]interface{})
			if !ok {
				continue
			}
			state.Unlocks = append(state.Unlocks, gamestate.KingdomTransportUnlock{
				KingdomID: intFromMapAny(row, "KID"),
				Unlocked:  intFromMapAny(row, "U"),
				Created:   intFromMapAny(row, "C"),
				Stage:     intFromMapAny(row, "SL"),
			})
		}
	}
	if rows, ok := root["RT"].([]interface{}); ok {
		state.Pending = make([]gamestate.KingdomResourceTransport, 0, len(rows))
		for _, rowRaw := range rows {
			row, ok := rowRaw.(map[string]interface{})
			if !ok {
				continue
			}
			pending := gamestate.KingdomResourceTransport{
				KingdomID:    intFromMapAny(row, "KID"),
				RemainingSec: intFromMapAny(row, "RS"),
			}
			if goods, ok := row["G"].([]interface{}); ok {
				for _, goodRaw := range goods {
					good, ok := goodRaw.([]interface{})
					if !ok || len(good) < 2 {
						continue
					}
					resource, _ := good[0].(string)
					amount := floatFromAny(good[1])
					if resource == "" || amount <= 0 {
						continue
					}
					pending.Goods = append(pending.Goods, gamestate.KingdomTransportGood{
						Resource: resource,
						Amount:   amount,
					})
				}
			}
			state.Pending = append(state.Pending, pending)
		}
	}

	gs := Models.GetGameState()
	previous := gs.KingdomTransportSnapshot()
	gs.SetKingdomTransportState(state)
	atomic.AddUint64(&kingdomTransportUpdateGeneration, 1)
	return !reflect.DeepEqual(previous, state)
}
