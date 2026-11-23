package state

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	assetdata "gunship2000-assets/game/assets"
	"gunship2000-assets/internal/testfixture"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

// Supplied arithmetic/row fixture, not an earned sortie. The branch answers
// follow the independent GS A1e4fd..1e527/section3:2cde operands in Step47.
func commissionFixture(t *testing.T, actors int) (*Pilot, SortieFacts) {
	t.Helper()
	p, f := flightFixture(t)
	p.Rank = RankWO1
	p.Statistics.CareerScore = 1000
	f.SourceFlags = 0x83
	f.RawScore = 0
	f.ResultContext.Actors[0].RankRaw = 1
	for i := range p.SquadronPilots {
		row := &p.SquadronPilots[i]
		row.Rank = RankWOC
		row.LegacyRaw[34] = 0
		copy(p.LegacyRaw[0x55+41*(i+1):], row.LegacyRaw)
		a := &f.ResultContext.Actors[i+1]
		a.RankRaw, a.RawB, a.Ship[29] = 0, 25, 25
		copy(a.StoredRaw[:], row.LegacyRaw)
	}
	if actors == 1 {
		p.MissionType, f.MissionType = MissionSingle, MissionSingle
		f.ActorCount = 1
		f.AcceptedSelectors, f.RecommendedSelectors = []byte{0}, []byte{2}
		f.ResultContext.Actors = f.ResultContext.Actors[:1]
		f.InactiveHeaderPolicy = SortieFreshInactiveHeaders
	}
	f.ResultContext.ReplacementNames = make([]string, 74)
	for i := range f.ResultContext.ReplacementNames {
		f.ResultContext.ReplacementNames[i] = fmt.Sprintf("NAME%02d", i)
	}
	f.ResultContext.OccupiedNames = []string{p.Name}
	return p, f
}

// Journal-only fixture: source branch/name arithmetic is asserted separately.
func pendingNames(t *testing.T) (*SaveStore, *SortieStart, *SortieProposal, *SortieCrewChoices) {
	t.Helper()
	s := saveTestStore(t)
	st := commitTestPilot(t, s, "Journal Commander")
	p, f := commissionFixture(t, 1)
	p.ID, p.Name = st.Slots[0].ID, st.Slots[0].Name
	st.Slots[0] = p
	if e := s.Commit(st); e != nil {
		t.Fatal(e)
	}
	start, e := s.BeginSortie(pendingTestStart())
	if e != nil {
		t.Fatal(e)
	}
	f.SortieID = start.ID
	out, e := ProposeSortie(start.PilotBefore, f, nResultTables())
	if e != nil {
		t.Fatal(e)
	}
	if e = s.StageSortieResult(start.ID, out); e != nil {
		t.Fatal(e)
	}
	if e = s.SetSortieCommission(start.ID, true); e != nil {
		t.Fatal(e)
	}
	c := DefaultSortieCrewChoices(out)
	c.Names = []string{"ONE", "TWO", "THREE", "FOUR"}
	if e = s.SetSortieCrewChoices(start.ID, *c); e != nil {
		t.Fatal(e)
	}
	return s, start, out, c
}

func TestRenamedCrewDurableJournalAndAbandonment(t *testing.T) {
	for cut := 0; cut <= 4; cut++ {
		t.Run(fmt.Sprintf("journal-cut%d", cut), func(t *testing.T) {
			s, start, out, names := pendingNames(t)
			pilot, e := ResolveSortieProposalWithChoices(out, true, names)
			if e != nil {
				t.Fatal(e)
			}
			next := s.State()
			next.Slots[0], next.Revision = pilot, next.Revision+1
			desired, e := stateFiles(next)
			if e != nil {
				t.Fatal(e)
			}
			doc, e := s.currentSortie()
			if e != nil {
				t.Fatal(e)
			}
			doc.Phase, doc.CommittedPilot = SortieCommitted, assetdata.Hash(desired[pilotFilename(pilot.ID)])
			raw, e := assetdata.JsonBytes(doc)
			if e != nil {
				t.Fatal(e)
			}
			recovery, e := assetdata.JsonBytes(pilotDocument{pilotSchemaVersion, start.PilotBefore})
			if e != nil {
				t.Fatal(e)
			}
			j := saveCommit{SchemaVersion: 1, Operations: []saveOperation{
				{Path: pilotFilename(pilot.ID), Before: assetdata.Hash(s.files[pilotFilename(pilot.ID)]), After: desired[pilotFilename(pilot.ID)]},
				{Path: sortieFilename(start.ID), Before: assetdata.Hash(s.files[sortieFilename(start.ID)]), After: raw},
				{Path: saveFilename, Before: assetdata.Hash(s.raw), After: desired[saveFilename]},
				{Path: backupFilename(pilot.ID), Before: start.BackupSHA256, After: recovery},
			}}
			b, _ := assetdata.JsonBytes(j)
			if e = os.WriteFile(filepath.Join(s.Directory, saveJournal), b, 0600); e != nil {
				t.Fatal(e)
			}
			for _, op := range j.Operations[:cut] {
				path := filepath.Join(s.Directory, filepath.FromSlash(op.Path))
				if e = os.MkdirAll(filepath.Dir(path), 0700); e != nil {
					t.Fatal(e)
				}
				if e = os.WriteFile(path, op.After, 0600); e != nil {
					t.Fatal(e)
				}
			}
			loaded, e := OpenSaveStore(s.Directory, "")
			if e != nil {
				t.Fatal(e)
			}
			for i, row := range loaded.State().SelectedPilot().SquadronPilots {
				if row.Name != names.Names[i] {
					t.Fatal("journal lost accepted name", i)
				}
			}
			before := loaded.State()
			if e = loaded.CommitSortie(start.ID); e != nil {
				t.Fatal(e)
			}
			if !reflect.DeepEqual(before, loaded.State()) {
				t.Fatal("durable restart repeated awards/name decision")
			}
		})
	}
	t.Run("abandon", func(t *testing.T) {
		s, start, _, _ := pendingNames(t)
		before := mustReadSaveBytes(t, filepath.Join(s.Directory, pilotFilename(start.PilotID)))
		loaded, e := OpenSaveStore(s.Directory, "")
		if e != nil {
			t.Fatal(e)
		}
		if !bytes.Equal(before, mustReadSaveBytes(t, filepath.Join(s.Directory, pilotFilename(start.PilotID)))) {
			t.Fatal("unfinished restart changed canonical crew")
		}
		if e = loaded.CommitSortie(start.ID); e == nil {
			t.Fatal("abandoned choices acquired publication")
		}
	})
	t.Run("external-conflict", func(t *testing.T) {
		s, start, _, _ := pendingNames(t)
		path := filepath.Join(s.Directory, pilotFilename(start.PilotID))
		before := append(mustReadSaveBytes(t, path), '\n')
		if e := os.WriteFile(path, before, 0600); e != nil {
			t.Fatal(e)
		}
		if e := s.CommitSortie(start.ID); e == nil {
			t.Fatal("external pilot fingerprint conflict ignored")
		}
		if !bytes.Equal(before, mustReadSaveBytes(t, path)) {
			t.Fatal("conflict overwrote external bytes")
		}
	})
}

func TestActualCommissionBranchesAndReplacementNames(t *testing.T) {
	for _, actors := range []int{1, 5} {
		t.Run(fmt.Sprintf("actors%d", actors), func(t *testing.T) {
			p, f := commissionFixture(t, actors)
			original, _ := json.Marshal(p)
			out := nProposal(t, p, f)
			if !out.Commission || out.Version != 4 {
				t.Fatal("fixture did not reach commission offer", out.CommanderRank)
			}
			if out.Result.CrewPro || out.Result.ReplacementCount != 4 || !out.Result.ReplacementInitial {
				t.Fatalf("accepted source branch %+v", out.Result)
			}
			refused := SortieResultForChoice(out, false)
			if refused == nil || refused.CrewPro != (actors > 1) || refused.ReplacementCount != map[bool]int{false: 0, true: 4}[actors > 1] {
				t.Fatalf("refused source branch %+v", refused)
			}
			for _, accepted := range []bool{false, true} {
				q, err := ResolveSortieProposal(out, accepted)
				if err != nil {
					t.Fatal(err)
				}
				if !accepted && actors == 1 && !reflect.DeepEqual(q.SquadronPilots, p.SquadronPilots) {
					t.Fatal("refused single commission replaced stored crew")
				}
				if accepted || actors > 1 {
					seen := map[int]bool{}
					for i, row := range q.SquadronPilots {
						if row.Rank != RankWO1 || seen[row.Portrait] || row.Name == p.SquadronPilots[i].Name {
							t.Fatal("replacement identity/rank/portrait", i, row)
						}
						seen[row.Portrait] = true
					}
				}
			}
			choices := DefaultSortieCrewChoices(out)
			choices.Names = []string{"ABCDEFGHIJ", "", "Duplicate", "Duplicate"}
			q, err := ResolveSortieProposalWithChoices(out, true, choices)
			if err != nil {
				t.Fatal(err)
			}
			if q.SquadronPilots[0].Name != choices.Names[0] || q.SquadronPilots[1].Name != out.Result.Replacements[1].Name || q.SquadronPilots[2].Name != q.SquadronPilots[3].Name {
				t.Fatal("sparse/duplicate source name semantics")
			}
			for i, row := range q.SquadronPilots {
				if !bytes.Equal(row.LegacyRaw[26:], out.Result.Replacements[i].LegacyRaw[26:]) {
					t.Fatal("rename changed rank/awards/portrait/tail", i)
				}
			}
			if actors == 1 {
				if _, err = ResolveSortieProposalWithChoices(out, false, choices); err == nil {
					t.Fatal("names accepted without source REPLACE page")
				}
				for _, draw := range refused.RNG.Draws {
					if len(draw.Site) >= 5 && draw.Site[:5] == "crew1" && draw.Site != "crew1-wound" {
						t.Fatal("refusal consumed replacement RNG", draw)
					}
				}
			}
			after, _ := json.Marshal(p)
			if !bytes.Equal(original, after) {
				t.Fatal("proposal changed original source rows")
			}
			if err = ValidateSortieProposal(out); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestSourceNameLimitsAndHistoricalProposalIsolation(t *testing.T) {
	for _, s := range []string{"A", "A B", "ABCDEFGHIJ", string([]byte{'A', 0x7f})} {
		if err := ValidateReplacementName(s); err != nil {
			t.Fatal(s, err)
		}
	}
	for _, s := range []string{"", " Leading", "ABCDEFGHIJK", "\tABC", "A\x80", "A\x00"} {
		if ValidateReplacementName(s) == nil {
			t.Fatalf("accepted unsupported name %q", s)
		}
	}
	p, f := commissionFixture(t, 1)
	for _, v := range []int{1, 2, 3} {
		out, err := proposeSortieVersion(p, f, nResultTables(), v)
		if err != nil {
			t.Fatal(err)
		}
		if out.RefusedResult != nil || out.Result.ReplacementCount != 0 || out.Result.ReplacementInitial {
			t.Fatal("new branch fields leaked into historical interpretation", v)
		}
		if err = ValidateSortieProposal(out); err != nil {
			t.Fatal(err)
		}
		if _, err = ResolveSortieProposalWithChoices(out, true, &SortieCrewChoices{Recipients: DefaultSortieCrewChoices(out).Recipients, Names: []string{"NEW", "", "", ""}}); err == nil {
			t.Fatal("historical name decision accepted", v)
		}
	}
}

func TestReplacementSourceRowsAndBypassBranches(t *testing.T) {
	p, f := commissionFixture(t, 5)
	p.Rank = RankCaptain
	p.Statistics.CareerScore = 0
	for i := range p.SquadronPilots {
		row := &p.SquadronPilots[i]
		row.Rank, row.LegacyRaw[34] = RankWO1, 1
		copy(p.LegacyRaw[0x55+41*(i+1):], row.LegacyRaw)
		actor := &f.ResultContext.Actors[i+1]
		actor.RankRaw, actor.RawB, actor.Ship[29] = 1, 127, 127
		copy(actor.StoredRaw[:], row.LegacyRaw)
	}
	out := nProposal(t, p, f)
	if out.Commission || !out.Result.CrewPro || out.Result.ReplacementCount != 0 {
		t.Fatal("healthy existing rows must skip REPLACE")
	}
	// Actual selected actor1 is stored SourceIndex4; actor4 owns source1.
	f.ResultContext.Actors[1].SourceIndex, f.ResultContext.Actors[4].SourceIndex = 4, 1
	f.ResultContext.Actors[1].Name, f.ResultContext.Actors[4].Name = p.SquadronPilots[3].Name, p.SquadronPilots[0].Name
	copy(f.ResultContext.Actors[1].StoredRaw[:], p.SquadronPilots[3].LegacyRaw)
	copy(f.ResultContext.Actors[4].StoredRaw[:], p.SquadronPilots[0].LegacyRaw)
	f.ResultContext.Actors[1].Flags = 0x10
	out = nProposal(t, p, f)
	if out.Result.ReplacementCount != 1 || out.Result.ReplacementInitial {
		t.Fatal("loss replacement count/paragraph")
	}
	if out.Result.Replacements[3].Name == p.SquadronPilots[3].Name || out.Result.Replacements[0].Name != p.SquadronPilots[0].Name {
		t.Fatal("replacement used actor ordinal instead of source row")
	}
	q, err := ResolveSortieProposal(out, true)
	if err != nil || q.SquadronPilots[3].Name != out.Result.Replacements[3].Name {
		t.Fatal("lost source row copyback", err)
	}
	f.SourceFlags, f.PlayerTerminal = 0x10, true
	f.Reason = "terminal"
	out = nProposal(t, p, f)
	if out.Result.ReplacementCount != 0 || out.Result.CrewPro || len(out.Result.Replacements) != 0 {
		t.Fatal("player loss entered replacement initializer")
	}
	p, f = commissionFixture(t, 5)
	p.Rank, p.Statistics.CareerScore = RankColonel, 28000
	out = nProposal(t, p, f)
	if !out.Retirement || out.Result.ReplacementCount != 0 || len(out.Result.Replacements) != 4 {
		t.Fatal("retirement must retain initializer writes but suppress REPLACE return", out.Retirement)
	}
}

func TestCrewRecommendationPrecedesReplacementInitializer(t *testing.T) {
	p, f := commissionFixture(t, 5)
	p.Rank, p.Statistics.CareerScore = RankCaptain, 0
	out := nProposal(t, p, f)
	if !out.Result.CrewPro || len(out.Result.Recommendations) == 0 || out.Result.Recommendations[0].Type != 0 || out.Result.Recommendations[0].Actor != 1 {
		t.Fatal("supplied WOC recommendation predecessor absent")
	}
	// CREWPRO promotes source1 before2cde tests rank0. Three WOCs, not four,
	// remain eligible for replacement. This differs from P.2's precomputed rows.
	if out.Result.ReplacementCount != 3 || out.Result.Replacements[0].Name != p.SquadronPilots[0].Name || out.Result.Replacements[0].Rank != RankWO1 {
		t.Fatal("replacement preceded default source crew promotion")
	}
	choices := DefaultSortieCrewChoices(out)
	choices.Recipients[0] = 3
	q, err := ResolveSortieProposalWithChoices(out, true, choices)
	if err != nil {
		t.Fatal(err)
	}
	if q.SquadronPilots[2].Name != p.SquadronPilots[2].Name || q.SquadronPilots[2].Rank != RankWO1 || q.SquadronPilots[0].Name == p.SquadronPilots[0].Name {
		t.Fatal("replacement did not follow actual source recipient")
	}
	r, err := resolveSortieResultForChoices(out, true, choices)
	if err != nil || r.ReplacementCount != 3 {
		t.Fatal("custom frozen continuation", err)
	}
	choices.Result = r
	if _, err = ResolveSortieProposalWithChoices(out, true, choices); err != nil {
		t.Fatal(err)
	}
	wrong := *r
	wrong.ReplacementCount++
	choices.Result = &wrong
	if _, err = ResolveSortieProposalWithChoices(out, true, choices); err == nil {
		t.Fatal("fabricated post-choice continuation accepted")
	}
	if err = ValidateSortieProposal(out); err != nil {
		t.Fatal("choice reconstruction mutated frozen proposal", err)
	}
	// Original v3 decisions remain pre-choice, including their admitted defect;
	// loading an old journal must never regenerate a different replacement list.
	old, err := proposeSortieVersion(p, f, nResultTables(), 3)
	if err != nil {
		t.Fatal(err)
	}
	if old.Result.Replacements[0].Name == p.SquadronPilots[0].Name {
		t.Fatal("historical initializer was retroactively reordered")
	}
}

func TestRetainedHistoricalEarnedSingleStrict(t *testing.T) {
	// Exact hash-bound Step47 copied human journal. No writes to retained data.
	path := filepath.Join(testfixture.Path("commission-sortie.json"))
	b, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		t.Skip("optional retained human journal unavailable; release fixture manifest must supply it")
	}
	if err != nil {
		t.Fatal(err)
	}
	if fmt.Sprintf("%x", sha256.Sum256(b)) != "9d374136c6f29a2a63701a261278bcaf5e4cd87670dce7a18c2cb1be31e3a75d" {
		t.Fatal("retained human journal identity differs")
	}
	var doc sortieDocument
	if err = json.Unmarshal(b, &doc); err != nil {
		t.Fatal(err)
	}
	if err = validateSortieDocument(&doc); err != nil {
		t.Fatal(err)
	}
	if doc.Proposal.Version != 3 || doc.Proposal.Score.Score != 1039 || !doc.Proposal.Commission {
		t.Fatal("wrong retained journal", doc.Proposal.Version, doc.Proposal.Score)
	}
}

func TestPendingRenameFinalAckAndRestart(t *testing.T) {
	s := saveTestStore(t)
	state := commitTestPilot(t, s, "New Commander")
	p, facts := commissionFixture(t, 1)
	p.ID, p.Name = state.Slots[0].ID, state.Slots[0].Name
	state.Slots[0] = p
	if err := s.Commit(state); err != nil {
		t.Fatal(err)
	}
	start, err := s.BeginSortie(pendingTestStart())
	if err != nil {
		t.Fatal(err)
	}
	facts.SortieID = start.ID
	proposal, err := ProposeSortie(start.PilotBefore, facts, nResultTables())
	if err != nil {
		t.Fatal(err)
	}
	if err = s.StageSortieResult(start.ID, proposal); err != nil {
		t.Fatal(err)
	}
	before := mustReadSaveBytes(t, filepath.Join(s.Directory, pilotFilename(p.ID)))
	if err = s.SetSortieCommission(start.ID, true); err != nil {
		t.Fatal(err)
	}
	choices := DefaultSortieCrewChoices(proposal)
	choices.Names = []string{"ALPHA", "BRAVO", "CHARLIE", "DELTA"}
	if err = s.SetSortieCrewChoices(start.ID, *choices); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, mustReadSaveBytes(t, filepath.Join(s.Directory, pilotFilename(p.ID)))) {
		t.Fatal("pending rename changed canonical pilot")
	}
	pending, err := s.PendingSortie()
	if err != nil || pending.CrewChoices == nil || !reflect.DeepEqual(choices.Names, pending.CrewChoices.Names) || len(pending.CrewChoices.Recipients) != len(choices.Recipients) {
		t.Fatal("pending choices readback", err)
	}
	if err = s.SetSortieCommission(start.ID, false); err == nil {
		t.Fatal("replacement choices reused on refused single branch")
	}
	if err = s.CommitSortie(start.ID); err != nil {
		t.Fatal(err)
	}
	for i, row := range s.State().SelectedPilot().SquadronPilots {
		if row.Name != choices.Names[i] {
			t.Fatal("final acknowledgement lost row name", i)
		}
	}
	after := mustReadSaveBytes(t, filepath.Join(s.Directory, pilotFilename(p.ID)))
	if err = s.CommitSortie(start.ID); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(after, mustReadSaveBytes(t, filepath.Join(s.Directory, pilotFilename(p.ID)))) {
		t.Fatal("repeated commit changed result")
	}
	reopened, err := OpenSaveStore(s.Directory, "")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(reopened.State().SelectedPilot(), s.State().SelectedPilot()) {
		t.Fatal("restart changed accepted assignment")
	}
	backup := mustReadSaveBytes(t, filepath.Join(s.Directory, filepath.FromSlash(backupFilename(p.ID))))
	phoenix, err := decodePilot(backup, p.ID)
	if err != nil || !reflect.DeepEqual(phoenix, start.PilotBefore) {
		t.Fatal("rename replaced pre-result Phoenix", err)
	}
}
