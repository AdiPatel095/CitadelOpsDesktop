package GameCommands

import "fmt"

// FUCPayload opens Beri troop-capacity UI state for the Beri castle (**fuc**).
func FUCPayload(beriCastleCID int) string {
	return empireExFrame("fuc", fmt.Sprintf(`{"CID":%d}`, beriCastleCID))
}

// SEIPayload follows **fuc** in the Beri troop-space check sequence.
func SEIPayload() string {
	return empireExFrame("sei", "{}")
}

// DCLRefreshPayload requests castle detail refresh (**dcl**, CD=0).
func DCLRefreshPayload() string {
	return empireExFrame("dcl", `{"CD":0}`)
}

// KUTPayload sends troops to the Beri world (**kut**).
// scid is the source castle instance id; kutCID is the wire CID field (often -1); troopsJSON is e.g. [[10,1]].
func KUTPayload(scid, kutCID, skid, tkid int, troopsJSON string) string {
	return empireExFrame("kut", fmt.Sprintf(
		`{"SCID":%d,"SKID":%d,"TKID":%d,"CID":%d,"A":%s}`, scid, skid, tkid, kutCID, troopsJSON))
}

// KUTTroopArrayJSON formats the kut "A" batch for one unit type.
func KUTTroopArrayJSON(unitWID, amount int) string {
	return fmt.Sprintf("[[%d,%d]]", unitWID, amount)
}

// MSKPayload applies a march speed-up on Beri world (**msk**). Static shape for now.
func MSKPayload() string {
	return empireExFrame("msk", `{"MST":"MS5","KID":"10","TT":"1"}`)
}
