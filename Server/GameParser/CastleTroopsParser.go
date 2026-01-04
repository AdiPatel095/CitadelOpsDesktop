package GameParser

import (
	"CitadelDesktop/Server/Models"
	"encoding/json"
)

// ParseCastleTroops parses the JAA response to extract troop counts for a castle.
// The response contains gui.I which is an array of [unitID, count] pairs.
func ParseCastleTroops(data string, kingdomID, x, y int) *Models.CastleTroops {
	var jaaData map[string]interface{}
	err := json.Unmarshal([]byte(data), &jaaData)
	if err != nil {
		return nil
	}

	// Get gui.I array
	gui, ok := jaaData["gui"].(map[string]interface{})
	if !ok {
		return nil
	}

	iArray, ok := gui["I"].([]interface{})
	if !ok {
		return nil
	}

	troops := make(map[int]int)

	// Parse each [unitID, count] pair
	for _, item := range iArray {
		pair, ok := item.([]interface{})
		if !ok || len(pair) < 2 {
			continue
		}

		unitID, ok1 := pair[0].(float64)
		count, ok2 := pair[1].(float64)
		if !ok1 || !ok2 {
			continue
		}

		// Only include if it's a troop (not a tool)
		if Models.IsTroop(int(unitID)) {
			troops[int(unitID)] = int(count)
		}
	}

	return &Models.CastleTroops{
		KingdomID: kingdomID,
		X:         x,
		Y:         y,
		Troops:    troops,
	}
}
