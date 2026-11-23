package menus

import (
	"reflect"
	"testing"
	"time"
)

func TestRotorLogicalRequestsIndependentOfDisplaySchedule(t *testing.T) {
	// Supplied collective16/running-rotor values isolate this caller test; they
	// are not claimed to be this mission's naturally initialized first frame.
	// Hand source arguments:17>>1=8,21>>1=10,20>>1=10,16>>1=8,rotorOFF=0.
	want := []uint16{8, 10, 10, 8, 0}
	for _, mode := range []string{"smooth", "stepped"} {
		for _, fps := range []int{30, 60, 144} {
			c := mastFixture(t, 0, "pass")
			jFinishFade(t, c)
			p := c.Rendering
			p.presentation.mode = mode
			p.Flight.State.Collective = 16
			p.Flight.State.Rotor = 1
			p.Flight.State.Hover = 0
			p.Flight.State.HeightControl = 0
			p.TakeRotorCommands()
			if err := c.Handle(Actions{}); err != nil {
				t.Fatal(err)
			}
			events := []struct {
				at      time.Duration
				command int
			}{
				{100 * time.Millisecond, 0}, {300 * time.Millisecond, 0x100},
				{500 * time.Millisecond, 1}, {700 * time.Millisecond, 0x101}, {900 * time.Millisecond, 2},
			}
			event := 0
			last := time.Duration(0)
			var got []uint16
			for frame := 1; frame <= fps*3/2; frame++ {
				now := time.Duration(frame) * time.Second / time.Duration(fps)
				for event < len(events) && events[event].at <= now {
					e := events[event]
					if err := c.Handle(Actions{InputTime: e.at, FlightCommands: []int{e.command}, FlightHeldValid: true}); err != nil {
						t.Fatal(err)
					}
					event++
				}
				if err := c.Handle(Actions{InputTime: now, FlightHeldValid: true}); err != nil {
					t.Fatal(err)
				}
				if err := c.AdvanceRendering(float64(now-last) / float64(time.Second)); err != nil {
					t.Fatal(err)
				}
				last = now
				got = append(got, p.TakeRotorCommands()...)
				c.Frame(false)
				p.Dirty = true
				c.Frame(false)
				if drawn := p.TakeRotorCommands(); len(drawn) != 0 {
					t.Fatal("drawing emitted rotor source command", mode, fps, drawn)
				}
			}
			if !reflect.DeepEqual(got, want) {
				t.Fatal(mode, fps, got, want)
			}
		}
	}
}
