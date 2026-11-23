//go:build !windows

package userpack

import (
	"os"
	"path/filepath"
)

func SyncDir(dir string) error {
	f, e := os.Open(dir)
	if e != nil {
		return e
	}
	defer f.Close()
	return f.Sync()
}
func Replace(stage, target string) error {
	if e := os.Rename(stage, target); e != nil {
		return e
	}
	if e := SyncDir(filepath.Dir(target)); e != nil {
		return e
	}
	if filepath.Dir(stage) != filepath.Dir(target) {
		return SyncDir(filepath.Dir(stage))
	}
	return nil
}
