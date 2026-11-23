package navigation

import (
	"bytes"
	"encoding/json"
	"gunship2000-assets/game/cockpit"
	"gunship2000-assets/game/world"
	"testing"
)

// Cases come from source4's control-flow operands, not a captured port route.
// Escape's REP MOVSW23 restores the start-of-command row; F1..F9 exits earlier.
func TestGPSReviewedTransactionSequences(t *testing.T) {
	t.Run("A high hidden movement and low viewing pan", func(t *testing.T) {
		g := routeFixture()
		before := *g.Routes
		g.Handle(Input{AxisX: 32, AxisY: -16})
		if g.Editing || *g.Routes != before {
			t.Fatal("view moved route")
		}
		g.Handle(Input{Key: "space"})
		start := g.Center
		g.Handle(Input{AxisX: 16, AxisY: -16})
		if !g.Low || g.Center.H0 != start.H0+512 || g.Center.H1 != start.H1+512 || *g.Routes != before {
			t.Fatal("normal detail navigation must remain available")
		}
	})
	t.Run("B finish then escape retains committed route", func(t *testing.T) {
		g := routeFixture()
		g.Handle(Input{Key: "f"})
		g.Handle(Input{Pointer: true, X: 100, Y: 100, Selector1: true})
		g.Handle(Input{Key: "f"})
		committed := g.Routes[0]
		exit, e := g.Handle(Input{Escape: true})
		if e != nil || exit != -1 || g.Editing || g.Routes[0] != committed || committed.Index() != 1 {
			t.Fatal("completed command was incorrectly rolled back/deleted")
		}
	})
	t.Run("C cancel restores prior row", func(t *testing.T) {
		g := routeFixture()
		original := g.Routes[0]
		g.Handle(Input{Key: "f"})
		g.Handle(Input{Selector1: true})
		exit, e := g.Handle(Input{Escape: true})
		if e != nil || exit != 0 || g.Editing || g.Routes[0] != original {
			t.Fatal("full70-byte rollback")
		}
	})
	t.Run("D finishing empty route deletes", func(t *testing.T) {
		g := routeFixture()
		g.Handle(Input{Key: "f"})
		g.Handle(Input{Key: "f"})
		exit, e := g.Handle(Input{Escape: true})
		if e != nil || exit != -1 || g.Routes[0].Index() != 0 {
			t.Fatal("empty completion")
		}
		for n := 1; n <= 3; n++ {
			if g.Routes[0].Point(n).Present() {
				t.Fatal("empty not deleted")
			}
		}
	})
	t.Run("E new command rollback snapshot is completed route", func(t *testing.T) {
		g := routeFixture()
		g.Handle(Input{Key: "f"})
		g.Handle(Input{Selector1: true})
		g.Handle(Input{Key: "f"})
		completed := g.Routes[0]
		g.Handle(Input{Key: "f"})
		exit, e := g.Handle(Input{Escape: true})
		if e != nil || exit != 0 || g.Routes[0] != completed || g.Editing {
			t.Fatal("second F snapshots completed route before clearing")
		}
	})
	t.Run("F view exit writes through before rollback", func(t *testing.T) {
		g := routeFixture()
		g.Handle(Input{Key: "f"})
		g.Handle(Input{Selector1: true})
		g.Handle(Input{Selector1: true, Pointer: true, X: 130, Y: 100})
		partial := g.Routes[0]
		exit, e := g.Handle(Input{ViewKey: 3, Escape: true})
		if e != nil || exit != 3 || g.Routes[0] != partial || partial.Index() != 2 {
			t.Fatal("source earlier view exit")
		}
	})
	t.Run("G space transfers viewport center without committing", func(t *testing.T) {
		g := routeFixture()
		g.Handle(Input{Key: "f"})
		g.Handle(Input{Pointer: true, X: 120, Y: 90})
		g.Handle(Input{Key: "space"})
		want := HighPoint(120, 90)
		if !g.Editing || !g.Low || g.Center != want || g.Routes[0].Index() != 0 {
			t.Fatal("editing scale transfer")
		}
		g.Handle(Input{Key: "space"})
		if g.Low || !g.Editing || g.CursorX != 120 || g.CursorY != 90 || g.Center.H0 != 0 || g.Center.H1 != 0 {
			t.Fatalf("signed truncation transfer: %d,%d", g.CursorX, g.CursorY)
		}
	})
}

func TestGPSNumericAdjustmentStopsOnEitherSelector(t *testing.T) {
	for _, selector := range []Input{{Selector1: true}, {Selector2: true}} {
		g := routeFixture()
		g.Low = true
		g.Handle(Input{Key: "1"})
		g.Handle(Input{AxisX: 16})
		changed := g.Routes[0].Point(1)
		g.Handle(selector)
		if g.Moving != 0 {
			t.Fatal("source069c selector did not end adjustment")
		}
		g.Handle(Input{AxisX: 16})
		if g.Routes[0].Point(1) != changed {
			t.Fatal("finished point continued following map pan")
		}
	}
	g := routeFixture()
	g.Low = true
	g.Handle(Input{Key: "1"})
	before := g.Routes[0]
	g.Low = false
	g.Handle(Input{AxisX: 16})
	if g.Routes[0] != before {
		t.Fatal("high-map hidden cursor must not run low-map point writer")
	}
}

func TestGPSViewingSelectorOneExitAndRightSelectorNoExit(t *testing.T) {
	g := routeFixture()
	before := g.Routes[0]
	exit, err := g.Handle(Input{Selector1: true})
	if err != nil || exit != -1 || g.Routes[0] != before {
		t.Fatal("source034b ordinary Selector1 leaves GPS without route mutation")
	}
	exit, err = g.Handle(Input{Selector2: true})
	if err != nil || exit != 0 || g.Routes[0] != before {
		t.Fatal("ordinary Selector2 is not an invented GPS exit")
	}
}

func gpsDrawingFixture() (*GPS, *cockpit.Assets, *world.World) {
	var routes [5]Route
	g := New(&routes, [5]Point{{H0: 0x30000, H1: 0x30000}}, [5]bool{true})
	sine := make([]int16, 1025)
	sine[1024] = 16384
	image := bytes.Repeat([]byte{42}, 64000)
	a := &cockpit.Assets{Sine: sine, Images: map[string]cockpit.Image{"TESTMAP.GS2": {Pixels: image}}, Sprites: map[string]cockpit.Image{"gps-overview-box": {Width: 11, Height: 9, Pixels: make([]byte, 99)}}, MapVectors: map[string][]cockpit.Vector{
		// Original13 bytes:9801fe000200980100fe000200, independently framed.
		"gps-fly-cursor": {{Colour: 0x98, Points: [][2]int16{{-2, 0}, {2, 0}}}, {Colour: 0x98, Points: [][2]int16{{0, -2}, {0, 2}}}},
	}}
	w := &world.World{ID: "TEST", Grid: make([]byte, 4096), TileAtlas: bytes.Repeat([]byte{3}, 192*144), Overview: make([]byte, 89*65)}
	return g, a, w
}
func TestGPSCursorSourceGlyphAndViewGate(t *testing.T) {
	g, a, w := gpsDrawingFixture()
	dst := make([]byte, 64000)
	if err := g.Draw(dst, w, a); err != nil {
		t.Fatal(err)
	}
	if dst[80*320+112] != 42 {
		t.Fatal("viewing must not display a free crosshair")
	}
	g.Editing = true
	if err := g.Draw(dst, w, a); err != nil {
		t.Fatal(err)
	}
	for n := -4; n <= 4; n++ {
		if dst[80*320+112+n] != 0x98 || dst[(80+n)*320+112] != 0x98 {
			t.Fatal("source98h glyph/hotspot/scale")
		}
	}
	if dst[80*320+107] != 42 || dst[75*320+112] != 42 {
		t.Fatal("cursor exceeds source endpoints")
	}
	g.Low = true
	g.Center = Point{H0: 0x30000, H1: 0x30000}
	if err := g.Draw(dst, w, a); err != nil {
		t.Fatal(err)
	}
	for n := -4; n <= 4; n++ {
		if dst[84*320+113+n] != 0x98 || dst[(84+n)*320+113] != 0x98 {
			t.Fatal("detail edit crosshair not centered")
		}
	}
	g.Editing = false
	if err := g.Draw(dst, w, a); err != nil {
		t.Fatal(err)
	}
	if dst[84*320+113] != 3 {
		t.Fatal("detail browsing crosshair visible")
	}
}
func TestGPSOverviewPositionAndSourceWaypointSymbols(t *testing.T) {
	g, a, w := gpsDrawingFixture()
	g.Low = true
	g.Center = Point{H0: 0x30000, H1: 0x30000}
	w.Overview[0] = 77
	dst := make([]byte, 64000)
	if err := g.Draw(dst, w, a); err != nil {
		t.Fatal(err)
	}
	if dst[1*320+230] != 77 || dst[4*320+186] != 3 {
		t.Fatal("source0448 args are x230,y1,flags4, not centered230,y4")
	}
	// Source waypoint1 is the numbered triangle from015b:078c; no HUD bitmap.
	g.Low = false
	g.Poses[0] = HighPoint(100, 80)
	g.Routes[0].SetIndex(1)
	g.Routes[0].Raw[1] = 3
	g.Routes[0].SetPoint(1, HighPoint(150, 80))
	g.BlinkPhase = 4
	a.MapVectors["map-078c"] = []cockpit.Vector{{Colour: 10, Points: [][2]int16{{0, 2}, {4, -2}, {-4, -2}, {0, 2}}}, {Colour: 13, Points: [][2]int16{{0, 2}, {0, -2}}}}
	before, _ := json.Marshal(g)
	if err := g.Draw(dst, w, a); err != nil {
		t.Fatal(err)
	}
	after, _ := json.Marshal(g)
	if !bytes.Equal(before, after) {
		t.Fatal("drawing advanced GPS state")
	}
	if dst[78*320+150] != 13 || dst[82*320+154] != 10 {
		t.Fatal("source numbered triangle literal palettes/shift8")
	}
	if dst[80*320+140] != 9 {
		t.Fatal("selected route line uses sourcegroup1 OR8, not26/31")
	}
	// High0108 bypasses local map clipping; the literal triangle endpoint is
	// allowed in the three-pixel margin. Detail0008 retains MapClip instead.
	g.Routes[0].SetPoint(1, HighPoint(225, 80))
	if err := g.Draw(dst, w, a); err != nil {
		t.Fatal(err)
	}
	if dst[82*320+229] != 10 {
		t.Fatal("high-map symbol was incorrectly cropped at local map right226")
	}
}
func TestGPSUIPhasesFollowReachedMapInvocations(t *testing.T) {
	g := routeFixture()
	g.BlinkPhase = 7
	g.SweepPhase = 65000
	g.AdvanceUI()
	if g.BlinkPhase != 8 || g.SweepPhase != 4464 {
		t.Fatal("sourceword sweep wrap")
	}
	g.Low = true
	g.AdvanceUI()
	if g.SweepPhase != 14464 || g.BlinkPhase != 9 {
		t.Fatal("main plus overview invocation")
	}
}
