package state

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Unfinished records are optional UI/session state, not recovery snapshots or
// durable result commits. Moving their exact bytes is restart-idempotent: if a
// process stops between the record and reference moves, the next open detaches
// the dangling half. No canonical pilot/roster file participates in these moves.
const abandonedSessionDirectory = "abandoned-sessions"

func (s *SaveStore) quarantineSessionFile(name, reason string) error {
	path := filepath.Join(s.Directory, filepath.FromSlash(name))
	info, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("optional session file must be regular: %s", name)
	}
	dir := filepath.Join(s.Directory, abandonedSessionDirectory)
	if info, err := os.Lstat(dir); err == nil && (!info.IsDir() || info.Mode()&os.ModeSymlink != 0) {
		return fmt.Errorf("session archive must be a non-symlink directory")
	} else if err != nil && !os.IsNotExist(err) {
		return err
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	var token [16]byte
	if _, err := rand.Read(token[:]); err != nil {
		return err
	}
	// Names retain the old stable identity; random archive suffixes use OS
	// entropy only and never touch the authoritative gameplay RNG stream.
	archive := filepath.Base(name) + "." + hex.EncodeToString(token[:])
	if err := os.Rename(path, filepath.Join(dir, archive)); err != nil {
		return fmt.Errorf("archive unfinished session %s: %w", name, err)
	}
	// The moved bytes are already safely detached even if writing this optional
	// human-readable explanation is interrupted. Startup does not consume it.
	return os.WriteFile(filepath.Join(dir, archive+".txt"), []byte(reason+"\nOriginal path: "+name+"\n"), 0600)
}

func sessionEntries(directory string) ([]os.DirEntry, error) {
	info, err := os.Lstat(directory)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return nil, fmt.Errorf("session payload directory must not be a symlink")
	}
	return os.ReadDir(directory)
}

func (s *SaveStore) quarantineUnfinishedSorties(reason string) error {
	entries, err := sessionEntries(filepath.Join(s.Directory, "sorties"))
	if err != nil {
		return err
	}
	committed := map[string]string{}
	for _, entry := range entries {
		name := entry.Name()
		if strings.HasPrefix(name, ".save-pending-") && strings.HasSuffix(name, ".json") && entry.Type().IsRegular() {
			continue // Never import a failed atomic-write temporary as a result.
		}
		id := strings.TrimSuffix(name, ".json")
		path := "sorties/" + name
		if validSortieID(id) && name == id+".json" {
			raw, readErr := ReadRegularSave(filepath.Join(s.Directory, filepath.FromSlash(path)))
			if readErr == nil {
				doc, decodeErr := decodeSortieRecord(raw, id)
				if decodeErr == nil && doc.Phase == SortieCommitted {
					committed[id] = doc.Start.PilotID
					continue // Consumed identities survive Phoenix and all restarts.
				}
			}
		}
		// Corrupt/missing optional state never manufactures a result or blocks an
		// otherwise valid pilot. Preserve even oversized regular files by rename.
		if err := s.quarantineSessionFile(path, reason); err != nil {
			return err
		}
	}
	entries, err = sessionEntries(filepath.Join(s.Directory, "pilots"))
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if !strings.HasSuffix(entry.Name(), ".sortie.json") {
			continue
		}
		id := strings.TrimSuffix(entry.Name(), ".sortie.json")
		name := "pilots/" + entry.Name()
		raw, readErr := ReadRegularSave(filepath.Join(s.Directory, filepath.FromSlash(name)))
		if readErr == nil && validPilotID(id) {
			ref, decodeErr := decodeSortieReference(raw, id)
			if decodeErr == nil && committed[ref.SortieID] == id {
				continue
			}
		}
		if err := s.quarantineSessionFile(name, reason); err != nil {
			return err
		}
	}
	return nil
}

// AbandonUnfinishedSorties is used on confirmed application quit. Forced process
// loss gets the same cleanup at the next OpenSaveStore. It does not finalize a
// sortie, restore PilotBefore, or undo independently saved settings/declines.
func (s *SaveStore) AbandonUnfinishedSorties(reason string) error {
	return s.Locked(func() error {
		// A final acknowledgement may already have made its journal durable.
		// Complete those exact bytes before deciding which UI records to discard.
		if err := resumeSaveCommit(s.Directory); err != nil {
			return err
		}
		if err := s.quarantine(reason); err != nil {
			return err
		}
		return s.load()
	})
}
