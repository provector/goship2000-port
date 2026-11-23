package controllers

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestDirectionsEightButtonsAndIndependentAnalogRoundtrip(t *testing.T) {
	c := testConfig()
	p := c.Profiles[c.Selected]
	for i, axis := range Basic[:4] {
		p.Bindings[axis] = Binding{Kind: "axis", Axis: i, Invert: i%2 == 0}
	}
	analog := Clone(Config{Version: 2, Profiles: map[string]Profile{p.ID: p}}).Profiles[p.ID].Bindings
	for i, d := range AxisDirections {
		state := neutral()
		capture := NewCapture(d.ID, state)
		state.Buttons[i] = true
		binding, done := capture.Update(state)
		if !done || binding.Control.Kind != "button" {
			t.Fatal("direction button capture", d.ID)
		}
		SetBinding(&p, d.ID, binding)
	}
	for i, d := range AxisDirections {
		b, _ := EffectiveBinding(p, d.ID)
		if b.Control == nil || b.Control.Index != i {
			t.Fatal(d.ID, b)
		}
	}
	if !reflect.DeepEqual(p.Bindings, analog) {
		t.Fatal("directions replaced analog assignments")
	}
	c.Profiles[p.ID] = p
	c.Version = RequiresVersion(c)
	doc, e := Load(filepath.Join(t.TempDir(), Filename))
	if e != nil {
		t.Fatal(e)
	}
	doc.Config = c
	if e = doc.Save(); e != nil {
		t.Fatal(e)
	}
	again, e := Load(doc.Path)
	if e != nil {
		t.Fatal(e)
	}
	p = again.Config.Profiles[p.ID]
	SetBinding(&p, "flight.pitch_forward", Binding{})
	if b, _ := EffectiveBinding(p, "flight.pitch_back"); b.Control == nil || b.Control.Index != 1 {
		t.Fatal("clear lost opposite")
	}
	if !reflect.DeepEqual(p.Bindings, analog) || p.AxisButtons["flight.roll"].Negative == nil {
		t.Fatal("clear changed another axis")
	}
}
func TestEndpointFreshNeutralAndPartialExistingPair(t *testing.T) {
	s := neutral()
	s.Axes[0] = 0
	c := NewCapture("flight.roll_left", s)
	s.Axes[0] = -1
	b, done := c.Update(s)
	if !done || b.Control.Kind != "axis_endpoint" || b.Control.Sign != -1 {
		t.Fatal("negative endpoint", b, done)
	}
	next := NewCapture("flight.roll_right", s)
	s.Axes[0] = 1
	if _, done = next.Update(s); done {
		t.Fatal("opposite captured without fresh neutral")
	}
	s.Axes[0] = 0
	next.Update(s)
	s.Axes[0] = 1
	positive, done := next.Update(s)
	if !done || positive.Control.Sign != 1 {
		t.Fatal("positive endpoint")
	}
	cfg := testConfig()
	p := cfg.Profiles[cfg.Selected]
	p.Bindings["flight.roll"] = Binding{Kind: "pair", Negative: &Control{Kind: "button", Index: 0}, Positive: &Control{Kind: "button", Index: 1}, Invert: true}
	SetBinding(&p, "flight.roll_left", b)
	SetBinding(&p, "flight.roll_right", positive)
	SetBinding(&p, "flight.roll_left", Binding{})
	pair := p.Bindings["flight.roll"]
	if pair.Negative == nil || pair.Negative.Sign != 1 || pair.Positive != nil || !pair.Invert {
		t.Fatal("partial pair changed opposite/invert", pair)
	}
	if pair.Value(s, false) != 1 {
		t.Fatal("partial pair no longer usable")
	}
	cfg.Version = 2
	cfg.Profiles[p.ID] = p
	if e := cfg.Validate(); e != nil {
		t.Fatal(e)
	}
	doc, _ := Load(filepath.Join(t.TempDir(), Filename))
	doc.Config = cfg
	if e := doc.Save(); e != nil {
		t.Fatal(e)
	}
	again, e := Load(doc.Path)
	if e != nil {
		t.Fatal(e)
	}
	q := again.Config.Profiles[p.ID]
	if q.Bindings["flight.roll"].Negative.Sign != 1 {
		t.Fatal("endpoint lost on disk")
	}
	cfg.Version = 1
	if e := cfg.Validate(); e == nil {
		t.Fatal("new data falsely labelled version1")
	}
}
func TestHiddenAndV1NoMigrationOnUnrelatedSave(t *testing.T) {
	cfg := testConfig()
	cfg.Version = 1
	p := cfg.Profiles[cfg.Selected]
	p.Bindings["editor.pick"] = Binding{Kind: "control", Control: &Control{Kind: "button", Index: 13}}
	cfg.Profiles[p.ID] = p
	a, _ := MappingAction("editor.pick")
	if ok, _ := AdvancedEligibility(a); ok {
		t.Fatal("unkeyed action visible")
	}
	doc, _ := Load(filepath.Join(t.TempDir(), Filename))
	doc.Config = cfg
	if e := doc.Save(); e != nil {
		t.Fatal(e)
	}
	before, _ := os.ReadFile(doc.Path)
	again, e := Load(doc.Path)
	if e != nil {
		t.Fatal(e)
	}
	if again.Config.Version != 1 {
		t.Fatal("read migrated version")
	}
	if e = again.Save(); e != nil {
		t.Fatal(e)
	}
	after, _ := os.ReadFile(doc.Path)
	if string(before) != string(after) {
		t.Fatal("unrelated save modified hidden binding/version")
	}
	for _, id := range []string{"flight.rotor", "menu.selector2", "manual.detail", "flight.yaw_left", "flight.yaw_right"} {
		a, _ := MappingAction(id)
		if ok, _ := AdvancedEligibility(a); !ok {
			t.Fatal("documented/authorized action hidden", id)
		}
	}
	a, _ = MappingAction("window.fullscreen")
	if ok, _ := AdvancedEligibility(a); ok {
		t.Fatal("native chord substituted for original reference")
	}
}

func TestExistingInvertedPairKeepsSemanticDirectionsWhenAnalogAdded(t *testing.T) {
	cfg := testConfig()
	p := cfg.Profiles[cfg.Selected]
	left := Control{Kind: "button", Index: 0}
	right := Control{Kind: "button", Index: 1}
	p.Bindings["flight.roll"] = Binding{Kind: "pair", Negative: &left, Positive: &right, Invert: true}
	b, _ := EffectiveBinding(p, "flight.roll_left")
	if b.Control.Index != 1 {
		t.Fatal("inverted legacy left labelled as right")
	}
	SetBinding(&p, "flight.roll", Binding{Kind: "axis", Axis: 0, Invert: true})
	if p.Bindings["flight.roll"].Kind != "axis" || !p.Bindings["flight.roll"].Invert {
		t.Fatal("analog changed")
	}
	b, _ = EffectiveBinding(p, "flight.roll_left")
	if b.Control == nil || b.Control.Index != 1 {
		t.Fatal("pair semantics lost moving beside analog")
	}
	SetBinding(&p, "flight.roll_left", Binding{})
	b, _ = EffectiveBinding(p, "flight.roll_right")
	if b.Control == nil || b.Control.Index != 0 {
		t.Fatal("opposite lost")
	}
}

func TestBothEndpointsReopenBeforeAndAfterSingleClear(t *testing.T) {
	cfg := testConfig()
	p := cfg.Profiles[cfg.Selected]
	p.Bindings["flight.roll"] = Binding{Kind: "axis", Axis: 2, Invert: true}
	for _, sign := range []int8{-1, 1} {
		s := neutral()
		s.Axes[0] = 0
		id := "flight.roll_left"
		if sign > 0 {
			id = "flight.roll_right"
		}
		capture := NewCapture(id, s)
		s.Axes[0] = float64(sign)
		b, done := capture.Update(s)
		if !done {
			t.Fatal("endpoint not captured")
		}
		SetBinding(&p, id, b)
	}
	cfg.Profiles[p.ID] = p
	cfg.Version = RequiresVersion(cfg)
	doc, _ := Load(filepath.Join(t.TempDir(), Filename))
	doc.Config = cfg
	if e := doc.Save(); e != nil {
		t.Fatal(e)
	}
	read, e := Load(doc.Path)
	if e != nil {
		t.Fatal(e)
	}
	p = read.Config.Profiles[p.ID]
	for _, d := range []struct {
		id   string
		sign int8
	}{{"flight.roll_left", -1}, {"flight.roll_right", 1}} {
		b, _ := EffectiveBinding(p, d.id)
		if b.Control == nil || b.Control.Index != 0 || b.Control.Sign != d.sign {
			t.Fatal("opposing endpoint lost", d.id)
		}
	}
	SetBinding(&p, "flight.roll_left", Binding{})
	read.Config.Profiles[p.ID] = p
	if e = read.Save(); e != nil {
		t.Fatal(e)
	}
	read, e = Load(doc.Path)
	if e != nil {
		t.Fatal(e)
	}
	p = read.Config.Profiles[p.ID]
	right, _ := EffectiveBinding(p, "flight.roll_right")
	left, _ := EffectiveBinding(p, "flight.roll_left")
	if left.Kind != "" || right.Control == nil || right.Control.Sign != 1 || p.Bindings["flight.roll"].Axis != 2 || !p.Bindings["flight.roll"].Invert {
		t.Fatal("single clear changed opposite/analog")
	}
}
