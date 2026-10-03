package Ingest

import (
	"CitadelDesktop/Server/Automation"
	"CitadelDesktop/Server/Configuration"
	"CitadelDesktop/Server/Protocol"
	"CitadelDesktop/Server/State"
	"encoding/json"
	"fmt"
	"reflect"
	"testing"
	"time"
)

func TestStormRoleSimulatedNewEventUnderExternalAuthority(t *testing.T) {
	config, err := Configuration.Open(t.TempDir(), map[string]json.RawMessage{
		"automation.autoBird":    json.RawMessage(`{"ignoreSettings":{"settings":{"storm":[{"id":489,"amount":37}]}}}`),
		"automation.autoStation": json.RawMessage(`{"settings":{"storm":[{"id":489,"amount":37}]}}`),
		"automation.autoTowers":  json.RawMessage(`{"castles":{"storm":{"enabled":true,"unitId":489}}}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	config.SetExternalAuthority(true)
	before := config.Snapshot()
	events, unsubscribe := config.Subscribe(8)
	defer unsubscribe()
	s := State.NewGameState()
	s.Player.ID = 42
	s.Alliance.ID = 9
	now := time.Now().UTC()
	s.Player.ProtectionMode.ObservedAt = now
	s.Player.AllianceObservedAt = now
	code := 0
	for _, id := range []int{20, 0, 21} {
		storm := ""
		if id > 0 {
			storm = fmt.Sprintf(`,{"KID":4,"AI":[{"AI":[12,100,100,%d,42,0,0,0,0,0,"Synthetic Storm"]}]}`, id)
		}
		raw := fmt.Sprintf(`{"PID":42,"C":[{"KID":0,"AI":[{"AI":[1,10,10,10,42,0,0,0,0,0,"Synthetic Main"]}]}%s]}`, storm)
		_, changed, err := reduceCastleList(t.Context(), Protocol.Frame{Direction: Protocol.DirectionInbound, Opcode: "gcl", ResponseCode: &code, Payload: json.RawMessage(raw), ReceivedAt: now}, &s, nil)
		if err != nil || !changed {
			t.Fatalf("event frame: %v %v", changed, err)
		}
		// Isolate Storm evaluation while retaining the actual reducer's state.
		evaluation := s
		evaluation.Castles = map[State.CastleID]State.CastleState{}
		for key, castle := range s.Castles {
			if castle.KingdomID == 4 {
				evaluation.Castles[key] = castle
			}
		}
		d, err := Automation.NewAutoBirdPolicy().Evaluate(t.Context(), Automation.Snapshot{State: evaluation, Configuration: config.Snapshot(), ConfigurationExternallyOwned: true, Now: now})
		if err != nil {
			t.Fatal(err)
		}
		if id == 0 {
			if d.Request != nil {
				t.Fatal("absent Storm produced request")
			}
			continue
		}
		if d.Request == nil {
			t.Fatalf("configured event did not resolve: %+v", d)
		}
		var args struct {
			Source   int    `json:"sourceCastleId"`
			Tracking string `json:"trackingId"`
			Reserves []struct {
				UnitID int64 `json:"unitId"`
				Amount int64 `json:"amount"`
			} `json:"reserves"`
		}
		json.Unmarshal(d.Request.Arguments, &args)
		if args.Source != id || args.Tracking != fmt.Sprintf("autoBird:%d", id) || len(args.Reserves) != 1 || args.Reserves[0].UnitID != 489 || args.Reserves[0].Amount != 37 {
			t.Fatalf("event payload: %s", d.Request.Arguments)
		}
	}
	if !reflect.DeepEqual(before, config.Snapshot()) {
		t.Fatal("event changed canonical config")
	}
	select {
	case event := <-events:
		t.Fatalf("unexpected configuration write: %+v", event)
	default:
	}
}
