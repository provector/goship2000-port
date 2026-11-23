package menus

import "testing"

// Start with an empty-fuel wingman and an inactive reserve; the aircraft failure,
// landing, reserve activation and eventual pickup are actual simulation updates.
func TestDownedPilotGPSLifecycle(t *testing.T) {
	c := developmentScenarioController(t, "n6-rescue")
	p := c.Rendering
	p.Flight.State.Pose.Height = 64
	p.Flight.State.HeightTarget = 64
	if err := c.Handle(Actions{Cancel: true}); err != nil {
		t.Fatal(err)
	}
	seen := false
	for i := 0; i < 80; i++ {
		if err := c.AdvanceRendering(.101); err != nil {
			t.Fatal(err)
		}
		e := p.Initialized.Entities[5]
		if e.Kind == 0x1b && e.Participation == 1 && !seen {
			if err := c.openGPS(); err != nil {
				t.Fatal(err)
			}
			marked := 0
			for _, m := range p.GPS.Markers {
				if m.Symbol == "map-070d" && m.H0 == e.Pose.H0 && m.H1 == e.Pose.H1 && m.ObjectiveTriangle {
					marked++
				}
			}
			if marked != 1 {
				t.Fatalf("live rescue pilot needs one GPS triangle, got %d", marked)
			}
			for _, m := range p.ContactMapMarkers() {
				if m.ObjectiveTriangle {
					t.Fatal("GPS decoration entered cockpit map")
				}
			}
			c.closeGPS()
			seen = true
		}
		if e.Kind == 0x41 {
			if !seen || e.DynamicRaw[3] != 0 {
				t.Fatal("unexpected carrier or no survivor")
			}
			if err := c.openGPS(); err != nil {
				t.Fatal(err)
			}
			for _, m := range p.GPS.Markers {
				if m.Symbol == "map-070d" && m.H0 == e.Pose.H0 && m.H1 == e.Pose.H1 {
					t.Fatal("rescued pilot still shown", m)
				}
			}
			return
		}
	}
	t.Fatal("never reached real rescue")
}
