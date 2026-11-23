package state

import (
	"encoding/json"
	"fmt"
	assetdata "gunship2000-assets/game/assets"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

const migrationArchive = "step-c-before-migration"

// Preserve the entire pre-migration directory before publishing any new pilot
// files. A completed archive is verified on retries; a partial staging copy is
// retained for diagnosis rather than being mistaken for a complete backup.
func preserveOldSaveDirectory(directory string) error {
	old, err := ReadRegularSave(filepath.Join(directory, oldSaveFilename))
	if err != nil {
		return err
	}
	archive := filepath.Join(directory, migrationArchive)
	markerPath := filepath.Join(archive, "migration-provenance.json")
	if _, err = os.Lstat(archive); err == nil {
		raw, e := ReadRegularSave(markerPath)
		if e != nil {
			return fmt.Errorf("incomplete migration archive %s: %w", archive, e)
		}
		var marker struct {
			SourceSHA256 string            `json:"source_sha256"`
			Files        map[string]string `json:"files_sha256"`
		}
		if e = json.Unmarshal(raw, &marker); e != nil || marker.SourceSHA256 != assetdata.Hash(old) {
			return fmt.Errorf("migration archive does not match current Step C save; preserve both before repair")
		}
		for name, want := range marker.Files {
			if filepath.IsAbs(name) || strings.Contains(name, "\\") || filepath.Clean(name) == ".." || strings.HasPrefix(filepath.Clean(name), ".."+string(filepath.Separator)) {
				return fmt.Errorf("unsafe migration archive manifest path")
			}
			data, e := ReadRegularSave(filepath.Join(archive, filepath.FromSlash(name)))
			if e != nil || assetdata.Hash(data) != want {
				return fmt.Errorf("migration archive file identity mismatch: %s", name)
			}
		}
		preserved, e := ReadRegularSave(filepath.Join(archive, oldSaveFilename))
		if e != nil || assetdata.Hash(preserved) != assetdata.Hash(old) {
			return fmt.Errorf("migration archive source identity mismatch")
		}
		return nil
	} else if !os.IsNotExist(err) {
		return err
	}
	temp, err := os.MkdirTemp(directory, ".migration-copy-*")
	if err != nil {
		return err
	}
	files := map[string]string{}
	err = filepath.WalkDir(directory, func(path string, d fs.DirEntry, e error) error {
		if e != nil {
			return e
		}
		if path == directory {
			return nil
		}
		rel, e := filepath.Rel(directory, path)
		if e != nil {
			return e
		}
		first := strings.Split(rel, string(filepath.Separator))[0]
		if first == careerLease || first == ".save.lock" || first == migrationArchive || strings.HasPrefix(first, ".migration-copy-") {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		info, e := d.Info()
		if e != nil {
			return e
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("refusing symlink in old save directory: %s", rel)
		}
		target := filepath.Join(temp, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0700)
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("unsupported old save file type: %s", rel)
		}
		raw, e := assetdata.ReadLimitedFile(path, 4<<20)
		if e != nil {
			return e
		}
		if e = writeNewSaveFile(target, raw); e != nil {
			return e
		}
		files[filepath.ToSlash(rel)] = assetdata.Hash(raw)
		return nil
	})
	if err != nil {
		return fmt.Errorf("migration preservation incomplete at %s: %w", temp, err)
	}
	marker := struct {
		SourceSHA256 string            `json:"source_sha256"`
		Files        map[string]string `json:"files_sha256"`
		Description  string            `json:"description"`
	}{assetdata.Hash(old), files, "Complete pre-migration save directory. Old revision backups are archival only, never pre-mission snapshots."}
	raw, err := assetdata.JsonBytes(marker)
	if err != nil {
		return err
	}
	if err = writeNewSaveFile(filepath.Join(temp, "migration-provenance.json"), raw); err != nil {
		return err
	}
	if err = os.Rename(temp, archive); err != nil {
		return fmt.Errorf("publish preserved migration directory: %w", err)
	}
	return nil
}
