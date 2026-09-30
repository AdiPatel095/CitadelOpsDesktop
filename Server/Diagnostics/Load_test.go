package Diagnostics

import (
	"context"
	"encoding/json"
	"math"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestSummarizeSharesMeanAndNearestRankP95(t *testing.T) {
	sequence := func(n int) []float64 {
		values := make([]float64, n)
		for i := range values {
			values[i] = float64(n - i)
		}
		return values
	}
	for _, test := range []struct {
		name      string
		shares    []float64
		mean, p95 float64
	}{
		{"empty", nil, 0, 0},
		{"one", []float64{0.42}, 0.42, 0.42},
		{"twenty", sequence(20), 10.5, 19},
		{"sixty", sequence(60), 30.5, 57},
		{"ties", []float64{0.8, 0.2, 0.8, 0.2}, 0.5, 0.8},
	} {
		t.Run(test.name, func(t *testing.T) {
			mean, p95 := summarizeShares(test.shares)
			if !closeTo(mean, test.mean) || !closeTo(p95, test.p95) {
				t.Fatalf("mean/p95 = %v/%v, want %v/%v", mean, p95, test.mean, test.p95)
			}
		})
	}
}

type loadFixture struct {
	now         time.Time
	cpu         float64
	ok          bool
	heap, limit uint64
	sampler     *LoadSampler
}

func newLoadFixture() *loadFixture {
	fixture := &loadFixture{now: time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC), ok: true, heap: 123, limit: 456}
	fixture.sampler = newLoadSampler(func() time.Time { return fixture.now }, func() (float64, bool) { return fixture.cpu, fixture.ok }, func() int { return 2 }, func() (uint64, uint64) { return fixture.heap, fixture.limit })
	return fixture
}

func (fixture *loadFixture) advance(share float64) {
	fixture.now = fixture.now.Add(LoadSampleInterval)
	fixture.cpu += share * 2 * LoadSampleInterval.Seconds()
	fixture.sampler.Sample()
}

func TestLoadSamplerKeepsTheNewestSixtyIntervals(t *testing.T) {
	fixture := newLoadFixture()
	fixture.sampler.Sample()
	for i := 1; i <= 70; i++ {
		fixture.advance(float64(i) / 100)
	}
	snapshot, ok := fixture.sampler.Snapshot()
	if !ok || snapshot.CPUSamples != 60 || snapshot.CPUWindowSeconds != 600 || !closeTo(snapshot.CPUMeanShare, 0.405) || !closeTo(snapshot.CPUP95Share, 0.67) || snapshot.ObservedAt != fixture.now {
		t.Fatalf("newest sixty intervals = %+v, available %t", snapshot, ok)
	}
}

func TestLoadSamplerIsUnavailableUntilOneInterval(t *testing.T) {
	fixture := newLoadFixture()
	if _, ok := fixture.sampler.Snapshot(); ok {
		t.Fatal("available before baseline")
	}
	fixture.sampler.Sample()
	if _, ok := fixture.sampler.Snapshot(); ok {
		t.Fatal("available after only baseline")
	}
	fixture.sampler.Sample() // Zero wall time must not add an interval.
	if _, ok := fixture.sampler.Snapshot(); ok {
		t.Fatal("available with zero wall time")
	}
	fixture.advance(0.5)
	if snapshot, ok := fixture.sampler.Snapshot(); !ok || snapshot.CPUSamples != 1 || snapshot.CPUMeanShare != 0.5 || snapshot.CPUCores != 2 {
		t.Fatalf("first interval = %+v, available %t", snapshot, ok)
	}
}

func TestLoadSamplerSkipsIntervalsWithoutACPUReading(t *testing.T) {
	fixture := newLoadFixture()
	fixture.ok = false
	fixture.advance(0.5)
	if _, ok := fixture.sampler.Snapshot(); ok {
		t.Fatal("failed initial reading produced a sample")
	}
	fixture.ok = true
	fixture.sampler.Sample()
	fixture.advance(0.5)
	before, _ := fixture.sampler.Snapshot()
	fixture.ok = false
	fixture.advance(1)
	after, _ := fixture.sampler.Snapshot()
	if before != after {
		t.Fatalf("failed reading changed snapshot: %+v -> %+v", before, after)
	}
	fixture.ok = true
	fixture.advance(1) // Re-establish the baseline after the missing reading.
	fixture.advance(0.25)
	snapshot, _ := fixture.sampler.Snapshot()
	if snapshot.CPUSamples != 2 || snapshot.CPUWindowSeconds != 20 || snapshot.CPUMeanShare != 0.375 {
		t.Fatalf("failed interval was included: %+v", snapshot)
	}
}

func TestLoadSamplerReportsHeapAndAnUnsetLimitAsZero(t *testing.T) {
	for _, limit := range []uint64{456, math.MaxInt64, math.MaxUint64} {
		fixture := newLoadFixture()
		fixture.sampler.Sample()
		fixture.heap, fixture.limit = 789, limit
		fixture.advance(0.5)
		snapshot, _ := fixture.sampler.Snapshot()
		wantLimit := limit
		if limit >= math.MaxInt64 {
			wantLimit = 0
		}
		if snapshot.PostGCHeapBytes != 789 || snapshot.MemoryLimitBytes != wantLimit {
			t.Fatalf("heap/limit = %d/%d, want 789/%d", snapshot.PostGCHeapBytes, snapshot.MemoryLimitBytes, wantLimit)
		}
	}
}

func TestLoadSnapshotJSONFields(t *testing.T) {
	data, err := json.Marshal(LoadSnapshot{})
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		t.Fatal(err)
	}
	keys := []string{"cpuMeanShare", "cpuP95Share", "cpuWindowSeconds", "cpuSamples", "cpuCores", "postGcHeapBytes", "memoryLimitBytes", "observedAt"}
	if len(fields) != len(keys) {
		t.Fatalf("unexpected fields: %s", data)
	}
	for _, key := range keys {
		if _, ok := fields[key]; !ok {
			t.Errorf("missing field %q", key)
		}
	}
	for _, forbidden := range []string{"account", "payload", "profile", "path", "token"} {
		if strings.Contains(strings.ToLower(string(data)), forbidden) {
			t.Fatalf("private data key %q", forbidden)
		}
	}
}

func TestLoadSamplerConcurrentSamplingAndSnapshots(t *testing.T) {
	now := time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC)
	var cpu float64
	sampler := newLoadSampler(func() time.Time { now = now.Add(LoadSampleInterval); return now }, func() (float64, bool) { cpu++; return cpu, true }, func() int { return 1 }, func() (uint64, uint64) { return 1, 2 })
	var workers sync.WaitGroup
	for range 4 {
		workers.Go(func() {
			for range 100 {
				sampler.Sample()
				sampler.Snapshot()
			}
		})
	}
	workers.Wait()
	if snapshot, ok := sampler.Snapshot(); !ok || snapshot.CPUSamples != 60 || snapshot.CPUWindowSeconds != 600 {
		t.Fatalf("concurrent snapshot = %+v, available %t", snapshot, ok)
	}
}

func TestLoadSamplerRunSamplesImmediatelyAndStops(t *testing.T) {
	fixture := newLoadFixture()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	fixture.sampler.Run(ctx)
	fixture.advance(0.5)
	if snapshot, ok := fixture.sampler.Snapshot(); !ok || snapshot.CPUSamples != 1 {
		t.Fatalf("Run did not establish baseline: %+v, available %t", snapshot, ok)
	}
}
