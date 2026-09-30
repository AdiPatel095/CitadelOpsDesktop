package Ingest

import (
	"context"
	"runtime/pprof"
	"testing"
	"time"

	"CitadelDesktop/Server/GameData"
	"CitadelDesktop/Server/Profiling"
	"CitadelDesktop/Server/Protocol"
	"CitadelDesktop/Server/State"
)

func labelsSeenByReducer(t *testing.T, ctx context.Context, opcode string) map[string]string {
	t.Helper()
	registry := NewRegistry()
	seen := map[string]string{}
	if err := registry.Register(opcode, func(reducerCtx context.Context, _ Protocol.Frame, _ *State.GameState, _ *GameData.Store) ([]string, bool, error) {
		for _, key := range []string{Profiling.LabelRuntime, Profiling.LabelStage, Profiling.LabelOpcode} {
			if value, found := pprof.Label(reducerCtx, key); found {
				seen[key] = value
			}
		}
		return nil, false, nil
	}); err != nil {
		t.Fatal(err)
	}
	pipeline := NewPipeline(State.NewStore(State.NewGameState()), nil, registry)
	code := 0
	if _, err := pipeline.HandleFrame(ctx, Protocol.Frame{
		Direction: Protocol.DirectionInbound, Opcode: opcode, ResponseCode: &code, ReceivedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatal(err)
	}
	return seen
}

func TestIngestLabelsEachFrameWithItsOpcodeUnderTheRuntime(t *testing.T) {
	listener, err := Profiling.Start(context.Background(), "127.0.0.1:0", t.Logf)
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Stop()
	ctx := Profiling.WithRuntime(context.Background(), "acct-3")
	for _, opcode := range []string{"gaa", "gam"} {
		seen := labelsSeenByReducer(t, ctx, opcode)
		if seen[Profiling.LabelRuntime] != "acct-3" || seen[Profiling.LabelStage] != Profiling.StageIngest || seen[Profiling.LabelOpcode] != opcode {
			t.Errorf("reducer for %s saw labels %v, want runtime acct-3, stage ingest, opcode %s", opcode, seen, opcode)
		}
	}
}

func TestIngestAddsNoLabelsWithoutProfiling(t *testing.T) {
	if seen := labelsSeenByReducer(t, context.Background(), "gaa"); len(seen) != 0 {
		t.Fatalf("labels applied with profiling off: %v", seen)
	}
}

// The per-frame label costs nothing measurable while profiling is off; this
// benchmark documents the hot-path overhead of the disabled check.
func BenchmarkCommitFrameProfilingOff(b *testing.B) {
	registry := NewRegistry()
	_ = registry.Register("gaa", func(context.Context, Protocol.Frame, *State.GameState, *GameData.Store) ([]string, bool, error) {
		return nil, false, nil
	})
	pipeline := NewPipeline(State.NewStore(State.NewGameState()), nil, registry)
	code := 0
	frame := Protocol.Frame{Direction: Protocol.DirectionInbound, Opcode: "gaa", ResponseCode: &code, ReceivedAt: time.Now().UTC()}
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if _, err := pipeline.HandleFrame(b.Context(), frame); err != nil {
			b.Fatal(err)
		}
	}
}
