package state

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gunship2000-assets/internal/userpack"
)

const careerBackup = ".t3-career-backup"

func plainDir(dir string) error {
	st, e := os.Lstat(dir)
	if os.IsNotExist(e) {
		return os.Mkdir(dir, 0700)
	}
	if e != nil {
		return e
	}
	if !st.IsDir() {
		return fmt.Errorf("expected ordinary directory %s", dir)
	}
	return nil
}

// Create each rollback path component without following a symlink into another store.
func backupParent(base, target string) error {
	rel, e := filepath.Rel(base, filepath.Dir(target))
	if e != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return fmt.Errorf("invalid rollback path %s", target)
	}
	if e = plainDir(base); e != nil {
		return e
	}
	if rel == "." {
		return nil
	}
	d := base
	for _, part := range strings.Split(rel, string(filepath.Separator)) {
		d = filepath.Join(d, part)
		if e = plainDir(d); e != nil {
			return e
		}
	}
	return nil
}
func snapshotNames(root string) ([]string, error) {
	var names []string
	for _, n := range []string{saveFilename, oldSaveFilename, saveJournal} {
		if _, e := os.Lstat(filepath.Join(root, n)); e == nil {
			names = append(names, n)
		} else if !os.IsNotExist(e) {
			return nil, e
		}
	}
	for _, dir := range []string{"pilots", "sorties"} {
		list, e := sessionEntries(filepath.Join(root, dir))
		if e != nil {
			return nil, e
		}
		for _, ent := range list {
			if ent.Type().IsRegular() {
				n := dir + "/" + ent.Name()
				if safeSaveTarget(n) {
					names = append(names, n)
				}
			}
		}
	}
	sort.Strings(names)
	return names, nil
}
func copyChecked(src, dest, want string) error {
	if h, e := userpack.HashFile(dest); e == nil {
		if h == want {
			return nil
		}
		return fmt.Errorf("different existing migration destination %s", dest)
	} else if !os.IsNotExist(e) {
		return e
	}
	f, e := os.Open(src)
	if e != nil {
		return e
	}
	defer f.Close()
	t, e := os.CreateTemp(filepath.Dir(dest), ".migration-stage-")
	if e != nil {
		return e
	}
	defer os.Remove(t.Name())
	if _, e = io.Copy(t, f); e == nil {
		e = t.Sync()
	}
	ce := t.Close()
	if e != nil {
		return e
	}
	if ce != nil {
		return ce
	}
	h, e := userpack.HashFile(t.Name())
	if e != nil || h != want {
		return fmt.Errorf("migration input changed: %s", src)
	}
	return userpack.Replace(t.Name(), dest)
}
func captureLegacy(root string) error {
	backup := filepath.Join(root, careerBackup)
	names, e := snapshotNames(root)
	if e != nil {
		return e
	}
	if len(names) == 0 {
		return nil
	}
	if e = plainDir(backup); e != nil {
		return e
	}
	inv := filepath.Join(backup, "before.json")
	if _, e = os.Lstat(inv); e == nil {
		return nil
	} else if !os.IsNotExist(e) {
		return e
	}
	hashes := map[string]string{}
	for _, n := range names {
		src := filepath.Join(root, n)
		h, e := userpack.HashFile(src)
		if e != nil {
			return e
		}
		dest := filepath.Join(backup, n)
		if e = backupParent(backup, dest); e != nil {
			return e
		}
		if e = copyChecked(src, dest, h); e != nil {
			return e
		}
		hashes[n] = h
	}
	b, e := json.MarshalIndent(hashes, "", "  ")
	if e != nil {
		return e
	}
	return writeSaveAtomic(backup, "before.json", b)
}
func (s *SaveStore) openPacked(app, install, original string, startup bool) error {
	cfg := filepath.Join(app, userpack.ConfigName)
	p := &careerPacks{dir: s.Directory, cfg: cfg}
	if raw, e := ReadRegularSave(filepath.Join(p.dir, packIntent)); e == nil {
		var j packJournal
		if e = json.Unmarshal(raw, &j); e != nil {
			return e
		}
		if e = userpack.Locked(cfg, func() error { return p.rollForward(j) }); e != nil {
			return e
		}
	} else if !os.IsNotExist(e) {
		return e
	}
	if !existingPacks(p.dir) {
		fresh := true
		for _, n := range []string{saveFilename, oldSaveFilename, saveJournal, "technical-last-good.json", "backups", "pilots", "sorties", careerBackup} {
			if _, e := os.Lstat(filepath.Join(p.dir, n)); e == nil {
				fresh = false
				break
			} else if !os.IsNotExist(e) {
				return e
			}
		}
		if fresh {
			state, e := importLegacyRoster(original)
			if e == nil && state.Legacy == nil && original != "" {
				state, e = defaultRoster(install)
			}
			if e != nil {
				return e
			}
			if e = state.Validate(); e != nil {
				return e
			}
			if state.Legacy != nil {
				if e = preserveLegacyImport(p.dir, *state.Legacy); e != nil {
					return e
				}
			}
			files, e := stateFiles(state)
			if e != nil {
				return e
			}
			id, e := newStoreID()
			if e != nil {
				return e
			}
			p.id = id
			changes := map[string]*[]byte{}
			for name, data := range files {
				value := data
				changes[name] = &value
			}
			if e = p.publish(changes, true); e != nil {
				return e
			}
		}
	}
	if !existingPacks(p.dir) {
		if e := captureLegacy(p.dir); e != nil {
			return e
		}
		if e := s.openLoose(original, startup); e != nil {
			return e
		}
		raw := s.raw
		var roster rosterDocument
		if e := json.Unmarshal(raw, &roster); e != nil {
			return e
		}
		id, e := newStoreID()
		if e != nil {
			return e
		}
		p.id = id
		p.home = map[string]int{}
		p.owners = map[string]string{}
		p.records = userpack.Entries{}
		for i, id := range roster.Slots {
			if id != "" {
				p.home[id] = i
			}
		}
		changes := map[string]*[]byte{saveFilename: &raw}
		names, e := snapshotNames(p.dir)
		if e != nil {
			return e
		}
		retire := map[string]string{}
		for _, n := range names {
			if n == saveJournal {
				return fmt.Errorf("prior career transaction remains")
			}
			src := filepath.Join(p.dir, n)
			h, e := userpack.HashFile(src)
			if e != nil {
				return e
			}
			retire[n] = h
			if n == saveFilename || n == oldSaveFilename {
				continue
			}
			b, e := ReadRegularSave(src)
			if e != nil {
				return e
			}
			owner := ""
			if isSortieTarget(n) {
				d, e := decodeSortieRecord(b, strings.TrimSuffix(strings.TrimPrefix(n, "sorties/"), ".json"))
				if e != nil {
					return e
				}
				owner = d.Start.PilotID
			} else {
				owner = strings.Split(strings.TrimPrefix(n, "pilots/"), ".")[0]
			}
			if !validPilotID(owner) {
				return fmt.Errorf("invalid historical owner for %s", n)
			}
			// Deleted pilots have no authoritative slot. Retain their namespaced
			// receipts in a stable storage bucket without assigning roster membership.
			if _, ok := p.home[owner]; !ok {
				digest := sha256.Sum256([]byte(owner))
				p.home[owner] = int(digest[0]) % len(p.slots)
			}
			if strings.HasSuffix(n, ".backup.json") {
				if _, e = decodePilot(b, owner); e != nil {
					return e
				}
			}
			p.owners[n] = owner
			value := b
			changes[n] = &value
		}
		info, _ := json.MarshalIndent(retire, "", "  ")
		if e = plainDir(filepath.Join(p.dir, careerBackup)); e != nil {
			return e
		}
		if e = writeSaveAtomic(filepath.Join(p.dir, careerBackup), "retire-pending.json", info); e != nil {
			return e
		}
		if e = p.publish(changes, true); e != nil {
			return e
		}
	}
	if e := p.refresh(); e != nil {
		return e
	}
	s.packed = p
	// Validate the reconstructed roster, all referenced results and immutable imports.
	if e := s.loadWithSessions(startup); e != nil {
		return e
	}
	if e := retireLoose(p.dir); e != nil {
		return e
	}
	if e := p.cleanCareerBackup(); e != nil {
		return e
	}
	if e := plainDir(filepath.Join(p.dir, "pilots")); e != nil {
		return e
	}
	return nil
}
func retireLoose(dir string) error {
	backup := filepath.Join(dir, careerBackup)
	pending := filepath.Join(backup, "retire-pending.json")
	b, e := ReadRegularSave(pending)
	if os.IsNotExist(e) {
		if _, e = os.Lstat(filepath.Join(dir, saveFilename)); !os.IsNotExist(e) {
			return fmt.Errorf("unexpected loose roster beside player packs; preserve conflicting files")
		}
		return nil
	}
	if e != nil {
		return e
	}
	var paths map[string]string
	if e = json.Unmarshal(b, &paths); e != nil {
		return e
	}
	for n, want := range paths {
		if !safeSaveTarget(n) && n != oldSaveFilename {
			return fmt.Errorf("invalid retirement path")
		}
		src := filepath.Join(dir, n)
		dest := filepath.Join(backup, "activated", n)
		h, e := userpack.HashFile(src)
		if os.IsNotExist(e) {
			h, e = userpack.HashFile(dest)
			if e == nil && h == want {
				continue
			}
		}
		if e != nil {
			return e
		}
		if h != want {
			return fmt.Errorf("modified legacy career record %s", n)
		}
		if e = backupParent(backup, dest); e != nil {
			return e
		}
		if _, e = os.Lstat(dest); !os.IsNotExist(e) {
			return fmt.Errorf("occupied retirement path %s", dest)
		}
		if e = userpack.Replace(src, dest); e != nil {
			return e
		}
	}
	// Earlier executables refuse initialization if pilots/ exists without a roster.
	if e = plainDir(filepath.Join(dir, "pilots")); e != nil {
		return e
	}
	return userpack.Replace(pending, filepath.Join(backup, "activated.json"))
}

func (p *careerPacks) cleanCareerBackup() error {
	dir := filepath.Join(p.dir, careerBackup)
	st, e := os.Lstat(dir)
	if os.IsNotExist(e) {
		return nil
	}
	if e != nil {
		return e
	}
	if !st.IsDir() {
		return nil
	}
	current := func(n string) ([]byte, bool) { b, e := p.records.Get(n); return b, e == nil }
	for _, pair := range [][2]string{{"before.json", ""}, {"activated.json", "activated"}} {
		if e = userpack.RetireCopies(dir, pair[0], pair[1], current); e != nil {
			return e
		}
	}
	return removeEmptyLibraryDir(dir)
}
