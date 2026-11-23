package state

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gunship2000-assets/internal/userpack"
)

// RelocateLibraries is called under the career owner before exposing either
// browser. During migration the inventory and rollback files allow restart.
func (s *SaveStore) RelocateLibraries(app string) error {
	if !s.liveOwner {
		return fmt.Errorf("library migration needs exclusive career ownership")
	}
	return relocateLibraries(app, s.Directory)
}
func relocateLibraries(app, root string) error {
	if err := migrateLibraries(app, root); err != nil {
		return err
	}
	return cleanLibraryBackup(app, root)
}

const libraryBackup = ".t3-library-backup"

func migrateLibraries(app, root string) error {
	archive := filepath.Join(app, libraryBackup)
	marker := filepath.Join(root, ".t3-libraries.json")
	if _, e := os.Lstat(marker); e == nil {
		return nil
	} else if !os.IsNotExist(e) {
		return e
	}
	files := map[string]string{}
	dirs := map[string]string{"films": "replays", "missions": "missions"}
	for _, srcName := range []string{"films", "missions"} {
		destName := dirs[srcName]
		src := filepath.Join(app, srcName)
		dst := filepath.Join(root, destName)
		if src == dst {
			if e := plainDir(dst); e != nil {
				return e
			}
			continue
		}
		if pathsOverlap(src, dst) {
			return fmt.Errorf("overlapping old and new %s libraries", srcName)
		}
		if e := plainDir(dst); e != nil {
			return e
		}
		st, e := os.Lstat(src)
		if os.IsNotExist(e) {
			continue
		}
		if e != nil {
			return e
		}
		if !st.IsDir() {
			return fmt.Errorf("invalid library directory %s", src)
		}
		lock := ".film-publish.lock"
		if srcName == "missions" {
			lock = ".authored-store.lock"
		}
		entries, e := os.ReadDir(src)
		if e != nil {
			return e
		}
		for _, ent := range entries {
			n := ent.Name()
			if n == lock || n == lock+".lease" {
				continue
			}
			if !ent.Type().IsRegular() {
				return fmt.Errorf("unidentified library item %s; source retained", filepath.Join(src, n))
			}
			h, e := userpack.HashFile(filepath.Join(src, n))
			if e != nil {
				return e
			}
			files[srcName+"/"+n] = h
		}
	}
	if app != root {
		if h, e := userpack.HashFile(filepath.Join(app, MissionSummaryFilename)); e == nil {
			files[MissionSummaryFilename] = h
		} else if !os.IsNotExist(e) {
			return e
		}
	}
	pending := filepath.Join(root, ".t3-library-pending.json")
	if b, e := ReadRegularSave(pending); e == nil {
		var old map[string]string
		if e = json.Unmarshal(b, &old); e != nil {
			return e
		}
		for n, h := range files {
			if old[n] != h {
				return fmt.Errorf("library changed during migration: %s", n)
			}
		}
		files = old
	} else if os.IsNotExist(e) {
		b, _ := json.MarshalIndent(files, "", "  ")
		if e = writeSaveAtomic(root, filepath.Base(pending), b); e != nil {
			return e
		}
	} else {
		return e
	}
	target := func(n string) (string, error) {
		if n == MissionSummaryFilename {
			return filepath.Join(root, n), nil
		}
		kind, name, ok := strings.Cut(n, "/")
		dest := dirs[kind]
		if !ok || dest == "" || filepath.Base(name) != name || strings.ContainsAny(name, ":\\") || name == "." || name == ".." {
			return "", fmt.Errorf("invalid library inventory entry")
		}
		return filepath.Join(root, dest, name), nil
	}
	// Copy and validate the complete set before retiring any inputs.
	for n, h := range files {
		dst, e := target(n)
		if e != nil {
			return e
		}
		src := filepath.Join(app, n)
		if _, e = os.Lstat(src); os.IsNotExist(e) {
			src = filepath.Join(archive, n)
		}
		if e = copyChecked(src, dst, h); e != nil {
			return e
		}
	}
	for n, h := range files {
		src := filepath.Join(app, n)
		dst := filepath.Join(archive, n)
		if _, e := os.Lstat(src); os.IsNotExist(e) {
			got, e := userpack.HashFile(dst)
			if e != nil || got != h {
				return fmt.Errorf("missing rollback file %s", n)
			}
			continue
		}
		got, e := userpack.HashFile(src)
		if e != nil || got != h {
			return fmt.Errorf("changed library input %s", n)
		}
		if e = backupParent(archive, dst); e != nil {
			return e
		}
		if _, e = os.Lstat(dst); !os.IsNotExist(e) {
			return fmt.Errorf("occupied rollback path %s", n)
		}
		if e = userpack.Replace(src, dst); e != nil {
			return e
		}
	}
	return userpack.Replace(pending, marker)
}

// Only visit inventoried files in the three known destinations. In particular
// an edited mission or a renamed recording can leave a unique rollback copy.
func cleanLibraryBackup(app, root string) error {
	archive := filepath.Join(app, libraryBackup)
	st, err := os.Lstat(archive)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if !st.IsDir() {
		return nil
	}
	if _, err = os.Lstat(filepath.Join(root, ".t3-library-pending.json")); !os.IsNotExist(err) {
		return err
	}
	raw, err := ReadRegularSave(filepath.Join(root, ".t3-libraries.json"))
	if err != nil {
		return err
	}
	var inventory map[string]string
	if err = json.Unmarshal(raw, &inventory); err != nil {
		return err
	}
	removeDuplicate := func(n, dst string) error {
		want, ok := inventory[n]
		if !ok {
			return nil
		}
		src := filepath.Join(archive, n)
		if h, e := userpack.HashFile(src); e != nil || h != want {
			return nil
		}
		if h, e := userpack.HashFile(dst); e != nil || h != want {
			return nil
		}
		return os.Remove(src)
	}
	if err = removeDuplicate(MissionSummaryFilename, filepath.Join(root, MissionSummaryFilename)); err != nil {
		return err
	}
	for _, group := range []struct{ old, new string }{{"films", "replays"}, {"missions", "missions"}} {
		dir := filepath.Join(archive, group.old)
		st, err := os.Lstat(dir)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return err
		}
		if !st.IsDir() {
			continue
		}
		dst := filepath.Join(root, group.new)
		if st, e := os.Lstat(dst); e != nil || !st.IsDir() {
			continue
		}
		err = func() error {
			entries, e := os.ReadDir(dir)
			if e != nil {
				return e
			}
			for _, ent := range entries {
				if ent.Type().IsRegular() {
					if e = removeDuplicate(group.old+"/"+ent.Name(), filepath.Join(dst, ent.Name())); e != nil {
						return e
					}
				}
			}
			return removeEmptyLibraryDir(dir)
		}()
		if err != nil {
			return err
		}
	}
	return removeEmptyLibraryDir(archive)
}

func removeEmptyLibraryDir(dir string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	if len(entries) != 0 {
		return nil
	}
	return os.Remove(dir)
}
