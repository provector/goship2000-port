package menus

import (
	"testing"
	"time"
)

func TestGraphicsModalDoesNotAgeFlightInput(t *testing.T) {
	var clock flightWindowClock
	var q flightCommandTimeline
	poll := func(seconds float64, blocked bool, command bool) float64 {
		timestamp, step := clock.frame(seconds, blocked)
		if !blocked {
			a := Actions{InputTime: timestamp}
			if command {
				a.FlightCommands = []int{3}
			}
			q.Sample(a)
		}
		return step
	}
	poll(0, false, false)
	for i := 0; i < 32; i++ {
		for _, d := range []float64{.017, .32, 1.6} {
			if step := poll(d, true, false); step != 0 {
				t.Fatal("modal advanced", step)
			}
		}
		if step := poll(.22, false, true); step != 0 {
			t.Fatal("closing frame charged to flight", step)
		}
		cmd, ok := q.NextLogical()
		if !ok || cmd != 3 || len(q.samples) != 0 {
			t.Fatalf("cycle %d: command delayed: %d, %v, queued %d", i, cmd, ok, len(q.samples))
		}
		// Model one complete active logical interval before the next menu.
		poll(float64(q.elapsed-clock.inputTime)/float64(time.Second), false, false)
	}
}
