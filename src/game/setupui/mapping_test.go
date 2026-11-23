package setupui

import (
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
	"gunship2000-assets/game/controllers"
	"reflect"
	"testing"
)

func TestAdvancedDocumentedRowsCaptureCancelAndUnrelatedSave(t *testing.T) {
	u := controllerUIForController(t)
	c := u.controllers
	p := c.session.Profiles[c.session.Selected]
	ids := []string{"view.primary_mfd", "view.secondary_mfd", "editor.help"}
	for i, id := range ids {
		p.Bindings[id] = controllers.Binding{Kind: "control", Control: &controllers.Control{Kind: "button", Index: 10 + i}}
	}
	c.session.Profiles[p.ID] = p
	if err := u.saveSession(); err != nil {
		t.Fatal(err)
	}
	u.openControllerEditor(true)
	for _, id := range ids {
		row := -1
		for r, a := range c.actionRows {
			if a.ID == id {
				row = r
				if a.Status == "deferred" {
					t.Fatal("new row is disabled", id)
				}
			}
		}
		if row < 0 || c.table.GetCell(row, 0).NotSelectable {
			t.Fatal("new action row absent/nonselectable", id, row)
		}
		// Whole-row activation from the action column reaches the real popup.
		c.table.Select(row, 0)
		c.table.InputHandler()(tcell.NewEventKey(tcell.KeyEnter, 0, 0), func(p tview.Primitive) { u.App.SetFocus(p) })
		if c.capture == nil || c.captureID != id || !u.popup {
			t.Fatal("Enter did not bind the selected action", id, c.captureID)
		}
		u.App.GetInputCapture()(tcell.NewEventKey(tcell.KeyEscape, 0, 0))
		if c.capture != nil || !reflect.DeepEqual(c.draft.Bindings[id], p.Bindings[id]) {
			t.Fatal("cancel changed retained assignment", id)
		}
		if selected, _ := c.table.GetSelection(); selected != row {
			t.Fatal("cancel lost the edited row", id, selected, row)
		}
	}
	u.finishControllerEditor(false)
	// A normal unrelated general preference save must not discard bindings.
	u.Doc.Config.MusicVolume = 37
	if err := u.saveSession(); err != nil {
		t.Fatal(err)
	}
	read, err := controllers.Load(c.doc.Path)
	if err != nil || !reflect.DeepEqual(read.Config.Profiles[p.ID].Bindings, p.Bindings) {
		t.Fatal("SETUP unrelated Save/reopen lost existing assignments", err)
	}
}
