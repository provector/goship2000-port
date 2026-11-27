package state

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	assetdata "gunship2000-assets/game/assets"
	"gunship2000-assets/internal/playerdirs"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
)

// Only references live in roster.json. Pilot files are the single authority for
// identity, selections and progression; OfficeState is always a derived view.
type rosterDocument struct {
	SchemaVersion   int              `json:"schema_version"`
	Revision        uint64           `json:"revision"`
	Slots           [6]string        `json:"roster_slots"`
	SelectedPilotID string           `json:"selected_pilot_id,omitempty"`
	Legacy          *legacyReference `json:"legacy_import,omitempty"`
}

type legacyReference struct {
	SourcePath       string `json:"source_path"`
	SourceSHA256     string `json:"source_sha256"`
	SupportedProfile string `json:"supported_profile"`
	PreservedPath    string `json:"preserved_path"`
	SelectionPolicy  string `json:"selection_policy"`
}

type pilotDocument struct {
	SchemaVersion int `json:"schema_version"`
	*Pilot
}

func statisticsFromLegacy(raw []byte) *PilotStatistics {
	if len(raw) != LegacyRecordSize {
		return nil
	}
	word := func(offset int) int32 { return int32(binary.LittleEndian.Uint32(raw[offset : offset+4])) }
	awards := make([]int8, 8)
	for i := range awards {
		awards[i] = int8(raw[0x24+i])
	}
	return &PilotStatistics{AwardCounts: awards, LastMissionScore: word(0x6a), BestMissionScore: word(0x66), CareerScore: word(0x72), MissionCampaignCounter: word(0x6e), PhoenixCounter: word(0xcc)}
}

func validPilotID(id string) bool {
	if id == "" || len(id) > 128 {
		return false
	}
	for _, c := range id {
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '-' || c == '_') {
			return false
		}
	}
	return true
}

func pilotFilename(id string) string { return filepath.ToSlash(filepath.Join("pilots", id+".json")) }

func backupFilename(id string) string {
	return filepath.ToSlash(filepath.Join("pilots", id+".backup.json"))
}

func ReadRegularSave(path string) ([]byte, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
		return nil, fmt.Errorf("save must be a regular non-symlink file: %s", path)
	}
	return assetdata.ReadLimitedFile(path, 4<<20)
}

func decodePilot(raw []byte, id string) (*Pilot, error) {
	var doc pilotDocument
	if err := assetdata.DecodeObject(raw, &doc); err != nil {
		return nil, err
	}
	if doc.SchemaVersion != pilotSchemaVersion || doc.Pilot == nil {
		return nil, fmt.Errorf("unsupported pilot schema %d", doc.SchemaVersion)
	}
	if doc.Pilot.ID != id {
		return nil, fmt.Errorf("pilot ID %q does not match roster reference %q", doc.Pilot.ID, id)
	}
	// Upgrade only source-witnessed statuses in memory; no load-time pilot write.
	NormalizeKnownPilotStatus(doc.Pilot)
	test := EmptySaveState()
	test.Slots[0] = doc.Pilot
	if err := test.Validate(); err != nil {
		return nil, err
	}
	return doc.Pilot, nil
}

func (s *SaveStore) load() error { return s.loadWithSessions(false) }

func (s *SaveStore) loadWithSessions(startup bool) error {
	if s.packed == nil {
		if info, err := os.Lstat(filepath.Join(s.Directory, "pilots")); err == nil && (!info.IsDir() || info.Mode()&os.ModeSymlink != 0) {
			return fmt.Errorf("pilots must be a non-symlink directory")
		} else if err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	raw, err := s.readRecord(saveFilename)
	if err != nil {
		return err
	}
	var doc rosterDocument
	if err = assetdata.DecodeObject(raw, &doc); err != nil {
		return fmt.Errorf("invalid %s: %w; no progress was reset", saveFilename, err)
	}
	if doc.SchemaVersion != rosterSchemaVersion {
		return fmt.Errorf("roster schema %d is unsupported (expected %d)", doc.SchemaVersion, rosterSchemaVersion)
	}
	var fields map[string]json.RawMessage
	_ = json.Unmarshal(raw, &fields)
	var slots []string
	if err = json.Unmarshal(fields["roster_slots"], &slots); err != nil || len(slots) != 6 {
		return fmt.Errorf("roster_slots requires exactly six ordered stable-ID slots")
	}
	state := EmptySaveState()
	state.Revision = doc.Revision
	state.SelectedPilotID = doc.SelectedPilotID
	files := map[string][]byte{saveFilename: raw}
	for i, id := range doc.Slots {
		if id == "" {
			continue
		}
		if !validPilotID(id) {
			return fmt.Errorf("roster slot %d has unsafe pilot identifier %q", i+1, id)
		}
		path := pilotFilename(id)
		data, err := s.readRecord(path)
		if err != nil {
			return fmt.Errorf("load pilot referenced by slot %d (%s): %w", i+1, id, err)
		}
		p, err := decodePilot(data, id)
		if err != nil {
			return fmt.Errorf("load pilot %s: %w", id, err)
		}
		state.Slots[i] = p
		files[path] = data
	}
	if doc.Legacy != nil {
		l := doc.Legacy
		if decoded, e := hex.DecodeString(l.SourceSHA256); e != nil || len(decoded) != 32 {
			return fmt.Errorf("invalid legacy import source hash")
		}
		var data []byte
		if s.packed != nil {
			data = s.packed.importRoster
		} else {
			var e error
			data, e = readArchivedRoster(s.Directory, l)
			if e != nil {
				return e
			}
		}
		if len(data) != 7*LegacyRecordSize || assetdata.Hash(data) != l.SourceSHA256 {
			return fmt.Errorf("immutable legacy import identity mismatch")
		}
		state.Legacy = &LegacyImport{l.SourcePath, l.SourceSHA256, l.SupportedProfile, l.PreservedPath, l.SelectionPolicy, data, append([]byte(nil), data[6*LegacyRecordSize:]...)}
	}
	if err = state.Validate(); err != nil {
		return fmt.Errorf("invalid roster: %w", err)
	}
	s.state = state
	s.raw = raw
	s.files = files
	if startup {
		// The durable career journal was recovered before this load. Optional
		// unfinished flight/debrief records never resume or roll back pilots.
		if err := s.quarantine("application restarted before final acknowledgement"); err != nil {
			return err
		}
	}
	return s.loadSortieFiles()
}

// The loose importer reads the seven-record source once. Managed stores carry
// those verified bytes with their shared provenance, independent of the archive.
func readArchivedRoster(directory string, l *legacyReference) ([]byte, error) {
	if decoded, e := hex.DecodeString(l.SourceSHA256); e != nil || len(decoded) != 32 {
		return nil, fmt.Errorf("invalid roster digest")
	}
	if l.PreservedPath != "imports/"+l.SourceSHA256+"/ROSTER.DAT" {
		return nil, fmt.Errorf("legacy import reference path is not canonical")
	}
	var last error = os.ErrNotExist
	for _, archive := range playerdirs.ArchiveReaders(directory, "imports") {
		parent := filepath.Join(archive, l.SourceSHA256)
		if err := requirePackDirectory(parent); err != nil {
			last = err
			continue
		}
		b, err := ReadRegularSave(filepath.Join(parent, "ROSTER.DAT"))
		if err == nil && len(b) == 7*LegacyRecordSize && assetdata.Hash(b) == l.SourceSHA256 {
			return b, nil
		}
		if err == nil {
			err = fmt.Errorf("archived roster digest differs at %s", parent)
		}
		last = err
	}
	return nil, fmt.Errorf("required archived roster %s: %w", l.SourceSHA256, last)
}
func stateFiles(state SaveState) (map[string][]byte, error) {
	doc := rosterDocument{SchemaVersion: rosterSchemaVersion, Revision: state.Revision, SelectedPilotID: state.SelectedPilotID}
	if l := state.Legacy; l != nil {
		doc.Legacy = &legacyReference{l.SourcePath, l.SourceSHA256, l.SupportedProfile, l.PreservedPath, l.SelectionPolicy}
	}
	files := map[string][]byte{}
	for i, p := range state.Slots {
		if p != nil {
			doc.Slots[i] = p.ID
			raw, err := assetdata.JsonBytes(pilotDocument{pilotSchemaVersion, p})
			if err != nil {
				return nil, err
			}
			files[pilotFilename(p.ID)] = raw
		}
	}
	raw, err := assetdata.JsonBytes(doc)
	if err != nil {
		return nil, err
	}
	files[saveFilename] = raw
	return files, nil
}

func (s *SaveStore) Initialize(state SaveState) error {
	if err := state.Validate(); err != nil {
		return err
	}
	if state.Legacy != nil {
		if err := preserveLegacyImport(s.Directory, *state.Legacy); err != nil {
			return err
		}
	}
	files, err := stateFiles(state)
	if err != nil {
		return err
	}
	if err = s.commitRecords(nil, files, nil); err != nil {
		return err
	}
	return s.load()
}

func (s *SaveStore) Commit(next SaveState) error {
	if err := s.guardPendingSortie(); err != nil {
		return err
	}
	if err := next.Validate(); err != nil {
		return err
	}
	if next.Revision != s.state.Revision {
		return fmt.Errorf("save draft revision is stale")
	}
	next.RecoveryBefore = nil
	if reflect.DeepEqual(s.state, next) {
		return nil
	}
	return s.store(next, nil)
}

func (s *SaveStore) store(next SaveState, extra map[string]*[]byte) error {
	return s.storeChecked(next, extra, "", "")
}

func (s *SaveStore) storeChecked(next SaveState, extra map[string]*[]byte, backupName, backupHash string) error {
	return s.Locked(func() error { return s.storeCheckedLocked(next, extra, backupName, backupHash) })
}

// Caller owns Locked. This keeps optional-peer refresh and its final CAS inside
// the same transaction lock, without changing the ordinary store boundary.
func (s *SaveStore) storeCheckedLocked(next SaveState, extra map[string]*[]byte, backupName, backupHash string) error {
	for name, expected := range s.files {
		current, err := s.readRecord(name)
		if err != nil {
			return err
		}
		if !bytes.Equal(current, expected) {
			return fmt.Errorf("save changed outside this session (%s); restart before committing", name)
		}
	}
	if backupName != "" {
		raw, e := s.currentRecord(backupName)
		if e != nil {
			return e
		}
		actual := ""
		if raw != nil {
			actual = assetdata.Hash(raw)
		}
		if actual != backupHash {
			return fmt.Errorf("selected pilot pre-mission backup changed before confirmation")
		}
	}
	if s.state.Revision == ^uint64(0) {
		return fmt.Errorf("save revision exhausted")
	}
	next.Revision = s.state.Revision + 1
	next.RecoveryBefore = nil
	if err := next.Validate(); err != nil {
		return err
	}
	files, err := stateFiles(next)
	if err != nil {
		return err
	}
	// Removing a roster reference never deletes the old pilot file; recovery and
	// migration do not get to erase unrelated pilot state.
	if err = s.commitRecords(s.files, files, extra); err != nil {
		return err
	}
	return s.load()
}

func (s *SaveStore) RecoveryCandidate() (*RecoverySnapshot, error) {
	p := s.state.SelectedPilot()
	if p == nil {
		return nil, nil
	}
	name := backupFilename(p.ID)
	raw, err := s.readRecord(name)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	backed, err := decodePilot(raw, p.ID)
	if err != nil {
		return nil, fmt.Errorf("invalid pre-mission backup: %w", err)
	}
	state := s.State()
	for i, slot := range state.Slots {
		if slot != nil && slot.ID == p.ID {
			state.Slots[i] = backed
		}
	}
	return &RecoverySnapshot{ID: p.ID + ":" + assetdata.Hash(raw), State: state}, nil
}

// PrepareMissionSnapshot is an explicit development/legacy snapshot operation.
// Normal mission entry never calls it: final acknowledgement owns Phoenix.
func (s *SaveStore) PrepareMissionSnapshot() error {
	if err := s.guardPendingSortie(); err != nil {
		return err
	}
	p := s.state.SelectedPilot()
	if p == nil {
		return fmt.Errorf("cannot snapshot without a selected pilot")
	}
	if p.Status != StatusActive {
		return fmt.Errorf("cannot snapshot a pilot that is not active")
	}
	if p.Statistics == nil {
		return fmt.Errorf("cannot snapshot unknown pilot statistics/Phoenix state")
	}
	return s.Locked(func() error {
		for name, raw := range s.files {
			actual, err := s.readRecord(name)
			if err != nil {
				return err
			}
			if !bytes.Equal(actual, raw) {
				return fmt.Errorf("save changed outside this session; restart before snapshotting")
			}
		}
		raw, err := assetdata.JsonBytes(pilotDocument{pilotSchemaVersion, p})
		if err != nil {
			return err
		}
		return s.commitRecords(s.files, s.files, map[string]*[]byte{backupFilename(p.ID): &raw})
	})
}

func (s *SaveStore) Restore(snapshotID string) error {
	if err := s.guardPendingSortie(); err != nil {
		return err
	}
	candidate, err := s.RecoveryCandidate()
	if err != nil {
		return err
	}
	if candidate == nil || candidate.ID != snapshotID {
		return fmt.Errorf("selected pilot pre-mission backup is unavailable or has changed")
	}
	next := s.State()
	backed := candidate.State.SelectedPilot()
	for i, p := range next.Slots {
		if p != nil && p.ID == backed.ID {
			next.Slots[i] = backed
		}
	}
	// Original recovery restores the saved block, SUB/SBB 1 from its +cch signed
	// accumulator, then consumes the snapshot. Preserve 32-bit wrap semantics.
	if backed.Statistics == nil {
		return fmt.Errorf("cannot recover pilot with unknown Phoenix/statistics state")
	}
	backed.Statistics.PhoenixCounter = int32(uint32(backed.Statistics.PhoenixCounter) - 1)
	return s.storeChecked(next, map[string]*[]byte{backupFilename(backed.ID): nil}, backupFilename(backed.ID), strings.TrimPrefix(candidate.ID, backed.ID+":"))
}

type saveOperation struct {
	Path   string `json:"path"`
	Before string `json:"before_sha256,omitempty"`
	After  []byte `json:"after_base64,omitempty"`
	Remove bool   `json:"remove,omitempty"`
}

type saveCommit struct {
	SchemaVersion int             `json:"schema_version"`
	Operations    []saveOperation `json:"operations"`
}

const saveJournal = ".pending-commit.json"

func safeSaveTarget(name string) bool {
	if name == saveFilename {
		return true
	}
	if isSortieTarget(name) || isSortieReferenceTarget(name) {
		return true
	}
	if !strings.HasPrefix(name, "pilots/") || strings.Contains(name, "\\") {
		return false
	}
	base := strings.TrimPrefix(name, "pilots/")
	if strings.HasSuffix(base, ".backup.json") {
		return validPilotID(strings.TrimSuffix(base, ".backup.json"))
	}
	if strings.HasSuffix(base, ".json") {
		return validPilotID(strings.TrimSuffix(base, ".json"))
	}
	return false
}

func currentSaveBytes(directory, name string) ([]byte, error) {
	raw, err := ReadRegularSave(filepath.Join(directory, filepath.FromSlash(name)))
	if os.IsNotExist(err) {
		return nil, nil
	}
	return raw, err
}

func commitSaveFiles(directory string, expected, desired map[string][]byte, extra map[string]*[]byte) error {
	if _, err := os.Lstat(filepath.Join(directory, saveJournal)); err == nil {
		return fmt.Errorf("pending save transaction exists; restart to finish it")
	} else if !os.IsNotExist(err) {
		return err
	}
	targets := map[string]*[]byte{}
	for name, data := range desired {
		if old, ok := expected[name]; !ok || !bytes.Equal(old, data) {
			v := data
			targets[name] = &v
		}
	}
	for name, data := range extra {
		targets[name] = data
	}
	var names []string
	for name := range targets {
		names = append(names, name)
	}
	sort.Strings(names)
	// Publish new roster references after their payloads. Consume recovery only
	// after those payloads/roster are published, preserving the usable backup if
	// an earlier write fails. The retained journal completes a process restart.
	order := func(name string) int {
		if targets[name] == nil {
			return 2
		}
		if name == saveFilename {
			return 1
		}
		return 0
	}
	sort.SliceStable(names, func(i, j int) bool { return order(names[i]) < order(names[j]) })
	journal := saveCommit{SchemaVersion: 1}
	for _, name := range names {
		if !safeSaveTarget(name) {
			return fmt.Errorf("unsafe save transaction path %q", name)
		}
		before, err := currentSaveBytes(directory, name)
		if err != nil {
			return err
		}
		if want, ok := expected[name]; ok && !bytes.Equal(before, want) {
			return fmt.Errorf("save changed outside this session (%s)", name)
		}
		if _, ok := expected[name]; !ok && targets[name] != nil && !strings.HasSuffix(name, ".backup.json") && before != nil {
			return fmt.Errorf("refusing to overwrite unreferenced pilot/roster file %s", name)
		}
		op := saveOperation{Path: name, Remove: targets[name] == nil}
		if before != nil {
			op.Before = assetdata.Hash(before)
		}
		if !op.Remove {
			op.After = *targets[name]
		}
		journal.Operations = append(journal.Operations, op)
	}
	if len(journal.Operations) == 0 {
		return nil
	}
	raw, err := assetdata.JsonBytes(journal)
	if err != nil {
		return err
	}
	if len(raw) > 4<<20 {
		return fmt.Errorf("save transaction exceeds bounded size")
	}
	if err = writeSaveAtomic(directory, saveJournal, raw); err != nil {
		return err
	}
	return resumeSaveCommit(directory)
}

func resumeSaveCommit(directory string) error {
	raw, err := ReadRegularSave(filepath.Join(directory, saveJournal))
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	var journal saveCommit
	if err = assetdata.DecodeObject(raw, &journal); err != nil {
		return fmt.Errorf("invalid pending save transaction: %w", err)
	}
	if journal.SchemaVersion != 1 || len(journal.Operations) > 14 {
		return fmt.Errorf("unsupported pending save transaction")
	}
	seen := map[string]bool{}
	for _, op := range journal.Operations {
		if seen[op.Path] {
			return fmt.Errorf("duplicate path in pending save transaction")
		}
		seen[op.Path] = true
		if op.Before != "" {
			if raw, e := hex.DecodeString(op.Before); e != nil || len(raw) != 32 {
				return fmt.Errorf("invalid pending save prior hash")
			}
		}
		if op.Remove && !strings.HasSuffix(op.Path, ".backup.json") {
			return fmt.Errorf("pending save may consume only a pre-mission backup")
		}
		if !op.Remove {
			if op.Path == saveFilename {
				var doc rosterDocument
				if e := assetdata.DecodeObject(op.After, &doc); e != nil || doc.SchemaVersion != rosterSchemaVersion {
					return fmt.Errorf("invalid roster in pending save transaction")
				}
			} else if isSortieReferenceTarget(op.Path) {
				id := strings.TrimSuffix(strings.TrimPrefix(op.Path, "pilots/"), ".sortie.json")
				if _, e := decodeSortieReference(op.After, id); e != nil {
					return fmt.Errorf("invalid sortie reference in pending save: %w", e)
				}
			} else if isSortieTarget(op.Path) {
				id := strings.TrimSuffix(strings.TrimPrefix(op.Path, "sorties/"), ".json")
				if _, e := decodeSortieRecord(op.After, id); e != nil {
					return fmt.Errorf("invalid sortie result in pending save: %w", e)
				}
			} else {
				base := strings.TrimPrefix(op.Path, "pilots/")
				id := strings.TrimSuffix(strings.TrimSuffix(base, ".json"), ".backup")
				if _, e := decodePilot(op.After, id); e != nil {
					return fmt.Errorf("invalid pilot in pending save transaction: %w", e)
				}
			}
		}
	}
	// Reject known external edits before applying any remaining payload. The
	// second pass still rechecks each path before its atomic replacement.
	for _, op := range journal.Operations {
		if !safeSaveTarget(op.Path) || op.Remove && len(op.After) != 0 || !op.Remove && len(op.After) == 0 {
			return fmt.Errorf("invalid pending save operation")
		}
		before, err := currentSaveBytes(directory, op.Path)
		if err != nil {
			return err
		}
		if op.Remove && before == nil || !op.Remove && bytes.Equal(before, op.After) {
			continue
		}
		actual := ""
		if before != nil {
			actual = assetdata.Hash(before)
		}
		if actual != op.Before {
			return fmt.Errorf("pending save transaction conflicts with edited %s; preserve the directory before repair", op.Path)
		}
	}
	for _, op := range journal.Operations {
		if !safeSaveTarget(op.Path) || op.Remove && len(op.After) != 0 || !op.Remove && len(op.After) == 0 {
			return fmt.Errorf("invalid pending save operation")
		}
		before, err := currentSaveBytes(directory, op.Path)
		if err != nil {
			return err
		}
		if op.Remove && before == nil || !op.Remove && bytes.Equal(before, op.After) {
			continue
		}
		actual := ""
		if before != nil {
			actual = assetdata.Hash(before)
		}
		if actual != op.Before {
			return fmt.Errorf("pending save transaction conflicts with edited %s; preserve the directory before repair", op.Path)
		}
		if strings.HasPrefix(op.Path, "pilots/") || strings.HasPrefix(op.Path, "sorties/") {
			dir := filepath.Join(directory, strings.SplitN(op.Path, "/", 2)[0])
			if info, err := os.Lstat(dir); err == nil && (!info.IsDir() || info.Mode()&os.ModeSymlink != 0) {
				return fmt.Errorf("save payload directory must be a non-symlink directory")
			}
			if err = os.MkdirAll(dir, 0700); err != nil {
				return err
			}
		}
		if op.Remove {
			if err = os.Remove(filepath.Join(directory, filepath.FromSlash(op.Path))); err != nil {
				return err
			}
		} else {
			dir, name := filepath.Split(filepath.Join(directory, filepath.FromSlash(op.Path)))
			if err = writeSaveAtomic(dir, name, op.After); err != nil {
				return err
			}
		}
	}
	return os.Remove(filepath.Join(directory, saveJournal))
}
