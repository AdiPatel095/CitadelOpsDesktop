package Session

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"CitadelDesktop/Server/Outbound"
	"CitadelDesktop/Server/Protocol"
	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/cdproto/page"
	"github.com/chromedp/cdproto/runtime"
	"github.com/chromedp/cdproto/target"
	"github.com/chromedp/chromedp"
	"github.com/chromedp/chromedp/kb"
)

const defaultGameURL = "https://empire.goodgamestudios.com/"

const (
	browserEvaluationTimeout = 5 * time.Second
	defaultRelogDelay        = 5 * time.Minute
	websocketTrafficTimeout  = 30 * time.Second
	websocketTrafficPoll     = 2 * time.Second
	authenticationTimeout    = 2 * time.Minute
	logoutReloadDelay        = 250 * time.Millisecond
	credentialRestorePoll    = 500 * time.Millisecond
	gameTargetPollInterval   = time.Second
	socketResyncPoll         = 250 * time.Millisecond
	socketResyncTimeout      = 5 * time.Second
)

type ChromiumConfig struct {
	DataDir        string
	DashboardURL   string
	GameURL        string
	Headless       bool
	Browser        string
	ExecutablePath string
}

type ChromiumTransport struct {
	config          ChromiumConfig
	browser         BrowserCandidate
	selectedBrowser BrowserCandidate
	resolveErr      error
	frames          chan RawFrame
	statuses        chan Status

	mu                         sync.RWMutex
	status                     Status
	browserContext             context.Context
	gameContext                context.Context
	gameCancel                 context.CancelFunc
	gameTargetID               target.ID
	cancel                     context.CancelFunc
	generation                 uint64
	executionContextID         runtime.ExecutionContextID
	trackedSockets             map[network.RequestID]string
	sockets                    map[chromiumSocketKey]*chromiumSocket
	closedSockets              map[chromiumSocketKey]uint64
	activeSocket               chromiumSocketKey
	activeToken                string
	activeOrdinal              uint64
	socketGeneration           uint64
	nextSocketOrdinal          uint64
	lastInboundAt              time.Time
	loginCredential            persistedLoginCredential
	restoreSuppressed          bool
	restoreAttempted           bool
	backgroundModePrepared     bool
	activationEvaluator        func(context.Context, runtime.ExecutionContextID, string, uint64) (bool, error)
	credentialRestoreEvaluator func(
		context.Context, runtime.ExecutionContextID, string, string,
	) (bool, error)
	credentialSubmitEvaluator func(context.Context) error
	reloadEvaluator           func(context.Context) error
	relogDelayProvider        func() time.Duration

	sendGateOnce   sync.Once
	sendGate       chan struct{}
	noticeMu       sync.Mutex
	noticeQueue    []queuedSocketNotice
	noticeWake     chan struct{}
	noticeOverflow bool
}

type chromiumSocketKey struct {
	executionContextID runtime.ExecutionContextID
	token              string
}

type chromiumSocket struct {
	serverURL         string
	ordinal           uint64
	lastSequence      uint64
	activatedSequence uint64
	pendingLoginFrame string
	loginUsername     string
	loginPassword     string
	clientBuild       string
	platform          string
	loginContext      map[string]json.RawMessage
}

type chromiumSocketNotice struct {
	Version              int                `json:"v"`
	Type                 string             `json:"type"`
	Token                string             `json:"token"`
	Sequence             uint64             `json:"seq"`
	URL                  string             `json:"url"`
	Direction            Protocol.Direction `json:"direction"`
	Payload              string             `json:"payload"`
	ResponseToken        string             `json:"responseToken"`
	CausationOperationID string             `json:"causationOperationId"`
	LoginUsername        string             `json:"loginUsername"`
	LoginPassword        string             `json:"loginPassword"`
	CloseCode            int                `json:"closeCode"`
	CloseReason          string             `json:"closeReason"`
	WasClean             bool               `json:"wasClean"`
	Eligible             bool               `json:"eligible"`
}

type queuedSocketNotice struct {
	generation         uint64
	executionContextID runtime.ExecutionContextID
	observedAt         time.Time
	payload            string
	destroyContext     bool
	clearContexts      bool
}

type socketActivation struct {
	serverURL            string
	connectionGeneration uint64
	changedAt            time.Time
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
	loginCredential, _ := loadLoginCredential(config.DataDir)
	state := "stopped"
	detail := ""
	if resolveErr != nil {
		state = "unavailable"
		detail = resolveErr.Error()
	}
	return &ChromiumTransport{
		config: config, browser: browser, selectedBrowser: browser, resolveErr: resolveErr,
		frames: make(chan RawFrame, 8192), statuses: make(chan Status, 32),
		status: Status{
			Mode: ConnectionModeFull, State: state, Namespace: "EmpireEx_21", Detail: detail,
			BrowserID: browser.ID, BrowserName: browser.Name, ChangedAt: time.Now().UTC(),
		},
		trackedSockets:    map[network.RequestID]string{},
		sockets:           map[chromiumSocketKey]*chromiumSocket{},
		closedSockets:     map[chromiumSocketKey]uint64{},
		loginCredential:   loginCredential,
		restoreSuppressed: loginCredential.Username != "" && !loginCredential.AutoRestore,
		noticeWake:        make(chan struct{}, 1),
	}
}

func (transport *ChromiumTransport) Start(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}
	transport.mu.Lock()
	if transport.resolveErr != nil {
		err := transport.resolveErr
		transport.mu.Unlock()
		return err
	}
	if transport.cancel != nil {
		if transport.status.State == "disconnected" && transport.gameContext != nil {
			gameContext := transport.gameContext
			generation := transport.generation
			status := transport.status
			status.State = "reconnecting"
			status.LoggedIn = false
			status.SocketReady = false
			status.RetryAt = nil
			status.ChangedAt = time.Now().UTC()
			transport.status = status
			transport.mu.Unlock()
			transport.enqueueStatus(status)
			if err := transport.reloadGame(gameContext); err != nil {
				transport.publishReloadFailure(status, "Reload disconnected game session", err)
				return fmt.Errorf("reload disconnected game session: %w", err)
			}
			transport.scheduleConnectionTimeout(
				generation, status.ConnectionGeneration, status.ChangedAt,
			)
			return nil
		}
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
	profileDir := chromiumProfileDir(transport.config.DataDir, browser.ID)
	if err := os.MkdirAll(profileDir, 0o700); err != nil {
		transport.publishStatus(Status{State: "error", Detail: err.Error(), ChangedAt: time.Now().UTC()})
		return fmt.Errorf("create %s profile: %w", browser.Name, err)
	}
	connection, err := connectChromiumProfile(
		ctx, browser, profileDir, transport.config.DashboardURL, transport.config.Headless,
	)
	if err != nil {
		transport.publishStatus(Status{State: "error", Detail: err.Error(), ChangedAt: time.Now().UTC()})
		return err
	}
	dashboardContext, cancelDashboard := context.WithTimeout(
		connection.context, chromiumEndpointStartupTimeout,
	)
	dashboardErr := ensureChromiumTarget(dashboardContext, transport.config.DashboardURL)
	cancelDashboard()
	if dashboardErr != nil {
		connection.cancel()
		transport.publishStatus(Status{State: "error", Detail: dashboardErr.Error(), ChangedAt: time.Now().UTC()})
		return fmt.Errorf("open CitadelOps dashboard: %w", dashboardErr)
	}
	var cancelOnce sync.Once
	cancelAll := func() {
		cancelOnce.Do(func() {
			transport.detachGameContext(generation)
			connection.cancel()
		})
	}
	transport.mu.Lock()
	if transport.generation != generation || transport.cancel != nil {
		transport.mu.Unlock()
		cancelAll()
		return fmt.Errorf("Chromium session start was superseded")
	}
	transport.browserContext = connection.context
	transport.cancel = cancelAll
	transport.resetSocketsLocked()
	transport.mu.Unlock()
	transport.resetSocketNoticeQueue()
	go transport.runSocketNotices(connection.context, generation)
	go transport.runSocketTrafficWatchdog(connection.context, generation)
	detail := fmt.Sprintf("Started %s profile; attaching to the game tab", browser.Name)
	if connection.reused {
		detail = fmt.Sprintf("Reconnected to the existing %s profile; attaching to the game tab", browser.Name)
	}
	transport.publishStatus(Status{
		State: "connecting", Namespace: "EmpireEx_21", Detail: detail, ChangedAt: time.Now().UTC(),
	})
	if err := transport.ensureGameTarget(connection.context, generation); err != nil {
		cancelAll()
		transport.clearRun(generation)
		transport.publishStatus(Status{
			State: "error", Namespace: "EmpireEx_21", Detail: err.Error(), ChangedAt: time.Now().UTC(),
		})
		return fmt.Errorf("open game: %w", err)
	}
	go transport.runGameTargetMonitor(connection.context, generation)
	go transport.watchBrowserRun(ctx, connection.context, generation, cancelAll)
	return nil
}

func (transport *ChromiumTransport) Stop(context.Context) error {
	transport.mu.Lock()
	cancel := transport.cancel
	gameContext := transport.gameContext
	gameCancel := transport.gameCancel
	transport.cancel = nil
	transport.browserContext = nil
	transport.gameContext = nil
	transport.gameCancel = nil
	transport.gameTargetID = ""
	transport.resetSocketsLocked()
	transport.generation++
	transport.mu.Unlock()
	detachChromedpTarget(gameContext, gameCancel)
	if cancel != nil {
		cancel()
	}
	transport.resetSocketNoticeQueue()
	transport.publishStatus(Status{State: "stopped", Namespace: "EmpireEx_21", ChangedAt: time.Now().UTC()})
	return nil
}

func (transport *ChromiumTransport) PrepareBackgroundMode() error {
	credential, _, err := prepareBackgroundLogin(transport.config.DataDir)
	if err != nil {
		return err
	}
	transport.mu.Lock()
	transport.loginCredential = credential
	transport.restoreSuppressed = false
	transport.backgroundModePrepared = true
	transport.mu.Unlock()
	return nil
}

func (transport *ChromiumTransport) watchBrowserRun(
	callerContext context.Context,
	browserContext context.Context,
	generation uint64,
	cancel context.CancelFunc,
) {
	select {
	case <-callerContext.Done():
	case <-browserContext.Done():
	}
	cancel()
	if transport.clearRun(generation) {
		transport.publishStatus(Status{
			State: "disconnected", Namespace: "EmpireEx_21", ChangedAt: time.Now().UTC(),
		})
	}
}

func (transport *ChromiumTransport) runGameTargetMonitor(
	browserContext context.Context,
	generation uint64,
) {
	ticker := time.NewTicker(gameTargetPollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-browserContext.Done():
			return
		case <-ticker.C:
			if err := transport.ensureGameTarget(browserContext, generation); err != nil {
				if !transport.isCurrent(generation) || browserContext.Err() != nil {
					return
				}
				status := transport.Status()
				status.State = "error"
				status.LoggedIn = false
				status.SocketReady = false
				status.Detail = fmt.Sprintf("Recover game tab: %v", err)
				status.ChangedAt = time.Now().UTC()
				transport.publishStatus(status)
			}
		}
	}
}

func (transport *ChromiumTransport) ensureGameTarget(
	browserContext context.Context,
	generation uint64,
) error {
	if !transport.isCurrent(generation) {
		return context.Canceled
	}
	queryContext, cancelQuery := context.WithTimeout(browserContext, browserEvaluationTimeout)
	targets, err := chromedp.Targets(queryContext)
	cancelQuery()
	if err != nil {
		return err
	}

	transport.mu.RLock()
	currentTargetID := transport.gameTargetID
	currentGameContext := transport.gameContext
	current := transport.generation == generation && transport.cancel != nil
	transport.mu.RUnlock()
	if !current {
		return context.Canceled
	}
	var currentTarget *target.Info
	for _, info := range targets {
		if info == nil || info.TargetID != currentTargetID {
			continue
		}
		currentTarget = info
		break
	}
	if currentTarget != nil && currentGameContext != nil {
		select {
		case <-currentGameContext.Done():
		default:
			if isGamePageURL(currentTarget.URL) {
				return nil
			}
			status := transport.Status()
			status.State = "connecting"
			status.LoggedIn = false
			status.SocketReady = false
			status.Detail = "Game tab navigated away; reopening the game in the same tab"
			status.RetryAt = nil
			status.ChangedAt = time.Now().UTC()
			transport.publishStatus(status)
			if err := transport.acquireSendGate(browserContext); err != nil {
				return err
			}
			navigateErr := chromedp.Run(
				currentGameContext, bringGameTabToFront(), chromedp.Navigate(transport.config.GameURL),
			)
			transport.releaseSendGate()
			if navigateErr != nil {
				return navigateErr
			}
			connecting := transport.Status()
			transport.scheduleConnectionTimeout(
				generation, connecting.ConnectionGeneration, connecting.ChangedAt,
			)
			return nil
		}
	}

	transport.detachGameContext(generation)
	status := transport.Status()
	status.State = "connecting"
	status.LoggedIn = false
	status.SocketReady = false
	if currentTargetID != "" {
		status.Detail = "Game tab closed; attaching to a replacement"
	} else if status.Detail == "" {
		status.Detail = "Attaching to the game tab"
	}
	status.RetryAt = nil
	status.ChangedAt = time.Now().UTC()
	transport.publishStatus(status)

	var gameTargetID target.ID
	for _, info := range targets {
		if info != nil && info.Type == "page" && isGamePageURL(info.URL) {
			gameTargetID = info.TargetID
			break
		}
	}
	navigate := false
	if gameTargetID == "" {
		gameTargetID, err = createChromiumTarget(browserContext, "about:blank")
		if err != nil {
			return err
		}
		navigate = true
	}
	if err := transport.attachGameTarget(
		browserContext, generation, gameTargetID, navigate,
	); err != nil {
		return err
	}
	connecting := transport.Status()
	transport.scheduleConnectionTimeout(
		generation, connecting.ConnectionGeneration, connecting.ChangedAt,
	)
	return nil
}

func (transport *ChromiumTransport) attachGameTarget(
	browserContext context.Context,
	generation uint64,
	gameTargetID target.ID,
	navigate bool,
) error {
	if err := transport.acquireSendGate(browserContext); err != nil {
		return err
	}
	defer transport.releaseSendGate()

	gameContext, gameCancel := chromedp.NewContext(
		browserContext, chromedp.WithTargetID(gameTargetID),
	)
	transport.mu.Lock()
	if transport.generation != generation || transport.cancel == nil ||
		transport.browserContext != browserContext || transport.gameContext != nil {
		transport.mu.Unlock()
		detachChromedpTarget(gameContext, gameCancel)
		return context.Canceled
	}
	transport.gameContext = gameContext
	transport.gameCancel = gameCancel
	transport.gameTargetID = gameTargetID
	transport.restoreAttempted = false
	transport.resetSocketsLocked()
	transport.mu.Unlock()
	transport.resetSocketNoticeQueue()

	chromedp.ListenTarget(gameContext, func(event any) {
		if transport.isCurrentGameContext(generation, gameContext) {
			transport.handleEvent(generation, event)
		}
	})
	actions := []chromedp.Action{
		chromedp.ActionFunc(func(ctx context.Context) error {
			_, err := page.AddScriptToEvaluateOnNewDocument(chromiumTransportInjection).Do(ctx)
			return err
		}),
		runtime.AddBinding("citadelTransportNotify"),
		network.Enable(),
		runtime.Enable(),
	}
	setupErr := chromedp.Run(gameContext, actions...)
	if setupErr == nil {
		if navigate {
			setupErr = chromedp.Run(
				gameContext, bringGameTabToFront(), chromedp.Navigate(transport.config.GameURL),
			)
		} else {
			// A running tab cannot replay the login/bootstrap frames Citadel missed
			// while detached. Reuse the tab and browser, but refresh the game once
			// so reducers receive a complete current-session baseline.
			setupErr = transport.reloadGame(gameContext)
		}
	}
	if setupErr != nil {
		transport.mu.Lock()
		if transport.generation == generation && transport.gameContext == gameContext {
			transport.gameContext = nil
			transport.gameCancel = nil
			transport.gameTargetID = ""
			transport.resetSocketsLocked()
		}
		transport.mu.Unlock()
		transport.resetSocketNoticeQueue()
		detachChromedpTarget(gameContext, gameCancel)
		return setupErr
	}
	return nil
}

func (transport *ChromiumTransport) isCurrentGameContext(
	generation uint64,
	gameContext context.Context,
) bool {
	transport.mu.RLock()
	defer transport.mu.RUnlock()
	return transport.generation == generation && transport.cancel != nil &&
		transport.gameContext == gameContext
}

func (transport *ChromiumTransport) detachGameContext(generation uint64) {
	transport.mu.Lock()
	if transport.generation != generation {
		transport.mu.Unlock()
		return
	}
	gameContext := transport.gameContext
	gameCancel := transport.gameCancel
	transport.gameContext = nil
	transport.gameCancel = nil
	transport.gameTargetID = ""
	transport.restoreAttempted = false
	transport.resetSocketsLocked()
	transport.mu.Unlock()
	transport.resetSocketNoticeQueue()
	detachChromedpTarget(gameContext, gameCancel)
}

func (transport *ChromiumTransport) Send(ctx context.Context, payload []byte) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := transport.acquireSendGate(ctx); err != nil {
		return err
	}
	defer transport.releaseSendGate()
	metadata := Outbound.MetadataFromContext(ctx)
	transport.mu.RLock()
	gameContext := transport.gameContext
	executionContextID := transport.executionContextID
	activeToken := transport.activeToken
	connectionGeneration := transport.status.ConnectionGeneration
	ready := transport.status.LoggedIn && transport.status.SocketReady
	transport.mu.RUnlock()
	if metadata.ConnectionGeneration > 0 && metadata.ConnectionGeneration != connectionGeneration {
		return Outbound.ErrConnectionChanged
	}
	if gameContext == nil || !ready || activeToken == "" {
		return fmt.Errorf("game websocket is not ready")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	literal, err := json.Marshal(string(payload))
	if err != nil {
		return err
	}
	tokenLiteral, err := json.Marshal(activeToken)
	if err != nil {
		return err
	}
	epochLiteral, err := json.Marshal(strconv.FormatUint(connectionGeneration, 10))
	if err != nil {
		return err
	}
	responseTokenLiteral, err := json.Marshal(metadata.ResponseToken)
	if err != nil {
		return err
	}
	responseOpcodesLiteral, err := json.Marshal(metadata.ResponseOpcodes)
	if err != nil {
		return err
	}
	causationOperationIDLiteral, err := json.Marshal(metadata.OperationID)
	if err != nil {
		return err
	}
	expression := fmt.Sprintf(
		"globalThis.__citadelSend && globalThis.__citadelSend(%s, %s, %s, %s, %s, %d, %s)",
		tokenLiteral, epochLiteral, literal, responseTokenLiteral, responseOpcodesLiteral,
		metadata.ResponseTimeoutMillis, causationOperationIDLiteral,
	)
	sent := false
	evaluationContext, cancelEvaluation := browserEvaluationContext(gameContext, ctx)
	defer cancelEvaluation()
	err = chromedp.Run(evaluationContext, chromedp.ActionFunc(func(ctx context.Context) error {
		evaluation := websocketRuntimeEvaluation(expression)
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
	if err != nil {
		if callerErr := ctx.Err(); callerErr != nil {
			err = callerErr
		}
		transport.invalidateActiveSend(connectionGeneration, err)
		return Outbound.MarkIndeterminate(err)
	}
	if !sent {
		err = fmt.Errorf("game websocket rejected the payload")
		transport.invalidateActiveSend(connectionGeneration, err)
		return err
	}
	return nil
}

func (transport *ChromiumTransport) CloseGameUI(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := transport.acquireSendGate(ctx); err != nil {
		return err
	}
	defer transport.releaseSendGate()
	metadata := Outbound.MetadataFromContext(ctx)
	transport.mu.RLock()
	gameContext := transport.gameContext
	executionContextID := transport.executionContextID
	connectionGeneration := transport.status.ConnectionGeneration
	ready := transport.status.LoggedIn && transport.status.SocketReady
	transport.mu.RUnlock()
	if metadata.ConnectionGeneration > 0 && metadata.ConnectionGeneration != connectionGeneration {
		return Outbound.ErrConnectionChanged
	}
	if gameContext == nil || !ready {
		return fmt.Errorf("game websocket is not ready")
	}
	operationIDLiteral, err := json.Marshal(metadata.OperationID)
	if err != nil {
		return err
	}
	expression := fmt.Sprintf(
		"globalThis.__citadelCloseGameUI && globalThis.__citadelCloseGameUI(%s)",
		operationIDLiteral,
	)
	closed := false
	evaluationContext, cancelEvaluation := browserEvaluationContext(gameContext, ctx)
	defer cancelEvaluation()
	err = chromedp.Run(evaluationContext, chromedp.ActionFunc(func(ctx context.Context) error {
		evaluation := websocketRuntimeEvaluation(expression)
		if executionContextID != 0 {
			evaluation = evaluation.WithContextID(executionContextID)
		}
		result, exception, evaluateErr := evaluation.Do(ctx)
		if evaluateErr != nil {
			return evaluateErr
		}
		if exception != nil {
			return fmt.Errorf("close game UI: %s", exception.Text)
		}
		if result != nil {
			_ = json.Unmarshal(result.Value, &closed)
		}
		return nil
	}))
	if err != nil {
		if callerErr := ctx.Err(); callerErr != nil {
			return callerErr
		}
		return err
	}
	if !closed {
		return fmt.Errorf("%w: live layout manager was not found", ErrFrontendInteractionUnavailable)
	}
	return nil
}

func (transport *ChromiumTransport) Frames() <-chan RawFrame {
	return transport.frames
}

func (*ChromiumTransport) CorrelatesResponses() bool { return true }

func (*ChromiumTransport) ReportsOutboundCausation() bool { return true }

func (transport *ChromiumTransport) invalidateActiveSend(
	connectionGeneration uint64,
	sendErr error,
) {
	transport.mu.Lock()
	if transport.status.ConnectionGeneration != connectionGeneration || transport.activeToken == "" {
		transport.mu.Unlock()
		return
	}
	generation := transport.generation
	transport.clearActiveSocketLocked()
	transport.mu.Unlock()
	status := transport.Status()
	status.State = "error"
	status.LoggedIn = false
	status.SocketReady = false
	status.Detail = fmt.Sprintf("Game websocket send became indeterminate: %v", sendErr)
	status.ChangedAt = time.Now().UTC()
	transport.publishStatus(status)
	go transport.reloadAfter(generation, connectionGeneration, 250*time.Millisecond)
}

func (transport *ChromiumTransport) StatusChanges() <-chan Status {
	return transport.statuses
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
	if err := saveBrowserPreference(transport.config.DataDir, candidate); err != nil {
		return err
	}
	transport.mu.Lock()
	transport.selectedBrowser = candidate
	transport.mu.Unlock()
	return nil
}

func (transport *ChromiumTransport) BrowserInventory() BrowserInventory {
	available := DiscoverChromiumBrowsers()
	transport.mu.RLock()
	current := transport.browser
	selected := transport.selectedBrowser
	transport.mu.RUnlock()
	return browserInventory(current, selected, available)
}

func browserInventory(current BrowserCandidate, selected BrowserCandidate, available []BrowserCandidate) BrowserInventory {
	inventory := BrowserInventory{
		Available:       available,
		RestartRequired: !sameBrowserCandidate(current, selected),
		SelectionIntent: "session.select_browser",
	}
	if current.ID != "" {
		currentCopy := current
		inventory.Current = &currentCopy
	}
	if selected.ID != "" {
		selectedCopy := selected
		inventory.Selected = &selectedCopy
	}
	return inventory
}

func sameBrowserCandidate(left BrowserCandidate, right BrowserCandidate) bool {
	if left.ID == "" || right.ID == "" {
		return left.ID == right.ID
	}
	return left.ID == right.ID && samePath(left.ExecutablePath, right.ExecutablePath)
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
		transport.enqueueSocketNotice(queuedSocketNotice{
			generation: generation, executionContextID: typed.ExecutionContextID,
			observedAt: time.Now().UTC(), payload: typed.Payload,
		})
	case *runtime.EventExecutionContextCreated:
		if typed.Context == nil || typed.Context.Name != "" ||
			!isGameLoginOrigin(typed.Context.Origin) {
			return
		}
		transport.scheduleSocketResync(generation, typed.Context.ID)
		transport.scheduleCredentialRestore(generation, typed.Context.ID)
	case *runtime.EventExecutionContextDestroyed:
		transport.enqueueSocketNotice(queuedSocketNotice{
			generation: generation, executionContextID: typed.ExecutionContextID,
			observedAt: time.Now().UTC(), destroyContext: true,
		})
	case *runtime.EventExecutionContextsCleared:
		transport.enqueueSocketNotice(queuedSocketNotice{
			generation: generation, observedAt: time.Now().UTC(), clearContexts: true,
		})
	case *network.EventWebSocketCreated:
		if !isGameSocketURL(typed.URL) {
			return
		}
		transport.mu.Lock()
		transport.trackedSockets[typed.RequestID] = typed.URL
		mayPublish := transport.activeToken == ""
		transport.mu.Unlock()
		if !mayPublish {
			return
		}
		transport.publishStatus(Status{
			State: "connecting", ServerURL: typed.URL, Namespace: transport.namespace(), ChangedAt: time.Now().UTC(),
		})
		connecting := transport.Status()
		transport.scheduleConnectionTimeout(
			generation, connecting.ConnectionGeneration, connecting.ChangedAt,
		)
	case *network.EventWebSocketHandshakeResponseReceived:
		transport.mu.RLock()
		_, tracked := transport.trackedSockets[typed.RequestID]
		mayPublish := tracked && transport.activeToken == ""
		transport.mu.RUnlock()
		if typed.Response != nil && typed.Response.Status == httpSwitchingProtocols {
			if !mayPublish {
				return
			}
			status := transport.Status()
			observedAt := time.Now().UTC()
			status.State = "authenticating"
			status.SocketReady = true
			status.ChangedAt = observedAt
			transport.publishStatus(status)
			transport.scheduleAuthenticationTimeout(
				generation, status.ConnectionGeneration, observedAt,
			)
		}
	case *network.EventWebSocketClosed:
		removed, noActive, remaining := transport.removeNetworkSocket(typed.RequestID)
		if !removed || !noActive || remaining > 0 {
			return
		}
		transport.mu.RLock()
		waitingForAccount := transport.restoreSuppressed
		transport.mu.RUnlock()
		if waitingForAccount {
			return
		}
		status := transport.Status()
		if status.State == "cooldown" || status.State == "error" || status.State == "stopped" {
			return
		}
		transport.publishSocketLoss("disconnected", "Game websocket closed", 0)
		if status.LoggedIn || status.State == "authenticating" {
			transport.scheduleSocketReconnect(generation, status.ConnectionGeneration)
		}
	case *network.EventWebSocketFrameError:
		removed, noActive, remaining := transport.removeNetworkSocket(typed.RequestID)
		if !removed || !noActive || remaining > 0 {
			return
		}
		transport.mu.RLock()
		waitingForAccount := transport.restoreSuppressed
		transport.mu.RUnlock()
		if waitingForAccount {
			return
		}
		status := transport.Status()
		if status.State == "cooldown" || status.State == "error" || status.State == "stopped" {
			return
		}
		transport.publishSocketLoss("error", typed.ErrorMessage, 0)
		if status.LoggedIn || status.State == "authenticating" {
			transport.scheduleSocketReconnect(generation, status.ConnectionGeneration)
		}
	}
}

func (transport *ChromiumTransport) observeLoginFrame(
	generation uint64,
	requestID network.RequestID,
	payload string,
	observedAt time.Time,
) {
	frame, err := Protocol.Decode(payload, Protocol.DirectionInbound, time.Now().UTC())
	if err != nil || frame.Opcode != "lli" || frame.ResponseCode == nil {
		return
	}
	if requestID != "" {
		return
	}
	switch *frame.ResponseCode {
	case 0:
		status := transport.Status()
		status.State = "connected"
		status.LoggedIn = true
		status.SocketReady = true
		status.Detail = ""
		status.CooldownUntil = nil
		status.RetryAt = nil
		status.ChangedAt = observedAt
		transport.publishStatus(status)
	default:
		transport.publishLoginFailure(generation, frame, observedAt)
	}
}

func (transport *ChromiumTransport) publishLoginFailure(
	generation uint64,
	frame Protocol.Frame,
	observedAt time.Time,
) {
	if frame.ResponseCode == nil {
		return
	}
	status := transport.Status()
	status.ChangedAt = observedAt
	status.LoggedIn = false
	status.SocketReady = false
	switch *frame.ResponseCode {
	case 453:
		var cooldown struct {
			Seconds int `json:"CD"`
		}
		_ = json.Unmarshal(frame.Payload, &cooldown)
		now := time.Now().UTC()
		cooldownUntil := now.Add(time.Duration(max(0, cooldown.Seconds)) * time.Second)
		relogDelay := transport.relogDelay()
		retryAt := cooldownUntil.Add(relogDelay)
		status.State = "cooldown"
		status.Detail = fmt.Sprintf("Login cooldown: %ds", cooldown.Seconds)
		status.CooldownUntil = &cooldownUntil
		status.RetryAt = &retryAt
		transport.publishStatus(status)
		if cooldown.Seconds > 0 {
			go transport.reloadAfter(
				generation, status.ConnectionGeneration,
				time.Duration(cooldown.Seconds)*time.Second+relogDelay,
			)
		}
	default:
		status.State = "error"
		status.CooldownUntil = nil
		status.RetryAt = nil
		status.Detail = fmt.Sprintf("Game login failed with code %d", *frame.ResponseCode)
		transport.publishStatus(status)
	}
}

func (transport *ChromiumTransport) reloadAfter(
	generation uint64,
	connectionGeneration uint64,
	delay time.Duration,
) {
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
	transport.mu.RLock()
	current := transport.generation == generation && transport.cancel != nil &&
		transport.status.ConnectionGeneration == connectionGeneration && !transport.restoreSuppressed
	transport.mu.RUnlock()
	if !current {
		return
	}
	status := transport.Status()
	status.State = "reconnecting"
	status.LoggedIn = false
	status.SocketReady = false
	status.RetryAt = nil
	status.ChangedAt = time.Now().UTC()
	transport.publishStatus(status)
	if err := transport.reloadGame(gameContext); err != nil {
		transport.publishReloadFailure(status, "Reload game while reconnecting", err)
		return
	}
	transport.scheduleConnectionTimeout(generation, connectionGeneration, status.ChangedAt)
}

func (transport *ChromiumTransport) scheduleConnectionTimeout(
	generation uint64,
	connectionGeneration uint64,
	connectingAt time.Time,
) {
	transport.mu.RLock()
	gameContext := transport.gameContext
	transport.mu.RUnlock()
	if gameContext == nil {
		return
	}
	go func() {
		timer := time.NewTimer(authenticationTimeout)
		defer timer.Stop()
		select {
		case <-gameContext.Done():
			return
		case <-timer.C:
		}
		transport.handleConnectionTimeout(
			gameContext, generation, connectionGeneration, connectingAt,
		)
	}()
}

func (transport *ChromiumTransport) handleConnectionTimeout(
	gameContext context.Context,
	generation uint64,
	connectionGeneration uint64,
	connectingAt time.Time,
) {
	transport.mu.Lock()
	if transport.generation != generation || transport.cancel == nil ||
		transport.restoreSuppressed ||
		transport.status.ConnectionGeneration != connectionGeneration ||
		(transport.status.State != "connecting" && transport.status.State != "reconnecting") ||
		!transport.status.ChangedAt.Equal(connectingAt) {
		transport.mu.Unlock()
		return
	}
	status := transport.status
	status.State = "reconnecting"
	status.LoggedIn = false
	status.SocketReady = false
	status.Detail = "Game websocket did not open before timeout"
	status.RetryAt = nil
	status.ChangedAt = time.Now().UTC()
	transport.status = status
	transport.mu.Unlock()
	transport.enqueueStatus(status)

	if err := transport.reloadGame(gameContext); err != nil {
		transport.publishReloadFailure(status, "Reload game after connection timeout", err)
		return
	}
	transport.scheduleConnectionTimeout(generation, connectionGeneration, status.ChangedAt)
}

func (transport *ChromiumTransport) scheduleAuthenticationTimeout(
	generation uint64,
	connectionGeneration uint64,
	handshakeAt time.Time,
) {
	transport.mu.RLock()
	gameContext := transport.gameContext
	transport.mu.RUnlock()
	if gameContext == nil {
		return
	}
	go func() {
		timer := time.NewTimer(authenticationTimeout)
		defer timer.Stop()
		select {
		case <-gameContext.Done():
			return
		case <-timer.C:
		}
		transport.handleAuthenticationTimeout(
			gameContext, generation, connectionGeneration, handshakeAt,
		)
	}()
}

func (transport *ChromiumTransport) handleAuthenticationTimeout(
	gameContext context.Context,
	generation uint64,
	connectionGeneration uint64,
	handshakeAt time.Time,
) {
	transport.mu.Lock()
	if transport.generation != generation || transport.cancel == nil ||
		transport.restoreSuppressed ||
		transport.status.ConnectionGeneration != connectionGeneration ||
		transport.status.State != "authenticating" ||
		!transport.status.ChangedAt.Equal(handshakeAt) {
		transport.mu.Unlock()
		return
	}
	status := transport.status
	status.State = "reconnecting"
	status.LoggedIn = false
	status.SocketReady = false
	status.Detail = "Game login handshake timed out"
	status.RetryAt = nil
	status.ChangedAt = time.Now().UTC()
	transport.status = status
	transport.mu.Unlock()
	transport.enqueueStatus(status)

	if err := transport.reloadGame(gameContext); err != nil {
		transport.publishReloadFailure(status, "Reload game after login timeout", err)
		return
	}
	transport.scheduleConnectionTimeout(generation, connectionGeneration, status.ChangedAt)
}

func (transport *ChromiumTransport) reloadGame(ctx context.Context) error {
	transport.mu.Lock()
	transport.restoreAttempted = false
	transport.mu.Unlock()
	if transport.reloadEvaluator != nil {
		return transport.reloadEvaluator(ctx)
	}
	return chromedp.Run(ctx, bringGameTabToFront(), chromedp.Reload())
}

func bringGameTabToFront() chromedp.Action {
	return chromedp.ActionFunc(func(ctx context.Context) error {
		return page.BringToFront().Do(ctx)
	})
}

func (transport *ChromiumTransport) publishReloadFailure(
	pending Status,
	detail string,
	reloadErr error,
) {
	failed := transport.Status()
	if failed.State != "reconnecting" || !failed.ChangedAt.Equal(pending.ChangedAt) {
		return
	}
	failed.State = "error"
	failed.Detail = fmt.Sprintf("%s: %v", detail, reloadErr)
	failed.ChangedAt = time.Now().UTC()
	transport.publishStatus(failed)
}

func (transport *ChromiumTransport) scheduleSocketReconnect(
	generation uint64,
	connectionGeneration uint64,
) {
	delay := transport.relogDelay()
	retryAt := time.Now().UTC().Add(delay)
	status := transport.Status()
	status.RetryAt = &retryAt
	status.ChangedAt = time.Now().UTC()
	transport.publishStatus(status)
	go transport.reloadAfter(generation, connectionGeneration, delay)
}

func (transport *ChromiumTransport) SetRelogDelayProvider(provider func() time.Duration) {
	transport.mu.Lock()
	transport.relogDelayProvider = provider
	transport.mu.Unlock()
}

func (transport *ChromiumTransport) relogDelay() time.Duration {
	transport.mu.RLock()
	provider := transport.relogDelayProvider
	transport.mu.RUnlock()
	if provider == nil {
		return defaultRelogDelay
	}
	delay := provider()
	if delay < 0 {
		return defaultRelogDelay
	}
	return delay
}

func (transport *ChromiumTransport) runSocketTrafficWatchdog(
	ctx context.Context,
	generation uint64,
) {
	ticker := time.NewTicker(websocketTrafficPoll)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-ticker.C:
			transport.mu.RLock()
			lastInboundAt := transport.lastInboundAt
			connectionGeneration := transport.status.ConnectionGeneration
			timedOut := transport.generation == generation && transport.cancel != nil &&
				transport.status.LoggedIn && transport.status.SocketReady &&
				transport.activeToken != "" && !lastInboundAt.IsZero() &&
				now.Sub(lastInboundAt) >= websocketTrafficTimeout
			transport.mu.RUnlock()
			if timedOut {
				transport.handleSocketTrafficTimeout(
					generation, connectionGeneration, lastInboundAt,
				)
			}
		}
	}
}

func (transport *ChromiumTransport) handleSocketTrafficTimeout(
	generation uint64,
	connectionGeneration uint64,
	lastInboundAt time.Time,
) {
	runContext := transport.runContext(generation)
	if runContext == nil || transport.acquireSendGate(runContext) != nil {
		return
	}
	defer transport.releaseSendGate()

	transport.mu.Lock()
	if transport.generation != generation || transport.cancel == nil ||
		transport.status.ConnectionGeneration != connectionGeneration ||
		!transport.status.LoggedIn || !transport.status.SocketReady ||
		transport.activeToken == "" || !transport.lastInboundAt.Equal(lastInboundAt) {
		transport.mu.Unlock()
		return
	}
	status := transport.status
	status.State = "reconnecting"
	status.LoggedIn = false
	status.SocketReady = false
	status.Detail = "Game websocket traffic stopped; reloading game and checking login"
	status.RetryAt = nil
	status.ChangedAt = time.Now().UTC()
	transport.status = status
	transport.restoreAttempted = false
	transport.resetSocketsLocked()
	gameContext := transport.gameContext
	transport.mu.Unlock()
	transport.enqueueStatus(status)

	if err := transport.reloadGame(gameContext); err != nil {
		transport.publishReloadFailure(status, "Reload game after websocket traffic stopped", err)
		return
	}
	transport.scheduleConnectionTimeout(generation, connectionGeneration, status.ChangedAt)
}

func (transport *ChromiumTransport) publishStatus(status Status) {
	if status.ChangedAt.IsZero() {
		status.ChangedAt = time.Now().UTC()
	}
	transport.mu.Lock()
	if status.Mode == "" {
		status.Mode = ConnectionModeFull
	}
	if status.ConnectionGeneration == 0 {
		status.ConnectionGeneration = transport.status.ConnectionGeneration
	}
	if status.ConnectionGeneration < transport.status.ConnectionGeneration {
		transport.mu.Unlock()
		return
	}
	if !transport.status.ChangedAt.IsZero() && status.ChangedAt.Before(transport.status.ChangedAt) &&
		status.ConnectionGeneration <= transport.status.ConnectionGeneration {
		transport.mu.Unlock()
		return
	}
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
	transport.enqueueStatus(status)
}

func (transport *ChromiumTransport) enqueueStatus(status Status) {
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

func (transport *ChromiumTransport) runContext(generation uint64) context.Context {
	transport.mu.RLock()
	defer transport.mu.RUnlock()
	if transport.generation != generation || transport.cancel == nil {
		return nil
	}
	return transport.browserContext
}

func (transport *ChromiumTransport) acquireSendGate(ctx context.Context) error {
	transport.sendGateOnce.Do(func() {
		transport.sendGate = make(chan struct{}, 1)
		transport.sendGate <- struct{}{}
	})
	if ctx == nil {
		ctx = context.Background()
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-transport.sendGate:
		return nil
	}
}

func (transport *ChromiumTransport) releaseSendGate() {
	transport.sendGate <- struct{}{}
}

func browserEvaluationContext(
	gameContext context.Context,
	callerContext context.Context,
) (context.Context, context.CancelFunc) {
	evaluationContext, cancel := context.WithTimeout(gameContext, browserEvaluationTimeout)
	if callerContext == nil {
		return evaluationContext, cancel
	}
	stopCallerCancellation := context.AfterFunc(callerContext, cancel)
	return evaluationContext, func() {
		stopCallerCancellation()
		cancel()
	}
}

func websocketRuntimeEvaluation(expression string) *runtime.EvaluateParams {
	return runtime.Evaluate(expression).WithReturnByValue(true).WithAwaitPromise(true)
}

const maximumQueuedSocketNotices = 32768

func (transport *ChromiumTransport) resetSocketNoticeQueue() {
	transport.noticeMu.Lock()
	transport.noticeQueue = nil
	transport.noticeOverflow = false
	transport.noticeMu.Unlock()
}

func (transport *ChromiumTransport) enqueueSocketNotice(notice queuedSocketNotice) {
	transport.noticeMu.Lock()
	if len(transport.noticeQueue) >= maximumQueuedSocketNotices {
		transport.noticeQueue = nil
		transport.noticeOverflow = true
	} else if !transport.noticeOverflow {
		transport.noticeQueue = append(transport.noticeQueue, notice)
	}
	transport.noticeMu.Unlock()
	select {
	case transport.noticeWake <- struct{}{}:
	default:
	}
}

func (transport *ChromiumTransport) nextSocketNotice(
	ctx context.Context,
) (queuedSocketNotice, bool, bool) {
	for {
		if ctx.Err() != nil {
			return queuedSocketNotice{}, false, false
		}
		transport.noticeMu.Lock()
		if ctx.Err() != nil {
			transport.noticeMu.Unlock()
			return queuedSocketNotice{}, false, false
		}
		if transport.noticeOverflow {
			transport.noticeOverflow = false
			transport.noticeMu.Unlock()
			return queuedSocketNotice{observedAt: time.Now().UTC()}, true, true
		}
		if len(transport.noticeQueue) > 0 {
			notice := transport.noticeQueue[0]
			transport.noticeQueue[0] = queuedSocketNotice{}
			transport.noticeQueue = transport.noticeQueue[1:]
			if len(transport.noticeQueue) == 0 {
				transport.noticeQueue = nil
			}
			transport.noticeMu.Unlock()
			return notice, false, true
		}
		transport.noticeMu.Unlock()
		select {
		case <-ctx.Done():
			return queuedSocketNotice{}, false, false
		case <-transport.noticeWake:
		}
	}
}

func (transport *ChromiumTransport) runSocketNotices(ctx context.Context, generation uint64) {
	for {
		queued, overflow, ok := transport.nextSocketNotice(ctx)
		if !ok {
			return
		}
		if overflow {
			transport.invalidateSocketNoticeStream(generation, queued.observedAt)
			continue
		}
		if queued.generation != generation {
			continue
		}
		transport.processSocketNotice(queued)
	}
}

func (transport *ChromiumTransport) processSocketNotice(queued queuedSocketNotice) {
	if queued.clearContexts {
		transport.removeExecutionContexts(queued.generation, 0, true, queued.observedAt)
		return
	}
	if queued.destroyContext {
		transport.removeExecutionContexts(
			queued.generation, queued.executionContextID, false, queued.observedAt,
		)
		return
	}
	var notice chromiumSocketNotice
	if json.Unmarshal([]byte(queued.payload), &notice) != nil || notice.Version != 1 {
		return
	}
	notice.Token = strings.TrimSpace(notice.Token)
	if notice.Token == "" || notice.Sequence == 0 {
		return
	}
	key := chromiumSocketKey{executionContextID: queued.executionContextID, token: notice.Token}
	switch notice.Type {
	case "created":
		transport.registerSocket(queued.generation, key, notice)
	case "frame":
		transport.processSocketFrame(queued.generation, key, notice, queued.observedAt)
	case "closed":
		transport.removeSocket(
			queued.generation, key, notice.Sequence, "disconnected", "Game websocket closed",
			notice.WasClean && (notice.CloseCode == 1000 || notice.CloseCode == 1001), queued.observedAt,
		)
	case "error":
		transport.removeSocket(
			queued.generation, key, notice.Sequence, "error", "Game websocket error", false, queued.observedAt,
		)
	}
}

func (transport *ChromiumTransport) registerSocket(
	generation uint64,
	key chromiumSocketKey,
	notice chromiumSocketNotice,
) {
	if !isGameSocketURL(notice.URL) {
		return
	}
	transport.mu.Lock()
	if transport.generation != generation || transport.cancel == nil {
		transport.mu.Unlock()
		return
	}
	if _, closed := transport.closedSockets[key]; closed {
		transport.mu.Unlock()
		return
	}
	if _, exists := transport.sockets[key]; exists {
		transport.mu.Unlock()
		return
	}
	transport.nextSocketOrdinal++
	transport.sockets[key] = &chromiumSocket{
		serverURL: notice.URL, ordinal: transport.nextSocketOrdinal, lastSequence: notice.Sequence,
	}
	transport.mu.Unlock()
}

func (transport *ChromiumTransport) processSocketFrame(
	generation uint64,
	key chromiumSocketKey,
	notice chromiumSocketNotice,
	observedAt time.Time,
) {
	if notice.Direction != Protocol.DirectionInbound && notice.Direction != Protocol.DirectionOutbound {
		return
	}
	transport.mu.Lock()
	socket := transport.sockets[key]
	if transport.generation != generation || transport.cancel == nil || socket == nil ||
		notice.Sequence <= socket.lastSequence {
		transport.mu.Unlock()
		return
	}
	socket.lastSequence = notice.Sequence
	transport.mu.Unlock()

	frame, frameErr := Protocol.Decode(notice.Payload, notice.Direction, observedAt)
	if notice.Direction == Protocol.DirectionOutbound && frameErr == nil {
		switch frame.Opcode {
		case "vck":
			transport.recordSocketVersionFrame(generation, key, notice.Payload)
		case "lli":
			transport.recordSocketLoginFrame(
				generation, key, notice.Payload, notice.LoginUsername, notice.LoginPassword,
			)
		}
	}
	if notice.Direction == Protocol.DirectionInbound {
		if frameErr == nil && frame.Opcode == "lli" && frame.ResponseCode != nil {
			if *frame.ResponseCode == 0 {
				connectionGeneration, activated, activationErr := transport.activateSocket(
					generation, key, notice.Sequence, observedAt,
				)
				if activated {
					transport.rememberSuccessfulLogin(generation, key, observedAt)
					transport.deliverSocketFrame(notice, observedAt, connectionGeneration)
				} else if activationErr != nil {
					transport.failSocketActivation(
						generation, key, notice.Sequence, activationErr, observedAt,
					)
				}
				return
			}
			transport.rejectRestoredLogin(generation, key, *frame.ResponseCode, observedAt)
			if transport.deactivateSocket(generation, key) ||
				transport.socketMayPublishLoginFailure(generation, key) {
				transport.publishLoginFailure(generation, frame, observedAt)
			}
			return
		}
	}
	if connectionGeneration, active := transport.activeSocketGeneration(generation, key); active {
		if notice.Direction == Protocol.DirectionInbound {
			transport.recordInboundTraffic(generation, key, observedAt)
		}
		transport.deliverSocketFrame(notice, observedAt, connectionGeneration)
	}
}

func (transport *ChromiumTransport) recordSocketVersionFrame(
	generation uint64,
	key chromiumSocketKey,
	frame string,
) {
	namespace, build, platform, ok := captureGameVersionFrame(frame)
	if !ok {
		return
	}
	transport.mu.Lock()
	defer transport.mu.Unlock()
	socket := transport.sockets[key]
	if transport.generation != generation || transport.cancel == nil || socket == nil {
		return
	}
	socket.clientBuild = build
	socket.platform = platform
	if transport.status.Namespace == "" {
		transport.status.Namespace = namespace
	}
}

func (transport *ChromiumTransport) recordInboundTraffic(
	generation uint64,
	key chromiumSocketKey,
	observedAt time.Time,
) {
	transport.mu.Lock()
	defer transport.mu.Unlock()
	if transport.generation != generation || transport.cancel == nil ||
		transport.activeSocket != key || !transport.status.LoggedIn ||
		!transport.status.SocketReady || observedAt.Before(transport.lastInboundAt) {
		return
	}
	transport.lastInboundAt = observedAt
}

func (transport *ChromiumTransport) recordSocketLoginFrame(
	generation uint64,
	key chromiumSocketKey,
	frame string,
	username string,
	password string,
) {
	decoded, err := Protocol.Decode(frame, Protocol.DirectionOutbound, time.Now().UTC())
	if err != nil || decoded.Opcode != "lli" || !strings.HasPrefix(decoded.Namespace, "EmpireEx_") {
		return
	}
	transport.mu.Lock()
	defer transport.mu.Unlock()
	socket := transport.sockets[key]
	if transport.generation != generation || transport.cancel == nil || socket == nil {
		return
	}
	socket.pendingLoginFrame = frame
	socket.loginContext = sanitizedGameLoginContext(decoded.Payload)
	credential := persistedLoginCredential{Username: username, Password: password}
	if validateLoginCredential(credential) == nil {
		socket.loginUsername = username
		socket.loginPassword = password
	}
}

func (transport *ChromiumTransport) rememberSuccessfulLogin(
	generation uint64,
	key chromiumSocketKey,
	observedAt time.Time,
) {
	transport.mu.Lock()
	socket := transport.sockets[key]
	if transport.generation != generation || transport.cancel == nil || socket == nil ||
		transport.activeSocket != key || socket.pendingLoginFrame == "" {
		transport.mu.Unlock()
		return
	}
	credential := transport.loginCredential
	newCredential := persistedLoginCredential{
		SchemaVersion: loginCredentialSchemaVersion, CapturedAt: observedAt, AutoRestore: true,
		Username: socket.loginUsername, Password: socket.loginPassword,
	}
	saveCredential := validateLoginCredential(newCredential) == nil
	if saveCredential {
		credential = newCredential
		transport.loginCredential = credential
	}
	profile := gameConnectionProfile{
		SchemaVersion: gameConnectionProfileSchemaVersion,
		CapturedAt:    observedAt,
		ServerURL:     socket.serverURL,
		Namespace:     transport.status.Namespace,
		ClientBuild:   socket.clientBuild,
		Platform:      socket.platform,
		LoginContext:  socket.loginContext,
	}
	saveProfile := validateGameConnectionProfile(profile) == nil &&
		validateLoginCredential(credential) == nil && credential.AutoRestore
	transport.restoreSuppressed = false
	dataDir := transport.config.DataDir
	transport.mu.Unlock()
	if saveCredential {
		_ = saveLoginCredential(dataDir, credential)
	}
	if saveProfile {
		_ = saveGameConnectionProfile(dataDir, profile)
	}
}

func (transport *ChromiumTransport) rejectRestoredLogin(
	generation uint64,
	key chromiumSocketKey,
	responseCode int,
	observedAt time.Time,
) {
	if responseCode == 453 {
		return
	}
	transport.mu.Lock()
	socket := transport.sockets[key]
	if transport.generation != generation || transport.cancel == nil || socket == nil ||
		!transport.restoreAttempted || socket.pendingLoginFrame == "" {
		transport.mu.Unlock()
		return
	}
	transport.restoreSuppressed = true
	transport.restoreAttempted = false
	credential := transport.loginCredential
	credential.AutoRestore = false
	credential.CapturedAt = observedAt
	transport.loginCredential = credential
	dataDir := transport.config.DataDir
	transport.mu.Unlock()
	_ = saveLoginCredential(dataDir, credential)
}

func (transport *ChromiumTransport) activateSocket(
	generation uint64,
	key chromiumSocketKey,
	sequence uint64,
	observedAt time.Time,
) (uint64, bool, error) {
	runContext := transport.runContext(generation)
	if runContext == nil || transport.acquireSendGate(runContext) != nil {
		return 0, false, nil
	}
	defer transport.releaseSendGate()
	transport.mu.RLock()
	socket := transport.sockets[key]
	if transport.generation != generation || transport.cancel == nil || socket == nil ||
		sequence <= socket.activatedSequence || transport.activeOrdinal > socket.ordinal ||
		transport.socketGeneration == ^uint64(0) {
		transport.mu.RUnlock()
		return 0, false, nil
	}
	gameContext := transport.gameContext
	serverURL := socket.serverURL
	ordinal := socket.ordinal
	connectionGeneration := transport.socketGeneration + 1
	evaluator := transport.activationEvaluator
	transport.mu.RUnlock()

	var activated bool
	var err error
	evaluationContext, cancelEvaluation := context.WithTimeout(gameContext, browserEvaluationTimeout)
	defer cancelEvaluation()
	if evaluator != nil {
		activated, err = evaluator(evaluationContext, key.executionContextID, key.token, connectionGeneration)
	} else {
		activated, err = transport.evaluateSocketActivation(
			evaluationContext, key.executionContextID, key.token, connectionGeneration,
		)
	}
	if err != nil || !activated {
		if err == nil {
			err = fmt.Errorf("game websocket activation was rejected")
		}
		return 0, false, err
	}

	transport.mu.Lock()
	socket = transport.sockets[key]
	if transport.generation != generation || transport.cancel == nil || socket == nil ||
		socket.ordinal != ordinal || sequence <= socket.activatedSequence ||
		transport.activeOrdinal > socket.ordinal {
		transport.mu.Unlock()
		return 0, false, nil
	}
	transport.socketGeneration = connectionGeneration
	transport.activeSocket = key
	transport.activeToken = key.token
	transport.activeOrdinal = socket.ordinal
	transport.executionContextID = key.executionContextID
	transport.lastInboundAt = observedAt
	socket.activatedSequence = sequence
	transport.mu.Unlock()

	transport.publishSocketActivation(socketActivation{
		serverURL: serverURL, connectionGeneration: connectionGeneration, changedAt: observedAt,
	})
	return connectionGeneration, true, nil
}

func (transport *ChromiumTransport) evaluateSocketActivation(
	ctx context.Context,
	executionContextID runtime.ExecutionContextID,
	token string,
	connectionGeneration uint64,
) (bool, error) {
	if ctx == nil {
		return false, fmt.Errorf("game websocket context is unavailable")
	}
	tokenLiteral, err := json.Marshal(token)
	if err != nil {
		return false, err
	}
	epochLiteral, err := json.Marshal(strconv.FormatUint(connectionGeneration, 10))
	if err != nil {
		return false, err
	}
	expression := fmt.Sprintf(
		"globalThis.__citadelActivate && globalThis.__citadelActivate(%s, %s)",
		tokenLiteral, epochLiteral,
	)
	activated := false
	err = chromedp.Run(ctx, chromedp.ActionFunc(func(ctx context.Context) error {
		evaluation := websocketRuntimeEvaluation(expression)
		if executionContextID != 0 {
			evaluation = evaluation.WithContextID(executionContextID)
		}
		result, exception, evaluateErr := evaluation.Do(ctx)
		if evaluateErr != nil {
			return evaluateErr
		}
		if exception != nil {
			return fmt.Errorf("game websocket activation: %s", exception.Text)
		}
		if result != nil {
			_ = json.Unmarshal(result.Value, &activated)
		}
		return nil
	}))
	return activated, err
}

func (transport *ChromiumTransport) scheduleSocketResync(
	generation uint64,
	executionContextID runtime.ExecutionContextID,
) {
	transport.mu.RLock()
	gameContext := transport.gameContext
	eligible := transport.generation == generation && transport.cancel != nil &&
		gameContext != nil
	transport.mu.RUnlock()
	if !eligible {
		return
	}
	go func() {
		deadline := time.NewTimer(socketResyncTimeout)
		defer deadline.Stop()
		ticker := time.NewTicker(socketResyncPoll)
		defer ticker.Stop()
		for {
			select {
			case <-gameContext.Done():
				return
			case <-deadline.C:
				return
			case <-ticker.C:
			}
			resynced, finished := transport.resyncExecutionContext(
				generation, executionContextID,
			)
			if finished || resynced >= 0 {
				return
			}
		}
	}()
}

func (transport *ChromiumTransport) resyncExecutionContext(
	generation uint64,
	executionContextID runtime.ExecutionContextID,
) (int, bool) {
	transport.mu.RLock()
	gameContext := transport.gameContext
	current := transport.generation == generation && transport.cancel != nil &&
		gameContext != nil
	transport.mu.RUnlock()
	if !current {
		return -1, true
	}
	evaluationContext, cancelEvaluation := context.WithTimeout(
		gameContext, browserEvaluationTimeout,
	)
	defer cancelEvaluation()
	resynced := -1
	err := chromedp.Run(evaluationContext, chromedp.ActionFunc(func(ctx context.Context) error {
		result, exception, err := runtime.Evaluate(
			`typeof globalThis.__citadelResync === "function"
				? globalThis.__citadelResync()
				: -1`,
		).WithContextID(executionContextID).
			WithReturnByValue(true).
			WithAwaitPromise(true).
			Do(ctx)
		if err != nil {
			return err
		}
		if exception != nil {
			return fmt.Errorf("resync game execution context: %s", exception.Text)
		}
		if result != nil {
			_ = json.Unmarshal(result.Value, &resynced)
		}
		return nil
	}))
	if err != nil {
		return -1, false
	}
	return resynced, false
}

func (transport *ChromiumTransport) scheduleCredentialRestore(
	generation uint64,
	executionContextID runtime.ExecutionContextID,
) {
	transport.mu.RLock()
	gameContext := transport.gameContext
	eligible := transport.generation == generation && transport.cancel != nil &&
		!transport.restoreSuppressed && transport.loginCredential.AutoRestore &&
		transport.loginCredential.Username != "" && transport.loginCredential.Password != ""
	transport.mu.RUnlock()
	if !eligible || gameContext == nil {
		return
	}
	go func() {
		deadline := time.NewTimer(authenticationTimeout)
		defer deadline.Stop()
		ticker := time.NewTicker(credentialRestorePoll)
		defer ticker.Stop()
		for {
			if transport.restoreCredentialsInContext(generation, executionContextID) {
				return
			}
			select {
			case <-gameContext.Done():
				return
			case <-deadline.C:
				return
			case <-ticker.C:
			}
		}
	}()
}

func (transport *ChromiumTransport) restoreCredentialsInContext(
	generation uint64,
	executionContextID runtime.ExecutionContextID,
) bool {
	transport.mu.RLock()
	if transport.generation != generation || transport.cancel == nil ||
		transport.status.LoggedIn || transport.restoreSuppressed || transport.restoreAttempted ||
		!transport.loginCredential.AutoRestore || transport.loginCredential.Username == "" ||
		transport.loginCredential.Password == "" || transport.gameContext == nil {
		transport.mu.RUnlock()
		return true
	}
	gameContext := transport.gameContext
	credential := transport.loginCredential
	evaluator := transport.credentialRestoreEvaluator
	submitter := transport.credentialSubmitEvaluator
	transport.mu.RUnlock()

	evaluationContext, cancelEvaluation := context.WithTimeout(gameContext, browserEvaluationTimeout)
	defer cancelEvaluation()
	var filled bool
	var err error
	if evaluator != nil {
		filled, err = evaluator(
			evaluationContext, executionContextID, credential.Username, credential.Password,
		)
	} else {
		filled, err = transport.evaluateCredentialRestore(
			evaluationContext, executionContextID, credential.Username, credential.Password,
		)
	}
	if err != nil || !filled {
		return false
	}
	transport.mu.Lock()
	if transport.generation != generation || transport.cancel == nil ||
		transport.status.LoggedIn || transport.restoreSuppressed || transport.restoreAttempted {
		transport.mu.Unlock()
		return true
	}
	transport.restoreAttempted = true
	transport.mu.Unlock()

	timer := time.NewTimer(100 * time.Millisecond)
	defer timer.Stop()
	select {
	case <-evaluationContext.Done():
		return true
	case <-timer.C:
	}
	if submitter != nil {
		err = submitter(evaluationContext)
	} else {
		err = chromedp.Run(evaluationContext, chromedp.KeyEvent(kb.Enter))
	}
	if err != nil {
		status := transport.Status()
		if !status.LoggedIn {
			status.Detail = "Saved game login could not be submitted; waiting for manual login"
			status.ChangedAt = time.Now().UTC()
			transport.publishStatus(status)
		}
	}
	return true
}

func (transport *ChromiumTransport) evaluateCredentialRestore(
	ctx context.Context,
	executionContextID runtime.ExecutionContextID,
	username string,
	password string,
) (bool, error) {
	usernameLiteral, err := json.Marshal(username)
	if err != nil {
		return false, err
	}
	passwordLiteral, err := json.Marshal(password)
	if err != nil {
		return false, err
	}
	expression := fmt.Sprintf(
		"globalThis.__citadelRestoreCredentials && globalThis.__citadelRestoreCredentials(%s, %s)",
		usernameLiteral, passwordLiteral,
	)
	filled := false
	err = chromedp.Run(ctx, chromedp.ActionFunc(func(ctx context.Context) error {
		evaluation := websocketRuntimeEvaluation(expression).WithContextID(executionContextID)
		result, exception, evaluateErr := evaluation.Do(ctx)
		if evaluateErr != nil {
			return evaluateErr
		}
		if exception != nil {
			return fmt.Errorf("restore game credentials: %s", exception.Text)
		}
		if result != nil {
			_ = json.Unmarshal(result.Value, &filled)
		}
		return nil
	}))
	return filled, err
}

func (transport *ChromiumTransport) activeSocketGeneration(
	generation uint64,
	key chromiumSocketKey,
) (uint64, bool) {
	transport.mu.RLock()
	defer transport.mu.RUnlock()
	return transport.socketGeneration,
		transport.generation == generation && transport.cancel != nil && transport.activeSocket == key
}

func (transport *ChromiumTransport) deliverSocketFrame(
	notice chromiumSocketNotice,
	observedAt time.Time,
	connectionGeneration uint64,
) {
	if transport.frames == nil {
		return
	}
	transport.frames <- RawFrame{
		Payload: notice.Payload, Direction: notice.Direction, ObservedAt: observedAt,
		ConnectionGeneration: connectionGeneration, ResponseToken: notice.ResponseToken,
		CausationOperationID: notice.CausationOperationID,
	}
}

func (transport *ChromiumTransport) publishSocketActivation(activation socketActivation) {
	status := transport.Status()
	status.State = "connected"
	status.LoggedIn = true
	status.SocketReady = true
	status.ConnectionGeneration = activation.connectionGeneration
	status.ServerURL = activation.serverURL
	status.Detail = ""
	status.CooldownUntil = nil
	status.RetryAt = nil
	status.ChangedAt = activation.changedAt
	transport.publishStatus(status)
}

func (transport *ChromiumTransport) deactivateSocket(
	generation uint64,
	key chromiumSocketKey,
) bool {
	runContext := transport.runContext(generation)
	if runContext == nil || transport.acquireSendGate(runContext) != nil {
		return false
	}
	defer transport.releaseSendGate()
	transport.mu.Lock()
	defer transport.mu.Unlock()
	if transport.generation != generation || transport.cancel == nil || transport.activeSocket != key {
		return false
	}
	transport.clearActiveSocketLocked()
	return true
}

func (transport *ChromiumTransport) socketMayPublishLoginFailure(
	generation uint64,
	key chromiumSocketKey,
) bool {
	transport.mu.RLock()
	defer transport.mu.RUnlock()
	if transport.generation != generation || transport.cancel == nil || transport.sockets[key] == nil {
		return false
	}
	return transport.activeToken == "" || transport.activeSocket == key
}

func (transport *ChromiumTransport) failSocketActivation(
	generation uint64,
	key chromiumSocketKey,
	sequence uint64,
	activationErr error,
	observedAt time.Time,
) {
	runContext := transport.runContext(generation)
	if runContext == nil || transport.acquireSendGate(runContext) != nil {
		return
	}
	defer transport.releaseSendGate()
	transport.mu.Lock()
	socket := transport.sockets[key]
	if transport.generation != generation || transport.cancel == nil || socket == nil ||
		socket.lastSequence != sequence {
		transport.mu.Unlock()
		return
	}
	delete(transport.sockets, key)
	transport.closedSockets[key] = sequence
	if transport.activeSocket == key {
		transport.clearActiveSocketLocked()
	}
	hasActive := transport.activeToken != ""
	transport.mu.Unlock()
	if hasActive {
		return
	}
	status := transport.Status()
	status.State = "error"
	status.LoggedIn = false
	status.SocketReady = false
	status.Detail = fmt.Sprintf("Game websocket activation failed: %v", activationErr)
	status.ChangedAt = observedAt
	transport.publishStatus(status)
}

func (transport *ChromiumTransport) removeSocket(
	generation uint64,
	key chromiumSocketKey,
	sequence uint64,
	state string,
	detail string,
	cleanClose bool,
	observedAt time.Time,
) {
	runContext := transport.runContext(generation)
	if runContext == nil || transport.acquireSendGate(runContext) != nil {
		return
	}
	defer transport.releaseSendGate()
	transport.mu.Lock()
	socket := transport.sockets[key]
	if transport.generation != generation || transport.cancel == nil || socket == nil ||
		sequence <= socket.lastSequence {
		transport.mu.Unlock()
		return
	}
	socket.lastSequence = sequence
	delete(transport.sockets, key)
	transport.closedSockets[key] = sequence
	wasActive := transport.activeSocket == key
	if wasActive {
		transport.clearActiveSocketLocked()
	}
	remaining := len(transport.sockets)
	intentionalLogout := wasActive && cleanClose && transport.status.LoggedIn
	transport.mu.Unlock()
	if wasActive {
		if intentionalLogout {
			transport.disableLoginRestore(observedAt)
		}
		status := transport.Status()
		wasLoggedIn := status.LoggedIn
		status.State = state
		if intentionalLogout {
			status.State = "reconnecting"
			status.Detail = "Game logout detected; waiting for account selection"
		} else if remaining > 0 {
			status.State = "connecting"
			status.Detail = detail
		} else {
			status.Detail = detail
		}
		status.LoggedIn = false
		status.SocketReady = false
		status.RetryAt = nil
		status.ChangedAt = observedAt
		transport.publishStatus(status)
		if intentionalLogout {
			transport.scheduleLogoutReload(generation)
		} else if wasLoggedIn && remaining == 0 {
			transport.scheduleSocketReconnect(generation, status.ConnectionGeneration)
		}
	}
}

func (transport *ChromiumTransport) removeExecutionContexts(
	generation uint64,
	executionContextID runtime.ExecutionContextID,
	clearAll bool,
	observedAt time.Time,
) {
	runContext := transport.runContext(generation)
	if runContext == nil || transport.acquireSendGate(runContext) != nil {
		return
	}
	defer transport.releaseSendGate()
	transport.mu.Lock()
	if transport.generation != generation || transport.cancel == nil {
		transport.mu.Unlock()
		return
	}
	wasActive := false
	for key, socket := range transport.sockets {
		if !clearAll && key.executionContextID != executionContextID {
			continue
		}
		if transport.activeSocket == key {
			wasActive = true
		}
		transport.closedSockets[key] = socket.lastSequence
		delete(transport.sockets, key)
	}
	if wasActive {
		transport.clearActiveSocketLocked()
	}
	remaining := len(transport.sockets)
	wasLoggedIn := wasActive && transport.status.LoggedIn
	transport.mu.Unlock()
	if wasActive {
		transport.publishSocketLossAt(
			"disconnected", "Game execution context closed", remaining, observedAt,
		)
		if wasLoggedIn && remaining == 0 {
			status := transport.Status()
			transport.scheduleSocketReconnect(generation, status.ConnectionGeneration)
		}
	}
}

func (transport *ChromiumTransport) disableLoginRestore(observedAt time.Time) {
	transport.mu.Lock()
	transport.restoreSuppressed = true
	transport.restoreAttempted = false
	backgroundModePrepared := transport.backgroundModePrepared
	credential := transport.loginCredential
	if credential.Username == "" || credential.Password == "" {
		transport.mu.Unlock()
		return
	}
	if backgroundModePrepared {
		transport.mu.Unlock()
		return
	}
	credential.AutoRestore = false
	credential.CapturedAt = observedAt
	transport.loginCredential = credential
	dataDir := transport.config.DataDir
	transport.mu.Unlock()
	_ = saveLoginCredential(dataDir, credential)
}

func (transport *ChromiumTransport) scheduleLogoutReload(generation uint64) {
	transport.mu.RLock()
	gameContext := transport.gameContext
	transport.mu.RUnlock()
	if gameContext == nil {
		return
	}
	go func() {
		timer := time.NewTimer(logoutReloadDelay)
		defer timer.Stop()
		select {
		case <-gameContext.Done():
			return
		case <-timer.C:
		}
		transport.mu.RLock()
		current := transport.generation == generation && transport.cancel != nil &&
			transport.restoreSuppressed && !transport.status.LoggedIn
		transport.mu.RUnlock()
		if !current {
			return
		}
		status := transport.Status()
		if err := transport.reloadGame(gameContext); err != nil {
			transport.publishReloadFailure(status, "Reload game after logout", err)
		}
	}()
}

func (transport *ChromiumTransport) invalidateSocketNoticeStream(
	generation uint64,
	observedAt time.Time,
) {
	runContext := transport.runContext(generation)
	if runContext == nil || transport.acquireSendGate(runContext) != nil {
		return
	}
	defer transport.releaseSendGate()
	transport.mu.Lock()
	if transport.generation != generation || transport.cancel == nil {
		transport.mu.Unlock()
		return
	}
	transport.sockets = map[chromiumSocketKey]*chromiumSocket{}
	transport.closedSockets = map[chromiumSocketKey]uint64{}
	transport.clearActiveSocketLocked()
	transport.mu.Unlock()
	status := transport.Status()
	status.State = "error"
	status.LoggedIn = false
	status.SocketReady = false
	status.Detail = "Game websocket event queue overflowed"
	status.ChangedAt = observedAt
	transport.publishStatus(status)
}

func (transport *ChromiumTransport) removeNetworkSocket(
	requestID network.RequestID,
) (removed bool, noActive bool, remaining int) {
	transport.mu.Lock()
	defer transport.mu.Unlock()
	if _, exists := transport.trackedSockets[requestID]; !exists {
		return false, transport.activeToken == "", len(transport.trackedSockets)
	}
	delete(transport.trackedSockets, requestID)
	return true, transport.activeToken == "", len(transport.trackedSockets)
}

func (transport *ChromiumTransport) clearActiveSocketLocked() {
	transport.activeSocket = chromiumSocketKey{}
	transport.activeToken = ""
	transport.activeOrdinal = 0
	transport.executionContextID = 0
	transport.lastInboundAt = time.Time{}
}

func (transport *ChromiumTransport) resetSocketsLocked() {
	transport.executionContextID = 0
	transport.trackedSockets = map[network.RequestID]string{}
	transport.sockets = map[chromiumSocketKey]*chromiumSocket{}
	transport.closedSockets = map[chromiumSocketKey]uint64{}
	transport.activeSocket = chromiumSocketKey{}
	transport.activeToken = ""
	transport.activeOrdinal = 0
	transport.nextSocketOrdinal = 0
	transport.lastInboundAt = time.Time{}
}

func (transport *ChromiumTransport) publishSocketLoss(
	state string,
	detail string,
	remaining int,
) {
	transport.publishSocketLossAt(state, detail, remaining, time.Now().UTC())
}

func (transport *ChromiumTransport) publishSocketLossAt(
	state string,
	detail string,
	remaining int,
	observedAt time.Time,
) {
	status := transport.Status()
	status.State = state
	if remaining > 0 {
		status.State = "connecting"
	}
	status.LoggedIn = false
	status.SocketReady = false
	status.Detail = detail
	status.ChangedAt = observedAt
	transport.publishStatus(status)
}

func (transport *ChromiumTransport) clearRun(generation uint64) bool {
	transport.mu.Lock()
	defer transport.mu.Unlock()
	if transport.generation != generation {
		return false
	}
	transport.cancel = nil
	transport.browserContext = nil
	transport.gameContext = nil
	transport.gameCancel = nil
	transport.gameTargetID = ""
	transport.resetSocketsLocked()
	return true
}

func isGameSocketURL(url string) bool {
	return strings.Contains(strings.ToLower(url), "ep-live")
}

func isGameLoginOrigin(origin string) bool {
	parsed, err := url.Parse(strings.TrimSpace(origin))
	if err != nil {
		return false
	}
	host := strings.ToLower(parsed.Hostname())
	return host == "empire.goodgamestudios.com" ||
		host == "empire-html5.goodgamestudios.com"
}

func isGamePageURL(value string) bool {
	parsed, err := url.Parse(strings.TrimSpace(value))
	if err != nil {
		return false
	}
	return (parsed.Scheme == "http" || parsed.Scheme == "https") &&
		isGameLoginOrigin(parsed.Scheme+"://"+parsed.Host)
}

const httpSwitchingProtocols = 101

const chromiumTransportInjection = `
(() => {
  const install = (root) => {
    if (!root || !root.WebSocket) return null;
    if (root.__citadelTransportInstalled) return root.__citadelTransportController || null;
    root.__citadelTransportInstalled = true;
    const NativeWebSocket = root.WebSocket;
    const sockets = new Map();
    let tokenCounter = 0;
    let frontendCausationOperationId = '';
    const notify = (message) => {
      try {
        if (typeof root.citadelTransportNotify === 'function') {
          root.citadelTransportNotify(JSON.stringify(message));
        } else if (typeof root.__citadelWorkerNotify === 'function') {
          root.__citadelWorkerNotify(message);
        }
      } catch (_) {}
    };
    const makeToken = () => {
      if (root.crypto && typeof root.crypto.randomUUID === 'function') {
        return root.crypto.randomUUID();
      }
      if (root.crypto && typeof root.crypto.getRandomValues === 'function') {
        const bytes = new Uint8Array(16);
        root.crypto.getRandomValues(bytes);
        return Array.from(bytes, (value) => value.toString(16).padStart(2, '0')).join('');
      }
      tokenCounter += 1;
      return Date.now().toString(36) + '-' + tokenCounter.toString(36) + '-' + Math.random().toString(36).slice(2);
    };
    const toText = async (data) => {
      if (typeof data === 'string') return data;
      if (typeof Blob !== 'undefined' && data instanceof Blob) return await data.text();
      if (data instanceof ArrayBuffer) return new TextDecoder().decode(new Uint8Array(data));
      if (ArrayBuffer.isView(data)) {
        return new TextDecoder().decode(new Uint8Array(data.buffer, data.byteOffset, data.byteLength));
      }
      return null;
    };
    const opcodeOf = (payload) => {
      if (typeof payload !== 'string') return '';
      const parts = payload.split('%');
      if (parts.length <= 4) return '';
      return String(parts[2] || '').startsWith('EmpireEx_')
        ? String(parts[3] || '').toLowerCase()
        : String(parts[2] || '').toLowerCase();
    };
    const payloadObjectOf = (payload) => {
      if (typeof payload !== 'string') return null;
      const parts = payload.split('%');
      if (parts.length <= 5 || !parts[5]) return null;
      try {
        return JSON.parse(parts[5]);
      } catch (_) {
        return null;
      }
    };
    const loginCredentialCandidate = { username: '', password: '' };
    const updateLoginCredential = (element) => {
      try {
        if (!element || String(element.tagName || '').toLowerCase() !== 'input') return;
        const autocomplete = String(element.getAttribute('autocomplete') || '').toLowerCase();
        if (autocomplete === 'username') {
          loginCredentialCandidate.username = String(element.value || '');
        } else if (autocomplete === 'current-password') {
          loginCredentialCandidate.password = String(element.value || '');
        }
      } catch (_) {}
    };
    if (root.document && typeof root.document.addEventListener === 'function') {
      root.document.addEventListener('input', (event) => updateLoginCredential(event.target), true);
      root.document.addEventListener('change', (event) => updateLoginCredential(event.target), true);
    }
    const loginCredentials = () => {
      try {
        if (root.document) {
          updateLoginCredential(root.document.querySelector('input[autocomplete="username"]'));
          updateLoginCredential(root.document.querySelector('input[autocomplete="current-password"]'));
        }
      } catch (_) {}
      return {
        username: loginCredentialCandidate.username,
        password: loginCredentialCandidate.password
      };
    };
    const visibleLoginInput = (autocomplete) => {
      if (!root.document) return null;
      for (const element of root.document.querySelectorAll('input[autocomplete="' + autocomplete + '"]')) {
        const rect = element.getBoundingClientRect();
        const style = root.getComputedStyle ? root.getComputedStyle(element) : null;
        if (rect.width > 0 && rect.height > 0 &&
            (!style || (style.display !== 'none' && style.visibility !== 'hidden'))) return element;
      }
      return null;
    };
    const setLoginInputValue = (element, value) => {
      if (!element) return false;
      try {
        const prototype = root.HTMLInputElement && root.HTMLInputElement.prototype;
        const descriptor = prototype && Object.getOwnPropertyDescriptor(prototype, 'value');
        if (descriptor && typeof descriptor.set === 'function') {
          descriptor.set.call(element, value);
        } else {
          element.value = value;
        }
        element.dispatchEvent(new root.Event('input', { bubbles: true }));
        element.dispatchEvent(new root.Event('change', { bubbles: true }));
        return true;
      } catch (_) {
        return false;
      }
    };
    const restoreCredentials = (username, password) => {
      const usernameInput = visibleLoginInput('username');
      const passwordInput = visibleLoginInput('current-password');
      if (!usernameInput || !passwordInput) return false;
      loginCredentialCandidate.username = String(username || '');
      loginCredentialCandidate.password = String(password || '');
      if (!loginCredentialCandidate.username || !loginCredentialCandidate.password ||
          !setLoginInputValue(usernameInput, loginCredentialCandidate.username) ||
          !setLoginInputValue(passwordInput, loginCredentialCandidate.password)) return false;
      try {
        passwordInput.focus();
      } catch (_) {}
      return true;
    };
    const manualResponseOpcode = (opcode) => opcode === 'jca' ? 'jaa' : opcode;
    const queueResponse = (record, payload, request) => {
      const opcode = opcodeOf(payload);
      const supplied = request && Array.isArray(request.opcodes)
        ? request.opcodes.map((value) => String(value || '').toLowerCase()).filter(Boolean)
        : [];
      const opcodes = supplied.length > 0 ? supplied : (opcode ? [manualResponseOpcode(opcode)] : []);
      if (opcodes.length === 0) return null;
      const timeoutMillis = request && Number.isFinite(request.timeoutMillis) && request.timeoutMillis > 0
        ? request.timeoutMillis : 30000;
      const pending = {
        token: request ? String(request.token || '') : '', opcodes,
        requestOpcode: opcode, requestPayload: payloadObjectOf(payload),
        expiresAt: Date.now() + timeoutMillis + 1000
      };
      record.pendingResponses.push(pending);
      return pending;
    };
    const responseMatchesRequest = (pending, opcode, payload) => {
      if (pending && pending.requestOpcode === 'gaa' && opcode === 'gaa') {
        const requestPayload = pending.requestPayload;
        const responsePayload = payloadObjectOf(payload);
        const responseParts = String(payload || '').split('%');
        const responseCode = responseParts.length > 4 ? Number(responseParts[4]) : 0;
        if (Number.isFinite(responseCode) && responseCode !== 0) return true;
        if (!requestPayload || !responsePayload) return false;
        const hasKingdom = Object.prototype.hasOwnProperty.call(responsePayload, 'KID');
        if (hasKingdom && Number(responsePayload.KID) !== Number(requestPayload.KID)) return false;
        const rows = Array.isArray(responsePayload.AI) ? responsePayload.AI : [];
        const coordinates = rows.filter((row) =>
          Array.isArray(row) && Number.isFinite(Number(row[1])) && Number.isFinite(Number(row[2])));
        if (coordinates.length === 0) {
          return hasKingdom && Number(responsePayload.KID) === Number(requestPayload.KID);
        }
        return coordinates.every((row) =>
          Number(row[1]) >= Number(requestPayload.AX1) &&
          Number(row[1]) <= Number(requestPayload.AX2) &&
          Number(row[2]) >= Number(requestPayload.AY1) &&
          Number(row[2]) <= Number(requestPayload.AY2));
      }
      if (!pending || pending.requestOpcode !== 'ahr' || opcode !== 'ahh') return true;
      const requestPayload = pending.requestPayload;
      const responsePayload = payloadObjectOf(payload);
      if (!requestPayload || !responsePayload || Number(responsePayload.TID) !== Number(requestPayload.T)) {
        return false;
      }
      if (Number(requestPayload.T) !== 2) return true;
      return responsePayload.OP && Number(responsePayload.OP.RID) === Number(requestPayload.ID);
    };
    const matchResponse = (record, opcode, payload) => {
      const now = Date.now();
      record.pendingResponses = record.pendingResponses.filter((pending) => pending.expiresAt > now);
      let index = -1;
      const responsePayload = opcode === 'gaa' ? payloadObjectOf(payload) : null;
      const responseRows = responsePayload && Array.isArray(responsePayload.AI) ? responsePayload.AI : [];
      if (opcode === 'gaa' && responseRows.length > 0) {
        let smallestArea = Number.POSITIVE_INFINITY;
        record.pendingResponses.forEach((pending, candidateIndex) => {
          if (!pending.opcodes.includes(opcode) || !responseMatchesRequest(pending, opcode, payload)) return;
          const requestPayload = pending.requestPayload;
          if (!requestPayload) return;
          const width = Number(requestPayload.AX2) - Number(requestPayload.AX1) + 1;
          const height = Number(requestPayload.AY2) - Number(requestPayload.AY1) + 1;
          const area = width > 0 && height > 0 ? width * height : Number.POSITIVE_INFINITY;
          if (area < smallestArea) {
            index = candidateIndex;
            smallestArea = area;
          }
        });
      }
      if (index < 0) {
        index = record.pendingResponses.findIndex((pending) =>
          pending.opcodes.includes(opcode) && responseMatchesRequest(pending, opcode, payload));
      }
      if (index < 0) return '';
      const [pending] = record.pendingResponses.splice(index, 1);
      return pending.token;
    };
    const schedule = (record, work) => {
      record.events = record.events.then(work).catch(() => {});
    };
    const emitFrame = (record, direction, data, causationOperationId) => {
      schedule(record, async () => {
        const payload = await toText(data);
        if (payload === null) return;
        const opcode = opcodeOf(payload);
        let responseToken = '';
        if (direction === 'inbound') {
          const parts = payload.split('%');
          responseToken = matchResponse(record, opcode, payload);
          if (parts.length > 4 && opcode === 'lli') {
            record.eligible = parts[4] === '0';
            if (!record.eligible) record.epoch = '';
          }
        }
        const credential = direction === 'outbound' && opcode === 'lli'
          ? loginCredentials() : { username: '', password: '' };
        record.sequence += 1;
        notify({
          v: 1, type: 'frame', token: record.token, seq: record.sequence,
          direction, payload, responseToken,
          causationOperationId: direction === 'outbound' ? String(causationOperationId || '') : '',
          loginUsername: credential.username, loginPassword: credential.password
        });
      });
    };
    const emitLifecycle = (record, type, event) => {
      schedule(record, () => {
        record.sequence += 1;
        notify({
          v: 1, type, token: record.token, seq: record.sequence, url: record.url,
          closeCode: event && Number.isFinite(event.code) ? event.code : 0,
          closeReason: event && typeof event.reason === 'string' ? event.reason : '',
          wasClean: Boolean(event && event.wasClean)
        });
      });
    };
    root.WebSocket = new Proxy(NativeWebSocket, {
      construct(target, args) {
        const socket = new target(...args);
        const url = args[0];
        if (typeof url === 'string' && url.toLowerCase().includes('ep-live')) {
          const record = {
            socket, url: String(url || ''), token: makeToken(), sequence: 1,
            events: Promise.resolve(), eligible: false, epoch: '', wrappedSend: false,
            pendingResponses: []
          };
          const nativeSend = socket.send.bind(socket);
          record.send = (data, request) => {
            const pending = queueResponse(record, data, request);
            const causationOperationId = request && request.causationOperationId
              ? request.causationOperationId : frontendCausationOperationId;
            let result;
            try {
              result = nativeSend(data);
            } catch (error) {
              if (pending) {
                const index = record.pendingResponses.indexOf(pending);
                if (index >= 0) record.pendingResponses.splice(index, 1);
              }
              throw error;
            }
            emitFrame(record, 'outbound', data, causationOperationId);
            return result;
          };
          try {
            Object.defineProperty(socket, 'send', { configurable: true, value: record.send });
            record.wrappedSend = true;
          } catch (_) {}
          sockets.set(record.token, record);
          notify({ v: 1, type: 'created', token: record.token, seq: record.sequence, url: record.url });
          socket.addEventListener('message', (event) => emitFrame(record, 'inbound', event.data));
          socket.addEventListener('close', (event) => {
            record.eligible = false;
            record.epoch = '';
            record.pendingResponses = [];
            sockets.delete(record.token);
            emitLifecycle(record, 'closed', event);
          }, { once: true });
          socket.addEventListener('error', () => emitLifecycle(record, 'error'), { once: true });
        }
        return socket;
      }
    });
    const activate = (token, epoch) => {
      const record = sockets.get(String(token));
      if (!record || !record.eligible || record.socket.readyState !== NativeWebSocket.OPEN) return false;
      record.epoch = String(epoch);
      return true;
    };
    const resync = () => {
      let count = 0;
      for (const record of sockets.values()) {
        if (!record || record.socket.readyState !== NativeWebSocket.OPEN) continue;
        notify({
          v: 1, type: 'created', token: record.token, seq: record.sequence, url: record.url,
          eligible: record.eligible === true
        });
        count += 1;
      }
      return count;
    };
    const send = (
      token, epoch, payload, responseToken, responseOpcodes, responseTimeoutMillis,
      causationOperationId
    ) => {
      const record = sockets.get(String(token));
      if (!record || !record.eligible || record.epoch !== String(epoch) ||
          record.socket.readyState !== NativeWebSocket.OPEN) return false;
      record.send(payload, {
        token: responseToken, opcodes: responseOpcodes, timeoutMillis: responseTimeoutMillis,
        causationOperationId
      });
      return true;
    };
    let layoutManagerClass = null;
    const closeGameUI = (causationOperationId) => {
      const previousCausationOperationId = frontendCausationOperationId;
      frontendCausationOperationId = String(causationOperationId || '');
      try {
        const require = root.ggs_lib;
        if (!layoutManagerClass && typeof require === 'function') {
          const cache = require.c;
          if (cache && typeof cache === 'object') {
            for (const id of Object.keys(cache)) {
              const exported = cache[id] && cache[id].exports;
              const candidate = exported && exported.BasicLayoutManager;
              if (candidate && typeof candidate.getInstance === 'function') {
                layoutManagerClass = candidate;
                break;
              }
            }
          }
        }
        if (!layoutManagerClass) return false;
        const manager = layoutManagerClass.layoutManager || layoutManagerClass.getInstance();
        if (!manager) return false;
        let invoked = false;
        for (const method of [
          'hideAllDialogs', 'hideAllPanels', 'hideAllAttackPanels',
          'hideAllNonPermanentUIComponents', 'hideAllRingMenus'
        ]) {
          if (typeof manager[method] !== 'function') continue;
          try {
            manager[method]();
            invoked = true;
          } catch (_) {}
        }
        return invoked;
      } catch (_) {
        return false;
      } finally {
        frontendCausationOperationId = previousCausationOperationId;
      }
    };
    const controller = {
      activate, resync, send, loginCredentials, restoreCredentials, closeGameUI, opcodeOf
    };
    root.__citadelActivate = activate;
    root.__citadelResync = resync;
    root.__citadelSend = send;
    root.__citadelRestoreCredentials = restoreCredentials;
    root.__citadelCloseGameUI = closeGameUI;
    root.__citadelTransportController = controller;
    return controller;
  };

  const pageController = install(globalThis);
  if (typeof window !== 'undefined' && window.Worker && !window.__citadelWorkerInstalled) {
    window.__citadelWorkerInstalled = true;
    const NativeWorker = window.Worker;
    const bridgeMarker = '__citadelDedicatedWorkerBridgeV1';
    const workerOwners = new Map();
    const pendingCommands = new Map();
    let commandCounter = 0;

    const workerBridge = (root, marker) => {
      const postMessage = root.postMessage.bind(root);
      root.__citadelWorkerNotify = (notice) => {
        postMessage({ marker, kind: 'notice', notice });
      };
      root.addEventListener('message', (event) => {
        const message = event && event.data;
        if (!message || message.marker !== marker || message.kind !== 'command') return;
        if (typeof event.stopImmediatePropagation === 'function') event.stopImmediatePropagation();
        const respond = (result) => {
          postMessage({ marker, kind: 'result', id: message.id, result: result === true });
        };
        try {
          let result = false;
          if (message.action === 'activate' && typeof root.__citadelActivate === 'function') {
            result = root.__citadelActivate(message.token, message.epoch);
          } else if (message.action === 'send' && typeof root.__citadelSend === 'function') {
            result = root.__citadelSend(
              message.token, message.epoch, message.payload, message.responseToken,
              message.responseOpcodes, message.responseTimeoutMillis,
              message.causationOperationId
            );
          }
          Promise.resolve(result).then(respond, () => respond(false));
        } catch (_) {
          respond(false);
        }
      });
    };

    const forwardNotice = (notice) => {
      try {
        let forwarded = notice;
        if (pageController && notice && notice.type === 'frame' &&
            notice.direction === 'outbound' && pageController.opcodeOf(notice.payload) === 'lli') {
          const credential = pageController.loginCredentials();
          forwarded = {
            ...notice,
            loginUsername: credential.username,
            loginPassword: credential.password
          };
        }
        if (typeof globalThis.citadelTransportNotify === 'function') {
          globalThis.citadelTransportNotify(JSON.stringify(forwarded));
        }
      } catch (_) {}
    };

    const cleanupWorker = (bridge) => {
      for (const [token, owner] of workerOwners) {
        if (owner.bridge !== bridge) continue;
        workerOwners.delete(token);
        forwardNotice({
          v: 1, type: 'closed', token,
          seq: Number.isSafeInteger(owner.sequence) ? owner.sequence + 1 : 1,
          url: owner.url || ''
        });
      }
      for (const [id, pending] of pendingCommands) {
        if (pending.bridge !== bridge) continue;
        pendingCommands.delete(id);
        clearTimeout(pending.timeout);
        pending.resolve(false);
      }
    };

    const attachWorker = (worker) => {
      const bridge = { worker, postMessage: worker.postMessage.bind(worker) };
      worker.addEventListener('message', (event) => {
        const message = event && event.data;
        if (!message || message.marker !== bridgeMarker ||
            (message.kind !== 'notice' && message.kind !== 'result')) return;
        if (typeof event.stopImmediatePropagation === 'function') event.stopImmediatePropagation();
        if (message.kind === 'notice') {
          const notice = message.notice;
          if (!notice || notice.v !== 1 || typeof notice.token !== 'string') return;
          if (notice.type === 'created') {
            workerOwners.set(notice.token, {
              bridge, sequence: notice.seq, url: typeof notice.url === 'string' ? notice.url : '',
              eligible: notice.eligible === true
            });
          } else {
            const owner = workerOwners.get(notice.token);
            if (owner && owner.bridge === bridge) {
              owner.sequence = notice.seq;
              if (typeof notice.url === 'string' && notice.url) owner.url = notice.url;
              if (notice.type === 'frame' && notice.direction === 'inbound' &&
                  pageController && pageController.opcodeOf(notice.payload) === 'lli') {
                const parts = String(notice.payload || '').split('%');
                owner.eligible = parts.length > 4 && parts[4] === '0';
              }
            }
          }
          forwardNotice(notice);
          if ((notice.type === 'closed' || notice.type === 'error') &&
              workerOwners.get(notice.token)?.bridge === bridge) {
            workerOwners.delete(notice.token);
          }
          return;
        }
        if (message.kind !== 'result') return;
        const pending = pendingCommands.get(message.id);
        if (!pending || pending.bridge !== bridge) return;
        pendingCommands.delete(message.id);
        clearTimeout(pending.timeout);
        pending.resolve(message.result === true);
      });
      try {
        const terminate = worker.terminate.bind(worker);
        Object.defineProperty(worker, 'terminate', {
          configurable: true,
          value: () => {
            cleanupWorker(bridge);
            return terminate();
          }
        });
      } catch (_) {}
      return bridge;
    };

    const runWorkerCommand = (
      bridge, action, token, epoch, payload, responseToken, responseOpcodes,
      responseTimeoutMillis, causationOperationId
    ) => {
      return new Promise((resolve) => {
        commandCounter += 1;
        const id = commandCounter;
        const timeout = setTimeout(() => {
          if (!pendingCommands.has(id)) return;
          pendingCommands.delete(id);
          resolve(false);
        }, 3000);
        pendingCommands.set(id, { bridge, resolve, timeout });
        try {
          bridge.postMessage({
            marker: bridgeMarker, kind: 'command', id, action,
            token: String(token), epoch: String(epoch), payload, responseToken,
            responseOpcodes, responseTimeoutMillis, causationOperationId
          });
        } catch (_) {
          pendingCommands.delete(id);
          clearTimeout(timeout);
          resolve(false);
        }
      });
    };

    const localActivate = pageController ? pageController.activate : () => false;
    const localSend = pageController ? pageController.send : () => false;
    globalThis.__citadelActivate = (token, epoch) => {
      const owner = workerOwners.get(String(token));
      if (owner) return runWorkerCommand(owner.bridge, 'activate', token, epoch);
      return localActivate(token, epoch);
    };
    globalThis.__citadelSend = (
      token, epoch, payload, responseToken, responseOpcodes, responseTimeoutMillis,
      causationOperationId
    ) => {
      const owner = workerOwners.get(String(token));
      if (owner) return runWorkerCommand(
        owner.bridge, 'send', token, epoch, payload, responseToken, responseOpcodes,
        responseTimeoutMillis, causationOperationId
      );
      return localSend(
        token, epoch, payload, responseToken, responseOpcodes, responseTimeoutMillis,
        causationOperationId
      );
    };

    const WrappedWorker = function(scriptURL, options) {
      let blobURL = '';
      let worker;
      try {
        const absoluteURL = new URL(scriptURL, document.baseURI).href;
        const loader = options && options.type === 'module'
          ? 'import(' + JSON.stringify(absoluteURL) + ').catch(function(error) { setTimeout(function() { throw error; }, 0); });'
          : 'importScripts(' + JSON.stringify(absoluteURL) + ');';
        const bootstrap =
          '(' + workerBridge.toString() + ')(self, ' + JSON.stringify(bridgeMarker) + ');' +
          '(' + install.toString() + ')(self);' + loader;
        blobURL = URL.createObjectURL(new Blob([bootstrap], { type: 'application/javascript' }));
        worker = new NativeWorker(blobURL, options);
      } catch (_) {
        if (blobURL) URL.revokeObjectURL(blobURL);
        return new NativeWorker(scriptURL, options);
      }
      try {
        attachWorker(worker);
      } catch (_) {}
      setTimeout(() => URL.revokeObjectURL(blobURL), 0);
      return worker;
    };
    WrappedWorker.prototype = NativeWorker.prototype;
    try {
      Object.setPrototypeOf(WrappedWorker, NativeWorker);
    } catch (_) {}
    window.Worker = WrappedWorker;
    const localResync = pageController && typeof pageController.resync === 'function'
      ? pageController.resync : () => 0;
    globalThis.__citadelResync = () => {
      let count = localResync();
      for (const [token, owner] of workerOwners) {
        forwardNotice({
          v: 1, type: 'created', token,
          seq: Number.isSafeInteger(owner.sequence) ? owner.sequence : 1,
          url: owner.url || '', eligible: owner.eligible === true
        });
        count += 1;
      }
      return count;
    };
  }
})();
`
