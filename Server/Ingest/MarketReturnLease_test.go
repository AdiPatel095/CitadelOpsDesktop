package Ingest

import (
	"encoding/json"
	"testing"
	"time"

	"CitadelDesktop/Server/Protocol"
	"CitadelDesktop/Server/State"
)

func TestMarketReturnWireKeepsCartsReservedAtHome(t *testing.T) {
	now := time.Date(2026, 9, 12, 17, 14, 46, 0, time.UTC)
	state := State.NewGameState()
	state.Player.ID = 1
	state.Session.ConnectionGeneration = 1
	market := State.MarketCastleState{CastleID: 100, TotalBarrows: 125, AvailableBarrows: 125}
	code := 0
	reducer := newMovementReducer(false)
	for _, step := range []struct {
		at      time.Time
		payload string
	}{
		{now, `{"A":{"M":{"MID":50,"PT":0,"TT":81,"D":0,"T":4,"KID":0,"OID":1,"TID":1,"SA":[1,212,941,100,1],"TA":[4,212,939,200,1]},"MM":{"C":125,"G":[]}}}`},
		{now.Add(81 * time.Second), `{"A":{"M":{"MID":50,"PT":0,"TT":81,"D":1,"T":4,"KID":0,"OID":1,"TID":1,"SA":[4,212,939,200,1],"TA":[1,212,941,100,1]},"MM":{"C":125,"G":[]}}}`},
	} {
		if _, _, err := reducer(t.Context(), Protocol.Frame{
			Opcode: "crm", Direction: Protocol.DirectionInbound, ResponseCode: &code,
			ReceivedAt: step.at, Payload: json.RawMessage(step.payload),
		}, &state, nil); err != nil {
			t.Fatal(err)
		}
		if got := State.AvailableMarketBarrowsAt(state, market, step.at); got != 0 {
			t.Fatalf("wire transition exposed %d unavailable home carts", got)
		}
	}
	// Persistence must retain the direction/endpoints needed to rebuild leases.
	raw, err := json.Marshal(state.Movements[50])
	if err != nil {
		t.Fatal(err)
	}
	var restored State.MovementState
	if err := json.Unmarshal(raw, &restored); err != nil {
		t.Fatal(err)
	}
	state.Movements[50] = restored
	returnsAt := now.Add(162 * time.Second)
	if got := State.AvailableMarketBarrowsAt(state, market, returnsAt.Add(-time.Second)); got != 0 {
		t.Fatalf("persisted return lost cart reservation: %d", got)
	}
	if got := State.AvailableMarketBarrowsAt(state, market, returnsAt); got != 125 {
		t.Fatalf("completed return did not release carts: %d", got)
	}
}
