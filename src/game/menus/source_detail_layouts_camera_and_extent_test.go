package menus

import (
	"bytes"
	"gunship2000-assets/game/world"
	"reflect"
	"testing"
)

func TestSourceDetailLayoutsCameraAndExtent(t *testing.T) {
	// Independent operands:20f4:0ab0..0b3b,5039..506d; no screenshot goldens.
	wantBounds := [4][4]int{{0, 0, 320, 200}, {170, 65, 147, 123}, {170, 3, 147, 194}, {3, 3, 160, 128}}
	wantRef := [4][2]int{{5, 130}, {231, 0}, {231, 0}, {171, 65}}
	for n := 0; n < 4; n++ {
		v := newEditorMapView(n)
		if v.Bounds != wantBounds[n] || v.Reference != wantRef[n] {
			t.Fatal(n, v)
		}
		r := wantBounds[n]
		v.CameraX = -10000
		v.CameraY = 10000
		v.clamp()
		if v.CameraX != float32(-r[0]) || v.CameraY != float32(r[1]+952) {
			t.Fatal("upper-left", n, v)
		}
		x, y := v.point(r[0], r[1])
		if x != 0 || y != 0 {
			t.Fatal("upper-left projection", x, y)
		}
		v.CameraX = 10000
		v.CameraY = -10000
		v.clamp()
		x, y = v.point(r[0]+r[2], r[1]+r[3])
		if x != 1536 || y != 1152 {
			t.Fatal("inclusive lower-right", n, x, y)
		}
		for _, p := range [][2]int{{r[0], r[1]}, {r[0] + r[2], r[1] + r[3]}} {
			x, y = v.point(p[0], p[1])
			sx, sy := v.screen(x, y)
			if sx != p[0] || sy != p[1] {
				t.Fatal("roundtrip", p, sx, sy)
			}
		}
	}
	v := newEditorMapView(1)
	if got := v.extent(); got != [4]int{281, 28, 8, 7} {
		t.Fatal("source yellow extent", got)
	}
	v = newEditorMapView(3)
	v.CameraX = -3.5
	x, _ := v.point(4, 3)
	if x != 1 {
		t.Fatal("truncate top-left before cursor delta", x)
	}
	v.layout(5)
	if v.Bounds != wantBounds[1] {
		t.Fatal("E289 mask")
	}
	// Source10f2 recenter uses current divisors, not reset constants.
	v.ReferenceScaleX = 20
	v.ReferenceScaleY = 25
	v.recenter(251, 20)
	if v.CameraX != 157 || v.CameraY != 578 {
		t.Fatal("current reference scales", v.CameraX, v.CameraY)
	}
	if x, y := editorOverviewPoint(3, 3); x != -9 || y != -9 {
		t.Fatal("overview doubles changed", x, y)
	}
}

func TestDetailTerrainUsesActualTilePixelsAndClips(t *testing.T) {
	w := &world.World{Grid: make([]byte, 4096), TileAtlas: make([]byte, 192*144)}
	// Explicit tile2 at row0,col1, one source pixel at its local(0,0).
	w.Grid[1] = 2
	w.TileAtlas[48] = 77
	v := newEditorMapView(3)
	v.CameraX = 21
	v.CameraY = 955
	f := bytes.Repeat([]byte{99}, 64000)
	drawEditorDetailTerrain(f, w, &v)
	if f[3*320+3] != 77 || f[3*320+2] != 99 || f[2*320+3] != 99 {
		t.Fatal("tile origin or clip", f[3*320+3], f[3*320+2], f[2*320+3])
	}
}

func TestActualMapPressMoveReleaseAndHeldScroll(t *testing.T) {
	c := &FrontEnd{Editor: &MissionEditor{Stage: "edit", Layout: 3, View: newEditorMapView(3), CursorX: 100, CursorY: 70}}
	c.Editor.View.Detail = true
	var latch inputLatch
	step := func(b Buttons) {
		a := latch.Read(b)
		a.PointerMoved = b.X != c.Editor.CursorX || b.Y != c.Editor.CursorY
		c.editorMoveCursor(a, a.Inside && (a.PointerMoved || a.PointerPressed))
		if _, e := c.handleEditorMapInput(a, a.Selector1, a.Selector2 && !a.Enter); e != nil {
			t.Fatal(e)
		}
	}
	step(Buttons{X: 100, Y: 70, Inside: true, RightMouse: true})
	step(Buttons{X: 110, Y: 75, Inside: true, RightMouse: true})
	if c.Editor.View.CameraX != 678 || c.Editor.View.CameraY != 517 {
		t.Fatal("09ba current pan", c.Editor.View)
	}
	step(Buttons{X: 110, Y: 75, Inside: true})
	if c.Editor.View.drag != "" {
		t.Fatal("release retained pan")
	}
	step(Buttons{X: 180, Y: 70, Inside: true, LeftMouse: true})
	step(Buttons{X: 200, Y: 80, Inside: true, LeftMouse: true})
	if c.Editor.View.Reference != [2]int{191, 75} {
		t.Fatal("reference drag", c.Editor.View.Reference)
	}
	step(Buttons{X: 200, Y: 80, Inside: true})
	if c.Editor.View.drag != "" {
		t.Fatal("reference release")
	}
	c.Editor.Held = true
	c.Editor.AutoScroll = true
	step(Buttons{X: 60, Y: 60, Inside: true})
	step(Buttons{X: 26, Y: 60, Inside: true})
	if c.Editor.View.CameraX != 677 || c.Editor.CursorX != 27 {
		t.Fatal("78b4 inset scroll", c.Editor.View, c.Editor.CursorX)
	}
	step(Buttons{X: 27, Y: 60, Inside: true})
	if c.Editor.View.CameraX != 677 {
		t.Fatal("stationary inset scrolled")
	}
}

func TestActualInputPreviewAdmissionAndKeyboardOwnership(t *testing.T) {
	c := authoredUIController(t)
	if err := c.openMissionFiles(); err != nil {
		t.Fatal(err)
	}
	var l inputLatch
	step := func(b Buttons) {
		t.Helper()
		if err := c.Handle(l.Read(b)); err != nil {
			t.Fatal(err)
		}
	}
	step(Buttons{X: 5, Y: 5, Inside: true, MenuEnd: true})
	r := c.missionFilesArea(21)
	if c.Files.Focus != 21 || c.Office.PointerX != r[0]+r[2]/2 || c.Files.PressedKey != 0 {
		t.Fatal("keyboard AREA feedback", c.Files, c.Office.PointerX)
	}
	// Mode1 uses EDITOR left art, and focus leaves source normal-style1.
	f := c.Frame(false)
	for y := 0; y < 190; y++ {
		if !bytes.Equal(f[y*320:y*320+160], c.editorAssets().Background[y*320:y*320+160]) {
			t.Fatal("mode1 background", y)
		}
	}
	step(Buttons{X: 5, Y: 5, Inside: true, Enter: true})
	if c.Editor == nil || c.Editor.Stage != "world" {
		t.Fatal("NEW did not activate on press")
	}
	step(Buttons{X: 5, Y: 5, Inside: true, Enter: true})
	if c.Editor.Document != nil {
		t.Fatal("opening held Enter leaked")
	}
	step(Buttons{X: 160, Y: 100, Inside: true})
	before := c.Store.State()
	c.takeAudioCues()
	step(Buttons{X: 160, Y: 100, Inside: true, Enter: true})
	if c.Editor.Document != nil || !reflect.DeepEqual(before, c.Store.State()) {
		t.Fatal("no-hit Enter accepted/mutated")
	}
	if got := c.takeAudioCues(); !reflect.DeepEqual(got, []uint16{gs4Denied}) {
		t.Fatal("wrong GS21 domain", got)
	}
	step(Buttons{X: 160, Y: 100, Inside: true})
	step(Buttons{X: 160, Y: 100, Inside: false, LeftMouse: true})
	if c.Editor.Document != nil {
		t.Fatal("disabled-capture fallback")
	}
	step(Buttons{X: 160, Y: 100, Inside: true})
	p := c.editorAssets().Previews[editorMapKey(c.Editor.Worlds[0])]
	for _, name := range c.Editor.Worlds {
		p := c.editorAssets().Previews[editorMapKey(name)]
		for _, xy := range [][2]int{{p.X, p.Y}, {p.X + p.Width, p.Y + p.Height}} {
			if c.editorHit(xy[0], xy[1]) < 300 {
				t.Fatal("inclusive preview edge", name, xy)
			}
		}
	}
	step(Buttons{X: p.X + p.Width, Y: p.Y + p.Height, Inside: true, LeftMouse: true})
	if c.Editor.Document == nil {
		t.Fatal("actual inclusive preview rejected")
	}
	step(Buttons{X: 80, Y: 60, Inside: true})
	step(Buttons{X: 80, Y: 60, Inside: true, Right: true})
	step(Buttons{X: 80, Y: 60, Inside: true, Right: true})
	step(Buttons{X: 80, Y: 60, Inside: true})
	if c.Editor.CursorX != 81 || c.Office.PointerX != 81 {
		t.Fatal("parked mouse stole keyboard cursor", c.Editor.CursorX, c.Office.PointerX)
	}
	step(Buttons{X: 81, Y: 60, Inside: true})
	if c.Editor.CursorX != 81 {
		t.Fatal("mouse intervention")
	}
	editorKey(t, c, "p")
	row := editorPopupArea(0, 8)
	step(Buttons{X: row[0] + 2, Y: row[1] + 2, Inside: true})
	step(Buttons{X: row[0] + 2, Y: row[1] + 2, Inside: true, Down: true})
	step(Buttons{X: row[0] + 2, Y: row[1] + 2, Inside: true})
	// P3 source4308 uses normalized cursor motion, not linear index
	// navigation. Down moves1px within the same6px inclusive row.
	if c.Editor.CursorY != row[1]+3 || c.Editor.Option != 0 {
		t.Fatal("source spatial popup movement lost", c.Editor.CursorY, c.Editor.Option)
	}
	step(Buttons{X: row[0] + 3, Y: row[1] + 2, Inside: true})
	if c.Editor.Option != 0 {
		t.Fatal("real pointer movement failed to reclaim row")
	}
}

func TestActualBrowserSelectorPressIsolationAndCancelledBevel(t *testing.T) {
	c := authoredUIController(t)
	if err := c.openMissionFiles(); err != nil {
		t.Fatal(err)
	}
	var l inputLatch
	step := func(b Buttons) {
		t.Helper()
		if err := c.Handle(l.Read(b)); err != nil {
			t.Fatal(err)
		}
	}
	r := c.missionFilesArea(21)
	x, y := r[0]+2, r[1]+2
	idle := append([]byte(nil), c.Frame(false)...)
	step(Buttons{X: 100, Y: 100, Inside: true, LeftMouse: true})
	step(Buttons{X: x, Y: y, Inside: true, LeftMouse: true})
	if c.Screen != ScreenMissionFiles || c.Editor != nil || c.Files.PressedKey != 0 {
		t.Fatal("held-entry reactivated", c.Screen, c.Files)
	}
	step(Buttons{X: x, Y: y, Inside: true})
	step(Buttons{X: x, Y: y, Inside: true, LeftMouse: true})
	if c.Editor == nil || c.Editor.Stage != "world" || c.Files.PressedKey != 21 {
		t.Fatal("press was delayed until release")
	}
	step(Buttons{X: 160, Y: 100, Inside: true, LeftMouse: true})
	if c.Editor.Document != nil {
		t.Fatal("held NEW drag leaked selection")
	}
	step(Buttons{X: 160, Y: 100, Inside: true, LeftMouse: true, Escape: true})
	if c.Screen != ScreenMissionFiles || c.Files.PressedKey != 0 {
		t.Fatal("cancel retained activated bevel")
	}
	after := c.Frame(false)
	for yy := r[1]; yy < r[1]+r[3]; yy++ {
		if !bytes.Equal(idle[yy*320+r[0]:yy*320+r[0]+r[2]], after[yy*320+r[0]:yy*320+r[0]+r[2]]) {
			t.Fatal("cancelled NEW not source normal style1", yy)
		}
	}
	// Nonempty lists also keep selection and activated press separate.
	c.Files.Focus = 15
	c.Files.PressedKey = 0
	frame := c.Frame(false)
	r = c.missionFilesArea(15)
	c.Files.Focus = 21
	other := c.Frame(false)
	for yy := r[1]; yy < r[1]+r[3]; yy++ {
		if !bytes.Equal(frame[yy*320+r[0]:yy*320+r[0]+r[2]], other[yy*320+r[0]:yy*320+r[0]+r[2]]) {
			t.Fatal("focus depressed BEGIN")
		}
	}
}

func TestMapAndInformationSourceAreasAndReturn(t *testing.T) {
	c := authoredUIController(t)
	if err := c.openMissionFiles(); err != nil {
		t.Fatal(err)
	}
	editorKey(t, c, "n")
	p := c.editorAssets().Previews[editorMapKey(c.Editor.Worlds[0])]
	editorClick(t, c, p.X+2, p.Y+2)
	before := *c.Editor.Document
	// OriginalDS2a06=(139,132,22,7) is command16/M, while
	// DS2a0f=(154,146,9,8) is command17/?. These were swapped in P.
	editorClick(t, c, 140, 133)
	if !c.Editor.View.Detail {
		t.Fatal("source MAP AREA did not match M")
	}
	editorKey(t, c, ",")
	if c.Editor.Layout != 0 || c.Editor.View.Bounds != [4]int{0, 0, 320, 200} {
		t.Fatal("source full layout")
	}
	editorAction(t, c, Actions{Cancel: true})
	if c.Editor.Layout != 1 || !c.Editor.View.Detail {
		t.Fatal("full layout Escape")
	}
	editorKey(t, c, "M")
	if c.Editor.View.Detail {
		t.Fatal("M did not return to overview")
	}
	editorClick(t, c, 155, 147)
	if c.Editor.ControlReturn != "information" {
		t.Fatal("source HELP AREA did not match ?")
	}
	editorKey(t, c, "b")
	if c.Editor.Stage != "option" {
		t.Fatal("base selector")
	}
	editorKey(t, c, "?")
	if c.Editor.ControlReturn != "" || c.Editor.Stage != "option" {
		t.Fatal("popup swallowed information key")
	}
	if !reflect.DeepEqual(before, *c.Editor.Document) {
		t.Fatal("view or information changed draft")
	}
}
