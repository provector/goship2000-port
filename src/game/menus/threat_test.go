package menus

import (
	"gunship2000-assets/game/avionics"
	"gunship2000-assets/game/combat"
	"reflect"
	"testing"
)

func TestLogicalThreatPublicationAndDisplayGate(t *testing.T) {
	c := armedMission(t)
	p := c.Rendering
	r := p.Flight
	r.SecondaryMFD = 5
	p.Cockpit.SecondaryMFD = 5
	p.Cockpit.HUD = true
	p.ViewControls.Mode = 0
	p.ViewControls.Arrangement = 0
	// Slots are real authoritative combat storage. Placement at the body means
	// hand-computed old-zoom offsets0,0 regardless of the current frame's zoom.
	r.Combat.Slots[23] = combat.Projectile{Flags: 0x1e21, Lifetime: 131, Shooter: byte(p.Initialized.PlayerRow), Parameter: 1}
	r.Combat.Slots[23].Pose.H0 = r.State.Pose.H0
	r.Combat.Slots[23].Pose.H1 = r.State.Pose.H1
	r.Avionics.Contacts = [avionics.EntityCapacity]avionics.Contact{}
	beforeSlots := r.Combat.Slots
	if e := p.advanceAvionicsDisplays(c.Office.Assets.Cockpit); e != nil {
		t.Fatal(e)
	}
	got := r.Avionics.Threat.Projectiles
	if got.Count != 1 || got.Plots[0].Slot != 23 || got.WarningFlags != 0 {
		t.Fatal(got)
	}
	if len(p.Cockpit.ContactAvionics.ProjectilePoints) != 1 {
		t.Fatal("snapshot not published")
	}
	// Mutation of the later owner cannot alias the published value snapshot.
	r.Combat.Slots[23].Lifetime = 0
	if len(p.Cockpit.ContactAvionics.ProjectilePoints) != 1 {
		t.Fatal("published plots alias combat owner")
	}
	r.Avionics.Threat.Zoom = 7
	p.ViewControls.Mode = 7
	if e := p.advanceAvionicsDisplays(c.Office.Assets.Cockpit); e != nil {
		t.Fatal(e)
	}
	if !reflect.DeepEqual(got, r.Avionics.Threat.Projectiles) || r.Avionics.Threat.Zoom != 7 {
		t.Fatal("unreached mast display mutated threat state")
	}
	r.Combat.Slots = beforeSlots
}
