package App

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"CitadelDesktop/Server/Intent"
	"CitadelDesktop/Server/Session"
)

func TestBackgroundPrepareIntentReauthorizesSavedLogin(t *testing.T) {
	dataDir := t.TempDir()
	sessionDir := filepath.Join(dataDir, "Session")
	if err := os.MkdirAll(sessionDir, 0o700); err != nil {
		t.Fatal(err)
	}
	credentialPath := filepath.Join(sessionDir, "LoginCredentials.json")
	writeTestJSON(t, credentialPath, map[string]any{
		"schemaVersion": 1,
		"capturedAt":    time.Now().UTC(),
		"autoRestore":   false,
		"username":      "saved-player",
		"password":      "saved-password",
	})
	writeTestJSON(t, filepath.Join(sessionDir, "GameConnection.json"), map[string]any{
		"schemaVersion": 1,
		"capturedAt":    time.Now().UTC(),
		"serverUrl":     "wss://ep-live-us1-game.goodgamestudios.com:443",
		"namespace":     "EmpireEx_21",
		"clientBuild":   "1165009",
		"platform":      "web-html5",
		"loginContext": map[string]any{
			"LANG": "en",
			"DID":  0,
			"AID":  "",
			"PL":   1,
		},
	})

	application, err := New(context.Background(), Config{
		DataDir:   dataDir,
		Offline:   true,
		Transport: Session.NewUnavailableTransport(),
	})
	if err != nil {
		t.Fatal(err)
	}
	receipt := application.Intents.Submit(t.Context(), Intent.Request{
		ID: "prepare-background-login", Name: "session.background.prepare", Actor: "ui",
	})
	if receipt.Status != Intent.StatusSucceeded {
		t.Fatalf("prepare intent failed: %+v", receipt)
	}
	var saved struct {
		AutoRestore bool `json:"autoRestore"`
	}
	contents, err := os.ReadFile(credentialPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(contents, &saved); err != nil {
		t.Fatal(err)
	}
	if !saved.AutoRestore {
		t.Fatal("prepare intent did not reauthorize the saved login")
	}
}

func writeTestJSON(t *testing.T, path string, value any) {
	t.Helper()
	contents, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, append(contents, '\n'), 0o600); err != nil {
		t.Fatal(err)
	}
}
