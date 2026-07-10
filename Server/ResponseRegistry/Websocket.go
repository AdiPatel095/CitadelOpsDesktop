package ResponseRegistry

import (
	"CitadelDesktop/Server/ChromeUserData"
	"CitadelDesktop/Server/GameFocus"
	"CitadelDesktop/Server/Logging"
	"CitadelDesktop/Server/Models"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/cdproto/page"
	"github.com/chromedp/cdproto/runtime"
	"github.com/chromedp/chromedp"
)

var (
	browserLifecycleMu     sync.RWMutex
	BrowserCtx             context.Context
	BrowserCancel          context.CancelFunc
	gameExecutionContextID runtime.ExecutionContextID
	activeBrowserSession   uint64
	IncomingMessages       = make(chan []string, 100)
	DashboardURL           string

	// SendAutoBirdStatusFunc is a callback to notify frontend of auto bird enabled + next wake (unix ms).
	SendAutoBirdStatusFunc func(enabled bool, nextWakeUp int64)

	// SendRecruitTroopsStatusFunc is a callback to notify frontend of recruit troops status changes
	SendRecruitTroopsStatusFunc func(bool)

	// SendAutoToolStatusFunc is a callback to notify frontend of Auto Tool status changes
	SendAutoToolStatusFunc func(bool)

	// SendAutoHospitalStatusFunc is a callback to notify frontend of Auto Hospital status changes
	SendAutoHospitalStatusFunc func(bool)

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

var (
	incomingMessageQueueMu    sync.Mutex
	incomingMessageQueueCond  = sync.NewCond(&incomingMessageQueueMu)
	incomingMessageQueue      [][]string
	incomingMessageParserOnce sync.Once
)

func enqueueIncomingMessage(message []string) {
	incomingMessageQueueMu.Lock()
	incomingMessageQueue = append(incomingMessageQueue, message)
	incomingMessageQueueCond.Signal()
	incomingMessageQueueMu.Unlock()
}

func dispatchIncomingMessages() {
	for {
		incomingMessageQueueMu.Lock()
		for len(incomingMessageQueue) == 0 {
			incomingMessageQueueCond.Wait()
		}
		message := incomingMessageQueue[0]
		incomingMessageQueue[0] = nil
		incomingMessageQueue = incomingMessageQueue[1:]
		if len(incomingMessageQueue) == 0 {
			incomingMessageQueue = nil
		}
		incomingMessageQueueMu.Unlock()
		IncomingMessages <- message
	}
}

func startIncomingMessageParser() {
	incomingMessageParserOnce.Do(func() {
		go dispatchIncomingMessages()
		go incomingMessageParserStartup()
	})
}

func browserStateSnapshot() (context.Context, context.CancelFunc, runtime.ExecutionContextID, uint64) {
	browserLifecycleMu.RLock()
	ctx := BrowserCtx
	cancel := BrowserCancel
	executionContextID := gameExecutionContextID
	browserSession := activeBrowserSession
	browserLifecycleMu.RUnlock()
	return ctx, cancel, executionContextID, browserSession
}

func installBrowserSession(
	browserSession uint64,
	ctx context.Context,
	cancel context.CancelFunc,
) {
	browserLifecycleMu.Lock()
	activeBrowserSession = browserSession
	BrowserCtx = ctx
	BrowserCancel = cancel
	gameExecutionContextID = 0
	browserLifecycleMu.Unlock()
}

func clearBrowserSession(browserSession uint64) {
	browserLifecycleMu.Lock()
	if activeBrowserSession == browserSession {
		activeBrowserSession = 0
		BrowserCtx = nil
		BrowserCancel = nil
		gameExecutionContextID = 0
	}
	browserLifecycleMu.Unlock()
}

func clearGameExecutionContext(browserSession uint64) {
	browserLifecycleMu.Lock()
	if activeBrowserSession == browserSession {
		gameExecutionContextID = 0
	}
	browserLifecycleMu.Unlock()
}

func isActiveBrowserSession(browserSession uint64) bool {
	browserLifecycleMu.RLock()
	active := activeBrowserSession == browserSession
	browserLifecycleMu.RUnlock()
	return active
}

func setGameExecutionContext(browserSession uint64, executionContextID runtime.ExecutionContextID) bool {
	browserLifecycleMu.Lock()
	defer browserLifecycleMu.Unlock()
	if activeBrowserSession != browserSession {
		return false
	}
	gameExecutionContextID = executionContextID
	return true
}

// IsGameWebSocketReady reports whether automation can safely send game commands.
// It intentionally does not launch, reload, or otherwise reconnect the game tab.
func IsGameWebSocketReady() bool {
	status := GetGameConnectionStatus()
	ctx, _, executionContextID, _ := browserStateSnapshot()
	return status.LoggedIn && hasTrackedOpenGameSocket() && ctx != nil && executionContextID != 0
}

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

func handleCDPEvent(browserSession uint64, ev interface{}) {
	if !isActiveBrowserSession(browserSession) {
		return
	}

	switch ev := ev.(type) {
	case *runtime.EventBindingCalled:
		if ev.Name == "citadelNotify" {
			if !handleCitadelNotifyPayload(ev.Payload) {
				if setGameExecutionContext(browserSession, ev.ExecutionContextID) &&
					GetGameConnectionStatus().State == GameConnectionStopped {
					ctx, _, _, currentBrowserSession := browserStateSnapshot()
					go func(executionContextID runtime.ExecutionContextID) {
						if ctx == nil || currentBrowserSession != browserSession ||
							GetGameConnectionStatus().State != GameConnectionStopped {
							return
						}
						if _, err := disconnectGameSocketInContext(ctx, executionContextID); err != nil {
							log.Printf("[WebSocket] Stop pending socket: %v", err)
						}
						clearGameExecutionContext(browserSession)
					}(ev.ExecutionContextID)
				}
			}
		}
	case *network.EventWebSocketCreated:
		if strings.Contains(ev.URL, "ep-live") {
			if trackGameRequestIDURL(browserSession, string(ev.RequestID), ev.URL) {
				if !GetGameConnectionStatus().LoggedIn {
					Models.GetGameState().Movement.InvalidateSnapshot()
				}
				Logging.BeginWebSocketGameLogSession()
			}
		}
	case *network.EventWebSocketHandshakeResponseReceived:
		requestID := string(ev.RequestID)
		if !isTrackedGameRequestID(browserSession, requestID) {
			break
		}
		if ev.Response != nil && ev.Response.Status == 101 {
			markGameSocketOpen(browserSession, requestID)
		} else {
			status := int64(0)
			if ev.Response != nil {
				status = ev.Response.Status
			}
			detail := fmt.Sprintf("Game WebSocket handshake failed (HTTP %d)", status)
			if markGameSocketFailure(browserSession, requestID, detail) {
				log.Printf("[WebSocket] %s", detail)
				Models.GetGameState().Movement.InvalidateSnapshot()
			}
		}
	case *network.EventWebSocketClosed:
		if tracked, connectionLost := closeTrackedGameSocket(browserSession, string(ev.RequestID)); tracked {
			log.Println("[WebSocket] Game WebSocket closed (disconnected/kicked)")
			if connectionLost {
				EmpireExToken = "EmpireEx_21"
				Models.GetGameState().Movement.InvalidateSnapshot()
				Models.PersistGameStateSnapshot()
				if BroadcastStaleSnapshot != nil {
					BroadcastStaleSnapshot()
				}
				notifyGameSessionInactive()
			}
		}
	case *network.EventWebSocketFrameError:
		requestID := string(ev.RequestID)
		if markGameSocketFailure(browserSession, requestID, ev.ErrorMessage) {
			log.Printf("[WebSocket] Game WebSocket frame error: %s", ev.ErrorMessage)
			Models.GetGameState().Movement.InvalidateSnapshot()
			Models.PersistGameStateSnapshot()
			if BroadcastStaleSnapshot != nil {
				BroadcastStaleSnapshot()
			}
			notifyGameSessionInactive()
		}
	case *network.EventWebSocketFrameReceived:
		requestID := string(ev.RequestID)
		if markGameSocketOpen(browserSession, requestID) {
			payload := ev.Response.PayloadData
			if decoded, err := base64.StdEncoding.DecodeString(payload); err == nil {
				payload = string(decoded)
			}
			messageParts := strings.Split(payload, "%")
			if len(messageParts) > 2 && messageParts[2] == "lli" {
				checkLoginStatus(browserSession, requestID, messageParts)
			}
			enqueueIncomingMessage(messageParts)
		}
	case *network.EventWebSocketFrameSent:
		if isTrackedGameRequestID(browserSession, string(ev.RequestID)) {
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

func handleCitadelNotifyPayload(payload string) bool {
	var msg struct {
		Type string `json:"type"`
		Kind string `json:"kind"`
	}
	if err := json.Unmarshal([]byte(payload), &msg); err != nil || msg.Type != "gameActivity" {
		return false
	}
	st := Models.GetSettingsState()
	GameFocus.RecordManualActivity(msg.Kind, st.ManualFocusIdleDuration())
	return true
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
				globalObj.authenticatedGameSocket = null;

				const handler = {
					construct(target, args) {
						const url = args[0] || "";
						const ws = new target(...args);

							const isGameServer = typeof url === 'string' && url.includes("ep-live");
						if (isGameServer) {
							globalObj.activeGameSockets.push(ws);
							ws.addEventListener("message", function(event) {
								if (typeof event.data !== "string") return;
								const parts = event.data.split("%");
								if (parts.length > 4 && parts[2] === "lli") {
									if (parts[4] === "0") {
										globalObj.authenticatedGameSocket = ws;
									} else if (globalObj.authenticatedGameSocket === ws) {
										globalObj.authenticatedGameSocket = null;
									}
								}
							});
							ws.addEventListener("close", function() {
								if (globalObj.authenticatedGameSocket === ws) {
									globalObj.authenticatedGameSocket = null;
								}
									globalObj.activeGameSockets = globalObj.activeGameSockets.filter(function(socket) {
										return socket !== ws;
									});
								}, { once: true });
								if (globalObj.citadelNotify) {
									globalObj.citadelNotify(url);
							}
						}
						return ws;
					}
				};

				globalObj.WebSocket = new Proxy(OriginalWebSocket, handler);
			
					const senderFunc = function(data) {
						const authenticated = globalObj.authenticatedGameSocket;
						if (authenticated && authenticated.readyState === 1) {
							authenticated.send(data);
							return true;
						}
						for (let i = 0; i < globalObj.activeGameSockets.length; i++) {
							const socket = globalObj.activeGameSockets[i];
							if (socket && socket.readyState === 1) {
								socket.send(data);
								return true;
							}
						}
						return false;
					};

				globalObj.sendToGameSocket = senderFunc;

					const disconnectFunc = function() {
						const count = globalObj.activeGameSockets.length;
						if (globalObj.activeGameSockets.length > 0) {
							const sockets = globalObj.activeGameSockets.slice();
							globalObj.activeGameSockets = [];
							for (let i = 0; i < sockets.length; i++) {
								sockets[i].close();
							}
						}
						globalObj.authenticatedGameSocket = null;
						return count;
				};

					globalObj.disconnectGameSocket = disconnectFunc;

					if (globalObj.document && !globalObj._citadelActivityHooked) {
						globalObj._citadelActivityHooked = true;
						let lastMoveNotify = 0;
						let lastAnyNotify = 0;
						const notifyActivity = function(kind) {
							const now = Date.now();
							if (kind === "pointermove" && now - lastMoveNotify < 1500) return;
							if (kind !== "pointermove" && now - lastAnyNotify < 250) return;
							if (kind === "pointermove") lastMoveNotify = now;
							lastAnyNotify = now;
							if (globalObj.citadelNotify) {
								globalObj.citadelNotify(JSON.stringify({ type: "gameActivity", kind: kind }));
							}
						};
						const opts = { capture: true, passive: true };
						globalObj.document.addEventListener("pointerdown", () => notifyActivity("pointerdown"), opts);
						globalObj.document.addEventListener("pointermove", () => notifyActivity("pointermove"), opts);
						globalObj.document.addEventListener("wheel", () => notifyActivity("wheel"), opts);
						globalObj.document.addEventListener("keydown", () => notifyActivity("keydown"), true);
						globalObj.addEventListener("focus", () => notifyActivity("focus"), true);
					}
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

	_, currentCancel, _, _ := browserStateSnapshot()
	if currentCancel != nil {
		log.Println("Browser already running")
		return
	}
	startGeneration, browserSession, started := beginGameBrowserStart()
	if !started {
		log.Println("Browser start already in progress")
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
	if !isCurrentGameBrowserStart(startGeneration) {
		browserCancel()
		allocCancel()
		return
	}

	// Create a 2nd tab for the game
	gameCtx, gameCancel := chromedp.NewContext(browserCtx)

	cancelThisBrowser := func() {
		if gameCancel != nil {
			gameCancel()
		}
		browserCancel()
		allocCancel()
		clearBrowserSession(browserSession)
	}
	installBrowserSession(browserSession, gameCtx, cancelThisBrowser)
	if !isCurrentGameBrowserStart(startGeneration) {
		cancelThisBrowser()
		return
	}

	chromedp.ListenTarget(gameCtx, func(ev interface{}) {
		handleCDPEvent(browserSession, ev)
	})

	go func() {
		// Reset game state for fresh connection (do not persist here: memory may still be empty on
		// first launch and would overwrite a good on-disk snapshot from a previous run).
		Models.GetGameState().Reset()
		notifyGameStateReset()
		startIncomingMessageParser()

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
			if markGameConnectionError(startGeneration, false, "Could not start the game browser") {
				cancelThisBrowser()
				notifyGameSessionInactive()
			}
			return
		}

		go StartWebsocketChannels(gameCtx, browserSession)
		go StartMemoryMonitor(gameCtx)

		<-gameCtx.Done()

		// Cleanup
		Models.GetGameState().Movement.InvalidateSnapshot()
		Models.PersistGameStateSnapshot()
		cancelThisBrowser()
		if markGameContextEnded(browserSession) {
			notifyGameSessionInactive()
		}
	}()
}

func StopGame() {
	Models.GetGameState().Movement.InvalidateSnapshot()
	Models.PersistGameStateSnapshot()
	markGameConnectionStopped(false)
	_, cancel, _, _ := browserStateSnapshot()
	if cancel != nil {
		cancel() // Cancel chromedp context
	}
	notifyGameSessionInactive()
}

// DisconnectGameWebSocket securely closes out the WebSocket specifically without shutting down the ChromeDP instance
func disconnectGameSocketInContext(ctx context.Context, executionContextID runtime.ExecutionContextID) (int, error) {
	closed := -1
	err := chromedp.Run(ctx, chromedp.ActionFunc(func(ctx context.Context) error {
		expression := `(() => {
			if (typeof globalThis.disconnectGameSocket !== "function") return -1;
			return globalThis.disconnectGameSocket();
		})()`
		result, exception, evalErr := runtime.Evaluate(expression).
			WithContextID(executionContextID).
			WithReturnByValue(true).
			Do(ctx)
		if evalErr != nil {
			return evalErr
		}
		if exception != nil {
			return exception
		}
		if result == nil || len(result.Value) == 0 {
			return fmt.Errorf("game socket disconnect returned no result")
		}
		if err := json.Unmarshal(result.Value, &closed); err != nil {
			return fmt.Errorf("decode game socket disconnect result: %w", err)
		}
		if closed < 0 {
			return fmt.Errorf("disconnectGameSocket is unavailable in the game execution context")
		}
		return nil
	}))
	return closed, err
}

func DisconnectGameWebSocket() {
	ctx, _, executionContextID, browserSession := browserStateSnapshot()
	status := GetGameConnectionStatus()
	if ctx == nil || executionContextID == 0 {
		if status.LoggedIn || status.SocketConnected {
			log.Println("Failed to disconnect game websocket: game execution context is not ready")
			return
		}
		markGameConnectionStopped(ctx != nil)
		Models.GetGameState().Movement.InvalidateSnapshot()
		notifyGameSessionInactive()
		return
	}

	stopGeneration := beginGameConnectionStop()
	closed, err := disconnectGameSocketInContext(ctx, executionContextID)
	if err != nil || (closed == 0 && (status.LoggedIn || status.SocketConnected)) {
		finishGameConnectionStop(stopGeneration, ctx.Err() == nil, false)
		if err == nil {
			err = fmt.Errorf("game execution context reported no active game sockets")
		}
		log.Println("Failed to disconnect game websocket via CDP:", err)
		if status.State == GameConnectionCooldown && status.RetryAt > 0 {
			scheduleGameLoginRetry(stopGeneration, status.RetryAt, status.Cooldown)
		}
		return
	}
	if !finishGameConnectionStop(stopGeneration, ctx.Err() == nil, true) {
		return
	}
	Models.GetGameState().Movement.InvalidateSnapshot()
	clearGameExecutionContext(browserSession)
	EmpireExToken = "EmpireEx_21"
	notifyGameSessionInactive()
}

// ReloadGameTab reloads the game tab to trigger a fresh login without restarting the browser
func ReloadGameTab() {
	ctx, cancel, _, browserSession := browserStateSnapshot()
	if ctx == nil || ctx.Err() != nil {
		log.Println("Browser not running. Launching browser first...")
		if cancel != nil {
			cancel()
		}
		StartGameBrowser(DashboardURL)
		return
	}

	Models.PersistGameStateSnapshot()
	wasLoggedIn := GetGameConnectionStatus().LoggedIn
	reloadGeneration := beginGameConnectionAttempt(GameConnectionReconnecting, true)
	clearGameExecutionContext(browserSession)
	if wasLoggedIn {
		notifyGameSessionInactive()
	}
	// Reset game state for fresh connection
	Models.GetGameState().Reset()
	notifyGameStateReset()
	EmpireExToken = "EmpireEx_21"

	go func() {
		err := chromedp.Run(ctx,
			chromedp.ActionFunc(func(ctx context.Context) error {
				return page.BringToFront().Do(ctx)
			}),
			chromedp.Navigate("https://empire.goodgamestudios.com/"),
			chromedp.WaitVisible(`body`, chromedp.ByQuery),
			chromedp.Evaluate(`window.showInitializingOverlay()`, nil),
		)
		if err != nil {
			log.Printf("Failed to reload game tab: %v", err)
			markGameConnectionError(reloadGeneration, ctx.Err() == nil, "Could not reload the game tab")
		}
	}()
}

func StartWebsocketChannels(ctx context.Context, browserSession uint64) {
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

				browserCtx, _, executionContextID, currentBrowserSession := browserStateSnapshot()
				if browserCtx == nil || executionContextID == 0 || currentBrowserSession != browserSession {
					log.Println("write via CDP: game execution context is not ready")
					continue
				}

				payloadLiteral, err := json.Marshal(string(payload))
				if err != nil {
					log.Println("write via CDP: encode payload:", err)
					continue
				}
				expression := fmt.Sprintf("globalThis.sendToGameSocket && globalThis.sendToGameSocket(%s)", payloadLiteral)
				sent := false
				err = chromedp.Run(browserCtx, chromedp.ActionFunc(func(ctx context.Context) error {
					result, exception, evalErr := runtime.Evaluate(expression).
						WithContextID(executionContextID).
						WithReturnByValue(true).
						Do(ctx)
					if evalErr != nil {
						return evalErr
					}
					if exception != nil {
						return fmt.Errorf("game socket send exception: %s", exception.Text)
					}
					sent = result != nil && string(result.Value) == "true"
					return nil
				}))
				if err != nil {
					log.Println("write via CDP:", err)
				} else if !sent {
					log.Println("write via CDP: no open game websocket")
				} else {
					logAppOutboundPayload(string(payload))
				}
				time.Sleep(25 * time.Millisecond)
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

		if MessageRouterFunc != nil {
			MessageRouterFunc(message)
		}
	}
}

func scheduleGameLoginRetry(generation uint64, retryAt int64, cooldown int) {
	go func() {
		waitDuration := time.Until(time.UnixMilli(retryAt))
		log.Printf("[Login] Cooldown %ds detected. Will reload game tab in %v", cooldown, waitDuration.Round(time.Second))
		if waitDuration > 0 {
			timer := time.NewTimer(waitDuration)
			defer timer.Stop()
			<-timer.C
		}
		if isCurrentGameLoginRetry(generation, retryAt) {
			log.Println("[Login] Cooldown expired. Reloading game tab...")
			ReloadGameTab()
		}
	}()
}

func checkLoginStatus(browserSession uint64, requestID string, message []string) {
	if len(message) <= 4 {
		return
	}
	if message[4] == "0" {
		if !markGameLoginSucceeded(browserSession, requestID) {
			return
		}
		if GameSessionActiveHandler != nil {
			go GameSessionActiveHandler()
		}
		ctx, _, _, _ := browserStateSnapshot()
		if ctx != nil {
			go chromedp.Run(ctx, chromedp.Evaluate(`window.hideInitializingOverlay()`, nil))
		}
		return
	}
	if message[4] == "453" {
		if len(message) <= 5 {
			return
		}
		var payload struct {
			Cooldown int `json:"CD"`
		}
		if err := json.Unmarshal([]byte(message[5]), &payload); err != nil {
			log.Printf("[Login] Could not parse cooldown payload: %v", err)
			return
		}
		cooldown := payload.Cooldown
		generation, retryAt, ok := markGameLoginCooldown(browserSession, requestID, cooldown)
		if !ok {
			return
		}
		Models.GetGameState().Movement.InvalidateSnapshot()
		// Auto-reload 5 seconds after cooldown expires
		scheduleGameLoginRetry(generation, retryAt, cooldown)
	}
}
