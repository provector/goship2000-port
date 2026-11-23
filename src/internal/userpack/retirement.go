package userpack

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Remove only files independently authenticated by the migration inventory and
// the validated current representation. Missing members allow interrupted cleanup
// to resume. Unrecognized files and unique pre-migration states remain in place.
func RetireCopies(dir, index, subdir string, known func(string) ([]byte, bool)) error {
	st, err := os.Lstat(dir)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if !st.IsDir() {
		return nil
	}
	path := filepath.Join(dir, index)
	info, err := ordinary(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if info.Size() > MaxEntry {
		return fmt.Errorf("migration inventory too large: %s", path)
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var rows map[string]string
	if err = json.Unmarshal(b, &rows); err != nil {
		return err
	}
	complete := true
	for name, want := range rows {
		if !filepath.IsLocal(name) || strings.ContainsAny(name, ":\\") || filepath.ToSlash(filepath.Clean(name)) != name {
			return fmt.Errorf("invalid migration inventory path %q", name)
		}
		src := filepath.Join(dir, subdir, filepath.FromSlash(name))
		if _, err = os.Lstat(src); os.IsNotExist(err) {
			continue
		} else if err != nil {
			return err
		}
		real, err := filepath.EvalSymlinks(src)
		if err != nil {
			return err
		}
		if real != src {
			complete = false
			continue
		}
		data, ok := known(name)
		h := sha256.Sum256(data)
		if !ok || hex.EncodeToString(h[:]) != want {
			complete = false
			continue
		}
		actual, err := HashFile(src)
		if err != nil {
			return err
		}
		if actual != want {
			complete = false
			continue
		}
		if err = os.Remove(src); err != nil {
			return err
		}
		for sub := filepath.Dir(src); sub != dir; sub = filepath.Dir(sub) {
			files, err := os.ReadDir(sub)
			if err != nil {
				return err
			}
			if len(files) > 0 {
				break
			}
			if err = os.Remove(sub); err != nil {
				return err
			}
		}
	}
	if complete {
		return os.Remove(path)
	}
	return nil
}
