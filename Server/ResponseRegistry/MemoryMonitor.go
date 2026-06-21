package ResponseRegistry

import (
	"CitadelDesktop/Server/ChromeUserData"
	"context"
	"log"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/shirou/gopsutil/v3/process"
)

// SendMemoryStatsFunc is a callback to notify frontend of memory usage (Go App MB, Chrome MB)
var SendMemoryStatsFunc func(int, int)

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
	appChromeDir, err := ChromeUserData.AppUserDataDir()
	if err != nil {
		log.Printf("[MemoryMonitor] Failed to resolve app Chrome profile: %v", err)
		return goMemMB, chromeMemMB
	}
	appChromeMarker := normalizePath(appChromeDir)

	var totalChromeRSS uint64
	for _, p := range procs {
		cmdline, err := p.Cmdline()
		if err != nil {
			continue
		}

		if isAppChromeProcess(cmdline, appChromeMarker) {
			mem, err := p.MemoryInfo()
			if err == nil && mem != nil {
				totalChromeRSS += mem.RSS
			}
		}
	}

	chromeMemMB = int(totalChromeRSS / 1024 / 1024)
	return goMemMB, chromeMemMB
}

func isAppChromeProcess(cmdline, appChromeMarker string) bool {
	if appChromeMarker == "" {
		return false
	}
	normalized := normalizeCmdline(cmdline)
	return strings.Contains(normalized, appChromeMarker)
}

func normalizePath(value string) string {
	return strings.ToLower(filepath.ToSlash(filepath.Clean(value)))
}

func normalizeCmdline(value string) string {
	return strings.ToLower(filepath.ToSlash(value))
}
