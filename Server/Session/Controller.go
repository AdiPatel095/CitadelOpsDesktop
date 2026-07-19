package Session

import (
	"context"
	"fmt"
	"sync"
	"time"

	"CitadelDesktop/Server/Ingest"
	"CitadelDesktop/Server/Outbound"
	"CitadelDesktop/Server/Protocol"
	"CitadelDesktop/Server/State"
)

type Controller struct {
	root      context.Context
	transport Transport
	ingest    *Ingest.Pipeline
	state     *State.Store

	lifecycleMu sync.Mutex
	mu          sync.Mutex
	cancel      context.CancelFunc
	runID       uint64

	outbound *Outbound.Router

	policyMu            sync.RWMutex
	attackDelayProvider func() time.Duration

	automationMu          sync.Mutex
	automationLocked      bool
	directTrafficUntil    time.Time
	directTrafficTimer    *time.Timer
	automationLockChanged chan struct{}
}

const directTrafficQuietPeriod = 2 * time.Second

type queuedIngestFrame struct {
	observed             Ingest.ObservedFrame
	connectionGeneration uint64
}

type ingestFrameQueue struct {
	mu       sync.Mutex
	changed  *sync.Cond
	frames   []queuedIngestFrame
	capacity int
	closed   bool
}

func newIngestFrameQueue(capacity int) *ingestFrameQueue {
	queue := &ingestFrameQueue{capacity: capacity}
	queue.changed = sync.NewCond(&queue.mu)
	return queue
}

func (queue *ingestFrameQueue) push(frame queuedIngestFrame) bool {
	queue.mu.Lock()
	defer queue.mu.Unlock()
	for !queue.closed && len(queue.frames) >= queue.capacity {
		queue.changed.Wait()
	}
	if queue.closed {
		return false
	}
	queue.frames = append(queue.frames, frame)
	queue.changed.Signal()
	return true
}

func (queue *ingestFrameQueue) pop() (queuedIngestFrame, bool) {
	queue.mu.Lock()
	defer queue.mu.Unlock()
	for !queue.closed && len(queue.frames) == 0 {
		queue.changed.Wait()
	}
	if len(queue.frames) == 0 {
		return queuedIngestFrame{}, false
	}
	frame := queue.frames[0]
	queue.frames[0] = queuedIngestFrame{}
	queue.frames = queue.frames[1:]
	queue.changed.Signal()
	return frame, true
}

func (queue *ingestFrameQueue) closeAndTakePending() []queuedIngestFrame {
	queue.mu.Lock()
	defer queue.mu.Unlock()
	if queue.closed {
		return nil
	}
	queue.closed = true
	pending := queue.frames
	queue.frames = nil
	queue.changed.Broadcast()
	return pending
}

func NewController(root context.Context, transport Transport, ingest *Ingest.Pipeline, state *State.Store) *Controller {
	if root == nil {
		root = context.Background()
	}
	controller := &Controller{
		root: root, transport: transport, ingest: ingest, state: state,
		automationLockChanged: make(chan struct{}, 1),
	}
	controller.outbound = Outbound.NewRouter(root, Outbound.Config{
		Send: func(ctx context.Context, payload []byte) error {
			if controller.transport == nil {
				return ErrTransportUnavailable
			}
			metadata := Outbound.MetadataFromContext(ctx)
			if metadata.ConnectionGeneration > 0 &&
				metadata.ConnectionGeneration != controller.ConnectionGeneration() {
				return Outbound.ErrConnectionChanged
			}
			if err := controller.transport.Send(ctx, payload); err != nil {
				return err
			}
			if controller.ingest != nil {
				controller.ingest.RecordAppOutbound(string(payload), metadata.Actor)
			}
			return nil
		},
		Ready:        controller.Ready,
		CommandDelay: func() time.Duration { return 25 * time.Millisecond },
		AttackDelay:  controller.attackDelay,
		Gate:         controller.outboundDispatchGate,
	})
	if transport != nil {
		controller.applyStatus(transport.Status())
	}
	return controller
}

func (controller *Controller) Start(_ context.Context) error {
	controller.lifecycleMu.Lock()
	defer controller.lifecycleMu.Unlock()
	controller.mu.Lock()
	if controller.cancel != nil {
		controller.mu.Unlock()
		return nil
	}
	if controller.transport == nil {
		controller.mu.Unlock()
		return ErrTransportUnavailable
	}
	runContext, cancel := context.WithCancel(controller.root)
	controller.runID++
	runID := controller.runID
	controller.cancel = cancel
	controller.mu.Unlock()

	starting := controller.transport.Status()
	starting.State = "starting"
	starting.ChangedAt = time.Now().UTC()
	controller.applyStatus(starting)
	if err := controller.transport.Start(runContext); err != nil {
		cancel()
		controller.mu.Lock()
		if controller.runID == runID {
			controller.cancel = nil
		}
		controller.mu.Unlock()
		status := controller.transport.Status()
		if status.Detail == "" {
			status.Detail = err.Error()
		}
		controller.applyStatus(status)
		return err
	}
	if controller.ingest != nil {
		controller.ingest.BeginWebSocketGameSession()
	}
	go controller.run(runContext, runID)
	controller.applyStatus(controller.transport.Status())
	return nil
}

func (controller *Controller) Stop(ctx context.Context) error {
	controller.lifecycleMu.Lock()
	defer controller.lifecycleMu.Unlock()
	controller.mu.Lock()
	cancel := controller.cancel
	controller.cancel = nil
	controller.runID++
	controller.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	if controller.transport == nil {
		return nil
	}
	controller.outbound.Reset()
	err := controller.transport.Stop(ctx)
	controller.applyStatus(controller.transport.Status())
	return err
}

func (controller *Controller) SetAttackDelayProvider(provider func() time.Duration) {
	controller.policyMu.Lock()
	controller.attackDelayProvider = provider
	controller.policyMu.Unlock()
}

func (controller *Controller) attackDelay() time.Duration {
	controller.policyMu.RLock()
	provider := controller.attackDelayProvider
	controller.policyMu.RUnlock()
	if provider == nil {
		return 0
	}
	return provider()
}

func (controller *Controller) SetAutomationLocked(locked bool) {
	if controller == nil {
		return
	}
	controller.automationMu.Lock()
	changed := controller.automationLocked != locked
	controller.automationLocked = locked
	if changed {
		close(controller.automationLockChanged)
		controller.automationLockChanged = make(chan struct{})
	}
	controller.automationMu.Unlock()
	if !changed {
		return
	}
	controller.outbound.Notify()
}

func (controller *Controller) observeDirectTraffic(observedAt time.Time) {
	if observedAt.IsZero() {
		observedAt = time.Now().UTC()
	}
	until := observedAt.Add(directTrafficQuietPeriod)
	controller.automationMu.Lock()
	if !until.After(controller.directTrafficUntil) {
		controller.automationMu.Unlock()
		return
	}
	controller.directTrafficUntil = until
	if controller.directTrafficTimer != nil {
		controller.directTrafficTimer.Stop()
	}
	close(controller.automationLockChanged)
	controller.automationLockChanged = make(chan struct{})
	delay := time.Until(until)
	if delay < 0 {
		delay = 0
	}
	controller.directTrafficTimer = time.AfterFunc(delay, func() {
		controller.expireDirectTrafficPause(until)
	})
	controller.automationMu.Unlock()
	controller.outbound.Notify()
}

func (controller *Controller) expireDirectTrafficPause(expected time.Time) {
	controller.automationMu.Lock()
	if !controller.directTrafficUntil.Equal(expected) || time.Now().Before(controller.directTrafficUntil) {
		controller.automationMu.Unlock()
		return
	}
	controller.directTrafficUntil = time.Time{}
	controller.directTrafficTimer = nil
	close(controller.automationLockChanged)
	controller.automationLockChanged = make(chan struct{})
	controller.automationMu.Unlock()
	controller.outbound.Notify()
}

func (controller *Controller) AutomationLocked() bool {
	if controller == nil {
		return false
	}
	controller.automationMu.Lock()
	locked := controller.automationLocked || time.Now().Before(controller.directTrafficUntil)
	controller.automationMu.Unlock()
	return locked
}

func (controller *Controller) outboundDispatchGate(_ context.Context, metadata Outbound.Metadata) error {
	if Outbound.YieldsToAutomationLock(metadata.Actor) && controller.AutomationLocked() {
		return Outbound.ErrAutomationLocked
	}
	if metadata.ConnectionGeneration > 0 &&
		metadata.ConnectionGeneration != controller.ConnectionGeneration() {
		return Outbound.ErrConnectionChanged
	}
	return nil
}

func (controller *Controller) WaitForAutomationUnlocked(ctx context.Context) error {
	for {
		controller.automationMu.Lock()
		locked := controller.automationLocked
		directTrafficUntil := controller.directTrafficUntil
		changed := controller.automationLockChanged
		controller.automationMu.Unlock()
		if !locked && !time.Now().Before(directTrafficUntil) {
			return nil
		}
		var timer <-chan time.Time
		var expiry *time.Timer
		if !locked && !directTrafficUntil.IsZero() {
			expiry = time.NewTimer(time.Until(directTrafficUntil))
			timer = expiry.C
		}
		select {
		case <-ctx.Done():
			if expiry != nil {
				expiry.Stop()
			}
			return ctx.Err()
		case <-changed:
			if expiry != nil {
				expiry.Stop()
			}
		case <-timer:
		}
	}
}

func (controller *Controller) SelectBrowser(preference string) error {
	controller.lifecycleMu.Lock()
	defer controller.lifecycleMu.Unlock()
	controller.mu.Lock()
	defer controller.mu.Unlock()
	if controller.cancel != nil {
		return fmt.Errorf("stop the active game session before changing browsers")
	}
	selector, ok := controller.transport.(BrowserSelector)
	if !ok {
		return fmt.Errorf("the configured game transport does not support browser selection")
	}
	if err := selector.SelectBrowser(preference); err != nil {
		return err
	}
	controller.applyStatus(controller.transport.Status())
	return nil
}

func (controller *Controller) Send(ctx context.Context, payload []byte) error {
	if controller.transport == nil {
		return ErrTransportUnavailable
	}
	if !controller.Ready() {
		return fmt.Errorf("game websocket is not ready")
	}
	return controller.outbound.Send(ctx, payload)
}

func (controller *Controller) NextAllowed(lane Outbound.Lane) time.Time {
	if controller == nil || controller.outbound == nil {
		return time.Time{}
	}
	return controller.outbound.NextAllowed(lane)
}

func (controller *Controller) DispatchLatency() Outbound.DispatchLatencyStats {
	if controller == nil || controller.outbound == nil {
		return Outbound.DispatchLatencyStats{}
	}
	return controller.outbound.DispatchLatency()
}

func (controller *Controller) ConnectionGeneration() uint64 {
	if controller == nil {
		return 0
	}
	return controller.Status().ConnectionGeneration
}

func (controller *Controller) CorrelatesResponses() bool {
	provider, ok := controller.transport.(ResponseCorrelationTransport)
	return ok && provider.CorrelatesResponses()
}

func (controller *Controller) Ready() bool {
	status := controller.Status()
	return status.SocketReady && status.LoggedIn
}

func (controller *Controller) Namespace() string {
	status := controller.Status()
	if status.Namespace == "" {
		return "EmpireEx_21"
	}
	return status.Namespace
}

func (controller *Controller) Status() Status {
	if controller.transport == nil {
		return Status{State: "unavailable", Namespace: "EmpireEx_21", ChangedAt: time.Now().UTC()}
	}
	return controller.transport.Status()
}

func (controller *Controller) run(ctx context.Context, runID uint64) {
	ingestQueue := newIngestFrameQueue(8192)
	ingestDone := make(chan struct{})
	discardPending := func(pending []queuedIngestFrame) {
		for _, queued := range pending {
			controller.ingest.DiscardObserved(queued.observed, Outbound.ErrConnectionChanged)
		}
	}
	go func() {
		defer close(ingestDone)
		for {
			queued, ok := ingestQueue.pop()
			if !ok {
				return
			}
			if ctx.Err() != nil || !controller.acceptFrameGeneration(queued.connectionGeneration) {
				controller.ingest.DiscardObserved(queued.observed, Outbound.ErrConnectionChanged)
				continue
			}
			_, _ = controller.ingest.CommitFrameGuarded(ctx, queued.observed, func() error {
				if !controller.transportFrameGenerationMatches(queued.connectionGeneration) {
					return Outbound.ErrConnectionChanged
				}
				return nil
			})
		}
	}()
	cancelQueueWatch := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			discardPending(ingestQueue.closeAndTakePending())
		case <-cancelQueueWatch:
		}
	}()
	defer func() {
		close(cancelQueueWatch)
		discardPending(ingestQueue.closeAndTakePending())
		<-ingestDone
	}()
	for {
		select {
		case <-ctx.Done():
			return
		case status, ok := <-controller.transport.StatusChanges():
			if !ok {
				return
			}
			controller.applyStatus(status)
		case frame, ok := <-controller.transport.Frames():
			if !ok {
				controller.mu.Lock()
				var cancel context.CancelFunc
				if controller.runID == runID {
					cancel = controller.cancel
					controller.cancel = nil
				}
				controller.mu.Unlock()
				if cancel != nil {
					cancel()
				}
				return
			}
			if frame.Direction == Protocol.DirectionOutbound && frame.CausationOperationID == "" {
				if reporter, ok := controller.transport.(OutboundCausationTransport); ok && reporter.ReportsOutboundCausation() {
					controller.observeDirectTraffic(frame.ObservedAt)
				}
			}
			if controller.ingest == nil {
				continue
			}
			if !controller.acceptFrameGeneration(frame.ConnectionGeneration) {
				continue
			}
			observed, err := controller.ingest.DecodeTransportFrameAt(
				frame.Payload, frame.Direction, frame.ObservedAt, frame.ResponseToken, frame.CausationOperationID,
			)
			if err != nil {
				continue
			}
			if observed.Frame.Namespace != "" {
				status := controller.Status()
				if status.Namespace != observed.Frame.Namespace {
					status.Namespace = observed.Frame.Namespace
					status.ChangedAt = time.Now().UTC()
					controller.applyStatus(status)
				}
			}
			observed.ConnectionGeneration = frame.ConnectionGeneration
			if !ingestQueue.push(queuedIngestFrame{
				observed: observed, connectionGeneration: frame.ConnectionGeneration,
			}) {
				controller.ingest.DiscardObserved(observed, ctx.Err())
				return
			}
		}
	}
}

func (controller *Controller) transportFrameGenerationMatches(connectionGeneration uint64) bool {
	if connectionGeneration == 0 {
		return true
	}
	status := controller.transport.Status()
	return status.ConnectionGeneration == 0 || status.ConnectionGeneration == connectionGeneration
}

func (controller *Controller) acceptFrameGeneration(connectionGeneration uint64) bool {
	if connectionGeneration == 0 {
		return true
	}
	status := controller.transport.Status()
	if status.ConnectionGeneration > 0 && status.ConnectionGeneration != connectionGeneration {
		return false
	}
	if controller.stateSessionConnectionGeneration() == connectionGeneration {
		return true
	}
	if status.ConnectionGeneration != connectionGeneration {
		return false
	}
	controller.applyStatus(status)
	return controller.stateSessionConnectionGeneration() == connectionGeneration
}

func (controller *Controller) stateSessionConnectionGeneration() uint64 {
	if controller == nil || controller.state == nil {
		return 0
	}
	return controller.state.Session().ConnectionGeneration
}

func (controller *Controller) applyStatus(status Status) {
	if status.ChangedAt.IsZero() {
		status.ChangedAt = time.Now().UTC()
	}
	if status.Namespace == "" {
		status.Namespace = "EmpireEx_21"
	}
	if controller.state == nil {
		return
	}
	_, _ = controller.state.Apply(func(gameState *State.GameState) ([]string, bool, error) {
		if status.ConnectionGeneration == 0 {
			status.ConnectionGeneration = gameState.Session.ConnectionGeneration
		}
		if status.ConnectionGeneration > 0 &&
			status.ConnectionGeneration < gameState.Session.ConnectionGeneration {
			return nil, false, nil
		}
		if !gameState.Session.ChangedAt.IsZero() && status.ChangedAt.Before(gameState.Session.ChangedAt) &&
			status.ConnectionGeneration <= gameState.Session.ConnectionGeneration {
			return nil, false, nil
		}
		generation := gameState.Session.Generation
		baselineGeneration := gameState.Session.BaselineGeneration
		wasReady := gameState.Session.LoggedIn && gameState.Session.SocketReady
		isReady := status.LoggedIn && status.SocketReady
		connectionChanged := status.ConnectionGeneration > 0 &&
			status.ConnectionGeneration != gameState.Session.ConnectionGeneration
		if isReady && (!wasReady || connectionChanged) {
			baselineGeneration = 0
			if generation != ^uint64(0) {
				generation++
			}
			if observation := gameState.Observations["gbd"]; observation.LastDirection == "inbound" &&
				observation.LastCode != nil && *observation.LastCode == 0 && observation.LastError == "" &&
				!observation.LastSeenAt.Before(status.ChangedAt) {
				baselineGeneration = generation
			}
		}
		next := State.SessionState{
			Generation: generation, BaselineGeneration: baselineGeneration,
			ConnectionGeneration: status.ConnectionGeneration,
			Status:               status.State, LoggedIn: status.LoggedIn, SocketReady: status.SocketReady,
			BrowserID: status.BrowserID, BrowserName: status.BrowserName,
			ServerURL: status.ServerURL, Namespace: status.Namespace, Detail: status.Detail,
			CooldownUntil: status.CooldownUntil, RetryAt: status.RetryAt, ChangedAt: status.ChangedAt,
		}
		if gameState.Session == next {
			return nil, false, nil
		}
		gameState.Session = next
		return []string{"session"}, true, nil
	})
}
