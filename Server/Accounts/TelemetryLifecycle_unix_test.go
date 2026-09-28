//go:build !windows

package Accounts

import (
	"CitadelDesktop/Server/Protocol"
	"CitadelDesktop/Server/Telemetry"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestStalledTelemetryKeepsStoppingAdmission(t *testing.T) {
	supervisor := newTestSupervisor(t)
	supervisor.config.MaxAccounts = 1
	app := addTestAccount(t, supervisor, "alpha")
	dir := filepath.Join(app.DataDir, "Logs", "channels", Telemetry.ChannelWebSocketGame)
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	prefix := time.Now().Format("2006-01-02") + "-"
	suffix := 0
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), prefix) {
			n, _ := strconv.Atoi(strings.TrimSuffix(strings.TrimPrefix(entry.Name(), prefix), ".log"))
			suffix = max(suffix, n)
		}
	}
	app.Telemetry.BeginWebSocketGameSession()
	path := filepath.Join(dir, fmt.Sprintf("%s%d.log", prefix, suffix+1))
	if err := syscall.Mkfifo(path, 0600); err != nil {
		t.Fatal(err)
	}
	readDone := make(chan struct{})
	resumed := false
	resume := func() {
		resumed = true
		go func() {
			f, err := os.Open(path)
			if err == nil {
				_, _ = io.Copy(io.Discard, f)
				_ = f.Close()
			}
			close(readDone)
		}()
	}
	defer func() {
		if !resumed {
			resume()
		}
		select {
		case <-readDone:
		case <-time.After(5 * time.Second):
			t.Error("FIFO writer did not clean up")
		}
	}()
	app.Telemetry.RecordRaw(strings.Repeat("x", 64*1024), Protocol.DirectionInbound, time.Now(), nil)
	deadline := time.Now().Add(time.Second)
	for app.Telemetry.PersistenceSnapshot().InFlightRecords == 0 {
		if time.Now().After(deadline) {
			t.Fatal("writer not stalled")
		}
		time.Sleep(time.Millisecond)
	}
	for i := 0; i < 12; i++ {
		ctx, cancel := context.WithTimeout(t.Context(), 20*time.Millisecond)
		err := supervisor.RemoveAccount(ctx, "alpha")
		cancel()
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("stalled removal=%v", err)
		}
		if supervisor.Capacity().Stopping != 1 {
			t.Fatal("stopping slot released before writer cleanup")
		}
		if _, err := supervisor.AddAccount(t.Context(), AccountConfig{ID: "alpha"}); err == nil {
			t.Fatal("same runtime reopened")
		}
		if _, err := supervisor.AddAccount(t.Context(), AccountConfig{ID: "bravo"}); err == nil {
			t.Fatal("capacity bypassed")
		}
	}
	supervisor.mu.RLock()
	watching := supervisor.stopping["alpha"].stopWatcher
	supervisor.mu.RUnlock()
	if !watching {
		t.Fatal("missing coalesced cleanup watcher")
	}
	resume()
	deadline = time.Now().Add(3 * time.Second)
	for supervisor.Capacity().Stopping != 0 {
		if time.Now().After(deadline) {
			t.Fatal("stopping slot did not release after writer resumed")
		}
		time.Sleep(time.Millisecond)
	}
	<-readDone
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	_ = addTestAccount(t, supervisor, "alpha")

}
