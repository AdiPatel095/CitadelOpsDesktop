package GameParser

import (
	"CitadelDesktop/Server/Models"
	"CitadelDesktop/Server/ResponseRegistry"
	"encoding/json"
)

func init() {
	ResponseRegistry.MessageRouterFunc = MessageRouter
}

func MessageRouter(messageParts []string) {
	messageType := messageParts[2]

	// Check if any waiters are registered for this message type
	ResponseRegistry.Global.CheckWaiters(messageType, messageParts)

	indexedList := []string{"cra", "cds", "jaa", "cat", "gam", "gie", "gbl", "dcl", "gcu", "gmu", "gpa", "grc", "sce", "gbd", "sei", "mcm", "gaa", "sti"}
	if contains(indexedList, messageType) {
		//log.Printf("Received message type: %s which has already been indexed", messageType)
	} else if !contains(indexedList, messageType) {
		//log.Printf("Received message type: %s, full message: %v", messageType, messageParts)
	}
	if messageType == "gbd" {
		InitiateDetails(messageParts[5])
	}
	if messageType == "gei" {
		UpdateEquipmentStorage(messageParts[5])
	}
	if messageType == "ggm" {
		UpdateGemStorage(messageParts[5])
	}

	if messageType == "gli" {
		var gliMap map[string]interface{}
		_ = json.Unmarshal([]byte(messageParts[5]), &gliMap)
		UpdateEquipmentList(gliMap)
	}
	if messageType == "ain" {
		ParseAllianceInfo(messageParts[5])
	}
	if messageType == "gam" || messageType == "cat" || messageType == "cds" || messageType == "cra" {
		ParseGAMMessage(messageParts[5])
	}
	if messageType == "gaa" {
		ParseGAAMessage(messageParts[5])
	}
	// jaa: update CastleFocus + parse gca.BG / gca.BD via BuildingParser (ParseBGFromJAAResponseJSON, ParseBDFromJAAResponseJSON) in JaaCastleFocus.ApplyJAABuildingRowsFromPayload.
	if messageType == "jaa" && len(messageParts) > 5 {
		data := messageParts[5]
		gs := Models.GetGameState()
		focusChanged := ApplyCastleFocusFromJAAPayload(gs, data)
		buildingsChanged := ApplyJAABuildingRowsFromPayload(gs, data)
		if focusChanged || buildingsChanged {
			if NotifyCastleFocusChanged != nil {
				NotifyCastleFocusChanged()
			}
		}
	}

	// spl / bup: barracks-style production queue (PS active, QS[].P queued); bup nests the same under "spl".
	if (messageType == "spl" || messageType == "bup") && len(messageParts) > 5 {
		gs := Models.GetGameState()
		if ApplySlotProductionFromSPLJSON(gs, messageParts[5]) {
			if NotifyCastleFocusChanged != nil {
				NotifyCastleFocusChanged()
			}
		}
	}

	// crin / crst: sovereign crafting — PS/QS use CRID (or RUT when CRID empty); CAI may be object {CBI} or array; optional **crai** wrapper (same as jaa).
	if (messageType == "crin" || messageType == "crst") && len(messageParts) > 5 {
		gs := Models.GetGameState()
		changed := false
		if messageType == "crin" {
			changed = ApplyCraftingFromCRINJSON(gs, messageParts[5])
		} else {
			changed = ApplyCraftingFromCRSTJSON(gs, messageParts[5])
		}
		if changed && NotifyCastleFocusChanged != nil {
			NotifyCastleFocusChanged()
		}
	}

}
