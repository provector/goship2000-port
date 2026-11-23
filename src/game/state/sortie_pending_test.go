package state

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	assetdata "gunship2000-assets/game/assets"
	"gunship2000-assets/game/flight"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func pendingTestTables() SortieTables {
	t := SortieTables{
		SourceProfile:       assetdata.IslandIce469085.Name,
		RankThresholds:      [12]int16{0, 100, 1000, 3000, 6000, 1000, 3000, 6000, 10000, 15000, 21000, 28000},
		MedalThresholds:     [6]int16{1700, 1300, 1000, 750, 500, 250},
		MedalPenaltyCredits: [6]int16{1500, 1000, 500, 200, 0, 0},
	}
	// Labels are supplied fixture data; persistence does not infer display names
	// or source eligibility from text. Source-name tests belong to preparation.
	for i := range t.RankNames {
		t.RankNames[i] = "supplied rank"
	}
	for i := range t.MedalNames {
		t.MedalNames[i] = "supplied award"
	}
	return t
}

func pendingTestStart() SortieStart {
	return SortieStart{SourceProfile: assetdata.IslandIce469085.Name, MissionDigest: strings.Repeat("a", 64), LaunchDigest: strings.Repeat("b", 64), ClockPolicy: flight.NativeClockPolicy}
}

func pendingTestPilot(t *testing.T, s *SaveStore) *Pilot {
	t.Helper()
	state := commitTestPilot(t, s, "Result fixture")
	p := state.SelectedPilot()
	p.MissionType, p.Rank = MissionSingle, RankCaptain
	binary.LittleEndian.PutUint16(p.LegacyRaw[0x59:], 300)
	if err := s.Commit(state); err != nil {
		t.Fatal(err)
	}
	return s.State().SelectedPilot()
}

func pendingTestProposal(t *testing.T, s *SaveStore, start *SortieStart) *SortieProposal {
	t.Helper()
	// Exact captured Step36 score inputs: no objectives, flagsC0 normalize80,
	// raw0 + selector50, coefficient6/10 ->30; predecline300 ->500.
	f := SortieFacts{SortieID: start.ID, SourceProfile: start.SourceProfile, MissionType: MissionSingle,
		ActorCount: 1, Reason: "base", SourceFlags: 0xc0, Coefficient: 6, ElapsedDA: 128,
		InactiveHeaderPolicy: SortieFreshInactiveHeaders,
		ObjectCount:          80, Targets: make([][62]byte, 5), RecommendedSelectors: []byte{0}, AcceptedSelectors: []byte{0}, SelectorMultisetMatch: true,
		LaunchObjectiveKinds: [2]int16{3, 3}}
	p, err := ProposeSortie(s.State().SelectedPilot(), f, pendingTestTables())
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func pendingTestEntry(t *testing.T) (*SaveStore, *SortieStart, *SortieProposal) {
	t.Helper()
	s := saveTestStore(t)
	pendingTestPilot(t, s)
	start, err := s.BeginSortie(pendingTestStart())
	if err != nil {
		t.Fatal(err)
	}
	return s, start, pendingTestProposal(t, s, start)
}

func TestSortiePendingWitnessCommitOnceAndRestart(t *testing.T) {
	s, start, proposal := pendingTestEntry(t)
	if proposal.Score.Score != 30 || proposal.Score.NormalizedFlags != 0x80 {
		t.Fatal(proposal.Score)
	}
	backupName := filepath.Join(s.Directory, filepath.FromSlash(backupFilename(start.PilotID)))
	if _, err := os.Stat(backupName); !os.IsNotExist(err) {
		t.Fatal("launch created Phoenix", err)
	}
	if err := s.StageSortieResult(start.ID, proposal); err != nil {
		t.Fatal(err)
	}
	if err := s.StageSortieResult(start.ID, proposal); err != nil {
		t.Fatal(err)
	}
	proposal.Facts.ElapsedDA++
	pending, err := s.PendingSortie()
	if err != nil || pending == nil || pending.Proposal.Facts.ElapsedDA != 128 {
		t.Fatal("facts not frozen", err)
	}
	if s.State().SelectedPilot().Statistics.CareerScore != 0 {
		t.Fatal("staging applied career")
	}
	if err := s.CommitSortie(start.ID); err != nil {
		t.Fatal(err)
	}
	state := s.State()
	p := state.SelectedPilot()
	if p.Statistics.CareerScore != 30 || p.Statistics.LastMissionScore != 30 || p.Statistics.MissionCampaignCounter != 1 || int16(binary.LittleEndian.Uint16(p.LegacyRaw[0x59:])) != 500 {
		t.Fatalf("source witness: %+v", p)
	}
	backup, err := decodePilot(mustReadSaveBytes(t, backupName), start.PilotID)
	if err != nil || !reflect.DeepEqual(backup, start.PilotBefore) {
		t.Fatal("wrong completed-mission Phoenix", err)
	}
	if err := s.CommitSortie(start.ID); err != nil {
		t.Fatal(err)
	}
	again, err := OpenSaveStore(s.Directory, "")
	if err != nil {
		t.Fatal(err)
	}
	if err := again.CommitSortie(start.ID); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(state, again.State()) {
		t.Fatal("duplicate/restart reapplied result")
	}
	if pending, err := again.PendingSortie(); err != nil || pending != nil {
		t.Fatal("reentered debrief", err)
	}
}

func TestSortieActiveInterruptedAndUnsupportedPreservePilot(t *testing.T) {
	s, start, _ := pendingTestEntry(t)
	before := s.State()
	duplicate, err := s.BeginSortie(pendingTestStart())
	if err != nil || duplicate.ID != start.ID {
		t.Fatal(err)
	}
	if err := s.Commit(s.State()); err == nil {
		t.Fatal("ordinary save allowed during flight")
	}
	if err := s.PrepareMissionSnapshot(); err == nil {
		t.Fatal("active sortie replaced Phoenix")
	}
	loaded, err := OpenSaveStore(s.Directory, "")
	if err != nil {
		t.Fatal(err)
	}
	if p, err := loaded.PendingSortie(); err != nil || p != nil {
		t.Fatal("unfinished flight resumed", err)
	}
	if err := loaded.CommitSortie(start.ID); err == nil {
		t.Fatal("abandoned callback granted result")
	}
	if !reflect.DeepEqual(before, loaded.State()) {
		t.Fatal("startup changed canonical state")
	}
	if c, err := loaded.RecoveryCandidate(); err != nil || c != nil {
		t.Fatal("unfinished flight manufactured Phoenix", err)
	}
}

func TestSortieMissingCorruptAndChangedResultAreNonDestructive(t *testing.T) {
	for _, kind := range []string{"missing", "corrupt", "altered proposal", "missing reference"} {
		t.Run(kind, func(t *testing.T) {
			s, start, proposal := pendingTestEntry(t)
			if err := s.StageSortieResult(start.ID, proposal); err != nil {
				t.Fatal(err)
			}
			pilotName := filepath.Join(s.Directory, filepath.FromSlash(pilotFilename(start.PilotID)))
			pilotBefore := mustReadSaveBytes(t, pilotName)
			name := filepath.Join(s.Directory, filepath.FromSlash(sortieFilename(start.ID)))
			switch kind {
			case "missing":
				if err := os.Remove(name); err != nil {
					t.Fatal(err)
				}
			case "corrupt":
				if err := os.WriteFile(name, []byte("{broken"), 0600); err != nil {
					t.Fatal(err)
				}
			case "altered proposal":
				doc, err := s.currentSortie()
				if err != nil {
					t.Fatal(err)
				}
				doc.Proposal.Score.Score++
				raw, _ := assetdata.JsonBytes(doc)
				if err := os.WriteFile(name, raw, 0600); err != nil {
					t.Fatal(err)
				}
			case "missing reference":
				if err := os.Remove(filepath.Join(s.Directory, filepath.FromSlash(sortieReferenceFilename(start.PilotID)))); err != nil {
					t.Fatal(err)
				}
			}
			if loaded, err := OpenSaveStore(s.Directory, ""); err != nil {
				t.Fatal("optional session blocked valid pilot", err)
			} else if pending, err := loaded.PendingSortie(); err != nil || pending != nil {
				t.Fatal("invalid session resumed", err)
			}
			if !bytes.Equal(pilotBefore, mustReadSaveBytes(t, pilotName)) {
				t.Fatal("failed pending recovery changed usable pilot/backup")
			}
		})
	}
}

func TestSortieExternalPilotEditIsNotOverwritten(t *testing.T) {
	s, start, proposal := pendingTestEntry(t)
	if err := s.StageSortieResult(start.ID, proposal); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(s.Directory, filepath.FromSlash(pilotFilename(start.PilotID)))
	var edited pilotDocument
	if err := json.Unmarshal(mustReadSaveBytes(t, path), &edited); err != nil {
		t.Fatal(err)
	}
	edited.Pilot.Name = "External edit"
	raw, _ := assetdata.JsonBytes(edited)
	if err := os.WriteFile(path, raw, 0600); err != nil {
		t.Fatal(err)
	}
	if err := s.CommitSortie(start.ID); err == nil {
		t.Fatal("external change overwritten")
	}
	if loaded, err := OpenSaveStore(s.Directory, ""); err != nil {
		t.Fatal("external valid pilot blocked by optional session", err)
	} else if loaded.State().SelectedPilot().Name != "External edit" {
		t.Fatal("external edit lost")
	}
	if !bytes.Equal(raw, mustReadSaveBytes(t, path)) {
		t.Fatal("conflict repair wrote personal edit")
	}
}

func TestSortieExistingDifferentBackupRetainedThroughCommit(t *testing.T) {
	s := saveTestStore(t)
	pendingTestPilot(t, s)
	if err := s.PrepareMissionSnapshot(); err != nil {
		t.Fatal(err)
	}
	old := mustReadSaveBytes(t, filepath.Join(s.Directory, filepath.FromSlash(backupFilename(s.State().SelectedPilotID))))
	next := s.State()
	next.SelectedPilot().Name = "New accepted pilot name"
	if err := s.Commit(next); err != nil {
		t.Fatal(err)
	}
	start, err := s.BeginSortie(pendingTestStart())
	if err != nil || start.OwnBackup {
		t.Fatalf("different usable backup was stolen: %+v %v", start, err)
	}
	if err := s.StageSortieResult(start.ID, pendingTestProposal(t, s, start)); err != nil {
		t.Fatal(err)
	}
	if err := s.CommitSortie(start.ID); err != nil {
		t.Fatal(err)
	}
	current := mustReadSaveBytes(t, filepath.Join(s.Directory, filepath.FromSlash(backupFilename(start.PilotID))))
	if bytes.Equal(old, current) {
		t.Fatal("completed result did not refresh Phoenix")
	}
	recovered, err := decodePilot(current, start.PilotID)
	if err != nil || !reflect.DeepEqual(recovered, start.PilotBefore) {
		t.Fatal("wrong pre-result snapshot", err)
	}
}

func TestSortieCommissionChoiceCommitsAtAcknowledgement(t *testing.T) {
	for _, accepted := range []bool{false, true} {
		t.Run(map[bool]string{false: "refuse", true: "accept"}[accepted], func(t *testing.T) {
			s := saveTestStore(t)
			state := commitTestPilot(t, s, "Supplied commission")
			p := state.SelectedPilot()
			p.MissionType, p.Rank = MissionSingle, RankWO1
			p.Statistics.CareerScore = 1000
			if err := s.Commit(state); err != nil {
				t.Fatal(err)
			}
			start, err := s.BeginSortie(pendingTestStart())
			if err != nil {
				t.Fatal(err)
			}
			facts := pendingTestProposal(t, s, start).Facts
			facts.RawScore, facts.SourceFlags, facts.Coefficient = 200, 0x81, 10
			proposal, err := ProposeSortie(s.State().SelectedPilot(), facts, pendingTestTables())
			if err != nil || !proposal.Commission {
				t.Fatalf("supplied commission prerequisite %+v %v", proposal, err)
			}
			if err := s.StageSortieResult(start.ID, proposal); err != nil {
				t.Fatal(err)
			}
			if err := s.CommitSortie(start.ID); err == nil {
				t.Fatal("offered commission committed without decision")
			}
			if err := s.SetSortieCommission(start.ID, accepted); err != nil {
				t.Fatal(err)
			}
			loaded := s
			pending, err := loaded.PendingSortie()
			if err != nil || pending.CommissionChoice == nil || *pending.CommissionChoice != accepted {
				t.Fatal("decision lost on restart", err)
			}
			want, err := ResolveSortieProposal(proposal, accepted)
			if err != nil {
				t.Fatal(err)
			}
			if err := loaded.CommitSortie(start.ID); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(want, loaded.State().SelectedPilot()) {
				t.Fatal("wrong branch applied")
			}
		})
	}
}

// The expected journal contains independent desired bytes, not an invocation of
// CommitSortie with injected success. Every crash boundary resumes the same final
// pilot/result, and the pre-result recovery is published in the same durable transaction.
func pendingCommitJournal(t *testing.T, s *SaveStore, id string) saveCommit {
	t.Helper()
	doc, err := s.currentSortie()
	if err != nil {
		t.Fatal(err)
	}
	pilot, err := ResolveSortieProposal(doc.Proposal, false)
	if err != nil {
		t.Fatal(err)
	}
	next := s.State()
	next.Slots[0] = pilot
	next.Revision++
	desired, err := stateFiles(next)
	if err != nil {
		t.Fatal(err)
	}
	doc.Phase = SortieCommitted
	doc.CommittedPilot = assetdata.Hash(desired[pilotFilename(pilot.ID)])
	raw, _ := assetdata.JsonBytes(doc)
	recovery, _ := assetdata.JsonBytes(pilotDocument{pilotSchemaVersion, doc.Start.PilotBefore})
	return saveCommit{SchemaVersion: 1, Operations: []saveOperation{
		{Path: pilotFilename(pilot.ID), Before: assetdata.Hash(s.files[pilotFilename(pilot.ID)]), After: desired[pilotFilename(pilot.ID)]},
		{Path: sortieFilename(id), Before: assetdata.Hash(s.files[sortieFilename(id)]), After: raw},
		{Path: saveFilename, Before: assetdata.Hash(s.raw), After: desired[saveFilename]},
		{Path: backupFilename(pilot.ID), Before: doc.Start.BackupSHA256, After: recovery},
	}}
}

func TestSortieCommitJournalResumesEveryBoundary(t *testing.T) {
	for cut := 0; cut <= 4; cut++ {
		t.Run(string(rune('0'+cut)), func(t *testing.T) {
			s, start, proposal := pendingTestEntry(t)
			if err := s.StageSortieResult(start.ID, proposal); err != nil {
				t.Fatal(err)
			}
			journal := pendingCommitJournal(t, s, start.ID)
			raw, _ := assetdata.JsonBytes(journal)
			if err := os.WriteFile(filepath.Join(s.Directory, saveJournal), raw, 0600); err != nil {
				t.Fatal(err)
			}
			for _, op := range journal.Operations[:cut] {
				path := filepath.Join(s.Directory, filepath.FromSlash(op.Path))
				if op.Remove {
					if err := os.Remove(path); err != nil {
						t.Fatal(err)
					}
				} else if err := os.WriteFile(path, op.After, 0600); err != nil {
					t.Fatal(err)
				}
			}
			loaded, err := OpenSaveStore(s.Directory, "")
			if err != nil {
				t.Fatal(err)
			}
			if loaded.State().SelectedPilot().Statistics.CareerScore != 30 || loaded.State().SelectedPilot().Statistics.MissionCampaignCounter != 1 {
				t.Fatal("restart changed frozen outcome")
			}
			state := loaded.State()
			if err := loaded.CommitSortie(start.ID); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(state, loaded.State()) {
				t.Fatal("resume awarded twice")
			}
			if candidate, err := loaded.RecoveryCandidate(); err != nil || candidate == nil {
				t.Fatal("completed recovery missing", err)
			}
		})
	}
}

func TestSortieResultRejectsIdentityChangeAndReplacement(t *testing.T) {
	s, start, proposal := pendingTestEntry(t)
	other := *proposal
	other.Facts.SortieID = strings.Repeat("e", 32)
	rebuilt, err := ProposeSortie(other.PilotBefore, other.Facts, other.Tables)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.StageSortieResult(start.ID, rebuilt); err == nil {
		t.Fatal("foreign sortie proposal accepted")
	}
	if err := s.StageSortieResult(start.ID, proposal); err != nil {
		t.Fatal(err)
	}
	other = *proposal
	other.Facts.RawScore = 1
	rebuilt, err = ProposeSortie(other.PilotBefore, other.Facts, other.Tables)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.StageSortieResult(start.ID, rebuilt); err == nil {
		t.Fatal("immutable result replaced")
	}
	if err := s.AbandonSortie(start.ID, "quit before acknowledgement"); err != nil {
		t.Fatal(err)
	}
	if err := s.StageSortieResult(start.ID, proposal); err == nil {
		t.Fatal("abandoned callback restaged result")
	}
	if err := s.CommitSortie(start.ID); err == nil {
		t.Fatal("abandoned callback committed")
	}
}

func TestSortieAtomicWriteTemporaryIsInertAfterRestart(t *testing.T) {
	s, start, proposal := pendingTestEntry(t)
	if err := s.StageSortieResult(start.ID, proposal); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(s.Directory, "sorties", ".save-pending-interrupted.json")
	raw := []byte("{partial atomic write; not a result")
	if err := os.WriteFile(path, raw, 0600); err != nil {
		t.Fatal(err)
	}
	loaded, err := OpenSaveStore(s.Directory, "")
	if err != nil {
		t.Fatal(err)
	}
	if err := loaded.CommitSortie(start.ID); err == nil {
		t.Fatal("abandoned result committed")
	}
	if !bytes.Equal(raw, mustReadSaveBytes(t, path)) {
		t.Fatal("retained failed atomic write was rewritten/deleted")
	}
}

func TestSortieLateDuplicateDoesNotReplaceLaterPilotState(t *testing.T) {
	s, start, proposal := pendingTestEntry(t)
	if err := s.StageSortieResult(start.ID, proposal); err != nil {
		t.Fatal(err)
	}
	if err := s.CommitSortie(start.ID); err != nil {
		t.Fatal(err)
	}
	second, err := s.BeginSortie(pendingTestStart())
	if err != nil {
		t.Fatal(err)
	}
	before := s.State()
	if err := s.CommitSortie(start.ID); err != nil {
		t.Fatal("late duplicate did not resolve consumed ID", err)
	}
	if !reflect.DeepEqual(before, s.State()) {
		t.Fatal("old consumed pilot overwrote later state")
	}
	pending, err := s.PendingSortie()
	if err != nil || pending == nil || pending.Start.ID != second.ID || pending.Phase != SortieActive {
		t.Fatal("old duplicate changed new sortie", err)
	}
}

func TestSortieNormalizedPilotHashDoesNotRejectUserFormatting(t *testing.T) {
	s := saveTestStore(t)
	pendingTestPilot(t, s)
	path := filepath.Join(s.Directory, filepath.FromSlash(pilotFilename(s.State().SelectedPilotID)))
	var doc pilotDocument
	if err := json.Unmarshal(mustReadSaveBytes(t, path), &doc); err != nil {
		t.Fatal(err)
	}
	compact, err := json.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, compact, 0600); err != nil {
		t.Fatal(err)
	}
	loaded, err := OpenSaveStore(s.Directory, "")
	if err != nil {
		t.Fatal(err)
	}
	start, err := loaded.BeginSortie(pendingTestStart())
	if err != nil {
		t.Fatal(err)
	}
	if err := loaded.StageSortieResult(start.ID, pendingTestProposal(t, loaded, start)); err != nil {
		t.Fatal(err)
	}
	if err := loaded.CommitSortie(start.ID); err != nil {
		t.Fatal(err)
	}
}

func TestSortieEntryJournalResumesEveryBoundary(t *testing.T) {
	for cut := 0; cut <= 4; cut++ {
		t.Run(string(rune('0'+cut)), func(t *testing.T) {
			s := saveTestStore(t)
			pilot := pendingTestPilot(t, s)
			pilotRaw := s.files[pilotFilename(pilot.ID)]
			start := pendingTestStart()
			start.ID, start.PilotID, start.PilotBefore = strings.Repeat("d", 32), pilot.ID, pilot
			start.PilotSHA256, start.BackupSHA256, start.OwnBackup = assetdata.Hash(pilotRaw), assetdata.Hash(pilotRaw), true
			doc := sortieDocument{SchemaVersion: 1, PendingSortie: PendingSortie{Start: start, Phase: SortieActive}}
			docRaw, _ := assetdata.JsonBytes(doc)
			refRaw, _ := assetdata.JsonBytes(sortieReference{sortieDocumentVersion, start.ID, pilot.ID})
			next := s.State()
			next.Revision++
			files, err := stateFiles(next)
			if err != nil {
				t.Fatal(err)
			}
			journal := saveCommit{SchemaVersion: 1, Operations: []saveOperation{
				{Path: backupFilename(pilot.ID), After: pilotRaw},
				{Path: sortieReferenceFilename(pilot.ID), After: refRaw},
				{Path: sortieFilename(start.ID), After: docRaw},
				{Path: saveFilename, Before: assetdata.Hash(s.raw), After: files[saveFilename]},
			}}
			raw, _ := assetdata.JsonBytes(journal)
			if err := os.WriteFile(filepath.Join(s.Directory, saveJournal), raw, 0600); err != nil {
				t.Fatal(err)
			}
			for _, op := range journal.Operations[:cut] {
				path := filepath.Join(s.Directory, filepath.FromSlash(op.Path))
				if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, op.After, 0600); err != nil {
					t.Fatal(err)
				}
			}
			loaded, err := OpenSaveStore(s.Directory, "")
			if err != nil {
				t.Fatal(err)
			}
			pending, err := loaded.PendingSortie()
			if err != nil || pending != nil {
				t.Fatalf("legacy entry resumed: %+v %v", pending, err)
			}
			if !reflect.DeepEqual(pilot, loaded.State().SelectedPilot()) {
				t.Fatal("entry replay changed pilot progression")
			}
			if candidate, err := loaded.RecoveryCandidate(); err != nil || candidate == nil {
				t.Fatal("entry replay lost pre-mission recovery", err)
			}
		})
	}
}
