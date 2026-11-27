package setupui

import (
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
	"gunship2000-assets/game/controllers"
)

const (
	controlsHelpRows    = 3
	controlsPreviewRows = 7                                                  // Top labels, five cross rows, bottom labels.
	controlsPanelRows   = 2 + controlsHelpRows + 1 + controlsPreviewRows + 1 // Borders and blank rows above/below the preview.
)

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
	textHeight := height
	showPreview := h.controls && height >= 1+1+controlsPreviewRows+1 && w >= 44
	if h.controls {
		if showPreview {
			textHeight = min(controlsHelpRows, height-controlsPreviewRows-2)
		} else {
			textHeight = max(0, height-1)
		}
	}
	h.text.SetRect(x, y, w, textHeight)
	if textHeight > 0 {
		h.text.Draw(s)
	}
	if showPreview {
		h.drawControls(s, x, y+textHeight+1, w)
	} else if h.controls {
		tview.Print(s, "Controls preview needs more room", x, y+height-1, w, tview.AlignLeft, yellow)
	}
}
