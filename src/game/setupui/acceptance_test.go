package setupui

import (
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
	"gunship2000-assets/game/controllers"
	"os"
	"reflect"
	"testing"
	"time"
)

func TestYawToSelectorWizardNoGlobalAxisGate(t *testing.T) {
	for run := 0; run < 50; run++ {
		u := controllerUIForController(t)
		u.openControllerEditor(false)
		c := u.controllers
		c.wizard = true
		c.step = 3
		u.beginCapture("flight.yaw")
		s := c.snapshot
		st := s.States[0]
		st.Axes = append([]float64(nil), st.Axes...)
		st.Buttons = append([]bool(nil), st.Buttons...)
		st.Axes[3] = 1
		s.States[0] = st
		u.applyControllerSnapshot(s)
		if c.step != 4 || c.capture == nil || c.captureID != "menu.selector1" {
			t.Fatal("selector prompt blocked", c.step, c.captureID)
		}
		st.Buttons[5] = true
		s.States[0] = st
		u.applyControllerSnapshot(s)
		if b := c.draft.Bindings["menu.selector1"]; b.Control == nil || b.Control.Index != 5 {
			t.Fatal("quick first selector press lost")
		}
		if run == 0 {
			for c.wizard {
				c.keyUntil = time.Time{}
				u.App.GetInputCapture()(tcell.NewEventKey(tcell.KeyEscape, 0, 0))
			}
			u.finishControllerEditor(true)
			if e := u.saveSession(); e != nil {
				t.Fatal(e)
			}
			d, e := controllers.Load(c.doc.Path)
			if e != nil {
				t.Fatal(e)
			}
			b := d.Config.Profiles[d.Config.Selected].Bindings["menu.selector1"]
			if b.Control == nil || b.Control.Index != 5 {
				t.Fatal("captured selector lost after save/reopen")
			}
		}
	}
}

func TestResetSessionSaveDiscardTwoProfiles(t *testing.T) {
	u := controllerUIForController(t)
	c := u.controllers
	id := c.session.Selected
	p := c.session.Profiles[id]
	p.Bindings = map[string]controllers.Binding{"flight.pitch": {Kind: "axis", Axis: 1, Invert: true}, "weapon.weapon": {Kind: "control", Control: &controllers.Control{Kind: "button", Index: 2}}, "view.night": {Kind: "control", Control: &controllers.Control{Kind: "button", Index: 3}}}
	p.AxisButtons = map[string]controllers.Binding{"flight.roll": {Kind: "pair", Positive: &controllers.Control{Kind: "button", Index: 4}}}
	p.CollectiveMode = "relative"
	c.session.Profiles[id] = p
	other := controllers.Clone(c.session).Profiles[id]
	other.ID = "other"
	other.Device.Serial = "other"
	c.session.Profiles[other.ID] = other
	if e := u.saveSession(); e != nil {
		t.Fatal(e)
	}
	before, _ := os.ReadFile(c.doc.Path)
	u.resetBindings()
	m := u.App.GetFocus()
	m.InputHandler()(tcell.NewEventKey(tcell.KeyEnter, 0, 0), func(p tview.Primitive) { u.App.SetFocus(p) })
	a := c.session.Profiles[id]
	if len(a.Bindings) != 0 || len(a.AxisButtons) != 0 || c.session.Selected != id || a.CollectiveMode != "relative" || !reflect.DeepEqual(c.session.Profiles[other.ID], other) {
		t.Fatal("reset scope")
	}
	after, _ := os.ReadFile(c.doc.Path)
	if string(before) != string(after) {
		t.Fatal("reset wrote before Save")
	}
	d, e := controllers.Load(c.doc.Path)
	if e != nil || len(d.Config.Profiles[id].Bindings) != 3 {
		t.Fatal("discard/reopen lost bindings", e)
	}
	if e := u.saveSession(); e != nil {
		t.Fatal(e)
	}
	d, e = controllers.Load(c.doc.Path)
	if e != nil || len(d.Config.Profiles[id].Bindings) != 0 || !reflect.DeepEqual(d.Config.Profiles[other.ID].Bindings, other.Bindings) {
		t.Fatal("saved reset", e)
	}
}

func TestWholeRowAndThreeCollectiveChoices(t *testing.T) {
	u := controllerUIForController(t)
	u.openControllerEditor(false)
	c := u.controllers
	c.table.Select(3, 0)
	s := frame(t, u, 80, 25)
	defer s.Fini()
	x, y, w, _ := c.table.GetInnerRect()
	for col := 0; col < w; col++ {
		_, _, style, _ := s.GetContent(x+col, y+3)
		_, bg, _ := style.Decompose()
		if bg != blue {
			t.Fatal("partial highlight", col, bg)
		}
	}
	u.finishControllerEditor(true)
	u.open("controls")
	u.activate(4)
	list := u.App.GetFocus().(*tview.List)
	if list.GetItemCount() != 3 {
		t.Fatal("collective choices")
	}
	for i, name := range []string{"Absolute", "Relative", "Relative Height"} {
		label, _ := list.GetItemText(i)
		if label != name {
			t.Fatal(label)
		}
	}
}
