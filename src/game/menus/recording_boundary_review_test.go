package menus

import (
	"encoding/json"
	assetdata "gunship2000-assets/game/assets"
	"gunship2000-assets/game/configuration"
	gamestate "gunship2000-assets/game/state"
	"os"
	"reflect"
	"strings"
	"testing"
)

// A supplied base approach is not continuous gameplay. All end/result/commit
// consumers are production, and the recorder failure is reached through its real
// closed transport rather than setting an Error field or replacing an I/O call.
func TestActualClosedRecorderCannotBlockCareerOrDuplicateAward(t *testing.T) {
	c := missionController(t)
	a, err := assetdata.LoadFrontEndAssets(os.Getenv("GS2000_TEST_MISSIONS"))
	if err != nil {
		t.Fatal(err)
	}
	c.Office.Assets, c.Menus = a, a.Menus
	c.RecordingLimit, err = configuration.ParseRecordingLimit("64")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = c.ConfigureDevelopmentScenario("o-short-campaign"); err != nil {
		t.Fatal(err)
	}
	r := c.Rendering.filmRecorder
	if r == nil || r.Spool == nil {
		t.Fatal("ordinary recorder absent")
	}
	if r.Spool.Limit() != 64<<20 || c.filmDiagnostics != r.Spool {
		t.Fatal("initial-adjustment spool lost selected budget/current diagnostic owner", r.Spool.Limit())
	}
	if err := r.Spool.Close(); err != nil {
		t.Fatal(err)
	}
	before, _ := json.Marshal(c.Store.State().SelectedPilot())
	proposal := campaignNaturalBaseEnd(t, c)
	if r.Error == "" || !strings.Contains(r.Error, "spool closed") || r.Spool.Failure() == nil || r.Spool.Failure().Phase != "append" {
		t.Fatal("real closed-transport append did not preserve first cause", r.Error, r.Spool.Failure())
	}
	if after, _ := json.Marshal(c.Store.State().SelectedPilot()); string(before) != string(after) {
		t.Fatal("career changed before finalack")
	}
	if _, err := c.SealRecordedFilm(); err == nil {
		t.Fatal("recorder-only failure admitted a film")
	}
	id := c.Debrief.Pending.Start.ID
	ackToLibrary(t, c)
	if c.Screen != ScreenFilms || c.Store.State().SelectedPilot().Statistics.LastMissionScore != proposal.Score.Score || !strings.Contains(c.Library.Status, "Result saved") {
		t.Fatal("optional recorder blocked valid result", c.Screen, c.Library.Status)
	}
	committed := c.Store.State()
	reopened, err := gamestate.OpenSaveStore(c.Store.Directory, "")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(committed, reopened.State()) {
		t.Fatal("restart lost career after recorder failure")
	}
	if err = reopened.CommitSortie(id); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(committed, reopened.State()) {
		t.Fatal("recorder failure caused duplicate awards")
	}
	if r.Spool.Failure().SessionID != id {
		t.Fatal("failure receipt lost actual career session", r.Spool.Failure().SessionID, id)
	}
	t.Logf("actual private-spool closed append; firstphase%s film%s session%s; career committed/reopened once", r.Spool.Failure().Phase, r.Spool.Failure().FilmID, id)
}
