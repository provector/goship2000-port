package menus

import (
	gamestate "gunship2000-assets/game/state"
	"testing"
)

func TestChooserRelativeEdgesCornersAndImmediateReversal(t *testing.T) {
	for _, n := range []int{1, 2, 5} {
		for _, delta := range [][2]int{{-1000, 0}, {1000, 0}, {0, -1000}, {0, 1000}, {-1000, -1000}, {1000, -1000}, {-1000, 1000}, {1000, 1000}} {
			c := &FrontEnd{Screen: ScreenMission, Choices: make([]gamestate.MissionType, n)}
			c.updateGamePointer(100, 85, true, true)
			rawX, rawY := 100, 85
			var x, y int
			for i := 0; i < 4; i++ {
				rawX += delta[0]
				rawY += delta[1]
				x, y = c.updateGamePointer(rawX, rawY, true, false)
				state := c.missionPointer
				for j := 0; j < 5; j++ {
					c.Hit(x, y)
				}
				if c.missionPointer != state {
					t.Fatal("hit query moved pointer")
				}
			}
			step := func(v int) int {
				if v < 0 {
					return 1
				}
				if v > 0 {
					return -1
				}
				return 0
			}
			x2, y2 := c.updateGamePointer(rawX+step(delta[0]), rawY+step(delta[1]), true, false)
			if x2 != x+step(delta[0]) || y2 != y+step(delta[1]) {
				t.Fatalf("edge reversal retained overshoot: %d,%d -> %d,%d", x, y, x2, y2)
			}
		}
	}
}

func TestChooserHandleUsesOnePositionAndConsumesMarginClicks(t *testing.T) {
	c := sourceFrontEnd(t, true)
	if err := c.openMission(); err != nil {
		t.Fatal(err)
	}
	before := c.Store.State().Revision
	if err := c.Handle(Actions{X: 100, Y: 90, Inside: true, PointerReset: true}); err != nil {
		t.Fatal(err)
	}
	for _, rawY := range []int{105, 110, 116, 116, 116} {
		if err := c.Handle(Actions{X: 100, Y: rawY, Inside: true}); err != nil {
			t.Fatal(err)
		}
		if c.Office.PointerY != 102 || c.missionPointer.y != 102 || c.Hit(c.Office.PointerX, c.Office.PointerY) != -1 {
			t.Fatal("draw/hover differ or lower margin not reachable")
		}
	}
	if err := c.Handle(Actions{X: 100, Y: 116, Inside: true, Selector1: true, PointerPressed: true}); err != nil {
		t.Fatal(err)
	}
	if c.Screen != ScreenMission || c.Store.State().Revision != before {
		t.Fatal("margin click committed highlight")
	}
	if err := c.Handle(Actions{X: 100, Y: 115, Inside: true}); err != nil {
		t.Fatal(err)
	}
	if c.Office.PointerY != 101 {
		t.Fatal("no immediate reversal")
	}
	if err := c.Handle(Actions{X: 0, Y: 0, Inside: false}); err != nil {
		t.Fatal(err)
	}
	if c.missionPointer.initialized {
		t.Fatal("focus/viewport loss kept anchor")
	}
	if err := c.Handle(Actions{X: 100, Y: 90, Inside: true, PointerReset: true}); err != nil {
		t.Fatal(err)
	}
	if c.Office.PointerY != 90 {
		t.Fatal("focus return did not reanchor")
	}
	if err := c.Handle(Actions{X: 300, Y: 180, Inside: true, Cancel: true}); err != nil {
		t.Fatal(err)
	}
	if c.Screen != ScreenOffice || c.Office.PointerX != 300 || c.missionPointer.initialized {
		t.Fatal("exit kept chooser transform")
	}
}
