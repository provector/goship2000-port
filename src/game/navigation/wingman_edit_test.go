package navigation

import "testing"

func TestWingmanEmptyRouteEditKeepsActiveOrder(t *testing.T) {
	g := routeFixture()
	g.PreserveWingmanRouteEdits()
	g.Selected = 1
	r := &g.Routes[1]
	r.Raw[1] = 3
	r.SetIndex(2)
	if err := r.SetPoint(2, Point{H0: 85000, H1: 95000}); err != nil {
		t.Fatal(err)
	}
	old := *r
	finished := 0
	g.SetFollowerOrders(true, func(int) { finished++ })
	for _, in := range []Input{{Key: "f"}, {Key: "f"}, {Escape: true}} {
		if _, err := g.Handle(in); err != nil {
			t.Fatal(err)
		}
	}
	if *r != old || finished != 0 || g.Editing {
		t.Fatalf("an empty edit changed the current wingman order: index %d, publications %d", r.Index(), finished)
	}
}

func TestWingmanRouteCompletionAndInterruptedView(t *testing.T) {
	for _, corrected := range []bool{false, true} {
		g := routeFixture()
		g.Selected = 1
		r := &g.Routes[1]
		r.SetIndex(1)
		r.Raw[1] = 3
		if err := r.SetPoint(1, Point{H0: 120000, H1: 95000}); err != nil {
			t.Fatal(err)
		}
		old := *r
		if corrected {
			g.PreserveWingmanRouteEdits()
		}
		published := 0
		g.SetFollowerOrders(true, func(slot int) {
			if slot != 1 {
				t.Fatal(slot)
			}
			published++
		})
		for _, in := range []Input{{Key: "f"}, {Selector1: true, Pointer: true, X: 80, Y: 75}, {ViewKey: 5}} {
			if _, err := g.Handle(in); err != nil {
				t.Fatal(err)
			}
		}
		g.CancelUnfinishedWingmanRoute()
		if (*r == old) != corrected || published != 0 {
			t.Fatalf("interrupted route, corrected=%t", corrected)
		}
		if !corrected {
			continue
		}
		for _, in := range []Input{{Key: "f"}, {Selector1: true, Pointer: true, X: 80, Y: 75}, {Key: "f"}} {
			if _, err := g.Handle(in); err != nil {
				t.Fatal(err)
			}
		}
		if *r == old || r.Index() != 1 || published != 1 || g.Editing {
			t.Fatal("completed replacement did not reach controller")
		}
	}
}
