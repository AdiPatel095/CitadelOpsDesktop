package GameWebsocket

import (
	"CitadelDesktop/Server/Core"
	"CitadelDesktop/Server/GameParser"
	"CitadelDesktop/Server/License"
	"CitadelDesktop/Server/Models"
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

type OutgoingMessageWithCost struct {
	Payload []byte
	Cost    int
}

var (
	LoginStatus      bool
	LoginCooldown    int
	GlobalSocket     *websocket.Conn
	IncomingMessages = make(chan []string)
	OutgoingMessages = make(chan interface{})

	// SendInsufficientCreditsFunc is a callback to notify frontend of insufficient credits
	SendInsufficientCreditsFunc func()

	// SendGameLoginStatusFunc is a callback to notify frontend of login status changes
	SendGameLoginStatusFunc func(bool, int)

	// SendAutoBirdStatusFunc is a callback to notify frontend of auto bird status changes
	SendAutoBirdStatusFunc func(bool, int64)

	// SendRequestCredentialsFunc is a callback to request credentials from frontend
	SendRequestCredentialsFunc func()

	// StoredCredentials holds the last used login info for auto-relogin
	StoredCredentials struct {
		Username string
		Password string
		Server   string
	}
)

// ServerURLMap maps frontend server display names to actual server identifiers
var ServerURLMap = map[string]string{
	"United States":         "ep-live-us1-game",
	"World: 2":              "ep-live-world2-game",
	"World: 1":              "ep-live-world1-game",
	"International: 1":      "ep-live-mz-int1-sk1-gb1-game",
	"International: 2":      "ep-live-mz-int2-es1-it1-game",
	"International: 3":      "ep-live-int3-game",
	"Germany":               "ep-live-de1-game",
	"France":                "ep-live-fr1-game",
	"Czech Republic":        "ep-live-mz-cz1-es2-game",
	"Poland":                "ep-live-pl1-game",
	"Portuguese":            "ep-live-pt1-game",
	"Spain: 1":              "ep-live-mz-int2-es1-it1-game",
	"Italy":                 "ep-live-mz-int2-es1-it1-game",
	"Turkey":                "ep-live-mz-tr1-nl1-bg1-game",
	"Netherlands":           "ep-live-mz-tr1-nl1-bg1-game",
	"Hungary: 1":            "ep-live-mz-hu1-skn1-gr1-lt1-game",
	"Nordic":                "ep-live-mz-hu1-skn1-gr1-lt1-game",
	"Russia":                "ep-live-ru1-game",
	"Romania":               "ep-live-ro1-game",
	"Bulgaria":              "ep-live-mz-tr1-nl1-bg1-game",
	"Hungary: 2":            "ep-live-hu2-game",
	"Slovakia":              "ep-live-mz-int1-sk1-gb1-game",
	"United Kingdom":        "ep-live-mz-int1-sk1-gb1-game",
	"Brazil":                "ep-live-br1-game",
	"Australia":             "ep-live-au1-game",
	"South Korea":           "ep-live-mz-kr1-jp1-in1-cn1-game",
	"Japan":                 "ep-live-mz-kr1-jp1-in1-cn1-game",
	"Hispanic America":      "ep-live-his1-game",
	"India":                 "ep-live-mz-kr1-jp1-in1-cn1-game",
	"China":                 "ep-live-mz-kr1-jp1-in1-cn1-game",
	"Greece":                "ep-live-mz-hu1-skn1-gr1-lt1-game",
	"Lithuania":             "ep-live-mz-hu1-skn1-gr1-lt1-game",
	"Saudi Arabia":          "ep-live-mz-sa1-ae1-eg1-arab1-game",
	"United Arab Emirates":  "ep-live-mz-sa1-ae1-eg1-arab1-game",
	"Egypt":                 "ep-live-mz-sa1-ae1-eg1-arab1-game",
	"Arab League":           "ep-live-mz-sa1-ae1-eg1-arab1-game",
	"Asia":                  "ep-live-asia1-hant1-game",
	"Chinese (traditional)": "ep-live-asia1-hant1-game",
	"Spain: 2":              "ep-live-mz-cz1-es2-game",
}

// SetInsufficientCreditsCallback sets the callback for insufficient credits notification
func SetInsufficientCreditsCallback(fn func()) {
	SendInsufficientCreditsFunc = fn
}

// SetGameLoginStatusCallback sets the callback for game login status notification
func SetGameLoginStatusCallback(fn func(bool, int)) {
	SendGameLoginStatusFunc = fn
}

// SetAutoBirdStatusCallback sets the callback for auto bird status notification
func SetAutoBirdStatusCallback(fn func(bool, int64)) {
	SendAutoBirdStatusFunc = fn
}

// SetRequestCredentialsCallback sets the callback for requesting credentials
func SetRequestCredentialsCallback(fn func()) {
	SendRequestCredentialsFunc = fn
}

func NewGameWebsocket(serverID string) error {
	// Get config from environment
	startURL := "https://empire.goodgamestudios.com/"
	origin := os.Getenv("ORIGIN")
	bearer := os.Getenv("AUTH_BEARER")
	authCookie := os.Getenv("AUTH_COOKIE")

	if serverID == "" {
		serverID = "ep-live-us1-game" // Default fallback
	}
	wssURL := "wss://" + serverID + ".goodgamestudios.com/"

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

	// Reset game state for fresh connection
	Models.GetGameState().Reset()

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
				if strings.Contains(err.Error(), "use of closed network connection") {
					log.Println("Logging out of the game")
				} else {
					log.Println("read:", err)
				}
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
				return
			case message := <-OutgoingMessages:
				var payload []byte
				cost := 0

				// Parse message type and determine cost
				switch v := message.(type) {
				case OutgoingMessageWithCost:
					payload = v.Payload
					cost = v.Cost
				case []byte:
					payload = v
					cost = 0
				case string:
					payload = []byte(v)
					cost = 0
				default:
					log.Printf("Unknown message type in OutgoingMessages: %T", v)
					continue
				}

				// Deduct credits based on message cost
				allowed := true
				if cost > 0 {
					if !License.UseCredits(cost, "Game Message") {
						allowed = false
					}
				}

				if allowed {
					err := GlobalSocket.WriteMessage(websocket.TextMessage, payload)
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

func StopGame() {
	if GlobalSocket != nil {
		err := GlobalSocket.Close()
		if err != nil {
			return
		}
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

// StartGameWithCredentials starts the game with provided login credentials
// Uses ChromeDP to automate the login process
func StartGameWithCredentials(username, password, server string) {
	// Store credentials for auto-relogin (e.g. by AutoBird)
	StoredCredentials.Username = username
	StoredCredentials.Password = password
	StoredCredentials.Server = server

	go func() {
		// Resolve Server Name to ID
		serverID := ServerURLMap[server]
		if serverID == "" {
			serverID = "ep-live-us1-game" // Default to US1 if unknown
			log.Printf("Warning: Unknown server '%s', defaulting to US1 (%s)", server, serverID)
		}

		// Use Core to get login bytes with automated credentials
		loginBytes := Core.GetLoginBytesWithCredentials(username, password, server, serverID)
		if loginBytes == nil {
			log.Println("Failed to get login bytes with credentials")
			if SendGameLoginStatusFunc != nil {
				SendGameLoginStatusFunc(false, 0)
			}
			return
		}

		if GlobalSocket != nil {
			log.Println("Game websocket already connected.")
			return
		}

		err := NewGameWebsocket(serverID)
		if err != nil {
			log.Printf("Failed to create game websocket: %v", err)
			if SendGameLoginStatusFunc != nil {
				SendGameLoginStatusFunc(false, 0)
			}
			return
		}

		success := LoginToGame(loginBytes)
		if success {
			if SendGameLoginStatusFunc != nil {
				SendGameLoginStatusFunc(true, 0)
			}
		} else {
			// Login failed after all retries - notify frontend to clear connected state
			if SendGameLoginStatusFunc != nil {
				SendGameLoginStatusFunc(false, 0)
			}
		}
	}()
}
