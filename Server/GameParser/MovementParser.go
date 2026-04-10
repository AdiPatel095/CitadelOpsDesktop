package GameParser

import (
	"CitadelDesktop/Server/Models"
	"CitadelDesktop/Server/Scheduler"
	"encoding/json"
	"fmt"
	"time"
)

// OnGAMParsed is a callback hook that fires after GAM messages are parsed.
var OnGAMParsed func()

// movementItemsFromGAMLikeRoot splits a **gam** / **cds** / **cat** / **cra** JSON root into per-movement objects (each holds M, UM, A).
func movementItemsFromGAMLikeRoot(gamData map[string]interface{}) []map[string]interface{} {
	var mArray []interface{}

	var wrapper map[string]interface{}
	if aWrapper, ok := gamData["A"].(map[string]interface{}); ok {
		wrapper = aWrapper
	} else if aamWrapper, ok := gamData["AAM"].(map[string]interface{}); ok {
		wrapper = aamWrapper
	}

	if wrapper != nil {
		movObj := make(map[string]interface{})
		if mObj, ok := wrapper["M"].(map[string]interface{}); ok {
			movObj["M"] = mObj
		}
		if umObj, ok := wrapper["UM"].(map[string]interface{}); ok {
			movObj["UM"] = umObj
		}
		if aArr, ok := wrapper["A"].([]interface{}); ok {
			movObj["A"] = aArr
		}
		mArray = []interface{}{movObj}
	} else if mVal, ok := gamData["M"]; ok {
		switch v := mVal.(type) {
		case []interface{}:
			mArray = v
		case map[string]interface{}:
			mArray = []interface{}{gamData}
		default:
			return nil
		}
	} else {
		return nil
	}

	out := make([]map[string]interface{}, 0, len(mArray))
	for _, item := range mArray {
		movObj, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		out = append(out, movObj)
	}
	return out
}

// ExtractMaxTTSecondsFromGAMLikeJSON returns the largest one-way travel time **TT** (seconds) from M blocks in a **gam**-like JSON payload (including **cds** responses).
func ExtractMaxTTSecondsFromGAMLikeJSON(data string) int {
	var gamData map[string]interface{}
	if err := json.Unmarshal([]byte(data), &gamData); err != nil {
		return 0
	}
	items := movementItemsFromGAMLikeRoot(gamData)
	maxTT := 0
	for _, movObj := range items {
		mDetails, ok := movObj["M"].(map[string]interface{})
		if !ok {
			continue
		}
		tt := getInt(mDetails, "TT")
		if tt > maxTT {
			maxTT = tt
		}
	}
	return maxTT
}

// ParseGAMMessage parses the GAM (Global Army Movement) message
// GAM message contains all active movements for the player and alliance
// We store ALL movements since troop composition matching is unique
func ParseGAMMessage(data string) {

	var gamData map[string]interface{}
	if err := json.Unmarshal([]byte(data), &gamData); err != nil {
		return
	}
	//log.Printf("Raw GAM Data: %s", data)

	items := movementItemsFromGAMLikeRoot(gamData)
	if len(items) == 0 {
		return
	}

	gs := Models.GetGameState()
	// gs.Movement.ActiveMovements is not cleared here; fetchers manage lifecycle.
	var parsedMovements []Models.GAMMovement

	for _, movObj := range items {

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
				// troopItem should be an array like [215, 1]
				if troopPair, ok := troopItem.([]interface{}); ok && len(troopPair) >= 2 {
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

		// Extract CommanderID from UM.L.ID
		commanderID := -1
		if umObj, ok := movObj["UM"].(map[string]interface{}); ok {
			if lObj, ok := umObj["L"].(map[string]interface{}); ok {
				// We can't use getInt directly because it returns 0 on missing/error.
				// We need to check if "ID" actually exists in the map.
				if idVal, ok := lObj["ID"]; ok {
					if fID, ok := idVal.(float64); ok {
						commanderID = int(fID)
					}
				} else {
				}
			} else {
			}
		} else {
		}

		if commanderID >= 0 {
		} else {
		}

		// Store ALL movements - we'll match by troop composition which is unique
		movement := Models.GAMMovement{
			MID:         mid,
			PT:          pt,
			TT:          tt,
			D:           d,
			KID:         kid,
			SID:         sid,
			OID:         oid,
			TargetX:     targetX,
			TargetY:     targetY,
			SourceX:     sourceX,
			SourceY:     sourceY,
			CommanderID: commanderID,
			TroopArray:  troopArray,
		}
		parsedMovements = append(parsedMovements, movement)

		// Set real travel time cooldown in Scheduler if target coordinates are present
		if targetX != 0 && targetY != 0 && tt > 0 {
			targetID := fmt.Sprintf("%d,%d", targetX, targetY)
			// Apply a generic returning cooldown (time to target and back) + small safety buffer
			totalCooldownMs := (tt * 2 * 1000) + 10000
			Scheduler.GetScheduler().CooldownTracker.SetCooldown(targetID, time.Duration(totalCooldownMs)*time.Millisecond)
		}
	}

	// Replace with the latest snapshot so reconciliation always sees current movements only.
	gs.Movement.ActiveMovements = parsedMovements
	markGAMParsed()

	// Trigger callback to auto-clean returned birds in real-time
	if OnGAMParsed != nil {
		go OnGAMParsed()
	}
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
