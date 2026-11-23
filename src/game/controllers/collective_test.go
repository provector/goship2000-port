package controllers

import (
	"reflect"
	"testing"
)

func TestCollectiveEvidenceAndExplicitPreference(t *testing.T) {
	p := Profile{Device: Device{Backend: "sdl2-raw-v1/linux", Vendor: 0x044f, Product: 0xb68d, Revision: 0x111, Axes: 8}, Bindings: map[string]Binding{"flight.collective": {Kind: "axis", Axis: 2}}}
	if s, _ := CollectiveSource(p); s != "slider" || CollectiveMode(p) != "absolute" {
		t.Fatal(s, CollectiveMode(p))
	}
	for _, mode := range []string{"absolute", "relative", "relative_height"} {
		p.CollectiveMode = mode
		for _, override := range []string{"", "centered", "slider"} {
			p.CollectiveSource = override
			if CollectiveMode(p) != mode {
				t.Fatal("explicit mode lost", mode, override)
			}
		}
	}
	p.CollectiveMode = ""
	p.CollectiveSource = ""
	p.Bindings["flight.collective"] = Binding{Kind: "axis", Axis: 0}
	if s, _ := CollectiveSource(p); s != "centered" || CollectiveMode(p) != "relative_height" {
		t.Fatal("mapped cyclic is not throttle", s)
	}
	p.Device.Backend = "sdl2-raw-v1/windows"
	p.Bindings["flight.collective"] = Binding{Kind: "axis", Axis: 2}
	if s, _ := CollectiveSource(p); s != "centered" {
		t.Fatal("Linux index evidence crossed backend")
	}
}
func TestOldProfileReadOnlyExplicitMode(t *testing.T) {
	c := testConfig()
	c.Version = 3
	p := c.Profiles[c.Selected]
	p.CollectiveMode = "relative"
	p.CollectiveSource = "slider"
	c.Profiles[c.Selected] = p
	before := Clone(c)
	d := t.TempDir() + "/controls.cfg"
	doc, _ := Load(d)
	doc.Config = c
	if e := doc.Save(); e != nil {
		t.Fatal(e)
	}
	loaded, e := Load(d)
	if e != nil || !reflect.DeepEqual(loaded.Config, before) {
		t.Fatal(e, loaded.Config, before)
	}
	if CollectiveMode(loaded.Config.Profiles[c.Selected]) != "relative" {
		t.Fatal("explicit changed")
	}
}
func TestAutomaticSourceAndModeStableAcrossFocusAndReplug(t *testing.T) {
	c, b := liveFixture()
	p := c.Profiles[c.Selected]
	p.CollectiveMode = "relative"
	p.CollectiveSource = "slider"
	c.Version = 3
	c.Profiles[c.Selected] = p
	l := NewLive(c, b)
	for _, focus := range []bool{true, false, true} {
		s := l.Poll("sortie", focus)
		if s.Source != "slider" || s.Mode != "relative" || !s.Slider {
			t.Fatal("policy changed on focus", s)
		}
	}
	b.s.Devices = nil
	b.s.States = nil
	l.Poll("sortie", true)
	b2 := b
	_, again := liveFixture()
	b2.s = again.s
	s := l.Poll("sortie", true)
	if s.Mode != "relative" || s.Source != "slider" {
		t.Fatal("replug mode changed", s)
	}
}
