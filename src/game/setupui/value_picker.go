package setupui

import (
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
	"strings"
)

type pickerValue struct {
	Key, Label string
	Accept     func()
}

// Value choices always focus their current draft identity after all entries are
// present. A missing identity only changes focus; it never commits a fallback.
func valuePicker(title string, values []pickerValue, current string) *tview.List {
	list := tview.NewList().ShowSecondaryText(false).SetHighlightFullLine(true).SetMainTextStyle(tcell.StyleDefault.Foreground(yellow).Background(amber).Bold(true)).SetSelectedStyle(tcell.StyleDefault.Foreground(yellow).Background(blue).Bold(true))
	list.SetBorder(true).SetTitle(tview.Escape(strings.ToUpper(title))).SetBackgroundColor(amber)
	selected := 0
	for i, v := range values {
		list.AddItem(tview.Escape(v.Label), "", 0, v.Accept)
		if v.Key == current {
			selected = i
		}
	}
	if len(values) > 0 {
		list.SetCurrentItem(selected)
	}
	return list
}
