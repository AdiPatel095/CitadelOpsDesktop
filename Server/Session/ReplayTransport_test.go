package Session

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestReplayTransportReadySinkIsOfflineAndVersioned(t *testing.T) {
	path := filepath.Join(t.TempDir(), "capture.log")
	if err := os.WriteFile(path, []byte(
		"2026-08-13 12:00:00.000000 [RECV] [gam] %xt%gam%1%0%{}%\n",
	), 0o600); err != nil {
		t.Fatal(err)
	}
	transport := NewReplayTransport(ReplayConfig{
		Path: path, Ready: true, AcceptOutbound: true,
	})
	initialStatus := transport.Status()
	if !initialStatus.LoggedIn || !initialStatus.SocketReady || initialStatus.ConnectionGeneration == 0 {
		t.Fatalf("initial ready replay status = %#v", initialStatus)
	}
	if err := transport.Send(context.Background(), []byte("discarded")); err != nil {
		t.Fatalf("offline sink rejected command: %v", err)
	}
	if accepted := transport.accepted.Load(); accepted != 1 {
		t.Fatalf("accepted commands = %d; want 1", accepted)
	}
	if err := transport.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	select {
	case frame := <-transport.Frames():
		if frame.ConnectionGeneration == 0 {
			t.Fatal("ready replay frame is not connection-versioned")
		}
		if frame.ObservedAt.Before(initialStatus.ChangedAt) {
			t.Fatalf("ready replay frame kept stale capture time %s", frame.ObservedAt)
		}
	case <-time.After(time.Second):
		t.Fatal("ready replay did not produce a frame")
	}
	status := transport.Status()
	if !status.LoggedIn || !status.SocketReady || status.ConnectionGeneration == 0 {
		t.Fatalf("ready replay status = %#v", status)
	}
}

func TestReplayTransportDefaultRemainsReadOnly(t *testing.T) {
	transport := NewReplayTransport(ReplayConfig{})
	if err := transport.Send(context.Background(), []byte("rejected")); err == nil {
		t.Fatal("default replay unexpectedly accepted a command")
	}
	if err := transport.Start(context.Background()); err == nil || errors.Is(err, context.Canceled) {
		t.Fatalf("missing capture start error = %v", err)
	}
}

func TestReplayTransportPublishesCompletedAfterFramesClose(t *testing.T) {
	path := filepath.Join(t.TempDir(), "capture.log")
	if err := os.WriteFile(path, []byte(
		"2026-08-13 12:00:00.000000 [RECV] [gam] %xt%gam%1%0%{}%\n",
	), 0o600); err != nil {
		t.Fatal(err)
	}
	transport := NewReplayTransport(ReplayConfig{Path: path})
	if err := transport.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	deadline := time.After(time.Second)
	for {
		select {
		case status := <-transport.StatusChanges():
			if status.State == "completed" {
				select {
				case _, open := <-transport.Frames():
					if open {
						t.Fatal("completed was published while frames remained open")
					}
				default:
					t.Fatal("completed was published before the frame channel closed")
				}
				return
			}
		case <-transport.Frames():
		case <-deadline:
			t.Fatal("replay did not complete")
		}
	}
}
