package Core

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
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

// getLoginFilePath returns the absolute path to the loginBytes file
// relative to the executable directory
func getLoginFilePath() string {
	ex, err := os.Executable()
	if err != nil {
		// Fallback to current working directory if executable path fails
		return loginFilename
	}
	return filepath.Join(filepath.Dir(ex), loginFilename)
}

// GetLoginBytes reads login information from loginFilename.
// If the file does not exist or is expired, it calls getLoginBytes to create it by prompting the user to log in.
func GetLoginBytes() [][]byte {
	loginPath := getLoginFilePath()
	needsRefresh := false

	data, err := os.ReadFile(loginPath)
	if err != nil {
		if os.IsNotExist(err) {
			log.Printf("'%s' not found. Starting login process to create it.", loginPath)
			needsRefresh = true
		} else {
			log.Printf("Error reading '%s': %v", loginPath, err)
			return nil
		}
	} else {
		// File exists, check if it's expired
		var storedLoginData LoginData
		if err := json.Unmarshal(data, &storedLoginData); err != nil {
			log.Printf("Error unmarshalling login data from '%s': %v. Refreshing credentials.", loginPath, err)
			needsRefresh = true
		} else if isLoginExpired(storedLoginData.CreatedAt) {
			log.Printf("Login credentials have expired (older than %d hours). Refreshing...", loginExpirationHours)
			// Delete the expired file
			if err := os.Remove(loginPath); err != nil {
				log.Printf("Warning: Could not delete expired login file: %v", err)
			}
			needsRefresh = true
		} else {
			// Valid and not expired
			hoursRemaining := loginExpirationHours - int(time.Since(storedLoginData.CreatedAt).Hours())
			log.Printf("Login credentials valid. Expires in approximately %d hours.", hoursRemaining)
			return [][]byte{storedLoginData.Login, storedLoginData.Name, storedLoginData.Vck}
		}
	}

	if needsRefresh {
		loginBytesSlices := getLoginBytes()
		if len(loginBytesSlices) < 3 {
			log.Println("Failed to retrieve login bytes. Aborting.")
			return nil
		}

		loginData := LoginData{
			Login:     loginBytesSlices[0],
			Name:      loginBytesSlices[1],
			Vck:       loginBytesSlices[2],
			CreatedAt: time.Now(), // Set the creation timestamp
		}

		data, err = json.Marshal(loginData)
		if err != nil {
			log.Printf("Error marshalling login data to JSON: %v", err)
			return nil
		}

		if err := os.WriteFile(loginPath, data, 0600); err != nil {
			log.Printf("Error writing to '%s': %v", loginPath, err)
			return nil
		}
		log.Printf("Successfully created and wrote login data to '%s' (expires in %d hours)", loginPath, loginExpirationHours)

		return [][]byte{loginData.Login, loginData.Name, loginData.Vck}
	}

	return nil
}

// isLoginExpired checks if the login credentials have expired based on the creation time.
func isLoginExpired(createdAt time.Time) bool {
	if createdAt.IsZero() {
		// If no timestamp exists (legacy file), consider it expired
		return true
	}
	expirationTime := createdAt.Add(time.Duration(loginExpirationHours) * time.Hour)
	return time.Now().After(expirationTime)
}

// ServerURLMap maps frontend server display names to actual server identifiers
// Used to construct the WebSocket URL dynamically
var ServerURLMap = map[string]string{
	"United States": "ep-live-us1-game",
	"World: 2":      "ep-live-world2-game",
	"World: 3":      "ep-live-world3-game",
	"World: 4":      "ep-live-world4-game",
	// Add more servers as needed
}

// GetLoginBytesWithCredentials automates the login process using provided credentials.
// It uses ChromeDP to:
// 1. Navigate to the login page
// 2. Click the login button to open the login form
// 3. Fill in username and password
// 4. Click submit
// 5. Capture WebSocket login bytes
func GetLoginBytesWithCredentials(username, password, server string) [][]byte {
	// Determine the game server URL based on the server parameter
	serverID := ServerURLMap[server]
	if serverID == "" {
		serverID = "ep-live-us1-game" // Default to US1
	}
	expectedWSURL := serverID + ".goodgamestudios.com"
	log.Printf("Using server: %s (%s)", server, expectedWSURL)

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

	// Prompt in console for visibility
	fmt.Println("\n=================================================================")
	fmt.Println("AUTOMATED LOGIN: Using saved credentials...")
	fmt.Printf("Username: %s\n", username)
	fmt.Printf("Server: %s\n", server)
	fmt.Println("=================================================================")

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
					finalSendBytes := make([][]byte, 0, 3)
					var accountNameOnSuccessLogin string
					for _, data := range loginSendBytes {
						if loginByte, accountString := processLoginBytes(data); loginByte != nil {
							accountNameOnSuccessLogin = accountString
							finalSendBytes = append(finalSendBytes, loginByte)
						}
						if nameByte := processNameBytes(data, accountNameOnSuccessLogin); nameByte != nil {
							finalSendBytes = append(finalSendBytes, nameByte)
						}
						if vckByte := processVckBytes(data); vckByte != nil {
							finalSendBytes = append(finalSendBytes, vckByte)
						}
					}
					byteChan <- finalSendBytes
					cancel()
					close(byteChan)
				}
			}
		}
	})

	// Run the automation
	log.Printf("Opening browser and navigating to %s...", loginURL)
	err := chromedp.Run(taskCtx,
		chromedp.EmulateViewport(1280, 633),
		// Navigate to the login page
		chromedp.Navigate(loginURL),
		// Wait for page to load - waiting for a longer time to ensure assets are ready
		chromedp.Sleep(5*time.Second),

		// 0a. Click World Selector dropdown
		chromedp.ActionFunc(func(ctx context.Context) error {
			log.Println("Clicking World Selector dropdown...")
			return chromedp.MouseClickXY(1100, 60).Do(ctx)
		}),
		chromedp.Sleep(1*time.Second),

		// 0b. Click the search box
		chromedp.ActionFunc(func(ctx context.Context) error {
			log.Println("Clicking World search box...")
			return chromedp.MouseClickXY(1100, 105).Do(ctx)
		}),
		chromedp.Sleep(500*time.Millisecond),

		// 0c. Type the world name (server parameter contains full name like "United States, World: 1")
		chromedp.ActionFunc(func(ctx context.Context) error {
			log.Printf("Typing world name: %s...", server)
			return input.InsertText(server).Do(ctx)
		}),
		chromedp.Sleep(1*time.Second),

		// 0d. Click the first search result to select the world
		chromedp.ActionFunc(func(ctx context.Context) error {
			log.Println("Selecting first search result...")
			return chromedp.MouseClickXY(1100, 130).Do(ctx)
		}),
		chromedp.Sleep(1*time.Second),

		// 1. Click "Log in!" button (to open form)
		chromedp.ActionFunc(func(ctx context.Context) error {
			log.Println("Clicking 'Log in!' button...")
			return chromedp.MouseClickXY(640, 570).Do(ctx)
		}),
		chromedp.Sleep(1*time.Second),

		// 2. Click Username field
		chromedp.ActionFunc(func(ctx context.Context) error {
			log.Println("Clicking Username field...")
			return chromedp.MouseClickXY(640, 270).Do(ctx)
		}),
		chromedp.Sleep(500*time.Millisecond),

		// 3. Type Username
		chromedp.ActionFunc(func(ctx context.Context) error {
			log.Printf("Typing username: %s...", username)
			// Clear fields might be needed? assuming empty start or overwrite
			return input.InsertText(username).Do(ctx)
		}),
		chromedp.Sleep(500*time.Millisecond),

		// 4. Click Password field
		chromedp.ActionFunc(func(ctx context.Context) error {
			log.Println("Clicking Password field...")
			return chromedp.MouseClickXY(640, 350).Do(ctx)
		}),
		chromedp.Sleep(500*time.Millisecond),

		// 5. Type Password
		chromedp.ActionFunc(func(ctx context.Context) error {
			log.Println("Typing password...")
			return input.InsertText(password).Do(ctx)
		}),
		chromedp.Sleep(500*time.Millisecond),

		// 6. Click "Log in" submit button
		chromedp.ActionFunc(func(ctx context.Context) error {
			log.Println("Clicking Submit button...")
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
			log.Println("Automated login successful! Credential bytes captured.")
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
		log.Printf("Error marshalling login data to JSON: %v", err)
		return
	}

	if err := os.WriteFile(loginPath, data, 0600); err != nil {
		log.Printf("Error writing to '%s': %v", loginPath, err)
		return
	}
	log.Printf("Saved login credentials to '%s' (expires in %d hours)", loginPath, loginExpirationHours)
}

// getLoginBytes launches a visible browser for the user to log in and retrieve an RCT.
// It prompts the user to paste the RCT into the console.
func getLoginBytes() [][]byte {
	// URL to navigate to for login.
	// IMPORTANT: Change this to the actual login URL.
	const loginURL = "https://empire.goodgamestudios.com/"

	// Set up chromedp options for a visible browser with a debugging port.
	opts := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.Flag("headless", false),
		chromedp.Flag("remote-debugging-port", "9222"),
		chromedp.Flag("start-maximized", true),
	)

	allocCtx, cancel := chromedp.NewExecAllocator(context.Background(), opts...)
	defer cancel()

	// Create a new browser context
	taskCtx, cancel := chromedp.NewContext(allocCtx, chromedp.WithLogf(log.Printf))
	defer cancel()

	// Prompt the user in the console to perform the login
	fmt.Println("\n=================================================================")
	fmt.Println("ACTION REQUIRED: A browser window has been opened.")
	fmt.Println("1. Please log in to your account.")
	fmt.Println("2. Wait for Citadel to process some data.")
	fmt.Println("   The application will automatically detect the login.")
	fmt.Println("=================================================================")

	// Channel to signal when the RCT is found
	byteChan := make(chan [][]byte, 1)
	// Slices to hold message data, scoped to be accessible by the listener
	var loginSendBytes [][]byte
	var _ [][]string
	var requestID network.RequestID

	chromedp.ListenTarget(taskCtx, func(ev interface{}) {
		switch e := ev.(type) {
		case *network.EventWebSocketCreated:
			if strings.Contains(e.URL, "ep-live-us1-game.goodgamestudios.com") {
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
					log.Printf("User password incorrect, please try again.")
				} else if cooldown == 9998 {
					log.Printf("Unknown login error.")
				} else if cooldown > 0 {
					log.Printf("Cooldown for login: %d seconds. Please wait and try again.", cooldown)
				}

				if loginSuccess {
					finalSendBytes := make([][]byte, 0, 2)
					var accountNameOnSuccessLogin string
					for _, data := range loginSendBytes {
						if loginByte, accountString := processLoginBytes(data); loginByte != nil {
							accountNameOnSuccessLogin = accountString
							finalSendBytes = append(finalSendBytes, loginByte)
						}
						if nameByte := processNameBytes(data, accountNameOnSuccessLogin); nameByte != nil {
							finalSendBytes = append(finalSendBytes, nameByte)
						}
						if vckByte := processVckBytes(data); vckByte != nil {
							finalSendBytes = append(finalSendBytes, vckByte)
						}
					}
					byteChan <- finalSendBytes
					cancel()
					close(byteChan)
				}
			}
		}
	})

	// Run the navigation task and block until a value is received on byteChan
	// or the context is cancelled by our listener (or a timeout).
	log.Printf("Opening browser and navigating to %s...", loginURL)
	err := chromedp.Run(taskCtx,
		chromedp.Navigate(loginURL),
		// This action will block until the context is canceled.
		chromedp.ActionFunc(func(ctx context.Context) error {
			select {
			case <-ctx.Done():
				return ctx.Err()
			}
		}),
	)

	if err != nil && err != context.Canceled {
		log.Printf("Chromedp error: %v", err)
	}

	// Read the RCT from the channel. This will have a value if the login was successful.
	select {
	case returnBytes := <-byteChan:
		log.Println("RCT has been successfully retrieved. Browser closed.")
		return returnBytes
	default:
		log.Println("Browser was closed before RCT could be retrieved.")
		return nil // Return empty if no RCT was found
	}
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
