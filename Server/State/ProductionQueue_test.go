package State

import (
	"testing"
	"time"
)

func TestProductionQueueNeedsRefreshOnlyForUntrustworthySlotState(t *testing.T) {
	now := time.Date(2026, 8, 29, 12, 0, 0, 0, time.UTC)
	changedAt := now.Add(-48 * time.Hour)
	futureCompletion := now.Add(time.Hour)
	elapsedCompletion := now

	for _, test := range []struct {
		name  string
		queue ProductionQueue
		want  bool
	}{
		{name: "missing observation", queue: ProductionQueue{}, want: true},
		{name: "future observation", queue: ProductionQueue{ObservedAt: now.Add(time.Second)}, want: true},
		{name: "previous session", queue: ProductionQueue{ObservedAt: changedAt.Add(-time.Second)}, want: true},
		{name: "session boundary is current", queue: ProductionQueue{ObservedAt: changedAt}, want: false},
		{name: "fresh open queue", queue: ProductionQueue{ObservedAt: now}, want: false},
		{name: "active stack still running", queue: ProductionQueue{
			ObservedAt: now.Add(-24 * time.Hour), Active: &QueueItem{CompletesAt: &futureCompletion},
		}, want: false},
		{name: "active stack completed", queue: ProductionQueue{
			ObservedAt: now.Add(-time.Minute), Active: &QueueItem{CompletesAt: &elapsedCompletion},
		}, want: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			state := NewGameState()
			state.Session.Generation = 7
			state.Session.ChangedAt = changedAt
			if got := ProductionQueueNeedsRefresh(state, test.queue, now); got != test.want {
				t.Fatalf("ProductionQueueNeedsRefresh() = %t, want %t", got, test.want)
			}
		})
	}
}

func TestProductionQueuePredatesLatestCastleSnapshot(t *testing.T) {
	now := time.Date(2026, 8, 29, 12, 0, 0, 0, time.UTC)
	castle := CastleState{ContextSnapshotObservedAt: now}
	if !ProductionQueuePredatesCastleSnapshot(castle, ProductionQueue{ObservedAt: now.Add(-time.Second)}) {
		t.Fatal("older production queue was accepted for a newer castle snapshot")
	}
	if ProductionQueuePredatesCastleSnapshot(castle, ProductionQueue{ObservedAt: now}) {
		t.Fatal("production queue from the castle snapshot frame was treated as stale")
	}
}
