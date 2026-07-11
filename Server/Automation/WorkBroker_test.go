package Automation

import (
	"context"
	"testing"
	"time"
)

func TestWorkBrokerKeepsLeaseUntilCommandsDrain(t *testing.T) {
	previousCoordinator := Global
	previousCommands := Commands
	previousWork := Work
	Global = NewCoordinator()
	Commands = NewCommandBroker()
	Work = NewWorkBroker()
	defer func() {
		Global = previousCoordinator
		Commands = previousCommands
		Work = previousWork
	}()

	queued := make(chan struct{})
	handle := SubmitWork(context.Background(), WorkItem{
		DedupeKey: "test-work",
		Request: Request{
			Owner:  "test-owner",
			Claims: []Claim{ExclusiveClaim("test:resource")},
		},
		Run: func(_ context.Context, lease *Lease) error {
			if _, ok := SubmitCommand([]byte(`%xt%EmpireEx_21%gam%1%{}%`), CommandOptions{
				Owner:    lease.Owner(),
				Priority: lease.Priority(),
				WorkID:   lease.WorkID(),
				Guard:    lease.Active,
			}); !ok {
				t.Error("lease-bound command was rejected")
			}
			close(queued)
			return nil
		},
	})
	if duplicate := SubmitWork(context.Background(), WorkItem{DedupeKey: "test-work"}); duplicate != handle {
		t.Fatal("duplicate work did not return the existing handle")
	}
	select {
	case <-queued:
	case <-time.After(time.Second):
		t.Fatal("work operation did not queue its command")
	}
	statuses := Work.Snapshot()
	if len(statuses) != 1 {
		t.Fatalf("active work status count=%d; want 1", len(statuses))
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	command, ok := NextCommand(ctx, LaneCommand)
	if !ok {
		t.Fatal("queued work command was not dequeued")
	}
	if command.WorkID != statuses[0].ID {
		t.Fatalf("command work id=%q; want %q", command.WorkID, statuses[0].ID)
	}
	select {
	case <-handle.Done():
		t.Fatal("work completed before its in-flight command drained")
	case <-time.After(20 * time.Millisecond):
	}
	CompleteCommand(command.ID)
	result, ok := handle.Wait(ctx)
	if !ok || result.Err != nil {
		t.Fatalf("work result ok=%v err=%v", ok, result.Err)
	}
	if status := Global.Snapshot(); len(status.Active) != 0 {
		t.Fatalf("work lease remained active after completion: %#v", status.Active)
	}
}
