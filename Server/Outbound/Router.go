package Outbound

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"CitadelDesktop/Server/Protocol"
)

const (
	defaultMaximumQueuedPerLane = 2048
	maximumMeasuredOperations   = 4096
)

var (
	ErrClosed            = errors.New("outbound router is closed")
	ErrNotReady          = errors.New("game websocket is not ready")
	ErrQueueFull         = errors.New("outbound lane queue is full")
	ErrReset             = errors.New("outbound queue was reset")
	ErrConnectionChanged = errors.New("game websocket connection changed before dispatch")
)

type SendFunc func(context.Context, []byte) error
type ReadyFunc func() bool
type DelayFunc func() time.Duration
type DispatchGate func(context.Context, Metadata) error

type Config struct {
	Send                 SendFunc
	Ready                ReadyFunc
	CommandDelay         DelayFunc
	AttackDelay          DelayFunc
	Gate                 DispatchGate
	MaximumQueuedPerLane int
}

type queuedCommand struct {
	id           uint64
	lane         Lane
	metadata     Metadata
	payload      []byte
	ctx          context.Context
	cancel       context.CancelFunc
	result       chan error
	enqueuedAt   time.Time
	dispatchedAt time.Time
	fastPath     bool
}

type Router struct {
	ctx    context.Context
	cancel context.CancelFunc
	config Config

	mu                 sync.Mutex
	nextID             uint64
	queues             map[Lane][]*queuedCommand
	inFlight           map[uint64]*queuedCommand
	notify             chan struct{}
	nextAllowed        map[Lane]time.Time
	latency            *dispatchLatencyWindow
	measuredOperations map[string]struct{}
	measuredOrder      []string
	closed             bool
	closeErr           error
}

func NewRouter(ctx context.Context, config Config) *Router {
	if ctx == nil {
		ctx = context.Background()
	}
	routerContext, cancel := context.WithCancel(ctx)
	if config.MaximumQueuedPerLane <= 0 {
		config.MaximumQueuedPerLane = defaultMaximumQueuedPerLane
	}
	router := &Router{
		ctx: routerContext, cancel: cancel, config: config,
		queues: map[Lane][]*queuedCommand{
			LaneCommand: {}, LaneAttackLaunch: {},
		},
		inFlight:           map[uint64]*queuedCommand{},
		notify:             make(chan struct{}, 1),
		nextAllowed:        map[Lane]time.Time{},
		latency:            newDispatchLatencyWindow(defaultDispatchLatencyWindow),
		measuredOperations: map[string]struct{}{},
	}
	go router.run()
	go func() {
		<-routerContext.Done()
		router.closeWithError(routerContext.Err())
	}()
	return router
}

func (router *Router) Send(ctx context.Context, payload []byte) error {
	if router == nil {
		return ErrClosed
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if len(payload) == 0 {
		return fmt.Errorf("outbound payload is empty")
	}
	frame, err := Protocol.Decode(string(payload), Protocol.DirectionOutbound, time.Now().UTC())
	if err != nil {
		return fmt.Errorf("route outbound payload: %w", err)
	}
	lane := LaneCommand
	if frame.Opcode == "cra" {
		lane = LaneAttackLaunch
	}
	if router.config.Ready != nil && !router.config.Ready() {
		return ErrNotReady
	}
	command, err := router.enqueue(ctx, lane, payload, MetadataFromContext(ctx))
	if err != nil {
		return err
	}
	select {
	case err := <-command.result:
		return err
	case <-ctx.Done():
		if router.removeQueued(command.id, command.lane) {
			return ctx.Err()
		}
		select {
		case err := <-command.result:
			return err
		case <-router.ctx.Done():
			return router.closedError()
		}
	case <-router.ctx.Done():
		return router.closedError()
	}
}

func (router *Router) Reset() {
	if router == nil {
		return
	}
	router.mu.Lock()
	commands := router.takeAllLocked()
	router.nextAllowed = map[Lane]time.Time{}
	router.mu.Unlock()
	for _, command := range commands {
		command.cancel()
		err := error(ErrReset)
		if !command.dispatchedAt.IsZero() {
			err = MarkIndeterminate(err)
		}
		completeQueuedCommand(command, err)
	}
	router.signal()
}

func (router *Router) Close() {
	if router == nil {
		return
	}
	router.closeWithError(ErrClosed)
	router.cancel()
}

func (router *Router) Notify() {
	if router != nil {
		router.signal()
	}
}

func (router *Router) NextAllowed(lane Lane) time.Time {
	if router == nil {
		return time.Time{}
	}
	router.mu.Lock()
	next := router.nextAllowed[lane]
	router.mu.Unlock()
	return next
}

func (router *Router) DispatchLatency() DispatchLatencyStats {
	if router == nil || router.latency == nil {
		return emptyDispatchLatencyStats()
	}
	return router.latency.snapshot()
}

func (router *Router) enqueue(ctx context.Context, lane Lane, payload []byte, metadata Metadata) (*queuedCommand, error) {
	router.mu.Lock()
	if router.closed {
		err := router.closeErr
		router.mu.Unlock()
		if err == nil {
			err = ErrClosed
		}
		return nil, err
	}
	queue := router.queues[lane]
	if len(queue) >= router.config.MaximumQueuedPerLane {
		router.mu.Unlock()
		return nil, fmt.Errorf("%w: %s", ErrQueueFull, lane)
	}
	now := time.Now()
	fastPath := len(router.inFlight) == 0 &&
		len(router.queues[LaneCommand]) == 0 && len(router.queues[LaneAttackLaunch]) == 0 &&
		!router.nextAllowed[lane].After(now)
	router.nextID++
	commandContext, cancel := context.WithCancel(ctx)
	command := &queuedCommand{
		id: router.nextID, lane: lane, metadata: metadata,
		payload: append([]byte(nil), payload...), ctx: commandContext, cancel: cancel, result: make(chan error, 1),
		enqueuedAt: now, fastPath: fastPath,
	}
	router.queues[lane] = append(queue, command)
	router.mu.Unlock()
	router.signal()
	return command, nil
}

func (router *Router) run() {
	for {
		command, ok := router.next()
		if !ok {
			return
		}
		err := router.dispatch(command)
		if err == nil {
			router.recordDispatch(command)
			router.recordLatency(command, time.Now())
		}
		router.finish(command, err)
	}
}

func (router *Router) recordLatency(command *queuedCommand, completedAt time.Time) {
	if router.latency == nil || command.dispatchedAt.IsZero() {
		return
	}
	sample := dispatchLatencySample{
		observedAt: completedAt,
		fastPath:   command.fastPath,
		queue:      nonNegativeDuration(command.dispatchedAt.Sub(command.enqueuedAt)),
		transport:  nonNegativeDuration(completedAt.Sub(command.dispatchedAt)),
	}
	if submittedAt := command.metadata.SubmittedAt; !submittedAt.IsZero() && !submittedAt.After(command.dispatchedAt) &&
		router.claimFirstSuccessfulIntentCommand(command.metadata.OperationID) {
		sample.intentOrigin = true
		sample.intentToDispatch = nonNegativeDuration(command.dispatchedAt.Sub(submittedAt))
		sample.intentToTransport = nonNegativeDuration(completedAt.Sub(submittedAt))
	}
	router.latency.record(sample)
}

func (router *Router) claimFirstSuccessfulIntentCommand(operationID string) bool {
	operationID = strings.TrimSpace(operationID)
	if operationID == "" {
		return true
	}
	router.mu.Lock()
	defer router.mu.Unlock()
	if _, measured := router.measuredOperations[operationID]; measured {
		return false
	}
	if len(router.measuredOrder) >= maximumMeasuredOperations {
		oldest := router.measuredOrder[0]
		delete(router.measuredOperations, oldest)
		copy(router.measuredOrder, router.measuredOrder[1:])
		router.measuredOrder = router.measuredOrder[:len(router.measuredOrder)-1]
	}
	router.measuredOperations[operationID] = struct{}{}
	router.measuredOrder = append(router.measuredOrder, operationID)
	return true
}

func (router *Router) next() (*queuedCommand, bool) {
	lanes := []Lane{LaneCommand, LaneAttackLaunch}
	for {
		router.mu.Lock()
		if router.closed {
			router.mu.Unlock()
			return nil, false
		}
		now := time.Now()
		var expired []*queuedCommand
		var selected *queuedCommand
		selectedLane := Lane("")
		selectedIndex := -1
		var wakeAt time.Time
		hasQueued := false
		for _, lane := range lanes {
			queue := router.queues[lane]
			remaining := queue[:0]
			for _, command := range queue {
				if command.ctx.Err() != nil || !command.metadata.Deadline.IsZero() && !command.metadata.Deadline.After(now) {
					expired = append(expired, command)
					continue
				}
				remaining = append(remaining, command)
			}
			router.queues[lane] = remaining
			if len(remaining) == 0 {
				continue
			}
			hasQueued = true
			allowedAt := router.nextAllowed[lane]
			for index, command := range remaining {
				if allowedAt.After(now) {
					candidateWake := allowedAt
					if !command.metadata.Deadline.IsZero() && command.metadata.Deadline.Before(candidateWake) {
						candidateWake = command.metadata.Deadline
					}
					if wakeAt.IsZero() || candidateWake.Before(wakeAt) {
						wakeAt = candidateWake
					}
					continue
				}
				if selected == nil || queuedCommandBeforeAt(command, selected, now) {
					selected = command
					selectedLane = lane
					selectedIndex = index
				}
			}
		}
		if selected != nil {
			queue := router.queues[selectedLane]
			router.queues[selectedLane] = append(queue[:selectedIndex], queue[selectedIndex+1:]...)
			router.inFlight[selected.id] = selected
		}
		router.mu.Unlock()
		for _, command := range expired {
			err := command.ctx.Err()
			if err == nil {
				err = context.DeadlineExceeded
			}
			completeQueuedCommand(command, err)
			command.cancel()
		}
		if selected != nil {
			return selected, true
		}
		if !hasQueued {
			select {
			case <-router.ctx.Done():
				return nil, false
			case <-router.notify:
			}
			continue
		}
		wait := time.Until(wakeAt)
		if wakeAt.IsZero() || wait <= 0 {
			continue
		}
		timer := time.NewTimer(wait)
		select {
		case <-router.ctx.Done():
			if !timer.Stop() {
				<-timer.C
			}
			return nil, false
		case <-router.notify:
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

func (router *Router) dispatch(command *queuedCommand) error {
	if err := command.ctx.Err(); err != nil {
		return err
	}
	if !command.metadata.Deadline.IsZero() && !command.metadata.Deadline.After(time.Now()) {
		return context.DeadlineExceeded
	}
	if router.config.Gate != nil {
		if err := router.config.Gate(command.ctx, command.metadata); err != nil {
			return err
		}
	}
	if router.config.Ready != nil && !router.config.Ready() {
		return ErrNotReady
	}
	if router.config.Send == nil {
		return ErrClosed
	}
	command.dispatchedAt = time.Now()
	return router.config.Send(command.ctx, command.payload)
}

func (router *Router) recordDispatch(command *queuedCommand) {
	var delay time.Duration
	switch command.lane {
	case LaneAttackLaunch:
		if router.config.AttackDelay != nil {
			delay = router.config.AttackDelay()
		}
	default:
		if router.config.CommandDelay != nil {
			delay = router.config.CommandDelay()
		}
	}
	if delay < 0 {
		delay = 0
	}
	dispatchedAt := command.dispatchedAt
	if dispatchedAt.IsZero() {
		dispatchedAt = time.Now()
	}
	router.mu.Lock()
	if router.inFlight[command.id] == command {
		router.nextAllowed[command.lane] = dispatchedAt.Add(delay)
	}
	router.mu.Unlock()
}

func (router *Router) finish(command *queuedCommand, err error) {
	router.mu.Lock()
	delete(router.inFlight, command.id)
	router.mu.Unlock()
	completeQueuedCommand(command, err)
	command.cancel()
}

func (router *Router) removeQueued(id uint64, lane Lane) bool {
	router.mu.Lock()
	queue := router.queues[lane]
	for index, command := range queue {
		if command.id == id {
			router.queues[lane] = append(queue[:index], queue[index+1:]...)
			router.mu.Unlock()
			command.cancel()
			router.signal()
			return true
		}
	}
	router.mu.Unlock()
	return false
}

func (router *Router) closeWithError(err error) {
	if err == nil {
		err = ErrClosed
	}
	router.mu.Lock()
	if router.closed {
		router.mu.Unlock()
		return
	}
	router.closed = true
	router.closeErr = err
	commands := router.takeAllLocked()
	router.mu.Unlock()
	for _, command := range commands {
		command.cancel()
		commandErr := err
		if !command.dispatchedAt.IsZero() {
			commandErr = MarkIndeterminate(commandErr)
		}
		completeQueuedCommand(command, commandErr)
	}
	router.signal()
}

func (router *Router) takeAllLocked() []*queuedCommand {
	commands := append([]*queuedCommand(nil), router.queues[LaneCommand]...)
	commands = append(commands, router.queues[LaneAttackLaunch]...)
	for _, command := range router.inFlight {
		commands = append(commands, command)
	}
	router.queues[LaneCommand] = nil
	router.queues[LaneAttackLaunch] = nil
	router.inFlight = map[uint64]*queuedCommand{}
	return commands
}

func (router *Router) closedError() error {
	router.mu.Lock()
	defer router.mu.Unlock()
	if router.closeErr != nil {
		return router.closeErr
	}
	return ErrClosed
}

func (router *Router) signal() {
	select {
	case router.notify <- struct{}{}:
	default:
	}
}

func queuedCommandBefore(left, right *queuedCommand) bool {
	return queuedCommandBeforeAt(left, right, time.Now())
}

func queuedCommandBeforeAt(left, right *queuedCommand, now time.Time) bool {
	leftPriority := AgedPriority(left.metadata.Priority, left.enqueuedAt, now)
	rightPriority := AgedPriority(right.metadata.Priority, right.enqueuedAt, now)
	if leftPriority != rightPriority {
		return leftPriority > rightPriority
	}
	if !left.metadata.Deadline.IsZero() || !right.metadata.Deadline.IsZero() {
		if left.metadata.Deadline.IsZero() {
			return false
		}
		if right.metadata.Deadline.IsZero() {
			return true
		}
		if !left.metadata.Deadline.Equal(right.metadata.Deadline) {
			return left.metadata.Deadline.Before(right.metadata.Deadline)
		}
	}
	return left.id < right.id
}

func completeQueuedCommand(command *queuedCommand, err error) {
	select {
	case command.result <- err:
	default:
	}
}
