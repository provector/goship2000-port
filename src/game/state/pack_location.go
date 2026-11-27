package state

import (
	"encoding/json"
	"errors"
	"fmt"
	"gunship2000-assets/internal/playerdirs"
	"os"
	"path/filepath"

	"gunship2000-assets/internal/userpack"
)

// This receipt switches all six slot images together. Its hashes describe only
// the retired generation. Current packs can advance independently of them.
const packLocation = ".pilot-pack-location.json"
const pendingLocation = ".relocating-pilot-packs.json"

type packLocationRecord struct {
	Store    string
	Original [6]string
}

func readPackLocation(root string) (packLocationRecord, error) {
	return readLocationRecord(root, packLocation)
}
func readLocationRecord(root, name string) (packLocationRecord, error) {
	var loc packLocationRecord
	raw, err := ReadRegularSave(filepath.Join(root, name))
	if err != nil {
		return loc, err
	}
	if json.Unmarshal(raw, &loc) != nil || !validStoreID(loc.Store) {
		return loc, fmt.Errorf("invalid career location receipt in %s", root)
	}
	for _, h := range loc.Original {
		if h != "" && !validSortieHash(h) {
			return loc, fmt.Errorf("invalid retired pack hash in %s", root)
		}
	}
	return loc, nil
}
func (p *careerPacks) recoverTransaction() error {
	raw, e := ReadRegularSave(filepath.Join(p.dir, packIntent))
	if os.IsNotExist(e) {
		return nil
	}
	if e != nil {
		return e
	}
	var j packJournal
	if e = json.Unmarshal(raw, &j); e != nil {
		return e
	}
	return userpack.Locked(p.cfg, func() error { return p.rollForward(j) })
}
func retireRootPacks(root string, loc packLocationRecord) {
	if pending, err := readLocationRecord(root, pendingLocation); err == nil && pending.Store == loc.Store {
		if err = os.Remove(filepath.Join(root, pendingLocation)); err != nil {
			fmt.Fprintln(os.Stderr, "Retained completed relocation record:", err)
		}
	}

	for i, want := range loc.Original {
		if want == "" {
			continue
		}
		path := filepath.Join(root, playerPack(i))
		got, e := userpack.HashFile(path)
		if os.IsNotExist(e) {
			continue
		}
		if e == nil && got == want {
			e = os.Remove(path)
		} else if e == nil {
			e = errors.New("contents differ from the migrated copy")
		}
		if e != nil {
			fmt.Fprintf(os.Stderr, "Retaining inactive career file %s: %v\n", path, e)
		}
	}
}

// Record which initially absent destination files this copy owns. If a write
// fails and the player continues on the old layout, the next attempt may
// replace its own outdated copies, but never a different pre-existing file.
func planPackCopies(old *careerPacks, dst string) (packLocationRecord, error) {
	loc := packLocationRecord{Store: old.id}
	pending, err := readLocationRecord(old.dir, pendingLocation)
	if err != nil && !os.IsNotExist(err) {
		return loc, err
	}
	if pending.Store != "" && pending.Store != old.id {
		return loc, fmt.Errorf("different career owns the previous relocation")
	}
	next := packLocationRecord{Store: old.id}
	for i := range loc.Original {
		src := filepath.Join(old.dir, playerPack(i))
		target := filepath.Join(dst, playerPack(i))
		want, e := userpack.HashFile(src)
		if e != nil {
			return loc, e
		}
		got, e := userpack.HashFile(target)
		switch {
		case os.IsNotExist(e):
			next.Original[i] = want
		case e != nil:
			return loc, e
		case got == want:
			if got == pending.Original[i] {
				next.Original[i] = want
			}
		case got == pending.Original[i]:
			if e = os.Remove(target); e != nil {
				return loc, e
			}
			next.Original[i] = want
		default:
			return loc, fmt.Errorf("different existing destination %s", target)
		}
		loc.Original[i] = want
	}
	raw, err := json.Marshal(next)
	if err != nil {
		return loc, err
	}
	return loc, writeSaveAtomic(old.dir, pendingLocation, raw)
}

// Resolve an existing career only. Both callers hold the application/store
// lease. Discovery must never import an original roster or create defaults.
func preparePackLocation(root, cfg string) (*careerPacks, error) {
	dst := filepath.Join(root, "pilots")
	old := &careerPacks{dir: root, cfg: cfg, root: root}
	next := &careerPacks{dir: dst, cfg: cfg, root: root}
	loc, e := readPackLocation(root)
	if e == nil {
		next.id = loc.Store
		if err := requirePackDirectory(dst); err != nil {
			return nil, err
		}
		if e = next.recoverTransaction(); e != nil {
			return nil, e
		}
		if e = next.refresh(); e != nil {
			return nil, fmt.Errorf("committed career at %s: %w", dst, e)
		}
		// A preceding attempt may have published the receipt before its directory sync failed.
		if e = userpack.SyncDir(root); e != nil {
			return nil, e
		}
		retireRootPacks(root, loc)
		return next, nil
	}
	if !os.IsNotExist(e) {
		return nil, e
	}
	hasOld := existingPacks(root)
	if hasOld {
		if e = old.recoverTransaction(); e != nil {
			return nil, e
		}
		if e = old.refresh(); e != nil {
			return nil, e
		}
	} else {
		if _, err := os.Lstat(filepath.Join(root, pendingLocation)); !os.IsNotExist(err) {
			return nil, fmt.Errorf("original career missing during unfinished relocation in %s; restore the original store", root)
		}
		if e = requirePackDirectory(dst); os.IsNotExist(e) {
			return nil, nil
		} else if e != nil {
			return nil, e
		}
		if !existingPacks(dst) {
			return nil, nil
		}
		for _, name := range []string{saveJournal, saveFilename, oldSaveFilename} {
			if _, err := os.Lstat(filepath.Join(root, name)); !os.IsNotExist(err) {
				return nil, fmt.Errorf("uncommitted pilot directory conflicts with loose career %s", name)
			}
		}
		if e = next.recoverTransaction(); e != nil {
			return nil, e
		}
	}
	deferOld := func(err error) (*careerPacks, error) {
		if !hasOld {
			return nil, err
		}
		fmt.Fprintf(os.Stderr, "Career remains active at %s; relocation deferred: %v\n", root, err)
		return old, nil
	}
	if e = plainDir(dst); e != nil {
		return deferOld(e)
	}
	if hasOld {
		loc, e = planPackCopies(old, dst)
		if e != nil {
			return deferOld(e)
		}
		for i, hash := range loc.Original {
			name := playerPack(i)
			if err := copyChecked(filepath.Join(root, name), filepath.Join(dst, name), hash); err != nil {
				return deferOld(err)
			}
		}
		next.id = old.id
	}
	if e = next.validatePacks(); e != nil {
		return deferOld(e)
	}
	probe := SaveStore{Directory: root, packed: next}
	if e = probe.loadWithSessions(false); e != nil {
		return deferOld(e)
	}
	loc.Store = next.id
	if e = userpack.SyncDir(dst); e != nil {
		return deferOld(e)
	}
	raw, e := json.Marshal(loc)
	if e != nil {
		return nil, e
	}
	if e = writeSaveAtomic(root, packLocation, raw); e != nil {
		// Rename may have succeeded before fsync failed. Never resume the old writer then.
		if _, stat := os.Lstat(filepath.Join(root, packLocation)); !os.IsNotExist(stat) {
			return nil, e
		}
		return deferOld(e)
	}
	retireRootPacks(root, loc)
	return next, nil
}
func requirePackDirectory(dir string) error {
	st, e := os.Lstat(dir)
	if e != nil {
		return e
	}
	if !st.IsDir() {
		return fmt.Errorf("invalid pilot directory %s", dir)
	}
	return nil
}
func PrepareExistingCareer(directory, data string) error {
	careerWrites.Lock()
	defer careerWrites.Unlock()
	if err := playerdirs.Directory(playerdirs.Sibling(directory, "temp")); err != nil {
		fmt.Fprintf(os.Stderr, "Optional work directory unavailable: %v\n", err)
	}
	packed, e := preparePackLocation(directory, filepath.Join(data, userpack.ConfigName))
	if e == nil && packed == nil {
		// Only upgrade positively identified loose careers here. SETUP and
		// startup before preparation must never manufacture an empty roster.
		if e = resumeSaveCommit(directory); e != nil {
			return e
		}
		for _, name := range []string{saveFilename, oldSaveFilename} {
			if _, err := os.Lstat(filepath.Join(directory, name)); os.IsNotExist(err) {
				continue
			} else if err != nil {
				return err
			}
			store := SaveStore{Directory: directory}
			e = store.openPacked(data, "", "", false)
			break
		}
	}
	if e == nil {
		playerdirs.Archive(directory, "imports", false)
	}
	return e
}
