package menus

import (
	"fmt"
)

// Opt-in semantic smoke actions exercise our controller, never host input or
// the original game. It owns only the disposable development save.
type briefingSmokeSequence struct {
	Step, Wait int
	Err        error
	Trace      []menuSmokeRecord
}

func missionActionPoint(c *FrontEnd, key int) (int, int, bool) {
	if c.Screen == ScreenOrdnance && c.Mission != nil && c.Mission.Ordnance != nil {
		v := c.Mission.Ordnance
		if key < 0 || key >= len(v.Choices) || v.Choices[key].Action == "heading" {
			return 0, 0, false
		}
		r := v.rowRect(key)
		// This development driver supplies raw viewport motion to the same
		// constrained selector; it does not reset or bypass its input state.
		return c.pointerX, v.rowPointer.rawY + r[1] + r[3]/2 - 1 - v.rowPointer.y, true
	}
	for y := 0; y < 191; y++ {
		for x := 0; x < 320; x++ {
			if c.Hit(x, y) == key {
				return x, y, true
			}
		}
	}
	return 0, 0, false
}

func (s *briefingSmokeSequence) Next(c *FrontEnd) Actions {
	expected := []Screen{ScreenOffice, ScreenBriefing, ScreenPapers, ScreenPapers, ScreenPapers, ScreenPapers, ScreenBriefing, ScreenSettings, ScreenSettings, ScreenQuit, ScreenSettings, ScreenSettings, ScreenBriefing, ScreenPapers, ScreenOffice, ScreenQuit}
	if s.Step >= len(expected) || c.Screen != expected[s.Step] {
		return Actions{}
	}
	s.Wait++
	if s.Wait < 24 {
		return Actions{}
	}
	s.Wait = 0
	step := s.Step
	s.Step++
	s.Trace = append(s.Trace, menuSmokeRecord{step, c.Screen, c.Store.State().Revision})
	click := func(key int) Actions {
		x, y, ok := missionActionPoint(c, key)
		if !ok {
			s.Err = fmt.Errorf("briefing smoke: no source action%d on%s", key, c.Screen)
			return Actions{}
		}
		return Actions{X: x, Y: y, Inside: true, Selector1: true, PointerPressed: true}
	}
	switch step {
	case 0:
		for _, a := range c.Office.Assets.Metadata.Areas {
			if a.Key == 4 {
				return Actions{X: a.Rectangle[0] + 2, Y: a.Rectangle[1] + 2, Inside: true, Selector1: true, PointerPressed: true}
			}
		}
		s.Err = fmt.Errorf("office door AREA is unavailable")
	case 1, 12:
		return click(2)
	case 2, 3, 4:
		return click(3)
	case 5:
		return click(4)
	case 6:
		return click(3)
	case 7:
		return click(1)
	case 8, 14:
		return Actions{Quit: true}
	case 9:
		return Actions{Text: []rune{'N'}}
	case 10:
		return click(9)
	case 11:
		return Actions{Cancel: true}
	case 13:
		return click(5)
	case 15:
		return Actions{Text: []rune{'Y'}}
	}
	return Actions{}
}
