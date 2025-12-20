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
	"strconv"
	"strings"
	"time"
)

// CurrentVersion is the hardcoded current desktop app version.
// Update this value when releasing a new version.
const CurrentVersion = "1.0.0"

// VersionInfo represents the response from the version check endpoint
type VersionInfo struct {
	Version     string `json:"version"`
	DownloadURL string `json:"downloadUrl"`
	ReleasedAt  string `json:"releasedAt"`
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
			sendVersionUpdate(versionInfo.Version, versionInfo.DownloadURL)
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

	// Then check every 24 hours
	ticker := time.NewTicker(24 * time.Hour)
	go func() {
		for range ticker.C {
			checkForUpdate()
		}
	}()

	log.Printf("[Version] Version check service started (checking every 24 hours)")
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

	newPath := filepath.Join(exeDir, baseName+"_new"+ext)
	oldPath := filepath.Join(exeDir, baseName+"_old"+ext)

	// Step 1: Download new binary
	if sendUpdateProgress != nil {
		sendUpdateProgress("Downloading update...", 0)
	}

	client := &http.Client{
		Timeout: 5 * time.Minute, // Allow up to 5 minutes for download
	}

	resp, err := client.Get(downloadUrl)
	if err != nil {
		return fmt.Errorf("failed to download update: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download failed with status: %d", resp.StatusCode)
	}

	// Create the new file
	newFile, err := os.Create(newPath)
	if err != nil {
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
				os.Remove(newPath)
				return fmt.Errorf("failed to write to new binary: %w", writeErr)
			}
			downloaded += int64(n)

			if totalSize > 0 && sendUpdateProgress != nil {
				percent := int(float64(downloaded) / float64(totalSize) * 100)
				sendUpdateProgress("Downloading update...", percent)
			}
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			newFile.Close()
			os.Remove(newPath)
			return fmt.Errorf("error during download: %w", err)
		}
	}
	newFile.Close()

	log.Printf("[Version] Download complete: %d bytes", downloaded)

	// Step 2: Rename current exe to _old
	if sendUpdateProgress != nil {
		sendUpdateProgress("Applying update...", 100)
	}

	// Remove old backup if it exists
	os.Remove(oldPath)

	// Rename current -> old
	if err := os.Rename(exePath, oldPath); err != nil {
		os.Remove(newPath)
		return fmt.Errorf("failed to backup current binary: %w", err)
	}

	// Step 3: Rename new -> current
	if err := os.Rename(newPath, exePath); err != nil {
		// Try to restore the original
		os.Rename(oldPath, exePath)
		return fmt.Errorf("failed to install new binary: %w", err)
	}

	log.Printf("[Version] Update applied successfully")

	// Step 4: Notify frontend and restart
	if sendUpdateComplete != nil {
		sendUpdateComplete()
	}

	// Give the frontend a moment to receive the message
	time.Sleep(500 * time.Millisecond)

	// Step 5: Restart the application
	go RestartApp()

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
