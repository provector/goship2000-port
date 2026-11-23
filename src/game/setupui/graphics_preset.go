package setupui

import (
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

func (u *UI) confirmGraphicsPreset(id string) {
	target := map[string]string{"render_scale": "1", "presentation": "stepped", "floating": "off"}
	if id == "modern_defaults" {
		target = map[string]string{"render_scale": "2", "presentation": "smooth", "floating": "on"}
	}
	proposed := u.Doc.Config
	rows := [][3]string{{"Setting", "Current value", "Proposed value"}}
	for _, setting := range u.settings {
		value, selected := target[setting.ID]
		if !selected {
			continue
		}
		if err := setting.Set(&proposed, value); err != nil {
			u.message(err.Error())
			return
		}
		rows = append(rows, [3]string{u.text(setting.ID).Label, u.displayValue(setting, &u.Doc.Config), u.displayValue(setting, &proposed)})
	}
	if err := proposed.Validate(); err != nil {
		u.message(err.Error())
		return
	}

	table := tview.NewTable().SetSeparator(' ')
	table.SetBackgroundColor(amber)
	for row, values := range rows {
		for col, value := range values {
			cell := tview.NewTableCell(tview.Escape(value)).SetExpansion(1).SetTextColor(white)
			if row == 0 {
				cell.SetTextColor(yellow)
			}
			table.SetCell(row, col, cell)
		}
	}
	buttons := tview.NewForm().SetButtonsAlign(tview.AlignCenter).SetButtonTextColor(yellow).SetButtonBackgroundColor(blue)
	buttons.SetBackgroundColor(amber)
	buttons.AddButton("Apply", func() {
		u.Doc.Config.Runtime = proposed.Runtime
		for i := range u.settings {
			u.row(i)
		}
		u.closePopup()
	}).AddButton("Cancel", u.closePopup).SetCancelFunc(u.closePopup)
	buttonArrows(buttons)
	u.presetButtons = buttons
	u.App.EnableMouse(true)
	panel := tview.NewFlex().SetDirection(tview.FlexRow).AddItem(table, 4, 0, false).AddItem(buttons, 3, 0, true)
	panel.SetBackgroundColor(amber).SetBorder(true).SetTitle(u.text(id).Label)
	u.overlay(panel, 78, 9)
}

// Match tview.Modal's directional navigation on plain Form buttons.
// Text fields retain their own cursor keys.
func buttonArrows(form *tview.Form) {
	for i := 0; i < form.GetButtonCount(); i++ {
		form.GetButton(i).SetInputCapture(func(e *tcell.EventKey) *tcell.EventKey {
			switch e.Key() {
			case tcell.KeyRight, tcell.KeyDown:
				return tcell.NewEventKey(tcell.KeyTab, 0, tcell.ModNone)
			case tcell.KeyLeft, tcell.KeyUp:
				return tcell.NewEventKey(tcell.KeyBacktab, 0, tcell.ModNone)
			}
			return e
		})
	}
}
