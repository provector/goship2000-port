package menus

// This opt-in developer scenario sends semantic actions to our own controller,
// not OS input or original-game calls. Normal startup never constructs it.
type menuSmokeSequence struct {
	Step, Wait int
	Trace      []menuSmokeRecord
	Err        error
}

type menuSmokeRecord struct {
	Step     int    `json:"step"`
	Screen   Screen `json:"screen_before"`
	Revision uint64 `json:"save_revision_before"`
}

func (s *menuSmokeSequence) Next(c *FrontEnd) Actions {
	expected := []Screen{ScreenOffice, ScreenRoster, ScreenRoster, ScreenRoster, ScreenRoster, ScreenOffice, ScreenTheatre, ScreenOffice, ScreenMission, ScreenMission, ScreenOffice, ScreenMission, ScreenOffice, ScreenRecovery, ScreenOffice, ScreenRecovery, ScreenOffice, ScreenQuit, ScreenOffice, ScreenQuit}
	if s.Step >= len(expected) || c.Screen != expected[s.Step] {
		return Actions{}
	}
	s.Wait++
	if s.Wait < 18 {
		return Actions{}
	}
	s.Wait = 0
	step := s.Step
	s.Step++
	s.Trace = append(s.Trace, menuSmokeRecord{step, c.Screen, c.Store.State().Revision})
	a := Actions{}
	click := func(x, y int) Actions {
		return Actions{X: x, Y: y, Inside: true, Selector1: true, PointerPressed: true, PointerReset: true}
	}
	switch step {
	case 0:
		a = click(184, 70)
	case 1:
		a = Actions{X: 30, Y: 90, Inside: true, Selector2: true, PointerPressed: true, PointerReset: true}
	case 2:
		a = Actions{Text: []rune{'X'}}
	case 3:
		a = Actions{Cancel: true}
	case 4:
		a = Actions{Cancel: true}
	case 5:
		a = click(270, 45)
	case 6:
		r := c.Menus.Theatres[3].Rectangle
		a = click(r[0]+2, r[1]+2)
	case 7, 10:
		a = click(100, 170)
	case 8:
		a = Actions{X: 100, Y: 92, Inside: true}
	case 9:
		a = Actions{Cancel: true}
	case 11:
		a = click(100, 92)
	case 12, 14:
		if step == 12 {
			// Explicit recovery fixture, never an ordinary player menu action.
			// The native scenario stands in for the unimplemented mission boundary.
			if err := c.Store.PrepareMissionSnapshot(); err != nil {
				s.Err = err
				return Actions{}
			}
		}
		a = click(300, 140)
	case 13:
		a = Actions{Cancel: true}
	case 15:
		a = Actions{Selector1: true}
	case 16:
		a = Actions{Quit: true}
	case 17:
		a = Actions{Text: []rune{'n'}}
	case 18:
		a = Actions{Close: true}
	case 19:
		a = Actions{Text: []rune{'Y'}}
	}
	return a
}
