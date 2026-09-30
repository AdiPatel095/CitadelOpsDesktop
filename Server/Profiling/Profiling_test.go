package Profiling

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"runtime"
	"runtime/metrics"
	"runtime/pprof"
	"strings"
	"testing"
	"time"
)

func TestLoopbackAddress(t *testing.T) {
	accepted := map[string]string{
		"127.0.0.1:6060":          "127.0.0.1:6060",
		"  127.0.0.1:6060 ":       "127.0.0.1:6060",
		"localhost:6060":          "127.0.0.1:6060",
		"LOCALHOST:0":             "127.0.0.1:0",
		"127.0.0.2:1":             "127.0.0.2:1",
		"[::1]:6060":              "[::1]:6060",
		"[::ffff:127.0.0.1]:6060": "127.0.0.1:6060",
	}
	for input, want := range accepted {
		if got, err := LoopbackAddress(input); err != nil || got != want {
			t.Errorf("LoopbackAddress(%q) = %q, %v; want %q", input, got, err, want)
		}
	}
	for _, input := range []string{
		"", ":6060", "0.0.0.0:6060", "[::]:6060", "10.0.0.5:6060", "192.168.1.10:6060", "8.8.8.8:6060",
		"[2001:db8::1]:6060", "example.com:6060", "citadelops.app:80", "127.0.0.1", "6060",
		"127.0.0.1:99999", "127.0.0.1:-1", "127.0.0.1:http", "127.0.0.1.evil.example:6060",
	} {
		if got, err := LoopbackAddress(input); err == nil {
			t.Errorf("LoopbackAddress(%q) = %q, want an error", input, got)
		}
	}
}

func TestStartWithoutAddressLeavesProfilingOff(t *testing.T) {
	mutexBefore := runtime.SetMutexProfileFraction(-1)
	for _, address := range []string{"", "   "} {
		listener, err := Start(context.Background(), address, t.Logf)
		if listener != nil || err != nil {
			t.Fatalf("Start(%q) = %v, %v; want no listener and no error", address, listener, err)
		}
	}
	if Enabled() {
		t.Fatal("profiler labels are enabled without an address")
	}
	if got := runtime.SetMutexProfileFraction(-1); got != mutexBefore {
		t.Fatalf("mutex profile fraction changed from %d to %d without an address", mutexBefore, got)
	}
	ctx := context.Background()
	if WithRuntime(ctx, "r1") != ctx {
		t.Fatal("WithRuntime changed the context while profiling is off")
	}
	Do(ctx, func(inner context.Context) {
		if _, found := pprof.Label(inner, LabelStage); found {
			t.Fatal("Do applied a label while profiling is off")
		}
	}, LabelStage, StageIngest)
}

func TestStartRefusesNonLoopbackAndStartsNothing(t *testing.T) {
	for _, address := range []string{"0.0.0.0:0", ":0", "[::]:0", "10.1.2.3:6060"} {
		var logged []string
		listener, err := Start(context.Background(), address, func(format string, args ...any) { logged = append(logged, format) })
		if listener != nil || err == nil {
			t.Fatalf("Start(%q) = %v, %v; want a refusal", address, listener, err)
		}
		if !strings.Contains(err.Error(), EnvAddr) {
			t.Fatalf("refusal for %q does not say why: %v", address, err)
		}
		if Enabled() {
			t.Fatalf("profiler labels enabled after refusing %q", address)
		}
	}
}

func TestListenerServesOnlyProfilingRoutesOnLoopbackAndStops(t *testing.T) {
	mutexBefore := runtime.SetMutexProfileFraction(-1)
	listener, err := Start(context.Background(), "127.0.0.1:0", t.Logf)
	if err != nil || listener == nil {
		t.Fatalf("Start: %v, %v", listener, err)
	}
	stopped := false
	t.Cleanup(func() {
		if !stopped {
			listener.Stop()
		}
	})
	tcp, ok := listener.Addr().(*net.TCPAddr)
	if !ok || !tcp.IP.IsLoopback() {
		t.Fatalf("listener bound to %v, want loopback", listener.Addr())
	}
	if !Enabled() {
		t.Fatal("labels are not enabled with the listener on")
	}
	if got := runtime.SetMutexProfileFraction(-1); got != mutexProfileFraction {
		t.Fatalf("mutex profile fraction = %d with the listener on, want %d", got, mutexProfileFraction)
	}
	if listener.server.ReadHeaderTimeout <= 0 {
		t.Fatal("profiling server has no ReadHeaderTimeout")
	}
	base := "http://" + listener.Addr().String()
	get := func(path string) (int, []byte) {
		t.Helper()
		response, err := http.Get(base + path)
		if err != nil {
			t.Fatalf("GET %s: %v", path, err)
		}
		defer response.Body.Close()
		body, _ := io.ReadAll(response.Body)
		return response.StatusCode, body
	}
	for _, path := range []string{"/debug/pprof/", "/debug/pprof/cmdline", "/debug/pprof/goroutine?debug=1", "/debug/pprof/heap?debug=1", "/debug/pprof/mutex?debug=1", "/debug/pprof/block?debug=1"} {
		if status, _ := get(path); status != http.StatusOK {
			t.Errorf("GET %s = %d, want 200", path, status)
		}
	}
	if status, body := get("/debug/pprof/profile?seconds=1"); status != http.StatusOK || len(body) < 2 || body[0] != 0x1f || body[1] != 0x8b {
		t.Errorf("CPU profile = %d, %d bytes, want a gzip-compressed profile", status, len(body))
	}
	if status, body := get("/debug/pprof/trace?seconds=1"); status != http.StatusOK || len(body) == 0 {
		t.Errorf("trace = %d, %d bytes", status, len(body))
	}
	if status, _ := get("/debug/pprof/symbol"); status != http.StatusOK {
		t.Errorf("symbol = %d", status)
	}
	status, body := get("/debug/runtime/metrics")
	if status != http.StatusOK {
		t.Fatalf("runtime metrics = %d", status)
	}
	var decoded map[string]any
	if err := json.Unmarshal(body, &decoded); err != nil {
		t.Fatalf("runtime metrics are not JSON: %v\n%s", err, body)
	}
	for _, key := range []string{"gcCpuSeconds", "liveHeapBytes", "schedLatencySeconds", "observedAt"} {
		if _, found := decoded[key]; !found {
			t.Errorf("runtime metrics miss %q: %s", key, body)
		}
	}
	// Nothing outside the profiling routes is served: the application's routes are not here.
	for _, path := range []string{"/", "/accounts/x/api/v2/state", "/orchestrator/v1/diagnostics", "/debug/vars"} {
		if status, _ := get(path); status != http.StatusNotFound {
			t.Errorf("GET %s = %d, want 404 on the profiling listener", path, status)
		}
	}
	request, _ := http.NewRequest(http.MethodPost, base+"/debug/runtime/metrics", strings.NewReader("x"))
	if response, err := http.DefaultClient.Do(request); err != nil || response.StatusCode != http.StatusMethodNotAllowed {
		t.Errorf("POST runtime metrics = %v, %v; want 405", response, err)
	}

	listener.Stop()
	stopped = true
	if _, err := net.DialTimeout("tcp", listener.Addr().String(), time.Second); err == nil {
		t.Fatal("the profiling port still accepts connections after Stop")
	}
	if Enabled() {
		t.Fatal("labels stay enabled after Stop")
	}
	if got := runtime.SetMutexProfileFraction(-1); got != mutexBefore {
		t.Fatalf("mutex profile fraction = %d after Stop, want %d", got, mutexBefore)
	}
}

func TestListenerStopsWithItsContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	listener, err := Start(ctx, "127.0.0.1:0", t.Logf)
	if err != nil {
		t.Fatal(err)
	}
	address := listener.Addr().String()
	cancel()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if connection, err := net.DialTimeout("tcp", address, 100*time.Millisecond); err != nil {
			return
		} else {
			connection.Close()
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("listener kept serving after its context was cancelled")
}

func TestLabelsNestAndReachChildGoroutines(t *testing.T) {
	listener, err := Start(context.Background(), "127.0.0.1:0", t.Logf)
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Stop()

	runtimeContext := WithRuntime(context.Background(), "acct-7")
	if value, found := pprof.Label(runtimeContext, LabelRuntime); !found || value != "acct-7" {
		t.Fatalf("runtime label = %q, %t", value, found)
	}
	release := make(chan struct{})
	started := make(chan struct{})
	Do(runtimeContext, func(stageContext context.Context) {
		Do(stageContext, func(policyContext context.Context) {
			for key, want := range map[string]string{LabelRuntime: "acct-7", LabelStage: StageAutomation, LabelPolicy: "autoStorm"} {
				if got, found := pprof.Label(policyContext, key); !found || got != want {
					t.Errorf("label %s = %q, %t; want %q", key, got, found, want)
				}
			}
			// A goroutine started under the labels inherits them.
			go func() {
				close(started)
				<-release
			}()
			<-started
			var dump bytes.Buffer
			if err := pprof.Lookup("goroutine").WriteTo(&dump, 1); err != nil {
				t.Error(err)
			}
			if !strings.Contains(dump.String(), `"policy":"autoStorm"`) || !strings.Contains(dump.String(), `"runtime":"acct-7"`) ||
				!strings.Contains(dump.String(), `"stage":"automation"`) {
				t.Errorf("goroutine profile does not show the inherited labels:\n%s", dump.String())
			}
		}, LabelPolicy, "autoStorm")
	}, LabelStage, StageAutomation)
	close(release)

	// Labels are restored after Do: the stage is gone from the outer context.
	if _, found := pprof.Label(runtimeContext, LabelStage); found {
		t.Fatal("the stage label leaked out of Do")
	}
}

func TestSummarizeHistogram(t *testing.T) {
	histogram := &metrics.Float64Histogram{
		Counts:  []uint64{0, 50, 40, 9, 1},
		Buckets: []float64{0, 1e-6, 1e-5, 1e-4, 1e-3, 1e30},
	}
	summary := summarizeHistogram(histogram)
	if summary.Count != 100 || summary.P50 != 1e-5 || summary.P90 != 1e-4 || summary.P99 != 1e-3 || summary.Max != 1e-3 {
		t.Fatalf("summary = %+v", summary)
	}
	tail := summarizeHistogram(&metrics.Float64Histogram{Counts: []uint64{1}, Buckets: []float64{2, 1e30}})
	if tail.Count != 1 || tail.Max != 2 {
		t.Fatalf("unbounded top bucket summary = %+v, want its lower bound", tail)
	}
	if empty := summarizeHistogram(&metrics.Float64Histogram{Counts: []uint64{0}, Buckets: []float64{0, 1}}); empty.Count != 0 || empty.Max != 0 {
		t.Fatalf("empty summary = %+v", empty)
	}
	if summarizeHistogram(nil).Count != 0 {
		t.Fatal("nil histogram must summarise to zero")
	}
}

func TestReadRuntimeMetricsReportsProcessValues(t *testing.T) {
	runtime.GC()
	sample := ReadRuntimeMetrics()
	if sample.ObservedAt.IsZero() || sample.LiveHeapBytes == 0 || sample.SchedLatencySeconds.Count == 0 {
		t.Fatalf("runtime metrics = %+v, want observed live heap and scheduler latencies", sample)
	}
}
