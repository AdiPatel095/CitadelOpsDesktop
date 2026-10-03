package Profiling

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/pprof"
	"net/netip"
	"runtime"
	"runtime/metrics"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	// mutexProfileFraction samples 1 in N contended mutex events while the listener is on.
	mutexProfileFraction = 100
	// blockProfileRateNS samples blocking events of about this many nanoseconds (1 ms) while the listener is on.
	blockProfileRateNS = 1_000_000
)

// LoopbackAddress validates a listen address. Only a loopback IP literal or the
// name "localhost" is accepted; an empty host, an unspecified address and any
// routable address are refused, because they would expose the profiler.
func LoopbackAddress(address string) (string, error) {
	address = strings.TrimSpace(address)
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return "", fmt.Errorf("%s %q must be host:port with a loopback host, for example 127.0.0.1:6060: %w", EnvAddr, address, err)
	}
	if number, portErr := strconv.Atoi(port); portErr != nil || number < 0 || number > 65535 {
		return "", fmt.Errorf("%s %q has an invalid port %q", EnvAddr, address, port)
	}
	if strings.EqualFold(host, "localhost") {
		return net.JoinHostPort("127.0.0.1", port), nil
	}
	ip, err := netip.ParseAddr(host)
	if err != nil {
		return "", fmt.Errorf("%s %q must use a loopback IP address or localhost, got host %q", EnvAddr, address, host)
	}
	if !ip.Unmap().IsLoopback() {
		return "", fmt.Errorf("%s %q is not a loopback address; profiling is reachable on loopback only", EnvAddr, address)
	}
	return net.JoinHostPort(ip.Unmap().String(), port), nil
}

// Listener is a running profiling server.
type Listener struct {
	server   *http.Server
	listener net.Listener
	done     chan struct{}
	stopOnce sync.Once
}

// Addr returns the bound address.
func (l *Listener) Addr() net.Addr { return l.listener.Addr() }

// Start binds the loopback listener and enables labels plus mutex/block
// sampling. An empty address returns (nil, nil): profiling stays off. A
// non-loopback address returns an error and starts nothing. The listener stops
// with ctx or Stop.
func Start(ctx context.Context, address string, logf func(format string, args ...any)) (*Listener, error) {
	if strings.TrimSpace(address) == "" {
		return nil, nil
	}
	if logf == nil {
		logf = func(string, ...any) {}
	}
	bind, err := LoopbackAddress(address)
	if err != nil {
		return nil, err
	}
	listener, err := net.Listen("tcp", bind)
	if err != nil {
		return nil, fmt.Errorf("listen for profiling on %s: %w", bind, err)
	}
	if tcp, ok := listener.Addr().(*net.TCPAddr); !ok || !tcp.IP.IsLoopback() {
		_ = listener.Close()
		return nil, fmt.Errorf("profiling listener bound to %s, which is not a loopback address", listener.Addr())
	}
	setEnabled(true)
	runtime.SetMutexProfileFraction(mutexProfileFraction)
	runtime.SetBlockProfileRate(blockProfileRateNS)
	l := &Listener{
		listener: listener,
		done:     make(chan struct{}),
		server: &http.Server{
			Handler:           Handler(),
			ReadHeaderTimeout: 5 * time.Second,
			IdleTimeout:       60 * time.Second,
			MaxHeaderBytes:    16 << 10,
			// A CPU profile or trace streams for its whole duration (?seconds=120).
			WriteTimeout: 0,
		},
	}
	go func() {
		defer close(l.done)
		if serveErr := l.server.Serve(listener); serveErr != nil && !errors.Is(serveErr, http.ErrServerClosed) {
			logf("profiling listener stopped: %v", serveErr)
		}
	}()
	go func() {
		select {
		case <-ctx.Done():
			l.Stop()
		case <-l.done:
		}
	}()
	logf("profiling listener enabled on %s (loopback only)", listener.Addr())
	return l, nil
}

// Stop closes the listener and turns labels and sampling back off.
func (l *Listener) Stop() {
	if l == nil {
		return
	}
	l.stopOnce.Do(func() {
		shutdown, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		if err := l.server.Shutdown(shutdown); err != nil {
			_ = l.server.Close()
		}
		<-l.done
		setEnabled(false)
		runtime.SetMutexProfileFraction(0)
		runtime.SetBlockProfileRate(0)
	})
}

// Handler is the profiling mux: its own mux, never the application's.
func Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/debug/pprof/", pprof.Index)
	mux.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
	mux.HandleFunc("/debug/pprof/profile", pprof.Profile)
	mux.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
	mux.HandleFunc("/debug/pprof/trace", pprof.Trace)
	mux.HandleFunc("/debug/runtime/metrics", serveRuntimeMetrics)
	return mux
}

// RuntimeMetrics is the JSON shape of /debug/runtime/metrics.
type RuntimeMetrics struct {
	ObservedAt          time.Time        `json:"observedAt"`
	GCCPUSeconds        float64          `json:"gcCpuSeconds"`
	LiveHeapBytes       uint64           `json:"liveHeapBytes"`
	SchedLatencySeconds HistogramSummary `json:"schedLatencySeconds"`
}

// HistogramSummary summarises a runtime/metrics histogram: total count and
// quantile estimates, each the upper bound of the bucket holding the quantile.
type HistogramSummary struct {
	Count uint64  `json:"count"`
	P50   float64 `json:"p50"`
	P90   float64 `json:"p90"`
	P99   float64 `json:"p99"`
	Max   float64 `json:"max"`
}

// ReadRuntimeMetrics samples /cpu/classes/gc/total:cpu-seconds,
// /gc/heap/live:bytes and /sched/latencies:seconds.
func ReadRuntimeMetrics() RuntimeMetrics {
	samples := []metrics.Sample{
		{Name: "/cpu/classes/gc/total:cpu-seconds"},
		{Name: "/gc/heap/live:bytes"},
		{Name: "/sched/latencies:seconds"},
	}
	metrics.Read(samples)
	result := RuntimeMetrics{ObservedAt: time.Now().UTC()}
	if samples[0].Value.Kind() == metrics.KindFloat64 {
		result.GCCPUSeconds = samples[0].Value.Float64()
	}
	if samples[1].Value.Kind() == metrics.KindUint64 {
		result.LiveHeapBytes = samples[1].Value.Uint64()
	}
	if samples[2].Value.Kind() == metrics.KindFloat64Histogram {
		result.SchedLatencySeconds = summarizeHistogram(samples[2].Value.Float64Histogram())
	}
	return result
}

func summarizeHistogram(histogram *metrics.Float64Histogram) HistogramSummary {
	var summary HistogramSummary
	if histogram == nil || len(histogram.Buckets) != len(histogram.Counts)+1 {
		return summary
	}
	for _, count := range histogram.Counts {
		summary.Count += count
	}
	if summary.Count == 0 {
		return summary
	}
	upper := func(bucket int) float64 {
		bound := histogram.Buckets[bucket+1]
		if bound > 1e18 { // +Inf top bucket: report its lower bound
			return histogram.Buckets[bucket]
		}
		return bound
	}
	targets := []struct {
		quantile float64
		into     *float64
	}{{0.5, &summary.P50}, {0.9, &summary.P90}, {0.99, &summary.P99}}
	sort.Slice(targets, func(i, j int) bool { return targets[i].quantile < targets[j].quantile })
	var cumulative uint64
	next := 0
	for bucket, count := range histogram.Counts {
		if count == 0 {
			continue
		}
		cumulative += count
		for next < len(targets) && float64(cumulative) >= targets[next].quantile*float64(summary.Count) {
			*targets[next].into = upper(bucket)
			next++
		}
		summary.Max = upper(bucket)
	}
	return summary
}

func serveRuntimeMetrics(writer http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodGet && request.Method != http.MethodHead {
		http.Error(writer, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	writer.Header().Set("Content-Type", "application/json; charset=utf-8")
	writer.Header().Set("Cache-Control", "no-store")
	encoder := json.NewEncoder(writer)
	encoder.SetIndent("", "  ")
	_ = encoder.Encode(ReadRuntimeMetrics())
}
