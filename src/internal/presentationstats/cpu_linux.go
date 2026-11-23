package presentationstats

import (
	"syscall"
	"time"
)

func processCPU() (time.Duration, bool) {
	var u syscall.Rusage
	if syscall.Getrusage(syscall.RUSAGE_SELF, &u) != nil {
		return 0, false
	}
	return time.Duration(u.Utime.Nano() + u.Stime.Nano()), true
}
