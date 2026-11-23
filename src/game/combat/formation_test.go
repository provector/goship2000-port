package combat

import (
	"encoding/binary"
	"gunship2000-assets/game/flight"
	gamestate "gunship2000-assets/game/state"
	"gunship2000-assets/game/world"
	"strings"
	"testing"
)

func formationFixture(t *testing.T) (*State, Input, *flight.Systems) {
	s, in, sys := followerFixture(t)
	s.SetFormationEnabled(true)
	m := s.init
	m.Offsets = map[int][][2]int16{0: {{400, -200}, {-400, -200}, {0, -400}, {0, -600}}}
	for i := 1; i < 5; i++ {
		e := &m.Entities[i]
		e.Kind = 0x21
		e.DynamicRaw[2] = byte(i)
		e.ControlRaw[5] = 0
		e.Pose = m.Entities[0].Pose
	}
	in.LowLimit = 128
	return s, in, sys
}
func TestFormationSourceArithmetic(t *testing.T) {
	for _, tc := range []struct {
		current, desired uint32
		draw             uint16
		delta            int16
		next             uint32
	}{
		{1000, 1000, 0, -2, 998}, {1000, 1000, 8, 0, 1000}, {1000, 1000, 15, 1, 1001}, {0, 131072, 8, -32768, 4294934528}, {0xffffffff, 0, 8, 0, 0xffffffff}, {1000, 900, 0, -27, 973},
	} {
		d := formationDelta(tc.current, tc.desired, tc.draw)
		if d != tc.delta || tc.current+uint32(int32(d)) != tc.next {
			t.Fatal(tc, d)
		}
	}
	for _, tc := range []struct{ own, leader, query, low, want int16 }{{399, 400, 401, 128, 463}, {399, 400, 400, 128, 399}, {1, 1, 1, 128, 0}, {-1, 0, 0, 128, -1}} {
		if got := formationHeight(tc.own, tc.leader, tc.query, tc.low); got != tc.want {
			t.Fatal(tc, got)
		}
	}
}
func TestFormationFirstVisitAndHistoricalInterpretation(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		s, in, sys := formationFixture(t)
		s.SetFormationEnabled(enabled)
		m := s.init
		for i := 2; i < 5; i++ {
			m.Entities[i].Kind = 0x20
		}
		m.Entities[0].Pose.Angles = [3]uint16{8, 0, 1}
		m.Entities[1].Pose.Angles = [3]uint16{uint16(65527), 0, uint16(65535)}
		in.PlayerIndicatedSpeed = 90
		calls := 0
		in.Random = func() uint16 {
			calls++
			if calls == 1 {
				return 8
			}
			return 15
		}
		if _, err := s.AdvanceFollowers(in, sys); err != nil {
			t.Fatal(err)
		}
		e := m.Entities[1]
		if !enabled {
			if calls != 0 || e.Pose.H0 != 0x40000 || e.Pose.H1 != 0x40000 {
				t.Fatal("historical stationary interpretation changed")
			}
			continue
		}
		if calls != 2 || e.Pose.H0 != 0x40000+100 || e.Pose.H1 != 0x40000-49 || e.RawMotion != 90 || int16(e.Pose.Angles[0]) != -7 || int16(e.Pose.Angles[2]) != -1 || e.Pose.Height != 400 || sys.UnsupportedFollowerEmptyBurn != 0 {
			t.Fatalf("source first visit %+v calls%d guard%x", e, calls, sys.UnsupportedFollowerEmptyBurn)
		}
	}
}
func TestFormationAscendingDependentLeaderAndTwoDraws(t *testing.T) {
	s, in, sys := formationFixture(t)
	m := s.init
	m.Entities[4].ControlRaw[5] = 3
	m.Entities[4].DynamicRaw[2] = 0x11
	m.Offsets[1] = [][2]int16{{0, -400}}
	draws := []uint16{8, 8, 8, 8, 8, 8, 8, 8}
	n := 0
	in.Random = func() uint16 { v := draws[n]; n++; return v }
	if _, err := s.AdvanceFollowers(in, sys); err != nil {
		t.Fatal(err)
	}
	if n != 8 || m.Entities[3].Pose.H1 != 0x40000-100 || m.Entities[4].Pose.H1 != 0x40000+75 || sys.UnsupportedFollowerEmptyBurn != 0 {
		t.Fatal(n, m.Entities[3].Pose, m.Entities[4].Pose, sys.UnsupportedFollowerEmptyBurn)
	}
}
func TestFormationP0AndEarlyExits(t *testing.T) {
	s, in, sys := formationFixture(t)
	m := s.init
	for i := 2; i < 5; i++ {
		m.Entities[i].Kind = 0x20
	}
	e := &m.Entities[1]
	e.Participation = 0
	n := 0
	in.Random = func() uint16 { n++; return 8 }
	s.AdvanceFollowers(in, sys)
	if n != 0 || e.Participation != 1 || e.Pose.H0 != 0x40000+400 || e.Pose.H1 != 0x40000-200 {
		t.Fatal("P0 initialized and redispatched", e, n)
	}
	m.Switches = 0x800
	m.Entities[0].Kind = 0x41
	m.Entities[0].ControlRaw[1] = 0
	e.DynamicRaw[2] = 0 // Early exit does not read offsets.
	s.AdvanceFollowers(in, sys)
	if e.Kind != 0x41 || e.ControlRaw[1] != 0 || n != 0 {
		t.Fatal("collected leader exit", e, n)
	}
	s, in, sys = formationFixture(t)
	for i := 2; i < 5; i++ {
		s.init.Entities[i].Kind = 0x20
	}
	s.init.Entities[0].ControlRaw[1] = 0
	s.init.Entities[1].DynamicRaw[2] = 0
	n = 0
	in.Random = func() uint16 { n++; return 0 }
	s.AdvanceFollowers(in, sys)
	e = &s.init.Entities[1]
	if e.Kind != 0x25 || e.Participation != 0 || e.ControlRaw[1] != 5 || binary.LittleEndian.Uint16(e.ControlRaw[6:]) != 1 || binary.LittleEndian.Uint16(e.ControlRaw[8:]) != 3 || n != 0 || sys.UnsupportedFollowerEmptyBurn != 2 {
		t.Fatal("defined producer/unclosed kind25 guard", e, n, sys.UnsupportedFollowerEmptyBurn)
	}
}
func TestFormationUnknownCornerOperandPartitions(t *testing.T) {
	for _, height := range []int32{0, 20} {
		s, in, sys := formationFixture(t)
		m := s.init
		for i := 2; i < 5; i++ {
			m.Entities[i].Kind = 0x20
		}
		g := s.assets.Geometries["flat"]
		threshold := g.Heights[0]
		threshold.Threshold = 100
		threshold.Plane = world.Plane{0, 1, 0, -height}
		g.Heights = append([]world.HeightRegion{threshold}, g.Heights...)
		n := 0
		in.Random = func() uint16 { n++; return 8 }
		s.AdvanceFollowers(in, sys)
		if height == 0 {
			if sys.UnsupportedFollowerEmptyBurn != 0 || n != 2 {
				t.Fatal("invariant partition rejected", n, m.FollowerLimits)
			}
		} else if sys.UnsupportedFollowerEmptyBurn != 2 || n != 0 || !strings.Contains(m.FollowerLimits[1], "BP-30") {
			t.Fatal("unknown operand guessed", n, m.FollowerLimits)
		}
	}
}

func TestFormationCornerPartitionsCoverSignedOperandDomain(t *testing.T) {
	// Enumerate every possible uninitialized signed height independently of the
	// admission's threshold selection, including source first-match precedence.
	for _, variant := range []string{"same", "different", "shadowed"} {
		s, _, _ := formationFixture(t)
		g := s.assets.Geometries["flat"]
		plain := g.Heights[0]
		low, high := plain, plain
		low.Threshold, high.Threshold = -100, 100
		if variant != "same" {
			low.Plane, high.Plane = world.Plane{0, 1, 0, -10}, world.Plane{0, 1, 0, -20}
		}
		g.Heights = []world.HeightRegion{high, low, plain}
		if variant == "shadowed" {
			g.Heights = []world.HeightRegion{plain, high, low}
		}
		p := s.init.Entities[1].Pose
		q := deathQuery(p)
		objects := s.deathQueryObjects()
		constant, initial := true, int16(0)
		for h := -32768; h <= 32767; h++ {
			q.Height = int16(h)
			v, err := s.assets.ContactHeight(s.init.World, q, objects)
			if err != nil {
				t.Fatal(err)
			}
			if h == -32768 {
				initial = v
			} else if v != initial {
				constant = false
			}
		}
		got, err := s.formationCornerHeight(p)
		if (err == nil) != constant || constant && got != initial {
			t.Fatal(variant, constant, initial, got, err)
		}
	}
}

func TestFormationFuelNextOldEmptyAndSuccessors(t *testing.T) {
	for _, skill := range []byte{0, 127} {
		s, in, sys := formationFixture(t)
		m := s.init
		for i := 2; i < 5; i++ {
			m.Entities[i].Kind = 0x20
			m.Entities[i].ControlRaw[1] = 0
		}
		*m.Stores.Ships[1].Proficiency = skill
		sys.Fuel[1] = 1
		sys.Countdown = 0
		n := 0
		in.Random = func() uint16 { n++; return 127 }
		sys.AdvanceWithFollowers(true, 16, s.FollowerFuelContext(in.Random))
		if sys.Fuel[1] != 0 || m.Stores.Damage[1][9] != 0 || n != 0 {
			t.Fatal("old1 incorrectly exhausted")
		}
		s.AdvanceFollowers(in, sys)
		if n != 2 || m.Entities[1].Kind != 0x21 || m.FollowerFailureKnown[1] {
			t.Fatal("positive branch successor")
		}
		sys.Countdown = 0
		sys.AdvanceWithFollowers(true, 16, s.FollowerFuelContext(in.Random))
		if n != 3 || m.Stores.Damage[1][9] == 0 || !m.FollowerFailureKnown[1] || sys.UnsupportedFollowerEmptyBurn != 0 {
			t.Fatal("oldempty producer", n, m.Stores.Damage[1], sys.UnsupportedFollowerEmptyBurn)
		}
		s.AdvanceFollowers(in, sys)
		if n != 5 {
			t.Fatal("geometry draws before disabled callback", n)
		}
		e := &m.Entities[1]
		if skill == 127 {
			if e.Kind != 0x21 || e.Participation != 2 || e.RawMotion != 4 {
				t.Fatal("6091 fallentry", e)
			}
		} else if e.Kind != 0x26 || e.Participation != 1 || e.ControlRaw[1] != 6 {
			t.Fatal("6092 disableentry", e)
		}
		for k := 0; k < 200 && e.Kind != 0x1c && m.Entities[5].Participation != 1; k++ {
			s.AdvanceFollowers(in, sys)
		}
		if skill == 127 {
			if e.Kind != 0x1c || e.ControlRaw[1] != 0 || s.Results[3].Kind != 3 {
				t.Fatal("real fall/final loss", e, s.Results)
			}
		} else {
			if m.Entities[5].Participation != 1 || e.Kind != 0x20 || e.ControlRaw[1] != 0 {
				t.Fatal("descent/reserve chain", e, m.Entities[5])
			}
			// A supplied low/slow approach exercises the real pickup consumer;
			// it is a boundary fixture, not a claim of continuous flight.
			m.Entities[0].Pose = m.Entities[5].Pose
			m.Entities[0].Pose.Height = 0
			cargo := m.Stores.Ships[0].Flags
			s.AdvanceObjectives(in)
			if m.Entities[5].Kind != 0x41 || m.Entities[5].DynamicRaw[3] != 0 || cargo != m.Stores.Ships[0].Flags || s.Results[3].Kind != 6 {
				t.Fatal("actual source reserve pickup/carrier/cargo facts", m.Entities[5], s.Results)
			}
		}
		if sys.UnsupportedFollowerEmptyBurn != 0 {
			t.Fatal("admitted chain retained exclusion", m.FollowerLimits)
		}
	}
}

func TestFormationFuelFARPServiceAndCapacityBoundary(t *testing.T) {
	for _, tc := range []struct{ quantity, losses, fuel int16 }{{1, 0, 100}, {1, 1, 0}, {0, 0, 0}} {
		s, in, sys := formationFixture(t)
		m := s.init
		for i := 2; i < 5; i++ {
			m.Entities[i].Kind = 0x20
			m.Entities[i].ControlRaw[1] = 0
		}
		e := &m.Entities[1]
		m.Entities[0].Pose.Height, e.Pose.Height = 0, 2
		m.Offsets[0][0] = [2]int16{}
		m.Sortie = &gamestate.SortieState{FARPLosses: tc.losses}
		farp := &m.Targets.Records[1]
		binary.LittleEndian.PutUint16(farp[32:], uint16(tc.quantity))
		binary.LittleEndian.PutUint16(farp[50:], uint16(tc.quantity))
		binary.LittleEndian.PutUint32(farp[42:], e.Pose.H0)
		binary.LittleEndian.PutUint32(farp[46:], e.Pose.H1)
		sys.Fuel[1], sys.Countdown = 0, 0
		in.Random = func() uint16 { return 8 }
		sys.AdvanceWithFollowers(true, 16, s.FollowerFuelContext(in.Random))
		s.AdvanceFollowers(in, sys)
		s.AdvanceFollowers(in, sys)
		if sys.Fuel[1] != tc.fuel || sys.UnsupportedFollowerEmptyBurn != 0 {
			t.Fatal("actual FARP service/capacity equality", tc, sys.Fuel, m.FollowerLimits)
		}
	}
}

func TestFormationDisabledLeaderRegisterQuirk(t *testing.T) {
	s, in, sys := formationFixture(t)
	m := s.init
	for i := 3; i < 5; i++ {
		m.Entities[i].Kind = 0x20
		m.Entities[i].ControlRaw[1] = 0
	}
	m.Entities[2].ControlRaw[5] = 1
	sys.Fuel[1], sys.Countdown = 0, 0
	in.Random = func() uint16 { return 127 }
	sys.AdvanceWithFollowers(true, 16, s.FollowerFuelContext(in.Random))
	s.AdvanceFollowers(in, sys)
	if m.Entities[1].Kind != 0x20 || m.Entities[1].Participation != 1 || m.Entities[2].ControlRaw[1] != 2 || m.Entities[2].Kind != 0x21 || m.Entities[2].Pose.Angles[0] != 0 || m.Entities[2].Pose.Angles[2] != 0 || sys.UnsupportedFollowerEmptyBurn != 0 {
		t.Fatal("source AX leader overwrite or child visit", m.Entities[1], m.Entities[2], m.FollowerLimits)
	}
}

func TestFormationCornerStrictBoundaryAndEarlyQueryExit(t *testing.T) {
	for _, first := range []int32{416, 417} {
		s, in, sys := formationFixture(t)
		m := s.init
		for i := 2; i < 5; i++ {
			m.Entities[i].Kind = 0x20
		}
		m.Offsets[0][0] = [2]int16{0, 0}
		box := func(x, z int16, height int32) world.HeightRegion {
			return world.HeightRegion{Points: [][2]int16{{x - 16, z - 16}, {x + 16, z - 16}, {x + 16, z + 16}, {x - 16, z + 16}, {x - 16, z - 16}}, Plane: world.Plane{0, 1, 0, -height}}
		}
		g := s.assets.Geometries["flat"]
		bad := box(3584, -3584, 0)
		bad.Plane[1] = 0
		g.Heights = []world.HeightRegion{box(-3584, -3584, first), box(-3584, 3584, 417), bad}
		in.Random = func() uint16 { return 8 }
		s.AdvanceFollowers(in, sys)
		if sys.UnsupportedFollowerEmptyBurn != 0 || m.Entities[1].Pose.H0 != 0x40000 || m.Entities[1].Pose.H1 != 0x40000-32 {
			t.Fatal("strict416 equality/417 fallback or laterquery executed", first, m.Entities[1].Pose, m.FollowerLimits)
		}
	}
}
func TestFormationSourceYawEqualityAndP2DoesNotReadOffsets(t *testing.T) {
	for _, off := range []int16{64, 68} {
		s, in, sys := formationFixture(t)
		m := s.init
		for i := 2; i < 5; i++ {
			m.Entities[i].Kind = 0x20
		}
		m.Offsets[0][0] = [2]int16{off, 0}
		m.Entities[0].Pose.Angles[1] = 0
		m.Entities[1].Pose.Angles[1] = 0
		in.Random = func() uint16 { return 8 }
		s.AdvanceFollowers(in, sys)
		if off == 64 && m.Entities[1].Pose.Angles[1] != 0 {
			t.Fatal("equality16 must smooth")
		}
		if off == 68 && m.Entities[1].Pose.Angles[1] != 0xc000 {
			t.Fatalf("delta17 atan got%04x", m.Entities[1].Pose.Angles[1])
		}
	}
	s, in, sys := formationFixture(t)
	for i := 2; i < 5; i++ {
		s.init.Entities[i].Kind = 0x20
	}
	e := &s.init.Entities[1]
	e.Participation = 2
	e.ControlRaw[5] = 0xff
	e.DynamicRaw[2] = 0
	s.init.Offsets = nil
	calls := 0
	in.Random = func() uint16 { calls++; return 0 }
	s.AdvanceFollowers(in, sys)
	if calls != 0 || sys.UnsupportedFollowerEmptyBurn != 0 || e.Pose.Angles[0] != 0xfe00 {
		t.Fatal("P2 incorrectly consumed formation owners/draws", calls, e, s.init.FollowerLimits)
	}
}
func TestFormationServiceAfterExhaustionAndUnclosedGuards(t *testing.T) {
	s, in, sys := formationFixture(t)
	m := s.init
	for i := 2; i < 5; i++ {
		m.Entities[i].Kind = 0x20
		m.Entities[i].ControlRaw[1] = 0
	}
	e := &m.Entities[1]
	m.Entities[0].Pose.Height = 0
	e.Pose.Height = 2
	m.Offsets[0][0] = [2]int16{}
	binary.LittleEndian.PutUint32(m.Targets.Records[0][42:], e.Pose.H0)
	binary.LittleEndian.PutUint32(m.Targets.Records[0][46:], e.Pose.H1)
	sys.Fuel[1] = 0
	sys.Countdown = 0
	in.Random = func() uint16 { return 127 }
	sys.AdvanceWithFollowers(true, 16, s.FollowerFuelContext(in.Random))
	s.AdvanceFollowers(in, sys)
	s.AdvanceFollowers(in, sys)
	if sys.Fuel[1] != 100 || m.Stores.Damage[1][9] != 0 || m.Entities[5].Participation != 0 || sys.UnsupportedFollowerEmptyBurn != 0 {
		t.Fatal("source actor service/refill or unwanted reserve", sys.Fuel, m.Stores.Damage[1], m.FollowerLimits)
	}
	s.AdvanceFollowers(in, sys)
	if e.Kind != 0x20 || e.Participation != 1 || e.Pose.Height != 2 {
		t.Fatal("next visit P0 service successor", e)
	}
	for _, badLeader := range []bool{false, true} {
		s, in, sys = formationFixture(t)
		for i := 2; i < 5; i++ {
			s.init.Entities[i].Kind = 0x20
		}
		calls := 0
		in.Random = func() uint16 { calls++; return 0 }
		if badLeader {
			s.init.Entities[1].ControlRaw[5] = 0xff
		} else {
			s.init.Switches = 0x1000
			for i := range s.assets.Worlds["flat"].Grid {
				s.assets.Worlds["flat"].Grid[i] = 62
			}
		}
		s.AdvanceFollowers(in, sys)
		if sys.UnsupportedFollowerEmptyBurn != 2 || calls != 0 {
			t.Fatal("unknown source owner admitted", badLeader, calls, s.init.FollowerLimits)
		}
	}
}
