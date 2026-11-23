package menus

import (
	"bytes"
	"encoding/json"
	"fmt"
	"gunship2000-assets/game/flightcontrol"
	"gunship2000-assets/game/navigation"
)

type filmModal struct {
	GPS          *navigation.ReplayState
	Axes         flightcontrol.Axes
	Held         [4]bool
	NeedsRelease bool
}

func (p *RenderingPreview) filmModalState() filmModal {
	r := p.Flight
	return filmModal{p.GPS.ReplayState(), r.gpsAxes, r.gpsHeld, r.gpsNeedsRelease}
}

type filmGPSTick struct {
	Held                            [4]bool
	Before, After                   flightcontrol.Axes
	RemainderBefore, RemainderAfter [2]int
	Handle                          bool
	Input                           navigation.Input
}

func (t filmGPSTick) validate() error {
	for _, a := range []int16{t.Before.X, t.Before.Y, t.After.X, t.After.Y, t.Input.AxisX, t.Input.AxisY} {
		if a < -127 || a > 127 {
			return fmt.Errorf("GPS axis outside source domain")
		}
	}
	for _, r := range [][2]int{t.RemainderBefore, t.RemainderAfter} {
		for _, n := range r {
			if n < -15 || n > 15 {
				return fmt.Errorf("GPS filter remainder outside domain")
			}
		}
	}
	i := t.Input
	i.AxisX = 0
	i.AxisY = 0
	if i != (navigation.Input{}) || !t.Handle && t.Input != (navigation.Input{}) {
		return fmt.Errorf("GPS tick contains host action")
	}
	return nil
}

// A GPS UI visit is not a flight quantum. Recompute ramp, filter and resolved
// input through the real consumer; a film may witness them but never assign them.
func (c *FrontEnd) consumeGPSTick() (*filmGPSTick, error) {
	p := c.Rendering
	if p == nil || p.GPS == nil || p.Flight == nil || p.Flight.gpsNeedsRelease {
		return nil, nil
	}
	r, g := p.Flight, p.GPS
	t := &filmGPSTick{Held: r.gpsHeld, Before: r.gpsAxes, RemainderBefore: g.ReplayRemainder()}
	g.AdvanceUI()
	p.Dirty = true
	h := r.gpsHeld
	r.gpsAxes.Advance(flightcontrol.Held{Left: h[0], Right: h[1], Up: h[2], Down: h[3]})
	if r.gpsAxes.X != 0 || r.gpsAxes.Y != 0 {
		t.Handle = true
		t.Input = g.KeyboardInput(r.gpsAxes.X, r.gpsAxes.Y, c.Runtime.Effective().GPSOverviewKeyboardDivisor)
		if _, err := g.Handle(t.Input); err != nil {
			return nil, err
		}
		p.publishGPSPanel()
		r.Navigation[0].ToFlight(r.State)
		p.publishNavigation()
	}
	t.After = r.gpsAxes
	t.RemainderAfter = g.ReplayRemainder()
	return t, nil
}
func (c *FrontEnd) applyFilmGPSTick(want *filmGPSTick) error {
	if want == nil {
		return fmt.Errorf("missing resolved GPS tick")
	}
	if err := want.validate(); err != nil {
		return err
	}
	got, err := c.consumeGPSTick()
	if err != nil {
		return err
	}
	if got == nil || !bytes.Equal(filmJSON(want), filmJSON(got)) {
		return fmt.Errorf("GPS consumed input/filter differs")
	}
	return nil
}
func isFilmReset(raw []byte) bool {
	var b filmBoundary
	return json.Unmarshal(raw, &b) == nil && b.Kind == "reset"
}
