package Session

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"
)

func TestPrepareBackgroundLoginReenablesProtectedBootstrap(t *testing.T) {
	dataDir := t.TempDir()
	capturedAt := time.Now().UTC().Add(-time.Hour)
	if err := saveLoginCredential(dataDir, persistedLoginCredential{
		SchemaVersion: loginCredentialSchemaVersion,
		CapturedAt:    capturedAt,
		AutoRestore:   false,
		Username:      "saved-player",
		Password:      "saved-password",
	}); err != nil {
		t.Fatal(err)
	}
	if err := saveGameConnectionProfile(dataDir, validBackgroundTestProfile()); err != nil {
		t.Fatal(err)
	}

	if err := PrepareBackgroundLogin(dataDir); err != nil {
		t.Fatal(err)
	}
	credential, err := loadLoginCredential(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	if !credential.AutoRestore || credential.Username != "saved-player" ||
		credential.Password != "saved-password" || !credential.CapturedAt.After(capturedAt) {
		t.Fatalf("unexpected prepared credential: %+v", credential)
	}
	info, err := os.Stat(loginCredentialPath(dataDir))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("saved login permissions = %o, want 600", info.Mode().Perm())
	}
}

func TestPrepareBackgroundLoginFailsClosedBeforeChangingCredential(t *testing.T) {
	dataDir := t.TempDir()
	if err := saveLoginCredential(dataDir, persistedLoginCredential{
		SchemaVersion: loginCredentialSchemaVersion,
		CapturedAt:    time.Now().UTC(),
		AutoRestore:   false,
		Username:      "saved-player",
		Password:      "saved-password",
	}); err != nil {
		t.Fatal(err)
	}

	err := PrepareBackgroundLogin(dataDir)
	if err == nil || !strings.Contains(err.Error(), "connection details") {
		t.Fatalf("unexpected missing-profile result: %v", err)
	}
	credential, loadErr := loadLoginCredential(dataDir)
	if loadErr != nil {
		t.Fatal(loadErr)
	}
	if credential.AutoRestore {
		t.Fatal("failed preparation changed the saved login authorization")
	}
}

func TestDirectBackgroundTransportRecoversDisabledLoginInPlace(t *testing.T) {
	dataDir := t.TempDir()
	if err := saveLoginCredential(dataDir, persistedLoginCredential{
		SchemaVersion: loginCredentialSchemaVersion,
		CapturedAt:    time.Now().UTC(),
		AutoRestore:   false,
		Username:      "saved-player",
		Password:      "saved-password",
	}); err != nil {
		t.Fatal(err)
	}
	if err := saveGameConnectionProfile(dataDir, validBackgroundTestProfile()); err != nil {
		t.Fatal(err)
	}
	transport := NewDirectWebSocketTransport(DirectWebSocketConfig{DataDir: dataDir})
	if status := transport.Status(); status.State != "unavailable" ||
		!strings.Contains(status.Detail, "has been disabled") {
		t.Fatalf("unexpected initial status: %+v", status)
	}

	if err := transport.PrepareBackgroundMode(); err != nil {
		t.Fatal(err)
	}
	status := transport.Status()
	if status.State != "stopped" || status.Detail != "" || transport.resolveErr != nil ||
		!transport.credential.AutoRestore {
		t.Fatalf("transport was not repaired in place: status=%+v error=%v", status, transport.resolveErr)
	}
}

func validBackgroundTestProfile() gameConnectionProfile {
	return gameConnectionProfile{
		SchemaVersion: gameConnectionProfileSchemaVersion,
		CapturedAt:    time.Now().UTC(),
		ServerURL:     "wss://ep-live-us1-game.goodgamestudios.com:443",
		Namespace:     "EmpireEx_21",
		ClientBuild:   "1165009",
		Platform:      "web-html5",
		LoginContext: map[string]json.RawMessage{
			"LANG": json.RawMessage(`"en"`),
			"DID":  json.RawMessage(`0`),
			"AID":  json.RawMessage(`""`),
			"PL":   json.RawMessage(`1`),
		},
	}
}
