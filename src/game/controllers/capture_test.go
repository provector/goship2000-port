package controllers

import "testing"

func TestFreshDiscreteEdgeWhilePriorAxisRemainsDeflected(t *testing.T) {
	for run := 0; run < 200; run++ {
		for _, position := range []float64{-1, 0, .01, .8, 1} {
			st := State{Axes: []float64{position}, Buttons: []bool{false, false}}
			c := NewCapture("menu.selector1", st)
			c.SuppressAxis(0, 0)
			st.Buttons[0] = true
			b, ok := c.Update(st)
			if !ok || b.Control.Index != 0 {
				t.Fatal(run, position, "fresh first press lost")
			}
			// Held across opening must release, even if the axis never does.
			c = NewCapture("menu.selector1", st)
			if _, ok = c.Update(st); ok {
				t.Fatal("held accepted")
			}
			st.Buttons[0] = false
			c.Update(st)
			st.Buttons[0] = true
			if _, ok = c.Update(st); !ok {
				t.Fatal("fresh edge after release lost")
			}
		}
	}
}

func TestAxisCompletionSuppressesOnlyThatAxis(t *testing.T) {
	st := State{Axes: []float64{1, 0}, Buttons: []bool{false}}
	c := NewCapture("flight.yaw", st)
	c.SuppressAxis(0, 0)
	st.Axes[0] = .1
	if _, ok := c.Update(st); ok {
		t.Fatal("return movement rebound next action")
	}
	st.Axes[1] = .7
	b, ok := c.Update(st)
	if !ok || b.Axis != 1 {
		t.Fatal("independent yaw blocked", b)
	}
}

func TestAxisBaselineInversionAndLegacyCompatibility(t *testing.T) {
	for _, id := range []string{"flight.pitch", "flight.roll", "flight.yaw", "flight.collective"} {
		p := Profile{AxisConvention: "native-v1", Bindings: map[string]Binding{id: {Kind: "axis"}}}
		for _, v := range []float64{-1, -.5, 0, .5, 1} {
			st := State{Axes: []float64{v}}
			want := v
			if id == "flight.collective" {
				want = -v
			}
			if got := AxisValue(p, id, st, true); got != want {
				t.Fatal(id, v, got)
			}
			b := p.Bindings[id]
			b.Invert = true
			p.Bindings[id] = b
			if got := AxisValue(p, id, st, true); got != -want {
				t.Fatal("double inversion", id, v, got)
			}
			p.AxisConvention = "" // Retained Q3 effective input is unchanged.
			if got := AxisValue(p, id, st, true); got != -v {
				t.Fatal("legacy changed")
			}
			b.Invert = false
			p.Bindings[id] = b
			p.AxisConvention = "native-v1"
		}
	}
}
