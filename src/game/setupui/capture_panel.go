package setupui

import (
	"fmt"
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
	"gunship2000-assets/game/controllers"
)

type capturePanel struct {
	*tview.Box
	u *UI
}

func newCapturePanel(u *UI) *capturePanel {
	b := tview.NewBox().SetBorder(true).SetTitle("Move an axis or press a control").SetBackgroundColor(amber).SetBorderColor(yellow)
	return &capturePanel{b, u}
}
func (p *capturePanel) SetText(_ string) {} // state is read at draw, including re-arm/resize
func (p *capturePanel) Draw(s tcell.Screen) {
	p.Box.DrawForSubclass(s, p)
	x, y, w, h := p.GetInnerRect()
	if w < 1 || h < 1 {
		return
	}
	c := p.u.controllers
	at := y
	line := func(text string, fg, bg tcell.Color) {
		if at >= y+h {
			return
		}
		for i := 0; i < w; i++ {
			s.SetContent(x+i, at, ' ', nil, tcell.StyleDefault.Foreground(fg).Background(bg))
		}
		tview.Print(s, tview.Escape(text), x, at, w, tview.AlignCenter, fg)
		at++
	}
	wrapped := func(text string, fg, bg tcell.Color) {
		for _, v := range tview.WordWrap(text, w) {
			line(v, fg, bg)
		}
	}
	if c.wizard {
		line(p.u.wizardStep(), yellow, amber)
	}
	assigned := p.u.bindingText(c.captureID)
	if assigned == "<Unassigned>" {
		line(assigned, yellow, blue)
	} else {
		wrapped("Assigned: "+assigned, yellow, blue)
	}
	line("", yellow, amber)
	wrapped(p.u.captureAction(), white, amber)
	prompt := "Release/recenter the used control to re-arm"
	if c.capture != nil {
		prompt = c.capture.Prompt
	}
	wrapped(prompt, yellow, amber)
	line("", yellow, amber)
	esc := "Esc: cancel (keep old)"
	if c.wizard {
		esc = "Esc: skip (keep old)"
	}
	wrapped(esc+"   Space: clear"+map[bool]string{true: " and advance", false: ""}[c.wizard], yellow, amber)
}

func (u *UI) wizardStep() string {
	return fmt.Sprintf("Wizard %d/%d", u.controllers.step+1, len(controllers.Basic))
}
func (u *UI) captureAction() string { return basicLabel(u.controllers.captureID) }
