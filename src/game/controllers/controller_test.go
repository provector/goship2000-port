package controllers

import (
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func testDevice() Device {
	return Device{Instance: 0, Backend: "test/raw", GUID: "guid", Name: "Raw stick", Axes: 4, Buttons: 16, Hats: 1}
}
func testConfig() Config {
	c := Defaults()
	d := testDevice()
	c.Selected = "profile"
	c.Profiles[c.Selected] = Profile{ID: c.Selected, Device: d, CollectiveMode: "absolute", Bindings: map[string]Binding{}}
	return c
}
func neutral() State {
	return State{Axes: []float64{0, 0, .8, 0}, Buttons: make([]bool, 16), Hats: []uint8{0}}
}
func TestCaptureFreshThrottleJitterHeldAndDigitalAxis(t *testing.T) {
	s := neutral()
	s.Buttons[0] = true
	c := NewCapture("flight.roll", s)
	for i := 0; i < 3; i++ {
		if _, ok := c.Update(s); ok {
			t.Fatal("held state captured")
		}
	}
	s.Axes[0] = .04
	if _, ok := c.Update(s); ok {
		t.Fatal("jitter captured")
	}
	s.Buttons[0] = false
	c.Update(s)
	s.Axes[0] = 1
	b, ok := c.Update(s)
	if !ok || b.Kind != "axis" || b.Axis != 0 {
		t.Fatal(b, ok)
	}
	c = NewCapture("menu.selector1", s)
	s.Buttons[15] = true
	b, ok = c.Update(s)
	if !ok || b.Control.Index != 15 {
		t.Fatal("higher control lost", b)
	}
	c = NewCapture("flight.collective", s)
	if _, ok = c.Update(s); ok {
		t.Fatal("stationary throttle captured")
	}
	s.Axes[2] = -.4
	b, ok = c.Update(s)
	if !ok || b.Axis != 2 {
		t.Fatal(b, ok)
	}
}
func TestPairWholeTransactionHatAndOpposites(t *testing.T) {
	s := neutral()
	c := NewCapture("flight.yaw", s)
	s.Hats[0] = 8
	if _, ok := c.Update(s); ok {
		t.Fatal("partial pair accepted")
	}
	s.Hats[0] = 8 | 2
	if _, ok := c.Update(s); ok {
		t.Fatal("opposite while held accepted")
	}
	s.Hats[0] = 0
	c.Update(s)
	s.Hats[0] = 2
	b, ok := c.Update(s)
	if !ok || b.Kind != "pair" || b.Negative.Direction != 8 || b.Positive.Direction != 2 {
		t.Fatal(b, ok)
	}
	if b.Value(s, false) != 1 {
		t.Fatal("right")
	}
	s.Hats[0] = 10
	if b.Value(s, false) != 0 {
		t.Fatal("opposing pair not neutral")
	}
	s.Hats[0] = 8
	b.Invert = true
	if b.Value(s, false) != 1 {
		t.Fatal("invert")
	}
}
func TestPreviewFivePercentAbsoluteTravelAndDistinctAxes(t *testing.T) {
	s := neutral()
	b := Binding{Kind: "axis", Axis: 0}
	for _, v := range []float64{-.05, 0, .05} {
		s.Axes[0] = v
		if b.Value(s, false) != 0 {
			t.Fatal(v)
		}
	}
	s.Axes[0] = .051
	if b.Value(s, false) != .051 {
		t.Fatal("outside dead zone")
	}
	s.Axes[0] = .02
	if b.Value(s, true) != .02 {
		t.Fatal("absolute middle dead band")
	}
	b.Invert = true
	if math.Abs(b.Value(s, true)+.02) > 1e-12 {
		t.Fatal("absolute invert")
	}
	s.Axes[1] = -.8
	if (Binding{Kind: "axis", Axis: 1}).Value(s, false) != -.8 || b.Value(s, false) != 0 {
		t.Fatal("axis owners mixed")
	}
}
func TestStableIdentityReorderAmbiguityUnplug(t *testing.T) {
	c := testConfig()
	d := testDevice()
	key := d.Key()
	d.Instance = 42
	d.Path = "/dev/input/js99"
	if d.Key() != key {
		t.Fatal("transient identity stored")
	}
	snap := Snapshot{Devices: []Device{d}, States: map[int32]State{42: neutral()}}
	if got, ok, _ := Resolve(c, snap, -1); !ok || got.Instance != 42 {
		t.Fatal("reorder")
	}
	other := d
	other.Instance = 43
	snap.Devices = append(snap.Devices, other)
	if _, ok, _ := Resolve(c, snap, -1); ok {
		t.Fatal("identical device guessed")
	}
	if got, ok, _ := Resolve(c, snap, 42); !ok || got.Instance != 42 {
		t.Fatal("explicit choice")
	}
	snap = Snapshot{}
	if _, ok, _ := Resolve(c, snap, 42); ok {
		t.Fatal("unplug retained live input")
	}
	c.Selected = ""
	if _, ok, _ := Resolve(c, Snapshot{Devices: []Device{d}}, -1); ok {
		t.Fatal("None consumed input")
	}
}
func TestCollisionContextsAndSharedSelectors(t *testing.T) {
	c := testConfig()
	p := c.Profiles[c.Selected]
	b := Binding{Kind: "control", Control: &Control{Kind: "button", Index: 0}}
	p.Bindings["weapon.fire"] = b
	if len(Conflicts(p, "weapon.chaff", b)) != 1 {
		t.Fatal("same context missed")
	}
	if len(Conflicts(p, "gps.scale", b)) != 0 {
		t.Fatal("different context blocked")
	}
	if len(Conflicts(p, "window.fullscreen", b)) != 1 {
		t.Fatal("global context missed")
	}
	p.Bindings = map[string]Binding{"gps.append": b}
	if len(Conflicts(p, "gps.support_confirm", b)) != 0 {
		t.Fatal("independent modal contexts collide")
	}
	n := 0
	for _, a := range Actions {
		if a.ID == "menu.selector2" {
			n++
		}
	}
	if n != 2 {
		t.Fatal("editor alias not shared", n)
	}
}
func TestControlsPersistenceMissingDiscardAtomicConflictRollback(t *testing.T) {
	path := filepath.Join(t.TempDir(), Filename)
	d, e := Load(path)
	if e != nil || d.Config.Selected != "" {
		t.Fatal(e)
	}
	if _, e = os.Stat(path); !os.IsNotExist(e) {
		t.Fatal("read created controls")
	}
	d.Config = testConfig()
	p := d.Config.Profiles["profile"]
	p.Bindings["flight.roll"] = Binding{Kind: "axis", Axis: 1, Invert: true}
	p.Bindings["menu.selector1"] = Binding{Kind: "control", Control: &Control{Kind: "button", Index: 15}}
	d.Config.Profiles["profile"] = p
	if e = d.Save(); e != nil {
		t.Fatal(e)
	}
	before, _ := os.ReadFile(path)
	again, e := Load(path)
	if e != nil || !again.Config.Profiles["profile"].Bindings["flight.roll"].Invert {
		t.Fatal(e)
	}
	again.Config.Selected = ""
	same, _ := os.ReadFile(path)
	if string(before) != string(same) {
		t.Fatal("draft/discard wrote")
	}
	cp := again.Checkpoint()
	if e = again.Save(); e != nil {
		t.Fatal(e)
	}
	if e = again.Rollback(cp); e != nil {
		t.Fatal(e)
	}
	same, _ = os.ReadFile(path)
	if string(before) != string(same) {
		t.Fatal("rollback changed old bytes")
	}
	if e = os.WriteFile(path, append(before, ' '), 0600); e != nil {
		t.Fatal(e)
	}
	if e = d.Save(); e == nil || !strings.Contains(e.Error(), "externally") {
		t.Fatal("external edit overwritten", e)
	}
}
func TestCorruptNewerReadOnlyAndIndependentProfiles(t *testing.T) {
	for _, raw := range []string{`null`, `{}`, `{"version":5,"profiles":{}}`, `{"version":1,"profiles":null}`, `broken`} {
		p := filepath.Join(t.TempDir(), Filename)
		os.WriteFile(p, []byte(raw), 0600)
		if _, e := Load(p); e == nil {
			t.Fatal("invalid file admitted", raw)
		}
		b, _ := os.ReadFile(p)
		if string(b) != raw {
			t.Fatal("bad controls overwritten")
		}
	}
	path := filepath.Join(t.TempDir(), Filename)
	c := testConfig()
	other := c.Profiles["profile"]
	other.ID = "other"
	other.Device.Serial = "unique2"
	other.Bindings = map[string]Binding{"flight.yaw": {Kind: "axis", Axis: 3}}
	c.Profiles[other.ID] = other
	b, _ := json.Marshal(c)
	os.WriteFile(path, b, 0400)
	d, e := Load(path)
	if e != nil {
		t.Fatal(e)
	}
	d.Config.Selected = "other"
	if e = d.Save(); e == nil {
		t.Fatal("read-only overwritten")
	}
	os.Chmod(path, 0600)
	if e = d.Save(); e != nil {
		t.Fatal(e)
	}
	d, _ = Load(path)
	if len(d.Config.Profiles) != 2 || d.Config.Profiles["other"].Bindings["flight.yaw"].Axis != 3 {
		t.Fatal(d.Config)
	}
}
func TestHatDiagonalAndHeldHatMustCentre(t *testing.T) {
	s := neutral()
	s.Hats[0] = 1
	c := NewCapture("menu.selector1", s)
	s.Hats[0] = 3
	if _, done := c.Update(s); done {
		t.Fatal("held hat sweep captured")
	}
	s.Hats[0] = 0
	c.Update(s)
	s.Hats[0] = 3
	b, done := c.Update(s)
	if !done || b.Control.Direction != 3 || b.Control.String() != "Hat 1 Up-Right" {
		t.Fatal("diagonal not discrete", b, done)
	}
	if !b.Control.Held(s) {
		t.Fatal("diagonal not held")
	}
	s.Hats[0] = 1
	if b.Control.Held(s) {
		t.Fatal("cardinal incorrectly activates diagonal")
	}
}
func TestPairSurvivesCloneSaveReopenAndDeviceReordering(t *testing.T) {
	c := testConfig()
	p := c.Profiles[c.Selected]
	p.Bindings["flight.roll"] = Binding{Kind: "pair", Negative: &Control{Kind: "button", Index: 0}, Positive: &Control{Kind: "hat", Index: 0, Direction: 3}, Invert: true}
	c.Profiles[p.ID] = p
	clone := Clone(c)
	b := clone.Profiles[p.ID].Bindings["flight.roll"]
	if b.Negative == nil || b.Positive == nil || b.Positive.Direction != 3 {
		t.Fatal("pair lost while cloning", b)
	}
	d, e := Load(filepath.Join(t.TempDir(), Filename))
	if e != nil {
		t.Fatal(e)
	}
	d.Config = clone
	if e = d.Save(); e != nil {
		t.Fatal(e)
	}
	again, e := Load(d.Path)
	if e != nil {
		t.Fatal(e)
	}
	b = again.Config.Profiles[p.ID].Bindings["flight.roll"]
	s := neutral()
	s.Hats[0] = 3
	if b.Negative == nil || b.Positive == nil || b.Value(s, false) != -1 {
		t.Fatal("pair failed disk round trip", b)
	}
	dev := p.Device
	dev.Instance = 42
	dev.Path = "/dev/changed"
	if _, ok, _ := Resolve(again.Config, Snapshot{Devices: []Device{dev}}, -1); !ok {
		t.Fatal("reordering lost profile")
	}
}
