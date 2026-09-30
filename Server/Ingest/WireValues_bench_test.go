package Ingest

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"CitadelDesktop/Server/Protocol"
	"CitadelDesktop/Server/State"
)

// gaaBenchmarkPayload is a 300-row map read in the shape the game sends for a
// map window: tower, invasion-target and camp rows, every number a plain integer.
func gaaBenchmarkPayload() json.RawMessage {
	rows := make([]string, 0, 300)
	for index := 0; index < 300; index++ {
		x, y := 200+index%30, 900+index/30
		switch index % 3 {
		case 0:
			rows = append(rows, fmt.Sprintf("[2,%d,%d,-1,845,%d,0]", x, y, 2759+index))
		case 1:
			rows = append(rows, fmt.Sprintf("[21,%d,%d,70,-1,%d]", x, y, index%2))
		default:
			rows = append(rows, fmt.Sprintf("[34,%d,%d,80,-1,%d]", x, y, index%2))
		}
	}
	return json.RawMessage(`{"KID":0,"AI":[` + strings.Join(rows, ",") + `]}`)
}

// gamBenchmarkPayload is a 60-movement reply with plain-integer identities and coordinates.
func gamBenchmarkPayload() json.RawMessage {
	movements := make([]string, 0, 60)
	for index := 0; index < 60; index++ {
		id := 5000 + index
		movements = append(movements, fmt.Sprintf(
			`{"M":{"MID":%d,"PT":2,"TT":20,"D":0,"T":0,"KID":0,"OID":%d,"TID":%d,"SA":[0,%d,%d,%d,%d],"TA":[0,%d,%d,%d,%d]},"UM":{"TWD":21600,"L":{"ID":%d}}}`,
			id, 1+index%2, 900+index, 10+index, 11+index, 100+index, 1+index%2, 20+index, 21+index, 300+index, 900+index, 7+index%2,
		))
	}
	return json.RawMessage(`{"M":[` + strings.Join(movements, ",") + `]}`)
}

func BenchmarkReduceMapSnapshotGaa300Rows(b *testing.B) {
	payload := gaaBenchmarkPayload()
	code := 0
	frame := Protocol.Frame{Opcode: "gaa", Direction: Protocol.DirectionInbound, ResponseCode: &code, ReceivedAt: time.Now().UTC(), Payload: payload}
	b.ReportAllocs()
	b.ResetTimer()
	for iteration := 0; iteration < b.N; iteration++ {
		gameState := State.NewGameState()
		if _, _, err := reduceMapSnapshot(b.Context(), frame, &gameState, nil); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkMovementReducerGam60Movements(b *testing.B) {
	payload := gamBenchmarkPayload()
	code := 0
	frame := Protocol.Frame{Opcode: "gam", Direction: Protocol.DirectionInbound, ResponseCode: &code, ReceivedAt: time.Now().UTC(), Payload: payload}
	reducer := newMovementReducer(true)
	b.ReportAllocs()
	b.ResetTimer()
	for iteration := 0; iteration < b.N; iteration++ {
		gameState := State.NewGameState()
		gameState.Player.ID = 1
		gameState.Castles[100] = newCastleState(100)
		if _, _, err := reducer(b.Context(), frame, &gameState, nil); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkRawInt64Plain(b *testing.B) {
	raw := json.RawMessage("845")
	b.ReportAllocs()
	for iteration := 0; iteration < b.N; iteration++ {
		if _, ok := rawInt64(raw); !ok {
			b.Fatal("rawInt64 rejected a plain integer")
		}
	}
}
