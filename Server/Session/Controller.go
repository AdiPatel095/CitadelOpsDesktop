package Session

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"CitadelDesktop/Server/Ingest"
	"CitadelDesktop/Server/Protocol"
	"CitadelDesktop/Server/State"
)

const defaultManualFocusHold = 30 * time.Second

type Controller struct {
	root      context.Context
	transport Transport
	ingest    *Ingest.Pipeline
	state     *State.Store

	mu     sync.Mutex
	cancel context.CancelFunc

	attackMu            sync.Mutex
	lastAttackSend      time.Time
	attackDelayProvider func() time.Duration

	manualMu                sync.Mutex
	manualFocusUntil        time.Time
	manualFocusHoldProvider func() time.Duration
	manualFocusChanged      chan struct{}
}

func NewController(root context.Context, transport Transport, ingest *Ingest.Pipeline, state *State.Store) *Controller {
	if root == nil {
		root = context.Background()
	}
	controller := &Controller{
		root: root, transport: transport, ingest: ingest, state: state,
		manualFocusChanged: make(chan struct{}, 1),
	}
	if transport != nil {
		controller.applyStatus(transport.Status())
	}
	return controller
}

func (controller *Controller) Start(_ context.Context) error {
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
	controller.cancel = cancel
	controller.mu.Unlock()

	starting := controller.transport.Status()
	starting.State = "starting"
	starting.ChangedAt = time.Now().UTC()
	controller.applyStatus(starting)
	if err := controller.transport.Start(runContext); err != nil {
		cancel()
		controller.mu.Lock()
		controller.cancel = nil
		controller.mu.Unlock()
		status := controller.transport.Status()
		if status.Detail == "" {
			status.Detail = err.Error()
		}
		controller.applyStatus(status)
		return err
	}
	go controller.run(runContext)
	controller.applyStatus(controller.transport.Status())
	return nil
}

func (controller *Controller) Stop(ctx context.Context) error {
	controller.mu.Lock()
	cancel := controller.cancel
	controller.cancel = nil
	controller.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	if controller.transport == nil {
		return nil
	}
	err := controller.transport.Stop(ctx)
	controller.attackMu.Lock()
	controller.lastAttackSend = time.Time{}
	controller.attackMu.Unlock()
	controller.applyStatus(controller.transport.Status())
	return err
}

func (controller *Controller) SetAttackDelayProvider(provider func() time.Duration) {
	controller.attackMu.Lock()
	controller.attackDelayProvider = provider
	controller.attackMu.Unlock()
}

func (controller *Controller) SetManualFocusHoldProvider(provider func() time.Duration) {
	controller.manualMu.Lock()
	controller.manualFocusHoldProvider = provider
	controller.manualMu.Unlock()
}

func (controller *Controller) RecordManualActivity(activity Activity) {
	observedAt := activity.ObservedAt
	if observedAt.IsZero() {
		observedAt = time.Now().UTC()
	}
	controller.manualMu.Lock()
	hold := defaultManualFocusHold
	if controller.manualFocusHoldProvider != nil {
		hold = controller.manualFocusHoldProvider()
	}
	if hold <= 0 {
		hold = defaultManualFocusHold
	}
	until := observedAt.Add(hold)
	if until.After(controller.manualFocusUntil) {
		controller.manualFocusUntil = until
	}
	controller.manualMu.Unlock()
	select {
	case controller.manualFocusChanged <- struct{}{}:
	default:
	}
}

func (controller *Controller) WaitForManualFocusIdle(ctx context.Context) error {
	for {
		controller.manualMu.Lock()
		until := controller.manualFocusUntil
		controller.manualMu.Unlock()
		wait := time.Until(until)
		if wait <= 0 {
			return nil
		}
		timer := time.NewTimer(wait)
		select {
		case <-ctx.Done():
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
			return ctx.Err()
		case <-controller.manualFocusChanged:
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
		case <-timer.C:
		}
	}
}

func (controller *Controller) SelectBrowser(preference string) error {
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
	frame, err := Protocol.Decode(string(payload), Protocol.DirectionOutbound, time.Now().UTC())
	if err != nil || frame.Opcode != "cra" {
		return controller.transport.Send(ctx, payload)
	}
	controller.attackMu.Lock()
	defer controller.attackMu.Unlock()
	if !controller.lastAttackSend.IsZero() && controller.attackDelayProvider != nil {
		wait := controller.attackDelayProvider() - time.Since(controller.lastAttackSend)
		if wait > 0 {
			timer := time.NewTimer(wait)
			defer timer.Stop()
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-timer.C:
			}
		}
	}
	if err := controller.transport.Send(ctx, payload); err != nil {
		return err
	}
	controller.lastAttackSend = time.Now()
	return nil
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

func (controller *Controller) run(ctx context.Context) {
	var activities <-chan Activity
	if source, ok := controller.transport.(ActivitySource); ok {
		activities = source.Activities()
	}
	for {
		select {
		case <-ctx.Done():
			return
		case status, ok := <-controller.transport.StatusChanges():
			if !ok {
				return
			}
			controller.applyStatus(status)
		case activity, ok := <-activities:
			if !ok {
				activities = nil
				continue
			}
			activity.Kind = strings.TrimSpace(activity.Kind)
			if activity.Kind != "" {
				controller.RecordManualActivity(activity)
			}
		case frame, ok := <-controller.transport.Frames():
			if !ok {
				controller.mu.Lock()
				cancel := controller.cancel
				controller.cancel = nil
				controller.mu.Unlock()
				if cancel != nil {
					cancel()
				}
				return
			}
			if controller.ingest == nil {
				continue
			}
			decoded, err := controller.ingest.HandleRawAt(ctx, frame.Payload, frame.Direction, frame.ObservedAt)
			if err == nil && decoded.Frame.Namespace != "" {
				status := controller.Status()
				if status.Namespace != decoded.Frame.Namespace {
					status.Namespace = decoded.Frame.Namespace
					status.ChangedAt = time.Now().UTC()
					controller.applyStatus(status)
				}
			}
		}
	}
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
		next := State.SessionState{
			Status: status.State, LoggedIn: status.LoggedIn, SocketReady: status.SocketReady,
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
