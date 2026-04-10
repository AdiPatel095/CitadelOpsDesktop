package GameParser

import (
	"CitadelDesktop/Server/Models"
	"encoding/json"
	"strconv"
)

// jsonIntFromIface coerces JSON-decoded numbers (float64, json.Number, etc.) to int.
func jsonIntFromIface(v interface{}) int {
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
	default:
		return 0
	}
}

// gcaBuildingFromRowArray uses the shared JAA layout for BG and BD rows:
// [0]=WID, [1]=OID, [2]=X, [3]=Y, [4]=R; optional level at index 14 when present (legacy GGE rows).
func gcaBuildingFromRowArray(row []interface{}) (Models.BuildingData, bool) {
	if len(row) < 4 {
		return Models.BuildingData{}, false
	}
	wid := jsonIntFromIface(row[0])
	oid := jsonIntFromIface(row[1])
	x := jsonIntFromIface(row[2])
	y := jsonIntFromIface(row[3])
	if wid <= 0 || oid <= 0 {
		return Models.BuildingData{}, false
	}
	r := 0
	if len(row) > 4 {
		r = jsonIntFromIface(row[4])
	}
	level := 0
	if len(row) > 14 {
		level = jsonIntFromIface(row[14])
	}
	info := Models.GetBuildingInfo(wid)
	return Models.BuildingData{
		BuildingID: wid,
		OID:        oid,
		Name:       info.Name,
		Level:      level,
		X:          x,
		Y:          y,
		R:          r,
	}, true
}

// gcaBuildingFromObject reads WID/OID/position/level from a JSON object (used when BD/BG items are maps).
func gcaBuildingFromObject(m map[string]interface{}, oidFromMapKey int) (Models.BuildingData, bool) {
	wid := jsonIntFromIface(m["WID"])
	if wid == 0 {
		wid = jsonIntFromIface(m["wodID"])
	}
	oid := jsonIntFromIface(m["OID"])
	if oid == 0 {
		oid = oidFromMapKey
	}
	x := jsonIntFromIface(m["X"])
	if x == 0 {
		x = jsonIntFromIface(m["PX"])
	}
	y := jsonIntFromIface(m["Y"])
	if y == 0 {
		y = jsonIntFromIface(m["PY"])
	}
	r := jsonIntFromIface(m["R"])
	level := jsonIntFromIface(m["L"])
	if wid <= 0 || oid <= 0 {
		return Models.BuildingData{}, false
	}
	info := Models.GetBuildingInfo(wid)
	return Models.BuildingData{
		BuildingID: wid,
		OID:        oid,
		Name:       info.Name,
		Level:      level,
		X:          x,
		Y:          y,
		R:          r,
	}, true
}

// appendBuildingsFromGCAGroup parses one gca.BG or gca.BD value (array of rows and/or map keyed by OID) and appends to dst.
func appendBuildingsFromGCAGroup(dst []Models.BuildingData, raw interface{}) []Models.BuildingData {
	if raw == nil {
		return dst
	}
	switch v := raw.(type) {
	case []interface{}:
		for _, item := range v {
			if m, ok := item.(map[string]interface{}); ok {
				if b, ok := gcaBuildingFromObject(m, 0); ok {
					dst = append(dst, b)
				}
				continue
			}
			row, ok := item.([]interface{})
			if !ok {
				continue
			}
			if b, ok := gcaBuildingFromRowArray(row); ok {
				dst = append(dst, b)
			}
		}
	case map[string]interface{}:
		for key, val := range v {
			oidKey, err := strconv.Atoi(key)
			if err != nil {
				continue
			}
			m, ok := val.(map[string]interface{})
			if !ok {
				continue
			}
			if b, ok := gcaBuildingFromObject(m, oidKey); ok {
				dst = append(dst, b)
			}
		}
	default:
	}
	return dst
}

// ParseBGFromGCA parses only gca.BG.
func ParseBGFromGCA(gcaObj map[string]interface{}) []Models.BuildingData {
	if gcaObj == nil {
		return nil
	}
	return appendBuildingsFromGCAGroup(nil, gcaObj["BG"])
}

// ParseBDFromGCA parses only gca.BD.
func ParseBDFromGCA(gcaObj map[string]interface{}) []Models.BuildingData {
	if gcaObj == nil {
		return nil
	}
	return appendBuildingsFromGCAGroup(nil, gcaObj["BD"])
}

// ParseBuildingsFromGCA merges gca.BG and gca.BD into one list using the same row semantics for both.
func ParseBuildingsFromGCA(gcaObj map[string]interface{}) []Models.BuildingData {
	var buildings []Models.BuildingData
	buildings = append(buildings, ParseBGFromGCA(gcaObj)...)
	buildings = append(buildings, ParseBDFromGCA(gcaObj)...)
	return buildings
}

func parseGCAFromJAAResponseJSON(data string) (map[string]interface{}, bool) {
	dataObj := make(map[string]interface{})
	if err := json.Unmarshal([]byte(data), &dataObj); err != nil {
		return nil, false
	}
	gcaObj, ok := dataObj["gca"].(map[string]interface{})
	if !ok {
		return nil, false
	}
	return gcaObj, true
}

// ParseBGFromJAAResponseJSON unmarshals jaa JSON and extracts gca.BG rows.
func ParseBGFromJAAResponseJSON(data string) []Models.BuildingData {
	gcaObj, ok := parseGCAFromJAAResponseJSON(data)
	if !ok {
		return nil
	}
	return ParseBGFromGCA(gcaObj)
}

// ParseBDFromJAAResponseJSON unmarshals jaa JSON and extracts gca.BD rows.
func ParseBDFromJAAResponseJSON(data string) []Models.BuildingData {
	gcaObj, ok := parseGCAFromJAAResponseJSON(data)
	if !ok {
		return nil
	}
	return ParseBDFromGCA(gcaObj)
}

// ParseBuildingsFromJAAResponseJSON merges BG+BD (same order as historical merged list).
func ParseBuildingsFromJAAResponseJSON(data string) []Models.BuildingData {
	gcaObj, ok := parseGCAFromJAAResponseJSON(data)
	if !ok {
		return nil
	}
	return ParseBuildingsFromGCA(gcaObj)
}
