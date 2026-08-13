package App

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"CitadelDesktop/Server/Configuration"
	"CitadelDesktop/Server/Session"
	"CitadelDesktop/Server/State"
)

func TestApplicationSelectsConfiguredSessionTransportAtStartup(t *testing.T) {
	for _, test := range []struct {
		name string
		mode string
		want Session.ConnectionMode
	}{
		{name: "full application is the default", mode: "full", want: Session.ConnectionModeFull},
		{name: "background only", mode: "background", want: Session.ConnectionModeBackground},
	} {
		t.Run(test.name, func(t *testing.T) {
			dataDir := t.TempDir()
			configuration, err := Configuration.Open(dataDir, defaultConfiguration())
			if err != nil {
				t.Fatal(err)
			}
			if _, err := configuration.Update(
				"session.connection", json.RawMessage(`{"mode":"`+test.mode+`"}`),
			); err != nil {
				t.Fatal(err)
			}
			application, err := New(context.Background(), Config{
				DataDir: dataDir, Offline: true, Chromium: &Session.ChromiumConfig{},
			})
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				application.Telemetry.Close()
				_ = application.OperationStore.Close()
				_ = application.ReportStore.Close()
				_ = application.ProfileLease.Close()
			})
			if got := application.Session.Status().Mode; got != test.want {
				t.Fatalf("startup session mode = %q, want %q", got, test.want)
			}
		})
	}
}

func TestApplicationUsesRecoveredServerSelectionWithoutCapturedConnectionProfile(t *testing.T) {
	dataDir := t.TempDir()
	configuration, err := Configuration.Open(dataDir, defaultConfiguration())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := configuration.Update(
		"session.connection", json.RawMessage(`{"mode":"background"}`),
	); err != nil {
		t.Fatal(err)
	}
	sessionDirectory := filepath.Join(dataDir, "Session")
	if err := os.MkdirAll(sessionDirectory, 0o700); err != nil {
		t.Fatal(err)
	}
	credential := []byte(`{"schemaVersion":1,"capturedAt":"2026-08-09T00:00:00Z","autoRestore":true,"username":"test-player","password":"test-password"}`)
	if err := os.WriteFile(filepath.Join(sessionDirectory, "LoginCredentials.json"), credential, 0o600); err != nil {
		t.Fatal(err)
	}
	state := State.NewGameState()
	state.Session.ServerURL = "wss://ep-live-us1-game.goodgamestudios.com:443"
	state.Session.ChangedAt = time.Now().UTC()
	if err := State.SaveSnapshot(dataDir, state); err != nil {
		t.Fatal(err)
	}

	application, err := New(context.Background(), Config{
		DataDir: dataDir, Offline: true, Chromium: &Session.ChromiumConfig{},
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		application.Telemetry.Close()
		_ = application.OperationStore.Close()
		_ = application.ReportStore.Close()
		_ = application.ProfileLease.Close()
	})
	status := application.Session.Status()
	if status.Mode != Session.ConnectionModeBackground || status.State != "stopped" ||
		status.ServerURL != state.Session.ServerURL {
		t.Fatalf("recovered server selection did not bootstrap background mode: %+v", status)
	}
}
