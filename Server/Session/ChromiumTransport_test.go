package Session

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"CitadelDesktop/Server/Outbound"
	"CitadelDesktop/Server/Protocol"
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
		"responseToken = matchResponse(record, opcode)",
		"direction, payload, responseToken",
		"responseOpcodes, responseTimeoutMillis",
	}
	for _, fragment := range required {
		if !strings.Contains(chromiumTransportInjection, fragment) {
			t.Errorf("Chromium worker bridge is missing %q", fragment)
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
