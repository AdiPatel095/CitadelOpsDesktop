// Package GameCommands queues raw %%xt%%EmpireEx_21%%…%% payloads onto ResponseRegistry.OutgoingMessages.
// The browser/game client must be connected; these mirror what the live Flash/HTML5 client sends.
//
// Conventions:
//   - Kingdom IDs (KID): 0 main, 1 desert, 2 ice, 3 fire, 4 storm, 10 beri world, etc.
//   - Castle instance id (CID/AID): per-player castle id used in sob/jca payloads.
//   - WID (wodID): global building/decoration type id from the items catalog — not the per-castle OID.
//
// Pattern (every command follows this so declarations and call sites stay uniform):
//   - empireExFrame(op, jsonBody) builds the wire envelope %xt%<token>%<op>%1%<body>% in one place.
//   - <OP>Payload(...) is a pure builder that returns the wire string (no side effects).
//   - Send<OP>(...) queues that string via QueueOutgoingPayload — the single outbound entry point.
//     A few opcodes are intentionally special and documented inline: sin (no JSON body) and aec (no token).
//
// Navigation before build/pickup: the game applies EBU/SOB to whichever castle view is focused.
// Use SendJCA or SendJAA so the client is on the target castle before SendEBU / SendSOB.
package GameCommands

import (
	"CitadelDesktop/Server/ResponseRegistry"
	"fmt"
)

// empireExFrame builds the standard EmpireEx command envelope: %xt%<token>%<op>%1%<jsonBody>%.
// jsonBody is the raw JSON object/array string (e.g. `{}` or `{"CID":1}`). The dynamic
// ResponseRegistry.EmpireExToken is injected here so individual builders never hard-code it.
func empireExFrame(op, jsonBody string) string {
	return fmt.Sprintf(`%%xt%%%s%%%s%%1%%%s%%`, ResponseRegistry.EmpireExToken, op, jsonBody)
}

// QueueOutgoingPayload enqueues a raw payload onto the single game websocket outbound queue.
// Every Send* function routes through here so there is one place that touches OutgoingMessages.
func QueueOutgoingPayload(payload string) {
	ResponseRegistry.OutgoingMessages <- []byte(payload)
}

// --- Focus / navigation ---

// JCAPayload builds EmpireEx_21 **jca** — jump/focus a castle by castle id and kingdom.
//
// Payload: {"CID":<castleInstanceId>,"KID":<kingdomId>}
//
// Use for kingdoms that address castles by CID (typically not KID 0/4/10; align with troop-fetch logic).
func JCAPayload(cid, kid int) string {
	return empireExFrame("jca", fmt.Sprintf(`{"CID":%d,"KID":%d}`, cid, kid))
}

// JAAPayload builds EmpireEx_21 **jaa** — focus map tile / area (main map, storm, beri, etc.).
//
// Payload: {"PX":<mapX>,"PY":<mapY>,"KID":<kingdomId>}
//
// Use when the game expects map coordinates (e.g. KID 0, 4, 10) instead of JCA.
func JAAPayload(px, py, kid int) string {
	return empireExFrame("jaa", fmt.Sprintf(`{"PX":%d,"PY":%d,"KID":%d}`, px, py, kid))
}

// SendJCA queues **jca** — focus a castle by CID/KID.
func SendJCA(cid int, kid int) {
	QueueOutgoingPayload(JCAPayload(cid, kid))
}

// SendJAA queues **jaa** — focus a map tile by PX/PY/KID.
func SendJAA(px int, py int, kid int) {
	QueueOutgoingPayload(JAAPayload(px, py, kid))
}

// GAAPayload builds EmpireEx_21 **gaa** — request map tiles for a rectangular viewport (inbound **gaa** response).
//
// Wire: %xt%EmpireEx_21%gaa%1%{"KID":0,"AX1":1196,"AY1":1144,"AX2":1208,"AY2":1156}%
func GAAPayload(kid, ax1, ay1, ax2, ay2 int) string {
	return empireExFrame("gaa", fmt.Sprintf(
		`{"KID":%d,"AX1":%d,"AY1":%d,"AX2":%d,"AY2":%d}`, kid, ax1, ay1, ax2, ay2))
}

// SendGAAViewport requests map tiles for a rectangular viewport (**gaa**).
func SendGAAViewport(kid, ax1, ay1, ax2, ay2 int) {
	QueueOutgoingPayload(GAAPayload(kid, ax1, ay1, ax2, ay2))
}

// SendGAAAroundTile requests GAA for a Chebyshev square around (x,y) with the given padding (min 1).
func SendGAAAroundTile(kid, x, y, padding int) {
	if padding < 1 {
		padding = 1
	}
	SendGAAViewport(kid, x-padding, y-padding, x+padding, y+padding)
}

// CastleFocusCommand returns the same JAA/JCA payload string used for troop fetching and castle focus
// (KID 0, 4, 10 → JAA with map coords; otherwise JCA). Use with alternate outgoing channels if needed.
func CastleFocusCommand(kingdomID, castleID, mapX, mapY int) string {
	if kingdomID == 0 || kingdomID == 4 || kingdomID == 10 {
		return JAAPayload(mapX, mapY, kingdomID)
	}
	return JCAPayload(castleID, kingdomID)
}

// SendCastleFocus sends JAA or JCA per kingdom rules so the client focuses the target castle.
func SendCastleFocus(kingdomID, castleID, mapX, mapY int) {
	QueueOutgoingPayload(CastleFocusCommand(kingdomID, castleID, mapX, mapY))
}

// --- Build / decoration / storage ---

// EBUPayload builds EmpireEx_21 **ebu** — place/erect a building or decoration at grid coordinates.
//
// JSON shape (verified vs live client captures):
//
//	{"WID":<globalType>,"X":<grid>,"Y":<grid>,"R":0,"PWR":0,"PO":-1,"DOID":-1}
//
// WID is the global type id (same for all players). The payload does not include CID; the focused
// castle in the client receives the command. Some third-party docs use "BT" for build type; this client uses "WID".
func EBUPayload(wid, gridX, gridY int) string {
	return EBUWithParamsPayload(wid, gridX, gridY, 0, 0, -1, -1)
}

// EBUWithParamsPayload is EBUPayload with explicit rotation/power/public-order/decoration-owner fields
// when a capture from your client differs from the default EBUPayload constants.
func EBUWithParamsPayload(wid, gridX, gridY, r, pwr, po, doid int) string {
	return empireExFrame("ebu", fmt.Sprintf(
		`{"WID":%d,"X":%d,"Y":%d,"R":%d,"PWR":%d,"PO":%d,"DOID":%d}`, wid, gridX, gridY, r, pwr, po, doid))
}

// SendEBU queues **ebu** with default rotation/power/order fields.
func SendEBU(wid, gridX, gridY int) {
	QueueOutgoingPayload(EBUPayload(wid, gridX, gridY))
}

// SendEBUWithParams queues **ebu** with explicit rotation/power/public-order/decoration-owner fields.
func SendEBUWithParams(wid, gridX, gridY, r, pwr, po, doid int) {
	QueueOutgoingPayload(EBUWithParamsPayload(wid, gridX, gridY, r, pwr, po, doid))
}

// SINPayload builds EmpireEx_21 **sin** — refresh decoration/building storage inventory (response lists RD rows per SID).
// Special case: sin carries NO JSON body, so it is the bare frame %xt%<token>%sin%1% (no trailing {} and no extra %).
// Response shape: Logs/JSONExamples/sin.json (array of {SID, RD, …}; each RD row [wodID, amount, …]).
func SINPayload() string {
	return fmt.Sprintf(`%%xt%%%s%%sin%%1%%`, ResponseRegistry.EmpireExToken)
}

// SendSIN queues **sin** — refresh decoration/building storage inventory.
func SendSIN() {
	QueueOutgoingPayload(SINPayload())
}

// GIIPayload builds EmpireEx_21 **gii** — request construction-item inventory (response root **CI** as [[wireCID,count],…]).
// Live shape: %xt%EmpireEx_21%gii%1%{}%
func GIIPayload() string {
	return empireExFrame("gii", "{}")
}

// SendGII queues **gii** — request construction-item inventory.
func SendGII() {
	QueueOutgoingPayload(GIIPayload())
}

// SOBPayload builds EmpireEx_21 **sob** — pick up an existing instance into inventory / storage.
//
// Payload: {"CID":<castleInstanceId>,"OID":<perCastleBuildingInstanceId>}
//
// OID comes from gca BG / Models.BuildingData.OID (not WID). If your captures use a different key than "OID", adjust here.
func SOBPayload(castleAID, castleSpecificBuildingID int) string {
	return empireExFrame("sob", fmt.Sprintf(`{"CID":%d,"OID":%d}`, castleAID, castleSpecificBuildingID))
}

// SendSOB queues **sob** — pick up an existing building/decoration instance.
func SendSOB(castleAID, castleSpecificBuildingID int) {
	QueueOutgoingPayload(SOBPayload(castleAID, castleSpecificBuildingID))
}

// --- Barracks / troops ---

// BUPPayload builds EmpireEx_21 **bup** — enqueue unit training at the barracks / production strip.
//
// Live client shape: {"LID":0,"WID":<unitType>,"AMT":<count>,"PO":-1,"PWR":0,"SK":<sessionKey>,"SID":0,"AID":<castleInstanceId>}
//
// WID is the global unit type id (not a building WOD). SK must match the current game session (from a browser capture for your account).
func BUPPayload(lid, unitWID, amount, po, pwr, sk, sid, castleAID int) string {
	return empireExFrame("bup", fmt.Sprintf(
		`{"LID":%d,"WID":%d,"AMT":%d,"PO":%d,"PWR":%d,"SK":%d,"SID":%d,"AID":%d}`,
		lid, unitWID, amount, po, pwr, sk, sid, castleAID))
}

// SendBarracksUnitPurchase queues **bup** — enqueue unit training.
func SendBarracksUnitPurchase(lid, unitWID, amount, po, pwr, sk, sid, castleAID int) {
	QueueOutgoingPayload(BUPPayload(lid, unitWID, amount, po, pwr, sk, sid, castleAID))
}

// --- Movement (bird dispatch) ---

// SDIPayload builds EmpireEx_21 **sdi** — select/preview a bird dispatch route.
//
// Payload: {"TX":<targetX>,"TY":<targetY>,"SX":<sourceX>,"SY":<sourceY>}
func SDIPayload(targetX, targetY, sourceX, sourceY int) string {
	return empireExFrame("sdi", fmt.Sprintf(
		`{"TX":%d,"TY":%d,"SX":%d,"SY":%d}`, targetX, targetY, sourceX, sourceY))
}

// CDSPayload builds EmpireEx_21 **cds** — send a bird movement with a troop batch.
//
// troopsJSON must be a valid JSON array string for "A", e.g. [[unitId,amount],...].
// JSON shape:
//
//	{"SID":<castleAID>,"TX":<targetX>,"TY":<targetY>,"LID":<sdiLID>,"WT":<delayHours>,
//	 "HBW":<hbw>,"BPC":1,"PTT":<ptt>,"SD":0,"A":<troopsJSON>}
//	 Valid (HBW,PTT) pairs: see CDSVariants.go (HBW=0,PTT=0 → code 5 in logs; app uses 1001/0 and -1/1).
func CDSPayload(castleAID, targetX, targetY, sdiLID, delayHours, hbw, ptt int, troopsJSON string) string {
	return empireExFrame("cds", fmt.Sprintf(
		`{"SID":%d,"TX":%d,"TY":%d,"LID":%d,"WT":%d,"HBW":%d,"BPC":1,"PTT":%d,"SD":0,"A":%s}`,
		castleAID, targetX, targetY, sdiLID, delayHours, hbw, ptt, troopsJSON))
}

// GAMPayload builds EmpireEx_21 **gam** — request active troop movements.
func GAMPayload() string {
	return empireExFrame("gam", "{}")
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

// --- Alliance ---

// AINPayload builds EmpireEx_21 **ain** — alliance info refresh (bird targets, etc.).
func AINPayload(allianceAID int) string {
	return empireExFrame("ain", fmt.Sprintf(`{"AID":%d}`, allianceAID))
}

// SendAIN queues **ain** — alliance info refresh. AID from alliance state; no-op for non-positive AID.
func SendAIN(allianceAID int) {
	if allianceAID <= 0 {
		return
	}
	QueueOutgoingPayload(AINPayload(allianceAID))
}

// --- Equipment / gems ---

// EEQPayload builds EmpireEx_21 **eeq** — equip/unequip an equipment item.
//
// equip=true -> E=1 (equip), equip=false -> E=0 (unequip).
// Payload: {"EID":<equipmentId>,"LID":<leaderId>,"E":<0|1>}
func EEQPayload(equipmentID, leaderID float64, equip bool) string {
	eFlag := 0
	if equip {
		eFlag = 1
	}
	return empireExFrame("eeq", fmt.Sprintf(`{"EID":%.0f,"LID":%.0f,"E":%d}`, equipmentID, leaderID, eFlag))
}

// BGEPayload builds EmpireEx_21 **bge** — attach/equip a gem to a specific equipment item.
//
// Payload: {"GID":<gemId>,"EID":<equipmentId>,"LID":<leaderId>,"M":0,"RGEM":1}
func BGEPayload(gemID, equipmentID, leaderID float64) string {
	return empireExFrame("bge", fmt.Sprintf(
		`{"GID":%.0f,"EID":%.0f,"LID":%.0f,"M":0,"RGEM":1}`, gemID, equipmentID, leaderID))
}

// EGEPayload builds EmpireEx_21 **ege** — remove a gem from an equipped item.
//
// Payload: {"EID":<equipmentId>,"LID":<leaderId>}
func EGEPayload(equipmentID, leaderID float64) string {
	return empireExFrame("ege", fmt.Sprintf(`{"EID":%.0f,"LID":%.0f}`, equipmentID, leaderID))
}

// EREPayload builds EmpireEx_21 **ere** — upgrade one level on equipment/hero (EQ=1) or gem (EQ=0).
// RIID is the instance id (equipment EID or gem GID). C2 selects cost currency (0 = default from captures).
func EREPayload(riid float64, eqFlag, c2 int) string {
	return empireExFrame("ere", fmt.Sprintf(`{"C2":%d,"RIID":%.0f,"EQ":%d}`, c2, riid, eqFlag))
}

// GEIPayload builds EmpireEx_21 **gei** — refresh equipment inventory from the game server.
func GEIPayload() string {
	return empireExFrame("gei", "{}")
}

// GGMPayload builds EmpireEx_21 **ggm** — refresh gem storage.
func GGMPayload() string {
	return empireExFrame("ggm", "{}")
}

// GLIPayload builds EmpireEx_21 **gli** — equipment list (e.g. before reconfigure / GLI parsers).
func GLIPayload() string {
	return empireExFrame("gli", "{}")
}

// GNRPayload builds EmpireEx_21 **gnr** — open the equipment/gem upgrade menu shell.
func GNRPayload() string {
	return empireExFrame("gnr", "{}")
}

// SEQPayload builds EmpireEx_21 **seq** — sell one stash equipment by instance EID.
func SEQPayload(equipmentID float64) string {
	return empireExFrame("seq", fmt.Sprintf(`{"EID":%.0f,"LID":-1,"EX":0,"LFID":-1}`, equipmentID))
}

// SGENonRelicGemPayload builds **sge** for non-relic gems (RGEM 0, GID zero-padded to 3 digits).
func SGENonRelicGemPayload(gemID float64) string {
	return empireExFrame("sge", fmt.Sprintf(`{"GID":%03.0f,"RGEM":0,"LFID":-1}`, gemID))
}

// SGERelicGemPayload builds **sge** for relic gems (RGEM 1).
func SGERelicGemPayload(gemID float64) string {
	return empireExFrame("sge", fmt.Sprintf(`{"GID":%.0f,"RGEM":1,"LFID":-1}`, gemID))
}

// SendGEI requests **gei** — refresh equipment inventory.
func SendGEI() {
	QueueOutgoingPayload(GEIPayload())
}

// SendGGM requests **ggm** — refresh gem storage.
func SendGGM() {
	QueueOutgoingPayload(GGMPayload())
}

// SendGLI requests **gli** — equipment list.
func SendGLI() {
	QueueOutgoingPayload(GLIPayload())
}

// SendGNR requests **gnr** — open the upgrade menu shell.
func SendGNR() {
	QueueOutgoingPayload(GNRPayload())
}

// SendUpgradeMenuRefresh queues **gnr**, **ggm**, **gei**, **gli** — load upgrade UI inventory (live game sequence).
func SendUpgradeMenuRefresh() {
	SendGNR()
	SendGGM()
	SendGEI()
	SendGLI()
}

// SendERE queues **ere** — single equipment or gem level upgrade.
func SendERE(riid float64, eqFlag, c2 int) {
	QueueOutgoingPayload(EREPayload(riid, eqFlag, c2))
}

// SendEEQ queues **eeq** using EEQPayload (equip or unequip).
func SendEEQ(equipmentID, leaderID float64, equip bool) {
	QueueOutgoingPayload(EEQPayload(equipmentID, leaderID, equip))
}

// SendEGE queues **ege** — remove gem from parent equipment.
func SendEGE(equipmentID, leaderID float64) {
	QueueOutgoingPayload(EGEPayload(equipmentID, leaderID))
}

// SendBGE queues **bge** — attach gem to equipment.
func SendBGE(gemID, equipmentID, leaderID float64) {
	QueueOutgoingPayload(BGEPayload(gemID, equipmentID, leaderID))
}

// SendSEQSellEquipment queues **seq** for one equipment row.
func SendSEQSellEquipment(equipmentID float64) {
	QueueOutgoingPayload(SEQPayload(equipmentID))
}

// SendSGENonRelicGem queues **sge** with RGEM 0 (non-relic sell path).
func SendSGENonRelicGem(gemID float64) {
	QueueOutgoingPayload(SGENonRelicGemPayload(gemID))
}

// SendSGERelicGem queues **sge** with RGEM 1 (relic gem sell path).
func SendSGERelicGem(gemID float64) {
	QueueOutgoingPayload(SGERelicGemPayload(gemID))
}

// --- Construction items / trivial shop (TCI) ---

// RPCPayload builds EmpireEx_21 **rpc** — equip a construction item (CID) on a building (OID) for a castle.
// Live shape: {"OID":<bldInstance>,"CID":<constructionItemID>,"SID":0,"M":0,"KID":<k>,"AID":<castleInstance>}
func RPCPayload(oid, cid, slotID, m, kid, aid int) string {
	return empireExFrame("rpc", fmt.Sprintf(
		`{"OID":%d,"CID":%d,"SID":%d,"M":%d,"KID":%d,"AID":%d}`, oid, cid, slotID, m, kid, aid))
}

// SendRPC queues **rpc** — equip construction item.
func SendRPC(oid, cid, slotID, m, kid, aid int) {
	QueueOutgoingPayload(RPCPayload(oid, cid, slotID, m, kid, aid))
}

// UBCPayload builds EmpireEx_21 **ubc** — upgrade an equipped TCI. SUC is a client offer/session code (e.g. 2001, 2002 from captures).
// Shape: {"OID":<bld>,"SUC":<code>,"SID":0,"KID":<k>,"AID":<castle>,"CID":<currentCid>}
func UBCPayload(oid, suc, slotID, kid, aid, cid int) string {
	return empireExFrame("ubc", fmt.Sprintf(
		`{"OID":%d,"SUC":%d,"SID":%d,"KID":%d,"AID":%d,"CID":%d}`, oid, suc, slotID, kid, aid, cid))
}

// SendUBC queues **ubc** — upgrade construction item in slot.
func SendUBC(oid, suc, slotID, kid, aid, cid int) {
	QueueOutgoingPayload(UBCPayload(oid, suc, slotID, kid, aid, cid))
}

// AECPayload is the exact wire frame to open the construction-item menu.
// Special case: aec uses no EmpireEx token and a literal []-body — %xt%aec%1%0%[]% — so it does not use empireExFrame.
func AECPayload() string {
	return "%xt%aec%1%0%[]%"
}

// SendAEC queues **aec** — open the construction-item menu shell before gbc/sbp flows.
func SendAEC() {
	QueueOutgoingPayload(AECPayload())
}

// GBCPayload builds EmpireEx_21 **gbc** — open the trivial-CI purchase list for a castle.
// Here CID is the castle instance id (AID), not a constructionItemID.
func GBCPayload(castleInstanceID, kid int) string {
	return empireExFrame("gbc", fmt.Sprintf(`{"CID":%d,"KID":%d}`, castleInstanceID, kid))
}

// SendGBC queues **gbc** — open purchase offers (response contains PL: PID+AMT rows).
func SendGBC(castleInstanceID, kid int) {
	QueueOutgoingPayload(GBCPayload(castleInstanceID, kid))
}

// SBPPayload builds EmpireEx_21 **sbp** — buy from the gbc list (product id, amount, type).
// Shape from captures: {"PID":<product>,"BT":0,"TID":116,"AMT":<n>,"KID":0,"AID":-1,"PC2":-1,"BA":0,"PWR":0,"_PO":-1}
func SBPPayload(pid, bt, tid, amt, kid, aid, pc2, ba, pwr, po int) string {
	return empireExFrame("sbp", fmt.Sprintf(
		`{"PID":%d,"BT":%d,"TID":%d,"AMT":%d,"KID":%d,"AID":%d,"PC2":%d,"BA":%d,"PWR":%d,"_PO":%d}`,
		pid, bt, tid, amt, kid, aid, pc2, ba, pwr, po))
}

// SendSBP queues **sbp** — complete a trivial purchase after gbc.
func SendSBP(pid, bt, tid, amt, kid, aid, pc2, ba, pwr, po int) {
	QueueOutgoingPayload(SBPPayload(pid, bt, tid, amt, kid, aid, pc2, ba, pwr, po))
}

// --- Battle reports ---

// BLSPayload builds EmpireEx **bls** - request the summary for a battle report message id.
//
// The MID comes from SNE MSG row[0]. IM is 0 for shared battle reports observed in captures.
func BLSPayload(mid int64, im int) string {
	return empireExFrame("bls", fmt.Sprintf(`{"MID":%d,"IM":%d}`, mid, im))
}

// SendBLS queues **bls** - load battle-report summary by message id.
func SendBLS(mid int64, im int) {
	QueueOutgoingPayload(BLSPayload(mid, im))
}

// BLMPayload builds EmpireEx **blm** - request per-wave battle aggregates by report LID.
//
// The LID is returned in the BLS payload.
func BLMPayload(lid int64) string {
	return empireExFrame("blm", fmt.Sprintf(`{"LID":%d}`, lid))
}

// SendBLM queues **blm** - load battle-report per-wave aggregates.
func SendBLM(lid int64) {
	QueueOutgoingPayload(BLMPayload(lid))
}

// BLDPayload builds EmpireEx **bld** - request detailed battle-report units/tools by report LID.
//
// The LID is returned in the BLS payload.
func BLDPayload(lid int64) string {
	return empireExFrame("bld", fmt.Sprintf(`{"LID":%d}`, lid))
}

// SendBLD queues **bld** - load battle-report detailed rows.
func SendBLD(lid int64) {
	QueueOutgoingPayload(BLDPayload(lid))
}

// --- Misc / tooling ---

// SendEmpireEx21EmptyCommand queues **EmpireEx_21%<code>%1%{}%** (custom / debug tooling).
func SendEmpireEx21EmptyCommand(messageCode string) {
	if messageCode == "" {
		return
	}
	QueueOutgoingPayload(empireExFrame(messageCode, "{}"))
}
