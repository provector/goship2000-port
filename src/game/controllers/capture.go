package controllers

import (
	"fmt"
	"math"
)

// Capture listens for a change from a fresh baseline. Held-at-start controls
// must release first; absolute throttle position alone is never a gesture.
type Capture struct {
	ignoredAxes   map[int]float64
	Action        string
	Baseline      State
	blocked       map[Control]bool
	endpointReady []bool
	Negative      *Control
	Done          bool
	Binding       Binding
	Prompt        string
}

func NewCapture(id string, s State) *Capture {
	a, _ := ActionByID(id)
	p := "Press a button or hat direction"
	if a.Axis {
		p = "Move an axis, or press " + PairPrompt(id, false) + " control"
	}
	c := &Capture{Action: id, Baseline: cloneState(s), blocked: map[Control]bool{}, Prompt: p}
	for _, v := range discrete(s) {
		if v.Held(s) || v.Kind == "hat" && s.Hats[v.Index] != 0 {
			c.blocked[v] = true
		}
	}
	if _, ok := DirectionByID(id); ok {
		c.Prompt = "Press a button/hat, or move a digital axis endpoint from neutral"
		c.endpointReady = make([]bool, len(s.Axes))
		for i, v := range s.Axes {
			c.endpointReady[i] = math.Abs(v) <= PreviewDeadZone
		}
	}
	return c
}
func cloneState(s State) State {
	return State{append([]float64(nil), s.Axes...), append([]bool(nil), s.Buttons...), append([]uint8(nil), s.Hats...)}
}
func discrete(s State) []Control {
	var out []Control
	for i := range s.Buttons {
		out = append(out, Control{Kind: "button", Index: i})
	}
	for i := range s.Hats {
		for _, d := range []uint8{1, 2, 4, 8, 3, 6, 9, 12} {
			out = append(out, Control{Kind: "hat", Index: i, Direction: d})
		}
	}
	return out
}
func (c *Capture) Update(s State) (Binding, bool) {
	if c.Done {
		return c.Binding, true
	}
	if _, ok := DirectionByID(c.Action); ok {
		for i, v := range s.Axes {
			if i >= len(c.endpointReady) {
				continue
			}
			if math.Abs(v) <= PreviewDeadZone {
				c.endpointReady[i] = true
				continue
			}
			if c.endpointReady[i] && math.Abs(v) >= 0.95 {
				sign := int8(1)
				if v < 0 {
					sign = -1
				}
				control := Control{Kind: "axis_endpoint", Index: i, Sign: sign}
				c.Binding = Binding{Kind: "control", Control: &control}
				c.Done = true
				return c.Binding, true
			}
		}
	}
	for i, base := range c.ignoredAxes {
		if i < len(s.Axes) && math.Abs(s.Axes[i]-base) < .12 {
			delete(c.ignoredAxes, i)
			c.Baseline.Axes[i] = s.Axes[i]
		}
	}
	a, _ := ActionByID(c.Action)
	if a.Axis && c.Negative == nil {
		best := -1
		delta := CaptureThreshold
		for i, v := range s.Axes {
			if _, ignore := c.ignoredAxes[i]; ignore {
				continue
			}
			if i >= len(c.Baseline.Axes) {
				continue
			}
			d := math.Abs(v - c.Baseline.Axes[i])
			if d > delta {
				best = i
				delta = d
			}
		}
		if best >= 0 {
			c.Done = true
			c.Binding = Binding{Kind: "axis", Axis: best}
			return c.Binding, true
		}
	}
	for _, v := range discrete(s) {
		held := v.Held(s)
		if c.blocked[v] {
			if !held && (v.Kind != "hat" || s.Hats[v.Index] == 0) {
				delete(c.blocked, v)
			}
			continue
		}
		if !held || v.Held(c.Baseline) {
			continue
		}
		c.blocked[v] = true
		if !a.Axis {
			c.Done = true
			c.Binding = Binding{Kind: "control", Control: &v}
			return c.Binding, true
		}
		if c.Negative == nil {
			q := v
			c.Negative = &q
			c.Prompt = fmt.Sprintf("Release, then press %s control", PairPrompt(c.Action, true))
			c.Baseline = cloneState(s)
			return Binding{}, false
		}
		if *c.Negative == v {
			continue
		}
		if c.Negative.Held(s) {
			continue
		}
		q := v
		c.Done = true
		c.Binding = Binding{Kind: "pair", Negative: c.Negative, Positive: &q}
		return c.Binding, true
	}
	c.Baseline.Buttons = append(c.Baseline.Buttons[:0], s.Buttons...)
	c.Baseline.Hats = append(c.Baseline.Hats[:0], s.Hats...)
	return Binding{}, false
}

// SuppressAxis consumes the prior completing movement only. Other axes and
// genuinely new discrete edges remain live while that axis returns or is parked.
func (c *Capture) SuppressAxis(i int, baseline float64) {
	if c.ignoredAxes == nil {
		c.ignoredAxes = map[int]float64{}
	}
	c.ignoredAxes[i] = baseline
}
