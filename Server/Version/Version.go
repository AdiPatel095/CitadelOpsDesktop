package Version

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
)

// CurrentVersion is the hardcoded current desktop app version.
// Update this value when releasing a new version.
const CurrentVersion = "1.3.7"

// Download URL configuration
const (
	// Base URL for the GCS bucket where binaries are stored
	DownloadBaseURL = "https://storage.googleapis.com/us-service-ggebot-desktop-bucket"
	// Image name used in the build
	ImageName = "citadel-ops-desktop"
)

// VersionInfo represents the response from the version check endpoint
type VersionInfo struct {
	Version string `json:"version"`
}

// BuildDownloadURL constructs the appropriate download URL for the current platform
// based on runtime.GOOS and runtime.GOARCH
func BuildDownloadURL(version string) string {
	switch runtime.GOOS {
	case "darwin":
		// macOS - check architecture
		if runtime.GOARCH == "arm64" {
			// Apple Silicon (M1/M2/M3/M4)
			log.Printf("[Version] Platform detected: macOS ARM64 (Apple Silicon)")
			return fmt.Sprintf("%s/macos-arm64-%s-%s", DownloadBaseURL, ImageName, version)
		} else {
			// Intel Mac (amd64)
			log.Printf("[Version] Platform detected: macOS AMD64 (Intel)")
			return fmt.Sprintf("%s/macos-amd64-%s-%s", DownloadBaseURL, ImageName, version)
		}
	case "windows":
		log.Printf("[Version] Platform detected: Windows x64")
		return fmt.Sprintf("%s/windows-%s-%s.exe", DownloadBaseURL, ImageName, version)
	default:
		// Fallback to Windows
		log.Printf("[Version] Platform detected: unknown (%s/%s), defaulting to Windows", runtime.GOOS, runtime.GOARCH)
		return fmt.Sprintf("%s/windows-%s-%s.exe", DownloadBaseURL, ImageName, version)
	}
}

// Callback function types
type VersionUpdateCallback func(newVersion string, downloadUrl string)
type UpdateProgressCallback func(stage string, percent int)
type UpdateCompleteCallback func()
type UpdateErrorCallback func(errMsg string)

var sendVersionUpdate VersionUpdateCallback
var sendUpdateProgress UpdateProgressCallback
var sendUpdateComplete UpdateCompleteCallback
var sendUpdateError UpdateErrorCallback

// SetVersionUpdateCallback sets the callback function for version updates
func SetVersionUpdateCallback(callback VersionUpdateCallback) {
	sendVersionUpdate = callback
}

// SetUpdateProgressCallback sets the callback for update progress
func SetUpdateProgressCallback(callback UpdateProgressCallback) {
	sendUpdateProgress = callback
}

// SetUpdateCompleteCallback sets the callback for update completion
func SetUpdateCompleteCallback(callback UpdateCompleteCallback) {
	sendUpdateComplete = callback
}

// SetUpdateErrorCallback sets the callback for update errors
func SetUpdateErrorCallback(callback UpdateErrorCallback) {
	sendUpdateError = callback
}

// compareVersions compares two semantic versions.
// Returns 1 if v1 > v2, -1 if v1 < v2, 0 if equal
func compareVersions(v1, v2 string) int {
	parts1 := strings.Split(v1, ".")
	parts2 := strings.Split(v2, ".")

	// Normalize to 3 parts
	for len(parts1) < 3 {
		parts1 = append(parts1, "0")
	}
	for len(parts2) < 3 {
		parts2 = append(parts2, "0")
	}

	for i := 0; i < 3; i++ {
		n1, _ := strconv.Atoi(parts1[i])
		n2, _ := strconv.Atoi(parts2[i])
		if n1 > n2 {
			return 1
		}
		if n1 < n2 {
			return -1
		}
	}
	return 0
}

// checkForUpdate checks the cloud backend for a newer version
func checkForUpdate() {
	log.Printf("[Version] Checking for updates (current: %s)...", CurrentVersion)

	client := &http.Client{
		Timeout: 30 * time.Second,
	}

	CloudBackendURL := os.Getenv("CLOUD_BACKEND_URL")
	if CloudBackendURL == "" {
		CloudBackendURL = "https://citadelops.app/api"
	}

	resp, err := client.Get(CloudBackendURL + "/version/latest")
	if err != nil {
		log.Printf("[Version] Failed to check for updates: %v", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		log.Printf("[Version] Version check returned status %d", resp.StatusCode)
		return
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Printf("[Version] Failed to read response: %v", err)
		return
	}

	var versionInfo VersionInfo
	if err := json.Unmarshal(body, &versionInfo); err != nil {
		log.Printf("[Version] Failed to parse version info: %v", err)
		return
	}

	log.Printf("[Version] Latest version: %s, current: %s", versionInfo.Version, CurrentVersion)

	// Compare versions
	if compareVersions(versionInfo.Version, CurrentVersion) > 0 {
		log.Printf("[Version] New version available: %s", versionInfo.Version)
		if sendVersionUpdate != nil {
			// Build the download URL dynamically based on version and current platform
			downloadURL := BuildDownloadURL(versionInfo.Version)
			sendVersionUpdate(versionInfo.Version, downloadURL)
		}
	} else {
		log.Printf("[Version] Already on latest version")
	}
}

// StartVersionCheck starts a goroutine that checks for updates every 24 hours
func StartVersionCheck() {
	// Check immediately on startup (after a short delay to let things initialize)
	go func() {
		time.Sleep(10 * time.Second)
		checkForUpdate()
	}()

	// Then check every 1 hour
	ticker := time.NewTicker(1 * time.Hour)
	go func() {
		for range ticker.C {
			checkForUpdate()
		}
	}()

	log.Printf("[Version] Version check service started (checking every 1 hour)")
}

// CleanupOldBinary removes the old binary (_old.exe) if it exists
// Should be called on app startup
func CleanupOldBinary() {
	exePath, err := os.Executable()
	if err != nil {
		log.Printf("[Version] Failed to get executable path: %v", err)
		return
	}

	exeDir := filepath.Dir(exePath)
	exeName := filepath.Base(exePath)
	ext := filepath.Ext(exeName)
	baseName := strings.TrimSuffix(exeName, ext)

	oldPath := filepath.Join(exeDir, baseName+"_old"+ext)

	if _, err := os.Stat(oldPath); err == nil {
		// Old binary exists, try to remove it
		time.Sleep(1 * time.Second) // Give the old process time to fully exit

		if err := os.Remove(oldPath); err != nil {
			log.Printf("[Version] Failed to remove old binary: %v", err)
		} else {
			log.Printf("[Version] Cleaned up old binary: %s", oldPath)
		}
	}
}

// PerformSelfUpdate downloads the new binary and applies the update
// After update, the user must manually restart the application
func PerformSelfUpdate(downloadUrl string) error {
	log.Printf("[Version] Starting self-update from: %s", downloadUrl)

	// Get current executable path
	exePath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("failed to get executable path: %w", err)
	}

	exeDir := filepath.Dir(exePath)
	exeName := filepath.Base(exePath)
	ext := filepath.Ext(exeName)
	baseName := strings.TrimSuffix(exeName, ext)

	oldPath := filepath.Join(exeDir, baseName+"_old"+ext)

	// Step 1: Rename current exe to _old (backup)
	if sendUpdateProgress != nil {
		sendUpdateProgress("Backing up current version...", 0)
	}

	// Remove old backup if it exists
	os.Remove(oldPath)

	// Rename current -> old
	if err := os.Rename(exePath, oldPath); err != nil {
		return fmt.Errorf("failed to backup current binary: %w", err)
	}

	log.Printf("[Version] Backed up current binary to: %s", oldPath)

	// Step 2: Download new binary directly to the original path
	if sendUpdateProgress != nil {
		sendUpdateProgress("Downloading update...", 5)
	}

	client := &http.Client{
		Timeout: 5 * time.Minute, // Allow up to 5 minutes for download
	}

	resp, err := client.Get(downloadUrl)
	if err != nil {
		// Restore the original on failure
		os.Rename(oldPath, exePath)
		return fmt.Errorf("failed to download update: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		// Restore the original on failure
		os.Rename(oldPath, exePath)
		return fmt.Errorf("download failed with status: %d", resp.StatusCode)
	}

	// Create the new file at the original path
	newFile, err := os.Create(exePath)
	if err != nil {
		// Restore the original on failure
		os.Rename(oldPath, exePath)
		return fmt.Errorf("failed to create new binary file: %w", err)
	}

	// Download with progress tracking
	totalSize := resp.ContentLength
	downloaded := int64(0)
	buffer := make([]byte, 32*1024) // 32KB buffer

	for {
		n, err := resp.Body.Read(buffer)
		if n > 0 {
			_, writeErr := newFile.Write(buffer[:n])
			if writeErr != nil {
				newFile.Close()
				os.Remove(exePath)
				os.Rename(oldPath, exePath) // Restore on failure
				return fmt.Errorf("failed to write to new binary: %w", writeErr)
			}
			downloaded += int64(n)

			if totalSize > 0 && sendUpdateProgress != nil {
				// Progress from 5% to 95%
				percent := 5 + int(float64(downloaded)/float64(totalSize)*90)
				sendUpdateProgress("Downloading update...", percent)
			}
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			newFile.Close()
			os.Remove(exePath)
			os.Rename(oldPath, exePath) // Restore on failure
			return fmt.Errorf("error during download: %w", err)
		}
	}
	newFile.Close()

	// Make the new binary executable (for macOS/Linux)
	if runtime.GOOS != "windows" {
		if err := os.Chmod(exePath, 0755); err != nil {
			log.Printf("[Version] Warning: failed to set executable permissions: %v", err)
		}
	}

	log.Printf("[Version] Download complete: %d bytes", downloaded)
	log.Printf("[Version] Update applied successfully - user must restart the application")

	// Step 3: Notify frontend that update is complete and restart is required
	if sendUpdateProgress != nil {
		sendUpdateProgress("Update complete! Please restart.", 100)
	}

	if sendUpdateComplete != nil {
		sendUpdateComplete()
	}

	return nil
}

// RestartApp spawns a new instance of the application and exits the current one
func RestartApp() {
	exePath, err := os.Executable()
	if err != nil {
		log.Printf("[Version] Failed to get executable path for restart: %v", err)
		return
	}

	log.Printf("[Version] Restarting application: %s", exePath)

	// Spawn new process
	cmd := exec.Command(exePath)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Dir = filepath.Dir(exePath)

	if err := cmd.Start(); err != nil {
		log.Printf("[Version] Failed to start new process: %v", err)
		return
	}

	log.Printf("[Version] New process started (PID: %d), exiting current process", cmd.Process.Pid)

	// Exit current process
	os.Exit(0)
}
