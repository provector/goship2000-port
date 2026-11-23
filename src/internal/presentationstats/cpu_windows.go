package presentationstats

import (
	"syscall"
	"time"
)

func processCPU() (time.Duration, bool) {
	var start, exit, kernel, user syscall.Filetime
	h, err := syscall.GetCurrentProcess()
	if err != nil || syscall.GetProcessTimes(h, &start, &exit, &kernel, &user) != nil {
		return 0, false
	}
	// GetProcessTimes returns durations in 100 ns ticks, not UTC timestamps.
	ticks := func(t syscall.Filetime) uint64 { return uint64(t.HighDateTime)<<32 | uint64(t.LowDateTime) }
	return time.Duration(ticks(kernel)+ticks(user)) * 100, true
}
