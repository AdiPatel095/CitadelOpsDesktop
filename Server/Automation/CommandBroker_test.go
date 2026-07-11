package Automation

import (
	"context"
	"testing"
	"time"
)

func TestCommandBrokerPrioritizesWithinIndependentLanes(t *testing.T) {
	b := NewCommandBroker()
	if _, ok := b.submit(Command{Payload: []byte("normal-low"), Lane: LaneCommand, Owner: OwnerBackground, Priority: PriorityBackground}); !ok {
		t.Fatal("normal low-priority command was rejected")
	}
	if _, ok := b.submit(Command{Payload: []byte("normal-high"), Lane: LaneCommand, Owner: OwnerManual, Priority: PriorityManual}); !ok {
		t.Fatal("normal high-priority command was rejected")
	}
	if _, ok := b.submit(Command{Payload: []byte("attack"), Lane: LaneAttackLaunch, Owner: OwnerAttack, Priority: PriorityAutoTCI}); !ok {
		t.Fatal("attack command was rejected")
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	attack, ok := b.Next(ctx, LaneAttackLaunch)
	if !ok || string(attack.Payload) != "attack" {
		t.Fatalf("attack lane returned %q, ok=%v", attack.Payload, ok)
	}
	b.Complete(attack.ID)
	normal, ok := b.Next(ctx, LaneCommand)
	if !ok || string(normal.Payload) != "normal-high" {
		t.Fatalf("normal lane returned %q, ok=%v; want normal-high", normal.Payload, ok)
	}
	b.Complete(normal.ID)
}

func TestCommandBrokerCoalescesOnlySameWork(t *testing.T) {
	b := NewCommandBroker()
	firstID, ok := b.submit(Command{Payload: []byte("old"), Lane: LaneCommand, CoalesceKey: "refresh:ain"})
	if !ok {
		t.Fatal("first refresh was rejected")
	}
	secondID, ok := b.submit(Command{Payload: []byte("new"), Lane: LaneCommand, CoalesceKey: "refresh:ain"})
	if !ok || secondID != firstID {
		t.Fatalf("same-work refresh did not coalesce: first=%d second=%d ok=%v", firstID, secondID, ok)
	}
	if got := b.Snapshot()[LaneCommand]; len(got) != 1 || string(got[0].Payload) != "new" {
		t.Fatalf("coalesced queue = %#v", got)
	}

	if _, ok := b.submit(Command{Payload: []byte("work-a"), Lane: LaneCommand, WorkID: "a", CoalesceKey: "refresh:dcl"}); !ok {
		t.Fatal("work-a refresh was rejected")
	}
	if _, ok := b.submit(Command{Payload: []byte("work-b"), Lane: LaneCommand, WorkID: "b", CoalesceKey: "refresh:dcl"}); !ok {
		t.Fatal("work-b refresh was rejected")
	}
	if got := b.Snapshot()[LaneCommand]; len(got) != 3 {
		t.Fatalf("commands from different work identities coalesced; queue length=%d", len(got))
	}
}

func TestCommandBrokerDropsInvalidAndExpiredCommands(t *testing.T) {
	b := NewCommandBroker()
	active := false
	if _, ok := b.submit(Command{Payload: []byte("stale"), Lane: LaneCommand, Guard: func() bool { return active }}); ok {
		t.Fatal("command with an already-false guard was accepted")
	}
	if _, ok := b.submit(Command{Payload: []byte("expired"), Lane: LaneCommand, Deadline: time.Now().Add(-time.Millisecond)}); ok {
		t.Fatal("already-expired command was accepted")
	}

	active = true
	if _, ok := b.submit(Command{
		Payload:   []byte("expires-before-ready"),
		Lane:      LaneCommand,
		Guard:     func() bool { return active },
		NotBefore: time.Now().Add(time.Second),
		Deadline:  time.Now().Add(20 * time.Millisecond),
	}); !ok {
		t.Fatal("future command was rejected before its deadline")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 80*time.Millisecond)
	defer cancel()
	if command, ok := b.Next(ctx, LaneCommand); ok {
		t.Fatalf("expired delayed command was dequeued: %#v", command)
	}
	if got := b.Snapshot()[LaneCommand]; len(got) != 0 {
		t.Fatalf("expired command remained queued: %#v", got)
	}
}

func TestCommandBrokerWaitForWorkIncludesInFlightCommand(t *testing.T) {
	b := NewCommandBroker()
	id, ok := b.submit(Command{Payload: []byte("payload"), Lane: LaneCommand, WorkID: "work-1"})
	if !ok {
		t.Fatal("work command was rejected")
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	command, ok := b.Next(ctx, LaneCommand)
	if !ok || command.ID != id {
		t.Fatalf("dequeued command id=%d ok=%v; want %d", command.ID, ok, id)
	}

	done := make(chan bool, 1)
	go func() { done <- b.WaitForWork(ctx, "work-1") }()
	select {
	case <-done:
		t.Fatal("work completed while its command was still in flight")
	case <-time.After(20 * time.Millisecond):
	}
	b.Complete(command.ID)
	select {
	case completed := <-done:
		if !completed {
			t.Fatal("work wait was cancelled after command completion")
		}
	case <-time.After(time.Second):
		t.Fatal("work did not complete after its command completed")
	}
}

func TestCommandBrokerRetryReturnsInFlightCommandToItsLane(t *testing.T) {
	b := NewCommandBroker()
	if _, ok := b.submit(Command{Payload: []byte("retry"), Lane: LaneCommand, WorkID: "work-retry"}); !ok {
		t.Fatal("retry command was rejected")
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	first, ok := b.Next(ctx, LaneCommand)
	if !ok {
		t.Fatal("retry command was not dequeued")
	}
	if !b.Retry(first, 10*time.Millisecond) {
		t.Fatal("in-flight command was not requeued")
	}
	second, ok := b.Next(ctx, LaneCommand)
	if !ok || second.ID != first.ID || second.Attempts != 1 {
		t.Fatalf("retried command id=%d attempts=%d ok=%v; want id=%d attempts=1", second.ID, second.Attempts, ok, first.ID)
	}
	b.Complete(second.ID)
	if !b.WaitForWork(ctx, "work-retry") {
		t.Fatal("retried work did not drain")
	}
}
