package menus

import (
	gamestate "gunship2000-assets/game/state"
	"reflect"
	"strings"
	"testing"
)

// This is a supplied base approach using real preparation/contact/rotor,
// normalization, debrief and durable final acknowledgement. No finished result,
// objective flag or follower performance is installed by the test.
func TestOngoingCampaignCommitStagesNextBriefingOnce(t *testing.T) {
	c := developmentScenarioController(t, "campaign-return")
	first := campaignNaturalBaseEnd(t, c)
	if first.Success {
		t.Fatal("no-objective return is not the successful-sortie case")
	}
	loopAcknowledge(t, c)
	if c.Screen != ScreenInterstitial || c.Mission == nil || c.Debrief != nil || c.Rendering != nil {
		t.Fatalf("ongoing Campaign did not stage source successor: %s", c.Screen)
	}
	next := c.Mission
	committed := c.Store.State()
	pilot := committed.SelectedPilot()
	if pilot.Campaign == nil || pilot.Campaign.ResultCounters != [2]int{0, 1} || next.Data.Inputs.MissionType != gamestate.MissionCampaign {
		t.Fatal("next mission/result identity mismatch")
	}
	if next.CampaignDraft == nil || next.CampaignDraft == pilot.Campaign || next.CampaignDraft.SelectedScenario != next.Data.SourceWorld.Suffix || len(next.Data.Objects) != 80 {
		t.Fatal("next preparation is not an independently owned real draft")
	}
	for i := 0; i < 3; i++ {
		if err := c.commitDebriefResult(); err != nil {
			t.Fatal(err)
		}
		if err := c.Handle(Actions{Enter: true}); err != nil {
			t.Fatal(err)
		}
		c.Frame(false)
	}
	if c.Mission != next || !reflect.DeepEqual(committed, c.Store.State()) {
		t.Fatal("old callback/input/redraw regenerated mission or awarded again")
	}
	for i := 0; i < 100 && c.Screen == ScreenInterstitial; i++ {
		if err := c.Tick(); err != nil {
			t.Fatal(err)
		}
	}
	if c.Screen != ScreenBriefing {
		t.Fatal("genuine interstitial did not reach briefing", c.Screen)
	}
	if err := c.Handle(Actions{Quit: true}); err != nil {
		t.Fatal(err)
	}
	if err := c.Handle(Actions{ConfirmQuit: true}); err != nil {
		t.Fatal(err)
	}
	reopened, err := gamestate.OpenSaveStore(c.Store.Directory, "")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(committed, reopened.State()) {
		t.Fatal("quitting next uncommitted briefing discarded completed result")
	}
	startup, err := NewFrontEnd(c.Office.Assets, reopened)
	if err != nil || startup.Screen != ScreenOffice || startup.Mission != nil {
		t.Fatal("restart must open office, not force unfinished briefing", err)
	}
}

func TestCampaignContinuesForTwoActuallyTerminatedSorties(t *testing.T) {
	c := developmentScenarioController(t, "campaign-return")
	initialCounter := c.Store.State().SelectedPilot().Statistics.MissionCampaignCounter
	for sortie := 1; sortie <= 2; sortie++ {
		p := campaignNaturalBaseEnd(t, c)
		if p.Result.Campaign.Campaign.ResultCounters != [2]int{0, sortie} {
			t.Fatal("wrong source failure counter", p.Result.Campaign.Campaign.ResultCounters)
		}
		loopAcknowledge(t, c)
		if c.Screen != ScreenInterstitial || c.Mission == nil {
			t.Fatal("no automatic next preparation", c.Screen)
		}
		if sortie == 2 {
			break
		}
		for i := 0; i < 100 && c.Screen == ScreenInterstitial; i++ {
			if err := c.Tick(); err != nil {
				t.Fatal(err)
			}
		}
		startRendering(t, c)
		if err := c.developmentScenarioPose(c.Rendering.ResultTargets.Records[0], true, 2, 1, 0); err != nil {
			t.Fatal(err)
		}
	}
	final := c.Store.State()
	if final.SelectedPilot().Statistics.MissionCampaignCounter != initialCounter+2 {
		t.Fatal("two actual acknowledgements did not increment twice")
	}
}

func TestFailedNextCampaignPreparationKeepsCompletedResult(t *testing.T) {
	c := developmentScenarioController(t, "campaign-return")
	initialCounter := c.Store.State().SelectedPilot().Statistics.MissionCampaignCounter
	// Labelled invalid retained-history consumer test: provenance is opaque until
	// a real next-generation FMHD lookup. Do not replace it with invented data.
	c.Mission.ResultLaunch.CampaignDraft.Retained.Provenance[0] = 255
	campaignNaturalBaseEnd(t, c)
	seed := int64(18)
	c.MissionSeed = &seed
	var failure error
	for i := 0; i < 25 && c.Screen == ScreenDebrief; i++ {
		if c.Debrief.Page == "crew" {
			c.Debrief.CrewSelection = len(c.Debrief.Pending.Proposal.Result.Recommendations)
		}
		c.Debrief.Tick = 400
		failure = c.Handle(Actions{Enter: true})
		if failure != nil {
			break
		}
		if err := c.Handle(Actions{}); err != nil {
			t.Fatal(err)
		}
	}
	// Next preparation now belongs to Library EXIT, after the result is durable.
	if failure == nil {
		if c.Screen != ScreenFilms {
			t.Fatal("Step O post-ack library missing", c.Screen)
		}
		failure = c.activateFilmControl(18)
	}
	if failure == nil || !strings.Contains(failure.Error(), "completed result saved; next Campaign preparation") || c.Screen != ScreenOffice || c.Mission != nil || c.Debrief != nil {
		t.Fatalf("failed next preparation not safely reported: %s %v", c.Screen, failure)
	}
	committed := c.Store.State()
	if committed.SelectedPilot().Statistics.MissionCampaignCounter != initialCounter+1 || committed.SelectedPilot().Campaign.ResultCounters != [2]int{0, 1} {
		t.Fatal("next preparation failure undid completed facts")
	}
	reopened, err := gamestate.OpenSaveStore(c.Store.Directory, "")
	if err != nil || !reflect.DeepEqual(committed, reopened.State()) {
		t.Fatal("failed next preparation corrupted durable result", err)
	}
	if rec, err := reopened.RecoveryCandidate(); err != nil || rec == nil {
		t.Fatal("failed next preparation consumed Phoenix", err)
	}
}
