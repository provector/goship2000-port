package filelease

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// Retire removes only the known V4.3 regular, empty anchors and their proven
// hard links. All reservations are checked before deleting any. Older programs
// must remain closed during the one-way upgrade.
func Retire(writers, anchors []string) error {
	var handles []*os.File
	defer func() {
		for _, f := range handles {
			f.Close()
		}
	}()
	removals := map[string]bool{}
	var markers []string
	internalDirs := map[string]bool{}
	reserve := func(p string) (os.FileInfo, error) {
		st, e := os.Lstat(p)
		if os.IsNotExist(e) {
			return nil, nil
		}
		if e != nil {
			return nil, e
		}
		if !ordinaryParent(p) {
			return nil, fmt.Errorf("unverified legacy directory %s", filepath.Dir(p))
		}
		if !st.Mode().IsRegular() || st.Size() != 0 {
			return nil, fmt.Errorf("unidentified legacy reservation %s; preserved", p)
		}
		f, e := openExisting(p)
		if e != nil {
			return nil, e
		}
		if e = tryLock(f); e != nil {
			f.Close()
			return nil, fmt.Errorf("store already in use by a legacy writer at %s: %w", p, e)
		}
		handles = append(handles, f)
		removals[p] = true
		return st, nil
	}
	for _, p := range anchors {
		if _, e := reserve(p); e != nil {
			return e
		}
	}
	for _, p := range writers {
		marker, e := os.Lstat(p)
		if e != nil && !os.IsNotExist(e) {
			return e
		}
		dir := filepath.Join(filepath.Dir(p), ".runtime-locks")
		proved := false
		for _, anchor := range []string{p + ".lease", filepath.Join(dir, filepath.Base(p)+".lease")} {
			st, e := reserve(anchor)
			if e != nil {
				return e
			}
			if marker != nil && st != nil && os.SameFile(marker, st) {
				proved = true
			}
		}
		if marker != nil {
			if !ordinaryParent(p) {
				return fmt.Errorf("unverified legacy directory %s", filepath.Dir(p))
			}
			if !proved && !deadLegacyPID(p, marker) {
				return fmt.Errorf("unverified legacy reservation %s; close older writers and inspect it", p)
			}
			markers = append(markers, p)
		}
		internalDirs[dir] = true
	}
	// A cut after removing the short-lived name leaves identifiable anchors.
	for _, p := range markers {
		if e := os.Remove(p); e != nil {
			return e
		}
	}
	for p := range removals {
		if e := os.Remove(p); e != nil {
			return e
		}
	}
	// On Windows a delete-pending name vanishes only after its last close.
	for _, f := range handles {
		if e := f.Close(); e != nil {
			return e
		}
	}
	handles = nil
	for d := range internalDirs {
		if st, e := os.Lstat(d); e == nil && st.IsDir() && ordinaryParent(d) {
			if entries, e := os.ReadDir(d); e == nil && len(entries) == 0 {
				if e = os.Remove(d); e != nil {
					return e
				}
			}
		}
	}
	return nil
}

func ordinaryParent(p string) bool {
	parent := filepath.Dir(p)
	real, e := filepath.EvalSymlinks(parent)
	return e == nil && real == parent
}

func deadLegacyPID(path string, st os.FileInfo) bool {
	if !st.Mode().IsRegular() || st.Size() > 64 {
		return false
	}
	raw, e := os.ReadFile(path)
	if e != nil {
		return false
	}
	var pid int
	switch {
	case filepath.Base(path) == ".save.lock":
		var owner struct {
			PID int `json:"pid"`
		}
		var fields map[string]json.RawMessage
		if json.Unmarshal(raw, &fields) != nil || len(fields) != 1 || fields["pid"] == nil || json.Unmarshal(raw, &owner) != nil {
			return false
		}
		pid = owner.PID
	case filepath.Base(filepath.Dir(path)) == "logs":
		pid, e = strconv.Atoi(strings.TrimSpace(string(raw)))
		if e != nil || string(raw) != strconv.Itoa(pid)+"\n" {
			return false
		}
	default:
		return false
	}
	return pid > 0 && pid <= 2147483647 && processGone(pid)
}
