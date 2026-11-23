package setupui

import (
	"github.com/rivo/tview"
	"gunship2000-assets/game/controllers"
)

// Reset is a session edit, never a direct file write. Main Discard retains the
// original bytes; other profiles and device/mechanism preferences are untouched.
func (u *UI) resetBindings() {
	c := u.controllers
	if c.err != nil {
		u.message(c.err.Error())
		return
	}
	p, ok := c.session.Profiles[c.session.Selected]
	if !ok {
		u.message("Select a controller first.")
		return
	}
	m := tview.NewModal().SetText("Clear all bindings for " + p.Device.Name + "? Device and preferences are retained. Main Save & Exit writes the change.").AddButtons([]string{"Reset bindings", "Cancel"}).SetDoneFunc(func(i int, _ string) {
		if i == 0 {
			p.Bindings = map[string]controllers.Binding{}
			p.AxisButtons = nil
			p.AxisConvention = "native-v1"
			c.session.Profiles[p.ID] = p
			c.session.Version = controllers.RequiresVersion(c.session)
		}
		u.closePopup()
		u.open("controls")
	})
	m.SetBackgroundColor(amber).SetTextColor(yellow).SetButtonBackgroundColor(blue)
	u.overlay(m, 64, 9)
}
