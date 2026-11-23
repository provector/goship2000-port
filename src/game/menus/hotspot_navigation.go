package menus

import assetdata "gunship2000-assets/game/assets"

// Illustrated menus use logical AREA hotspots. Keep the selected point
// in logical coordinates so the cursor, caption and selector use the same hit.
func (c *FrontEnd) navigateHotspots(a Actions) Actions {
	moved := a.PointerMoved
	if a.PointerRelative {
		// A controller-owned pointer can echo our last logical position even
		// when neither the mouse nor the stick moved.
		moved = a.PointerDX != 0 || a.PointerDY != 0
	}
	// A page turn consumes input, but a stationary keyboard cursor remains
	// over the page-turn zone when the new paper finishes scrolling.
	if c.Screen == ScreenPaperTransition && c.Mission.PaperTransition.Motion == paperNext && c.keyboardHotspot == ScreenPapers && !moved && !a.PointerPressed && !a.FlightFocusLost {
		a.X, a.Y, a.Inside = c.Office.PointerX, c.Office.PointerY, true
		return a
	}
	if (c.Screen != ScreenOffice && c.Screen != ScreenBriefing && c.Screen != ScreenSettings && c.Screen != ScreenPapers) || c.Draft != nil || c.fade != nil || a.FlightFocusLost {
		c.keyboardHotspot = ""
		return a
	}
	if c.keyboardHotspot != c.Screen || moved || a.PointerPressed {
		c.keyboardHotspot = ""
	}
	if c.keyboardHotspot == c.Screen {
		a.X, a.Y, a.Inside = c.Office.PointerX, c.Office.PointerY, true
	}
	step := 0
	if a.Left || a.Up {
		step--
	}
	if a.Right || a.Down {
		step++
	}
	if step != 0 && !a.PointerPressed {
		areas := c.Office.Assets.Metadata.Areas
		switch c.Screen {
		case ScreenBriefing:
			v := c.Mission.View
			areas = v.Assets.Metadata.Screens[v.Mission.StableScene+".SCN"].Areas
		case ScreenSettings:
			areas = c.Office.Assets.Missions.Metadata.Screens["REALITY.SCN"].Areas
		case ScreenPapers:
			areas = c.Mission.View.paperAreas(c.Mission.Page)
		}
		if x, y, ok := nextHotspot(c.Screen, areas, a.X, a.Y, a.Inside, step); ok {
			c.keyboardHotspot = c.Screen
			a.X, a.Y, a.Inside = x, y, true
		}
	}
	if c.keyboardHotspot == c.Screen && a.Enter {
		a.Selector1 = true
	}
	return a
}

func nextHotspot(screen Screen, areas []assetdata.OfficeArea, x, y int, inside bool, step int) (int, int, bool) {
	current := -1
	for i := len(areas) - 1; inside && i >= 0; i-- {
		if insideRect(x, y, areas[i].Rectangle) {
			current = i
			break
		}
	}
	if current == -1 && step < 0 {
		current = 0
	}
	for range areas {
		current = (current + step + len(areas)) % len(areas)
		a := areas[current]
		// Office clock and desk decorations have no selectable action.
		if a.Key <= 0 || screen == ScreenSettings && a.Key > 11 || screen == ScreenOffice && (a.Key == 5 || a.Key == 7) {
			continue
		}
		r := a.Rectangle
		return r[0] + r[2]/2, r[1] + r[3]/2, true
	}
	return x, y, false
}
