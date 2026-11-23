package storeowner

import (
	"errors"
	"fmt"
	"golang.org/x/sys/unix"
	"os"
)

func openTable() (*os.File, error) {
	// A machine-wide stable inode. Never unlink this while another descriptor
	// may exist. Do not follow links or depend on a caller's TMPDIR.
	const path = "/run/lock/goship2000-ownership-v1"
	flags := unix.O_RDWR | unix.O_NOFOLLOW | unix.O_CLOEXEC | unix.O_NONBLOCK
	fd, e := unix.Open(path, flags, 0)
	if errors.Is(e, unix.ENOENT) {
		fd, e = unix.Open(path, flags|unix.O_CREAT|unix.O_EXCL, 0666)
		if e == nil {
			e = unix.Fchmod(fd, 0666)
			if e != nil {
				unix.Close(fd)
				return nil, e
			}
		} else if errors.Is(e, unix.EEXIST) {
			fd, e = unix.Open(path, flags, 0)
		}
	}
	if e != nil {
		return nil, e
	}
	f := os.NewFile(uintptr(fd), path)
	st, e := f.Stat()
	if e != nil || !st.Mode().IsRegular() {
		f.Close()
		if e == nil {
			e = unix.EINVAL
		}
		return nil, e
	}
	return f, nil
}
func lockRange(f *os.File, offset uint64, write bool) error {
	kind := int16(unix.F_RDLCK)
	if write {
		kind = unix.F_WRLCK
	}
	return unix.FcntlFlock(f.Fd(), unix.F_OFD_SETLK, &unix.Flock_t{Type: kind, Start: int64(offset), Len: 1})
}

func objectID(path string) (string, error) {
	var st unix.Stat_t
	if e := unix.Stat(path, &st); e != nil {
		return "", e
	}
	return fmt.Sprintf("@%x-%x", st.Dev, st.Ino), nil
}
