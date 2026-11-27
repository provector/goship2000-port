package controllers

import "fmt"

// Stable SETUP directions shared with the Q3 game input adapter.
type AxisDirection struct {
	ID, Axis, Label, Keyboard, Reference string
	Positive                             bool
}

var AxisDirections = []AxisDirection{
	{"flight.pitch_forward", "flight.pitch", "Pitch forward", "Up Arrow", "PC base printed26/PDF29 cyclic", false},
	{"flight.pitch_back", "flight.pitch", "Pitch back", "Down Arrow", "PC base printed26/PDF29 cyclic", true},
	{"flight.roll_left", "flight.roll", "Roll/Bank left", "Left Arrow", "PC base printed26/PDF29 cyclic", false},
	{"flight.roll_right", "flight.roll", "Roll/Bank right", "Right Arrow", "PC base printed26/PDF29 cyclic", true},
	{"flight.collective_raise", "flight.collective", "Collective raise", "+", "PC base printed26/PDF29 collective (= / Shift+=); + label requested Q2.2", true},
	{"flight.collective_lower", "flight.collective", "Collective lower", "-", "PC base printed26/PDF29 collective", false},
	{"flight.yaw_left", "flight.yaw", "Yaw/Rotor left", "Alt+Left Arrow", "Port extension Q2.2; Q3 native flight dispatch; not a DOS default", false},
	{"flight.yaw_right", "flight.yaw", "Yaw/Rotor right", "Alt+Right Arrow", "Port extension Q2.2; Q3 native flight dispatch; not a DOS default", true},
}

func DirectionByID(id string) (AxisDirection, bool) {
	for _, d := range AxisDirections {
		if d.ID == id {
			return d, true
		}
	}
	return AxisDirection{}, false
}
func directionAction(d AxisDirection) Action {
	status := "supported"
	label := d.Label
	if d.Axis == "flight.yaw" {
		status = "port extension"
		label += " (port extension)"
	}
	return Action{ID: d.ID, Category: "Flight", Label: label, Keyboard: d.Keyboard, Reference: d.Reference, Status: status, Contexts: []string{"flight"}}
}
func ButtonPair(p Profile, axis string) Binding {
	if b, ok := p.AxisButtons[axis]; ok {
		return b
	}
	if b := p.Bindings[axis]; b.Kind == "pair" {
		return b
	}
	return Binding{Kind: "pair"}
}
func setDirection(p *Profile, d AxisDirection, b Binding) {
	pair := ButtonPair(*p, d.Axis)
	pair.Kind = "pair"
	positive := d.Positive
	if pair.Invert {
		positive = !positive
	}
	pair.Invert = false
	if positive {
		pair.Positive = b.Control
	} else {
		pair.Negative = b.Control
	}
	if old := p.Bindings[d.Axis]; old.Kind == "pair" {
		pair.Invert = old.Invert
		if pair.Negative == nil && pair.Positive == nil {
			p.Bindings[d.Axis] = Binding{Invert: old.Invert}
		} else {
			p.Bindings[d.Axis] = pair
		}
	} else {
		if p.AxisButtons == nil {
			p.AxisButtons = map[string]Binding{}
		}
		if pair.Negative == nil && pair.Positive == nil {
			delete(p.AxisButtons, d.Axis)
		} else {
			p.AxisButtons[d.Axis] = pair
		}
	}
}
func partialPairValid(b Binding) error {
	if b.Kind != "pair" || b.Axis != 0 || b.Control != nil || b.Negative == nil && b.Positive == nil {
		return fmt.Errorf("pair needs at least one direction")
	}
	if b.Negative != nil {
		if e := b.Negative.Validate(); e != nil {
			return e
		}
	}
	if b.Positive != nil {
		if e := b.Positive.Validate(); e != nil {
			return e
		}
	}
	if b.Negative != nil && b.Positive != nil && *b.Negative == *b.Positive {
		return fmt.Errorf("opposing controls must differ")
	}
	return nil
}
func RequiresVersion(c Config) int {
	v := c.Version
	for _, p := range c.Profiles {
		if p.CollectiveMode == "" {
			v = max(v, 4)
		}
		if p.AxisConvention != "" || p.CollectiveSource != "" || p.CollectiveMode == "relative_height" {
			v = max(v, 3)
		}
		if len(p.AxisButtons) > 0 {
			v = max(v, 2)
		}
		for _, b := range p.Bindings {
			if b.Kind == "pair" && (b.Negative == nil || b.Positive == nil) {
				v = max(v, 2)
			}
			for _, x := range b.Uses() {
				if x.Kind == "axis_endpoint" {
					v = max(v, 2)
				}
			}
		}
	}
	return v
}
