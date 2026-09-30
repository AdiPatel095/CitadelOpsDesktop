//go:build !windows

package Diagnostics

import "syscall"

// processCPUSeconds is the cumulative user+system CPU time of this process.
func processCPUSeconds() (float64, bool) {
	var usage syscall.Rusage
	if err := syscall.Getrusage(syscall.RUSAGE_SELF, &usage); err != nil {
		return 0, false
	}
	return timevalSeconds(usage.Utime) + timevalSeconds(usage.Stime), true
}

func timevalSeconds(value syscall.Timeval) float64 {
	return float64(value.Sec) + float64(value.Usec)/1e6
}
