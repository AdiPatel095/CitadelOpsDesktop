package Session

import (
	"context"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestBackgroundLoginStoreConfiguresIndependentLogin(t *testing.T) {
	dataDir := t.TempDir()
	store := NewBackgroundLoginStore(dataDir)
	status, err := store.Configure(BackgroundLoginInput{
		Username: " test player ",
		Password: " test password ",
		Server:   "us1",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !status.Configured || status.Server != "US1" ||
		status.ServerURL != "wss://ep-live-us1-game.goodgamestudios.com:443" || status.Language != "en" {
		t.Fatalf("unexpected background login status: %+v", status)
	}
	credential, err := loadBackgroundLoginCredential(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	if credential.Username != "test player" || credential.Password != " test password " ||
		credential.ServerURL != status.ServerURL || credential.Language != "en" || !credential.AutoRestore {
		t.Fatal("protected credential did not preserve the configured login")
	}
	reloaded, err := store.Status()
	if err != nil {
		t.Fatal(err)
	}
	if reloaded.Configured != status.Configured || reloaded.Server != status.Server ||
		reloaded.ServerURL != status.ServerURL || reloaded.Language != status.Language ||
		reloaded.UpdatedAt == nil || status.UpdatedAt == nil || !reloaded.UpdatedAt.Equal(*status.UpdatedAt) {
		t.Fatalf("reloaded background login status = %+v, want %+v", reloaded, status)
	}
	if runtime.GOOS != "windows" {
		directoryInfo, err := os.Stat(filepath.Join(dataDir, "Session"))
		if err != nil {
			t.Fatal(err)
		}
		credentialInfo, err := os.Stat(backgroundLoginCredentialPath(dataDir))
		if err != nil {
			t.Fatal(err)
		}
		if directoryInfo.Mode().Perm() != 0o700 || credentialInfo.Mode().Perm() != 0o600 {
			t.Fatalf(
				"background login permissions = directory %o file %o, want 700 and 600",
				directoryInfo.Mode().Perm(), credentialInfo.Mode().Perm(),
			)
		}
	}
}

func TestBackgroundLoginStoreRequiresExplicitServerCode(t *testing.T) {
	store := NewBackgroundLoginStore(t.TempDir())
	for _, selection := range []string{
		"", "https://empire.goodgamestudios.com", "wss://ep-live-us1-game.goodgamestudios.com:443", "us1.example",
	} {
		_, err := store.Configure(BackgroundLoginInput{
			Username: "player", Password: "password", Server: selection,
		})
		if err == nil || !strings.Contains(err.Error(), "server code") {
			t.Fatalf("server selection %q returned %v", selection, err)
		}
	}
}

func TestBackgroundLoginStatusDoesNotTreatLegacyCredentialWithoutServerAsConfigured(t *testing.T) {
	dataDir := t.TempDir()
	if err := saveLoginCredential(dataDir, persistedLoginCredential{
		AutoRestore: true, Username: "player", Password: "password",
	}); err != nil {
		t.Fatal(err)
	}
	status, err := NewBackgroundLoginStore(dataDir).Status()
	if err != nil {
		t.Fatal(err)
	}
	if status.Configured || status.Server != "" || status.ServerURL != "" {
		t.Fatalf("legacy credential unexpectedly reported as configured: %+v", status)
	}
}

func TestBackgroundLoginDerivesDirectHandshakeWithoutFullModeCapture(t *testing.T) {
	dataDir := t.TempDir()
	if _, err := NewBackgroundLoginStore(dataDir).Configure(BackgroundLoginInput{
		Username: "test-player", Password: "test-password", Server: "US1",
	}); err != nil {
		t.Fatal(err)
	}
	transport := NewDirectWebSocketTransport(DirectWebSocketConfig{DataDir: dataDir})
	status := transport.Status()
	if status.State != "stopped" || status.ServerURL != "wss://ep-live-us1-game.goodgamestudios.com:443" ||
		status.Namespace != defaultGameNamespace {
		t.Fatalf("explicit background login did not derive the connection profile: %+v", status)
	}
	frame, err := transport.loginFrame(7, 1500, 75)
	if err != nil {
		t.Fatal(err)
	}
	for _, fragment := range []string{
		`"NOM":"test-player"`, `"PW":"test-password"`, `"LANG":"en"`,
		`"DID":"0"`, `"LT":null`, `"REF":"https://empire.goodgamestudios.com"`,
	} {
		if !strings.Contains(frame, fragment) {
			t.Fatalf("derived login frame is missing %s: %s", fragment, frame)
		}
	}
	if strings.Contains(frame, `"RCT"`) {
		t.Fatalf("derived login frame unexpectedly contains a rotating login token: %s", frame)
	}
}

func TestLegacyLoginUsesExplicitConfiguredServerWithoutCapturedProfile(t *testing.T) {
	dataDir := t.TempDir()
	if err := saveLoginCredential(dataDir, persistedLoginCredential{
		AutoRestore: true, Username: "test-player", Password: "test-password",
	}); err != nil {
		t.Fatal(err)
	}
	transport := NewDirectWebSocketTransport(DirectWebSocketConfig{
		DataDir: dataDir, Server: "US1",
	})
	status := transport.Status()
	if status.State != "stopped" || status.ServerURL != "wss://ep-live-us1-game.goodgamestudios.com:443" ||
		status.Namespace != defaultGameNamespace {
		t.Fatalf("configured server did not bootstrap the legacy login: %+v", status)
	}
}

func TestResolveCurrentGameBuildUsesHashedOfficialFrontendBundle(t *testing.T) {
	client := &http.Client{Transport: directRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		body := ""
		switch request.URL.String() {
		case directGameIndexURL:
			body = `<link rel="prefetch" href="CacheBreaker.bundle.abcdef123456.js">`
		case "https://empire-html5.goodgamestudios.com/default/CacheBreaker.bundle.abcdef123456.js":
			body = `module.exports={name:"TranspilationEmpire",version:"1.165.9"}`
		default:
			return &http.Response{
				StatusCode: http.StatusNotFound,
				Body:       io.NopCloser(strings.NewReader("")),
				Header:     make(http.Header),
				Request:    request,
			}, nil
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(body)),
			Header:     make(http.Header),
			Request:    request,
		}, nil
	})}
	build, err := resolveCurrentGameBuild(context.Background(), client)
	if err != nil {
		t.Fatal(err)
	}
	if build != "1165009" {
		t.Fatalf("resolved client build = %q, want 1165009", build)
	}
}

func TestResolveCacheBreakerBundleURLRejectsOffOriginAsset(t *testing.T) {
	_, err := resolveCacheBreakerBundleURL([]byte(
		`<script src="https://example.invalid/default/CacheBreaker.bundle.abcdef.js"></script>`,
	))
	if err == nil {
		t.Fatal("off-origin game frontend version bundle was accepted")
	}
}

func TestLiveResolveCurrentGameBuild(t *testing.T) {
	if os.Getenv("CITADEL_TEST_LIVE_FRONTEND") != "1" {
		t.Skip("set CITADEL_TEST_LIVE_FRONTEND=1 to check the current official game frontend")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	build, err := resolveCurrentGameBuild(ctx, &http.Client{Timeout: 10 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	if !gameBuildPattern.MatchString(build) {
		t.Fatalf("current official game frontend returned invalid build %q", build)
	}
	t.Logf("current official game client build: %s", build)
}

type directRoundTripFunc func(*http.Request) (*http.Response, error)

func (function directRoundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return function(request)
}
