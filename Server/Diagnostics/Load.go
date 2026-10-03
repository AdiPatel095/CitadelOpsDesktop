package Diagnostics

import (
	"context"
	"math"
	"runtime"
	"runtime/metrics"
	"sort"
	"sync"
	"time"
)

const (
	LoadSampleInterval = 10 * time.Second
	LoadWindowSamples  = 60
)

// LoadSnapshot describes recent process-wide load without account data.
type LoadSnapshot struct {
	CPUMeanShare     float64   `json:"cpuMeanShare"`
	CPUP95Share      float64   `json:"cpuP95Share"`
	CPUWindowSeconds float64   `json:"cpuWindowSeconds"`
	CPUSamples       int       `json:"cpuSamples"`
	CPUCores         int       `json:"cpuCores"`
	PostGCHeapBytes  uint64    `json:"postGcHeapBytes"`
	MemoryLimitBytes uint64    `json:"memoryLimitBytes"`
	ObservedAt       time.Time `json:"observedAt"`
}

type loadInterval struct {
	share, seconds float64
}

// LoadSampler keeps the newest CPU intervals independently of diagnostics reads.
// Its sources and state are protected by mu, including concurrent Sample calls.
type LoadSampler struct {
	mu        sync.Mutex
	now       func() time.Time
	cpu       func() (float64, bool)
	cores     func() int
	memory    func() (heap, limit uint64)
	previous  cpuCounters
	intervals []loadInterval
	latest    LoadSnapshot
}

func NewLoadSampler() *LoadSampler {
	return newLoadSampler(time.Now, processCPUSeconds, func() int { return runtime.GOMAXPROCS(0) }, func() (heap, limit uint64) {
		values := []metrics.Sample{{Name: "/gc/heap/live:bytes"}, {Name: "/gc/gomemlimit:bytes"}}
		metrics.Read(values)
		if values[0].Value.Kind() == metrics.KindUint64 {
			heap = values[0].Value.Uint64()
		}
		if values[1].Value.Kind() == metrics.KindUint64 {
			limit = values[1].Value.Uint64()
		}
		return heap, limit
	})
}

func newLoadSampler(now func() time.Time, cpu func() (float64, bool), cores func() int, memory func() (uint64, uint64)) *LoadSampler {
	return &LoadSampler{now: now, cpu: cpu, cores: cores, memory: memory, intervals: make([]loadInterval, 0, LoadWindowSamples)}
}

func (sampler *LoadSampler) Run(ctx context.Context) {
	sampler.Sample()
	ticker := time.NewTicker(LoadSampleInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			sampler.Sample()
		}
	}
}

func (sampler *LoadSampler) Sample() {
	sampler.mu.Lock()
	defer sampler.mu.Unlock()
	seconds, ok := sampler.cpu()
	if !ok {
		// A missing counter breaks the interval; the next read establishes a
		// fresh baseline instead of attributing several intervals to one sample.
		sampler.previous = cpuCounters{}
		return
	}
	current := cpuCounters{process: seconds, observedAt: sampler.now()}
	previous := sampler.previous
	sampler.previous = current
	if previous.observedAt.IsZero() {
		return
	}
	cores := sampler.cores()
	share, _, window := cpuShares(previous, current, cores)
	if window <= 0 {
		return
	}
	if len(sampler.intervals) == LoadWindowSamples {
		copy(sampler.intervals, sampler.intervals[1:])
		sampler.intervals = sampler.intervals[:LoadWindowSamples-1]
	}
	sampler.intervals = append(sampler.intervals, loadInterval{share: share, seconds: window})
	heap, limit := sampler.memory()
	if limit >= math.MaxInt64 {
		limit = 0
	}
	sampler.latest = LoadSnapshot{CPUCores: cores, PostGCHeapBytes: heap, MemoryLimitBytes: limit, ObservedAt: current.observedAt.UTC()}
}

func (sampler *LoadSampler) Snapshot() (LoadSnapshot, bool) {
	sampler.mu.Lock()
	defer sampler.mu.Unlock()
	if len(sampler.intervals) == 0 {
		return LoadSnapshot{}, false
	}
	snapshot := sampler.latest
	shares := make([]float64, len(sampler.intervals))
	for i, interval := range sampler.intervals {
		shares[i] = interval.share
		snapshot.CPUWindowSeconds += interval.seconds
	}
	snapshot.CPUSamples = len(shares)
	snapshot.CPUMeanShare, snapshot.CPUP95Share = summarizeShares(shares)
	return snapshot, true
}

func summarizeShares(shares []float64) (mean, p95 float64) {
	if len(shares) == 0 {
		return 0, 0
	}
	for _, share := range shares {
		mean += share
	}
	mean /= float64(len(shares))
	ordered := append([]float64(nil), shares...)
	sort.Float64s(ordered)
	p95 = ordered[int(math.Ceil(0.95*float64(len(ordered))))-1]
	return mean, p95
}
