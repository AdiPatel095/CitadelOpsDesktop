package State

import (
	"crypto/sha256"
	"encoding/json"
	"time"
)

type volatileFingerprints struct {
	value [32]byte
	known bool
}

// automationsPersistenceFingerprint hashes the automations component without its
// volatile display fields: NextCheckAt and UpdatedAt of every entry.
func automationsPersistenceFingerprint(automations map[string]AutomationState) ([32]byte, error) {
	copy := make(map[string]AutomationState, len(automations))
	for id, automation := range automations {
		automation.NextCheckAt = nil
		automation.UpdatedAt = time.Time{}
		copy[id] = automation
	}
	contents, err := json.Marshal(copy)
	if err != nil {
		return [32]byte{}, err
	}
	return sha256.Sum256(contents), nil
}
