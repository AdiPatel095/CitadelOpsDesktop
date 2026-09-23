package App

import (
	"CitadelDesktop/Server/State"
	"testing"
	"time"
)

func TestAutoStationGateFinalAuthority(t *testing.T) {
	now := time.Now()
	planned := now.Add(-time.Second)
	for _, tc := range []struct {
		name     string
		fallback bool
		mutate   func(*State.GameState)
		allow    bool
	}{
		{"opted in", true, func(s *State.GameState) {}, true},
		{"opted out", false, func(s *State.GameState) {}, false},
		{"protection began after plan", false, func(s *State.GameState) { s.Player.ProtectionMode.RemainingSec = 600 }, true},
		{"protection expired", false, func(s *State.GameState) { s.Player.ProtectionMode.RemainingSec = 0 }, false},
		{"unknown protection", true, func(s *State.GameState) { s.Player.ProtectionMode.ObservedAt = time.Time{} }, false},
		{"attack cancelled", true, func(s *State.GameState) { s.Movements = map[State.MovementID]State.MovementState{} }, false},
		{"stale movements", true, func(s *State.GameState) { s.MovementSnapshot.ObservedAt = planned.Add(-time.Second) }, false},
		{"new session", true, func(s *State.GameState) { s.Session.ConnectionGeneration++ }, false},
		{"gate already open", true, func(s *State.GameState) {
			c := s.Castles[10]
			until := now.Add(time.Hour)
			c.Defense.OpenGateUntil = &until
			s.Castles[10] = c
		}, false},
		{"other kingdom", true, func(s *State.GameState) { c := s.Castles[10]; c.KingdomID = 2; s.Castles[10] = c }, false},
		{"outside castle window", true, func(s *State.GameState) {
			m := s.Movements[1]
			later := now.Add(3 * time.Minute)
			m.ArrivesAt = &later
			s.Movements[1] = m
		}, false},
		{"later wave beyond six hours", true, func(s *State.GameState) {
			m := s.Movements[1]
			m.ID = 2
			later := now.Add(7 * time.Hour)
			m.ArrivesAt = &later
			s.Movements[2] = m
		}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := State.NewGameState()
			s.Player.ID = 7
			s.Player.ProtectionMode.ObservedAt = now
			s.MovementSnapshot.ObservedAt = now
			s.Castles[10] = State.CastleState{ID: 10, SlotType: 1}
			arrival := now.Add(30 * time.Second)
			s.Movements[1] = State.MovementState{ID: 1, TypeID: 0, Direction: 0, OwnerPlayerID: 8, TargetPlayerID: 7, SourceTypeID: 1, SourceCastleID: 20, TargetTypeID: 1, TargetCastleID: 10, ArrivesAt: &arrival}
			tc.mutate(&s)
			err := validateAutoStationGate(s, defenseOpenGateRequest{CastleID: 10, AutoStation: true, PlannedAt: planned}, tc.fallback, 60, now)
			if (err == nil) != tc.allow {
				t.Fatalf("allow=%v err=%v", tc.allow, err)
			}
		})
	}
}
