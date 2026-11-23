package flight

import "time"

// NativeClockPolicy decouples the source minimum six driver clocks from render
// cost. It selects the AdLib baseline divisor, not the transient noise divisor
// or an original universally fixed simulation rate.
const NativeClockPolicy = "adlib-baseline-six-clock-quantum-v1"

const (
	SourcePITHz           int64 = 1193182
	SourcePITDivisor      int64 = 19600
	NativeClocksPerUpdate int64 = 6
)

type Timing struct {
	DF    uint8
	E0    bool
	Phase uint16
}

// Clock owns scheduling and the source DA/DC stagger. Pause drops fractional
// elapsed time; no wall-time spent in a modal is replayed on resume.
type Clock struct {
	accumulator int64
	started     bool
	dc          int16
	phase       uint16
}

func (c *Clock) ResetElapsed() { c.accumulator = 0 }

// InterpolationFraction is presentation-only. It never consumes source clocks.
func (c *Clock) InterpolationFraction() float64 {
	return float64(c.accumulator) / float64(SourcePITDivisor*NativeClocksPerUpdate*int64(time.Second))
}

func StepDuration() time.Duration {
	return time.Duration(SourcePITDivisor * NativeClocksPerUpdate * int64(time.Second) / SourcePITHz)
}

func (c *Clock) next() Timing {
	df := uint8(NativeClocksPerUpdate)
	if !c.started {
		df = 10 // section0:1556, rather than the zero-initialized data byte.
		c.started = true
	}
	c.dc -= int16(df)
	e0 := c.dc <= 0
	if e0 {
		c.phase++
		c.dc += 10 // source does this once, not a catch-up while loop.
	}
	return Timing{DF: df, E0: e0, Phase: c.phase}
}

// Schedule credits host time without advancing DA/DC. PopScheduled commits one
// reached update, so a cancellation cannot advance counters for discarded work.
// Compression is a named native logical-rate mapping, not DOS wall calibration.
func (c *Clock) Schedule(elapsed time.Duration, rate int) {
	if elapsed <= 0 {
		return
	}
	if rate < 1 || rate > 16 {
		rate = 1
	}
	if elapsed > 250*time.Millisecond {
		elapsed = 250 * time.Millisecond
	}
	c.accumulator += int64(elapsed) * SourcePITHz * int64(rate)
	period := SourcePITDivisor * NativeClocksPerUpdate * int64(time.Second)
	// Bounded16-step batch: drop excess whole host debt, retain its fraction.
	if c.accumulator/period > 16 {
		c.accumulator = 16*period + c.accumulator%period
	}
}
func (c *Clock) PopScheduled() (Timing, bool) {
	period := SourcePITDivisor * NativeClocksPerUpdate * int64(time.Second)
	if c.accumulator < period {
		return Timing{}, false
	}
	c.accumulator -= period
	return c.next(), true
}
func (c *Clock) Advance(elapsed time.Duration, paused bool) []Timing {
	if paused {
		c.ResetElapsed()
		return nil
	}
	c.Schedule(elapsed, 1)
	var out []Timing
	for t, ok := c.PopScheduled(); ok; t, ok = c.PopScheduled() {
		out = append(out, t)
	}
	return out
}

// Compression retains source AF3 requests. Its labels are native nominal rates.
type Compression struct{ Request int16 }

func (c *Compression) Rate() int {
	if c.Request == 0 {
		return 1
	}
	return int(c.Request)
}
func (c *Compression) Increase() {
	if c.Request < 16 {
		c.Request += 2
	}
}
func (c *Compression) Reset() { c.Request = 0 }
func (c *Compression) Limit() {
	if c.Request > 4 {
		c.Request = 0
	} else {
		c.Request &= 6
	}
}
