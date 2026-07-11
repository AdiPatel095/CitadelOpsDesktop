package Session

import (
	"context"
	"fmt"
	"sync"
	"time"

	"CitadelDesktop/Server/Ingest"
	"CitadelDesktop/Server/State"
)

type Controller struct {
	root      context.Context
	transport Transport
	ingest    *Ingest.Pipeline
	state     *State.Store

	mu     sync.Mutex
	cancel context.CancelFunc
}

func NewController(root context.Context, transport Transport, ingest *Ingest.Pipeline, state *State.Store) *Controller {
	if root == nil {
		root = context.Background()
	}
	controller := &Controller{root: root, transport: transport, ingest: ingest, state: state}
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
	controller.applyStatus(controller.transport.Status())
	return err
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
	return controller.transport.Send(ctx, payload)
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
			decoded, err := controller.ingest.HandleRaw(ctx, frame.Payload, frame.Direction)
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
			ChangedAt: status.ChangedAt,
		}
		if gameState.Session == next {
			return nil, false, nil
		}
		gameState.Session = next
		return []string{"session"}, true, nil
	})
}
