package GameParser

import (
	"CitadelDesktop/Server/Models"
	"encoding/json"
	"time"
)

// OnGAMParsed is a callback hook that fires after GAM messages are parsed.
var OnGAMParsed func()

// NotifyMovementChanged is wired by FrontendWebsocket to push active movements after GAM-like parses.
var NotifyMovementChanged func()

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

// CommanderAndTTFromGAMLikeJSON returns outbound (**M.D**=0) commander wire id and one-way **TT** from a **gam**-like payload.
func CommanderAndTTFromGAMLikeJSON(data string) (commanderID, tt int, ok bool) {
	var gamData map[string]interface{}
	if err := json.Unmarshal([]byte(data), &gamData); err != nil {
		return 0, 0, false
	}
	items := movementItemsFromGAMLikeRoot(gamData)
	for _, movObj := range items {
		commanderID = -1
		mDetails, okM := movObj["M"].(map[string]interface{})
		if !okM {
			continue
		}
		if getInt(mDetails, "D") != 0 {
			continue
		}
		tt = getInt(mDetails, "TT")
		if umObj, okUM := movObj["UM"].(map[string]interface{}); okUM {
			if lObj, okL := umObj["L"].(map[string]interface{}); okL {
				if idVal, okID := lObj["ID"]; okID {
					if fID, okF := idVal.(float64); okF {
						commanderID = int(fID)
					}
				}
			}
		}
		if commanderID >= 0 && tt > 0 {
			return commanderID, tt, true
		}
	}
	return 0, 0, false
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
	isFullSnapshot := gamPayloadIsFullSnapshot(gamData)
	// An empty full **gam** snapshot ({"M":[],...}) is the authoritative "no active movements" frame
	// the game sends once a march finishes (e.g. a commander reaching home). It must clear
	// ActiveMovements so the commander frees; only bail when there is nothing to apply AND this is a
	// partial delta (cat/cra) that carries no parsable leg.
	if len(items) == 0 && !isFullSnapshot {
		return
	}

	nowUnix := time.Now().Unix()
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

		// Extract target type and coordinates from TA array ([0]=type, [1]=X, [2]=Y)
		targetType := 0
		targetX := 0
		targetY := 0
		if taArray, ok := mDetails["TA"].([]interface{}); ok && len(taArray) > 2 {
			if tt, ok := taArray[0].(float64); ok {
				targetType = int(tt)
			}
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

		// Extract CommanderID from UM.L.ID, plus the posted-wait window (UM.PWD/UM.TWD) used to keep a
		// support/bird leg alive while it sits at its destination instead of expiring it mid-wait.
		commanderID := -1
		pwd, twd := 0, 0
		if umObj, ok := movObj["UM"].(map[string]interface{}); ok {
			pwd = getInt(umObj, "PWD")
			twd = getInt(umObj, "TWD")
			if lObj, ok := umObj["L"].(map[string]interface{}); ok {
				// We can't use getInt directly because it returns 0 on missing/error.
				// We need to check if "ID" actually exists in the map.
				if idVal, ok := lObj["ID"]; ok {
					if fID, ok := idVal.(float64); ok {
						commanderID = int(fID)
					}
				}
			}
		}

		// Store ALL movements - we'll match by troop composition which is unique
		movement := Models.GAMMovement{
			MID:          mid,
			PT:           pt,
			TT:           tt,
			D:            d,
			KID:          kid,
			SID:          sid,
			OID:          oid,
			TargetType:   targetType,
			TargetX:      targetX,
			TargetY:      targetY,
			SourceX:      sourceX,
			SourceY:      sourceY,
			CommanderID:  commanderID,
			TroopArray:   troopArray,
			PWD:          pwd,
			TWD:          twd,
			ReceivedUnix: nowUnix,
		}
		parsedMovements = append(parsedMovements, movement)

	}
	startMovementPollTicker()
	if isFullSnapshot {
		pageSize := len(items)
		if rawPage, ok := gamData["M"].([]interface{}); ok {
			pageSize = len(rawPage)
		}
		queueGAMSnapshotPage(parsedMovements, pageSize, nowUnix)
		return
	}
	applyGAMMovementDeltas(parsedMovements)
}

// ParseMRMRemoveMovement handles **mrm** — pushed by the game as {"MID": n} when a march ends
// (e.g. a commander reaches home). Removing the leg immediately frees the commander without waiting
// for the next **gam** poll.
func ParseMRMRemoveMovement(payload string) {
	var body struct {
		MID int `json:"MID"`
	}
	if err := json.Unmarshal([]byte(payload), &body); err != nil || body.MID == 0 {
		return
	}

	removePendingGAMMovement(body.MID)
	removed := Models.GetGameState().Movement.RemoveMovement(body.MID)

	if removed {
		MaybeNotifyRiftCRALaunchBusyChanged()
		if NotifyMovementChanged != nil {
			NotifyMovementChanged()
		}
	}
}

// gamPayloadIsFullSnapshot is true for **gam** list payloads (root "M" is an array, possibly empty);
// **cat**/**cra**/**cds** are single-leg deltas wrapped in "A"/"AAM".
func gamPayloadIsFullSnapshot(gamData map[string]interface{}) bool {
	mVal, ok := gamData["M"]
	if !ok {
		return false
	}
	_, ok = mVal.([]interface{})
	return ok
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
