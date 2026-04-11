package GameParser

import (
	"encoding/json"
	"fmt"
)

// ParseDecorationStorageCountsFromSINJSON parses the **sin** response JSON body.
// Shape matches Logs/JSONExamples/sin.json: a top-level array of segments { "SID", "RD", "CD", "UD" }.
// Each RD row is [ decorationWodID, amount, … ]; index 0 = type id, index 1 = count. Counts are summed across all segments and rows.
func ParseDecorationStorageCountsFromSINJSON(payload string) (map[int]int, error) {
	var top []json.RawMessage
	if err := json.Unmarshal([]byte(payload), &top); err != nil {
		return nil, fmt.Errorf("sin: top-level JSON: %w", err)
	}
	out := make(map[int]int)
	for _, segRaw := range top {
		var seg struct {
			RD []json.RawMessage `json:"RD"`
		}
		if err := json.Unmarshal(segRaw, &seg); err != nil {
			continue
		}
		for _, rowRaw := range seg.RD {
			var row []interface{}
			if err := json.Unmarshal(rowRaw, &row); err != nil || len(row) < 2 {
				continue
			}
			wid, wok := sinJSONInt(row[0])
			amt, aok := sinJSONInt(row[1])
			if !wok || !aok || wid <= 0 || amt < 0 {
				continue
			}
			out[wid] += amt
		}
	}
	return out, nil
}

// ParseDecorationStorageCountsFromSINFrame parses a full %-split **sin** game frame (payload at index 5).
func ParseDecorationStorageCountsFromSINFrame(parts []string) (map[int]int, error) {
	payload, ok := Payload(parts)
	if !ok {
		return nil, fmt.Errorf("sin: missing payload segment")
	}
	return ParseDecorationStorageCountsFromSINJSON(payload)
}

func sinJSONInt(v interface{}) (int, bool) {
	switch x := v.(type) {
	case float64:
		return int(x), true
	case int:
		return x, true
	case int64:
		return int(x), true
	default:
		return 0, false
	}
}
