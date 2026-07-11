package GameParser

import (
	"CitadelDesktop/Server/Models/Castle"
	gamestate "CitadelDesktop/Server/Models/GameState"
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
	"strings"
	"sync/atomic"
	"time"
)

var craftingUpdateGeneration uint64

// CraftingUpdateGeneration increments whenever a valid crafting snapshot is applied. Automation uses
// it to wait for crin/crst/crun responses before making another decision from queue state.
func CraftingUpdateGeneration() uint64 {
	return atomic.LoadUint64(&craftingUpdateGeneration)
}

func bumpCraftingUpdateGeneration() {
	atomic.AddUint64(&craftingUpdateGeneration, 1)
}

// ApplyCraftingFromCRINJSON replaces **CraftingQueues** and effective recipe entitlements from **crin**.
// Accepts wire shapes seen from GGE:
//   - {"CAI":{"CBI":[...],"CE":[...]}}  (same as jaa **crai**)
//   - {"CAI":[{"CBI":[...],"CE":[...]}, ...]} (one entry per crafting castle)
//   - {"crai":{"CAI":...}}
//   - a flat CBI snapshot emitted by completion-time **crin** updates.
func ApplyCraftingFromCRINJSON(gs *gamestate.GameState, data string) bool {
	root, err := unmarshalJSONObjectPayload(data)
	if err != nil {
		return false
	}

	if flat, ok := normalizeSingleCBIMap(root); ok {
		changed, valid := mergeCraftingSnapshot(gs, flat)
		if valid {
			bumpCraftingUpdateGeneration()
		}
		return changed
	}

	groups := extractCAIEntries(root)
	if len(groups) == 0 {
		return false
	}
	changed := false
	valid := false
	for _, group := range groups {
		cbis := cbiEntriesFromCAI(group)
		if len(cbis) == 0 {
			continue
		}
		aid := intFromMapAny(cbis[0], "AID", "aid")
		if aid <= 0 || !gs.IsKnownPlayerCastleID(aid) {
			continue
		}
		c := gs.GetCastleByID(aid)
		if c == nil {
			continue
		}
		next := make([]castle.CraftingBuildingSnapshot, 0, len(cbis))
		for _, m := range cbis {
			snap, ok := snapshotFromFlexibleMap(m)
			if ok && snap.AID == aid {
				next = append(next, snap)
			}
		}
		if len(next) == 0 {
			continue
		}
		valid = true
		if !craftingQueuesDeepEqual(c.CraftingQueues, next) {
			c.CraftingQueues = next
			changed = true
		}
		entitlements := entitlementsFromCAI(group)
		if !reflect.DeepEqual(c.CraftingEntitlements, entitlements) {
			c.CraftingEntitlements = entitlements
			changed = true
		}
	}
	if valid {
		bumpCraftingUpdateGeneration()
	}
	return changed
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
	changed, valid := mergeCraftingSnapshot(gs, m)
	if valid {
		bumpCraftingUpdateGeneration()
	}
	return changed
}

func mergeCraftingSnapshot(gs *gamestate.GameState, m map[string]interface{}) (bool, bool) {
	snap, ok := snapshotFromFlexibleMap(m)
	if !ok || snap.AID <= 0 || !gs.IsKnownPlayerCastleID(snap.AID) {
		return false, false
	}
	c := gs.GetCastleByID(snap.AID)
	if c == nil {
		return false, false
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
	return !craftingQueuesDeepEqual(prev, c.CraftingQueues), true
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

func extractCAIEntries(root map[string]interface{}) []map[string]interface{} {
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
		out = append(out, v)
	case []interface{}:
		for _, el := range v {
			em, ok := el.(map[string]interface{})
			if !ok {
				continue
			}
			out = append(out, em)
		}
	}
	return out
}

func cbiEntriesFromCAI(cai map[string]interface{}) []map[string]interface{} {
	var out []map[string]interface{}
	appendCBIFromInterfaceSlice(cai["CBI"], &out)
	appendCBIFromInterfaceSlice(cai["cbi"], &out)
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
		KID:          intFromMapAny(m, "KID", "kid"),
		AID:          intFromMapAny(m, "AID", "aid"),
		OID:          intFromMapAny(m, "OID", "oid"),
		CQID:         intFromMapAny(m, "CQID", "cqid"),
		WID:          wid,
		S:            intFromMapAny(m, "S", "s"),
		ObservedUnix: time.Now().Unix(),
		PS:           bundleFromFlexibleMap(subobject(m, "PS", "ps")),
		QS:           bundleFromFlexibleMap(subobject(m, "QS", "qs")),
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
	bv := floatSliceFromMap(m, "BV", "bv")
	return castle.CraftingSlotBundle{
		CRID: crid,
		BV:   bv,
		RUT:  intSliceFromMap(m, "RUT", "rut"),
		RCT:  intSliceFromMap(m, "RCT", "rct"),
	}
}

func entitlementsFromCAI(cai map[string]interface{}) castle.CraftingEntitlements {
	recipeIDs := make(map[int]bool)
	groupIDs := make(map[int]bool)
	boosts := make(map[int]float64)
	raw := cai["CE"]
	if raw == nil {
		raw = cai["ce"]
	}
	entries, _ := raw.([]interface{})
	for _, entryRaw := range entries {
		entry, ok := entryRaw.([]interface{})
		if !ok || len(entry) < 2 {
			continue
		}
		effectID := intFromAny(entry[0])
		values, _ := entry[1].([]interface{})
		switch effectID {
		case 616:
			for _, value := range values {
				if id := intFromAny(value); id > 0 {
					recipeIDs[id] = true
				}
			}
		case 377:
			for _, value := range values {
				if id := intFromAny(value); id > 0 {
					groupIDs[id] = true
				}
			}
		case 373:
			if len(values) >= 2 {
				queueID := intFromAny(values[0])
				if queueID > 0 {
					boosts[queueID] += floatFromAny(values[1])
				}
			}
		}
	}
	return castle.CraftingEntitlements{
		EnabledRecipeIDs:      sortedIntSet(recipeIDs),
		EnabledRecipeGroupIDs: sortedIntSet(groupIDs),
		OutputBoostByQueue:    boosts,
	}
}

func sortedIntSet(values map[int]bool) []int {
	out := make([]int, 0, len(values))
	for value := range values {
		out = append(out, value)
	}
	sort.Ints(out)
	return out
}

func intFromAny(value interface{}) int {
	switch n := value.(type) {
	case float64:
		return int(n)
	case float32:
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
	default:
		return 0
	}
}

func floatFromAny(value interface{}) float64 {
	switch n := value.(type) {
	case float64:
		return n
	case float32:
		return float64(n)
	case int:
		return float64(n)
	case int64:
		return float64(n)
	case json.Number:
		f, _ := n.Float64()
		return f
	case string:
		var f float64
		_, _ = fmt.Sscanf(strings.TrimSpace(n), "%f", &f)
		return f
	default:
		return 0
	}
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
