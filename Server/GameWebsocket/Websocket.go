package GameWebsocket

import (
	"CitadelDesktop/Server/GameParser"
	"CitadelDesktop/Server/License"
	"CitadelDesktop/Server/Models"
	"context"
	"encoding/base64"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/cdproto/page"
	"github.com/chromedp/cdproto/runtime"
	"github.com/chromedp/chromedp"
)

var gameRequestIDs = make(map[string]bool)

type OutgoingMessageWithCost struct {
	Payload []byte
	Cost    int
}

var (
	LoginStatus            bool
	LoginCooldown          int
	BrowserCtx             context.Context
	BrowserCancel          context.CancelFunc
	gameExecutionContextID runtime.ExecutionContextID
	IncomingMessages       = make(chan []string, 100)
	OutgoingMessages       = make(chan interface{}, 100)

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
		Server string
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

func handleCDPEvent(ev interface{}) {
	switch ev := ev.(type) {
	case *runtime.EventBindingCalled:
		if ev.Name == "citadelNotify" {
			gameExecutionContextID = ev.ExecutionContextID
		}
	case *network.EventWebSocketCreated:
		if strings.Contains(ev.URL, "ep-live") {
			gameRequestIDs[string(ev.RequestID)] = true
		}
	case *network.EventWebSocketClosed:
		if gameRequestIDs[string(ev.RequestID)] {
			log.Println("[WebSocket] Game WebSocket closed (disconnected/kicked)")
			delete(gameRequestIDs, string(ev.RequestID))
			LoginStatus = false
			LoginCooldown = 0
			if SendGameLoginStatusFunc != nil {
				go SendGameLoginStatusFunc(false, 0)
			}
		}
	case *network.EventWebSocketFrameReceived:
		if gameRequestIDs[string(ev.RequestID)] {
			payload := ev.Response.PayloadData
			if decoded, err := base64.StdEncoding.DecodeString(payload); err == nil {
				payload = string(decoded)
			}
			messageParts := strings.Split(payload, "%")
			go func() {
				IncomingMessages <- messageParts
			}()
		}
	}
}

const gameInjectionJS = `
			const applyHook = function(globalObj) {
				if (globalObj._citadelHooked) return;
				globalObj._citadelHooked = true;
				
				const OriginalWebSocket = globalObj.WebSocket;
				if (!OriginalWebSocket) return;

				globalObj.activeGameSockets = [];

				const handler = {
					construct(target, args) {
						const url = args[0] || "";
						const ws = new target(...args);

						const isGameServer = typeof url === 'string' && url.includes("ep-live");
						if (isGameServer) {
							globalObj.activeGameSockets.push(ws);
							if (globalObj.citadelNotify) {
								globalObj.citadelNotify(url);
							}
						}
						return ws;
					}
				};

				globalObj.WebSocket = new Proxy(OriginalWebSocket, handler);
			
				const senderFunc = function(data) {
					if (globalObj.activeGameSockets.length > 0 && globalObj.activeGameSockets[0].readyState === 1) {
						globalObj.activeGameSockets[0].send(data);
					}
				};

				globalObj.sendToGameSocket = senderFunc;

				const disconnectFunc = function() {
					if (globalObj.activeGameSockets.length > 0) {
						for (let i = 0; i < globalObj.activeGameSockets.length; i++) {
							globalObj.activeGameSockets[i].close();
						}
						globalObj.activeGameSockets = [];
					}
				};

				globalObj.disconnectGameSocket = disconnectFunc;
			};

			if (typeof globalThis !== 'undefined') applyHook(globalThis);
			if (typeof window !== 'undefined') applyHook(window);
			if (typeof self !== 'undefined') applyHook(self);

			if (typeof window !== 'undefined' && window.Worker && !window._workerHooked) {
				window._workerHooked = true;
				const NativeWorker = window.Worker;
				window.Worker = function(scriptURL, options) {
					try {
						const absoluteUrl = new URL(scriptURL, document.baseURI).href;
						const hookStr = "(" + applyHook.toString() + ")(self);";
						const blobCode = hookStr + " importScripts('" + absoluteUrl + "');";
						const blob = new Blob([blobCode], { type: 'application/javascript' });
						const blobUrl = URL.createObjectURL(blob);
						return new NativeWorker(blobUrl, options);
					} catch(e) {
						return new NativeWorker(scriptURL, options);
					}
				};
			}

			// Overlay functions
			window.showInitializingOverlay = function() {
				if (!document.body) {
					setTimeout(window.showInitializingOverlay, 100);
					return;
				}
				const div = document.createElement('div');
				div.id = 'citadelOpsOverlay';
				div.style.position = 'fixed';
				div.style.top = '0';
				div.style.left = '0';
				div.style.width = '100vw';
				div.style.height = '100vh';
				div.style.backgroundColor = 'rgba(0, 0, 0, 0.9)';
				div.style.color = 'white';
				div.style.display = 'flex';
				div.style.justifyContent = 'center';
				div.style.alignItems = 'center';
				div.style.zIndex = '999999';
				div.style.fontSize = '24px';
				div.style.fontFamily = 'Arial, sans-serif';
				div.innerText = 'Initializing CitadelOps... Please Wait for Game...';
				document.body.appendChild(div);
			};
			
			window.hideInitializingOverlay = function() {
				const div = document.getElementById('citadelOpsOverlay');
				if (div) {
					div.remove();
				}
			};

			// Failsafe: Remove overlay after 10 seconds regardless of login status
			setTimeout(window.hideInitializingOverlay, 10000);
`

// StartGameBrowser launches ChromeDP and hooks the game socket
func StartGameBrowser(dashboardURL string) {

	if BrowserCancel != nil {
		log.Println("Browser already running")
		return
	}

	// Clean up any stale lock files from previous crashes
	lockFile := filepath.Join("chrome-profile", "SingletonLock")
	os.Remove(lockFile)

	opts := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.Flag("headless", false),
		chromedp.Flag("remote-debugging-port", "9222"),
		chromedp.Flag("start-maximized", false),
		chromedp.Flag("disable-site-isolation-trials", true),
		chromedp.Flag("disable-features", "IsolateOrigins,site-per-process"),
		chromedp.UserDataDir("chrome-profile"),
	)

	allocCtx, allocCancel := chromedp.NewExecAllocator(context.Background(), opts...)

	// Launch browser with the dashboard as the initial tab
	browserCtx, browserCancel := chromedp.NewContext(allocCtx, chromedp.WithLogf(log.Printf))
	if dashboardURL != "" {
		if err := chromedp.Run(browserCtx, chromedp.Navigate(dashboardURL)); err != nil {
			log.Printf("Failed to navigate to dashboard: %v", err)
		}
	} else {
		if err := chromedp.Run(browserCtx); err != nil {
			log.Printf("Failed to initialize browser: %v", err)
		}
	}

	// Create a 2nd tab for the game
	gameCtx, gameCancel := chromedp.NewContext(browserCtx)

	BrowserCtx = gameCtx
	BrowserCancel = func() {
		if gameCancel != nil {
			gameCancel()
		}
		browserCancel()
		allocCancel()
		BrowserCancel = nil
		BrowserCtx = nil
	}

	chromedp.ListenTarget(gameCtx, handleCDPEvent)

	go func() {
		// Reset game state for fresh connection
		Models.GetGameState().Reset()

		err := chromedp.Run(gameCtx,
			chromedp.ActionFunc(func(ctx context.Context) error {
				_, err := page.AddScriptToEvaluateOnNewDocument(gameInjectionJS).Do(ctx)
				return err
			}),
			runtime.AddBinding("citadelNotify"),
			network.Enable(),
			runtime.Enable(), // Explicitly enable Runtime to ensure console events fire
			chromedp.Navigate("https://empire.goodgamestudios.com/"),
			// Wait for body then inject overlay
			chromedp.WaitVisible(`body`, chromedp.ByQuery),
			chromedp.Evaluate(`window.showInitializingOverlay()`, nil),
		)

		if err != nil {
			log.Printf("Chromedp setup error: %v", err)
			StopGame()
			return
		}

		go StartWebsocketChannels(gameCtx)
		go incomingMessageParserStartup()

		<-gameCtx.Done()

		// Cleanup
		LoginStatus = false
		LoginCooldown = 0
		if SendGameLoginStatusFunc != nil {
			SendGameLoginStatusFunc(false, 0)
		}
	}()
}

func StopGame() {
	if BrowserCancel != nil {
		BrowserCancel() // Cancel chromedp context
	}
	LoginStatus = false
	LoginCooldown = 0
	if SendGameLoginStatusFunc != nil {
		SendGameLoginStatusFunc(false, 0)
	}
}

// DisconnectGameWebSocket securely closes out the WebSocket specifically without shutting down the ChromeDP instance
func DisconnectGameWebSocket() {
	if BrowserCtx != nil && gameExecutionContextID != 0 {
		err := chromedp.Run(BrowserCtx, chromedp.ActionFunc(func(ctx context.Context) error {
			exp := "if (window.disconnectGameSocket) window.disconnectGameSocket();"
			_, _, err := runtime.Evaluate(exp).WithContextID(gameExecutionContextID).Do(ctx)
			return err
		}))
		if err != nil {
			log.Println("Failed to disconnect game websocket via CDP:", err)
		}
	}
	// Clear stale WebSocket request IDs so they don't interfere on reconnect
	gameRequestIDs = make(map[string]bool)
	LoginStatus = false
	if SendGameLoginStatusFunc != nil {
		SendGameLoginStatusFunc(false, 0)
	}
}

// ReloadGameTab reloads the game tab to trigger a fresh login without restarting the browser
func ReloadGameTab() {
	if BrowserCtx == nil {
		log.Println("Cannot reload game tab: browser not running")
		return
	}

	// Reset game state for fresh connection
	Models.GetGameState().Reset()
	gameRequestIDs = make(map[string]bool)

	go func() {
		err := chromedp.Run(BrowserCtx,
			chromedp.ActionFunc(func(ctx context.Context) error {
				return page.BringToFront().Do(ctx)
			}),
			chromedp.Navigate("https://empire.goodgamestudios.com/"),
			chromedp.WaitVisible(`body`, chromedp.ByQuery),
			chromedp.Evaluate(`window.showInitializingOverlay()`, nil),
		)
		if err != nil {
			log.Printf("Failed to reload game tab: %v", err)
		}
	}()
}

func StartWebsocketChannels(ctx context.Context) {
	// The read portion is now handled by the JS hook pushing to IncomingMessages

	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case message := <-OutgoingMessages:
				var payload []byte
				cost := 0

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

				allowed := true
				if cost > 0 {
					if !License.UseCredits(cost, "Game Message") {
						allowed = false
					}
				}

				if allowed {
					if BrowserCtx != nil && gameExecutionContextID != 0 {
						// Escape single quotes in payload
						safePayload := strings.ReplaceAll(string(payload), "'", "\\'")
						safePayload = strings.ReplaceAll(safePayload, "\n", "")
						safePayload = strings.ReplaceAll(safePayload, "\r", "")

						err := chromedp.Run(BrowserCtx, chromedp.ActionFunc(func(ctx context.Context) error {
							exp := fmt.Sprintf("window.sendToGameSocket('%s')", safePayload)
							_, _, err := runtime.Evaluate(exp).WithContextID(gameExecutionContextID).Do(ctx)
							return err
						}))

						// We still sleep to preserve rate limiting
						time.Sleep(50 * time.Millisecond)
						if err != nil {
							log.Println("write via CDP:", err)
						}
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
		if BrowserCtx != nil {
			go chromedp.Run(BrowserCtx, chromedp.Evaluate(`window.hideInitializingOverlay()`, nil))
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
		// Auto-reload 5 seconds after cooldown expires
		if LoginCooldown > 0 {
			go func(cd int) {
				waitDuration := time.Duration(cd+5) * time.Second
				log.Printf("[Login] Cooldown %ds detected. Will reload game tab in %v", cd, waitDuration)
				time.Sleep(waitDuration)
				if !LoginStatus {
					log.Println("[Login] Cooldown expired. Reloading game tab...")
					ReloadGameTab()
				}
			}(LoginCooldown)
		}
	}
}
