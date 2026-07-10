package movement

import "testing"

func TestCommanderStatusLifecycle(t *testing.T) {
	const now int64 = 1_700_000_000
	state := NewPlayerMovement()
	state.SetCommanderRoster([]CommanderRosterEntry{
		{CommanderID: 8, Name: "Eight", VisiblePosition: 2},
		{CommanderID: 3, Name: "Three", VisiblePosition: 1},
	})

	initial := state.StatusSnapshot(now)
	if len(initial.CommanderStatuses) != 2 {
		t.Fatalf("initial commander count = %d, want 2", len(initial.CommanderStatuses))
	}
	if initial.CommanderStatuses[0].CommanderID != 3 || initial.CommanderStatuses[0].Status != CommanderStateSyncing {
		t.Fatalf("initial first row = %+v, want LID 3 syncing", initial.CommanderStatuses[0])
	}
	if !state.IsCommanderUnavailable(8, now) {
		t.Fatal("commander must be unavailable before a complete snapshot")
	}

	state.ReplaceSnapshot([]GAMMovement{{
		MID:          100,
		PT:           2,
		TT:           30,
		CommanderID:  3,
		ReceivedUnix: now,
	}}, now)
	live := state.StatusSnapshot(now + 1)
	if live.CommanderStatuses[0].Status != CommanderStateOutbound {
		t.Fatalf("LID 3 status = %q, want outbound", live.CommanderStatuses[0].Status)
	}
	if live.CommanderStatuses[1].Status != CommanderStateFree || live.CommanderStatuses[1].Busy {
		t.Fatalf("LID 8 row = %+v, want free", live.CommanderStatuses[1])
	}
	if state.IsCommanderUnavailable(8, now+1) {
		t.Fatal("complete fresh snapshot should prove absent commander free")
	}
	if !state.IsCommanderUnavailable(99, now+1) {
		t.Fatal("commander absent from the owned roster must remain unavailable")
	}

	state.ApplyDelta(GAMMovement{
		MID:          101,
		PT:           0,
		TT:           12,
		D:            1,
		CommanderID:  3,
		ReceivedUnix: now + 2,
	})
	returning := state.StatusSnapshot(now + 2)
	if len(returning.ActiveMovements) != 1 || returning.ActiveMovements[0].MID != 101 {
		t.Fatalf("active movements = %+v, want only new return MID 101", returning.ActiveMovements)
	}
	if returning.CommanderStatuses[0].Status != CommanderStateReturning {
		t.Fatalf("LID 3 status = %q, want returning", returning.CommanderStatuses[0].Status)
	}

	state.ReplaceSnapshot(nil, now+3)
	complete := state.StatusSnapshot(now + 3)
	if complete.CommanderStatuses[0].Status != CommanderStateFree {
		t.Fatalf("LID 3 status = %q after empty snapshot, want free", complete.CommanderStatuses[0].Status)
	}

	stale := state.StatusSnapshot(now + 3 + CommanderSnapshotFreshnessSeconds + 1)
	if stale.CommanderStatuses[0].Status != CommanderStateUnknown {
		t.Fatalf("stale LID 3 status = %q, want unknown", stale.CommanderStatuses[0].Status)
	}
	if !state.IsCommanderUnavailable(3, now+3+CommanderSnapshotFreshnessSeconds+1) {
		t.Fatal("stale snapshot must fail closed")
	}
}
