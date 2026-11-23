package setupui

import (
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
	"gunship2000-assets/game/controllers"
	"testing"
)

func TestSwapUnderCollectiveDraftSaveAndCancel(t *testing.T) {
	u := controllerUIForController(t)
	u.Doc.Config.Runtime.SwapYawRollAxis = true // Explicit saved On remains supported; Q31 default is Off.
	if !u.Doc.Config.Runtime.SwapYawRollAxis {
		t.Fatal("new default")
	}
	u.openControllerEditor(false)
	c := u.controllers
	if c.table.GetCell(6, 0).Text != "Collective" || c.table.GetCell(7, 0).Text != "Swap Yaw/Roll Axis" || c.table.GetCell(7, 1).Text != "On" || c.table.GetCell(8, 0).Text != "Yaw" {
		t.Fatal("placement")
	}
	c.table.Select(7, 0)
	c.table.InputHandler()(tcell.NewEventKey(tcell.KeyEnter, 0, 0), func(p tview.Primitive) { u.App.SetFocus(p) })
	if c.draftSwap || c.table.GetCell(7, 1).Text != "Off" {
		t.Fatal("toggle")
	}
	u.finishControllerEditor(false)
	if !u.Doc.Config.Runtime.SwapYawRollAxis {
		t.Fatal("cancel committed")
	}
	u.openControllerEditor(false)
	c.table.Select(7, 1)
	c.table.InputHandler()(tcell.NewEventKey(tcell.KeyEnter, 0, 0), func(p tview.Primitive) { u.App.SetFocus(p) })
	u.finishControllerEditor(true)
	if u.Doc.Config.Runtime.SwapYawRollAxis {
		t.Fatal("local save lost")
	}
	u.openControllerEditor(false)
	if c.draftSwap {
		t.Fatal("session reopen")
	}
	// Yaw still owns its own inversion cell after the inserted preference.
	c.table.Select(8, 0)
	u.App.GetInputCapture()(tcell.NewEventKey(tcell.KeyRune, ' ', 0))
	if !c.draft.Bindings["flight.yaw"].Invert {
		t.Fatal("yaw inversion lifecycle")
	}
}

func TestPreviewUsesSwappedChannelsWithoutChangingAssignments(t *testing.T) {
	u := controllerUIForController(t)
	u.Doc.Config.Runtime.SwapYawRollAxis = true // Explicit saved On remains supported; Q31 default is Off.
	u.openControllerEditor(false)
	c := u.controllers
	c.draft.Bindings["flight.roll"] = controllers.Binding{Kind: "axis", Axis: 0}
	c.draft.Bindings["flight.yaw"] = controllers.Binding{Kind: "axis", Axis: 1}
	snap := c.snapshot
	snap.States[0].Axes[0] = 1
	snap.States[0].Axes[1] = -.5
	u.applyControllerSnapshot(snap)
	screen := frame(t, u, 76, 24)
	defer screen.Fini()
	for _, x := range []int{8, 33} {
		ch, _, _, _ := screen.GetContent(x, 6)
		if ch != '█' {
			t.Fatal("swapped cross marker", x, ch)
		}
	}
	if c.draft.Bindings["flight.roll"].Axis != 0 || c.draft.Bindings["flight.yaw"].Axis != 1 {
		t.Fatal("swap rewrote bindings")
	}
}
