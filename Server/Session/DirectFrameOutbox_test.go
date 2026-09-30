package Session

import (
	"context"
	"fmt"
	"reflect"
	"sync"
	"testing"
	"time"

	"CitadelDesktop/Server/Protocol"
)

// Only for tests that synchronously exercise producers without a running
// transport. Real transport tests consume Frames through the forwarder.
func drainOutbox(transport *DirectWebSocketTransport) []RawFrame {
	outbox := transport.frameOutbox()
	var frames []RawFrame
	for {
		outbox.mu.Lock()
		if len(outbox.queue) == 0 {
			outbox.mu.Unlock()
			return frames
		}
		head := outbox.queue[0]
		outbox.mu.Unlock()
		if outbox.remove(head.seq) {
			frames = append(frames, head.frame)
		}
	}
}

func waitTestSignal(t *testing.T, signal <-chan struct{}) {
	t.Helper()
	select {
	case <-signal:
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for signal")
	}
}

func TestDirectOutboxOrderAndHysteresis(t *testing.T) {
	t.Run("eight producers preserve push order", func(t *testing.T) {
		outbox := newDirectFrameOutbox(100000, 50000)
		var wg sync.WaitGroup
		for g := 0; g < 8; g++ {
			wg.Add(1)
			go func(g int) {
				defer wg.Done()
				for i := 0; i < 32; i++ {
					outbox.push(RawFrame{Payload: fmt.Sprintf("%d:%d", g, i)})
				}
			}(g)
		}
		wg.Wait()
		outbox.mu.Lock()
		want := make([]string, len(outbox.queue))
		for i, item := range outbox.queue {
			want[i] = item.frame.Payload
			if item.seq != uint64(i+1) {
				t.Errorf("sequence=%d at %d", item.seq, i)
			}
		}
		outbox.mu.Unlock()
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		frames := make(chan RawFrame)
		done := make(chan struct{})
		go func() { defer close(done); outbox.forward(ctx, frames) }()
		for i, payload := range want {
			select {
			case got := <-frames:
				if got.Payload != payload {
					t.Fatalf("position %d got %s want %s", i, got.Payload, payload)
				}
			case <-time.After(3 * time.Second):
				t.Fatal("FIFO drain timed out")
			}
		}
		cancel()
		waitTestSignal(t, done)
		if outbox.bytes != 0 || len(outbox.queue) != 0 {
			t.Fatal("drained FIFO retained accounting")
		}
	})
	t.Run("byte accounting and pause resume cycles", func(t *testing.T) {
		outbox := newDirectFrameOutbox(10, 5)
		for cycle := 0; cycle < 2; cycle++ {
			outbox.push(RawFrame{Payload: "aaaa", Decoded: &Protocol.Frame{Payload: []byte("bbbb")}})
			if outbox.bytes != 8 || outbox.isPaused() {
				t.Fatal("incorrect decoded accounting or early pause")
			}
			outbox.push(RawFrame{Payload: "cc"})
			if !outbox.isPaused() || outbox.bytes != 10 {
				t.Fatal("pause mark did not pause")
			}
			pausedAt := outbox.lastForward
			if outbox.stalled(pausedAt.Add(time.Second), time.Second) {
				t.Fatal("stall boundary should not expire")
			}
			if !outbox.stalled(pausedAt.Add(time.Second+1), time.Second) {
				t.Fatal("paused outbox did not stall")
			}
			// A stale sequence changes neither queue, progress nor the pause.
			if outbox.remove(outbox.nextSeq+1) || outbox.bytes != 10 || len(outbox.queue) != 2 || !outbox.lastForward.Equal(pausedAt) || !outbox.isPaused() {
				t.Fatal("stale removal changed the FIFO")
			}
			outbox.remove(outbox.queue[0].seq)
			if outbox.isPaused() {
				t.Fatal("below resume mark stayed paused")
			}
			select {
			case <-outbox.resumed():
			default:
				t.Fatal("missing resume pulse")
			}
			outbox.remove(outbox.queue[0].seq)
			select {
			case <-outbox.resumed():
				t.Fatal("extra resume pulse")
			default:
			}
			if outbox.stalled(time.Now().Add(time.Hour), time.Second) {
				t.Fatal("unpaused outbox stalled")
			}
		}
		exact := newDirectFrameOutbox(10, 5)
		exact.push(RawFrame{Payload: "12345"})
		exact.push(RawFrame{Payload: "67890"})
		exact.remove(exact.queue[0].seq)
		if !exact.isPaused() {
			t.Fatal("exact resume mark resumed early")
		}
		exact.remove(exact.queue[0].seq)
		if exact.isPaused() {
			t.Fatal("empty outbox remained paused")
		}
	})
	t.Run("cancelled send preserves head", func(t *testing.T) {
		outbox := newDirectFrameOutbox(10, 5)
		outbox.push(RawFrame{Payload: "head"})
		ctx, cancel := context.WithCancel(t.Context())
		frames := make(chan RawFrame)
		done := make(chan struct{})
		go func() { defer close(done); outbox.forward(ctx, frames) }()
		cancel()
		waitTestSignal(t, done)
		if outbox.bytes != 4 || len(outbox.queue) != 1 || outbox.queue[0].seq != 1 {
			t.Fatal("cancelled send lost head")
		}
	})
	t.Run("exclusive lease and keyed removal", func(t *testing.T) {
		testOutboxImmediateRestart(t)
	})
	t.Run("cancel while waiting on lease or wake", func(t *testing.T) {
		outbox := newDirectFrameOutbox(10, 5)
		outbox.lease <- struct{}{}
		ctx, cancel := context.WithCancel(t.Context())
		done := make(chan struct{})
		go func() { defer close(done); outbox.forward(ctx, make(chan RawFrame)) }()
		cancel()
		waitTestSignal(t, done)
		<-outbox.lease
		ctx, cancel = context.WithCancel(t.Context())
		done = make(chan struct{})
		go func() { defer close(done); outbox.forward(ctx, make(chan RawFrame)) }()
		cancel()
		waitTestSignal(t, done)
	})
}

// Reproduces the valid interleaving that lost a new-run frame in the original
// plan. The second forwarder must now wait through the old post-send removal.
func testOutboxImmediateRestart(t *testing.T) {
	t.Helper()
	outbox := newDirectFrameOutbox(100, 50)
	sent, release := make(chan struct{}), make(chan struct{})
	var releaseOnce sync.Once
	unblock := func() { releaseOnce.Do(func() { close(release) }) }
	defer unblock()
	var seenMu sync.Mutex
	var seen []uint64
	outbox.afterSend = func(seq uint64) {
		seenMu.Lock()
		seen = append(seen, seq)
		seenMu.Unlock()
		if seq == 1 {
			close(sent)
			<-release
		}
	}
	outbox.push(RawFrame{Payload: "head"})
	frames := make(chan RawFrame, 4)
	oldCtx, stop := context.WithCancel(t.Context())
	defer stop()
	oldDone := make(chan struct{})
	go func() { defer close(oldDone); outbox.forward(oldCtx, frames) }()
	waitTestSignal(t, sent)
	stop()
	outbox.push(RawFrame{Payload: "new-run-authoritative"})
	outbox.push(RawFrame{Payload: "third"})
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	nextDone := make(chan struct{})
	go func() { defer close(nextDone); outbox.forward(ctx, frames) }()
	select {
	case got := <-frames:
		if got.Payload != "head" {
			t.Fatal(got.Payload)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("missing head")
	}
	select {
	case got := <-frames:
		t.Fatalf("new forwarder bypassed lease: %s", got.Payload)
	case <-time.After(20 * time.Millisecond):
	}
	unblock()
	waitTestSignal(t, oldDone)
	for _, want := range []string{"new-run-authoritative", "third"} {
		select {
		case got := <-frames:
			if got.Payload != want {
				t.Fatalf("got %s want %s", got.Payload, want)
			}
		case <-time.After(3 * time.Second):
			t.Fatal("missing frame")
		}
	}
	cancel()
	waitTestSignal(t, nextDone)
	seenMu.Lock()
	defer seenMu.Unlock()
	if !reflect.DeepEqual(seen, []uint64{1, 2, 3}) || outbox.bytes != 0 || len(outbox.queue) != 0 {
		t.Fatalf("sent sequences=%v bytes=%d", seen, outbox.bytes)
	}
}
