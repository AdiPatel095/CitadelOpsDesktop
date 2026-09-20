package Accounts

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"CitadelDesktop/Server/State"
)

func TestSettingsOnlySwitchKeepsBothLocalProfilesAndLatestSettings(t *testing.T) {
	stable, _, stableWorker, identity, _ := handoverSourceFixture(t)
	beta, betaWorker := adoptionTarget(t)
	first, _ := stable.Application("alpha")
	stableDir := first.DataDir
	if err := os.WriteFile(filepath.Join(stableDir, "local-history"), []byte("stable history"), 0600); err != nil {
		t.Fatal(err)
	}
	stableID, _ := os.ReadFile(filepath.Join(stableDir, "Runtime", "ProfileID"))
	source, target := stable, beta
	from, to := stableWorker, betaWorker
	var betaDir string
	var betaID []byte
	for round := 0; round < 3; round++ {
		identity.OperationID = "local-round-" + strconv.Itoa(round)
		identity.SourceCellID = from.cellID
		if _, err := from.prepareSourceHandover(t.Context(), identity, false); err != nil {
			t.Fatal(err)
		}
		if len(source.AccountIDs()) != 0 {
			t.Fatal("source did not stop")
		}
		binding, err := to.PrepareLocalProfile(t.Context(), identity)
		if err != nil {
			t.Fatal(err)
		}
		directory := filepath.Join(target.config.DataRoot, filepath.FromSlash(binding.Directory))
		if round == 0 {
			betaDir = directory
			betaID, _ = os.ReadFile(filepath.Join(betaDir, "Runtime", "ProfileID"))
			if bytes.Equal(betaID, stableID) {
				t.Fatal("copied the source profile identity")
			}
			if _, err := os.Stat(filepath.Join(betaDir, "local-history")); !os.IsNotExist(err) {
				t.Fatal("source history was copied")
			}
			if err := os.WriteFile(filepath.Join(betaDir, "local-history"), []byte("beta history"), 0600); err != nil {
				t.Fatal(err)
			}
		}
		if _, err := target.AddAccount(t.Context(), AccountConfig{ID: "alpha"}); err == nil {
			t.Fatal("unactivated target started")
		}
		if target.Capacity().Starting != 1 {
			t.Fatal("reservation not counted")
		}
		if _, err := to.ActivateLocalProfile(t.Context(), binding); err != nil {
			t.Fatal(err)
		}
		if _, err := to.ActivateLocalProfile(t.Context(), binding); err != nil {
			t.Fatal("activation not retry-safe", err)
		}
		assignment := testAssignment("alpha", "tenant-one", binding.TargetEpoch, to.now().Add(10*time.Minute))
		assignment.DesiredConfigurationRevision = identity.ConfigurationRevision
		assignment.DesiredConfigurationDigest = identity.ConfigurationDigest
		if _, err := to.Reconcile(t.Context(), ReconcileRequest{SchemaVersion: 1, Revision: to.Status().DesiredRevision + 1, Runtimes: []RuntimeAssignment{assignment}}); err != nil {
			t.Fatal(err)
		}
		app, ok := target.Application("alpha")
		if !ok || app.DataDir != directory {
			t.Fatal("target did not use its own profile")
		}
		if to.configurationReady("alpha", assignment) {
			t.Fatal("target accepted stale/missing canonical settings")
		}
		snapshot := adoptionSnapshot()
		snapshot.Revision = identity.ConfigurationRevision
		snapshot.Sections["scheduler"] = json.RawMessage(`{"minAttackDelay":` + strconv.Itoa(9+round) + `}`)
		server := httptest.NewServer(to.Handler())
		response, body := controlRequest(t, http.MethodPut, server.URL+"/orchestrator/v1/runtimes/alpha/configuration", ConfigurationSyncRequest{1, binding.TargetEpoch, identity.ConfigurationDigest, snapshot})
		server.Close()
		if response.StatusCode != http.StatusOK || !to.configurationReady("alpha", assignment) {
			t.Fatalf("settings not applied: %d %s", response.StatusCode, body)
		}
		// Save a newer canonical revision while on this worker. The next switch
		// must use that revision, without importing any local history.
		snapshot.Revision++
		snapshot.Sections["scheduler"] = json.RawMessage(`{"minAttackDelay":` + strconv.Itoa(10+round) + `}`)
		_, digest, err := canonicalConfigurationDigest(snapshot)
		if err != nil {
			t.Fatal(err)
		}
		assignment.DesiredConfigurationRevision = snapshot.Revision
		assignment.DesiredConfigurationDigest = digest
		if _, err := to.Reconcile(t.Context(), ReconcileRequest{SchemaVersion: 1, Revision: to.Status().DesiredRevision + 1, Runtimes: []RuntimeAssignment{assignment}}); err != nil {
			t.Fatal(err)
		}
		server = httptest.NewServer(to.Handler())
		response, body = controlRequest(t, http.MethodPut, server.URL+"/orchestrator/v1/runtimes/alpha/configuration", ConfigurationSyncRequest{1, binding.TargetEpoch, digest, snapshot})
		server.Close()
		if response.StatusCode != http.StatusOK {
			t.Fatalf("new settings: %d %s", response.StatusCode, body)
		}
		identity.SourceEpoch = binding.TargetEpoch
		identity.ConfigurationRevision = snapshot.Revision
		identity.ConfigurationDigest = digest
		if err := from.ReconcileEmptyForLocalTest(t); err != nil {
			t.Fatal(err)
		}
		source, target = target, source
		from, to = to, from
	}
	for _, test := range []struct {
		s            *Supervisor
		dir, history string
		id           []byte
	}{{stable, stableDir, "stable history", stableID}, {beta, betaDir, "beta history", betaID}} {
		content, err := os.ReadFile(filepath.Join(test.dir, "local-history"))
		if err != nil || string(content) != test.history {
			t.Fatal("local history changed", err)
		}
		id, err := os.ReadFile(filepath.Join(test.dir, "Runtime", "ProfileID"))
		if err != nil || !bytes.Equal(id, test.id) {
			t.Fatal("local profile identity changed", err)
		}
		if _, err := os.Stat(filepath.Join(test.s.config.DataRoot, "Transfers")); !os.IsNotExist(err) {
			t.Fatal("archive transport was used")
		}
		config := test.s.config
		if err := test.s.Close(t.Context()); err != nil {
			t.Fatal(err)
		}
		config.RuntimeContext = context.Background()
		restarted, err := New(context.Background(), config)
		if err != nil {
			t.Fatal(err)
		}
		if len(restarted.localProfiles) != 1 {
			t.Fatal("local binding lost on restart")
		}
		if _, err := restarted.AddAccount(t.Context(), AccountConfig{ID: "alpha"}); err == nil {
			t.Fatal("restart lost exact ownership gate")
		}
		if err := restarted.Close(t.Context()); err != nil {
			t.Fatal(err)
		}
	}
}

func (o *Orchestrator) ReconcileEmptyForLocalTest(t *testing.T) error {
	t.Helper()
	_, err := o.Reconcile(t.Context(), ReconcileRequest{SchemaVersion: 1, Revision: o.Status().DesiredRevision + 1})
	return err
}

func TestSettingsOnlyRoutesRequireFenceAndNeverArchive(t *testing.T) {
	s, _, o, identity, _ := handoverSourceFixture(t)
	raw, _ := json.Marshal(identity)
	for _, path := range []string{"local/stop", "local/prepare", "local/activate"} {
		response := transferControlRequest(o, path, "application/json", "1", testControlToken, bytes.NewReader(raw))
		if response.Code != http.StatusNotFound {
			t.Fatalf("disabled route %s = %d", path, response.Code)
		}
		o.handoverTransport = true
		response = transferControlRequest(o, path, "application/json", "", testControlToken, bytes.NewReader(raw))
		if response.Code != http.StatusPreconditionRequired {
			t.Fatalf("unfenced route %s = %d", path, response.Code)
		}
		o.handoverTransport = false
	}
	o.handoverTransport = true
	response := transferControlRequest(o, "local/stop", "application/json", "1", testControlToken, bytes.NewReader(raw))
	if response.Code != http.StatusOK {
		t.Fatalf("stop = %d %s", response.Code, response.Body.String())
	}
	if _, err := os.Stat(filepath.Join(s.config.DataRoot, "Transfers")); !os.IsNotExist(err) {
		t.Fatal("created an archive")
	}
}

func TestSettingsOnlySwitchCannotBypassActiveSafetyLock(t *testing.T) {
	s, _, o, identity, _ := handoverSourceFixture(t)
	app, _ := s.Application("alpha")
	_, err := app.State.ApplyComponents(State.Components(State.ComponentAutomations), func(state *State.GameState) ([]string, bool, error) {
		state.Automations["autoTower"] = State.AutomationState{SafetyLock: State.AutomationSafetyLock{OperationID: "rejection", Opcode: "msd", Code: 311, ObservedAt: o.now()}}
		return nil, true, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if !o.Status().Runtimes[0].ActiveSafetyLock {
		t.Fatal("status hid the active safety lock")
	}
	if _, err = o.prepareSourceHandover(t.Context(), identity, false); err == nil {
		t.Fatal("switch bypassed safety lock")
	}
	if _, ok := s.Application("alpha"); !ok {
		t.Fatal("blocked switch stopped the source")
	}
	if _, ok := s.sourceFence("alpha"); ok {
		t.Fatal("blocked switch fenced the source")
	}
}

func TestLocalReservationCannotBeReplacedOrStartMissingProfile(t *testing.T) {
	_, _, _, identity, _ := handoverSourceFixture(t)
	s, o := adoptionTarget(t)
	binding, err := o.PrepareLocalProfile(t.Context(), identity)
	if err != nil {
		t.Fatal(err)
	}
	changed := identity
	changed.OperationID = "other-operation"
	changed.SourceEpoch += 2
	if _, err = o.PrepareLocalProfile(t.Context(), changed); err == nil {
		t.Fatal("replaced an unresolved reservation")
	}
	if _, err = o.ActivateLocalProfile(t.Context(), binding); err != nil {
		t.Fatal(err)
	}
	profileID := filepath.Join(s.config.DataRoot, filepath.FromSlash(binding.Directory), "Runtime", "ProfileID")
	// Retain the file, but simulate an unavailable identity; never recover by
	// generating a fresh empty profile for an already-known target.
	if err = os.Rename(profileID, profileID+".test-backup"); err != nil {
		t.Fatal(err)
	}
	assignment := testAssignment("alpha", "tenant-one", binding.TargetEpoch, o.now().Add(time.Minute))
	assignment.DesiredConfigurationRevision = identity.ConfigurationRevision
	assignment.DesiredConfigurationDigest = identity.ConfigurationDigest
	if _, err = o.Reconcile(t.Context(), ReconcileRequest{SchemaVersion: 1, Revision: 1, Runtimes: []RuntimeAssignment{assignment}}); err == nil {
		t.Fatal("missing profile was recreated")
	}
}
