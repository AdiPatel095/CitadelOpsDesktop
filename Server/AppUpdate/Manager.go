package AppUpdate

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	DefaultEndpoint      = "https://citadelops.app/api/version/latest"
	DefaultCheckInterval = time.Hour
	DefaultMaxBytes      = int64(512 << 20)
)

// DefaultDownloadBase is intentionally empty in public source. Official
// distribution builds inject their trusted HTTPS artifact root at link time.
// A source build therefore fails closed instead of trusting an unpublished
// provider-specific location.
var DefaultDownloadBase string

type Config struct {
	CurrentVersion   string
	Endpoint         string
	DownloadBaseURL  string
	Client           *http.Client
	CheckInterval    time.Duration
	MaxBytes         int64
	InstallSupported bool
}

type Snapshot struct {
	CurrentVersion   string     `json:"currentVersion"`
	LatestVersion    string     `json:"latestVersion,omitempty"`
	Available        bool       `json:"available"`
	DownloadURL      string     `json:"downloadUrl,omitempty"`
	ExpectedSHA256   string     `json:"expectedSha256,omitempty"`
	InstallSupported bool       `json:"installSupported"`
	Status           string     `json:"status"`
	Stage            string     `json:"stage,omitempty"`
	Progress         int        `json:"progress"`
	Error            string     `json:"error,omitempty"`
	CheckedAt        *time.Time `json:"checkedAt,omitempty"`
	RestartRequired  bool       `json:"restartRequired"`
}

type Manager struct {
	config Config

	mu        sync.RWMutex
	snapshot  Snapshot
	operation sync.Mutex
}

type versionResponse struct {
	Version          string `json:"version"`
	DownloadURL      string `json:"downloadUrl"`
	WindowsURL       string `json:"windowsDownloadUrl"`
	MacOSARM64URL    string `json:"macosArm64DownloadUrl"`
	MacOSAMD64URL    string `json:"macosAmd64DownloadUrl"`
	LinuxAMD64URL    string `json:"linuxAmd64DownloadUrl"`
	ExpectedSHA256   string `json:"sha256"`
	WindowsSHA256    string `json:"windowsSha256"`
	MacOSARM64SHA256 string `json:"macosArm64Sha256"`
	MacOSAMD64SHA256 string `json:"macosAmd64Sha256"`
	LinuxAMD64SHA256 string `json:"linuxAmd64Sha256"`
}

func NewManager(config Config) *Manager {
	config.CurrentVersion = normalizeVersion(config.CurrentVersion)
	if config.Endpoint == "" {
		config.Endpoint = DefaultEndpoint
	}
	if config.DownloadBaseURL == "" {
		config.DownloadBaseURL = DefaultDownloadBase
	}
	if config.Client == nil {
		config.Client = &http.Client{Timeout: 10 * time.Minute}
	}
	if config.CheckInterval <= 0 {
		config.CheckInterval = DefaultCheckInterval
	}
	if config.MaxBytes <= 0 {
		config.MaxBytes = DefaultMaxBytes
	}
	return &Manager{
		config: config,
		snapshot: Snapshot{
			CurrentVersion: config.CurrentVersion, InstallSupported: config.InstallSupported,
			Status: "idle",
		},
	}
}

func (manager *Manager) Snapshot() Snapshot {
	if manager == nil {
		return Snapshot{Status: "unavailable"}
	}
	manager.mu.RLock()
	defer manager.mu.RUnlock()
	return manager.snapshot
}

func (manager *Manager) Run(ctx context.Context) {
	if manager == nil {
		return
	}
	_ = manager.Check(ctx)
	ticker := time.NewTicker(manager.config.CheckInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			_ = manager.Check(ctx)
		}
	}
}

func (manager *Manager) Check(ctx context.Context) error {
	if manager == nil {
		return fmt.Errorf("application update service is unavailable")
	}
	manager.operation.Lock()
	defer manager.operation.Unlock()
	if manager.Snapshot().RestartRequired {
		return nil
	}
	manager.update(func(snapshot *Snapshot) {
		snapshot.Status = "checking"
		snapshot.Stage = "Checking for updates..."
		snapshot.Error = ""
		snapshot.Progress = 0
	})
	checkContext, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	request, err := http.NewRequestWithContext(checkContext, http.MethodGet, manager.config.Endpoint, nil)
	if err != nil {
		return manager.fail("check", err)
	}
	response, err := manager.config.Client.Do(request)
	if err != nil {
		return manager.fail("check", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return manager.fail("check", fmt.Errorf("version endpoint returned HTTP %d", response.StatusCode))
	}
	raw, err := io.ReadAll(io.LimitReader(response.Body, (1<<20)+1))
	if err != nil {
		return manager.fail("check", err)
	}
	if len(raw) > 1<<20 {
		return manager.fail("check", fmt.Errorf("version response exceeds 1 MiB"))
	}
	var latest versionResponse
	if err := json.Unmarshal(raw, &latest); err != nil {
		return manager.fail("check", fmt.Errorf("decode version response: %w", err))
	}
	latest.Version = normalizeVersion(latest.Version)
	if !isSemanticVersion(latest.Version) {
		return manager.fail("check", fmt.Errorf("version response has no semantic version"))
	}
	available := compareVersions(latest.Version, manager.config.CurrentVersion) > 0
	downloadURL, expectedSHA := manager.platformArtifact(latest)
	if available {
		hasURL := strings.TrimSpace(downloadURL) != ""
		hasChecksum := strings.TrimSpace(expectedSHA) != ""
		if hasURL != hasChecksum {
			return manager.fail("check", fmt.Errorf("version response must provide the artifact URL and SHA-256 together"))
		}
		if !hasURL {
			return manager.fail("check", fmt.Errorf("version response has no artifact URL and SHA-256 for this platform"))
		}
		if err := manager.validateDownloadURL(downloadURL); err != nil {
			return manager.fail("check", err)
		}
		expectedSHA, err = normalizeSHA256(expectedSHA)
		if err != nil {
			return manager.fail("check", err)
		}
	}
	now := time.Now().UTC()
	manager.update(func(snapshot *Snapshot) {
		snapshot.LatestVersion = latest.Version
		snapshot.Available = available
		snapshot.DownloadURL = ""
		snapshot.ExpectedSHA256 = ""
		if available {
			snapshot.DownloadURL = downloadURL
			snapshot.ExpectedSHA256 = expectedSHA
		}
		snapshot.CheckedAt = &now
		snapshot.Error = ""
		snapshot.Progress = 0
		if available {
			snapshot.Status = "available"
			snapshot.Stage = "Version " + latest.Version + " is available"
		} else {
			snapshot.Status = "current"
			snapshot.Stage = "CitadelOps is up to date"
		}
	})
	return nil
}

func (manager *Manager) Install(ctx context.Context) error {
	if manager == nil {
		return fmt.Errorf("application update service is unavailable")
	}
	manager.operation.Lock()
	defer manager.operation.Unlock()
	snapshot := manager.Snapshot()
	if !snapshot.InstallSupported {
		return manager.fail("install", fmt.Errorf("this server build cannot replace its executable; use the desktop artifact"))
	}
	if !snapshot.Available || snapshot.DownloadURL == "" {
		return manager.fail("install", fmt.Errorf("no newer application update has been checked"))
	}
	if err := manager.validateDownloadURL(snapshot.DownloadURL); err != nil {
		return manager.fail("install", err)
	}
	expectedSHA, err := normalizeSHA256(snapshot.ExpectedSHA256)
	if err != nil {
		return manager.fail("install", err)
	}
	manager.update(func(current *Snapshot) {
		current.Status = "downloading"
		current.Stage = "Downloading update..."
		current.Progress = 1
		current.Error = ""
	})
	err = manager.downloadAndInstall(ctx, snapshot.DownloadURL, expectedSHA)
	if err != nil {
		return manager.fail("install", err)
	}
	manager.update(func(current *Snapshot) {
		current.Status = "restart-required"
		current.Stage = "Update installed. Restart CitadelOps to finish."
		current.Progress = 100
		current.Error = ""
		current.RestartRequired = true
	})
	return nil
}

func (manager *Manager) update(change func(*Snapshot)) {
	manager.mu.Lock()
	change(&manager.snapshot)
	manager.mu.Unlock()
}

func (manager *Manager) fail(stage string, err error) error {
	manager.update(func(snapshot *Snapshot) {
		snapshot.Status = "error"
		snapshot.Stage = "Application update " + stage + " failed"
		snapshot.Error = err.Error()
	})
	return err
}

func (manager *Manager) platformArtifact(response versionResponse) (string, string) {
	artifactURL := strings.TrimSpace(response.DownloadURL)
	checksum := response.ExpectedSHA256
	switch runtime.GOOS + "/" + runtime.GOARCH {
	case "windows/amd64":
		artifactURL, checksum = firstNonempty(response.WindowsURL, artifactURL), firstNonempty(response.WindowsSHA256, checksum)
	case "darwin/arm64":
		artifactURL, checksum = firstNonempty(response.MacOSARM64URL, artifactURL), firstNonempty(response.MacOSARM64SHA256, checksum)
	case "darwin/amd64":
		artifactURL, checksum = firstNonempty(response.MacOSAMD64URL, artifactURL), firstNonempty(response.MacOSAMD64SHA256, checksum)
	case "linux/amd64":
		artifactURL, checksum = firstNonempty(response.LinuxAMD64URL, artifactURL), firstNonempty(response.LinuxAMD64SHA256, checksum)
	}
	return artifactURL, checksum
}

func (manager *Manager) artifactHTTPClient() *http.Client {
	client := *manager.config.Client
	existingCheckRedirect := client.CheckRedirect
	client.CheckRedirect = func(request *http.Request, via []*http.Request) error {
		if len(via) >= 10 {
			return fmt.Errorf("stopped after 10 redirects")
		}
		if err := manager.validateDownloadURL(request.URL.String()); err != nil {
			return fmt.Errorf("application update redirect is invalid: %w", err)
		}
		if existingCheckRedirect != nil {
			return existingCheckRedirect(request, via)
		}
		return nil
	}
	return &client
}

func (manager *Manager) validateDownloadURL(raw string) error {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed.Scheme != "https" || parsed.Hostname() == "" || parsed.User != nil ||
		parsed.RawQuery != "" || parsed.Fragment != "" || parsed.Opaque != "" {
		return fmt.Errorf("application update URL must be an absolute HTTPS URL")
	}
	base, err := url.Parse(strings.TrimSpace(manager.config.DownloadBaseURL))
	if err != nil || base.Scheme != "https" || base.Hostname() == "" || base.User != nil ||
		base.RawQuery != "" || base.Fragment != "" || base.Opaque != "" {
		return fmt.Errorf("application update base URL is invalid")
	}
	if !strings.EqualFold(parsed.Host, base.Host) {
		return fmt.Errorf("application update origin is not trusted")
	}
	basePath := strings.TrimRight(base.EscapedPath(), "/")
	candidatePath := parsed.EscapedPath()
	if basePath == "" || basePath == "/" || !strings.HasPrefix(candidatePath, basePath+"/") {
		return fmt.Errorf("application update path is not trusted")
	}
	if strings.Contains(candidatePath, "%") || strings.Contains(parsed.Path, "\\") || path.Clean(parsed.Path) != parsed.Path {
		return fmt.Errorf("application update path is invalid")
	}
	return nil
}

func normalizeSHA256(value string) (string, error) {
	value = strings.ToLower(strings.TrimSpace(value))
	if !isLowerHex(value, 64) {
		return "", fmt.Errorf("application update SHA-256 is missing or invalid")
	}
	return value, nil
}

func isLowerHex(value string, length int) bool {
	if len(value) != length {
		return false
	}
	for _, character := range value {
		if (character < '0' || character > '9') && (character < 'a' || character > 'f') {
			return false
		}
	}
	return true
}

func compareVersions(left string, right string) int {
	leftParts, leftPrerelease := versionParts(left)
	rightParts, rightPrerelease := versionParts(right)
	for index := 0; index < 3; index++ {
		if leftParts[index] > rightParts[index] {
			return 1
		}
		if leftParts[index] < rightParts[index] {
			return -1
		}
	}
	if leftPrerelease == rightPrerelease {
		return 0
	}
	if leftPrerelease == "" {
		return 1
	}
	if rightPrerelease == "" {
		return -1
	}
	return strings.Compare(leftPrerelease, rightPrerelease)
}

func versionParts(version string) ([3]int, string) {
	var result [3]int
	version = normalizeVersion(version)
	main, prerelease, _ := strings.Cut(version, "-")
	for index, raw := range strings.Split(main, ".") {
		if index >= len(result) {
			break
		}
		result[index], _ = strconv.Atoi(raw)
	}
	return result, prerelease
}

func normalizeVersion(version string) string {
	version = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(version), "v"))
	version, _, _ = strings.Cut(version, "+")
	return version
}

func isSemanticVersion(version string) bool {
	version = normalizeVersion(version)
	main, prerelease, _ := strings.Cut(version, "-")
	parts := strings.Split(main, ".")
	if len(parts) < 1 || len(parts) > 3 {
		return false
	}
	for _, part := range parts {
		if part == "" {
			return false
		}
		for _, character := range part {
			if character < '0' || character > '9' {
				return false
			}
		}
	}
	if prerelease != "" {
		for _, character := range prerelease {
			if character != '.' && character != '-' &&
				(character < '0' || character > '9') &&
				(character < 'A' || character > 'Z') &&
				(character < 'a' || character > 'z') {
				return false
			}
		}
	}
	return true
}

func firstNonempty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
