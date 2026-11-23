package combat

import (
	"gunship2000-assets/game/flight"
	"reflect"
	"testing"
)

func incomingFixture(t *testing.T, flags uint16) (*State, Input) {
	s, in := fixture(t, 0x1e20)
	s.init.Entities[0].Participation = 0
	in.Incoming = &IncomingContext{Defenses: &flight.Defenses{}, ApplyPlayerHit: func(int) (uint16, error) { return 0, nil }}
	in.SpeechEnabled = true
	return s, in
}
func incomingAtTarget(s *State, flags uint16, threshold byte) Projectile {
	p := Projectile{Flags: flags, Lifetime: 10, Shooter: 1, Target: 0, Speed: 64, Packed: threshold, Pose: s.init.Entities[0].Pose}
	p.Pose.H1 -= 64
	return p
}
func TestIncomingPlayerPrecedenceAndInlineSlotDrawOrder(t *testing.T) {
	s, in := incomingFixture(t, 3)
	seq := []uint16{1, 0x100, 0, 0x200, 2, 0x300}
	at := 0
	var order []string
	in.Random = func() uint16 { r := seq[at]; at++; order = append(order, "rng"); return r }
	var hitSlots []int
	in.Incoming.ApplyPlayerHit = func(slot int) (uint16, error) {
		hitSlots = append(hitSlots, slot)
		order = append(order, "damage")
		in.Random()
		return uint16(len(hitSlots) + 1), nil
	}
	for slot, flags := range map[int]uint16{20: 0x43, 21: 3} {
		if err := s.SupplyIncoming(slot, incomingAtTarget(s, flags, 0), true); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.Step(in); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(hitSlots, []int{20, 20, 21}) || at != 6 || !reflect.DeepEqual(order, []string{"rng", "damage", "rng", "rng", "damage", "rng", "rng", "damage", "rng"}) {
		t.Fatal(hitSlots, at, order)
	}
	if s.Results[2].Target != 0x32 || s.Results[3].Target != 4 || s.Slots[20].Packed != 2 || s.Slots[21].Packed != 2 {
		t.Fatal(s.Results, s.Slots[20], s.Slots[21])
	}
	if s.init.Entities[0].Kind != 0xff || len(events(s, "shake")) != 2 {
		t.Fatal("kindFF must take player before stationary bit10", s.init.Entities[0])
	}
}
func TestIncomingStrictContactAcceptanceAndFinalOldLife(t *testing.T) {
	for _, tc := range []struct {
		name         string
		dx           uint32
		dh           uint16
		threshold, r byte
		hit          bool
	}{
		{"inside", 48, 63, 0, 1, true}, {"metric-equality", 64, 0, 0, 1, false}, {"height-equality", 0, 64, 0, 1, false},
		{"threshold-equality", 0, 0, 5, 5, false}, {"threshold255", 0, 0, 255, 255, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, in := incomingFixture(t, 3)
			p := incomingAtTarget(s, 3, tc.threshold)
			p.Lifetime = 1
			p.Pose.H0 += tc.dx
			p.Pose.Height += tc.dh
			hits, draws := 0, 0
			in.Incoming.ApplyPlayerHit = func(int) (uint16, error) { hits++; return 0, nil }
			in.Random = func() uint16 { draws++; return uint16(tc.r) }
			if err := s.SupplyIncoming(20, p, false); err != nil {
				t.Fatal(err)
			}
			if err := s.Step(in); err != nil {
				t.Fatal(err)
			}
			if (hits == 1) != tc.hit {
				t.Fatal(hits, draws, s.Slots[20])
			}
			if s.Slots[20].Lifetime != 0 {
				t.Fatal("old1 still moves then expires")
			}
			if tc.dx == 64 || tc.dh == 64 {
				if draws != 0 {
					t.Fatal("geometry miss consumed acceptance RNG")
				}
			} else if draws != 1 {
				t.Fatal(draws)
			}
		})
	}
}
func TestIncomingJammerDecoyWarningAndGuidanceBoundaries(t *testing.T) {
	for _, tc := range []struct {
		name       string
		flags, yaw uint16
		radar      uint16
		ir         uint16
		flare      int16
		gain       int16
		wantFlags  uint16
		wantGain   int16
		wantDraw   int
		warn       bool
	}{
		{"radar", 0x43, 0, 2, 0, 0, 64, 0x23, 0, 1, false},
		{"ir", 0x83, 0, 0, 1, 0, 64, 0x23, 0, 0, false},
		{"bypass-decoy", 0x8083, 0, 0, 1, 5, 64, 0x8083, 64, 0, true},
		{"bearing0800", 0x43, 0x800, 0, 0, 0, 64, 3, 64, 0, false},
		{"bearing4000", 0x43, 0x4000, 0, 0, 0, 64, 3, 64, 0, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, in := incomingFixture(t, 3)
			p := incomingAtTarget(s, tc.flags, 0)
			p.Pose.H1 -= 4096
			p.Pose.Angles[1] = tc.yaw
			p.Parameter = tc.gain
			// Movement precedes bearing. Cardinal yaw4000 moves H0 by-1;
			// independently compensate that integer step to keep exact postmove yaw0.
			p.Speed = 1
			p.Pose.H0 = s.init.Entities[0].Pose.H0
			p.Pose.H1 = s.init.Entities[0].Pose.H1 - 4096
			if tc.yaw == 0x4000 {
				p.Pose.H0++
			}
			in.Incoming.Defenses = &flight.Defenses{RadarSignature: tc.radar, IRActive: tc.ir, FlareTimer: tc.flare, Decoy: flight.Pose{Height: 1024}}
			draws := 0
			in.Random = func() uint16 { draws++; return 4 }
			if err := s.SupplyIncoming(20, p, true); err != nil {
				t.Fatal(err)
			}
			if err := s.Step(in); err != nil {
				t.Fatal(err)
			}
			got := s.Slots[20]
			if got.Flags != tc.wantFlags || got.Parameter != tc.wantGain || draws != tc.wantDraw {
				t.Fatal(got, draws)
			}
			if tc.yaw == 0x4000 && got.Lifetime != 9 {
				t.Fatal("exact4000 must not take greater-than kill", got)
			}
			if tc.warn && s.incomingWarning[20] {
				t.Fatal("warning not consumed")
			}
		})
	}
}
func TestIncomingWarningConsumedWhileSpeechOffAndOccupiedSlots(t *testing.T) {
	s, in := incomingFixture(t, 3)
	p := incomingAtTarget(s, 0x83, 0)
	p.Pose.H1 -= 4000
	p.Parameter = 64
	p.Speed = 1
	if err := s.SupplyIncoming(19, p, true); err == nil {
		t.Fatal("follower partition admitted")
	}
	if err := s.SupplyIncoming(20, p, true); err != nil {
		t.Fatal(err)
	}
	if err := s.SupplyIncoming(20, p, true); err == nil {
		t.Fatal("occupied overwrite")
	}
	in.SpeechEnabled = false
	if err := s.Step(in); err != nil {
		t.Fatal(err)
	}
	if s.incomingWarning[20] {
		t.Fatal("DE80 must gate voice after latch consumption")
	}
	in.SpeechEnabled = true
	if err := s.Step(in); err != nil {
		t.Fatal(err)
	}
	if len(events(s, "sound")) != 0 {
		t.Fatal("speech replayed old warning")
	}
}

func TestIncomingLaterSlotReadsLiveSpeechAndInactiveShooterScratch(t *testing.T) {
	s, in := incomingFixture(t, 3)
	voice := true
	in.Incoming.SpeechEnabled = func() bool { return voice }
	in.Incoming.ApplyPlayerHit = func(int) (uint16, error) { voice = false; return 0, nil }
	in.Random = func() uint16 { return 1 }
	for _, slot := range []int{20, 21} {
		if err := s.SupplyIncoming(slot, incomingAtTarget(s, 3, 0), false); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.Step(in); err != nil {
		t.Fatal(err)
	}
	if len(events(s, "sound")) != 1 {
		t.Fatal("later slot reused stale DE80", events(s, "sound"))
	}
	s, in = incomingFixture(t, 3)
	p := incomingAtTarget(s, 0x43, 0)
	p.Pose.H1 -= 4096
	p.Speed = 1
	p.Parameter = 64
	s.init.Entities[1].Participation = 0
	if err := s.SupplyIncoming(20, p, false); err != nil {
		t.Fatal(err)
	}
	if err := s.Step(in); err == nil {
		t.Fatal("first-use unknown shared35E0 was fabricated")
	}
	s, in = incomingFixture(t, 3)
	in.Incoming.Defenses.ChaffTimer = 4
	in.Incoming.Defenses.Decoy.Height = 1024
	if err := s.SupplyIncoming(20, p, false); err != nil {
		t.Fatal(err)
	}
	if err := s.Step(in); err != nil {
		t.Fatal(err)
	}
	if !s.incomingAimKnown || s.incomingAimPose.Height != 1024 {
		t.Fatal("source decoy copy did not establish shared scratch")
	}
	s.init.Entities[1].Participation = 0
	if err := s.Step(in); err != nil {
		t.Fatal(err)
	}
	if s.Slots[20].Flags != 0x23 || s.Slots[20].Parameter != 0 || s.incomingAimPose.Height != 1024 {
		t.Fatal("inactive shooter must retain last actual35E0", s.Slots[20], s.incomingAimPose)
	}
}
