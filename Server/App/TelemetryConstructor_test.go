package App

import (
	"CitadelDesktop/Server/Session"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func telemetryWorkerCount() int {
	b := make([]byte, 1<<20)
	n := runtime.Stack(b, true)
	s := string(b[:n])
	return strings.Count(s, "Telemetry.(*Store).runPersistence(") + strings.Count(s, "Telemetry.(*Store).runRetention(")
}
func TestFailedConstructorCleansTelemetryWorkers(t *testing.T) {
	baseline := telemetryWorkerCount()
	for i := 0; i < 5; i++ {
		dir := t.TempDir()
		if err := os.MkdirAll(filepath.Join(dir, "Runtime", "Operations.sqlite"), 0700); err != nil {
			t.Fatal(err)
		}
		_, err := New(t.Context(), Config{DataDir: dir, Offline: true, Transport: Session.NewUnavailableTransport()})
		if err == nil {
			t.Fatal("synthetic operation-store failure missing")
		}
	}
	deadline := time.Now().Add(time.Second)
	for telemetryWorkerCount() > baseline {
		if time.Now().After(deadline) {
			t.Fatal("failed starts retained telemetry workers")
		}
		time.Sleep(time.Millisecond)
	}
}
