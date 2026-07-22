package Session

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"CitadelDesktop/Server/Outbound"
	"CitadelDesktop/Server/Protocol"
	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/cdproto/runtime"
)

func newSocketTestTransport() *ChromiumTransport {
	transport := &ChromiumTransport{
		frames:      make(chan RawFrame, 32),
		statuses:    make(chan Status, 32),
		generation:  1,
		cancel:      func() {},
		gameContext: context.Background(),
		status: Status{
			State: "connecting", Namespace: "EmpireEx_21", ChangedAt: time.Now().UTC(),
		},
		activationEvaluator: func(context.Context, runtime.ExecutionContextID, string, uint64) (bool, error) {
			return true, nil
		},
		reloadEvaluator: func(context.Context) error { return nil },
	}
	transport.resetSocketsLocked()
	return transport
}

func processSocketTestNotice(
	t *testing.T,
	transport *ChromiumTransport,
	executionContextID runtime.ExecutionContextID,
	notice chromiumSocketNotice,
) {
	t.Helper()
	payload, err := json.Marshal(notice)
	if err != nil {
		t.Fatal(err)
	}
	transport.processSocketNotice(queuedSocketNotice{
		generation: 1, executionContextID: executionContextID,
		observedAt: time.Now().UTC(), payload: string(payload),
	})
}

func createdSocketNotice(token string, sequence uint64) chromiumSocketNotice {
	return chromiumSocketNotice{
		Version: 1, Type: "created", Token: token, Sequence: sequence,
		URL: "wss://example/ep-live",
	}
}

func TestStartRetriesDisconnectedActiveSession(t *testing.T) {
	transport := newSocketTestTransport()
	gameContext, cancel := context.WithCancel(context.Background())
	defer cancel()
	transport.gameContext = gameContext
	transport.status.State = "disconnected"
	reloaded := false
	transport.reloadEvaluator = func(context.Context) error {
		reloaded = true
		return nil
	}

	if err := transport.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	if !reloaded {
		t.Fatal("disconnected active session was not reloaded")
	}
	status := transport.Status()
	if status.State != "reconnecting" || status.LoggedIn || status.SocketReady {
		t.Fatalf("unexpected retry status: %+v", status)
	}
}

func socketFrameNotice(
	token string,
	sequence uint64,
	direction string,
	payload string,
) chromiumSocketNotice {
	return chromiumSocketNotice{
		Version: 1, Type: "frame", Token: token, Sequence: sequence,
		Direction: Protocol.Direction(direction), Payload: payload,
	}
}

func TestLoginCooldownPublishesCountdownDeadlines(t *testing.T) {
	transport := &ChromiumTransport{statuses: make(chan Status, 1), status: Status{Namespace: "EmpireEx_21"}}
	before := time.Now().UTC()
	transport.observeLoginFrame(0, "", `%xt%lli%1%453%{"CD":10}%`, time.Now().UTC())
	status := transport.Status()
	if status.State != "cooldown" || status.CooldownUntil == nil || status.RetryAt == nil {
		t.Fatalf("unexpected cooldown status: %+v", status)
	}
	if status.CooldownUntil.Before(before.Add(9*time.Second)) || status.RetryAt.Sub(*status.CooldownUntil) != 5*time.Second {
		t.Fatalf("unexpected cooldown deadlines: %+v", status)
	}
}

func TestSuccessfulManualLoginIsSavedPrivately(t *testing.T) {
	transport := newSocketTestTransport()
	transport.config.DataDir = t.TempDir()
	loginFrame := `%xt%EmpireEx_21%lli%1%{"LT":"test-login-token"}%`
	loginNotice := socketFrameNotice("manual", 2, "outbound", loginFrame)
	loginNotice.LoginUsername = "test-player"
	loginNotice.LoginPassword = "test-password"
	processSocketTestNotice(t, transport, 4, createdSocketNotice("manual", 1))
	processSocketTestNotice(t, transport, 4, loginNotice)
	processSocketTestNotice(t, transport, 4, socketFrameNotice("manual", 3, "inbound", `%xt%lli%1%0%{}%`))
	<-transport.frames

	credential, err := loadLoginCredential(transport.config.DataDir)
	if err != nil {
		t.Fatal(err)
	}
	if credential.Username != "test-player" || credential.Password != "test-password" ||
		!credential.AutoRestore {
		t.Fatalf("unexpected saved login metadata: autoRestore=%v", credential.AutoRestore)
	}
	info, err := os.Stat(loginCredentialPath(transport.config.DataDir))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("saved login permissions = %o, want 600", info.Mode().Perm())
	}
}

func TestSavedCredentialsFillAndSubmitVisibleLogin(t *testing.T) {
	transport := newSocketTestTransport()
	transport.loginCredential = persistedLoginCredential{
		SchemaVersion: loginCredentialSchemaVersion,
		AutoRestore:   true,
		Username:      "saved-player",
		Password:      "saved-password",
	}
	fills := 0
	transport.credentialRestoreEvaluator = func(
		_ context.Context,
		contextID runtime.ExecutionContextID,
		username string,
		password string,
	) (bool, error) {
		fills++
		if contextID != 8 || username != "saved-player" || password != "saved-password" {
			t.Fatalf("unexpected credential fill request for context %d", contextID)
		}
		return true, nil
	}
	submits := 0
	transport.credentialSubmitEvaluator = func(context.Context) error {
		submits++
		return nil
	}

	if !transport.restoreCredentialsInContext(transport.generation, 8) {
		t.Fatal("saved credentials did not finish after filling visible login inputs")
	}
	if fills != 1 || submits != 1 || !transport.restoreAttempted {
		t.Fatalf(
			"credential restore calls: fills=%d submits=%d attempted=%v",
			fills, submits, transport.restoreAttempted,
		)
	}

	transport.restoreSuppressed = true
	if !transport.restoreCredentialsInContext(transport.generation, 9) {
		t.Fatal("suppressed credential restore did not stop")
	}
	if fills != 1 || submits != 1 {
		t.Fatal("suppressed credential restore touched the login form")
	}
}

func TestCleanAuthenticatedCloseDisablesRestoreAndReloadsForAccountSelection(t *testing.T) {
	transport := newSocketTestTransport()
	transport.config.DataDir = t.TempDir()
	loginFrame := `%xt%EmpireEx_21%lli%1%{"LT":"test-login-token"}%`
	loginNotice := socketFrameNotice("active", 2, "outbound", loginFrame)
	loginNotice.LoginUsername = "test-player"
	loginNotice.LoginPassword = "test-password"
	processSocketTestNotice(t, transport, 5, createdSocketNotice("active", 1))
	processSocketTestNotice(t, transport, 5, loginNotice)
	processSocketTestNotice(t, transport, 5, socketFrameNotice("active", 3, "inbound", `%xt%lli%1%0%{}%`))
	<-transport.frames
	reloaded := make(chan struct{}, 1)
	transport.reloadEvaluator = func(context.Context) error {
		reloaded <- struct{}{}
		return nil
	}
	requestID := network.RequestID("logout-socket")
	transport.trackedSockets[requestID] = "wss://example/ep-live"
	processSocketTestNotice(t, transport, 5, chromiumSocketNotice{
		Version: 1, Type: "closed", Token: "active", Sequence: 4,
		CloseCode: 1000, WasClean: true,
	})

	status := transport.Status()
	if status.State != "reconnecting" || status.LoggedIn || status.SocketReady ||
		status.Detail != "Game logout detected; waiting for account selection" {
		t.Fatalf("unexpected logout status: %+v", status)
	}
	select {
	case <-reloaded:
	case <-time.After(time.Second):
		t.Fatal("logout did not refresh the game tab")
	}
	transport.handleEvent(transport.generation, &network.EventWebSocketClosed{RequestID: requestID})
	status = transport.Status()
	if status.Detail != "Game logout detected; waiting for account selection" {
		t.Fatalf("network close replaced account-selection status: %+v", status)
	}
	credential, err := loadLoginCredential(transport.config.DataDir)
	if err != nil {
		t.Fatal(err)
	}
	if credential.AutoRestore {
		t.Fatal("logout left saved login auto-restore enabled")
	}
}

func TestAccountSelectionSuppressesReconnectAndLoginTimeoutReloads(t *testing.T) {
	transport := newSocketTestTransport()
	transport.restoreSuppressed = true
	reloads := 0
	transport.reloadEvaluator = func(context.Context) error {
		reloads++
		return nil
	}

	transport.reloadAfter(transport.generation, transport.status.ConnectionGeneration, 0)
	connectingAt := time.Now().UTC()
	transport.status.State = "connecting"
	transport.status.ChangedAt = connectingAt
	transport.handleConnectionTimeout(
		transport.gameContext, transport.generation,
		transport.status.ConnectionGeneration, connectingAt,
	)
	handshakeAt := time.Now().UTC()
	transport.status.State = "authenticating"
	transport.status.ChangedAt = handshakeAt
	transport.handleAuthenticationTimeout(
		transport.gameContext, transport.generation,
		transport.status.ConnectionGeneration, handshakeAt,
	)

	if reloads != 0 {
		t.Fatalf("account-selection state triggered %d automatic reloads", reloads)
	}
	if status := transport.Status(); status.State != "authenticating" ||
		!status.ChangedAt.Equal(handshakeAt) {
		t.Fatalf("account-selection state was replaced by a timeout: %+v", status)
	}
}

func TestNetworkSocketCloseSchedulesReconnect(t *testing.T) {
	transport := newSocketTestTransport()
	transport.status.State = "connected"
	transport.status.LoggedIn = true
	transport.status.SocketReady = true
	gameContext, cancel := context.WithCancel(context.Background())
	cancel()
	transport.gameContext = gameContext
	requestID := network.RequestID("game-socket")
	transport.trackedSockets[requestID] = "wss://example/ep-live"

	before := time.Now().UTC()
	transport.handleEvent(transport.generation, &network.EventWebSocketClosed{RequestID: requestID})

	status := transport.Status()
	if status.State != "disconnected" || status.Detail != "Game websocket closed" {
		t.Fatalf("unexpected socket-close status: %+v", status)
	}
	if status.RetryAt == nil || status.RetryAt.Before(before.Add(socketReconnectDelay-time.Second)) {
		t.Fatalf("socket close did not schedule reconnect: %+v", status)
	}
}

func TestInjectedSocketCloseSchedulesReconnect(t *testing.T) {
	transport := newSocketTestTransport()
	gameContext, cancel := context.WithCancel(context.Background())
	defer cancel()
	transport.gameContext = gameContext
	processSocketTestNotice(t, transport, 1, createdSocketNotice("active", 1))
	processSocketTestNotice(t, transport, 1, socketFrameNotice("active", 2, "inbound", `%xt%lli%1%0%{}%`))
	<-transport.frames

	before := time.Now().UTC()
	processSocketTestNotice(t, transport, 1, chromiumSocketNotice{
		Version: 1, Type: "closed", Token: "active", Sequence: 3,
	})

	status := transport.Status()
	if status.State != "disconnected" || status.Detail != "Game websocket closed" {
		t.Fatalf("unexpected injected socket-close status: %+v", status)
	}
	if status.RetryAt == nil || status.RetryAt.Before(before.Add(socketReconnectDelay-time.Second)) {
		t.Fatalf("injected socket close did not schedule reconnect: %+v", status)
	}
}

func TestUnauthenticatedSocketCloseLeavesLoginPageStable(t *testing.T) {
	transport := newSocketTestTransport()
	requestID := network.RequestID("game-socket")
	transport.trackedSockets[requestID] = "wss://example/ep-live"

	transport.handleEvent(transport.generation, &network.EventWebSocketClosed{RequestID: requestID})

	status := transport.Status()
	if status.State != "disconnected" || status.RetryAt != nil {
		t.Fatalf("unauthenticated socket close scheduled a page reload: %+v", status)
	}
}

func TestAuthenticatingSocketCloseSchedulesReconnect(t *testing.T) {
	transport := newSocketTestTransport()
	transport.status.State = "authenticating"
	transport.status.SocketReady = true
	gameContext, cancel := context.WithCancel(context.Background())
	cancel()
	transport.gameContext = gameContext
	requestID := network.RequestID("game-socket")
	transport.trackedSockets[requestID] = "wss://example/ep-live"

	before := time.Now().UTC()
	transport.handleEvent(transport.generation, &network.EventWebSocketClosed{RequestID: requestID})

	status := transport.Status()
	if status.State != "disconnected" || status.Detail != "Game websocket closed" {
		t.Fatalf("unexpected authenticating socket-close status: %+v", status)
	}
	if status.RetryAt == nil || status.RetryAt.Before(before.Add(socketReconnectDelay-time.Second)) {
		t.Fatalf("authentication socket close did not schedule reconnect: %+v", status)
	}
}

func TestNetworkSocketClosePreservesLoginCooldown(t *testing.T) {
	transport := newSocketTestTransport()
	retryAt := time.Now().UTC().Add(time.Minute)
	transport.status.State = "cooldown"
	transport.status.Detail = "Login cooldown: 55s"
	transport.status.RetryAt = &retryAt
	requestID := network.RequestID("game-socket")
	transport.trackedSockets[requestID] = "wss://example/ep-live"

	transport.handleEvent(transport.generation, &network.EventWebSocketClosed{RequestID: requestID})

	status := transport.Status()
	if status.State != "cooldown" || status.RetryAt == nil || !status.RetryAt.Equal(retryAt) {
		t.Fatalf("socket close replaced login cooldown: %+v", status)
	}
}

func TestAuthenticationTimeoutReloadsOnlyThePendingHandshake(t *testing.T) {
	transport := newSocketTestTransport()
	handshakeAt := time.Now().UTC()
	transport.status.State = "authenticating"
	transport.status.SocketReady = true
	transport.status.ChangedAt = handshakeAt
	reloaded := false
	transport.reloadEvaluator = func(context.Context) error {
		reloaded = true
		return nil
	}

	transport.handleAuthenticationTimeout(
		transport.gameContext, transport.generation,
		transport.status.ConnectionGeneration, handshakeAt,
	)

	status := transport.Status()
	if !reloaded || status.State != "reconnecting" || status.SocketReady ||
		status.Detail != "Game login handshake timed out" {
		t.Fatalf("authentication timeout did not reload the pending handshake: %+v", status)
	}

	reloaded = false
	transport.handleAuthenticationTimeout(
		transport.gameContext, transport.generation,
		transport.status.ConnectionGeneration, handshakeAt,
	)
	if reloaded {
		t.Fatal("stale authentication timeout reloaded a newer session state")
	}
}

func TestConnectionTimeoutReloadsOnlyThePendingAttempt(t *testing.T) {
	transport := newSocketTestTransport()
	connectingAt := time.Now().UTC().Add(-time.Second)
	transport.status.State = "reconnecting"
	transport.status.ChangedAt = connectingAt
	reloads := 0
	transport.reloadEvaluator = func(context.Context) error {
		reloads++
		return nil
	}

	transport.handleConnectionTimeout(
		transport.gameContext, transport.generation,
		transport.status.ConnectionGeneration, connectingAt,
	)

	status := transport.Status()
	if reloads != 1 || status.State != "reconnecting" ||
		status.Detail != "Game websocket did not open before timeout" {
		t.Fatalf("connection timeout did not reload the pending attempt: %+v", status)
	}

	transport.handleConnectionTimeout(
		transport.gameContext, transport.generation,
		transport.status.ConnectionGeneration, connectingAt,
	)
	if reloads != 1 {
		t.Fatal("stale connection timeout reloaded a newer session state")
	}
}

func TestClosingSupersededSocketKeepsAuthenticatedReplacementReady(t *testing.T) {
	transport := newSocketTestTransport()
	processSocketTestNotice(t, transport, 11, createdSocketNotice("old", 1))
	processSocketTestNotice(t, transport, 11, socketFrameNotice(
		"old", 2, "inbound", `%xt%lli%1%0%{}%`,
	))
	<-transport.frames
	oldGeneration := transport.Status().ConnectionGeneration
	processSocketTestNotice(t, transport, 12, createdSocketNotice("new", 1))
	processSocketTestNotice(t, transport, 12, socketFrameNotice(
		"new", 2, "inbound", `%xt%EmpireEx_21%gbd%0%{}%`,
	))
	if transport.executionContextID != 11 {
		t.Fatalf("unauthenticated candidate stole execution context %d", transport.executionContextID)
	}
	processSocketTestNotice(t, transport, 12, socketFrameNotice(
		"new", 3, "inbound", `%xt%lli%1%0%{}%`,
	))
	<-transport.frames
	processSocketTestNotice(t, transport, 11, chromiumSocketNotice{
		Version: 1, Type: "closed", Token: "old", Sequence: 3,
	})

	status := transport.Status()
	if !status.LoggedIn || !status.SocketReady || status.State != "connected" {
		t.Fatalf("superseded socket closure replaced authenticated status: %+v", status)
	}
	if transport.activeSocket.token != "new" {
		t.Fatalf("active socket = %q, want new", transport.activeSocket.token)
	}
	if status.ConnectionGeneration <= oldGeneration || transport.executionContextID != 12 {
		t.Fatalf("replacement did not advance the active connection: %+v context=%d", status, transport.executionContextID)
	}
}

func TestCandidateAndSupersededFramesNeverReachIngest(t *testing.T) {
	transport := newSocketTestTransport()
	processSocketTestNotice(t, transport, 7, createdSocketNotice("a", 1))
	processSocketTestNotice(t, transport, 7, socketFrameNotice("a", 2, "inbound", `%xt%lli%1%0%{}%`))
	first := <-transport.frames
	processSocketTestNotice(t, transport, 7, createdSocketNotice("b", 1))
	processSocketTestNotice(t, transport, 7, socketFrameNotice(
		"b", 2, "inbound", `%xt%EmpireEx_21%gbd%0%{"candidate":true}%`,
	))
	select {
	case frame := <-transport.frames:
		t.Fatalf("candidate frame was delivered: %+v", frame)
	default:
	}
	processSocketTestNotice(t, transport, 7, socketFrameNotice("b", 3, "inbound", `%xt%lli%1%0%{}%`))
	second := <-transport.frames
	processSocketTestNotice(t, transport, 7, socketFrameNotice(
		"a", 3, "inbound", `%xt%EmpireEx_21%gbd%0%{"stale":true}%`,
	))
	select {
	case frame := <-transport.frames:
		t.Fatalf("superseded frame was delivered: %+v", frame)
	default:
	}
	if first.ConnectionGeneration == 0 || second.ConnectionGeneration <= first.ConnectionGeneration {
		t.Fatalf("frame generations did not advance: first=%d second=%d", first.ConnectionGeneration, second.ConnectionGeneration)
	}
}

func TestClosedTokenCannotAuthenticateLaterOrPolluteSameURLReplacement(t *testing.T) {
	transport := newSocketTestTransport()
	processSocketTestNotice(t, transport, 4, createdSocketNotice("closed", 1))
	processSocketTestNotice(t, transport, 4, chromiumSocketNotice{
		Version: 1, Type: "closed", Token: "closed", Sequence: 2,
	})
	processSocketTestNotice(t, transport, 4, socketFrameNotice("closed", 3, "inbound", `%xt%lli%1%0%{}%`))
	processSocketTestNotice(t, transport, 9, createdSocketNotice("replacement", 1))
	processSocketTestNotice(t, transport, 9, socketFrameNotice("replacement", 2, "inbound", `%xt%lli%1%0%{}%`))
	<-transport.frames
	if transport.activeSocket.token != "replacement" || transport.executionContextID != 9 {
		t.Fatalf("stale token polluted replacement: active=%+v context=%d", transport.activeSocket, transport.executionContextID)
	}
}

func TestInitialSocketLoginFailurePublishesCooldown(t *testing.T) {
	transport := newSocketTestTransport()
	processSocketTestNotice(t, transport, 4, createdSocketNotice("failed", 1))
	processSocketTestNotice(t, transport, 4, socketFrameNotice(
		"failed", 2, "inbound", `%xt%lli%1%453%{"CD":0}%`,
	))
	status := transport.Status()
	if status.State != "cooldown" || status.LoggedIn || status.SocketReady {
		t.Fatalf("initial login failure left transport authenticating: %+v", status)
	}
}

func TestActivationFailureDoesNotLeaveTransportAuthenticating(t *testing.T) {
	transport := newSocketTestTransport()
	transport.activationEvaluator = func(
		context.Context, runtime.ExecutionContextID, string, uint64,
	) (bool, error) {
		return false, nil
	}
	processSocketTestNotice(t, transport, 4, createdSocketNotice("failed", 1))
	processSocketTestNotice(t, transport, 4, socketFrameNotice("failed", 2, "inbound", `%xt%lli%1%0%{}%`))
	status := transport.Status()
	if status.State != "error" || status.LoggedIn || status.SocketReady {
		t.Fatalf("activation failure left transport usable: %+v", status)
	}
	if _, exists := transport.sockets[chromiumSocketKey{executionContextID: 4, token: "failed"}]; exists {
		t.Fatal("failed activation remained registered")
	}
}

func TestOlderSocketCannotReclaimConnectionAfterReplacement(t *testing.T) {
	transport := newSocketTestTransport()
	processSocketTestNotice(t, transport, 3, createdSocketNotice("older", 1))
	processSocketTestNotice(t, transport, 3, socketFrameNotice("older", 2, "inbound", `%xt%lli%1%0%{}%`))
	<-transport.frames
	processSocketTestNotice(t, transport, 3, createdSocketNotice("newer", 1))
	processSocketTestNotice(t, transport, 3, socketFrameNotice("newer", 2, "inbound", `%xt%lli%1%0%{}%`))
	<-transport.frames
	generation := transport.Status().ConnectionGeneration
	processSocketTestNotice(t, transport, 3, socketFrameNotice("older", 3, "inbound", `%xt%lli%1%0%{}%`))
	if transport.activeSocket.token != "newer" || transport.Status().ConnectionGeneration != generation {
		t.Fatalf("older socket reclaimed connection: active=%+v status=%+v", transport.activeSocket, transport.Status())
	}
}

func TestSocketActivationWaitsForPhysicalSendGate(t *testing.T) {
	transport := newSocketTestTransport()
	processSocketTestNotice(t, transport, 5, createdSocketNotice("a", 1))
	processSocketTestNotice(t, transport, 5, socketFrameNotice("a", 2, "inbound", `%xt%lli%1%0%{}%`))
	<-transport.frames
	processSocketTestNotice(t, transport, 5, createdSocketNotice("b", 1))

	started := make(chan struct{})
	done := make(chan struct{})
	transport.activationEvaluator = func(
		context.Context, runtime.ExecutionContextID, string, uint64,
	) (bool, error) {
		return true, nil
	}
	if err := transport.acquireSendGate(context.Background()); err != nil {
		t.Fatal(err)
	}
	noticePayload, err := json.Marshal(socketFrameNotice("b", 2, "inbound", `%xt%lli%1%0%{}%`))
	if err != nil {
		t.Fatal(err)
	}
	go func() {
		close(started)
		transport.processSocketNotice(queuedSocketNotice{
			generation: 1, executionContextID: 5,
			observedAt: time.Now().UTC(), payload: string(noticePayload),
		})
		close(done)
	}()
	<-started
	deadline := time.Now().Add(time.Second)
	for {
		transport.mu.RLock()
		queued := transport.sockets[chromiumSocketKey{executionContextID: 5, token: "b"}].lastSequence == 2
		activeToken := transport.activeSocket.token
		transport.mu.RUnlock()
		if queued {
			if activeToken != "a" {
				t.Fatalf("activation crossed the physical send gate: %q", activeToken)
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("replacement frame did not reach the activation gate")
		}
		time.Sleep(time.Millisecond)
	}
	transport.releaseSendGate()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("replacement did not activate after send gate opened")
	}
	<-transport.frames
	if transport.activeSocket.token != "b" {
		t.Fatalf("active socket = %+v, want b", transport.activeSocket)
	}
}

func TestExecutionContextDestructionOnlyInvalidatesMatchingSocket(t *testing.T) {
	transport := newSocketTestTransport()
	processSocketTestNotice(t, transport, 1, createdSocketNotice("old", 1))
	processSocketTestNotice(t, transport, 1, socketFrameNotice("old", 2, "inbound", `%xt%lli%1%0%{}%`))
	<-transport.frames
	processSocketTestNotice(t, transport, 2, createdSocketNotice("active", 1))
	processSocketTestNotice(t, transport, 2, socketFrameNotice("active", 2, "inbound", `%xt%lli%1%0%{}%`))
	<-transport.frames
	transport.processSocketNotice(queuedSocketNotice{
		generation: 1, executionContextID: 1, destroyContext: true, observedAt: time.Now().UTC(),
	})
	if !transport.Status().LoggedIn || transport.activeSocket.token != "active" {
		t.Fatalf("unrelated context destruction invalidated active socket: %+v", transport.Status())
	}
	transport.processSocketNotice(queuedSocketNotice{
		generation: 1, executionContextID: 2, destroyContext: true, observedAt: time.Now().UTC(),
	})
	if transport.Status().LoggedIn || transport.Status().SocketReady || transport.activeToken != "" {
		t.Fatalf("active context destruction kept connection ready: %+v", transport.Status())
	}
}

func TestChromiumSendRejectsMismatchedExpectedConnectionGeneration(t *testing.T) {
	transport := newSocketTestTransport()
	transport.status.LoggedIn = true
	transport.status.SocketReady = true
	transport.status.ConnectionGeneration = 2
	transport.activeToken = "active"
	ctx := Outbound.WithMetadata(context.Background(), Outbound.Metadata{ConnectionGeneration: 1})
	if err := transport.Send(ctx, []byte("payload")); !errors.Is(err, Outbound.ErrConnectionChanged) {
		t.Fatalf("send error = %v", err)
	}
}

func TestChromiumSendHonorsCallerCancellationWhileDispatchGateIsBusy(t *testing.T) {
	transport := newSocketTestTransport()
	if err := transport.acquireSendGate(context.Background()); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	result := make(chan error, 1)
	go func() { result <- transport.Send(ctx, []byte("payload")) }()
	select {
	case err := <-result:
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("send error = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("cancelled send stayed blocked on the dispatch gate")
	}
	transport.releaseSendGate()
}

func TestChromiumWebSocketEvaluationAwaitsWorkerBridgePromises(t *testing.T) {
	evaluation := websocketRuntimeEvaluation("Promise.resolve(true)")
	if !evaluation.AwaitPromise || !evaluation.ReturnByValue {
		t.Fatalf("websocket evaluation does not await a by-value result: %+v", evaluation)
	}
}

func TestBrowserEvaluationContextHonorsCallerCancellation(t *testing.T) {
	callerContext, cancelCaller := context.WithCancel(context.Background())
	evaluationContext, cancelEvaluation := browserEvaluationContext(context.Background(), callerContext)
	defer cancelEvaluation()
	cancelCaller()
	select {
	case <-evaluationContext.Done():
		if !errors.Is(evaluationContext.Err(), context.Canceled) {
			t.Fatalf("evaluation cancellation error = %v", evaluationContext.Err())
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("browser evaluation ignored caller cancellation")
	}
}

func TestChromiumTransportInjectionBridgesDedicatedWorkerSockets(t *testing.T) {
	required := []string{
		"root.__citadelWorkerNotify",
		"const postMessage = root.postMessage.bind(root)",
		"postMessage({ marker, kind: 'notice', notice })",
		"workerOwners.set(notice.token, {",
		"forwardNotice(notice)",
		"type: 'closed', token",
		"owner.sequence + 1",
		"kind: 'command'",
		"message.action === 'activate'",
		"message.action === 'send'",
		"return runWorkerCommand(owner.bridge, 'activate'",
		"owner.bridge, 'send', token",
		"options && options.type === 'module'",
		"? 'import('",
		": 'importScripts('",
		"record.pendingResponses.push(pending)",
		"pending.requestOpcode !== 'ahr' || opcode !== 'ahh'",
		"Number(responsePayload.OP.RID) === Number(requestPayload.ID)",
		"responseToken = matchResponse(record, opcode, payload)",
		"direction, payload, responseToken",
		"responseOpcodes, responseTimeoutMillis",
		"const loginCredentialCandidate = { username: '', password: '' }",
		"input[autocomplete=\"username\"]",
		"input[autocomplete=\"current-password\"]",
		"root.document.addEventListener('input'",
		"const visibleLoginInput = (autocomplete) =>",
		"loginUsername: credential.username, loginPassword: credential.password",
		"const credential = pageController.loginCredentials()",
		"activate, send, loginCredentials, restoreCredentials, closeGameUI, opcodeOf",
		"root.__citadelRestoreCredentials = restoreCredentials",
		"root.__citadelCloseGameUI = closeGameUI",
		"closeCode: event && Number.isFinite(event.code)",
		"wasClean: Boolean(event && event.wasClean)",
		"const cache = require.c",
		"exported && exported.BasicLayoutManager",
		"'hideAllDialogs', 'hideAllPanels', 'hideAllAttackPanels'",
		"'hideAllNonPermanentUIComponents', 'hideAllRingMenus'",
	}
	for _, fragment := range required {
		if !strings.Contains(chromiumTransportInjection, fragment) {
			t.Errorf("Chromium worker bridge is missing %q", fragment)
		}
	}
}

func TestChromiumTransportDoesNotCloseGameUIAfterCRA(t *testing.T) {
	for _, fragment := range []string{
		"closeAttackUIAfterSend",
		"pageController.opcodeOf(payload) === 'cra'",
		"pageController.closeGameUI(causationOperationId)",
	} {
		if strings.Contains(chromiumTransportInjection, fragment) {
			t.Errorf("Chromium worker bridge still contains post-CRA UI close %q", fragment)
		}
	}
}

func TestChromiumTransportIgnoresOutOfOrderStatus(t *testing.T) {
	currentAt := time.Now().UTC()
	transport := &ChromiumTransport{
		statuses: make(chan Status, 1),
		status: Status{
			State: "connected", LoggedIn: true, SocketReady: true,
			Namespace: "EmpireEx_21", ChangedAt: currentAt,
		},
	}
	transport.publishStatus(Status{State: "connecting", ChangedAt: currentAt.Add(-time.Millisecond)})

	status := transport.Status()
	if !status.LoggedIn || !status.SocketReady || status.State != "connected" {
		t.Fatalf("out-of-order status replaced current transport state: %+v", status)
	}
}
