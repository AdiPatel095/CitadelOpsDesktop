package Automation

import (
	"CitadelDesktop/Server/Configuration"
	"CitadelDesktop/Server/State"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestStormRoleBindings(t *testing.T) {
	s := State.NewGameState()
	s.Castles[10] = State.CastleState{ID: 10, KingdomID: 0}
	s.Castles[20] = State.CastleState{ID: 20, KingdomID: 4}
	entries := map[string]string{"10": "main", "20": "legacy", "99": "stale", "storm": "role"}
	got := BoundCastleEntries(entries, &s)
	if len(got) != 2 || got[0].Entry != "main" || got[1].Entry != "role" || got[1].Key != "storm" {
		t.Fatalf("bindings: %#v", got)
	}
	delete(entries, "storm")
	if got, ok := CastleSettingsEntry(entries, s.Castles[20]); !ok || got != "legacy" {
		t.Fatalf("fallback: %q %v", got, ok)
	}
	delete(entries, "20")
	if _, ok := CastleSettingsEntry(entries, s.Castles[20]); ok {
		t.Fatal("stale key guessed as Storm")
	}
	entries["storm"] = "role"
	delete(s.Castles, 20)
	if len(BoundCastleEntries(entries, &s)) != 1 {
		t.Fatal("absent Storm bound")
	}
	s.Castles[21] = State.CastleState{ID: 21, KingdomID: 4}
	s.Castles[22] = State.CastleState{ID: 22, KingdomID: 4}
	if len(BoundCastleEntries(entries, &s)) != 3 {
		t.Fatal("role must apply to every owned Storm castle")
	}
}

func TestStormRoleAutoBirdGuardAndReserveGolden(t *testing.T) {
	now := time.Now().UTC()
	s, _ := autoBirdEligibleTestState(t, now)
	storm := State.CastleState{ID: 20, KingdomID: 4, Name: "Synthetic Storm"}
	s.Castles[20] = storm
	for _, raw := range []string{`{"ignoreSettings":{"settings":{}}}`, `{"ignoreSettings":{"settings":{"storm":[]}}}`, `{"ignoreSettings":{"settings":{"storm":[],"20":[{"id":489,"amount":37}]}}}`} {
		config := Configuration.Snapshot{Sections: map[string]json.RawMessage{"automation.autoBird": json.RawMessage(raw)}}
		decision, err := NewAutoBirdPolicy().Evaluate(t.Context(), Snapshot{State: s, Configuration: config, Now: now})
		if err != nil || decision.Request == nil {
			t.Fatalf("main castle should proceed: %+v %v", decision, err)
		}
		if !strings.Contains(decision.Detail, "Auto Bird skips Synthetic Storm: no troops to keep are set for the Storm castle.") {
			t.Fatalf("missing Storm guard notice: %s", decision.Detail)
		}
		var args struct {
			CastleID int64 `json:"sourceCastleId"`
		}
		json.Unmarshal(decision.Request.Arguments, &args)
		if args.CastleID != 10 {
			t.Fatalf("unconfigured Storm selected: %d", args.CastleID)
		}
		delete(s.Castles, 10)
		decision, err = NewAutoBirdPolicy().Evaluate(t.Context(), Snapshot{State: s, Configuration: config, Now: now})
		if err != nil || decision.Request != nil || decision.Detail != "Auto Bird skips Synthetic Storm: no troops to keep are set for the Storm castle." {
			t.Fatalf("Storm guard: %+v %v", decision, err)
		}
		s.Castles[10] = State.CastleState{ID: 10, KingdomID: 0}
	}
	cfg := defaultAutoBirdConfiguration()
	cfg.IgnoreSettings.Settings["storm"] = []reserveSetting{{ID: 489, Amount: 37}}
	var args map[string]json.RawMessage
	json.Unmarshal(autoBirdCycleArguments(storm, cfg), &args)
	if string(args["sourceCastleId"]) != "20" || string(args["trackingId"]) != `"autoBird:20"` || string(args["reserves"]) != `[{"unitId":489,"amount":37}]` {
		t.Fatalf("payload golden: %s", autoBirdCycleArguments(storm, cfg))
	}
	// Active preset is subject to the same guard and role resolution.
	config := Configuration.Snapshot{Sections: map[string]json.RawMessage{"automation.autoBird": json.RawMessage(`{"activePresetId":"synthetic","presets":{"presets":[{"id":"synthetic","settings":{"storm":[{"id":489,"amount":37}]}}]}}`)}}
	delete(s.Castles, 10)
	decision, err := NewAutoBirdPolicy().Evaluate(t.Context(), Snapshot{State: s, Configuration: config, Now: now})
	if err != nil || decision.Request == nil {
		t.Fatalf("configured preset: %+v %v", decision, err)
	}
}

func TestStormRoleAutoTowersEventAndIdle(t *testing.T) {
	now := time.Now().UTC()
	snapshot := autoTowerPolicySnapshot(now)
	snapshot.Configuration.Sections["automation.autoTowers"] = json.RawMessage(`{"castles":{"storm":{"enabled":true,"unitId":77,"radius":1}}}`)
	snapshot.State.Castles[1] = State.CastleState{ID: 1, KingdomID: 0}
	snapshot.Configuration.Sections["automation.autoTowers"] = json.RawMessage(`{"castles":{"storm":{"enabled":true,"unitId":77,"radius":1},"1":{"enabled":false}}}`)
	d, err := NewAutoTowerPolicy().Evaluate(t.Context(), snapshot)
	if err != nil || d.Request != nil || d.Detail != "Waiting for a Storm castle" {
		t.Fatalf("idle: %+v %v", d, err)
	}
	for _, id := range []State.CastleID{20, 21} {
		snapshot.State.Castles = map[State.CastleID]State.CastleState{id: {ID: id, KingdomID: 4, X: 100, Y: 100}}
		d, err = NewAutoTowerPolicy().Evaluate(t.Context(), snapshot)
		if err != nil || d.Request == nil || d.ScheduleKey != "autoTowers:storm" {
			t.Fatalf("event castle %d: %+v %v", id, d, err)
		}
		var args map[string]any
		json.Unmarshal(d.Request.Arguments, &args)
		if args["sourceCastleId"] != float64(id) {
			t.Fatalf("live source payload: %s", d.Request.Arguments)
		}
	}
	snapshot.Configuration.Sections["automation.autoTowers"] = json.RawMessage(`{"castles":{"21":{"enabled":true,"unitId":77,"radius":1}}}`)
	d, err = NewAutoTowerPolicy().Evaluate(t.Context(), snapshot)
	if err != nil || d.Request == nil {
		t.Fatalf("legacy tower: %+v %v", d, err)
	}
	// A closed role schedule suppresses the same live castle.
	snapshot.Configuration.Sections["scheduler"] = json.RawMessage(`{"featureSchedules":{"autoTowers:storm":{"enabled":true,"timeZone":"UTC","slots":[]}}}`)
	d, err = NewAutoTowerPolicy().Evaluate(t.Context(), snapshot)
	if err != nil || d.Request != nil {
		t.Fatalf("closed role schedule: %+v %v", d, err)
	}
}

func TestStormRoleAutoStationKeepsReserves(t *testing.T) {
	now := time.Now().UTC()
	s, data := autoBirdEligibleTestState(t, now)
	s.Player.ID = 7
	castle := s.Castles[10]
	castle.KingdomID = 4
	castle.SlotType = 1
	s.Castles[10] = castle
	s.Alliance.Holdings[0].KingdomID = 4
	arrives := now.Add(30 * time.Second)
	s.Movements[1] = State.MovementState{ID: 1, TypeID: 0, Direction: 0, OwnerPlayerID: 8, TargetPlayerID: 7, SourceTypeID: 1, SourceCastleID: 200, TargetTypeID: 1, TargetCastleID: 10, ArrivesAt: &arrives}
	cfg := Configuration.Snapshot{Sections: map[string]json.RawMessage{"automation.autoStation": json.RawMessage(`{"settings":{"storm":[{"id":489,"amount":37}]}}`)}}
	d, err := NewAutoStationPolicy().Evaluate(t.Context(), Snapshot{State: s, GameData: data, Configuration: cfg, Now: now})
	if err != nil || d.Request == nil || d.Request.Name != "troops.station" {
		t.Fatalf("station: %+v %v", d, err)
	}
	var args struct {
		Source State.CastleID `json:"sourceCastleId"`
		Units  []stationUnit  `json:"units"`
	}
	if err := json.Unmarshal(d.Request.Arguments, &args); err != nil {
		t.Fatal(err)
	}
	if args.Source != 10 || len(args.Units) != 1 || args.Units[0].Amount != 63 {
		t.Fatalf("station reserves lost: %s", d.Request.Arguments)
	}
}

func TestStormRoleCurrentDispatchReserveRows(t *testing.T) {
	storm := State.CastleState{ID: 20, KingdomID: 4}
	for _, test := range []struct {
		rows string
		want bool
	}{
		{`{}`, false}, {`{"storm":[]}`, false}, {`{"20":[{"id":489,"amount":37}]}`, true},
		{`{"storm":[],"20":[{"id":489,"amount":37}]}`, false}, {`{"storm":[{"id":489,"amount":0}]}`, true},
		{`{"storm":[{"id":0,"amount":37}]}`, true}, {`{"storm":[{"id":489,"amount":-1}]}`, true},
	} {
		for _, preset := range []string{"", "synthetic"} {
			raw := `{"ignoreSettings":{"settings":` + test.rows + `}}`
			if preset != "" {
				raw = `{"presets":{"presets":[{"id":"synthetic","settings":` + test.rows + `}]}}`
			}
			config := Configuration.Snapshot{Sections: map[string]json.RawMessage{"automation.autoBird": json.RawMessage(raw)}}
			if got := AutoBirdStormReserveConfigured(config, storm, preset); got != test.want {
				t.Fatalf("rows %s preset %q: got %v", test.rows, preset, got)
			}
			if !AutoBirdStormReserveConfigured(config, State.CastleState{ID: 10, KingdomID: 0}, preset) {
				t.Fatal("main castle semantics changed")
			}
		}
	}
}
