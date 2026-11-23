package combat

import (
	"encoding/binary"
	"encoding/json"
	"gunship2000-assets/game/world"
	"strings"
	"testing"
)

func logisticsFollowerFixture(t *testing.T) (*State, Input) {
	s, in, _ := formationFixture(t)
	s.SetLogisticsEnabled(true)
	for actor := 2; actor < 5; actor++ {
		s.init.Entities[actor].Kind = 0x20
		s.init.Entities[actor].ControlRaw[1] = 0
	}
	return s, in
}

func TestCornerUsesOnlyInvariantSourceDecision(t *testing.T) {
	s, _ := logisticsFollowerFixture(t)
	g := s.assets.Geometries["flat"]
	plain := g.Heights[0]
	low, high := plain, plain
	low.Threshold = 100
	low.Plane = world.Plane{0, 1, 0, -990}
	high.Plane = world.Plane{0, 1, 0, -1300}
	g.Heights = []world.HeightRegion{low, high}
	p := s.init.Entities[1].Pose
	for _, tc := range []struct {
		limit          int16
		known, blocked bool
	}{{989, true, true}, {990, false, false}, {1299, false, false}, {1300, true, false}, {1936, true, false}} {
		got, err := s.formationCornerBlocked(p, tc.limit)
		if (err == nil) != tc.known || err == nil && got != tc.blocked {
			t.Fatal(tc, got, err)
		}
		// Independent exhaustive signed-word oracle, not the partition helper.
		var seenTrue, seenFalse bool
		for h := -32768; h <= 32767; h++ {
			q := deathQuery(p)
			q.Height = int16(h)
			v, e := s.assets.ContactHeight(s.init.World, q, s.deathQueryObjects())
			if e != nil {
				t.Fatal(e)
			}
			if tc.limit < v {
				seenTrue = true
			} else {
				seenFalse = true
			}
		}
		if tc.known == (seenTrue && seenFalse) {
			t.Fatal("partition omitted a source decision", tc)
		}
	}
	if _, err := s.formationCornerHeight(p); err == nil {
		t.Fatal("historical numeric guard changed")
	}
}

func TestCarrierUsesReachedSupportAndSourceObjectEpoch(t *testing.T) {
	for _, support := range []int16{359, 360, 361, -1} {
		s, in := logisticsFollowerFixture(t)
		m := s.init
		m.Switches = 0x1000
		for i := range s.assets.Worlds["flat"].Grid {
			s.assets.Worlds["flat"].Grid[i] = 0x3e
		}
		e := &m.Entities[1]
		m.Entities[0].Pose.Height = 400
		e.Pose.Height = 400
		m.Offsets[0][0] = [2]int16{400, -200}
		binary.LittleEndian.PutUint32(m.Rows[0].Raw[2:], e.Pose.H0+800)
		binary.LittleEndian.PutUint32(m.Rows[0].Raw[6:], e.Pose.H1+1200)
		in.PlayerContactHeight = support
		in.Random = func() uint16 { return 8 }
		before := e.Pose
		if err := s.advanceFormation(1, in); err != nil {
			t.Fatal(err)
		}
		if support == 360 {
			if e.Pose.H0 != before.H0+200 || e.Pose.H1 != before.H1+400 || e.Pose.Angles != [3]uint16{} {
				t.Fatal("carrier must use asymmetric OBJECT/offset", e.Pose)
			}
		} else if e.Pose.H0 == before.H0+200 {
			t.Fatal("body/leader incorrectly substituted for support360", support, e.Pose)
		}
	}
}

func TestKind26SupportDescentFallbackAndContact(t *testing.T) {
	for _, tc := range []struct {
		q, height         int16
		damage            byte
		switches          uint16
		material          byte
		wantH             int16
		wantP, wantDamage byte
		forward           bool
	}{
		{-1, 0, 0, 0, 0, 8, 0, 0, false}, {0, 1, 0, 0, 0, 0, 0, 0, false}, {0, 400, 0, 0, 0, 349, 1, 0, false},
		{360, 400, 0, 0x1000, 0x3e, 394, 1, 0, false}, {360, 360, 2, 0x1000, 0x3e, 360, 2, 1, false},
		{100, 400, 0, 0, 0, 380, 1, 0, true}, {100, 400, 2, 0, 0, 400, 2, 2, true},
	} {
		s, in, sys := followerFixture(t)
		s.SetLogisticsEnabled(true)
		m := s.init
		e := &m.Entities[1]
		e.Kind = 0x26
		e.Participation = 1
		e.Pose.Height = uint16(tc.height)
		m.Switches = tc.switches
		m.Stores.Damage[1][9] = tc.damage
		g := s.assets.Geometries["flat"]
		g.Heights[0].Plane = world.Plane{0, 1, 0, -int32(tc.q)}
		if tc.material != 0 {
			for i := range s.assets.Worlds["flat"].Grid {
				s.assets.Worlds["flat"].Grid[i] = tc.material
			}
		}
		binary.LittleEndian.PutUint16(e.ControlRaw[8:], 3)
		before := e.Pose
		if err := s.advanceApproach(1, in, sys); err != nil {
			t.Fatal(tc, err)
		}
		if e.Participation != tc.wantP || m.Stores.Damage[1][9] != tc.wantDamage {
			t.Fatal(tc, e, m.Stores.Damage[1])
		}
		if tc.forward {
			if e.Pose.H1 != before.H1+36 || e.RawMotion != 72 && e.RawMotion != 4 {
				t.Fatal("separate SAR speed halves/half motion", tc, e)
			}
			if int16(e.Pose.Height) != tc.wantH {
				t.Fatal("fallback source SAR4/DEC or damaged early exit", tc, e.Pose.Height)
			}
		} else if int16(e.Pose.Height) != tc.wantH {
			t.Fatal(tc, e.Pose.Height)
		}
		if tc.q != 0 && len(events(s, "notification")) != 0 {
			t.Fatal("nonzero support fabricated ordinary landed/SOS", tc, s.Events)
		}
	}
}

func TestKind24And25KeepSeparateVisitsAndSourceAliases(t *testing.T) {
	s, in, sys := formationFixture(t)
	s.SetLogisticsEnabled(true)
	m := s.init
	for actor := 2; actor < 5; actor++ {
		m.Entities[actor].Kind = 0x20
	}
	e := &m.Entities[1]
	e.Kind = 0x24
	e.Participation = 0
	e.ControlRaw[1] = 4
	binary.LittleEndian.PutUint16(e.ControlRaw[8:], 3)
	s.AdvanceFollowers(in, sys)
	if e.Kind != 0x24 || e.Participation != 1 || e.Pose.Angles[0] != 0xf800 {
		t.Fatal("P0 must end before P1", e)
	}
	e.Pose = m.Entities[0].Pose
	s.AdvanceFollowers(in, sys)
	if e.Kind != 0x21 || e.ControlRaw[1] != 1 || len(events(s, "notification")) != 1 {
		t.Fatal("source cleanup continuation", e, s.Events)
	}
	m.Entities[0].ControlRaw[1] = 0
	s.AdvanceFollowers(in, sys)
	if e.Kind != 0x25 || e.Participation != 0 || sys.UnsupportedFollowerEmptyBurn != 0 {
		t.Fatal("defined orphan producer still excluded", e, m.FollowerLimits)
	}
	s.AdvanceFollowers(in, sys)
	if e.Participation != 1 || e.DynamicRaw[3] != 30 {
		t.Fatal("orphan P0 performed P1", e)
	}
	in.E0 = false
	s.AdvanceFollowers(in, sys)
	if e.DynamicRaw[3] != 30 {
		t.Fatal("countdown decremented withoutE0")
	}
	in.E0 = true
	s.AdvanceFollowers(in, sys)
	if e.DynamicRaw[3] != 29 {
		t.Fatal("countdown did not decrement once")
	}
	child := &m.Entities[2]
	child.ControlRaw[1] = 1
	child.ControlRaw[5] = 1
	child.ControlRaw[3] = 11
	s.AdvanceFollowers(in, sys)
	if !strings.Contains(m.FollowerLimits[1], "pre-row0") || sys.UnsupportedFollowerEmptyBurn&2 == 0 {
		t.Fatal("undefined alias silently repaired", m.FollowerLimits)
	}
}

func TestHistoricalCarrierFieldIsAbsent(t *testing.T) {
	s, _ := logisticsFollowerFixture(t)
	if len(s.init.ActorFlags) != 5 {
		t.Fatal("new source flag owner absent")
	}
	s.SetLogisticsEnabled(false)
	b, err := json.Marshal(s.init)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(b), "ActorFlags") {
		t.Fatal("historical snapshots changed")
	}
}

func TestActualFollowerObjectivesAndRecoveryFlags(t *testing.T) {
	for role := 0; role < 2; role++ {
		for _, kind := range []int{1, 2, 4} {
			for _, actor := range []int{1, 4} {
				s, in, _ := followerFixture(t)
				s.SetLogisticsEnabled(true)
				m := s.init
				r := &m.Targets.Records[role+2]
				binary.LittleEndian.PutUint16(r[32:], uint16(kind))
				binary.LittleEndian.PutUint16(r[52:], 10)
				for i := 0; i < 5; i++ {
					m.Stores.Ships[i].Flags = 0
					m.Entities[i].ControlRaw[1] = 1
				}
				e := &m.Entities[actor]
				e.Pose.Height = 15
				e.RawMotion = 15
				m.Stores.Ships[actor].Flags = 6
				if kind == 1 {
					m.Stores.Ships[actor].Flags |= 8
				}
				if kind == 4 {
					m.Stores.Ships[actor].Flags |= 0x20
					e.Pose.Height = 1000
				}
				binary.LittleEndian.PutUint32(r[34:], e.Pose.H0)
				binary.LittleEndian.PutUint32(r[38:], e.Pose.H1)
				m.Entities[10].Object.Raw[0] = byte(0x10 << role)
				m.Entities[10].Kind = 0x15
				m.Entities[10].Participation = 1
				in.PlayerIndicatedSpeed = 1000
				in.Random = func() uint16 { t.Fatal("objective RNG"); return 0 }
				s.AdvanceObjectives(in)
				if r[32] != 0 {
					t.Fatal("eligible actual follower omitted", role, kind, actor)
				}
				if kind == 2 && (m.ActorFlags[actor] != 1<<uint(role) || m.ObjectiveAssignments[role] != uint16(actor+1) || m.Stores.Ships[actor].Flags != 0x1e) {
					t.Fatal("pickup owner", m.ActorFlags, m.ObjectiveAssignments)
				}
				if ns := events(s, "notification"); len(ns) != 1 || ns[0].Actor != actor {
					t.Fatal("notice uses wrong actor", ns)
				}
				if len(events(s, "mass")) != 0 {
					t.Fatal("follower changed player mass")
				}
			}
		}
	}
}

func TestNearbyHighLandingCleanupAndSelectedDiscard(t *testing.T) {
	s, in, _ := followerFixture(t)
	s.SetLogisticsEnabled(true)
	m := s.init
	e := &m.Entities[1]
	r := &m.Targets.Records[2]
	for i := 0; i < 5; i++ {
		m.Stores.Ships[i].Flags = 0
	}
	m.Stores.Ships[1].Flags = 0x16
	e.Pose.Height = 16
	e.Kind = 0x20
	binary.LittleEndian.PutUint16(r[32:], 1)
	binary.LittleEndian.PutUint32(r[34:], e.Pose.H0)
	binary.LittleEndian.PutUint32(r[38:], e.Pose.H1)
	s.AdvanceObjectives(in)
	if e.Kind != 0x26 || e.ControlRaw[1] != 7 || r[32] != 1 {
		t.Fatal("missing actual approach producer", e)
	}
	e.Pose.Height = 15
	s.AdvanceObjectives(in)
	if r[32] != 0 || e.Kind != 0x24 || e.ControlRaw[1] != 4 {
		t.Fatal("missing cleanup producer", e)
	}
	m.Stores.Ships[1].Flags = 0x1e
	if !s.DropCargo(1) || m.Stores.Ships[1].Flags != 6 || len(events(s, "mass")) != 0 {
		t.Fatal("selected follower discard ownership")
	}
}

func TestFuelOldEmptyAndPostContactInitializeRemainOrdered(t *testing.T) {
	s, in, sys := formationFixture(t)
	s.SetLogisticsEnabled(true)
	m := s.init
	for actor := 2; actor < 5; actor++ {
		m.Entities[actor].Kind = 0x20
		m.Entities[actor].ControlRaw[1] = 0
	}
	sys.Fuel[1] = 1
	sys.Countdown = 0
	draws := 0
	in.Random = func() uint16 { draws++; return 127 }
	sys.AdvanceWithFollowers(true, 16, s.FollowerFuelContext(in.Random))
	if sys.Fuel[1] != 0 || m.FollowerFailureKnown[1] || draws != 0 {
		t.Fatal("reaching0 acted as oldempty", sys.Fuel, m.FollowerFailureKnown, draws)
	}
	s.AdvanceFollowers(in, sys)
	if draws != 2 {
		t.Fatal("formation lost its two shared draws", draws)
	}
	sys.Countdown = 0
	sys.AdvanceWithFollowers(true, 16, s.FollowerFuelContext(in.Random))
	if draws != 3 || m.Stores.Damage[1][9] != 2 || !m.FollowerFailureKnown[1] {
		t.Fatal("next oldempty visit lost exact fuel/proficiency producer", draws, m.Stores.Damage[1])
	}
	s.AdvanceFollowers(in, sys)
	if draws != 5 || m.Entities[1].Kind != 0x26 {
		t.Fatal("old21 must finish own motion before kind26", draws, m.Entities[1])
	}
	for i := 0; i < 100 && m.Entities[1].Kind == 0x26; i++ {
		s.AdvanceFollowers(in, sys)
	}
	e := &m.Entities[1]
	if e.Kind != 0x20 || e.Participation != 0 || m.Entities[5].Participation != 1 {
		t.Fatal("real ground0 contact/reserve omitted", e, m.Entities[5])
	}
	s.AdvanceFollowers(in, sys)
	if e.Participation != 1 || e.Pose.Height != 2 || sys.UnsupportedFollowerEmptyBurn != 0 {
		t.Fatal("post-contact P0 not one separate initialization", e, m.FollowerLimits)
	}
}

func TestUnknownObjectiveReadRemainsStickyResultExclusion(t *testing.T) {
	s, in := objectiveFixture(t, 0, 2)
	s.SetLogisticsEnabled(true)
	s.init.Entities[10].Kind = 0x40
	s.AdvanceObjectives(in)
	first := s.init.Targets.ProgressFailure
	if !strings.Contains(first, "row80") {
		t.Fatal("actual unknown pickup read did not exclude result", first)
	}
	s.init.Entities[10].Kind = 0x15
	s.init.Entities[10].Participation = 1
	s.AdvanceObjectives(in)
	if s.init.Targets.ProgressFailure != first || s.init.ObjectiveLimits[0] == "" {
		t.Fatal("later progress erased an unknown source read")
	}
}

func TestIdleFollowerActualSupportAndCarrierInitialization(t *testing.T) {
	for _, tc := range []struct {
		q            int16
		phase        byte
		height, want int16
		material     byte
		snap         bool
	}{
		{359, 1, 362, 361, 0x3e, false}, {360, 1, 362, 362, 0x3e, false}, {361, 1, 362, 363, 0x3e, false},
		{100, 0, 0, 102, 0, false}, {100, 1, 102, 102, 0, false}, {-1, 0, 0, 1, 0, false},
		{360, 0, 10, 362, 0x3e, true}, {360, 0, 10, 362, 0, false},
	} {
		s, _, _ := followerFixture(t)
		s.SetLogisticsEnabled(true)
		m := s.init
		e := &m.Entities[1]
		e.Participation = tc.phase
		e.Kind = 0x20
		e.Pose.Height = uint16(tc.height)
		e.Pose.Angles = [3]uint16{123, 456, 789}
		m.Switches = 0x1000
		s.assets.Geometries["flat"].Heights[0].Plane = world.Plane{0, 1, 0, -int32(tc.q)}
		for i := range s.assets.Worlds["flat"].Grid {
			s.assets.Worlds["flat"].Grid[i] = tc.material
		}
		oldH0, oldH1 := e.Pose.H0, e.Pose.H1
		binary.LittleEndian.PutUint32(m.Rows[0].Raw[2:], 0x52000)
		if err := s.advanceIdleFollower(1); err != nil {
			t.Fatal(tc, err)
		}
		if int16(e.Pose.Height) != tc.want || e.Pose.H1 != oldH1 {
			t.Fatal("actual q/clamp/SAR3 or asymmetric carrier reference", tc, e.Pose)
		}
		if tc.snap {
			if e.Pose.H0 != 0x52000 || e.Pose.Angles[1] != 0 {
				t.Fatal("source P0 carrier H0/yaw owner", tc, e.Pose)
			}
		} else if e.Pose.H0 != oldH0 {
			t.Fatal("noncarrier H0 changed", tc, e.Pose)
		}
	}
}
