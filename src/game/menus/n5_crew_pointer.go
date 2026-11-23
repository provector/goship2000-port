package menus

import (
	assetdata "gunship2000-assets/game/assets"
	"strings"
)

// Original %c swaps inks without consuming a printf argument (GS A1cc60).
func crewRecommendationText(format string, arguments []string, base, alternate int) string {
	var out strings.Builder
	arg := 0
	for i := 0; i < len(format); i++ {
		if format[i] == '%' && i+1 < len(format) {
			i++
			switch format[i] {
			case 'c':
				base, alternate = alternate, base
				out.WriteByte(byte(128 | base))
			case 's':
				if arg < len(arguments) {
					out.WriteString(arguments[arg])
					arg++
				}
			case '%':
				out.WriteByte('%')
			default:
				out.WriteByte('%')
				out.WriteByte(format[i])
			}
		} else {
			out.WriteByte(format[i])
		}
	}
	return out.String()
}

func (c *FrontEnd) crewPointerPresented() bool {
	return c.Screen == ScreenDebrief && c.Debrief != nil && c.Debrief.Page == "crew"
}

// Source AREA0 is a one-pixel-wide lane, not the width of its recommendation text.
func crewAreaAt(n, x, y int) int {
	if x != 3 {
		return -1
	}
	if y >= 146 && y < 146+7*n {
		return (y - 146) / 7
	}
	if y >= 173 && y <= 182 {
		return n
	}
	return -1
}

func (c *FrontEnd) updateCrewPointer(a Actions) (int, int) {
	p := &c.crewPointer
	if !a.Inside {
		return p.x, p.y
	}
	if !p.initialized {
		p.x, p.y = 3, 146
		if c.Debrief.CrewSelection >= len(c.Debrief.Pending.Proposal.Result.Recommendations) {
			p.y = 173
		}
		p.rawX, p.rawY, p.initialized = a.X, a.Y, true
	} else if !a.PointerReset {
		if a.PointerRelative {
			p.y += a.PointerDY
		} else {
			p.y += a.Y - p.rawY
		}
	}
	p.rawX, p.rawY = a.X, a.Y
	p.x, p.y = 3, min(182, max(146, p.y))
	return p.x, p.y
}

func (c *FrontEnd) setCrewSelection(index, n int) {
	c.Debrief.CrewSelection = index
	y := 146 + 7*index
	if index == n {
		y = 173
	}
	c.crewPointer.x, c.crewPointer.y, c.crewPointer.initialized = 3, y, true
	c.Office.SetPointer(3, y, true)
}

func (c *FrontEnd) drawCrewPointer(frame []byte) {
	if !c.Office.PointerInside {
		return
	}
	x, y := c.Office.PointerX, c.Office.PointerY
	if !c.crewPointer.initialized {
		x, y = 3, 146
		if c.Debrief.CrewSelection >= len(c.Debrief.Pending.Proposal.Result.Recommendations) {
			y = 173
		}
	}
	sequence := [...]int{8, 9, 10, 11, 10, 9}
	phase := c.Debrief.Tick / 5 % len(sequence)
	// Shared source pointer clips at191; the footer is not part of its destination.
	_ = assetdata.OfficeBlit(c.Office.Assets.Images["POINTER.PIC"], frame, [4]int{sequence[phase] * 16, 0, 16, min(16, 191-y)}, x, y, true)
}
