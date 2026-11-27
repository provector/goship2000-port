package setupui

import (
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
	"gunship2000-assets/game/configuration"
	"path/filepath"
	"strings"
	"testing"
)

func uiUI(t *testing.T) *UI {
	t.Helper()
	d, _, e := configuration.Initialize(filepath.Join(t.TempDir(), configuration.Filename))
	if e != nil {
		t.Fatal(e)
	}
	return New(d)
}
func settingRow(t *testing.T, u *UI, id string) int {
	t.Helper()
	for row, setting := range u.settings {
		if setting.ID == id {
			return row
		}
	}
	t.Fatalf("missing %s setting on %s", id, u.menu)
	return -1
}
func TestMenuOwnershipInlinePresetsAndBack(t *testing.T) {
	u := uiUI(t)
	for _, menu := range []string{"graphics", "sound", "controls", "settings", "debug", "advanced"} {
		a := Catalogue(menu)
		if a[0].ID != "back" || a[0].Kind != "back" {
			t.Fatal(menu)
		}
		for _, s := range a {
			if s.ID == "fps" || s.ID == "resolution" || s.ID == "rebuild" {
				t.Fatal("removed submenu entry", menu, s.ID)
			}
			if s.ID == "fade" && menu != "advanced" {
				t.Fatal(menu)
			}
			if s.ID == "mouse_capture" && menu != "settings" {
				t.Fatal(menu)
			}
		}
	}
	u.RevealDebug()
	u.Menu.SetCurrentItem(4)
	u.open("debug")
	u.Table.Select(3, 0)
	u.activate(3)
	if u.menu != "advanced" {
		t.Fatal(u.menu)
	}
	u.activate(0)
	r, _ := u.Table.GetSelection()
	if u.menu != "debug" || r != 3 {
		t.Fatal("parent focus", u.menu, r)
	}
	u.back()
	if u.Menu.GetCurrentItem() != 4 {
		t.Fatal("main focus")
	}
	u.open("graphics")
	u.activate(1)
	list := u.App.GetFocus().(*tview.List)
	list.SetCurrentItem(0)
	list.InputHandler()(tcell.NewEventKey(tcell.KeyEnter, 0, 0), func(p tview.Primitive) { u.App.SetFocus(p) })
	if u.popup || u.Doc.Config.Runtime.SimulationRenderScale != 1 {
		t.Fatal("render cycle")
	}
	u.activate(1)
	list = u.App.GetFocus().(*tview.List)
	list.SetCurrentItem(1)
	list.InputHandler()(tcell.NewEventKey(tcell.KeyEnter, 0, 0), func(p tview.Primitive) { u.App.SetFocus(p) })
	if u.Doc.Config.Runtime.SimulationRenderScale != 2 {
		t.Fatal("cycle back")
	}
	for _, mode := range []string{"stepped", "smooth"} {
		u.change(settingRow(t, u, "presentation"), mode)
		if u.Doc.Config.Runtime.PresentationMode != mode || u.Doc.Config.Runtime.PresentationFPS != 60 {
			t.Fatal(mode)
		}
	}
	if u.text("presentation").Choices["stepped"].Label != "10fps (Legacy)" || u.text("presentation").Choices["smooth"].Label != "60fps" {
		t.Fatal("presets")
	}
	// External cache guard tests routing without running/rebuilding any assets.
	u.home()
	u.Doc.Config.Assets = filepath.Join(t.TempDir(), "protected")
	u.Menu.SetCurrentItem(5)
	u.Menu.InputHandler()(tcell.NewEventKey(tcell.KeyEnter, 0, 0), func(tview.Primitive) {})
	if !u.popup || u.menu != "" {
		t.Fatal("main rebuild inaccessible")
	}
	u.closePopup()
	if u.App.GetFocus() != u.Menu || u.Menu.GetCurrentItem() != 5 {
		t.Fatal("rebuild return focus")
	}
}
func TestRenderedPaletteFooterAndInertMouse(t *testing.T) {
	u := uiUI(t)
	s := tcell.NewSimulationScreen("UTF-8")
	s.Init()
	defer s.Fini()
	s.SetSize(80, 25)
	for _, menu := range []string{"", "sound", "controls"} {
		if menu == "" {
			u.home()
		} else {
			u.open(menu)
		}
		u.Main.SetRect(0, 0, 80, 25)
		u.Main.Draw(s)
		for _, x := range []int{1, 20, 50, 77} {
			_, _, style, _ := s.GetContent(x, 12)
			fg, bg, _ := style.Decompose()
			if bg.Hex() != red.Hex() {
				t.Fatal("ordinary row patch", menu, x, fg, bg)
			}
			_, _, style, _ = s.GetContent(x, 11)
			_, bg, _ = style.Decompose()
			if bg.Hex() != blue.Hex() {
				t.Fatal("broken selected highlight", menu, x, bg)
			}
		}
		var footer strings.Builder
		for x := 0; x < 80; x++ {
			ch, _, _, _ := s.GetContent(x, 24)
			footer.WriteRune(ch)
		}
		if !strings.Contains(footer.String(), "Bksp Back") || !strings.Contains(footer.String(), "D - Debug Menu") {
			t.Fatal(footer.String())
		}
	}
	for _, action := range []tview.MouseAction{tview.MouseMove, tview.MouseLeftDown, tview.MouseLeftUp, tview.MouseLeftClick, tview.MouseRightClick, tview.MouseScrollUp, tview.MouseScrollDown} {
		ev, _ := u.App.GetMouseCapture()(tcell.NewEventMouse(2, 11, tcell.Button1, 0), action)
		if ev != nil {
			t.Fatal("mouse not swallowed", action)
		}
	}
	if !u.helpPanel.controls || !strings.Contains(NeutralAxes, "Collective") || strings.Count(NeutralAxes, "Yaw") != 1 {
		t.Fatal("axes")
	}
	for _, n := range []int{0, 1, 37, 100} {
		if !strings.Contains(volumeBar(n), "%") || strings.Contains(volumeBar(n), "#") {
			t.Fatal(n)
		}
	}
}

func TestBackspaceTextAndPopupOwnership(t *testing.T) {
	u := uiUI(t)
	back := tcell.NewEventKey(tcell.KeyBackspace2, 0, 0)
	if u.App.GetInputCapture()(back) != nil || u.menu != "" {
		t.Fatal("main back")
	}
	u.open("settings")
	row := -1
	for i, setting := range u.settings {
		if setting.ID == "mouse_capture" {
			row = i
			break
		}
	}
	if row < 0 {
		t.Fatal("mouse capture picker missing")
	}
	u.Table.Select(row, 0)
	u.activate(row)
	u.App.GetInputCapture()(back)
	r, _ := u.Table.GetSelection()
	if u.popup || u.menu != "settings" || u.App.GetFocus() != u.Table || r != row {
		t.Fatal("popup back escaped parent")
	}
	u.open("advanced")
	u.input(2)
	field, ok := u.App.GetFocus().(*tview.InputField)
	if !ok {
		t.Fatalf("editable focus %T", u.App.GetFocus())
	}
	field.SetText("baseX")
	event := u.App.GetInputCapture()(back)
	if event == nil {
		t.Fatal("backspace stolen from text field")
	}
	field.InputHandler()(event, func(tview.Primitive) {})
	if field.GetText() != "base" || !u.popup {
		t.Fatal(field.GetText(), u.popup)
	}
	u.App.GetInputCapture()(tcell.NewEventKey(tcell.KeyEscape, 0, 0))
	if u.popup || u.menu != "advanced" {
		t.Fatal("Esc did not cancel just input")
	}
}
