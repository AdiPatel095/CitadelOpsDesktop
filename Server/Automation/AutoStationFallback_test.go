package Automation

import (
	"CitadelDesktop/Server/Configuration"
	"CitadelDesktop/Server/State"
	"encoding/json"
	"testing"
	"time"
)

func stationThreatFixture(now time.Time) State.GameState {
	s := State.NewGameState()
	s.Player.ID = 7
	s.Player.AllianceObservedAt = now
	s.Player.ProtectionMode.ObservedAt = now
	s.Castles[100] = State.CastleState{ID: 100, KingdomID: 0, SlotType: 1}
	arrival := now.Add(30 * time.Second)
	s.Movements[1] = State.MovementState{ID: 1, TypeID: 0, Direction: 0, OwnerPlayerID: 8, TargetPlayerID: 7, SourceTypeID: 1, SourceCastleID: 200, TargetTypeID: 1, TargetCastleID: 100, ArrivesAt: &arrival}
	return s
}
func TestAutoStationFallbackForUnavailableStationing(t *testing.T) {
	now := time.Now()
	for _, tc := range []struct {
		name                 string
		fallback, protection bool
		mutate               func(*State.GameState)
		wantGate             bool
	}{
		{"no alliance opt in", true, false, func(s *State.GameState) {}, true},
		{"no alliance opt out", false, false, func(s *State.GameState) {}, false},
		{"no eligible same kingdom", true, false, func(s *State.GameState) {
			s.Alliance = State.AllianceState{ID: 9, ObservedAt: now, Members: []State.AllianceMember{{PlayerID: 8, ReturnProtectionSec: 999999}}, Holdings: []State.AllianceHolding{{CastleID: 300, PlayerID: 8, KingdomID: 2, SlotType: 12, X: 1, Y: 1}}}
		}, true},
		{"no troops", true, false, func(s *State.GameState) {
			s.Alliance = State.AllianceState{ID: 9, ObservedAt: now, Members: []State.AllianceMember{{PlayerID: 8, ReturnProtectionSec: 999999}}, Holdings: []State.AllianceHolding{{CastleID: 300, PlayerID: 8, SlotType: 1, X: 1, Y: 1}}}
		}, true},
		{"protection default", false, true, func(s *State.GameState) {}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := stationThreatFixture(now)
			tc.mutate(&s)
			if tc.protection {
				s.Player.ProtectionMode.RemainingSec = 600
			}
			raw, _ := json.Marshal(map[string]any{"openGateFallback": tc.fallback})
			d, err := NewAutoStationPolicy().Evaluate(t.Context(), Snapshot{State: s, Now: now, Configuration: Configuration.Snapshot{Sections: map[string]json.RawMessage{"automation.autoStation": raw}}})
			if err != nil {
				t.Fatal(err)
			}
			gate := d.Request != nil && d.Request.Name == "defense.open_gate"
			if gate != tc.wantGate || d.Status == "protected" {
				t.Fatalf("decision %+v", d)
			}
		})
	}
}
func TestAutoStationProtectionGatesRespectEachCastleWindow(t *testing.T) {
	now := time.Now()
	s := stationThreatFixture(now)
	s.Player.ProtectionMode.RemainingSec = 600
	s.Castles[50] = State.CastleState{ID: 50, SlotType: 1}
	later := now.Add(10 * time.Minute)
	s.Movements[2] = s.Movements[1]
	m := s.Movements[2]
	m.ID = 2
	m.TargetCastleID = 50
	m.ArrivesAt = &later
	s.Movements[2] = m
	d, err := NewAutoStationPolicy().Evaluate(t.Context(), Snapshot{State: s, Now: now})
	if err != nil || d.Request == nil {
		t.Fatalf("%+v %v", d, err)
	}
	var args struct {
		CastleID int64 `json:"castleId"`
	}
	_ = json.Unmarshal(d.Request.Arguments, &args)
	if args.CastleID != 100 {
		t.Fatal("later castle opened outside its own lead window")
	}
}
func TestAutoStationRosterRefreshFailureHasGuardedGateFallback(t *testing.T) {
	now := time.Now()
	s := stationThreatFixture(now)
	s.Alliance.ID = 9
	d, err := NewAutoStationPolicy().Evaluate(t.Context(), Snapshot{State: s, Now: now})
	if err != nil || d.Request == nil || d.Request.Name != "alliance.refresh" || d.FailureFallback == nil {
		t.Fatalf("%+v %v", d, err)
	}
}
func TestStationUnknownFutureProtectionMustRefresh(t *testing.T) {
	now := time.Now()
	for _, when := range []time.Time{{}, now.Add(time.Minute), now.Add(-3 * time.Minute)} {
		s := stationThreatFixture(now)
		s.Player.ProtectionMode.ObservedAt = when
		d, _ := NewAutoStationPolicy().Evaluate(t.Context(), Snapshot{State: s, Now: now})
		if d.Request == nil || d.Request.Name != "map.query" {
			t.Fatalf("missing refresh: %+v", d)
		}
	}
}
