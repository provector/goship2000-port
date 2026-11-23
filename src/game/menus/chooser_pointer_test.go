package menus

import (
	gamestate "gunship2000-assets/game/state"
	"reflect"
	"testing"
)

func TestMissionCursorBoundsFollowAvailableRows(t *testing.T) {
	for _, n := range []int{1, 2, 5} {
		c := &FrontEnd{Screen: ScreenMission, Choices: make([]gamestate.MissionType, n)}
		bounds, ok := c.missionCursorBounds()
		if !ok || bounds != [4]int{73, 75, 88, n*10 + 8} {
			t.Fatalf("%d rows: %v", n, bounds)
		}
		for _, point := range [][2]int{{0, 0}, {319, 199}, {100, 80}} {
			x, y := c.updateGamePointer(point[0], point[1], true, true)
			before := c.missionPointer
			if !insideRect(x, y, bounds) || before != c.missionPointer {
				t.Fatal("cursor/hover/selector disagree")
			}
		}
		c.Screen = ScreenOffice
		if x, y := c.updateGamePointer(319, 199, true, false); x != 319 || y != 199 {
			t.Fatal("confinement survived chooser exit")
		}
	}
	if _, ok := (&FrontEnd{}).missionCursorBounds(); ok {
		t.Fatal("empty chooser invented a row")
	}
}

func TestMissionBlankPaperCannotCommitAndCancelReleasesCursor(t *testing.T) {
	c := sourceFrontEnd(t, true)
	before := c.Store.State()
	if err := c.openMission(); err != nil {
		t.Fatal(err)
	}
	for _, p := range [][2]int{{10, 20}, {100, 150}, {319, 199}, {74, 82}} {
		act(t, c, Actions{X: p[0], Y: p[1], Inside: true, PointerReset: true, Selector1: true, PointerPressed: true})
		if c.Screen != ScreenMission || !reflect.DeepEqual(before, c.Store.State()) {
			t.Fatal("blank paper committed preselection")
		}
		if c.Office.PointerX != max(73, min(p[0], 160)) {
			t.Fatal("drawn pointer not confined")
		}
	}
	act(t, c, Actions{X: 300, Y: 180, Inside: true, Cancel: true})
	if c.Screen != ScreenOffice || c.Office.PointerX != 300 || c.Office.PointerY != 180 || !reflect.DeepEqual(before, c.Store.State()) {
		t.Fatal("cancel retained confinement or changed save")
	}
	if err := c.openMission(); err != nil {
		t.Fatal(err)
	}
	act(t, c, Actions{X: 100, Y: 92, Inside: true, PointerReset: true, Selector1: true, PointerPressed: true})
	if c.Screen != ScreenOffice || c.Store.State().SelectedPilot().MissionType != gamestate.MissionSingle {
		t.Fatal("actual choice did not commit")
	}
	act(t, c, Actions{X: 300, Y: 180, Inside: true})
	if c.Office.PointerX != 300 || c.Office.PointerY != 180 {
		t.Fatal("commit retained confinement")
	}
}
