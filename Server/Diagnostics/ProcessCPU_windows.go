//go:build windows

package Diagnostics

import "syscall"

// processCPUSeconds is the cumulative user+kernel CPU time of this process.
func processCPUSeconds() (float64, bool) {
	var creation, exit, kernel, user syscall.Filetime
	handle, err := syscall.GetCurrentProcess()
	if err != nil {
		return 0, false
	}
	if err := syscall.GetProcessTimes(handle, &creation, &exit, &kernel, &user); err != nil {
		return 0, false
	}
	return filetimeSeconds(kernel) + filetimeSeconds(user), true
}

// filetimeSeconds converts 100 ns FILETIME units to seconds.
func filetimeSeconds(value syscall.Filetime) float64 {
	return float64(uint64(value.HighDateTime)<<32|uint64(value.LowDateTime)) / 1e7
}
