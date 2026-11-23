package menus

import (
	"encoding/binary"
	"testing"

	"gunship2000-assets/game/navigation"
)

func TestGPSDeliveryDestinationThroughCompletion(t *testing.T) {
	for _, withoutTroops := range []bool{false, true} {
		name := "intact site"
		if withoutTroops {
			name = "site without infantry"
		}
		t.Run(name, func(t *testing.T) {
			c := objectiveController(t, 1, 5, true)
			p := c.Rendering
			raw := p.ResultTargets.Records[2]
			if binary.LittleEndian.Uint16(raw[32:]) != 1 {
				t.Fatal("fixture needs an active delivery")
			}
			site := navigation.Point{H0: binary.LittleEndian.Uint32(raw[34:]), H1: binary.LittleEndian.Uint32(raw[38:])}
			check := func(want int) {
				t.Helper()
				nav := p.Flight.Navigation
				if e := c.openGPS(); e != nil {
					t.Fatal(e)
				}
				if sites := p.GPS.DeliverySites; len(sites) != want || want == 1 && sites[0] != site {
					t.Fatalf("delivery sites %v, want %d at %v", sites, want, site)
				}
				c.closeGPS()
				if nav != p.Flight.Navigation {
					t.Fatal("GPS altered a flight route")
				}
			}
			if withoutTroops {
				for i := range p.Initialized.Entities {
					e := &p.Initialized.Entities[i]
					if e.Object.Raw[0]&0x10 != 0 && e.Kind == 0x15 {
						e.Kind = 0x41
						e.Participation = 0
					}
				}
			}
			check(0)
			check(0)
			// Place the running aircraft on a low approach. Only flight LOS may sight the formation.
			s := p.Flight.State
			s.Pose.H0, s.Pose.H1, s.Pose.Height = site.H0, site.H1-1600, 150
			s.Pose.Angles = [3]uint16{}
			s.HeightTarget = 150
			check(0)
			if e := c.AdvanceRendering(.12); e != nil {
				t.Fatal(e)
			}
			check(1)
			s.Pose.H1 -= 100000
			if e := c.AdvanceRendering(.12); e != nil {
				t.Fatal(e)
			}
			check(1)
			// Both roles may be bound to the same visible site. Preserve one triangle.
			goals, e := c.gpsPanelObjectives()
			if e != nil {
				t.Fatal(e)
			}
			objectiveApproach(t, c)
			if e := c.AdvanceRendering(.12); e != nil {
				t.Fatal(e)
			}
			if p.ResultTargets.Records[2][32] != 0 {
				t.Fatal("cargo delivery incomplete")
			}
			check(0)
			goals[1] = goals[0]
			for i := range p.Initialized.Entities {
				obj := &p.Initialized.Entities[i].Object
				if obj.Raw[0]&0x10 != 0 {
					obj.Raw[0] |= 0x20
				}
			}
			if sites := p.discoveredDeliverySites(goals); len(sites) != 1 || sites[0] != site {
				t.Fatal("shared site", sites)
			}
			goals[0].Available = false
			if sites := p.discoveredDeliverySites(goals); len(sites) != 1 || sites[0] != site {
				t.Fatal("secondary site", sites)
			}
			goals[1].Raw[32] = 2
			if sites := p.discoveredDeliverySites(goals); len(sites) != 0 {
				t.Fatal("inactive or search objective marked", sites)
			}

		})
	}
}
