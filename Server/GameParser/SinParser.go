package GameParser

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

// ParseDecorationStorageCountsFromSINJSON parses the **sin** response JSON body.
// Shape matches Logs/RecvCommandsJSON/sin.json: a top-level array of segments { "SID", "RD", "CD", "UD" }.
// Each RD row is [ typeId, amount, … ]; index 0 = type id, index 1 = count. Counts are summed across all segments and rows.
// For decoration storage that id is a decor wodID; for other SIDs the same field is still the wire "WID"/wod slot
// (e.g. construction-item rows use the same id as JAA gca.CI CIL.CID / construction_items constructionItemID — not decoration wodIDs).
//
// MessageRouter applies successful parses with GameState.MergeSINItemCountsFromMap so a partial **sin** frame
// does not wipe unrelated ids from an earlier update.
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

// ParseEmbeddedSINStorageCountsFromEnvelopeJSON extracts the same RD counts as [ParseDecorationStorageCountsFromSINJSON]
// from a root JSON object that embeds **sin** (array of segments), e.g. **jaa** or **ebu** payloads.
func ParseEmbeddedSINStorageCountsFromEnvelopeJSON(payload string) (map[int]int, bool) {
	var root map[string]interface{}
	if err := json.Unmarshal([]byte(payload), &root); err != nil {
		return nil, false
	}
	sinRaw, ok := root["sin"]
	if !ok || sinRaw == nil {
		return nil, false
	}
	b, err := json.Marshal(sinRaw)
	if err != nil {
		return nil, false
	}
	m, err := ParseDecorationStorageCountsFromSINJSON(string(b))
	if err != nil || len(m) == 0 {
		return nil, false
	}
	return m, true
}

// ParseConstructionInventoryPairsFromRootJSON parses inbound **gii** root `{"CI":[[wireCID,count],…]}` construction
// inventory (account stash). Not **sin** and not gca.CI (per-building equipped slots); first CI element must be a numeric pair.
func ParseConstructionInventoryPairsFromRootJSON(payload string) (map[int]int, bool) {
	var root map[string]interface{}
	if err := json.Unmarshal([]byte(payload), &root); err != nil {
		return nil, false
	}
	raw, ok := root["CI"].([]interface{})
	if !ok || len(raw) == 0 {
		return nil, false
	}
	switch raw[0].(type) {
	case map[string]interface{}:
		return nil, false
	case []interface{}:
	default:
		return nil, false
	}
	out := make(map[int]int)
	for _, item := range raw {
		pair, ok := item.([]interface{})
		if !ok || len(pair) < 2 {
			continue
		}
		cid, cok := sinJSONInt(pair[0])
		amt, aok := sinJSONInt(pair[1])
		if !cok || !aok || cid <= 0 || amt < 0 {
			continue
		}
		out[cid] += amt
	}
	if len(out) == 0 {
		return nil, false
	}
	return out, true
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
	case json.Number:
		i, err := x.Int64()
		if err != nil {
			return 0, false
		}
		return int(i), true
	case string:
		i, err := strconv.Atoi(strings.TrimSpace(x))
		if err != nil {
			return 0, false
		}
		return i, true
	default:
		return 0, false
	}
}
