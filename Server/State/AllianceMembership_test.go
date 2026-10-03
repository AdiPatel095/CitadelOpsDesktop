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
	if restored.AllianceID != 9 || !restored.AllianceMembershipObservedAt.IsZero() ||
		restored.AllianceMembershipGeneration != 0 || AllianceMembershipCurrent(&state) {
		t.Fatal("saved alliance ID restored process-local membership authority")
	}
}
