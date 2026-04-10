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

// QueueOutgoingPayload enqueues a raw payload onto the single game websocket outbound queue.
func QueueOutgoingPayload(payload string) {
	ResponseRegistry.OutgoingMessages <- []byte(payload)
}

// SDIPayload returns EmpireEx_21 **sdi** — select/preview a bird dispatch route.
//
// Payload: {"TX":<targetX>,"TY":<targetY>,"SX":<sourceX>,"SY":<sourceY>}
func SDIPayload(targetX, targetY, sourceX, sourceY int) string {
	return fmt.Sprintf(
		`%%xt%%EmpireEx_21%%sdi%%1%%{"TX":%d,"TY":%d,"SX":%d,"SY":%d}%%`,
		targetX, targetY, sourceX, sourceY,
	)
}

// CDSPayload returns EmpireEx_21 **cds** — send a bird movement with a troop batch.
//
// troopsJSON must be a valid JSON array string for "A", e.g. [[unitId,amount],...].
// JSON shape:
//
//	{"SID":<castleAID>,"TX":<targetX>,"TY":<targetY>,"LID":<sdiLID>,"WT":<delayHours>,
//	 "HBW":<hbw>,"BPC":1,"PTT":<ptt>,"SD":0,"A":<troopsJSON>}
//	 Valid (HBW,PTT) pairs: see CDSVariants.go (HBW=0,PTT=0 → code 5 in logs; app uses 1001/0 and -1/1).
func CDSPayload(castleAID, targetX, targetY, sdiLID, delayHours, hbw, ptt int, troopsJSON string) string {
	return fmt.Sprintf(
		`%%xt%%EmpireEx_21%%cds%%1%%{"SID":%d,"TX":%d,"TY":%d,"LID":%d,"WT":%d,"HBW":%d,"BPC":1,"PTT":%d,"SD":0,"A":%s}%%`,
		castleAID, targetX, targetY, sdiLID, delayHours, hbw, ptt, troopsJSON,
	)
}

// GAMPayload returns EmpireEx_21 **gam** — request active troop movements.
func GAMPayload() string {
	return `%xt%EmpireEx_21%gam%1%{}%`
}

// SendGAM requests active movements (**gam**). Parsed into GameState.Movement.ActiveMovements.
func SendGAM() {
	QueueOutgoingPayload(GAMPayload())
}

// SendSDI queues **sdi** — preview bird route source → target map tiles.
func SendSDI(targetX, targetY, sourceX, sourceY int) {
	QueueOutgoingPayload(SDIPayload(targetX, targetY, sourceX, sourceY))
}

// SendCDS queues **cds** — dispatch bird with troop batch **A** (JSON array string).
func SendCDS(castleAID, targetX, targetY, sdiLID, delayHours, hbw, ptt int, troopsJSON string) {
	QueueOutgoingPayload(CDSPayload(castleAID, targetX, targetY, sdiLID, delayHours, hbw, ptt, troopsJSON))
}

// SendAIN queues **ain** — alliance info refresh (bird targets, etc.). AID from alliance state.
func SendAIN(allianceAID int) {
	if allianceAID <= 0 {
		return
	}
	payload := fmt.Sprintf(`%%xt%%EmpireEx_21%%ain%%1%%{"AID":%d}%%`, allianceAID)
	QueueOutgoingPayload(payload)
}

// EEQPayload returns EmpireEx_21 **eeq** — equip/unequip an equipment item.
//
// equip=true -> E=1 (equip), equip=false -> E=0 (unequip).
// Payload: {"EID":<equipmentId>,"LID":<leaderId>,"E":<0|1>}
func EEQPayload(equipmentID, leaderID float64, equip bool) string {
	eFlag := 0
	if equip {
		eFlag = 1
	}
	return fmt.Sprintf(`%%xt%%EmpireEx_21%%eeq%%1%%{"EID":%.0f,"LID":%.0f,"E":%d}%%`, equipmentID, leaderID, eFlag)
}

// BGEPayload returns EmpireEx_21 **bge** — attach/equip a gem to a specific equipment item.
//
// Payload: {"GID":<gemId>,"EID":<equipmentId>,"LID":<leaderId>,"M":0,"RGEM":1}
func BGEPayload(gemID, equipmentID, leaderID float64) string {
	return fmt.Sprintf(`%%xt%%EmpireEx_21%%bge%%1%%{"GID":%.0f,"EID":%.0f,"LID":%.0f,"M":0,"RGEM":1}%%`, gemID, equipmentID, leaderID)
}

// EGEPayload returns EmpireEx_21 **ege** — remove a gem from an equipped item.
//
// Payload: {"EID":<equipmentId>,"LID":<leaderId>}
func EGEPayload(equipmentID, leaderID float64) string {
	return fmt.Sprintf(`%%xt%%EmpireEx_21%%ege%%1%%{"EID":%.0f,"LID":%.0f}%%`, equipmentID, leaderID)
}
