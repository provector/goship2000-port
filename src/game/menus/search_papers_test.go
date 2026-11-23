package menus

import (
	"bytes"
	gamestate "gunship2000-assets/game/state"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestRetainedSearchDocumentExplicitSaveFeedsCorrectOrders(t *testing.T) {
	c := authoredUIController(t)
	d := copyDocumentFixture(t, c, "GS2000_P6_SEARCH_DOCUMENT")
	state := c.Store.State()
	state.SelectedPilot().Theatre = gamestate.TheatreAntarctica
	if err := c.Store.Commit(state); err != nil {
		t.Fatal(err)
	}
	if err := c.RefreshOffice(); err != nil {
		t.Fatal(err)
	}
	if err := c.openMissionFiles(); err != nil || len(c.Files.Entries) != 1 {
		t.Fatal(err)
	}
	oldRows := append([]gamestate.MissionObject(nil), d.Compiled.Objects...)
	path := filepath.Join(c.MissionDirectory, d.ID+".g2m")
	before, _ := os.ReadFile(path)
	if err := c.activateMissionFile(15); err != nil || c.Mission == nil || c.Mission.Data.Objectives["primary"].Name != "Sector" {
		t.Fatal("old BEGIN changed retained interpretation", err, c.Files.Status)
	}
	if after, _ := os.ReadFile(path); !bytes.Equal(before, after) {
		t.Fatal("BEGIN rewrote the document")
	}
	// Preflight has not created a sortie/result capability. Return to the file
	// controller fixture, then use its real EDIT/X/SAVE consumers to opt in.
	c.Mission, c.Screen = nil, ScreenMissionFiles
	if err := c.activateMissionFile(19); err != nil {
		t.Fatal(err)
	}
	editorKey(t, c, "x")
	if c.Editor.Stage != "exit" {
		t.Fatal("valid file did not reach SAVE")
	}
	editorKey(t, c, "s")
	if c.Screen != ScreenMissionFiles {
		t.Fatal("resave failed", c.Editor.Status)
	}
	loaded, _, err := c.Files.Store.Load(d.ID)
	if err != nil || loaded.TargetNameProfile != gamestate.AuthoredTargetNameProfile || loaded.Compiled.Objectives["primary"].Name != "KURASSIER" || !reflect.DeepEqual(oldRows, loaded.Compiled.Objects) {
		t.Fatal("explicit SAVE lost source unit/objects", err)
	}
	if err := c.activateMissionFile(15); err != nil || c.Mission == nil {
		t.Fatal(err, c.Files.Status)
	}
	orders := c.Mission.View.OrdersText()
	if !strings.Contains(orders, "KURASSIER") || strings.Contains(strings.ToUpper(orders), "SECTOR") || !strings.Contains(orders, "AAA Battery") {
		t.Fatal("wrong unit reached source Orders templates", orders)
	}
	goal := c.Mission.Data.Objectives["primary"]
	if goal.RawKind != 1 || goal.DisplayKind != 3 || goal.Point == goal.OriginalPoint {
		t.Fatal("paper fix relabelled/moved the independent search target", goal)
	}
	// The distinct Recon branch must still name its actual source Sector.
	q, _ := gamestate.CloneAuthored(loaded)
	for i := range q.Placements {
		if q.Placements[i].Role == "primary" {
			q.Placements[i].Kind = 6
			q.Placements[i].Search = nil
		}
	}
	q.Compiled = nil
	in := c.Mission.Data.Inputs
	recon, err := gamestate.CompileAuthoredMission(c.editorData(), q, in)
	if err != nil || recon.Objectives["primary"].RawKind != 7 || recon.Objectives["primary"].Name != "Sector" {
		t.Fatal("source Recon Sector was globally replaced", err)
	}
	t.Logf("identified old ANTC2 S&D document %s: explicit SAVE->BEGIN Name=%s raw%d mapped%d; search=%v actual=%v", d.ID, goal.Name, goal.RawKind, goal.DisplayKind, goal.Point, goal.OriginalPoint)
}
