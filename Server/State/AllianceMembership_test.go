package State

import (
	"encoding/json"
	"testing"
	"time"
)

func TestAllianceMembershipCurrentRequiresProcessAndSessionObservation(t *testing.T) {
	now := time.Now().UTC()
	for _, tc := range []struct {
		name       string
		id         AllianceID
		at         time.Time
		generation uint64
		want       bool
	}{
		{"unknown", 9, time.Time{}, 7, false},
		{"none", 0, now, 7, false},
		{"negative", -1, now, 7, false},
		{"previous session", 9, now, 6, false},
		{"current", 9, now, 7, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			state := NewGameState()
			state.Session.Generation = 7
			state.Player.AllianceID = tc.id
			state.Player.AllianceMembershipID = tc.id
			state.Player.AllianceMembershipObservedAt = tc.at
			state.Player.AllianceMembershipGeneration = tc.generation
			if got := AllianceMembershipCurrent(&state); got != tc.want {
				t.Fatalf("current = %t, want %t", got, tc.want)
			}
		})
	}
	if AllianceMembershipCurrent(nil) {
		t.Fatal("nil state authorized help")
	}
}

func TestAllianceMembershipAuthorityIsNotPersisted(t *testing.T) {
	state := NewGameState()
	state.Session.Generation = 7
	state.Player.AllianceID = 9
	state.Player.AllianceMembershipID = 9
	state.Player.AllianceMembershipObservedAt = time.Now().UTC()
	state.Player.AllianceMembershipGeneration = 7
	raw, err := json.Marshal(state.Player)
	if err != nil {
		t.Fatal(err)
	}
	var restored PlayerState
	if err := json.Unmarshal(raw, &restored); err != nil {
		t.Fatal(err)
	}
	state.Player = restored
	if restored.AllianceID != 9 || restored.AllianceMembershipID != 0 || !restored.AllianceMembershipObservedAt.IsZero() ||
		restored.AllianceMembershipGeneration != 0 || AllianceMembershipCurrent(&state) {
		t.Fatal("saved alliance ID restored process-local membership authority")
	}
}

func TestObserveAllianceMembershipCommitOrder(t *testing.T) {
	at := time.Now().UTC()
	for _, tc := range []struct {
		name                    string
		firstID, nextID, wantID AllianceID
		nextAt                  time.Time
		wantChanged             bool
	}{
		{"positive then none at same time", 9, 0, 0, at, true},
		{"none then positive at same time", 0, 9, 0, at, false},
		{"none then positive after backward clock step", 0, 9, 0, at.Add(-time.Millisecond), false},
		{"none then later positive", 0, 9, 9, at.Add(time.Millisecond), true},
		{"none immediately replaces positive despite clock step", 9, 0, 0, at.Add(-time.Millisecond), true},
		{"positive follows commit order despite clock step", 9, 12, 12, at.Add(-time.Millisecond), true},
		{"negative normalized to none", 9, -1, 0, at, true},
		{"unchanged record", 9, 9, 9, at, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			state := NewGameState()
			state.Session.Generation = 7
			if !ObserveAllianceMembership(&state, tc.firstID, at) {
				t.Fatal("first observation was not recorded")
			}
			if changed := ObserveAllianceMembership(&state, tc.nextID, tc.nextAt); changed != tc.wantChanged {
				t.Fatalf("changed = %t, want %t", changed, tc.wantChanged)
			}
			wantAt := at
			if tc.wantChanged {
				wantAt = tc.nextAt
			}
			if state.Player.AllianceMembershipID != tc.wantID ||
				!state.Player.AllianceMembershipObservedAt.Equal(wantAt) ||
				state.Player.AllianceMembershipGeneration != 7 {
				t.Fatalf("record = %+v", state.Player)
			}
			if state.Player.AllianceID != 0 {
				t.Fatal("setter changed legacy alliance ID")
			}
		})
	}
	if ObserveAllianceMembership(nil, 9, at) {
		t.Fatal("nil state changed")
	}
}

func TestAllianceMembershipGenerationAndLegacyID(t *testing.T) {
	at := time.Now().UTC()
	state := NewGameState()
	state.Session.Generation = 7
	state.Player.AllianceID = 9
	ObserveAllianceMembership(&state, 0, at)
	state.Session.Generation = 8
	if !ObserveAllianceMembership(&state, 9, at) || !AllianceMembershipCurrent(&state) {
		t.Fatal("new session inherited previous session's none")
	}
	state.Session.Generation = 9
	if AllianceMembershipCurrent(&state) {
		t.Fatal("previous generation authorized help")
	}
	ObserveAllianceMembership(&state, 9, at)
	for _, id := range []AllianceID{0, 12} {
		state.Player.AllianceID = id
		if AllianceMembershipCurrent(&state) {
			t.Fatalf("legacy ID %d authorized record 9", id)
		}
	}
}
