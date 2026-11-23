//go:build unix

package filelease

import (
	"errors"
	"golang.org/x/sys/unix"
)

func processGone(pid int) bool { return errors.Is(unix.Kill(pid, 0), unix.ESRCH) }
