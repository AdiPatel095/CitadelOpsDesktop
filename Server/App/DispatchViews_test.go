package App

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"testing"
	"time"

	"CitadelDesktop/Server/Configuration"
	"CitadelDesktop/Server/State"
)

func dispatchGuardFixture(tb testing.TB) (*Application, json.RawMessage, json.RawMessage, json.RawMessage) {
	tb.Helper()
	now := time.Now().UTC()
	state := State.NewGameState()
	state.Session.LoggedIn = true
	state.Session.SocketReady = true
	state.Session.ConnectionGeneration = 3
	state.Alliance.Holdings = []State.AllianceHolding{{CastleID: 20, X: 20, Y: 20, SlotType: 1}}
	seedStationAuthority(&state, now)
	for i := 0; i < 20; i++ {
		id := State.CastleID(10 + i)
		state.Castles[id] = State.CastleState{ID: id, KingdomID: 0, SlotType: 1, Focused: i == 0, UnitsObservedAt: now, Units: State.CastleUnits{Stationed: map[State.UnitID]int64{215: 100}}}
	}
	state.Map[0] = map[string]State.MapObservation{}
	for i := 0; i < 5000; i++ {
		state.Map[0][fmt.Sprintf("%d:100", i)] = State.MapObservation{KingdomID: 0, X: i, Y: 100, TypeID: 1, OwnerID: 1, ObservedAt: now}
	}
	arrival := now.Add(30 * time.Second)
	for i := 1; i <= 200; i++ {
		id := State.MovementID(i)
		state.Movements[id] = State.MovementState{ID: id, TypeID: 0, Direction: 0, OwnerPlayerID: 1, SourceCastleID: 100, SourceTypeID: 1, TargetPlayerID: 99, TargetCastleID: 10, TargetTypeID: 1, ArrivesAt: &arrival, ObservedAt: now, Units: map[State.UnitID]int64{215: 100}}
	}
	state.MovementSnapshot.ObservedAt = now
	state.MovementSnapshot.ConnectionGeneration = 3
	state.Stationing["bird"] = State.StationingOperation{ID: "bird", Purpose: "autoBird", SourceCastleID: 10, TargetCastleID: 20}
	config, err := Configuration.Open(tb.TempDir(), map[string]json.RawMessage{
		"automation.enabled":     json.RawMessage(`{"auto_station":true,"auto_bird":true}`),
		"automation.autoStation": json.RawMessage(`{"openGateFallback":true}`),
	})
	if err != nil {
		tb.Fatal(err)
	}
	payload := json.RawMessage(`{"SID":10,"TX":20,"TY":20,"A":[[215,1]]}`)
	station, err := json.Marshal(stationDispatchGuard{TargetOwner: 1, Request: stationRequest{Purpose: "autoStation", SourceCastleID: 10, TargetCastleID: 20, ConnectionGeneration: 3, DispatchStartedAt: now.Add(-time.Second), MinimumRPTDays: 3}, Payload: payload})
	if err != nil {
		tb.Fatal(err)
	}
	bird, err := json.Marshal(autoBirdBatchGuardRequest{TargetOwner: 1, Cycle: autoBirdCycleRequest{SourceCastleID: 10, ExpectedTargetCastle: 20, TrackingID: "bird", ConnectionGeneration: 3, DispatchStartedAt: now.Add(-time.Second), MinimumRPTDays: 3}, Payload: payload})
	if err != nil {
		tb.Fatal(err)
	}
	gate, err := json.Marshal(defenseOpenGateRequest{CastleID: 10, PlannedAt: now.Add(-time.Second), ConnectionGeneration: 3})
	if err != nil {
		tb.Fatal(err)
	}
	return &Application{State: State.NewStore(state), Configuration: config}, station, bird, gate
}

func BenchmarkStationDispatchGuard(b *testing.B) {
	app, station, _, _ := dispatchGuardFixture(b)
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		if err := app.guardStationDispatch(context.Background(), station); err != nil {
			b.Fatal(err)
		}
	}
}

func TestDispatchGuardsUnderConcurrentMutation(t *testing.T) {
	app, station, bird, gate := dispatchGuardFixture(t)
	payload := json.RawMessage(`{"SID":10,"TX":20,"TY":20,"A":[[215,1]]}`)
	guards := []struct {
		name string
		call func() error
	}{
		{"station", func() error { return app.guardStationDispatch(t.Context(), station) }},
		{"support", func() error { return app.guardSupportBatch(t.Context(), payload) }},
		{"bird", func() error { return app.guardAutoBirdBatch(t.Context(), bird) }},
		{"gate", func() error { return app.guardOpenGate(t.Context(), gate) }},
	}
	for _, guard := range guards {
		if err := guard.call(); err != nil {
			t.Fatalf("%s baseline: %v", guard.name, err)
		}
	}
	before := app.State.ReadOnlyView()
	stop := make(chan struct{})
	done := make(chan struct{})
	started := make(chan struct{})
	go func() {
		defer close(done)
		for i := 0; ; i++ {
			select {
			case <-stop:
				return
			default:
			}
			_, err := app.State.Apply(func(s *State.GameState) ([]string, bool, error) {
				castle := s.Castles[11]
				castle.Units.Stationed[215] = int64(i + 101)
				s.Castles[11] = castle
				movement := s.Movements[200]
				movement.Units[215] = int64(i + 101)
				s.Movements[200] = movement
				s.Map[0]["0:100"] = State.MapObservation{KingdomID: 0, X: 0, Y: 100, TypeID: 1, OwnerID: State.PlayerID(i + 1)}
				s.Alliance.Members[1].Might = float64(i)
				s.Stationing[State.AutoBirdControlID(11)] = State.StationingOperation{UpdatedAt: time.Now().UTC()}
				return []string{"castles", "movements", "map", "alliance", "autoBirdControls"}, true, nil
			})
			if i == 0 {
				close(started)
			}
			if err != nil {
				t.Errorf("mutation: %v", err)
				return
			}
		}
	}()
	<-started
	var readers sync.WaitGroup
	for _, guard := range guards {
		readers.Add(1)
		go func() {
			defer readers.Done()
			for i := 0; i < 100; i++ {
				if err := guard.call(); err != nil {
					t.Errorf("%s: %v", guard.name, err)
					return
				}
			}
		}()
	}
	readers.Wait()
	close(stop)
	<-done
	movement, _ := before.LookupMovement(200)
	if before.Castles[11].Units.Stationed[215] != 100 || movement.Units[215] != 100 || before.Alliance.Members[1].Might != 0 {
		t.Fatal("immutable generation mutated")
	}
}
