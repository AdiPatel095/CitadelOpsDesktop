package License

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
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

// CreditUsageChannel receives credit deductions from other parts of the app
var CreditUsageChannel = make(chan int, 100)

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

// InitRegistration initializes the hardware ID (creates file if needed)
func InitRegistration() error {
	data, err := os.ReadFile(hardwareFile)
	if err != nil {
		if os.IsNotExist(err) {
			log.Printf("'%s' not found. Creating new hardware ID.", hardwareFile)
			newHardwareID := uuid.New()
			if err := os.WriteFile(hardwareFile, []byte(newHardwareID.String()), 0600); err != nil {
				log.Printf("Error writing to '%s': %v", hardwareFile, err)
				return err
			}
			log.Printf("Successfully created hardware ID: %s", newHardwareID.String())
			CurrentRegistration.HardwareID = newHardwareID.String()

			// Register with cloud backend (unverified)
			registerWithCloud(CurrentRegistration.HardwareID)
			return nil
		}
		log.Printf("Error reading '%s': %v", hardwareFile, err)
		return err
	}

	CurrentRegistration.HardwareID = string(data)
	if CurrentRegistration.HardwareID == "" {
		return fmt.Errorf("hardware ID file is empty")
	}
	log.Printf("Loaded hardware ID: %s", CurrentRegistration.HardwareID)
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
		return
	}

	CurrentRegistration.mu.RLock()
	defer CurrentRegistration.mu.RUnlock()

	SendCreditsFunc(CurrentRegistration.Credits)
}

// StartCreditsSync starts a goroutine that periodically syncs credits with the cloud
// and listens for credit usage from the CreditUsageChannel
func StartCreditsSync() {
	ticker := time.NewTicker(creditsSyncInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			// Periodic sync with cloud
			syncCreditsFromCloud()
		case usage := <-CreditUsageChannel:
			// Handle credit usage
			handleCreditUsage(usage)
		case <-stopCreditsSync:
			log.Println("Credits sync stopped")
			return
		}
	}
}

// syncCreditsFromCloud fetches the latest credits from the cloud
func syncCreditsFromCloud() {
	CurrentRegistration.mu.Lock()
	defer CurrentRegistration.mu.Unlock()

	reqUrl := fmt.Sprintf("%s/license/check/%s", cloudBaseURL(), CurrentRegistration.HardwareID)
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

	if CurrentRegistration.Credits != response.Credits {
		CurrentRegistration.Credits = response.Credits
		log.Printf("Credits synced: %d", CurrentRegistration.Credits)
		// Unlock before sending to avoid deadlock
		CurrentRegistration.mu.Unlock()
		SendCreditsUpdate()
		CurrentRegistration.mu.Lock()
	}
}

// handleCreditUsage processes a credit usage and updates the cloud
func handleCreditUsage(amount int) {
	CurrentRegistration.mu.Lock()

	// Deduct locally first
	CurrentRegistration.Credits -= amount
	newCredits := CurrentRegistration.Credits
	hardwareID := CurrentRegistration.HardwareID
	CurrentRegistration.mu.Unlock()

	log.Printf("Credit usage: -%d, new balance: %d", amount, newCredits)

	// Send update to frontend
	SendCreditsUpdate()

	// Update cloud backend
	go updateCloudCredits(hardwareID, amount)
}

// updateCloudCredits notifies the cloud of credit usage
func updateCloudCredits(hardwareID string, usedAmount int) {
	reqUrl := fmt.Sprintf("%s/license/use-credits/%s", cloudBaseURL(), hardwareID)

	payload := map[string]int{"amount": usedAmount}
	payloadBytes, _ := json.Marshal(payload)

	resp, err := httpClient.Post(reqUrl, "application/json", nil)
	if err != nil {
		log.Printf("Failed to update cloud credits: %v", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		log.Printf("Cloud credits update failed (status %d)", resp.StatusCode)
	}

	// Log the payload for debugging even though we didn't use it in the body
	_ = payloadBytes
}

// UseCredits is a helper function to deduct credits
func UseCredits(amount int) bool {
	CurrentRegistration.mu.RLock()
	hasEnough := CurrentRegistration.Credits >= amount
	CurrentRegistration.mu.RUnlock()

	if !hasEnough {
		return false
	}

	CreditUsageChannel <- amount
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
