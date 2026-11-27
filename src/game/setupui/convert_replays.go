package setupui

import (
	"github.com/rivo/tview"
	"gunship2000-assets/game/configuration"
	"gunship2000-assets/internal/legacyreplay"
	"path/filepath"
)

func (u *UI) convertReplays() {
	saved, e := configuration.LoadAt(u.Doc.Path, u.Doc.InstallDirectory())
	if e != nil {
		u.message(e.Error())
		return
	}
	dir := configuration.SavePath(saved.InstallDirectory(), filepath.Dir(saved.Path), saved.Config.Saves)
	view := tview.NewTextView().SetTextAlign(tview.AlignCenter).SetTextColor(white).SetText("\nChecking and converting recordings…")
	view.SetBackgroundColor(amber).SetBorder(true).SetBorderColor(yellow).SetTitle("Convert/Update Replays")
	u.overlay(view, 64, 5)
	u.busy = true
	u.writers.Add(1)
	go func() {
		result := func() string {
			defer u.writers.Done()
			return legacyreplay.ConvertLibrary(filepath.Join(dir, "replays")).String()
		}()
		u.App.QueueUpdateDraw(func() { u.busy = false; u.closePopup(); u.message(result) })
	}()
}
