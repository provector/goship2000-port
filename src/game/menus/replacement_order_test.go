package menus

import "testing"

func TestFlyToSupersedesControllableOrders(t *testing.T) {
	c := objectiveController(t, 38, 5, false)
	p := c.Rendering
	for _, tc := range []struct {
		slot                int
		kind, participation byte
		allowed             bool
	}{
		{1, 0x24, 1, true}, {2, 0x25, 1, true}, {3, 0x26, 1, true}, {4, 0x26, 2, false}, {1, 0x41, 1, false},
	} {
		e := &p.Initialized.Entities[tc.slot]
		e.Kind, e.Participation, e.ControlRaw[1] = tc.kind, tc.participation, 2
		if err := c.openGPS(); err != nil {
			t.Fatal(err)
		}
		p.GPS.Selected = tc.slot
		act := func(a Actions) {
			t.Helper()
			if err := c.handleGPS(a); err != nil {
				t.Fatal(err)
			}
		}
		act(Actions{GPSKey: "f"})
		if p.GPS.Editing != tc.allowed {
			t.Fatalf("slot %d kind %x participation %d editing %v", tc.slot, tc.kind, tc.participation, p.GPS.Editing)
		}
		if tc.allowed {
			before := e.ControlRaw
			act(Actions{Selector1: true, Inside: true, X: 115, Y: 90})
			if e.ControlRaw != before {
				t.Fatal("unconfirmed route leaked")
			}
			act(Actions{GPSKey: "f"})
			route := p.Flight.Navigation[tc.slot]
			if e.Kind != 0x23 || e.ControlRaw != route.Raw || route.Index() != 1 || !route.Point(1).Present() {
				t.Fatalf("route failed to publish to aircraft %d: kind %x index %d", tc.slot, e.Kind, route.Index())
			}
		}
		c.closeGPS()
	}
}
