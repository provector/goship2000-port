package setupui

import (
	"github.com/gdamore/tcell/v2"
	"strings"
	"testing"
)

func TestAnchoredControlsAndTerminalTarget(t *testing.T) {
	u := uiUI(t)
	u.open("controls")
	s := tcell.NewSimulationScreen("UTF-8")
	s.Init()
	defer s.Fini()
	s.SetSize(80, 25)
	for _, text := range []string{"Short help", strings.Repeat("Long wrapping help for a controller preview. ", 7)} {
		u.Help.SetText(text)
		u.App.GetBeforeDrawFunc()(s)
		u.Main.SetRect(0, 0, 80, 25)
		u.Main.Draw(s)
		for _, x := range []int{10, 29} {
			ch, _, _, _ := s.GetContent(x, 8)
			if ch != '█' {
				t.Fatalf("center moved x=%d got=%q", x, ch)
			}
		}
	}
	for _, h := range []int{6, 9, 12, 18} {
		s.SetSize(60, h)
		u.App.GetBeforeDrawFunc()(s)
		u.Main.SetRect(0, 0, 60, h)
		u.Main.Draw(s)
		_, y, _, height := u.helpPanel.GetRect()
		if y+height >= h {
			t.Fatal("help overlaps footer", h, height)
		}
	}
	u.open("logging")
	row := len(u.settings) - 1
	u.activate(row)
	if u.popup || u.Doc.Config.Logging.TerminalTarget != "system" {
		t.Fatal("not inline", u.Doc.Config.Logging)
	}
	u.activate(row)
	if u.Doc.Config.Logging.TerminalTarget != "custom" {
		t.Fatal(u.Doc.Config.Logging)
	}
	if e := u.Doc.Save(); e != nil {
		t.Fatal(e)
	}
}
