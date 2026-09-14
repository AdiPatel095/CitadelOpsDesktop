package Accounts

import (
	"context"
	"errors"
	"net/http"

	"CitadelDesktop/Server/Runtime"
)

func (o *Orchestrator) preflightSourceProfile(ctx context.Context, identity Runtime.ProfileTransferIdentity) error {
	o.reconcileMu.Lock()
	defer o.reconcileMu.Unlock()
	o.supervisor.rebindMu.Lock()
	defer o.supervisor.rebindMu.Unlock()
	id := AccountID(identity.RuntimeID)
	o.mu.RLock()
	assignment, present := o.runtimes[id]
	o.mu.RUnlock()
	app, running := o.supervisor.Application(id)
	if !present || !running || app == nil || identity.SourceCellID != o.cellID || assignment.TenantID != identity.TenantID || assignment.PlacementEpoch != identity.SourceEpoch || assignment.DesiredConfigurationRevision != identity.ConfigurationRevision || assignment.DesiredConfigurationDigest != identity.ConfigurationDigest || !o.configurationReady(id, assignment) {
		return errors.New("source preflight placement mismatch")
	}
	return Runtime.InspectProfileArchive(ctx, app.DataDir)
}

func (o *Orchestrator) handleProfilePreflight(w http.ResponseWriter, r *http.Request) {
	var identity Runtime.ProfileTransferIdentity
	if err := decodeControlJSON(w, r, &identity); err != nil {
		return
	}
	if err := o.preflightSourceProfile(r.Context(), identity); err != nil {
		writeControlError(w, 409, "profile_preflight_failed")
		return
	}
	writeControlJSON(w, 200, struct {
		Ready bool `json:"ready"`
	}{true})
}
