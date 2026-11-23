package setupui

import (
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
	"slices"
	"testing"
)

func TestAltDRevealsDebugOnce(t *testing.T) {
	u := uiUI(t)
	input := u.App.GetInputCapture()
	key := func(k tcell.Key, r rune, m tcell.ModMask) *tcell.EventKey { return input(tcell.NewEventKey(k, r, m)) }
	if key(tcell.KeyF10, 0, 0) == nil || key(tcell.KeyRune, 'd', 0) == nil || slices.Contains(u.mainIDs, "debug") {
		t.Fatal("DEBUG must be hidden")
	}
	item := u.Menu.GetCurrentItem()
	if key(tcell.KeyRune, 'd', tcell.ModAlt) != nil || !slices.Contains(u.mainIDs, "debug") {
		t.Fatal("Alt+D did not reveal DEBUG")
	}
	n := len(u.mainIDs)
	if key(tcell.KeyRune, 'D', tcell.ModAlt) != nil || len(u.mainIDs) != n || u.Menu.GetCurrentItem() != item {
		t.Fatal("repeat must retain focus and visibility")
	}
}

func TestHomeDChangesDebugVisibility(t *testing.T) {
	u := uiUI(t)
	press := func(ch rune, mod tcell.ModMask) {
		e := tcell.NewEventKey(tcell.KeyRune, ch, mod)
		if e = u.App.GetInputCapture()(e); e != nil {
			u.App.GetFocus().InputHandler()(e, func(p tview.Primitive) { u.App.SetFocus(p) })
		}
	}
	u.home()
	press('d', 0)
	row := slices.Index(u.mainIDs, "debug")
	if row < 0 {
		t.Fatal("plain d did not reveal DEBUG")
	}
	u.Menu.SetCurrentItem(row)
	press('D', tcell.ModShift)
	if slices.Contains(u.mainIDs, "debug") || u.currentID() != "rebuild" {
		t.Fatal("hiding selected DEBUG did not focus next action")
	}
	u.open("settings")
	u.input(1)
	edited, ok := u.App.GetFocus().(*tview.InputField)
	if !ok {
		t.Fatal("original path input missing")
	}
	edited.SetText("")
	press('d', 0)
	if edited.GetText() != "d" || slices.Contains(u.mainIDs, "debug") {
		t.Fatal("D did not stay in the field")
	}
}
