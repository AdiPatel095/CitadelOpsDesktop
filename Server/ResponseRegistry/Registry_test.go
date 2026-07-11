package ResponseRegistry

import (
	"testing"
	"time"
)

func TestRegistryDeliversSameOpcodeFrameToExactlyOneWaiter(t *testing.T) {
	r := NewRegistry()
	first := r.RegisterWaiter("eeq", time.Second)
	second := r.RegisterWaiter("eeq", time.Second)
	defer first.Cleanup()
	defer second.Cleanup()

	r.CheckWaiters("eeq", []string{"first-frame"})
	select {
	case got := <-first.ResponseCh:
		if got[0] != "first-frame" {
			t.Fatalf("first waiter received %q", got)
		}
	case <-time.After(time.Second):
		t.Fatal("oldest waiter did not receive frame")
	}
	select {
	case got := <-second.ResponseCh:
		t.Fatalf("second waiter also received first frame: %q", got)
	default:
	}

	r.CheckWaiters("eeq", []string{"second-frame"})
	select {
	case got := <-second.ResponseCh:
		if got[0] != "second-frame" {
			t.Fatalf("second waiter received %q", got)
		}
	case <-time.After(time.Second):
		t.Fatal("second waiter did not receive next frame")
	}
}

func TestRegistryUsesMatchersAndPreservesWaiterOrder(t *testing.T) {
	r := NewRegistry()
	removed := r.RegisterWaiter("ain", time.Second)
	second := r.RegisterWaiterMatching("ain", time.Second, func(parts []string) bool { return parts[0] == "alliance-2" }, nil)
	third := r.RegisterWaiterMatching("ain", time.Second, func(parts []string) bool { return parts[0] == "alliance-3" }, nil)
	removed.Cleanup()
	defer second.Cleanup()
	defer third.Cleanup()

	r.CheckWaiters("ain", []string{"alliance-3"})
	select {
	case got := <-third.ResponseCh:
		if got[0] != "alliance-3" {
			t.Fatalf("third waiter received %q", got)
		}
	case <-time.After(time.Second):
		t.Fatal("matching third waiter did not receive frame")
	}
	select {
	case got := <-second.ResponseCh:
		t.Fatalf("non-matching second waiter received frame: %q", got)
	default:
	}

	r.CheckWaiters("ain", []string{"alliance-2"})
	select {
	case <-second.ResponseCh:
	case <-time.After(time.Second):
		t.Fatal("remaining second waiter did not retain its place")
	}
}

func TestRegistryCleanupPreservesFIFO(t *testing.T) {
	r := NewRegistry()
	removed := r.RegisterWaiter("ege", time.Second)
	second := r.RegisterWaiter("ege", time.Second)
	third := r.RegisterWaiter("ege", time.Second)
	removed.Cleanup()
	defer second.Cleanup()
	defer third.Cleanup()

	r.CheckWaiters("ege", []string{"frame"})
	select {
	case <-second.ResponseCh:
	case <-time.After(time.Second):
		t.Fatal("oldest remaining waiter did not receive frame")
	}
	select {
	case <-third.ResponseCh:
		t.Fatal("cleanup reordered remaining waiters")
	default:
	}
}
