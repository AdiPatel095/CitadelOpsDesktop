package Automation

import (
	"context"
	"sync"
	"sync/atomic"
	"time"
)

const maxQueuedCommandsPerLane = 2048

type CommandBroker struct {
	mu       sync.Mutex
	id       uint64
	nextID   uint64
	queues   map[Lane][]Command
	inFlight map[uint64]Command
	notify   map[Lane]chan struct{}
	changed  chan struct{}
}

var nextCommandBrokerID atomic.Uint64

func NewCommandBroker() *CommandBroker {
	return &CommandBroker{
		id: nextCommandBrokerID.Add(1),
		queues: map[Lane][]Command{
			LaneCommand:      {},
			LaneAttackLaunch: {},
		},
		inFlight: make(map[uint64]Command),
		notify: map[Lane]chan struct{}{
			LaneCommand:      make(chan struct{}, 1),
			LaneAttackLaunch: make(chan struct{}, 1),
		},
		changed: make(chan struct{}),
	}
}

var Commands = NewCommandBroker()

// SubmitCommand is the compatibility adapter for legacy single-frame callers. New app code should
// use GameCommands.DispatchPayload or DispatchCommands to retain the full command receipt.
func SubmitCommand(payload []byte, options CommandOptions) (uint64, bool) {
	if options.Surface == "" {
		if options.Owner == OwnerToolkit {
			options.Surface = CommandSurfaceToolkit
		} else {
			options.Surface = CommandSurfaceInternalApp
		}
	}
	receipt := DispatchCommands(context.Background(), CommandSubmission{
		Command: options.Builder,
		Intent:  options.Intent,
		Frames:  []CommandFrame{{Payload: append([]byte(nil), payload...)}},
		Options: options,
	})
	if !receipt.Accepted || len(receipt.Frames) != 1 {
		return 0, false
	}
	return receipt.Frames[0].CommandID, true
}

func NextCommand(ctx context.Context, lane Lane) (Command, bool) {
	return Commands.Next(ctx, lane)
}

func CompleteCommand(commandID uint64) {
	Commands.Complete(commandID)
}

func RetryCommand(command Command, delay time.Duration) bool {
	if !Commands.Retry(command, delay) {
		return false
	}
	command.Attempts++
	ObserveCommandRetry(command)
	return true
}

func WaitForWork(ctx context.Context, workID string) bool {
	return Commands.WaitForWork(ctx, workID)
}

func (b *CommandBroker) submit(command Command) (uint64, bool) {
	now := time.Now()
	command, ok := normalizeCommandForSubmit(command, now)
	if !ok {
		return 0, false
	}

	b.mu.Lock()
	queue := b.queues[command.Lane]
	queue, pruned := pruneInvalidCommands(queue, now)
	b.queues[command.Lane] = queue
	if index := coalescedCommandIndex(queue, command); index >= 0 {
		coalesceCommand(&queue[index], command)
		b.queues[command.Lane] = queue
		b.broadcastChangedLocked()
		id := queue[index].ID
		b.mu.Unlock()
		b.observeCancelled(pruned)
		b.signal(command.Lane)
		return id, true
	}
	if len(queue) >= maxQueuedCommandsPerLane {
		b.mu.Unlock()
		b.observeCancelled(pruned)
		return 0, false
	}
	b.nextID++
	command.ID = b.nextID
	b.queues[command.Lane] = append(queue, command)
	b.broadcastChangedLocked()
	b.mu.Unlock()
	b.observeCancelled(pruned)
	b.signal(command.Lane)
	return command.ID, true
}

// submitBatch atomically admits a group of already-built wire commands. Either every frame is
// queued/coalesced, or none of the new frames are. Invalid commands are rejected before the broker
// is changed; independently stale commands already in affected lanes are still pruned.
func (b *CommandBroker) submitBatch(commands []Command) ([]Command, bool) {
	if len(commands) == 0 {
		return nil, false
	}
	now := time.Now()
	normalized := make([]Command, len(commands))
	affected := make(map[Lane]struct{}, 2)
	for index, command := range commands {
		var ok bool
		normalized[index], ok = normalizeCommandForSubmit(command, now)
		if !ok {
			return nil, false
		}
		affected[normalized[index].Lane] = struct{}{}
	}

	b.mu.Lock()
	var pruned []cancelledCommand
	prunedAny := false
	for lane := range affected {
		queue := b.queues[lane]
		var removed []cancelledCommand
		queue, removed = pruneInvalidCommands(queue, now)
		if len(removed) > 0 {
			prunedAny = true
			pruned = append(pruned, removed...)
		}
		b.queues[lane] = queue
	}

	simulated := make(map[Lane][]Command, len(affected))
	for lane := range affected {
		simulated[lane] = append([]Command(nil), b.queues[lane]...)
	}
	for _, command := range normalized {
		queue := simulated[command.Lane]
		if coalescedCommandIndex(queue, command) < 0 {
			if len(queue) >= maxQueuedCommandsPerLane {
				if prunedAny {
					b.broadcastChangedLocked()
				}
				b.mu.Unlock()
				b.observeCancelled(pruned)
				for lane := range affected {
					b.signal(lane)
				}
				return nil, false
			}
			queue = append(queue, command)
			simulated[command.Lane] = queue
		}
	}

	accepted := make([]Command, len(normalized))
	for index, command := range normalized {
		queue := b.queues[command.Lane]
		if existing := coalescedCommandIndex(queue, command); existing >= 0 {
			coalesceCommand(&queue[existing], command)
			b.queues[command.Lane] = queue
			accepted[index] = queue[existing]
			continue
		}
		b.nextID++
		command.ID = b.nextID
		b.queues[command.Lane] = append(queue, command)
		accepted[index] = command
	}
	b.broadcastChangedLocked()
	b.mu.Unlock()
	b.observeCancelled(pruned)
	for lane := range affected {
		b.signal(lane)
	}
	return accepted, true
}

func normalizeCommandForSubmit(command Command, now time.Time) (Command, bool) {
	if len(command.Payload) == 0 {
		return Command{}, false
	}
	if command.Lane != LaneCommand && command.Lane != LaneAttackLaunch {
		command.Lane = LaneCommand
	}
	if command.Owner == "" {
		command.Owner = OwnerManual
	}
	if command.Priority == 0 {
		command.Priority = DefaultPriority(command.Owner)
	}
	if command.Guard != nil && !command.Guard() {
		return Command{}, false
	}
	if !command.Deadline.IsZero() && !command.Deadline.After(now) {
		return Command{}, false
	}
	command.Payload = append([]byte(nil), command.Payload...)
	command.RequestFields = append([]string(nil), command.RequestFields...)
	command.QueuedAt = now
	return command, true
}

func coalescedCommandIndex(queue []Command, command Command) int {
	if command.CoalesceKey == "" {
		return -1
	}
	for index := range queue {
		if queue[index].CoalesceKey == command.CoalesceKey && queue[index].WorkID == command.WorkID {
			return index
		}
	}
	return -1
}

func coalesceCommand(existing *Command, next Command) {
	existing.Payload = append(existing.Payload[:0], next.Payload...)
	existing.BrokerID = next.BrokerID
	existing.HarnessID = next.HarnessID
	existing.SubmissionID = next.SubmissionID
	existing.FrameIndex = next.FrameIndex
	existing.Opcode = next.Opcode
	existing.RequestShape = next.RequestShape
	existing.RequestFields = append(existing.RequestFields[:0], next.RequestFields...)
	if next.Builder != "" {
		existing.Builder = next.Builder
	}
	if next.Intent != "" {
		existing.Intent = next.Intent
	}
	if next.Surface != "" {
		existing.Surface = next.Surface
	}
	if next.Effect != "" {
		existing.Effect = next.Effect
	}
	if next.Priority > existing.Priority {
		existing.Priority = next.Priority
		existing.Owner = next.Owner
	}
	if !next.Deadline.IsZero() && (existing.Deadline.IsZero() || next.Deadline.Before(existing.Deadline)) {
		existing.Deadline = next.Deadline
	}
	existing.Guard = combineCommandGuards(existing.Guard, next.Guard)
}

func combineCommandGuards(a, b func() bool) func() bool {
	if a == nil || b == nil {
		return nil
	}
	return func() bool { return a() || b() }
}

func (b *CommandBroker) Next(ctx context.Context, lane Lane) (Command, bool) {
	if ctx == nil {
		ctx = context.Background()
	}
	for {
		now := time.Now()
		b.mu.Lock()
		queue := b.queues[lane]
		queuedBeforePrune := len(queue)
		queue, pruned := pruneInvalidCommands(queue, now)
		if len(queue) != queuedBeforePrune {
			b.queues[lane] = queue
			b.broadcastChangedLocked()
		}
		best := -1
		var nextReady time.Time
		for i := range queue {
			if !queue[i].NotBefore.IsZero() && queue[i].NotBefore.After(now) {
				wakeAt := queue[i].NotBefore
				if !queue[i].Deadline.IsZero() && queue[i].Deadline.Before(wakeAt) {
					wakeAt = queue[i].Deadline
				}
				if nextReady.IsZero() || wakeAt.Before(nextReady) {
					nextReady = wakeAt
				}
				continue
			}
			if best == -1 || commandBefore(queue[i], queue[best]) {
				best = i
			}
		}
		if best >= 0 {
			command := queue[best]
			b.queues[lane] = append(queue[:best], queue[best+1:]...)
			b.inFlight[command.ID] = command
			b.broadcastChangedLocked()
			b.mu.Unlock()
			b.observeCancelled(pruned)
			if command.Guard != nil && !command.Guard() {
				if b == Commands {
					ObserveCommandCancelled(command, "command guard became inactive before dispatch")
				}
				b.Complete(command.ID)
				continue
			}
			return command, true
		}
		b.queues[lane] = queue
		b.mu.Unlock()
		b.observeCancelled(pruned)

		if nextReady.IsZero() {
			select {
			case <-ctx.Done():
				return Command{}, false
			case <-b.notify[lane]:
			}
			continue
		}
		timer := time.NewTimer(time.Until(nextReady))
		select {
		case <-ctx.Done():
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
			return Command{}, false
		case <-b.notify[lane]:
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

func (b *CommandBroker) Complete(commandID uint64) {
	if commandID == 0 {
		return
	}
	b.mu.Lock()
	if _, ok := b.inFlight[commandID]; ok {
		delete(b.inFlight, commandID)
		b.broadcastChangedLocked()
	}
	b.mu.Unlock()
}

// Retry moves a command from in-flight back to its lane without completing its work. The
// transport uses this only when the browser socket disappears during the narrow send window.
func (b *CommandBroker) Retry(command Command, delay time.Duration) bool {
	if command.ID == 0 {
		return false
	}
	now := time.Now()
	b.mu.Lock()
	current, ok := b.inFlight[command.ID]
	if !ok || current.ID != command.ID {
		b.mu.Unlock()
		return false
	}
	delete(b.inFlight, command.ID)
	if (command.Guard != nil && !command.Guard()) ||
		(!command.Deadline.IsZero() && !command.Deadline.After(now)) ||
		len(b.queues[command.Lane]) >= maxQueuedCommandsPerLane {
		b.broadcastChangedLocked()
		b.mu.Unlock()
		return false
	}
	command.Attempts++
	command.QueuedAt = now
	if delay > 0 {
		command.NotBefore = now.Add(delay)
	} else {
		command.NotBefore = time.Time{}
	}
	b.queues[command.Lane] = append(b.queues[command.Lane], command)
	b.broadcastChangedLocked()
	b.mu.Unlock()
	b.signal(command.Lane)
	return true
}

func (b *CommandBroker) WaitForWork(ctx context.Context, workID string) bool {
	if workID == "" {
		return true
	}
	if ctx == nil {
		ctx = context.Background()
	}
	for {
		b.mu.Lock()
		pending := b.hasWorkLocked(workID)
		changed := b.changed
		b.mu.Unlock()
		if !pending {
			return true
		}
		select {
		case <-ctx.Done():
			return false
		case <-changed:
		}
	}
}

func (b *CommandBroker) hasWorkLocked(workID string) bool {
	for _, queue := range b.queues {
		for _, command := range queue {
			if command.WorkID == workID {
				return true
			}
		}
	}
	for _, command := range b.inFlight {
		if command.WorkID == workID {
			return true
		}
	}
	return false
}

func (b *CommandBroker) CancelOwner(owner string) int {
	return b.cancel(func(command Command) bool { return command.Owner == owner }, "command owner was cancelled")
}

func (b *CommandBroker) CancelWork(workID string) int {
	if workID == "" {
		return 0
	}
	return b.cancel(func(command Command) bool { return command.WorkID == workID }, "command work was cancelled")
}

func (b *CommandBroker) cancel(match func(Command) bool, reason string) int {
	b.mu.Lock()
	removed := 0
	var cancelled []cancelledCommand
	for lane, queue := range b.queues {
		kept := queue[:0]
		for _, command := range queue {
			if match(command) {
				removed++
				cancelled = append(cancelled, cancelledCommand{command: command, reason: reason})
				continue
			}
			kept = append(kept, command)
		}
		b.queues[lane] = kept
	}
	if removed > 0 {
		b.broadcastChangedLocked()
	}
	b.mu.Unlock()
	b.observeCancelled(cancelled)
	if removed > 0 {
		b.signal(LaneCommand)
		b.signal(LaneAttackLaunch)
	}
	return removed
}

func (b *CommandBroker) Snapshot() map[Lane][]Command {
	b.mu.Lock()
	defer b.mu.Unlock()
	out := make(map[Lane][]Command, len(b.queues))
	for lane, queue := range b.queues {
		copyQueue := make([]Command, len(queue))
		copy(copyQueue, queue)
		for i := range copyQueue {
			copyQueue[i].Payload = append([]byte(nil), copyQueue[i].Payload...)
			copyQueue[i].RequestFields = append([]string(nil), copyQueue[i].RequestFields...)
			copyQueue[i].Guard = nil
		}
		out[lane] = copyQueue
	}
	return out
}

func (b *CommandBroker) InFlightSnapshot() []Command {
	b.mu.Lock()
	defer b.mu.Unlock()
	out := make([]Command, 0, len(b.inFlight))
	for _, command := range b.inFlight {
		command.Payload = append([]byte(nil), command.Payload...)
		command.RequestFields = append([]string(nil), command.RequestFields...)
		command.Guard = nil
		out = append(out, command)
	}
	return out
}

func (b *CommandBroker) signal(lane Lane) {
	select {
	case b.notify[lane] <- struct{}{}:
	default:
	}
}

func (b *CommandBroker) broadcastChangedLocked() {
	close(b.changed)
	b.changed = make(chan struct{})
}

type cancelledCommand struct {
	command Command
	reason  string
}

func pruneInvalidCommands(queue []Command, now time.Time) ([]Command, []cancelledCommand) {
	kept := queue[:0]
	var cancelled []cancelledCommand
	for _, command := range queue {
		if !command.Deadline.IsZero() && !command.Deadline.After(now) {
			cancelled = append(cancelled, cancelledCommand{command: command, reason: "command deadline expired before dispatch"})
			continue
		}
		if command.Guard != nil && !command.Guard() {
			cancelled = append(cancelled, cancelledCommand{command: command, reason: "command guard became inactive before dispatch"})
			continue
		}
		kept = append(kept, command)
	}
	return kept, cancelled
}

func (b *CommandBroker) observeCancelled(cancelled []cancelledCommand) {
	if b != Commands {
		return
	}
	for _, item := range cancelled {
		ObserveCommandCancelled(item.command, item.reason)
	}
}

func commandBefore(a, b Command) bool {
	if a.Priority != b.Priority {
		return a.Priority > b.Priority
	}
	if !a.Deadline.IsZero() || !b.Deadline.IsZero() {
		if a.Deadline.IsZero() {
			return false
		}
		if b.Deadline.IsZero() {
			return true
		}
		if !a.Deadline.Equal(b.Deadline) {
			return a.Deadline.Before(b.Deadline)
		}
	}
	return a.ID < b.ID
}
