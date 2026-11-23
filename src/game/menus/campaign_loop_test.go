package menus

import (
	"encoding/json"
	gamestate "gunship2000-assets/game/state"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

// Supplied base approaches call the normal contact/rotor/end controllers. No
// objective, score, completion flag or result record is installed by this test.
func campaignNaturalBaseEnd(t *testing.T, c *FrontEnd) *gamestate.SortieProposal {
	t.Helper()
	if err := c.Handle(Actions{Cancel: true}); err != nil {
		t.Fatal(err)
	}
	if err := c.AdvanceRendering(.12); err != nil {
		t.Fatal(err)
	}
	if err := c.Handle(Actions{}); err != nil {
		t.Fatal(err)
	}
	if err := c.Handle(Actions{FlightCommands: []int{2}}); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 60 && c.Screen == ScreenScene; i++ {
		if err := c.AdvanceRendering(.12); err != nil {
			t.Fatal(err)
		}
	}
	if c.Screen != ScreenQuit {
		t.Fatalf("actual base/rotor path did not request ending: %s %s", c.Screen, c.Message)
	}
	if err := c.Handle(Actions{ConfirmQuit: true}); err != nil {
		t.Fatal(err)
	}
	if c.Screen != ScreenDebrief {
		t.Fatalf("Campaign source result not admitted: %s %s", c.Screen, c.Message)
	}
	pending, err := c.Store.PendingSortie()
	if err != nil || pending == nil || pending.Proposal.Result == nil || pending.Proposal.Result.Campaign == nil {
		t.Fatalf("Campaign proposal missing: %v", err)
	}
	return pending.Proposal
}

func TestCampaignTwoCommittedSortiesAndRestart(t *testing.T) {
	c := developmentScenarioController(t, "campaign-return")
	initial := c.Store.State().SelectedPilot()
	for sortie := 1; sortie <= 2; sortie++ {
		before := c.Store.State()
		proposal := campaignNaturalBaseEnd(t, c)
		if proposal.Facts.MissionType != gamestate.MissionCampaign || proposal.Facts.ActorCount != 5 || len(proposal.Result.Crew) != 4 {
			t.Fatal("genuine Campaign/follower ownership changed")
		}
		if proposal.Result.Campaign.Concluded || proposal.Result.Campaign.Campaign.ResultCounters != [2]int{0, sortie} {
			t.Fatalf("no-credit Campaign counter path%d: %+v", sortie, proposal.Result.Campaign)
		}
		// Independent source no-shoot subtotal: neither objective, no final1c
		// kinds/credit; selector multiset may supply50 before coefficient.
		var subtotal int32
		if proposal.Facts.SelectorMultisetMatch {
			subtotal = 50
		}
		wantScore := subtotal * int32(proposal.Facts.Coefficient) / 10
		if proposal.Score.Score != wantScore || proposal.Score.RawScore != 0 || proposal.Score.NormalizedFlags&3 != 0 {
			t.Fatalf("no-shoot score inputs: %+v", proposal.Score)
		}
		pendingState := c.Store.State()
		pendingState.Revision = before.Revision
		if !reflect.DeepEqual(before, pendingState) {
			t.Fatal("unacknowledged Campaign result changed canonical pilot/crew/cache")
		}
		pages := loopAcknowledge(t, c)
		committed := c.Store.State()
		pilot := committed.SelectedPilot()
		if pilot.MissionType != gamestate.MissionCampaign || pilot.Campaign == nil || pilot.Campaign.ResultCounters != [2]int{0, sortie} || pilot.Statistics.MissionCampaignCounter != initial.Statistics.MissionCampaignCounter+int32(sortie) {
			t.Fatal("committed Campaign/packed mission counter not derived from real proposal")
		}
		if err := c.Store.CommitSortie(proposal.Facts.SortieID); err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(committed, c.Store.State()) {
			t.Fatal("duplicate Campaign acknowledgement applied twice")
		}
		reopened, err := gamestate.OpenSaveStore(c.Store.Directory, "")
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(committed, reopened.State()) {
			t.Fatal("Campaign restart changed committed pilot/crew/cache")
		}
		if recovery, err := reopened.RecoveryCandidate(); err != nil || recovery == nil {
			t.Fatal("Campaign Phoenix missing", err)
		}
		if out := os.Getenv("GS2000_N4_EVIDENCE"); out != "" {
			record := map[string]any{"evidence": "native supplied approach; genuine preparation and normal contact/end/debrief/commit; no original execution claim", "sortie": sortie, "proposal": proposal, "pages": pages, "committed": committed}
			b, _ := json.MarshalIndent(record, "", "  ")
			if err := os.MkdirAll(out, 0700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(out, []string{"campaign-consecutive1.json", "campaign-consecutive2.json"}[sortie-1]), b, 0600); err != nil {
				t.Fatal(err)
			}
		}
		if sortie == 2 {
			break
		}
		// Reopen the same canonical scratch pilot normally; do not recreate
		// its squadron, overwrite rank/history or rerun scenario creation.
		c.Store = reopened
		if err := c.RefreshOffice(); err != nil {
			t.Fatal(err)
		}
		// This is a bounded two-result persistence fixture, not acceptance of
		// every generated successor. From this exact committed history seed2
		// genuinely chooses active Air Support, whose controller remains guarded.
		// Source-generated seed3 chooses None; keep that declared selection rather
		// than deleting Support from the ordinary generated mission.
		seed := int64(3)
		c.MissionSeed = &seed
		readyMission(t, c)
		if c.Mission.Data.Support != nil || c.Mission.Data.World != "GULF5.WRL" || c.Mission.Data.Scenario != "PERS_02.SNR" || c.Mission.Data.ObjectHash != "556fb3c9002ead2904e3c48db39e2249e2b236ca89db416f4e749831f6776b54" {
			t.Fatal("declared None-Support successor fixture changed", c.Mission.Data.Support, c.Mission.Data.ObjectHash)
		}
		startRendering(t, c)
		if c.Mission.ResultLaunch == nil {
			t.Fatal(c.Mission.ResultUnavailable)
		}
		if err := c.developmentScenarioPose(c.Rendering.ResultTargets.Records[0], true, 2, 1, 0); err != nil {
			t.Fatal(err)
		}
	}
}

func TestCampaignControlledConclusionCommitsAndNextFlight(t *testing.T) {
	c := developmentScenarioController(t, "campaign-win")
	before := c.Store.State()
	proposal := campaignNaturalBaseEnd(t, c)
	if !proposal.Result.Campaign.Concluded || proposal.Result.Campaign.Verdict != 1 || proposal.Result.Campaign.SortieSuccess {
		t.Fatal("controlled initial counter history did not yield positive Campaign verdict with failed current sortie")
	}
	pages := loopAcknowledge(t, c)
	sawConclusion := false
	for _, page := range pages {
		sawConclusion = sawConclusion || page == "campaign"
	}
	if !sawConclusion {
		t.Fatal("WIN controller not reached")
	}
	final := c.Store.State()
	p := final.SelectedPilot()
	if p.Campaign != nil || p.MissionType != gamestate.MissionFlight || p.LegacyRaw[0x4f] != 0 || p.Statistics.MissionCampaignCounter != before.SelectedPilot().Statistics.MissionCampaignCounter+101 {
		t.Fatal("Campaign conclusion did not atomically publish source stage/type/packedcounter")
	}
	if err := c.Store.CommitSortie(proposal.Facts.SortieID); err != nil {
		t.Fatal(err)
	}
	reopened, err := gamestate.OpenSaveStore(c.Store.Directory, "")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(final, reopened.State()) {
		t.Fatal("conclusion restart/duplicate changed result")
	}
	c.Store = reopened
	if err = c.RefreshOffice(); err != nil {
		t.Fatal(err)
	}
	seed := int64(3)
	c.MissionSeed = &seed
	readyMission(t, c)
	if c.Mission.Data.Inputs.MissionType != gamestate.MissionFlight || c.Mission.CampaignDraft != nil {
		t.Fatal("next genuine Flight preparation retained completed Campaign draft")
	}
}
