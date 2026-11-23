package menus

import "fmt"

// Own native-port actions on scratch saves. Continuous hover is deliberate:
// original settings feedback clears when the pointer leaves its AREA.
const renderingSmokeSteps = 69

type renderingSmokeSequence struct {
	Step, Wait, Hover int
	Err               error
	Trace             []menuSmokeRecord
}

func (s *renderingSmokeSequence) Next(c *FrontEnd) Actions {
	// Admitted Single/Training now reaches debrief/office after Alt+E. Older
	// unsupported fixtures still return directly to briefing. Drive the real
	// result acknowledgement and new office preparation before the reentry probe.
	if (s.Step == 19 || s.Step == 67) && (c.Screen == ScreenDebrief || c.Screen == ScreenFilms || c.Screen == ScreenOffice) {
		s.Wait++
		if s.Wait < 24 {
			return Actions{}
		}
		s.Wait = 0
		if c.Screen == ScreenDebrief {
			return Actions{Enter: true}
		}
		if c.Screen == ScreenFilms {
			r := c.filmArea(18)
			return Actions{X: r[0] + 2, Y: r[1] + 2, Inside: true, Selector1: true, PointerPressed: true}
		}
		for _, a := range c.Office.Assets.Metadata.Areas {
			if a.Key == 4 {
				return Actions{X: a.Rectangle[0] + 2, Y: a.Rectangle[1] + 2, Inside: true, Selector1: true, PointerPressed: true}
			}
		}
		s.Err = fmt.Errorf("scene smoke missing office reentry area")
		return Actions{}
	}
	expected := []Screen{ScreenOffice, ScreenBriefing, ScreenSettings, ScreenSettings, ScreenSettings, ScreenSettings, ScreenSettings, ScreenSettings, ScreenSettings, ScreenSettings, ScreenSettings, ScreenSceneInitialization, ScreenQuit, ScreenScene, ScreenQuit, ScreenScene, ScreenQuit, ScreenScene, ScreenQuit, ScreenBriefing, ScreenSettings, ScreenSettings, ScreenSettings}
	for i := 23; i <= 65; i++ {
		expected = append(expected, ScreenScene)
	}
	expected = append(expected, ScreenQuit, ScreenBriefing, ScreenQuit)
	hover := func() Actions {
		if c.Screen == ScreenScene {
			if s.Step == 41 && c.Rendering != nil && c.Rendering.GPS != nil {
				return Actions{SceneDirections: [4]bool{false, true, false, false}}
			}
			if s.Step == 32 {
				return Actions{SceneDirections: [4]bool{false, false, true, false}}
			}
			if s.Step == 33 {
				return Actions{SceneDirections: [4]bool{false, true, false, false}}
			}
		}
		if c.Screen == ScreenSettings {
			x, y, ok := missionActionPoint(c, s.Hover)
			return Actions{X: x, Y: y, Inside: ok}
		}
		return Actions{}
	}
	if s.Step >= len(expected) || c.Screen != expected[s.Step] {
		return hover()
	}
	// These are controller-state checks, not merely a count of actions.
	if s.Step >= 38 && s.Step <= 45 || s.Step == 51 {
		if c.Rendering == nil || c.Rendering.GPS == nil {
			s.Err = fmt.Errorf("GPS missing at scene smoke step%d", s.Step)
			return Actions{}
		}
	}
	if s.Step == 46 || s.Step == 52 {
		if c.Rendering.GPS != nil {
			s.Err = fmt.Errorf("GPS did not release atstep%d", s.Step)
			return Actions{}
		}
	}
	s.Wait++
	hold := 24
	if s.Step == 11 {
		hold = 8
	}
	if s.Step == 13 || s.Step == 15 || s.Step >= 23 && s.Step <= 36 || s.Step >= 52 && s.Step <= 65 {
		hold = 48
	}
	if s.Wait < hold {
		return hover()
	}
	s.Wait = 0
	step := s.Step
	s.Step++
	s.Trace = append(s.Trace, menuSmokeRecord{step, c.Screen, c.Store.State().Revision})
	// Exercise real source keyboard actions between normal scene entry and the
	// existing view/quit route. This is our native smoke, not original input.
	if step >= 23 && step <= 36 {
		switch {
		case step == 23:
			return Actions{FlightCommands: []int{2}}
		case step >= 24 && step <= 31:
			return Actions{FlightCommands: []int{0x100}}
		case step == 35:
			return Actions{FlightPause: true}
		case step == 36:
			return Actions{AnyKey: true}
		default:
			return Actions{}
		}
	}
	if step >= 37 && step <= 51 {
		switch step {
		case 37, 50:
			return Actions{GPSOpen: true}
		case 38, 43:
			return Actions{GPSKey: "f"}
		case 39, 44:
			return Actions{Selector1: true, Inside: true, X: 135, Y: 90}
		case 40:
			return Actions{GPSKey: "space"}
		case 41:
			return Actions{Selector1: true}
		case 42, 51:
			return Actions{Cancel: true}
		case 45:
			return Actions{ViewKey: 1}
		case 46:
			return Actions{DefenseCommands: []int{0, 1}}
		case 47:
			return Actions{DefenseCommands: []int{2, 3}}
		case 48:
			return Actions{Backspace: true}
		case 49:
			return Actions{WaypointNext: true}
		}
	}
	if step >= 52 {
		step -= 15
	}
	if step >= 37 {
		step -= 14
	}
	click := func(key int) Actions {
		x, y, ok := missionActionPoint(c, key)
		if !ok {
			s.Err = fmt.Errorf("scene smoke missing action%d on%s", key, c.Screen)
		}
		return Actions{X: x, Y: y, Inside: ok, Selector1: ok, PointerPressed: ok}
	}
	switch step {
	case 0:
		for _, a := range c.Office.Assets.Metadata.Areas {
			if a.Key == 4 {
				return Actions{X: a.Rectangle[0] + 2, Y: a.Rectangle[1] + 2, Inside: true, Selector1: true, PointerPressed: true}
			}
		}
		s.Err = fmt.Errorf("scene smoke has no office door")
	case 1:
		s.Hover = 2
		return click(3)
	case 2, 3, 4, 5:
		return click(2) // Cycle back to POOR through real settings actions.
	case 6:
		s.Hover = 5
		return hover()
	case 7:
		return click(5) // Same-AREA activation: detailed avoidance OFF.
	case 8, 20:
		s.Hover = 9
		return hover()
	case 9, 10, 21, 22:
		return click(9)
	case 11, 13, 38:
		return Actions{Quit: true}
	case 12, 14:
		return Actions{Text: []rune{'N'}}
	case 15, 17, 36:
		return Actions{EndMission: true}
	case 16:
		return Actions{Cancel: true}
	case 18, 37, 39:
		return Actions{ConfirmQuit: true}
	case 19:
		s.Hover = 9
		return click(3)
	case 23:
		return Actions{ViewKey: 3}
	case 24:
		return Actions{ViewKey: 4}
	case 25:
		return Actions{ViewKey: 5}
	case 26:
		return Actions{ViewKey: 7}
	case 27:
		return Actions{ViewKey: 1}
	case 28, 29, 30, 31:
		return Actions{CycleArrangement: true}
	case 32:
		return Actions{ViewKey: 2}
	case 33:
		return Actions{ViewKey: 6}
	case 34:
		return Actions{ViewKey: 8}
	case 35:
		return Actions{ViewKey: 9}
	}
	return Actions{}
}
