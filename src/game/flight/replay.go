package flight

// ReplayTiming advances the same source counter leaf without host scheduling.
// Film timing is a checked witness; it never assigns DF/DC/DA from disk.
func (c *Clock) ReplayTiming() Timing { return c.next() }

type ReplayClockState struct {
	Started bool
	DC      int16
	Phase   uint16
}

func (c *Clock) ReplayState() ReplayClockState { return ReplayClockState{c.started, c.dc, c.phase} }
