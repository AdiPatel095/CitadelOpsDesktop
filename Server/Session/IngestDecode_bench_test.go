package Session

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"testing"
	"time"

	"CitadelDesktop/Server/GameData"
	"CitadelDesktop/Server/Ingest"
	"CitadelDesktop/Server/Outbound"
	"CitadelDesktop/Server/State"
)

type decodeBenchGameData struct{ store *GameData.Store }

func (p decodeBenchGameData) Current() (*GameData.Store, bool) { return p.store, p.store != nil }
func decodeBenchPipeline(b *testing.B) *Ingest.Pipeline {
	b.Helper()
	raw, err := os.ReadFile("../Ingest/testdata/ingest_session_gamedata.json")
	if err != nil {
		b.Fatal(err)
	}
	data, err := GameData.DecodeStore(raw, GameData.SourceMetadata{ItemVersion: "fixture"})
	if err != nil {
		b.Fatal(err)
	}
	state := State.NewGameState()
	state.Player.ID = 424242
	state.Session = State.SessionState{LoggedIn: true, SocketReady: true, Generation: 1, ConnectionGeneration: 1}
	state.Castles[100001] = State.CastleState{ID: 100001}
	state.Castles[100002] = State.CastleState{ID: 100002}
	registry := Ingest.NewRegistry()
	if err := Ingest.RegisterCoreReducers(registry); err != nil {
		b.Fatal(err)
	}
	return Ingest.NewPipeline(State.NewStore(state), decodeBenchGameData{data}, registry)
}
func decodeBenchGaa() string {
	var rows, owners []any
	for i := 0; i < 300; i++ {
		if i%5 == 0 {
			rows = append(rows, []any{1, 100 + i, 100, 100001 + i, 500000 + i/5, 0, 0, 0, 0, 0, fmt.Sprintf("Fixture Castle %d", i)})
		} else {
			rows = append(rows, []any{2, 100 + i, 100, -1, 845, 60, 0})
		}
	}
	for i := 0; i < 60; i++ {
		id := 500000 + i
		if i == 0 {
			id = 424242
		}
		owners = append(owners, map[string]any{"OID": id, "N": fmt.Sprintf("Fixture Player %d", i), "RPT": 600})
	}
	raw, _ := json.Marshal(map[string]any{"KID": 0, "AI": rows, "OI": owners})
	return fmt.Sprintf("%%xt%%gaa%%1%%0%%%s%%", raw)
}
func decodeBenchGam() string {
	var movements []any
	for i := 0; i < 60; i++ {
		um := map[string]any{"L": map[string]any{"ID": i + 1}}
		if i == 0 {
			um["AAT"] = 1
			um["AAN"] = 1
			um["AAC"] = 12
		}
		movements = append(movements, map[string]any{"M": map[string]any{"MID": 700001 + i, "PT": 1, "TT": 60, "D": 0, "T": 0, "KID": 0, "OID": 424242, "TID": 500001, "SA": []any{1, 100, 100, 100001, 424242}, "TA": []any{1, 120 + i, 120, 100010 + i, 500001}}, "UM": um})
	}
	raw, _ := json.Marshal(map[string]any{"M": movements, "O": []any{map[string]any{"OID": 424242, "PRE": 116, "SUF": 31, "TOPX": 50, "CF": 123456}}})
	return fmt.Sprintf("%%xt%%gam%%1%%0%%%s%%", raw)
}
func runDecodeBench(b *testing.B, raw string, pending int) {
	pipeline := decodeBenchPipeline(b)
	transport := &DirectWebSocketTransport{frames: make(chan RawFrame, 1)}
	ctx := context.Background()
	start := time.Now().UTC().Add(-time.Hour)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for j := 0; j < pending; j++ {
			x1, x2 := 100, 399
			if j > 0 {
				x1 = 1000 * j
				x2 = x1 + 299
			}
			request := fmt.Sprintf("%%xt%%EmpireEx_21%%gaa%%1%%{\"KID\":0,\"AX1\":%d,\"AY1\":100,\"AX2\":%d,\"AY2\":100}%%", x1, x2)
			if _, err := transport.registerPending(Outbound.Metadata{ResponseToken: fmt.Sprintf("bench-%d", j), ResponseOpcodes: []string{"gaa"}}, request); err != nil {
				b.Fatal(err)
			}
		}
		transport.deliverInbound(raw, 1)
		rf := drainOutbox(transport)[0]
		observed := pipeline.ObserveTransportFrame(*rf.Decoded, rf.ResponseToken, rf.CausationOperationID)
		// Advance frame time deterministically, so repeated map rows always change.
		observed.Frame.ReceivedAt = start.Add(time.Duration(i) * time.Millisecond)
		if _, err := pipeline.CommitFrame(ctx, observed); err != nil {
			b.Fatal(err)
		}
		transport.pending = nil
	}
}
func BenchmarkDirectInboundGaa300Rows(b *testing.B) {
	raw := decodeBenchGaa()
	for _, pending := range []int{1, 3} {
		b.Run(fmt.Sprintf("pending=%d", pending), func(b *testing.B) { runDecodeBench(b, raw, pending) })
	}
}
func BenchmarkDirectInboundGam60Movements(b *testing.B) { runDecodeBench(b, decodeBenchGam(), 0) }
