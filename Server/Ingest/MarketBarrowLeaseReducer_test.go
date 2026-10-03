package Ingest

import (
	"CitadelDesktop/Server/Protocol"
	"CitadelDesktop/Server/State"
	"encoding/json"
	"reflect"
	"testing"
	"time"
)

// Synthetic server-shaped rows and movements, using only invented identities.
func TestMarketBarrowRecordsSurviveCleanupAndBaseline(t *testing.T) {
	now := time.Date(2026, 10, 3, 12, 0, 0, 0, time.UTC)
	r := now.Add(time.Minute)
	for _, kind := range []string{"clock", "scoped gam", "fresh baseline gam"} {
		t.Run(kind, func(t *testing.T) {
			gs := State.NewGameState()
			gs.Player.ID = 1
			gs.Session.ConnectionGeneration = 1
			gs.Castles[10] = State.CastleState{ID: 10}
			gs.Market.Castles[10] = State.MarketCastleState{CastleID: 10, TotalBarrows: 125, AvailableBarrows: 125, ObservedAt: now}
			gs.Movements[50] = State.MovementState{ID: 50, Direction: 1, OwnerPlayerID: 1, SourceCastleID: 20, TargetCastleID: 10, MarketBarrows: 125, ReturnsAt: &r}
			domains, changed, err := reduceMarketBarrowLeases(t.Context(), Protocol.Frame{ReceivedAt: now}, &gs, nil)
			if err != nil || !changed || !reflect.DeepEqual(domains, []string{"market"}) {
				t.Fatalf("record %v %t %v", domains, changed, err)
			}
			raw, _ := json.Marshal(gs)
			var restored State.GameState
			if err := json.Unmarshal(raw, &restored); err != nil {
				t.Fatal(err)
			}
			gs = restored
			if kind == "clock" {
				ReconcileExpiredMovements(&gs, r)
			} else {
				if kind == "fresh baseline gam" {
					gs.Session.ConnectionGeneration++
				}
				code := 0
				reducer := newMovementReducer(true)
				if _, _, err := reducer(t.Context(), Protocol.Frame{Opcode: "gam", Direction: Protocol.DirectionInbound, ResponseCode: &code, ReceivedAt: r, Payload: json.RawMessage(`{"M":[]}`)}, &gs, nil); err != nil {
					t.Fatal(err)
				}
			}
			if _, exists := gs.Movements[50]; exists {
				t.Fatal("movement cleanup changed")
			}
			if got := State.AvailableMarketBarrowsAt(&gs, gs.Market.Castles[10], r); got != 0 {
				t.Fatalf("exposed projected barrows: %d", got)
			}
			if _, exists := gs.Market.BarrowLeases[50]; !exists {
				t.Fatal("durable record missing")
			}
			code := 0
			frame := Protocol.Frame{Opcode: "cmi", ResponseCode: &code, ReceivedAt: r, Payload: json.RawMessage(`{"C":[{"CID":10,"KID":0,"TC":125,"AC":125,"AE":[]}]}`)}
			if _, _, err := reduceMarketInfo(t.Context(), frame, &gs, nil); err != nil {
				t.Fatal(err)
			}
			if len(gs.Market.BarrowLeases) != 1 {
				t.Fatal("observation at R confirmed")
			}
			frame.ReceivedAt = r.Add(time.Nanosecond)
			if _, _, err := reduceMarketInfo(t.Context(), frame, &gs, nil); err != nil {
				t.Fatal(err)
			}
			if len(gs.Market.BarrowLeases) != 0 || !gs.Market.Castles[10].ObservedAt.Equal(frame.ReceivedAt) {
				t.Fatal("source confirmation missing")
			}
			if domains, changed, err := reduceMarketBarrowLeases(t.Context(), frame, &gs, nil); err != nil || changed || len(domains) != 0 {
				t.Fatalf("unchanged market domain %v %t %v", domains, changed, err)
			}
		})
	}
}

func TestMarketBarrowRecordPipelineReturnTransition(t *testing.T) {
	now := time.Date(2026, 10, 3, 12, 0, 0, 0, time.UTC)
	gs := State.NewGameState()
	gs.Player.ID = 1
	gs.Session.ConnectionGeneration = 1
	gs.Castles[100] = State.CastleState{ID: 100}
	store := State.NewStore(&gs)
	registry := NewRegistry()
	if err := RegisterCoreReducers(registry); err != nil {
		t.Fatal(err)
	}
	pipeline := NewPipeline(store, nil, registry)
	code := 0
	for _, tc := range []struct {
		at      time.Time
		payload string
	}{
		{now, `{"A":{"M":{"MID":50,"PT":0,"TT":81,"D":0,"T":4,"KID":0,"OID":1,"TID":1,"SA":[1,212,941,100,1],"TA":[4,212,939,200,1]},"MM":{"C":125,"G":[]}}}`},
		{now.Add(81 * time.Second), `{"A":{"M":{"MID":50,"PT":0,"TT":81,"D":1,"T":4,"KID":0,"OID":1,"TID":1,"SA":[4,212,939,200,1],"TA":[1,212,941,100,1]},"MM":{"C":125,"G":[]}}}`},
	} {
		if _, err := pipeline.HandleFrame(t.Context(), Protocol.Frame{Opcode: "crm", Direction: Protocol.DirectionInbound, ResponseCode: &code, ReceivedAt: tc.at, Payload: json.RawMessage(tc.payload)}); err != nil {
			t.Fatal(err)
		}
		state := store.Snapshot()
		record := state.Market.BarrowLeases[50]
		if record.HomeCastleID != 100 || record.Barrows != 125 || !record.ReleasesAt.Equal(now.Add(162*time.Second)) {
			t.Fatalf("record=%+v", record)
		}
		if got := State.MarketBarrowLeaseAt(&state, 100, tc.at).Barrows; got != 125 {
			t.Fatalf("count=%d", got)
		}
	}
}
