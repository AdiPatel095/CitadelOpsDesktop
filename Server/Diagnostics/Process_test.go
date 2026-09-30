package Diagnostics

import (
	"encoding/json"
	"math"
	"strings"
	"testing"
	"time"
)

func TestProcessSampleCachedAndAggregate(t *testing.T) {
	a, b := SampleProcess(), SampleProcess()
	if a != b || a.Scope != "process" || a.ObservedAt.IsZero() || a.GoMemoryLimit == 0 || a.HeapSys < a.HeapInuse {
		t.Fatalf("invalid process snapshot: %+v", a)
	}
	data, err := json.Marshal(a)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"account", "payload", "profile", "path", "token"} {
		if strings.Contains(strings.ToLower(string(data)), forbidden) {
			t.Fatal("private label present")
		}
	}
}

func TestCPUSharesAreFractionsOfCoresOfWallTime(t *testing.T) {
	start := time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC)
	previous := cpuCounters{process: 100, gc: 4, observedAt: start}
	tests := []struct {
		name            string
		current         cpuCounters
		cores           int
		share, gc, wall float64
	}{
		{name: "half of one core", current: cpuCounters{process: 105, gc: 4.5, observedAt: start.Add(10 * time.Second)}, cores: 1, share: 0.5, gc: 0.05, wall: 10},
		{name: "same work on two cores", current: cpuCounters{process: 105, gc: 4.5, observedAt: start.Add(10 * time.Second)}, cores: 2, share: 0.25, gc: 0.025, wall: 10},
		{name: "idle", current: cpuCounters{process: 100, gc: 4, observedAt: start.Add(10 * time.Second)}, cores: 1, share: 0, gc: 0, wall: 10},
		{name: "overcommitted is not clamped", current: cpuCounters{process: 130, gc: 4, observedAt: start.Add(10 * time.Second)}, cores: 2, share: 1.5, gc: 0, wall: 10},
		{name: "counter going backwards is zero", current: cpuCounters{process: 99, gc: 3, observedAt: start.Add(10 * time.Second)}, cores: 1, share: 0, gc: 0, wall: 10},
		{name: "no elapsed time", current: cpuCounters{process: 200, gc: 9, observedAt: start}, cores: 1},
		{name: "no cores", current: cpuCounters{process: 200, gc: 9, observedAt: start.Add(time.Second)}, cores: 0},
	}
	for _, test := range tests {
		share, gc, wall := cpuShares(previous, test.current, test.cores)
		if !closeTo(share, test.share) || !closeTo(gc, test.gc) || !closeTo(wall, test.wall) {
			t.Errorf("%s: cpuShares = share %.4f gc %.4f wall %.1f; want %.4f %.4f %.1f", test.name, share, gc, wall, test.share, test.gc, test.wall)
		}
	}
}

func closeTo(got float64, want float64) bool { return math.Abs(got-want) < 1e-9 }

func TestProcessSampleReportsCPUUseOverConsecutiveWindows(t *testing.T) {
	processCache.Lock()
	processCache.snapshot = ProcessSnapshot{}
	processCache.previous = cpuCounters{}
	processCache.Unlock()
	first := SampleProcess()
	if first.ProcessCPUSeconds <= 0 || first.CPUCores < 1 || first.CPUShare < 0 || first.CPUWindowSeconds != 0 {
		t.Fatalf("first snapshot = %+v, want lifetime CPU use and no window", first)
	}
	// Expire the cache, burn CPU on this goroutine, and sample the next window.
	processCache.Lock()
	processCache.snapshot.ObservedAt = time.Now().Add(-time.Minute)
	processCache.Unlock()
	deadline := time.Now().Add(200 * time.Millisecond)
	for time.Now().Before(deadline) {
	}
	second := SampleProcess()
	if second.CPUWindowSeconds <= 0 || second.ProcessCPUSeconds <= first.ProcessCPUSeconds {
		t.Fatalf("second snapshot = %+v (first %+v): CPU seconds must advance over a busy window", second, first)
	}
	// A busy loop uses one core: about 1/CPUCores of the machine, and never more than the window allows.
	if second.CPUShare <= 0.2/float64(second.CPUCores) || second.CPUShare > 1.5 {
		t.Fatalf("CPU share %.3f over a %.3fs busy window on %d cores", second.CPUShare, second.CPUWindowSeconds, second.CPUCores)
	}
	data, err := json.Marshal(second)
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{`"processCpuSeconds"`, `"cpuCores"`, `"cpuShare"`, `"gcCpuShare"`, `"cpuWindowSeconds"`, `"postGcLiveHeap"`, `"gcTotalCpuSeconds"`} {
		if !strings.Contains(string(data), key) {
			t.Errorf("process snapshot JSON misses %s: %s", key, data)
		}
	}
}
