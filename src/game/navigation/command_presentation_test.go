package navigation

import "testing"

func TestCommandAvailabilityTracksActorAndModalOwners(t *testing.T) {
	g := routeFixture()
	g.Active[1] = true
	g.Selected = 1
	g.Routes[1].Raw[1] = 3
	g.Routes[1].Raw[5] = 0
	g.SetFollowerOrders(true, nil)
	g.SetR11Presentation(true)
	for _, key := range []string{"n", "f", "h", "s", "a", "l", "b", "d", "g", "w"} {
		if !g.CommandAvailable(key) {
			t.Fatal("eligible command grey", key)
		}
	}
	if g.CommandAvailable("c") {
		t.Fatal("cargo invented")
	}
	g.CargoReleaseAvailable = true
	if !g.CommandAvailable("c") {
		t.Fatal("actual cargo disabled")
	}
	before := *g.Routes
	for _, modal := range []string{"detail", "route", "support"} {
		g.Low = modal == "detail"
		g.Editing = modal == "route"
		g.Support = &SupportMode{Editing: modal == "support"}
		for _, key := range []string{"h", "s", "a", "l", "b", "d", "g", "w", "c"} {
			if g.CommandAvailable(key) {
				t.Fatal(modal, key)
			}
		}
	}
	g.Low = false
	g.Editing = false
	g.Support = nil
	g.Routes[1].Raw[1] = 1
	if g.CommandAvailable("d") || g.CommandAvailable("g") {
		t.Fatal("already formed detach/regroup enabled")
	}
	g.Routes[1].Raw[1] = 0
	for _, key := range []string{"h", "w", "b"} {
		if g.CommandAvailable(key) {
			t.Fatal("inactive", key)
		}
	}
	g.Routes[1] = before[1]
	if *g.Routes != before {
		t.Fatal("presentation query mutated route")
	}
	g.SetFollowerOrders(false, nil)
	if g.CommandAvailable("h") {
		t.Fatal("historical read-only follower")
	}
}

func TestFollowerLegendPreservesHistoricalPolicy(t *testing.T) {
	g := routeFixture()
	g.Selected = 1
	g.Active[1] = true
	g.Routes[1].Raw[1] = 3
	g.SetFollowerOrders(true, nil)
	_, old := g.legend()
	if len(old) < 2 || old[0].width != 230 {
		t.Fatal("historical follower legend changed", old)
	}
	g.SetR11Presentation(true)
	_, now := g.legend()
	for _, r := range now {
		if r.width == 230 {
			t.Fatal("R1 blanket unsupported ink survived", now)
		}
	}
}
