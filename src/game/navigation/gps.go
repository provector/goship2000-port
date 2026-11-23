package navigation

import (
	"encoding/binary"
	"fmt"
	"gunship2000-assets/game/cockpit"
	"gunship2000-assets/game/world"
	"slices"
	"strings"
)

var MapClip = cockpit.Rect{Left: 2, Top: 3, Right: 226, Bottom: 163}

type Input struct {
	PrimaryHeld, HoldValid       bool
	Key                          string
	Escape, Selector1, Selector2 bool
	ViewKey                      int
	AxisX, AxisY                 int16
	Pointer                      bool
	X, Y                         int
}
type GPS struct {
	followerOrders          bool
	r11Presentation         bool
	routeFinished           func(int)
	nativeSelectors         bool
	entryOnlySelectors      bool
	primaryDrain            bool
	Support                 *SupportMode
	supportCommand          *SupportCommand
	sourceSupportInterlocks bool // Input-profile policy, never a recorded mutable state field.
	Low, Editing            bool
	Selected, Moving        int
	CursorX, CursorY        int
	Center                  Point
	Routes                  *[5]Route
	Active                  [5]bool
	Poses                   [5]Point
	Headings                [5]uint16
	Base, Home, FARP        Point
	CargoReleaseAvailable   bool // Published source-selected SHIP cargo eligibility; player-only integration.
	FARPPresent             bool
	HomeError               string
	Markers                 []cockpit.MapMarker
	DeliverySites           []Point // Sighted pending sites, for GPS presentation only.
	Mission                 *cockpit.MapMission
	SupportStrike           *cockpit.MapPoint // Drawing snapshot, excluded from ReplayState.
	Panel                   *PanelSnapshot
	BlinkPhase, SweepPhase  uint16
	HeadingPerturbation     int16
	backup                  Route
	overviewKeyRemainder    [2]int

	confirmPartialRoutes       bool
	keepUnfinishedWingmanRoute bool
	separateDraft              bool
	draft                      *Route
	routeOwner                 func(int) (uint16, bool)
	draftOwner                 uint16
}

func New(routes *[5]Route, poses [5]Point, active [5]bool) *GPS {
	return &GPS{Routes: routes, Poses: poses, Active: active, CursorX: 112, CursorY: 80}
}
func clamp(n, lo, hi int) int {
	if n < lo {
		return lo
	}
	if n > hi {
		return hi
	}
	return n
}
func clampWorld(n int32) uint32 {
	if n < 0xa000 {
		return 0xa000
	}
	if n > 0x76000 {
		return 0x76000
	}
	return uint32(n)
}
func HighPoint(x, y int) Point {
	return Point{H0: uint32(int32(x-2) * 2341), H1: uint32(int32(y-162) * -3279)}
}
func HighPixel(p Point) (int, int) { return 2 + int(int32(p.H0)/2341), 162 + int(int32(p.H1)/-3279) }
func (g *GPS) CursorPoint() Point {
	if g.Low {
		return g.Center
	}
	return HighPoint(g.CursorX, g.CursorY)
}
func (g *GPS) Pixel(p Point) (int, int) {
	if !g.Low {
		return HighPixel(p)
	}
	return 113 + int(int32(p.H0-g.Center.H0)/341), 84 + int(int32(p.H1-g.Center.H1)/-455)
}

func (g *GPS) toggleMapScale() {
	if g.Moving != 0 && g.draft != nil {
		g.CancelPathGesture()
	}
	if !g.Low {
		if g.Editing || g.Support != nil && g.Support.Editing {
			g.Center = g.CursorPoint()
		} else {
			g.Center = g.Poses[g.Selected]
		}
		g.Center.H0 = clampWorld(int32(g.Center.H0))
		g.Center.H1 = clampWorld(int32(g.Center.H1))
		g.Low = true
	} else {
		g.CursorX, g.CursorY = HighPixel(g.Center)
		g.CursorX = clamp(g.CursorX, 10, 218)
		g.CursorY = clamp(g.CursorY, 10, 154)
		g.Low = false
		g.Center.H0, g.Center.H1 = 0, 0 // source051b..0526 clears only the coordinates.
	}
}
func (g *GPS) StartEdit() error {
	if g.Selected != 0 && !g.followerOrders {
		return fmt.Errorf("wingman routes are read-only while autonomous orders are deferred")
	}
	if g.Editing {
		return nil
	}
	r := &g.Routes[g.Selected]
	g.backup = *r
	g.overviewKeyRemainder = [2]int{}
	if g.confirmPartialRoutes && g.Selected == 0 {
		draft := g.backup
		g.draft = &draft
		r = g.draft
	} else if g.separateDraft && g.Selected != 0 {
		if !g.stageDraft() {
			return nil
		}
		r = g.draft
	}
	g.Editing = true
	g.Moving = 0
	r.SetIndex(0)
	r.ClearPoints()
	if !g.Low {
		g.CursorX, g.CursorY = HighPixel(g.Poses[g.Selected])
		g.CursorX = clamp(g.CursorX, 10, 218)
		g.CursorY = clamp(g.CursorY, 10, 154)
	}
	return nil
}

// PreserveWingmanRouteEdits keeps an active flight order until its replacement is complete.
func (g *GPS) PreserveWingmanRouteEdits() { g.keepUnfinishedWingmanRoute = true }

func (g *GPS) CancelUnfinishedWingmanRoute() {
	if g.keepUnfinishedWingmanRoute && g.Selected != 0 && (g.Editing || g.draft != nil) {
		g.CancelPathGesture()
	}
}

// StageWingmanRoutes isolates a waypoint gesture from live CONTROL until confirmation.
func (g *GPS) StageWingmanRoutes(eligible func(int) (uint16, bool)) {
	g.separateDraft = true
	g.routeOwner = eligible
}
func (g *GPS) stageDraft() bool {
	owner, ok := g.routeOwner(g.Selected)
	if !ok {
		return false
	}
	g.backup = g.Routes[g.Selected]
	draft := g.backup
	g.draft = &draft
	g.draftOwner = owner
	return true
}
func (g *GPS) commitDraft() bool {
	r := g.draft
	valid := g.Routes[g.Selected] == g.backup && r.Index() >= 1 && r.Index() <= 3 && r.Point(int(r.Index())).Present()
	if valid && g.Selected != 0 {
		owner, ok := g.routeOwner(g.Selected)
		valid = ok && owner == g.draftOwner
	}
	if valid {
		g.Routes[g.Selected] = *r
	}
	g.draft = nil
	g.draftOwner = 0
	return valid
}
func (g *GPS) endPointAdjustment() {
	if g.Moving != 0 && g.draft != nil {
		g.commitDraft()
	}
	g.Moving = 0
}
func (g *GPS) displayedRoute(slot int) *Route {
	if g.draft != nil && slot == g.Selected {
		return g.draft
	}
	return &g.Routes[slot]
}
func (g *GPS) FinishEdit() {
	if !g.Editing {
		return
	}
	r := g.displayedRoute(g.Selected)
	if g.draft != nil {
		if !g.commitDraft() {
			g.Editing = false
			g.Moving = 0
			return
		}
		r = &g.Routes[g.Selected]
	}
	if r.Index() == 0 {
		g.CancelUnfinishedWingmanRoute()
	} else {
		r.SetIndex(1)
		r.Raw[1] = 3
		if g.followerOrders && g.Selected != 0 {
			binary.LittleEndian.PutUint16(r.Raw[6:], 1)
			binary.LittleEndian.PutUint16(r.Raw[8:], 3)
			if g.routeFinished != nil {
				g.routeFinished(g.Selected)
			}
		}
	}
	g.Editing = false
	g.Moving = 0
}
func (g *GPS) Append() {
	if !g.Editing {
		return
	}
	r := g.displayedRoute(g.Selected)
	n := int(r.Index()) + 1
	if n > 3 {
		return
	}
	p := g.CursorPoint()
	p.Tail = r.Point(n).Tail
	_ = r.SetPoint(n, p)
	r.SetIndex(int8(n))
	if n == 3 {
		g.FinishEdit()
	}
}

// Handle returns an exit view key: -1 is Escape,1..9 the actual forwarded view.
// View exits intentionally bypass edit rollback/finish, matching source0398.
func (g *GPS) Handle(in Input) (int, error) {
	if in.ViewKey >= 1 && in.ViewKey <= 9 {
		return in.ViewKey, nil
	}
	if g.sourceSupportInterlocks && (in.Key == "space" || in.Key == " ") {
		// Source0398/03CE handles BIOS3920h before map-axis/selector work.
		// The native shared selector adapter also maps Space to Selector1;
		// that duplicate host meaning must not exit GPS or launch Support.
		g.toggleMapScale()
		in.Key = ""
		in.Selector1, in.Selector2 = false, false
	}
	if handled, err := g.handleSupport(in); handled || err != nil {
		return 0, err
	}
	if g.confirmPartialRoutes && g.Editing && in.Selector2 && !in.Escape {
		g.FinishEdit()
		return 0, nil
	}
	if g.nativeSelectors && in.Selector2 {
		active := g.Editing || g.Moving > 0
		g.CancelPathGesture()
		if g.entryOnlySelectors && active {
			return 0, nil
		}
		return -1, nil
	}
	if in.Escape {
		if g.Editing {
			g.CancelPathGesture()
			return 0, nil
		}
		return -1, nil
	}
	// v9 entry is exactly keyboard F, with the complete initiating gesture
	// drained before a later fresh press may place a point. Support owns its click
	// above. Do not fabricate a center/cursor waypoint on mode entry.
	if g.entryOnlySelectors {
		if in.Key != "" {
			in.Selector1 = false
		} // an explicit GPS control owns its gesture
		if g.primaryDrain {
			if in.HoldValid && !in.PrimaryHeld || !in.HoldValid && !in.Selector1 {
				g.primaryDrain = false
			}
			in.Selector1 = false
		} else if in.Selector1 && !g.Editing && g.Moving == 0 && in.Key == "" {
			in.Key = "f"
			in.Selector1 = false
			g.primaryDrain = true
		}
	}
	// Source034b exits on ordinary Selector1. Edit/point-adjustment paths
	// drain that selector themselves and stay modal, so they are distinct here.
	if in.Selector1 && !g.Editing && g.Moving == 0 && (g.Support == nil || !g.Support.Editing) {
		if !g.nativeSelectors {
			return -1, nil
		}
		if g.Selected == 0 || g.followerOrders {
			x, y := g.CursorX, g.CursorY
			if err := g.StartEdit(); err != nil {
				return 0, err
			}
			g.CursorX, g.CursorY = x, y
		}
	}
	if in.Pointer && !g.Low {
		g.CursorX = clamp(in.X, 10, 218)
		g.CursorY = clamp(in.Y, 10, 154)
	}
	if g.Low {
		// Word shift wraps before CWD. This is not a floating world/pixel delta.
		g.Center.H0 = clampWorld(int32(g.Center.H0) + int32(int16(uint16(in.AxisX)<<5)))
		g.Center.H1 = clampWorld(int32(g.Center.H1) - int32(int16(uint16(in.AxisY)<<5)))
	} else {
		g.CursorX = clamp(g.CursorX+int(in.AxisX>>4), 10, 218)
		g.CursorY = clamp(g.CursorY+int(in.AxisY>>4), 10, 154)
	}
	if g.Low && g.Moving > 0 && (g.Selected == 0 || g.followerOrders) {
		p := g.Center
		r := g.displayedRoute(g.Selected)
		p.Tail = r.Point(g.Moving).Tail
		_ = r.SetPoint(g.Moving, p)
	}
	switch strings.ToLower(in.Key) {
	case "space", " ":
		g.toggleMapScale() // Historical v4 keeps its original later consumer.
	case "f":
		if g.sourceSupportInterlocks && g.Support != nil && g.Support.Editing {
			break // Source BA4 refuses route editing while FF6A.bit2 is set.
		}
		if g.Editing {
			g.FinishEdit()
		} else if g.Selected == 0 || g.followerOrders {
			return 0, g.StartEdit()
		}
	case "n":
		if !g.Editing && (!g.sourceSupportInterlocks || g.Support == nil || !g.Support.Editing) {
			for i := 1; i <= 5; i++ {
				n := (g.Selected + i) % 5
				if g.Active[n] {
					if g.draft != nil {
						g.CancelPathGesture()
					}
					g.Selected = n
					g.Moving = 0
					if g.Low {
						g.Center = g.Poses[n]
						g.Center.H0 = clampWorld(int32(g.Center.H0))
						g.Center.H1 = clampWorld(int32(g.Center.H1))
					}
					break
				}
			}
		}
	case "b":
		if !g.Editing && !g.Low && g.Selected == 0 && (!g.sourceSupportInterlocks || g.Support == nil || !g.Support.Editing) {
			if g.HomeError != "" {
				return 0, fmt.Errorf("%s", g.HomeError)
			}
			g.Routes[0].HomeFrom(g.Base, g.Home, g.FARP, g.FARPPresent)
		}
	case "1", "2", "3":
		if g.Low && !g.Editing && (g.Selected == 0 || g.followerOrders) && g.Routes[g.Selected].Raw[1] != 8 {
			n := int(in.Key[0] - '0')
			if g.Moving == n {
				g.endPointAdjustment()
			} else if p := g.Routes[g.Selected].Point(n); p.Present() {
				if g.separateDraft && g.Selected != 0 {
					if !g.stageDraft() {
						break
					}
				}
				g.Moving = n
				g.Center = p
			}
		}
	}
	if in.Selector1 {
		g.endPointAdjustment() // source069c stops numeric adjustment on either selector.
		g.Append()
	}
	if in.Selector2 {
		g.Moving = 0
		g.FinishEdit()
	}
	return 0, nil
}
func (g *GPS) Draw(dst []byte, w *world.World, a *cockpit.Assets) error {
	if len(dst) != 64000 || w == nil || a == nil {
		return fmt.Errorf("GPS requires320x200 prepared world/cockpit")
	}
	art, ok := a.Images[w.ID+"MAP.GS2"]
	if !ok || len(art.Pixels) != 64000 {
		return fmt.Errorf("GPS high-map artwork missing for%s", w.ID)
	}
	copy(dst, art.Pixels)
	if g.Low {
		if len(w.Grid) != 4096 || len(w.TileAtlas) != 192*144 || len(w.Overview) != 89*65 {
			return fmt.Errorf("GPS detail grid/atlas/overview missing; prepare assets13")
		}
		for y := MapClip.Top; y <= MapClip.Bottom; y++ {
			for x := MapClip.Left; x <= MapClip.Right; x++ {
				dst[y*320+x] = 0
			}
		}
		first := 0xfc0 - 64*int(g.Center.H1>>13) + int(g.Center.H0>>13) - 325
		ox := -7 - int((g.Center.H0&8191)/341)
		oy := -24 + int((g.Center.H1&8191)/455)
		for row := 0; row < 11; row++ {
			for col := 0; col < 11; col++ {
				x, y := ox+24*col, oy+18*row
				// Original37b0 rejects entirely clipped tiles before3857 atlas use.
				// Skip the native grid read too; no fabricated adjacent RAM at76000.
				if x+23 < MapClip.Left || x > MapClip.Right || y+17 < MapClip.Top || y > MapClip.Bottom {
					continue
				}
				at := first + row*64 + col
				if at < 0 || at >= len(w.Grid) {
					return fmt.Errorf("visible GPS grid index%d outside admitted domain", at)
				}
				id := int(w.Grid[at])
				if id >= 64 {
					return fmt.Errorf("GPS tile%d outside64", id)
				}
				for yy := 0; yy < 18; yy++ {
					for xx := 0; xx < 24; xx++ {
						dx, dy := x+xx, y+yy
						if dx >= MapClip.Left && dx <= MapClip.Right && dy >= MapClip.Top && dy <= MapClip.Bottom {
							dst[dy*320+dx] = w.TileAtlas[((id>>3)*18+yy)*192+(id&7)*24+xx]
						}
					}
				}
			}
		}
		for y := 0; y < 65; y++ {
			for x := 0; x < 89; x++ {
				v := w.Overview[y*89+x]
				if v != 0 {
					dst[(y+1)*320+x+230] = v
				}
			}
		}
	}
	// The main map uses ordinary shift8; the reduced overview uses shift10.
	// The source's extra low-map invocation has its own sweep increment.
	mainSweep := g.SweepPhase
	if g.Low {
		mainSweep -= 5000
	}
	if err := g.drawMapOverlay(dst, a, false, mainSweep); err != nil {
		return err
	}
	if g.Low {
		if err := g.drawMapOverlay(dst, a, true, g.SweepPhase); err != nil {
			return err
		}
		// VGA1929 flags7 centers both axes; zero pixels preserve the overview.
		mx, my := g.overviewPixel(g.Center)
		if err := a.Sprite(dst, "gps-overview-box", mx-5, my-1-4, cockpit.Full); err != nil {
			return err
		}
	}
	// The logical producer replaces the selected-actor snapshot after input;
	// drawing neither follows mutable SHIP/TARGET rows nor advances their state.
	if a.Font.Height > 0 {
		if err := drawPanel(dst, a, g.Panel); err != nil {
			return err
		}
		if g.Panel != nil && !g.Low && (g.Panel.DamageKnown || g.r11Presentation) {
			// Original ad8c's arg8 is the overview/detail selector. The detail
			// invocation preserves its reduced map instead of drawing this diagram.
			if err := drawDamageDiagram(dst, a, g.Panel.Damage); err != nil {
				return err
			}
		}
		help := cockpit.Rect{Left: 4, Top: 170, Right: 316, Bottom: 188}
		for y := 170; y <= 188; y++ {
			for x := 2; x <= 317; x++ {
				dst[y*320+x] = 8
			}
		}
		if e := a.Text(dst, a.Literals["gps-commands"], 4, 172, 2, 2, help); e != nil {
			return e
		}
		if e := g.drawLegend(dst, a, help); e != nil {
			return e
		}
	}
	if g.Editing || g.Support != nil && g.Support.Editing {
		x, y := g.CursorX, g.CursorY
		if g.Low {
			x, y = 113, 84
		}
		name := "gps-fly-cursor"
		if g.Support != nil && g.Support.Editing {
			name = "gps-support-cursor"
		}
		v, ok := a.MapVectors[name]
		if !ok {
			return fmt.Errorf("GPS %s source vector unavailable in prepared content", name)
		}
		// Source4:0e86/15f3 selects212B for Support,2144 for route editing;
		// style0107 preserves literal98h ink and uses the same source clipping.
		a.DrawVectors(dst, v, x, y, 0, 7, 0, MapClip)
	}
	return nil
}

// AdvanceUI owns source-style map phases. Original GPS poll wall-time remains
// uncalibrated; the native modal's70Hz policy is separate from frozen flight.
func (g *GPS) AdvanceUI() {
	g.BlinkPhase++
	g.SweepPhase += 5000
	if g.Low {
		g.SweepPhase += 5000 // main and reduced-map08a8 calls are both reached.
	}
}

func (g *GPS) overviewPixel(p Point) (int, int) {
	return 230 + int(int32(p.H0)/0x1702), 66 + int(int32(p.H1)/-0x2000)
}

func (g *GPS) routeLineInk(slot int) byte {
	// Source015b:08dd..0930/b10..b34 selects source group colors from the
	// retained formation leaders, not visual allegiance or selected waypoint.
	ink := byte(7)
	firstGroup := slot >= 2
	if int8(g.Routes[3].Raw[5]) == 0 {
		firstGroup = slot < 3
	}
	if int8(g.Routes[4].Raw[5]) == 0 || firstGroup {
		ink = 1
	}
	if slot == g.Selected && g.BlinkPhase&4 != 0 {
		ink |= 8
	}
	return ink
}

func (g *GPS) drawMapOverlay(dst []byte, a *cockpit.Assets, overview bool, sweep uint16) error {
	project, clip, shift := g.Pixel, MapClip, uint(8)
	verticalDivisor := int32(-455)
	if !g.Low {
		clip = cockpit.Full // source style0108 bypasses the local vector clip.
		verticalDivisor = -3279
	}
	if overview {
		project, clip, shift = g.overviewPixel, cockpit.Full, 10 // style010a, still safe inside the native target.
		verticalDivisor = -8192
	}
	// Original000a contact/object phase precedes08a8 routes/ownship.
	for _, m := range g.Markers {
		x, y := project(Point{H0: m.H0, H1: m.H1})
		a.DrawVectors(dst, a.MapVectors[m.Symbol], x, y, m.Heading, shift, m.Colour, clip)
		if m.ObjectiveTriangle && !slices.Contains(g.DeliverySites, Point{H0: m.H0, H1: m.H1}) {
			scale := shift
			if g.BlinkPhase&4 == 0 {
				scale++
			}
			a.DrawVectors(dst, a.MapVectors["map-078c"], x, y-6, 0, scale, 0, clip)
		}
	}
	projectMission := func(h0, h1 uint32) (int, int) { return project(Point{H0: h0, H1: h1}) }
	if err := a.DrawMissionMarkers(dst, g.Mission, projectMission, verticalDivisor, shift, clip); err != nil {
		return err
	}
	// The fixed P/S symbol remains underneath the delivery triangle.
	if g.BlinkPhase&4 != 0 {
		for _, site := range g.DeliverySites {
			x, y := project(site)
			a.DrawVectors(dst, a.MapVectors["map-078c"], x, y, 0, shift, 0, clip)
		}
	}
	for i := 0; i < 5; i++ {
		r := g.displayedRoute(i)
		if !g.Active[i] || r.Index() == 0 || r.Raw[1] == 0 {
			continue
		}
		ink := g.routeLineInk(i)
		points := [3]Point{r.Point(1), r.Point(2), r.Point(3)}
		line := func(from, to Point) {
			x0, y0 := project(from)
			x1, y1 := project(to)
			cockpit.Line(dst, x0, y0, x1, y1, ink, clip)
		}
		if r.Index() == 1 && points[0].Present() {
			line(g.Poses[i], points[0])
		}
		for n := 1; n < 3; n++ {
			if points[n].Present() {
				line(points[n-1], points[n])
			}
		}
		for n, p := range points {
			if !p.Present() {
				break // source0bdf stops this row at the first empty H0.
			}
			x, y := project(p)
			symbolShift := shift
			if int(r.Index()) == n+1 && g.BlinkPhase&4 == 0 {
				symbolShift++
			}
			a.DrawVectors(dst, a.MapVectors[[]string{"map-078c", "map-0773", "map-073d"}[n]], x, y, 0, symbolShift, 0, clip)
		}
	}
	if err := a.DrawSupportStrike(dst, g.SupportStrike, projectMission, shift, clip); err != nil {
		return err
	}
	px, py := project(g.Poses[0])
	heading := g.Headings[0] + uint16(g.HeadingPerturbation)
	a.DrawVectors(dst, a.MapVectors["map-07aa"], px, py, heading, shift, 0, clip)
	shadow := func(name string, x, y int, angle uint16, scale uint) {
		vectors := a.MapVectors[name]
		black := append([]cockpit.Vector(nil), vectors...)
		for i := range black {
			black[i].Colour = 0
		}
		a.DrawVectors(dst, black, x+1, y+1, angle, scale, 0, clip)
		a.DrawVectors(dst, vectors, x, y, angle, scale, 0, clip)
	}
	shadow("map-079d", px, py, g.Headings[0]+sweep, shift)
	if g.Selected != 0 && g.Active[g.Selected] {
		x, y := project(g.Poses[g.Selected])
		a.DrawVectors(dst, a.MapVectors["map-07b7"], x, y, 0, shift+uint(g.BlinkPhase&1), 0, clip)
	}
	return nil
}

// Native selector policy is input-versioned; route construction itself stays the
// existing source operation. Input v11 confirms a placed route with either
// Selector2 or F. Prior recordings retain their original rollback behavior.
func (g *GPS) SetPartialRouteConfirmation(on bool) {
	// A historical player edit stored its draft in the live route. When Resume
	// hands that gesture to the player, keep the reached points and restore the
	// accepted route until the player confirms. Read-only playback keeps its layout.
	if on && !g.confirmPartialRoutes && g.Editing && g.Selected == 0 && g.draft == nil {
		draft := g.Routes[0]
		g.draft = &draft
		g.Routes[0] = g.backup
	}
	g.confirmPartialRoutes = on
}
func (g *GPS) SetNativeSelectors(on bool)    { g.nativeSelectors = on }
func (g *GPS) SetEntryOnlySelectors(on bool) { g.entryOnlySelectors = on }
func (g *GPS) CancelPathGesture() {
	if g.draft != nil {
		g.draftOwner = 0
		g.draft = nil
	} else if g.Editing {
		g.Routes[g.Selected] = g.backup
	}
	g.Editing = false
	g.Moving = 0
}
