package Session

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
)

const browserSelectionFile = "Browser.json"

type BrowserCandidate struct {
	ID             string `json:"id"`
	Name           string `json:"name"`
	ExecutablePath string `json:"executablePath"`
}

type browserDefinition struct {
	ID       string
	Name     string
	Commands []string
	Paths    []string
}

func DiscoverChromiumBrowsers() []BrowserCandidate {
	seen := map[string]struct{}{}
	candidates := make([]BrowserCandidate, 0)
	for _, definition := range chromiumBrowserDefinitions() {
		for _, path := range definition.Paths {
			candidate, ok := browserCandidateAt(definition.ID, definition.Name, path)
			if !ok || pathSeen(seen, candidate.ExecutablePath) {
				continue
			}
			candidates = append(candidates, candidate)
		}
		for _, command := range definition.Commands {
			path, err := exec.LookPath(command)
			if err != nil {
				continue
			}
			candidate, ok := browserCandidateAt(definition.ID, definition.Name, path)
			if !ok || pathSeen(seen, candidate.ExecutablePath) {
				continue
			}
			candidates = append(candidates, candidate)
		}
	}
	return candidates
}

func ResolveChromiumBrowser(preference string, executablePath string) (BrowserCandidate, error) {
	if strings.TrimSpace(executablePath) != "" {
		return resolveCustomBrowser(executablePath)
	}

	preference = strings.TrimSpace(preference)
	if preference == "" {
		if environmentPath := strings.TrimSpace(os.Getenv("CITADEL_BROWSER_PATH")); environmentPath != "" {
			return resolveCustomBrowser(environmentPath)
		}
		preference = strings.TrimSpace(os.Getenv("CITADEL_BROWSER"))
	}
	candidates := DiscoverChromiumBrowsers()
	if preference == "" || strings.EqualFold(preference, "auto") {
		if len(candidates) == 0 {
			return BrowserCandidate{}, fmt.Errorf("no CDP-capable Chromium browser was detected; choose an executable with -browser or CITADEL_BROWSER_PATH")
		}
		return candidates[0], nil
	}
	if filepath.IsAbs(preference) || strings.ContainsAny(preference, `/\\`) {
		return resolveCustomBrowser(preference)
	}

	wanted := normalizeBrowserID(preference)
	for _, candidate := range candidates {
		if candidate.ID == wanted || strings.EqualFold(candidate.Name, preference) {
			return candidate, nil
		}
	}
	if path, err := exec.LookPath(preference); err == nil {
		return resolveCustomBrowser(path)
	}
	available := make([]string, 0, len(candidates))
	for _, candidate := range candidates {
		available = append(available, candidate.ID)
	}
	if len(available) == 0 {
		return BrowserCandidate{}, fmt.Errorf("browser %q was not found and no supported Chromium browser was detected", preference)
	}
	return BrowserCandidate{}, fmt.Errorf("browser %q was not found; detected: %s", preference, strings.Join(available, ", "))
}

func loadBrowserPreference(dataDir string) string {
	contents, err := os.ReadFile(filepath.Join(dataDir, "Config", browserSelectionFile))
	if err != nil {
		return ""
	}
	var saved struct {
		Browser string `json:"browser"`
	}
	if json.Unmarshal(contents, &saved) != nil {
		return ""
	}
	return strings.TrimSpace(saved.Browser)
}

func saveBrowserPreference(dataDir string, candidate BrowserCandidate) error {
	directory := filepath.Join(dataDir, "Config")
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return fmt.Errorf("create browser settings directory: %w", err)
	}
	preference := candidate.ID
	if strings.HasPrefix(candidate.ID, "custom-") {
		preference = candidate.ExecutablePath
	}
	contents, err := json.MarshalIndent(struct {
		Browser string `json:"browser"`
	}{Browser: preference}, "", "  ")
	if err != nil {
		return err
	}
	temporary, err := os.CreateTemp(directory, ".BrowserSelection-*")
	if err != nil {
		return fmt.Errorf("create browser settings: %w", err)
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if err := temporary.Chmod(0o600); err != nil {
		temporary.Close()
		return err
	}
	if _, err := temporary.Write(contents); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	destination := filepath.Join(directory, browserSelectionFile)
	if err := os.Rename(temporaryPath, destination); err != nil {
		_ = os.Remove(destination)
		if retryErr := os.Rename(temporaryPath, destination); retryErr != nil {
			return fmt.Errorf("save browser settings: %w", retryErr)
		}
	}
	return nil
}

func chromiumBrowserDefinitions() []browserDefinition {
	home, _ := os.UserHomeDir()
	definitions := []browserDefinition{
		{ID: "brave", Name: "Brave", Commands: []string{"brave-browser", "brave"}},
		{ID: "chromium", Name: "Chromium", Commands: []string{"chromium", "chromium-browser"}},
		{ID: "chrome", Name: "Google Chrome", Commands: []string{"google-chrome-stable", "google-chrome", "chrome"}},
		{ID: "edge", Name: "Microsoft Edge", Commands: []string{"microsoft-edge-stable", "microsoft-edge", "msedge"}},
		{ID: "vivaldi", Name: "Vivaldi", Commands: []string{"vivaldi-stable", "vivaldi"}},
		{ID: "opera", Name: "Opera", Commands: []string{"opera"}},
		{ID: "arc", Name: "Arc", Commands: []string{"arc"}},
	}
	switch runtime.GOOS {
	case "darwin":
		applications := []string{"/Applications"}
		if home != "" {
			applications = append(applications, filepath.Join(home, "Applications"))
		}
		macExecutables := map[string]string{
			"brave":    "Brave Browser.app/Contents/MacOS/Brave Browser",
			"chromium": "Chromium.app/Contents/MacOS/Chromium",
			"chrome":   "Google Chrome.app/Contents/MacOS/Google Chrome",
			"edge":     "Microsoft Edge.app/Contents/MacOS/Microsoft Edge",
			"vivaldi":  "Vivaldi.app/Contents/MacOS/Vivaldi",
			"opera":    "Opera.app/Contents/MacOS/Opera",
			"arc":      "Arc.app/Contents/MacOS/Arc",
		}
		for index := range definitions {
			for _, applicationsDir := range applications {
				definitions[index].Paths = append(definitions[index].Paths, filepath.Join(applicationsDir, macExecutables[definitions[index].ID]))
			}
		}
	case "windows":
		programFiles := nonEmptyStrings(os.Getenv("PROGRAMFILES"), os.Getenv("PROGRAMFILES(X86)"), os.Getenv("LOCALAPPDATA"))
		windowsPaths := map[string][]string{
			"brave":    {"BraveSoftware/Brave-Browser/Application/brave.exe"},
			"chromium": {"Chromium/Application/chrome.exe"},
			"chrome":   {"Google/Chrome/Application/chrome.exe"},
			"edge":     {"Microsoft/Edge/Application/msedge.exe"},
			"vivaldi":  {"Vivaldi/Application/vivaldi.exe"},
			"opera":    {"Programs/Opera/opera.exe", "Opera/launcher.exe"},
			"arc":      {"Packages/TheBrowserCompany.Arc_ttt1ap7aakyb4/LocalCache/Local/Arc/Application/arc.exe"},
		}
		for index := range definitions {
			for _, root := range programFiles {
				for _, relative := range windowsPaths[definitions[index].ID] {
					definitions[index].Paths = append(definitions[index].Paths, filepath.Join(root, filepath.FromSlash(relative)))
				}
			}
		}
	}
	return definitions
}

func resolveCustomBrowser(path string) (BrowserCandidate, error) {
	path = strings.TrimSpace(path)
	if !filepath.IsAbs(path) && !strings.ContainsAny(path, `/\\`) {
		resolved, err := exec.LookPath(path)
		if err != nil {
			return BrowserCandidate{}, fmt.Errorf("find browser executable %q: %w", path, err)
		}
		path = resolved
	}
	path, err := filepath.Abs(path)
	if err != nil {
		return BrowserCandidate{}, err
	}
	for _, candidate := range DiscoverChromiumBrowsers() {
		if samePath(candidate.ExecutablePath, path) {
			return candidate, nil
		}
	}
	name := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	if name == "" {
		name = "Custom Chromium"
	}
	digest := sha256.Sum256([]byte(canonicalPath(path)))
	nameSlug := slug(name)
	if nameSlug == "" {
		nameSlug = "chromium"
	}
	id := "custom-" + nameSlug + "-" + hex.EncodeToString(digest[:4])
	return browserCandidateAtOrError(id, name, path)
}

func browserCandidateAt(id string, name string, path string) (BrowserCandidate, bool) {
	candidate, err := browserCandidateAtOrError(id, name, path)
	return candidate, err == nil
}

func browserCandidateAtOrError(id string, name string, path string) (BrowserCandidate, error) {
	path = filepath.Clean(path)
	info, err := os.Stat(path)
	if err != nil {
		return BrowserCandidate{}, err
	}
	if info.IsDir() {
		return BrowserCandidate{}, fmt.Errorf("browser executable %q is a directory", path)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm()&0o111 == 0 {
		return BrowserCandidate{}, fmt.Errorf("browser executable %q is not executable", path)
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		return BrowserCandidate{}, err
	}
	return BrowserCandidate{ID: id, Name: name, ExecutablePath: absolute}, nil
}

func pathSeen(seen map[string]struct{}, path string) bool {
	path = canonicalPath(path)
	if _, exists := seen[path]; exists {
		return true
	}
	seen[path] = struct{}{}
	return false
}

func samePath(left string, right string) bool {
	return canonicalPath(left) == canonicalPath(right)
}

func canonicalPath(path string) string {
	if resolved, err := filepath.EvalSymlinks(path); err == nil {
		path = resolved
	}
	path = filepath.Clean(path)
	if runtime.GOOS == "windows" || runtime.GOOS == "darwin" {
		path = strings.ToLower(path)
	}
	return path
}

func normalizeBrowserID(value string) string {
	value = slug(value)
	aliases := map[string]string{
		"brave-browser": "brave", "google-chrome": "chrome", "google-chrome-stable": "chrome",
		"microsoft-edge": "edge", "microsoft-edge-stable": "edge", "msedge": "edge",
		"vivaldi-stable": "vivaldi", "google-chrome-canary": "chrome",
	}
	if normalized, ok := aliases[value]; ok {
		return normalized
	}
	return value
}

func slug(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	var builder strings.Builder
	lastDash := false
	for _, character := range value {
		if (character >= 'a' && character <= 'z') || (character >= '0' && character <= '9') {
			builder.WriteRune(character)
			lastDash = false
			continue
		}
		if !lastDash && builder.Len() > 0 {
			builder.WriteByte('-')
			lastDash = true
		}
	}
	return strings.Trim(builder.String(), "-")
}

func nonEmptyStrings(values ...string) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			result = append(result, value)
		}
	}
	sort.Strings(result)
	return result
}
