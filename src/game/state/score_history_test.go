package state

import (
	"encoding/json"
	assetdata "gunship2000-assets/game/assets"
	"gunship2000-assets/internal/testfixture"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func historicalProposal(t *testing.T) *SortieProposal {
	t.Helper()
	raw, err := os.ReadFile(testfixture.Path("flight-reference/n4-flight-v1-350.json"))
	if err != nil {
		t.Fatal(err)
	}
	// Captured before the N.5 calculator change; provenance is in stepN5 evidence.
	var p SortieProposal
	if err = json.Unmarshal(raw, &p); err != nil {
		t.Fatal(err)
	}
	if p.Version != 1 || p.Score.Score != 350 || p.Score.AircraftContribution != 250 {
		t.Fatal("changed historical witness")
	}
	if err = ValidateSortieProposal(&p); err != nil {
		t.Fatal(err)
	}
	return &p
}

func TestSourceFollowerContributionAndStrictHistory(t *testing.T) {
	old := historicalProposal(t)
	p, err := ProposeSortie(old.PilotBefore, old.Facts, old.Tables)
	if err != nil {
		t.Fatal(err)
	}
	// Source A1e8ac doubles player25; A1e8f1 adds four25 once; primary100.
	// Step P adds proposal3 raw2 admission; source-correct v2 arithmetic stays unchanged.
	if p.Version != SortieProposalVersion || p.Score.AircraftContribution != 150 || p.Score.Score != 250 {
		t.Fatalf("source100+50+4*25: %+v", p.Score)
	}
	if p.Score.NormalizedFlags != old.Score.NormalizedFlags || p.Success != old.Success {
		t.Fatal("score fix changed success predicate")
	}
	if err = ValidateSortieProposal(p); err != nil {
		t.Fatal(err)
	}
	for _, version := range []int{1, 2} {
		bad := *p
		if version == 1 {
			bad = *old
		}
		bad.Score.Score++
		if err = ValidateSortieProposal(&bad); err == nil {
			t.Fatal("accepted inconsistent versioned score")
		}
	}
	// Equal source-sign/return rules at all four quality settings; only the
	// expected signed sum changes, not the prescribed quality operation.
	for quality, aircraft := range []int16{0, 75, 150, 150} {
		f := old.Facts
		f.LaunchSwitches = f.LaunchSwitches&^3 | uint16(quality)
		p, err := ProposeSortie(old.PilotBefore, f, old.Tables)
		if err != nil || p.Score.AircraftContribution != aircraft || p.Score.Score != 100+int32(aircraft) {
			t.Fatalf("quality%d %+v %v", quality, p, err)
		}
	}
}

func historicalEntry(t *testing.T) (*SaveStore, *SortieStart, *SortieProposal) {
	t.Helper()
	old := historicalProposal(t)
	s := saveTestStore(t)
	next := s.State()
	next.Slots[0] = cloneSortiePilot(old.PilotBefore)
	next.SelectedPilotID = old.PilotBefore.ID
	if err := s.Commit(next); err != nil {
		t.Fatal(err)
	}
	start, err := s.BeginSortie(pendingTestStart())
	if err != nil {
		t.Fatal(err)
	}
	// Model an on-disk N.4 launch, whose contract version field did not exist.
	// The normal new BeginSortie always pins the current explicit interpretation and cannot stage legacy scoring.
	doc, err := s.currentSortie()
	if err != nil {
		t.Fatal(err)
	}
	doc.Start.ProposalVersion = 0
	doc.SchemaVersion = 3
	if err = s.storeSortieDocument(doc); err != nil {
		t.Fatal(err)
	}
	start = &doc.Start
	// Rebind only the transaction identity in a new disposable store. Retained
	// original fixture bytes above remain the historical validation oracle.
	f := old.Facts
	f.SortieID = start.ID
	context := *f.ResultContext
	context.Seed = ResultSeed(start.ID)
	f.ResultContext = &context
	p, err := proposeSortieVersion(start.PilotBefore, f, old.Tables, 1)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.StageSortieResult(start.ID, p); err != nil {
		t.Fatal(err)
	}
	return s, start, p
}

func TestHistoricalCommitRestartPhoenixConsumeOnce(t *testing.T) {
	s, start, p := historicalEntry(t)
	if err := s.CommitSortie(start.ID); err != nil {
		t.Fatal(err)
	}
	want, err := ResolveSortieProposal(p, false)
	if err != nil {
		t.Fatal(err)
	}
	q, err := OpenSaveStore(s.Directory, "")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(q.State().SelectedPilot(), want) || want.Statistics.LastMissionScore != 350 {
		t.Fatal("historical career was rescored")
	}
	candidate, err := q.RecoveryCandidate()
	if err != nil || candidate == nil {
		t.Fatal("lost historical Phoenix", err)
	}
	if err = q.Restore(candidate.ID); err != nil {
		t.Fatal(err)
	}
	recovered := q.State().SelectedPilot()
	if err = q.CommitSortie(start.ID); err != nil {
		t.Fatal(err)
	}
	again, err := OpenSaveStore(s.Directory, "")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(recovered, again.State().SelectedPilot()) {
		t.Fatal("historical result re-awarded after Phoenix")
	}
}

func TestHistoricalDurableJournalEveryBoundary(t *testing.T) {
	for cut := 0; cut <= 4; cut++ {
		t.Run(string(rune('0'+cut)), func(t *testing.T) {
			s, start, _ := historicalEntry(t)
			journal := pendingCommitJournal(t, s, start.ID)
			raw, _ := assetdata.JsonBytes(journal)
			if err := os.WriteFile(filepath.Join(s.Directory, saveJournal), raw, 0600); err != nil {
				t.Fatal(err)
			}
			for _, op := range journal.Operations[:cut] {
				if err := os.WriteFile(filepath.Join(s.Directory, filepath.FromSlash(op.Path)), op.After, 0600); err != nil {
					t.Fatal(err)
				}
			}
			q, err := OpenSaveStore(s.Directory, "")
			if err != nil {
				t.Fatal(err)
			}
			if q.State().SelectedPilot().Statistics.LastMissionScore != 350 {
				t.Fatal("journal reran corrected scoring")
			}
			if err = q.CommitSortie(start.ID); err != nil {
				t.Fatal(err)
			}
			if q.State().SelectedPilot().Statistics.MissionCampaignCounter != 1 {
				t.Fatal("duplicate historical commit")
			}
		})
	}
}

func TestNewLaunchCannotSelectHistoricalScoring(t *testing.T) {
	s := saveTestStore(t)
	pendingTestPilot(t, s)
	start, err := s.BeginSortie(pendingTestStart())
	if err != nil {
		t.Fatal(err)
	}
	f := pendingTestProposal(t, s, start).Facts
	old, err := proposeSortieVersion(start.PilotBefore, f, pendingTestTables(), 1)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.StageSortieResult(start.ID, old); err == nil {
		t.Fatal("new launch admitted historical scoring")
	}
}

func TestHistoricalUncommittedMetadataAbandons(t *testing.T) {
	s, _, p := historicalEntry(t)
	q, err := OpenSaveStore(s.Directory, "")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(q.State().SelectedPilot(), p.PilotBefore) {
		t.Fatal("abandoned old proposal applied progression")
	}
	if pending, err := q.PendingSortie(); err != nil || pending != nil {
		t.Fatal("old uncommitted UI forced resumption", err)
	}
}
