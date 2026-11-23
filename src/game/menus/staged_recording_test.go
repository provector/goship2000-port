package menus

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Supplied multi-stage controls are not clean film initialization. Refusal to
// publish their incomplete stream must not impede the real career result.
func TestStagedCampaignFilmFailureDoesNotUndoResult(t *testing.T) {
	c := developmentScenarioController(t, "campaign-defeat")
	if c.Rendering.filmRecorder == nil || !strings.Contains(c.Rendering.filmRecorder.Error, "multi-stage") {
		t.Fatal("missing explicit staged recording admission")
	}
	last := filepath.Join(c.FilmDirectory, "last.json")
	prior := []byte("{\"Version\":1,\"ID\":\"11111111111111111111111111111111\"}\n")
	if e := os.WriteFile(last, prior, 0600); e != nil {
		t.Fatal(e)
	}
	scenarioSafeReturn(t, c)
	id := c.Debrief.Pending.Proposal.Facts.SortieID
	acknowledgeLibrary(t, c)
	if c.Store.State().SelectedPilot().Campaign != nil {
		t.Fatal("film failure blocked actual defeat commit")
	}
	if e := c.Store.CommitSortie(id); e != nil {
		t.Fatal(e)
	}
	b, e := os.ReadFile(last)
	if e != nil || !bytes.Equal(b, prior) {
		t.Fatal("incomplete staging replaced previous pointer")
	}
	if e = c.exitFilmLibrary(); e != nil || c.Screen != ScreenOffice {
		t.Fatal("film failure stranded successor", e, c.Screen)
	}
}
