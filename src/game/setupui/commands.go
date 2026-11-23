package setupui

import (
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

func (u *UI) commands() {
	v := tview.NewTextView().SetDynamicColors(true).SetWrap(true).SetWordWrap(true).SetScrollable(true)
	v.SetBackgroundColor(amber)
	v.SetBorder(true).SetTitle("Explicit diagnostic commands - arrows scroll, Esc closes").SetBackgroundColor(amber)
	v.SetTextColor(tcell.ColorWhite)
	v.SetText(`Run these tools from a terminal when you need them.

prepare             Prepare assets in a new folder.
films identify       Read a recording’s ID.
films verify         Check a recording.
films inspect-prefix Check how much of an interrupted recording is intact.
films recover        Write a recovery receipt for an eligible older film.
import-campaign      Import an original Campaign.
dev                 Open a preview or capture an image.

Use -help with a command for its options.`)
	v.SetInputCapture(func(e *tcell.EventKey) *tcell.EventKey {
		if e.Key() == tcell.KeyEscape {
			u.closePopup()
			return nil
		}
		return e
	})
	u.overlay(v, 76, 21)
}
