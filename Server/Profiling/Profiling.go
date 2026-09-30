// Package Profiling is the opt-in, loopback-only profiling surface of a hosted
// cell (CIT-42). It is off unless CITADEL_PPROF_ADDR is set: no listener exists,
// no mutex/block sampling runs, and Do/WithRuntime never touch profiler labels,
// so an unprofiled cell pays one atomic load per labelled call site.
package Profiling

import (
	"context"
	"runtime/pprof"
	"sync/atomic"
)

// EnvAddr names the environment variable holding the loopback listen address.
const EnvAddr = "CITADEL_PPROF_ADDR"

// Label keys and stage values. A CPU profile can be sliced by any of them, for
// example `go tool pprof -tagfocus=runtime=<id>` or `-tagroot=stage`.
const (
	LabelRuntime = "runtime"
	LabelStage   = "stage"
	LabelPolicy  = "policy"
	LabelOpcode  = "opcode"

	StageIngest        = "ingest"
	StageTransport     = "transport"
	StageAutomation    = "automation"
	StagePersist       = "persist"
	StageWorldMapAdopt = "worldmap-adopt"
	StageAPI           = "api"
)

var enabled atomic.Bool

// Enabled reports whether profiler labels are being applied.
func Enabled() bool { return enabled.Load() }

// WithRuntime returns ctx carrying the runtime label, so every stage label added
// below it (and every goroutine started under Do with it) is attributed to the
// runtime. Without profiling it returns ctx unchanged.
func WithRuntime(ctx context.Context, runtimeID string) context.Context {
	if !enabled.Load() {
		return ctx
	}
	if ctx == nil {
		ctx = context.Background()
	}
	return pprof.WithLabels(ctx, pprof.Labels(LabelRuntime, runtimeID))
}

// Do runs fn with the given key/value labels added to those ctx carries, and
// restores the previous labels afterwards. Goroutines started inside fn inherit
// the labels. fn receives the labelled context; pass it down so nested labels
// keep the outer ones. Without profiling it just calls fn(ctx).
func Do(ctx context.Context, fn func(context.Context), keyValues ...string) {
	if ctx == nil {
		ctx = context.Background()
	}
	if !enabled.Load() {
		fn(ctx)
		return
	}
	pprof.Do(ctx, pprof.Labels(keyValues...), fn)
}

func setEnabled(value bool) { enabled.Store(value) }
