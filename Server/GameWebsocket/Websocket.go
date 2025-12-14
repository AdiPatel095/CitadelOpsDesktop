package GameWebsocket

import (
	"CitadelDesktop/Server/Core"
	"CitadelDesktop/Server/GameParser"
	"CitadelDesktop/Server/License"
	"log"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"context"

	"github.com/gorilla/websocket"
)

type AutoSendPackage struct {
	Interval int    `json:"interval"`
	Payload  string `json:"payload"`
}

var (
	LoginStatus      bool
	LoginCooldown    int
	GlobalSocket     *websocket.Conn
	IncomingMessages = make(chan []string)
	OutgoingMessages = make(chan []byte)

	// SendInsufficientCreditsFunc is a callback to notify frontend of insufficient credits
	SendInsufficientCreditsFunc func()

	// SendGameLoginStatusFunc is a callback to notify frontend of login status changes
	SendGameLoginStatusFunc func(bool, int)
)

// SetInsufficientCreditsCallback sets the callback for insufficient credits notification
func SetInsufficientCreditsCallback(fn func()) {
	SendInsufficientCreditsFunc = fn
}

// SetGameLoginStatusCallback sets the callback for game login status notification
func SetGameLoginStatusCallback(fn func(bool, int)) {
	SendGameLoginStatusFunc = fn
}

func NewGameWebsocket() error {
	// Get config from environment
	startURL := "https://empire.goodgamestudios.com/"
	origin := os.Getenv("ORIGIN")
	bearer := os.Getenv("AUTH_BEARER")
	authCookie := os.Getenv("AUTH_COOKIE")

	wssURL := "wss://ep-live-us1-game.goodgamestudios.com/"

	wsHeaders := http.Header{}
	if origin != "" {
		wsHeaders.Set("Origin", origin)
	}
	if bearer != "" {
		wsHeaders.Set("Authorization", "Bearer "+bearer)
	}

	var allCookies []string

	// If START_URL is provided, make a request to get session cookies
	jar, err := cookiejar.New(nil)
	if err != nil {
		return err
	}
	client := &http.Client{
		Jar:       jar,
		Timeout:   10 * time.Second,
		Transport: http.DefaultTransport,
	}
	resp, err := client.Get(startURL)
	if err != nil {
		log.Printf("Failed to make request to START_URL: %v. Proceeding without session cookies.", err)
	} else {
		err := resp.Body.Close()
		if err != nil {
			return err
		}

		if u, err := url.Parse(wssURL); err == nil {
			for _, c := range jar.Cookies(u) {
				allCookies = append(allCookies, c.String())
			}
		}
	}

	// Add manually specified cookie from .env
	if authCookie != "" {
		allCookies = append(allCookies, authCookie)
	}

	// Set the combined cookie header
	if len(allCookies) > 0 {
		wsHeaders.Set("Cookie", strings.Join(allCookies, "; "))
	}

	// Dial the WebSocket authServer
	log.Printf("Connecting to %s", wssURL)
	GlobalSocket, _, err = websocket.DefaultDialer.Dial(wssURL, wsHeaders)
	if err != nil {
		return err
	}
	// Create a context that can be canceled to signal goroutines to stop
	ctx, cancel := context.WithCancel(context.Background())
	StartWebsocketChannels(ctx, cancel)
	SendPreparatoryPackets(ctx)
	go incomingMessageParserStartup() // Start the background message parser
	return nil
}

func StartWebsocketChannels(ctx context.Context, cancel context.CancelFunc) {
	// When the reading goroutine exits (e.g., connection closed), cancel the context.

	go func() {
		// When this goroutine exits, it means the connection is dead.
		// We cancel the context to signal all other goroutines to stop.
		defer func() {
			cancel()
			// Update login status to disconnected when connection is force closed
			LoginStatus = false
			LoginCooldown = 0
			// Reset global socket to allow reconnection
			GlobalSocket = nil
			if SendGameLoginStatusFunc != nil {
				SendGameLoginStatusFunc(false, 0)
			}
		}()
		for {
			_, message, err := GlobalSocket.ReadMessage()
			if err != nil {
				log.Println("read:", err)
				return
			}
			messageRawString := string(message)
			messageParts := strings.Split(messageRawString, "%")
			IncomingMessages <- messageParts
		}
	}()

	go func() {
		for {
			select {
			case <-ctx.Done():
				// Context was canceled, stop trying to write.
				log.Println("Write goroutine stopping.")
				return
			case message := <-OutgoingMessages:
				// Deduct 1 credit for every message
				// Note: License.UseCredits will handle the logic and return false if not enough credits
				if License.UseCredits(1, "Game Message") {
					err := GlobalSocket.WriteMessage(websocket.TextMessage, message)
					time.Sleep(50 * time.Millisecond)
					if err != nil {
						log.Println("write:", err)
						// The read goroutine will handle cancellation, so we just exit.
						return
					}
				} else {
					log.Println("Failed to send message: Insufficient credits")
					if SendInsufficientCreditsFunc != nil {
						go SendInsufficientCreditsFunc()
					}
				}
			}
		}
	}()
}

func SendPreparatoryPackets(ctx context.Context) {
	packages := []AutoSendPackage{
		{
			Interval: 99999999,
			Payload:  "<msg t='sys'><body action='verChk' r='0'><ver v='166' /></body></msg>",
		},
		{
			Interval: 99999999,
			Payload:  "<msg t='sys'><body action='login' r='0'><login z='EmpireEx_21'><nick><![CDATA[]]></nick><pword><![CDATA[1147002%en%0]]></pword></login></body></msg>",
		},
		{
			Interval: 99999999,
			Payload:  "<msg t='sys'><body action='autoJoin' r='-1'></body></msg>",
		},
		{
			Interval: 99999999,
			Payload:  "<msg t='sys'><body action='roundTrip' r='1'></body></msg>",
		},
		{
			Interval: 60000,
			Payload:  "%xt%EmpireEx_21%pin%1%<RoundHouseKick>%",
		},
	}

	for _, pkg := range packages {
		// A large, specific interval number is used as a signal to send the packet only once.
		if pkg.Interval == 99999999 {
			OutgoingMessages <- []byte(pkg.Payload)
		} else if pkg.Interval > 0 {
			// For any other positive interval, set up a recurring send in a new goroutine.
			go func(p AutoSendPackage) {
				// The ticker will start its countdown now, and the first packet
				// will be sent after the first interval has passed.
				ticker := time.NewTicker(time.Duration(p.Interval) * time.Millisecond)
				defer ticker.Stop()
				for {
					select {
					case <-ctx.Done():
						// The context was canceled (e.g., websocket closed), so stop the ticker.
						return
					case <-ticker.C:
						OutgoingMessages <- []byte(p.Payload)
					}
				}
			}(pkg)
		}
	}
}

func LoginToGame(loginBytes [][]byte) bool {
	maxRetries := 3

	if len(loginBytes) == 0 {
		log.Println("Login failed: login payload is empty.")
		return false
	}

	for i := 0; i < maxRetries; i++ {
		log.Printf("Login attempt %d/%d...", i+1, maxRetries)
		LoginStatus = false // Reset status before each attempt
		LoginCooldown = 0   // Reset cooldown

		if SendGameLoginStatusFunc != nil {
			go SendGameLoginStatusFunc(LoginStatus, LoginCooldown)
		}

		OutgoingMessages <- loginBytes[0]
		OutgoingMessages <- loginBytes[1]
		OutgoingMessages <- loginBytes[2]

		// Wait a moment for the server to respond and the parser to process it.
		time.Sleep(2 * time.Second)

		if LoginStatus {
			log.Println("Login successful!")
			return true
		}

		if LoginCooldown > 0 {
			if LoginCooldown == 9999 {
				log.Println("Wrong password, please delete login file and try again !!!")
			}
			waitDuration := time.Duration(LoginCooldown+2) * time.Second
			log.Printf("Login failed with cooldown. Waiting for %v to retry...", waitDuration)
			time.Sleep(waitDuration)
		}
	}

	log.Printf("Login failed after %d attempts.", maxRetries)
	return false
}

func incomingMessageParserStartup() {
	for message := range IncomingMessages {
		if len(message) > 3 {
			if message[2] == "lli" {
				checkLoginStatus(message)
			}
			GameParser.MessageRouter(message)

		}
	}
}

func checkLoginStatus(message []string) {
	if message[4] == "0" {
		LoginStatus = true
		LoginCooldown = 0
		if SendGameLoginStatusFunc != nil {
			go SendGameLoginStatusFunc(LoginStatus, LoginCooldown)
		}
	}
	if message[4] == "453" {
		cooldownString := message[5]
		cooldownStr := strings.TrimPrefix(cooldownString, "{\"CD\":")
		cooldownStr = strings.TrimSuffix(cooldownStr, "}")
		LoginCooldown, _ = strconv.Atoi(cooldownStr)
		if SendGameLoginStatusFunc != nil {
			go SendGameLoginStatusFunc(LoginStatus, LoginCooldown)
		}
	}
}

func StartGame() {
	go func() {
		// Use Core to get login bytes
		loginBytes := Core.GetLoginBytes()
		if loginBytes == nil {
			log.Println("Failed to get login bytes")
			return
		}

		if GlobalSocket != nil {
			log.Println("Game websocket already connected.")
			return
		}

		err := NewGameWebsocket()
		if err != nil {
			log.Printf("Failed to create game websocket: %v", err)
			return
		}

		success := LoginToGame(loginBytes)
		if success {
			if SendGameLoginStatusFunc != nil {
				SendGameLoginStatusFunc(true, 0)
			}
			// Trigger initial data load here if needed, or wait for frontend to request it
			// Based on current architecture, SendInitialData in FrontendWebsocket might need to be triggered or
			// the frontend will request data via `refreshEquipment` or other calls.
			// Since `isGameDataReady` in frontend relies on `gameLoginStatus`, setting it to true here is key.
		}
	}()
}

func StopGame() {
	if GlobalSocket != nil {
		GlobalSocket.Close()
		GlobalSocket = nil
	}
	// Cancel context? We need to store the cancel function from NewGameWebsocket
	// For now, closing the socket should trigger the read loop to exit, which calls cancel.

	LoginStatus = false
	LoginCooldown = 0
	if SendGameLoginStatusFunc != nil {
		SendGameLoginStatusFunc(false, 0)
	}
}
