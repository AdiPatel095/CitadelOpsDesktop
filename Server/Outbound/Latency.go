package Outbound

import (
	"math"
	"sort"
	"sync"
	"time"
)

const (
	defaultDispatchLatencyWindow = 512
	DispatchLatencyTarget        = 25 * time.Millisecond
)

type LatencyDistribution struct {
	Count               int     `json:"count"`
	LastMilliseconds    float64 `json:"lastMilliseconds"`
	P50Milliseconds     float64 `json:"p50Milliseconds"`
	P95Milliseconds     float64 `json:"p95Milliseconds"`
	P99Milliseconds     float64 `json:"p99Milliseconds"`
	MaximumMilliseconds float64 `json:"maximumMilliseconds"`
}

type DispatchLatencyProfile struct {
	Count             int                 `json:"count"`
	IntentToDispatch  LatencyDistribution `json:"intentToDispatch"`
	Queue             LatencyDistribution `json:"queue"`
	Transport         LatencyDistribution `json:"transport"`
	IntentToTransport LatencyDistribution `json:"intentToTransport"`
}

type DispatchLatencyStats struct {
	TargetMilliseconds float64                `json:"targetMilliseconds"`
	WindowSize         int                    `json:"windowSize"`
	UpdatedAt          *time.Time             `json:"updatedAt,omitempty"`
	All                DispatchLatencyProfile `json:"all"`
	FastPath           DispatchLatencyProfile `json:"fastPath"`
	FastPathTargetMet  bool                   `json:"fastPathTargetMet"`
}

type dispatchLatencySample struct {
	observedAt        time.Time
	intentOrigin      bool
	fastPath          bool
	intentToDispatch  time.Duration
	queue             time.Duration
	transport         time.Duration
	intentToTransport time.Duration
}

type dispatchLatencyWindow struct {
	mu        sync.Mutex
	capacity  int
	samples   []dispatchLatencySample
	next      int
	full      bool
	updatedAt time.Time
}

func newDispatchLatencyWindow(capacity int) *dispatchLatencyWindow {
	if capacity < 1 {
		capacity = defaultDispatchLatencyWindow
	}
	return &dispatchLatencyWindow{capacity: capacity, samples: make([]dispatchLatencySample, 0, capacity)}
}

func (window *dispatchLatencyWindow) record(sample dispatchLatencySample) {
	window.mu.Lock()
	defer window.mu.Unlock()
	window.updatedAt = sample.observedAt
	if len(window.samples) < window.capacity {
		window.samples = append(window.samples, sample)
		if len(window.samples) == window.capacity {
			window.full = true
			window.next = 0
		}
		return
	}
	window.samples[window.next] = sample
	window.next = (window.next + 1) % window.capacity
}

func (window *dispatchLatencyWindow) snapshot() DispatchLatencyStats {
	window.mu.Lock()
	samples := make([]dispatchLatencySample, 0, len(window.samples))
	if window.full {
		samples = append(samples, window.samples[window.next:]...)
		samples = append(samples, window.samples[:window.next]...)
	} else {
		samples = append(samples, window.samples...)
	}
	updatedAt := window.updatedAt
	capacity := window.capacity
	window.mu.Unlock()

	fastPathSamples := make([]dispatchLatencySample, 0, len(samples))
	for _, sample := range samples {
		if sample.fastPath {
			fastPathSamples = append(fastPathSamples, sample)
		}
	}
	stats := DispatchLatencyStats{
		TargetMilliseconds: durationMilliseconds(DispatchLatencyTarget),
		WindowSize:         capacity,
		All:                latencyProfile(samples),
		FastPath:           latencyProfile(fastPathSamples),
	}
	if !updatedAt.IsZero() {
		stats.UpdatedAt = &updatedAt
	}
	stats.FastPathTargetMet = stats.FastPath.IntentToTransport.Count > 0 &&
		stats.FastPath.IntentToTransport.P95Milliseconds <= stats.TargetMilliseconds
	return stats
}

func emptyDispatchLatencyStats() DispatchLatencyStats {
	return DispatchLatencyStats{
		TargetMilliseconds: durationMilliseconds(DispatchLatencyTarget),
		WindowSize:         defaultDispatchLatencyWindow,
	}
}

func latencyProfile(samples []dispatchLatencySample) DispatchLatencyProfile {
	intentToDispatch := make([]time.Duration, 0, len(samples))
	queue := make([]time.Duration, 0, len(samples))
	transport := make([]time.Duration, 0, len(samples))
	intentToTransport := make([]time.Duration, 0, len(samples))
	for _, sample := range samples {
		queue = append(queue, sample.queue)
		transport = append(transport, sample.transport)
		if sample.intentOrigin {
			intentToDispatch = append(intentToDispatch, sample.intentToDispatch)
			intentToTransport = append(intentToTransport, sample.intentToTransport)
		}
	}
	return DispatchLatencyProfile{
		Count:             len(samples),
		IntentToDispatch:  latencyDistribution(intentToDispatch),
		Queue:             latencyDistribution(queue),
		Transport:         latencyDistribution(transport),
		IntentToTransport: latencyDistribution(intentToTransport),
	}
}

func latencyDistribution(values []time.Duration) LatencyDistribution {
	if len(values) == 0 {
		return LatencyDistribution{}
	}
	sorted := append([]time.Duration(nil), values...)
	sort.Slice(sorted, func(left, right int) bool { return sorted[left] < sorted[right] })
	return LatencyDistribution{
		Count:               len(values),
		LastMilliseconds:    durationMilliseconds(values[len(values)-1]),
		P50Milliseconds:     durationMilliseconds(nearestRank(sorted, 0.50)),
		P95Milliseconds:     durationMilliseconds(nearestRank(sorted, 0.95)),
		P99Milliseconds:     durationMilliseconds(nearestRank(sorted, 0.99)),
		MaximumMilliseconds: durationMilliseconds(sorted[len(sorted)-1]),
	}
}

func nearestRank(sorted []time.Duration, percentile float64) time.Duration {
	index := int(math.Ceil(percentile*float64(len(sorted)))) - 1
	if index < 0 {
		index = 0
	}
	if index >= len(sorted) {
		index = len(sorted) - 1
	}
	return sorted[index]
}

func durationMilliseconds(duration time.Duration) float64 {
	return math.Round(float64(duration)/float64(time.Microsecond)) / 1000
}

func nonNegativeDuration(duration time.Duration) time.Duration {
	if duration < 0 {
		return 0
	}
	return duration
}
