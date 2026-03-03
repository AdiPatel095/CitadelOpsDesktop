package GameParser

import (
	"CitadelDesktop/Server/Models"
	"encoding/json"
)

// ParseCastleTroops parses the JAA response to extract troop counts for a castle.
// The response contains gui.I which is an array of [unitID, count] pairs.
func ParseCastleTroops(data string, kingdomID, x, y int) *Models.CastleTroops {
	// In the real payload (per cmds3.json/gui_packet), the JSON is nested inside an array element and is itself a string:
	// ["","xt","gui","1","0","{\"I\":[[216..],...]...}"]
	// However ParseCastleTroops is called with `data string` being the 6th element.
	// We need to unmarshal `data` into a map where "I", "TU", etc exist directly.

	dataObj := make(map[string]interface{})
	if err := json.Unmarshal([]byte(data), &dataObj); err != nil {
		return nil
	}

	guiObj, ok := dataObj["gui"].(map[string]interface{})
	if !ok {
		return nil
	}

	// Helper function to parse an array of [unitID, count] pairs
	parseUnitArray := func(key string) map[int]int {
		result := make(map[int]int)

		val, exists := guiObj[key]
		if !exists {
			return result
		}

		arr, ok := val.([]interface{})
		if !ok {
			return result
		}

		for _, item := range arr {
			pair, ok := item.([]interface{})
			if !ok {
				continue
			}
			if len(pair) < 2 {
				continue
			}

			unitID, ok1 := pair[0].(float64)
			count, ok2 := pair[1].(float64)
			if !ok1 || !ok2 {
				continue
			}

			// Only include if it's a troop (not a tool)
			if Models.IsTroop(int(unitID)) {
				result[int(unitID)] = int(count)
			} else {
			}
		}
		return result
	}

	troopsI := parseUnitArray("I")
	troopsTU := parseUnitArray("TU")
	troopsHI := parseUnitArray("HI")
	troopsSHI := parseUnitArray("SHI")

	// Parse buildings from gca.BD and gca.BG

	// Create mixed map (I + TU)
	troopsMixed := make(map[int]int)
	for id, count := range troopsI {
		troopsMixed[id] += count
	}
	for id, count := range troopsTU {
		troopsMixed[id] += count
	}

	return &Models.CastleTroops{
		KingdomID:   kingdomID,
		X:           x,
		Y:           y,
		Troops:      troopsI, // Keep backward compatibility
		TroopsI:     troopsI,
		TroopsTU:    troopsTU,
		TroopsHI:    troopsHI,
		TroopsSHI:   troopsSHI,
		TroopsMixed: troopsMixed,
	}
}
