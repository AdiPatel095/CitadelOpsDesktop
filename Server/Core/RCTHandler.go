package Core

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"

	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/chromedp"
)

const loginFilename = "loginBytes.json"

// LoginData holds the byte slices for login information.
type LoginData struct {
	Login []byte `json:"login"`
	Name  []byte `json:"name"`
	Vck   []byte `json:"vck"`
}

// GetLoginBytes reads login information from loginFilename.
// If the file does not exist, it calls getLoginBytes to create it by prompting the user to log in.
func GetLoginBytes() [][]byte {
	data, err := os.ReadFile(loginFilename)
	if err != nil {
		if os.IsNotExist(err) {
			log.Printf("'%s' not found. Starting login process to create it.", loginFilename)
			loginBytesSlices := getLoginBytes()
			if len(loginBytesSlices) < 3 {
				log.Println("Failed to retrieve login bytes. Aborting.")
				return nil
			}

			loginData := LoginData{
				Login: loginBytesSlices[0],
				Name:  loginBytesSlices[1],
				Vck:   loginBytesSlices[2],
				// Vck is not populated by getLoginBytes currently
			}

			data, err = json.Marshal(loginData)
			if err != nil {
				log.Printf("Error marshalling login data to JSON: %v", err)
				return nil
			}

			if err := os.WriteFile(loginFilename, data, 0600); err != nil {
				log.Printf("Error writing to '%s': %v", loginFilename, err)
				return nil
			}
			log.Printf("Successfully created and wrote login data to '%s'", loginFilename)
		} else {
			log.Printf("Error reading '%s': %v", loginFilename, err)
			return nil
		}
	}

	var storedLoginData LoginData
	if err := json.Unmarshal(data, &storedLoginData); err != nil {
		log.Printf("Error unmarshalling login data from '%s': %v", loginFilename, err)
		return nil
	}

	return [][]byte{storedLoginData.Login, storedLoginData.Name, storedLoginData.Vck}
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
