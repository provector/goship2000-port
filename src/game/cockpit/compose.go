package cockpit

import "fmt"

type Station struct {
	Present         bool // Explicit runtime presence; raw WSHD IDs0/1 are real weapons.
	SystemID        int
	Name            string
	Quantity        int16
	Mode, Flags     byte
	DefinitionFlags uint16 // compact definition word; high1000 is rocket salvo eligibility
}

// Compact0 is NO BAY; compact1 is a visible EMPTY entry without ammunition.
// Present also permits an explicit raw-WSHD diagnostic adapter witness, so
// genuine raw0/1 weapons are never mistaken for these compact sentinels.
func (s Station) displayed() bool { return s.Present || s.SystemID > 0 }

type DefenseInputs struct {
	Chaff, Flares            int16
	IRActive, RadarSignature int16
	ChaffTimer, FlareTimer   int16 // Source AF7/AF5; phase is supplied by the logical owner.
}
type MapMarker struct {
	ObjectiveTriangle bool `json:",omitempty"`
	H0, H1            uint32
	Symbol            string
	Colour            byte
	Heading           uint16
}
type MapInputs struct {
	Grid, Atlas         []byte
	H0, H1              uint32
	Markers             []MapMarker
	Mission             *MapMission
	SupportStrike       *MapPoint // Read-only AA2!=0 / AA5,AA9 projection; source015b:0ca7.
	Heading, SweepPhase uint16
	RoutePhase          uint16 // Logical DA plus source retained phase byte, supplied once.
	DrawOwnship         bool
	Routes              [5]MapRoute
	Observer            byte // Source mainDS593c; supported player runtime row, not IFF.
}
type BarInputs struct {
	LoadOrdinal, FuelNumerator, FuelDivisor int16
	Current                                 [2]int16
}

// Values whose producers are not established remain explicitly raw. A caller
// must label diagnostic values; these fields do not implement aircraft physics.
type Inputs struct {
	Notifications                       *NotificationInputs
	Navigation                          *NavigationBearing
	Route                               *NavigationInputs
	Objectives                          [2]ObjectiveDisplay
	AircraftID, Arrangement, CameraMode int
	HUD                                 bool
	MastEmpty                           bool // No acquired target: source222c fill, not a fabricated target camera.
	Height, Heading, Pitch, Bank        uint16
	SystemFlags                         byte  // source mainDS E1: alternate instruments, bay and gear
	PrimaryMFD, SecondaryMFD            int16 // source AD7/AD9; secondary -1 is attitude
	Vertical, Lateral, BarLoad          int16
	HUDScale, HUDVertical               int16
	BarValues                           [2]int16
	Bars                                *BarInputs
	// Historical table names call these matrix words;0d93 actually caches
	// {sin(a),cos(c),sin(c)} here before constructing the9-word view matrix.
	AttitudeMatrix           [3]int16
	HeadingPerturbation      int16
	Fuel, FuelMaximum        int16
	SelectedStation          int
	RocketSalvo              int16
	Stations                 [4]Station
	MissionCounter           uint16
	CompressionRequest       int16 // Source AF3 request, not the nominal1 normal-rate label or viewer speed.
	BlinkPhase               uint16
	PrimaryMap, SecondaryMap bool
	Map                      *MapInputs
	AutoPilot, AutoHover     int16
	Rotor                    int16 // source DATA272d:ae1, separate from collectiveAAF
	Damage                   [10]byte
	DamageOverlay            byte        // Physical608, not part of the ten source lamps.
	DamageOverlayPoints      [2][2]int16 // Retained B03/B05 and B07/B09.
	TargetSymbol             int
	TargetX, TargetY         int16
	TargetAim                *TargetAimInputs
	Defense                  *DefenseInputs
	ContactAvionics          *ContactAvionicsInputs
}

func Bottom(a *Assets, aircraft, arrangement int) (int, error) {
	if a == nil {
		return 0, fmt.Errorf("cockpit assets unavailable")
	}
	f, e := Family(aircraft)
	if e != nil {
		return 0, e
	}
	if arrangement < 0 || arrangement > 3 {
		return 0, fmt.Errorf("cockpit arrangement outside 0..3")
	}
	if arrangement == 1 || arrangement == 3 {
		art, ok := a.Images[f+"_CPIT.PIC"]
		if !ok {
			return 0, fmt.Errorf("cockpit family %s missing", f)
		}
		return art.Bottom, nil
	}
	return 199, nil
}

func Compose(dst []byte, a *Assets, in Inputs) error {
	if !LogicalTarget(dst) || a == nil {
		return fmt.Errorf("cockpit composition requires indexed320x200 target")
	}
	f, e := Family(in.AircraftID)
	if e != nil {
		return e
	}
	art, ok := a.Images[f+"_CPIT.PIC"]
	if !ok || len(art.Pixels) != 64000 || len(art.FullMask) != 64000 {
		return fmt.Errorf("cockpit family%s missing", f)
	}
	if in.Arrangement < 0 || in.Arrangement > 3 {
		return fmt.Errorf("invalid cockpit arrangement")
	}
	if in.CameraMode == 0 {
		switch in.Arrangement {
		case 0, 1:
			for i, v := range art.FullMask {
				if v != 0 {
					PutLogical(dst, i, art.Pixels[i])
				}
			}
		case 3:
			copy(dst[art.Split:64000], art.Pixels[art.Split:])
			if len(dst) == 128000 {
				clear(dst[64000+art.Split:])
			}
		}
	}
	if !in.HUD {
		return nil
	}
	if in.CameraMode == 7 && (in.MastEmpty || in.Damage[0]&0x20 != 0) {
		fill(dst, 69, 132, 183, 65, 0x15, Full)
		if e = a.DrawMastFrame(dst); e != nil {
			return e
		}
	}
	d := a.Definitions[f]
	p := func(off int) int { return int(d.Parameters[off/2]) }
	primary := in.CameraMode == 0 && in.Arrangement != 2
	if primary {
		if in.SelectedStation < 0 || in.SelectedStation > 3 {
			return fmt.Errorf("selected HUD station outside0..3")
		}
		s := in.Stations[in.SelectedStation]
		if s.displayed() {
			if e = a.Text(dst, primaryWeaponName(s, in.RocketSalvo), p(0), p(4), 0x0a, 0x802, Full); e != nil {
				return e
			}
			quantity := primaryAmmunitionText(s, in.Damage[3], in.Damage[8])
			if e = a.Text(dst, quantity, p(0), p(6), 0x0a, 0x802, Full); e != nil {
				return e
			}
		}
		if e = a.Sprite(dst, "station-baseline", p(0)-17, p(2), Full); e != nil {
			return e
		}
		counter := 1
		for i, st := range in.Stations {
			if !st.displayed() {
				continue
			}
			name := "station-plain"
			if i == in.SelectedStation {
				name = "station-highlight"
			}
			if i == 0 {
				if e = a.Sprite(dst, name, p(0)-1, p(2)+2, Full); e != nil {
					return e
				}
				continue
			}
			y := p(2) + 2
			if i == 3 {
				y = p(2)
			}
			if st.Flags&2 != 0 {
				counter++
				if e = a.Sprite(dst, name, p(0)-3*(counter&0xfe)-1, y, Full); e != nil {
					return e
				}
			}
			if st.Flags&1 != 0 {
				counter++
				if e = a.Sprite(dst, name, p(0)+3*(counter&0xfe)-1, y, Full); e != nil {
					return e
				}
			}
		}
		if in.Defense != nil && in.Damage[0]&0x40 == 0 {
			drawDefenseIndicators(dst, d, *in.Defense, in.BlinkPhase)
			if e = a.drawDefenseCounts(dst, d, *in.Defense, in.BlinkPhase); e != nil {
				return e
			}
		}
		if in.Damage[0]&0x10 == 0 {
			if e = a.drawRotorIndicator(dst, d, in); e != nil {
				return e
			}
			if in.Bars != nil {
				if e = drawBars(dst, d, *in.Bars); e != nil {
					return e
				}
			}
			h := int16(in.Height - 2)
			if h < 0 {
				h = 0
			}
			display := int16(uint16(uint16(h)*25) >> 5)
			if e = a.Number(dst, p(0x24), p(0x26), display, 4); e != nil {
				return e
			}
			if e = a.Number(dst, p(0x2c), p(0x2e), in.Vertical, 4); e != nil {
				return e
			}
			heading := uint16(-uint16(in.Heading + uint16(in.HeadingPerturbation)))
			if e = a.Number(dst, p(0x38), p(0x3a), int16(heading/183), 3); e != nil {
				return e
			}
			if in.SystemFlags&1 == 0 {
				indices := [4]int{2 * (int(h) & 63), 0, 0, int(byte(heading>>8) >> 1)}
				v := int(in.Vertical >> 1)
				if v < 0 {
					v += 128
					if v < 112 {
						v = 112
					}
				} else if v > 96 {
					v = 96
				}
				indices[1] = v
				v = int(int16(in.Lateral + 96))
				if v > 128 {
					v -= 128
					if v > 24 {
						v = 24
					}
				} else if v < 40 {
					v = 40
				}
				indices[2] = v
				origins := [4][2]int{{p(0x20), p(0x22)}, {p(0x28), p(0x2a)}, {p(0x30), p(0x32)}, {p(0x34), p(0x36)}}
				for i, index := range indices {
					if index < 0 || index >= 128 {
						return fmt.Errorf("needle index%d outside generated128pairs", index)
					}
					v := a.Needles[index]
					dx, dy := int(v[0]), int(v[1])
					if i == 3 {
						dx /= 2
						dy /= 2
					}
					o := origins[i]
					Line(dst, o[0], o[1], o[0]+dx, o[1]+dy, 15, Full)
				}
				a.attitude(dst, d, in.AttitudeMatrix)
			} else {
				drawAlternateIndicators(dst, d, in)
			}
			if e = a.drawNavigationIndicator(dst, d, heading, in.Navigation); e != nil {
				return e
			}
		}
		for index, v := range in.Damage {
			x := p(8) + (index%5)*14
			y := p(10) + (index/5)*8
			state := v & 3
			if state != 0 {
				c := byte(12)
				if state == 1 {
					c = 14
				}
				exactReplace(dst, x, y, 13, 7, 2, c)
			}
		}
		if in.Map != nil {
			if in.PrimaryMap && in.PrimaryMFD == 0 && in.Damage[0]&0x20 == 0 {
				if e = a.DrawMap(dst, d, false, *in.Map); e != nil {
					return e
				}
				if e = a.remapMFD(dst, d, false, in.SystemFlags); e != nil {
					return e
				}
			}
			if in.SecondaryMap && in.SecondaryMFD == 0 && in.Damage[0]&4 == 0 {
				if e = a.DrawMap(dst, d, true, *in.Map); e != nil {
					return e
				}
				if e = a.remapMFD(dst, d, true, in.SystemFlags); e != nil {
					return e
				}
			}
		}
		if in.SystemFlags&1 != 0 && in.SecondaryMFD == -1 {
			if e = a.alternateAttitude(dst, d, in.AttitudeMatrix); e != nil {
				return e
			}
		}
		// Source section2:0dff skips the entire primary dispatch on5fe20,
		// including stores text and its destination brightness remap.
		if in.PrimaryMFD == 4 && in.Damage[0]&0x20 == 0 {
			if e = a.storesMFD(dst, d, false, in); e != nil {
				return e
			}
		}
		if in.SecondaryMFD == 4 {
			if e = a.storesMFD(dst, d, true, in); e != nil {
				return e
			}
		}
		if e = a.drawNavigationMFDs(dst, d, in); e != nil {
			return e
		}
	}
	if e = a.drawTargeting(dst, in); e != nil {
		return e
	}
	// Source1d1f sends external cameras directly to24d8. Only target/mast
	// operations are restricted to0/7; the common flight HUD reaches all views.
	if in.Damage[0]&0x80 == 0 {
		heading := uint16(-uint16(in.Heading + uint16(in.HeadingPerturbation)))
		if e = a.heading(dst, heading); e != nil {
			return e
		}
		if e = blit(dst, a.Atlases["fixed-scale"], 0, 0, 16, 65, 2, 18, true, Full); e != nil {
			return e
		}
		if e = a.verticalStrip(dst, in.HUDScale, in.HUDVertical); e != nil {
			return e
		}
		if e = a.clearanceNumbers(dst, in.HUDScale); e != nil {
			return e
		}
		if e = a.speedNumber(dst, in.Vertical, in.BlinkPhase); e != nil {
			return e
		}
		if e = blit(dst, a.Atlases["fixed-strip"], 0, 0, 5, 32, 308, 40, true, Full); e != nil {
			return e
		}
		pitch := clamp(int(int8(in.Pitch>>8)), -40, 20)
		a.DrawVectors(dst, a.Vectors, 46, 60+pitch, uint16(-in.Bank), 6, 31, Full)
		if e = a.Sprite(dst, "lateral-marker", 314, 54-clamp(int(in.Lateral), -16, 16), Full); e != nil {
			return e
		}
		if e = a.drawHUDNavigationMarker(dst, heading, in.Navigation); e != nil {
			return e
		}
	}
	if e = a.DrawContactAvionics(dst, in); e != nil {
		return e
	}
	if e = a.drawCommonModeText(dst, in); e != nil {
		return e
	}
	// Original mode lamps are reached after the common HUD, independently of
	// its damage80 gate. Their exact-index replacement needs restored artwork.
	drawModeIndicators(dst, d, in)
	if in.MissionCounter >= 36000 {
		return fmt.Errorf("mission counter outside safe two-digit HUD clock domain")
	}
	if e = a.Number(dst, 4, 2, int16(in.MissionCounter/360), 0x102); e != nil {
		return e
	}
	if e = a.Number(dst, 16, 2, int16((in.MissionCounter/6)%60), 0x102); e != nil {
		return e
	}
	// Section1:20f5..210e follows the common clock in every reached full view.
	// Style0102 is two zero-masked4x6 digits; request0 draws nothing.
	if in.CompressionRequest != 0 {
		return a.Number(dst, 16, 8, in.CompressionRequest, 0x102)
	}
	return nil
}

func drawBars(dst []byte, d Definition, in BarInputs) error {
	p := func(off int) int { return int(d.Parameters[off/2]) }
	if in.FuelDivisor == 0 {
		return fmt.Errorf("HUD raw fuel divisor cannot be zero")
	}
	product := int16(int16(p(0x16)) * in.FuelNumerator)
	if product == -32768 && in.FuelDivisor == -1 {
		return fmt.Errorf("HUD source fuel IDIV overflow")
	}
	heights := []int{p(0x10) - int(int16(int16(p(0x10))*in.LoadOrdinal)>>5), p(0x16) - int(product/in.FuelDivisor), 16 - int(in.Current[0]), 16 - int(in.Current[1])}
	for i, off := range []int{0x0c, 0x12, 0x18, 0x1c} {
		if i == 3 && p(off) == 0 {
			continue
		}
		limit := 16
		if i == 0 {
			limit = p(0x10)
		} else if i == 1 {
			limit = p(0x16)
		}
		h := heights[i]
		if h < 0 || h > limit {
			return fmt.Errorf("HUD bar height%d outside supplied diagnostic domain0..%d", h, limit)
		}
		w := 2
		if i < 2 {
			w = 4
		}
		match := byte(0x40)
		if i == 1 {
			match = 0x90
		}
		x, y := p(off), p(off+2)
		for yy := y; yy < y+h; yy++ {
			for xx := x; xx < x+w; xx++ {
				if xx >= 0 && xx < 320 && yy >= 0 && yy < 200 && ((dst[yy*320+xx]^match)&0xf0) == 0 {
					PutLogical(dst, yy*320+xx, 0)
				}
			}
		}
	}
	return nil
}

func (a *Assets) heading(dst []byte, heading uint16) error {
	img := a.Atlases["heading-strip"]
	start := int(byte(uint16(heading-0x2000) >> 8))
	if len(img.Pixels) != 256*16 {
		return fmt.Errorf("heading atlas incompatible")
	}
	for y := 0; y < 16; y++ {
		for x := 0; x < 129; x++ {
			v := img.Pixels[y*256+(start+x)&255]
			if v != 0 {
				pixel(dst, 96+x, 1+y, v, Full)
			}
		}
	}
	for y := 2; y < 14; y++ {
		pixel(dst, 160, y, 0x15, Full)
	}
	return nil
}

func (a *Assets) verticalStrip(dst []byte, scale, value int16) error {
	img := a.Atlases["vertical-strip"]
	if len(img.Pixels) != 480 {
		return fmt.Errorf("vertical HUD atlas incompatible")
	}
	delta := 0
	if uint16(value) >= uint16(scale) {
		delta = int(uint16(value-scale) >> 2)
		if delta > 64 {
			delta = 64
		}
	}
	start := 3 * int((uint16(-scale)>>1)&0x1e)
	for row := 0; row < 64; row++ {
		for col := 0; col < 6; col++ {
			v := img.Pixels[start+6*row+col]
			if v != 0 {
				pixel(dst, 300+col, 24+row, v, Full)
			}
		}
		if row >= 64-delta {
			pixel(dst, 306, 24+row, 12, Full)
			pixel(dst, 307, 24+row, 4, Full)
		}
	}
	return nil
}

func (a *Assets) DrawVectors(dst []byte, vectors []Vector, x, y int, angle uint16, shift uint, colour byte, clip Rect) {
	s, c := int32(a.Sin(angle)), int32(a.Sin(angle+0x4000))
	point := func(p [2]int16) (int, int) {
		px, py := int32(int16(p[0]<<8)), int32(int16(p[1]<<8))
		xx := int16(int16(px*c>>14) - int16(py*s>>14))
		yy := int16(int16(px*s>>14) + int16(py*c>>14))
		return x + int(xx>>shift), y - int(yy>>shift)
	}
	for _, v := range vectors {
		ink := v.Colour
		if ink == 255 {
			ink = colour
		}
		for i := 1; i < len(v.Points); i++ {
			x0, y0 := point(v.Points[i-1])
			x1, y1 := point(v.Points[i])
			Line(dst, x0, y0, x1, y1, ink, clip)
		}
	}
}

func (a *Assets) attitude(dst []byte, d Definition, m [3]int16) {
	cx, cy := int(d.Parameters[0x4c/2]), int(d.Parameters[0x4e/2])
	a.attitudeClipped(dst, d, m, Rect{cx - 16, cy - 12, cx + 16, cy + 12})
}

func (a *Assets) attitudeClipped(dst []byte, d Definition, m [3]int16, clip Rect) {
	t := int16(int32(m[0]) * int32(m[2]) >> 14)
	q := func(z int16, n uint) int16 { return int16((int32(z)*256)/24064) >> n }
	x0, x1, y0, y1 := q(int16(t+m[1]), 2), q(int16(t-m[1]), 2), q(int16(m[0]+m[2]), 3), q(int16(m[0]-m[2]), 3)
	cx, cy := int(d.Parameters[0x4c/2]), int(d.Parameters[0x4e/2])
	points := [][2]int{{cx + int(x0), cy + int(y0)}, {cx + int(x1), cy + int(y1)}, {cx + int(int16(x1+y1-y0)), cy + int(int16(y1+x0-x1))}, {cx + int(int16(x0+y1-y0)), cy + int(int16(y0+x0-x1))}}
	match, to := byte(d.SpanReplacement>>8), byte(d.SpanReplacement)
	for y := clip.Top; y <= clip.Bottom; y++ {
		left, right := 32767, -32768
		for i := range points {
			p, n := points[i], points[(i+1)%len(points)]
			if p[1] == n[1] {
				continue
			}
			if y < min(p[1], n[1]) || y > max(p[1], n[1]) {
				continue
			}
			x := p[0] + (n[0]-p[0])*(y-p[1])/(n[1]-p[1])
			left = min(left, x)
			right = max(right, x)
		}
		for x := max(left, clip.Left); x <= min(right, clip.Right); x++ {
			if x >= 0 && x < 320 && y >= 0 && y < 200 && dst[y*320+x] == match {
				PutLogical(dst, y*320+x, to)
			}
		}
	}
}

func (a *Assets) DrawMap(dst []byte, d Definition, secondary bool, in MapInputs) error {
	if len(in.Grid) != 4096 || len(in.Atlas) != 192*144 {
		return fmt.Errorf("MFD map requires64x64grid and192x144tile atlas")
	}
	base, anchor := 0x54, 0x50
	if secondary {
		base, anchor = 0x60, 0x5c
	}
	p := func(off int) int { return int(d.Parameters[off/2]) }
	left, top, right, bottom := p(base), p(base+2), p(base+4), p(base+6)
	if left < 0 || top < 0 || right > 320 || bottom > 200 || left >= right || top >= bottom {
		return fmt.Errorf("invalid MFD map panel")
	}
	gridRAM := uint16(0xfc0 - 64*int((in.H1>>13)&255) + int(in.H0>>13) + 14)
	first := int(gridRAM) - 130 - 14
	for row := 0; row < 5; row++ {
		for col := 0; col < 5; col++ {
			at := first + 64*row + col
			if at < 0 || at >= 4096 {
				return fmt.Errorf("MFD map needs unresolved world-edge neighbor RAM")
			}
			if in.Grid[at] >= 64 {
				return fmt.Errorf("MFD tile selector%d outside64", in.Grid[at])
			}
		}
	}
	x := left + (right-left)/2 - 48 - int((in.H0&8191)/341)
	y := top + (bottom-top)/2 - 52 + int((in.H1&8191)/455)
	clip := Rect{left, top, right - 1, bottom - 1}
	for row := 0; row < 5; row++ {
		for col := 0; col < 5; col++ {
			id := int(in.Grid[first+64*row+col])
			sx, sy := (id&7)*24, (id>>3)*18
			if e := blit(dst, Image{Width: 192, Height: 144, Pixels: in.Atlas}, sx, sy, 24, 18, x+24*col, y+18*row, false, clip); e != nil {
				return e
			}
		}
	}
	for _, m := range in.Markers {
		v, ok := a.MapVectors[m.Symbol]
		if !ok {
			return fmt.Errorf("unsupported supplied MFD marker%s", m.Symbol)
		}
		dx := int(int16(int32(m.H0-in.H0) / 341))
		dy := int(int16(int32(m.H1-in.H1) / -455))
		a.DrawVectors(dst, v, p(anchor)+dx, p(anchor+2)+dy, m.Heading, 8, m.Colour, clip)
	}
	projectMission := func(h0, h1 uint32) (int, int) {
		return int(int16(int16(p(anchor)) + int16(int32(h0-in.H0)/341))), int(int16(int16(p(anchor+2)) + int16(int32(h1-in.H1)/-455)))
	}
	if err := a.DrawMissionMarkers(dst, in.Mission, projectMission, -455, 8, clip); err != nil {
		return err
	}
	a.drawMapRoutes(dst, in, p(anchor), p(anchor+2), clip)
	if err := a.DrawSupportStrike(dst, in.SupportStrike, projectMission, 8, clip); err != nil {
		return err
	}
	if in.DrawOwnship {
		a.DrawVectors(dst, a.MapVectors["map-07aa"], p(anchor), p(anchor+2), in.Heading, 8, 31, clip)
		// Source015b:0d73 selects0208: a one-pixel black shadow precedes
		// the sweep's literal-colour vectors. This is not an extra contact.
		sweep := a.MapVectors["map-079d"]
		for _, v := range sweep {
			v.Colour = 0
			a.DrawVectors(dst, []Vector{v}, p(anchor)+1, p(anchor+2)+1, in.Heading+in.SweepPhase, 8, 0, clip)
		}
		a.DrawVectors(dst, sweep, p(anchor), p(anchor+2), in.Heading+in.SweepPhase, 8, 31, clip)
	}
	return nil
}
