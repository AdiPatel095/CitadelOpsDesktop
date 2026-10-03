package Session

import (
	"context"
	"sync"
	"time"
)

const (
	directOutboxPauseBytes  = 8 << 20
	directOutboxResumeBytes = 4 << 20
	directIngestStallLimit  = 2 * time.Minute
)

type directOutboxFrame struct {
	seq   uint64
	frame RawFrame
	bytes int
}

type directPauseEpoch struct {
	epoch     uint64
	pausedAt  time.Time
	resumedAt time.Time
}

// directFrameOutbox retains accepted frames across transport runs. The lease
// serializes the send and keyed removal even during an immediate Stop/Start.
type directFrameOutbox struct {
	mu          sync.Mutex
	queue       []directOutboxFrame
	nextSeq     uint64
	bytes       int
	pauseBytes  int
	resumeBytes int
	paused      bool
	pausedAt    time.Time
	pauseEpoch  uint64
	lastForward time.Time
	wake        chan struct{}
	resume      chan struct{}
	lease       chan struct{}
	afterSend   func(seq uint64)   // nil in production; set before starting forwarders
	pauseEpochs []directPauseEpoch // test-only history; nil in production
}

func newDirectFrameOutbox(pauseBytes, resumeBytes int) *directFrameOutbox {
	if pauseBytes <= 0 {
		pauseBytes = directOutboxPauseBytes
	}
	if resumeBytes <= 0 {
		resumeBytes = directOutboxResumeBytes
	}
	return &directFrameOutbox{pauseBytes: pauseBytes, resumeBytes: resumeBytes, wake: make(chan struct{}, 1), resume: make(chan struct{}, 1), lease: make(chan struct{}, 1)}
}

func directRawFrameBytes(frame RawFrame) int {
	bytes := len(frame.Payload)
	if frame.Decoded != nil {
		bytes += len(frame.Decoded.Payload)
	}
	return bytes
}

func (outbox *directFrameOutbox) push(frame RawFrame) {
	outbox.mu.Lock()
	outbox.nextSeq++
	bytes := directRawFrameBytes(frame)
	outbox.queue = append(outbox.queue, directOutboxFrame{seq: outbox.nextSeq, frame: frame, bytes: bytes})
	outbox.bytes += bytes
	if !outbox.paused && outbox.bytes >= outbox.pauseBytes {
		outbox.paused = true
		outbox.lastForward = time.Now()
		outbox.pausedAt = outbox.lastForward
		outbox.pauseEpoch++
		if outbox.pauseEpochs != nil {
			outbox.pauseEpochs = append(outbox.pauseEpochs, directPauseEpoch{epoch: outbox.pauseEpoch, pausedAt: outbox.pausedAt})
		}
	}
	outbox.mu.Unlock()
	select {
	case outbox.wake <- struct{}{}:
	default:
	}
}

func (outbox *directFrameOutbox) isPaused() bool {
	paused, _, _ := outbox.pauseState()
	return paused
}

// pauseState snapshots the pause and its identity under the same lock.
func (outbox *directFrameOutbox) pauseState() (bool, uint64, time.Time) {
	outbox.mu.Lock()
	defer outbox.mu.Unlock()
	return outbox.paused, outbox.pauseEpoch, outbox.pausedAt
}

func (outbox *directFrameOutbox) resumed() <-chan struct{} { return outbox.resume }

func (outbox *directFrameOutbox) stalled(now time.Time, limit time.Duration) bool {
	outbox.mu.Lock()
	defer outbox.mu.Unlock()
	return outbox.paused && now.Sub(outbox.lastForward) > limit
}

// remove changes accounting only when seq still identifies the sent head.
func (outbox *directFrameOutbox) remove(seq uint64) bool {
	outbox.mu.Lock()
	defer outbox.mu.Unlock()
	if len(outbox.queue) == 0 || outbox.queue[0].seq != seq {
		return false
	}
	outbox.bytes -= outbox.queue[0].bytes
	outbox.queue[0] = directOutboxFrame{}
	outbox.queue = outbox.queue[1:]
	if len(outbox.queue) == 0 {
		outbox.queue = nil
	}
	outbox.lastForward = time.Now()
	if outbox.paused && outbox.bytes < outbox.resumeBytes {
		if len(outbox.pauseEpochs) > 0 {
			outbox.pauseEpochs[len(outbox.pauseEpochs)-1].resumedAt = outbox.lastForward
		}
		outbox.paused = false
		select {
		case outbox.resume <- struct{}{}:
		default:
		}
	}
	return true
}

func (outbox *directFrameOutbox) forward(ctx context.Context, frames chan<- RawFrame) {
	select {
	case outbox.lease <- struct{}{}:
	case <-ctx.Done():
		return
	}
	defer func() { <-outbox.lease }()
	for {
		if ctx.Err() != nil {
			return
		}
		outbox.mu.Lock()
		if len(outbox.queue) == 0 {
			outbox.mu.Unlock()
			select {
			case <-outbox.wake:
			case <-ctx.Done():
				return
			}
			continue
		}
		head := outbox.queue[0]
		outbox.mu.Unlock()
		select {
		case frames <- head.frame:
		case <-ctx.Done():
			return
		}
		if outbox.afterSend != nil {
			outbox.afterSend(head.seq)
		}
		outbox.remove(head.seq)
	}
}
