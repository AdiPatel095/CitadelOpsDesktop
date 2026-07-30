package Session

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"time"

	"CitadelDesktop/Server/Protocol"
)

type ReplayConfig struct {
	Path            string
	Speed           float64
	IncludeOutbound bool
}

type ReplayTransport struct {
	config   ReplayConfig
	frames   chan RawFrame
	statuses chan Status

	mu         sync.RWMutex
	status     Status
	cancel     context.CancelFunc
	generation uint64
	completed  bool
}

func NewReplayTransport(config ReplayConfig) *ReplayTransport {
	return &ReplayTransport{
		config: config, frames: make(chan RawFrame, 8192), statuses: make(chan Status, 16),
		status: Status{
			State: "stopped", Namespace: "EmpireEx_21", Detail: "Capture replay is configured",
			ChangedAt: time.Now().UTC(),
		},
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
	transport.publishStatus(Status{
		State: "replaying", Namespace: "EmpireEx_21", Detail: transport.config.Path, ChangedAt: time.Now().UTC(),
	})
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

func (transport *ReplayTransport) Send(context.Context, []byte) error {
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
	}
	transport.publishStatus(Status{State: state, Namespace: "EmpireEx_21", Detail: detail, ChangedAt: time.Now().UTC()})
	transport.mu.Lock()
	if transport.generation == generation && !transport.completed {
		transport.completed = true
		close(transport.frames)
	}
	transport.mu.Unlock()
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
