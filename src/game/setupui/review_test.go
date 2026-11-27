package setupui

import (
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
	"gunship2000-assets/game/configuration"
	"gunship2000-assets/game/controllers"
)

func TestConfirmationKeysFocusAndIsolation(t *testing.T) {
	u := uiUI(t)
	u.open("graphics")
	row := slices.IndexFunc(u.settings, func(s Setting) bool { return s.ID == "legacy_defaults" })
	u.Table.Select(row, 0)
	press := func(key tcell.Key) {
		e := u.App.GetInputCapture()(tcell.NewEventKey(key, 0, tcell.ModNone))
		if e != nil {
			u.App.GetFocus().InputHandler()(e, func(p tview.Primitive) { u.App.SetFocus(p) })
		}
	}
	original := u.Doc.Config
	press(tcell.KeyEnter)
	for _, direction := range []tcell.Key{tcell.KeyRight, tcell.KeyUp, tcell.KeyDown, tcell.KeyLeft, tcell.KeyBacktab, tcell.KeyTab} {
		button := u.App.GetFocus().(*tview.Button)
		press(direction)
		if u.App.GetFocus() == button || !reflect.DeepEqual(original, u.Doc.Config) {
			t.Fatal("focus did not move without editing", direction)
		}
	}
	press(tcell.KeyRight)
	press(tcell.KeyEnter)
	r, _ := u.Table.GetSelection()
	if u.popup || u.App.GetFocus() != u.Table || r != row || !reflect.DeepEqual(original, u.Doc.Config) {
		t.Fatal("Cancel or return focus")
	}
	press(tcell.KeyEnter)
	press(tcell.KeyLeft)
	press(tcell.KeyDown)
	press(tcell.KeyEnter)
	if u.popup || u.Doc.Config.Runtime.PresentationMode != "stepped" || u.Doc.Config.Runtime.PresentationFPS != 60 {
		t.Fatal("Apply")
	}
}

func TestCompactHelpAndMenuWalk(t *testing.T) {
	u := controllerUIForController(t)
	u.RevealDebug()
	screen := tcell.NewSimulationScreen("UTF-8")
	if err := screen.Init(); err != nil {
		t.Fatal(err)
	}
	defer screen.Fini()
	screen.SetSize(80, 25)
	checkHelp := func(id string) {
		u.App.GetBeforeDrawFunc()(screen)
		u.Main.SetRect(0, 0, 80, 25)
		u.Main.Draw(screen)
		_, _, w, h := u.Help.GetInnerRect()
		txt := u.Help.GetText(false)
		if strings.TrimSpace(txt) == "" || len(tview.WordWrap(txt, w)) > h {
			t.Errorf("%s help overflows %dx%d: %q", id, w, h, txt)
		}
	}
	for _, menu := range []string{"graphics", "sound", "controls", "settings", "debug", "advanced", "logging"} {
		u.open(menu)
		ids := []string{}
		for r, s := range u.settings {
			ids = append(ids, s.ID)
			u.Table.Select(r, 0)
			checkHelp(s.ID)
			if s.Kind == "input" || s.Kind == "level" {
				u.activate(r)
				if s.ID == "original" { // The installation list initially owns focus.
					list, ok := u.App.GetFocus().(*tview.List)
					if !ok {
						t.Fatal("missing installation selector")
					}
					list.SetCurrentItem(list.GetItemCount() - 1) // Custom has the editable path.
					list.InputHandler()(tcell.NewEventKey(tcell.KeyTab, 0, 0), func(p tview.Primitive) { u.App.SetFocus(p) })
				}
				field, ok := u.App.GetFocus().(*tview.InputField)
				if !ok {
					t.Fatal(s.ID, "did not open editable field")
				}
				key := func(k tcell.Key) {
					u.App.GetFocus().InputHandler()(tcell.NewEventKey(k, 0, 0), func(p tview.Primitive) { u.App.SetFocus(p) })
				}
				field.SetText("discard this input")
				key(tcell.KeyLeft)
				if u.App.GetFocus() != field {
					t.Fatal(s.ID, "stole text cursor")
				}
				key(tcell.KeyTab)
				key(tcell.KeyRight)
				b, ok := u.App.GetFocus().(*tview.Button)
				if !ok || b.GetLabel() != "Cancel" {
					t.Fatal(s.ID, "arrows did not select Cancel")
				}
				key(tcell.KeyEnter)
				if u.popup || u.App.GetFocus() != u.Table {
					t.Fatal(s.ID, "lost return focus")
				}
			}
			if s.Kind == "commands" {
				u.activate(r)
				if !strings.Contains(u.App.GetFocus().(*tview.TextView).GetText(false), "films identify") {
					t.Fatal("film command unavailable")
				}
				u.closePopup()
			}
			if s.Kind == "choice" {
				u.activate(r)
				p := u.App.GetFocus().(*tview.List)
				for i := 0; i < p.GetItemCount(); i++ {
					p.SetCurrentItem(i)
					checkHelp(s.ID)
				}
				u.closePopup()
			}
		}
		if menu == "graphics" && !slices.Equal(ids, []string{"back", "render_scale", "presentation", "floating", "game_window", "draw_fps_cpu", "legacy_defaults", "modern_defaults"}) {
			t.Fatal(ids)
		}
		t.Log(menu, ids)
	}
	for _, adv := range []bool{false, true} {
		u.openControllerEditor(adv)
		for r := 2; r < u.controllers.table.GetRowCount(); r++ {
			if !u.controllers.table.GetCell(r, 0).NotSelectable {
				u.controllers.table.Select(r, 0)
				checkHelp(u.controllers.table.GetCell(r, 0).Text)
			}
		}
		u.finishControllerEditor(false)
	}
}

func TestBindingConflictBackReturnsToEditor(t *testing.T) {
	u := controllerUIForController(t)
	u.openControllerEditor(true)
	c := u.controllers
	for r, a := range c.actionRows {
		if a.ID == "flight.rotor" {
			c.returnRow, c.returnCol = r, 2
			c.table.Select(r, 2)
		}
	}
	b := controllers.Binding{Kind: "control", Control: &controllers.Control{Kind: "button", Index: 1}}
	c.draft.Bindings["flight.hover"] = b
	c.captureID = "flight.rotor"
	u.acceptCaptured(b)
	if !u.popup || c.pending == nil {
		t.Fatal("conflict not reached")
	}
	u.App.GetInputCapture()(tcell.NewEventKey(tcell.KeyBackspace2, 0, 0))
	if u.popup || c.pending != nil || u.App.GetFocus() != c.table || !reflect.DeepEqual(c.draft.Bindings["flight.hover"], b) {
		t.Fatal("Back lost the editor focus or pending binding ownership")
	}
}

func TestSavedStockTextAndCustomEdits(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	u := controllerUIForController(t)
	cfg := &u.Doc.Config
	cfg.Runtime.PresentationFPS, cfg.Runtime.PresentationMode = 37, "smooth"
	cfg.Original = "/custom/example"
	cfg.UI["presentation"] = configuration.Text{Label: "Presentation", Tooltip: "Choose stepped or smooth pose presentation independently of the drawing cap.", Choices: map[string]configuration.ChoiceText{"stepped": {Label: "Stepped"}, "smooth": {Label: "Smooth"}}}
	cfg.UI["render_fps"] = configuration.Text{Label: "Rendering FPS cap"}
	cfg.UI["speech"] = configuration.Text{Label: "My radio", Tooltip: "My own speech help"}
	for i := 0; i < 2; i++ {
		if err := u.Doc.Save(); err != nil {
			t.Fatal(err)
		}
		d, err := configuration.Load(u.Doc.Path)
		if err != nil {
			t.Fatal(err)
		}
		u = New(d)
		u.open("graphics")
		row := -1
		for k, v := range u.settings {
			if v.ID == "presentation" {
				if row != -1 {
					t.Fatal("duplicate Framerate control")
				}
				row = k
			}
		}
		if row < 0 || u.Table.GetCell(row, 0).Text != "Framerate" || u.Table.GetCell(row, 1).Text != "60fps" || u.Doc.Config.Runtime.PresentationFPS != 37 || u.Doc.Config.UI["speech"].Tooltip != "My own speech help" || u.Doc.Config.Original != "/custom/example" {
			t.Fatal("lost setting or stale text", u.Doc.Config.UI["presentation"])
		}
		for _, s := range u.settings {
			if s.ID == "render_fps" {
				t.Fatal("stock label created a selectable row")
			}
		}
	}
}
