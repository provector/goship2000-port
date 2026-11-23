package combat

import (
	"encoding/binary"
	gamestate "gunship2000-assets/game/state"
	"testing"
)

func artilleryFixture(t *testing.T) (*State, Input) {
	s, in := fixture(t, 1)
	s.assets.Models = []string{"flat"}
	for len(s.init.Entities) < 80 {
		row := len(s.init.Entities)
		s.init.Entities = append(s.init.Entities, gamestate.InitializedEntity{RuntimeRow: row, SourceRow: row, Kind: 0xff, FlightSlot: -1})
	}
	s.init.Stores.Definitions[0].RawWSHDID = 25
	s.init.Support = &gamestate.SupportRuntime{Configuration: gamestate.SupportConfiguration{Profile: gamestate.SupportInitializationProfile, Mode: 1, PrefixRaw: [2]byte{25, 0}, Rows: []int{77, 78, 79}}, Availability: 0x31, Target: gamestate.InitializedPose{H0: 0x40000, H1: 0x48000}}
	for row := 77; row < 80; row++ {
		e := &s.init.Entities[row]
		e.Kind = 0x19
		e.Participation = 1
		e.Pose = gamestate.InitializedPose{H0: 0x40000, H1: 0x40000}
		e.RawMotion = 30
	}
	in.E0 = true
	return s, in
}
func TestArtillerySharedParityCounterAndExactTwoDraws(t *testing.T) {
	s, in := artilleryFixture(t)
	s.init.Support.Request = 18
	draws := 0
	in.Random = func() uint16 { draws++; return uint16(draws) }
	//77 cannot consume even18;78 consumes to17 (no shell);79 consumes to16.
	for row := 77; row < 80; row++ {
		if err := s.UpdateArtillery(row, in); err != nil {
			t.Fatal(err)
		}
	}
	if s.init.Support.Request != 16 || draws != 0 {
		t.Fatal(s.init.Support.Request, draws)
	}
	//78 next consumes16->15, source low7<16 now emits exactly one impact.
	if err := s.UpdateArtillery(78, in); err != nil {
		t.Fatal(err)
	}
	slot := int((byte(s.NextSlot) - (15 & 7) + 8) & 15)
	p := s.Slots[slot]
	if s.init.Support.Request != 15 || draws != 2 || p.Lifetime != 32 || p.Flags != 1 || p.Parameter != 0 || p.Packed != 8 || p.Shooter != 78 || p.Target != 0 || p.Pose.Angles[0] != 0xc000 {
		t.Fatal("source impact payload", p, s.init.Support, draws)
	}
	if p.Pose.H0 != 0x40001 || p.Pose.H1 != 0x47ffe+64 || p.Pose.Height != 1936 {
		t.Fatal("ordered signed jitter", p.Pose)
	}
}
func TestArtilleryBoundariesBusyAndDeath(t *testing.T) {
	for _, before := range []uint16{1, 2, 16, 17, 97, 98, 112, 113, 255} {
		s, in := artilleryFixture(t)
		row := 78
		if before&1 != 0 {
			row = 77
		}
		s.init.Support.Request = before
		slot := int((byte(s.NextSlot) - byte((before-1)&7) + 8) & 15)
		if err := s.UpdateArtillery(row, in); err != nil {
			t.Fatal(err)
		}
		low := byte(before-1) & 127
		want := low < 16 || low > 96 && low < 112
		if (s.Slots[slot].Lifetime != 0) != want {
			t.Fatal("strict low7 range", before, s.Slots[slot])
		}
		if low > 96 && low < 112 && s.Slots[slot].Packed != 0 {
			t.Fatal("muzzle final packed writer")
		}
	}
	s, in := artilleryFixture(t)
	s.init.Support.Request = 16
	in.E0 = false
	s.UpdateArtillery(78, in)
	if s.init.Support.Request != 16 {
		t.Fatal("E0 gate")
	}
	in.E0 = true
	s.Slots[1].Lifetime = 3
	s.NextSlot = 0 // (0-7+8)&15=1 after16->15
	draws := 0
	in.Random = func() uint16 { draws++; return 1 }
	s.UpdateArtillery(78, in)
	if draws != 0 || s.init.Support.Request != 15 {
		t.Fatal("busy slot draw/counter", draws)
	}
	e := &s.init.Entities[78]
	e.Participation = 2
	e.RawMotion = 4
	s.UpdateArtillery(78, in)
	if e.Participation != 3 || e.RawMotion != 4 {
		t.Fatal("death2->3 samevisit")
	}
	for i := 0; i < 4; i++ {
		if err := s.UpdateArtillery(78, in); err != nil {
			t.Fatal(err)
		}
	}
	if e.Kind != 0x1c || e.Participation != 0 || binary.LittleEndian.Uint16(e.DynamicRaw[6:]) != 4 {
		t.Fatal("death successor", e)
	}
}
