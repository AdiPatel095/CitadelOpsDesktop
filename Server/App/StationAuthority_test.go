package App

import (
	"CitadelDesktop/Server/Ingest"
	"CitadelDesktop/Server/Protocol"
	"CitadelDesktop/Server/State"
	"encoding/json"
	"testing"
	"time"
)

func seedStationAuthority(s *State.GameState, now time.Time) {
	s.Player.ID = 99
	s.Player.AllianceID = 9
	s.Player.AllianceObservedAt = now
	s.Player.ProtectionMode.ObservedAt = now
	s.Alliance.ID = 9
	s.Alliance.ObservedAt = now
	s.Alliance.Members = []State.AllianceMember{{PlayerID: 99}, {PlayerID: 1, ReturnProtectionSec: 4 * 86400}}
	for i := range s.Alliance.Holdings {
		s.Alliance.Holdings[i].PlayerID = 1
	}
}
func stationFreshTestArguments(raw json.RawMessage) json.RawMessage {
	var args map[string]any
	_ = json.Unmarshal(raw, &args)
	args["dispatchStartedAt"] = time.Now().Add(-time.Second)
	next, _ := json.Marshal(args)
	return next
}
func TestStationAuthorityRejectsMembershipAndTargetRaces(t *testing.T) {
	now := time.Now()
	after := now.Add(-time.Second)
	for _, tc := range []struct {
		name   string
		mutate func(*State.GameState)
	}{
		{"leave", func(s *State.GameState) { s.Player.AllianceID = 0 }},
		{"switch", func(s *State.GameState) { s.Player.AllianceID = 10 }},
		{"stale roster", func(s *State.GameState) { s.Alliance.ObservedAt = after.Add(-time.Second) }},
		{"missing self", func(s *State.GameState) { s.Alliance.Members = s.Alliance.Members[1:] }},
		{"missing target", func(s *State.GameState) { s.Alliance.Members = s.Alliance.Members[:1] }},
		{"elapsed RPT", func(s *State.GameState) { s.Alliance.Members[1].ReturnProtectionSec = 3 * 86400 }},
		{"protection started", func(s *State.GameState) { s.Player.ProtectionMode.RemainingSec = 100 }},
		{"unknown protection", func(s *State.GameState) { s.Player.ProtectionMode.ObservedAt = time.Time{} }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := State.NewGameState()
			s.Castles[10] = State.CastleState{ID: 10}
			s.Alliance.Holdings = []State.AllianceHolding{{CastleID: 20, PlayerID: 1, SlotType: 1, X: 1, Y: 1}}
			seedStationAuthority(&s, now.Add(-time.Millisecond))
			if err := validateStationAuthority(s, 10, 20, after, 3, now); err != nil {
				t.Fatal(err)
			}
			tc.mutate(&s)
			if err := validateStationAuthority(s, 10, 20, after, 3, now); err == nil {
				t.Fatal("stale station authority accepted")
			}
		})
	}
}

func TestOwnJAAThenCurrentAINGrantsDispatchAuthority(t *testing.T) {
	now := time.Now()
	s := State.NewGameState()
	s.Player.ID = 99
	s.Castles[10] = State.CastleState{ID: 10}
	store := State.NewStore(s)
	registry := Ingest.NewRegistry()
	if err := Ingest.RegisterCoreReducers(registry); err != nil {
		t.Fatal(err)
	}
	pipeline := Ingest.NewPipeline(store, nil, registry)
	code := 0
	apply := func(opcode, payload string, at time.Time) {
		_, err := pipeline.HandleFrame(t.Context(), Protocol.Frame{Opcode: opcode, Direction: Protocol.DirectionInbound, ResponseCode: &code, ReceivedAt: at, Payload: json.RawMessage(payload)})
		if err != nil {
			t.Fatal(err)
		}
	}
	apply("jaa", `{"gca":{"A":[1,0,0,10],"O":{"OID":99,"AID":9,"RPT":0}}}`, now.Add(-time.Millisecond))
	apply("ain", `{"A":{"AID":9,"M":[{"OID":99,"AID":9,"RPT":0},{"OID":1,"AID":9,"RPT":345600,"AP":[[0,20,20,20,1]]}]}}`, now)
	if err := validateStationAuthority(store.Snapshot(), 10, 20, now.Add(-time.Second), 3, now); err != nil {
		t.Fatal(err)
	}
	apply("jaa", `{"gca":{"A":[1,0,0,10],"O":{"OID":99,"AID":0,"RPT":0}}}`, now.Add(time.Millisecond))
	if err := validateStationAuthority(store.Snapshot(), 10, 20, now.Add(-time.Second), 3, now.Add(time.Millisecond)); err == nil {
		t.Fatal("departure after refresh allowed CDS")
	}
}

func TestStationSessionGuardRejectsDisabledTransportAndSafetyLock(t *testing.T) {
	now := time.Now()
	s := State.NewGameState()
	s.Session.LoggedIn = true
	s.Session.SocketReady = true
	s.Session.ConnectionGeneration = 3
	if err := validateStationSession(s, "autoStation", 3, now); err != nil {
		t.Fatal(err)
	}
	s.Session.SocketReady = false
	if err := validateStationSession(s, "autoStation", 3, now); err == nil {
		t.Fatal("disconnected session accepted")
	}
	s.Session.SocketReady = true
	if err := validateStationSession(s, "autoStation", 2, now); err == nil {
		t.Fatal("changed session accepted")
	}
	s.Automations["autoStation"] = State.AutomationState{SafetyLock: State.AutomationSafetyLock{OperationID: "rejection", Opcode: "cds", Code: 5, ObservedAt: now}}
	if err := validateStationSession(s, "autoStation", 3, now); err == nil {
		t.Fatal("safety lock bypassed")
	}
}
