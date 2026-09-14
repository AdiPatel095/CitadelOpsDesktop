package Accounts

// Adoption is an internal, stopped-profile protocol. No HTTP route exposes it
// until the backend transfer executor and recovery path have been reviewed.

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

	"CitadelDesktop/Server/Configuration"
	"CitadelDesktop/Server/Runtime"
)

const profileAdoptionFile = "profile-adoptions.json"

// TargetProfile identifies an immutable restore generation. Receipt contains no
// secrets; the configuration and archive never enter this ownership journal.
type TargetProfile struct {
	Receipt      Runtime.ProfileArchiveReceipt `json:"receipt"`
	TargetCellID string                        `json:"targetCellId"`
	TargetEpoch  uint64                        `json:"targetEpoch"`
	Directory    string                        `json:"directory"`
	State        string                        `json:"state"` // reserved, restored, active
}

type profileAdoptionDocument struct {
	SchemaVersion int                      `json:"schemaVersion"`
	Operations    map[string]TargetProfile `json:"operations"`
	Current       map[AccountID]string     `json:"current"`
	Retired       map[string]AccountID     `json:"retired"`
}

func emptyProfileAdoptions() profileAdoptionDocument {
	return profileAdoptionDocument{1, map[string]TargetProfile{}, map[AccountID]string{}, map[string]AccountID{}}
}

func validateTargetProfile(profile TargetProfile) error {
	i := profile.Receipt.Identity
	if err := validateSourceFence(AccountID(i.RuntimeID), SourceProfileFence{Identity: i, ProfileDirectory: profile.Directory}); err != nil {
		return err
	}
	cell, err := ParseAccountID(profile.TargetCellID)
	if err != nil || string(cell) != profile.TargetCellID || profile.TargetCellID == i.SourceCellID ||
		i.SourceEpoch >= (1<<63)-1 || profile.TargetEpoch != i.SourceEpoch+1 ||
		profile.Directory != "Accounts/transfer-"+i.OperationID || profile.Receipt.SchemaVersion != 1 ||
		profile.Directory == "Accounts/"+i.RuntimeID ||
		!validConfigurationDigest(profile.Receipt.SHA256) || profile.Receipt.ProfileID == "" || len(profile.Receipt.ProfileID) > 128 ||
		profile.Receipt.Files < 1 || profile.Receipt.Files > 100000 || profile.Receipt.Bytes < 1 || profile.Receipt.Bytes > 16<<30 {
		return errors.New("invalid target profile receipt or placement")
	}
	if profile.State != "reserved" && profile.State != "restored" && profile.State != "active" {
		return errors.New("invalid profile adoption state")
	}
	return nil
}

func loadProfileAdoptions(root string) (profileAdoptionDocument, error) {
	document := emptyProfileAdoptions()
	file, err := os.Open(filepath.Join(root, "Accounts", profileAdoptionFile))
	if os.IsNotExist(err) {
		return document, nil
	}
	if err != nil {
		return document, err
	}
	defer file.Close()
	raw, err := io.ReadAll(io.LimitReader(file, (4<<20)+1))
	if err != nil || len(raw) > 4<<20 {
		return document, errors.New("invalid profile adoption journal size")
	}
	if err := json.Unmarshal(raw, &document); err != nil {
		return document, err
	}
	canonical, err := json.Marshal(document)
	if err != nil || !bytes.Equal(raw, canonical) || document.SchemaVersion != 1 ||
		document.Operations == nil || document.Current == nil || document.Retired == nil {
		return document, errors.New("invalid or non-canonical profile adoption journal")
	}
	for operation, profile := range document.Operations {
		if operation != profile.Receipt.Identity.OperationID || validateTargetProfile(profile) != nil {
			return document, errors.New("invalid profile adoption record")
		}
		if _, ok := document.Current[AccountID(profile.Receipt.Identity.RuntimeID)]; !ok {
			return document, errors.New("profile adoption has no current owner")
		}
	}
	for id, operation := range document.Current {
		profile, ok := document.Operations[operation]
		if !ok || profile.Receipt.Identity.RuntimeID != string(id) || document.Retired["Accounts/"+string(id)] != id {
			return document, errors.New("invalid current profile binding")
		}
		for key, previous := range document.Operations {
			if previous.Receipt.Identity.RuntimeID == string(id) && key != operation &&
				(previous.TargetEpoch >= profile.TargetEpoch || previous.Receipt.Identity.AccountID != profile.Receipt.Identity.AccountID ||
					previous.Receipt.Identity.TenantID != profile.Receipt.Identity.TenantID || previous.TargetCellID != profile.TargetCellID ||
					previous.Receipt.ProfileID != profile.Receipt.ProfileID || document.Retired[previous.Directory] != id) {
				return document, errors.New("profile generation ownership regressed")
			}
		}
	}
	for directory, id := range document.Retired {
		profile, ok := document.Operations[document.Current[id]]
		if !ok || validateSourceFence(id, SourceProfileFence{Identity: profile.Receipt.Identity, ProfileDirectory: directory}) != nil || directory == profile.Directory {
			return document, errors.New("invalid retired profile binding")
		}
	}
	return document, nil
}

func cloneProfileAdoptions(source profileAdoptionDocument) profileAdoptionDocument {
	target := emptyProfileAdoptions()
	for key, value := range source.Operations {
		target.Operations[key] = value
	}
	for key, value := range source.Current {
		target.Current[key] = value
	}
	for key, value := range source.Retired {
		target.Retired[key] = value
	}
	return target
}

// Caller holds mu. A failed reservation remains fenced in memory. Activation
// is published in memory only after a successful file and directory fsync.
func (s *Supervisor) saveProfileAdoptionsLocked(document profileAdoptionDocument) error {
	raw, err := json.Marshal(document)
	if err != nil || len(raw) > 4<<20 {
		return errors.New("profile adoption journal exceeds limit")
	}
	directory := filepath.Join(s.config.DataRoot, "Accounts")
	if err := os.MkdirAll(directory, 0700); err != nil {
		return err
	}
	file, err := os.CreateTemp(directory, ".profile-adoption-*")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	_, writeErr := file.Write(raw)
	if err := errors.Join(writeErr, file.Sync(), file.Close()); err != nil {
		return err
	}
	if err := os.Rename(file.Name(), filepath.Join(directory, profileAdoptionFile)); err != nil {
		return err
	}
	return syncProfileParent(directory)
}

func syncProfileParent(directory string) error {
	parent, err := os.Open(directory)
	if err != nil {
		return err
	}
	return errors.Join(parent.Sync(), parent.Close())
}

func (s *Supervisor) currentProfileLocked(id AccountID) (TargetProfile, bool) {
	profile, ok := s.profileAdoptions.Operations[s.profileAdoptions.Current[id]]
	return profile, ok
}

func (s *Supervisor) reservedProfileSlotsLocked(exclude AccountID) int {
	count := 0
	for id := range s.localProfiles {
		if id == exclude {
			continue
		}
		if _, ok := s.accounts[id]; ok {
			continue
		}
		if _, ok := s.pending[id]; ok {
			continue
		}
		if _, ok := s.stopping[id]; ok {
			continue
		}
		if _, active := s.activeLocalProfileLocked(id); active || s.localProfiles[id].State == "reserved" {
			count++
		}
	}
	for id := range s.profileAdoptions.Current {
		if _, local := s.localProfiles[id]; local {
			continue
		}
		if id == exclude {
			continue
		}
		if _, ok := s.accounts[id]; ok {
			continue
		}
		if _, ok := s.pending[id]; ok {
			continue
		}
		if _, ok := s.stopping[id]; ok {
			continue
		}
		profile, _ := s.currentProfileLocked(id)
		if profile.State == "active" {
			if _, active := s.activeProfileLocked(id); !active {
				continue
			}
		}
		count++
	}
	return count
}

func verifyAdoptedProfileIdentity(directory, expectedID string) error {
	for _, relative := range []string{".", "Runtime", "Runtime/ProfileID"} {
		info, err := os.Lstat(filepath.Join(directory, relative))
		if err != nil || (relative != "Runtime/ProfileID" && !info.IsDir()) ||
			(relative == "Runtime/ProfileID" && (!info.Mode().IsRegular() || info.Size() > 128)) {
			return errors.New("adopted profile identity is missing or unsafe")
		}
	}
	raw, err := os.ReadFile(filepath.Join(directory, "Runtime", "ProfileID"))
	if err != nil || strings.TrimSpace(string(raw)) != expectedID {
		return errors.New("adopted profile identity changed")
	}
	return nil
}

func (s *Supervisor) activeProfileLocked(id AccountID) (TargetProfile, bool) {
	profile, ok := s.currentProfileLocked(id)
	if !ok || profile.State != "active" {
		return profile, false
	}
	if fence, exists := s.sourceFences[id]; exists && fence.Identity.SourceEpoch >= profile.TargetEpoch {
		return profile, false
	}
	return profile, true
}

func (s *Supervisor) runtimeHandoverFenced(id AccountID) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if _, local := s.localProfiles[id]; local {
		_, active := s.activeLocalProfileLocked(id)
		return !active
	}
	if _, active := s.activeProfileLocked(id); active {
		return false
	}
	_, imported := s.currentProfileLocked(id)
	_, fenced := s.sourceFences[id]
	return imported || fenced
}

func (s *Supervisor) validateAdoptedAssignmentLocked(id AccountID, assignment *RuntimeAssignment) error {
	if _, local := s.localProfiles[id]; local {
		return s.validateLocalAssignmentLocked(id, assignment)
	}
	profile, imported := s.currentProfileLocked(id)
	if !imported {
		return nil
	}
	_, active := s.activeProfileLocked(id)
	if !active || assignment == nil || assignment.RuntimeID != string(id) || assignment.TenantID != profile.Receipt.Identity.TenantID ||
		assignment.PlacementEpoch != profile.TargetEpoch || assignment.DesiredConfigurationRevision < profile.Receipt.Identity.ConfigurationRevision ||
		(assignment.DesiredConfigurationRevision == profile.Receipt.Identity.ConfigurationRevision && assignment.DesiredConfigurationDigest != profile.Receipt.Identity.ConfigurationDigest) {
		return errors.New("adopted profile requires its exact active placement and current configuration")
	}
	return nil
}

// Verify settings without opening Configuration.Store: opening can migrate
// defaults and increment its LOCAL revision. Backend revision is a separate
// fence; compare every pinned canonical section, retaining all extra state.
func verifyProfileConfiguration(directory string, expected Configuration.Snapshot, identity Runtime.ProfileTransferIdentity) error {
	if expected.SchemaVersion != Configuration.SchemaVersion || expected.Revision != identity.ConfigurationRevision || len(expected.Sections) == 0 || len(expected.Sections) > maximumConfigurationSections {
		return errors.New("invalid canonical configuration proof")
	}
	sections, digest, err := canonicalConfigurationDigest(expected)
	if err != nil || digest != identity.ConfigurationDigest {
		return errors.New("canonical configuration proof does not match handover")
	}
	root, err := os.OpenRoot(directory)
	if err != nil {
		return err
	}
	defer root.Close()
	file, err := root.Open("Config/Settings.json")
	if err != nil {
		return err
	}
	defer file.Close()
	raw, err := io.ReadAll(io.LimitReader(file, maximumConfigurationSyncBytes+1))
	if err != nil || len(raw) > maximumConfigurationSyncBytes {
		return errors.New("invalid restored configuration size")
	}
	var local Configuration.Snapshot
	if err := json.Unmarshal(raw, &local); err != nil || local.SchemaVersion != Configuration.SchemaVersion {
		return errors.New("unsupported restored configuration")
	}
	for section, value := range sections {
		var compact bytes.Buffer
		if err := json.Compact(&compact, local.Sections[section]); err != nil || !bytes.Equal(value, compact.Bytes()) {
			return fmt.Errorf("restored configuration section %q does not match canonical state", section)
		}
	}
	return nil
}

// RestoreTargetProfile reserves an exact runtime/generation before reading any
// archive bytes. It never starts an App, issues a grant or changes placement.
// The trusted caller must first persist a stopped source receipt in the backend.
func (o *Orchestrator) RestoreTargetProfile(ctx context.Context, receipt Runtime.ProfileArchiveReceipt, targetEpoch uint64, snapshot Configuration.Snapshot, archive io.Reader) (TargetProfile, error) {
	profile := TargetProfile{receipt, o.cellID, targetEpoch, "Accounts/transfer-" + receipt.Identity.OperationID, "reserved"}
	if err := validateTargetProfile(profile); err != nil {
		return TargetProfile{}, err
	}
	if snapshot.SchemaVersion != Configuration.SchemaVersion || snapshot.Revision != receipt.Identity.ConfigurationRevision {
		return TargetProfile{}, errors.New("invalid canonical configuration")
	}
	if _, digest, err := canonicalConfigurationDigest(snapshot); err != nil || digest != receipt.Identity.ConfigurationDigest {
		return TargetProfile{}, errors.New("configuration digest mismatch")
	}
	if err := ctx.Err(); err != nil {
		return TargetProfile{}, err
	}
	o.reconcileMu.Lock()
	defer o.reconcileMu.Unlock()
	s := o.supervisor
	s.rebindMu.Lock()
	defer s.rebindMu.Unlock()
	id, operation := AccountID(receipt.Identity.RuntimeID), receipt.Identity.OperationID
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return TargetProfile{}, errors.New("supervisor is closed")
	}
	previous, exists := s.profileAdoptions.Operations[operation]
	if exists {
		if previous.Receipt != receipt || previous.TargetEpoch != targetEpoch || previous.TargetCellID != o.cellID || s.profileAdoptions.Current[id] != operation {
			s.mu.Unlock()
			return TargetProfile{}, errors.New("handover operation was reused or superseded")
		}
		profile = previous
		if profile.State == "active" {
			_, active := s.activeProfileLocked(id)
			s.mu.Unlock()
			if !active {
				return TargetProfile{}, errors.New("target generation has already departed")
			}
			return profile, nil
		}
	} else {
		if err := o.reserveTargetProfileLocked(id, profile); err != nil {
			s.mu.Unlock()
			return TargetProfile{}, err
		}
	}
	// Repeat persistence after an uncertain previous reservation acknowledgement.
	if err := s.saveProfileAdoptionsLocked(s.profileAdoptions); err != nil {
		s.mu.Unlock()
		return TargetProfile{}, err
	}
	s.mu.Unlock()
	destination := filepath.Join(s.config.DataRoot, filepath.FromSlash(profile.Directory))
	if _, err := os.Lstat(destination); os.IsNotExist(err) {
		if profile.State != "reserved" {
			return TargetProfile{}, errors.New("previously restored profile is missing")
		}
		stage, err := os.MkdirTemp(filepath.Dir(destination), ".profile-transfer-*")
		if err != nil {
			return TargetProfile{}, err
		}
		defer os.RemoveAll(stage) // Only this call's newly-created private stage.
		staged := filepath.Join(stage, "profile")
		actual, err := Runtime.RestoreProfileArchive(ctx, archive, staged, receipt.Identity, receipt.SHA256)
		if err != nil {
			return TargetProfile{}, err
		}
		if actual != receipt {
			return TargetProfile{}, errors.New("restored receipt mismatch")
		}
		if err := verifyProfileConfiguration(staged, snapshot, receipt.Identity); err != nil {
			return TargetProfile{}, err
		}
		if err := ctx.Err(); err != nil {
			return TargetProfile{}, err
		}
		if err := os.Rename(staged, destination); err != nil {
			return TargetProfile{}, err
		}
	} else if err != nil {
		return TargetProfile{}, err
	}
	// Also handles a crash after rename but before publishing "restored". Never
	// overwrite the existing generation; re-hash it under its exclusive lease.
	actual, err := Runtime.WriteProfileArchive(ctx, destination, io.Discard, receipt.Identity)
	if err != nil || actual != receipt {
		return TargetProfile{}, errors.New("restored generation failed integrity verification")
	}
	if err := verifyProfileConfiguration(destination, snapshot, receipt.Identity); err != nil {
		return TargetProfile{}, err
	}
	if err := syncProfileParent(filepath.Dir(destination)); err != nil {
		return TargetProfile{}, err
	}
	profile.State = "restored"
	s.mu.Lock()
	defer s.mu.Unlock()
	document := cloneProfileAdoptions(s.profileAdoptions)
	document.Operations[operation] = profile
	if err := s.saveProfileAdoptionsLocked(document); err != nil {
		return TargetProfile{}, err
	}
	s.profileAdoptions = document
	return profile, nil
}

// Caller holds reconcileMu, rebindMu and supervisor.mu.
func (o *Orchestrator) reserveTargetProfileLocked(id AccountID, profile TargetProfile) error {
	s := o.supervisor
	if _, local := s.localProfiles[id]; local {
		return errors.New("settings-only profile cannot import an archive")
	}
	if s.closed {
		return errors.New("supervisor is closed")
	}
	if _, ok := s.accounts[id]; ok {
		return errors.New("target runtime is running")
	}
	if _, ok := s.pending[id]; ok {
		return errors.New("target runtime is starting")
	}
	if _, ok := s.stopping[id]; ok {
		return errors.New("target runtime is stopping")
	}
	if s.config.MaxAccounts > 0 && len(s.accounts)+len(s.pending)+len(s.stopping)+s.reservedProfileSlotsLocked(id) >= s.config.MaxAccounts {
		return errors.New("target has no unreserved runtime capacity")
	}
	if old, ok := s.currentProfileLocked(id); ok {
		fence, fenced := s.sourceFences[id]
		if !fenced || !fence.SourceStopped || fence.Identity.SourceEpoch < old.TargetEpoch ||
			profile.Receipt.Identity.SourceEpoch <= fence.Identity.SourceEpoch || old.State != "active" ||
			old.Receipt.Identity.AccountID != profile.Receipt.Identity.AccountID || old.Receipt.Identity.TenantID != profile.Receipt.Identity.TenantID {
			return errors.New("previous imported generation has not been retired")
		}
	}
	if fence, ok := s.sourceFences[id]; ok {
		if !fence.SourceStopped || profile.Receipt.Identity.SourceEpoch <= fence.Identity.SourceEpoch ||
			fence.Identity.AccountID != profile.Receipt.Identity.AccountID || fence.Identity.TenantID != profile.Receipt.Identity.TenantID || fence.ProfileID != profile.Receipt.ProfileID {
			return errors.New("return handover does not continue the retired profile")
		}
	}
	destination := filepath.Join(s.config.DataRoot, filepath.FromSlash(profile.Directory))
	if _, err := os.Lstat(destination); !os.IsNotExist(err) {
		return errors.New("target generation already exists")
	}
	document := cloneProfileAdoptions(s.profileAdoptions)
	oldDirectories := []string{"Accounts/" + string(id)}
	if key := s.playerBindings[string(id)]; key != "" {
		oldDirectories = append(oldDirectories, playerDirsName+"/"+key)
	}
	if old, ok := s.currentProfileLocked(id); ok {
		oldDirectories = append(oldDirectories, old.Directory)
	}
	if fence, ok := s.sourceFences[id]; ok {
		oldDirectories = append(oldDirectories, fence.ProfileDirectory)
	}
	for _, directory := range append(oldDirectories, profile.Directory) {
		full := filepath.Join(s.config.DataRoot, filepath.FromSlash(directory))
		for live := range s.dataDirs {
			if pathWithin(full, live) || pathWithin(live, full) {
				return errors.New("target profile path is in use")
			}
		}
		for _, imported := range document.Operations {
			if imported.Directory == directory && imported.Receipt.Identity.RuntimeID != string(id) {
				return errors.New("profile directory belongs to another runtime")
			}
		}
		if owner, ok := document.Retired[directory]; ok && owner != id {
			return errors.New("retired directory belongs to another runtime")
		}
		if directory == profile.Directory {
			continue
		}
		if _, err := os.Lstat(full); err != nil && !os.IsNotExist(err) {
			return err
		} else if err == nil {
			if _, fenced := s.sourceFences[id]; !fenced {
				return errors.New("existing target profile has no stopped source fence")
			}
		}
		document.Retired[directory] = id
	}
	document.Current[id] = profile.Receipt.Identity.OperationID
	document.Operations[profile.Receipt.Identity.OperationID] = profile
	s.profileAdoptions = document // Remain closed if the subsequent fsync fails.
	return nil
}

// ActivateTargetProfile is permitted only after the backend atomically commits
// target placement at TargetEpoch. It changes only the durable directory
// pointer. Reconcile/configuration/login are separate exact-epoch operations.
func (o *Orchestrator) ActivateTargetProfile(ctx context.Context, expected TargetProfile) (TargetProfile, error) {
	if err := validateTargetProfile(expected); err != nil {
		return TargetProfile{}, err
	}
	if err := ctx.Err(); err != nil {
		return TargetProfile{}, err
	}
	o.reconcileMu.Lock()
	defer o.reconcileMu.Unlock()
	s := o.supervisor
	s.rebindMu.Lock()
	defer s.rebindMu.Unlock()
	id, operation := AccountID(expected.Receipt.Identity.RuntimeID), expected.Receipt.Identity.OperationID
	s.mu.RLock()
	if s.closed {
		s.mu.RUnlock()
		return TargetProfile{}, errors.New("supervisor is closed")
	}
	current, ok := s.currentProfileLocked(id)
	s.mu.RUnlock()
	if !ok || current.Receipt != expected.Receipt || current.TargetEpoch != expected.TargetEpoch || current.TargetCellID != o.cellID || expected.TargetCellID != o.cellID || current.State == "reserved" {
		return TargetProfile{}, errors.New("target has not restored this exact handover")
	}
	if s.runtimeHandoverFenced(id) && current.State == "active" {
		return TargetProfile{}, errors.New("target generation has already departed")
	}
	alreadyActive := current.State == "active"
	if current.State != "active" {
		actual, err := Runtime.WriteProfileArchive(ctx, filepath.Join(s.config.DataRoot, filepath.FromSlash(current.Directory)), io.Discard, current.Receipt.Identity)
		if err != nil || actual != current.Receipt {
			return TargetProfile{}, errors.New("target changed before activation")
		}
	}
	if err := ctx.Err(); err != nil {
		return TargetProfile{}, err
	}
	current.State = "active"
	s.mu.Lock()
	defer s.mu.Unlock()
	document := cloneProfileAdoptions(s.profileAdoptions)
	document.Operations[operation] = current
	if err := s.saveProfileAdoptionsLocked(document); err != nil {
		return TargetProfile{}, err
	}
	s.profileAdoptions = document
	// PrepareSourceHandover may have retained an obsolete in-memory assignment
	// on this returning cell. It must not suppress construction of the new App.
	if !alreadyActive {
		o.mu.Lock()
		delete(o.runtimes, id)
		delete(o.configurationSyncs, id)
		o.mu.Unlock()
	}
	return current, nil
}
