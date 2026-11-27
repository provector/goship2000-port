package configuration

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"gunship2000-assets/internal/userpack"
)

const switchScratch = ".setup-location-copy.tmp"
const switchRecord = ".setup-location-transfer.json"

var ErrExistingLocation = errors.New("this location already contains player data")

type locationChoice struct {
	Version int
	Path    string
}

func locatorPath() (string, error) {
	dir, e := os.UserConfigDir()
	if e != nil {
		return "", e
	}
	return filepath.Join(dir, "GoShip 2000", "saves-location.json"), nil
}
func locatedData() (string, bool, error) {
	name, e := locatorPath()
	if e == nil {
		b, err := os.ReadFile(name)
		if err == nil {
			var choice locationChoice
			if json.Unmarshal(b, &choice) != nil || choice.Version != 1 || !filepath.IsAbs(choice.Path) {
				return "", true, fmt.Errorf("invalid Saves Location file %s", name)
			}
			return choice.Path, true, nil
		}
		if !os.IsNotExist(err) {
			return "", true, err
		}
	}
	dir, e := preferredData()
	return dir, false, e
}

// Revalidate the complete source on retry. A previously copied film or pack
// must never supersede a file added or changed since the interrupted switch.
func verifySwitchCopy(src, dst, install string, items []string, tr transfer) error {
	doc, err := LoadAt(filepath.Join(src, Filename), install)
	if err != nil {
		return err
	}
	skipScreens := filepath.Join(SavePath(install, src, doc.Config.Saves), "screenshots")
	for _, item := range items {
		root := filepath.Join(src, item)
		if _, e := os.Lstat(root); os.IsNotExist(e) {
			continue
		}
		e := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				if path == skipScreens {
					return filepath.SkipDir
				}
				return nil
			}
			rel, err := filepath.Rel(src, path)
			if err != nil {
				return err
			}
			want, ok := tr.Files[rel]
			if !ok {
				return fmt.Errorf("new source file %s; retry with an empty destination", rel)
			}
			h, err := userpack.HashFile(path)
			if err != nil {
				return err
			}
			if h != want {
				return fmt.Errorf("source changed during transfer: %s; choose an empty destination", rel)
			}
			return nil
		})
		if e != nil {
			return e
		}
	}
	for n := range tr.Files {
		if _, e := os.Stat(filepath.Join(src, n)); e != nil {
			return e
		}
	}
	excludedDestination := filepath.Join(SavePath(install, dst, doc.Config.Saves), "screenshots")
	return filepath.WalkDir(dst, func(path string, d fs.DirEntry, err error) error {
		if err == nil && d.IsDir() && path == excludedDestination {
			return filepath.SkipDir
		}
		if err != nil || d.IsDir() {
			return err
		}
		n, err := filepath.Rel(dst, path)
		if err != nil {
			return err
		}
		if n != switchRecord && !(n == saveOriginName && tr.External != nil) {
			if _, ok := tr.Files[n]; !ok {
				return fmt.Errorf("unexpected file at destination: %s", n)
			}
		}
		return nil
	})
}

// SwitchData is called only during Save & Exit, with the source and locator
// owned. Keep the old store independently usable until and after publication.
func (l *Location) SwitchData(path string, confirmed bool, save func() error, validate func(dir, config string) error) error {
	if !filepath.IsAbs(path) {
		return fmt.Errorf("enter an absolute Saves Location")
	}
	dst, e := ResolvedNewPath(path)
	if e != nil {
		return e
	}
	if dst == l.Data {
		return save()
	}
	if Within(l.Data, dst) || Within(dst, l.Data) {
		return fmt.Errorf("select a folder separate from the current player data")
	}
	loc, e := locatorPath()
	if e != nil {
		return e
	}
	if Within(loc, dst) {
		return fmt.Errorf("choose a separate player-data folder")
	}
	doc, e := LoadAt(filepath.Join(l.Data, Filename), l.Install)
	if e != nil {
		return e
	}
	for _, p := range []string{doc.Config.Original, doc.Config.Assets} {
		protected, e := ResolvedNewPath(Relative(l.Install, p))
		if e != nil {
			return e
		}
		if Within(dst, protected) || Within(protected, dst) {
			return fmt.Errorf("choose a folder outside original and prepared assets")
		}
	}
	if !Within(dst, l.Install) {
		owner, e := OwnOutput(dst)
		if e != nil {
			return e
		}
		defer owner.Close()
	}
	if e = os.MkdirAll(dst, 0700); e != nil {
		return e
	}
	if e = probeWritable(dst); e != nil {
		return fmt.Errorf("Saves Location is not writable: %w", e)
	}
	entries, e := os.ReadDir(dst)
	if e != nil {
		return e
	}
	_, pending := os.Stat(filepath.Join(dst, switchRecord))
	if pending != nil && !os.IsNotExist(pending) {
		return pending
	}
	copyStore := len(entries) == 0 || pending == nil
	if !copyStore {
		if _, err := os.Lstat(filepath.Join(dst, transferRecord)); !os.IsNotExist(err) {
			return fmt.Errorf("unfinished installation migration; finish recovery at %s first", dst)
		}
		d, e := LoadAt(filepath.Join(dst, Filename), l.Install)
		if e != nil {
			return fmt.Errorf("destination must be empty or contain a complete store: %w", e)
		}
		saves, e := ResolvedNewPath(SavePath(l.Install, dst, d.Config.Saves))
		if e != nil {
			return e
		}
		active, e := ResolvedNewPath(SavePath(l.Install, l.Data, doc.Config.Saves))
		if e != nil {
			return e
		}
		if !Within(saves, dst) && !Within(saves, l.Install) && !Within(saves, active) {
			owner, e := OwnOutput(saves)
			if e != nil {
				return e
			}
			defer owner.Close()
		}
		if e = ValidateStorePaths(l.Install, dst, d.Config); e != nil {
			return e
		}
		// Probe the career directory only after checking the directory's recorded
		// ownership. Never create a missing career root during validation.
		original, e := ResolvedNewPath(Relative(l.Install, d.Config.Original))
		if e != nil {
			return e
		}
		if Within(saves, original) || Within(original, saves) {
			return fmt.Errorf("career path overlaps original installation")
		}
		if e = validate(saves, d.Path); e != nil {
			return fmt.Errorf("incomplete destination; open it to recover or choose an empty folder: %w", e)
		}
		if e = probeWritable(saves); e != nil {
			return fmt.Errorf("career folder is not writable: %w", e)
		}
		if !confirmed {
			return ErrExistingLocation
		}
	}
	if e = save(); e != nil {
		return e
	}
	if copyStore {
		if e = transferUserStore(l.Data, dst, l.Install, true); e != nil {
			return e
		}
	}
	if e = os.MkdirAll(filepath.Dir(loc), 0700); e != nil {
		return e
	}
	b, e := json.Marshal(locationChoice{1, dst})
	if e != nil {
		return e
	}
	if e = replaceRecord(loc, b); e != nil {
		return fmt.Errorf("could not publish Saves Location: %w", e)
	}
	if copyStore {
		if e = os.Remove(filepath.Join(dst, switchRecord)); e != nil {
			return fmt.Errorf("location switched; transfer receipt cleanup: %w", e)
		}
	}
	return nil
}
