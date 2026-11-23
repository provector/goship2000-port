package menus

import (
	"gunship2000-assets/game/flight"
	"gunship2000-assets/game/render3d"
	"testing"
	"time"
)

func TestPresentationWrapAndDiscreteState(t *testing.T) {
	// These are native interpolation expectations, not original physics claims.
	if got := lerpAngle(0xff00, 0x100, .5); got != 0 {
		t.Fatalf("angle wrapped wrong:%x", got)
	}
	if got := lerpAngle(0x100, 0xff00, .5); got != 0 {
		t.Fatalf("reverse angle:%x", got)
	}
	a, b := render3d.Pose{H0: 100, H1: 200, Height: 50}, render3d.Pose{H0: 200, H1: 300, Height: 70}
	if got := lerpPose(a, b, .25); got != (render3d.Pose{H0: 125, H1: 225, Height: 55}) {
		t.Fatal(got)
	}
}
func TestClockInterpolationDoesNotConsumeUpdates(t *testing.T) {
	var c flight.Clock
	h := flight.StepDuration() / 2
	if len(c.Advance(h, false)) != 0 {
		t.Fatal("half tick advanced")
	}
	f := c.InterpolationFraction()
	if f < .499 || f > .501 {
		t.Fatal(f)
	}
	for i := 0; i < 144; i++ {
		if c.InterpolationFraction() != f {
			t.Fatal("drawing consumed clocks")
		}
	}
	if len(c.Advance(h+time.Nanosecond, false)) != 1 {
		t.Fatal("logical tick missing")
	}
	c.ResetElapsed()
	if c.InterpolationFraction() != 0 {
		t.Fatal("pause retained elapsed")
	}
}
