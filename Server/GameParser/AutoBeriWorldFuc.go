package GameParser

import (
	"encoding/json"
)

// FucTroopCheckResult is parsed from an inbound **fuc** JSON body after the troop-space sequence.
type FucTroopCheckResult struct {
	TroopAmount int // how many of the configured unit can/should be sent
	ParsedSCID  int // optional source castle id from the response (falls back to settings when 0)
}

// ParseFucTroopCheckResponse extracts troop send amount (and optional SCID) from a **fuc** frame payload.
// Live captures use {"FUC":<n>} (troop count for kut A[[unit,n]]). Also supports A/TU arrays and N/AMT/CNT.
func ParseFucTroopCheckResponse(payload string, unitWID int) (FucTroopCheckResult, bool) {
	if payload == "" || payload == "{}" {
		return FucTroopCheckResult{}, false
	}
	var root map[string]interface{}
	if err := json.Unmarshal([]byte(payload), &root); err != nil {
		return FucTroopCheckResult{}, false
	}
	out := FucTroopCheckResult{}
	if v, ok := jsonInt(root["SCID"]); ok && v > 0 {
		out.ParsedSCID = v
	}
	for _, key := range []string{"FUC", "N", "AMT", "CNT", "TU", "C", "FC", "AMOUNT"} {
		if v, ok := jsonInt(root[key]); ok && v > 0 {
			out.TroopAmount = v
			return out, true
		}
	}
	if amt, ok := troopAmountFromWireA(root["A"], unitWID); ok {
		out.TroopAmount = amt
		return out, true
	}
	if amt, ok := troopAmountFromWireA(root["TU"], unitWID); ok {
		out.TroopAmount = amt
		return out, true
	}
	// Walk one level of nested objects (some frames wrap counters).
	for _, v := range root {
		m, ok := v.(map[string]interface{})
		if !ok {
			continue
		}
		if v, ok := jsonInt(m["SCID"]); ok && v > 0 && out.ParsedSCID == 0 {
			out.ParsedSCID = v
		}
		for _, key := range []string{"FUC", "N", "AMT", "CNT", "TU", "C"} {
			if n, ok := jsonInt(m[key]); ok && n > 0 {
				out.TroopAmount = n
				return out, true
			}
		}
		if amt, ok := troopAmountFromWireA(m["A"], unitWID); ok {
			out.TroopAmount = amt
			return out, true
		}
	}
	return out, false
}

func troopAmountFromWireA(raw interface{}, unitWID int) (int, bool) {
	rows, ok := raw.([]interface{})
	if !ok || len(rows) == 0 {
		return 0, false
	}
	best := 0
	onlyAmt := 0
	for _, row := range rows {
		pair, ok := row.([]interface{})
		if !ok || len(pair) < 2 {
			continue
		}
		wid, wOk := jsonInt(pair[0])
		amt, aOk := jsonInt(pair[1])
		if !wOk || !aOk || amt <= 0 {
			continue
		}
		if unitWID <= 0 {
			if amt > best {
				best = amt
			}
			continue
		}
		if wid == unitWID {
			if amt > best {
				best = amt
			}
		} else if onlyAmt == 0 {
			onlyAmt = amt
		}
	}
	if best > 0 {
		return best, true
	}
	// Single troop row in **fuc** even when WID does not match filter — still one [[wid,amt]] pair.
	if len(rows) == 1 && onlyAmt > 0 {
		return onlyAmt, true
	}
	return 0, false
}

func jsonInt(v interface{}) (int, bool) {
	switch t := v.(type) {
	case float64:
		return int(t), true
	case int:
		return t, true
	case json.Number:
		i, err := t.Int64()
		return int(i), err == nil
	default:
		return 0, false
	}
}
