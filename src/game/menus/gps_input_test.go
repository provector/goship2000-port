package menus

import (
	"gunship2000-assets/game/controllers"
	"testing"
	"time"
)

func TestRemappedGPSGestureOwnsWholeClickAndExit(t *testing.T) {
	c := renderingController(t)
	startRendering(t, c)
	jFinishFade(t, c)
	c.Handle(Actions{})
	if e := c.openGPS(); e != nil {
		t.Fatal(e)
	}
	c.Rendering.Flight.gpsNeedsRelease = false
	g, b := controllerDevice(c)
	p := g.live.Config.Profiles["p"]
	p.Bindings["menu.selector1"] = controllers.Binding{Kind: "control", Control: &controllers.Control{Kind: "button", Index: 10}}
	p.Bindings["menu.selector2"] = controllers.Binding{Kind: "control", Control: &controllers.Control{Kind: "button", Index: 11}}
	g.live.Config.Profiles["p"] = p
	var latch inputLatch
	tick := time.Duration(0)
	sample := func(primary, secondary bool, x, y int) {
		t.Helper()
		s := b.s.States[9]
		s.Buttons[10], s.Buttons[11] = primary, secondary
		b.s.States[9] = s
		buttons := g.Merge(c, Buttons{Inside: true, X: x, Y: y, InputTime: tick}, .016, 0, 0, false, nil)
		tick += 16 * time.Millisecond
		a := latch.Read(buttons)
		a = normalizeSceneSelectors(a, a.PointerLeftHeld, a.PointerRightHeld, true)
		if c.Rendering.GPS != nil {
			if e := c.handleGPS(a); e != nil {
				t.Fatal(e)
			}
		} else {
			if a.WeaponTriggerHeld || a.ControllerTarget {
				t.Fatal("flight action leaked exit gesture", a)
			}
		}
	}
	sample(false, false, 120, 70)
	saved := c.Rendering.Flight.Navigation
	sample(true, false, 120, 70)
	entered := c.Rendering.Flight.Navigation
	if !c.Rendering.GPS.Editing || entered[0].Index() != 0 {
		t.Fatal("entry placed")
	}
	for i := 0; i < 10; i++ {
		sample(true, false, 180, 45)
	}
	if entered != c.Rendering.Flight.Navigation {
		t.Fatal("held entry mutated route")
	}
	sample(false, false, 180, 45)
	if entered != c.Rendering.Flight.Navigation {
		t.Fatal("release mutated")
	}
	// Controller owns the logical pointer; set its visible point as an actual
	// pointer position receipt, not an OS cursor warp. Secondary cancels first.
	sample(true, false, 180, 45)
	if c.Rendering.Flight.Navigation[0].Index() != 1 {
		t.Fatal("fresh click missing")
	}
	sample(false, false, 180, 45)
	sample(false, true, 180, 45)
	if c.Rendering.GPS == nil || c.Rendering.GPS.Editing || c.Rendering.Flight.Navigation != saved {
		t.Fatal("cancel differs from native")
	}
	sample(false, false, 180, 45)
	sample(false, true, 180, 45)
	if c.Rendering.GPS != nil {
		t.Fatal("ordinary exit missing")
	}
	sample(false, true, 180, 45)
	sample(false, false, 180, 45)
}
