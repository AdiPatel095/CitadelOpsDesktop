package Automation

import (
	"context"
	"testing"
	"time"
)

func TestStateTrackerVersionsEachObservationOnce(t *testing.T) {
	s := NewStateTracker()
	s.Observe(StateCastles, StateCastles, StateAll)
	if got := s.Snapshot(StateCastles).Version; got != 1 {
		t.Fatalf("castle version=%d; want 1", got)
	}
	if got := s.Snapshot(StateAll).Version; got != 1 {
		t.Fatalf("all-state version=%d; want 1", got)
	}
	if got := StateOpcode(" AIN "); got != "state:opcode:ain" {
		t.Fatalf("StateOpcode normalized to %q", got)
	}
	if got := StateEntity(" Alliance ", 42); got != "state:alliance:42" {
		t.Fatalf("StateEntity normalized to %q", got)
	}
}

func TestStateTrackerAwaitAfter(t *testing.T) {
	s := NewStateTracker()
	before := s.Snapshot(StateMovement).Version
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	result := make(chan StateStamp, 1)
	go func() {
		stamp, ok := s.AwaitAfter(ctx, StateMovement, before)
		if ok {
			result <- stamp
			return
		}
		result <- StateStamp{}
	}()
	s.Observe(StateMovement)
	select {
	case stamp := <-result:
		if stamp.Version != before+1 {
			t.Fatalf("observed version=%d; want %d", stamp.Version, before+1)
		}
	case <-time.After(time.Second):
		t.Fatal("state waiter did not wake after observation")
	}
}
