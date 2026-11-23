package setupui

import (
	"gunship2000-assets/game/controllers"
)

func (u *UI) collectiveSettingValue(kind string) string {
	p, ok := u.controllers.session.Profiles[u.controllers.session.Selected]
	if !ok {
		return "<None>"
	}
	if kind == "collective_mode" {
		return collectiveModeLabel(controllers.CollectiveMode(p))
	}
	if p.CollectiveSource == "" {
		return "None"
	}
	return collectiveSourceLabel(&p)
}
func (u *UI) chooseCollective(source bool) {
	c := u.controllers
	p, ok := c.session.Profiles[c.session.Selected]
	if !ok {
		u.message("Select a controller first; settings belong to its profile.")
		return
	}
	keys := []string{"absolute", "relative", "relative_height"}
	labels := []string{"Absolute", "Relative", "Relative Height"}
	current := controllers.CollectiveMode(p)
	title := "COLLECTIVE MODE"
	if source {
		keys = []string{"", "centered", "slider"}
		labels = []string{"None", "Centered stick", "Slider"}
		current = p.CollectiveSource
		title = "COLLECTIVE SOURCE OVERRIDE"
	}
	values := []pickerValue{}
	for i, k := range keys {
		key, label := k, labels[i]
		values = append(values, pickerValue{Key: key, Label: label, Accept: func() {
			p := c.session.Profiles[c.session.Selected]
			if source {
				p.CollectiveSource = key
			} else {
				p.CollectiveMode = key
			}
			c.session.Profiles[p.ID] = p
			c.session.Version = controllers.RequiresVersion(c.session)
			u.closePopup()
			for i := range u.settings {
				u.row(i)
			}
			row, _ := u.Table.GetSelection()
			if row >= 0 && row < len(u.settings) {
				u.showHelp(u.settings[row].ID)
			}
		}})
	}
	picker := valuePicker(title, values, current)
	if !source {
		picker.SetChangedFunc(func(i int, _ string, _ string, _ rune) { u.controllerHelp(collectiveHelp(keys[i])) })
	}
	u.overlay(picker, 44, 7)
}
