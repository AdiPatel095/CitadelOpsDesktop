package GameParser

import (
	"CitadelDesktop/Server/Models"
	"CitadelDesktop/Server/ResponseRegistry"
	"encoding/json"
	"log"
)

func init() {
	ResponseRegistry.MessageRouterFunc = MessageRouter
}

// MessageRouter dispatches %-split game websocket frames to parsers.
func MessageRouter(messageParts []string) {
	cmd, ok := CommandType(messageParts)
	if !ok {
		return
	}

	ResponseRegistry.Global.CheckWaiters(cmd, messageParts)

	payload, hasPayload := Payload(messageParts)

	switch cmd {
	case "gbd":
		if !hasPayload {
			return
		}
		InitiateDetails(payload)
	case "gei":
		if !hasPayload {
			return
		}
		UpdateEquipmentStorage(payload)
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
	case "gam", "cat", "cra":
		if !hasPayload {
			return
		}
		ParseGAMMessage(payload)
	case "sdi":
		if !hasPayload {
			return
		}
		ParseSDIMessage(payload)
	case "gaa":
		if !hasPayload {
			return
		}
		ParseGAAMessage(payload)
	case "jaa":
		if !hasPayload {
			return
		}
		gs := Models.GetGameState()
		focusChanged := ApplyCastleFocusFromJAAPayload(gs, payload)
		buildingsChanged := ApplyJAABuildingRowsFromPayload(gs, payload)
		troopsChanged := ApplyJAATroopsFromPayload(gs, payload)
		if (focusChanged || buildingsChanged || troopsChanged) && NotifyCastleFocusChanged != nil {
			NotifyCastleFocusChanged()
		}
		markJAAProcessed()
	case "spl", "bup":
		if !hasPayload {
			return
		}
		gs := Models.GetGameState()
		if ApplySlotProductionFromSPLJSON(gs, payload) && NotifyCastleFocusChanged != nil {
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
	}
}
