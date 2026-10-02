package Diagnostics

import (
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/shirou/gopsutil/v3/process"
)

const sampleInterval = 5 * time.Second

type Snapshot struct {
	MapRequests         map[string]uint64 `json:"mapRequests,omitempty"`
	ApplicationMemoryMB int               `json:"applicationMemoryMb"`
	BrowserMemoryMB     int               `json:"browserMemoryMb"`
	ObservedAt          time.Time         `json:"observedAt"`
}

type Monitor struct {
	profileRoot string
	mapRequests map[string]uint64

	mu        sync.RWMutex
	snapshot  Snapshot
	collectMu sync.Mutex
}

func NewMonitor(dataDir string) *Monitor {
	monitor := &Monitor{profileRoot: normalizePath(filepath.Join(dataDir, "Browser"))}
	monitor.collect(false)
	return monitor
}

func (monitor *Monitor) Snapshot() Snapshot {
	if monitor == nil {
		return Snapshot{}
	}
	monitor.mu.RLock()
	current := monitor.snapshotWithRequestsLocked()
	monitor.mu.RUnlock()
	if time.Since(current.ObservedAt) < sampleInterval {
		return current
	}
	monitor.collectMu.Lock()
	defer monitor.collectMu.Unlock()
	monitor.mu.RLock()
	current = monitor.snapshotWithRequestsLocked()
	monitor.mu.RUnlock()
	if time.Since(current.ObservedAt) >= sampleInterval {
		monitor.collect(true)
	}
	monitor.mu.RLock()
	defer monitor.mu.RUnlock()
	return monitor.snapshotWithRequestsLocked()
}

func (monitor *Monitor) collect(includeBrowser bool) {
	var memory runtime.MemStats
	runtime.ReadMemStats(&memory)
	next := Snapshot{ApplicationMemoryMB: int(memory.Alloc / 1024 / 1024), ObservedAt: time.Now().UTC()}
	if includeBrowser {
		next.BrowserMemoryMB = browserMemoryMB(monitor.profileRoot)
	}
	monitor.mu.Lock()
	monitor.snapshot = next
	monitor.mu.Unlock()
}

func browserMemoryMB(profileRoot string) int {
	if profileRoot == "" {
		return 0
	}
	processes, err := process.Processes()
	if err != nil {
		return 0
	}
	total := uint64(0)
	for _, candidate := range processes {
		command, commandErr := candidate.Cmdline()
		if commandErr != nil || !strings.Contains(normalizePath(command), profileRoot) {
			continue
		}
		memory, memoryErr := candidate.MemoryInfo()
		if memoryErr == nil && memory != nil {
			total += memory.RSS
		}
	}
	return int(total / 1024 / 1024)
}

func normalizePath(value string) string {
	return strings.ToLower(filepath.ToSlash(filepath.Clean(value)))
}

func (m *Monitor) RecordMapRequest(kind string) {
	if m == nil || kind != "storm" && kind != "fortress" {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.mapRequests == nil {
		m.mapRequests = map[string]uint64{}
	}
	m.mapRequests[kind]++
}
func (m *Monitor) snapshotWithRequestsLocked() Snapshot {
	s := m.snapshot
	if len(m.mapRequests) > 0 {
		s.MapRequests = map[string]uint64{}
		for k, v := range m.mapRequests {
			s.MapRequests[k] = v
		}
	}
	return s
}
