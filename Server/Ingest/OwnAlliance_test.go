package Ingest

import (
	"CitadelDesktop/Server/Automation"
	"CitadelDesktop/Server/Configuration"
	"CitadelDesktop/Server/Protocol"
	"CitadelDesktop/Server/State"
	"encoding/json"
	"testing"
	"time"
)

func TestOwnAllianceSnapshotLeavesSwitchesAndIgnoresMissingAID(t *testing.T) {
	now := time.Now()
	s := State.NewGameState()
	s.Player.ID = 7
	s.Player.AllianceID = 9
	s.Alliance = State.AllianceState{ID: 9, Holdings: []State.AllianceHolding{{CastleID: 20}}, ObservedAt: now.Add(-time.Second)}
	apply := func(raw string) {
		var root map[string]json.RawMessage
		_ = json.Unmarshal([]byte(raw), &root)
		applyOwnAllianceSnapshot(root, now, &s)
	}
	apply(`{"OI":[{"OID":7,"RPT":0}]}`)
	if s.Player.AllianceID != 9 || len(s.Alliance.Holdings) != 1 {
		t.Fatal("missing AID erased membership")
	}
	apply(`{"gca":{"O":{"OID":7,"AID":0}}}`)
	if s.Player.AllianceID != 0 || s.Alliance.ID != 0 || len(s.Alliance.Holdings) != 0 {
		t.Fatal("departure retained targets")
	}
	code := 0
	_, _, err := reduceAllianceInfo(t.Context(), Protocol.Frame{Payload: json.RawMessage(`{"A":{"AID":9,"M":[{"OID":7,"AID":9},{"OID":8,"AP":[[0,20,10,10,1]],"RPT":999999}]}}`), ReceivedAt: now.Add(time.Second), ResponseCode: &code}, &s, nil)
	if err != nil {
		t.Fatal(err)
	}
	if s.Player.AllianceID != 0 || s.Alliance.ID != 0 {
		t.Fatal("late old roster rebound membership")
	}
	if s.Alliances[9].ID != 9 {
		t.Fatal("directory record lost")
	}
	apply(`{"OI":[{"OID":8,"AID":9},{"OID":7,"AID":10}]}`)
	if s.Player.AllianceID != 10 || s.Alliance.ID != 10 || len(s.Alliance.Holdings) != 0 {
		t.Fatal("switch retained old holdings")
	}
}

func TestOwnGAANoAllianceUnlocksOptInAutoStationGate(t *testing.T) {
	now := time.Date(2026, 9, 24, 20, 30, 0, 0, time.UTC)
	state := State.NewGameState()
	state.Player.ID = 7
	state.Player.AllianceID = 9
	state.Alliance = State.AllianceState{ID: 9, Holdings: []State.AllianceHolding{{CastleID: 20}}}
	state.Castles[100] = State.CastleState{ID: 100, KingdomID: 0, SlotType: 1}
	arrival := now.Add(30 * time.Second)
	state.Movements[1] = State.MovementState{
		ID: 1, TypeID: 0, Direction: 0, OwnerPlayerID: 8, TargetPlayerID: 7,
		SourceTypeID: 1, SourceCastleID: 200, TargetTypeID: 1, TargetCastleID: 100, ArrivesAt: &arrival,
	}
	code := 0
	domains, changed, err := reducePlayerProtectionMode(t.Context(), Protocol.Frame{
		Payload: json.RawMessage(`{"OI":[{"OID":7,"AID":-1,"RPT":0}]}`), ReceivedAt: now, ResponseCode: &code,
	}, &state, nil)
	if err != nil || !changed || !containsDomain(domains, "alliance") {
		t.Fatalf("own GAA no-alliance update: domains=%v changed=%t err=%v", domains, changed, err)
	}
	if state.Player.AllianceID != 0 || state.Alliance.ID != 0 || len(state.Alliance.Holdings) != 0 || !state.Player.AllianceObservedAt.Equal(now) {
		t.Fatalf("explicit no-alliance was not committed: player=%d alliance=%+v observed=%v", state.Player.AllianceID, state.Alliance, state.Player.AllianceObservedAt)
	}
	if !state.Player.ProtectionMode.ObservedAt.Equal(now) {
		t.Fatalf("own GAA did not refresh Protection Mode: %+v", state.Player.ProtectionMode)
	}
	decision, err := Automation.NewAutoStationPolicy().Evaluate(t.Context(), Automation.Snapshot{
		State: state, Now: now,
		Configuration: Configuration.Snapshot{Sections: map[string]json.RawMessage{
			"automation.autoStation": json.RawMessage(`{"leadTimeSec":180,"openGateFallback":true}`),
		}},
	})
	if err != nil || decision.Request == nil || decision.Request.Name != "defense.open_gate" {
		t.Fatalf("opt-in Auto Station after no-alliance GAA = %+v, err=%v", decision, err)
	}
}

func TestOwnAllianceRejectsOtherNegativeAID(t *testing.T) {
	now := time.Now().UTC()
	state := State.NewGameState()
	state.Player.ID = 7
	state.Player.AllianceID = 9
	state.Alliance = State.AllianceState{ID: 9, Holdings: []State.AllianceHolding{{CastleID: 20}}}
	var root map[string]json.RawMessage
	if err := json.Unmarshal([]byte(`{"OI":[{"OID":7,"AID":-2,"RPT":0}]}`), &root); err != nil {
		t.Fatal(err)
	}
	if applyOwnAllianceSnapshot(root, now, &state) || state.Player.AllianceID != 9 || !state.Player.AllianceObservedAt.IsZero() {
		t.Fatalf("unsupported negative AID changed membership: %+v", state.Player)
	}
}
