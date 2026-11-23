package setupui

import (
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
	"gunship2000-assets/game/controllers"
	"reflect"
	"strings"
	"testing"
	"time"
)

func frame(t *testing.T, u *UI, w, h int) tcell.SimulationScreen {
	t.Helper()
	s := tcell.NewSimulationScreen("UTF-8")
	if e := s.Init(); e != nil {
		t.Fatal(e)
	}
	s.SetSize(w, h)
	u.App.GetBeforeDrawFunc()(s)
	u.Main.SetRect(0, 0, w, h)
	u.Main.Draw(s)
	return s
}
func TestWidthActionsSpaceAndSevenRows(t *testing.T) {
	u := controllerUIForController(t)
	c := u.controllers
	u.openControllerEditor(false)
	if len(controllers.Basic) != 7 || controllers.Basic[6] != "weapon.weapon" {
		t.Fatal(controllers.Basic)
	}
	for _, row := range []int{2, 3, 12} {
		if c.table.GetCell(row, 0).NotSelectable || strings.TrimSpace(c.table.GetCell(row, 1).Text) != "" {
			t.Fatal("action in wrong column", row)
		}
	}
	for _, w := range []int{76, 80, 132, 76} {
		s := frame(t, u, w, 24)
		_, _, inner, _ := c.table.GetInnerRect()
		net := inner - 2
		a, b, d := c.table.GetCell(0, 0).MaxWidth, c.table.GetCell(0, 1).MaxWidth, c.table.GetCell(0, 2).MaxWidth
		if a+b+d != net || abs(a*2-net) > 1 || abs(b*4-net) > 3 || abs(d*4-net) > 3 {
			t.Fatal("column ratios", w, a, b, d, inner)
		}
		s.Fini()
	}
	c.table.Select(4, 0)
	space := tcell.NewEventKey(tcell.KeyRune, ' ', 0)
	u.App.GetInputCapture()(space)
	if b, _ := controllers.EffectiveBinding(*c.draft, "flight.pitch"); !b.Invert {
		t.Fatal("action-column Space missed")
	}
	if c.table.GetCell(4, 2).Text != "On" || !strings.Contains(u.Help.GetText(false), "Enter: bind   Space: invert") {
		t.Fatal("invert UI")
	}
	c.returnRow, c.returnCol = 4, 0
	u.beginCapture("flight.pitch")
	u.App.GetInputCapture()(space)
	if b, _ := controllers.EffectiveBinding(*c.draft, "flight.pitch"); b.Kind != "" || b.Invert {
		t.Fatal("popup Space leaked to inversion")
	}
	for row := 9; row <= 11; row++ {
		if c.table.GetCell(row, 2).Text != "" || !c.table.GetCell(row, 2).NotSelectable {
			t.Fatal("invert on nonaxis")
		}
	}
	c.wizard = true
	c.step = 0
	u.beginCapture(controllers.Basic[0])
	for i := 0; i < 7; i++ {
		if c.captureID != controllers.Basic[i] {
			t.Fatal("wizard order", i, c.captureID)
		}
		c.keyUntil = time.Time{}
		u.App.GetInputCapture()(tcell.NewEventKey(tcell.KeyEscape, 0, 0))
	}
	u.App.GetInputCapture()(tcell.NewEventKey(tcell.KeyEscape, 0, 0))
	if c.draft == nil {
		t.Fatal("held final wizard Escape discarded draft")
	}
	if c.wizard || c.capture != nil {
		t.Fatal("wizard did not finish seventh")
	}
	for _, v := range Catalogue("controls") {
		if v.ID == "calibration" {
			t.Fatal("Calibration still visible")
		}
	}
}
func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}
func TestAdvancedOneListScrollAndSharedAliases(t *testing.T) {
	u := controllerUIForController(t)
	c := u.controllers
	u.openControllerEditor(true)
	if len(c.actionRows) != len(controllers.AdvancedActions())+len(controllers.AxisDirections) {
		t.Fatal("catalogue row lost", len(c.actionRows))
	}
	seen := map[string]bool{}
	dividers := 0
	for row := 0; row < c.table.GetRowCount(); row++ {
		if strings.HasPrefix(c.table.GetCell(row, 0).Text, "-------") {
			dividers++
			if !c.table.GetCell(row, 0).NotSelectable {
				t.Fatal("selectable divider")
			}
		}
	}
	for _, a := range c.actionRows {
		if seen[a.ID] {
			t.Fatal("duplicate logical row", a.ID)
		}
		seen[a.ID] = true
	}
	if dividers < 1 || !seen["menu.selector1"] || !seen["menu.selector2"] || seen["weapon.fire"] || seen["weapon.target"] {
		t.Fatal("consolidation")
	}
	s := frame(t, u, 80, 25)
	defer s.Fini()
	for _, key := range []tcell.Key{tcell.KeyDown, tcell.KeyPgDn, tcell.KeyPgDn, tcell.KeyPgUp, tcell.KeyEnd, tcell.KeyHome} {
		c.table.InputHandler()(tcell.NewEventKey(key, 0, 0), func(_ tview.Primitive) {})
		c.table.Draw(s)
		r, col := c.table.GetSelection()
		if c.table.GetCell(r, col).NotSelectable {
			t.Fatal("navigation landed on divider", key, r, col)
		}
	}
	var row int
	for r, a := range c.actionRows {
		if a.ID == "weapon.weapon" {
			row = r
		}
	}
	c.table.Select(row, 2)
	c.table.Draw(s)
	before, _ := c.table.GetOffset()
	s.SetSize(128, 25)
	u.Main.SetRect(0, 0, 128, 25)
	u.Main.Draw(s)
	r, col := c.table.GetSelection()
	after, _ := c.table.GetOffset()
	if r != row || col != 2 || before != after {
		t.Fatal("resize moved selection/scroll", r, col, before, after)
	}
	// Different Q2 bindings are retained until explicitly resolved in a draft.
	oldMenu := controllers.Binding{Kind: "control", Control: &controllers.Control{Kind: "button", Index: 1}}
	oldFire := controllers.Binding{Kind: "control", Control: &controllers.Control{Kind: "button", Index: 2}}
	c.draft.Bindings["menu.selector1"] = oldMenu
	c.draft.Bindings["weapon.fire"] = oldFire
	snapshot := controllers.Clone(controllers.Config{Version: 1, Profiles: map[string]controllers.Profile{c.draft.ID: *c.draft}})
	c.returnRow, c.returnCol = row, 2
	u.beginCapture("menu.selector1")
	if !u.popup || c.pending == nil {
		t.Fatal("legacy ambiguity silently chosen")
	}
	u.App.GetInputCapture()(tcell.NewEventKey(tcell.KeyEscape, 0, 0))
	if !reflect.DeepEqual(c.draft.Bindings, snapshot.Profiles[c.draft.ID].Bindings) {
		t.Fatal("legacy cancel mutated")
	}
	u.beginCapture("menu.selector1")
	// Drive the actual modal buttons: move from Use menu to Use flight.
	focus := func(p tview.Primitive) { u.App.SetFocus(p) }
	u.App.GetFocus().InputHandler()(tcell.NewEventKey(tcell.KeyRight, 0, 0), focus)
	u.App.GetFocus().InputHandler()(tcell.NewEventKey(tcell.KeyEnter, 0, 0), focus)
	if b, conflict := controllers.EffectiveBinding(*c.draft, "menu.selector1"); conflict || !reflect.DeepEqual(b, oldFire) {
		t.Fatal("selector not shared")
	}
	if _, ok := c.draft.Bindings["weapon.fire"]; ok {
		t.Fatal("edited alias retained divergent storage")
	}
	u.finishControllerEditor(true)
	u.openControllerEditor(false)
	if b, _ := controllers.EffectiveBinding(*c.draft, "menu.selector1"); !reflect.DeepEqual(b, oldFire) {
		t.Fatal("Basic differs from Advanced")
	}
}
func TestHatAllDirectionsNeutralSwitchUnplugAndBands(t *testing.T) {
	u := controllerUIForController(t)
	c := u.controllers
	u.openControllerEditor(false)
	positions := map[uint8][2]int{1: {43, 4}, 2: {46, 6}, 4: {43, 8}, 8: {40, 6}, 3: {45, 5}, 6: {45, 7}, 9: {41, 5}, 12: {41, 7}}
	for dir, at := range positions {
		ss := c.snapshot
		ss.States[0].Hats[0] = dir
		u.applyControllerSnapshot(ss)
		s := frame(t, u, 76, 24)
		for d, p := range positions {
			_, _, style, _ := s.GetContent(p[0], p[1])
			_, bg, _ := style.Decompose()
			if (bg.Hex() == yellow.Hex()) != (d == dir) {
				t.Fatal("hat direction highlight", dir, d)
			}
		}
		ch, _, _, _ := s.GetContent(at[0], at[1])
		if ch == ' ' {
			t.Fatal("hat absent")
		}
		s.Fini()
	}
	ss := c.snapshot
	ss.States[0].Hats[0] = 0
	u.applyControllerSnapshot(ss)
	s := frame(t, u, 76, 24)
	ch, _, _, _ := s.GetContent(43, 6)
	if ch != '●' {
		t.Fatal("neutral hat missing")
	}
	s.Fini()
	c.wizard = true
	c.step = 6
	c.captureID = "weapon.weapon"
	c.capture = controllers.NewCapture(c.captureID, ss.States[0])
	c.captureView = newCapturePanel(u)
	c.captureView.SetRect(3, 10, 68, 12)
	s = tcell.NewSimulationScreen("UTF-8")
	s.Init()
	s.SetSize(76, 24)
	c.captureView.Draw(s)
	for x := 4; x < 70; x++ {
		_, _, style, _ := s.GetContent(x, 11)
		_, bg, _ := style.Decompose()
		if bg.Hex() != amber.Hex() {
			t.Fatal("step band padding", x)
		}
		_, _, style, _ = s.GetContent(x, 12)
		_, bg, _ = style.Decompose()
		if bg.Hex() != blue.Hex() {
			t.Fatal("assignment band padding", x)
		}
	}
	s.Fini()
	u.applyControllerSnapshot(controllers.Snapshot{})
	s = frame(t, u, 76, 24)
	ch, _, _, _ = s.GetContent(42, 3)
	if ch == 'H' {
		t.Fatal("hat remains after unplug")
	}
	s.Fini()
	// Endpoint-valued axes alone never produce a hat widget.
	d := controllers.Device{Instance: 7, Backend: "test/raw", GUID: "digital", Name: "Digital axis stick", Axes: 3, Buttons: 10, Hats: 0}
	ss = controllers.Snapshot{Devices: []controllers.Device{d}, States: map[int32]controllers.State{7: {Axes: []float64{1, -1, 0}, Buttons: make([]bool, 10)}}}
	u.applyControllerSnapshot(ss)
	u.selectDevice(d, "")
	s = frame(t, u, 80, 25)
	ch, _, _, _ = s.GetContent(42, 3)
	if ch == 'H' {
		t.Fatal("digital axis became hat")
	}
	s.Fini()
}

func TestClippedBindingKeyboardAndLongDeviceTooltips(t *testing.T) {
	u := controllerUIForController(t)
	c := u.controllers
	p := c.session.Profiles[c.session.Selected]
	p.Device.Name = strings.Repeat("Long device name ", 6)
	p.Device.Buttons = 512
	p.Device.Hats = 16
	c.session.Profiles[p.ID] = p
	d := p.Device
	d.Instance = 0
	ss := c.snapshot
	ss.Devices = []controllers.Device{d}
	u.applyControllerSnapshot(ss)
	u.open("controls")
	u.Table.Select(1, 0)
	if !strings.Contains(u.Help.GetText(false), p.Device.Name) {
		t.Fatal("device name missing from help")
	}
	u.openControllerEditor(true)
	b := controllers.Binding{Kind: "pair", Negative: &controllers.Control{Kind: "button", Index: 511}, Positive: &controllers.Control{Kind: "hat", Index: 15, Direction: 12}}
	controllers.SetBinding(c.draft, "flight.roll", b)
	var row, selector int
	for r, a := range c.actionRows {
		if a.ID == "flight.roll" {
			row = r
		}
		if a.ID == "menu.selector2" {
			selector = r
		}
	}
	c.table.Select(row, 2)
	if !strings.Contains(u.Help.GetText(false), b.String()) {
		t.Fatal("clipped pair not readable")
	}
	c.table.Select(selector, 0)
	if !strings.Contains(u.Help.GetText(false), "Space (menu/editor); Backspace (flight)") {
		t.Fatal("context keyboard aliases missing")
	}
	c.wizard = true
	c.step = 1
	c.captureID = "flight.roll"
	c.capture = controllers.NewCapture(c.captureID, c.snapshot.States[0])
	v := newCapturePanel(u)
	v.SetRect(3, 10, 68, 12)
	screen := tcell.NewSimulationScreen("UTF-8")
	screen.Init()
	defer screen.Fini()
	screen.SetSize(76, 24)
	v.Draw(screen)
	var line strings.Builder
	for x := 4; x < 70; x++ {
		ch, _, _, _ := screen.GetContent(x, 12)
		line.WriteRune(ch)
	}
	if strings.TrimSpace(line.String()) != "Assigned: "+b.String() {
		t.Fatal("assigned band lost long control", line.String())
	}
}
