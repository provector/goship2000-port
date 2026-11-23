package setupui

import (
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
	"gunship2000-assets/game/configuration"
	"testing"
)

func TestAvoidanceDraftAndSavedChoice(t *testing.T) {
	u := controllerUIForController(t)
	u.open("settings")
	row := -1
	for i, s := range u.settings {
		if s.ID == "avoidance_policy" {
			row = i
		}
	}
	if row < 0 {
		t.Fatal("missing avoidance preference")
	}
	u.activate(row)
	l := u.App.GetFocus().(*tview.List)
	if l.GetCurrentItem() != 1 {
		t.Fatal("default corrected selection missing")
	}
	focus := func(p tview.Primitive) { u.App.SetFocus(p) }
	l.SetCurrentItem(0)
	l.InputHandler()(tcell.NewEventKey(tcell.KeyEscape, 0, 0), focus)
	if u.Doc.Config.Runtime.AvoidancePolicy != configuration.AvoidanceCorrected {
		t.Fatal("cancel changed policy")
	}
	u.activate(row)
	l = u.App.GetFocus().(*tview.List)
	l.SetCurrentItem(0)
	l.InputHandler()(tcell.NewEventKey(tcell.KeyEnter, 0, 0), focus)
	if err := u.saveSession(); err != nil {
		t.Fatal(err)
	}
	doc, err := configuration.Load(u.Doc.Path)
	if err != nil || doc.Config.Runtime.AvoidancePolicy != configuration.AvoidanceLegacy {
		t.Fatal("saved selection", err)
	}
	u.activate(row)
	if u.App.GetFocus().(*tview.List).GetCurrentItem() != 0 {
		t.Fatal("picker reset saved choice")
	}
	doc.Config.Runtime.AvoidancePolicy = "unsupported"
	if doc.Config.Validate() == nil {
		t.Fatal("invalid policy admitted")
	}
}
