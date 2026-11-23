//go:build !linux && !windows

package presentationstats

import "time"

func processCPU() (time.Duration, bool) { return 0, false }
