package state

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	assetdata "gunship2000-assets/game/assets"
	"os"
	"path/filepath"
	"testing"
)

func TestSaveFreshEditedPilotAuthorityAcrossRestart(t *testing.T) {
	s := saveTestStore(t)
	state := commitTestPilot(t, s, "Authority")
	id := state.SelectedPilotID
	path := filepath.Join(s.Directory, filepath.FromSlash(pilotFilename(id)))
	raw := mustReadSaveBytes(t, path)
	var doc pilotDocument
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	doc.Pilot.Rank = RankCaptain
	doc.Pilot.MissionType = MissionSingle
	changed, err := assetdata.JsonBytes(doc)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(path, changed, 0600); err != nil {
		t.Fatal(err)
	}
	loaded, err := OpenSaveStore(s.Directory, "")
	if err != nil {
		t.Fatal(err)
	}
	got := loaded.State()
	if got.SelectedPilot().Rank != RankCaptain || got.OfficeState().Rank != 7 || got.OfficeState().MissionType != 1 {
		t.Fatalf("stale presentation source %+v", got.SelectedPilot())
	}
	var fields map[string]json.RawMessage
	if err = json.Unmarshal(loaded.raw, &fields); err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"rank", "office_state", "pilots", "statistics"} {
		if _, ok := fields[forbidden]; ok {
			t.Fatal("redundant display/pilot fields in roster", forbidden)
		}
	}
}

func TestSaveMigrationPreservesTypedEditsAndCompleteDirectory(t *testing.T) {
	directory := t.TempDir()
	state, err := ParseLegacyRoster("old/ROSTER.DAT", syntheticLegacyRoster())
	if err != nil {
		t.Fatal(err)
	}
	state.Slots[0].Rank = RankCaptain
	state.Slots[0].Name = "Edited Captain"
	state.Slots[0].MissionType = MissionSingle
	state.Revision = 12
	binary.LittleEndian.PutUint32(state.Slots[0].LegacyRaw[0x6a:], 1234)
	raw, err := assetdata.JsonBytes(state)
	if err != nil {
		t.Fatal(err)
	}
	// Step C did not model statistics. Simulate its actual original JSON.
	var fields map[string]json.RawMessage
	_ = json.Unmarshal(raw, &fields)
	var slots []map[string]json.RawMessage
	_ = json.Unmarshal(fields["roster_slots"], &slots)
	delete(slots[0], "statistics")
	fields["roster_slots"], _ = json.Marshal(slots)
	raw, _ = json.Marshal(fields)
	os.WriteFile(filepath.Join(directory, oldSaveFilename), raw, 0600)
	os.Mkdir(filepath.Join(directory, "backups"), 0700)
	os.WriteFile(filepath.Join(directory, "backups", "old-backup.json"), []byte("old history"), 0600)
	s, err := OpenSaveStore(directory, "")
	if err != nil {
		t.Fatal(err)
	}
	if p := s.State().SelectedPilot(); p.Rank != RankCaptain || p.Name != "Edited Captain" || p.MissionType != MissionSingle || p.Statistics.LastMissionScore != 1234 {
		t.Fatalf("migration reset typed data %+v", p)
	}
	if !bytes.Equal(raw, mustReadSaveBytes(t, filepath.Join(directory, migrationArchive, oldSaveFilename))) {
		t.Fatal("old committed save not preserved exactly")
	}
	if string(mustReadSaveBytes(t, filepath.Join(directory, migrationArchive, "backups", "old-backup.json"))) != "old history" {
		t.Fatal("complete old directory not retained")
	}
	if candidate, err := s.RecoveryCandidate(); err != nil || candidate != nil {
		t.Fatal("revision history became mission snapshot")
	}
	edited := s.State()
	edited.Slots[0].Name = "Newer C1"
	if err = s.Commit(edited); err != nil {
		t.Fatal(err)
	}
	again, err := OpenSaveStore(directory, "")
	if err != nil {
		t.Fatal(err)
	}
	if again.State().SelectedPilot().Name != "Newer C1" {
		t.Fatal("re-migration overwrote newer state")
	}
}

func TestSaveMigrationTypedStatisticsOverrideLegacy(t *testing.T) {
	directory := t.TempDir()
	state, err := ParseLegacyRoster("old/ROSTER.DAT", syntheticLegacyRoster())
	if err != nil {
		t.Fatal(err)
	}
	state.Slots[0].Statistics.LastMissionScore = 88
	binary.LittleEndian.PutUint32(state.Slots[0].LegacyRaw[0x6a:], 999)
	raw, _ := assetdata.JsonBytes(state)
	os.WriteFile(filepath.Join(directory, oldSaveFilename), raw, 0600)
	s, err := OpenSaveStore(directory, "")
	if err != nil {
		t.Fatal(err)
	}
	if s.State().SelectedPilot().Statistics.LastMissionScore != 88 {
		t.Fatal("legacy bytes overwrote typed JSON")
	}
}

func TestSavePendingCommitResumesAfterPartialApplication(t *testing.T) {
	s := saveTestStore(t)
	state := commitTestPilot(t, s, "Before")
	state.Slots[0].Rank = RankCaptain
	state.Revision++
	desired, err := stateFiles(state)
	if err != nil {
		t.Fatal(err)
	}
	pilotPath := pilotFilename(state.SelectedPilotID)
	journal := saveCommit{SchemaVersion: 1, Operations: []saveOperation{{Path: pilotPath, Before: assetdata.Hash(s.files[pilotPath]), After: desired[pilotPath]}, {Path: saveFilename, Before: assetdata.Hash(s.raw), After: desired[saveFilename]}}}
	raw, _ := assetdata.JsonBytes(journal)
	os.WriteFile(filepath.Join(s.Directory, saveJournal), raw, 0600)
	// Crash after the pilot replacement but before roster publication.
	os.WriteFile(filepath.Join(s.Directory, filepath.FromSlash(pilotPath)), desired[pilotPath], 0600)
	loaded, err := OpenSaveStore(s.Directory, "")
	if err != nil {
		t.Fatal(err)
	}
	if loaded.State().SelectedPilot().Rank != RankCaptain || loaded.State().Revision != state.Revision {
		t.Fatal("pending update not resumed")
	}
	if _, err = os.Stat(filepath.Join(s.Directory, saveJournal)); !os.IsNotExist(err) {
		t.Fatal("completed journal retained")
	}
}

func TestSaveRecoveryIsPerPilotAndUnknownBackupFails(t *testing.T) {
	s := saveTestStore(t)
	state := commitTestPilot(t, s, "First")
	first := state.SelectedPilotID
	if err := s.PrepareMissionSnapshot(); err != nil {
		t.Fatal(err)
	}
	state = s.State()
	state.Slots[1] = saveTestPilot(t, "Second")
	state.SelectedPilotID = state.Slots[1].ID
	if err := s.Commit(state); err != nil {
		t.Fatal(err)
	}
	if candidate, err := s.RecoveryCandidate(); err != nil || candidate != nil {
		t.Fatal("another pilot's backup made recovery available")
	}
	state = s.State()
	state.SelectedPilotID = first
	if err := s.Commit(state); err != nil {
		t.Fatal(err)
	}
	if candidate, err := s.RecoveryCandidate(); err != nil || candidate == nil {
		t.Fatal("first pilot lost its own backup")
	}
	backup := filepath.Join(s.Directory, filepath.FromSlash(backupFilename(first)))
	os.WriteFile(backup, []byte("damaged"), 0600)
	if _, err := s.RecoveryCandidate(); err == nil {
		t.Fatal("damaged backup silently ignored")
	}
}

func TestSaveStatisticsSourceWidthsAndArithmetic(t *testing.T) {
	raw := make([]byte, LegacyRecordSize)
	binary.LittleEndian.PutUint32(raw[0x6a:], uint32(0xfffffffd))
	binary.LittleEndian.PutUint32(raw[0x66:], 17)
	binary.LittleEndian.PutUint32(raw[0x72:], 81)
	binary.LittleEndian.PutUint32(raw[0x6e:], 207)
	binary.LittleEndian.PutUint32(raw[0xcc:], 0xfffffffe)
	stats := statisticsFromLegacy(raw)
	if stats.LastMissionScore != -3 || stats.BestMissionScore != 17 || stats.CareerScore != 81 || stats.Missions() != 7 || stats.Campaigns() != 2 || stats.PhoenixCount() != 2 {
		t.Fatalf("source arithmetic %+v", stats)
	}
}

func TestSaveMigrationResumesAfterArchiveBeforePublication(t *testing.T) {
	directory := t.TempDir()
	state := EmptySaveState()
	state.Slots[0] = saveTestPilot(t, "Migration Retry")
	state.SelectedPilotID = state.Slots[0].ID
	raw, _ := assetdata.JsonBytes(state)
	os.WriteFile(filepath.Join(directory, oldSaveFilename), raw, 0600)
	if err := preserveOldSaveDirectory(directory); err != nil {
		t.Fatal(err)
	}
	// Restart after preservation completed but no new roster was published.
	loaded, err := OpenSaveStore(directory, "")
	if err != nil {
		t.Fatal(err)
	}
	if loaded.State().SelectedPilot().Name != "Migration Retry" {
		t.Fatal("migration retry lost data")
	}
}

func TestSavePendingRecoveryConsumesOnceAfterInterruptedWrite(t *testing.T) {
	s := saveTestStore(t)
	state := commitTestPilot(t, s, "Recover Once")
	if err := s.PrepareMissionSnapshot(); err != nil {
		t.Fatal(err)
	}
	state = s.State()
	state.SelectedPilot().Statistics.PhoenixCounter = -1
	state.Revision++
	desired, err := stateFiles(state)
	if err != nil {
		t.Fatal(err)
	}
	pilotPath := pilotFilename(state.SelectedPilotID)
	backup := backupFilename(state.SelectedPilotID)
	backupRaw := mustReadSaveBytes(t, filepath.Join(s.Directory, filepath.FromSlash(backup)))
	journal := saveCommit{SchemaVersion: 1, Operations: []saveOperation{{Path: pilotPath, Before: assetdata.Hash(s.files[pilotPath]), After: desired[pilotPath]}, {Path: backup, Before: assetdata.Hash(backupRaw), Remove: true}, {Path: saveFilename, Before: assetdata.Hash(s.raw), After: desired[saveFilename]}}}
	raw, _ := assetdata.JsonBytes(journal)
	os.WriteFile(filepath.Join(s.Directory, saveJournal), raw, 0600)
	os.WriteFile(filepath.Join(s.Directory, filepath.FromSlash(pilotPath)), desired[pilotPath], 0600)
	loaded, err := OpenSaveStore(s.Directory, "")
	if err != nil {
		t.Fatal(err)
	}
	if loaded.State().SelectedPilot().Statistics.PhoenixCount() != 1 {
		t.Fatal("interrupted recovery lost increment")
	}
	if candidate, err := loaded.RecoveryCandidate(); err != nil || candidate != nil {
		t.Fatal("interrupted recovery did not consume backup")
	}
	again, err := OpenSaveStore(s.Directory, "")
	if err != nil {
		t.Fatal(err)
	}
	if again.State().SelectedPilot().Statistics.PhoenixCount() != 1 {
		t.Fatal("recovery increment duplicated on retry")
	}
}

func TestSaveMigrationUnknownStatisticsStayUnknown(t *testing.T) {
	directory := t.TempDir()
	state := EmptySaveState()
	p := saveTestPilot(t, "Unknown Imported State")
	p.LegacyRaw = nil
	p.Statistics = nil
	state.Slots[0] = p
	state.SelectedPilotID = p.ID
	raw, _ := assetdata.JsonBytes(state)
	var fields map[string]json.RawMessage
	_ = json.Unmarshal(raw, &fields)
	var slots []map[string]json.RawMessage
	_ = json.Unmarshal(fields["roster_slots"], &slots)
	delete(slots[0], "statistics")
	fields["roster_slots"], _ = json.Marshal(slots)
	raw, _ = json.Marshal(fields)
	os.WriteFile(filepath.Join(directory, oldSaveFilename), raw, 0600)
	s, err := OpenSaveStore(directory, "")
	if err != nil {
		t.Fatal(err)
	}
	if s.State().SelectedPilot().Statistics != nil {
		t.Fatal("unknown imported counters fabricated as zero")
	}
	if err = s.PrepareMissionSnapshot(); err == nil {
		t.Fatal("unknown Phoenix state snapshot accepted")
	}
}

func TestSavePilotFileAndRosterReferenceValidation(t *testing.T) {
	for _, kind := range []string{"missing", "future", "invalidrank", "mismatchedid", "unsafeid"} {
		t.Run(kind, func(t *testing.T) {
			s := saveTestStore(t)
			state := commitTestPilot(t, s, "Validate")
			id := state.SelectedPilotID
			path := filepath.Join(s.Directory, filepath.FromSlash(pilotFilename(id)))
			if kind == "missing" {
				os.Remove(path)
			} else if kind == "unsafeid" {
				var doc rosterDocument
				_ = json.Unmarshal(s.raw, &doc)
				doc.Slots[0] = "../outside"
				raw, _ := assetdata.JsonBytes(doc)
				os.WriteFile(filepath.Join(s.Directory, saveFilename), raw, 0600)
			} else {
				var doc pilotDocument
				_ = json.Unmarshal(mustReadSaveBytes(t, path), &doc)
				switch kind {
				case "future":
					doc.SchemaVersion = 999
				case "invalidrank":
					doc.Rank = "invented"
				case "mismatchedid":
					doc.ID = "other"
				}
				raw, _ := assetdata.JsonBytes(doc)
				os.WriteFile(path, raw, 0600)
			}
			if _, err := OpenSaveStore(s.Directory, ""); err == nil {
				t.Fatal("invalid pilot/reference accepted")
			}
		})
	}
}

func TestSaveAwardCountersSignedWidthAndTypedAuthority(t *testing.T) {
	fresh := saveTestPilot(t, "Awards")
	if len(fresh.Statistics.AwardCounts) != 8 {
		t.Fatal("new pilot lacks known zero award counters")
	}
	for _, count := range fresh.Statistics.AwardCounts {
		if count != 0 {
			t.Fatal("new pilot has a fabricated award")
		}
	}
	raw := make([]byte, LegacyRecordSize)
	raw[0x24] = 0xff
	raw[0x25] = 0x80
	raw[0x26] = 0x7f
	statistics := statisticsFromLegacy(raw)
	if len(statistics.AwardCounts) != 8 || statistics.AwardCounts[0] != -1 || statistics.AwardCounts[1] != -128 || statistics.AwardCounts[2] != 127 {
		t.Fatal("legacy award signed-byte mapping changed")
	}
	state := EmptySaveState()
	state.Slots[0] = fresh
	for _, counts := range [][]int8{make([]int8, 7), make([]int8, 9)} {
		copy := cloneSave(state)
		copy.Slots[0].Statistics.AwardCounts = counts
		if err := copy.Validate(); err == nil {
			t.Fatal("wrong award counter arity accepted")
		}
	}
	state.Slots[0].Statistics.AwardCounts = nil
	if err := state.Validate(); err != nil {
		t.Fatal("unknown award counters rejected", err)
	}
	// Statistics already typed in an old Step C document are authoritative;
	// missing optional counters are not silently repopulated from historic bytes.
	directory := t.TempDir()
	state.Slots[0].LegacyRaw = raw
	data, _ := assetdata.JsonBytes(state)
	os.WriteFile(filepath.Join(directory, oldSaveFilename), data, 0600)
	migrated, err := OpenSaveStore(directory, "")
	if err != nil {
		t.Fatal(err)
	}
	if migrated.State().Slots[0].Statistics.AwardCounts != nil {
		t.Fatal("historical award values overwrote missing typed optional state")
	}
}
