package state

import (
	"bytes"
	"encoding/json"
	assetdata "gunship2000-assets/game/assets"
	"gunship2000-assets/game/configuration"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func saveTestStore(t *testing.T) *SaveStore {
	t.Helper()
	s, err := OpenSaveStore(filepath.Join(t.TempDir(), "saves"), "")
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func saveTestPilot(t *testing.T, name string) *Pilot {
	t.Helper()
	p, err := NewPilot()
	if err != nil {
		t.Fatal(err)
	}
	p.Name = name
	p.Squadron = "Test Squadron Name"
	return p
}

func commitTestPilot(t *testing.T, s *SaveStore, name string) SaveState {
	t.Helper()
	state := s.State()
	if state.Slots[0] == nil {
		state.Slots[0] = saveTestPilot(t, name)
	} else {
		state.Slots[0].Name = name
	}
	state.SelectedPilotID = state.Slots[0].ID
	if err := s.Commit(state); err != nil {
		t.Fatal(err)
	}
	return s.State()
}

func mustReadSaveBytes(t *testing.T, path string) []byte {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func TestSaveEmptyInitializationAndAuthoritativeReuse(t *testing.T) {
	s := saveTestStore(t)
	state := s.State()
	if state.SelectedPilot() != nil || state.Slots[0] != nil || state.Revision != 0 {
		t.Fatalf("not genuine empty state: %+v", state)
	}
	if state.OfficeState().Theatre != 0 || state.OfficeState().Active {
		t.Fatal("empty roster inherited named-fixture display values")
	}
	if pilot := saveTestPilot(t, "New Pilot"); pilot.Theatre != TheatrePersianGulf {
		t.Fatal("new pilot no longer uses the established Gulf initialization")
	}
	filename := filepath.Join(s.Directory, saveFilename)
	before := mustReadSaveBytes(t, filename)
	if err := s.Commit(state); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(s.Directory, "backups")); !os.IsNotExist(err) {
		t.Fatal("unchanged load/commit created snapshots")
	}
	// A valid JSON save takes precedence even when an unrelated legacy path is corrupt.
	original := t.TempDir()
	os.WriteFile(filepath.Join(original, "ROSTER.DAT"), []byte("broken"), 0600)
	loaded, err := OpenSaveStore(s.Directory, original)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, loaded.raw) {
		t.Fatal("authoritative JSON rewritten")
	}
}

func TestSavePilotSelectionRoundTripAndIsolation(t *testing.T) {
	s := saveTestStore(t)
	state := commitTestPilot(t, s, "Test Pilot Name")
	state.Slots[0].Rank = RankCaptain
	state.Slots[0].Theatre = TheatrePhilippines
	state.Slots[0].MissionType = MissionSingle
	state.Slots[0].Emblem = 17
	state.Slots[5] = saveTestPilot(t, "Second Pilot")
	state.SelectedPilotID = state.Slots[5].ID
	if err := s.Commit(state); err != nil {
		t.Fatal(err)
	}
	loaded, err := OpenSaveStore(s.Directory, "")
	if err != nil {
		t.Fatal(err)
	}
	if got := loaded.State(); got.SelectedPilot().Name != "Second Pilot" || got.Slots[0].Rank != RankCaptain || got.Slots[0].Theatre != TheatrePhilippines || got.Slots[0].MissionType != MissionSingle {
		t.Fatalf("roundtrip: %+v", got)
	}
	draft := loaded.State()
	draft.Slots[0].Name = "Uncommitted"
	draft.Slots[0].LegacyRaw[0x4f] = 45
	if loaded.State().Slots[0].Name == "Uncommitted" || loaded.State().Slots[0].LegacyRaw[0x4f] != 0 {
		t.Fatal("draft mutated live state")
	}
	if got := loaded.State().OfficeState(); !got.Active || got.PilotName != "Second Pilot" {
		t.Fatalf("office mapping %+v", got)
	}
}

func TestSaveTypedEnumsAndTextValidation(t *testing.T) {
	state := EmptySaveState()
	state.Slots[0] = saveTestPilot(t, "Valid Pilot")
	for _, bad := range []func(*SaveState){
		func(s *SaveState) { s.SchemaVersion = 2 }, func(s *SaveState) { s.Slots[0].Rank = "president" }, func(s *SaveState) { s.Slots[0].MissionType = "any" }, func(s *SaveState) { s.Slots[0].Theatre = "moon" }, func(s *SaveState) { s.Slots[0].Status = "any" }, func(s *SaveState) { s.Slots[0].Emblem = 18 }, func(s *SaveState) { s.Slots[0].Name = strings.Repeat("X", 36) }, func(s *SaveState) { s.Slots[0].Squadron = strings.Repeat("X", 26) }, func(s *SaveState) { s.Slots[0].Name = "Bad\nName" }, func(s *SaveState) { s.SelectedPilotID = "missing" }, func(s *SaveState) { s.Slots[1] = s.Slots[0] }, func(s *SaveState) { s.Slots[0].LegacyRaw = []byte{1} }, func(s *SaveState) { s.Slots[0].Status = StatusLegacyUnknown },
	} {
		copy := cloneSave(state)
		bad(&copy)
		if err := copy.Validate(); err == nil {
			t.Fatalf("accepted invalid state %+v", copy)
		}
	}
	for n, rank := range saveRanks {
		if rank.Raw() != n {
			t.Fatalf("rank mapping %s", rank)
		}
		r, err := RankFromRaw(n)
		if err != nil || r != rank {
			t.Fatal("rank roundtrip")
		}
	}
	if RankCaptain.Raw() != 7 || RankSecondLieutenant.Raw() != 5 || MissionSimulated.Raw() != 2 || MissionFlight.Raw() != 3 {
		t.Fatal("original table ordering changed")
	}
	if _, err := RankFromRaw(12); err == nil {
		t.Fatal("unknown rank accepted")
	}
	if _, err := MissionTypeFromRaw(-1); err == nil {
		t.Fatal("unknown mission accepted")
	}
	if _, err := TheatreFromRaw(4); err == nil {
		t.Fatal("unknown theatre accepted")
	}
}

func TestSaveMalformedExistingFileDoesNotReset(t *testing.T) {
	for _, raw := range []string{`not json`, `{"schema_version":999,"roster_slots":[null,null,null,null,null,null]}`, `{"schema_version":1,"roster_slots":[null]}`, `{"schema_version":1,"roster_slots":[null,null,null,null,null,null,null]}`, `{"schema_version":1,"roster_slots":[null,null,null,null,null,null],"unknown":2}`} {
		dir := t.TempDir()
		path := filepath.Join(dir, saveFilename)
		os.WriteFile(path, []byte(raw), 0600)
		if _, err := OpenSaveStore(dir, ""); err == nil {
			t.Fatalf("accepted malformed save %q", raw)
		}
		if string(mustReadSaveBytes(t, path)) != raw {
			t.Fatal("corrupt save overwritten")
		}
	}
}

func TestSaveFailureRetainsLiveAndPreviousSave(t *testing.T) {
	s := saveTestStore(t)
	commitTestPilot(t, s, "Before")
	before := append([]byte(nil), s.raw...)
	// A non-file journal target blocks the transaction before publishing anything.
	if err := os.Mkdir(filepath.Join(s.Directory, saveJournal), 0700); err != nil {
		t.Fatal(err)
	}
	draft := s.State()
	draft.Slots[0].Name = "After"
	if err := s.Commit(draft); err == nil {
		t.Fatal("save unexpectedly succeeded")
	}
	if !bytes.Equal(before, mustReadSaveBytes(t, filepath.Join(s.Directory, saveFilename))) || s.State().Slots[0].Name != "Before" {
		t.Fatal("failed write changed committed save")
	}
}

func TestSaveSelectedPilotRecoveryConsumedAndCounterPreserved(t *testing.T) {
	s := saveTestStore(t)
	state := commitTestPilot(t, s, "Alpha")
	state.Slots[1] = saveTestPilot(t, "Second")
	if err := s.Commit(state); err != nil {
		t.Fatal(err)
	}
	if candidate, err := s.RecoveryCandidate(); err != nil || candidate != nil {
		t.Fatal("menu commits manufactured a gameplay backup")
	}
	if err := s.PrepareMissionSnapshot(); err != nil {
		t.Fatal(err)
	}
	state = s.State()
	state.Slots[0].Statistics.CareerScore = 999
	state.Slots[0].Theatre = TheatrePhilippines
	state.Slots[1].Rank = RankCaptain
	if err := s.Commit(state); err != nil {
		t.Fatal(err)
	}
	before := mustReadSaveBytes(t, filepath.Join(s.Directory, saveFilename))
	candidate, err := s.RecoveryCandidate()
	if err != nil || candidate == nil {
		t.Fatalf("candidate %+v %v", candidate, err)
	}
	if !bytes.Equal(before, mustReadSaveBytes(t, filepath.Join(s.Directory, saveFilename))) {
		t.Fatal("preview/cancel mutated save")
	}
	if err = s.Restore(candidate.ID); err != nil {
		t.Fatal(err)
	}
	got := s.State()
	if got.Slots[0].Statistics.CareerScore != 0 || got.Slots[0].Theatre != TheatrePersianGulf || got.Slots[0].Statistics.PhoenixCounter != -1 || got.Slots[0].Statistics.PhoenixCount() != 1 || got.Slots[1].Rank != RankCaptain {
		t.Fatalf("restore changed wrong state %+v", got)
	}
	if err = s.Restore(candidate.ID); err == nil {
		t.Fatal("consumed snapshot reused")
	}
	loaded, err := OpenSaveStore(s.Directory, "")
	if err != nil {
		t.Fatal(err)
	}
	if loaded.State().SelectedPilot().Statistics.PhoenixCounter != -1 {
		t.Fatal("Phoenix count lost on restart")
	}
	if candidate, err = loaded.RecoveryCandidate(); err != nil || candidate != nil {
		t.Fatal("consumed recovery reappeared")
	}
	if err = loaded.PrepareMissionSnapshot(); err != nil {
		t.Fatal(err)
	}
	candidate, err = loaded.RecoveryCandidate()
	if err != nil {
		t.Fatal(err)
	}
	if err = loaded.Restore(candidate.ID); err != nil {
		t.Fatal(err)
	}
	if loaded.State().SelectedPilot().Statistics.PhoenixCount() != 2 {
		t.Fatal("next mission recovery did not increment once")
	}
}

func TestSaveDetectsExternalChangesAndLocks(t *testing.T) {
	s := saveTestStore(t)
	other, err := OpenSaveStore(s.Directory, "")
	if err != nil {
		t.Fatal(err)
	}
	commitTestPilot(t, s, "Writer One")
	state := other.State()
	state.Slots[0] = saveTestPilot(t, "Writer Two")
	if err = other.Commit(state); err == nil || !strings.Contains(err.Error(), "outside this session") {
		t.Fatalf("stale writer accepted: %v", err)
	}
	lock := filepath.Join(s.Directory, ".save.lock")
	os.WriteFile(lock, []byte("test"), 0600)
	if owner, e := configuration.BeginOwnership(filepath.Dir(s.Directory), "installed", "", false); e == nil {
		owner.Close()
		t.Fatal("ambiguous prior save reservation ignored")
	}
}

func TestSaveMissingMainWithBackupsDoesNotResetProgress(t *testing.T) {
	s := saveTestStore(t)
	commitTestPilot(t, s, "Preserve Me")
	if err := os.Remove(filepath.Join(s.Directory, saveFilename)); err != nil {
		t.Fatal(err)
	}
	if _, err := OpenSaveStore(s.Directory, ""); err == nil || !strings.Contains(err.Error(), "explicitly recover") {
		t.Fatalf("missing save reset progress: %v", err)
	}
	if _, err := os.Stat(filepath.Join(s.Directory, saveFilename)); !os.IsNotExist(err) {
		t.Fatal("missing save recreated over history")
	}
}

func TestSaveRefusesOverlapAndSymlinkTargets(t *testing.T) {
	root := t.TempDir()
	original := filepath.Join(root, "original")
	os.Mkdir(original, 0700)
	for _, out := range []string{original, filepath.Join(original, "saves"), root} {
		if _, err := OpenSaveStore(out, original); err == nil {
			t.Fatalf("overlap accepted %s", out)
		}
	}
	outside := t.TempDir()
	link := filepath.Join(root, "linked")
	if err := os.Symlink(outside, link); err == nil {
		if _, err = OpenSaveStore(link, ""); err == nil {
			t.Fatal("symlink save directory accepted")
		}
	}
}

func syntheticLegacyRoster() []byte {
	raw := make([]byte, 7*LegacyRecordSize)
	copy(raw, "Test Pilot Name")
	copy(raw[0x34:], "Test Squadron Name")
	raw[0x53] = 1
	raw[0x56] = 1
	raw[0x64] = 1
	raw[0x4e] = 77
	raw[0x11c] = 123
	raw[6*LegacyRecordSize] = 0xee
	return raw
}

func TestSaveLegacyBoundedFieldsAndSeventhBlock(t *testing.T) {
	raw := syntheticLegacyRoster()
	state, err := ParseLegacyRoster("source/ROSTER.DAT", raw)
	if err != nil {
		t.Fatal(err)
	}
	if state.Slots[0].Name != "Test Pilot Name" || state.Slots[0].Squadron != "Test Squadron Name" || state.Slots[0].Emblem != 1 || state.Slots[0].Rank != RankWOC || state.Slots[0].Theatre != TheatrePersianGulf || state.Slots[1] != nil {
		t.Fatalf("bad import %+v", state)
	}
	if !bytes.Equal(raw, state.Legacy.RawRoster) || state.Legacy.RecoveryRecord[0] != 0xee {
		t.Fatal("opaque/seventh bytes not retained")
	}
	if b, ok := state.Slots[0].LegacyByte(0x11c); !ok || b != 123 {
		t.Fatal("unknown record state lost")
	}
	if state.SelectedPilotID != state.Slots[0].ID {
		t.Fatal("first Active import policy")
	}
	dir := t.TempDir()
	if err := preserveLegacyImport(dir, *state.Legacy); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, filepath.FromSlash(state.Legacy.PreservedPath))
	if !bytes.Equal(raw, mustReadSaveBytes(t, path)) {
		t.Fatal("immutable copy differs")
	}
	if err := preserveLegacyImport(dir, *state.Legacy); err != nil {
		t.Fatal("interrupted import reuse", err)
	}
	changed := *state.Legacy
	changed.RawRoster = append([]byte(nil), raw...)
	changed.RawRoster[1] ^= 1
	if err := preserveLegacyImport(dir, changed); err == nil {
		t.Fatal("immutable import overwritten")
	}
}

func TestSaveLegacyUnknownStatusIsOpaqueNotActive(t *testing.T) {
	raw := syntheticLegacyRoster()
	raw[0x56] = 5 // Source table0..4 is now explicitly decoded;5 remains opaque.
	state, err := ParseLegacyRoster("ROSTER.DAT", raw)
	if err != nil {
		t.Fatal(err)
	}
	if state.Slots[0].Status != StatusLegacyUnknown || state.Slots[0].LegacyStatus != 5 || state.SelectedPilotID != "" {
		t.Fatal("unknown status assigned Active meaning")
	}
	for _, edit := range []func([]byte){func(b []byte) { b[0x54] = 255 }, func(b []byte) { b[0x64] = 9 }, func(b []byte) { b[0x65] = 9 }, func(b []byte) {
		for i := 0; i < 0x34; i++ {
			b[i] = 'A'
		}
	}} {
		bad := append([]byte(nil), raw...)
		edit(bad)
		if _, err := ParseLegacyRoster("ROSTER.DAT", bad); err == nil {
			t.Fatal("unsupported legacy fields accepted")
		}
	}
	if _, err := ParseLegacyRoster("ROSTER.DAT", raw[:len(raw)-1]); err == nil {
		t.Fatal("truncated legacy roster accepted")
	}
}

func TestSaveUnsupportedLegacySourceDoesNotInitialize(t *testing.T) {
	original := t.TempDir()
	os.WriteFile(filepath.Join(original, "ROSTER.DAT"), syntheticLegacyRoster(), 0600)
	os.WriteFile(filepath.Join(original, "GS.GS2"), []byte("unsupported"), 0600)
	saves := filepath.Join(t.TempDir(), "saves")
	if _, err := OpenSaveStore(saves, original); err == nil || !strings.Contains(err.Error(), "unsupported") {
		t.Fatalf("unsupported import %v", err)
	}
	if _, err := os.Stat(filepath.Join(saves, saveFilename)); !os.IsNotExist(err) {
		t.Fatal("failed import initialized empty save")
	}
}

func TestSaveLegacySupportedFixtureWhenRequested(t *testing.T) {
	original, fixture := os.Getenv("GS2000_SAVE_ORIGINAL"), os.Getenv("GS2000_SAVE_ROSTER")
	if original == "" || fixture == "" {
		t.Skip("set GS2000_SAVE_ORIGINAL and GS2000_SAVE_ROSTER for source-backed import check")
	}
	root := t.TempDir()
	game := filepath.Join(root, "game")
	os.Mkdir(game, 0700)
	code := mustReadSaveBytes(t, filepath.Join(original, "GS.GS2"))
	raw := mustReadSaveBytes(t, fixture)
	os.WriteFile(filepath.Join(game, "GS.GS2"), code, 0600)
	os.WriteFile(filepath.Join(game, "ROSTER.DAT"), raw, 0600)
	s, err := OpenSaveStore(filepath.Join(root, "saves"), game)
	if err != nil {
		t.Fatal(err)
	}
	if p := s.State().SelectedPilot(); p == nil || p.Name != "Test Pilot Name" || p.Emblem != 1 {
		t.Fatal("fixture import mismatch")
	}
	if !bytes.Equal(raw, mustReadSaveBytes(t, filepath.Join(game, "ROSTER.DAT"))) {
		t.Fatal("original roster was written")
	}
	data, _ := json.Marshal(s.State())
	if !bytes.Contains(data, []byte(`"rank":"warrant_officer_candidate"`)) {
		t.Fatal("rank enum not named JSON")
	}
}

// This opt-in authoring helper uses the production importer, never a normal
// application save. Its explicit output must not already exist.
func TestSaveExportCaptainComparisonWhenRequested(t *testing.T) {
	source, output := os.Getenv("GS2000_CAPTAIN_SOURCE"), os.Getenv("GS2000_CAPTAIN_SAVE_OUTPUT")
	if source == "" || output == "" {
		t.Skip("set GS2000_CAPTAIN_SOURCE and GS2000_CAPTAIN_SAVE_OUTPUT to author the separate Captain comparison save")
	}
	if _, err := os.Lstat(output); !os.IsNotExist(err) {
		t.Fatalf("comparison output must not exist: %s", output)
	}
	sourcePath, err := legacyRosterPath(source)
	if err != nil || sourcePath == "" {
		t.Fatalf("Captain roster source: %s %v", sourcePath, err)
	}
	before := mustReadSaveBytes(t, sourcePath)
	s, err := OpenSaveStore(output, source)
	if err != nil {
		t.Fatal(err)
	}
	state := s.State()
	p := state.SelectedPilot()
	if p == nil || p != state.Slots[0] || p.Name != "Test Pilot Name" || p.Squadron != "Test Squadron Name" || p.Emblem != 1 || p.Rank != RankCaptain || p.Status != StatusActive || p.MissionType != MissionTraining || p.Theatre != TheatrePersianGulf {
		t.Fatalf("unexpected Captain comparison values: %+v", p)
	}
	if !bytes.Equal(before[:LegacyRecordSize], p.LegacyRaw) || !bytes.Equal(before, mustReadSaveBytes(t, sourcePath)) {
		t.Fatal("source/retained Captain roster mismatch")
	}
	manifest := struct {
		SourcePath      string   `json:"source_roster_path"`
		SourceSHA256    string   `json:"source_roster_sha256"`
		Profile         string   `json:"profile"`
		SavePath        string   `json:"save_path"`
		SaveSHA256      string   `json:"save_sha256"`
		SelectedPilotID string   `json:"selected_pilot_id"`
		Rank            Rank     `json:"rank"`
		RankRaw         int      `json:"rank_raw"`
		Evidence        []string `json:"evidence"`
	}{sourcePath, assetdata.Hash(before), assetdata.IslandIce469085.Name, saveFilename, assetdata.Hash(s.raw), state.SelectedPilotID, p.Rank, p.Rank.Raw(), []string{"evidence/stepC/captain-byte-change.json", "evidence/step17/runtime-to-file-fields.csv", "evidence/step14/source-table-provenance.json"}}
	raw, err := assetdata.JsonBytes(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if err = writeNewSaveFile(filepath.Join(output, "comparison.json"), raw); err != nil {
		t.Fatal(err)
	}
	readme := "# Captain port comparison save\n\nSeparate, read-only Step C comparison fixture; not the user's normal save. Imported through OpenSaveStore from the dedicated Captain original-game comparison run. Selected slot0 contains Test Pilot Name / Test Squadron Name, emblem1, Captain (raw7), Active, Training, Persian Gulf. All legacy roster bytes, including the seventh block, are retained unchanged.\n\nTo use in a test run, copy this directory to a NEW mutable save directory, make copied files writable, and configure saves to that copy. Never run against this immutable directory. Keep assets and saves separate. The matching original-game delta fixture is fixtures/stepC-captain-test-pilot. This fixture does not assert that Captain unlocks campaign-dependent branches.\n"
	if err = writeNewSaveFile(filepath.Join(output, "README.md"), []byte(readme)); err != nil {
		t.Fatal(err)
	}
	err = filepath.WalkDir(output, func(path string, d os.DirEntry, e error) error {
		if e != nil {
			return e
		}
		if !d.IsDir() {
			return os.Chmod(path, 0400)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("Captain comparison generated at %s, save SHA256 %s", output, assetdata.Hash(s.raw))
}
