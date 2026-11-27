package setupui

import (
	"fmt"
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
	"gunship2000-assets/game/controllers"
	"math"
)

// Cross origins match the fixed Q1.4 band; tooltip wrapping cannot move them.
func (h *helpPanel) drawControls(s tcell.Screen, x, y, w int) {
	var p controllers.Profile
	var state controllers.State
	live := false
	if h.live != nil {
		p, state, live = h.live()
	}
	value := func(id string) float64 {
		role := id
		if h.swapYawRoll != nil && h.swapYawRoll() {
			if id == "flight.roll" {
				id = "flight.yaw"
			} else if id == "flight.yaw" {
				id = "flight.roll"
			}
		}
		if !live {
			return 0
		}
		if id == h.buttonPreviewAxis {
			return controllers.ButtonPair(p, id).Value(state, false)
		}
		source, _ := controllers.CollectiveSource(p)
		v := controllers.AxisValue(p, id, state, id == "flight.collective" && (controllers.CollectiveMode(p) == "absolute" || source == "slider"))
		if p.AxisConvention == "native-v1" {
			if p.Bindings[id].Invert {
				v = -v
			}
			if p.Bindings[role].Invert {
				v = -v
			}
		}
		return v
	}
	for n, ids := range [][2]string{{"flight.roll", "flight.pitch"}, {"flight.yaw", "flight.collective"}} {
		cx, cy := x+9+19*n, y+3
		vertical := "Pitch"
		horizontal := "Roll/Bank"
		if n == 1 {
			vertical = "Collective"
			horizontal = "Yaw/Rotor"
		}
		tview.Print(s, vertical, cx-len(vertical)/2, y, 14, tview.AlignLeft, white)
		for dy := -2; dy <= 2; dy++ {
			s.SetContent(cx, cy+dy, '░', nil, tcell.StyleDefault.Foreground(white).Background(blue))
		}
		for dx := -4; dx <= 4; dx++ {
			s.SetContent(cx+dx, cy, '░', nil, tcell.StyleDefault.Foreground(white).Background(blue))
		}
		tview.Print(s, horizontal, cx-len(horizontal)/2, y+6, 16, tview.AlignLeft, white)
		vx, vy := value(ids[0]), value(ids[1])
		px := cx + int(math.Round(vx*4))
		py := cy + int(math.Round(vy*2))
		if n == 1 {
			py = cy - int(math.Round(vy*2))
		}
		s.SetContent(px, py, '█', nil, tcell.StyleDefault.Foreground(yellow).Background(blue))
	}
	if w < 72 {
		return
	}

	if live && p.Device.Hats > 0 && len(state.Hats) > 0 {
		cx, cy := x+42, y+3
		tview.Print(s, "H1", cx-1, y, 3, tview.AlignLeft, white)
		for _, v := range []struct {
			d      uint8
			dx, dy int
			glyph  rune
		}{{1, 0, -2, '↑'}, {2, 3, 0, '→'}, {4, 0, 2, '↓'}, {8, -3, 0, '←'}, {3, 2, -1, '↗'}, {6, 2, 1, '↘'}, {9, -2, -1, '↖'}, {12, -2, 1, '↙'}} {
			style := tcell.StyleDefault.Foreground(white).Background(blue)
			if state.Hats[0] == v.d {
				style = style.Foreground(blue).Background(yellow).Bold(true)
			}
			s.SetContent(cx+v.dx, cy+v.dy, v.glyph, nil, style)
		}
		fg := tcell.ColorDarkGray.TrueColor()
		glyph := '·'
		if state.Hats[0] == 0 {
			fg = yellow
			glyph = '●'
		}
		s.SetContent(cx, cy, glyph, nil, tcell.StyleDefault.Foreground(fg).Background(blue))
	}
	tview.Print(s, "Physical buttons", x+52, y, 24, tview.AlignLeft, white)
	for i := 0; i < 12; i++ {
		bx, by := x+52+(i%4)*5, y+1+i/4
		fg, bg := tcell.ColorDarkGray.TrueColor(), blue
		held := live && i < len(state.Buttons) && state.Buttons[i]
		available := live && i < p.Device.Buttons
		if available {
			fg = white
		}
		if held {
			fg, bg = blue, yellow
		}
		label := fmt.Sprintf("[%02d]", i+1)
		for j, ch := range label {
			s.SetContent(bx+j, by, ch, nil, tcell.StyleDefault.Foreground(fg).Background(bg).Bold(held))
		}
		if held {
			s.SetContent(bx+4, by, '█', nil, tcell.StyleDefault.Foreground(yellow).Background(blue))
		}
	}
}
