package Telemetry

import (
	"CitadelDesktop/Server/Protocol"
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"strings"
	"sync"
	"testing"
	"time"
	"unsafe"
)

type gatedWriter struct {
	entered chan struct{}
	release chan struct{}
	once    sync.Once
	data    bytes.Buffer
}

func (w *gatedWriter) Write(p []byte) (int, error) {
	w.once.Do(func() { close(w.entered); <-w.release })
	return w.data.Write(p)
}
func blockedActualWriter(t *testing.T) (*Store, *gatedWriter) {
	t.Helper()
	s := NewStore(5000)
	if err := s.SetDataDir(t.TempDir()); err != nil {
		t.Fatal(err)
	}
	s.stopRetention()
	w := &gatedWriter{entered: make(chan struct{}), release: make(chan struct{})}
	s.fileMu.Lock()
	if _, err := s.channelBufferLocked(ChannelWebSocketGame, time.Now()); err != nil {
		t.Fatal(err)
	}
	s.buffers[ChannelWebSocketGame] = bufio.NewWriterSize(w, 1)
	s.fileMu.Unlock()
	return s, w
}

func TestActualStalledWriterRetainedByteBound(t *testing.T) {
	s, w := blockedActualWriter(t)
	released := false
	defer func() {
		if !released {
			close(w.release)
		}
		s.Close()
	}()
	s.RecordRaw(strings.Repeat("x", 64*1024), Protocol.DirectionInbound, time.Now(), nil)
	<-w.entered
	var wg sync.WaitGroup
	for p := 0; p < 4; p++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 256; i++ {
				s.RecordRaw(strings.Repeat("x", 64*1024), Protocol.DirectionInbound, time.Now(), nil)
			}
		}()
	}
	done := make(chan struct{})
	go func() { wg.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("producers blocked on disk")
	}
	// Same physical retention predicate can run against the original source.
	s.persistMu.Lock()
	queued := 0
	for _, e := range s.persistQueue {
		queued += len(e.line) + len(e.channel)
	}
	s.persistMu.Unlock()
	retained := queued + 64*1024 + 128 // Conservative first in-flight line metadata.
	t.Logf("queued_payload_bytes=%d conservative_retained_bytes=%d", queued, retained)
	if retained > 16*1024*1024+128 {
		t.Fatalf("retained payload exceeds 16MiB: %d", retained)
	}
	stats := s.PersistenceSnapshot()
	if stats.RetainedBytes > PersistenceMaxBytes || stats.RetainedRecords > PersistenceMaxRecords || stats.InFlightRecords == 0 || stats.DroppedRecords == 0 {
		t.Fatalf("invalid bounds: %+v", stats)
	}
	close(w.release)
	released = true
	drainCtx, drainCancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer drainCancel()
	if err := s.flushPersistenceContext(drainCtx); err != nil {
		t.Fatalf("resumed flush failed: %v", err)
	}
	stats = s.PersistenceSnapshot()
	if stats.RetainedBytes != 0 || stats.InFlightBytes != 0 {
		t.Fatal("drain retained references")
	}
}

func TestFlushWaitersCancellationAndClose(t *testing.T) {
	s, w := blockedActualWriter(t)
	defer s.Close()
	s.RecordRaw(strings.Repeat("x", 1024), Protocol.DirectionInbound, time.Now(), nil)
	<-w.entered
	ctx, cancel := context.WithCancel(t.Context())
	results := make(chan error, 8)
	for i := 0; i < 8; i++ {
		go func() { results <- s.flushPersistenceContext(ctx) }()
	}
	deadline := time.Now().Add(time.Second)
	for {
		s.persistMu.Lock()
		n := s.persistFlushWaiters
		s.persistMu.Unlock()
		if n == 8 {
			break
		}
		if time.Now().After(deadline) {
			close(w.release)
			t.Fatal("flush waiters not entered")
		}
		time.Sleep(time.Millisecond)
	}
	if err := s.flushPersistenceContext(t.Context()); err == nil {
		t.Error("ninth flush not rejected")
	}
	cancel()
	for i := 0; i < 8; i++ {
		if err := <-results; err == nil {
			t.Error("canceled flush succeeded")
		}
	}
	closeCtx, end := context.WithTimeout(t.Context(), 20*time.Millisecond)
	defer end()
	if err := s.CloseContext(closeCtx); err == nil {
		t.Error("stalled close claimed completion")
	}
	close(w.release)
	if err := s.CloseContext(t.Context()); err != nil {
		t.Fatal(err)
	}
	if stats := s.PersistenceSnapshot(); stats.RetainedBytes != 0 || stats.FlushRejected != 1 {
		t.Fatalf("shutdown stats=%+v", stats)
	}
}

type failedWriter struct{}

func (failedWriter) Write([]byte) (int, error) { return 0, io.ErrClosedPipe }
func TestWriteFailureAndDataDirBarrier(t *testing.T) {
	s := NewStore(100)
	defer s.Close()
	if err := s.SetDataDir(t.TempDir()); err != nil {
		t.Fatal(err)
	}
	s.fileMu.Lock()
	_, err := s.channelBufferLocked(ChannelWebSocketGame, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	s.buffers[ChannelWebSocketGame] = bufio.NewWriterSize(failedWriter{}, 1)
	old := s.channelsDir
	s.fileMu.Unlock()
	s.RecordRaw("failed diagnostic", Protocol.DirectionInbound, time.Now(), nil)
	if s.flushPersistence() {
		t.Fatal("write failure reported successful flush")
	}
	if s.PersistenceSnapshot().WriteFailures == 0 {
		t.Fatal("missing failure count")
	}
	if err := s.SetDataDir(t.TempDir()); err == nil {
		t.Fatal("failed barrier allowed directory switch")
	}
	if s.channelsDir != old {
		t.Fatal("directory changed")
	}
}
func TestBorrowedLineOwnershipAndOversize(t *testing.T) {
	s := NewStore(100)
	defer s.Close()
	s.fileMu.Lock()
	backing := strings.Repeat("z", 2*1024*1024)
	view := backing[:32]
	s.append(ChannelActivity, view)
	s.mu.RLock()
	owned := s.tails[ChannelActivity].activeLines()[0]
	s.mu.RUnlock()
	if unsafe.StringData(owned) == unsafe.StringData(view) {
		s.fileMu.Unlock()
		t.Fatal("borrowed backing storage retained")
	}
	s.append(ChannelActivity, strings.Repeat("x", featureLiveTailMaxBytes+1))
	s.append(ChannelWebSocketGame, strings.Repeat("x", PersistenceMaxBytes+1))
	s.fileMu.Unlock()
	s.flushPersistence()
	stats := s.PersistenceSnapshot()
	if stats.OversizeRecords == 0 || stats.LiveTailOversizeRecords == 0 {
		t.Fatalf("missing oversize counters: %+v", stats)
	}
	s.mu.RLock()
	if s.tails[ChannelActivity].bytes > featureLiveTailMaxBytes {
		t.Error("feature tail unbounded")
	}
	s.mu.RUnlock()
}
func TestAcceptedFIFOAndIndependentAttackCount(t *testing.T) {
	s, w := blockedActualWriter(t)
	defer s.Close()
	s.RecordRaw("first", Protocol.DirectionInbound, time.Now(), nil)
	<-w.entered
	for i := 0; i < 100; i++ {
		s.RecordRaw(fmt.Sprintf("sequence-%03d", i), Protocol.DirectionInbound, time.Now(), nil)
	}
	s.recordAttackLaunch(ChannelAutoTowers, time.Now())
	close(w.release)
	if !s.flushPersistence() {
		t.Fatal("flush failed")
	}
	prior := -1
	for i := 0; i < 100; i++ {
		index := strings.Index(w.data.String(), fmt.Sprintf("sequence-%03d", i))
		if index <= prior {
			t.Fatal("FIFO lost")
		}
		prior = index
	}
	s.attackMu.Lock()
	count := len(s.attackLaunches[ChannelAutoTowers])
	s.attackMu.Unlock()
	if count != 1 {
		t.Fatal("attack accounting lost")
	}
}

func TestRecordCountBoundAndAccountIsolation(t *testing.T) {
	s, w := blockedActualWriter(t)
	defer s.Close()
	s.RecordRaw("first", Protocol.DirectionInbound, time.Now(), nil)
	<-w.entered
	for i := 0; i < PersistenceMaxRecords+100; i++ {
		s.RecordRaw("tiny", Protocol.DirectionInbound, time.Now(), nil)
	}
	stats := s.PersistenceSnapshot()
	if stats.RetainedRecords != PersistenceMaxRecords || stats.DroppedRecords == 0 {
		t.Fatalf("count guard=%+v", stats)
	}
	sibling := NewStore(100)
	defer sibling.Close()
	sibling.RecordRaw("sibling", Protocol.DirectionInbound, time.Now(), nil)
	if !sibling.flushPersistence() || sibling.PersistenceSnapshot().DroppedRecords != 0 {
		t.Fatal("one saturated account affected sibling")
	}
	s.recordAttackLaunch(ChannelAutoTowers, time.Now())
	s.attackMu.Lock()
	count := len(s.attackLaunches[ChannelAutoTowers])
	s.attackMu.Unlock()
	if count != 1 {
		t.Error("saturation affected attack count")
	}
	close(w.release)
	if !s.flushPersistence() {
		t.Fatal("drain failed")
	}
}

func TestTailFallsBackDuringLockContention(t *testing.T) {
	s := NewStore(100)
	defer s.Close()
	if err := s.SetDataDir(t.TempDir()); err != nil {
		t.Fatal(err)
	}
	s.RecordRaw("current memory", Protocol.DirectionInbound, time.Now(), nil)
	if !s.flushPersistence() {
		t.Fatal("initial flush failed")
	}
	s.fileMu.Lock()
	started := time.Now()
	lines := s.tailRecords(ChannelWebSocketGame, 10)
	s.fileMu.Unlock()
	if time.Since(started) > time.Second || len(lines) != 1 || !strings.Contains(lines[0], "current memory") {
		t.Fatal("tail did not fall back promptly")
	}
	s.persistMu.Lock()
	s.persistStats.WriteFailures++
	s.persistMu.Unlock()
	if _, err := s.pruneLogs(time.Now()); err == nil {
		t.Fatal("retention ignored failed flush")
	}
}

func TestProductionFlushDeadline(t *testing.T) {
	s, w := blockedActualWriter(t)
	defer s.Close()
	defer close(w.release)
	s.RecordRaw("stalled", Protocol.DirectionInbound, time.Now(), nil)
	<-w.entered
	started := time.Now()
	if s.flushPersistence() {
		t.Fatal("stalled writer reported successful flush")
	}
	elapsed := time.Since(started)
	if elapsed < 4*time.Second || elapsed > 10*time.Second {
		t.Fatalf("flush deadline elapsed=%v", elapsed)
	}
	stats := s.PersistenceSnapshot()
	if stats.FlushTimeouts != 1 || stats.FlushCancelled != 0 {
		t.Fatalf("deadline counters=%+v", stats)
	}
}

func TestStalledDiskReadersRemainOwnedUntilClose(t *testing.T) {
	s := NewStore(100)
	defer s.Close()
	if err := s.SetDataDir(t.TempDir()); err != nil {
		t.Fatal(err)
	}
	s.stopRetention()
	s.RecordRaw("memory fallback", Protocol.DirectionInbound, time.Now(), nil)
	if !s.flushPersistence() {
		t.Fatal("initial flush failed")
	}
	entered := make(chan struct{}, 2)
	release := make(chan struct{})
	s.readPersistedTail = func([]string, int) ([]string, error) {
		entered <- struct{}{}
		<-release
		return []string{"persisted"}, nil
	}
	finished := make(chan struct{}, 2)
	for i := 0; i < 2; i++ {
		go func() { s.tailRecords(ChannelWebSocketGame, 10); finished <- struct{}{} }()
	}
	<-entered
	<-entered
	if len(s.tailRecords(ChannelWebSocketGame, 10)) != 1 {
		t.Error("saturated reader did not fall back")
	}
	for i := 0; i < 10; i++ {
		ctx, cancel := context.WithTimeout(t.Context(), time.Millisecond)
		err := s.CloseContext(ctx)
		cancel()
		if err == nil {
			t.Error("close claimed completed stalled readers")
		}
	}
	if s.PersistenceSnapshot().TailReaders != 2 {
		t.Error("reader accounting lost")
	}
	// Close forbids new disk readers even after a flush frontier passes.
	s.tailRecords(ChannelWebSocketGame, 10)
	if s.PersistenceSnapshot().TailReaders != 2 {
		t.Error("new reader admitted after close")
	}
	close(release)
	<-finished
	<-finished
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	if err := s.CloseContext(ctx); err != nil {
		t.Fatal(err)
	}
	if s.PersistenceSnapshot().TailReaders != 0 {
		t.Fatal("reader references retained")
	}
}
