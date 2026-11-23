package setupui

import (
	"path/filepath"
	"testing"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
	"gunship2000-assets/game/configuration"
)

func TestResolutionPickerSaveReopen(t *testing.T) {
	path := filepath.Join(t.TempDir(), configuration.Filename)
	doc, _, err := configuration.Initialize(path)
	if err != nil {
		t.Fatal(err)
	}
	names := []string{"320x200 (Legacy)", "640x480 (VGA High)", "800x600 (SVGA)", "1024x768 (Experimental)", "1280x1024 (Experimental)"}
	modes := []string{"320x200", "640x480", "800x600", "1024x768", "1280x1024"}
	previous := 1
	for i, mode := range modes {
		ui := New(doc)
		ui.open("graphics")
		ui.activate(1)
		list := ui.App.GetFocus().(*tview.List)
		if list.GetItemCount() != len(names) || list.GetCurrentItem() != previous {
			t.Fatal("picker focus", list.GetCurrentItem(), previous)
		}
		for index, want := range names {
			if got, _ := list.GetItemText(index); got != want {
				t.Fatalf("choice %d: %q", index, got)
			}
		}
		press := func(key tcell.Key) {
			list.InputHandler()(tcell.NewEventKey(key, 0, 0), func(p tview.Primitive) { ui.App.SetFocus(p) })
		}
		press(tcell.KeyDown)
		press(tcell.KeyEscape)
		if ui.popup || doc.Config.Runtime.SimulationResolution != modes[previous] {
			t.Fatal("cancel changed selection")
		}
		ui.activate(1)
		list = ui.App.GetFocus().(*tview.List)
		for list.GetCurrentItem() != i {
			press(tcell.KeyDown)
		}
		press(tcell.KeyEnter)
		if ui.popup {
			t.Fatal("confirmation left picker open")
		}
		if err = doc.Save(); err != nil {
			t.Fatal(err)
		}
		doc, err = configuration.Load(path)
		if err != nil || doc.Config.Runtime.SimulationResolution != mode {
			t.Fatal(mode, err)
		}
		previous = i
	}
}
