package Automation

import (
	"CitadelDesktop/Server/Configuration"
	"CitadelDesktop/Server/GameData"
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

func TestAutoStationPartialTrackedBatchUsesGateWithoutRepeatingCDS(t *testing.T) {
	now := time.Now()
	data, err := GameData.DecodeStore([]byte(`{"versionInfo":[],"buildings":[],"units":[{"wodID":489},{"wodID":735,"toolCategory":"Premium","slotTypes":"1,2,9"}]}`), GameData.SourceMetadata{ItemVersion: "test"})
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name              string
		amount, reserve   int64
		protection, stale bool
		want              string
	}{
		{"partial batch", 20, 0, false, false, "defense.open_gate"},
		{"fully evacuated", 0, 0, false, false, "protected"},
		{"reserved only", 20, 20, false, false, "protected"},
		{"reserved troops under protection", 20, 20, true, false, "defense.open_gate"},
		{"fully evacuated under protection", 0, 0, true, false, "protected"},
		{"fully evacuated via AutoBird", 0, 0, true, false, "protected"},
		{"stale inventory", 20, 0, false, true, "castle.focus"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := stationThreatFixture(now)
			if tc.protection {
				s.Player.ProtectionMode.RemainingSec = 600
			}
			until := now.Add(time.Hour)
			s.Stationing["autoStation:100"] = State.StationingOperation{ID: "autoStation:100", Purpose: "autoStation", SourceCastleID: 100, UpdatedAt: now.Add(-time.Second), SuccessCooldownUntil: &until, Units: map[State.UnitID]int64{215: 100}}
			if tc.name == "fully evacuated via AutoBird" {
				op := s.Stationing["autoStation:100"]
				delete(s.Stationing, "autoStation:100")
				op.ID = "autoBird:100"
				op.Purpose = "autoBird"
				s.Stationing[op.ID] = op
			}
			c := s.Castles[100]
			c.UnitsObservedAt = now
			c.Units.Stationed = map[State.UnitID]int64{489: tc.amount, 735: 50}
			if tc.stale {
				c.UnitsObservedAt = now.Add(-time.Minute)
			}
			s.Castles[100] = c
			raw, _ := json.Marshal(map[string]any{"openGateFallback": true, "settings": map[string]any{"100": []map[string]any{{"id": 489, "amount": tc.reserve}}}})
			d, err := NewAutoStationPolicy().Evaluate(t.Context(), Snapshot{State: s, GameData: data, Now: now, Configuration: Configuration.Snapshot{Sections: map[string]json.RawMessage{"automation.autoStation": raw}}})
			if err != nil {
				t.Fatal(err)
			}
			got := d.Status
			if d.Request != nil {
				got = d.Request.Name
			}
			if got != tc.want {
				t.Fatalf("want %s: %+v", tc.want, d)
			}
		})
	}
}

func TestAutoStationTrackedEmptyInventoryFreshnessBoundary(t *testing.T) {
	now := time.Now()
	for _, test := range []struct {
		name string
		age  time.Duration
		want string
	}{
		{"inside boundary", 30*time.Second - time.Nanosecond, "protected"},
		{"at boundary", 30 * time.Second, "protected"},
		{"outside boundary", 30*time.Second + time.Nanosecond, "castle.focus"},
		{"old empty AutoBird inventory", 59 * time.Minute, "castle.focus"},
		{"future inventory", -time.Second, "castle.focus"},
	} {
		t.Run(test.name, func(t *testing.T) {
			s := stationThreatFixture(now)
			s.Player.ProtectionMode.RemainingSec = 600
			until := now.Add(time.Hour)
			s.Stationing["autoBird:100"] = State.StationingOperation{ID: "autoBird:100", Purpose: "autoBird", SourceCastleID: 100, UpdatedAt: now.Add(-time.Hour), SuccessCooldownUntil: &until}
			castle := s.Castles[100]
			castle.UnitsObservedAt = now.Add(-test.age)
			castle.Units.Stationed = map[State.UnitID]int64{}
			s.Castles[100] = castle
			decision, err := NewAutoStationPolicy().Evaluate(t.Context(), Snapshot{State: s, Now: now})
			if err != nil {
				t.Fatal(err)
			}
			got := decision.Status
			if decision.Request != nil {
				got = decision.Request.Name
			}
			if got != test.want {
				t.Fatalf("want %s, got %+v", test.want, decision)
			}
		})
	}
}
