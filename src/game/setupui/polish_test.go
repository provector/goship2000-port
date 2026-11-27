package setupui

import (
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
	"gunship2000-assets/game/controllers"
	"reflect"
	"strings"
	"testing"
)

func TestHeaderRuleCategoryFullWidthAndSelection(t *testing.T) {
	u := controllerUIForController(t)
	c := u.controllers
	for _, advanced := range []bool{false, true} {
		u.openControllerEditor(advanced)
		for _, width := range []int{76, 80, 124, 76} {
			s := frame(t, u, width, 24)
			x, y, w, _ := c.table.GetInnerRect()
			left, _, _, _ := s.GetContent(x-1, y+1)
			right, _, _, _ := s.GetContent(x+w, y+1)
			if left != '╟' || right != '╢' {
				t.Fatal("rule ends", left, right)
			}
			for i := 0; i < w; i++ {
				ch, _, _, _ := s.GetContent(x+i, y+1)
				if ch != '─' {
					t.Fatal("rule interrupted", i, ch)
				}
			}
			if advanced {
				for row := 0; row < c.table.GetRowCount(); row++ {
					if _, ok := c.table.GetCell(row, 0).Reference.(categoryHeader); ok {
						if !c.table.GetCell(row, 0).NotSelectable {
							t.Fatal("category selectable")
						}
						off, _ := c.table.GetOffset()
						at := y + 2 + row - 2 - off
						if at >= y+2 && at < y+10 {
							ch, _, _, _ := s.GetContent(x+w-1, at)
							if ch != '-' {
								t.Fatal("category did not reach edge", width, ch)
							}
						}
					}
				}
			}
			s.Fini()
		}
		if advanced {
			for _, axis := range controllers.Basic[:4] {
				var axisRow int
				for row := 0; row < c.table.GetRowCount(); row++ {
					if strings.HasPrefix(c.table.GetCell(row, 0).Text, controllers.Label(axis)) {
						axisRow = row
						break
					}
				}
				if axisRow == 0 {
					t.Fatal("axis label absent", axis)
				}
				n := 0
				for row := axisRow + 1; row <= axisRow+2; row++ {
					d, ok := controllers.DirectionByID(c.actionRows[row].ID)
					if !ok || d.Axis != axis {
						t.Fatal("directions not below axis", axis, row)
					}
					n++
				}
				if n != 2 {
					t.Fatal(axis)
				}
			}
		}
		u.finishControllerEditor(false)
	}
}
func TestPickersCurrentDraftIdentityReorderMissingAndCancel(t *testing.T) {
	u := controllerUIForController(t)
	c := u.controllers
	s := c.snapshot
	d := s.Devices[0]
	d.Instance = 8
	d.Name = "Second device"
	d.GUID = "second"
	s.Devices = append(s.Devices, d)
	s.States[8] = s.States[0]
	u.applyControllerSnapshot(s)
	u.selectDevice(d, "")
	id := c.session.Selected
	u.deviceChoices()
	list := u.App.GetFocus().(*tview.List)
	if list.GetCurrentItem() != 2 {
		t.Fatal("non-first selected device not focused")
	}
	u.closePopup()
	s.Devices = []controllers.Device{d, s.Devices[0]}
	u.applyControllerSnapshot(s)
	u.deviceChoices()
	list = u.App.GetFocus().(*tview.List)
	if list.GetCurrentItem() != 1 {
		t.Fatal("enumeration position used as identity")
	}
	u.closePopup()
	// A selection made in this SETUP session controls the next popup immediately.
	u.selectDevice(s.Devices[1], "")
	u.deviceChoices()
	list = u.App.GetFocus().(*tview.List)
	if list.GetCurrentItem() != 2 {
		t.Fatal("draft device not focused")
	}
	selected := c.session.Selected
	u.App.GetInputCapture()(tcell.NewEventKey(tcell.KeyEscape, 0, 0))
	if c.session.Selected != selected {
		t.Fatal("picker Esc committed")
	}
	c.session.Selected = id
	s.Devices = s.Devices[1:]
	u.applyControllerSnapshot(s)
	u.deviceChoices()
	list = u.App.GetFocus().(*tview.List)
	if list.GetCurrentItem() != 0 || c.session.Selected != id {
		t.Fatal("missing-device fallback modified saved choice")
	}
	u.closePopup()
	c.session.Selected = ""
	u.deviceChoices()
	if u.App.GetFocus().(*tview.List).GetCurrentItem() != 0 {
		t.Fatal("None not first")
	}
	u.closePopup()
	u.open("settings")
	row := 0
	for i, v := range u.settings {
		if v.ID == "mouse_capture" {
			row = i
		}
	}
	u.change(row, "disabled")
	u.choose(row)
	list = u.App.GetFocus().(*tview.List)
	if list.GetCurrentItem() != 1 {
		t.Fatal("shared setting picker stale")
	}
	before := u.Doc.Config.Runtime
	u.App.GetInputCapture()(tcell.NewEventKey(tcell.KeyEscape, 0, 0))
	if !reflect.DeepEqual(before, u.Doc.Config.Runtime) {
		t.Fatal("cancel changed value")
	}
}

func TestAllDirectionUICaptureSaveClearAndReopen(t *testing.T) {
	u := controllerUIForController(t)
	c := u.controllers
	c.session.Version = 1
	u.openControllerEditor(true)
	for i, d := range controllers.AxisDirections {
		for row, a := range c.actionRows {
			if a.ID == d.ID {
				c.returnRow, c.returnCol = row, 2
			}
		}
		u.beginCapture(d.ID)
		ss := c.snapshot
		ss.States[0].Buttons[i] = true
		u.applyControllerSnapshot(ss)
		if c.capture != nil || c.pending != nil {
			t.Fatal("UI direction capture incomplete", d.ID)
		}
		b, _ := controllers.EffectiveBinding(*c.draft, d.ID)
		if b.Control == nil || b.Control.Index != i {
			t.Fatal("wrong UI binding", d.ID)
		}
		ss.States[0].Buttons[i] = false
		u.applyControllerSnapshot(ss)
	}
	u.finishControllerEditor(true)
	if c.session.Version != 2 {
		t.Fatal("new pair fields saved as old schema")
	}
	if e := u.saveSession(); e != nil {
		t.Fatal(e)
	}
	read, e := controllers.Load(c.doc.Path)
	if e != nil {
		t.Fatal(e)
	}
	p := read.Config.Profiles[read.Config.Selected]
	for i, d := range controllers.AxisDirections {
		b, _ := controllers.EffectiveBinding(p, d.ID)
		if b.Control == nil || b.Control.Index != i {
			t.Fatal("UI saved direction lost", d.ID)
		}
	}
	u.openControllerEditor(true)
	c.returnRow, c.returnCol = 5, 2
	u.beginCapture("flight.pitch_forward")
	u.App.GetInputCapture()(tcell.NewEventKey(tcell.KeyRune, ' ', 0))
	u.finishControllerEditor(true)
	if e = u.saveSession(); e != nil {
		t.Fatal(e)
	}
	read, e = controllers.Load(c.doc.Path)
	if e != nil {
		t.Fatal(e)
	}
	p = read.Config.Profiles[read.Config.Selected]
	if b, _ := controllers.EffectiveBinding(p, "flight.pitch_forward"); b.Kind != "" {
		t.Fatal("clear not saved")
	}
	if b, _ := controllers.EffectiveBinding(p, "flight.pitch_back"); b.Control == nil || b.Control.Index != 1 {
		t.Fatal("UI clear lost opposite")
	}
}

func TestDirectionPreviewDoesNotOverwriteIndependentAnalog(t *testing.T) {
	u := controllerUIForController(t)
	u.Doc.Config.Runtime.SwapYawRollAxis = false // Retained Q22 Off interpretation; Q3 swap has a separate fixture.
	c := u.controllers
	u.openControllerEditor(true)
	c.draft.Bindings["flight.roll"] = controllers.Binding{Kind: "axis", Axis: 0}
	controllers.SetBinding(c.draft, "flight.roll_left", controllers.Binding{Kind: "control", Control: &controllers.Control{Kind: "button", Index: 0}})
	ss := c.snapshot
	ss.States[0].Axes[0] = 1
	ss.States[0].Buttons[0] = true
	u.applyControllerSnapshot(ss)
	for row, a := range c.actionRows {
		if a.ID == "flight.roll_left" {
			c.table.Select(row, 0)
		}
	}
	s := frame(t, u, 76, 24)
	ch, _, _, _ := s.GetContent(6, 8)
	s.Fini()
	if ch != '█' {
		t.Fatal("focused direction not previewed")
	}
	if c.draft.Bindings["flight.roll"].Kind != "axis" {
		t.Fatal("preview replaced analog")
	}
	u.finishControllerEditor(true)
	u.openControllerEditor(false)
	s = frame(t, u, 76, 24)
	ch, _, _, _ = s.GetContent(14, 8)
	s.Fini()
	if ch != '█' {
		t.Fatal("Basic no longer previews dedicated analog")
	}
}
