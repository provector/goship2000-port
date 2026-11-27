package setupui

import (
	"testing"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

func TestInstallationButtonFocusOccupiesLabelRow(t *testing.T) {
	u := controllerUIForController(t)
	u.open("settings")
	u.selectInstallation(1, nil)
	press := func(k tcell.Key) {
		u.App.GetFocus().InputHandler()(tcell.NewEventKey(k, 0, 0), func(p tview.Primitive) { u.App.SetFocus(p) })
	}
	press(tcell.KeyEnter) // Custom path
	press(tcell.KeyTab)   // Apply
	for _, size := range [][2]int{{96, 30}, {72, 22}} {
		for _, label := range []string{"Apply", "Cancel"} {
			b, ok := u.App.GetFocus().(*tview.Button)
			if !ok || b.GetLabel() != label {
				t.Fatal("unexpected focus", u.App.GetFocus())
			}
			s := frame(t, u, size[0], size[1])
			u.Pages.SetRect(0, 0, size[0], size[1])
			u.Pages.Draw(s)
			x, y, width, height := b.GetRect()
			if height != 1 || width != 12 {
				t.Fatalf("%s rectangle %d x %d", label, width, height)
			}
			for _, test := range []struct {
				row        int
				background tcell.Color
			}{{y - 1, amber}, {y, blue}} {
				_, _, style, _ := s.GetContent(x+width/2, test.row)
				_, background, _ := style.Decompose()
				if background != test.background {
					t.Fatalf("%s row %d: background %v, want %v", label, test.row, background, test.background)
				}
			}
			s.Fini()
			press(tcell.KeyRight)
		}
	}
}
