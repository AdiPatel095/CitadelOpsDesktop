// Package GameCommands queues raw %%xt%%EmpireEx_21%%…%% payloads onto ResponseRegistry.OutgoingMessages.
// The browser/game client must be connected; these mirror what the live Flash/HTML5 client sends.
//
// Conventions:
//   - Kingdom IDs (KID): 0 main, 1 desert, 2 ice, 3 fire, 4 storm, 10 beri world, etc.
//   - Castle instance id (CID/AID): per-player castle id used in sob/jca payloads.
//   - WID (wodID): global building/decoration type id from the items catalog — not the per-castle OID.
//
// Navigation before build/pickup: the game applies EBU/SOB to whichever castle view is focused.
// Use SendJCA or SendJAA so the client is on the target castle before SendEBU / SendSOB.
package GameCommands

import (
	"CitadelDesktop/Server/ResponseRegistry"
	"fmt"
	"time"
)

func jcaPayload(cid, kid int) string {
	return fmt.Sprintf(`%%xt%%EmpireEx_21%%jca%%1%%{"CID":%d,"KID":%d}%%`, cid, kid)
}

func jaaPayload(px, py, kid int) string {
	return fmt.Sprintf(`%%xt%%EmpireEx_21%%jaa%%1%%{"PX":%d,"PY":%d,"KID":%d}%%`, px, py, kid)
}

// SendJCA sends EmpireEx_21 **jca** — jump/focus a castle by castle id and kingdom.
//
// Payload: {"CID":<castleInstanceId>,"KID":<kingdomId>}
//
// Use for kingdoms that address castles by CID (typically not KID 0/4/10; align with troop-fetch logic).
func SendJCA(cid int, kid int) {
	ResponseRegistry.OutgoingMessages <- jcaPayload(cid, kid)
}

// SendJAA sends EmpireEx_21 **jaa** — focus map tile / area (main map, storm, beri, etc.).
//
// Payload: {"PX":<mapX>,"PY":<mapY>,"KID":<kingdomId>}
//
// Use when the game expects map coordinates (e.g. KID 0, 4, 10) instead of JCA.
func SendJAA(px int, py int, kid int) {
	ResponseRegistry.OutgoingMessages <- jaaPayload(px, py, kid)
}

// TroopFocusCommand returns the same JAA/JCA payload string used for troop fetching and castle focus
// (KID 0, 4, 10 → JAA with map coords; otherwise JCA). Use with alternate outgoing channels if needed.
func TroopFocusCommand(kingdomID, castleID, mapX, mapY int) string {
	if kingdomID == 0 || kingdomID == 4 || kingdomID == 10 {
		return jaaPayload(mapX, mapY, kingdomID)
	}
	return jcaPayload(castleID, kingdomID)
}

// SendTroopFocus sends JAA or JCA per troop-fetch rules so the client focuses the target castle.
func SendTroopFocus(kingdomID, castleID, mapX, mapY int) {
	ResponseRegistry.OutgoingMessages <- TroopFocusCommand(kingdomID, castleID, mapX, mapY)
}

// SendEBU sends EmpireEx_21 **ebu** — place/erect a building or decoration at grid coordinates.
//
// JSON shape (verified vs live client captures):
//
//	{"WID":<globalType>,"X":<grid>,"Y":<grid>,"R":0,"PWR":0,"PO":-1,"DOID":-1}
//
// WID is the global type id (same for all players). The payload does not include CID; the focused
// castle in the client receives the command. Some third-party docs use "BT" for build type; this client uses "WID".
func SendEBU(wid, gridX, gridY int) {
	payload := fmt.Sprintf(
		`%%xt%%EmpireEx_21%%ebu%%1%%{"WID":%d,"X":%d,"Y":%d,"R":0,"PWR":0,"PO":-1,"DOID":-1}%%`,
		wid, gridX, gridY)
	ResponseRegistry.OutgoingMessages <- payload
}

// SendEBUWithParams is SendEBU with explicit rotation/power/public-order/decoration-owner fields
// when a capture from your client differs from the default SendEBU constants.
func SendEBUWithParams(wid, gridX, gridY, r, pwr, po, doid int) {
	payload := fmt.Sprintf(
		`%%xt%%EmpireEx_21%%ebu%%1%%{"WID":%d,"X":%d,"Y":%d,"R":%d,"PWR":%d,"PO":%d,"DOID":%d}%%`,
		wid, gridX, gridY, r, pwr, po, doid)
	ResponseRegistry.OutgoingMessages <- payload
}

// SendSOB sends EmpireEx_21 **sob** — pick up an existing instance into inventory / storage.
//
// Payload: {"CID":<castleInstanceId>,"OID":<perCastleBuildingInstanceId>}
//
// OID comes from gca BG / Models.BuildingData.OID (not WID). If your captures use a different key than "OID", adjust here.
func SendSOB(castleAID, castleSpecificBuildingID int) {
	payload := fmt.Sprintf(`%%xt%%EmpireEx_21%%sob%%1%%{"CID":%d,"OID":%d}%%`, castleAID, castleSpecificBuildingID)
	ResponseRegistry.OutgoingMessages <- payload
}

// BarracksUnitPurchasePayload returns EmpireEx_21 **bup** — enqueue unit training at the barracks / production strip.
//
// Live client shape: {"LID":0,"WID":<unitType>,"AMT":<count>,"PO":-1,"PWR":0,"SK":<sessionKey>,"SID":0,"AID":<castleInstanceId>}
//
// WID is the global unit type id (not a building WOD). SK must match the current game session (from a browser capture for your account).
func BarracksUnitPurchasePayload(lid, unitWID, amount, po, pwr, sk, sid, castleAID int) string {
	return fmt.Sprintf(
		`%%xt%%EmpireEx_21%%bup%%1%%{"LID":%d,"WID":%d,"AMT":%d,"PO":%d,"PWR":%d,"SK":%d,"SID":%d,"AID":%d}%%`,
		lid, unitWID, amount, po, pwr, sk, sid, castleAID)
}

// SendBarracksUnitPurchase queues **bup** on OutgoingMessages (same as other commands in this package).
func SendBarracksUnitPurchase(lid, unitWID, amount, po, pwr, sk, sid, castleAID int) {
	ResponseRegistry.OutgoingMessages <- BarracksUnitPurchasePayload(lid, unitWID, amount, po, pwr, sk, sid, castleAID)
}

// SendSPL requests EmpireEx_21 **spl** — production slot list for the focused castle (**LID** = strip: 0 barracks, 1 tool workshop, …).
func SendSPL(lid int) {
	payload := fmt.Sprintf(`%%xt%%EmpireEx_21%%spl%%1%%{"LID":%d}%%`, lid)
	ResponseRegistry.OutgoingMessages <- payload
}

// SendSPLRefreshDefaultProductionLIDs requests **spl** for each LID in DefaultProductionSplLIDs (staggered)
// so barracks, workshops, refinery, toolsmith, and dragon queues can populate without opening each panel.
func SendSPLRefreshDefaultProductionLIDs() {
	go func() {
		time.Sleep(80 * time.Millisecond)
		for i, lid := range DefaultProductionSplLIDs {
			if i > 0 {
				time.Sleep(110 * time.Millisecond)
			}
			SendSPL(lid)
		}
	}()
}
