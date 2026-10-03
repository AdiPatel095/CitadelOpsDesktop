package Diagnostics

import (
	"runtime"
	"runtime/metrics"
	"sync"
	"time"
)

// ProcessSnapshot is process-wide, including all accounts and shared stores.
// It does not attribute allocations to an account or force a GC cycle.
type ProcessSnapshot struct {
	CompletedGCCycles uint64    `json:"completedGcCycles"`
	Scope             string    `json:"scope"`
	ObservedAt        time.Time `json:"observedAt"`
	HeapAlloc         uint64    `json:"heapAlloc"`
	HeapInuse         uint64    `json:"heapInuse"`
	HeapSys           uint64    `json:"heapSys"`
	HeapIdle          uint64    `json:"heapIdle"`
	HeapReleased      uint64    `json:"heapReleased"`
	StackInuse        uint64    `json:"stackInuse"`
	StackSys          uint64    `json:"stackSys"`
	Sys               uint64    `json:"sys"`
	HeapObjects       uint64    `json:"heapObjects"`
	PostGCLiveHeap    uint64    `json:"postGcLiveHeap"`
	NextGC            uint64    `json:"nextGc"`
	NumGC             uint32    `json:"numGc"`
	LastGC            uint64    `json:"lastGcUnixNano"`
	GCPauseTotalNS    uint64    `json:"gcPauseTotalNs"`
	GCTotalCPUSeconds float64   `json:"gcTotalCpuSeconds"`
	// ProcessCPUSeconds is the cumulative user+system CPU time of the whole
	// process. CPUCores is the CPU capacity it is measured against (GOMAXPROCS,
	// which follows the container CPU limit). CPUShare is the process CPU time
	// used over the last CPUWindowSeconds as a fraction of CPUCores of wall time;
	// GCCPUShare is the garbage collector's part of it. Both use the same window
	// and denominator so they compare directly. The first snapshot covers the
	// whole process lifetime and reports CPUWindowSeconds 0.
	ProcessCPUSeconds float64 `json:"processCpuSeconds"`
	CPUCores          int     `json:"cpuCores"`
	CPUShare          float64 `json:"cpuShare"`
	GCCPUShare        float64 `json:"gcCpuShare"`
	CPUWindowSeconds  float64 `json:"cpuWindowSeconds"`
	GoMemoryLimit     uint64  `json:"goMemoryLimit"`
	GoGCPercent       uint64  `json:"goGcPercent"`
}

var processCache struct {
	sync.Mutex
	snapshot ProcessSnapshot
	previous cpuCounters
}

// processStart anchors the first snapshot's CPU window to the process lifetime.
var processStart = time.Now()

// cpuCounters are the cumulative CPU values at one sample.
type cpuCounters struct {
	process, gc float64
	observedAt  time.Time
}

// cpuShares reports the process and GC CPU use between two samples as fractions
// of cores of wall time, and the window length in seconds.
func cpuShares(previous cpuCounters, current cpuCounters, cores int) (share float64, gc float64, window float64) {
	wall := current.observedAt.Sub(previous.observedAt).Seconds()
	if wall <= 0 || cores < 1 {
		return 0, 0, 0
	}
	share = nonNegative((current.process - previous.process) / (wall * float64(cores)))
	gc = nonNegative((current.gc - previous.gc) / (wall * float64(cores)))
	return share, gc, wall
}

func nonNegative(value float64) float64 {
	if value < 0 {
		return 0
	}
	return value
}

func SampleProcess() ProcessSnapshot {
	processCache.Lock()
	defer processCache.Unlock()
	if time.Since(processCache.snapshot.ObservedAt) < sampleInterval {
		return processCache.snapshot
	}
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	values := []metrics.Sample{{Name: "/gc/heap/live:bytes"}, {Name: "/gc/gomemlimit:bytes"}, {Name: "/gc/gogc:percent"}, {Name: "/cpu/classes/gc/total:cpu-seconds"}, {Name: "/gc/cycles/total:gc-cycles"}}
	metrics.Read(values)
	next := ProcessSnapshot{Scope: "process", ObservedAt: time.Now().UTC(), HeapAlloc: m.HeapAlloc, HeapInuse: m.HeapInuse, HeapSys: m.HeapSys, HeapIdle: m.HeapIdle, HeapReleased: m.HeapReleased, StackInuse: m.StackInuse, StackSys: m.StackSys, Sys: m.Sys, HeapObjects: m.HeapObjects, NextGC: m.NextGC, NumGC: m.NumGC, LastGC: m.LastGC, GCPauseTotalNS: m.PauseTotalNs}
	if values[0].Value.Kind() == metrics.KindUint64 {
		next.PostGCLiveHeap = values[0].Value.Uint64()
	}
	if values[1].Value.Kind() == metrics.KindUint64 {
		next.GoMemoryLimit = values[1].Value.Uint64()
	}
	if values[2].Value.Kind() == metrics.KindUint64 {
		next.GoGCPercent = values[2].Value.Uint64()
	}
	if values[3].Value.Kind() == metrics.KindFloat64 {
		next.GCTotalCPUSeconds = values[3].Value.Float64()
	}
	if values[4].Value.Kind() == metrics.KindUint64 {
		next.CompletedGCCycles = values[4].Value.Uint64()
	}
	current := cpuCounters{observedAt: next.ObservedAt, gc: next.GCTotalCPUSeconds}
	if seconds, ok := processCPUSeconds(); ok {
		current.process = seconds
	}
	next.ProcessCPUSeconds = current.process
	next.CPUCores = runtime.GOMAXPROCS(0)
	previous := processCache.previous
	firstWindow := previous.observedAt.IsZero()
	if firstWindow {
		previous.observedAt = processStart.UTC()
	}
	next.CPUShare, next.GCCPUShare, next.CPUWindowSeconds = cpuShares(previous, current, next.CPUCores)
	if firstWindow {
		next.CPUWindowSeconds = 0
	}
	processCache.previous = current
	processCache.snapshot = next
	return next
}
