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
			if got := ProductionQueueNeedsRefresh(&state, test.queue, now); got != test.want {
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

func TestProductionQueueFreeSlotsUsesPerSlotEntitlement(t *testing.T) {
	at := time.Date(2026, 9, 21, 20, 15, 27, 0, time.UTC)
	for _, test := range []struct {
		name  string
		slots []QueueSlot
		want  int
	}{
		{"all permanent", []QueueSlot{{Permanent: true}, {Permanent: true, Occupied: true}}, 1},
		{"active VIP", []QueueSlot{{ExpiresAt: at.Add(time.Second)}}, 1},
		{"expired VIP", []QueueSlot{{ExpiresAt: at.Add(-time.Second)}}, 0},
		{"expiry boundary", []QueueSlot{{ExpiresAt: at}}, 0},
		{"active effect", []QueueSlot{{ExpiresAt: at.Add(time.Minute)}}, 1},
		{"locked empty", []QueueSlot{{}}, 0},
		{"occupied locked", []QueueSlot{{Occupied: true}}, 0},
		{"occupied active", []QueueSlot{{Occupied: true, ExpiresAt: at.Add(time.Minute)}}, 0},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := ProductionQueueFreeSlots(ProductionQueue{Slots: test.slots}, at); got != test.want {
				t.Fatalf("free slots = %d, want %d", got, test.want)
			}
		})
	}
}

func TestProductionQueueSlotsSurviveRestartAndLegacyQueuesRequireRefresh(t *testing.T) {
	now := time.Date(2026, 9, 21, 20, 15, 27, 0, time.UTC)
	state := NewGameState()
	state.Castles[1] = CastleState{ID: 1, Production: map[int]ProductionQueue{
		0: {LineID: 0, Capacity: 2, ObservedAt: now, Slots: []QueueSlot{
			{Permanent: true, Occupied: true}, {ExpiresAt: now.Add(time.Minute)},
		}},
		1: {LineID: 1, Capacity: 5, ObservedAt: now},
		2: {LineID: 2, Capacity: 2, ObservedAt: now},
	}}
	directory := t.TempDir()
	if err := SaveSnapshot(directory, state); err != nil {
		t.Fatal(err)
	}
	restored, err := LoadSnapshot(directory)
	if err != nil {
		t.Fatal(err)
	}
	queues := restored.Castles[1].Production
	if len(queues[0].Slots) != 2 || !queues[0].Slots[0].Occupied ||
		ProductionQueueFreeSlots(queues[0], now) != 1 || ProductionQueueFreeSlots(queues[0], now.Add(time.Minute)) != 0 {
		t.Fatal("slot occupancy or expiry was lost across restart")
	}
	if ProductionQueueNeedsRefresh(&restored, queues[0], now) || !ProductionQueueNeedsRefresh(&restored, queues[1], now) {
		t.Fatal("current and legacy slot observations were not distinguished after restart")
	}
	if ProductionQueueNeedsRefresh(&restored, queues[2], now) || ProductionQueueFreeSlots(queues[2], now) != 2 {
		t.Fatal("compact hospital queue behavior changed")
	}
	cloned := cloneCastleState(state.Castles[1])
	cloned.Production[0].Slots[0].Occupied = false
	if !state.Castles[1].Production[0].Slots[0].Occupied {
		t.Fatal("cloned production slots alias the source state")
	}
}
