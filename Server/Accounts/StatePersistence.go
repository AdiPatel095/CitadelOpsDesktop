package Accounts

import (
	"CitadelDesktop/Server/App"
	"CitadelDesktop/Server/State"
)

func (supervisor *Supervisor) statePersistenceStats() (State.PersistenceStats, int) {
	supervisor.mu.RLock()
	defer supervisor.mu.RUnlock()
	var total State.PersistenceStats
	seen := map[*App.Application]bool{}
	for _, group := range []map[AccountID]accountRuntime{supervisor.accounts, supervisor.stopping} {
		for _, runtime := range group {
			if runtime.application != nil && !seen[runtime.application] {
				seen[runtime.application] = true
				total.Add(runtime.application.StatePersistenceStats())
			}
		}
	}
	return total, len(seen)
}
