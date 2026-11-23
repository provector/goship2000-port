package state

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestRecoveryTargetIndependentOfPostingAndBackupHash(t *testing.T) {
	s := saveTestStore(t)
	st := commitTestPilot(t, s, "First Pilot")
	id := st.SelectedPilotID
	if err := s.PrepareMissionSnapshot(); err != nil {
		t.Fatal(err)
	}
	st = s.State()
	st.Slots[0].Status, st.Slots[0].LegacyStatus, st.Slots[0].LegacyRaw[0x56] = StatusKIA, 4, 4
	other := saveTestPilot(t, "Other Pilot")
	st.Slots[1] = other
	st.SelectedPilotID = other.ID
	if err := s.Commit(st); err != nil {
		t.Fatal(err)
	}
	canonicalBefore := s.State()
	if v, err := s.RecoveryCandidate(); err != nil || v != nil {
		t.Fatal("posted other unexpectedly owns first backup", err)
	}
	candidate, err := s.RecoveryCandidateForPilot(id)
	if err != nil || candidate == nil || candidate.State.SelectedPilotID != id {
		t.Fatal(candidate, err)
	}
	if err = s.RestoreForPilot(other.ID, candidate.ID); err == nil {
		t.Fatal("other identity consumed first backup")
	}
	if err = s.RestoreForPilot(id, candidate.ID); err != nil {
		t.Fatal(err)
	}
	after := s.State()
	if after.SelectedPilotID != other.ID || after.Slots[0].Status != StatusActive || after.Slots[0].Statistics.PhoenixCounter != -1 {
		t.Fatal(after)
	}
	if !reflect.DeepEqual(after.Slots[1], canonicalBefore.Slots[1]) {
		t.Fatal("unrelated pilot changed")
	}
	if err = s.RestoreForPilot(id, candidate.ID); err == nil {
		t.Fatal("backup consumed twice")
	}
	reopened, err := OpenSaveStore(s.Directory, "")
	if err != nil {
		t.Fatal(err)
	}
	if reopened.State().Slots[0].Statistics.PhoenixCounter != -1 || reopened.State().SelectedPilotID != other.ID {
		t.Fatal("restart changed recovery owner")
	}
	// Recreate an explicit disposable backup solely to test stale/hash and corrupt guards.
	st = reopened.State()
	st.SelectedPilotID = id
	if err = reopened.Commit(st); err != nil {
		t.Fatal(err)
	}
	if err = reopened.PrepareMissionSnapshot(); err != nil {
		t.Fatal(err)
	}
	candidate, err = reopened.RecoveryCandidateForPilot(id)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(s.Directory, filepath.FromSlash(backupFilename(id)))
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(path, append(raw, ' '), 0600); err != nil {
		t.Fatal(err)
	}
	if err = reopened.RestoreForPilot(id, candidate.ID); err == nil {
		t.Fatal("stale hash accepted")
	}
	if err = os.WriteFile(path, []byte("{bad"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err = reopened.RecoveryCandidateForPilot(id); err == nil {
		t.Fatal("corrupt backup accepted")
	}
}
