package Accounts

import (
	"CitadelDesktop/Server/Diagnostics"
	"CitadelDesktop/Server/Telemetry"
	"net/http"
)

func (orchestrator *Orchestrator) handleDiagnostics(w http.ResponseWriter, r *http.Request) {
	stores := orchestrator.supervisor.telemetryStores()
	var total Telemetry.PersistenceStats
	for _, store := range stores {
		total.Add(store.PersistenceSnapshot())
	}
	var load *Diagnostics.LoadSnapshot
	if snapshot, ok := orchestrator.load.Snapshot(); ok {
		load = &snapshot
	}
	writeControlJSON(w, http.StatusOK, struct {
		Load            *Diagnostics.LoadSnapshot   `json:"load,omitempty"`
		Process         Diagnostics.ProcessSnapshot `json:"process"`
		Telemetry       Telemetry.PersistenceStats  `json:"telemetry"`
		TelemetryStores int                         `json:"telemetryStores"`
	}{load, Diagnostics.SampleProcess(), total, len(stores)})
}
func (supervisor *Supervisor) telemetryStores() []*Telemetry.Store {
	supervisor.mu.RLock()
	defer supervisor.mu.RUnlock()
	stores := make([]*Telemetry.Store, 0, len(supervisor.accounts)+len(supervisor.stopping))
	seen := map[*Telemetry.Store]bool{}
	for _, group := range []map[AccountID]accountRuntime{supervisor.accounts, supervisor.stopping} {
		for _, runtime := range group {
			if runtime.application != nil && runtime.application.Telemetry != nil && !seen[runtime.application.Telemetry] {
				seen[runtime.application.Telemetry] = true
				stores = append(stores, runtime.application.Telemetry)
			}
		}
	}
	return stores
}
