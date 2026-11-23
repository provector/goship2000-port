package menus

import (
	"gunship2000-assets/game/avionics"
	"gunship2000-assets/game/state"
	"testing"
)

func TestActiveAndReturnedSupportMarker(t *testing.T) {
	p := &RenderingPreview{Flight: &FlightRuntime{Avionics: &avionics.State{}}, Initialized: &state.InitializedMission{Entities: []state.InitializedEntity{{RuntimeRow: 79, Kind: 0x11, Participation: 1}}}}
	a := p.Flight.Avionics
	a.Contacts[9] = avionics.Contact{Kind: 1, Flags: 1, Yaw: 321, H0: 91, H1: 92}
	a.Contacts[79] = avionics.Contact{Kind: 0x11, Flags: 1}
	frozen := a.Contacts
	e := &p.Initialized.Entities[0]
	for _, yaw := range []uint16{0, 0x2000, 0x4000, 0x8000, 0xc000, 0xffff, 0} {
		e.Pose.H0, e.Pose.H1, e.Pose.Angles[1] = 0x7ffff, 512, yaw
		v := p.ContactMapMarkers()
		if len(v) != 2 || v[1].Heading != yaw || v[1].H0 != e.Pose.H0 || v[1].H1 != 512 || v[1].Symbol != "map-0870" {
			t.Fatalf("active boundary aircraft: %+v", v)
		}
	}
	e.Participation = 0
	v := p.ContactMapMarkers()
	if len(v) != 1 || v[0].H0 != 91 || v[0].Colour != 6 || v[0].Heading != 321 || a.Contacts != frozen {
		t.Fatalf("departure changed last known contacts: %+v", v)
	}
}
