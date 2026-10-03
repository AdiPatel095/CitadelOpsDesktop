package Session

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"CitadelDesktop/Server/Protocol"
	"github.com/gorilla/websocket"
)

type backpressureReply struct {
	raw   string
	flood bool
}

type backpressureGame struct {
	server       *httptest.Server
	pins         atomic.Int64
	pausedPins   atomic.Int64
	movements    *backpressureMovementTrace
	transport    atomic.Pointer[DirectWebSocketTransport]
	floodStarted chan struct{}
}

type backpressureMovementReceive struct {
	seq   uint64
	epoch uint64 // zero for a receive outside a pause
}

type backpressureMovementTrace struct {
	mu           sync.Mutex
	sends        []time.Time
	observations map[uint64]time.Time
	receives     []backpressureMovementReceive
}

func (trace *backpressureMovementTrace) sent(seq uint64, at time.Time) {
	trace.mu.Lock()
	defer trace.mu.Unlock()
	// Store by the poller's sequence so receive n matches send n.
	for uint64(len(trace.sends)) < seq {
		trace.sends = append(trace.sends, time.Time{})
	}
	trace.sends[seq-1] = at
}

func (trace *backpressureMovementTrace) observed(epoch uint64, at time.Time) {
	trace.mu.Lock()
	defer trace.mu.Unlock()
	if _, exists := trace.observations[epoch]; !exists {
		trace.observations[epoch] = at
	}
}

func (trace *backpressureMovementTrace) assertPauses(t *testing.T, epochs []directPauseEpoch) {
	t.Helper()
	trace.mu.Lock()
	defer trace.mu.Unlock()
	if len(trace.observations) == 0 {
		t.Fatal("poller never observed the blocked ingest pause")
	}
	byEpoch := make(map[uint64]directPauseEpoch, len(epochs))
	for _, pause := range epochs {
		byEpoch[pause.epoch] = pause
		observedAt := trace.observations[pause.epoch]
		sendsAfterPause := 0
		for i, sentAt := range trace.sends {
			if sentAt.IsZero() {
				t.Fatalf("missing send time for movement %d", i+1)
			}
			if sentAt.Before(pause.pausedAt) || !pause.resumedAt.IsZero() && !sentAt.Before(pause.resumedAt) {
				continue
			}
			sendsAfterPause++
			if !observedAt.IsZero() && !sentAt.Before(observedAt) {
				t.Fatalf("epoch %d: movement %d began at %v, at or after pause observation %v", pause.epoch, i+1, sentAt, observedAt)
			}
		}
		if sendsAfterPause > 1 {
			t.Fatalf("epoch %d: %d sends began after pause at %v, want at most one", pause.epoch, sendsAfterPause, pause.pausedAt)
		}
	}
	for _, receive := range trace.receives {
		if receive.seq > uint64(len(trace.sends)) || trace.sends[receive.seq-1].IsZero() {
			t.Fatalf("movement receive %d has no matching send", receive.seq)
		}
		if receive.epoch == 0 {
			continue
		}
		if _, exists := byEpoch[receive.epoch]; !exists {
			t.Fatalf("movement receive %d references unknown pause epoch %d", receive.seq, receive.epoch)
		}
		// A short pause may resume before any movement tick observes it.
		// Such an epoch has no observation cutoff; its send bound still applies.
		if observedAt := trace.observations[receive.epoch]; !observedAt.IsZero() && !trace.sends[receive.seq-1].Before(observedAt) {
			t.Fatalf("epoch %d: paused receive %d matched a send at %v, at or after observation %v", receive.epoch, receive.seq, trace.sends[receive.seq-1], observedAt)
		}
	}
}

func backpressureWireFrame(seq, size int) string {
	prefix := fmt.Sprintf("%%xt%%flood%%1%%0%%{\"seq\":%d,\"pad\":\"", seq)
	suffix := "\"}%"
	return prefix + strings.Repeat("x", size-len(prefix)-len(suffix)) + suffix
}

func newBackpressureGame(t *testing.T, count int) *backpressureGame {
	t.Helper()
	game := &backpressureGame{floodStarted: make(chan struct{})}
	var floodOnce sync.Once
	upgrader := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
	game.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		replies := make(chan backpressureReply, 16)
		quit := make(chan struct{})
		defer close(quit)
		go func() {
			for {
				select {
				case <-quit:
					return
				case reply := <-replies:
					if reply.raw != "" {
						if err := conn.WriteMessage(websocket.TextMessage, []byte(reply.raw)); err != nil {
							return
						}
					}
					if reply.flood {
						close(game.floodStarted)
						for i := 1; i <= count; i++ {
							_ = conn.SetWriteDeadline(time.Now().Add(15 * time.Second))
							if err := conn.WriteMessage(websocket.TextMessage, []byte(backpressureWireFrame(i, 64<<10))); err != nil {
								return
							}
						}
					}
				}
			}
		}()
		for {
			_, payload, err := conn.ReadMessage()
			if err != nil {
				return
			}
			message := string(payload)
			paused := false
			var epoch uint64
			if transport := game.transport.Load(); transport != nil {
				paused, epoch, _ = transport.frameOutbox().pauseState()
			}
			if strings.Contains(message, "%pin%") {
				game.pins.Add(1)
				if paused {
					game.pausedPins.Add(1)
				}
			}
			if strings.Contains(message, "%gam%") && game.movements != nil {
				trace := game.movements
				if !paused {
					epoch = 0
				}
				trace.mu.Lock()
				trace.receives = append(trace.receives, backpressureMovementReceive{seq: uint64(len(trace.receives)) + 1, epoch: epoch})
				trace.mu.Unlock()
			}
			reply := backpressureReply{}
			switch {
			case strings.Contains(message, "action='verChk'"):
				reply.raw = `<msg t='sys'><body action='apiOK' r='0'></body></msg>`
			case strings.Contains(message, "action='login'"):
				reply.raw = `%xt%rlu%-1%0%{}%`
			case strings.Contains(message, "action='autoJoin'"):
				reply.raw = `<msg t='sys'><body action='joinOK' r='1'><pid id='0'/></body></msg>`
			case strings.Contains(message, "action='roundTrip'"):
				reply.raw = `<msg t='sys'><body action='roundTripRes' r='1'></body></msg>`
			case strings.Contains(message, "%vck%"):
				reply.raw = `%xt%vck%1%0%{}%`
			case strings.Contains(message, "%lli%"):
				reply.raw = `%xt%lli%1%0%{}%`
			case strings.Contains(message, "%sie%") && count > 0:
				floodOnce.Do(func() { reply = backpressureReply{raw: `%xt%gbd%1%0%{"gpi":{"UID":1,"PID":2}}%`, flood: true} })
			}
			if reply.raw != "" || reply.flood {
				select {
				case replies <- reply:
				case <-quit:
					return
				}
			}
		}
	}))
	t.Cleanup(game.server.Close)
	return game
}

func backpressureTransport(t *testing.T, game *backpressureGame, config DirectWebSocketConfig) *DirectWebSocketTransport {
	t.Helper()
	config.DataDir = t.TempDir()
	if err := saveLoginCredential(config.DataDir, persistedLoginCredential{SchemaVersion: loginCredentialSchemaVersion, CapturedAt: time.Now().UTC(), AutoRestore: true, Username: "test-player", Password: "test-password"}); err != nil {
		t.Fatal(err)
	}
	if err := saveGameConnectionProfile(config.DataDir, gameConnectionProfile{SchemaVersion: gameConnectionProfileSchemaVersion, CapturedAt: time.Now().UTC(), ServerURL: "wss://ep-live-us1-game.goodgamestudios.com:443", Namespace: "EmpireEx_21", ClientBuild: "1165009", Platform: "web-html5", LoginContext: map[string]json.RawMessage{"LANG": json.RawMessage(`"en"`), "DID": json.RawMessage(`0`), "AID": json.RawMessage(`""`), "PL": json.RawMessage(`1`)}}); err != nil {
		t.Fatal(err)
	}
	config.serverURLOverride = "ws" + strings.TrimPrefix(game.server.URL, "http")
	config.handshakeTimeout = 3 * time.Second
	config.buildResolver = func(context.Context, string) (string, error) { return "1165009", nil }
	transport := NewDirectWebSocketTransport(config)
	transport.SetRelogDelayProvider(func() time.Duration { return 10 * time.Millisecond })
	game.transport.Store(transport)
	t.Cleanup(func() { _ = transport.Stop(context.Background()) })
	return transport
}

func waitBackpressureCondition(t *testing.T, condition func() bool) {
	t.Helper()
	deadline := time.NewTimer(5 * time.Second)
	defer deadline.Stop()
	tick := time.NewTicker(time.Millisecond)
	defer tick.Stop()
	for !condition() {
		select {
		case <-deadline.C:
			t.Fatal("timed out waiting for backpressure state")
		case <-tick.C:
		}
	}
}

func waitBackpressureStatus(t *testing.T, ctx context.Context, transport *DirectWebSocketTransport, state string) Status {
	t.Helper()
	for {
		select {
		case status := <-transport.StatusChanges():
			if status.State == state {
				return status
			}
		case <-ctx.Done():
			t.Fatalf("missing %s status: %v", state, ctx.Err())
			return Status{}
		}
	}
}

func floodSequence(frame RawFrame) (int, bool) {
	if frame.Direction != Protocol.DirectionInbound || frame.Decoded == nil || frame.Decoded.Opcode != "flood" {
		return 0, false
	}
	var body struct {
		Seq int `json:"seq"`
	}
	if json.Unmarshal(frame.Decoded.Payload, &body) != nil {
		return 0, false
	}
	return body.Seq, true
}

func collectFlood(t *testing.T, ctx context.Context, transport *DirectWebSocketTransport, count int) {
	t.Helper()
	for want := 1; want <= count; {
		select {
		case frame := <-transport.Frames():
			if seq, ok := floodSequence(frame); ok {
				if seq != want {
					t.Fatalf("flood sequence=%d want %d", seq, want)
				}
				want++
			}
		case <-ctx.Done():
			t.Fatalf("missing flood frame %d: %v", want, ctx.Err())
		}
	}
}

func TestDirectTransportKeepsPingingWhileIngestIsBlocked(t *testing.T) {
	game := newBackpressureGame(t, 400)
	trace := &backpressureMovementTrace{observations: make(map[uint64]time.Time)}
	game.movements = trace
	transport := backpressureTransport(t, game, DirectWebSocketConfig{
		pingInterval: 25 * time.Millisecond, movementInterval: 15 * time.Millisecond,
		outboxPauseBytes: 1 << 20, outboxResumeBytes: 512 << 10,
		movementSent: trace.sent, movementPauseObserved: trace.observed,
	})
	outbox := transport.frameOutbox()
	// Enable epoch history before any goroutine starts; production retains none.
	outbox.pauseEpochs = make([]directPauseEpoch, 0)
	ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
	defer cancel()
	if err := transport.Start(ctx); err != nil {
		t.Fatal(err)
	}
	waitBackpressureStatus(t, ctx, transport, "connected")
	waitTestSignal(t, game.floodStarted)
	waitBackpressureCondition(t, outbox.isPaused)
	pauseEnd := time.NewTimer(time.Second)
	defer pauseEnd.Stop()
	<-pauseEnd.C
	pingRaw := fmt.Sprintf("%%xt%%%s%%pin%%1%%%s%%", transport.profile.Namespace, directEmptyArgument)
	pingFrameBytes := directRawFrameBytes(RawFrame{Payload: pingRaw, Direction: Protocol.DirectionOutbound})
	pingsDuringBlock := game.pausedPins.Load()

	outbox.mu.Lock()
	bytes := outbox.bytes
	// Frames() stays unread, so the full forwarding channel prevents removals
	// throughout the pause. Count inbound pushes after the queued prefix that
	// crossed the pause mark, including any arriving before pause was observed.
	queuedBytes, inboundPushesAfterPause := 0, 0
	for _, item := range outbox.queue {
		if queuedBytes >= outbox.pauseBytes && item.frame.Direction == Protocol.DirectionInbound {
			inboundPushesAfterPause++
		}
		queuedBytes += item.bytes
	}
	pauseBytes := outbox.pauseBytes
	outbox.mu.Unlock()
	if inboundPushesAfterPause > 1 {
		t.Fatalf("inbound pushes after pause=%d, want at most one", inboundPushesAfterPause)
	}
	inboundRaw := backpressureWireFrame(400, 64<<10)
	inboundDecoded, err := Protocol.Decode(inboundRaw, Protocol.DirectionInbound, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	inboundFrameBytes := directRawFrameBytes(RawFrame{Payload: inboundRaw, Decoded: &inboundDecoded, Direction: Protocol.DirectionInbound})
	maxBytes := pauseBytes + inboundFrameBytes + int(pingsDuringBlock)*pingFrameBytes
	if bytes > maxBytes {
		t.Fatalf("outbox bytes=%d exceed pause + inbound fixture + %d pings × %d bytes = %d; inbound pushes after pause=%d", bytes, pingsDuringBlock, pingFrameBytes, maxBytes, inboundPushesAfterPause)
	}
	if game.pausedPins.Load() < 5 {
		t.Fatalf("pings stopped during pause: %d", game.pausedPins.Load())
	}
	// Check every epoch recorded through the blocked interval. Frames() is
	// still unread, so the current pause cannot resume while we snapshot it.
	outbox.mu.Lock()
	epochs := append([]directPauseEpoch(nil), outbox.pauseEpochs...)
	outbox.mu.Unlock()
	trace.assertPauses(t, epochs)
	collectFlood(t, ctx, transport, 400)
}

func assertBackpressureStall(t *testing.T, ctx context.Context, transport *DirectWebSocketTransport, status Status, returned <-chan error) {
	t.Helper()
	wantDetail := fmt.Sprintf("Game connection closed; waiting the configured relog delay (%s) before reconnecting", transport.relogDelay().Round(time.Second))
	if status.Detail != wantDetail {
		t.Fatalf("reconnect detail=%q, want generic %q", status.Detail, wantDetail)
	}
	select {
	case err := <-returned:
		if !errors.Is(err, errDirectIngestStalled) {
			t.Fatalf("connection returned %v, want ingest stall", err)
		}
	case <-ctx.Done():
		t.Fatalf("missing connection result: %v", ctx.Err())
	}
}

func TestDirectTransportReconnectsWhenIngestStalls(t *testing.T) {
	game := newBackpressureGame(t, 400)
	returned := make(chan error, 4)
	transport := backpressureTransport(t, game, DirectWebSocketConfig{pingInterval: 25 * time.Millisecond, movementInterval: 15 * time.Millisecond, outboxPauseBytes: 1 << 20, outboxResumeBytes: 512 << 10, ingestStallLimit: 200 * time.Millisecond, serveReturned: returned})
	ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
	defer cancel()
	if err := transport.Start(ctx); err != nil {
		t.Fatal(err)
	}
	waitBackpressureStatus(t, ctx, transport, "connected")
	status := waitBackpressureStatus(t, ctx, transport, "reconnecting")
	assertBackpressureStall(t, ctx, transport, status, returned)
	// Only the first connection floods. Snapshot the highest accepted frame;
	// unread socket data is resynchronized by the next authenticated baseline.
	outbox := transport.frameOutbox()
	outbox.mu.Lock()
	accepted := 0
	for _, item := range outbox.queue {
		if seq, ok := floodSequence(item.frame); ok && seq > accepted {
			accepted = seq
		}
	}
	outbox.mu.Unlock()
	if accepted == 0 {
		t.Fatal("stall had no retained authoritative frames")
	}
	collectFlood(t, ctx, transport, accepted)
	next := waitBackpressureStatus(t, ctx, transport, "connected")
	if next.ConnectionGeneration <= status.ConnectionGeneration {
		t.Fatal("reconnect did not advance connection generation")
	}
}

func TestDirectTransportForwarderStopsWithTheRun(t *testing.T) {
	t.Run("Stop", func(t *testing.T) {
		game := newBackpressureGame(t, 0)
		exited := make(chan uint64, 4)
		transport := backpressureTransport(t, game, DirectWebSocketConfig{pingInterval: time.Hour, movementInterval: time.Hour, forwarderStopped: exited})
		ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
		defer cancel()
		if err := transport.Start(ctx); err != nil {
			t.Fatal(err)
		}
		waitBackpressureStatus(t, ctx, transport, "connected")
		if err := transport.Stop(ctx); err != nil {
			t.Fatal(err)
		}
		select {
		case generation := <-exited:
			if generation != 1 {
				t.Fatalf("forwarder generation=%d", generation)
			}
		case <-ctx.Done():
			t.Fatal("forwarder survived Stop")
		}
	})
	t.Run("ImmediateReconnectWhileSendInFlight", func(t *testing.T) {
		for _, mode := range []string{"StopStart", "Stall"} {
			t.Run(mode, func(t *testing.T) {
				game := newBackpressureGame(t, 0)
				exited := make(chan uint64, 4)
				returned := make(chan error, 4)
				config := DirectWebSocketConfig{pingInterval: time.Hour, movementInterval: time.Hour, forwarderStopped: exited, serveReturned: returned}
				if mode == "Stall" {
					config.outboxPauseBytes = 128
					config.outboxResumeBytes = 64
					config.ingestStallLimit = 200 * time.Millisecond
				}
				transport := backpressureTransport(t, game, config)
				outbox := transport.frameOutbox()
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
				ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
				defer cancel()
				if err := transport.Start(ctx); err != nil {
					t.Fatal(err)
				}
				waitBackpressureStatus(t, ctx, transport, "connected")
				waitTestSignal(t, sent)
				// Initial subscription producers have finished before Stop mutates the
				// bootstrap; the only held operation is the forwarder's post-send removal.
				waitBackpressureCondition(t, func() bool {
					outbox.mu.Lock()
					defer outbox.mu.Unlock()
					for _, item := range outbox.queue {
						if strings.Contains(item.frame.Payload, "%gie%") {
							return true
						}
					}
					return false
				})
				for i := 1; i <= 3; i++ {
					transport.deliverInbound(backpressureWireFrame(i, 256), 1)
				}
				if mode == "StopStart" {
					if err := transport.Stop(ctx); err != nil {
						t.Fatal(err)
					}
					if err := transport.Start(ctx); err != nil {
						t.Fatal(err)
					}
					waitBackpressureStatus(t, ctx, transport, "connected")
				} else {
					status := waitBackpressureStatus(t, ctx, transport, "reconnecting")
					assertBackpressureStall(t, ctx, transport, status, returned)
					waitBackpressureStatus(t, ctx, transport, "connected")
				}
				// There was one successful send before the hook. A restarted run cannot
				// send the same queued head while the old forwarder still owns the lease.
				if len(transport.frames) != 1 {
					t.Fatalf("forwarder overlap sent %d frames", len(transport.frames))
				}
				unblock()
				receiveCtx, cancelReceive := context.WithTimeout(ctx, 5*time.Second)
				defer cancelReceive()
				// Only the three sequence-numbered server fixtures determine completion.
				// Reconnect bootstrap producers may still append transport frames.
				collectFlood(t, receiveCtx, transport, 3)
				if err := transport.Stop(ctx); err != nil {
					t.Fatal(err)
				}
				wantExits := 1
				if mode == "StopStart" {
					wantExits = 2
				}
				generations := map[uint64]bool{}
				for i := 0; i < wantExits; i++ {
					select {
					case generation := <-exited:
						if generations[generation] {
							t.Fatal("forwarder exited twice")
						}
						generations[generation] = true
					case <-ctx.Done():
						t.Fatal("forwarder did not exit")
					}
				}
				if !generations[1] {
					t.Fatal("old forwarder did not exit")
				}
				// Both forwarders have exited, so accepted frames and the retained tail are
				// stable. Successful forwards followed by that tail conserve every seq.
				outbox.mu.Lock()
				nextSeq := outbox.nextSeq
				tail := append([]directOutboxFrame(nil), outbox.queue...)
				outbox.mu.Unlock()
				seenMu.Lock()
				forwarded := append([]uint64(nil), seen...)
				seenMu.Unlock()
				got := append([]uint64(nil), forwarded...)
				drained := drainOutbox(transport)
				if len(drained) != len(tail) {
					t.Fatalf("drained=%d retained=%d", len(drained), len(tail))
				}
				for i, item := range tail {
					if !reflect.DeepEqual(drained[i], item.frame) {
						t.Fatalf("retained seq %d changed while draining", item.seq)
					}
					got = append(got, item.seq)
				}
				want := make([]uint64, nextSeq)
				for i := range want {
					want[i] = uint64(i + 1)
				}
				if !reflect.DeepEqual(got, want) {
					t.Fatalf("forwarded=%v retained=%v accepted=%v", forwarded, got[len(forwarded):], want)
				}
				// No authoritative duplicate may hide behind transport-generated frames
				// buffered after the last expected fixture was consumed.
				remaining := append([]RawFrame(nil), drained...)
				for len(transport.frames) > 0 {
					remaining = append(remaining, <-transport.Frames())
				}
				for _, frame := range remaining {
					if seq, ok := floodSequence(frame); ok {
						t.Fatalf("server sequence %d remained after all fixtures arrived", seq)
					}
				}
			})
		}
	})
}
