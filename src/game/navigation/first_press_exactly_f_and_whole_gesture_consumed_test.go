package navigation

import (
	"reflect"
	"testing"
)

func TestFirstPressExactlyFAndWholeGestureConsumed(t *testing.T) {
	for _, xy := range [][2]int{{112, 80}, {190, 45}} {
		a, b := [5]Route{}, [5]Route{}
		a[0].SetPoint(1, Point{H0: 150000, H1: 120000})
		a[0].SetIndex(1)
		b[0] = a[0]
		g := New(&a, [5]Point{{H0: 100000, H1: 200000}}, [5]bool{true})
		k := New(&b, g.Poses, g.Active)
		g.SetNativeSelectors(true)
		g.SetEntryOnlySelectors(true)
		before := a
		g.Handle(Input{Pointer: true, X: xy[0], Y: xy[1], Selector1: true, PrimaryHeld: true, HoldValid: true})
		k.Handle(Input{Pointer: true, X: xy[0], Y: xy[1], Key: "f"})
		if g.backup != before[0] || g.backup != k.backup || a != b || a[0].Index() != 0 || !g.Editing || g.CursorX != k.CursorX || g.CursorY != k.CursorY {
			t.Fatal("entry differs from F", a, b, g, k)
		}
		for i := 0; i < 20; i++ {
			g.Handle(Input{Selector1: true, PrimaryHeld: true, HoldValid: true})
		}
		if a != b {
			t.Fatal("held/repeat placed")
		}
		g.Handle(Input{HoldValid: true})
		if a != b {
			t.Fatal("release placed")
		}
		g.Handle(Input{Pointer: true, X: xy[0], Y: xy[1], Selector1: true, PrimaryHeld: true, HoldValid: true})
		if a[0].Index() != 1 || a[0].Point(1).H0 != HighPoint(xy[0], xy[1]).H0 {
			t.Fatal("fresh click wrong position")
		}
		exit, _ := g.Handle(Input{Selector2: true})
		if exit != 0 || g.Editing || a != before {
			t.Fatal("cancel should stay GPS and roll back")
		}
		exit, _ = g.Handle(Input{Selector2: true})
		if exit != -1 {
			t.Fatal("ordinary secondary exit")
		}
	}
}
func TestSupportOwnerAndHistoricalSelectorPolicy(t *testing.T) {
	r := [5]Route{}
	g := New(&r, [5]Point{}, [5]bool{true})
	g.SetNativeSelectors(true)
	g.SetEntryOnlySelectors(true)
	g.EnableSupport(0x31, 0, 0)
	g.Handle(Input{Key: "m"})
	before := r
	g.Handle(Input{Selector1: true, Pointer: true, X: 120, Y: 80})
	if g.TakeSupportCommand() == nil || r != before {
		t.Fatal("Support stolen")
	}
	old := New(&r, [5]Point{}, [5]bool{true})
	old.SetNativeSelectors(true)
	old.Handle(Input{Selector1: true, Pointer: true, X: 100, Y: 70})
	if old.Routes[0].Index() != 1 {
		t.Fatal("v7/v8 semantics changed")
	}
	if reflect.DeepEqual(old.ReplayState(), g.ReplayState()) {
		t.Fatal("fixture not discriminating")
	}
}
