package navigation

import "testing"

func TestFirstSelectorStartsAndPlacesPathSecondExitsWithRollback(t *testing.T) {
	routes := [5]Route{}
	g := New(&routes, [5]Point{}, [5]bool{true})
	g.SetNativeSelectors(true)
	before := routes
	exit, e := g.Handle(Input{Pointer: true, X: 100, Y: 70, Selector1: true})
	if e != nil || exit != 0 || !g.Editing || routes[0].Index() != 1 || routes[0].Point(1).H0 != HighPoint(100, 70).H0 {
		t.Fatal(exit, e, g, routes[0])
	}
	exit, e = g.Handle(Input{Selector2: true})
	if e != nil || exit != -1 || g.Editing || routes != before {
		t.Fatal("unfinished path not cancelled", exit, e, routes)
	}
}

func TestThreePathPointsCommitAndSupportKeepsOwnership(t *testing.T) {
	routes := [5]Route{}
	g := New(&routes, [5]Point{}, [5]bool{true})
	g.SetNativeSelectors(true)
	for _, p := range [][2]int{{100, 70}, {110, 80}, {120, 90}} {
		g.Handle(Input{Pointer: true, X: p[0], Y: p[1], Selector1: true})
	}
	if g.Editing || routes[0].Point(3).H0 != HighPoint(120, 90).H0 {
		t.Fatal("commit lost")
	}
	g.EnableSupport(0x31, 0, 0)
	g.Handle(Input{Key: "m"})
	exit, e := g.Handle(Input{Pointer: true, X: 100, Y: 80, Selector1: true})
	cmd := g.TakeSupportCommand()
	if exit != 0 || e != nil || cmd == nil || cmd.Cancel || g.Editing {
		t.Fatal("support click became path")
	}
}
