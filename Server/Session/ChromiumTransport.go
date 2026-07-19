package Session

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"CitadelDesktop/Server/Outbound"
	"CitadelDesktop/Server/Protocol"
	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/cdproto/page"
	"github.com/chromedp/cdproto/runtime"
	"github.com/chromedp/chromedp"
)

const defaultGameURL = "https://empire.goodgamestudios.com/"

const browserEvaluationTimeout = 5 * time.Second

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

	mu                  sync.RWMutex
	status              Status
	gameContext         context.Context
	cancel              context.CancelFunc
	generation          uint64
	executionContextID  runtime.ExecutionContextID
	trackedSockets      map[network.RequestID]string
	sockets             map[chromiumSocketKey]*chromiumSocket
	closedSockets       map[chromiumSocketKey]uint64
	activeSocket        chromiumSocketKey
	activeToken         string
	activeOrdinal       uint64
	socketGeneration    uint64
	nextSocketOrdinal   uint64
	activationEvaluator func(context.Context, runtime.ExecutionContextID, string, uint64) (bool, error)

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
	state := "stopped"
	detail := ""
	if resolveErr != nil {
		state = "unavailable"
		detail = resolveErr.Error()
	}
	return &ChromiumTransport{
		config: config, browser: browser, resolveErr: resolveErr,
		frames: make(chan RawFrame, 8192), statuses: make(chan Status, 32),
		status: Status{
			State: state, Namespace: "EmpireEx_21", Detail: detail,
			BrowserID: browser.ID, BrowserName: browser.Name, ChangedAt: time.Now().UTC(),
		},
		trackedSockets: map[network.RequestID]string{},
		sockets:        map[chromiumSocketKey]*chromiumSocket{},
		closedSockets:  map[chromiumSocketKey]uint64{},
		noticeWake:     make(chan struct{}, 1),
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
	transport.resetSocketsLocked()
	transport.mu.Unlock()
	transport.resetSocketNoticeQueue()
	go transport.runSocketNotices(gameContext, generation)

	chromedp.ListenTarget(gameContext, func(event any) {
		transport.handleEvent(generation, event)
	})
	transport.publishStatus(Status{State: "connecting", Namespace: "EmpireEx_21", ChangedAt: time.Now().UTC()})
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
	transport.resetSocketsLocked()
	transport.generation++
	transport.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	transport.publishStatus(Status{State: "stopped", Namespace: "EmpireEx_21", ChangedAt: time.Now().UTC()})
	return nil
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
		transport.enqueueSocketNotice(queuedSocketNotice{
			generation: generation, executionContextID: typed.ExecutionContextID,
			observedAt: time.Now().UTC(), payload: typed.Payload,
		})
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
			status.State = "authenticating"
			status.SocketReady = true
			status.ChangedAt = time.Now().UTC()
			transport.publishStatus(status)
		}
	case *network.EventWebSocketClosed:
		removed, noActive, remaining := transport.removeNetworkSocket(typed.RequestID)
		if !removed || !noActive || remaining > 0 {
			return
		}
		transport.publishSocketLoss("disconnected", "Game websocket closed", 0)
	case *network.EventWebSocketFrameError:
		removed, noActive, remaining := transport.removeNetworkSocket(typed.RequestID)
		if !removed || !noActive || remaining > 0 {
			return
		}
		transport.publishSocketLoss("error", typed.ErrorMessage, 0)
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
		retryAt := cooldownUntil.Add(5 * time.Second)
		status.State = "cooldown"
		status.Detail = fmt.Sprintf("Login cooldown: %ds", cooldown.Seconds)
		status.CooldownUntil = &cooldownUntil
		status.RetryAt = &retryAt
		transport.publishStatus(status)
		if cooldown.Seconds > 0 {
			go transport.reloadAfter(
				generation, status.ConnectionGeneration,
				time.Duration(cooldown.Seconds)*time.Second+5*time.Second,
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
	if !transport.isCurrent(generation) ||
		transport.Status().ConnectionGeneration != connectionGeneration {
		return
	}
	transport.publishStatus(Status{State: "reconnecting", Namespace: transport.namespace(), ChangedAt: time.Now().UTC()})
	_ = chromedp.Run(gameContext, chromedp.Reload())
}

func (transport *ChromiumTransport) publishStatus(status Status) {
	if status.ChangedAt.IsZero() {
		status.ChangedAt = time.Now().UTC()
	}
	transport.mu.Lock()
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
	return transport.gameContext
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
			queued.generation, key, notice.Sequence, "disconnected", "Game websocket closed", queued.observedAt,
		)
	case "error":
		transport.removeSocket(
			queued.generation, key, notice.Sequence, "error", "Game websocket error", queued.observedAt,
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
	defer transport.mu.Unlock()
	if transport.generation != generation || transport.cancel == nil {
		return
	}
	if _, closed := transport.closedSockets[key]; closed {
		return
	}
	if _, exists := transport.sockets[key]; exists {
		return
	}
	transport.nextSocketOrdinal++
	transport.sockets[key] = &chromiumSocket{
		serverURL: notice.URL, ordinal: transport.nextSocketOrdinal, lastSequence: notice.Sequence,
	}
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

	if notice.Direction == Protocol.DirectionInbound {
		frame, err := Protocol.Decode(notice.Payload, notice.Direction, observedAt)
		if err == nil && frame.Opcode == "lli" && frame.ResponseCode != nil {
			if *frame.ResponseCode == 0 {
				connectionGeneration, activated, activationErr := transport.activateSocket(
					generation, key, notice.Sequence, observedAt,
				)
				if activated {
					transport.deliverSocketFrame(notice, observedAt, connectionGeneration)
				} else if activationErr != nil {
					transport.failSocketActivation(
						generation, key, notice.Sequence, activationErr, observedAt,
					)
				}
				return
			}
			if transport.deactivateSocket(generation, key) ||
				transport.socketMayPublishLoginFailure(generation, key) {
				transport.publishLoginFailure(generation, frame, observedAt)
			}
			return
		}
	}
	if connectionGeneration, active := transport.activeSocketGeneration(generation, key); active {
		transport.deliverSocketFrame(notice, observedAt, connectionGeneration)
	}
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
	transport.mu.Unlock()
	if wasActive {
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
	transport.mu.Unlock()
	if wasActive {
		transport.publishSocketLossAt(
			"disconnected", "Game execution context closed", remaining, observedAt,
		)
	}
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
	transport.gameContext = nil
	transport.resetSocketsLocked()
	return true
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
    if (!root || !root.WebSocket) return null;
    if (root.__citadelTransportInstalled) return root.__citadelTransportController || null;
    root.__citadelTransportInstalled = true;
    const NativeWebSocket = root.WebSocket;
    const sockets = new Map();
    let tokenCounter = 0;
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
        expiresAt: Date.now() + timeoutMillis + 1000
      };
      record.pendingResponses.push(pending);
      return pending;
    };
    const matchResponse = (record, opcode) => {
      const now = Date.now();
      record.pendingResponses = record.pendingResponses.filter((pending) => pending.expiresAt > now);
      const index = record.pendingResponses.findIndex((pending) => pending.opcodes.includes(opcode));
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
          responseToken = matchResponse(record, opcode);
          if (parts.length > 4 && opcode === 'lli') {
            record.eligible = parts[4] === '0';
            if (!record.eligible) record.epoch = '';
          }
        }
        record.sequence += 1;
        notify({
          v: 1, type: 'frame', token: record.token, seq: record.sequence,
          direction, payload, responseToken,
          causationOperationId: direction === 'outbound' ? String(causationOperationId || '') : ''
        });
      });
    };
    const emitLifecycle = (record, type) => {
      schedule(record, () => {
        record.sequence += 1;
        notify({ v: 1, type, token: record.token, seq: record.sequence, url: record.url });
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
            emitFrame(record, 'outbound', data, request && request.causationOperationId);
            return result;
          };
          try {
            Object.defineProperty(socket, 'send', { configurable: true, value: record.send });
            record.wrappedSend = true;
          } catch (_) {}
          sockets.set(record.token, record);
          notify({ v: 1, type: 'created', token: record.token, seq: record.sequence, url: record.url });
          socket.addEventListener('message', (event) => emitFrame(record, 'inbound', event.data));
          socket.addEventListener('close', () => {
            record.eligible = false;
            record.epoch = '';
            record.pendingResponses = [];
            sockets.delete(record.token);
            emitLifecycle(record, 'closed');
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
    const controller = { activate, send };
    root.__citadelActivate = activate;
    root.__citadelSend = send;
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
        if (typeof globalThis.citadelTransportNotify === 'function') {
          globalThis.citadelTransportNotify(JSON.stringify(notice));
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
              bridge, sequence: notice.seq, url: typeof notice.url === 'string' ? notice.url : ''
            });
          } else {
            const owner = workerOwners.get(notice.token);
            if (owner && owner.bridge === bridge) {
              owner.sequence = notice.seq;
              if (typeof notice.url === 'string' && notice.url) owner.url = notice.url;
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
  }
})();
`
