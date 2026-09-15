package Session

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"CitadelDesktop/Server/Outbound"
	"github.com/gorilla/websocket"
)

type finalDispatchDoneObservedContext struct {
	context.Context
	once     sync.Once
	observed chan struct{}
}

func (ctx *finalDispatchDoneObservedContext) Done() <-chan struct{} {
	ctx.once.Do(func() { close(ctx.observed) })
	return ctx.Context.Done()
}

type finalDispatchErrObservedContext struct {
	context.Context
	once     sync.Once
	observed chan struct{}
}

func (ctx *finalDispatchErrObservedContext) Err() error {
	ctx.once.Do(func() { close(ctx.observed) })
	return ctx.Context.Err()
}

func TestChromiumFinalDispatchValidationRunsAfterSendGateWait(t *testing.T) {
	transport := newSocketTestTransport()
	transport.status.LoggedIn = true
	transport.status.SocketReady = true
	transport.status.ConnectionGeneration = 1
	transport.activeToken = "active"
	if err := transport.acquireSendGate(t.Context()); err != nil {
		t.Fatal(err)
	}

	var stateValid atomic.Bool
	stateValid.Store(true)
	stale := errors.New("dispatch state changed while waiting for Chromium send gate")
	waitingOnGate := make(chan struct{})
	result := make(chan error, 1)
	observedContext := &finalDispatchDoneObservedContext{Context: t.Context(), observed: waitingOnGate}
	ctx := Outbound.WithFinalDispatchValidation(observedContext, func(context.Context) error {
		if !stateValid.Load() {
			return stale
		}
		return nil
	})
	go func() {
		result <- transport.Send(ctx, []byte("guarded"))
	}()
	<-waitingOnGate
	stateValid.Store(false)
	transport.releaseSendGate()

	select {
	case err := <-result:
		if !errors.Is(err, stale) {
			t.Fatalf("send error = %v, want %v", err, stale)
		}
		if Outbound.IsIndeterminate(err) {
			t.Fatalf("pre-send validation error became indeterminate: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("send stayed blocked after Chromium send gate opened")
	}
	select {
	case frame := <-transport.frames:
		t.Fatalf("rejected send emitted outbound frame: %+v", frame)
	default:
	}
}

func TestDirectWebSocketFinalDispatchValidationRunsAfterWriteMutexWait(t *testing.T) {
	received := make(chan string, 2)
	upgrader := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		connection, err := upgrader.Upgrade(writer, request, nil)
		if err != nil {
			return
		}
		defer connection.Close()
		for {
			_, payload, readErr := connection.ReadMessage()
			if readErr != nil {
				return
			}
			received <- string(payload)
		}
	}))
	defer server.Close()
	connection, _, err := websocket.DefaultDialer.Dial(
		"ws"+strings.TrimPrefix(server.URL, "http"), nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	defer connection.Close()
	transport := &DirectWebSocketTransport{
		connection: connection,
		frames:     make(chan RawFrame, 2),
		status: Status{
			LoggedIn: true, SocketReady: true, ConnectionGeneration: 1,
		},
	}

	transport.writeMu.Lock()
	var stateValid atomic.Bool
	stateValid.Store(true)
	stale := errors.New("dispatch state changed while waiting for direct write mutex")
	waitingOnWriteMutex := make(chan struct{})
	result := make(chan error, 1)
	validate := func(context.Context) error {
		if !stateValid.Load() {
			return stale
		}
		return nil
	}
	observedContext := &finalDispatchErrObservedContext{Context: t.Context(), observed: waitingOnWriteMutex}
	ctx := Outbound.WithFinalDispatchValidation(observedContext, validate)
	go func() {
		result <- transport.Send(ctx, []byte("stale"))
	}()
	<-waitingOnWriteMutex
	stateValid.Store(false)
	transport.writeMu.Unlock()

	select {
	case sendErr := <-result:
		if !errors.Is(sendErr, stale) {
			t.Fatalf("send error = %v, want %v", sendErr, stale)
		}
		if Outbound.IsIndeterminate(sendErr) {
			t.Fatalf("pre-send validation error became indeterminate: %v", sendErr)
		}
	case <-time.After(time.Second):
		t.Fatal("send stayed blocked after direct write mutex opened")
	}
	select {
	case payload := <-received:
		t.Fatalf("rejected send reached websocket: %q", payload)
	default:
	}

	stateValid.Store(true)
	if err := transport.Send(ctx, []byte("allowed")); err != nil {
		t.Fatalf("allowed guarded send error = %v", err)
	}
	if err := transport.Send(t.Context(), []byte("unguarded")); err != nil {
		t.Fatalf("unguarded send error = %v", err)
	}
	for _, want := range []string{"allowed", "unguarded"} {
		select {
		case payload := <-received:
			if payload != want {
				t.Fatalf("websocket payload = %q, want %q", payload, want)
			}
		case <-time.After(time.Second):
			t.Fatalf("websocket did not receive %q", want)
		}
	}
}
