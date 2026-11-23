package menus

import (
	"gunship2000-assets/game/navigation"
	"testing"
)

func TestRouteFinishDeviceEdges(t *testing.T) {
	for _, device := range []string{"keyboard", "mouse", "controller"} {
		var l inputLatch
		var next Buttons
		switch device {
		case "keyboard":
			next.Backspace = true
		case "mouse":
			next.RightMouse = true
		case "controller":
			next.ControllerTarget = true
		}
		for i, held := range []bool{false, true, true, true, false, false, true} {
			var input Buttons
			if held {
				input = next
			}
			a := l.Read(input)
			a = normalizeSceneSelectors(a, false, a.PointerPressed && input.RightMouse, true)
			if a.Selector2 != (i == 1 || i == 6) {
				t.Fatal(device, i, a.Selector2)
			}
		}
	}
}

func TestIsolatedResumeConfirmsPartialRoute(t *testing.T) {
	c := objectiveController(t, 38, 5, false)
	c.simulationProfile = "recorded"
	c.filmIsolation = &filmSessionIsolation{Mode: "playback"}
	playback := &FilmPlayback{Engine: c}
	if err := playback.Resume(); err != nil {
		t.Fatal(err)
	}
	if err := c.openGPS(); err != nil {
		t.Fatal(err)
	}
	for _, action := range []Actions{{GPSKey: "f"}, {Selector1: true, Inside: true, X: 100, Y: 70}, {Backspace: true}} {
		if err := c.handleGPS(action); err != nil {
			t.Fatal(err)
		}
	}
	r := c.Rendering.Flight.Navigation[0]
	if c.Rendering.GPS == nil || c.Rendering.GPS.Editing || r.Index() != 1 || r.Point(1) != navigation.HighPoint(100, 70) {
		t.Fatalf("practice route was not confirmed: %+v", r)
	}
}
