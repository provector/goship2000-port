package menus

import (
	"gunship2000-assets/game/replay"
	gamestate "gunship2000-assets/game/state"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func ackToLibrary(t *testing.T, c *FrontEnd) {
	t.Helper()
	for i := 0; i < 25 && c.Screen == ScreenDebrief; i++ {
		if c.Debrief.Page == "crew" {
			c.Debrief.CrewSelection = len(c.Debrief.Pending.Proposal.Result.Recommendations)
		}
		c.Debrief.Tick = 400
		if e := c.Handle(Actions{Enter: true}); e != nil {
			t.Fatal(e)
		}
		if e := c.Handle(Actions{}); e != nil {
			t.Fatal(e)
		}
	}
	if c.Screen != ScreenFilms || c.Library == nil {
		t.Fatal("ack must enter library", c.Screen, c.Message)
	}
}
func TestPostAckLibraryPlaybackPracticeAndCampaignSuccessorOnce(t *testing.T) {
	c := developmentScenarioController(t, "o-short-campaign")
	before := c.Store.State()
	p := campaignNaturalBaseEnd(t, c) // actual source base/contact and rotor-off
	id := p.Facts.SortieID
	ackToLibrary(t, c)
	committed := c.Store.State()
	if committed.SelectedPilot().Statistics.MissionCampaignCounter != before.SelectedPilot().Statistics.MissionCampaignCounter+1 {
		t.Fatal("final ack did not commit once")
	}
	if c.Mission != nil || len(c.Library.Entries) != 1 {
		t.Fatal("library must precede next preparation", c.Mission, c.Library.Entries, c.Library.Status)
	}
	entry := c.Library.Entries[0]
	b, e := os.ReadFile(publishedFilmPath(t, c, entry.ID))
	if e != nil {
		t.Fatal(e)
	}
	if e = c.activateFilmControl(15); e != nil {
		t.Fatal(e)
	}
	if c.Screen != ScreenReplay {
		t.Fatal(c.Library.Status)
	}
	for !c.Viewer.Playback.Done {
		if e = c.Viewer.Playback.Step(); e != nil {
			t.Fatal(e)
		}
	}
	if e = c.AdvanceReplay(0); e != nil {
		t.Fatal(e)
	}
	if c.Screen != ScreenFilms || !reflect.DeepEqual(committed, c.Store.State()) {
		t.Fatal("playback changed commit or return token")
	}
	// A new playback can yield an actual runtime to practice before ending.
	if e = c.activateFilmControl(15); e != nil {
		t.Fatal(e)
	}
	if e = c.activateReplayControl(16); e != nil {
		t.Fatal(e)
	}
	if !c.Viewer.Practice || c.Viewer.Playback.Engine.Store != nil {
		t.Fatal("practice acquired a writer")
	}
	if e = c.handleReplay(Actions{EndMission: true}); e != nil {
		t.Fatal(e)
	}
	if c.Screen == ScreenReplay {
		if rgba, err := c.RenderRGBA(false); err != nil || rgba == nil || rgba.Bounds().Dx() < 320 {
			t.Fatal("practice confirmation has no source framebuffer", err)
		}
		if e = c.handleReplay(Actions{ConfirmQuit: true}); e != nil {
			t.Fatal(e)
		}
		_ = c.AdvanceReplay(0)
	}
	if c.Screen != ScreenFilms || !reflect.DeepEqual(committed, c.Store.State()) {
		t.Fatal("practice published a result", c.Screen)
	}
	after, e := os.ReadFile(publishedFilmPath(t, c, entry.ID))
	if e != nil || string(b) != string(after) {
		t.Fatal("practice modified film", e)
	}
	if e = c.exitFilmLibrary(); e != nil {
		t.Fatal(e)
	}
	if c.Screen != ScreenInterstitial || c.Mission == nil {
		t.Fatal("EXIT did not prepare next mission", c.Screen)
	}
	next := c.Mission
	for i := 0; i < 3; i++ {
		if e = c.exitFilmLibrary(); e != nil {
			t.Fatal(e)
		}
		if e = c.commitDebriefResult(); e != nil {
			t.Fatal(e)
		}
		c.Frame(false)
	}
	if c.Mission != next || !reflect.DeepEqual(committed, c.Store.State()) {
		t.Fatal("duplicate exit/ack repeated preparation or rewards")
	}
	if e = c.Store.CommitSortie(id); e != nil {
		t.Fatal(e)
	}
	reopened, e := gamestate.OpenSaveStore(c.Store.Directory, "")
	if e != nil || !reflect.DeepEqual(committed, reopened.State()) {
		t.Fatal("restart changed result", e)
	}
	startup, e := NewFrontEnd(c.Office.Assets, reopened)
	if e != nil || startup.Screen != ScreenOffice {
		t.Fatal("restart forced library", e)
	}
	if rec, e := reopened.RecoveryCandidate(); e != nil || rec == nil {
		t.Fatal("replay consumed Phoenix", e)
	}
}

func TestFilmWriteFailureAndLibraryQuitPreserveCommittedCareer(t *testing.T) {
	c := developmentScenarioController(t, "o-short-campaign")
	p := campaignNaturalBaseEnd(t, c)
	blocker := filepath.Join(t.TempDir(), "not-a-directory")
	if e := os.WriteFile(blocker, []byte("preserve"), 0600); e != nil {
		t.Fatal(e)
	}
	c.FilmDirectory = blocker // supplied disk-failure boundary after a valid recording
	ackToLibrary(t, c)
	committed := c.Store.State()
	if c.Library.Status == "" || committed.SelectedPilot().Statistics.LastMissionScore != p.Score.Score {
		t.Fatal("film error blocked result or was silent")
	}
	if e := c.Handle(Actions{Quit: true}); e != nil {
		t.Fatal(e)
	}
	if e := c.Handle(Actions{ConfirmQuit: true}); e != nil {
		t.Fatal(e)
	}
	reopened, e := gamestate.OpenSaveStore(c.Store.Directory, "")
	if e != nil || !reflect.DeepEqual(committed, reopened.State()) {
		t.Fatal("library quit undid durable result", e)
	}
	if rec, e := reopened.RecoveryCandidate(); e != nil || rec == nil {
		t.Fatal("film failure lost Phoenix", e)
	}
	b, e := os.ReadFile(blocker)
	if e != nil || string(b) != "preserve" {
		t.Fatal("film writer changed foreign file", e)
	}
	if e := reopened.CommitSortie(p.Facts.SortieID); e != nil || !reflect.DeepEqual(committed, reopened.State()) {
		t.Fatal("failure re-awarded sortie", e)
	}
}

func TestPausedHostReleaseDoesNotCreateFilmWaits(t *testing.T) {
	c := developmentScenarioController(t, "o-clean-flight")
	p := c.Rendering
	for i := 0; i < 200; i++ {
		if e := c.Handle(Actions{FlightFocusLost: true}); e != nil {
			t.Fatal(e)
		}
		if e := c.AdvanceRendering(.25); e != nil {
			t.Fatal(e)
		}
	}
	if p.Flight.Steps != 0 {
		t.Fatal("paused host time created physics")
	}
	if p.filmRecorder.operationCount() > 2 {
		t.Fatal("identical release transactions created host-wait stream", p.filmRecorder.operationCount())
	}
}

func TestActualMovingApproachReconstructs(t *testing.T) {
	c := developmentScenarioController(t, "o-clean-flight")
	p := c.Rendering
	initial := p.Flight.State.Pose
	a := FilmInitialAdjustment{Label: "Step O supplied initial forward approach, not a recorded pose correction", Approach: &FilmApproach{H0: initial.H0, H1: initial.H1, AboveSupport: 400, Rotor: 1, Stage: 128, Forward: 64}}
	if e := c.ApplyFilmInitialAdjustment(a); e != nil {
		t.Fatal(e)
	}
	initial = p.Flight.State.Pose
	if e := c.Handle(Actions{Cancel: true}); e != nil {
		t.Fatal(e)
	}
	for i := 0; i < 20; i++ {
		if e := c.advanceFlight(.1); e != nil {
			t.Fatal(e)
		}
	}
	if p.Flight.State.Pose.H0 == initial.H0 && p.Flight.State.Pose.H1 == initial.H1 {
		t.Fatal("fixture did not exercise actual motion")
	}
	endFilm(t, c)
	verifyFilm(t, c)
}

func publishedFilmPath(t *testing.T, c *FrontEnd, id string) string {
	t.Helper()
	rows, err := (replay.Store{Root: c.FilmDirectory}).List()
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range rows {
		if r.ID == id && r.Error == "" {
			return filepath.Join(c.FilmDirectory, r.Title+".g2r")
		}
	}
	t.Fatalf("missing published film %s", id)
	return ""
}
