package avionics

import "testing"

func TestAirObserverActualBodyLOSAndWrappedContactOwner(t *testing.T) {
	in, _ := contactFixture(t, 0)
	in.Entities[79] = Entity{Present: true, Row: 79, FlightSlot: -1, Kind: 0x11, Participation: 1, Pose: in.Player}
	in.Entities[1].Kind = 3
	for _, sw := range []uint16{0, 0x100, 0x200} {
		in.Switches = sw
		s := New()
		draws := 0
		in.Random = func() uint16 { draws++; return 0 }
		ok, err := s.ObserveSupport(in, 79, 1)
		if err != nil || !ok || s.Contacts[1].Flags != 0x7e || draws != 0 {
			t.Fatal(sw, ok, err, s.Contacts[1], draws)
		}
	}
	// Support does not acquire through a mast even if the caller has one selected.
	in.Entities[79].Pose.Height = -64
	in.Mast = true
	in.Head.Height = 1536
	s := New()
	if ok, err := s.ObserveSupport(in, 79, 1); err != nil || ok {
		t.Fatal("body occultation", ok, err)
	}
	in.Entities[79].Present = false
	if _, err := s.ObserveSupport(in, 79, 1); err == nil {
		t.Fatal("missing actual body admitted")
	}
}
