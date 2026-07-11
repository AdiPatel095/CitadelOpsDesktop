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
