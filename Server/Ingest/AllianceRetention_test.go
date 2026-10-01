package Ingest

import (
	"encoding/json"
	"reflect"
	"slices"
	"testing"
	"time"

	"CitadelDesktop/Server/Protocol"
	"CitadelDesktop/Server/State"
)

func TestAllianceDirectoryRetention(t *testing.T) {
	now := time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC)
	cutoff := now.Add(-7 * 24 * time.Hour)
	for _, tc := range []struct {
		name    string
		seed    func(*State.GameState)
		want    []State.AllianceID
		changed bool
	}{
		{"TTL uses frame time", func(s *State.GameState) {
			s.Alliances[1] = State.AllianceState{ID: 1, ObservedAt: cutoff.Add(-time.Nanosecond)}
			s.Alliances[2] = State.AllianceState{ID: 2, ObservedAt: cutoff}
			s.Alliances[3] = State.AllianceState{ID: 3}
		}, []State.AllianceID{2}, true},
		{"cap sorts observation then ID", func(s *State.GameState) {
			for i := State.AllianceID(1); i <= 66; i++ {
				s.Alliances[i] = State.AllianceState{ID: i, ObservedAt: now}
			}
			s.Alliances[66] = State.AllianceState{ID: 66, ObservedAt: now.Add(-time.Hour)}
		}, func() []State.AllianceID {
			ids := []State.AllianceID{}
			for i := State.AllianceID(2); i <= 65; i++ {
				ids = append(ids, i)
			}
			return ids
		}(), true},
		{"kept IDs survive age and cap", func(s *State.GameState) {
			s.Alliance.ID = 1
			s.Player.AllianceID = 2
			for i := State.AllianceID(1); i <= 67; i++ {
				s.Alliances[i] = State.AllianceState{ID: i, ObservedAt: now}
			}
			for _, id := range []State.AllianceID{1, 2, 99} {
				s.Alliances[id] = State.AllianceState{ID: id, ObservedAt: cutoff.Add(-time.Hour)}
			}
		}, func() []State.AllianceID {
			ids := []State.AllianceID{1, 2}
			for i := State.AllianceID(4); i <= 67; i++ {
				ids = append(ids, i)
			}
			return append(ids, 99)
		}(), true},
		{"nothing expired", func(s *State.GameState) { s.Alliances[1] = State.AllianceState{ID: 1, ObservedAt: now} }, []State.AllianceID{1}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := State.NewGameState()
			tc.seed(&s)
			replay := State.NewGameState()
			tc.seed(&replay)
			if changed := pruneAllianceDirectory(&s, now, 99); changed != tc.changed {
				t.Fatalf("changed=%v want %v", changed, tc.changed)
			}
			ids := make([]State.AllianceID, 0, len(s.Alliances))
			for id := range s.Alliances {
				ids = append(ids, id)
			}
			slices.Sort(ids)
			if !reflect.DeepEqual(ids, tc.want) {
				t.Fatalf("IDs=%v want %v", ids, tc.want)
			}
			pruneAllianceDirectory(&replay, now, 99)
			if !reflect.DeepEqual(replay.Alliances, s.Alliances) {
				t.Fatal("replay differed")
			}
			if pruneAllianceDirectory(&s, now, 99) {
				t.Fatal("unchanged replay reported a prune")
			}
		})
	}
	t.Run("prune alone publishes alliances", func(t *testing.T) {
		s := State.NewGameState()
		s.Alliances[99] = State.AllianceState{ID: 99, Name: "Observed", Members: []State.AllianceMember{}, Holdings: []State.AllianceHolding{}, ObservedAt: now}
		s.Alliances[1] = State.AllianceState{ID: 1, ObservedAt: cutoff.Add(-time.Second)}
		store := State.NewStore(&s)
		domains, changed, err := reduceAllianceInfo(t.Context(), Protocol.Frame{Opcode: "ain", Direction: Protocol.DirectionInbound, ResponseCode: new(int), ReceivedAt: now, Payload: json.RawMessage(`{"A":{"AID":99,"N":"Observed","M":[]}}`)}, &s, nil)
		if err != nil || !changed || !slices.Contains(domains, "alliances") {
			t.Fatalf("prune alone: changed=%v domains=%v err=%v", changed, domains, err)
		}
		_, changed, err = reduceAllianceInfo(t.Context(), Protocol.Frame{Opcode: "ain", Direction: Protocol.DirectionInbound, ResponseCode: new(int), ReceivedAt: now, Payload: json.RawMessage(`{"A":{"AID":99,"N":"Observed","M":[]}}`)}, &s, nil)
		if err != nil || changed {
			t.Fatalf("replay changed directory: %v %v", changed, err)
		}
		registry := NewRegistry()
		if err := RegisterCoreReducers(registry); err != nil {
			t.Fatal(err)
		}
		pipeline := NewPipeline(store, nil, registry)
		code := 0
		frame := Protocol.Frame{Opcode: "ain", Direction: Protocol.DirectionInbound, ResponseCode: &code, ReceivedAt: now, Payload: json.RawMessage(`{"A":{"AID":99,"N":"Observed","M":[]}}`)}
		event, err := pipeline.HandleFrame(t.Context(), frame)
		if err != nil {
			t.Fatal(err)
		}
		if !slices.Contains(event.Domains, "alliances") || store.Revision() == 0 {
			t.Fatalf("prune event=%+v", event)
		}
		if _, ok := store.ReadOnlyView().Alliances[1]; ok {
			t.Fatal("expired alliance survived")
		}
	})
}
