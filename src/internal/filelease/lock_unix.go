//go:build unix

package filelease

import (
	"golang.org/x/sys/unix"
	"os"
)

func tryLock(f *os.File) error { return unix.Flock(int(f.Fd()), unix.LOCK_EX|unix.LOCK_NB) }

func openExisting(path string) (*os.File, error) { return os.OpenFile(path, os.O_RDWR, 0) }
