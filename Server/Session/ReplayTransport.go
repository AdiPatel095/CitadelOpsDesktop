package Session

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"CitadelDesktop/Server/Protocol"
)

type ReplayConfig struct {
	Path            string
	Speed           float64
	IncludeOutbound bool
	Ready           bool
	AcceptOutbound  bool
	RecordAccepted  bool
}

type ReplayTransport struct {
	config   ReplayConfig
	frames   chan RawFrame
	statuses chan Status

	mu               sync.RWMutex
	status           Status
	cancel           context.CancelFunc
	generation       uint64
	completed        bool
	accepted         atomic.Uint64
	acceptedMu       sync.Mutex
	acceptedByOpcode map[string]uint64
}

func NewReplayTransport(config ReplayConfig) *ReplayTransport {
	status := Status{
		State: "stopped", Namespace: "EmpireEx_21", Detail: "Capture replay is configured",
		ChangedAt: time.Now().UTC(),
	}
	if config.Ready {
		// A ready replay begins from an already-established captured session. Keep
		// it ready before Controller.Start so the controller does not invalidate
		// the persisted baseline during its intermediate "starting" status.
		status.Mode = ConnectionModeFull
		status.State = "connected"
		status.LoggedIn = true
		status.SocketReady = true
		status.ConnectionGeneration = 1
	}
	return &ReplayTransport{
		config: config, frames: make(chan RawFrame, 8192), statuses: make(chan Status, 16),
		status: status, acceptedByOpcode: map[string]uint64{},
	}
}

func (transport *ReplayTransport) Start(ctx context.Context) error {
	transport.mu.Lock()
	if transport.cancel != nil {
		transport.mu.Unlock()
		return nil
	}
	if transport.completed {
		transport.mu.Unlock()
		return fmt.Errorf("capture replay has already completed")
	}
	if strings.TrimSpace(transport.config.Path) == "" {
		transport.mu.Unlock()
		return fmt.Errorf("replay capture path is required")
	}
	file, err := os.Open(transport.config.Path)
	if err != nil {
		transport.mu.Unlock()
		return fmt.Errorf("open replay capture: %w", err)
	}
	runContext, cancel := context.WithCancel(ctx)
	transport.cancel = cancel
	transport.generation++
	generation := transport.generation
	transport.mu.Unlock()
	transport.publishStatus(transport.replayStatus("replaying", transport.config.Path, generation))
	go transport.replay(runContext, generation, file)
	return nil
}

func (transport *ReplayTransport) Stop(context.Context) error {
	transport.mu.Lock()
	cancel := transport.cancel
	transport.cancel = nil
	transport.generation++
	transport.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	transport.publishStatus(Status{State: "stopped", Namespace: "EmpireEx_21", ChangedAt: time.Now().UTC()})
	return nil
}

func (transport *ReplayTransport) Send(_ context.Context, payload []byte) error {
	if transport.config.AcceptOutbound {
		transport.accepted.Add(1)
		if transport.config.RecordAccepted {
			opcode := "unknown"
			if frame, err := Protocol.Decode(string(payload), Protocol.DirectionOutbound, time.Now().UTC()); err == nil {
				opcode = strings.ToLower(strings.TrimSpace(frame.Opcode))
			}
			transport.acceptedMu.Lock()
			transport.acceptedByOpcode[opcode]++
			transport.acceptedMu.Unlock()
		}
		return nil
	}
	return fmt.Errorf("capture replay is read-only")
}

func (transport *ReplayTransport) Frames() <-chan RawFrame {
	return transport.frames
}

func (transport *ReplayTransport) StatusChanges() <-chan Status {
	return transport.statuses
}

func (transport *ReplayTransport) Status() Status {
	transport.mu.RLock()
	defer transport.mu.RUnlock()
	return transport.status
}

func (transport *ReplayTransport) replay(ctx context.Context, generation uint64, file *os.File) {
	defer file.Close()
	reader := bufio.NewReaderSize(file, 256*1024)
	var previous time.Time
	frames := 0
	for {
		line, err := reader.ReadString('\n')
		if len(line) > 0 {
			frame, observedAt, ok := replayFrame(line, transport.config.IncludeOutbound)
			if ok {
				if transport.config.Ready {
					frame.ConnectionGeneration = generation
				}
				if !previous.IsZero() && transport.config.Speed > 0 {
					delay := observedAt.Sub(previous)
					if delay > 0 {
						delay = time.Duration(float64(delay) / transport.config.Speed)
						if !waitForReplay(ctx, delay) {
							return
						}
					}
				}
				previous = observedAt
				if transport.config.Ready {
					// Captures are historical, but ready replay represents the same
					// workload entering an established session now. Re-anchor receipt
					// time so the captured gbd is a valid baseline and time-based
					// automation does not treat every replayed frame as stale.
					frame.ObservedAt = time.Now().UTC()
				}
				select {
				case <-ctx.Done():
					return
				case transport.frames <- frame:
					frames++
				}
			}
		}
		if err != nil {
			if errors.Is(err, io.EOF) {
				transport.completeReplay(generation, frames, "")
			} else {
				transport.completeReplay(generation, frames, err.Error())
			}
			return
		}
	}
}

func (transport *ReplayTransport) completeReplay(generation uint64, frames int, detail string) {
	transport.mu.Lock()
	if transport.generation != generation {
		transport.mu.Unlock()
		return
	}
	transport.cancel = nil
	transport.mu.Unlock()
	state := "completed"
	if detail != "" {
		state = "error"
	} else {
		detail = fmt.Sprintf("Replayed %d frames", frames)
		if transport.config.AcceptOutbound {
			detail += fmt.Sprintf("; accepted %d commands", transport.accepted.Load())
			if transport.config.RecordAccepted {
				transport.acceptedMu.Lock()
				opcodes := make([]string, 0, len(transport.acceptedByOpcode))
				for opcode := range transport.acceptedByOpcode {
					opcodes = append(opcodes, opcode)
				}
				sort.Strings(opcodes)
				for _, opcode := range opcodes {
					detail += fmt.Sprintf(" %s=%d", opcode, transport.acceptedByOpcode[opcode])
				}
				transport.acceptedMu.Unlock()
			}
		}
	}
	transport.mu.Lock()
	if transport.generation == generation && !transport.completed {
		transport.completed = true
		close(transport.frames)
	}
	transport.mu.Unlock()
	// Closing Frames tells the controller that no more input can arrive. Wait
	// until it drains every buffered frame before publishing completed; callers
	// may safely use completed as the replay measurement boundary.
	transport.publishStatus(transport.replayStatus(state, detail, generation))
}

func (transport *ReplayTransport) replayStatus(state string, detail string, generation uint64) Status {
	status := Status{
		Mode: ConnectionModeFull, State: state, Namespace: "EmpireEx_21",
		Detail: detail, ChangedAt: time.Now().UTC(),
	}
	if transport.config.Ready {
		status.LoggedIn = true
		status.SocketReady = true
		status.ConnectionGeneration = generation
	}
	return status
}

func (transport *ReplayTransport) publishStatus(status Status) {
	transport.mu.Lock()
	transport.status = status
	transport.mu.Unlock()
	select {
	case transport.statuses <- status:
	default:
	}
}

func replayFrame(line string, includeOutbound bool) (RawFrame, time.Time, bool) {
	direction := Protocol.DirectionInbound
	switch {
	case strings.Contains(line, "[RECV]"):
	case includeOutbound && strings.Contains(line, "[SEND]"):
		direction = Protocol.DirectionOutbound
	default:
		return RawFrame{}, time.Time{}, false
	}
	start := strings.Index(line, "%xt%")
	if start < 0 {
		return RawFrame{}, time.Time{}, false
	}
	payload := strings.TrimSpace(line[start:])
	if !strings.HasSuffix(payload, "%") {
		return RawFrame{}, time.Time{}, false
	}
	observedAt := time.Now().UTC()
	if len(line) >= len("2006-01-02 15:04:05.000000") {
		if parsed, err := time.ParseInLocation("2006-01-02 15:04:05.000000", line[:26], time.Local); err == nil {
			observedAt = parsed.UTC()
		}
	}
	return RawFrame{Payload: payload, Direction: direction, ObservedAt: observedAt}, observedAt, true
}

func waitForReplay(ctx context.Context, delay time.Duration) bool {
	if delay <= 0 {
		return true
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}
