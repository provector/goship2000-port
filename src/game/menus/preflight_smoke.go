package menus

import (
	"fmt"
)

// This opt-in development scenario sends semantic actions only to our own
// controller. Its source-derived wingmen and save directory are disposable.
type preflightSmokeSequence struct {
	Step, Wait int
	Err        error
	Trace      []menuSmokeRecord
}

func (s *preflightSmokeSequence) Next(c *FrontEnd) Actions {
	expected := []Screen{ScreenOffice, ScreenBriefing, ScreenOutfit, ScreenOutfit, ScreenOutfit, ScreenOrdnance, ScreenOrdnance, ScreenOrdnance, ScreenOrdnance, ScreenOutfit, ScreenBriefing, ScreenSettings, ScreenSettings, ScreenSettings, ScreenQuit, ScreenSettings, ScreenSettings, ScreenSettings, ScreenBriefing, ScreenPapers, ScreenOffice, ScreenQuit}
	if s.Step >= len(expected) || c.Screen != expected[s.Step] {
		return Actions{}
	}
	s.Wait++
	if s.Wait < 24 {
		return Actions{X: c.pointerX, Y: c.pointerY, Inside: c.pointerInside}
	}
	s.Wait = 0
	step := s.Step
	s.Step++
	s.Trace = append(s.Trace, menuSmokeRecord{step, c.Screen, c.Store.State().Revision})
	click := func(key int) Actions {
		x, y, ok := missionActionPoint(c, key)
		if !ok {
			s.Err = fmt.Errorf("preflight smoke missing action%d on%s", key, c.Screen)
		}
		return Actions{X: x, Y: y, Inside: ok, Selector1: ok, PointerPressed: ok}
	}
	ord := func(action string) Actions {
		for i, row := range c.Mission.Ordnance.Choices {
			if row.Action == action && row.Enabled {
				return click(i)
			}
		}
		s.Err = fmt.Errorf("preflight smoke missing ordnance action%s", action)
		return Actions{}
	}
	switch step {
	case 0:
		for _, a := range c.Office.Assets.Metadata.Areas {
			if a.Key == 4 {
				return Actions{X: a.Rectangle[0] + 2, Y: a.Rectangle[1] + 2, Inside: true, Selector1: true, PointerPressed: true}
			}
		}
	case 1:
		return click(1)
	case 2:
		return Actions{X: 120, Y: 20, Inside: true, Text: []rune{'H'}}
	case 3:
		return Actions{Cancel: true}
	case 4:
		return click(1)
	case 5:
		return ord("aircraft")
	case 6:
		for i, row := range c.Mission.Ordnance.Choices {
			if row.Action == "replace" && row.Value == int(c.Mission.ReadinessRows[0][0]) {
				return click(i)
			}
		}
	case 7:
		return ord("back")
	case 8:
		return ord("back")
	case 9:
		return click(6)
	case 10:
		return click(3)
	case 11:
		return click(10)
	case 12:
		return click(11)
	case 13:
		return Actions{Quit: true}
	case 14:
		return Actions{Text: []rune{'N'}}
	case 15:
		return click(1)
	case 16:
		return click(9)
	case 17:
		return Actions{Cancel: true}
	case 18:
		c.Mission.Page = 3
		return click(2)
	case 19:
		return click(5)
	case 20:
		return Actions{Quit: true}
	case 21:
		return Actions{Text: []rune{'Y'}}
	}
	s.Err = fmt.Errorf("preflight smoke action%d unresolved", step)
	return Actions{}
}
