package menus

import (
	gamestate "gunship2000-assets/game/state"
	"testing"
)

func TestSingleGroupPaintAndActualMenuOwner(t *testing.T) {
	c := gulfEditor(t)
	e := c.Editor
	before := gamestate.AuthoredRevision(e.Document)
	for n := 0; n < 8; n++ {
		frame := append([]byte(nil), c.editorAssets().Panel...)
		c.drawEditorControlOverlay(frame)
		// Upper AREA18 is SINGLE, lower AREA19 GROUP. The bank's old bevel
		// is explicitly restored even when the current owner is not selected.
		single, group := byte(8), byte(15)
		if e.Formation {
			single, group = group, single
		}
		if frame[158*320+143] != single || frame[178*320+143] != group {
			t.Fatalf("mode%v: single/group%v/%v", e.Formation, frame[158*320+143], frame[178*320+143])
		}
		opts, err := c.editorOptions("defender", 0, 10)
		if err != nil {
			t.Fatal(err)
		}
		w, err := gamestate.AuthoredEditorWorld(c.editorData(), e.Document.World)
		if err != nil {
			t.Fatal(err)
		}
		for _, o := range opts {
			want := byte(2)
			if e.Formation {
				want = 1
			}
			if w.FRMD[o.Component*8+2] != want {
				t.Fatal("paint/menu descriptor diverged", e.Formation, o)
			}
		}
		// Source both button handlers toggle C01A; do not replace them with setters.
		if !c.editorControlCommand(18 + n%2) {
			t.Fatal("mode command")
		}
	}
	if before != gamestate.AuthoredRevision(e.Document) {
		t.Fatal("mode view wrote authored data/RNG")
	}
}

func TestFormationMapMarksUseSourceScreenTriangle(t *testing.T) {
	c := gulfEditor(t)
	e := c.Editor
	w, err := gamestate.AuthoredEditorWorld(c.editorData(), e.Document.World)
	if err != nil {
		t.Fatal(err)
	}
	var group, single gamestate.AuthoredPlacement
	foundGroup, foundSingle := false, false
	for _, formation := range []bool{false, true} {
		opts, _, err := gamestate.AuthoredEditorOptions(c.editorData(), e.Document, "defender", 0, 10, formation)
		if err != nil {
			t.Fatal(err)
		}
		for _, o := range opts {
			p := gamestate.AuthoredPlacement{Role: "defender", Formation: o.Formation, Component: o.Component, Template: o.Template, SelectionProfile: o.SelectionProfile}
			if formation && w.FRMD[p.Component*8+2] == 1 {
				group = p
				foundGroup = true
				break
			}
			if !formation && w.FRMD[p.Component*8+2] == 2 {
				single = p
				foundSingle = true
				break
			}
		}
	}
	if !foundGroup || !foundSingle {
		t.Fatal("source fixture formation/single missing")
	}
	const x, y = 80, 60
	actual, want := make([]byte, 64000), make([]byte, 64000)
	c.editorFormationIcon(actual, group, 9, x, y, 0)
	c.editorIconRow(want, 9, x-4, y+5, false, 0)
	c.editorIconRow(want, 9, x+4, y+5, false, 0)
	if string(actual) != string(want) {
		t.Fatal("formation map overlay differs from source triangle")
	}
	empty := make([]byte, 64000)
	c.editorFormationIcon(empty, single, 9, x, y, 0)
	if string(empty) != string(make([]byte, 64000)) {
		t.Fatal("single became formation glyphs")
	}
	for _, index := range []int{10, 11} {
		c.editorFormationIcon(empty, group, index, x, y, 0)
	}
	if string(empty) != string(make([]byte, 64000)) {
		t.Fatal("base/FARP repeated")
	}
}

func TestSearchDestroyTwoReachedPointerPlacements(t *testing.T) {
	c := gulfEditor(t)
	e := c.Editor
	opts, _, err := gamestate.AuthoredEditorOptions(c.editorData(), e.Document, "defender", 0, 10, false)
	if err != nil || len(opts) == 0 {
		t.Fatal(err)
	}
	if err = gamestate.PlaceAuthoredOption(c.editorData(), e.Document, "defender", 0, opts[0], 400, 400); err != nil {
		t.Fatal(err)
	}
	unit := e.Document.Placements[len(e.Document.Placements)-1]
	x, y := c.editorScreen(unit.X, unit.Y)
	if err = c.editorArm("primary", 0, 0); err != nil {
		t.Fatal(err)
	}
	editorClick(t, c, x, y)
	if e.SearchRole != "primary" || !e.Held {
		t.Fatal("first click did not retain second marker", e.Status, e.ShadowError)
	}
	editorClick(t, c, 95, 90)
	if e.SearchRole != "" || e.Held {
		t.Fatal("second marker did not complete", e.Status, e.ShadowError)
	}
	var goal *gamestate.AuthoredPlacement
	for i := range e.Document.Placements {
		if e.Document.Placements[i].Role == "primary" {
			goal = &e.Document.Placements[i]
		}
	}
	if goal == nil || goal.Search == nil || goal.Reference != unit.ID {
		t.Fatal("reached operation lost search marker/reference", goal)
	}
	wx, wy := editorOverviewPoint(95, 90)
	if goal.Search.X != wx || goal.Search.Y != wy {
		t.Fatal("second marker was snapped", goal.Search)
	}
	// Controller inputs are not OS-input evidence; this checks reached consumer ordering.
}
