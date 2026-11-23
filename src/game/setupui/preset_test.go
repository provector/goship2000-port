package setupui

import (
	"path/filepath"
	"reflect"
	"testing"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
	"gunship2000-assets/game/configuration"
)

func TestPresetDraftAndPlacement(t *testing.T) {
	d, _, err := configuration.Initialize(filepath.Join(t.TempDir(), configuration.Filename))
	if err != nil {
		t.Fatal(err)
	}
	d.Config.Runtime.PresentationMode = "stepped"
	d.Config.Runtime.PresentationFPS = 37
	d.Config.Runtime.SimulationResolution = "1280x1024"
	d.Config.Runtime.AvoidancePolicy = configuration.AvoidanceLegacy
	d.Config.SimplifiedFields = true
	u := New(d)
	if settings := Catalogue("settings"); settings[0].ID != "back" || settings[1].ID != "original" {
		t.Fatal(settings[1].ID)
	}
	row := func(id string) int {
		for i, s := range u.settings {
			if s.ID == id {
				return i
			}
		}
		t.Fatal("row missing", id)
		return -1
	}
	u.open("graphics")
	before := d.Config
	press := func(k tcell.Key) {
		event := u.App.GetInputCapture()(tcell.NewEventKey(k, 0, 0))
		if event != nil {
			u.App.GetFocus().InputHandler()(event, func(p tview.Primitive) { u.App.SetFocus(p) })
		}
	}
	for _, key := range []tcell.Key{tcell.KeyEscape, tcell.KeyTab} {
		u.activate(row("legacy_defaults"))
		if !u.popup {
			t.Fatal("missing confirmation")
		}
		press(key)
		if key == tcell.KeyTab {
			press(tcell.KeyEnter)
		}
		if u.popup || !reflect.DeepEqual(before, d.Config) {
			t.Fatal("cancel mutated graphics")
		}
	}
	for _, id := range []string{"legacy_defaults", "modern_defaults"} {
		u.activate(row(id))
		press(tcell.KeyEnter)
		want := before
		if id == "legacy_defaults" {
			want.Runtime.SimulationResolution = "320x200"
			want.Runtime.SimulationRenderScale = 1
			want.Runtime.PresentationMode = "stepped"
			want.Runtime.PresentationFPS = 60
			want.Runtime.FloatingGeometry = false
		} else {
			want.Runtime.SimulationResolution = "640x480"
			want.Runtime.SimulationRenderScale = 2
			want.Runtime.PresentationFPS = 60
			want.Runtime.PresentationMode = "smooth"
			want.Runtime.FloatingGeometry = true
		}
		if u.popup || !reflect.DeepEqual(d.Config, want) {
			t.Fatal("unexpected changes", id, d.Config.Runtime)
		}
		disk, err := configuration.Load(d.Path)
		if err != nil || disk.Config.Runtime.PresentationMode != "smooth" {
			t.Fatal("premature save", err)
		}
	}
	// After either preset, an explicit Framerate edit restores the accepted cap.
	u.activate(row("legacy_defaults"))
	press(tcell.KeyEnter)
	if value := u.Table.GetCell(row("presentation"), 1).Text; value != "10fps (Legacy)" {
		t.Fatal(value)
	}
	u.activate(row("presentation"))
	if d.Config.Runtime.PresentationFPS != 60 || d.Config.Runtime.PresentationMode != "smooth" {
		t.Fatal("Framerate did not restore smooth motion at 60 FPS")
	}
	if err = u.saveSession(); err != nil {
		t.Fatal(err)
	}
	disk, err := configuration.Load(d.Path)
	if err != nil || disk.Config.Runtime != d.Config.Runtime {
		t.Fatal("reopen", err)
	}
}
