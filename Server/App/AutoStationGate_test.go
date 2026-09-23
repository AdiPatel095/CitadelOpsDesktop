package App

import (
	"CitadelDesktop/Server/Configuration"
	"CitadelDesktop/Server/GameData"
	"CitadelDesktop/Server/State"
	"encoding/json"
	"fmt"
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

func TestPartialTrackedGateRequiresFreshUnsentTroops(t *testing.T) {
	now := time.Now()
	data, err := GameData.DecodeStore([]byte(`{"versionInfo":[],"buildings":[],"units":[{"wodID":489},{"wodID":735,"toolCategory":"Premium","slotTypes":"1,2,9"}]}`), GameData.SourceMetadata{ItemVersion: "test"})
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name             string
		amount, reserved int64
		stale            bool
		allow            bool
	}{{"partial", 20, 0, false, true}, {"empty", 0, 0, false, false}, {"reserved", 20, 20, false, false}, {"stale", 20, 0, true, false}} {
		t.Run(tc.name, func(t *testing.T) {
			s := State.NewGameState()
			until := now.Add(time.Hour)
			s.Stationing["autoStation:10"] = State.StationingOperation{SourceCastleID: 10, UpdatedAt: now.Add(-time.Second), SuccessCooldownUntil: &until}
			c := State.CastleState{ID: 10, UnitsObservedAt: now, Units: State.CastleUnits{Stationed: map[State.UnitID]int64{489: tc.amount, 735: 20}}}
			if tc.stale {
				c.UnitsObservedAt = now.Add(-time.Minute)
			}
			s.Castles[10] = c
			err := validateTrackedGateRemainder(s, defenseOpenGateRequest{CastleID: 10, PlannedAt: now.Add(-time.Second)}, map[State.UnitID]int64{489: tc.reserved}, data, now)
			if (err == nil) != tc.allow {
				t.Fatalf("allow=%v err=%v", tc.allow, err)
			}
		})
	}
}

func TestOpenGateCallbackUsesAbsentSettingsDefaults(t *testing.T) {
	for _, section := range []struct {
		name, raw string
		malformed bool
	}{
		{"absent", "", false}, {"empty object", "{}", false}, {"null", "null", false},
		{"explicit opt out", `{"openGateFallback":false}`, false},
		{"malformed present", `{"leadTimeSec":"bad"}`, true},
	} {
		for _, protection := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/protection=%v", section.name, protection), func(t *testing.T) {
				defaults := map[string]json.RawMessage{"automation.enabled": json.RawMessage(`{"auto_station":true}`)}
				if section.raw != "" {
					defaults["automation.autoStation"] = json.RawMessage(section.raw)
				}
				configuration, err := Configuration.Open(t.TempDir(), defaults)
				if err != nil {
					t.Fatal(err)
				}
				now := time.Now()
				s := State.NewGameState()
				s.Player.ID = 7
				s.Player.ProtectionMode.ObservedAt = now
				if protection {
					s.Player.ProtectionMode.RemainingSec = 600
				}
				s.Session.LoggedIn = true
				s.Session.SocketReady = true
				s.Session.ConnectionGeneration = 3
				s.MovementSnapshot = State.MovementSnapshot{ObservedAt: now, ConnectionGeneration: 3}
				s.Castles[10] = State.CastleState{ID: 10, SlotType: 1}
				arrival := now.Add(30 * time.Second)
				s.Movements[1] = State.MovementState{ID: 1, TypeID: 0, Direction: 0, OwnerPlayerID: 8, TargetPlayerID: 7, SourceTypeID: 1, SourceCastleID: 20, TargetTypeID: 1, TargetCastleID: 10, ArrivesAt: &arrival}
				application := &Application{Configuration: configuration, State: State.NewStore(s)}
				args, _ := json.Marshal(defenseOpenGateRequest{CastleID: 10, AutoStation: true, RequireIncomingAttack: true, PlannedAt: now.Add(-time.Second), ConnectionGeneration: 3})
				err = application.guardOpenGate(t.Context(), args)
				if want := protection && !section.malformed; (err == nil) != want {
					t.Fatalf("allow=%v error=%v", want, err)
				}
			})
		}
	}
}
