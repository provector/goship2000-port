package state

import (
	"bytes"
	"encoding/json"
	assetdata "gunship2000-assets/game/assets"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestCompletedRecoverySurvivesNextUnfinishedSortieAndUndo(t *testing.T) {
	s, start, proposal := pendingTestEntry(t)
	if err := s.StageSortieResult(start.ID, proposal); err != nil {
		t.Fatal(err)
	}
	if err := s.CommitSortie(start.ID); err != nil {
		t.Fatal(err)
	}
	previous := mustReadSaveBytes(t, filepath.Join(s.Directory, backupFilename(start.PilotID)))
	// Independent legitimate settings/roster writes must survive abandonment.
	next := s.State()
	next.SelectedPilot().Name = "Saved setting and roster edit"
	next.SelectedPilot().Preflight.Avoidance = 1
	other, err := NewPilot()
	if err != nil {
		t.Fatal(err)
	}
	other.Name = "Unrelated pilot"
	next.Slots[1] = other
	if err := s.Commit(next); err != nil {
		t.Fatal(err)
	}
	canonical := s.State()
	second, err := s.BeginSortie(pendingTestStart())
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(previous, mustReadSaveBytes(t, filepath.Join(s.Directory, backupFilename(start.PilotID)))) {
		t.Fatal("launch replaced last completed recovery")
	}
	if err := s.StageSortieResult(second.ID, pendingTestProposal(t, s, second)); err != nil {
		t.Fatal(err)
	}
	if err := s.AbandonUnfinishedSorties("application quit"); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(canonical.Slots, s.State().Slots) {
		t.Fatal("quit rolled back legitimate prior writes")
	}
	if err := s.CommitSortie(second.ID); err == nil {
		t.Fatal("abandoned result awarded")
	}
	reloaded, err := OpenSaveStore(s.Directory, "")
	if err != nil {
		t.Fatal(err)
	}
	candidate, err := reloaded.RecoveryCandidate()
	if err != nil || candidate == nil {
		t.Fatal("previous Phoenix lost", err)
	}
	if err := reloaded.Restore(candidate.ID); err != nil {
		t.Fatal(err)
	}
	restored := reloaded.State()
	want := *start.PilotBefore
	stats := *want.Statistics
	want.Statistics = &stats
	want.Statistics.PhoenixCounter--
	if !reflect.DeepEqual(&want, restored.SelectedPilot()) {
		t.Fatal("Phoenix did not restore only selected pre-result pilot")
	}
	if !reflect.DeepEqual(canonical.Slots[1], restored.Slots[1]) {
		t.Fatal("Phoenix rolled back another pilot")
	}
	if err := reloaded.Restore(candidate.ID); err == nil {
		t.Fatal("Phoenix consumed twice")
	}
	if err := reloaded.CommitSortie(start.ID); err != nil {
		t.Fatal("consumed identity not retained", err)
	}
	if !reflect.DeepEqual(restored, reloaded.State()) {
		t.Fatal("old result re-awarded after Phoenix")
	}
	again, err := OpenSaveStore(s.Directory, "")
	if err != nil {
		t.Fatal(err)
	}
	if err := again.CommitSortie(start.ID); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(restored, again.State()) {
		t.Fatal("restart re-awarded undone result")
	}
}

func TestOldPendingChoiceAndCorruptOptionalMetadataAbandoned(t *testing.T) {
	for _, kind := range []string{"old active", "old pending", "corrupt reference", "oversized payload", "unknown optional file"} {
		t.Run(kind, func(t *testing.T) {
			s, start, proposal := pendingTestEntry(t)
			if err := s.StageSortieResult(start.ID, proposal); err != nil {
				t.Fatal(err)
			}
			before := s.State()
			path := filepath.Join(s.Directory, sortieFilename(start.ID))
			switch kind {
			case "old active", "old pending":
				doc, err := s.currentSortie()
				if err != nil {
					t.Fatal(err)
				}
				doc.SchemaVersion = 1
				if kind == "old active" {
					doc.Phase = SortieActive
					doc.Proposal = nil
					doc.ResultSHA256 = ""
				}
				raw, _ := assetdata.JsonBytes(doc)
				if err := os.WriteFile(path, raw, 0600); err != nil {
					t.Fatal(err)
				}
			case "corrupt reference":
				if err := os.WriteFile(filepath.Join(s.Directory, sortieReferenceFilename(start.PilotID)), []byte("broken"), 0600); err != nil {
					t.Fatal(err)
				}
			case "oversized payload":
				if err := os.WriteFile(path, bytes.Repeat([]byte("?"), 2<<20), 0600); err != nil {
					t.Fatal(err)
				}
			case "unknown optional file":
				if err := os.WriteFile(filepath.Join(s.Directory, "sorties", "orphan.json"), []byte("retained raw failure"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			loaded, err := OpenSaveStore(s.Directory, "")
			if err != nil {
				t.Fatal("optional metadata blocked canonical pilot", err)
			}
			if !reflect.DeepEqual(before, loaded.State()) {
				t.Fatal("startup awarded/rolled back state")
			}
			entries, err := os.ReadDir(filepath.Join(s.Directory, abandonedSessionDirectory))
			if err != nil || len(entries) < 2 {
				t.Fatal("raw session not retained", err)
			}
			if err := s.CommitSortie(start.ID); err == nil {
				t.Fatal("stale pre-restart callback awarded")
			}
			if pending, err := loaded.PendingSortie(); err != nil || pending != nil {
				t.Fatal("session resumed", err)
			}
		})
	}
}

func TestFinalAckJournalWinsOverQuitAtEveryBoundary(t *testing.T) {
	for cut := 0; cut <= 4; cut++ {
		t.Run(string(rune('0'+cut)), func(t *testing.T) {
			s, start, p := pendingTestEntry(t)
			if err := s.StageSortieResult(start.ID, p); err != nil {
				t.Fatal(err)
			}
			journal := pendingCommitJournal(t, s, start.ID)
			raw, _ := assetdata.JsonBytes(journal)
			if err := os.WriteFile(filepath.Join(s.Directory, saveJournal), raw, 0600); err != nil {
				t.Fatal(err)
			}
			for _, op := range journal.Operations[:cut] {
				if err := os.WriteFile(filepath.Join(s.Directory, op.Path), op.After, 0600); err != nil {
					t.Fatal(err)
				}
			}
			if err := s.AbandonUnfinishedSorties("quit after durable final acknowledgement"); err != nil {
				t.Fatal(err)
			}
			if s.State().SelectedPilot().Statistics.CareerScore != 30 {
				t.Fatal("durable acknowledged result discarded")
			}
			if candidate, err := s.RecoveryCandidate(); err != nil || candidate == nil {
				t.Fatal("durable recovery lost", err)
			}
			state := s.State()
			if err := s.CommitSortie(start.ID); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(state, s.State()) {
				t.Fatal("durable result awarded twice")
			}
		})
	}
}

func TestCommitProtectsNewOrEditedRecovery(t *testing.T) {
	s, start, p := pendingTestEntry(t)
	if err := s.StageSortieResult(start.ID, p); err != nil {
		t.Fatal(err)
	}
	// Creation after launch is a conflict too, even though launch recorded absence.
	backup, _ := assetdata.JsonBytes(pilotDocument{pilotSchemaVersion, start.PilotBefore})
	if err := os.WriteFile(filepath.Join(s.Directory, backupFilename(start.PilotID)), backup, 0600); err != nil {
		t.Fatal(err)
	}
	before := s.State()
	if err := s.CommitSortie(start.ID); err == nil {
		t.Fatal("external recovery creation overwritten")
	}
	if !reflect.DeepEqual(before, s.State()) {
		t.Fatal("conflicting recovery partially committed")
	}
	if _, err := os.Stat(filepath.Join(s.Directory, saveJournal)); !os.IsNotExist(err) {
		t.Fatal("conflict made a durable result")
	}
}

func TestJournalChecksAllExternalConflictsBeforeRemainingWrites(t *testing.T) {
	s, start, p := pendingTestEntry(t)
	if err := s.StageSortieResult(start.ID, p); err != nil {
		t.Fatal(err)
	}
	journal := pendingCommitJournal(t, s, start.ID)
	// Conflict on the final payload must not mutate the earlier pilot/result.
	external := []byte("external recovery; retain verbatim")
	backup := filepath.Join(s.Directory, backupFilename(start.PilotID))
	if err := os.WriteFile(backup, external, 0600); err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(journal)
	if err := os.WriteFile(filepath.Join(s.Directory, saveJournal), raw, 0600); err != nil {
		t.Fatal(err)
	}
	before := mustReadSaveBytes(t, filepath.Join(s.Directory, pilotFilename(start.PilotID)))
	if _, err := OpenSaveStore(s.Directory, ""); err == nil || !strings.Contains(err.Error(), "conflict") {
		t.Fatal("edited recovery was not protected", err)
	}
	if !bytes.Equal(before, mustReadSaveBytes(t, filepath.Join(s.Directory, pilotFilename(start.PilotID)))) || !bytes.Equal(external, mustReadSaveBytes(t, backup)) {
		t.Fatal("conflicting journal performed partial new writes")
	}
}

func TestUnacknowledgedCommissionChoiceIsAbandoned(t *testing.T) {
	for _, accepted := range []bool{false, true} {
		t.Run(map[bool]string{false: "refused", true: "accepted"}[accepted], func(t *testing.T) {
			s := saveTestStore(t)
			st := commitTestPilot(t, s, "Supplied pending commission")
			p := st.SelectedPilot()
			p.MissionType, p.Rank = MissionSingle, RankWO1
			p.Statistics.CareerScore = 1000
			if err := s.Commit(st); err != nil {
				t.Fatal(err)
			}
			before := s.State().SelectedPilot()
			start, err := s.BeginSortie(pendingTestStart())
			if err != nil {
				t.Fatal(err)
			}
			facts := pendingTestProposal(t, s, start).Facts
			facts.RawScore, facts.SourceFlags, facts.Coefficient = 200, 0x81, 10
			proposal, err := ProposeSortie(before, facts, pendingTestTables())
			if err != nil || !proposal.Commission {
				t.Fatal("fixture not eligible", err)
			}
			if err := s.StageSortieResult(start.ID, proposal); err != nil {
				t.Fatal(err)
			}
			if err := s.SetSortieCommission(start.ID, accepted); err != nil {
				t.Fatal(err)
			}
			loaded, err := OpenSaveStore(s.Directory, "")
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(before, loaded.State().SelectedPilot()) {
				t.Fatal("unacknowledged choice granted rank/award")
			}
			if err := s.SetSortieCommission(start.ID, !accepted); err == nil {
				t.Fatal("stale choice callback accepted")
			}
			if err := s.CommitSortie(start.ID); err == nil {
				t.Fatal("abandoned commission committed")
			}
		})
	}
}
