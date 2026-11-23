package menus

// Leave a small non-selectable rim so confinement never turns a blank-paper
// click into a choice. Units are logical pixels, independent of window scaling.
const missionChoiceCursorMargin = 4

func (c *FrontEnd) updateGamePointerAction(a Actions) (int, int) {
	if c.crewPointerPresented() {
		return c.updateCrewPointer(a)
	}
	c.crewPointer.initialized = false
	if !a.PointerRelative {
		return c.updateGamePointer(a.X, a.Y, a.Inside, a.PointerReset)
	}
	screen := c.Screen
	if screen == ScreenQuit {
		screen = c.previousScreen
	}
	if screen != ScreenMission {
		c.missionPointer.relative = false
		c.missionPointer.initialized = false
		return a.X, a.Y
	}
	if !a.Inside {
		c.missionPointer.initialized = false
		return a.X, a.Y
	}
	b, ok := c.missionCursorBounds()
	if !ok {
		return a.X, a.Y
	}
	p := &c.missionPointer
	if !p.initialized {
		if !p.relative {
			p.x, p.y = a.X, a.Y
		}
		p.initialized = true
	}
	p.relative = true
	// Epoch changes preserve the confined position; only movement is discarded.
	if !a.PointerReset {
		p.x += a.PointerDX
		p.y += a.PointerDY
	}
	p.x = max(b[0], min(p.x, b[0]+b[2]-1))
	p.y = max(b[1], min(p.y, b[1]+b[3]-1))
	return p.x, p.y
}

func missionChoiceRect(index int) [4]int {
	return [4]int{77, 79 + index*10, 80, 10}
}

func (c *FrontEnd) missionCursorBounds() ([4]int, bool) {
	if len(c.Choices) == 0 {
		return [4]int{}, false
	}
	first, last := missionChoiceRect(0), missionChoiceRect(len(c.Choices)-1)
	x, y := max(0, first[0]-missionChoiceCursorMargin), max(0, first[1]-missionChoiceCursorMargin)
	right := min(320, last[0]+last[2]+missionChoiceCursorMargin)
	bottom := min(200, last[1]+last[3]+missionChoiceCursorMargin)
	return [4]int{x, y, right - x, bottom - y}, true
}

// updateGamePointer consumes raw viewport movement once per input update. Hit
// testing and drawing never call this stateful operation.
func (c *FrontEnd) updateGamePointer(x, y int, active, reset bool) (int, int) {
	c.missionPointer.relative = false
	screen := c.Screen
	if screen == ScreenQuit {
		screen = c.previousScreen
	}

	if screen != ScreenMission {
		c.missionPointer.initialized = false
		return x, y
	}
	if !active {
		c.missionPointer.initialized = false
		return x, y
	}
	if reset {
		c.missionPointer.initialized = false
	}

	bounds, ok := c.missionCursorBounds()
	if !ok {
		c.missionPointer.initialized = false
		return x, y
	}

	p := &c.missionPointer
	if !p.initialized {
		p.x, p.y = x, y
		p.initialized = true
	} else {
		// Discard movement beyond the boundary: reversing direction
		// immediately moves the logical cursor away from the edge.
		p.x += x - p.rawX
		p.y += y - p.rawY
	}

	p.rawX, p.rawY = x, y
	p.x = max(bounds[0], min(p.x, bounds[0]+bounds[2]-1))
	p.y = max(bounds[1], min(p.y, bounds[1]+bounds[3]-1))

	return p.x, p.y
}

func (c *FrontEnd) seedMissionPointer() {
	c.missionPointer.initialized = false
	if bounds, ok := c.missionCursorBounds(); ok {
		x := max(bounds[0], min(c.pointerX, bounds[0]+bounds[2]-1))
		y := max(bounds[1], min(c.pointerY, bounds[1]+bounds[3]-1))
		c.missionPointer = missionPointerState{initialized: c.pointerInside, rawX: c.pointerX, rawY: c.pointerY, x: x, y: y}
		c.Office.SetPointer(x, y, c.pointerInside)
	}
}

type missionPointerState struct {
	initialized bool
	rawX, rawY  int
	x, y        int
	relative    bool
}
