package menus

// S1589..1597 confines the original input to one column. S15cc..15e1
// selects POINTER template8 with zero hotspot; the source ACCEPT AREA is only
// four rows high even though its visible button is ten pixels high.
const ordnanceSelectorX = 229
const ordnanceSelectorTop = 67
const ordnanceAcceptHitHeight = 4

type ordnanceRowPointer struct {
	initialized bool
	rawY, y     int
}

func (v *OrdnanceView) rowRect(i int) [4]int {
	r := v.Choices[i].Rectangle
	r[0], r[2] = ordnanceSelectorX, 1
	if v.Choices[i].Action == "back" {
		r[3] = ordnanceAcceptHitHeight
	}
	return r
}

func (v *OrdnanceView) rowHit(y int) int {
	for i := len(v.Choices) - 1; i >= 0; i-- {
		r := v.rowRect(i)
		if v.Choices[i].Action != "heading" && y >= r[1] && y < r[1]+r[3] {
			return i
		}
	}
	return -1
}

func (v *OrdnanceView) restoreRowSelection() {
	selected := v.stack[len(v.stack)-1].Selected
	if selected < 0 || selected >= len(v.Choices) || v.Choices[selected].Action == "heading" {
		selected = -1
		for i, c := range v.Choices {
			if c.Action != "heading" {
				selected = i
				break
			}
		}
	}
	v.Hover = selected
	if selected >= 0 {
		r := v.rowRect(selected)
		// S15bf..15c9: selected AREA top+trunc(height/2)-1.
		v.rowPointer.y = r[1] + r[3]/2 - 1
		v.stack[len(v.stack)-1].Selected = selected
	}
}

func (v *OrdnanceView) seedRowPointer(rawY int) {
	v.rowPointer.rawY = rawY
	v.rowPointer.initialized = true
}

func (v *OrdnanceView) resetRowAnchor() { v.rowPointer.initialized = false }

// Consumes raw vertical movement once. Rendering and row hit tests never feed
// constrained coordinates back here; outward movement is discarded at each edge.
func (v *OrdnanceView) updateRowPointer(a Actions) {
	p := &v.rowPointer
	if !a.Inside {
		p.initialized = false
	} else if a.PointerRelative {
		p.initialized = true
		if !a.PointerReset && a.PointerDY != 0 {
			v.moveRowPointer(a.PointerDY)
		}
	} else if !p.initialized || a.PointerReset {
		v.seedRowPointer(a.Y)
	} else {
		delta := a.Y - p.rawY
		p.rawY = a.Y
		if delta != 0 {
			v.moveRowPointer(delta)
		}
	}
	if a.Up || a.Down || a.MenuTab || a.MenuBackTab || a.MenuHome || a.MenuEnd {
		index := v.Hover
		step := 1
		if a.Up || a.MenuBackTab || a.MenuEnd {
			step = -1
		}
		if a.MenuHome {
			index = -1
		}
		if a.MenuEnd {
			index = len(v.Choices)
		}
		if a.Up || a.Down {
			// A109ac chooses the nearest AREA in that direction, retaining the
			// current one when none exists. A vertical column has no lateral candidate.
			bestDistance := 1 << 30
			for i, c := range v.Choices {
				if c.Action == "heading" {
					continue
				}
				r := v.rowRect(i)
				delta := (r[1] + r[3]/2 - 1 - v.rowPointer.y) * step
				if delta > 0 && delta < bestDistance {
					index, bestDistance = i, delta
				}
			}
		} else {
			// Tab/ShiftTab alone wrap. Home/End start outside the first/last AREA.
			if index < 0 && step < 0 {
				index = 0
			}
			for n := 0; n < len(v.Choices); n++ {
				index = (index + step + len(v.Choices)) % len(v.Choices)
				if v.Choices[index].Action != "heading" {
					break
				}
			}
		}
		if index >= 0 && index < len(v.Choices) && v.Choices[index].Action != "heading" {
			v.stack[len(v.stack)-1].Selected = index
			v.restoreRowSelection()
		}
	}
}

func (v *OrdnanceView) moveRowPointer(delta int) {
	bottom := ordnanceSelectorTop
	for i := range v.Choices {
		r := v.rowRect(i)
		bottom = max(bottom, r[1]+r[3]-1)
	}
	v.rowPointer.y = max(ordnanceSelectorTop, min(v.rowPointer.y+delta, bottom))
	v.Hover = v.rowHit(v.rowPointer.y)
}

func (v *OrdnanceView) drawRowPointer(frame, source []byte, phase int) {
	if len(source) != 64000 {
		return
	}
	sequence := [6]int{0, 1, 2, 3, 2, 1}
	sx := (8 + sequence[phase%len(sequence)]) * 16
	for y := 0; y < 16; y++ {
		dy := v.rowPointer.y + y
		if dy < 0 || dy >= 191 {
			continue
		}
		for x := 0; x < 16 && ordnanceSelectorX+x < 320; x++ {
			if value := source[y*320+sx+x]; value != 0 {
				frame[dy*320+ordnanceSelectorX+x] = value
			}
		}
	}
}
