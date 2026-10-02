package Intent

import (
	"sync"
	"testing"
)

func TestEventSequenceLabelsHistoryAfterSubscription(t *testing.T) {
	engine := NewEngine(nil, nil, nil, nil, nil)
	if engine.EventSequence() != 0 {
		t.Fatal("new stream must start at zero")
	}
	events, cancel := engine.Subscribe(256)
	defer cancel()
	engine.publish(Receipt{ID: "before"})
	sequence := engine.EventSequence()
	engine.publish(Receipt{ID: "after"})
	receipts, err := engine.RecentOperations(t.Context(), 100)
	if err != nil || sequence != 1 || engine.EventSequence() != 2 || len(receipts) != 2 {
		t.Fatalf("label %d receipts %v error %v", sequence, receipts, err)
	}
	if (<-events).StreamSequence != 1 || (<-events).StreamSequence != 2 {
		t.Fatal("sequence is not the subscriber stream head")
	}
	var group sync.WaitGroup
	group.Add(1)
	go func() {
		defer group.Done()
		for range 100 {
			engine.publish(Receipt{ID: "concurrent"})
		}
	}()
	for range 100 {
		_ = engine.EventSequence()
	}
	group.Wait()
	if engine.EventSequence() != 102 {
		t.Fatal("concurrent sequence read lost publication")
	}
}
