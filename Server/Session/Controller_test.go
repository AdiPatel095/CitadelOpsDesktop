package Session

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"CitadelDesktop/Server/GameData"
	"CitadelDesktop/Server/Ingest"
	"CitadelDesktop/Server/Outbound"
	"CitadelDesktop/Server/Protocol"
	"CitadelDesktop/Server/State"
)

type pacingTransport struct {
	mu                       sync.Mutex
	sends                    []time.Time
	frames                   chan RawFrame
	statuses                 chan Status
	status                   Status
	sendDelay                time.Duration
	reportsOutboundCausation bool
	starts                   int
}

type frontendInteractionTransport struct {
	*pacingTransport
	closed bool
}

func (transport *frontendInteractionTransport) CloseGameUI(context.Context) error {
	transport.closed = true
	return nil
}

func newPacingTransport() *pacingTransport {
	return &pacingTransport{
		frames: make(chan RawFrame), statuses: make(chan Status),
		status: Status{
			State: "connected", LoggedIn: true, SocketReady: true,
			ConnectionGeneration: 1, Namespace: "EmpireEx_21", ChangedAt: time.Now().UTC(),
		},
	}
}

func (transport *pacingTransport) Start(context.Context) error {
	transport.mu.Lock()
	transport.starts++
	transport.mu.Unlock()
	return nil
}
func (*pacingTransport) Stop(context.Context) error { return nil }

func (transport *pacingTransport) Send(_ context.Context, _ []byte) error {
	transport.mu.Lock()
	transport.sends = append(transport.sends, time.Now())
	delay := transport.sendDelay
	transport.mu.Unlock()
	if delay > 0 {
		time.Sleep(delay)
	}
	return nil
}

func (transport *pacingTransport) Frames() <-chan RawFrame      { return transport.frames }
func (transport *pacingTransport) StatusChanges() <-chan Status { return transport.statuses }
func (transport *pacingTransport) ReportsOutboundCausation() bool {
	return transport.reportsOutboundCausation
}
func (transport *pacingTransport) Status() Status {
	transport.mu.Lock()
	defer transport.mu.Unlock()
	return transport.status
}

func (transport *pacingTransport) setConnectionGeneration(generation uint64) {
	transport.mu.Lock()
	transport.status.ConnectionGeneration = generation
	transport.status.ChangedAt = time.Now().UTC()
	transport.mu.Unlock()
}

func TestControllerRetriesDisconnectedActiveTransport(t *testing.T) {
	transport := newPacingTransport()
	controller := NewController(context.Background(), transport, nil, nil)
	defer controller.outbound.Close()
	defer controller.Stop(context.Background())
	if err := controller.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	transport.mu.Lock()
	transport.status.State = "disconnected"
	transport.status.LoggedIn = false
	transport.status.SocketReady = false
	transport.mu.Unlock()

	if err := controller.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	transport.mu.Lock()
	starts := transport.starts
	transport.mu.Unlock()
	if starts != 2 {
		t.Fatalf("transport starts = %d, want 2", starts)
	}
}

func TestControllerClosesGameUIThroughFrontendInteractionTransport(t *testing.T) {
	transport := &frontendInteractionTransport{pacingTransport: newPacingTransport()}
	controller := NewController(context.Background(), transport, nil, nil)
	defer controller.outbound.Close()
	if err := controller.CloseGameUI(t.Context()); err != nil {
		t.Fatal(err)
	}
	if !transport.closed {
		t.Fatal("frontend interaction transport did not receive the close request")
	}
}

func TestControllerRejectsGameUICloseWithoutFrontendInteractionTransport(t *testing.T) {
	controller := NewController(context.Background(), newPacingTransport(), nil, nil)
	defer controller.outbound.Close()
	if err := controller.CloseGameUI(t.Context()); !errors.Is(err, ErrFrontendInteractionUnavailable) {
		t.Fatalf("close game UI error = %v", err)
	}
}

func TestControllerPacesConsecutiveAttackLaunches(t *testing.T) {
	transport := newPacingTransport()
	controller := NewController(context.Background(), transport, nil, nil)
	defer controller.outbound.Close()
	controller.SetAttackDelayProvider(func() time.Duration { return 40 * time.Millisecond })
	payload, err := Protocol.Encode(Protocol.Command{Namespace: "EmpireEx_21", Opcode: "cra", Payload: []byte(`{}`)})
	if err != nil {
		t.Fatal(err)
	}
	if err := controller.Send(context.Background(), payload); err != nil {
		t.Fatal(err)
	}
	if err := controller.Send(context.Background(), payload); err != nil {
		t.Fatal(err)
	}
	transport.mu.Lock()
	delay := transport.sends[1].Sub(transport.sends[0])
	transport.mu.Unlock()
	if delay < 30*time.Millisecond {
		t.Fatalf("consecutive attacks were separated by %s", delay)
	}
}

func TestControllerDispatchesSixNormalCommandsWithinThreeHundredMilliseconds(t *testing.T) {
	transport := newPacingTransport()
	controller := NewController(context.Background(), transport, nil, nil)
	defer controller.outbound.Close()
	payload, err := Protocol.Encode(Protocol.Command{
		Namespace: "EmpireEx_21", Opcode: "bup", Payload: []byte(`{}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	for range 6 {
		if err := controller.Send(context.Background(), payload); err != nil {
			t.Fatal(err)
		}
	}
	transport.mu.Lock()
	span := transport.sends[len(transport.sends)-1].Sub(transport.sends[0])
	transport.mu.Unlock()
	if span < 100*time.Millisecond || span >= 300*time.Millisecond {
		t.Fatalf("six-command normal-lane span = %s, want fast 25ms cadence", span)
	}
}

func TestControllerCadenceDoesNotAddTransportRoundTripToEveryGap(t *testing.T) {
	transport := newPacingTransport()
	transport.sendDelay = 20 * time.Millisecond
	controller := NewController(context.Background(), transport, nil, nil)
	defer controller.outbound.Close()
	payload, err := Protocol.Encode(Protocol.Command{
		Namespace: "EmpireEx_21", Opcode: "bup", Payload: []byte(`{}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	for range 6 {
		if err := controller.Send(context.Background(), payload); err != nil {
			t.Fatal(err)
		}
	}
	transport.mu.Lock()
	span := transport.sends[len(transport.sends)-1].Sub(transport.sends[0])
	transport.mu.Unlock()
	if span < 100*time.Millisecond || span >= 180*time.Millisecond {
		t.Fatalf("six-command span with transport latency = %s, want cadence anchored at dispatch", span)
	}
}

func TestControllerRejectsQueuedCommandAfterConnectionReplacement(t *testing.T) {
	transport := newPacingTransport()
	controller := NewController(context.Background(), transport, nil, nil)
	defer controller.outbound.Close()
	controller.SetAttackDelayProvider(func() time.Duration { return 100 * time.Millisecond })
	payload, err := Protocol.Encode(Protocol.Command{
		Namespace: "EmpireEx_21", Opcode: "cra", Payload: []byte(`{}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	ctx := Outbound.WithMetadata(context.Background(), Outbound.Metadata{
		Actor: "automation:autoBird", ConnectionGeneration: 1,
	})
	if err := controller.Send(ctx, payload); err != nil {
		t.Fatal(err)
	}
	result := make(chan error, 1)
	go func() { result <- controller.Send(ctx, payload) }()
	time.Sleep(10 * time.Millisecond)
	transport.setConnectionGeneration(2)
	select {
	case err := <-result:
		if !errors.Is(err, Outbound.ErrConnectionChanged) {
			t.Fatalf("queued command error = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("queued command did not finish")
	}
	transport.mu.Lock()
	defer transport.mu.Unlock()
	if len(transport.sends) != 1 {
		t.Fatalf("physical sends = %d, want 1", len(transport.sends))
	}
}

func TestControllerOrdersConnectionStatusBeforeCurrentFramesAndDropsStaleFrames(t *testing.T) {
	gameState := State.NewGameState()
	gameState.Session = State.SessionState{
		Generation: 5, BaselineGeneration: 5, ConnectionGeneration: 1,
		Status: "connected", LoggedIn: true, SocketReady: true, ChangedAt: time.Now().UTC(),
	}
	state := State.NewStore(gameState)
	transport := newPacingTransport()
	controller := NewController(context.Background(), transport, nil, state)
	defer controller.outbound.Close()

	transport.setConnectionGeneration(2)
	if controller.acceptFrameGeneration(1) {
		t.Fatal("stale connection frame was accepted after transport replacement")
	}
	if !controller.acceptFrameGeneration(2) {
		t.Fatal("current connection frame was rejected")
	}
	session := state.Snapshot().Session
	if session.ConnectionGeneration != 2 || session.Generation != 6 || session.BaselineGeneration != 0 {
		t.Fatalf("current frame did not establish status ordering: %+v", session)
	}
	if !controller.acceptFrameGeneration(0) {
		t.Fatal("unversioned replay frame was rejected")
	}
}

func TestControllerCancellationDiscardsQueuedObservedFrames(t *testing.T) {
	transport := newPacingTransport()
	gameState := State.NewGameState()
	gameState.Session = State.SessionState{
		Generation: 1, BaselineGeneration: 1, ConnectionGeneration: 1,
		Status: "connected", LoggedIn: true, SocketReady: true,
		Namespace: "EmpireEx_21", ChangedAt: transport.status.ChangedAt,
	}
	state := State.NewStore(gameState)
	registry := Ingest.NewRegistry()
	reducerStarted := make(chan struct{})
	releaseReducer := make(chan struct{})
	if err := registry.Register("one", func(
		context.Context,
		Protocol.Frame,
		*State.GameState,
		*GameData.Store,
	) ([]string, bool, error) {
		close(reducerStarted)
		<-releaseReducer
		return []string{"one"}, true, nil
	}); err != nil {
		t.Fatal(err)
	}
	pipeline := Ingest.NewPipeline(state, nil, registry)
	root, cancelRoot := context.WithCancel(context.Background())
	controller := NewController(root, transport, pipeline, state)
	if err := controller.Start(t.Context()); err != nil {
		t.Fatal(err)
	}
	transport.frames <- RawFrame{
		Payload: `%xt%EmpireEx_21%one%1%0%{}%`, Direction: Protocol.DirectionInbound,
		ObservedAt: time.Now().UTC(), ConnectionGeneration: 1,
	}
	<-reducerStarted
	wire, cancelWire := pipeline.WatchWire("two")
	defer cancelWire()
	transport.frames <- RawFrame{
		Payload: `%xt%EmpireEx_21%two%1%0%{}%`, Direction: Protocol.DirectionInbound,
		ObservedAt: time.Now().UTC(), ConnectionGeneration: 1,
	}
	wireFrame := <-wire
	cancelRoot()
	waitContext, cancelWait := context.WithTimeout(context.Background(), 250*time.Millisecond)
	defer cancelWait()
	if _, err := pipeline.WaitCommitted(waitContext, wireFrame.IngressID); !errors.Is(err, Outbound.ErrConnectionChanged) {
		t.Fatalf("queued frame cancellation error = %v", err)
	}
	close(releaseReducer)
}

func TestControllerWaitsUntilAutomationIsUnlocked(t *testing.T) {
	controller := NewController(context.Background(), newPacingTransport(), nil, nil)
	defer controller.outbound.Close()
	controller.SetAutomationLocked(true)
	start := time.Now()
	results := make(chan error, 3)
	for range 3 {
		go func() { results <- controller.WaitForAutomationUnlocked(context.Background()) }()
	}
	go func() {
		time.Sleep(25 * time.Millisecond)
		controller.SetAutomationLocked(false)
	}()
	for range 3 {
		if err := <-results; err != nil {
			t.Fatal(err)
		}
	}
	if elapsed := time.Since(start); elapsed < 20*time.Millisecond {
		t.Fatalf("automation lock ended too early: %s", elapsed)
	}
}

func TestControllerPausesAutomationForDirectBrowserTraffic(t *testing.T) {
	transport := newPacingTransport()
	transport.reportsOutboundCausation = true
	root, cancel := context.WithCancel(context.Background())
	defer cancel()
	controller := NewController(root, transport, nil, nil)
	defer controller.outbound.Close()
	if err := controller.Start(t.Context()); err != nil {
		t.Fatal(err)
	}
	transport.frames <- RawFrame{
		Payload: `%xt%EmpireEx_21%bup%1%0%{}%`, Direction: Protocol.DirectionOutbound,
		ObservedAt: time.Now().UTC(), ConnectionGeneration: 1,
	}
	deadline := time.Now().Add(time.Second)
	for !controller.AutomationLocked() && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if !controller.AutomationLocked() {
		t.Fatal("direct browser traffic did not pause automation")
	}
	if controller.AutomationLockedFor("automation:autoStation") {
		t.Fatal("direct browser traffic paused emergency Auto Station")
	}
	if !controller.AutomationLockedFor("automation:autoBird") {
		t.Fatal("direct browser traffic did not pause routine Auto Bird work")
	}
	if err := controller.outboundDispatchGate(
		context.Background(), Outbound.Metadata{Actor: "automation:autoStation"},
	); err != nil {
		t.Fatalf("direct traffic blocked emergency Auto Station dispatch: %v", err)
	}
	if err := controller.outboundDispatchGate(
		context.Background(), Outbound.Metadata{Actor: "automation:autoBird"},
	); !errors.Is(err, Outbound.ErrAutomationLocked) {
		t.Fatalf("routine automation dispatch error = %v", err)
	}
	controller.automationMu.Lock()
	if controller.directTrafficTimer != nil {
		controller.directTrafficTimer.Stop()
	}
	pausedUntil := time.Now().Add(-time.Millisecond)
	controller.directTrafficUntil = pausedUntil
	controller.automationMu.Unlock()
	controller.expireDirectTrafficPause(pausedUntil)
	transport.frames <- RawFrame{
		Payload: `%xt%EmpireEx_21%bup%1%0%{}%`, Direction: Protocol.DirectionOutbound,
		ObservedAt: time.Now().UTC(), ConnectionGeneration: 1, CausationOperationID: "op-1",
	}
	time.Sleep(5 * time.Millisecond)
	if controller.AutomationLocked() {
		t.Fatal("CitadelOps-caused outbound traffic was treated as direct browser traffic")
	}
}

func TestControllerDoesNotPauseAutomationForPassiveCastleDetailsRefresh(t *testing.T) {
	transport := newPacingTransport()
	transport.reportsOutboundCausation = true
	root, cancel := context.WithCancel(context.Background())
	defer cancel()
	controller := NewController(root, transport, nil, nil)
	defer controller.outbound.Close()
	if err := controller.Start(t.Context()); err != nil {
		t.Fatal(err)
	}
	transport.frames <- RawFrame{
		Payload: `%xt%EmpireEx_21%dcl%1%{}%`, Direction: Protocol.DirectionOutbound,
		ObservedAt: time.Now().UTC(), ConnectionGeneration: 1,
	}
	time.Sleep(5 * time.Millisecond)
	if controller.AutomationLocked() {
		t.Fatal("automatic castle-details refresh was treated as direct player traffic")
	}
}

func TestControllerLockBlocksAutomatedDispatchButKeepsInteractiveWorkAvailable(t *testing.T) {
	transport := newPacingTransport()
	controller := NewController(context.Background(), transport, nil, nil)
	defer controller.outbound.Close()
	controller.SetAutomationLocked(true)
	if !controller.AutomationLocked() {
		t.Fatal("automation lock was not active")
	}
	payload, err := Protocol.Encode(Protocol.Command{
		Namespace: "EmpireEx_21", Opcode: "ain", Payload: []byte(`{}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	background := Outbound.WithMetadata(context.Background(), Outbound.Metadata{Actor: "automation:autoBird"})
	if err := controller.Send(background, payload); !errors.Is(err, Outbound.ErrAutomationLocked) {
		t.Fatalf("background dispatch error = %v", err)
	}
	autoStation := Outbound.WithMetadata(context.Background(), Outbound.Metadata{Actor: "automation:autoStation"})
	if err := controller.Send(autoStation, payload); !errors.Is(err, Outbound.ErrAutomationLocked) {
		t.Fatalf("explicit lock Auto Station dispatch error = %v", err)
	}
	interactive := Outbound.WithMetadata(context.Background(), Outbound.Metadata{Actor: "ui"})
	if err := controller.Send(interactive, payload); err != nil {
		t.Fatalf("interactive dispatch: %v", err)
	}
	scheduled := Outbound.WithMetadata(context.Background(), Outbound.Metadata{Actor: "scheduler:rift"})
	if err := controller.Send(scheduled, payload); !errors.Is(err, Outbound.ErrAutomationLocked) {
		t.Fatalf("scheduled dispatch error = %v", err)
	}
	controller.SetAutomationLocked(false)
	if err := controller.Send(background, payload); err != nil {
		t.Fatalf("background dispatch after unlock: %v", err)
	}
	transport.mu.Lock()
	defer transport.mu.Unlock()
	if len(transport.sends) != 2 {
		t.Fatalf("physical sends = %d, want 2", len(transport.sends))
	}
}

func TestControllerSessionGenerationTracksReadyTransitions(t *testing.T) {
	gameState := State.NewGameState()
	gameState.Session.Generation = 40
	gameState.Session.BaselineGeneration = 40
	state := State.NewStore(gameState)
	controller := NewController(context.Background(), nil, nil, state)
	defer controller.outbound.Close()

	assertGeneration := func(want uint64) {
		t.Helper()
		if got := state.Snapshot().Session.Generation; got != want {
			t.Fatalf("session generation = %d, want %d", got, want)
		}
	}
	controller.applyStatus(Status{State: "authenticating", SocketReady: true})
	assertGeneration(40)
	controller.applyStatus(Status{State: "connected", LoggedIn: true, SocketReady: true})
	assertGeneration(41)
	if baseline := state.Snapshot().Session.BaselineGeneration; baseline != 0 {
		t.Fatalf("new session retained baseline generation %d", baseline)
	}
	controller.applyStatus(Status{
		State: "connected", LoggedIn: true, SocketReady: true, Namespace: "EmpireEx_22",
	})
	assertGeneration(41)
	controller.applyStatus(Status{State: "disconnected"})
	assertGeneration(41)
	controller.applyStatus(Status{State: "authenticating", SocketReady: true})
	assertGeneration(41)
	readyAt := time.Now().UTC()
	code := 0
	if _, err := state.Apply(func(gameState *State.GameState) ([]string, bool, error) {
		gameState.Observations["gbd"] = State.ProtocolObservation{
			Opcode: "gbd", LastDirection: "inbound", LastCode: &code, LastSeenAt: readyAt.Add(time.Millisecond),
		}
		return []string{"protocol"}, true, nil
	}); err != nil {
		t.Fatal(err)
	}
	controller.applyStatus(Status{
		State: "connected", LoggedIn: true, SocketReady: true, ChangedAt: readyAt,
	})
	assertGeneration(42)
	if baseline := state.Snapshot().Session.BaselineGeneration; baseline != 42 {
		t.Fatalf("current gbd baseline generation = %d, want 42", baseline)
	}
}

func TestControllerIgnoresStaleBufferedSessionStatus(t *testing.T) {
	gameState := State.NewGameState()
	gameState.Session.Generation = 9
	gameState.Session.BaselineGeneration = 9
	state := State.NewStore(gameState)
	controller := NewController(context.Background(), nil, nil, state)
	defer controller.outbound.Close()

	startedAt := time.Now().UTC()
	controller.applyStatus(Status{State: "connecting", ChangedAt: startedAt})
	readyAt := startedAt.Add(2 * time.Millisecond)
	controller.applyStatus(Status{
		State: "connected", LoggedIn: true, SocketReady: true, ChangedAt: readyAt,
	})
	controller.applyStatus(Status{State: "connecting", ChangedAt: startedAt.Add(time.Millisecond)})

	session := state.Snapshot().Session
	if session.Generation != 10 || !session.LoggedIn || !session.SocketReady || session.Status != "connected" {
		t.Fatalf("stale status replaced the current session: %+v", session)
	}
	if !session.ChangedAt.Equal(readyAt) {
		t.Fatalf("session timestamp = %s, want %s", session.ChangedAt, readyAt)
	}
}

func TestControllerAdvancesGenerationForReadySocketReplacement(t *testing.T) {
	gameState := State.NewGameState()
	connectedAt := time.Now().UTC()
	gameState.Session = State.SessionState{
		Generation: 20, BaselineGeneration: 20, ConnectionGeneration: 4,
		Status: "connected", LoggedIn: true, SocketReady: true, ChangedAt: connectedAt,
	}
	state := State.NewStore(gameState)
	controller := NewController(context.Background(), nil, nil, state)
	defer controller.outbound.Close()

	controller.applyStatus(Status{
		State: "connected", LoggedIn: true, SocketReady: true,
		ConnectionGeneration: 5, ChangedAt: connectedAt.Add(-time.Millisecond),
	})

	session := state.Snapshot().Session
	if session.Generation != 21 || session.BaselineGeneration != 0 || session.ConnectionGeneration != 5 {
		t.Fatalf("socket replacement did not start a fresh session epoch: %+v", session)
	}
}

// parkableTransport reports whether its connection loop is running so the
// controller can restart a transport that parked itself on a login failure
// while leaving a still-running errored transport alone.
type parkableTransport struct {
	*pacingTransport
	running bool
}

func (transport *parkableTransport) Running() bool {
	transport.mu.Lock()
	defer transport.mu.Unlock()
	return transport.running
}

func TestControllerRestartsOnlyParkedErroredTransports(t *testing.T) {
	transport := &parkableTransport{pacingTransport: newPacingTransport(), running: true}
	controller := NewController(context.Background(), transport, nil, nil)
	defer controller.outbound.Close()
	defer controller.Stop(context.Background())
	if err := controller.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	transport.mu.Lock()
	transport.status.State = "error"
	transport.status.LoggedIn = false
	transport.status.SocketReady = false
	transport.mu.Unlock()

	// A running transport in an error state keeps its own retry loop; the
	// controller must not stack a second start on top of it.
	if err := controller.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	transport.mu.Lock()
	starts := transport.starts
	transport.running = false
	transport.mu.Unlock()
	if starts != 1 {
		t.Fatalf("running errored transport was restarted: starts = %d", starts)
	}

	// Once the transport parked itself, Start restarts it (the transport
	// decides whether the saved login changed enough to try again).
	if err := controller.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	transport.mu.Lock()
	starts = transport.starts
	transport.mu.Unlock()
	if starts != 2 {
		t.Fatalf("parked transport was not restarted: starts = %d", starts)
	}
}
