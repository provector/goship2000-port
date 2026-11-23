package navigation

import "testing"

func TestWingmanDraftCannotReplaceNewOrder(t *testing.T) {
	g := routeFixture()
	g.Selected = 1
	g.PreserveWingmanRouteEdits()
	g.StageWingmanRoutes(func(int) (uint16, bool) { return 0, true })
	g.SetFollowerOrders(true, func(int) { t.Fatal("published an obsolete order") })
	before := g.Routes[1]
	if err := g.StartEdit(); err != nil {
		t.Fatal(err)
	}
	g.Append()
	if g.Routes[1] != before {
		t.Fatal("unconfirmed waypoint escaped into live aircraft controls")
	}
	g.Routes[1].Raw[1] = 8 // the live aircraft has received a return order
	g.FinishEdit()
	if g.Routes[1].Raw[1] != 8 || g.Editing {
		t.Fatal("lost newer return order")
	}
}

func TestWingmanDraftCancelEmptyAndTerminalTransition(t *testing.T) {
	g := routeFixture()
	g.Selected = 1
	var owner uint16 = 0x2101
	g.StageWingmanRoutes(func(int) (uint16, bool) { return owner, true })
	count := 0
	g.SetFollowerOrders(true, func(int) { count++ })
	g.PreserveWingmanRouteEdits()
	g.Routes[1] = Route{}
	for _, key := range []string{"f", "f", "f", "n"} {
		if _, err := g.Handle(Input{Key: key}); err != nil {
			t.Fatal(err)
		}
	}
	if count != 0 || !g.Editing || g.Selected != 1 {
		t.Fatal("empty or switched edit published")
	}
	g.Append()
	owner = 0x2601
	g.FinishEdit()
	if g.Editing || g.Routes[1] != (Route{}) || count != 0 {
		t.Fatal("stale edit overwrote later state")
	}
	for _, cancel := range []Input{{Escape: true}, {ViewKey: 5}} {
		g.StartEdit()
		g.Append()
		g.Handle(cancel)
		g.CancelUnfinishedWingmanRoute()
		if g.Routes[1] != (Route{}) || g.Editing || count != 0 {
			t.Fatal("leaked interrupted draft")
		}
	}
	g.StartEdit()
	g.Append()
	g.FinishEdit()
	if count != 1 || g.Routes[1].Index() != 1 || !g.Routes[1].Point(1).Present() {
		t.Fatal("one-point route missing")
	}
}
