package Session

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestChromiumDefinitionsCoverSupportedBrowsers(t *testing.T) {
	wanted := map[string]bool{
		"brave": false, "chromium": false, "chrome": false, "edge": false,
		"vivaldi": false, "opera": false, "arc": false,
	}
	for _, definition := range chromiumBrowserDefinitions() {
		if _, ok := wanted[definition.ID]; ok {
			wanted[definition.ID] = true
		}
	}
	for id, found := range wanted {
		if !found {
			t.Errorf("browser definition %q is missing", id)
		}
	}
}

func TestNormalizeBrowserAliases(t *testing.T) {
	cases := map[string]string{
		"Brave Browser":        "brave",
		"google-chrome-stable": "chrome",
		"Microsoft Edge":       "edge",
		"msedge":               "edge",
		"Vivaldi Stable":       "vivaldi",
	}
	for input, expected := range cases {
		if actual := normalizeBrowserID(input); actual != expected {
			t.Errorf("normalizeBrowserID(%q) = %q, want %q", input, actual, expected)
		}
	}
}

func TestBrowserIDFromSystemIdentifier(t *testing.T) {
	cases := map[string]string{
		"com.brave.Browser":          "brave",
		"chromium.desktop":           "chromium",
		"com.google.Chrome":          "chrome",
		"ChromeHTML":                 "chrome",
		"MSEdgeHTM":                  "edge",
		"vivaldi-stable.desktop":     "vivaldi",
		"OperaStable":                "opera",
		"company.thebrowser.Browser": "arc",
		"org.mozilla.firefox":        "",
	}
	for input, expected := range cases {
		if actual := browserIDFromSystemIdentifier(input); actual != expected {
			t.Errorf("browserIDFromSystemIdentifier(%q) = %q, want %q", input, actual, expected)
		}
	}
}

func TestAutomaticBrowserCandidatePrefersSystemDefault(t *testing.T) {
	candidates := []BrowserCandidate{
		{ID: "brave", Name: "Brave"},
		{ID: "chrome", Name: "Google Chrome", IsDefault: true},
	}
	if actual := automaticBrowserCandidate(candidates); actual.ID != "chrome" {
		t.Fatalf("automaticBrowserCandidate() = %q, want chrome", actual.ID)
	}
	if actual := automaticBrowserCandidate(candidates[:1]); actual.ID != "brave" {
		t.Fatalf("automaticBrowserCandidate(single) = %q, want brave", actual.ID)
	}
}

func TestBrowserPreferenceRoundTrip(t *testing.T) {
	dataDir := t.TempDir()
	candidate := BrowserCandidate{ID: "brave", Name: "Brave", ExecutablePath: "/example/brave"}
	if err := saveBrowserPreference(dataDir, candidate); err != nil {
		t.Fatal(err)
	}
	if actual := loadBrowserPreference(dataDir); actual != "brave" {
		t.Fatalf("loadBrowserPreference() = %q, want brave", actual)
	}
	info, err := os.Stat(filepath.Join(dataDir, "Config", browserSelectionFile))
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm()&0o077 != 0 {
		t.Fatalf("browser preference permissions = %o, want owner-only", info.Mode().Perm())
	}
}

func TestResolveCustomBrowser(t *testing.T) {
	executable := filepath.Join(t.TempDir(), "custom-browser")
	if runtime.GOOS == "windows" {
		executable += ".exe"
	}
	if err := os.WriteFile(executable, []byte("test browser"), 0o700); err != nil {
		t.Fatal(err)
	}
	candidate, err := resolveCustomBrowser(executable)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(candidate.ID, "custom-custom-browser-") {
		t.Fatalf("custom browser id = %q", candidate.ID)
	}
	if candidate.ExecutablePath != executable {
		t.Fatalf("custom browser path = %q, want %q", candidate.ExecutablePath, executable)
	}
}

func TestSelectBrowserIsSavedForNextRestart(t *testing.T) {
	dataDir := t.TempDir()
	browserDir := t.TempDir()
	firstExecutable := filepath.Join(browserDir, "first-browser")
	secondExecutable := filepath.Join(browserDir, "second-browser")
	if runtime.GOOS == "windows" {
		firstExecutable += ".exe"
		secondExecutable += ".exe"
	}
	for _, executable := range []string{firstExecutable, secondExecutable} {
		if err := os.WriteFile(executable, []byte("test browser"), 0o700); err != nil {
			t.Fatal(err)
		}
	}

	transport := NewChromiumTransport(ChromiumConfig{
		DataDir: dataDir, ExecutablePath: firstExecutable,
	})
	if err := transport.SelectBrowser(secondExecutable); err != nil {
		t.Fatal(err)
	}
	inventory := transport.BrowserInventory()
	if inventory.Current == nil || inventory.Current.ExecutablePath != firstExecutable {
		t.Fatalf("current browser = %#v, want %q", inventory.Current, firstExecutable)
	}
	if inventory.Selected == nil || inventory.Selected.ExecutablePath != secondExecutable {
		t.Fatalf("selected browser = %#v, want %q", inventory.Selected, secondExecutable)
	}
	if !inventory.RestartRequired {
		t.Fatal("browser change should require an app restart")
	}

	t.Setenv("CITADEL_BROWSER", "")
	t.Setenv("CITADEL_BROWSER_PATH", "")
	restarted := NewChromiumTransport(ChromiumConfig{DataDir: dataDir})
	restartedInventory := restarted.BrowserInventory()
	if restartedInventory.Current == nil || restartedInventory.Current.ExecutablePath != secondExecutable {
		t.Fatalf("restarted browser = %#v, want %q", restartedInventory.Current, secondExecutable)
	}
	if restartedInventory.RestartRequired {
		t.Fatal("saved browser should be current after restart")
	}
}
