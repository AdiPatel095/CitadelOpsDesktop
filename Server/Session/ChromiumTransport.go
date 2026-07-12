package Session

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"CitadelDesktop/Server/Protocol"
	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/cdproto/page"
	"github.com/chromedp/cdproto/runtime"
	"github.com/chromedp/chromedp"
)

const defaultGameURL = "https://empire.goodgamestudios.com/"

type ChromiumConfig struct {
	DataDir        string
	DashboardURL   string
	GameURL        string
	Headless       bool
	Browser        string
	ExecutablePath string
}

type ChromiumTransport struct {
	config     ChromiumConfig
	browser    BrowserCandidate
	resolveErr error
	frames     chan RawFrame
	statuses   chan Status
	activities chan Activity

	mu                 sync.RWMutex
	status             Status
	gameContext        context.Context
	cancel             context.CancelFunc
	generation         uint64
	executionContextID runtime.ExecutionContextID
	trackedSockets     map[network.RequestID]string

	sendMu   sync.Mutex
	lastSend time.Time
}

func NewChromiumTransport(config ChromiumConfig) *ChromiumTransport {
	if config.GameURL == "" {
		config.GameURL = defaultGameURL
	}
	preference := strings.TrimSpace(config.Browser)
	if preference == "" && strings.TrimSpace(config.ExecutablePath) == "" && strings.TrimSpace(os.Getenv("CITADEL_BROWSER")) == "" && strings.TrimSpace(os.Getenv("CITADEL_BROWSER_PATH")) == "" {
		preference = loadBrowserPreference(config.DataDir)
	}
	browser, resolveErr := ResolveChromiumBrowser(preference, config.ExecutablePath)
	state := "stopped"
	detail := ""
	if resolveErr != nil {
		state = "unavailable"
		detail = resolveErr.Error()
	}
	return &ChromiumTransport{
		config: config, browser: browser, resolveErr: resolveErr,
		frames: make(chan RawFrame, 8192), statuses: make(chan Status, 32), activities: make(chan Activity, 64),
		status: Status{
			State: state, Namespace: "EmpireEx_21", Detail: detail,
			BrowserID: browser.ID, BrowserName: browser.Name, ChangedAt: time.Now().UTC(),
		},
		trackedSockets: map[network.RequestID]string{},
	}
}

func (transport *ChromiumTransport) Start(ctx context.Context) error {
	transport.mu.Lock()
	if transport.resolveErr != nil {
		err := transport.resolveErr
		transport.mu.Unlock()
		return err
	}
	if transport.cancel != nil {
		transport.mu.Unlock()
		return nil
	}
	browser := transport.browser
	transport.generation++
	generation := transport.generation
	transport.mu.Unlock()
	transport.publishStatus(Status{State: "starting", Namespace: "EmpireEx_21", ChangedAt: time.Now().UTC()})

	if _, err := browserCandidateAtOrError(browser.ID, browser.Name, browser.ExecutablePath); err != nil {
		transport.publishStatus(Status{State: "error", Detail: err.Error(), ChangedAt: time.Now().UTC()})
		return fmt.Errorf("open %s: %w", browser.Name, err)
	}
	profileDir := filepath.Join(transport.config.DataDir, "Browser", browser.ID, "Profile")
	if err := os.MkdirAll(profileDir, 0o700); err != nil {
		transport.publishStatus(Status{State: "error", Detail: err.Error(), ChangedAt: time.Now().UTC()})
		return fmt.Errorf("create %s profile: %w", browser.Name, err)
	}
	removeStaleChromiumLocks(profileDir)
	opts := append([]chromedp.ExecAllocatorOption{}, chromedp.DefaultExecAllocatorOptions[:]...)
	opts = append(opts,
		chromedp.ExecPath(browser.ExecutablePath),
		chromedp.Flag("headless", transport.config.Headless),
		chromedp.Flag("disable-site-isolation-trials", true),
		chromedp.Flag("disable-features", "IsolateOrigins,site-per-process"),
		chromedp.UserDataDir(profileDir),
	)
	allocatorContext, allocatorCancel := chromedp.NewExecAllocator(ctx, opts...)
	browserContext, browserCancel := chromedp.NewContext(allocatorContext)
	cleanup := func() {
		browserCancel()
		allocatorCancel()
	}
	if transport.config.DashboardURL != "" {
		if err := chromedp.Run(browserContext, chromedp.Navigate(transport.config.DashboardURL)); err != nil {
			cleanup()
			transport.publishStatus(Status{State: "error", Detail: err.Error(), ChangedAt: time.Now().UTC()})
			return fmt.Errorf("open CitadelOps dashboard: %w", err)
		}
	} else if err := chromedp.Run(browserContext); err != nil {
		cleanup()
		transport.publishStatus(Status{State: "error", Detail: err.Error(), ChangedAt: time.Now().UTC()})
		return fmt.Errorf("start %s: %w", browser.Name, err)
	}

	gameContext, gameCancel := chromedp.NewContext(browserContext)
	cancelAll := func() {
		gameCancel()
		cleanup()
	}
	transport.mu.Lock()
	if transport.generation != generation || transport.cancel != nil {
		transport.mu.Unlock()
		cancelAll()
		return fmt.Errorf("Chromium session start was superseded")
	}
	transport.gameContext = gameContext
	transport.cancel = cancelAll
	transport.executionContextID = 0
	transport.trackedSockets = map[network.RequestID]string{}
	transport.mu.Unlock()

	chromedp.ListenTarget(gameContext, func(event any) {
		transport.handleEvent(generation, event)
	})
	setupErr := chromedp.Run(gameContext,
		chromedp.ActionFunc(func(ctx context.Context) error {
			_, err := page.AddScriptToEvaluateOnNewDocument(chromiumTransportInjection).Do(ctx)
			return err
		}),
		runtime.AddBinding("citadelTransportNotify"),
		network.Enable(),
		runtime.Enable(),
		chromedp.Navigate(transport.config.GameURL),
	)
	if setupErr != nil {
		transport.clearRun(generation)
		cancelAll()
		transport.publishStatus(Status{
			State: "error", Namespace: "EmpireEx_21", Detail: setupErr.Error(), ChangedAt: time.Now().UTC(),
		})
		return fmt.Errorf("open game: %w", setupErr)
	}
	transport.publishStatus(Status{State: "connecting", Namespace: "EmpireEx_21", ChangedAt: time.Now().UTC()})
	go func() {
		<-gameContext.Done()
		if transport.clearRun(generation) {
			transport.publishStatus(Status{State: "disconnected", Namespace: "EmpireEx_21", ChangedAt: time.Now().UTC()})
		}
	}()
	return nil
}

func (transport *ChromiumTransport) Stop(context.Context) error {
	transport.mu.Lock()
	cancel := transport.cancel
	transport.cancel = nil
	transport.gameContext = nil
	transport.executionContextID = 0
	transport.trackedSockets = map[network.RequestID]string{}
	transport.generation++
	transport.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	transport.publishStatus(Status{State: "stopped", Namespace: "EmpireEx_21", ChangedAt: time.Now().UTC()})
	return nil
}

func (transport *ChromiumTransport) Send(ctx context.Context, payload []byte) error {
	transport.sendMu.Lock()
	defer transport.sendMu.Unlock()
	if err := transport.waitForSendPacing(ctx); err != nil {
		return err
	}
	transport.mu.RLock()
	gameContext := transport.gameContext
	executionContextID := transport.executionContextID
	ready := transport.status.LoggedIn && transport.status.SocketReady
	transport.mu.RUnlock()
	if gameContext == nil || !ready {
		return fmt.Errorf("game websocket is not ready")
	}
	literal, err := json.Marshal(string(payload))
	if err != nil {
		return err
	}
	expression := fmt.Sprintf("globalThis.__citadelSend && globalThis.__citadelSend(%s)", literal)
	sent := false
	err = chromedp.Run(gameContext, chromedp.ActionFunc(func(ctx context.Context) error {
		evaluation := runtime.Evaluate(expression).WithReturnByValue(true)
		if executionContextID != 0 {
			evaluation = evaluation.WithContextID(executionContextID)
		}
		result, exception, evaluateErr := evaluation.Do(ctx)
		if evaluateErr != nil {
			return evaluateErr
		}
		if exception != nil {
			return fmt.Errorf("game websocket send: %s", exception.Text)
		}
		if result != nil {
			_ = json.Unmarshal(result.Value, &sent)
		}
		return nil
	}))
	transport.lastSend = time.Now()
	if err != nil {
		return err
	}
	if !sent {
		return fmt.Errorf("game websocket rejected the payload")
	}
	return nil
}

func (transport *ChromiumTransport) Frames() <-chan RawFrame {
	return transport.frames
}

func (transport *ChromiumTransport) StatusChanges() <-chan Status {
	return transport.statuses
}

func (transport *ChromiumTransport) Activities() <-chan Activity {
	return transport.activities
}

func (transport *ChromiumTransport) Status() Status {
	transport.mu.RLock()
	defer transport.mu.RUnlock()
	return transport.status
}

func (transport *ChromiumTransport) SelectBrowser(preference string) error {
	candidate, err := ResolveChromiumBrowser(preference, "")
	if err != nil {
		return err
	}
	transport.mu.Lock()
	if transport.cancel != nil {
		transport.mu.Unlock()
		return fmt.Errorf("stop the active game session before changing browsers")
	}
	if err := saveBrowserPreference(transport.config.DataDir, candidate); err != nil {
		transport.mu.Unlock()
		return err
	}
	transport.browser = candidate
	transport.resolveErr = nil
	transport.config.Browser = candidate.ID
	transport.config.ExecutablePath = candidate.ExecutablePath
	status := transport.status
	status.State = "stopped"
	status.LoggedIn = false
	status.SocketReady = false
	status.BrowserID = candidate.ID
	status.BrowserName = candidate.Name
	status.Detail = ""
	status.ChangedAt = time.Now().UTC()
	transport.mu.Unlock()
	transport.publishStatus(status)
	return nil
}

func (transport *ChromiumTransport) handleEvent(generation uint64, event any) {
	if !transport.isCurrent(generation) {
		return
	}
	switch typed := event.(type) {
	case *runtime.EventBindingCalled:
		if typed.Name != "citadelTransportNotify" {
			return
		}
		var notice struct {
			Type string `json:"type"`
			URL  string `json:"url"`
			Kind string `json:"kind"`
		}
		if json.Unmarshal([]byte(typed.Payload), &notice) != nil {
			return
		}
		switch notice.Type {
		case "socket":
			if !isGameSocketURL(notice.URL) {
				return
			}
			transport.mu.Lock()
			if transport.generation == generation {
				transport.executionContextID = typed.ExecutionContextID
			}
			transport.mu.Unlock()
		case "activity":
			notice.Kind = strings.TrimSpace(notice.Kind)
			if notice.Kind == "" {
				return
			}
			select {
			case transport.activities <- Activity{Kind: notice.Kind, ObservedAt: time.Now().UTC()}:
			default:
			}
		}
	case *network.EventWebSocketCreated:
		if !isGameSocketURL(typed.URL) {
			return
		}
		transport.mu.Lock()
		transport.trackedSockets[typed.RequestID] = typed.URL
		transport.mu.Unlock()
		transport.publishStatus(Status{
			State: "connecting", ServerURL: typed.URL, Namespace: transport.namespace(), ChangedAt: time.Now().UTC(),
		})
	case *network.EventWebSocketHandshakeResponseReceived:
		if !transport.isTracked(typed.RequestID) {
			return
		}
		if typed.Response != nil && typed.Response.Status == httpSwitchingProtocols {
			status := transport.Status()
			status.State = "authenticating"
			status.SocketReady = true
			status.ChangedAt = time.Now().UTC()
			transport.publishStatus(status)
		}
	case *network.EventWebSocketFrameReceived:
		if !transport.isTracked(typed.RequestID) || typed.Response == nil {
			return
		}
		payload, err := decodeWebSocketPayload(typed.Response)
		if err != nil {
			return
		}
		transport.observeLoginFrame(generation, payload)
		transport.frames <- RawFrame{Payload: payload, Direction: Protocol.DirectionInbound, ObservedAt: time.Now().UTC()}
	case *network.EventWebSocketFrameSent:
		if !transport.isTracked(typed.RequestID) || typed.Response == nil {
			return
		}
		payload, err := decodeWebSocketPayload(typed.Response)
		if err != nil {
			return
		}
		transport.frames <- RawFrame{Payload: payload, Direction: Protocol.DirectionOutbound, ObservedAt: time.Now().UTC()}
	case *network.EventWebSocketClosed:
		if !transport.removeTracked(typed.RequestID) {
			return
		}
		status := transport.Status()
		status.State = "disconnected"
		status.LoggedIn = false
		status.SocketReady = false
		status.Detail = "Game websocket closed"
		status.ChangedAt = time.Now().UTC()
		transport.publishStatus(status)
	case *network.EventWebSocketFrameError:
		if !transport.isTracked(typed.RequestID) {
			return
		}
		status := transport.Status()
		status.State = "error"
		status.LoggedIn = false
		status.SocketReady = false
		status.Detail = typed.ErrorMessage
		status.ChangedAt = time.Now().UTC()
		transport.publishStatus(status)
	}
}

func (transport *ChromiumTransport) observeLoginFrame(generation uint64, payload string) {
	frame, err := Protocol.Decode(payload, Protocol.DirectionInbound, time.Now().UTC())
	if err != nil || frame.Opcode != "lli" || frame.ResponseCode == nil {
		return
	}
	status := transport.Status()
	status.ChangedAt = time.Now().UTC()
	switch *frame.ResponseCode {
	case 0:
		status.State = "connected"
		status.LoggedIn = true
		status.SocketReady = true
		status.Detail = ""
		status.CooldownUntil = nil
		status.RetryAt = nil
		transport.publishStatus(status)
	case 453:
		var cooldown struct {
			Seconds int `json:"CD"`
		}
		_ = json.Unmarshal(frame.Payload, &cooldown)
		now := time.Now().UTC()
		cooldownUntil := now.Add(time.Duration(max(0, cooldown.Seconds)) * time.Second)
		retryAt := cooldownUntil.Add(5 * time.Second)
		status.State = "cooldown"
		status.LoggedIn = false
		status.Detail = fmt.Sprintf("Login cooldown: %ds", cooldown.Seconds)
		status.CooldownUntil = &cooldownUntil
		status.RetryAt = &retryAt
		transport.publishStatus(status)
		if cooldown.Seconds > 0 {
			go transport.reloadAfter(generation, time.Duration(cooldown.Seconds)*time.Second+5*time.Second)
		}
	default:
		status.State = "error"
		status.LoggedIn = false
		status.CooldownUntil = nil
		status.RetryAt = nil
		status.Detail = fmt.Sprintf("Game login failed with code %d", *frame.ResponseCode)
		transport.publishStatus(status)
	}
}

func (transport *ChromiumTransport) reloadAfter(generation uint64, delay time.Duration) {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	transport.mu.RLock()
	gameContext := transport.gameContext
	transport.mu.RUnlock()
	if gameContext == nil {
		return
	}
	select {
	case <-gameContext.Done():
		return
	case <-timer.C:
	}
	if !transport.isCurrent(generation) {
		return
	}
	transport.publishStatus(Status{State: "reconnecting", Namespace: transport.namespace(), ChangedAt: time.Now().UTC()})
	_ = chromedp.Run(gameContext, chromedp.Reload())
}

func (transport *ChromiumTransport) waitForSendPacing(ctx context.Context) error {
	wait := 25*time.Millisecond - time.Since(transport.lastSend)
	if wait <= 0 {
		return nil
	}
	timer := time.NewTimer(wait)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func (transport *ChromiumTransport) publishStatus(status Status) {
	if status.ChangedAt.IsZero() {
		status.ChangedAt = time.Now().UTC()
	}
	transport.mu.Lock()
	if status.Namespace == "" {
		status.Namespace = transport.status.Namespace
		if status.Namespace == "" {
			status.Namespace = "EmpireEx_21"
		}
	}
	if status.BrowserID == "" {
		status.BrowserID = transport.browser.ID
	}
	if status.BrowserName == "" {
		status.BrowserName = transport.browser.Name
	}
	transport.status = status
	transport.mu.Unlock()
	select {
	case transport.statuses <- status:
	default:
		select {
		case <-transport.statuses:
		default:
		}
		select {
		case transport.statuses <- status:
		default:
		}
	}
}

func (transport *ChromiumTransport) namespace() string {
	transport.mu.RLock()
	defer transport.mu.RUnlock()
	if transport.status.Namespace == "" {
		return "EmpireEx_21"
	}
	return transport.status.Namespace
}

func (transport *ChromiumTransport) isCurrent(generation uint64) bool {
	transport.mu.RLock()
	defer transport.mu.RUnlock()
	return transport.generation == generation && transport.cancel != nil
}

func (transport *ChromiumTransport) isTracked(requestID network.RequestID) bool {
	transport.mu.RLock()
	defer transport.mu.RUnlock()
	_, ok := transport.trackedSockets[requestID]
	return ok
}

func (transport *ChromiumTransport) removeTracked(requestID network.RequestID) bool {
	transport.mu.Lock()
	defer transport.mu.Unlock()
	if _, ok := transport.trackedSockets[requestID]; !ok {
		return false
	}
	delete(transport.trackedSockets, requestID)
	return true
}

func (transport *ChromiumTransport) clearRun(generation uint64) bool {
	transport.mu.Lock()
	defer transport.mu.Unlock()
	if transport.generation != generation {
		return false
	}
	transport.cancel = nil
	transport.gameContext = nil
	transport.executionContextID = 0
	transport.trackedSockets = map[network.RequestID]string{}
	return true
}

func decodeWebSocketPayload(frame *network.WebSocketFrame) (string, error) {
	if frame.Opcode == 1 {
		return frame.PayloadData, nil
	}
	decoded, err := base64.StdEncoding.DecodeString(frame.PayloadData)
	if err != nil {
		return "", err
	}
	return string(decoded), nil
}

func isGameSocketURL(url string) bool {
	return strings.Contains(strings.ToLower(url), "ep-live")
}

func removeStaleChromiumLocks(profileDir string) {
	for _, name := range []string{"SingletonLock", "SingletonSocket", "SingletonCookie"} {
		_ = os.Remove(filepath.Join(profileDir, name))
	}
}

const httpSwitchingProtocols = 101

const chromiumTransportInjection = `
(() => {
  const install = (root) => {
    if (!root || root.__citadelTransportInstalled || !root.WebSocket) return;
    root.__citadelTransportInstalled = true;
    const NativeWebSocket = root.WebSocket;
    const sockets = [];
    let authenticated = null;
    const notify = (message) => {
      if (typeof root.citadelTransportNotify === 'function') {
        root.citadelTransportNotify(JSON.stringify(message));
      }
    };
    root.WebSocket = new Proxy(NativeWebSocket, {
      construct(target, args) {
        const socket = new target(...args);
        const url = args[0];
        if (typeof url === 'string' && url.toLowerCase().includes('ep-live')) {
          sockets.push(socket);
          notify({ type: 'socket', url: String(url || '') });
          socket.addEventListener('message', (event) => {
            if (typeof event.data !== 'string') return;
            const parts = event.data.split('%');
            if (parts.length > 4 && parts[2] === 'lli') {
              authenticated = parts[4] === '0' ? socket : null;
            }
          });
          socket.addEventListener('close', () => {
            if (authenticated === socket) authenticated = null;
            const index = sockets.indexOf(socket);
            if (index >= 0) sockets.splice(index, 1);
          }, { once: true });
        }
        return socket;
      }
    });
    root.__citadelSend = (payload) => {
      if (authenticated && authenticated.readyState === NativeWebSocket.OPEN) {
        authenticated.send(payload);
        return true;
      }
      const socket = sockets.find((candidate) => candidate && candidate.readyState === NativeWebSocket.OPEN);
      if (!socket) return false;
      socket.send(payload);
      return true;
    };
    if (root.document && !root.__citadelActivityInstalled) {
      root.__citadelActivityInstalled = true;
      let lastMoveNotice = 0;
      let lastNotice = 0;
      const notifyActivity = (kind) => {
        const now = Date.now();
        if (kind === 'pointermove' && now - lastMoveNotice < 1500) return;
        if (kind !== 'pointermove' && now - lastNotice < 250) return;
        if (kind === 'pointermove') lastMoveNotice = now;
        lastNotice = now;
        notify({ type: 'activity', kind });
      };
      const options = { capture: true, passive: true };
      root.document.addEventListener('pointerdown', () => notifyActivity('pointerdown'), options);
      root.document.addEventListener('pointermove', () => notifyActivity('pointermove'), options);
      root.document.addEventListener('wheel', () => notifyActivity('wheel'), options);
      root.document.addEventListener('keydown', () => notifyActivity('keydown'), true);
      root.addEventListener('focus', () => notifyActivity('focus'), true);
    }
  };

  install(globalThis);
  if (typeof window !== 'undefined' && window.Worker && !window.__citadelWorkerInstalled) {
    window.__citadelWorkerInstalled = true;
    const NativeWorker = window.Worker;
    const WrappedWorker = function(scriptURL, options) {
      try {
        const absoluteURL = new URL(scriptURL, document.baseURI).href;
        const bootstrap = '(' + install.toString() + ')(self); importScripts(' + JSON.stringify(absoluteURL) + ');';
        const blobURL = URL.createObjectURL(new Blob([bootstrap], { type: 'application/javascript' }));
        return new NativeWorker(blobURL, options);
      } catch (_) {
        return new NativeWorker(scriptURL, options);
      }
    };
    WrappedWorker.prototype = NativeWorker.prototype;
    Object.setPrototypeOf(WrappedWorker, NativeWorker);
    window.Worker = WrappedWorker;
  }
})();
`
