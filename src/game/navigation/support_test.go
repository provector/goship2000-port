package navigation

import (
	"reflect"
	"testing"
)

func TestSupportGPSRealModalCoordinatesAndAvailability(t *testing.T) {
	routes := [5]Route{}
	g := New(&routes, [5]Point{{H0: 0x40000, H1: 0x40000}}, [5]bool{true})
	if g.ReplayState().Support != nil {
		t.Fatal("historical modal changed")
	}
	g.EnableSupport(0x31, 0, 0)
	before := routes
	if _, err := g.Handle(Input{Key: "m"}); err != nil || !g.Support.Editing {
		t.Fatal(err)
	}
	if _, err := g.Handle(Input{Selector1: true, Pointer: true, X: 120, Y: 95}); err != nil {
		t.Fatal(err)
	}
	got := g.TakeSupportCommand()
	if got == nil || got.Cancel || got.Point != HighPoint(120, 95) {
		t.Fatal("reached target", got)
	}
	if g.Support.Editing || !reflect.DeepEqual(before, routes) {
		t.Fatal("support changed routes")
	}
	if g.TakeSupportCommand() != nil {
		t.Fatal("command repeated")
	}
	if a := g.CloseSupport(255); a != 0x21 {
		t.Fatal("first new request availability", a)
	}
	g.EnableSupport(0x21, 200, 0)
	if a := g.CloseSupport(200); a != 0x21 {
		t.Fatal("reopened request spent again")
	}
}
func TestSupportGPSCancelIndependentOfRouteRollback(t *testing.T) {
	routes := [5]Route{}
	g := New(&routes, [5]Point{{H0: 0x40000, H1: 0x40000}}, [5]bool{true})
	g.EnableSupport(0x31, 0, 0)
	g.Handle(Input{Key: "m"})
	g.Handle(Input{Selector2: true})
	c := g.TakeSupportCommand()
	if c == nil || !c.Cancel {
		t.Fatal(c)
	}
	if a := g.CloseSupport(0); a != 0x31 {
		t.Fatal("cancel spent availability")
	}
	g.EnableSupport(0x01, 0, 0)
	g.Handle(Input{Key: "m"})
	if g.Support.Editing {
		t.Fatal("zero highnibble availability admitted")
	}
	g.EnableSupport(0x30, 0, 0)
	g.Handle(Input{Key: "m"})
	if g.Support.Editing {
		t.Fatal("zero lownibble availability admitted")
	}
	g.EnableSupport(0x31, 0, 0)
	g.Editing = true
	g.Handle(Input{Key: "m"})
	if g.Support.Editing {
		t.Fatal("route edit admitted support")
	}
}
