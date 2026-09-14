package Accounts

// Durable source fences never expire. Only an exact, explicitly activated
// newer imported generation may supersede runtime ownership; retired source
// directories remain blocked permanently, including after multiple returns.

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"CitadelDesktop/Server/Runtime"
)

const sourceFenceFile = "source-handover-fences.json"

type SourceProfileFence struct {
	Identity         Runtime.ProfileTransferIdentity `json:"identity"`
	ProfileDirectory string                          `json:"profileDirectory"`
	ProfileID        string                          `json:"profileId,omitempty"`
	SourceStopped    bool                            `json:"sourceStopped"`
}

type sourceFenceDocument struct {
	SchemaVersion int                              `json:"schemaVersion"`
	Fences        map[AccountID]SourceProfileFence `json:"fences"`
}

func validateSourceFence(id AccountID, fence SourceProfileFence) error {
	identity := fence.Identity
	for _, value := range []string{identity.RuntimeID, identity.TenantID, identity.SourceCellID, identity.OperationID, identity.AccountID} {
		parsed, err := ParseAccountID(value)
		if err != nil || string(parsed) != value {
			return errors.New("invalid handover identity")
		}
	}
	if string(id) != identity.RuntimeID || identity.SourceEpoch == 0 || identity.ConfigurationRevision == 0 ||
		!validConfigurationDigest(identity.ConfigurationDigest) {
		return errors.New("invalid handover configuration or epoch")
	}
	parts := strings.Split(fence.ProfileDirectory, "/")
	if len(parts) != 2 || (parts[0] != "Accounts" && parts[0] != playerDirsName) || parts[1] == "" ||
		strings.ContainsAny(parts[1], "\\:\x00") || parts[1] == "." || parts[1] == ".." {
		return errors.New("invalid fenced profile directory")
	}
	if fence.SourceStopped && (fence.ProfileID == "" || len(fence.ProfileID) > 128) {
		return errors.New("stopped fence lacks profile identity")
	}
	return nil
}

func loadSourceFences(dataRoot string) (map[AccountID]SourceProfileFence, error) {
	fences := map[AccountID]SourceProfileFence{}
	file, err := os.Open(filepath.Join(dataRoot, "Accounts", sourceFenceFile))
	if os.IsNotExist(err) {
		return fences, nil
	}
	if err != nil {
		return nil, err
	}
	defer file.Close()
	raw, err := io.ReadAll(io.LimitReader(file, 1<<20+1))
	if err != nil || len(raw) > 1<<20 {
		return nil, errors.New("invalid handover fence journal size")
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var document sourceFenceDocument
	if err := decoder.Decode(&document); err != nil {
		return nil, fmt.Errorf("read handover fences: %w", err)
	}
	var trailing any
	if decoder.Decode(&trailing) != io.EOF || document.SchemaVersion != 1 || document.Fences == nil {
		return nil, errors.New("invalid handover fence journal")
	}
	canonical, marshalErr := json.Marshal(document)
	if marshalErr != nil || !bytes.Equal(raw, canonical) {
		return nil, errors.New("non-canonical or duplicate-key handover fence journal")
	}
	for id, fence := range document.Fences {
		if err := validateSourceFence(id, fence); err != nil {
			return nil, err
		}
		fences[id] = fence
	}
	return fences, nil
}

// Caller holds supervisor.mu. Publication includes a directory fsync; if it
// fails the in-memory fence remains closed and no successful receipt is issued.
func (supervisor *Supervisor) saveSourceFenceLocked(id AccountID, fence SourceProfileFence) error {
	if err := validateSourceFence(id, fence); err != nil {
		return err
	}
	supervisor.sourceFences[id] = fence
	raw, err := json.Marshal(sourceFenceDocument{SchemaVersion: 1, Fences: supervisor.sourceFences})
	if err != nil {
		return err
	}
	if len(raw) > 1<<20 {
		return errors.New("handover fence journal exceeds limit")
	}
	directory := filepath.Join(supervisor.config.DataRoot, "Accounts")
	if err := os.MkdirAll(directory, 0700); err != nil {
		return err
	}
	file, err := os.CreateTemp(directory, ".handover-fence-*")
	if err != nil {
		return err
	}
	temporary := file.Name()
	defer os.Remove(temporary)
	_, writeErr := file.Write(raw)
	syncErr := file.Sync()
	closeErr := file.Close()
	if err := errors.Join(writeErr, syncErr, closeErr); err != nil {
		return err
	}
	if err := os.Rename(temporary, filepath.Join(directory, sourceFenceFile)); err != nil {
		return err
	}
	parent, err := os.Open(directory)
	if err != nil {
		return err
	}
	return errors.Join(parent.Sync(), parent.Close())
}

func (supervisor *Supervisor) sourceProfileFencedLocked(id AccountID, directory string) bool {
	activeProfile, active := supervisor.activeProfileLocked(id)
	if _, imported := supervisor.currentProfileLocked(id); imported {
		if !active || directory != filepath.Join(supervisor.config.DataRoot, filepath.FromSlash(activeProfile.Directory)) {
			return true
		}
	}
	for retired := range supervisor.profileAdoptions.Retired {
		full := filepath.Join(supervisor.config.DataRoot, filepath.FromSlash(retired))
		if pathWithin(full, directory) || pathWithin(directory, full) {
			return true
		}
	}
	for _, profile := range supervisor.profileAdoptions.Operations {
		full := filepath.Join(supervisor.config.DataRoot, filepath.FromSlash(profile.Directory))
		if (pathWithin(full, directory) || pathWithin(directory, full)) &&
			(!active || profile != activeProfile || directory != full) {
			return true
		}
	}
	for owner, fence := range supervisor.sourceFences {
		full := filepath.Join(supervisor.config.DataRoot, filepath.FromSlash(fence.ProfileDirectory))
		if (owner == id && !active) || pathWithin(full, directory) || pathWithin(directory, full) {
			return true
		}
	}
	return false
}

func (supervisor *Supervisor) sourceFence(id AccountID) (SourceProfileFence, bool) {
	supervisor.mu.RLock()
	defer supervisor.mu.RUnlock()
	fence, exists := supervisor.sourceFences[id]
	return fence, exists
}

// PrepareSourceHandover is not exposed by the HTTP router yet. A future
// executor must freeze canonical writes and reserve a compatible target before
// calling it. The source is persistently fenced before stopping any goroutine.
func (orchestrator *Orchestrator) PrepareSourceHandover(ctx context.Context, identity Runtime.ProfileTransferIdentity) (SourceProfileFence, error) {
	if err := ctx.Err(); err != nil {
		return SourceProfileFence{}, err
	}
	orchestrator.reconcileMu.Lock()
	defer orchestrator.reconcileMu.Unlock()
	supervisor := orchestrator.supervisor
	supervisor.rebindMu.Lock()
	defer supervisor.rebindMu.Unlock()
	supervisor.mu.RLock()
	closed := supervisor.closed
	supervisor.mu.RUnlock()
	if closed {
		return SourceProfileFence{}, errors.New("supervisor is closed")
	}
	id := AccountID(identity.RuntimeID)
	fence, exists := supervisor.sourceFence(id)
	supervisor.mu.RLock()
	imported, activeImport := supervisor.activeProfileLocked(id)
	supervisor.mu.RUnlock()
	if activeImport && (imported.TargetEpoch != identity.SourceEpoch || imported.Receipt.Identity.AccountID != identity.AccountID || imported.Receipt.Identity.TenantID != identity.TenantID) {
		return SourceProfileFence{}, errors.New("handover does not own the current imported generation")
	}
	if exists && fence.Identity != identity {
		supervisor.mu.RLock()
		profile, active := supervisor.activeProfileLocked(id)
		supervisor.mu.RUnlock()
		if !active || profile.TargetEpoch != identity.SourceEpoch || profile.Receipt.Identity.AccountID != identity.AccountID ||
			profile.Receipt.Identity.TenantID != identity.TenantID {
			return SourceProfileFence{}, errors.New("profile is fenced by another handover")
		}
		exists = false // A new departure of the latest imported generation only.
	}
	if !exists {
		orchestrator.mu.RLock()
		assignment, present := orchestrator.runtimes[id]
		orchestrator.mu.RUnlock()
		application, running := supervisor.Application(id)
		if !present || !running || application == nil || identity.SourceCellID != orchestrator.cellID ||
			assignment.TenantID != identity.TenantID || assignment.PlacementEpoch != identity.SourceEpoch ||
			assignment.DesiredConfigurationRevision != identity.ConfigurationRevision ||
			assignment.DesiredConfigurationDigest != identity.ConfigurationDigest || !orchestrator.configurationReady(id, assignment) {
			return SourceProfileFence{}, errors.New("source placement or configuration is not ready for handover")
		}
		relative, err := filepath.Rel(supervisor.config.DataRoot, application.DataDir)
		if err != nil {
			return SourceProfileFence{}, err
		}
		fence = SourceProfileFence{Identity: identity, ProfileDirectory: filepath.ToSlash(relative)}
	}
	// Retry journal persistence even for an existing in-memory fence. A previous
	// fsync/rename failure must never be turned into an acknowledged stop.
	supervisor.mu.Lock()
	err := supervisor.saveSourceFenceLocked(id, fence)
	supervisor.mu.Unlock()
	if err != nil {
		return SourceProfileFence{}, err
	}
	orchestrator.dashboardAuth.RevokeRuntime(id)
	if application, running := supervisor.Application(id); running && application != nil {
		application.SetControlConfigurationReady(true, false)
	}
	supervisor.mu.RLock()
	_, running := supervisor.accounts[id]
	_, stopping := supervisor.stopping[id]
	supervisor.mu.RUnlock()
	if running || stopping {
		if err := supervisor.RemoveAccount(ctx, id); err != nil {
			return SourceProfileFence{}, err
		}
	}
	profile := filepath.Join(supervisor.config.DataRoot, filepath.FromSlash(fence.ProfileDirectory))
	if err := supervisor.waitForDataDirRelease(ctx, profile); err != nil {
		return SourceProfileFence{}, err
	}
	// Acquiring a lease ordinarily creates a profile identity when absent.
	// A handover must never turn a lost source directory into a fresh profile.
	identityPath := filepath.Join(profile, "Runtime", "ProfileID")
	info, statErr := os.Lstat(identityPath)
	if statErr != nil || !info.Mode().IsRegular() || info.Size() > 128 {
		return SourceProfileFence{}, errors.New("source durable profile identity is missing or invalid")
	}
	rawID, readErr := os.ReadFile(identityPath)
	if readErr != nil || strings.TrimSpace(string(rawID)) == "" ||
		(fence.SourceStopped && strings.TrimSpace(string(rawID)) != fence.ProfileID) {
		return SourceProfileFence{}, errors.New("source durable profile identity changed")
	}
	lease, err := Runtime.AcquireProfileLease(profile)
	if err != nil {
		return SourceProfileFence{}, err
	}
	if lease.ProfileID != strings.TrimSpace(string(rawID)) {
		_ = lease.Close()
		return SourceProfileFence{}, errors.New("source profile identity changed during stop verification")
	}
	fence.SourceStopped, fence.ProfileID = true, lease.ProfileID
	if err := lease.Close(); err != nil {
		return SourceProfileFence{}, err
	}
	supervisor.mu.Lock()
	err = supervisor.saveSourceFenceLocked(id, fence)
	supervisor.mu.Unlock()
	if err != nil {
		return SourceProfileFence{}, err
	}
	return fence, nil
}
