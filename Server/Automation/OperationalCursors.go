package Automation

import (
	"strings"

	"CitadelDesktop/Server/State"
)

func operationalCursor(state State.GameState, policyID string, key string) (int, bool) {
	policyID = strings.TrimSpace(policyID)
	key = strings.TrimSpace(key)
	if policyID == "" || key == "" {
		return 0, false
	}
	automation, exists := state.Automations[policyID]
	if !exists || automation.OperationalCursors == nil {
		return 0, false
	}
	value, exists := automation.OperationalCursors[key]
	return value, exists
}

func productionOperationalCursorKey(castleKey string) string {
	return "castle/" + strings.TrimSpace(castleKey)
}

func craftingOperationalCursorKey(castleKey string, queueKey string) string {
	return "castle/" + strings.TrimSpace(castleKey) + "/queue/" + strings.TrimSpace(queueKey)
}
