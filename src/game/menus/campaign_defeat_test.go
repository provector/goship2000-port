package menus

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	gamestate "gunship2000-assets/game/state"
)

// These are explicitly supplied near-terminal histories, not twenty flown
// losses. The last failure is made by the real contact/rotor/result path.
func TestCampaignNearDefeatFinalMechanicsAndRestart(t *testing.T) {
	c := developmentScenarioController(t, "campaign-defeat")
	for _, draft := range []*gamestate.CampaignState{c.Mission.CampaignDraft, c.Mission.ResultLaunch.CampaignDraft} {
		draft.TargetCounter, draft.ResultCounters = 20, [2]int{0, 19}
	}
	if gamestate.CampaignTerminal(c.Mission.ResultLaunch.CampaignDraft, gamestate.MissionCampaign) != 0 {
		t.Fatal("last nonterminal boundary")
	}
	c.takeAudioCues()
	before := c.Store.State()
	scenarioSafeReturn(t, c)
	proposal := c.Debrief.Pending.Proposal
	result := proposal.Result.Campaign
	if proposal.Success || result.Campaign.ResultCounters != [2]int{0, 20} || result.Verdict != -1 || !result.Concluded || proposal.Result.MusicCue != 0x1f || c.Debrief.Page != "campaign" || c.Debrief.Scene != "LOSE.SCN" {
		t.Fatal("actual failure did not cross threshold20 to LOSE/1f", proposal.Score, result, c.Debrief)
	}
	countCue := func(cues []uint16, cue uint16) int {
		n := 0
		for _, v := range cues {
			if v == cue {
				n++
			}
		}
		return n
	}
	if got := countCue(c.takeAudioCues(), 0x1f); got != 1 {
		t.Fatal("debrief must issue defeat cue exactly once", got)
	}
	for i := 0; i < 3; i++ {
		c.Frame(false)
	}
	if countCue(c.takeAudioCues(), 0x1f) != 0 {
		t.Fatal("redraw repeated defeat cue")
	}
	canonical := c.Store.State()
	canonical.Revision = before.Revision
	if !reflect.DeepEqual(before, canonical) {
		t.Fatal("unacknowledged ending changed career")
	}
	id := proposal.Facts.SortieID
	pages := acknowledgeLibrary(t, c)
	committed := c.Store.State()
	pilot := committed.SelectedPilot()
	// Source adds100 for any concluded Campaign, in addition to the sortie1.
	if pilot.Campaign != nil || pilot.MissionType != gamestate.MissionFlight || pilot.LegacyRaw[0x4f] != 0 || pilot.Statistics.MissionCampaignCounter != before.SelectedPilot().Statistics.MissionCampaignCounter+101 {
		t.Fatal("defeat finalack did not publish exact terminal ownership", pilot)
	}
	if err := c.Store.CommitSortie(id); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(committed, c.Store.State()) {
		t.Fatal("repeat finalack changed canonical state")
	}
	if err := c.exitFilmLibrary(); err != nil {
		t.Fatal(err)
	}
	if c.Screen != ScreenOffice || c.Mission != nil {
		t.Fatal("defeat library successor was not office", c.Screen)
	}
	if err := c.exitFilmLibrary(); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(committed, c.Store.State()) {
		t.Fatal("obsolete EXIT reconsumed result")
	}
	reopened, err := gamestate.OpenSaveStore(c.Store.Directory, "")
	if err != nil || !reflect.DeepEqual(committed, reopened.State()) {
		t.Fatal("defeat restart changed result", err)
	}
	phoenix, err := reopened.RecoveryCandidate()
	if err != nil || phoenix == nil || phoenix.State.SelectedPilot().Statistics.MissionCampaignCounter != before.SelectedPilot().Statistics.MissionCampaignCounter {
		t.Fatal("independent pre-result Phoenix", err)
	}
	campaignEvidence(t, "near-defeat", map[string]any{"qualification": "Supplied threshold20/failures19; actual safe contact/rotor return, result, LOSE, cue-selection and finalack. No device or listening claim.", "proposal": proposal, "pages": pages, "before": before, "committed": committed, "after_restart": reopened.State()})
}

func TestCampaignDeclineMarginAndTerminalControls(t *testing.T) {
	c := developmentScenarioController(t, "campaign-defeat")
	draft := c.Mission.CampaignDraft
	draft.TargetCounter, draft.ResultCounters = 20, [2]int{0, 19}
	pilot := c.Store.State().SelectedPilot()
	if err := campaignDecline(pilot, c.Mission); err != nil {
		t.Fatal(err)
	}
	if pilot.Campaign.TargetCounter != 21 || pilot.Campaign.ResultCounters != [2]int{0, 20} || gamestate.CampaignTerminal(pilot.Campaign, gamestate.MissionCampaign) != 0 {
		t.Fatal("decline must preserve the threshold-minus-failure margin", pilot.Campaign)
	}
	for _, tc := range []struct {
		successes, failures int
		mode                gamestate.MissionType
		verdict             int8
	}{{0, 19, gamestate.MissionCampaign, 0}, {0, 20, gamestate.MissionCampaign, -1}, {20, 20, gamestate.MissionCampaign, -1}, {20, 19, gamestate.MissionCampaign, 1}, {20, 20, gamestate.MissionFlight, 0}} {
		copy, err := gamestate.CloneCampaign(draft)
		if err != nil {
			t.Fatal(err)
		}
		copy.ResultCounters = [2]int{tc.successes, tc.failures}
		if got := gamestate.CampaignTerminal(copy, tc.mode); got != tc.verdict {
			t.Fatal("independent equality/tie/current-mode predicate", tc, got)
		}
	}
	// The real ending above covers counter and publication. This separate
	// consumer control checks the source loss18 conclusion bypass precisely.
	scenarioSafeReturn(t, c)
	input := *c.Debrief.Pending.Proposal.Facts.ResultContext.Campaign
	input.NormalizedFlags = 0x18
	input.LaunchDraft = draft
	input.ScoredSuccess = false
	result, err := gamestate.PrepareCampaignResult(input)
	if err != nil || result.Verdict != -1 || result.Concluded || result.Campaign.ResultCounters != [2]int{0, 20} {
		t.Fatal("loss18 must bypass conclusion after counter update", err, result)
	}
}

func campaignEvidence(t *testing.T, name string, value any) {
	t.Helper()
	if dir := os.Getenv("GS2000_P3_OUTPUT"); dir != "" {
		if err := os.MkdirAll(dir, 0700); err != nil {
			t.Fatal(err)
		}
		data, err := json.MarshalIndent(value, "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, name+".json"), data, 0600); err != nil {
			t.Fatal(err)
		}
	}
}
