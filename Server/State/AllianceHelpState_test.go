package State

import (
	"testing"
	"time"
)

func TestRecruitmentAllianceHelpStateIsScopedToSessionGeneration(t *testing.T) {
	state := NewGameState()
	state.Session.Generation = 8
	state.AllianceHelpRequests = AllianceHelpRequestState{
		RecruitmentCastleIDs:  []CastleID{77},
		HospitalProductionIDs: []int64{201},
		ObservedAt:            time.Date(2026, 8, 28, 11, 0, 0, 0, time.UTC),
		OwnObservedGeneration: 7,
	}
	if HasOutstandingRecruitmentAllianceHelpRequest(state, 77) {
		t.Fatal("prior-session recruitment request blocked the current session")
	}
	if !ReconcileOwnRecruitmentAllianceHelp(&state, 88, true) {
		t.Fatal("current own recruitment help did not update state")
	}
	if state.AllianceHelpRequests.OwnObservedGeneration != 8 ||
		len(state.AllianceHelpRequests.RecruitmentCastleIDs) != 1 ||
		state.AllianceHelpRequests.RecruitmentCastleIDs[0] != 88 {
		t.Fatalf("current recruitment state = %#v", state.AllianceHelpRequests)
	}
	if len(state.AllianceHelpRequests.HospitalProductionIDs) != 0 ||
		!state.AllianceHelpRequests.ObservedAt.IsZero() {
		t.Fatalf("prior-session hospital state survived generation change: %#v", state.AllianceHelpRequests)
	}
}

func TestRecruitmentAllianceHelpFallsBackOnlyToCurrentSessionQueueEvidence(t *testing.T) {
	changedAt := time.Date(2026, 8, 28, 12, 0, 0, 0, time.UTC)
	state := NewGameState()
	state.Session.Generation = 8
	state.Session.ChangedAt = changedAt
	state.AllianceHelpRequests = AllianceHelpRequestState{
		RecruitmentCastleIDs:  []CastleID{88},
		ObservedAt:            changedAt.Add(-time.Minute),
		OwnObservedGeneration: 7,
	}
	state.Castles[77] = CastleState{
		ID: 77,
		Production: map[int]ProductionQueue{
			0: {
				LineID: 0, ObservedAt: changedAt.Add(time.Second),
				Active: &QueueItem{ProductionID: 201, AllianceHelpRequested: true},
			},
		},
	}
	if !ReconcileOwnRecruitmentAllianceHelp(&state, 88, true) {
		t.Fatal("failed to move recruitment help state into the current generation")
	}
	if !HasOutstandingRecruitmentAllianceHelpRequest(state, 77) {
		t.Fatal("fresh current-session RAH queue evidence did not block a duplicate request")
	}

	castle := state.Castles[77]
	queue := castle.Production[0]
	queue.ObservedAt = changedAt.Add(-time.Second)
	castle.Production[0] = queue
	state.Castles[77] = castle
	if HasOutstandingRecruitmentAllianceHelpRequest(state, 77) {
		t.Fatal("persisted prior-session RAH queue evidence blocked the current session")
	}
}

func TestRecruitmentAllianceHelpPreservesPreSessionQueueFallback(t *testing.T) {
	state := NewGameState()
	observedAt := time.Now().UTC()
	state.Session.Generation = 0
	state.Castles[77] = CastleState{
		ID: 77,
		Production: map[int]ProductionQueue{
			0: {
				LineID: 0, ObservedAt: observedAt,
				Active: &QueueItem{ProductionID: 201, AllianceHelpRequested: true},
			},
		},
	}
	if !HasOutstandingRecruitmentAllianceHelpRequest(state, 77) {
		t.Fatal("generation-zero recruitment queue evidence was not preserved before session start")
	}
}

func TestOwnAllianceHelpListRequiresCurrentSessionObservation(t *testing.T) {
	changedAt := time.Date(2026, 8, 29, 13, 30, 0, 0, time.UTC)
	state := NewGameState()
	state.Session.Generation = 8
	state.Session.ChangedAt = changedAt
	state.AllianceHelpRequests.OwnObservedGeneration = 8

	if OwnAllianceHelpListCurrent(state) {
		t.Fatal("missing full alliance-help list was treated as current")
	}
	state.AllianceHelpRequests.ObservedAt = changedAt.Add(-time.Second)
	if OwnAllianceHelpListCurrent(state) {
		t.Fatal("pre-session alliance-help list was treated as current")
	}
	state.AllianceHelpRequests.ObservedAt = changedAt.Add(time.Second)
	if !OwnAllianceHelpListCurrent(state) {
		t.Fatal("current-generation alliance-help list was not accepted")
	}
	state.AllianceHelpRequests.OwnObservedGeneration = 7
	if OwnAllianceHelpListCurrent(state) {
		t.Fatal("prior-generation alliance-help list was treated as current")
	}
}

func TestRecruitmentAllianceHelpCoverageUsesExactLifecycleAndSafeHorizon(t *testing.T) {
	now := time.Date(2026, 8, 29, 12, 0, 0, 0, time.UTC)
	state := NewGameState()
	state.Session.Generation = 7
	state.Session.ChangedAt = now.Add(-time.Minute)
	state.AllianceHelpRequests.OwnRecruitmentObservedGeneration = 7
	state.AllianceHelpRequests.OwnRecruitmentRequests = []RecruitmentAllianceHelpRequest{
		{ListID: 91, CastleID: 77, Progress: 2, MaximumHelpers: 3, ObservedAt: now},
	}
	if !RecruitmentAllianceHelpCovers(state, 77, now, time.Minute) {
		t.Fatal("retained pending request did not cover its castle")
	}
	state.Session.ChangedAt = now.Add(time.Second)
	if RecruitmentAllianceHelpCovers(state, 77, now.Add(2*time.Second), 0) {
		t.Fatal("pre-session lifecycle evidence covered a reconnected session")
	}
	state.Session.ChangedAt = now.Add(-time.Minute)
	state.AllianceHelpRequests.OwnRecruitmentRequests[0].RemovedAt = now.Add(time.Second)
	if RecruitmentAllianceHelpCovers(state, 77, now.Add(2*time.Second), 0) {
		t.Fatal("removed pending request still covered its castle")
	}

	completedAt := now.Add(time.Minute)
	state.AllianceHelpRequests.OwnRecruitmentRequests[0] = RecruitmentAllianceHelpRequest{
		ListID: 91, CastleID: 77, Progress: 3, MaximumHelpers: 3,
		ObservedAt: completedAt, CompletedAt: completedAt, RemovedAt: completedAt.Add(time.Millisecond),
	}
	if !RecruitmentAllianceHelpCovers(
		state, 77, completedAt.Add(2*time.Minute), 30*time.Second,
	) {
		t.Fatal("completed request lost its bounded post-AHD grace")
	}
	if RecruitmentAllianceHelpCovers(
		state, 77, completedAt.Add(2*time.Minute+55*time.Second), 10*time.Second,
	) {
		t.Fatal("completion grace with an unsafe remaining horizon covered another BUP")
	}
	if RecruitmentAllianceHelpCovers(
		state, 77, completedAt.Add(RecruitmentAllianceHelpCompletionGrace), 0,
	) {
		t.Fatal("expired completion grace covered another BUP")
	}
	state.Session.Generation = 8
	if RecruitmentAllianceHelpCovers(state, 77, completedAt.Add(time.Minute), 0) {
		t.Fatal("prior-session lifecycle covered the new session")
	}
}
