package setupui

import (
	"path/filepath"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
	"gunship2000-assets/game/configuration"
	"gunship2000-assets/game/managedassets"
)

func (u *UI) selectInstallation(row int, detected []managedassets.Installation) {
	dir := u.Doc.InstallDirectory()
	draft := u.Doc.Config
	preferred, admitted := managedassets.PreferredInstallation(draft, dir, detected)
	custom := draft.OriginalCustom
	if draft.OriginalSource == configuration.OriginalCustom || draft.OriginalSource == "" {
		custom = draft.Original
	}
	if custom == "" {
		custom = filepath.Join(dir, configuration.DefaultOriginal)
	}
	list := tview.NewList().ShowSecondaryText(false).SetHighlightFullLine(true).
		SetMainTextStyle(tcell.StyleDefault.Foreground(yellow).Background(amber)).
		SetSelectedStyle(tcell.StyleDefault.Foreground(yellow).Background(blue).Bold(true))
	list.SetBackgroundColor(amber)
	field := tview.NewInputField().SetLabel("Custom path ").SetLabelColor(yellow).SetFieldTextColor(white).SetFieldBackgroundColor(blue).SetText(custom)
	field.SetBackgroundColor(amber)
	apply, cancel := tview.NewButton("Apply"), tview.NewButton("Cancel")
	for _, button := range []*tview.Button{apply, cancel} {
		button.SetStyle(tcell.StyleDefault.Foreground(yellow).Background(amber)).
			SetActivatedStyle(tcell.StyleDefault.Foreground(yellow).Background(blue).Bold(true))
	}
	detail := tview.NewTextView().SetTextColor(white).SetWrap(true).SetWordWrap(true)
	detail.SetBackgroundColor(amber)
	var choices []managedassets.Installation
	selected := -1
	for _, i := range detected {
		if preferred.Source != configuration.OriginalCustom && managedassets.SameInstallation(i.Path, preferred.Path) {
			// An explicitly stored marketplace identity takes precedence
			// when both stores resolve to the same physical installation.
			i = preferred
			selected = len(choices)
		}
		choices = append(choices, i)
	}
	if preferred.Source != configuration.OriginalCustom && admitted && selected < 0 {
		selected = len(choices)
		choices = append(choices, preferred)
	}
	customIndex := len(choices)
	choices = append(choices, managedassets.Installation{Source: configuration.OriginalCustom, Path: custom})
	if selected < 0 {
		selected = customIndex
	}
	for i, item := range choices {
		label := item.String()
		if i == customIndex {
			label = "Custom"
		}
		list.AddItem(tview.Escape(label), "", 0, nil)
	}
	showSelection := func(i int) {
		if i == customIndex {
			detail.SetText("Enter edits the Custom path. Tab reaches Apply and Cancel.").ScrollToBeginning()
			return
		}
		detail.SetText(choices[i].String()).ScrollToBeginning()
	}
	list.SetChangedFunc(func(i int, _, _ string, _ rune) { showSelection(i) })
	list.SetCurrentItem(selected)
	showSelection(selected)
	apply.SetSelectedFunc(func() {
		i := choices[list.GetCurrentItem()]
		draft.OriginalCustom = field.GetText()
		if i.Source == configuration.OriginalCustom {
			if draft.OriginalCustom == "" {
				detail.SetText("Enter a Custom path before applying.")
				u.App.SetFocus(field)
				return
			}
			i.Path = filepath.Clean(draft.OriginalCustom)
		}
		draft.OriginalSource, draft.Original = i.Source, i.Path
		if err := draft.Validate(); err != nil {
			detail.SetText(err.Error())
			return
		}
		u.Doc.Config = draft
		u.closePopup()
		u.row(row)
		u.showHelp("original")
	})
	cancel.SetSelectedFunc(u.closePopup)
	list.SetSelectedFunc(func(i int, _, _ string, _ rune) {
		if i == customIndex {
			u.App.SetFocus(field)
		} else {
			u.App.SetFocus(apply)
		}
	})
	controls := []tview.Primitive{list, field, apply, cancel}
	for index, control := range controls {
		capture := func(event *tcell.EventKey) *tcell.EventKey {
			i, next := list.GetCurrentItem(), index
			switch event.Key() {
			case tcell.KeyPgUp, tcell.KeyPgDn:
				if index != 0 {
					return event
				}
				detail.InputHandler()(event, func(p tview.Primitive) { u.App.SetFocus(p) })
				return nil
			case tcell.KeyTab:
				next = (index + 1) % len(controls)
			case tcell.KeyBacktab:
				next = (index + len(controls) - 1) % len(controls)
			case tcell.KeyLeft, tcell.KeyRight:
				if index < 2 {
					return event
				}
				if index == 2 {
					next = 3
				} else {
					next = 2
				}
			case tcell.KeyEnter:
				if index != 1 {
					return event
				}
				next = 2
			default:
				return event
			}
			if next == 1 && i != customIndex {
				if event.Key() == tcell.KeyBacktab {
					next = 0
				} else {
					next = 2
				}
			}
			if next == 1 {
				list.SetCurrentItem(customIndex)
			}
			u.App.SetFocus(controls[next])
			return nil
		}
		switch v := control.(type) {
		case *tview.List:
			v.SetInputCapture(capture)
		case *tview.InputField:
			v.SetInputCapture(capture)
		case *tview.Button:
			v.SetInputCapture(capture)
		}
	}
	field.SetFocusFunc(func() { list.SetCurrentItem(customIndex) })
	buttons := tview.NewFlex().AddItem(nil, 0, 1, false).AddItem(apply, 12, 0, false).AddItem(nil, 2, 0, false).AddItem(cancel, 12, 0, false).AddItem(nil, 0, 1, false)
	buttons.SetBackgroundColor(amber)
	panel := tview.NewFlex().SetDirection(tview.FlexRow).AddItem(list, 0, 1, true).AddItem(detail, 4, 0, false).AddItem(field, 1, 0, false).AddItem(nil, 1, 0, false).AddItem(buttons, 1, 0, false)
	// Flex normally leaves spacer cells transparent. This is a dialog
	// surface; paint the complete rectangle before drawing its children.
	panel.Box = tview.NewBox()
	panel.SetBorder(true).SetTitle("Original Installation").SetBackgroundColor(amber)
	u.originalPicker = panel
	u.App.EnableMouse(true)
	u.overlay(panel, 76, min(19, max(12, len(choices)+10)))
}
