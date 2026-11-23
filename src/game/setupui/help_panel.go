package setupui

import (
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
	"gunship2000-assets/game/controllers"
)

// The preview has its own bottom-anchored band; text wrapping never owns its y.
type helpPanel struct {
	*tview.Box
	text              *tview.TextView
	controls          bool
	swapYawRoll       func() bool
	buttonPreviewAxis string
	live              func() (controllers.Profile, controllers.State, bool)
}

func newHelpPanel(text *tview.TextView) *helpPanel {
	b := tview.NewBox().SetBorder(true).SetTitle(Title).SetBorderColor(white).SetBackgroundColor(blue)
	return &helpPanel{Box: b, text: text}
}
func (h *helpPanel) Draw(s tcell.Screen) {
	h.Box.DrawForSubclass(s, h)
	x, y, w, height := h.GetInnerRect()
	if w < 1 || height < 1 {
		return
	}
	band := 0
	if h.controls && height >= 7 && w >= 44 {
		band = 6
	}
	textHeight := height - band
	if h.controls && band == 0 {
		textHeight = max(0, height-1)
	}
	h.text.SetRect(x, y, w, textHeight)
	if textHeight > 0 {
		h.text.Draw(s)
	}
	if band > 0 {
		h.drawControls(s, x, y+height-band, w)
	} else if h.controls {
		tview.Print(s, "Controls preview needs more room", x, y+height-1, w, tview.AlignLeft, yellow)
	}
}
