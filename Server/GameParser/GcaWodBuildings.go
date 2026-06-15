package GameParser

import (
	"CitadelDesktop/Server/Models"
	dec "CitadelDesktop/Server/Models/Decoration"
	"encoding/json"
	"strconv"
)

// gcaWodLayout holds fields from one gca BG/BD instance (array row or object form).
// WID is the global building/decoration type id (wodID in object maps, index 0 in array rows).
type gcaWodLayout struct {
	WID, OID, X, Y, R, Level int
}

// GCAWodIDFromArrayRow returns the wodID / building type id from a JAA gca.BG or gca.BD **array** row
// (first element, index 0). For decoration- or structure-only parsing, filter rows with
// decoration.IsKnownDecorationWID or the Parse*Decorations* / Parse*Structure* entry points.
func GCAWodIDFromArrayRow(row []interface{}) (wid int, ok bool) {
	lay, ok := gcaLayoutFromRowArray(row)
	if !ok {
		return 0, false
	}
	return lay.WID, true
}

// GCAWodIDFromObjectMap returns the WID for a gca.BG/BD object-shaped entry. Pass oidFromKey when the
// parent map is keyed by OID (non-zero when read from a map key).
func GCAWodIDFromObjectMap(m map[string]interface{}, oidFromKey int) (wid int, ok bool) {
	lay, ok := gcaLayoutFromObject(m, oidFromKey)
	if !ok {
		return 0, false
	}
	return lay.WID, true
}

// ResolveGCAJAAWodName is the default display name for a BG/BD row: EmpireItems decoration
// metadata when [decoration.IsKnownDecorationWID] is true, otherwise the static building catalog name.
func ResolveGCAJAAWodName(wid int) string {
	if dec.IsKnownDecorationWID(wid) {
		return dec.ResolvedWodDisplayName(wid)
	}
	return Models.GetBuildingInfo(wid).Name
}

// ResolveGCADecorationWodName is for rows known to be catalog decorations; uses the same labels as
// [decoration.DecorationSummaryLinesForCastle] tooling.
func ResolveGCADecorationWodName(wid int) string {
	return dec.ResolvedWodDisplayName(wid)
}

// ResolveGCAStructureWodName is for non-decoration buildings (barracks, walls, non-deco wodIDs, etc.).
func ResolveGCAStructureWodName(wid int) string {
	return Models.GetBuildingInfo(wid).Name
}

// gcaLayoutFromRowArray uses the JAA row layout: [0]=WID, [1]=OID, [2]=X, [3]=Y, [4]=R; optional level at index 14.
func gcaLayoutFromRowArray(row []interface{}) (gcaWodLayout, bool) {
	if len(row) < 4 {
		return gcaWodLayout{}, false
	}
	lay := gcaWodLayout{
		WID: gcaJSONInt(row[0]),
		OID: gcaJSONInt(row[1]),
		X:   gcaJSONInt(row[2]),
		Y:   gcaJSONInt(row[3]),
	}
	if len(row) > 4 {
		lay.R = gcaJSONInt(row[4])
	}
	if len(row) > 14 {
		lay.Level = gcaJSONInt(row[14])
	}
	if lay.WID <= 0 || lay.OID <= 0 {
		return gcaWodLayout{}, false
	}
	return lay, true
}

// gcaLayoutFromObject reads WID/OID/position/level from a JSON object.
func gcaLayoutFromObject(m map[string]interface{}, oidFromKey int) (gcaWodLayout, bool) {
	wid := gcaJSONInt(m["WID"])
	if wid == 0 {
		wid = gcaJSONInt(m["wodID"])
	}
	oid := gcaJSONInt(m["OID"])
	if oid == 0 {
		oid = oidFromKey
	}
	x := gcaJSONInt(m["X"])
	if x == 0 {
		x = gcaJSONInt(m["PX"])
	}
	y := gcaJSONInt(m["Y"])
	if y == 0 {
		y = gcaJSONInt(m["PY"])
	}
	r := gcaJSONInt(m["R"])
	level := gcaJSONInt(m["L"])
	if wid <= 0 || oid <= 0 {
		return gcaWodLayout{}, false
	}
	return gcaWodLayout{WID: wid, OID: oid, X: x, Y: y, R: r, Level: level}, true
}

func buildingDataFromLayout(lay gcaWodLayout, name string) Models.BuildingData {
	return Models.BuildingData{
		BuildingID: lay.WID,
		OID:        lay.OID,
		Name:       name,
		Level:      lay.Level,
		X:          lay.X,
		Y:          lay.Y,
		R:          lay.R,
	}
}

func gcaBuildingFromRowArrayWithResolver(row []interface{}, nameFor func(int) string) (Models.BuildingData, bool) {
	lay, ok := gcaLayoutFromRowArray(row)
	if !ok {
		return Models.BuildingData{}, false
	}
	name := nameFor(lay.WID)
	return buildingDataFromLayout(lay, name), true
}

func gcaBuildingFromObjectWithResolver(m map[string]interface{}, oidFromKey int, nameFor func(int) string) (Models.BuildingData, bool) {
	lay, ok := gcaLayoutFromObject(m, oidFromKey)
	if !ok {
		return Models.BuildingData{}, false
	}
	return buildingDataFromLayout(lay, nameFor(lay.WID)), true
}

// gcaWodRowFilter, if non-nil, keeps only rows where it returns true for the row WID.
type gcaWodRowFilter func(wid int) bool

func appendGCAWodBuildingsFromGroup(dst []Models.BuildingData, raw interface{}, nameFor func(int) string, keep gcaWodRowFilter) []Models.BuildingData {
	if raw == nil {
		return dst
	}
	switch v := raw.(type) {
	case []interface{}:
		for _, item := range v {
			if m, ok := item.(map[string]interface{}); ok {
				if b, ok := gcaBuildingFromObjectWithResolver(m, 0, nameFor); ok {
					if keep == nil || keep(b.BuildingID) {
						dst = append(dst, b)
					}
				}
				continue
			}
			row, ok := item.([]interface{})
			if !ok {
				continue
			}
			if b, ok := gcaBuildingFromRowArrayWithResolver(row, nameFor); ok {
				if keep == nil || keep(b.BuildingID) {
					dst = append(dst, b)
				}
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
			if b, ok := gcaBuildingFromObjectWithResolver(m, oidKey, nameFor); ok {
				if keep == nil || keep(b.BuildingID) {
					dst = append(dst, b)
				}
			}
		}
	default:
	}
	return dst
}

// parseGCAObjectFromJAAJSON unmarshals a jaa response and returns the gca object.
func parseGCAObjectFromJAAJSON(data string) (map[string]interface{}, bool) {
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

// ParseBGFromGCA parses gca.BG with default decoration+structure name resolution.
func ParseBGFromGCA(gcaObj map[string]interface{}) []Models.BuildingData {
	if gcaObj == nil {
		return nil
	}
	return appendGCAWodBuildingsFromGroup(nil, gcaObj["BG"], ResolveGCAJAAWodName, nil)
}

// ParseBDFromGCA parses gca.BD with default decoration+structure name resolution.
func ParseBDFromGCA(gcaObj map[string]interface{}) []Models.BuildingData {
	if gcaObj == nil {
		return nil
	}
	return appendGCAWodBuildingsFromGroup(nil, gcaObj["BD"], ResolveGCAJAAWodName, nil)
}

// ParseDecorationsFromGCA returns only rows whose WID is a known catalog decoration
// (see [decoration.IsKnownDecorationWID]).
func ParseDecorationsFromGCA(gcaObj map[string]interface{}) []Models.BuildingData {
	if gcaObj == nil {
		return nil
	}
	keep := func(wid int) bool { return dec.IsKnownDecorationWID(wid) }
	bg := appendGCAWodBuildingsFromGroup(nil, gcaObj["BG"], ResolveGCADecorationWodName, keep)
	return appendGCAWodBuildingsFromGroup(bg, gcaObj["BD"], ResolveGCADecorationWodName, keep)
}

// ParseStructureBuildingsFromGCA returns BG+BD rows that are not known catalog decorations
// (barracks, walls, and other wodIDs; includes unknown-type rows not listed as decos).
func ParseStructureBuildingsFromGCA(gcaObj map[string]interface{}) []Models.BuildingData {
	if gcaObj == nil {
		return nil
	}
	keep := func(wid int) bool { return !dec.IsKnownDecorationWID(wid) }
	bg := appendGCAWodBuildingsFromGroup(nil, gcaObj["BG"], ResolveGCAStructureWodName, keep)
	return appendGCAWodBuildingsFromGroup(bg, gcaObj["BD"], ResolveGCAStructureWodName, keep)
}

// ParseBuildingsFromGCA merges gca.BG and gca.BD (default combined naming).
func ParseBuildingsFromGCA(gcaObj map[string]interface{}) []Models.BuildingData {
	var buildings []Models.BuildingData
	buildings = append(buildings, ParseBGFromGCA(gcaObj)...)
	buildings = append(buildings, ParseBDFromGCA(gcaObj)...)
	return buildings
}

// ParseBGFromJAAResponseJSON unmarshals jaa JSON and extracts gca.BG rows.
func ParseBGFromJAAResponseJSON(data string) []Models.BuildingData {
	gcaObj, ok := parseGCAObjectFromJAAJSON(data)
	if !ok {
		return nil
	}
	return ParseBGFromGCA(gcaObj)
}

// ParseBDFromJAAResponseJSON unmarshals jaa JSON and extracts gca.BD rows.
func ParseBDFromJAAResponseJSON(data string) []Models.BuildingData {
	gcaObj, ok := parseGCAObjectFromJAAJSON(data)
	if !ok {
		return nil
	}
	return ParseBDFromGCA(gcaObj)
}

// ParseBuildingsFromJAAResponseJSON merges BG+BD (same order as historical merged list).
func ParseBuildingsFromJAAResponseJSON(data string) []Models.BuildingData {
	gcaObj, ok := parseGCAObjectFromJAAJSON(data)
	if !ok {
		return nil
	}
	return ParseBuildingsFromGCA(gcaObj)
}

// ParseDecorationsFromJAAResponseJSON is decoration-only: known EmpireItems deco wodIDs from BG+BD.
func ParseDecorationsFromJAAResponseJSON(data string) []Models.BuildingData {
	gcaObj, ok := parseGCAObjectFromJAAJSON(data)
	if !ok {
		return nil
	}
	return ParseDecorationsFromGCA(gcaObj)
}

// ParseStructureBuildingsFromJAAResponseJSON is non-decoration BG+BD (structures and other wodIDs).
func ParseStructureBuildingsFromJAAResponseJSON(data string) []Models.BuildingData {
	gcaObj, ok := parseGCAObjectFromJAAJSON(data)
	if !ok {
		return nil
	}
	return ParseStructureBuildingsFromGCA(gcaObj)
}
