package state

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	assetdata "gunship2000-assets/game/assets"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func miaRecoveryEntry(t *testing.T) (*SaveStore, *SortieStart, SortieFacts, string, string) {
	t.Helper()
	s := saveTestStore(t)
	pendingTestPilot(t, s)
	// Real normalization/proposal/commit produces the missing pilot and its
	// Phoenix. Only the accepted final inputs/CRT seed are supplied, not an award
	// page or an injected finished pilot status.
	lossStart, err := s.BeginSortie(pendingTestStart())
	if err != nil {
		t.Fatal(err)
	}
	loss := contextFacts()
	loss.SortieID = lossStart.ID
	loss.SourceFlags = 0x20
	loss.Reason = "voluntary"
	loss.FinalPose.H0 = 100 * 32768
	loss.ResultContext.Seed = 415139642
	loss.ResultContext.SelectedScenario = 3
	p, err := ProposeSortie(lossStart.PilotBefore, loss, pendingTestTables())
	if err != nil {
		t.Fatal(err)
	}
	if p.AcceptedPilot.Status != StatusMIA {
		t.Fatal("source seed no longer produces MIA")
	}
	if err = s.StageSortieResult(lossStart.ID, p); err != nil {
		t.Fatal(err)
	}
	if err = s.CommitSortie(lossStart.ID); err != nil {
		t.Fatal(err)
	}
	missingID := s.State().SelectedPilotID
	next := s.State()
	player := cloneSortiePilot(lossStart.PilotBefore)
	player.ID, player.Name = "n5-rescue-player", "Rescue Leader"
	player.Statistics.CareerScore = 10200
	sortiePutLong(player, 0x7a, 6500)
	player.LegacyRaw[0x55], player.LegacyRaw[0x54], player.LegacyRaw[0x56] = 5, 7, 1
	player.LegacyStatus = 1
	next.Slots[3], next.SelectedPilotID = player, player.ID
	if err = s.Commit(next); err != nil {
		t.Fatal(err)
	}
	start, err := s.BeginSortie(pendingTestStart())
	if err != nil {
		t.Fatal(err)
	}
	f := contextFacts()
	f.SortieID = start.ID
	f.SourceFlags = 0x81
	f.ResultContext.SelectedScenario = 3
	f.ResultContext.Actors[0].Flags = f.SourceFlags
	f.ResultContext.Actors[0].Ship[0] = 0x1e // landed6 + actual retained cargo18.
	launch := make([][62]byte, 5)
	for i := range launch[2] {
		launch[2][i] = byte(i*7 + 3)
	}
	binary.LittleEndian.PutUint16(launch[2][32:], 2)
	f.Targets = append([][62]byte(nil), launch...)
	f.Targets[2][32], f.Targets[2][33] = 0, 0
	f.ResultContext.MissionRecovery = &MissionRecoveryContext{Policy: MissionRecoveryPolicy, LaunchTargets: launch, ObjectiveAssignments: [2]uint16{1, 0}, Candidates: start.RecoveryRoster}
	return s, start, f, missingID, lossStart.ID
}

func TestMissionMiaRecoveryAtomicStatusOnlyAndPhoenix(t *testing.T) {
	s, start, f, missingID, oldLoss := miaRecoveryEntry(t)
	p, err := ProposeSortie(start.PilotBefore, f, pendingTestTables())
	if err != nil {
		t.Fatal(err)
	}
	if p.RecoveredPilot == nil || p.RecoveredPilot.Before.ID != missingID || p.RecoveredPilot.SourceRow != 3 {
		t.Fatal("native stable slot0 was incorrectly excluded")
	}
	if p.InitialRankRecommendation != 8 || p.AcceptedPilot.Rank != RankCaptain || p.RecommendedRank != 0 {
		t.Fatal("special JUMP did not bypass ordinary promotion", p.InitialRankRecommendation, p.AcceptedPilot.Rank, p.RecommendedRank)
	}
	backup := mustReadSaveBytes(t, filepath.Join(s.Directory, backupFilename(missingID)))
	before := cloneSortiePilot(p.RecoveredPilot.Before)
	if err = s.StageSortieResult(start.ID, p); err != nil {
		t.Fatal(err)
	}
	if s.State().Slots[0].Status != StatusMIA {
		t.Fatal("unacknowledged result revived pilot")
	}
	if err = s.CommitSortie(start.ID); err != nil {
		t.Fatal(err)
	}
	want := cloneSortiePilot(before)
	want.Status, want.LegacyStatus, want.LegacyRaw[0x56] = StatusActive, 1, 1
	if !reflect.DeepEqual(s.State().Slots[0], want) {
		t.Fatal("recovery changed more than status")
	}
	if !bytes.Equal(backup, mustReadSaveBytes(t, filepath.Join(s.Directory, backupFilename(missingID)))) {
		t.Fatal("mission rescue consumed/replaced other Phoenix")
	}
	again, err := OpenSaveStore(s.Directory, "")
	if err != nil {
		t.Fatal(err)
	}
	if err = again.CommitSortie(start.ID); err != nil {
		t.Fatal(err)
	}
	next := again.State()
	next.SelectedPilotID = missingID
	if err = again.Commit(next); err != nil {
		t.Fatal(err)
	}
	if err = again.CommitSortie(oldLoss); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(again.State().SelectedPilot(), want) {
		t.Fatal("prior consumed loss re-awarded after mission rescue")
	}
	candidate, err := again.RecoveryCandidate()
	if err != nil || candidate == nil {
		t.Fatal("other pilot independent Phoenix unavailable", err)
	}
}

func TestMissionRecoveryMissingPrerequisitesAndAmbiguity(t *testing.T) {
	_, start, base, _, _ := miaRecoveryEntry(t)
	for _, tc := range []struct {
		name      string
		change    func(*SortieFacts)
		wantError bool
	}{
		{"no-return", func(f *SortieFacts) {
			f.SourceFlags &^= 0x80
			f.Reason = "voluntary"
			f.ResultContext.Actors[0].Flags = f.SourceFlags
		}, false},
		{"no-completion", func(f *SortieFacts) { f.SourceFlags &^= 1; f.ResultContext.Actors[0].Flags = f.SourceFlags }, false},
		{"drop-cargo", func(f *SortieFacts) { f.ResultContext.Actors[0].Ship[0] &= 7 }, false},
		{"wrong-assigned-actor", func(f *SortieFacts) { f.ResultContext.MissionRecovery.ObjectiveAssignments[0] = 2 }, false},
		{"different-scenario", func(f *SortieFacts) { f.ResultContext.SelectedScenario = 4 }, false},
		{"no-mia-retasked-target", func(f *SortieFacts) { f.ResultContext.MissionRecovery.Candidates = nil; f.Targets[2][61] ^= 1 }, false},
		{"nonmatching-mia-retasked-target", func(f *SortieFacts) { f.ResultContext.SelectedScenario = 4; f.Targets[2][61] ^= 1 }, false},
		{"replacement-tail", func(f *SortieFacts) { f.Targets[2][61] ^= 1 }, true},
		{"wrong-fingerprint", func(f *SortieFacts) { f.ResultContext.MissionRecovery.Candidates[0].BeforeSHA256 = "" }, true},
		{"ambiguous", func(f *SortieFacts) {
			r := f.ResultContext.MissionRecovery.Candidates[0]
			r.SourceRow = 4
			r.Before = cloneSortiePilot(r.Before)
			r.Before.ID = "another-mia"
			raw, _ := assetdata.JsonBytes(pilotDocument{pilotSchemaVersion, r.Before})
			r.BeforeSHA256 = assetdata.Hash(raw)
			f.ResultContext.MissionRecovery.Candidates = append(f.ResultContext.MissionRecovery.Candidates, r)
		}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			raw, _ := json.Marshal(base)
			var f SortieFacts
			if err := json.Unmarshal(raw, &f); err != nil {
				t.Fatal(err)
			}
			tc.change(&f)
			p, err := ProposeSortie(start.PilotBefore, f, pendingTestTables())
			if tc.wantError {
				if err == nil {
					t.Fatal("unsafe recovery admitted")
				}
				return
			}
			if err != nil || p.RecoveredPilot != nil {
				t.Fatal("ineligible recovery", err)
			}
		})
	}
}

func TestMissionRecoveryQuitAndExternalConflict(t *testing.T) {
	for _, external := range []bool{false, true} {
		t.Run(map[bool]string{false: "abandon", true: "external"}[external], func(t *testing.T) {
			s, start, f, missingID, _ := miaRecoveryEntry(t)
			p, err := ProposeSortie(start.PilotBefore, f, pendingTestTables())
			if err != nil {
				t.Fatal(err)
			}
			if err = s.StageSortieResult(start.ID, p); err != nil {
				t.Fatal(err)
			}
			if external {
				path := filepath.Join(s.Directory, pilotFilename(missingID))
				raw := mustReadSaveBytes(t, path)
				if err = os.WriteFile(path, append(raw, ' '), 0600); err != nil {
					t.Fatal(err)
				}
				if err = s.CommitSortie(start.ID); err == nil {
					t.Fatal("overwrote externally changed recovery owner")
				}
			} else {
				q, err := OpenSaveStore(s.Directory, "")
				if err != nil {
					t.Fatal(err)
				}
				if q.State().Slots[0].Status != StatusMIA {
					t.Fatal("restart revived uncommitted MIA")
				}
				if err = q.CommitSortie(start.ID); err == nil {
					t.Fatal("abandoned callback committed")
				}
			}
		})
	}
}

func TestMissionRecoveryCannotDropAcceptedOwnerContext(t *testing.T) {
	s, start, f, _, _ := miaRecoveryEntry(t)
	f.ResultContext.MissionRecovery = nil
	p, err := ProposeSortie(start.PilotBefore, f, pendingTestTables())
	if err != nil {
		t.Fatal(err)
	}
	if err = s.StageSortieResult(start.ID, p); err == nil {
		t.Fatal("omitted frozen MIA owner admitted")
	}
}

func TestMissionRecoveryJournalEveryPublicationBoundary(t *testing.T) {
	for cut := 0; cut <= 5; cut++ {
		t.Run(string(rune('0'+cut)), func(t *testing.T) {
			s, start, f, missingID, _ := miaRecoveryEntry(t)
			p, err := ProposeSortie(start.PilotBefore, f, pendingTestTables())
			if err != nil {
				t.Fatal(err)
			}
			if err = s.StageSortieResult(start.ID, p); err != nil {
				t.Fatal(err)
			}
			oldOtherBackup := mustReadSaveBytes(t, filepath.Join(s.Directory, backupFilename(missingID)))
			doc, err := s.currentSortie()
			if err != nil {
				t.Fatal(err)
			}
			player, err := ResolveSortieProposal(p, false)
			if err != nil {
				t.Fatal(err)
			}
			peer := cloneSortiePilot(p.RecoveredPilot.Before)
			peer.Status, peer.LegacyStatus, peer.LegacyRaw[0x56] = StatusActive, 1, 1
			next := s.State()
			next.Slots[0], next.Slots[3] = peer, player
			next.Revision++
			desired, err := stateFiles(next)
			if err != nil {
				t.Fatal(err)
			}
			doc.Phase = SortieCommitted
			doc.CommittedPilot = assetdata.Hash(desired[pilotFilename(player.ID)])
			doc.CommittedRecovery = assetdata.Hash(desired[pilotFilename(peer.ID)])
			docRaw, _ := assetdata.JsonBytes(doc)
			backup, _ := assetdata.JsonBytes(pilotDocument{pilotSchemaVersion, start.PilotBefore})
			journal := saveCommit{SchemaVersion: 1, Operations: []saveOperation{
				{Path: pilotFilename(player.ID), Before: assetdata.Hash(s.files[pilotFilename(player.ID)]), After: desired[pilotFilename(player.ID)]},
				{Path: pilotFilename(peer.ID), Before: assetdata.Hash(s.files[pilotFilename(peer.ID)]), After: desired[pilotFilename(peer.ID)]},
				{Path: sortieFilename(start.ID), Before: assetdata.Hash(s.files[sortieFilename(start.ID)]), After: docRaw},
				{Path: saveFilename, Before: assetdata.Hash(s.raw), After: desired[saveFilename]},
				{Path: backupFilename(player.ID), Before: start.BackupSHA256, After: backup},
			}}
			raw, _ := assetdata.JsonBytes(journal)
			if err = os.WriteFile(filepath.Join(s.Directory, saveJournal), raw, 0600); err != nil {
				t.Fatal(err)
			}
			for _, op := range journal.Operations[:cut] {
				if err = os.WriteFile(filepath.Join(s.Directory, filepath.FromSlash(op.Path)), op.After, 0600); err != nil {
					t.Fatal(err)
				}
			}
			q, err := OpenSaveStore(s.Directory, "")
			if err != nil {
				t.Fatal(err)
			}
			if q.State().Slots[0].Status != StatusActive || q.State().SelectedPilot().Statistics.CareerScore != 10300 || q.State().SelectedPilot().Rank != RankCaptain {
				t.Fatal("wrong recovered outcome at cut", cut)
			}
			if err = q.CommitSortie(start.ID); err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(oldOtherBackup, mustReadSaveBytes(t, filepath.Join(q.Directory, backupFilename(missingID)))) {
				t.Fatal("peer Phoenix changed during journal recovery")
			}
		})
	}
}
