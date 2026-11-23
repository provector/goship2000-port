package navigation

// Support mode is sourceGPS FF6A.bit2, AA4 availability and the request-lowbyte
// captured at GPSentry. Nil preserves every historical route-only checkpoint.
type SupportMode struct {
	Editing      bool `json:"editing"`
	Availability byte `json:"availability"`
	EntryRequest byte `json:"entry_request"`
	Player       int  `json:"player"`
}
type SupportCommand struct {
	Cancel bool  `json:"cancel"`
	Point  Point `json:"point"`
}

// UseSourceSupportInterlocks selects the corrected source command-mode readers
// for new v5 input and isolated practice. Historical v4 films leave this false.
// It changes no coordinates, route, support, clock, RNG or serialized checkpoint.
func (g *GPS) UseSourceSupportInterlocks() { g.sourceSupportInterlocks = true }

func (g *GPS) EnableSupport(availability byte, request uint16, player int) {
	g.Support = &SupportMode{Availability: availability, EntryRequest: byte(request), Player: player}
}
func (g *GPS) TakeSupportCommand() *SupportCommand {
	c := g.supportCommand
	g.supportCommand = nil
	return c
}

// Source19a2 spends one availability highnibble on the first new request when
// leaving GPS. Reopening an existing request cannot spend it again.
func (g *GPS) CloseSupport(request uint16) byte {
	if g.Support == nil {
		return 0
	}
	a := g.Support.Availability
	if request != 0 && g.Support.EntryRequest == 0 {
		a -= 0x10
		if a&0xf0 == 0 {
			a = 0
		}
	}
	return a
}

// Source section4 D16..D56 and13B6..13F6 test the two AA4 nibbles,
// GPS-entry AA2 low byte, route-edit flag and selected/player identity.
// Availability comes only from an admitted actual Support runtime owner.
func (g *GPS) SupportAvailable() bool {
	s := g.Support
	return s != nil && !g.Editing && s.Availability&0xf0 != 0 && s.Availability&0xf != 0 && s.EntryRequest == 0 && g.Selected == s.Player
}

func (g *GPS) handleSupport(in Input) (bool, error) {
	s := g.Support
	if s == nil {
		return false, nil
	}
	if in.Key == "m" || in.Key == "M" {
		if g.SupportAvailable() {
			s.Editing = !s.Editing
			if s.Editing && !g.Low {
				g.CursorX, g.CursorY = HighPixel(g.Poses[g.Selected])
				g.CursorX = clamp(g.CursorX, 10, 218)
				g.CursorY = clamp(g.CursorY, 10, 154)
			}
		}
		return true, nil
	}
	if !s.Editing {
		return false, nil
	}
	if in.Escape {
		s.Editing = false
		return true, nil
	}
	if in.Selector1 || in.Selector2 {
		if g.Low {
			g.Center.H0 = clampWorld(int32(g.Center.H0) + int32(int16(uint16(in.AxisX)<<5)))
			g.Center.H1 = clampWorld(int32(g.Center.H1) - int32(int16(uint16(in.AxisY)<<5)))
		} else {
			if in.Pointer {
				g.CursorX = clamp(in.X, 10, 218)
				g.CursorY = clamp(in.Y, 10, 154)
			}
			g.CursorX = clamp(g.CursorX+int(in.AxisX>>4), 10, 218)
			g.CursorY = clamp(g.CursorY+int(in.AxisY>>4), 10, 154)
		}
		g.supportCommand = &SupportCommand{Cancel: in.Selector2, Point: g.CursorPoint()}
		s.Editing = false
		return true, nil
	}
	return false, nil
}
