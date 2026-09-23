package Ingest

import (
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
	apply(`{"gca":{"O":{"OID":7}}}`)
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
