package configuration

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"gunship2000-assets/game/controllers"
	"gunship2000-assets/internal/gpsref"
	"gunship2000-assets/internal/userpack"
)

func ensureConfig(path, install string) (*Document, bool, error) {
	dir := filepath.Dir(path)
	if _, e := os.Lstat(path); e == nil {
		// Interrupted retirement is finished under the same locks as migration.
		if _, e = os.Lstat(filepath.Join(dir, ".t3-config-pending")); os.IsNotExist(e) {
			for _, n := range []string{"config.cfg", "controls.cfg", "gps_codes.json"} {
				if _, e := os.Lstat(filepath.Join(dir, n)); e == nil {
					return nil, false, fmt.Errorf("unexpected loose %s beside config.g2pak; preserve both and resolve the competing file", n)
				} else if !os.IsNotExist(e) {
					return nil, false, e
				}
			}
			d, e := LoadAt(path, install)
			if e == nil {
				e = cleanConfigBackup(dir)
			}
			return d, false, e
		}
	} else if !os.IsNotExist(e) {
		return nil, false, e
	}
	var created bool
	e := userpack.Locked(path, func() error {
		backup := filepath.Join(dir, ".t3-config-backup")
		pending := filepath.Join(dir, ".t3-config-pending")
		names := map[string]string{"config.cfg": "settings.json", "controls.cfg": "controls.json", "gps_codes.json": "gps_codes.json"}
		if _, e := os.Lstat(path); os.IsNotExist(e) {
			if _, e := os.Lstat(filepath.Join(dir, ".t3-config-migrated.json")); e == nil {
				return fmt.Errorf("missing previously migrated config.g2pak; restore it before continuing")
			} else if !os.IsNotExist(e) {
				return e
			}
			if _, be := os.Lstat(filepath.Join(backup, "inventory.json")); be == nil {
				if _, pe := os.Lstat(pending); os.IsNotExist(pe) {
					raw, e := os.ReadFile(filepath.Join(backup, "inventory.json"))
					if e != nil {
						return e
					}
					var inputs map[string]string
					if e = json.Unmarshal(raw, &inputs); e != nil {
						return e
					}
					for n, want := range inputs {
						if _, ok := names[n]; !ok {
							return fmt.Errorf("invalid configuration inventory")
						}
						h, e := userpack.HashFile(filepath.Join(dir, n))
						if e != nil || h != want {
							return fmt.Errorf("missing migrated config.g2pak; preserve backup and restore before continuing")
						}
					}
				}
			}
			settings, _ := json.MarshalIndent(DefaultConfig(), "", "  ")
			controls, _ := json.MarshalIndent(controllers.Defaults(), "", "  ")
			entries := userpack.Entries{"format.json": userpack.Value([]byte("{\"version\":1}\n")), "settings.json": userpack.Value(settings), "controls.json": userpack.Value(controls), "gps_codes.json": userpack.Value(gpsref.Default)}
			inventory := map[string]string{}
			for n, dest := range names {
				src := filepath.Join(dir, n)
				st, e := os.Lstat(src)
				if os.IsNotExist(e) {
					continue
				}
				if e != nil {
					return e
				}
				if !st.Mode().IsRegular() || st.Size() > MaxConfigBytes {
					return fmt.Errorf("invalid legacy %s", n)
				}
				b, e := os.ReadFile(src)
				if e != nil {
					return e
				}
				if n == "config.cfg" {
					doc, e := decodeSettingsAt(src, install, b)
					if e != nil {
						return e
					}
					oldRoot, e := ResolvedNewPath(Relative(dir, doc.Config.Saves))
					if e != nil {
						return e
					}
					orig, e := ResolvedNewPath(Relative(install, doc.Config.Original))
					if e != nil {
						return e
					}
					if Within(oldRoot, orig) || Within(orig, oldRoot) {
						return fmt.Errorf("save directory overlaps original installation")
					}

				}
				if n == "controls.cfg" {
					if _, e = controllers.Load(src); e != nil {
						return e
					}
				}
				entries[dest] = userpack.Value(b)
				inventory[n] = fmt.Sprintf("%x", sha256.Sum256(b))
			}
			if len(inventory) > 0 {
				if e := os.Mkdir(backup, 0700); e != nil && !os.IsExist(e) {
					return e
				}
				st, e := os.Lstat(backup)
				if e != nil || !st.IsDir() {
					return fmt.Errorf("invalid configuration rollback directory")
				}
				b, _ := json.MarshalIndent(inventory, "", "  ")
				if e := safeRecord(filepath.Join(backup, "inventory.json"), b); e != nil {
					return e
				}
				if e := safeRecord(pending, b); e != nil {
					return e
				}
			}
			stage, e := userpack.Stage(dir, entries)
			if e != nil {
				return e
			}
			defer os.Remove(stage)
			if e = userpack.Replace(stage, path); e != nil {
				return e
			}
			created = true
		} else if e != nil {
			return e
		}
		if _, e := LoadAt(path, install); e != nil {
			return e
		}
		b, e := os.ReadFile(pending)
		if os.IsNotExist(e) {
			return nil
		}
		if e != nil {
			return e
		}
		var inventory map[string]string
		if e = json.Unmarshal(b, &inventory); e != nil {
			return e
		}
		for n, want := range inventory {
			entry, ok := names[n]
			if !ok {
				return fmt.Errorf("unknown migration input %q", n)
			}
			packed, e := userpack.ReadConfig(dir, entry)
			if e != nil || fmt.Sprintf("%x", sha256.Sum256(packed)) != want {
				return fmt.Errorf("configuration migration conflict on %s", n)
			}
			if e = retireConfigFile(filepath.Join(dir, n), filepath.Join(backup, n), want); e != nil {
				return e
			}
		}
		if e = os.Remove(pending); e != nil {
			return e
		}
		return userpack.SyncDir(dir)
	})
	if e != nil {
		return nil, false, e
	}
	d, e := LoadAt(path, install)
	if e == nil {
		e = cleanConfigBackup(dir)
	}
	return d, created, e
}
func safeRecord(path string, b []byte) error {
	if v, e := os.ReadFile(path); e == nil {
		if !bytes.Equal(v, b) {
			return fmt.Errorf("existing migration record differs: %s", path)
		}
		return nil
	} else if !os.IsNotExist(e) {
		return e
	}
	return replaceRecord(path, b)
}
func replaceRecord(path string, b []byte) error {
	f, e := os.CreateTemp(filepath.Dir(path), ".inventory-")
	if e != nil {
		return e
	}
	defer os.Remove(f.Name())
	if _, e = f.Write(b); e == nil {
		e = f.Sync()
	}
	ce := f.Close()
	if e != nil {
		return e
	}
	if ce != nil {
		return ce
	}
	return userpack.Replace(f.Name(), path)
}
func retireConfigFile(src, dest, hash string) error {
	h, e := userpack.HashFile(src)
	if os.IsNotExist(e) {
		h, e = userpack.HashFile(dest)
		if e == nil && h == hash {
			return nil
		}
	}
	if e != nil {
		return e
	}
	if h != hash {
		return fmt.Errorf("migration source changed: %s", src)
	}
	if _, e = os.Lstat(dest); !os.IsNotExist(e) {
		return fmt.Errorf("occupied backup: %s", dest)
	}
	return userpack.Replace(src, dest)
}

func (d *Document) SaveTogether(ctrl *controllers.Document) error {
	if e := d.Config.Validate(); e != nil {
		return e
	}
	if e := ValidateStorePaths(d.InstallDirectory(), filepath.Dir(d.Path), d.Config); e != nil {
		return e
	}
	var all map[string]json.RawMessage
	var b []byte
	var control userpack.Change
	e := userpack.Locked(d.Path, func() error {
		old, e := userpack.ReadConfig(filepath.Dir(d.Path), "settings.json")
		if e != nil {
			return e
		}
		if sha256.Sum256(old) != d.fingerprint {
			return ErrChanged
		}
		knownRaw, e := json.Marshal(d.Config)
		if e != nil {
			return e
		}
		var known map[string]json.RawMessage
		if e = json.Unmarshal(knownRaw, &known); e != nil {
			return e
		}
		all = merge(d.raw, known)
		b, e = json.MarshalIndent(all, "", "  ")
		if e != nil {
			return e
		}
		b = append(b, '\n')
		if len(b) > MaxConfigBytes {
			return errors.New("settings exceed 1 MiB")
		}
		cs := []userpack.Change{{Name: "settings.json", Before: old, After: b}}
		if ctrl != nil {
			if ctrl.Path != d.Path {
				return errors.New("settings and controls pack paths differ")
			}
			control, e = ctrl.PackedChange()
			if e != nil {
				return e
			}
			cs = append(cs, control)
		}
		return userpack.PatchLocked(d.Path, cs)
	})
	if e != nil {
		return e
	}
	d.raw = all
	d.fingerprint = sha256.Sum256(b)
	if ctrl != nil {
		ctrl.PackedSaved(control.After)
	}
	return nil
}

func cleanConfigBackup(dir string) error {
	backup := filepath.Join(dir, ".t3-config-backup")
	st, e := os.Lstat(backup)
	if os.IsNotExist(e) {
		return nil
	}
	if e != nil {
		return e
	}
	if !st.IsDir() {
		return nil
	}
	if _, e = os.Lstat(filepath.Join(dir, ".t3-config-pending")); !os.IsNotExist(e) {
		return e
	}
	// Keep the missing-pack guard after retiring the bulk rollback copies.
	if _, e = os.Lstat(filepath.Join(backup, "inventory.json")); e == nil {
		if e = safeRecord(filepath.Join(dir, ".t3-config-migrated.json"), []byte("{\"version\":1}\n")); e != nil {
			return e
		}
		if e = userpack.SyncDir(dir); e != nil {
			return e
		}
	} else if !os.IsNotExist(e) {
		return e
	}
	entries := map[string]string{"config.cfg": "settings.json", "controls.cfg": "controls.json", "gps_codes.json": "gps_codes.json"}
	if e = userpack.RetireCopies(backup, "inventory.json", "", func(n string) ([]byte, bool) {
		key, ok := entries[n]
		if !ok {
			return nil, false
		}
		b, e := userpack.ReadConfig(dir, key)
		return b, e == nil
	}); e != nil {
		return e
	}
	files, e := os.ReadDir(backup)
	if e != nil {
		return e
	}
	if len(files) == 0 {
		return os.Remove(backup)
	}
	return nil
}
