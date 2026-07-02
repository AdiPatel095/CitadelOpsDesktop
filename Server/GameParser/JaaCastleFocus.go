package GameParser

import (
	"CitadelDesktop/Server/Models"
	"encoding/json"
	"reflect"
)

// NotifyCastleFocusChanged is wired by FrontendWebsocket to push castleFocus when jaa updates the in-view castle.
var NotifyCastleFocusChanged func()

// NotifyAutoTCIConstructionUpdated is wired by GameFeatures to reschedule ubc timers when gca.CI changes (JAA, rpc, ubc).
var NotifyAutoTCIConstructionUpdated func()

// NotifyBeriCastleDiscovered is wired by FeatureView when GCL (KID 10) provides Beri CID.
var NotifyBeriCastleDiscovered func(castleCID, mapX, mapY int)

// NotifyMainCastleAIDForAutoBeri is wired by FeatureView when GCL (KID 0) sets the main castle (kut SCID).
var NotifyMainCastleAIDForAutoBeri func(mainCastleAID int)

// parseJAACastleIdentityFromPayload extracts kingdom id, player castle instance id, and map coordinates
// from a jaa/jca response JSON body. Layout matches jsonExamples/jaa.json: root KID, gca.A[1]=PX, A[2]=PY, A[3]=castle CID.
func parseJAACastleIdentityFromPayload(data string) (kid int, castleID int, mapPX int, mapPY int, ok bool) {
	root := make(map[string]interface{})
	if err := json.Unmarshal([]byte(data), &root); err != nil {
		return
	}
	kid = gcaJSONInt(root["KID"])
	gcaObj, _ := root["gca"].(map[string]interface{})
	if gcaObj == nil {
		return
	}
	aArr, _ := gcaObj["A"].([]interface{})
	if len(aArr) < 4 {
		return
	}
	mapPX = gcaJSONInt(aArr[1])
	mapPY = gcaJSONInt(aArr[2])
	castleID = gcaJSONInt(aArr[3])
	if castleID <= 0 {
		return
	}
	ok = true
	return
}

func castleFocusPositionChanged(prev, cur Models.CastleFocus) bool {
	return prev.CastleAID != cur.CastleAID || prev.KingdomID != cur.KingdomID || prev.MapPX != cur.MapPX || prev.MapPY != cur.MapPY
}

// ApplyCastleFocusFromJAAPayload updates GameState.CastleFocus position when jaa describes one of the player's castles.
// Returns true if the focused castle or map coordinates changed.
func ApplyCastleFocusFromJAAPayload(gs *Models.GameState, data string) bool {
	kid, cid, px, py, ok := parseJAACastleIdentityFromPayload(data)
	if !ok {
		return false
	}
	if !gs.IsKnownPlayerCastleID(cid) {
		return false
	}
	prev := gs.CastleFocus
	gs.UpdateCastleFocusPosition(cid, kid, px, py)
	return castleFocusPositionChanged(prev, gs.CastleFocus)
}

func buildingRowSliceEqual(a, b []Models.BuildingData) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func buildingRowsSnapshotEqual(bg1, bd1, bg2, bd2 []Models.BuildingData) bool {
	return buildingRowSliceEqual(bg1, bg2) && buildingRowSliceEqual(bd1, bd2)
}

// ApplyJAABuildingRowsFromPayload parses gca BG/BD from jaa and stores them on the matching PlayerCastleInfo.
// Row parsing is ParseBGFromJAAResponseJSON / ParseBDFromJAAResponseJSON (GcaWodBuildings, gca.BG / gca.BD).
// Returns true if BG/BD changed (for pushing castleFocus to the frontend).
func ApplyJAABuildingRowsFromPayload(gs *Models.GameState, data string) bool {
	_, cid, _, _, ok := parseJAACastleIdentityFromPayload(data)
	if !ok || !gs.IsKnownPlayerCastleID(cid) {
		return false
	}
	bg := ParseBGFromJAAResponseJSON(data)
	bd := ParseBDFromJAAResponseJSON(data)
	c := gs.GetCastleByID(cid)
	if c == nil {
		return false
	}
	if buildingRowsSnapshotEqual(c.BGRows, c.BDRows, bg, bd) {
		return false
	}
	Models.SetCastleBuildingRows(c, bg, bd)
	return true
}

func applyConstructionCIToCastle(gs *Models.GameState, castleID int, data string) bool {
	next := ParseGCAConstructionFromGameJSON(data)
	if next == nil {
		return false
	}
	c := gs.GetCastleByID(castleID)
	if c == nil {
		return false
	}
	if constructionSlotsEqual(c.ConstructionByBuilding, next) {
		return false
	}
	Models.SetCastleConstructionSlots(c, next)
	if NotifyAutoTCIConstructionUpdated != nil {
		go NotifyAutoTCIConstructionUpdated()
	}
	return true
}

// ApplyJAAConstructionSlotsFromPayload parses gca.CI and stores equipped CIDs, remaining time (RS), and levels per OID.
func ApplyJAAConstructionSlotsFromPayload(gs *Models.GameState, data string) bool {
	_, cid, _, _, ok := parseJAACastleIdentityFromPayload(data)
	if !ok || !gs.IsKnownPlayerCastleID(cid) {
		return false
	}
	return applyConstructionCIToCastle(gs, cid, data)
}

// ApplyFocusCastleConstructionCIFromPayload applies root `CI` (e.g. **rpc** / **ubc** responses) to the currently focused castle.
func ApplyFocusCastleConstructionCIFromPayload(gs *Models.GameState, data string) bool {
	cid := gs.CastleFocus.CastleAID
	if cid <= 0 || !gs.IsKnownPlayerCastleID(cid) {
		return false
	}
	return applyConstructionCIToCastle(gs, cid, data)
}

// ApplyJAATroopsFromPayload parses gui troop arrays from the jaa JSON and updates the castle's CastleTroopData.
// Returns true if troop snapshot changed (for notifying the frontend).
func ApplyJAATroopsFromPayload(gs *Models.GameState, data string) bool {
	kid, cid, px, py, ok := parseJAACastleIdentityFromPayload(data)
	if !ok || !gs.IsKnownPlayerCastleID(cid) {
		return false
	}
	troops := ParseCastleTroops(data, kid, px, py)
	if troops == nil {
		return false
	}
	troops.CastleID = cid
	castle := gs.GetCastleByID(cid)
	if castle == nil {
		return false
	}
	next := Models.CastleTroopData{
		KingdomID:   troops.KingdomID,
		X:           troops.X,
		Y:           troops.Y,
		TroopsI:     troops.TroopsI,
		TroopsTU:    troops.TroopsTU,
		TroopsHI:    troops.TroopsHI,
		TroopsSHI:   troops.TroopsSHI,
		TroopsMixed: troops.TroopsMixed,
	}
	if reflect.DeepEqual(castle.Troops, next) {
		return false
	}
	applyTroopFetchToCastle(gs, cid, troops)
	return true
}

// ApplyFocusedCastleTroopsFromPayload parses a root gui object from a focused-castle response
// such as **hru** and updates the currently focused castle's troop snapshot.
func ApplyFocusedCastleTroopsFromPayload(gs *Models.GameState, data string) bool {
	if gs == nil {
		return false
	}
	cid := gs.CastleFocus.CastleAID
	if cid <= 0 || !gs.IsKnownPlayerCastleID(cid) {
		return false
	}
	troops := ParseCastleTroops(data, gs.CastleFocus.KingdomID, gs.CastleFocus.MapPX, gs.CastleFocus.MapPY)
	if troops == nil {
		return false
	}
	troops.CastleID = cid
	castle := gs.GetCastleByID(cid)
	if castle == nil {
		return false
	}
	next := Models.CastleTroopData{
		KingdomID:   troops.KingdomID,
		X:           troops.X,
		Y:           troops.Y,
		TroopsI:     troops.TroopsI,
		TroopsTU:    troops.TroopsTU,
		TroopsHI:    troops.TroopsHI,
		TroopsSHI:   troops.TroopsSHI,
		TroopsMixed: troops.TroopsMixed,
	}
	if reflect.DeepEqual(castle.Troops, next) {
		return false
	}
	applyTroopFetchToCastle(gs, cid, troops)
	return true
}
