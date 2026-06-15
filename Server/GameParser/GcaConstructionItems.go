package GameParser

import (
	"CitadelDesktop/Server/Models"
	"encoding/json"
	"reflect"
)

// GCAConstructionItemIDFromCILMap returns a single CIL.CID (construction item id) from one CIL object
// in gca.CI. The id is the same as construction_items constructionItemID; not a BD/BG wodID.
func GCAConstructionItemIDFromCILMap(m map[string]interface{}) (cid int, ok bool) {
	if m == nil {
		return 0, false
	}
	cid = gcaJSONInt(m["CID"])
	if cid <= 0 {
		return 0, false
	}
	return cid, true
}

// ParseGCAConstructionFromGameJSON extracts CI from a JAA body (gca.CI) or a compact frame like
// **rpc** / **ubc** where the root object is `{"CI":[...]}` (optional BCID on ubc).
func ParseGCAConstructionFromGameJSON(data string) []Models.GCAConstructionBuilding {
	var root map[string]interface{}
	if err := json.Unmarshal([]byte(data), &root); err != nil {
		return nil
	}
	if gcaObj, _ := root["gca"].(map[string]interface{}); gcaObj != nil {
		if raw, ok := gcaObj["CI"].([]interface{}); ok {
			return parseCIBuildingArray(raw)
		}
	}
	if raw, ok := root["CI"].([]interface{}); ok {
		return parseCIBuildingArray(raw)
	}
	return nil
}

// ParseGCAConstructionFromJAAResponseJSON extracts gca.CI from a full jaa JSON body.
// Prefer [ParseGCAConstructionFromGameJSON], which also handles rpc/ubc.
func ParseGCAConstructionFromJAAResponseJSON(data string) []Models.GCAConstructionBuilding {
	return ParseGCAConstructionFromGameJSON(data)
}

func parseCIBuildingArray(raw []interface{}) []Models.GCAConstructionBuilding {
	if len(raw) == 0 {
		return nil
	}
	out := make([]Models.GCAConstructionBuilding, 0, len(raw))
	for _, item := range raw {
		m, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		oid := gcaJSONInt(m["OID"])
		if oid <= 0 {
			continue
		}
		cilRaw, _ := m["CIL"].([]interface{})
		if len(cilRaw) == 0 {
			out = append(out, Models.GCAConstructionBuilding{OID: oid, Slots: nil})
			continue
		}
		slots := make([]Models.GCAConstructionSlot, 0, len(cilRaw))
		for _, slot := range cilRaw {
			sm, ok := slot.(map[string]interface{})
			if !ok {
				continue
			}
			cid, ok := GCAConstructionItemIDFromCILMap(sm)
			if !ok {
				continue
			}
			s := Models.GCAConstructionSlot{CID: cid, S: gcaJSONInt(sm["S"])}
			if sm["RS"] != nil {
				rs := gcaJSONInt(sm["RS"])
				if rs > 0 {
					s.RemainingSec = &rs
				}
			}
			slots = append(slots, s)
		}
		out = append(out, Models.GCAConstructionBuilding{OID: oid, Slots: slots})
	}
	if len(out) == 0 {
		return nil
	}
	enrichGCAConstructionBuildingsLevels(out)
	return out
}

func constructionSlotsEqual(a, b []Models.GCAConstructionBuilding) bool {
	return reflect.DeepEqual(a, b)
}
