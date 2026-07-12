package Session

import (
	"context"
	"sync"
	"testing"
	"time"

	"CitadelDesktop/Server/Protocol"
)

type pacingTransport struct {
	mu       sync.Mutex
	sends    []time.Time
	frames   chan RawFrame
	statuses chan Status
}

func newPacingTransport() *pacingTransport {
	return &pacingTransport{frames: make(chan RawFrame), statuses: make(chan Status)}
}

func (*pacingTransport) Start(context.Context) error { return nil }
func (*pacingTransport) Stop(context.Context) error  { return nil }

func (transport *pacingTransport) Send(_ context.Context, _ []byte) error {
	transport.mu.Lock()
	transport.sends = append(transport.sends, time.Now())
	transport.mu.Unlock()
	return nil
}

func (transport *pacingTransport) Frames() <-chan RawFrame      { return transport.frames }
func (transport *pacingTransport) StatusChanges() <-chan Status { return transport.statuses }
func (*pacingTransport) Status() Status {
	return Status{State: "connected", LoggedIn: true, SocketReady: true, Namespace: "EmpireEx_21"}
}

func TestControllerPacesConsecutiveAttackLaunches(t *testing.T) {
	transport := newPacingTransport()
	controller := NewController(context.Background(), transport, nil, nil)
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

func TestControllerExtendsManualFocusHold(t *testing.T) {
	controller := NewController(context.Background(), newPacingTransport(), nil, nil)
	controller.SetManualFocusHoldProvider(func() time.Duration { return 50 * time.Millisecond })
	controller.RecordManualActivity(Activity{Kind: "pointerdown", ObservedAt: time.Now()})
	start := time.Now()
	go func() {
		time.Sleep(25 * time.Millisecond)
		controller.RecordManualActivity(Activity{Kind: "pointermove", ObservedAt: time.Now()})
	}()
	if err := controller.WaitForManualFocusIdle(context.Background()); err != nil {
		t.Fatal(err)
	}
	if elapsed := time.Since(start); elapsed < 60*time.Millisecond {
		t.Fatalf("manual focus hold ended too early after extension: %s", elapsed)
	}
}
