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
	GoMemoryLimit     uint64    `json:"goMemoryLimit"`
	GoGCPercent       uint64    `json:"goGcPercent"`
}

var processCache struct {
	sync.Mutex
	snapshot ProcessSnapshot
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
	processCache.snapshot = next
	return next
}
