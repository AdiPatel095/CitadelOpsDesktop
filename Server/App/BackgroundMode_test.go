package App

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
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

func TestBackgroundOnlyApplicationReservesLoginWritesForControlPlane(t *testing.T) {
	dataDir := t.TempDir()
	runtimeContext, cancelRuntime := context.WithCancel(context.Background())
	application, err := New(runtimeContext, Config{
		DataDir: dataDir, Offline: true, BackgroundOnly: true,
		Transport: Session.NewUnavailableTransport(),
	})
	if err != nil {
		cancelRuntime()
		t.Fatal(err)
	}
	application.Start(runtimeContext)
	t.Cleanup(func() {
		cancelRuntime()
		waitContext, cancelWait := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancelWait()
		if err := application.Wait(waitContext); err != nil {
			t.Errorf("wait for hosted application shutdown: %v", err)
		}
	})

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(
		http.MethodPost,
		"/api/v2/session/background-login",
		strings.NewReader(`{"username":"public-user","password":"public-password","server":"US1"}`),
	)
	request.Header.Set("Content-Type", "application/json")
	application.API.Handler().ServeHTTP(recorder, request)
	if recorder.Code != http.StatusForbidden || !strings.Contains(recorder.Body.String(), "background_login_managed") {
		t.Fatalf("hosted application background login returned HTTP %d: %s", recorder.Code, recorder.Body.String())
	}
	credentialPath := filepath.Join(dataDir, "Session", "BackgroundLogin.json")
	if _, err := os.Stat(credentialPath); !os.IsNotExist(err) {
		t.Fatalf("hosted public API wrote %s: %v", credentialPath, err)
	}

	// The authenticated orchestrator installs credentials through this direct
	// account-private store; the API gate must not disable that control path.
	status, err := application.BackgroundLogin.Configure(Session.BackgroundLoginInput{
		Username: "control-user", Password: "control-password", Server: "US1", Language: "en",
	})
	if err != nil || !status.Configured || status.Server != "US1" {
		t.Fatalf("control-plane login install = %+v err=%v", status, err)
	}
	if _, err := os.Stat(credentialPath); err != nil {
		t.Fatalf("control-plane credential was not installed: %v", err)
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
