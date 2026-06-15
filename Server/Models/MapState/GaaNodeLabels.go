package mapstate

import "fmt"

// GaaNodeTypeLabels maps inbound **gaa** AI[0] → short label (from 2026-06-07 websocket_game.log scan).
// Refresh: go run ./cmd/ScanGaaLog/
var GaaNodeTypeLabels = map[int]string{
	1:  "Castle (main)",
	2:  "Kingdom tower",
	3:  "Castle (capital)",
	4:  "Castle (occupied)",
	10: "Alliance camp",
	11: "Unknown (type 11)",
	12: "Castle (foreign kingdom)",
	22: "Castle (type 22)",
	23: "Coord marker",
	25: "Unknown (type 25)",
	26: "Monument",
	27: "Nomad camp",
	28: "Resource node",
	29: "Unknown (type 29)",
	31: "Empty terrain",
	34: "Unknown (type 34)",
	35: "Khan camp",
	37: "Unknown (type 37)",
	38: "Unknown (type 38)",
	43: "Rift",
}

// CastleSlotTypeLabels maps GCL/ain/AP[][4] slot types (same scan + historical captures).
var CastleSlotTypeLabels = map[int]string{
	1:  "Main",
	3:  "Unknown (slot 3)",
	4:  "Outpost",
	5:  "Metropolis",
	6:  "Capital",
	12: "Foreign kingdom",
	22: "Unknown (slot 22)",
	23: "Unknown (slot 23)",
	24: "Unknown (slot 24)",
	26: "Monument",
	28: "Laboratory",
}

// LabelGaaNodeType returns a label for a gaa map-node type id.
func LabelGaaNodeType(typeID int) string {
	if s, ok := GaaNodeTypeLabels[typeID]; ok {
		return s
	}
	return fmt.Sprintf("Unknown (type %d)", typeID)
}

// LabelCastleSlotType returns a label for AP/GCL castle slot type id.
func LabelCastleSlotType(typeID int) string {
	if s, ok := CastleSlotTypeLabels[typeID]; ok {
		return s
	}
	return fmt.Sprintf("Unknown (slot %d)", typeID)
}
