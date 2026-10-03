package Ingest

import (
	"encoding/json"
	"fmt"
	"os"
	"testing"
	"time"

	"CitadelDesktop/Server/Protocol"
	"CitadelDesktop/Server/State"
)

func TestStormArrivalKRSAndSenderShapedGAM(t *testing.T) {
	now := time.Date(2026, 9, 20, 12, 0, 0, 0, time.UTC)
	state := State.NewGameState()
	state.Player.ID = 71001
	state.Session.ConnectionGeneration = 7
	state.Session.ChangedAt = now.Add(-time.Hour)
	state.Castles[81001] = State.CastleState{ID: 81001, KingdomID: 4, X: 100, Y: 100}
	code := 0
	frame := Protocol.Frame{Opcode: "kpi", Direction: Protocol.DirectionInbound, ResponseCode: &code, ReceivedAt: now,
		Payload: json.RawMessage(`{"kpi":{"UL":[{"KID":4,"U":1,"C":1,"KRS":3600}]}}`)}
	if _, _, err := reduceKingdomTransport(t.Context(), frame, &state, nil); err != nil {
		t.Fatal(err)
	}
	end, known := state.StormEventEndAt(now)
	if !known || !end.Equal(now.Add(time.Hour)) {
		t.Fatalf("KRS deadline: %s known=%t", end, known)
	}
	frame.ReceivedAt = now.Add(time.Minute)
	frame.Payload = json.RawMessage(`{"UL":[{"KID":4,"U":1,"KRS":3540}]}`)
	if _, _, err := reduceKingdomTransport(t.Context(), frame, &state, nil); err != nil {
		t.Fatal(err)
	}
	if !state.KingdomTransport.Unlocks[4].EventObservedFrom.Equal(now) {
		t.Fatal("KPI refresh lost event observation boundary")
	}
	raw, err := os.ReadFile("testdata/storm_arrival.gam.json")
	if err != nil {
		t.Fatal(err)
	}
	frame.Opcode, frame.Payload = "gam", raw
	reducer := newMovementReducer(true)
	if _, _, err := reducer(t.Context(), frame, &state, nil); err != nil {
		t.Fatal(err)
	}
	movement, found := state.LookupMovement(91001)
	if !found || movement.TravelSeconds != 180 || movement.HorseBoosterWID == nil || *movement.HorseBoosterWID != -1 {
		t.Fatalf("decoded official travel fields: %+v", movement)
	}
	if len(state.Storm.TravelObservations) != 1 {
		t.Fatal("official TT was not retained")
	}
	frame.ReceivedAt = now.Add(61 * time.Second)
	domains, _, err := reducer(t.Context(), frame, &state, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, domain := range domains {
		if domain == "storm" {
			t.Fatal("unchanged GAM timing jitter rewrote the travel cache")
		}
	}
	frame.ReceivedAt = now.Add(5 * time.Minute)
	frame.Payload = json.RawMessage(`{"M":[]}`)
	if _, _, err := reducer(t.Context(), frame, &state, nil); err != nil {
		t.Fatal(err)
	}
	if len(state.Storm.TravelObservations) != 1 {
		t.Fatal("completed GAM movement erased the event's travel report")
	}
	// UT-only replies cannot refresh the full snapshot's deadline or its authority.
	frame.Opcode, frame.Payload = "kut", json.RawMessage(`{"UT":[]}`)
	frame.ReceivedAt = now.Add(6 * time.Minute)
	if _, _, err := reduceKingdomTransport(t.Context(), frame, &state, nil); err != nil {
		t.Fatal(err)
	}
	if !state.KingdomTransport.Unlocks[4].EventEndObservedAt.Equal(now.Add(time.Minute)) {
		t.Fatal("partial KUT refreshed deadline authority")
	}
	state.Session.ConnectionGeneration++
	if _, known := state.StormEventEndAt(frame.ReceivedAt); known {
		t.Fatal("old connection authorized the deadline")
	}
}

func TestStormArrivalKRSInvalidOrMissingClearsDeadline(t *testing.T) {
	for _, field := range []string{"", `,"KRS":0`, `,"KRS":-1`, `,"KRS":"invalid"`, `,"KRS":1.5`, `,"KRS":9223372036854775807`} {
		t.Run(field, func(t *testing.T) {
			now := time.Now().UTC()
			state := State.NewGameState()
			state.KingdomTransport.Unlocks[4] = State.KingdomTransportUnlock{KingdomID: 4, Unlocked: true, EventEndsAt: now.Add(time.Hour)}
			code := 0
			_, _, err := reduceKingdomTransport(t.Context(), Protocol.Frame{
				Opcode: "kpi", Direction: Protocol.DirectionInbound, ResponseCode: &code, ReceivedAt: now,
				Payload: json.RawMessage(fmt.Sprintf(`{"UL":[{"KID":4,"U":1%s}]}`, field)),
			}, &state, nil)
			if err != nil {
				t.Fatal(err)
			}
			if !state.KingdomTransport.Unlocks[4].EventEndsAt.IsZero() {
				t.Fatal("invalid/missing KRS retained old deadline")
			}
		})
	}
}
