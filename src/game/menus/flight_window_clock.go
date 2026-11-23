package menus

import (
	"math"
	"time"
)

// Raylib's frame interval describes the preceding frame. The first poll after
// a modal closes still measures modal work (including saving or resizing).
// Timestamp flight input and advance the simulation from the same live frames.
type flightWindowClock struct {
	inputTime time.Duration
	modal     bool
}

func (c *flightWindowClock) frame(seconds float64, blocked bool) (time.Duration, float64) {
	if blocked || c.modal {
		seconds = 0
	}
	c.modal = blocked
	c.inputTime += time.Duration(math.Round(seconds * float64(time.Second)))
	return c.inputTime, seconds
}
