package setupui

import (
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
	"gunship2000-assets/game/controllers"
	"os"
	"strings"
	"testing"
)

func TestCollectiveModeAndSourceOverrideDraft(t *testing.T) {
	u := controllerUIForController(t)
	c := u.controllers
	initial, err := os.ReadFile(c.doc.Path)
	if err != nil {
		t.Fatal(err)
	}
	id := c.session.Selected
	p := c.session.Profiles[id]
	p.CollectiveMode = "relative"
	c.session.Profiles[id] = p
	u.open("controls")
	u.Table.Select(4, 0)
	u.activate(4)
	l := u.App.GetFocus().(*tview.List)
	if l.GetItemCount() != 3 || l.GetCurrentItem() != 1 {
		t.Fatal("mode current selection", l.GetCurrentItem())
	}
	l.SetCurrentItem(2)
	l.InputHandler()(tcell.NewEventKey(tcell.KeyEnter, 0, 0), func(p tview.Primitive) { u.App.SetFocus(p) })
	if c.session.Profiles[id].CollectiveMode != "relative_height" {
		t.Fatal("session mode lost")
	}
	u.open("controls")
	u.Table.Select(5, 0)
	u.activate(5)
	l = u.App.GetFocus().(*tview.List)
	if l.GetItemCount() != 3 || l.GetCurrentItem() != 0 {
		t.Fatal("override current None")
	}
	l.SetCurrentItem(2)
	l.InputHandler()(tcell.NewEventKey(tcell.KeyEnter, 0, 0), func(p tview.Primitive) { u.App.SetFocus(p) })
	if c.session.Profiles[id].CollectiveSource != "slider" || c.session.Profiles[id].CollectiveMode != "relative_height" {
		t.Fatal("override changed pilot mode")
	}
	if disk, e := os.ReadFile(c.doc.Path); e != nil || string(disk) != string(initial) {
		t.Fatal("draft wrote disk")
	}
	if e := u.saveSession(); e != nil {
		t.Fatal(e)
	}
	read, e := controllers.Load(c.doc.Path)
	if e != nil || read.Config.Profiles[id].CollectiveMode != "relative_height" || read.Config.Profiles[id].CollectiveSource != "slider" {
		t.Fatal(e)
	}
	u.open("controls")
	u.Table.Select(5, 0)
	u.activate(5)
	l = u.App.GetFocus().(*tview.List)
	l.SetCurrentItem(0)
	l.InputHandler()(tcell.NewEventKey(tcell.KeyEnter, 0, 0), func(p tview.Primitive) { u.App.SetFocus(p) })
	if !strings.Contains(u.Help.GetText(true), "Unknown controller type") || c.session.Profiles[id].CollectiveMode != "relative_height" {
		t.Fatal("source tooltip/default stale", u.Help.GetText(true))
	}
	u.openControllerEditor(false)
	if c.table.GetRowCount() != 13 || c.table.GetCell(basicWizard, 0).Text != "<Run Wizard>" {
		t.Fatal("source/mode still Basic")
	}
}
