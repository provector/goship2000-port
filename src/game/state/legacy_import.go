package state

import (
	"bytes"
	"fmt"
	assetdata "gunship2000-assets/game/assets"
	"os"
	"path/filepath"
	"strings"
)

// A configured installation may be the game directory itself or its conventional
// GS2000CD parent. Never search other directories or silently choose between both.
func legacyRosterPath(original string) (string, error) {
	if original == "" {
		return "", nil
	}
	var found []string
	for _, dir := range []string{original, filepath.Join(original, "GS2000CD")} {
		entries, err := os.ReadDir(dir)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return "", err
		}
		for _, v := range entries {
			if !strings.EqualFold(v.Name(), "roster.dat") {
				continue
			}
			if !v.Type().IsRegular() {
				return "", fmt.Errorf("legacy roster must be a regular non-symlink file: %s", v.Name())
			}
			found = append(found, filepath.Join(dir, v.Name()))
		}
	}
	if len(found) > 1 {
		return "", fmt.Errorf("two ROSTER.DAT inputs exist under %s; configure the game directory explicitly", original)
	}
	if len(found) == 0 {
		return "", nil
	}
	return filepath.Abs(found[0])
}

func importLegacyRoster(original string) (SaveState, error) {
	state := EmptySaveState()
	path, err := legacyRosterPath(original)
	if err != nil || path == "" {
		return state, err
	}
	// Record offsets are a supported-release interpretation, never guessed from size alone.
	executable := filepath.Join(filepath.Dir(path), "GS.GS2")
	entries, err := os.ReadDir(filepath.Dir(path))
	if err != nil {
		return state, err
	}
	for _, entry := range entries {
		if strings.EqualFold(entry.Name(), "GS.GS2") {
			executable = filepath.Join(filepath.Dir(path), entry.Name())
			break
		}
	}
	code, err := assetdata.ReadLimitedFile(executable, 16<<20)
	if err != nil {
		return state, fmt.Errorf("legacy import requires supported GS.GS2 beside %s: %w", path, err)
	}
	if !assetdata.IslandIce469085.SourceMatches(assetdata.RoleMainExecutable, assetdata.Hash(code)) {
		return state, fmt.Errorf("legacy ROSTER.DAT import at %s has unsupported GS.GS2 identity; no save was initialized", path)
	}
	raw, err := assetdata.ReadLimitedFile(path, 7*LegacyRecordSize+1)
	if err != nil {
		return state, fmt.Errorf("read legacy roster: %w", err)
	}
	return ParseLegacyRoster(path, raw)
}

func legacyString(record []byte, offset, capacity, limit int, field string) (string, error) {
	data := record[offset : offset+capacity]
	end := bytes.IndexByte(data, 0)
	if end < 0 {
		return "", fmt.Errorf("legacy %s at +%02xh is not NUL-terminated", field, offset)
	}
	value := string(data[:end])
	if err := validSaveText(field, value, limit); err != nil {
		return "", fmt.Errorf("legacy %w", err)
	}
	return value, nil
}

func ParseLegacyRoster(path string, raw []byte) (SaveState, error) {
	state := EmptySaveState()
	if len(raw) != 7*LegacyRecordSize {
		return state, fmt.Errorf("legacy roster %s has %d bytes; supported layout is six 290-byte slots plus one 290-byte recovery block (2030 bytes)", path, len(raw))
	}
	sha := assetdata.Hash(raw)
	state.Legacy = &LegacyImport{SourcePath: path, SourceSHA256: sha, SupportedProfile: assetdata.IslandIce469085.Name, PreservedPath: "imports/" + sha + "/ROSTER.DAT", SelectionPolicy: "Port policy: select the first Active occupied slot in roster order; otherwise no selected pilot. The original persisted no verified selected index.", RawRoster: append([]byte(nil), raw...), RecoveryRecord: append([]byte(nil), raw[6*LegacyRecordSize:]...)}
	for i := 0; i < 6; i++ {
		record := raw[i*LegacyRecordSize : (i+1)*LegacyRecordSize]
		name, err := legacyString(record, 0, 0x34, 35, "pilot name")
		if err != nil {
			return state, fmt.Errorf("roster slot %d: %w", i+1, err)
		}
		if name == "" && record[0x56] == 0 {
			continue
		}
		unit, err := legacyString(record, 0x34, 0x4e-0x34, 25, "squadron")
		if err != nil {
			return state, fmt.Errorf("roster slot %d: %w", i+1, err)
		}
		p := &Pilot{ID: fmt.Sprintf("legacy-%s-slot-%d", sha[:16], i), Name: name, Squadron: unit, Emblem: int(record[0x53]), LegacyRaw: append([]byte(nil), record...)}
		if p.Rank, err = RankFromRaw(int(record[0x54])); err != nil {
			return state, fmt.Errorf("roster slot %d: %w", i+1, err)
		}
		if p.Theatre, err = TheatreFromRaw(int(record[0x64])); err != nil {
			return state, fmt.Errorf("roster slot %d: %w", i+1, err)
		}
		if p.MissionType, err = MissionTypeFromRaw(int(record[0x65])); err != nil {
			return state, fmt.Errorf("roster slot %d: %w", i+1, err)
		}
		switch record[0x56] {
		case 0:
			p.Status = StatusInactive
		case 1:
			p.Status = StatusActive
		case 2:
			p.Status = StatusRetired
			p.LegacyStatus = 2
		case 3:
			p.Status = StatusMIA
			p.LegacyStatus = 3
		case 4:
			p.Status = StatusKIA
			p.LegacyStatus = 4
		default:
			p.Status = StatusLegacyUnknown
			p.LegacyStatus = record[0x56]
		}
		p.Statistics = statisticsFromLegacy(record)
		state.Slots[i] = p
		if state.SelectedPilotID == "" && p.Status == StatusActive {
			state.SelectedPilotID = p.ID
		}
	}
	if err := state.Validate(); err != nil {
		return state, fmt.Errorf("legacy roster %s: %w", path, err)
	}
	return state, nil
}

func preserveLegacyImport(directory string, legacy LegacyImport) error {
	dir := filepath.Join(directory, "imports")
	for _, next := range []string{dir, filepath.Join(dir, legacy.SourceSHA256)} {
		if info, err := os.Lstat(next); err == nil && (!info.IsDir() || info.Mode()&os.ModeSymlink != 0) {
			return fmt.Errorf("legacy import destination must be a non-symlink directory: %s", next)
		}
		if err := os.MkdirAll(next, 0700); err != nil {
			return err
		}
	}
	dir = filepath.Join(dir, legacy.SourceSHA256)
	provenance := struct {
		SourcePath string `json:"source_path"`
		SHA256     string `json:"sha256"`
		Bytes      int    `json:"bytes"`
		Profile    string `json:"profile"`
		Operation  string `json:"operation"`
	}{legacy.SourcePath, legacy.SourceSHA256, len(legacy.RawRoster), legacy.SupportedProfile, "Read-only import; exact original bytes retained, including seventh recovery block and unimplemented fields. Never written back to the original."}
	meta, err := assetdata.JsonBytes(provenance)
	if err != nil {
		return err
	}
	for name, data := range map[string][]byte{"ROSTER.DAT": legacy.RawRoster, "provenance.json": meta} {
		path := filepath.Join(dir, name)
		if info, err := os.Lstat(path); err == nil {
			if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
				return fmt.Errorf("invalid existing immutable import %s", path)
			}
			existing, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			if !bytes.Equal(existing, data) {
				return fmt.Errorf("immutable import already exists with different bytes: %s", path)
			}
			continue
		} else if !os.IsNotExist(err) {
			return err
		}
		if err = writeNewSaveFile(path, data); err != nil {
			return fmt.Errorf("preserve immutable legacy import: %w", err)
		}
	}
	return nil
}
