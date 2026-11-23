package avionics

import "testing"

func tailFixture(t *testing.T) (*State, Input, *[5]int8) {
	in, _ := contactFixture(t, 0)
	for i := 1; i < 5; i++ {
		in.Entities[i] = Entity{Present: true, Row: i, Kind: 0x21, Participation: 1, FlightSlot: i, Pose: in.Player}
		in.Entities[i].Object[10] = 0x21
		in.Entities[i].Control[1] = 1
	}
	in.Entities[10] = Entity{Present: true, Row: 10, Kind: 3, Participation: 1, FlightSlot: -1, Pose: Pose{H0: in.Player.H0, H1: in.Player.H1 + 2048, Height: 128}}
	s := New()
	for i := range s.Candidates {
		s.Candidates[i].Distance = 0xffff
	}
	s.Candidates[0] = Candidate{10, 128, 0}
	s.Contacts[10] = Contact{Flags: 0xfa, Kind: 3}
	target := new([5]int8)
	in.SetControlTarget = func(a int, v int8) error { target[a] = v; return nil }
	in.ModernTail = true
	return s, in, target
}
func TestFollowingMastHandoffUsesExistingDrawsNoBodyRay(t *testing.T) {
	s, in, target := tailFixture(t)
	in.Assets = nil // A following-player handoff never reaches the own-body query.
	s.TailDraws = [5]uint16{0, 0, 0, 0, 0}
	if e := s.advanceActorTail(in); e != nil {
		t.Fatal(e)
	}
	for a := 1; a < 5; a++ {
		if target[a] != 11 {
			t.Fatalf("actor%d target%d", a, target[a])
		}
	}
	if s.Contacts[10].Flags != 0xfa {
		t.Fatal("mast observer token31 was replaced")
	}
	if s.Cursor != 0 {
		t.Fatal("nonplayer selector changed AB5")
	}
}
func TestFollowingSelectorBoundariesAndOrdinaryObserver(t *testing.T) {
	for _, tc := range []struct {
		distance, angle uint16
		state           byte
		want            int8
	}{{4095, 8191, 2, 11}, {4096, 0, 2, 0}, {4095, 8192, 2, 0}, {4095, 0, 6, 0}} {
		s, in, target := tailFixture(t)
		s.Candidates[0].Distance, s.Candidates[0].Angle = tc.distance, tc.angle
		s.Contacts[10].Flags = tc.state
		if e := s.advanceActorTail(in); e != nil {
			t.Fatal(e)
		}
		if target[1] != tc.want {
			t.Fatalf("%+v target%d", tc, target[1])
		}
		if tc.want != 0 && s.Contacts[10].Flags != 4<<3|2 {
			t.Fatalf("ordinary observer%02x", s.Contacts[10].Flags)
		}
	}
}
func TestNegativeCountdownInactiveAndInvalidHeldAssignment(t *testing.T) {
	s, in, target := tailFixture(t)
	in.Entities[1].Control[3] = 0xfe
	in.Entities[2].Control[1] = 0
	in.Entities[3].Control[3] = 12
	in.Entities[11] = Entity{Present: true, Row: 11, Participation: 2}
	if e := s.advanceActorTail(in); e != nil {
		t.Fatal(e)
	}
	if target[1] != -1 || target[2] != 0 || target[3] != 0 || target[4] != 11 {
		t.Fatal(*target)
	}
}
func TestIndependentBodyQueryPublishesSix(t *testing.T) {
	s, in, target := tailFixture(t)
	// Only actor1 visits. Known state2 candidates are skipped by this path.
	in.Entities[2].Object[10] = 0
	in.Entities[1].Control[1] = 2
	s.Contacts[10].Flags = 1
	if e := s.advanceActorTail(in); e != nil {
		t.Fatal(e)
	}
	if target[1] != 11 || s.Contacts[10].Flags != 14 {
		t.Fatalf("target%v contact%+v", *target, s.Contacts[10])
	}
}
func TestRetainedTargetParticipationDiffersFromAcquisition(t *testing.T) {
	for _, modern := range []bool{false, true} {
		in, _ := contactFixture(t, 0)
		in.ModernTail = modern
		in.Entities[0].Control[3] = 0xff
		in.Entities[1].Participation = 2
		s := New()
		s.Target = 2
		s.Contacts[1] = Contact{Flags: 2, Kind: 0x17}
		if e := s.Advance(in); e != nil {
			t.Fatal(e)
		}
		if modern && s.Target != 2 {
			t.Fatal("dying retained target reused acquisitionP1 gate")
		}
		if !modern && s.Target != 0 {
			t.Fatal("historical P1 interpretation changed")
		}
	}
}
func TestTailStillConsumesExactlyFiveSharedDraws(t *testing.T) {
	s, in, target := tailFixture(t)
	// AB5 follows the player's initial candidate0; requester search excludes
	// that current player target. Candidate1 is a separate eligible target.
	in.Entities[11] = in.Entities[10]
	in.Entities[11].Row = 11
	in.Entities[11].Pose.H1 += 256
	n := 0
	in.Random = func() uint16 { n++; return 0 }
	if e := s.Advance(in); e != nil {
		t.Fatal(e)
	}
	if n != 5 {
		t.Fatalf("tail drew%d shared words", n)
	}
	if target[1] != 12 {
		t.Fatal("real Advance did not publish actual assignment")
	}
}
func TestUnknownAssignedTargetRemainsGuarded(t *testing.T) {
	for _, target := range []byte{12, 81} {
		s, in, received := tailFixture(t)
		in.Entities[1].Control[3] = target
		received[1] = int8(target)
		if e := s.advanceActorTail(in); e == nil {
			t.Fatal("unproven target alias was silently cleared", target)
		}
		if received[1] != int8(target) {
			t.Fatal("unsupported source target was repaired", target, received[1])
		}
	}
}
