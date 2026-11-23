package state

import (
	"encoding/json"
	assetdata "gunship2000-assets/game/assets"
	"testing"
)

func TestAuthoredSourceScoreAndHistoricalInterpretation(t *testing.T) {
	old := historicalProposal(t)
	p := cloneSortiePilot(old.PilotBefore)
	p.MissionType = MissionSimulated
	f := old.Facts
	f.MissionType = MissionSimulated
	f.LaunchSwitches = f.LaunchSwitches&0x1fff | 0x4000
	q, e := ProposeSortie(p, f, old.Tables)
	if e != nil {
		t.Fatal(e)
	}
	// Same actual GS score operands: primary100 + player2*25 + 4 returned25.
	if q.Version != SortieProposalVersion || q.Score.Score != 250 || q.Score.AircraftContribution != 150 || !q.Score.CareerApplied {
		t.Fatalf("source Simulation contribution: %+v", q.Score)
	}
	for _, v := range []int{1, 2} {
		if _, e = proposeSortieVersion(p, f, old.Tables, v); e == nil {
			t.Fatal("new type admitted through historical version", v)
		}
	}
	if e = ValidateSortieProposal(q); e != nil {
		t.Fatal(e)
	}
	f.ActorCount = 1
	if _, e = ProposeSortie(p, f, old.Tables); e == nil {
		t.Fatal("raw2 fabricated a single-participant crew")
	}
}
func TestHistoricalV2JournalCommitRestart(t *testing.T) {
	old := historicalProposal(t)
	s := saveTestStore(t)
	next := s.State()
	next.Slots[0] = cloneSortiePilot(old.PilotBefore)
	next.SelectedPilotID = old.PilotBefore.ID
	if e := s.Commit(next); e != nil {
		t.Fatal(e)
	}
	start, e := s.BeginSortie(pendingTestStart())
	if e != nil {
		t.Fatal(e)
	}
	doc, e := s.currentSortie()
	if e != nil {
		t.Fatal(e)
	}
	doc.SchemaVersion = 4
	doc.Start.ProposalVersion = 2
	if e = s.storeSortieDocument(doc); e != nil {
		t.Fatal(e)
	}
	f := old.Facts
	f.SortieID = start.ID
	ctx := *f.ResultContext
	ctx.Seed = ResultSeed(start.ID)
	f.ResultContext = &ctx
	proposal, e := proposeSortieVersion(doc.Start.PilotBefore, f, old.Tables, 2)
	if e != nil {
		t.Fatal(e)
	}
	if proposal.Score.Score != 250 {
		t.Fatal("old source-correct v2 score changed")
	}
	if e = s.StageSortieResult(start.ID, proposal); e != nil {
		t.Fatal(e)
	}
	if e = s.CommitSortie(start.ID); e != nil {
		t.Fatal(e)
	}
	reopened, e := OpenSaveStore(s.Directory, "")
	if e != nil {
		t.Fatal(e)
	}
	if reopened.State().SelectedPilot().Statistics.LastMissionScore != 250 {
		t.Fatal("historical result changed")
	}
	before, _ := json.Marshal(reopened.State())
	if e = reopened.CommitSortie(start.ID); e != nil {
		t.Fatal(e)
	}
	after, _ := json.Marshal(reopened.State())
	if string(before) != string(after) {
		t.Fatal("duplicate historical commit")
	}
	bad := *doc
	bad.SchemaVersion = 4
	bad.Start.ProposalVersion = 3
	if e = validateSortieDocument(&bad); e == nil {
		t.Fatal("historical schema accepted later domain")
	}
	_ = assetdata.IslandIce469085
}
