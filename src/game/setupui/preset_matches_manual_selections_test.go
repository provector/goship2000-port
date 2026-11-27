package setupui

import (
	"reflect"

	"testing"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

func TestPresetMatchesManualSelections(t *testing.T) {
	for _, modern := range []bool{false, true} {
		manual, preset := uiUI(t), uiUI(t)
		for _, u := range []*UI{manual, preset} {
			cfg := &u.Doc.Config
			cfg.WindowWidth, cfg.WindowHeight = 1280, 1024
			cfg.Fullscreen, cfg.SimplifiedFields, cfg.DrawFPSCPU = true, true, true
			cfg.Runtime.SimulationResolution = "1280x1024"
			cfg.Runtime.PresentationMode = "smooth"
			if modern {
				cfg.Runtime.PresentationMode = "stepped"
			}
			cfg.Runtime.FloatingGeometry = !modern
			cfg.Runtime.PresentationFPS = 37
			u.open("graphics")
		}
		choice, id := 0, "legacy_defaults"
		if modern {
			choice, id = 1, "modern_defaults"
		}
		manual.activate(settingRow(t, manual, "render_scale"))
		list := manual.App.GetFocus().(*tview.List)
		list.SetCurrentItem(choice)
		list.InputHandler()(tcell.NewEventKey(tcell.KeyEnter, 0, 0), manual.focusForTest)
		manual.activate(settingRow(t, manual, "presentation"))
		manual.activate(settingRow(t, manual, "floating"))
		preset.activate(settingRow(t, preset, id))
		preset.App.GetFocus().InputHandler()(tcell.NewEventKey(tcell.KeyEnter, 0, 0), preset.focusForTest)
		if !reflect.DeepEqual(manual.Doc.Config, preset.Doc.Config) {
			t.Fatalf("modern=%t: manual %+v preset %+v", modern, manual.Doc.Config.Runtime, preset.Doc.Config.Runtime)
		}
	}
}

func (u *UI) focusForTest(p tview.Primitive) { u.App.SetFocus(p) }
