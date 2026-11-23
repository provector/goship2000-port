package state

import "testing"

func TestLossCommitPhoenixAndConsumedIdentity(t *testing.T) {
	s := saveTestStore(t)
	pendingTestPilot(t, s)
	start, err := s.BeginSortie(pendingTestStart())
	if err != nil {
		t.Fatal(err)
	}
	f := contextFacts()
	f.SortieID = start.ID
	f.SourceFlags = 0x10
	f.Reason = "damage-fuel"
	f.PlayerTerminal = true
	proposal, err := ProposeSortie(s.State().SelectedPilot(), f, pendingTestTables())
	if err != nil {
		t.Fatal(err)
	}
	if err = s.StageSortieResult(start.ID, proposal); err != nil {
		t.Fatal(err)
	}
	if s.State().SelectedPilot().Status != StatusActive {
		t.Fatal("unacknowledged loss changed durable status")
	}
	if err = s.CommitSortie(start.ID); err != nil {
		t.Fatal(err)
	}
	if p := s.State().SelectedPilot(); p == nil || p.Status != StatusKIA || p.Statistics.AwardCounts[6] != 1 {
		t.Fatal("KIA/PH commit missing", p)
	}
	reopened, err := OpenSaveStore(s.Directory, "")
	if err != nil {
		t.Fatal(err)
	}
	candidate, err := reopened.RecoveryCandidate()
	if err != nil || candidate == nil {
		t.Fatal("loss did not retain pre-result Phoenix", err)
	}
	if err = reopened.Restore(candidate.ID); err != nil {
		t.Fatal(err)
	}
	p := reopened.State().SelectedPilot()
	if p.Status != StatusActive || p.Statistics.AwardCounts[6] != 0 || p.Statistics.PhoenixCounter != -1 {
		t.Fatal("Phoenix failed to restore only pre-result career", p)
	}
	if err = reopened.CommitSortie(start.ID); err != nil {
		t.Fatal(err)
	}
	if p = reopened.State().SelectedPilot(); p.Status != StatusActive || p.Statistics.AwardCounts[6] != 0 {
		t.Fatal("consumed loss awarded again after recovery")
	}
}

func TestCrewRecipientTransactionPersistsOnce(t *testing.T) {
	s := saveTestStore(t)
	pendingTestPilot(t, s)
	next := s.State()
	old := next.SelectedPilot()
	p, f := flightFixture(t)
	p.ID = old.ID
	p.Name = old.Name
	for i, slot := range next.Slots {
		if slot != nil && slot.ID == old.ID {
			next.Slots[i] = p
		}
	}
	if err := s.Commit(next); err != nil {
		t.Fatal(err)
	}
	start, err := s.BeginSortie(pendingTestStart())
	if err != nil {
		t.Fatal(err)
	}
	f.SortieID = start.ID
	f.SourceFlags = 0x81
	if err = FinalizeSortieFacts(&f); err != nil {
		t.Fatal(err)
	}
	proposal, err := ProposeSortie(s.State().SelectedPilot(), f, pendingTestTables())
	if err != nil {
		t.Fatal(err)
	}
	if err = s.StageSortieResult(start.ID, proposal); err != nil {
		t.Fatal(err)
	}
	choices := DefaultSortieCrewChoices(proposal)
	choices.Recipients[0] = 3
	if err = s.SetSortieCrewChoices(start.ID, *choices); err != nil {
		t.Fatal(err)
	}
	if s.State().SelectedPilot().SquadronPilots[2].Rank.Raw() != 1 {
		t.Fatal("pending choice committed before final acknowledgement")
	}
	if err = s.CommitSortie(start.ID); err != nil {
		t.Fatal(err)
	}
	if err = s.CommitSortie(start.ID); err != nil {
		t.Fatal(err)
	}
	again, err := OpenSaveStore(s.Directory, "")
	if err != nil {
		t.Fatal(err)
	}
	q := again.State().SelectedPilot()
	if q.SquadronPilots[2].Rank.Raw() != 2 || q.SquadronPilots[0].Rank.Raw() != 1 || q.Statistics.MissionCampaignCounter != 1 {
		t.Fatal("recipient/counter copyback missing or duplicated", q)
	}
}
