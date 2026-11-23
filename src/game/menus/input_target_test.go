package menus

import (
	"bytes"
	"encoding/json"
	"gunship2000-assets/game/flight"
	"gunship2000-assets/game/render3d"
	"testing"
	"time"
)

func TestSceneKeyboardSelectorDomainsAndEdges(t *testing.T) {
	var latch inputLatch
	for _, row := range []struct {
		b             Buttons
		ord, one, two bool
	}{
		{Buttons{Space: true, OrdnanceNext: true}, true, false, false},
		{Buttons{Space: true, OrdnanceNext: true}, false, false, false},
		{Buttons{}, false, false, false},
		{Buttons{Enter: true}, false, true, false},
		{Buttons{}, false, false, false},
		{Buttons{Backspace: true}, false, false, true},
	} {
		a := normalizeSceneSelectors(latch.Read(row.b), false, false, true)
		if a.OrdnanceNext != row.ord || a.Selector1 != row.one || a.Selector2 != row.two {
			t.Fatalf("%+v -> %+v", row, a)
		}
	}
	var menu inputLatch
	if !menu.Read(Buttons{Space: true}).Selector1 {
		t.Fatal("menu Space changed")
	}
	a := normalizeSceneSelectors(Actions{Backspace: true, Enter: true, Selector1: true, Selector2: true}, false, false, false)
	if a.Selector1 || a.Selector2 || a.Backspace {
		t.Fatal("modified keyboard leaked into selectors", a)
	}
	a = normalizeSceneSelectors(Actions{PointerPressed: false}, true, true, true)
	if a.Selector1 || a.Selector2 {
		t.Fatal("capture acquisition click leaked")
	}
}

func TestOrdnanceSelectionSharesLogicalBoundaryAndNoFiring(t *testing.T) {
	c := mastFixture(t, 0, "land")
	jFinishFade(t, c)
	c.Handle(Actions{})
	p := c.Rendering
	// Explicit source selector fixture: NO BAY0 is skipped; EMPTY1 is selectable.
	ship := &p.Initialized.Stores.Ships[0]
	ship.Stations[0].CompactID = 2
	ship.Stations[1].CompactID = 0
	ship.Stations[2].CompactID = 1
	ship.Stations[3].CompactID = 3
	p.Initialized.SelectedStation = 0
	p.Cockpit.SelectedStation = 0
	before, _ := json.Marshal(p.Initialized.Stores)
	r := p.Flight
	r.commandInput.ResetTimeline()
	c.Handle(Actions{InputTime: 0, FlightHeldValid: true})
	c.Handle(Actions{InputTime: 101 * time.Millisecond, OrdnanceNext: true, FlightHeldValid: true})
	c.AdvanceRendering(.1)
	if p.Initialized.SelectedStation != 0 {
		t.Fatal("future input crossed logical boundary")
	}
	c.AdvanceRendering(.1)
	if p.Initialized.SelectedStation != 2 || p.Cockpit.SelectedStation != 2 {
		t.Fatal("did not skip NO BAY to EMPTY", p.Initialized.SelectedStation)
	}
	after, _ := json.Marshal(p.Initialized.Stores)
	if !bytes.Equal(before, after) {
		t.Fatal("selection changed ammunition/loadout/mass")
	}
	if p.Cockpit.TargetAim == nil || p.Cockpit.TargetAim.WeaponFlags != 0 {
		t.Fatal("source EMPTY prefix was hidden or guessed", p.Cockpit.TargetAim)
	}
}

func TestTargetBodyEligibilityAndFinalMastDisplayAreSeparate(t *testing.T) {
	c := mastFixture(t, 0, "land")
	jFinishFade(t, c)
	p := c.Rendering
	s := p.Flight.State
	s.Pose = flight.Pose{H0: 200000, H1: 200000, Height: 512}
	row := 1
	e := &p.Initialized.Entities[row]
	e.Pose.H0 = 200000
	e.Pose.H1 = 201600
	e.Pose.Height = 512
	p.Flight.Avionics.Target = uint16(row + 1)
	p.Flight.Avionics.Contacts[row].Kind = 3
	// Source EMPTY prefix has zero flags; targetkind3 is ineligible =>ordinal3.
	p.Initialized.Stores.Ships[0].Stations[0].CompactID = 1
	p.Initialized.SelectedStation = 0
	p.ViewControls.Mode = 7
	p.ViewControls.Camera = render3d.Camera{Pose: render3d.Pose{H0: 200000, H1: 200000, Height: 768}}
	p.publishAvionics()
	if p.Cockpit.TargetSymbol != 3 || p.Cockpit.TargetY != 20 {
		t.Fatalf("body classification/final mast projection %+v", p.Cockpit)
	}
	// Body behind camera =>0, later valid mast projection promotes to3.
	s.Pose.Angles[1] = 0x8000
	p.publishAvionics()
	if p.Cockpit.TargetSymbol != 3 {
		t.Fatal("mast did not promote body0 to source3", p.Cockpit.TargetSymbol)
	}
	p.ViewControls.Camera.Angles[1] = 0x8000
	p.publishAvionics()
	if p.Cockpit.TargetSymbol != 0 {
		t.Fatal("invalid display depth kept old symbol")
	}
}

func TestTargetProjectionKeepsIndependentWordShifts(t *testing.T) {
	c := mastFixture(t, 0, "land")
	jFinishFade(t, c)
	// Bodyx delta is (1000h>>4)-(0fffh>>4)=1; native H1delta24.
	// Source Q14 matrix has X=2000h and Z=4000h: depth=(24*4000h)>>16=6,
	// X numerator=(1*2000h)>>8=32, so retained quotient is32/6=5.
	x, y, z, ok := projectTargetPose(flight.Pose{H0: 0x1000, H1: 16 * 24, Height: 0}, flight.Pose{H0: 0x0fff}, c.Rendering.Renderer.Assets)
	if !ok || x != 5 || y != 0 || z != 6 {
		t.Fatal(x, y, z, ok)
	}
}
