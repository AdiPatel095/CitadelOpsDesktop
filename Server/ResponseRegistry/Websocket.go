package ResponseRegistry

import (
	"CitadelDesktop/Server/ChromeUserData"
	"CitadelDesktop/Server/Logging"
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

var (
	LoginStatus            bool
	LoginCooldown          int
	BrowserCtx             context.Context
	BrowserCancel          context.CancelFunc
	gameExecutionContextID runtime.ExecutionContextID
	IncomingMessages       = make(chan []string, 100)
	DashboardURL           string

	// SendGameLoginStatusFunc is a callback to notify frontend of login status changes
	SendGameLoginStatusFunc func(bool, int)

	// SendAutoBirdStatusFunc is a callback to notify frontend of auto bird enabled + next wake (unix ms).
	SendAutoBirdStatusFunc func(enabled bool, nextWakeUp int64)

	// SendRecruitTroopsStatusFunc is a callback to notify frontend of recruit troops status changes
	SendRecruitTroopsStatusFunc func(bool)

	// SendAutoToolStatusFunc is a callback to notify frontend of Auto Tool status changes
	SendAutoToolStatusFunc func(bool)

	// SendAutoTCIStatusFunc notifies the frontend when AutoTCI (temporary construction items) is toggled.
	SendAutoTCIStatusFunc func(bool)

	// SendAutoBeriWorldStatusFunc notifies the frontend of Auto Beri World enabled + next pass (unix ms).
	SendAutoBeriWorldStatusFunc func(enabled bool, nextWakeUp int64)

	// BroadcastStaleSnapshot is set from main to push lastKnownGameStateSnapshot after game disconnect (avoids import cycle with FrontendWebsocket).
	BroadcastStaleSnapshot func()

	// GameSessionActiveHandler is called when the game reports logged-in (e.g. lli payload 0).
	GameSessionActiveHandler func()
	// GameSessionInactiveHandler is called whenever LoginStatus becomes false (disconnect, StopGame, etc.).
	GameSessionInactiveHandler func()
	// AfterGameStateReset runs after [Models.GetGameState().Reset] (fresh browser/tab load). Used so AutoTCI can re-fetch **gii**.
	AfterGameStateReset func()

	// EmpireExToken is dynamically parsed from incoming game messages and used for outgoing messages
	EmpireExToken string = "EmpireEx_21"
)

// notifyGameSessionInactive runs the optional inactive handler (e.g. cancel AutoTCI ubc one-shots).
func notifyGameSessionInactive() {
	if GameSessionInactiveHandler != nil {
		GameSessionInactiveHandler()
	}
}

func notifyGameStateReset() {
	if AfterGameStateReset != nil {
		AfterGameStateReset()
	}
}

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

// SetGameLoginStatusCallback sets the callback for game login status notification
func SetGameLoginStatusCallback(fn func(bool, int)) {
	SendGameLoginStatusFunc = fn
}

// SetAutoBirdStatusCallback sets the callback for auto bird status notification.
func SetAutoBirdStatusCallback(fn func(bool, int64)) {
	SendAutoBirdStatusFunc = fn
}

// SetAutoBeriWorldStatusCallback sets the callback for Auto Beri World status notification.
func SetAutoBeriWorldStatusCallback(fn func(bool, int64)) {
	SendAutoBeriWorldStatusFunc = fn
}

func SetMemoryStatsCallback(fn func(int, int)) {
	SendMemoryStatsFunc = fn
}

// SetDashboardURL sets the dashboard URL used for bootstrapping Chrome tabs (and as a fallback when ReloadGameTab
// is called before a browser is running).
func SetDashboardURL(url string) {
	DashboardURL = url
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
			Logging.BeginWebSocketGameLogSession()
		}
	case *network.EventWebSocketClosed:
		if gameRequestIDs[string(ev.RequestID)] {
			log.Println("[WebSocket] Game WebSocket closed (disconnected/kicked)")
			delete(gameRequestIDs, string(ev.RequestID))
			EmpireExToken = "EmpireEx_21"
			LoginStatus = false
			LoginCooldown = 0
			Models.PersistGameStateSnapshot()
			if BroadcastStaleSnapshot != nil {
				BroadcastStaleSnapshot()
			}
			notifyGameSessionInactive()
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
	case *network.EventWebSocketFrameSent:
		if gameRequestIDs[string(ev.RequestID)] {
			payload := ev.Response.PayloadData
			if decoded, err := base64.StdEncoding.DecodeString(payload); err == nil {
				payload = string(decoded)
			}

			// Dynamically capture the EmpireEx version token from native client SEND events
			// Our own bot sends skip this parsing since they format using the existing token anyway
			parts := strings.Split(payload, "%")
			if len(parts) > 2 && strings.HasPrefix(parts[2], "EmpireEx_") {
				if parts[2] != EmpireExToken {
					EmpireExToken = parts[2]
					log.Printf("[WebSocket] Captured dynamic EmpireExToken from outbound frame: %s", EmpireExToken)
				}
			}

			msgType := extractMessageType(payload)
			Logging.AppendChannelLine(Logging.ChannelWebSocketGame, "SEND", msgType, payload)
			if OutboundGameWireSendHook != nil {
				OutboundGameWireSendHook(payload)
			}
		}
	}
}

func extractMessageType(payload string) string {
	op := effectiveWireOpcode(strings.Split(payload, "%"))
	if op != "" {
		return op
	}
	return "UNKNOWN"
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

	appChromeDir, err := ChromeUserData.AppUserDataDir()
	if err != nil {
		log.Printf("Chrome: could not create app profile directory: %v — falling back to chromedp temporary profile", err)
	}

	opts := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.Flag("headless", false),
		chromedp.Flag("remote-debugging-port", "9222"),
		chromedp.Flag("start-maximized", false),
		chromedp.Flag("disable-site-isolation-trials", true),
		chromedp.Flag("disable-features", "IsolateOrigins,site-per-process"),
	)

	if err == nil && appChromeDir != "" {
		for _, lockName := range []string{"SingletonLock", "SingletonSocket", "SingletonCookie"} {
			_ = os.Remove(filepath.Join(appChromeDir, lockName))
		}
		opts = append(opts,
			chromedp.UserDataDir(appChromeDir),
			chromedp.Flag("disable-extensions", false),
			chromedp.Flag("disable-sync", false),
			chromedp.Flag("use-mock-keychain", false),
		)
		log.Printf("Chrome: dedicated app profile at %s", appChromeDir)
	}

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
		// Reset game state for fresh connection (do not persist here: memory may still be empty on
		// first launch and would overwrite a good on-disk snapshot from a previous run).
		Models.GetGameState().Reset()
		notifyGameStateReset()

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
		go StartMemoryMonitor(gameCtx)

		<-gameCtx.Done()

		// Cleanup
		Models.PersistGameStateSnapshot()
		LoginStatus = false
		LoginCooldown = 0
		notifyGameSessionInactive()
		if SendGameLoginStatusFunc != nil {
			SendGameLoginStatusFunc(false, 0)
		}
	}()
}

func StopGame() {
	Models.PersistGameStateSnapshot()
	if BrowserCancel != nil {
		BrowserCancel() // Cancel chromedp context
	}
	LoginStatus = false
	LoginCooldown = 0
	notifyGameSessionInactive()
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
	EmpireExToken = "EmpireEx_21"
	LoginStatus = false
	notifyGameSessionInactive()
	if SendGameLoginStatusFunc != nil {
		SendGameLoginStatusFunc(false, 0)
	}
}

// ReloadGameTab reloads the game tab to trigger a fresh login without restarting the browser
func ReloadGameTab() {
	if BrowserCtx == nil {
		log.Println("Browser not running. Launching browser first...")
		StartGameBrowser(DashboardURL)
		return
	}

	Models.PersistGameStateSnapshot()
	// Reset game state for fresh connection
	Models.GetGameState().Reset()
	notifyGameStateReset()
	gameRequestIDs = make(map[string]bool)
	EmpireExToken = "EmpireEx_21"

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
		log.Printf("[WebSocket] Starting OutgoingMessages loop with %s", EmpireExToken)

		for {
			select {
			case <-ctx.Done():
				return
			case message := <-OutgoingMessages:
				var payload []byte

				switch v := message.(type) {
				case []byte:
					payload = v
				case string:
					payload = []byte(v)
				default:
					log.Printf("Unknown message type in OutgoingMessages: %T", v)
					continue
				}

				if BrowserCtx != nil && gameExecutionContextID != 0 {
					logAppOutboundPayload(string(payload))
					// Escape single quotes in payload
					safePayload := strings.ReplaceAll(string(payload), "'", "\\'")
					safePayload = strings.ReplaceAll(safePayload, "\n", "")
					safePayload = strings.ReplaceAll(safePayload, "\r", "")

					// Run CDP command asynchronously so we don't add CDP overhead to the strict 25ms rate limit
					go func(sp string) {
						err := chromedp.Run(BrowserCtx, chromedp.ActionFunc(func(ctx context.Context) error {
							exp := fmt.Sprintf("window.sendToGameSocket('%s')", sp)
							_, _, err := runtime.Evaluate(exp).WithContextID(gameExecutionContextID).Do(ctx)
							return err
						}))
						if err != nil {
							log.Println("write via CDP:", err)
						}
					}(safePayload)

					// We still sleep to preserve rate limiting
					time.Sleep(25 * time.Millisecond)
				}
			}
		}
	}()
}

// MessageRouterFunc is set by GameParser to route incoming messages.
// This avoids a circular import between ResponseRegistry and GameParser.
var MessageRouterFunc func([]string)

func incomingMessageParserStartup() {
	for message := range IncomingMessages {
		if len(message) < 3 {
			LogIncomingGameWireParts(message)
			continue
		}

		// Dynamically capture the EmpireEx version token (e.g. EmpireEx_21)
		if strings.HasPrefix(message[2], "EmpireEx_") && EmpireExToken != message[2] {
			EmpireExToken = message[2]
			log.Printf("[WebSocket] Captured dynamic EmpireExToken: %s", EmpireExToken)
		}

		if message[2] == "lli" {
			checkLoginStatus(message)
		}
		if MessageRouterFunc != nil {
			MessageRouterFunc(message)
		}
	}
}

func checkLoginStatus(message []string) {
	if len(message) <= 4 {
		return
	}
	if message[4] == "0" {
		LoginStatus = true
		LoginCooldown = 0
		if GameSessionActiveHandler != nil {
			go GameSessionActiveHandler()
		}
		if SendGameLoginStatusFunc != nil {
			go SendGameLoginStatusFunc(LoginStatus, LoginCooldown)
		}
		if BrowserCtx != nil {
			go chromedp.Run(BrowserCtx, chromedp.Evaluate(`window.hideInitializingOverlay()`, nil))
		}
	}
	if message[4] == "453" {
		if len(message) <= 5 {
			return
		}
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
