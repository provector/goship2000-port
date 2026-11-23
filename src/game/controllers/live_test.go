package controllers

import "testing"

type liveBackend struct {
	s      Snapshot
	closed bool
}

func (b *liveBackend) Poll() (Snapshot, error) { return b.s, nil }
func (b *liveBackend) Close()                  { b.closed = true }
func liveFixture() (Config, *liveBackend) {
	d := Device{Instance: 42, Backend: "test", GUID: "abc", Name: "raw", Axes: 4, Buttons: 8, Hats: 1}
	p := Profile{ID: "saved", Device: d, CollectiveMode: "absolute", Bindings: map[string]Binding{"flight.pitch": {Kind: "axis", Axis: 0, Invert: true}, "flight.roll": {Kind: "axis", Axis: 1}, "flight.collective": {Kind: "axis", Axis: 2}, "flight.yaw": {Kind: "axis", Axis: 3}, "menu.selector1": {Kind: "control", Control: &Control{Kind: "button", Index: 0}}}, AxisButtons: map[string]Binding{"flight.roll": {Kind: "pair", Negative: &Control{Kind: "axis_endpoint", Index: 1, Sign: -1}, Positive: &Control{Kind: "axis_endpoint", Index: 1, Sign: 1}}}}
	c := Config{Version: 2, Selected: p.ID, Profiles: map[string]Profile{p.ID: p}}
	b := &liveBackend{s: Snapshot{Devices: []Device{d}, States: map[int32]State{42: {Axes: []float64{0, 0, -1, 0}, Buttons: make([]bool, 8), Hats: []uint8{0}}}}}
	return c, b
}
func TestReadOnlyReorderAxesAndRearm(t *testing.T) {
	c, b := liveFixture()
	l := NewLive(c, b)
	defer l.Close()
	s := l.Poll("office", true)
	if !s.Connected || s.Lever != 0 {
		t.Fatal(s)
	}
	raw := b.s.States[42]
	raw.Axes[0] = .5
	raw.Axes[1] = 1
	raw.Buttons[0] = true
	b.s.States[42] = raw
	s = l.Poll("office", true)
	if s.Axes[0] != -64 || s.Axes[1] != 127 || !s.Down["menu.selector1"] || !s.Directions[1][1] {
		t.Fatal(s)
	}
	if s = l.Poll("gps", true); s.Down["menu.selector1"] || s.Axes[1] != 0 {
		t.Fatal("held across context", s)
	}
	raw.Buttons[0] = false
	raw.Axes[1] = 0
	b.s.States[42] = raw
	l.Poll("gps", true)
	raw.Buttons[0] = true
	raw.Axes[1] = -1
	b.s.States[42] = raw
	if s = l.Poll("gps", true); !s.Down["menu.selector1"] || !s.Directions[1][0] {
		t.Fatal("rearm", s)
	}
	l.Poll("gps", false)
	if s = l.Poll("gps", true); s.Down["menu.selector1"] || s.Axes[1] != 0 {
		t.Fatal("focus", s)
	}
	b.s.Devices = nil
	b.s.States = nil
	if s = l.Poll("gps", true); s.Connected || s.Down["menu.selector1"] {
		t.Fatal("disconnect", s)
	}
	d := c.Profiles["saved"].Device
	d.Instance = 99
	b.s = Snapshot{Devices: []Device{d}, States: map[int32]State{99: raw}}
	if s = l.Poll("gps", true); !s.Connected || s.Down["menu.selector1"] {
		t.Fatal("reattach", s)
	}
	if c.Profiles["saved"].Device.Instance != 42 {
		t.Fatal("mutated config")
	}
}
func TestPairInversionHiddenAndNone(t *testing.T) {
	c, b := liveFixture()
	p := c.Profiles["saved"]
	p.Bindings["flight.roll"] = Binding{Kind: "pair", Negative: &Control{Kind: "button", Index: 2}, Positive: &Control{Kind: "button", Index: 3}, Invert: true}
	p.AxisButtons = nil
	p.Bindings["editor.pick"] = Binding{Kind: "control", Control: &Control{Kind: "button", Index: 4}}
	c.Profiles["saved"] = p
	l := NewLive(c, b)
	l.Poll("flight", true)
	raw := b.s.States[42]
	raw.Buttons[2], raw.Buttons[4] = true, true
	b.s.States[42] = raw
	s := l.Poll("flight", true)
	if !s.Directions[1][1] || s.Down["editor.pick"] {
		t.Fatal(s)
	}
	c.Selected = ""
	if s = NewLive(c, b).Poll("flight", true); s.Connected {
		t.Fatal(s)
	}
}
