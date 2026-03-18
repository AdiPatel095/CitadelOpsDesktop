package ResponseRegistry

import (
	"context"
	"log"
	"runtime"
	"strings"
	"time"

	"github.com/shirou/gopsutil/v3/process"
)

// SendMemoryStatsFunc is a callback to notify frontend of memory usage (Go App MB, Chrome MB)
var SendMemoryStatsFunc func(int, int)

// SetMemoryStatsCallback sets the callback for memory stats notification
func SetMemoryStatsCallback(fn func(int, int)) {
	SendMemoryStatsFunc = fn
}

// StartMemoryMonitor periodically calculates Go and Chrome memory and broadcasts stats using WebSockets
func StartMemoryMonitor(ctx context.Context) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			goMemMB, chromeMemMB := getMemoryStats()
			if SendMemoryStatsFunc != nil {
				SendMemoryStatsFunc(goMemMB, chromeMemMB)
			}
		}
	}
}

func getMemoryStats() (int, int) {
	// 1. Get Go App Memory
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	goMemMB := int(m.Alloc / 1024 / 1024)

	// 2. Get Chrome Memory
	chromeMemMB := 0
	procs, err := process.Processes()
	if err != nil {
		log.Printf("[MemoryMonitor] Failed to list processes: %v", err)
		return goMemMB, chromeMemMB
	}

	var totalChromeRSS uint64
	for _, p := range procs {
		cmdline, err := p.Cmdline()
		if err != nil {
			continue
		}

		// Look for Chrome processes spawned by our app
		if strings.Contains(strings.ToLower(cmdline), "chrome") {
			mem, err := p.MemoryInfo()
			if err == nil && mem != nil {
				totalChromeRSS += mem.RSS
			}
		}
	}

	chromeMemMB = int(totalChromeRSS / 1024 / 1024)
	return goMemMB, chromeMemMB
}
