package Ingest

import (
	"encoding/json"
	"testing"
	"time"

	"CitadelDesktop/Server/Protocol"
	"CitadelDesktop/Server/State"
)

func TestAutoKhanReducersTrackTauntImpactAndResolution(t *testing.T) {
	now := time.Date(2026, 7, 14, 15, 0, 0, 0, time.UTC)
	gameState := State.NewGameState()
	gameState.Player.ID = 158
	main := newCastleState(1)
	main.KingdomID = 0
	main.SlotType = 1
	main.X = 212
	main.Y = 941
	gameState.Castles[1] = main
	gameState.EventScores.ByEvent[72] = State.ScalableEventScore{EventID: 72, RemainingSec: 7_200, ObservedAt: now}
	code := 0
	domains, changed, err := newMovementReducer(true)(t.Context(), Protocol.Frame{
		Opcode: "gam", Direction: Protocol.DirectionInbound, ResponseCode: &code, ReceivedAt: now,
		Payload: json.RawMessage(`{"M":[{
			"M":{"MID":10,"PT":0,"TT":22,"D":1,"TID":158,"T":2,"KID":0,"SA":[2,210,942,-1,845,10798,0],"TA":[1,212,941,1,158]},
			"A":[[215,197]]
		}]}`),
	}, &gameState, nil)
	if err != nil || !changed {
		t.Fatalf("Khan taunt snapshot: domains=%v changed=%t err=%v", domains, changed, err)
	}
	taunt, exists := gameState.Khan.Taunts[10]
	if !exists || !taunt.ImpactAt.Equal(now.Add(22*time.Second)) || gameState.Khan.TauntsObserved != 1 {
		t.Fatalf("tracked Khan taunt = %#v, state=%#v", taunt, gameState.Khan)
	}
	if !ReconcileExpiredMovements(&gameState, now.Add(23*time.Second)) {
		t.Fatal("expired Khan taunt did not reconcile")
	}
	if len(gameState.Khan.Taunts) != 0 || gameState.Khan.TauntsResolved != 1 || gameState.Khan.LastTauntResolvedAt.IsZero() {
		t.Fatalf("resolved Khan state = %#v", gameState.Khan)
	}
}

func TestAutoKhanReducersStoreSixHourOpenGateConfirmation(t *testing.T) {
	now := time.Date(2026, 7, 14, 15, 0, 0, 0, time.UTC)
	gameState := State.NewGameState()
	main := newCastleState(1)
	main.KingdomID = 0
	main.SlotType = 1
	main.Focused = true
	gameState.Castles[1] = main
	code := 0
	domains, changed, err := reduceOpenGate(t.Context(), Protocol.Frame{
		Opcode: "mos", Direction: Protocol.DirectionInbound, ResponseCode: &code, ReceivedAt: now,
		Payload: json.RawMessage(`{"CID":1,"RPT":21600}`),
	}, &gameState, nil)
	if err != nil || !changed {
		t.Fatalf("open gate response: domains=%v changed=%t err=%v", domains, changed, err)
	}
	until := gameState.Castles[1].Defense.OpenGateUntil
	if until == nil || !until.Equal(now.Add(6*time.Hour)) {
		t.Fatalf("open gate until = %v, want %v", until, now.Add(6*time.Hour))
	}
}

func TestCastleOpenGateUntilSupportsRemainingAndAbsoluteWireValues(t *testing.T) {
	now := time.Date(2026, 7, 14, 15, 0, 0, 0, time.UTC)
	for name, value := range map[string]int64{
		"remaining":   21_600,
		"unixSeconds": now.Add(6 * time.Hour).Unix(),
		"unixMillis":  now.Add(6 * time.Hour).UnixMilli(),
	} {
		t.Run(name, func(t *testing.T) {
			until := castleOpenGateUntil(value, now)
			if until == nil || !until.Equal(now.Add(6*time.Hour)) {
				t.Fatalf("open gate until = %v, want %v", until, now.Add(6*time.Hour))
			}
		})
	}
}
