package setupui

import "gunship2000-assets/internal/setupwindow"

func (u *UI) RunWindow() error {
	return setupwindow.Run(Title, u.App, u.Run, u.ControllerBackend, u.applyControllerSnapshot, func() {
		c := u.controllers
		if c.capture != nil || c.rearm != nil || c.pending != nil {
			c.capture = nil
			c.rearm = nil
			c.pending = nil
			c.wizard = false
			u.closeControllerPopup()
			u.controllerHelp("Capture cancelled when SETUP lost focus. Your previous binding is unchanged.")
		}
	}, func() {
		if !u.busy {
			u.App.Stop()
		}
	})
}
