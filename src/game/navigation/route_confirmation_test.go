package navigation

import "testing"

func TestPartialRouteConfirmation(t *testing.T) {
	for _, slot := range []int{0, 1} {
		for points := 0; points <= 3; points++ {
			var accepted Route
			for _, finish := range []Input{{Key: "f"}, {Selector2: true}, {Escape: true}} {
				for _, prior := range []bool{false, true} {
					g := routeFixture()
					g.Selected = slot
					g.SetNativeSelectors(true)
					g.SetEntryOnlySelectors(true)
					g.SetPartialRouteConfirmation(true)
					published := 0
					g.StageWingmanRoutes(func(int) (uint16, bool) { return 1, true })
					g.SetFollowerOrders(true, func(int) { published++ })
					r := &g.Routes[slot]
					*r = Route{}
					if prior {
						r.SetPoint(1, HighPoint(11, 12))
						r.SetIndex(1)
					}
					initial := *r
					g.Handle(Input{Key: "f"})
					for i := 0; i < points; i++ {
						g.Handle(Input{Selector1: true, Pointer: true, X: 60 + i, Y: 70 + i})
						if i < 2 && *r != initial {
							t.Fatalf("slot %d draft changed accepted route", slot)
						}
					}
					if points < 3 {
						if exit, e := g.Handle(finish); exit != 0 || e != nil {
							t.Fatal("finish exited GPS", exit, e)
						}
					}
					if g.Editing {
						t.Fatal("gesture remained active")
					}
					if points == 0 || points < 3 && finish.Escape {
						if *r != initial || published != 0 {
							t.Fatal("cancel/empty replaced previous order")
						}
						continue
					}
					if r.Index() != 1 || !r.Point(points).Present() || points < 3 && r.Point(points+1).Present() || slot == 1 && published != 1 {
						t.Fatal("wrong confirmed points or publication count", points, *r, published)
					}
					if finish.Key == "f" {
						accepted = *r
					} else if *r != accepted {
						t.Fatal("F and selector routes differ")
					}
				}
			}
		}
	}
}

func TestResumeMidHistoricalPlayerEdit(t *testing.T) {
	for _, cancel := range []bool{false, true} {
		g := routeFixture()
		g.SetNativeSelectors(true)
		g.Routes[0].SetPoint(1, HighPoint(52, 81))
		g.Routes[0].SetIndex(1)
		accepted := g.Routes[0]
		g.Handle(Input{Key: "f"})
		g.Handle(Input{Selector1: true, Pointer: true, X: 94, Y: 60})
		reached := g.Routes[0].Point(1)
		if reached == accepted.Point(1) {
			t.Fatal("historical gesture did not place a point")
		}
		g.SetPartialRouteConfirmation(true)
		if g.Routes[0] != accepted || g.displayedRoute(0).Point(1) != reached {
			t.Fatal("practice lost accepted or pending route")
		}
		g.Handle(Input{Escape: cancel, Selector2: !cancel})
		if g.Editing || cancel && g.Routes[0] != accepted || !cancel && g.Routes[0].Point(1) != reached {
			t.Fatal("incorrect resolution of resumed edit")
		}
	}
}
