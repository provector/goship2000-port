package navigation

import "testing"

func TestOverviewKeyboardNativeDividerUsesSourcePixelSteps(t *testing.T) {
	var routes [5]Route
	for _, divisor := range []int{1, 4, 16} {
		g := New(&routes, [5]Point{}, [5]bool{})
		g.CursorX, g.CursorY = 100, 100
		// Original seven held samples16..112 give1+2+...+7=28 pixels.
		// Subsequent positive SAR-decay56,28,14,7,3,1 gives3+1=4 more.
		for _, x := range []int16{16, 32, 48, 64, 80, 96, 112, 56, 28, 14, 7, 3, 1, 0} {
			if _, e := g.Handle(g.KeyboardInput(x, 0, divisor)); e != nil {
				t.Fatal(e)
			}
		}
		if g.CursorX != 100+32/divisor || g.CursorY != 100 {
			t.Fatalf("divisor%d source32-pixel stream=>%d,%d", divisor, g.CursorX, g.CursorY)
		}
	}
}

func TestOverviewKeyboardSignedRemainderAndContextIsolation(t *testing.T) {
	var routes [5]Route
	g := New(&routes, [5]Point{}, [5]bool{})
	g.CursorX, g.CursorY = 100, 100
	// Three single-pixel positives, reversed by three negatives, make no
	// four-pixel quotient. A direction change must not round every negative
	// fraction down to a new unwanted pixel.
	for _, x := range []int16{16, 16, 16, -16, -16, -16} {
		g.Handle(g.KeyboardInput(x, 0, 4))
	}
	if g.CursorX != 100 || g.overviewKeyRemainder[0] != 0 {
		t.Fatal("asymmetric fractional rounding", g.CursorX, g.overviewKeyRemainder)
	}
	g.Handle(Input{Pointer: true, X: 200, Y: 120})
	if g.CursorX != 200 || g.CursorY != 120 {
		t.Fatal("keyboard policy changed absolute mouse")
	}
	g.Low = true
	g.Center = Point{H0: 0x40000, H1: 0x40000}
	got := g.KeyboardInput(127, -127, 4)
	if got.AxisX != 127 || got.AxisY != -127 {
		t.Fatal("detail producer divided", got)
	}
	g.Handle(got)
	if g.Center.H0 != 0x40000+4064 || g.Center.H1 != 0x40000+4064 {
		t.Fatal("detail source shift changed", g.Center)
	}
	if g.overviewKeyRemainder != [2]int{} {
		t.Fatal("detail retained overview fraction")
	}
}
