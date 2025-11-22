package Websocket

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strconv"
	"strings"

	"github.com/chromedp/cdproto/target"
	"github.com/chromedp/chromedp"
)

// AppMessage defines a generic structure for parsed WebSocket messages.
// You can customize this struct to match the actual format of your messages.
type AppMessage struct {
	Type    string          `json:"type"`
	Payload json.RawMessage `json:"payload"`
}

// StartMessageProcessor launches a goroutine to listen on the message channels.
// It parses messages and dispatches them to handler functions.
func StartMessageProcessor(ctx context.Context, hub *Hub) {
	log.Println("Starting WebSocket message processor...")

	go func() {
		for {
			select {
			case <-ctx.Done():
				log.Println("Stopping WebSocket message processor...")
				return
			case msgBytes := <-IncomingMessages:
				handleParsedMessage(hub, msgBytes, "INCOMING")
			case msgBytes := <-OutgoingMessages:
				handleParsedMessage(hub, msgBytes, "OUTGOING")
			}
		}
	}()
}

// handleParsedMessage is an internal handler that parses a message and logs it.
// This is where you would add logic to act on specific message types.
func handleParsedMessage(hub *Hub, msgBytes []byte, direction string) {
	messageParts := ParseMessageIntoParts(msgBytes)
	if len(messageParts) > 1 {
		messageType := messageParts[2]
		if messageType == "lli" {
			loginOk, cd := CheckForSuccessfulLogin(messageParts)
			if cd == 9999 {
				log.Printf("User password incorrect")
			}
			if cd == 9998 {
				log.Printf("Unknown login error")
			}
			if loginOk {
				// Construct a structured message for the frontend
				responsePayload := map[string]string{"status": "success", "message": "Game login successful"}
				responsePayloadBytes, err := json.Marshal(responsePayload)
				if err != nil {
					log.Printf("Error marshalling login success payload: %v", err)
					return
				}

				responseMsgBytes, err := json.Marshal(AppMessage{Type: "LOGIN_STATUS", Payload: responsePayloadBytes})
				if err != nil {
					log.Printf("Error marshalling login success message: %v", err)
					return
				}
				hub.Broadcast <- responseMsgBytes
			}
		}
	}
	log.Printf("Received message: %s", messageParts)

}

func ParseMessageIntoParts(msgBytes []byte) []string {
	var msg []string
	messageString := string(msgBytes)
	msg = strings.Split(messageString, "%")
	return msg
}

func CheckForSuccessfulLogin(messageParts []string) (bool, int) {
	if messageParts[4] == "0" {
		log.Printf("Login successful")
		return true, 0
	}
	if messageParts[4] == "453" {
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
	if messageParts[4] == "20" {
		return false, 9999
	}
	return false, 0
}

// SendCommandToPage finds a browser tab with a URL containing `pageName` (e.g., "index.html")
// and executes a JavaScript function in its console.
func SendCommandToPage(ctx context.Context, message interface{}) error {
	// 1. Find all available browser targets (tabs, workers, etc.).
	targets, err := chromedp.Targets(ctx)
	if err != nil {
		return fmt.Errorf("could not get browser targets: %w", err)
	}

	pageName := "index.html"
	functionName := "ggs_lib(4).BasicModel._smartfoxClient.socket.send"

	var targetID target.ID
	// 2. Iterate through targets to find the one matching our page name.
	for _, t := range targets {
		if t.Type == "page" && strings.Contains(t.URL, pageName) {
			targetID = t.TargetID
			log.Printf("Found target page: %s (ID: %s)", t.URL, targetID)
			break
		}
	}

	if targetID == "" {
		return fmt.Errorf("no page target found with URL containing '%s'", pageName)
	}

	// 3. Create a new context specifically for the found target.
	targetCtx, cancel := chromedp.NewContext(ctx, chromedp.WithTargetID(targetID))
	defer cancel()

	// 4. Marshal the message into a JSON string to be safely passed into JavaScript.
	messageJSON, err := json.Marshal(message)
	if err != nil {
		return fmt.Errorf("failed to marshal message to JSON: %w", err)
	}

	// 5. Construct the JavaScript to execute.
	// This script calls the specified function on the window object with the JSON message.
	script := fmt.Sprintf(`%s(%s)`, functionName, string(messageJSON))

	log.Printf("Executing script on target %s: %s", targetID, script)

	// 6. Run the evaluation task.
	var res interface{} // To store any result from the JS function.
	err = chromedp.Run(targetCtx,
		chromedp.Evaluate(script, &res),
	)

	if err != nil {
		return fmt.Errorf("failed to execute script on page '%s': %w", pageName, err)
	}

	log.Printf("Script executed successfully. Result: %v", res)
	return nil
}
