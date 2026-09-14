package Accounts

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"CitadelDesktop/Server/Configuration"
	"CitadelDesktop/Server/Runtime"
)

func adoptionSnapshot() Configuration.Snapshot {
	return Configuration.Snapshot{SchemaVersion: 1, Revision: 39, Sections: map[string]json.RawMessage{"scheduler": json.RawMessage(`{"minAttackDelay":9}`)}}
}

func stoppedAdoptionFixture(t *testing.T) (*Supervisor, *Orchestrator, Runtime.ProfileArchiveReceipt, []byte, string) {
	t.Helper()
	source, _, orchestrator, identity, _ := handoverSourceFixture(t)
	app, _ := source.Application("alpha")
	directory := app.DataDir
	if err := os.WriteFile(filepath.Join(directory, "unknown-history.bin"), []byte("all original history\x00"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := orchestrator.PrepareSourceHandover(t.Context(), identity); err != nil {
		t.Fatal(err)
	}
	var archive bytes.Buffer
	receipt, err := Runtime.WriteProfileArchive(t.Context(), directory, &archive, identity)
	if err != nil {
		t.Fatal(err)
	}
	return source, orchestrator, receipt, archive.Bytes(), directory
}

func adoptionTarget(t *testing.T) (*Supervisor, *Orchestrator) {
	t.Helper()
	s, _, o, _ := newTestOrchestrator(t)
	o.cellID = "cell-two"
	return s, o
}

func restoreAdoption(t *testing.T, o *Orchestrator, receipt Runtime.ProfileArchiveReceipt, archive []byte) TargetProfile {
	t.Helper()
	profile, err := o.RestoreTargetProfile(t.Context(), receipt, receipt.Identity.SourceEpoch+1, adoptionSnapshot(), bytes.NewReader(archive))
	if err != nil {
		t.Fatal(err)
	}
	if profile.State != "restored" {
		t.Fatalf("restore state = %s", profile.State)
	}
	return profile
}

func startAdoptionParked(t *testing.T, o *Orchestrator, profile TargetProfile, revision uint64) RuntimeAssignment {
	t.Helper()
	if _, err := o.ActivateTargetProfile(t.Context(), profile); err != nil {
		t.Fatal(err)
	}
	assignment := testAssignment("alpha", "tenant-one", profile.TargetEpoch, o.now().Add(10*time.Minute))
	assignment.DesiredConfigurationRevision = profile.Receipt.Identity.ConfigurationRevision
	assignment.DesiredConfigurationDigest = profile.Receipt.Identity.ConfigurationDigest
	if _, err := o.Reconcile(t.Context(), ReconcileRequest{1, revision, []RuntimeAssignment{assignment}}); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(o.Handler())
	defer server.Close()
	response, body := controlRequest(t, http.MethodPut, server.URL+"/orchestrator/v1/runtimes/alpha/configuration", ConfigurationSyncRequest{1, profile.TargetEpoch, profile.Receipt.Identity.ConfigurationDigest, adoptionSnapshot()})
	if response.StatusCode != http.StatusOK {
		t.Fatalf("sync imported profile: %d %s", response.StatusCode, body)
	}
	return assignment
}

func TestProfileAdoptionPreservesExactStateAndRequiresActivation(t *testing.T) {
	source, _, receipt, archive, original := stoppedAdoptionFixture(t)
	target, o := adoptionTarget(t)
	target.config.MaxAccounts = 1
	settings, err := os.ReadFile(filepath.Join(original, "Config", "Settings.json"))
	if err != nil {
		t.Fatal(err)
	}
	profile := restoreAdoption(t, o, receipt, archive)
	destination := filepath.Join(target.config.DataRoot, filepath.FromSlash(profile.Directory))
	for _, file := range []string{"Config/Settings.json", "Runtime/ProfileID", "unknown-history.bin"} {
		before, err := os.ReadFile(filepath.Join(original, file))
		if err != nil {
			t.Fatal(err)
		}
		after, err := os.ReadFile(filepath.Join(destination, file))
		if err != nil || !bytes.Equal(before, after) {
			t.Fatalf("state changed: %s (%v)", file, err)
		}
	}
	if !target.runtimeHandoverFenced("alpha") || target.Capacity().Starting != 1 || target.Capacity().Active != 0 {
		t.Fatal("restored target was not parked/reserved")
	}
	if _, err := target.AddAccount(t.Context(), AccountConfig{ID: "alpha"}); err == nil {
		t.Fatal("direct start bypassed adoption")
	}
	if _, err := target.AddAccount(t.Context(), AccountConfig{ID: "sibling"}); err == nil {
		t.Fatal("sibling stole reserved capacity")
	}
	request := httptest.NewRequest(http.MethodPost, "/orchestrator/v1/runtimes/alpha/reconnect", strings.NewReader(`{}`))
	request.Header.Set("Authorization", "Bearer "+testControlToken)
	response := httptest.NewRecorder()
	o.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusLocked {
		t.Fatalf("unactivated control status = %d", response.Code)
	}
	assignment := startAdoptionParked(t, o, profile, 1)
	app, ok := target.Application("alpha")
	if !ok || app.DataDir != destination {
		t.Fatal("wrong adopted directory")
	}
	if _, err := o.ActivateTargetProfile(t.Context(), profile); err != nil {
		t.Fatal(err)
	}
	if _, exists := o.runtimes["alpha"]; !exists {
		t.Fatal("activation retry deleted running assignment")
	}
	for _, change := range []string{"old-epoch", "future-epoch", "tenant", "config-revision", "config-digest"} {
		wrong := assignment
		switch change {
		case "old-epoch":
			wrong.PlacementEpoch--
		case "future-epoch":
			wrong.PlacementEpoch++
		case "tenant":
			wrong.TenantID = "other"
		case "config-revision":
			wrong.DesiredConfigurationRevision--
		case "config-digest":
			wrong.DesiredConfigurationDigest = strings.Repeat("a", 64)
		}
		if _, err := o.Reconcile(t.Context(), ReconcileRequest{1, 2, []RuntimeAssignment{wrong}}); err == nil {
			t.Fatalf("accepted %s", change)
		}
	}
	if _, err := target.AddAccount(t.Context(), AccountConfig{ID: "alias", DataDir: destination}); err == nil {
		t.Fatal("alias acquired imported profile")
	}
	if _, err := source.AddAccount(t.Context(), AccountConfig{ID: "alpha"}); err == nil {
		t.Fatal("source restarted")
	}
	unchanged, _ := os.ReadFile(filepath.Join(original, "Config", "Settings.json"))
	if !bytes.Equal(settings, unchanged) {
		t.Fatal("source settings changed")
	}
	if target.Capacity().Starting != 0 || target.Capacity().Active != 1 {
		t.Fatal("reservation double-counted after start")
	}
}

func TestProfileAdoptionRoundTripsLatestHistoryWithoutUnlockingOldDirectories(t *testing.T) {
	original, stable, receipt, archive, firstDirectory := stoppedAdoptionFixture(t)
	betaSupervisor, beta := adoptionTarget(t)
	profile := restoreAdoption(t, beta, receipt, archive)
	startAdoptionParked(t, beta, profile, 1)
	previousDirectories := []string{firstDirectory}
	from, to := beta, stable
	for index, operation := range []string{"return-stable", "return-beta", "stable-again"} {
		app, _ := from.supervisor.Application("alpha")
		directory := app.DataDir
		marker := "history-" + operation
		if err := os.WriteFile(filepath.Join(directory, marker), []byte(operation), 0600); err != nil {
			t.Fatal(err)
		}
		identity := receipt.Identity
		identity.OperationID, identity.SourceCellID, identity.SourceEpoch = operation, from.cellID, profile.TargetEpoch
		if _, err := from.PrepareSourceHandover(t.Context(), identity); err != nil {
			t.Fatal(err)
		}
		var transfer bytes.Buffer
		nextReceipt, err := Runtime.WriteProfileArchive(t.Context(), directory, &transfer, identity)
		if err != nil {
			t.Fatal(err)
		}
		next := restoreAdoption(t, to, nextReceipt, transfer.Bytes())
		startAdoptionParked(t, to, next, uint64(index+2))
		current, _ := to.supervisor.Application("alpha")
		if current.DataDir == directory || current.DataDir == firstDirectory {
			t.Fatal("return reused obsolete profile")
		}
		for _, file := range []string{marker, "unknown-history.bin"} {
			if _, err := os.Stat(filepath.Join(current.DataDir, file)); err != nil {
				t.Fatal("latest history was lost", err)
			}
		}
		if _, err := from.ActivateTargetProfile(t.Context(), profile); err == nil {
			t.Fatal("departed activation was replayable")
		}
		if _, err := stable.PrepareSourceHandover(t.Context(), receipt.Identity); err == nil {
			t.Fatal("old departure replay stopped latest generation")
		}
		if _, exists := to.supervisor.Application("alpha"); !exists {
			t.Fatal("stale operation stopped current runtime")
		}
		previousDirectories = append(previousDirectories, directory)
		for _, old := range previousDirectories {
			if _, err := os.Stat(filepath.Join(old, "Runtime", "ProfileID")); err != nil {
				t.Fatal("old profile was destroyed", err)
			}
			for _, supervisor := range []*Supervisor{original, betaSupervisor} {
				if pathWithin(supervisor.config.DataRoot, old) {
					for _, path := range []string{old, filepath.Join(old, "Runtime")} {
						supervisor.mu.RLock()
						fenced := supervisor.sourceProfileFencedLocked("alias", path)
						supervisor.mu.RUnlock()
						if !fenced {
							t.Fatal("retired alias path can be reopened")
						}
					}
				}
			}
		}
		profile, from, to = next, to, from
	}
	for _, supervisor := range []*Supervisor{original, betaSupervisor} {
		if _, err := loadProfileAdoptions(supervisor.config.DataRoot); err != nil {
			t.Fatal("round-trip journal cannot restart", err)
		}
	}
}

func TestProfileAdoptionInterruptedRestoreAndRestart(t *testing.T) {
	_, _, receipt, archive, _ := stoppedAdoptionFixture(t)
	s, o := adoptionTarget(t)
	if _, err := o.RestoreTargetProfile(t.Context(), receipt, 5, adoptionSnapshot(), bytes.NewReader(archive[:len(archive)/2])); err == nil {
		t.Fatal("accepted truncated archive")
	}
	if !s.runtimeHandoverFenced("alpha") {
		t.Fatal("failed transfer lost reservation")
	}
	profile := restoreAdoption(t, o, receipt, archive)
	// Rehearse a crash after durable directory rename but before restored CAS.
	s.mu.Lock()
	record := s.profileAdoptions.Operations[receipt.Identity.OperationID]
	record.State = "reserved"
	s.profileAdoptions.Operations[receipt.Identity.OperationID] = record
	err := s.saveProfileAdoptionsLocked(s.profileAdoptions)
	s.mu.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Close(t.Context()); err != nil {
		t.Fatal(err)
	}
	restarted, err := New(context.Background(), s.config)
	if err != nil {
		t.Fatal(err)
	}
	defer restarted.Close(t.Context())
	o, err = NewOrchestrator(OrchestratorConfig{CellID: "cell-two", Token: testControlToken, Supervisor: restarted, DashboardAuth: o.dashboardAuth})
	if err != nil {
		t.Fatal(err)
	}
	recovered, err := o.RestoreTargetProfile(t.Context(), receipt, 5, adoptionSnapshot(), nil)
	if err != nil || recovered != profile {
		t.Fatalf("rename recovery failed: %v", err)
	}
	startAdoptionParked(t, o, profile, 1)
	if err := restarted.Close(t.Context()); err != nil {
		t.Fatal(err)
	}
	third, err := New(context.Background(), s.config)
	if err != nil {
		t.Fatal(err)
	}
	defer third.Close(t.Context())
	thirdOrchestrator, err := NewOrchestrator(OrchestratorConfig{CellID: "cell-two", Token: testControlToken, Supervisor: third, DashboardAuth: o.dashboardAuth})
	if err != nil {
		t.Fatal(err)
	}
	startAdoptionParked(t, thirdOrchestrator, profile, 1)
	app, _ := third.Application("alpha")
	if app.DataDir != filepath.Join(s.config.DataRoot, filepath.FromSlash(profile.Directory)) {
		t.Fatal("restart forgot active generation")
	}
}

func TestProfileAdoptionRejectsTamperingBeforeActivation(t *testing.T) {
	_, _, receipt, archive, _ := stoppedAdoptionFixture(t)
	for _, failure := range []string{"digest", "epoch", "profile-id", "source-cell", "config", "running", "existing", "canceled"} {
		t.Run(failure, func(t *testing.T) {
			s, o := adoptionTarget(t)
			candidate, snapshot, epoch, ctx := receipt, adoptionSnapshot(), uint64(5), t.Context()
			switch failure {
			case "digest":
				candidate.SHA256 = strings.Repeat("a", 64)
			case "epoch":
				epoch++
			case "profile-id":
				candidate.ProfileID = "wrong"
			case "source-cell":
				candidate.Identity.SourceCellID = "cell-two"
			case "config":
				snapshot.Sections["scheduler"] = json.RawMessage(`{"minAttackDelay":10}`)
			case "running":
				if _, err := s.AddAccount(t.Context(), AccountConfig{ID: "alpha"}); err != nil {
					t.Fatal(err)
				}
			case "existing":
				if err := os.MkdirAll(filepath.Join(s.config.DataRoot, "Accounts", "alpha"), 0700); err != nil {
					t.Fatal(err)
				}
			case "canceled":
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				cancel()
			}
			if _, err := o.RestoreTargetProfile(ctx, candidate, epoch, snapshot, bytes.NewReader(archive)); err == nil {
				t.Fatal("accepted invalid transfer")
			}
		})
	}
	s, o := adoptionTarget(t)
	profile := restoreAdoption(t, o, receipt, archive)
	destination := filepath.Join(s.config.DataRoot, filepath.FromSlash(profile.Directory))
	if err := os.WriteFile(filepath.Join(destination, "unknown-history.bin"), []byte("tampered"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := o.ActivateTargetProfile(t.Context(), profile); err == nil {
		t.Fatal("activated altered profile")
	}
	if !s.runtimeHandoverFenced("alpha") {
		t.Fatal("failed activation opened target")
	}
}

func TestProfileAdoptionMissingIdentityNeverCreatesFreshProfile(t *testing.T) {
	_, _, receipt, archive, _ := stoppedAdoptionFixture(t)
	s, o := adoptionTarget(t)
	profile := restoreAdoption(t, o, receipt, archive)
	if _, err := o.ActivateTargetProfile(t.Context(), profile); err != nil {
		t.Fatal(err)
	}
	identityPath := filepath.Join(s.config.DataRoot, filepath.FromSlash(profile.Directory), "Runtime", "ProfileID")
	if err := os.Rename(identityPath, identityPath+".saved"); err != nil {
		t.Fatal(err)
	}
	assignment := testAssignment("alpha", "tenant-one", 5, o.now().Add(time.Minute))
	assignment.DesiredConfigurationRevision, assignment.DesiredConfigurationDigest = 39, receipt.Identity.ConfigurationDigest
	if _, err := o.Reconcile(t.Context(), ReconcileRequest{1, 1, []RuntimeAssignment{assignment}}); err == nil {
		t.Fatal("lost profile was recreated")
	}
	if _, err := os.Stat(identityPath); !os.IsNotExist(err) {
		t.Fatal("created new profile identity")
	}
}

func TestProfileAdoptionCorruptJournalFailsClosed(t *testing.T) {
	for _, raw := range []string{`{}`, `{"schemaVersion":1,"operations":{},"current":{},"retired":{},"extra":1}`, `{"schemaVersion":1,"schemaVersion":1,"operations":{},"current":{},"retired":{}}`, `{"schemaVersion":1,"operations":{},"current":{"alpha":"lost"},"retired":{}}`} {
		root := t.TempDir()
		if err := os.Mkdir(filepath.Join(root, "Accounts"), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, "Accounts", profileAdoptionFile), []byte(raw), 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := New(t.Context(), Config{DataRoot: root, Offline: true}); err == nil {
			t.Fatal("accepted corrupt journal")
		}
	}
	_, _, receipt, archive, _ := stoppedAdoptionFixture(t)
	s, o := adoptionTarget(t)
	restoreAdoption(t, o, receipt, archive)
	if _, err := NewOrchestrator(OrchestratorConfig{CellID: "wrong-cell", Token: testControlToken, Supervisor: s, DashboardAuth: o.dashboardAuth}); err == nil {
		t.Fatal("opened another cell's adoption journal")
	}
}

// Ensure recovery of a durable generation never reads or copies an untrusted
// retry stream over the already-restored profile.
type rejectingAdoptionReader struct{ t *testing.T }

func (reader rejectingAdoptionReader) Read([]byte) (int, error) {
	reader.t.Error("read retry stream over existing generation")
	return 0, io.ErrUnexpectedEOF
}

func TestProfileAdoptionRestoreRetryDoesNotOverwrite(t *testing.T) {
	_, _, receipt, archive, _ := stoppedAdoptionFixture(t)
	_, o := adoptionTarget(t)
	profile := restoreAdoption(t, o, receipt, archive)
	got, err := o.RestoreTargetProfile(t.Context(), receipt, 5, adoptionSnapshot(), rejectingAdoptionReader{t})
	if err != nil || got != profile {
		t.Fatal("idempotent restore failed", err)
	}
}

func TestProfileAdoptionRootOwnershipExcludesAnotherSupervisor(t *testing.T) {
	s := newTestSupervisor(t)
	if other, err := New(t.Context(), s.config); err == nil {
		other.Close(t.Context())
		t.Fatal("second supervisor can choose a different profile generation")
	}
	if err := s.Close(t.Context()); err != nil {
		t.Fatal(err)
	}
	other, err := New(t.Context(), s.config)
	if err != nil {
		t.Fatal("clean shutdown did not release root ownership", err)
	}
	defer other.Close(t.Context())
}

func TestProfileAdoptionJournalWriteFailureRemainsClosed(t *testing.T) {
	_, _, receipt, archive, _ := stoppedAdoptionFixture(t)
	s, o := adoptionTarget(t)
	journal := filepath.Join(s.config.DataRoot, "Accounts", profileAdoptionFile)
	// An empty directory at the journal filename forces atomic rename to fail.
	if err := os.Mkdir(journal, 0700); err != nil {
		t.Fatal(err)
	}
	if _, err := o.RestoreTargetProfile(t.Context(), receipt, 5, adoptionSnapshot(), bytes.NewReader(archive)); err == nil {
		t.Fatal("acknowledged unpersisted reservation")
	}
	if !s.runtimeHandoverFenced("alpha") {
		t.Fatal("failed reservation did not stay closed in memory")
	}
	if err := os.Remove(journal); err != nil {
		t.Fatal(err)
	} // This test's empty obstacle only.
	profile := restoreAdoption(t, o, receipt, archive)
	if err := os.Rename(journal, journal+".saved"); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(journal, 0700); err != nil {
		t.Fatal(err)
	}
	if _, err := o.ActivateTargetProfile(t.Context(), profile); err == nil {
		t.Fatal("acknowledged unpersisted activation")
	}
	if !s.runtimeHandoverFenced("alpha") {
		t.Fatal("failed activation opened runtime")
	}
	if err := os.Remove(journal); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(journal+".saved", journal); err != nil {
		t.Fatal(err)
	}
	startAdoptionParked(t, o, profile, 1)
}
