package menus

import (
	"gunship2000-assets/game/configuration"
	"testing"
	"time"
)

func TestGPSOverviewSpeedPolicyLeavesZoomAndFlightUnchanged(t *testing.T) {
	for _, fps := range []int{30, 60, 144} {
		c := mastFixture(t, 0, "pass")
		c.Runtime = configuration.Defaults()
		jFinishFade(t, c)
		c.Handle(Actions{})
		c.Handle(Actions{GPSOpen: true})
		p := c.Rendering
		if p.GPS == nil {
			t.Fatal("GPS missing")
		}
		c.Handle(Actions{GPSKey: "f"})
		c.Handle(Actions{Inside: true, X: 100, Y: 100})
		pose, rng, steps := p.Flight.State.Pose, p.Initialized.RandomState, p.Flight.Steps
		advance := func() {
			ticks := 0
			for phase := 0; phase < 2; phase++ {
				held := [4]bool{}
				if phase == 0 {
					held[1] = true
				}
				c.Handle(Actions{SceneDirections: held})
				for frame := 1; ; frame++ {
					elapsed := min(time.Duration(frame)*time.Second/time.Duration(fps), 100*time.Millisecond)
					goal := int(int64(time.Duration(phase)*100*time.Millisecond+elapsed) * 70 / int64(time.Second))
					for ticks < goal {
						if err := c.Tick(); err != nil {
							t.Fatal(err)
						}
						ticks++
					}
					c.Frame(false)
					if elapsed == 100*time.Millisecond {
						break
					}
				}
			}
		}
		advance()
		// Source28 held+4 release pixels / native default4 =8.
		if p.GPS.CursorX != 108 || p.GPS.CursorY != 100 {
			t.Fatalf("fps%d overview%v", fps, p.GPS.CursorPoint())
		}
		c.Handle(Actions{GPSKey: "space"})
		center := p.GPS.Center
		advance()
		// Detail keeps source Axis<<5. Held sum448 and decay sum109 =>557*32.
		if p.GPS.Center.H0 != center.H0+17824 || p.GPS.Center.H1 != center.H1 {
			t.Fatalf("fps%d zoom delta%v->%v", fps, center, p.GPS.Center)
		}
		if p.Flight.State.Pose != pose || p.Initialized.RandomState != rng || p.Flight.Steps != steps || p.Flight.Axes.X != 0 {
			t.Fatal("GPS keyboard affected paused flight")
		}
		c.Handle(Actions{Cancel: true}) // rollback edit, remain GPS.
		c.Handle(Actions{Cancel: true}) // ordinary return.
		if p.GPS != nil {
			t.Fatal("GPS transaction ownership changed")
		}
	}
}
