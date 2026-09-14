package Accounts

// Settings-only switching keeps an independent persistent profile on each
// cell. This journal selects that cell's existing profile (or its initial empty
// account directory); no archive, source history or local database is imported.

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"

	"CitadelDesktop/Server/Runtime"
)

const localProfileBindingsFile = "local-profile-bindings.json"

type LocalProfileBinding struct {
	Identity     Runtime.ProfileTransferIdentity `json:"identity"`
	TargetCellID string                          `json:"targetCellId"`
	TargetEpoch  uint64                          `json:"targetEpoch"`
	Directory    string                          `json:"directory"`
	ProfileID    string                          `json:"profileId"`
	State        string                          `json:"state"`
}

func (binding LocalProfileBinding) valid() bool {
	i := binding.Identity
	target, err := ParseAccountID(binding.TargetCellID)
	return err == nil && string(target) == binding.TargetCellID && binding.TargetCellID != i.SourceCellID && i.SourceEpoch < (1<<63)-1 && binding.TargetEpoch == i.SourceEpoch+1 &&
		(binding.State == "reserved" || binding.State == "active") && validateSourceFence(AccountID(i.RuntimeID), SourceProfileFence{
		Identity: i, ProfileDirectory: binding.Directory, ProfileID: binding.ProfileID, SourceStopped: true,
	}) == nil
}

func loadLocalProfileBindings(root string) (map[AccountID]LocalProfileBinding, error) {
	bindings := map[AccountID]LocalProfileBinding{}
	f, err := os.Open(filepath.Join(root, "Accounts", localProfileBindingsFile))
	if os.IsNotExist(err) {
		return bindings, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()
	raw, err := io.ReadAll(io.LimitReader(f, (1<<20)+1))
	if err != nil || len(raw) > 1<<20 {
		return nil, errors.New("invalid local profile journal size")
	}
	if err = json.Unmarshal(raw, &bindings); err != nil || bindings == nil {
		return nil, errors.New("invalid local profile journal")
	}
	canonical, err := json.Marshal(bindings)
	if err != nil || !bytes.Equal(raw, canonical) {
		return nil, errors.New("non-canonical local profile journal")
	}
	paths := map[string]AccountID{}
	for id, b := range bindings {
		if !b.valid() || b.Identity.RuntimeID != string(id) {
			return nil, errors.New("invalid local profile binding")
		}
		if prior, ok := paths[b.Directory]; ok && prior != id {
			return nil, errors.New("local profile has multiple owners")
		}
		paths[b.Directory] = id
	}
	return bindings, nil
}

// Caller holds supervisor.mu. Retain the new in-memory fence on fsync failure.
func (s *Supervisor) saveLocalProfileBindingLocked(id AccountID, binding LocalProfileBinding) error {
	if !binding.valid() || binding.Identity.RuntimeID != string(id) {
		return errors.New("invalid local profile binding")
	}
	s.localProfiles[id] = binding
	raw, err := json.Marshal(s.localProfiles)
	if err != nil || len(raw) > 1<<20 {
		return errors.New("local profile journal exceeds limit")
	}
	dir := filepath.Join(s.config.DataRoot, "Accounts")
	f, err := os.CreateTemp(dir, ".local-binding-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	_, writeErr := f.Write(raw)
	if err = errors.Join(writeErr, f.Sync(), f.Close()); err != nil {
		return err
	}
	if err = os.Rename(f.Name(), filepath.Join(dir, localProfileBindingsFile)); err != nil {
		return err
	}
	return syncProfileParent(dir)
}

func (s *Supervisor) activeLocalProfileLocked(id AccountID) (LocalProfileBinding, bool) {
	b, ok := s.localProfiles[id]
	if !ok || b.State != "active" {
		return b, false
	}
	if fence, exists := s.sourceFences[id]; exists && fence.Identity.SourceEpoch >= b.TargetEpoch {
		return b, false
	}
	return b, true
}

func (s *Supervisor) validateLocalAssignmentLocked(id AccountID, a *RuntimeAssignment) error {
	b, active := s.activeLocalProfileLocked(id)
	if !active || a == nil || a.RuntimeID != string(id) || a.PlacementEpoch != b.TargetEpoch || a.TenantID != b.Identity.TenantID ||
		a.DesiredConfigurationRevision < b.Identity.ConfigurationRevision || (a.DesiredConfigurationRevision == b.Identity.ConfigurationRevision && a.DesiredConfigurationDigest != b.Identity.ConfigurationDigest) {
		return errors.New("local profile requires its exact active placement and current settings")
	}
	return verifyAdoptedProfileIdentity(filepath.Join(s.config.DataRoot, filepath.FromSlash(b.Directory)), b.ProfileID)
}

// A target reservation changes ownership metadata only. The ordinary parked
// runtime/configuration acknowledgement path installs the canonical settings
// after placement commits, and MUST run before login or automation starts.
func (o *Orchestrator) PrepareLocalProfile(ctx context.Context, identity Runtime.ProfileTransferIdentity) (LocalProfileBinding, error) {
	if err := ctx.Err(); err != nil {
		return LocalProfileBinding{}, err
	}
	if identity.SourceCellID == o.cellID || validateSourceFence(AccountID(identity.RuntimeID), SourceProfileFence{Identity: identity, ProfileDirectory: "Accounts/" + identity.RuntimeID}) != nil {
		return LocalProfileBinding{}, errors.New("invalid local profile request")
	}
	o.reconcileMu.Lock()
	defer o.reconcileMu.Unlock()
	s := o.supervisor
	s.rebindMu.Lock()
	defer s.rebindMu.Unlock()
	id := AccountID(identity.RuntimeID)
	directory, err := s.accountDataDir(id, "")
	if err != nil {
		return LocalProfileBinding{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return LocalProfileBinding{}, errors.New("supervisor is closed")
	}
	if current, ok := s.localProfiles[id]; ok {
		if current.Identity == identity && current.TargetCellID == o.cellID {
			if current.State != "reserved" {
				return LocalProfileBinding{}, errors.New("local generation already activated")
			}
			if err := s.saveLocalProfileBindingLocked(id, current); err != nil {
				return LocalProfileBinding{}, err
			}
			return current, nil
		}
		if current.Identity.AccountID != identity.AccountID || current.Identity.TenantID != identity.TenantID || current.TargetEpoch >= identity.SourceEpoch {
			return LocalProfileBinding{}, errors.New("local profile ownership regressed")
		}
		fence, stopped := s.sourceFences[id]
		if current.State != "active" || !stopped || !fence.SourceStopped || fence.Identity.SourceEpoch < current.TargetEpoch {
			return LocalProfileBinding{}, errors.New("previous local placement has not stopped")
		}
	}
	if _, ok := s.accounts[id]; ok {
		return LocalProfileBinding{}, errors.New("target runtime is running")
	}
	if _, ok := s.pending[id]; ok {
		return LocalProfileBinding{}, errors.New("target runtime is starting")
	}
	if _, ok := s.stopping[id]; ok {
		return LocalProfileBinding{}, errors.New("target runtime is stopping")
	}
	if s.config.MaxAccounts > 0 && len(s.accounts)+len(s.pending)+len(s.stopping)+s.reservedProfileSlotsLocked(id) >= s.config.MaxAccounts {
		return LocalProfileBinding{}, errors.New("target has no reserved capacity")
	}
	if old, ok := s.currentProfileLocked(id); ok && (old.Receipt.Identity.AccountID != identity.AccountID || old.Receipt.Identity.TenantID != identity.TenantID || old.TargetEpoch >= identity.SourceEpoch) {
		return LocalProfileBinding{}, errors.New("imported profile owner or epoch mismatch")
	}
	if old, ok := s.currentProfileLocked(id); ok {
		fence, stopped := s.sourceFences[id]
		if old.State != "active" || !stopped || !fence.SourceStopped || fence.Identity.SourceEpoch < old.TargetEpoch {
			return LocalProfileBinding{}, errors.New("imported placement has not stopped")
		}
	}
	if fence, ok := s.sourceFences[id]; ok {
		if !fence.SourceStopped || fence.Identity.SourceEpoch >= identity.SourceEpoch || fence.Identity.AccountID != identity.AccountID || fence.Identity.TenantID != identity.TenantID {
			return LocalProfileBinding{}, errors.New("local source has not stopped")
		}
	}
	for path := range s.dataDirs {
		if pathWithin(path, directory) || pathWithin(directory, path) {
			return LocalProfileBinding{}, errors.New("local profile is in use")
		}
	}
	for owner, b := range s.localProfiles {
		if owner != id && filepath.Join(s.config.DataRoot, filepath.FromSlash(b.Directory)) == directory {
			return LocalProfileBinding{}, errors.New("local profile belongs to another runtime")
		}
	}
	// Only a genuinely new target may create a new identity. A missing known
	// source/imported/local profile must never silently become an empty account.
	_, knownLocal := s.localProfiles[id]
	_, knownImport := s.currentProfileLocked(id)
	fence, knownSource := s.sourceFences[id]
	if knownLocal {
		if err := verifyAdoptedProfileIdentity(directory, s.localProfiles[id].ProfileID); err != nil {
			return LocalProfileBinding{}, err
		}
	} else if knownImport {
		b, _ := s.currentProfileLocked(id)
		if err := verifyAdoptedProfileIdentity(directory, b.Receipt.ProfileID); err != nil {
			return LocalProfileBinding{}, err
		}
	} else if knownSource {
		if err := verifyAdoptedProfileIdentity(directory, fence.ProfileID); err != nil {
			return LocalProfileBinding{}, err
		}
	} else if s.playerBindings[string(id)] != "" {
		if _, err := os.Stat(filepath.Join(directory, "Runtime", "ProfileID")); err != nil {
			return LocalProfileBinding{}, errors.New("known player profile is missing")
		}
	}
	canonicalRoot, rootErr := filepath.EvalSymlinks(s.config.DataRoot)
	relative, relativeErr := filepath.Rel(s.config.DataRoot, directory)
	if rootErr != nil || relativeErr != nil {
		return LocalProfileBinding{}, errors.New("invalid local profile root")
	}
	if resolved, err := filepath.EvalSymlinks(directory); err == nil && resolved != filepath.Join(canonicalRoot, relative) {
		return LocalProfileBinding{}, errors.New("unsafe local profile path")
	} else if err != nil && !os.IsNotExist(err) {
		return LocalProfileBinding{}, err
	}
	lease, err := Runtime.AcquireProfileLease(directory)
	if err != nil {
		return LocalProfileBinding{}, err
	}
	defer lease.Close()
	b := LocalProfileBinding{Identity: identity, TargetCellID: o.cellID, TargetEpoch: identity.SourceEpoch + 1, Directory: filepath.ToSlash(relative), ProfileID: lease.ProfileID, State: "reserved"}
	if err = s.saveLocalProfileBindingLocked(id, b); err != nil {
		return LocalProfileBinding{}, err
	}
	return b, nil
}

func (o *Orchestrator) ActivateLocalProfile(ctx context.Context, expected LocalProfileBinding) (LocalProfileBinding, error) {
	if !expected.valid() || expected.TargetCellID != o.cellID || expected.State != "reserved" {
		return LocalProfileBinding{}, errors.New("invalid local activation")
	}
	if err := ctx.Err(); err != nil {
		return LocalProfileBinding{}, err
	}
	o.reconcileMu.Lock()
	defer o.reconcileMu.Unlock()
	s := o.supervisor
	s.rebindMu.Lock()
	defer s.rebindMu.Unlock()
	s.mu.Lock()
	defer s.mu.Unlock()
	id := AccountID(expected.Identity.RuntimeID)
	current, ok := s.localProfiles[id]
	comparable := current
	comparable.State = "reserved"
	if s.closed || !ok || comparable != expected {
		return LocalProfileBinding{}, errors.New("exact local reservation required")
	}
	if fence, ok := s.sourceFences[id]; ok && fence.Identity.SourceEpoch >= expected.TargetEpoch {
		return LocalProfileBinding{}, errors.New("local generation already departed")
	}
	if err := verifyAdoptedProfileIdentity(filepath.Join(s.config.DataRoot, filepath.FromSlash(current.Directory)), current.ProfileID); err != nil {
		return LocalProfileBinding{}, err
	}
	current.State = "active"
	if err := s.saveLocalProfileBindingLocked(id, current); err != nil {
		return LocalProfileBinding{}, err
	}
	return current, nil
}

func (o *Orchestrator) handleLocalProfileStop(w http.ResponseWriter, r *http.Request) {
	var identity Runtime.ProfileTransferIdentity
	if decodeControlJSON(w, r, &identity) != nil {
		return
	}
	fence, err := o.prepareSourceHandover(r.Context(), identity, false)
	if err != nil {
		writeControlError(w, http.StatusConflict, "local_profile_stop_not_ready")
		return
	}
	writeControlJSON(w, http.StatusOK, fence)
}

func (o *Orchestrator) handleLocalProfilePrepare(w http.ResponseWriter, r *http.Request) {
	var identity Runtime.ProfileTransferIdentity
	if decodeControlJSON(w, r, &identity) != nil {
		return
	}
	binding, err := o.PrepareLocalProfile(r.Context(), identity)
	if err != nil {
		writeControlError(w, http.StatusConflict, "local_profile_not_ready")
		return
	}
	writeControlJSON(w, http.StatusOK, binding)
}

func (o *Orchestrator) handleLocalProfileActivate(w http.ResponseWriter, r *http.Request) {
	var expected LocalProfileBinding
	if decodeControlJSON(w, r, &expected) != nil {
		return
	}
	binding, err := o.ActivateLocalProfile(r.Context(), expected)
	if err != nil {
		writeControlError(w, http.StatusConflict, "local_profile_not_active")
		return
	}
	writeControlJSON(w, http.StatusOK, binding)
}
