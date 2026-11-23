package state

import (
	"bytes"
	"fmt"
	assetdata "gunship2000-assets/game/assets"

	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func fixtureSaveManifest(directory string) (map[string]string, error) {
	files := map[string]string{}
	err := filepath.WalkDir(directory, func(path string, d fs.DirEntry, e error) error {
		if e != nil {
			return e
		}
		if d.IsDir() {
			return nil
		}
		info, e := d.Info()
		if e != nil {
			return e
		}
		if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("fixture input must contain regular files only: %s", path)
		}
		rel, e := filepath.Rel(directory, path)
		if e != nil {
			return e
		}
		raw, e := assetdata.ReadLimitedFile(path, 4<<20)
		if e != nil {
			return e
		}
		files[filepath.ToSlash(rel)] = assetdata.Hash(raw)
		return nil
	})
	return files, err
}

func fixtureSaveCopy(source, destination string) error {
	return filepath.WalkDir(source, func(path string, d fs.DirEntry, e error) error {
		if e != nil {
			return e
		}
		rel, e := filepath.Rel(source, path)
		if e != nil {
			return e
		}
		target := filepath.Join(destination, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0700)
		}
		info, e := d.Info()
		if e != nil {
			return e
		}
		if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("unsupported copy input %s", path)
		}
		raw, e := assetdata.ReadLimitedFile(path, 4<<20)
		if e != nil {
			return e
		}
		f, e := os.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if e != nil {
			return e
		}
		_, e = f.Write(raw)
		closeErr := f.Close()
		if e != nil {
			return e
		}
		return closeErr
	})
}

// Opt-in authoring only. No normal application path imports this fixture or
// seeds the user's roster with a test pilot or manufactured mission backup.
func TestSaveC1AuthorDisposableMigrationAndRecoveryWhenRequested(t *testing.T) {
	output, source := os.Getenv("GS2000_C1_FIXTURE_OUTPUT"), os.Getenv("GS2000_C1_SAVE_SOURCE")
	if output == "" || source == "" {
		t.Skip("set explicit new GS2000_C1_FIXTURE_OUTPUT and GS2000_C1_SAVE_SOURCE to author disposable C.1 fixtures")
	}
	var err error
	source, err = filepath.Abs(source)
	if err != nil {
		t.Fatal(err)
	}
	output, err = filepath.Abs(output)
	if err != nil {
		t.Fatal(err)
	}
	if pathsOverlap(source, output) {
		t.Fatal("fixture input/output must not overlap")
	}
	if _, err = os.Lstat(output); !os.IsNotExist(err) {
		t.Fatalf("fixture output must not exist: %s", output)
	}
	before, err := fixtureSaveManifest(source)
	if err != nil {
		t.Fatal(err)
	}
	old, oldRaw, err := readSave(filepath.Join(source, oldSaveFilename))
	if err != nil {
		t.Fatal(err)
	}
	if old.SelectedPilot() == nil || old.SelectedPilot().Rank != RankCaptain {
		t.Fatal("acceptance input selected pilot is not the expected user's edited Captain")
	}
	if err = os.MkdirAll(output, 0700); err != nil {
		t.Fatal(err)
	}
	migrated := filepath.Join(output, "migrated-acceptance")
	if err = fixtureSaveCopy(source, migrated); err != nil {
		t.Fatal(err)
	}
	copied, err := fixtureSaveManifest(migrated)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(before, copied) {
		t.Fatal("initial copy does not match every original file")
	}
	store, err := OpenSaveStore(migrated, "")
	if err != nil {
		t.Fatal(err)
	}
	got := store.State()
	if got.SelectedPilotID != old.SelectedPilotID {
		t.Fatal("migration changed selected pilot")
	}
	for i, p := range old.Slots {
		after := got.Slots[i]
		if p == nil {
			if after != nil {
				t.Fatalf("empty slot%d occupied by migration", i)
			}
			continue
		}
		if after == nil {
			t.Fatalf("slot%d lost", i)
		}
		// Preserve every previously typed pilot value and opaque byte. Newly decoded
		// statistics are intentionally a separate check from the Step C type set.
		expected := *p
		actual := *after
		expected.Statistics = nil
		actual.Statistics = nil
		if !reflect.DeepEqual(expected, actual) {
			t.Fatalf("typed slot%d changed during migration", i)
		}
	}
	for name, want := range before {
		raw, err := ReadRegularSave(filepath.Join(migrated, migrationArchive, filepath.FromSlash(name)))
		if err != nil || assetdata.Hash(raw) != want {
			t.Fatalf("old-directory archive mismatch %s: %v", name, err)
		}
	}
	if candidate, err := store.RecoveryCandidate(); err != nil || candidate != nil {
		t.Fatal("old revision backups became mission recovery")
	}
	reopened, err := OpenSaveStore(migrated, "")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, reopened.State()) {
		t.Fatal("migrated state changed on restart")
	}
	current, err := fixtureSaveManifest(source)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(before, current) {
		t.Fatal("protected acceptance source changed")
	}
	if !bytes.Equal(oldRaw, mustReadSaveBytes(t, filepath.Join(source, oldSaveFilename))) {
		t.Fatal("source save was rewritten")
	}

	recoveryDir := filepath.Join(output, "recovery-fixture")
	recovery, err := OpenSaveStore(recoveryDir, "")
	if err != nil {
		t.Fatal(err)
	}
	pilot := saveTestPilot(t, "Test Pilot Name")
	pilot.Squadron = "Test Squadron Name"
	pilot.Emblem = 1
	pilot.Rank = RankCaptain
	pilot.Statistics = &PilotStatistics{LastMissionScore: 101, BestMissionScore: 202, CareerScore: 303, MissionCampaignCounter: 207, PhoenixCounter: -1}
	state := recovery.State()
	state.Slots[0] = pilot
	state.SelectedPilotID = pilot.ID
	if err = recovery.Commit(state); err != nil {
		t.Fatal(err)
	}
	if err = recovery.PrepareMissionSnapshot(); err != nil {
		t.Fatal(err)
	}
	beforeMission := recovery.State()
	state = recovery.State()
	state.SelectedPilot().Statistics.CareerScore = 1000
	state.SelectedPilot().Theatre = TheatrePhilippines
	state.SelectedPilot().Status = StatusInactive
	if err = recovery.Commit(state); err != nil {
		t.Fatal(err)
	}
	candidate, err := recovery.RecoveryCandidate()
	if err != nil || candidate == nil {
		t.Fatalf("fixture backup missing: %v", err)
	}
	if !reflect.DeepEqual(candidate.State.SelectedPilot(), beforeMission.SelectedPilot()) {
		t.Fatal("fixture pre-mission snapshot differs from intended source")
	}
	recoveryBefore := recovery.State()
	// Validate restore on another copy; retain the authoring fixture unconsumed.
	validation := filepath.Join(t.TempDir(), "restore-validation")
	if err = fixtureSaveCopy(recoveryDir, validation); err != nil {
		t.Fatal(err)
	}
	validationStore, err := OpenSaveStore(validation, "")
	if err != nil {
		t.Fatal(err)
	}
	validationCandidate, err := validationStore.RecoveryCandidate()
	if err != nil {
		t.Fatal(err)
	}
	if err = validationStore.Restore(validationCandidate.ID); err != nil {
		t.Fatal(err)
	}
	restored := validationStore.State()
	if restored.SelectedPilot().Statistics.PhoenixCount() != 2 || restored.SelectedPilot().Statistics.CareerScore != 303 || restored.SelectedPilot().Theatre != TheatrePersianGulf || restored.SelectedPilot().Status != StatusActive {
		t.Fatal("fixture recovery failed")
	}
	if next, err := validationStore.RecoveryCandidate(); err != nil || next != nil {
		t.Fatal("fixture recovery was not consumed")
	}
	migratedFiles, err := fixtureSaveManifest(migrated)
	if err != nil {
		t.Fatal(err)
	}
	recoveryFiles, err := fixtureSaveManifest(recoveryDir)
	if err != nil {
		t.Fatal(err)
	}
	provenance := struct {
		Description     string            `json:"description"`
		Source          string            `json:"protected_source_directory"`
		SourceFiles     map[string]string `json:"source_before_and_after_sha256"`
		OldSaveSHA256   string            `json:"old_committed_save_sha256"`
		MigratedFiles   map[string]string `json:"migrated_acceptance_files_sha256"`
		MigratedState   SaveState         `json:"migrated_acceptance_state"`
		RecoveryFiles   map[string]string `json:"recovery_fixture_files_sha256"`
		PreMission      SaveState         `json:"recovery_pre_mission"`
		PostMission     SaveState         `json:"recovery_fixture_current_state"`
		ExpectedRestore SaveState         `json:"validated_restore_state"`
	}{"Disposable test authoring, not game progression or observed original execution. Source save directory was copied/read only; its every file hash remained unchanged. Recovery fixture is explicitly manufactured through the internal future pre-mission hook.", source, before, assetdata.Hash(oldRaw), migratedFiles, got, recoveryFiles, recoveryBefore, beforeMission, restored}
	// Keep labels accurate: PreMission is the backup state; PostMission is current.
	provenance.PreMission = beforeMission
	provenance.PostMission = recoveryBefore
	raw, err := assetdata.JsonBytes(provenance)
	if err != nil {
		t.Fatal(err)
	}
	if err = writeNewSaveFile(filepath.Join(output, "fixture-provenance.json"), raw); err != nil {
		t.Fatal(err)
	}
	readme := `# Disposable C.1 comparison saves

migrated-acceptance/ is an exact copy of the user's existing Step C save directory,
then migrated by the production loader. Every source file was preserved unchanged
and checked in its complete step-c-before-migration/ archive. Selected Captain and
all typed values survived, with statistics added only from retained established
source fields where they had not previously been modelled.

recovery-fixture/ is a clearly manufactured test state, not an original-game
mission result. Its selected Test Pilot Name / Test Squadron Name is Captain,
emblem1. The pre-mission backup has last101, best202, career303, campaign/missions
packed207 (=2 campaigns and7 missions), Phoenix accumulator-1 (=one recovery),
Training/Persian Gulf/Active. Its current state instead has career1000,
Philippines/Inactive. Confirming recovery should restore only this pilot to
career303/Gulf/Active, consume its backup and produce Phoenix accumulator-2.
That result and absence of repeat recovery were checked on a disposable second
copy; the retained fixture itself remains unconsumed.

Copy either directory to a NEW mutable saves directory before a comparison run.
Set config.cfg saves to that copy, independently of generated assets. These test
states must never become automatic normal-startup defaults. No original file,
previous fixture or source user save was changed. See fixture-provenance.json for
all paths, hashes and expected before/after state.
`
	if err = writeNewSaveFile(filepath.Join(output, "README.md"), []byte(readme)); err != nil {
		t.Fatal(err)
	}
	t.Logf("created two disposable C.1 save fixtures at %s; protected source files verified: %d", output, len(before))
}
