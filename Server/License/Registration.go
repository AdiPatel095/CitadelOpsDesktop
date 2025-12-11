package License

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

const hardwareFile = "hardwareID.txt"
const registrationPollInterval = 1 * time.Second
const creditsSyncInterval = 1 * time.Second
const httpTimeout = 10 * time.Second

// httpClient is a shared HTTP client with timeout for all cloud API requests
var httpClient = &http.Client{
	Timeout: httpTimeout,
}

// cloudBaseURL returns the base URL for cloud API, configurable via environment
func cloudBaseURL() string {
	if url := os.Getenv("CLOUD_BACKEND_URL"); url != "" {
		return url
	}
	return "https://citadelops.app/api"
}

// RegistrationState holds the current license state
type RegistrationState struct {
	HardwareID string
	Registered bool
	Credits    int
	mu         sync.RWMutex
}

// CurrentRegistration is the global registration state
var CurrentRegistration = &RegistrationState{}

// CreditUsage represents a deduction of credits with a reason
type CreditUsage struct {
	Amount int
	Type   string
}

// CreditUsageChannel receives credit deductions from other parts of the app
var CreditUsageChannel = make(chan CreditUsage, 100)

// stopCreditsSync is used to signal the credits sync goroutine to stop
var stopCreditsSync = make(chan struct{})

// SendStatusFunc is a callback for sending registration status to frontend
var SendStatusFunc func(registered bool, hardwareID string, credits int)

// SendCreditsFunc is a callback for sending credits updates to frontend
var SendCreditsFunc func(credits int)

// SetSendStatusCallback sets the callback for sending registration status
func SetSendStatusCallback(fn func(registered bool, hardwareID string, credits int)) {
	SendStatusFunc = fn
}

// SetSendCreditsCallback sets the callback for sending credits updates
func SetSendCreditsCallback(fn func(credits int)) {
	SendCreditsFunc = fn
}

// getMachineFingerprint returns a unique identifier for this machine
// based on the first valid network interface MAC address.
// Interfaces are sorted by name to ensure deterministic result.
func getMachineFingerprint() (string, error) {
	interfaces, err := net.Interfaces()
	if err != nil {
		return "", err
	}

	// Sort interfaces by name to ensure stability across reboots/launches
	sort.Slice(interfaces, func(i, j int) bool {
		return interfaces[i].Name < interfaces[j].Name
	})

	for _, iface := range interfaces {
		// Skip loopback, down, and virtual interfaces
		if iface.Flags&net.FlagLoopback != 0 || iface.Flags&net.FlagUp == 0 {
			continue
		}
		// Skip interfaces with no hardware address
		if len(iface.HardwareAddr) == 0 {
			continue
		}
		// Return the MAC address
		return iface.HardwareAddr.String(), nil
	}

	// Fallback: use hostname hash if no suitable interface found
	hostname, _ := os.Hostname()
	hash := sha256.Sum256([]byte(hostname))
	return hex.EncodeToString(hash[:8]), nil
}

// generateHardwareID creates a new hardware ID combining machine fingerprint and instance UUID
func generateHardwareID() (string, error) {
	fingerprint, err := getMachineFingerprint()
	if err != nil {
		return "", err
	}
	instanceID := uuid.New().String()
	// Use pipe as separator because MAC addresses contain colons
	return fmt.Sprintf("%s|%s", fingerprint, instanceID), nil
}

// parseHardwareID splits a hardware ID into machine fingerprint and instance UUID
func parseHardwareID(id string) (fingerprint, instanceID string, valid bool) {
	// Split on pipe first
	parts := strings.SplitN(id, "|", 2)
	if len(parts) == 2 {
		return parts[0], parts[1], true
	}

	// Fallback/Legacy check: try splitting on last colon if pipe not found?
	// But the user said previous ones were broken anyway.
	// Actually, let's just valid = false if format is wrong.
	return "", "", false
}

func getHWFilePath() string {
	ex, err := os.Executable()
	if err != nil {
		// Fallback to current working directory if executable path fails
		return hardwareFile
	}
	return filepath.Join(filepath.Dir(ex), hardwareFile)
}

// InitRegistration initializes the hardware ID (creates file if needed)
// Logic:
// 1. Read hardwareID.txt
// 2. Validate format and local machine fingerprint (prevent copying ID to another machine)
// 3. If valid locally, check with cloud
// 4. If cloud check passes, keep it
// 5. If any check fails, regenerate and register new
func InitRegistration() error {
	// 1. Get current fingerprint
	currentFingerprint, err := getMachineFingerprint()
	if err != nil {
		log.Printf("Warning: Could not get machine fingerprint: %v", err)
	}

	hwFile := getHWFilePath()

	log.Printf("Hardware License File Path: %s", hwFile)

	data, err := os.ReadFile(hwFile)
	storedID := ""
	if err == nil {
		storedID = strings.TrimSpace(string(data))
	} else if !os.IsNotExist(err) {
		log.Printf("Error reading existing hardware file: %v", err)
	}

	// shouldRegenerate is implied if we fall through

	// Case 1: File exists and has content
	if storedID != "" {
		// Parse and check fingerprint
		storedFingerprint, _, valid := parseHardwareID(storedID)

		// Check validity and fingerprint match
		// We only check fingerprint mismatch if we successfully got a current fingerprint
		localValid := valid
		if localValid && currentFingerprint != "" && storedFingerprint != currentFingerprint {
			log.Printf("REGENERATING: Machine fingerprint mismatch! Stored: '%s', Current: '%s'", storedFingerprint, currentFingerprint)
			localValid = false
		} else if !localValid {
			log.Printf("REGENERATING: Hardware ID format invalid.")
		}

		if localValid {
			log.Printf("Local hardware ID valid: %s. Checking with cloud...", storedID)

			// Set it temporarily to check with cloud
			CurrentRegistration.HardwareID = storedID

			// Check with cloud
			if CheckRegistration() {
				log.Printf("Hardware ID verified with cloud. Keeping existing ID.")
				return nil
			}
			log.Printf("REGENERATING: Hardware ID not registered on cloud.")
		}
	} else {
		log.Printf("REGENERATING: No existing hardware ID found (or file empty).")
	}

	// Case 2: Regenerate (if we reached here, shouldRegenerate is effectively true)
	// Generate new ID
	newHardwareID, genErr := generateHardwareID()
	if genErr != nil {
		log.Printf("Error generating hardware ID: %v", genErr)
		return genErr
	}

	// Write to file
	if err := os.WriteFile(hwFile, []byte(newHardwareID), 0600); err != nil {
		log.Printf("Error writing to '%s': %v", hwFile, err)
		return err
	}

	log.Printf("Successfully created and saved new hardware ID: %s", newHardwareID)
	CurrentRegistration.HardwareID = newHardwareID

	// Register with cloud backend (unverified)
	registerWithCloud(CurrentRegistration.HardwareID)

	// Double check to populate state
	CheckRegistration()

	return nil
}

// registerWithCloud registers a new hardware ID with the cloud (unverified)
func registerWithCloud(hardwareID string) {
	reqUrl := fmt.Sprintf("%s/license/create/%s", cloudBaseURL(), hardwareID)
	resp, err := httpClient.Post(reqUrl, "application/json", nil)
	if err != nil {
		log.Printf("Failed to register with cloud: %v", err)
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		log.Printf("Cloud registration returned status: %d", resp.StatusCode)
	}
}

// CheckRegistration checks if the hardware ID is registered with the cloud
// Returns true if registered, false otherwise
func CheckRegistration() bool {
	CurrentRegistration.mu.Lock()
	defer CurrentRegistration.mu.Unlock()

	reqUrl := fmt.Sprintf("%s/license/check/%s", cloudBaseURL(), CurrentRegistration.HardwareID)
	resp, err := httpClient.Get(reqUrl)
	if err != nil {
		log.Printf("Failed to check registration: %v", err)
		return false
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		// Only log if this is a status change to avoid spam
		if CurrentRegistration.Registered {
			log.Printf("Hardware not registered (status %d). Please register at cloud dashboard.", resp.StatusCode)
		}
		CurrentRegistration.Registered = false
		return false
	}

	var response struct {
		Credits int `json:"credits"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		log.Printf("Error decoding registration response: %v", err)
		return false
	}

	CurrentRegistration.Registered = true
	CurrentRegistration.Credits = response.Credits
	log.Printf("Hardware registered with %d credits", CurrentRegistration.Credits)
	return true
}

// WaitForRegistration polls for registration every 15 seconds
// Sends status updates to connected frontend clients
// Returns true when registered
func WaitForRegistration() bool {
	// Initial check
	if CheckRegistration() {
		SendRegistrationStatus()
		return true
	}

	// Send initial unregistered status
	SendRegistrationStatus()
	log.Printf("Please register hardware ID '%s' on your cloud dashboard", CurrentRegistration.HardwareID)

	// Poll every 15 seconds
	ticker := time.NewTicker(registrationPollInterval)
	defer ticker.Stop()

	for range ticker.C {
		if CheckRegistration() {
			SendRegistrationStatus()
			log.Printf("Registration verified!")
			return true
		}
		// Send updated status to frontend
		SendRegistrationStatus()
	}

	return false
}

// SendRegistrationStatus sends the current registration status to all connected clients
func SendRegistrationStatus() {
	if SendStatusFunc == nil {
		return
	}

	CurrentRegistration.mu.RLock()
	defer CurrentRegistration.mu.RUnlock()

	SendStatusFunc(CurrentRegistration.Registered, CurrentRegistration.HardwareID, CurrentRegistration.Credits)
}

// SendCreditsUpdate sends a credits update to all connected clients
func SendCreditsUpdate() {
	if SendCreditsFunc == nil {
		log.Println("SendCreditsFunc is nil!")
		return
	}

	CurrentRegistration.mu.RLock()
	defer CurrentRegistration.mu.RUnlock()

	SendCreditsFunc(CurrentRegistration.Credits)
}

// StartCreditsSync starts a goroutine that periodically syncs credits with the cloud
// and listens for credit usage from the CreditUsageChannel
func StartCreditsSync() {
	// Poll every 10 seconds for credit updates
	syncTicker := time.NewTicker(10 * time.Second)
	defer syncTicker.Stop()

	// Flush buffer every 1.5 seconds
	flushTicker := time.NewTicker(1500 * time.Millisecond)
	defer flushTicker.Stop()

	// Local buffer for credit usage: Type -> Amount
	usageBuffer := make(map[string]int)

	for {
		select {
		case <-syncTicker.C:
			// Periodic sync with cloud
			go syncCreditsFromCloud()

		case <-flushTicker.C:
			// Flush buffered usage
			if len(usageBuffer) > 0 {
				for usageType, amount := range usageBuffer {
					if amount > 0 {
						// We launch a goroutine for each type to not block the loop, similar to before
						// but now it's one request per type per 1.5s instead of per item.
						// Capture variables for the goroutine
						t := usageType
						a := amount
						// We need the hardware ID. We can get it safely.
						// Note: handling credit usage previously did RLock.
						// We can do it inside the helper or here.
						// Let's do it here to pass it to the function.
						CurrentRegistration.mu.RLock()
						hwID := CurrentRegistration.HardwareID
						CurrentRegistration.mu.RUnlock()

						go updateCloudCredits(hwID, CreditUsage{Amount: a, Type: t})
					}
				}
				// Clear buffer
				usageBuffer = make(map[string]int)
			}

		case usage := <-CreditUsageChannel:
			// Buffer the usage
			usageBuffer[usage.Type] += usage.Amount

		case <-stopCreditsSync:
			log.Println("Credits sync stopped")
			return
		}
	}
}

// syncCreditsFromCloud fetches the latest credits from the cloud
func syncCreditsFromCloud() {
	CurrentRegistration.mu.RLock()
	hardwareID := CurrentRegistration.HardwareID
	CurrentRegistration.mu.RUnlock()

	reqUrl := fmt.Sprintf("%s/license/check/%s", cloudBaseURL(), hardwareID)
	resp, err := httpClient.Get(reqUrl)
	if err != nil {
		log.Printf("Failed to sync credits: %v", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		log.Printf("Credits sync failed (status %d)", resp.StatusCode)
		return
	}

	var response struct {
		Credits int `json:"credits"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		log.Printf("Error decoding credits response: %v", err)
		return
	}

	CurrentRegistration.mu.Lock()

	if CurrentRegistration.Credits != response.Credits {
		CurrentRegistration.Credits = response.Credits
		log.Printf("Credits synced: %d", CurrentRegistration.Credits)

		// Unlock before sending to avoid deadlock and allow other readers
		CurrentRegistration.mu.Unlock()
		SendCreditsUpdate()
		return
	}

	CurrentRegistration.mu.Unlock()
}

// updateCloudCredits notifies the cloud of credit usage
func updateCloudCredits(hardwareID string, usage CreditUsage) {
	reqUrl := fmt.Sprintf("%s/license/use-credits/%s", cloudBaseURL(), hardwareID)

	payload := map[string]interface{}{
		"amount": usage.Amount,
		"type":   usage.Type,
	}
	payloadBytes, _ := json.Marshal(payload)

	// Use bytes.NewBuffer(payloadBytes) to create a reader for the body
	// But we need to import "bytes"
	// For now, let's use strings.NewReader if we don't want to add imports, or just add "bytes"
	// Adding "bytes" to imports is better.

	// Wait, I can't easily add imports here without finding the import block.
	// I'll try to do it without "bytes" if possible, or assume I can add it.
	// Actually, I am replacing the whole file content basically (or a large chunk).
	// Let's check imports.
	// Imports are at lines 3-19. I didn't include them in the chunk.
	// I should probably use `strings` since it is already imported.
	// `strings.NewReader(string(payloadBytes))` works.

	resp, err := httpClient.Post(reqUrl, "application/json", strings.NewReader(string(payloadBytes)))
	if err != nil {
		log.Printf("Failed to update cloud credits: %v", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
	}
}

// UseCredits is a helper function to deduct credits
// UseCredits is a helper function to deduct credits
func UseCredits(amount int, usageType string) bool {
	CurrentRegistration.mu.Lock()
	hasEnough := CurrentRegistration.Credits >= amount
	if hasEnough {
		CurrentRegistration.Credits -= amount
	}
	CurrentRegistration.mu.Unlock()

	if !hasEnough {
		return false
	}

	// Notify frontend immediately of the change
	SendCreditsUpdate()

	CreditUsageChannel <- CreditUsage{Amount: amount, Type: usageType}
	return true
}

// GetCredits returns the current credit balance
func GetCredits() int {
	CurrentRegistration.mu.RLock()
	defer CurrentRegistration.mu.RUnlock()
	return CurrentRegistration.Credits
}

// GetHardwareID returns the current hardware ID
func GetHardwareID() string {
	CurrentRegistration.mu.RLock()
	defer CurrentRegistration.mu.RUnlock()
	return CurrentRegistration.HardwareID
}

// IsRegistered returns whether the hardware is registered
func IsRegistered() bool {
	CurrentRegistration.mu.RLock()
	defer CurrentRegistration.mu.RUnlock()
	return CurrentRegistration.Registered
}
