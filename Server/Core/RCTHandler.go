package Core

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/chromedp/cdproto/input"
	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/chromedp"
)

const loginFilename = "loginBytes.json"
const loginExpirationHours = 24 // Login credentials expire after 24 hours

// LoginData holds the byte slices for login information.
type LoginData struct {
	Login     []byte    `json:"login"`
	Name      []byte    `json:"name"`
	Vck       []byte    `json:"vck"`
	CreatedAt time.Time `json:"createdAt"` // Timestamp for expiration tracking
}

// isLoginExpired checks if the login credentials have expired based on the creation time.
func isLoginExpired(createdAt time.Time) bool {
	if createdAt.IsZero() {
		return true
	}
	expirationTime := createdAt.Add(time.Duration(loginExpirationHours) * time.Hour)
	return time.Now().After(expirationTime)
}

// getLoginFilePath returns the absolute path to the loginBytes file
// using the current working directory for consistency.
func getLoginFilePath() string {
	wd, err := os.Getwd()
	if err != nil {
		log.Printf("[DEBUG] Failed to get working directory: %v. Using relative path.", err)
		return loginFilename
	}
	path := filepath.Join(wd, loginFilename)
	return path
}

// LoadCachedLoginData retrieves and validates stored login data.
// Returns nil if the file is missing, invalid, or expired.
func LoadCachedLoginData() *LoginData {
	loginPath := getLoginFilePath()

	data, err := os.ReadFile(loginPath)
	if err != nil {
		if !os.IsNotExist(err) {
			log.Printf("[DEBUG] Error reading '%s': %v", loginPath, err)
		} else {
		}
		return nil
	}

	var storedLoginData LoginData
	if err := json.Unmarshal(data, &storedLoginData); err != nil {
		log.Printf("[DEBUG] Error unmarshalling login data from '%s': %v", loginPath, err)
		return nil
	}

	if isLoginExpired(storedLoginData.CreatedAt) {
		log.Printf("[DEBUG] Login credentials have expired (older than %d hours).", loginExpirationHours)
		// Delete the expired file
		if err := os.Remove(loginPath); err != nil {
			log.Printf("[DEBUG] Warning: Could not delete expired login file: %v", err)
		}
		return nil
	}

	return &storedLoginData
}

// GetCachedLoginBytes returns the login bytes from the cache if available and valid.
// Returns nil if no valid cache exists.
func GetCachedLoginBytes() [][]byte {
	cached := LoadCachedLoginData()
	if cached != nil {
		hoursRemaining := loginExpirationHours - int(time.Since(cached.CreatedAt).Hours())
		log.Printf("[DEBUG] Login credentials valid. Expires in approximately %d hours.", hoursRemaining)
		return [][]byte{cached.Login, cached.Name, cached.Vck}
	}
	return nil
}

// GetLoginBytesWithCredentials is the main entry point.
// It tries to load from cache first. If that fails or mismatches, it triggers AutomatedLogin.
func GetLoginBytesWithCredentials(username, password, serverName, serverID string) [][]byte {
	// 1. Try to use cached credentials first
	cached := LoadCachedLoginData()
	if cached != nil {
		// Verify if the cached credentials belong to the same user
		_, cachedName := processLoginBytes(cached.Login)
		if cachedName != "" && cachedName == username {
			return [][]byte{cached.Login, cached.Name, cached.Vck}
		} else {
			log.Printf("[DEBUG] Cached credentials (user: '%s') do not match requested user '%s'. Proceeding with new login.", cachedName, username)
		}
	} else {
	}

	return AutomatedLogin(username, password, serverName, serverID)
}

// AutomatedLogin performs the browser automation to log in and retrieve the RCT/Login bytes.
func AutomatedLogin(username, password, serverName, serverID string) [][]byte {
	// Determine the game server URL based on the server parameter
	if serverID == "" {
		serverID = "ep-live-us1-game" // Default to US1
	}
	expectedWSURL := serverID + ".goodgamestudios.com"

	// URL to navigate to for login
	const loginURL = "https://empire.goodgamestudios.com/"

	// Set up chromedp options for a visible browser
	opts := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.Flag("headless", false),
		chromedp.Flag("remote-debugging-port", "9222"),
		chromedp.Flag("start-maximized", false),  // Don't start maximized if we are setting viewport
		chromedp.Flag("window-size", "1280,633"), // Set window size to match viewport roughly
	)

	allocCtx, cancel := chromedp.NewExecAllocator(context.Background(), opts...)
	defer cancel()

	// Create a new browser context
	taskCtx, cancel := chromedp.NewContext(allocCtx, chromedp.WithLogf(log.Printf))
	defer cancel()

	// Set a timeout for the entire login process
	taskCtx, cancel = context.WithTimeout(taskCtx, 2*time.Minute)
	defer cancel()

	// Channel to signal when the RCT is found
	byteChan := make(chan [][]byte, 1)
	// Slices to hold message data
	var loginSendBytes [][]byte
	var requestID network.RequestID

	chromedp.ListenTarget(taskCtx, func(ev interface{}) {
		switch e := ev.(type) {
		case *network.EventWebSocketCreated:
			if strings.Contains(e.URL, expectedWSURL) {
				log.Println("Game WebSocket connection detected.")
				requestID = e.RequestID
			}
		case *network.EventWebSocketFrameSent:
			if requestID != "" && e.RequestID == requestID {
				loginSendBytes = append(loginSendBytes, processSentFrame(e.Response))
			}
		case *network.EventWebSocketFrameReceived:
			if requestID != "" && e.RequestID == requestID {
				lliRes := processReceivedFrame(e.Response)
				loginSuccess, cooldown := checkForLogin(lliRes)

				if cooldown == 9999 {
					log.Printf("Password incorrect for user: %s", username)
				} else if cooldown == 9998 {
					log.Printf("Unknown login error.")
				} else if cooldown > 0 {
					log.Printf("Cooldown for login: %d seconds.", cooldown)
				}

				if loginSuccess {
					var loginByte, nameByte, vckByte []byte
					var accountNameOnSuccessLogin string

					// First pass: Find login bytes to extract account name
					for _, data := range loginSendBytes {
						if lByte, accName := processLoginBytes(data); lByte != nil {
							loginByte = lByte
							accountNameOnSuccessLogin = accName
							break
						}
					}

					// Second pass: Find name and vck bytes
					for _, data := range loginSendBytes {
						if nByte := processNameBytes(data, accountNameOnSuccessLogin); nByte != nil {
							nameByte = nByte
						}
						if vByte := processVckBytes(data); vByte != nil {
							vckByte = vByte
						}
					}

					// Ensure all parts are found
					if loginByte != nil && nameByte != nil && vckByte != nil {
						// Return in strict order: Login, Name, Vck
						finalSendBytes := [][]byte{loginByte, nameByte, vckByte}
						byteChan <- finalSendBytes
						cancel()
						close(byteChan)
					}
				}
			}
		}
	})

	// Run the automation
	err := chromedp.Run(taskCtx,
		chromedp.EmulateViewport(1280, 633),
		// Navigate to the login page
		chromedp.Navigate(loginURL),
		// Wait for page to load - waiting for a longer time to ensure assets are ready
		chromedp.Sleep(5*time.Second),

		// Pre-Selection: Select English(US) to ensure mapping works
		chromedp.ActionFunc(func(ctx context.Context) error {
			return chromedp.MouseClickXY(800, 60).Do(ctx)
		}),
		chromedp.Sleep(500*time.Millisecond),

		chromedp.ActionFunc(func(ctx context.Context) error {
			return chromedp.MouseClickXY(800, 105).Do(ctx)
		}),
		chromedp.Sleep(500*time.Millisecond),

		chromedp.ActionFunc(func(ctx context.Context) error {
			return input.InsertText("English (US)").Do(ctx)
		}),
		chromedp.Sleep(500*time.Millisecond),

		chromedp.ActionFunc(func(ctx context.Context) error {
			return chromedp.MouseClickXY(800, 130).Do(ctx)
		}),
		chromedp.Sleep(1*time.Second),

		// 0a. Click World Selector dropdown
		chromedp.ActionFunc(func(ctx context.Context) error {
			return chromedp.MouseClickXY(1100, 60).Do(ctx)
		}),
		chromedp.Sleep(1*time.Second),

		// 0b. Click the search box
		chromedp.ActionFunc(func(ctx context.Context) error {
			return chromedp.MouseClickXY(1100, 105).Do(ctx)
		}),
		chromedp.Sleep(500*time.Millisecond),

		// 0c. Type the world name (server parameter contains full name like "United States, World: 1")
		chromedp.ActionFunc(func(ctx context.Context) error {
			return input.InsertText(serverName).Do(ctx) // Keeping this simple for now, but flagging risk.
		}),
		chromedp.Sleep(1*time.Second),

		// 0d. Click the first search result to select the world
		chromedp.ActionFunc(func(ctx context.Context) error {
			return chromedp.MouseClickXY(1100, 130).Do(ctx)
		}),
		chromedp.Sleep(1*time.Second),

		// 1. Click "Log in!" button (to open form)
		chromedp.ActionFunc(func(ctx context.Context) error {
			return chromedp.MouseClickXY(640, 570).Do(ctx)
		}),
		chromedp.Sleep(1*time.Second),

		// 2. Click Username field
		chromedp.ActionFunc(func(ctx context.Context) error {
			return chromedp.MouseClickXY(640, 270).Do(ctx)
		}),
		chromedp.Sleep(500*time.Millisecond),

		// 3. Type Username
		chromedp.ActionFunc(func(ctx context.Context) error {
			// Clear fields might be needed? assuming empty start or overwrite
			return input.InsertText(username).Do(ctx)
		}),
		chromedp.Sleep(500*time.Millisecond),

		// 4. Click Password field
		chromedp.ActionFunc(func(ctx context.Context) error {
			return chromedp.MouseClickXY(640, 350).Do(ctx)
		}),
		chromedp.Sleep(500*time.Millisecond),

		// 5. Type Password
		chromedp.ActionFunc(func(ctx context.Context) error {
			return input.InsertText(password).Do(ctx)
		}),
		chromedp.Sleep(500*time.Millisecond),

		// 6. Click "Log in" submit button
		chromedp.ActionFunc(func(ctx context.Context) error {
			return chromedp.MouseClickXY(640, 460).Do(ctx)
		}),

		// Wait for WebSocket connection or context cancellation
		chromedp.ActionFunc(func(ctx context.Context) error {
			select {
			case <-ctx.Done():
				return ctx.Err()
			}
		}),
	)

	if err != nil && err != context.Canceled {
		log.Printf("Chromedp automation error: %v", err)
	}

	// Read the login bytes from the channel
	select {
	case returnBytes := <-byteChan:
		if len(returnBytes) >= 3 {
			// Save to file for future use
			saveLoginBytesToFile(returnBytes)
			return returnBytes
		}
	default:
		log.Println("Automated login did not complete. Browser was closed or timed out.")
	}
	return nil
}

// saveLoginBytesToFile saves the login bytes to the loginBytes.json file
func saveLoginBytesToFile(loginBytesSlices [][]byte) {
	if len(loginBytesSlices) < 3 {
		return
	}
	loginPath := getLoginFilePath()
	loginData := LoginData{
		Login:     loginBytesSlices[0],
		Name:      loginBytesSlices[1],
		Vck:       loginBytesSlices[2],
		CreatedAt: time.Now(),
	}

	data, err := json.Marshal(loginData)
	if err != nil {
		log.Printf("[DEBUG] Error marshalling login data to JSON: %v", err)
		return
	}

	if err := os.WriteFile(loginPath, data, 0600); err != nil {
		log.Printf("[DEBUG] Error writing to '%s': %v", loginPath, err)
		return
	}
	log.Printf("[DEBUG] Saved login credentials to '%s' (expires in %d hours)", loginPath, loginExpirationHours)
}

func processSentFrame(resp *network.WebSocketFrame) []byte {
	var payloadBytes []byte
	var err error
	switch resp.Opcode {
	case 1: // Text Frame
		payloadBytes = []byte(resp.PayloadData)
	case 2:
		payloadBytes, err = base64.StdEncoding.DecodeString(resp.PayloadData)
		if err != nil {
			log.Printf("Error decoding base64 binary payload: %v | Raw: %s", err, resp.PayloadData)
			return nil
		}
	default:
		log.Printf("Unknown opcode type: %d | Raw: %s", int(resp.Opcode), resp.PayloadData)
		return nil
	}

	return payloadBytes
}

func processReceivedFrame(resp *network.WebSocketFrame) []string {
	var payloadBytes []byte
	var err error
	switch resp.Opcode {
	case 1: // Text Frame
		payloadBytes = []byte(resp.PayloadData)
	case 2:
		payloadBytes, err = base64.StdEncoding.DecodeString(resp.PayloadData)
		if err != nil {
			log.Printf("Error decoding base64 binary payload: %v | Raw: %s", err, resp.PayloadData)
			return nil
		}
	default:
		log.Printf("Unknown opcode type: %d | Raw: %s", int(resp.Opcode), resp.PayloadData)
		return nil
	}
	var msg []string
	messageString := string(payloadBytes)
	msg = strings.Split(messageString, "%")

	if len(msg) > 2 && msg[2] == "lli" {
		return msg
	}
	return nil

}

func checkForLogin(messageParts []string) (bool, int) {
	if len(messageParts) <= 2 {
		return false, 0
	}
	if messageParts[4] == "0" && messageParts[2] == "lli" {
		log.Printf("Login successful")
		return true, 0
	}
	if messageParts[4] == "453" && messageParts[2] == "lli" {
		cooldownString := messageParts[5]
		// The cooldown value is wrapped in JSON-like text, e.g., {"CD":120}
		cooldownStr := strings.TrimPrefix(cooldownString, "{\"CD\":")
		cooldownStr = strings.TrimSuffix(cooldownStr, "}")
		cooldown, err := strconv.Atoi(cooldownStr)
		if err != nil {
			return false, 9998 // Return 0 if parsing fails
		}
		return false, cooldown
	}
	if messageParts[4] == "20" && messageParts[2] == "lli" {
		return false, 9999
	}
	return false, 0

}

func processLoginBytes(data []byte) ([]byte, string) {
	var msg []string
	messageString := string(data)
	msg = strings.Split(messageString, "%")
	if len(msg) > 3 {
		messageType := msg[3]
		if messageType == "lli" {
			var tempData map[string]interface{}
			err := json.Unmarshal([]byte(msg[5]), &tempData)
			if err != nil {
				log.Printf("Error unmarshalling: %v", err)
			}
			accountString := tempData["NOM"].(string)
			return data, accountString
		}

	}
	return nil, ""
}

func processNameBytes(data []byte, accountName string) []byte {
	var msg []string
	messageString := string(data)
	msg = strings.Split(messageString, "%")
	if len(msg) > 3 {
		if msg[3] == "vln" {
			if strings.Contains(msg[5], accountName) {
				return data
			}
		}
	}
	return nil
}

func processVckBytes(data []byte) []byte {
	var msg []string
	messageString := string(data)
	msg = strings.Split(messageString, "%")
	if len(msg) > 3 {
		if msg[3] == "vck" {
			return data
		}
	}
	return nil
}
