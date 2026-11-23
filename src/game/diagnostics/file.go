package diagnostics

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
)

const fileHeader = "# GoShip 2000 runtime log v1\n"
const MaxRecordBytes = 4096

var openLogs sync.Map

type rotatingFile struct {
	path      string
	cap, size int64
	file      *os.File
}

func ownedFile(path string) error {
	i, e := os.Lstat(path)
	if e != nil {
		return e
	}
	if !i.Mode().IsRegular() || i.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("%s: expected a regular owned log, not symlink/directory", path)
	}
	if i.Mode().Perm()&0222 == 0 {
		return fmt.Errorf("%s: file is read-only", path)
	}
	f, e := os.Open(path)
	if e != nil {
		return e
	}
	defer f.Close()
	b := make([]byte, len(fileHeader))
	_, e = io.ReadFull(f, b)
	if e != nil || !bytes.Equal(b, []byte(fileHeader)) {
		return fmt.Errorf("%s: existing file is not a GoShip log; choose another Log File Name", path)
	}
	return nil
}
func openRotating(dir, name string, cap int64) (*rotatingFile, error) {
	path := filepath.Join(dir, name)
	i, e := os.Lstat(dir)
	if os.IsNotExist(e) {
		if e = os.Mkdir(dir, 0755); e != nil {
			return nil, e
		}
	} else if e != nil {
		return nil, e
	} else if !i.IsDir() || i.Mode()&os.ModeSymlink != 0 {
		return nil, fmt.Errorf("%s: log directory must not be a symlink", dir)
	}
	if i, e = os.Stat(dir); e != nil || i.Mode().Perm()&0222 == 0 {
		return nil, fmt.Errorf("%s: log directory is not writable", dir)
	}
	if _, active := openLogs.LoadOrStore(path, true); active {
		return nil, fmt.Errorf("log already in use: %s", path)
	}
	r := &rotatingFile{path: path, cap: cap}
	if e = r.open(); e != nil {
		openLogs.Delete(path)
		return nil, e
	}
	return r, nil
}
func (r *rotatingFile) open() error {
	if _, e := os.Lstat(r.path); e == nil {
		if e = ownedFile(r.path); e != nil {
			return e
		}
		f, e := os.OpenFile(r.path, os.O_WRONLY|os.O_APPEND, 0600)
		if e != nil {
			return e
		}
		a, e := f.Stat()
		b, be := os.Lstat(r.path)
		if e != nil || be != nil || b.Mode()&os.ModeSymlink != 0 || !os.SameFile(a, b) {
			f.Close()
			return errors.New("log changed during open")
		}
		r.file = f
		r.size = a.Size()
		return nil
	} else if !os.IsNotExist(e) {
		return e
	}
	f, e := os.OpenFile(r.path, os.O_CREATE|os.O_EXCL|os.O_WRONLY|os.O_APPEND, 0600)
	if e != nil {
		return e
	}
	r.file = f
	n, e := io.WriteString(f, fileHeader)
	r.size = int64(n)
	if e != nil {
		f.Close()
	}
	return e
}
func (r *rotatingFile) Write(b []byte) (int, error) {
	if int64(len(b))+int64(len(fileHeader)) > r.cap {
		return 0, errors.New("log record exceeds file cap")
	}
	if r.size+int64(len(b)) > r.cap {
		prev := r.path + ".previous"
		if _, e := os.Lstat(prev); e == nil {
			if e = ownedFile(prev); e != nil {
				return 0, e
			}
		} else if !os.IsNotExist(e) {
			return 0, e
		}
		if e := r.file.Sync(); e != nil {
			return 0, e
		}
		if e := r.file.Close(); e != nil {
			return 0, e
		}
		r.file = nil
		if e := os.Rename(r.path, prev); e != nil {
			return 0, fmt.Errorf("rotate %s: %w", r.path, e)
		}
		if e := r.open(); e != nil {
			return 0, e
		}
	}
	n, e := r.file.Write(b)
	r.size += int64(n)
	return n, e
}
func (r *rotatingFile) Close() error {
	var e error
	if r.file != nil {
		e = r.file.Sync()
		if ce := r.file.Close(); e == nil {
			e = ce
		}
		r.file = nil
	}
	openLogs.Delete(r.path)
	return e
}
