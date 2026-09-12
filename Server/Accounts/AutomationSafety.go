package Accounts

import (
	"fmt"
	"os"
	"time"

	"CitadelDesktop/Server/State"
)

// Adopting an older player corpus must never drop an active staging lock.
// Existing indefinite locks win; otherwise retain the stricter/latest expiry.
// Reviews from a different profile never implicitly clear an active lock.
func mergeAutomationSafetyLocks(staging, player string) error {
	source, err := State.LoadSnapshot(staging)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("load staging safety state: %w", err)
	}
	now := time.Now().UTC()
	active := false
	for _, automation := range source.Automations {
		active = active || automation.SafetyLock.Active(now)
	}
	if !active {
		return nil
	}
	destination, err := State.LoadSnapshot(player)
	if os.IsNotExist(err) {
		destination = State.NewGameState()
	} else if err != nil {
		return fmt.Errorf("load player safety state: %w", err)
	}
	store := State.NewStore(destination)
	event, err := store.ApplyComponents(State.Components(State.ComponentAutomations), func(state *State.GameState) ([]string, bool, error) {
		changed := false
		for lane, incoming := range source.Automations {
			lock := incoming.SafetyLock
			if !lock.Active(now) {
				continue
			}
			current := state.Automations[lane]
			if current.SafetyLock.Active(now) && (current.SafetyLock.Until.IsZero() || (!lock.Until.IsZero() && !lock.Until.After(current.SafetyLock.Until))) {
				continue
			}
			current.ID = lane
			current.SafetyLock = lock
			current.Status = "gated"
			current.Detail = lock.Detail()
			current.LastError = current.Detail
			current.LastOperationID = lock.OperationID
			current.UpdatedAt = now
			state.Automations[lane] = current
			changed = true
		}
		return []string{"automation-safety"}, changed, nil
	})
	if err != nil {
		return err
	}
	if event.Patch == nil {
		return nil
	}
	return State.SaveComponentSnapshot(player, event, State.Components(State.ComponentAutomations))
}
