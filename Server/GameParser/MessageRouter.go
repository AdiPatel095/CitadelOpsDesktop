package GameParser

import (
	"strings"

	"CitadelDesktop/Server/Models"
	gamestate "CitadelDesktop/Server/Models/GameState"
	"CitadelDesktop/Server/ResponseRegistry"
	"CitadelDesktop/Server/UTCTime"
	"encoding/json"
	"log"
)

func init() {
	ResponseRegistry.MessageRouterFunc = MessageRouter
}

// MessageRouter dispatches %-split game websocket frames to parsers.
// Waiters run after parsers (defer) so GameState matches the frame — e.g. **ain** updates BirdLocations
// before anything blocked on RegisterWaiter("ain") resumes; **cds** TT callbacks see consistent ordering.
func MessageRouter(messageParts []string) {
	ResponseRegistry.LogIncomingGameWireParts(messageParts)

	cmd, ok := CommandType(messageParts)
	if !ok {
		return
	}

	// Inbound frames often look like %xt%EmpireEx_21%<opcode>%1%<json> — index 2 is "EmpireEx_21" and the real opcode is index 3.
	effectiveCmd := cmd
	waiterCmd := cmd
	if cmd == "EmpireEx_21" && len(messageParts) > 3 {
		effectiveCmd = messageParts[3]
		waiterCmd = strings.ToLower(effectiveCmd)
	}

	payload, hasPayload := Payload(messageParts)

	defer ResponseRegistry.Global.CheckWaiters(waiterCmd, messageParts)

	switch strings.ToLower(effectiveCmd) {
	case "gbd":
		if !hasPayload {
			return
		}
		InitiateDetails(payload)
		ApplyGPCQueueableFromPayload(Models.GetGameState(), payload)
	case "gpc":
		if !hasPayload {
			return
		}
		ApplyGPCQueueableFromPayload(Models.GetGameState(), payload)
	case "gei":
		if !hasPayload {
			return
		}
		UpdateEquipmentStorage(payload)
	case "gcu":
		if !hasPayload {
			return
		}
		var gcuMap map[string]interface{}
		if err := json.Unmarshal([]byte(payload), &gcuMap); err == nil {
			UpdateCoins(gcuMap)
		}
	case "gmu":
		if !hasPayload {
			return
		}
		var gmuMap map[string]interface{}
		if err := json.Unmarshal([]byte(payload), &gmuMap); err == nil {
			UpdateMight(gmuMap)
			if NotifyGlobalResourcesChanged != nil {
				NotifyGlobalResourcesChanged()
			}
		}
	case "ufa":
		if !hasPayload {
			return
		}
		var ufaMap map[string]interface{}
		if err := json.Unmarshal([]byte(payload), &ufaMap); err == nil {
			UpdateGlory(ufaMap)
			if NotifyGlobalResourcesChanged != nil {
				NotifyGlobalResourcesChanged()
			}
		}
	case "ufp":
		if !hasPayload {
			return
		}
		var ufpMap map[string]interface{}
		if err := json.Unmarshal([]byte(payload), &ufpMap); err == nil {
			UpdateGallantry(ufpMap)
			if NotifyGlobalResourcesChanged != nil {
				NotifyGlobalResourcesChanged()
			}
		}
	case "gdi":
		if !hasPayload {
			return
		}
		var gdiMap map[string]interface{}
		if err := json.Unmarshal([]byte(payload), &gdiMap); err == nil && UpdateSelfPlayerSummary(gdiMap) {
			if NotifyGlobalResourcesChanged != nil {
				NotifyGlobalResourcesChanged()
			}
		}
	case "dcl":
		if !hasPayload {
			return
		}
		var dclMap map[string]interface{}
		if err := json.Unmarshal([]byte(payload), &dclMap); err != nil {
			log.Printf("[parser] dcl unmarshal: %v", err)
			return
		}
		if err := parseDCL(dclMap); err != nil {
			log.Printf("[parser] dcl parse: %v", err)
			return
		}
		if NotifyCastleFocusChanged != nil {
			NotifyCastleFocusChanged()
		}
	case "sce":
		if !hasPayload {
			return
		}
		var sceArray []interface{}
		if err := json.Unmarshal([]byte(payload), &sceArray); err == nil {
			UpdateSCE(sceArray)
		}
	case "sie", "upc":
		if !hasPayload {
			return
		}
		UpdateSubscriptionInfoFromPayload(payload)
	case "eqe":
		if !hasPayload {
			return
		}
		var eqeMap map[string]interface{}
		if err := json.Unmarshal([]byte(payload), &eqeMap); err == nil {
			ApplyEQEResponse(eqeMap)
		}
	case "ggm":
		if !hasPayload {
			return
		}
		UpdateGemStorage(payload)
	case "gli":
		if !hasPayload {
			return
		}
		var gliMap map[string]interface{}
		if err := json.Unmarshal([]byte(payload), &gliMap); err != nil {
			log.Printf("[parser] gli unmarshal: %v", err)
			return
		}
		UpdateEquipmentList(gliMap)
	case "ain":
		if !hasPayload {
			return
		}
		ParseAllianceInfo(payload)
	case "sne":
		if !hasPayload {
			return
		}
		HandleSNESharedBattleReports(payload)
		HandleSNESpyReports(payload)
	case "cra":
		if !hasPayload {
			return
		}
		ParseCRAResponse(messageParts, payload)
	case "gam", "cat", "csm":
		if !hasPayload {
			return
		}
		ParseGAMMessage(payload)
	case "mrm":
		// **mrm** is the game's remove-movement push ({"MID": n}) when a march ends; handling it frees
		// the commander promptly on return instead of waiting for the next **gam** snapshot. Confirmed
		// against live captures (websocket_game.log); **aam** is never pushed, so returns arrive via
		// **gam** (a D=1 leg) and are cleared by the follow-up empty **gam**.
		if !hasPayload {
			return
		}
		ParseMRMRemoveMovement(payload)
	case "gaa":
		if !hasPayload {
			return
		}
		ParseGAAMessage(payload)
	case "ssi":
		if !hasPayload {
			return
		}
		var response map[string]interface{}
		if json.Unmarshal([]byte(payload), &response) == nil {
			if gaa, ok := response["gaa"]; ok {
				if nested, err := json.Marshal(gaa); err == nil {
					ParseGAAMessage(string(nested))
				}
			}
		}
	case "gbc":
		if !hasPayload {
			return
		}
		rows, err := ParseGbcTrivialCIPLFromJSON(payload)
		if err != nil {
			return
		}
		gs := Models.GetGameState()
		grows := make([]gamestate.GbcCIPLRow, len(rows))
		for i := range rows {
			grows[i] = gamestate.GbcCIPLRow{PID: rows[i].PID, AMT: rows[i].AMT}
		}
		gs.ReplaceGbcTrivialCIPL(grows, utctime.Now().UnixMilli())
	case "sin":
		if !hasPayload {
			return
		}
		if m, err := ParseDecorationStorageCountsFromSINJSON(payload); err == nil && len(m) > 0 {
			Models.GetGameState().MergeSINItemCountsFromMap(m)
		}
	case "gii":
		if !hasPayload {
			return
		}
		// Payload may be "{}" on error paths; parser requires root "CI" array of pairs.
		if m, ok := ParseConstructionInventoryPairsFromRootJSON(payload); ok {
			Models.GetGameState().ReplaceCIInventoryCountsFromMap(m)
		}
	case "jaa":
		if !hasPayload {
			return
		}
		gs := Models.GetGameState()
		focusChanged := ApplyCastleFocusFromJAAPayload(gs, payload)
		buildingsChanged := ApplyJAABuildingRowsFromPayload(gs, payload)
		troopsChanged := ApplyJAATroopsFromPayload(gs, payload)
		constructionChanged := ApplyJAAConstructionSlotsFromPayload(gs, payload)
		slotProductionChanged := ApplyJAASlotProductionFromPayload(gs, payload)
		resourcesChanged := ApplyCastleResourceAmountsFromPayload(gs, payload)
		if (focusChanged || buildingsChanged || troopsChanged || constructionChanged || slotProductionChanged || resourcesChanged) && NotifyCastleFocusChanged != nil {
			NotifyCastleFocusChanged()
		}
		if m, ok := ParseEmbeddedSINStorageCountsFromEnvelopeJSON(payload); ok {
			gs.MergeSINItemCountsFromMap(m)
		}
		markJAAProcessed()
	case "jca":
		// A **jca** frame only arrives when a castle-focus was REJECTED by the server (a successful focus
		// is answered with **jaa**). Record it so trySendAndAwaitJAA fails this attempt instead of acting
		// on the stale optimistic focus. Verified in websocket_game.log: success => jaa code 0;
		// rejected focus => jca code != 0 (observed code 6).
		if code, ok := empireExResponseCode(messageParts); !ok || code != 0 {
			markJCAError()
		}
	case "rpc", "ubc":
		if !hasPayload {
			return
		}
		gs := Models.GetGameState()
		if ApplyFocusCastleConstructionCIFromPayload(gs, payload) && NotifyCastleFocusChanged != nil {
			NotifyCastleFocusChanged()
		}
	case "spl", "bup":
		if !hasPayload {
			return
		}
		gs := Models.GetGameState()
		var root map[string]interface{}
		if err := json.Unmarshal([]byte(payload), &root); err == nil {
			UpdateCoinsFromPayload(root)
			UpdateSCEFromPayload(root)
		}
		slotProductionChanged := ApplySlotProductionFromSPLJSON(gs, payload)
		resourcesChanged := ApplyCastleResourceAmountsFromPayload(gs, payload)
		if (slotProductionChanged || resourcesChanged) && NotifyCastleFocusChanged != nil {
			NotifyCastleFocusChanged()
		}
	case "hru", "hdu":
		if !hasPayload {
			return
		}
		gs := Models.GetGameState()
		var root map[string]interface{}
		if err := json.Unmarshal([]byte(payload), &root); err == nil {
			UpdateCoinsFromPayload(root)
			UpdateSCEFromPayload(root)
		}
		troopsChanged := ApplyFocusedCastleTroopsFromPayload(gs, payload)
		slotProductionChanged := ApplySlotProductionFromSPLJSON(gs, payload)
		resourcesChanged := ApplyCastleResourceAmountsFromPayload(gs, payload)
		if (troopsChanged || slotProductionChanged || resourcesChanged) && NotifyCastleFocusChanged != nil {
			NotifyCastleFocusChanged()
		}
	case "crin":
		if !hasPayload {
			return
		}
		gs := Models.GetGameState()
		if ApplyCraftingFromCRINJSON(gs, payload) && NotifyCastleFocusChanged != nil {
			NotifyCastleFocusChanged()
		}
	case "crst":
		if !hasPayload {
			return
		}
		gs := Models.GetGameState()
		if ApplyCraftingFromCRSTJSON(gs, payload) && NotifyCastleFocusChanged != nil {
			NotifyCastleFocusChanged()
		}
	case "fuc":
		if !hasPayload {
			return
		}
		gs := Models.GetGameState()
		if gs == nil {
			return
		}
		unitWID := 0
		if st := Models.GetSettingsState(); st != nil {
			unitWID = st.AutoBeriWorld.TransferTroopWID
		}
		if res, ok := ParseFucTroopCheckResponse(payload, unitWID); ok {
			gs.SetAutoBeriWorldFucResult(res.TroopAmount, res.ParsedSCID)
		}
	}
}
