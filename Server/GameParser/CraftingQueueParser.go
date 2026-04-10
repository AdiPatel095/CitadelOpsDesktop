package GameParser

import (
	"CitadelDesktop/Server/Models/Castle"
	gamestate "CitadelDesktop/Server/Models/GameState"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
)

// ApplyCraftingFromCRINJSON replaces **CraftingQueues** from **crin** (CAI / CBI).
// Accepts wire shapes seen from GGE:
//   - {"CAI":{"CBI":[...]}}  (same as jaa **crai**)
//   - {"CAI":[{"CBI":[...]}]}
//   - {"crai":{"CAI":...}}
//   - PS/QS keys uppercase or lowercase; CRID or crid; when CRID is empty, RUT/rut is used as slot ids (labels fall back in craftingrecipes).
func ApplyCraftingFromCRINJSON(gs *gamestate.GameState, data string) bool {
	root, err := unmarshalJSONObjectPayload(data)
	if err != nil {
		return false
	}
	cbis := extractCBIEntries(root)
	if len(cbis) == 0 {
		return false
	}
	aid := intFromMapAny(cbis[0], "AID", "aid")
	if aid <= 0 || !gs.IsKnownPlayerCastleID(aid) {
		return false
	}
	c := gs.GetCastleByID(aid)
	if c == nil {
		return false
	}
	next := make([]castle.CraftingBuildingSnapshot, 0, len(cbis))
	for _, m := range cbis {
		snap, ok := snapshotFromFlexibleMap(m)
		if !ok {
			continue
		}
		next = append(next, snap)
	}
	if len(next) == 0 {
		return false
	}
	if craftingQueuesDeepEqual(c.CraftingQueues, next) {
		return false
	}
	c.CraftingQueues = next
	return true
}

// ApplyCraftingFromCRSTJSON merges one building from **crst** into **CraftingQueues** (match **OID**).
// Root may be a flat CBI object or {"CBI":{...}} / {"CBI":[{...}]}.
func ApplyCraftingFromCRSTJSON(gs *gamestate.GameState, data string) bool {
	root, err := unmarshalJSONObjectPayload(data)
	if err != nil {
		return false
	}
	m, ok := normalizeSingleCBIMap(root)
	if !ok {
		return false
	}
	snap, ok := snapshotFromFlexibleMap(m)
	if !ok {
		return false
	}
	if snap.AID <= 0 || !gs.IsKnownPlayerCastleID(snap.AID) {
		return false
	}
	c := gs.GetCastleByID(snap.AID)
	if c == nil {
		return false
	}
	prev := append([]castle.CraftingBuildingSnapshot(nil), c.CraftingQueues...)
	found := false
	for i := range c.CraftingQueues {
		if c.CraftingQueues[i].OID == snap.OID {
			c.CraftingQueues[i] = snap
			found = true
			break
		}
	}
	if !found {
		c.CraftingQueues = append(c.CraftingQueues, snap)
	}
	if craftingQueuesDeepEqual(prev, c.CraftingQueues) {
		return false
	}
	return true
}

func craftingQueuesDeepEqual(a, b []castle.CraftingBuildingSnapshot) bool {
	return reflect.DeepEqual(a, b)
}

// --- Flexible JSON helpers (mirror SlotProductionParser style) ---

func unmarshalJSONObjectPayload(data string) (map[string]interface{}, error) {
	s := strings.TrimSpace(data)
	if s == "" {
		return nil, fmt.Errorf("empty")
	}
	var root map[string]interface{}
	if err := json.Unmarshal([]byte(s), &root); err != nil {
		var inner string
		if err2 := json.Unmarshal([]byte(s), &inner); err2 == nil && strings.TrimSpace(inner) != "" {
			if err3 := json.Unmarshal([]byte(inner), &root); err3 != nil {
				return nil, err
			}
			return root, nil
		}
		return nil, err
	}
	return root, nil
}

func extractCBIEntries(root map[string]interface{}) []map[string]interface{} {
	if cra, ok := root["crai"].(map[string]interface{}); ok {
		root = cra
	}
	if cra, ok := root["CRAI"].(map[string]interface{}); ok {
		root = cra
	}
	raw, ok := root["CAI"]
	if !ok {
		raw, ok = root["cai"]
	}
	if !ok {
		return nil
	}
	var out []map[string]interface{}
	switch v := raw.(type) {
	case map[string]interface{}:
		appendCBIFromInterfaceSlice(v["CBI"], &out)
		appendCBIFromInterfaceSlice(v["cbi"], &out)
	case []interface{}:
		for _, el := range v {
			em, ok := el.(map[string]interface{})
			if !ok {
				continue
			}
			appendCBIFromInterfaceSlice(em["CBI"], &out)
			appendCBIFromInterfaceSlice(em["cbi"], &out)
		}
	}
	return out
}

func appendCBIFromInterfaceSlice(raw interface{}, out *[]map[string]interface{}) {
	arr, ok := raw.([]interface{})
	if !ok {
		return
	}
	for _, x := range arr {
		if m, ok := x.(map[string]interface{}); ok {
			*out = append(*out, m)
		}
	}
}

func normalizeSingleCBIMap(root map[string]interface{}) (map[string]interface{}, bool) {
	if inner, ok := root["CBI"].([]interface{}); ok && len(inner) == 1 {
		if m, ok := inner[0].(map[string]interface{}); ok {
			return m, true
		}
	}
	if inner, ok := root["cbi"].([]interface{}); ok && len(inner) == 1 {
		if m, ok := inner[0].(map[string]interface{}); ok {
			return m, true
		}
	}
	if inner, ok := root["CBI"].(map[string]interface{}); ok {
		return inner, true
	}
	if inner, ok := root["cbi"].(map[string]interface{}); ok {
		return inner, true
	}
	// flat CBI body
	if intFromMapAny(root, "WID", "wid") > 0 || intFromMapAny(root, "OID", "oid") > 0 {
		return root, true
	}
	return nil, false
}

func snapshotFromFlexibleMap(m map[string]interface{}) (castle.CraftingBuildingSnapshot, bool) {
	wid := intFromMapAny(m, "WID", "wid")
	if wid == 0 {
		return castle.CraftingBuildingSnapshot{}, false
	}
	return castle.CraftingBuildingSnapshot{
		KID:  intFromMapAny(m, "KID", "kid"),
		AID:  intFromMapAny(m, "AID", "aid"),
		OID:  intFromMapAny(m, "OID", "oid"),
		CQID: intFromMapAny(m, "CQID", "cqid"),
		WID:  wid,
		PS:   bundleFromFlexibleMap(subobject(m, "PS", "ps")),
		QS:   bundleFromFlexibleMap(subobject(m, "QS", "qs")),
	}, true
}

func subobject(m map[string]interface{}, keys ...string) map[string]interface{} {
	for _, k := range keys {
		if v, ok := m[k]; ok {
			if sm, ok := v.(map[string]interface{}); ok {
				return sm
			}
		}
	}
	return nil
}

func bundleFromFlexibleMap(m map[string]interface{}) castle.CraftingSlotBundle {
	if m == nil {
		return castle.CraftingSlotBundle{}
	}
	crid := intSliceFromMap(m, "CRID", "crid")
	if len(crid) == 0 {
		crid = intSliceFromMap(m, "RUT", "rut")
	}
	bv := floatSliceFromMap(m, "BV", "bv")
	return castle.CraftingSlotBundle{CRID: crid, BV: bv}
}

func intFromMapAny(m map[string]interface{}, keys ...string) int {
	for _, k := range keys {
		v, ok := m[k]
		if !ok || v == nil {
			continue
		}
		switch n := v.(type) {
		case float64:
			return int(n)
		case int:
			return n
		case int64:
			return int(n)
		case json.Number:
			i, _ := n.Int64()
			return int(i)
		case string:
			var i int
			_, _ = fmt.Sscanf(strings.TrimSpace(n), "%d", &i)
			return i
		}
	}
	return 0
}

func intSliceFromMap(m map[string]interface{}, keys ...string) []int {
	for _, k := range keys {
		raw, ok := m[k]
		if !ok || raw == nil {
			continue
		}
		switch arr := raw.(type) {
		case []interface{}:
			return normalizeIntSlice(arr)
		case []int:
			out := make([]int, len(arr))
			copy(out, arr)
			return out
		case []float64:
			out := make([]int, len(arr))
			for i, v := range arr {
				out[i] = int(v)
			}
			return out
		}
	}
	return nil
}

func normalizeIntSlice(arr []interface{}) []int {
	out := make([]int, 0, len(arr))
	for _, x := range arr {
		switch n := x.(type) {
		case float64:
			out = append(out, int(n))
		case int:
			out = append(out, n)
		case int64:
			out = append(out, int(n))
		case json.Number:
			i, _ := n.Int64()
			out = append(out, int(i))
		case string:
			var i int
			_, _ = fmt.Sscanf(strings.TrimSpace(n), "%d", &i)
			out = append(out, i)
		default:
			out = append(out, 0)
		}
	}
	return out
}

func floatSliceFromMap(m map[string]interface{}, keys ...string) []float64 {
	for _, k := range keys {
		raw, ok := m[k]
		if !ok || raw == nil {
			continue
		}
		switch arr := raw.(type) {
		case []interface{}:
			out := make([]float64, 0, len(arr))
			for _, x := range arr {
				switch n := x.(type) {
				case float64:
					out = append(out, n)
				case int:
					out = append(out, float64(n))
				case int64:
					out = append(out, float64(n))
				case json.Number:
					f, _ := n.Float64()
					out = append(out, f)
				case string:
					var f float64
					_, _ = fmt.Sscanf(strings.TrimSpace(n), "%f", &f)
					out = append(out, f)
				default:
					out = append(out, 0)
				}
			}
			return out
		}
	}
	return nil
}
