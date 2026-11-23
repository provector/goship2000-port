package menus

import (
	"bytes"
	"gunship2000-assets/game/configuration"
	"gunship2000-assets/game/diagnostics"
	gamestate "gunship2000-assets/game/state"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestSummaryAcknowledgmentRuntimeOutputAndDuplicateIsolation(t *testing.T) {
	c := developmentScenarioController(t, "o-short-campaign")
	c.SummaryDirectory = t.TempDir()
	target := filepath.Join(c.SummaryDirectory, gamestate.MissionSummaryFilename)
	before := c.Store.State().SelectedPilot().Statistics.MissionCampaignCounter
	p := campaignNaturalBaseEnd(t, c)
	if _, e := os.Stat(target); !os.IsNotExist(e) {
		t.Fatal("unacknowledged result created summary", e)
	}
	if e := c.Handle(Actions{Quit: true}); e != nil || c.Screen != ScreenQuit {
		t.Fatal("debrief quit confirmation not reached", e)
	}
	if e := c.Handle(Actions{Cancel: true}); e != nil || c.Screen != ScreenDebrief {
		t.Fatal("cancel must return to pending debrief", e)
	}
	if _, e := os.Stat(target); !os.IsNotExist(e) || c.Store.State().SelectedPilot().Statistics.MissionCampaignCounter != before {
		t.Fatal("canceled acknowledgement changed report/career", e)
	}
	// The text must use the frozen result owner, never these unrelated labels.
	c.Office.State.PilotName, c.Debrief.Pilot.Name = "Mutable office name", "Mutable view name"
	ackToLibrary(t, c)
	b, e := os.ReadFile(target)
	if e != nil || !strings.Contains(string(b), p.PilotBefore.Name) || strings.Contains(string(b), "Mutable") || !strings.Contains(string(b), "DESTRUCTION LIST:") {
		t.Fatal("actual acknowledged result was not reported", string(b), e)
	}
	if _, e = os.Stat(filepath.Join(c.Store.Directory, gamestate.MissionSummaryFilename)); !os.IsNotExist(e) {
		t.Fatal("runtime report leaked into saves directory", e)
	}
	if c.Store.State().SelectedPilot().Statistics.MissionCampaignCounter != before+1 || len(c.Library.Entries) != 1 {
		t.Fatal("summary changed career or complete film")
	}
	reader, e := c.Library.Store.Load(c.Library.Entries[0].ID)
	if e != nil {
		t.Fatal(e)
	}
	play, e := NewFilmPlayback(c.Office.Assets, reader, configuration.Defaults())
	if e != nil {
		t.Fatal(e)
	}
	for !play.Done {
		if e = play.Step(); e != nil {
			t.Fatal(e)
		}
	}
	saved := c.Store.State()
	// An obsolete callback must not replace a newer successfully retained text.
	newer := []byte("newer completed summary marker\n")
	if e = os.WriteFile(target, newer, 0600); e != nil {
		t.Fatal(e)
	}
	if e = c.commitDebriefResult(); e != nil {
		t.Fatal(e)
	}
	restart, e := gamestate.OpenSaveStore(c.Store.Directory, "")
	if e != nil {
		t.Fatal(e)
	}
	if e = restart.CommitSortie(p.Facts.SortieID); e != nil {
		t.Fatal(e)
	}
	got, _ := os.ReadFile(target)
	if !bytes.Equal(got, newer) || !reflect.DeepEqual(saved, restart.State()) {
		t.Fatal("duplicate/restart rewrote summary or career")
	}
}

func TestSummarySecondAcknowledgedMissionReplacesPriorOutput(t *testing.T) {
	c := developmentScenarioController(t, "campaign-return")
	c.SummaryDirectory = t.TempDir()
	target := filepath.Join(c.SummaryDirectory, gamestate.MissionSummaryFilename)
	var first []byte
	for sortie := 0; sortie < 2; sortie++ {
		campaignNaturalBaseEnd(t, c)
		if sortie == 1 {
			b, e := os.ReadFile(target)
			if e != nil || !bytes.Equal(b, first) {
				t.Fatal("unacknowledged second mission replaced first report", e)
			}
		}
		loopAcknowledge(t, c)
		b, e := os.ReadFile(target)
		if e != nil || !strings.Contains(string(b), "MISSION SUMMARY") {
			t.Fatal("completed mission report missing", e)
		}
		if sortie == 1 {
			info, e := os.Stat(target)
			if e != nil || !info.ModTime().After(time.Unix(1, 0)) {
				t.Fatal("second final acknowledgement did not replace prior complete output", e)
			}
			break
		}
		first = b
		// Preserve the complete bytes, but give the prior file a known old time
		// so identical no-shoot summaries still prove a new atomic publication.
		if e = os.Chtimes(target, time.Unix(1, 0), time.Unix(1, 0)); e != nil {
			t.Fatal(e)
		}
		for i := 0; i < 100 && c.Screen == ScreenInterstitial; i++ {
			if e = c.Tick(); e != nil {
				t.Fatal(e)
			}
		}
		startRendering(t, c)
		// Reuse N5's labelled approach fixture, not a staged result/credit.
		if e = c.developmentScenarioPose(c.Rendering.ResultTargets.Records[0], true, 2, 1, 0); e != nil {
			t.Fatal(e)
		}
	}
}

func TestSummaryWriteFailureDoesNotBlockCareerFilmOrRestart(t *testing.T) {
	var output bytes.Buffer
	logger := diagnostics.Start(t.TempDir(), configuration.Logging{ShowLog: true, Level: "DEBUG", FileName: "unused.txt", FileLimitMB: 1}, diagnostics.Options{Console: &output})
	defer logger.Close()
	c := developmentScenarioController(t, "o-short-campaign")
	bad := filepath.Join(t.TempDir(), "unwritable-runtime")
	if e := os.WriteFile(bad, []byte("unrelated existing file"), 0600); e != nil {
		t.Fatal(e)
	}
	c.SummaryDirectory = bad
	before := c.Store.State().SelectedPilot().Statistics.MissionCampaignCounter
	p := campaignNaturalBaseEnd(t, c)
	ackToLibrary(t, c)
	if c.Store.State().SelectedPilot().Statistics.MissionCampaignCounter != before+1 || len(c.Library.Entries) != 1 || !strings.Contains(output.String(), "Completed mission summary not saved") || !strings.Contains(output.String(), bad) {
		t.Fatal("optional report failure blocked result/film or lost cause", output.String())
	}
	got, _ := os.ReadFile(bad)
	if string(got) != "unrelated existing file" {
		t.Fatal("failed destination was replaced")
	}
	restart, e := gamestate.OpenSaveStore(c.Store.Directory, "")
	if e != nil {
		t.Fatal(e)
	}
	if e = restart.CommitSortie(p.Facts.SortieID); e != nil || !reflect.DeepEqual(c.Store.State(), restart.State()) {
		t.Fatal("report failure affected exactly-once restart", e)
	}
}

func TestSummaryRejectedCommitKeepsPreviousReport(t *testing.T) {
	c := developmentScenarioController(t, "o-short-campaign")
	c.SummaryDirectory = t.TempDir()
	target := filepath.Join(c.SummaryDirectory, gamestate.MissionSummaryFilename)
	old := []byte("previous acknowledged mission\n")
	if e := os.WriteFile(target, old, 0600); e != nil {
		t.Fatal(e)
	}
	campaignNaturalBaseEnd(t, c)
	path := filepath.Join(c.Store.Directory, "pilots", c.Debrief.Pending.Start.PilotID+".json")
	b, e := os.ReadFile(path)
	if e != nil {
		t.Fatal(e)
	}
	// Controlled external-edit conflict in an owned fixture: valid JSON, changed
	// accepted hash. The result must stay pending and the old summary untouched.
	if e = os.WriteFile(path, append(b, '\n'), 0600); e != nil {
		t.Fatal(e)
	}
	if e = c.commitDebriefResult(); e == nil {
		t.Fatal("controlled career conflict unexpectedly committed")
	}
	got, _ := os.ReadFile(target)
	if !bytes.Equal(got, old) {
		t.Fatal("rejected commit published a report")
	}
}
