package GameParser

import (
	"encoding/json"
	"strings"

	"CitadelDesktop/Server/Automation"
)

func automationStateKeysForOpcode(opcode string) []string {
	opcode = strings.ToLower(strings.TrimSpace(opcode))
	keys := []string{Automation.StateOpcode(opcode)}
	switch opcode {
	case "lli":
		keys = append(keys, Automation.StateSession)
	case "jaa", "jca":
		keys = append(keys, Automation.StateFocus, Automation.StateCastles)
	case "gbd", "dcl", "gpc", "spl", "bup", "hru", "hdu", "rpc", "ubc", "crin", "crst", "crun", "crsk", "crca":
		keys = append(keys, Automation.StateCastles)
	case "gcu", "gmu", "ufa", "ufp", "gdi", "sce":
		keys = append(keys, Automation.StateResources)
	case "gam", "cat", "cra", "cds", "csm", "mcm", "mrm", "crm":
		keys = append(keys, Automation.StateMovement)
	case "gii", "gbc", "sin":
		keys = append(keys, Automation.StateInventory)
	case "kpi", "kgt", "msk", "cmi", "boi":
		keys = append(keys, Automation.StateTransport)
	case "gei", "ggm", "gli", "eqe":
		keys = append(keys, Automation.StateEquipment)
	case "ain":
		keys = append(keys, Automation.StateAlliance)
	}
	return keys
}

func automationStateKeysForFrame(opcode, payload string) []string {
	keys := automationStateKeysForOpcode(opcode)
	if strings.EqualFold(strings.TrimSpace(opcode), "ain") && payload != "" {
		var envelope struct {
			Alliance struct {
				AllianceID int `json:"AID"`
			} `json:"A"`
		}
		if json.Unmarshal([]byte(payload), &envelope) == nil && envelope.Alliance.AllianceID > 0 {
			keys = append(keys, Automation.StateEntity("alliance", envelope.Alliance.AllianceID))
		}
	}
	return keys
}
