package Ingest

import (
	"fmt"
	"reflect"
	"testing"
	"time"

	"CitadelDesktop/Server/Protocol"
	"CitadelDesktop/Server/State"
)

func membershipTestState(at time.Time) State.GameState {
	s := State.NewGameState()
	s.Player.ID = 7
	s.Player.AllianceID = 9
	s.Player.AllianceObservedAt = at
	s.Player.AllianceMembershipObservedAt = at
	s.Player.AllianceMembershipGeneration = 7
	s.Session.Generation = 7
	s.Session.ChangedAt = at.Add(-time.Minute)
	s.Alliance = State.AllianceState{ID: 9, Members: []State.AllianceMember{{PlayerID: 7}}}
	return s
}

// Passing version of Ethan's Addendum 1 wiring repro. All IDs are synthetic;
// response payloads echo sender fields rather than inventing a membership body.
func TestCIT121AddendumResponseWiringRepro(t *testing.T) {
	base := time.Date(2026, 9, 24, 20, 30, 30, 0, time.UTC)
	for _, opcode := range []string{"aha", "ahr"} {
		for _, code := range []int{270, 114} {
			t.Run(fmt.Sprintf("%s/%d", opcode, code), func(t *testing.T) {
				state := membershipTestState(base.Add(-time.Second))
				store, pipeline := commandCorrelationPipeline(t, state)
				beforeAlliance := store.ReadOnlyView().Alliance
				payload := `{"KID":15}`
				if opcode == "ahr" {
					payload = `{"ID":205,"T":2}`
				}
				if _, err := pipeline.HandleRawAt(t.Context(), fmt.Sprintf("%%xt%%%s%%1%%%d%%%s%%", opcode, code, payload), Protocol.DirectionInbound, base); err != nil {
					t.Fatal(err)
				}
				got := store.ReadOnlyView()
				if got.Player.AllianceID != 0 || State.AllianceMembershipCurrent(&got) ||
					!got.Player.AllianceMembershipObservedAt.Equal(base) || got.Player.AllianceMembershipGeneration != 7 {
					t.Fatalf("rejection did not commit authoritative none: %+v", got.Player)
				}
				if !reflect.DeepEqual(got.Alliance, beforeAlliance) || !got.Player.AllianceObservedAt.Equal(state.Player.AllianceObservedAt) {
					t.Fatal("help rejection changed roster authority or AllianceObservedAt")
				}
			})
		}
	}
}

func TestCIT121PlanFreshGBDMembershipRepro(t *testing.T) {
	base := time.Date(2026, 9, 24, 20, 30, 30, 0, time.UTC)
	state := membershipTestState(base.Add(-time.Second))
	store, pipeline := commandCorrelationPipeline(t, state)
	if _, err := pipeline.HandleRawAt(t.Context(), `%xt%aha%1%270%{"KID":15}%`, Protocol.DirectionInbound, base); err != nil {
		t.Fatal(err)
	}
	fresh := base.Add(2 * time.Second)
	if _, err := pipeline.HandleRawAt(t.Context(), `%xt%gbd%1%0%{"gpi":{"PID":7},"gal":{"AID":9}}%`, Protocol.DirectionInbound, fresh); err != nil {
		t.Fatal(err)
	}
	got := store.ReadOnlyView()
	if !State.AllianceMembershipCurrent(&got) || !got.Player.AllianceMembershipObservedAt.Equal(fresh) || got.Player.AllianceMembershipGeneration != 7 {
		t.Fatalf("fresh GBD cannot resume help: %+v", got.Player)
	}
	// Legacy GBD freshness stays deliberately separate from the help record.
	if !got.Player.AllianceObservedAt.IsZero() {
		t.Fatal("GBD changed legacy freshness semantics")
	}
}

func TestGBDAllianceMembershipPresenceAndRefresh(t *testing.T) {
	base := time.Now().UTC()
	for _, tc := range []struct {
		name, gal string
		id        State.AllianceID
		current   bool
		observed  bool
	}{
		{"omitted", `{}`, 9, false, false},
		{"positive", `{"AID":9}`, 9, true, true},
		{"zero", `{"AID":0}`, 0, false, true},
		{"explicit minus one", `{"AID":-1}`, 0, false, true},
		{"invalid negative", `{"AID":-2}`, 9, false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			state := membershipTestState(base)
			state.Player.AllianceMembershipObservedAt = time.Time{}
			state.Player.AllianceMembershipGeneration = 0
			store, pipeline := commandCorrelationPipeline(t, state)
			before := store.ReadOnlyView()
			raw := fmt.Sprintf("%%xt%%gbd%%1%%0%%{\"gpi\":{\"PID\":7},\"gal\":%s}%%", tc.gal)
			if _, err := pipeline.HandleRawAt(t.Context(), raw, Protocol.DirectionInbound, base); err != nil {
				t.Fatal(err)
			}
			got := store.ReadOnlyView()
			if got.Player.AllianceID != tc.id || State.AllianceMembershipCurrent(&got) != tc.current || (!got.Player.AllianceMembershipObservedAt.IsZero()) != tc.observed {
				t.Fatalf("GBD membership = %+v", got.Player)
			}
			if tc.name == "explicit minus one" && (!reflect.DeepEqual(got.Alliance, before.Alliance) || !got.Player.AllianceObservedAt.Equal(state.Player.AllianceObservedAt)) {
				t.Fatal("explicit none changed legacy gal roster authority or freshness")
			}
			if tc.current {
				fresh := base.Add(time.Second)
				if _, err := pipeline.HandleRawAt(t.Context(), raw, Protocol.DirectionInbound, fresh); err != nil {
					t.Fatal(err)
				}
				if !store.ReadOnlyView().Player.AllianceMembershipObservedAt.Equal(fresh) {
					t.Fatal("unchanged AID did not refresh authority")
				}
			}
		})
	}
}

func TestOwnPlayerSnapshotMembershipAuthority(t *testing.T) {
	base := time.Now().UTC()
	for _, tc := range []struct {
		name, raw string
		id        State.AllianceID
		current   bool
	}{
		{"own member", `{"OI":[{"OID":7,"AID":10}]}`, 10, true},
		{"own none", `{"OI":[{"OID":7,"AID":-1}]}`, 0, false},
		{"another player", `{"OI":[{"OID":8,"AID":10}]}`, 9, false},
		{"missing AID", `{"OI":[{"OID":7}]}`, 9, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			state := membershipTestState(base.Add(-time.Second))
			state.Player.AllianceMembershipObservedAt = time.Time{}
			store, pipeline := commandCorrelationPipeline(t, state)
			if _, err := pipeline.HandleRawAt(t.Context(), "%xt%gaa%1%0%{\"KID\":0,\"AI\":[],"+tc.raw[1:]+"%", Protocol.DirectionInbound, base); err != nil {
				t.Fatal(err)
			}
			got := store.ReadOnlyView()
			if got.Player.AllianceID != tc.id || State.AllianceMembershipCurrent(&got) != tc.current {
				t.Fatalf("own snapshot authority = %+v", got.Player)
			}
		})
	}
}

func TestMembershipReducerIgnoresOtherCodesAndDirections(t *testing.T) {
	base := time.Now().UTC()
	for _, tc := range []struct {
		opcode    string
		code      int
		direction Protocol.Direction
	}{
		{"ahr", 269, Protocol.DirectionInbound}, {"ahr", 273, Protocol.DirectionInbound},
		{"aha", 0, Protocol.DirectionInbound}, {"hru", 114, Protocol.DirectionInbound},
		{"ahr", 270, Protocol.DirectionOutbound},
	} {
		state := membershipTestState(base)
		before := state.Player
		_, changed, err := reduceAllianceHelpMembershipRejection(t.Context(), Protocol.Frame{Opcode: tc.opcode, ResponseCode: &tc.code, Direction: tc.direction, ReceivedAt: base.Add(time.Second)}, &state, nil)
		if err != nil || changed || !reflect.DeepEqual(before, state.Player) {
			t.Fatalf("unrelated response changed membership: %+v", tc)
		}
	}
}

func TestAHR269PreservesMembershipAndRecruitmentIneligibility(t *testing.T) {
	base := time.Now().UTC()
	state := membershipTestState(base)
	castle := newCastleState(77)
	castle.Focused = true
	castle.Production[0] = State.ProductionQueue{LineID: 0, ObservedAt: base, Active: &State.QueueItem{ProductionID: 201, Amount: 5}}
	state.Castles[77] = castle
	store, pipeline := commandCorrelationPipeline(t, state)
	if _, err := pipeline.HandleRawAt(t.Context(), `%xt%EmpireEx_21%ahr%1%{"ID":0,"T":6}%`, Protocol.DirectionOutbound, base); err != nil {
		t.Fatal(err)
	}
	if _, err := pipeline.HandleRawAt(t.Context(), `%xt%ahr%1%269%{"ID":0,"T":6}%`, Protocol.DirectionInbound, base.Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	got := store.ReadOnlyView()
	record, ok := got.AllianceHelpRequests.IneligibleRecruitment[77]
	if !ok || !reflect.DeepEqual(record.ProductionIDs, []int64{201}) || len(got.CommandContext.PendingRequests) != 0 {
		t.Fatalf("269 correlation changed: %+v", got.AllianceHelpRequests)
	}
	if !State.AllianceMembershipCurrent(&got) || !got.Player.AllianceMembershipObservedAt.Equal(base) {
		t.Fatal("269 invalidated membership")
	}
	if !State.AutomationRejectionWhitelisted("ahr", 273) || State.AutomationRejectionWhitelisted("ahr", 269) {
		t.Fatal("CIT-13 whitelist contract changed")
	}
}
