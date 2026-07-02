package GameParser

import (
	"encoding/json"

	"CitadelDesktop/Server/Models"
)

// ApplyCastleResourceAmountsFromPayload applies root **grc** resource amounts from JAA/BUP-like payloads.
func ApplyCastleResourceAmountsFromPayload(gs *Models.GameState, data string) bool {
	if gs == nil {
		return false
	}
	var root map[string]interface{}
	if err := json.Unmarshal([]byte(data), &root); err != nil || root == nil {
		return false
	}
	grc, ok := root["grc"].(map[string]interface{})
	if !ok || grc == nil {
		return false
	}
	castleID := jsonIntFromAny(grc["AID"])
	if castleID <= 0 {
		castleID = gs.CastleFocus.CastleAID
	}
	if castleID <= 0 || !gs.IsKnownPlayerCastleID(castleID) {
		return false
	}
	c := gs.GetCastleByID(castleID)
	if c == nil {
		return false
	}
	changed := false
	set := func(key string, dst *float64) {
		raw, exists := grc[key]
		if !exists {
			return
		}
		value := jsonFloatFromAny(raw)
		if *dst == value {
			return
		}
		*dst = value
		changed = true
	}
	set("W", &c.Amount.WoodAmount)
	set("S", &c.Amount.StoneAmount)
	set("F", &c.Amount.FoodAmount)
	set("C", &c.Amount.CoalAmount)
	set("O", &c.Amount.OilAmount)
	set("G", &c.Amount.GlassAmount)
	set("I", &c.Amount.IronAmount)
	set("HONEY", &c.Amount.HoneyAmount)
	set("MEAD", &c.Amount.MeadAmount)
	set("BEEF", &c.Amount.BeefAmount)
	return changed
}
