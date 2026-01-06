package GameParser

import (
	"CitadelDesktop/Server/Models"
	"encoding/json"
)

// ParseGAMMessage parses the GAM (Global Army Movement) message
// GAM message contains all active movements for the player and alliance
// We store ALL movements since troop composition matching is unique
func ParseGAMMessage(data string) {

	var gamData map[string]interface{}
	if err := json.Unmarshal([]byte(data), &gamData); err != nil {
		return
	}

	// Get M array (movements)
	mArray, ok := gamData["M"].([]interface{})
	if !ok {
		return
	}

	gs := Models.GetGameState()
	// gs.ActiveMovements is NOT cleared here anymore - handled by AutoBird/Scheduler before fetch
	var parsedMovements []Models.GAMMovement

	for _, item := range mArray {
		movObj, ok := item.(map[string]interface{})
		if !ok {
			continue
		}

		mDetails, ok := movObj["M"].(map[string]interface{})
		if !ok {
			continue
		}

		// Extract fields
		mid := getInt(mDetails, "MID")
		pt := getInt(mDetails, "PT")
		tt := getInt(mDetails, "TT")
		d := getInt(mDetails, "D")
		kid := getInt(mDetails, "KID")
		sid := getInt(mDetails, "SID")
		oid := getInt(mDetails, "OID")

		// Extract target coordinates from TA array (indices 1 and 2)
		targetX := 0
		targetY := 0
		if taArray, ok := mDetails["TA"].([]interface{}); ok && len(taArray) > 2 {
			if tx, ok := taArray[1].(float64); ok {
				targetX = int(tx)
			}
			if ty, ok := taArray[2].(float64); ok {
				targetY = int(ty)
			}
		}

		// Extract source coordinates from SA array (indices 1 and 2)
		sourceX := 0
		sourceY := 0
		if saArray, ok := mDetails["SA"].([]interface{}); ok && len(saArray) > 2 {
			if sx, ok := saArray[1].(float64); ok {
				sourceX = int(sx)
			}
			if sy, ok := saArray[2].(float64); ok {
				sourceY = int(sy)
			}
		}

		// Extract troop composition from A field (at the parent level, not in M)
		var troopArray [][]int
		if aField, ok := movObj["A"].([]interface{}); ok {
			for _, troopItem := range aField {
				if troopPair, ok := troopItem.([]interface{}); ok && len(troopPair) == 2 {
					if troopID, ok := troopPair[0].(float64); ok {
						if troopCount, ok := troopPair[1].(float64); ok {
							if int(troopID) > 0 && int(troopCount) > 0 {
								troopArray = append(troopArray, []int{int(troopID), int(troopCount)})
							}
						}
					}
				}
			}
		}

		// Log each movement for debugging

		// Store ALL movements - we'll match by troop composition which is unique
		movement := Models.GAMMovement{
			MID:        mid,
			PT:         pt,
			TT:         tt,
			D:          d,
			KID:        kid,
			SID:        sid,
			OID:        oid,
			TargetX:    targetX,
			TargetY:    targetY,
			SourceX:    sourceX,
			SourceY:    sourceY,
			TroopArray: troopArray,
		}
		parsedMovements = append(parsedMovements, movement)
	}

	// Append new movements to the existing list (allows multiple GAM messages to be accumulated)
	gs.ActiveMovements = append(gs.ActiveMovements, parsedMovements...)
}

// Helper to safely get int from map
func getInt(m map[string]interface{}, key string) int {
	if val, ok := m[key]; ok {
		if fVal, ok := val.(float64); ok {
			return int(fVal)
		}
	}
	return 0
}
