// Package playerdirs resolves storage beside the configured career directory.
// Callers pass the effective absolute saves path, never the process directory.
package playerdirs

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gunship2000-assets/internal/userpack"
)

func Sibling(saves, name string) string {
	return filepath.Join(filepath.Dir(filepath.Clean(saves)), name)
}

func Directory(path string) error {
	if e := os.MkdirAll(filepath.Dir(path), 0700); e != nil {
		return e
	}
	e := os.Mkdir(path, 0700)
	if e == nil {
		return nil
	}
	if !os.IsExist(e) {
		return e
	}
	st, e := os.Lstat(path)
	if e != nil {
		return e
	}
	if !st.IsDir() {
		return fmt.Errorf("expected an ordinary directory at %s", path)
	}
	return nil
}

// A legacy custom career directory may itself be named reports, imports, temp,
// or screenshots. It is still a career, never a sibling output directory.
func IsCareerDirectory(saves, output string) bool {
	return strings.EqualFold(filepath.Clean(saves), filepath.Clean(output))
}

const archiveOwner = ".goship-archive-owner"

func owned(path, saves string) bool {
	st, e := os.Lstat(path)
	if e != nil || !st.IsDir() {
		return false
	}
	marker := filepath.Join(path, archiveOwner)
	st, e = os.Lstat(marker)
	if e != nil || !st.Mode().IsRegular() || st.Size() > 1024 {
		return false
	}
	b, e := os.ReadFile(marker)
	return e == nil && string(b) == filepath.Base(filepath.Clean(saves))
}

func mark(dir, saves string) error {
	if owned(dir, saves) {
		return nil
	}
	path := filepath.Join(dir, archiveOwner)
	if _, e := os.Lstat(path); !os.IsNotExist(e) {
		if e != nil {
			return e
		}
		return fmt.Errorf("different archive owner in %s", dir)
	}
	f, e := os.CreateTemp(dir, ".archive-owner-")
	if e != nil {
		return e
	}
	defer os.Remove(f.Name())
	_, e = f.WriteString(filepath.Base(filepath.Clean(saves)))
	if e = errors.Join(e, f.Sync(), f.Close()); e != nil {
		return e
	}
	return userpack.Replace(f.Name(), path)
}

// Under store ownership a same-volume directory rename preserves hard links,
// report companions and unknown members as a unit. If either path is occupied
// or on another volume, keep using the original archive without merging trees.
func Archive(saves, name string, create bool) string {
	old := filepath.Join(saves, name)
	dst := Sibling(saves, name)
	if IsCareerDirectory(saves, dst) {
		return old
	}
	source, e := os.Lstat(old)
	if e != nil && !os.IsNotExist(e) {
		return old
	}
	target, te := os.Lstat(dst)
	if source != nil {
		if !source.IsDir() || !os.IsNotExist(te) {
			fmt.Fprintf(os.Stderr, "Retained archive %s; destination %s unavailable or occupied\n", old, dst)
			return old
		}
		if name == "reports" {
			entries, err := os.ReadDir(old)
			if err != nil {
				fmt.Fprintln(os.Stderr, "Report archive left in place:", err)
				return old
			}
			for _, entry := range entries {
				if entry.IsDir() && strings.HasPrefix(entry.Name(), "session-") {
					anchor := filepath.Join(old, entry.Name(), "owner.lease")
					if _, err = os.Lstat(anchor); !os.IsNotExist(err) {
						fmt.Fprintln(os.Stderr, "Report archive left in place; unresolved lease", anchor)
						return old
					}
				}
			}
		}
		if err := mark(old, saves); err == nil {
			if err = userpack.Replace(old, dst); err == nil {
				return dst
			} else if owned(dst, saves) {
				fmt.Fprintf(os.Stderr, "Archive moved to %s; durability check failed: %v\n", dst, err)
				return dst
			} else {
				fmt.Fprintln(os.Stderr, "Archive relocation deferred:", err)
			}
		} else {
			fmt.Fprintln(os.Stderr, "Retained archive:", err)
		}
		return old
	}
	if target != nil && owned(dst, saves) {
		return dst
	}
	if !os.IsNotExist(te) {
		return old
	}
	if !create {
		return dst
	}
	if e = Directory(dst); e == nil {
		if e = mark(dst, saves); e == nil {
			return dst
		}
	}
	fmt.Fprintf(os.Stderr, "Could not initialize %s; using %s: %v\n", dst, old, e)
	return old
}

func Scratch(saves string) (string, error) {
	dest := Sibling(saves, "temp")
	if !IsCareerDirectory(saves, dest) {
		if err := Directory(dest); err == nil {
			if path, err := os.MkdirTemp(dest, "goship-work-"); err == nil {
				return path, nil
			}
		}
	}
	return os.MkdirTemp("", "goship-work-")
}

// OwnedArchive identifies directories created or moved by this store. A location
// transfer must not sweep unrelated sibling directories into its inventory.
func OwnedArchive(saves, name string) bool {
	dst := Sibling(saves, name)
	return !IsCareerDirectory(saves, dst) && owned(dst, saves)
}

// ArchiveReaders keeps companions available if a collision prevented merging
// two generations. An unclaimed sibling can belong to another player store.
func ArchiveReaders(saves, name string) []string {
	old := filepath.Join(saves, name)
	var dirs []string
	if st, e := os.Lstat(old); e == nil && st.IsDir() {
		dirs = append(dirs, old)
	}
	if OwnedArchive(saves, name) {
		dirs = append(dirs, Sibling(saves, name))
	}
	return dirs
}
