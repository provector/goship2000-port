package combat

import (
	"encoding/binary"
	"testing"

	"gunship2000-assets/game/avionics"
	"gunship2000-assets/game/flight"
	gamestate "gunship2000-assets/game/state"
	"gunship2000-assets/game/world"
)

func routeFixture(t *testing.T) (*State, Input) {
	s, in := logisticsFollowerFixture(t)
	s.SetOrdersEnabled(true)
	s.init.CrewPerformance = make([]gamestate.SourceCrewPerformance, 5)
	for a := range s.init.CrewPerformance {
		s.init.CrewPerformance[a].SkillKnown = true
	}
	e := &s.init.Entities[1]
	e.Kind, e.Participation, e.ControlRaw[0], e.ControlRaw[1] = 0x23, 0, 1, 3
	binary.LittleEndian.PutUint16(e.ControlRaw[6:], 1)
	binary.LittleEndian.PutUint16(e.ControlRaw[8:], 3)
	return s, in
}

func TestRouteInitMoveArrivalAndHistoricalGate(t *testing.T) {
	for _, kind := range []byte{0x22, 0x23} {
		s, in := routeFixture(t)
		e := &s.init.Entities[1]
		e.Kind = kind
		x, z := e.Pose.H0, e.Pose.H1+6000
		binary.LittleEndian.PutUint32(e.ControlRaw[40:], x)
		binary.LittleEndian.PutUint32(e.ControlRaw[44:], z)
		if err := s.advanceFollowerRoute(1, in); err != nil {
			t.Fatal(err)
		}
		if e.Participation != 1 || e.Pose.Angles[0] != 0xf800 || e.Pose.H1 != z-6000 {
			t.Fatal("P0 also moved", e)
		}
		for i := 0; i < 500 && e.Kind == kind; i++ {
			if err := s.advanceFollowerRoute(1, in); err != nil {
				t.Fatal(err)
			}
		}
		if kind == 0x23 {
			if e.Kind != 0x20 || e.Participation != 0 || e.ControlRaw[1] != 2 {
				t.Fatal(e)
			}
		} else if e.Kind != 0x26 || e.ControlRaw[1] != 6 || e.Pose.H0 != x || e.Pose.H1 != z {
			t.Fatal(e)
		}
		if e.ControlRaw[0] != 0 {
			t.Fatal("completion index")
		}
	}
	s, _, sys := followerFixture(t)
	s.SetLogisticsEnabled(true)
	s.init.Entities[1].Kind = 0x23
	before := s.init.Entities[1]
	_, err := s.AdvanceFollowers(Input{}, sys)
	if err != nil || s.init.Entities[1] != before {
		t.Fatal("historical profile silently enabled new controller", err)
	}
}

func TestRouteDerivedEpochPointsAndSourceBoundaries(t *testing.T) {
	s, in := routeFixture(t)
	e := &s.init.Entities[1]
	e.Participation = 1
	// Desired is remote; cached waypoint is exactly reached. Consumer must
	// advance, without reading desired or running the next visit immediately.
	binary.LittleEndian.PutUint32(e.ControlRaw[10:], 0x60000)
	binary.LittleEndian.PutUint32(e.ControlRaw[40:], e.Pose.H0)
	binary.LittleEndian.PutUint32(e.ControlRaw[44:], e.Pose.H1)
	binary.LittleEndian.PutUint32(e.ControlRaw[50:], e.Pose.H0+1600)
	p := e.Pose
	if err := s.advanceFollowerRoute(1, in); err != nil {
		t.Fatal(err)
	}
	if e.ControlRaw[0] != 2 || e.Pose != p || e.Kind != 0x23 {
		t.Fatal(e)
	}
	// Approximation8 is not arrival (<8, unsigned);7 is arrival.
	for _, delta := range []uint32{7 * 16, 8 * 16} {
		s, in := routeFixture(t)
		e := &s.init.Entities[1]
		e.Participation = 1
		binary.LittleEndian.PutUint32(e.ControlRaw[40:], e.Pose.H0+delta)
		binary.LittleEndian.PutUint32(e.ControlRaw[44:], e.Pose.H1)
		if err := s.advanceFollowerRoute(1, in); err != nil {
			t.Fatal(err)
		}
		if (e.Kind == 0x20) != (delta == 112) {
			t.Fatal(delta, e.Kind)
		}
	}
}

func TestCorrelatedAvoidanceDecision(t *testing.T) {
	s, _ := routeFixture(t)
	g := s.assets.Geometries["flat"]
	low, high := g.Heights[0], g.Heights[0]
	low.Threshold = 100
	low.Plane = world.Plane{0, 1, 0, -1000}
	high.Plane = world.Plane{0, 1, 0, -2000}
	g.Heights = []world.HeightRegion{low, high}
	a, b := s.init.Entities[1].Pose, s.init.Entities[1].Pose
	b.H0 += 256
	for _, limit := range []int16{1999, 2000, 3999, 4000} {
		v, err := s.followerRouteBlocked(a, b, limit)
		seen := map[bool]bool{}
		for h := -32768; h <= 32767; h++ {
			qa, qb := deathQuery(a), deathQuery(b)
			qa.Height, qb.Height = int16(h), int16(h)
			x, _ := s.assets.ContactHeight(s.init.World, qa, s.deathQueryObjects())
			y, _ := s.assets.ContactHeight(s.init.World, qb, s.deathQueryObjects())
			seen[limit < int16(x+y)] = true
		}
		if (err == nil) != (len(seen) == 1) || err == nil && !seen[v] {
			t.Fatal(limit, v, err, seen)
		}
	}
	// Wrapped sum must be evaluated as a word before the signed comparison.
	low.Plane = world.Plane{0, 1, 0, -20000}
	g.Heights = []world.HeightRegion{low}
	if v, err := s.followerRouteBlocked(a, b, 0); err != nil || v {
		t.Fatal("word overflow changed", v, err)
	}
}

func TestGPSReturnCacheProficiencyDrawsAndOwnership(t *testing.T) {
	for _, skill := range []byte{0, 25, 127, 128, 255} {
		s, _ := routeFixture(t)
		m := s.init
		var contacts [80]avionics.Contact
		for actor := 1; actor < 5; actor++ {
			e := &m.Entities[actor]
			e.ControlRaw[1] = 3
			v := skill
			m.Stores.Ships[actor].Proficiency = &v
			contacts[actor].Flags = 0xff
			for p := 0; p < 3; p++ {
				o := 10 + p*10
				binary.LittleEndian.PutUint32(e.ControlRaw[o:], uint32(0x40000+actor*1000+p*400))
				binary.LittleEndian.PutUint32(e.ControlRaw[o+4:], 0x40000)
				binary.LittleEndian.PutUint16(e.ControlRaw[o+8:], 0xabcd)
			}
		}
		calls := 0
		draw := func() uint16 { calls++; return uint16(calls * 23) }
		before := m.Entities[0]
		if err := s.FinalizeFollowerRoutes(draw, &contacts); err != nil {
			t.Fatal(err)
		}
		want := 48
		if skill == 0 {
			want = 0
		}
		if calls != want {
			t.Fatal(skill, calls)
		}
		if m.Entities[0] != before {
			t.Fatal("player route mutated")
		}
		for actor := 1; actor < 5; actor++ {
			e := &m.Entities[actor]
			if contacts[actor].Flags != 0xf8 {
				t.Fatal(contacts[actor])
			}
			for p := 0; p < 3; p++ {
				o := 40 + p*10
				tail := binary.LittleEndian.Uint16(e.ControlRaw[o+8:])
				if skill == 0 && tail != 0xabcd || skill != 0 && tail != 0 {
					t.Fatal(skill, tail)
				}
			}
		}
	}
	for _, tc := range []struct {
		v    uint32
		r    byte
		want uint32
	}{{0x40000, 128, 0x48000}, {0x40000, 129, 0x38100}, {0x4000, 2, 0x4000}, {0x7c000, 1, 0x7c000}} {
		if v := routeCoordinate(tc.v, tc.r); v != tc.want {
			t.Fatal(tc, v)
		}
	}
}

func TestGPSCacheEarlyPrefixAvoidanceAndDirectReturn(t *testing.T) {
	s, _ := routeFixture(t)
	m := s.init
	e := &m.Entities[1]
	v := byte(25)
	m.Stores.Ships[1].Proficiency = &v
	binary.LittleEndian.PutUint16(m.Targets.Records[2][32:], 1)
	binary.LittleEndian.PutUint32(m.Targets.Records[2][34:], 0x50000)
	binary.LittleEndian.PutUint32(m.Targets.Records[2][38:], 0x50000)
	binary.LittleEndian.PutUint32(e.ControlRaw[10:], 0x50000)
	binary.LittleEndian.PutUint32(e.ControlRaw[14:], 0x50000)
	binary.LittleEndian.PutUint16(e.ControlRaw[18:], 0xaaaa)
	// First unassigned row terminates the scan; actor3 cache must survive.
	m.Rows[2].Raw[10] = 0
	m.Entities[3].ControlRaw[40] = 0xef
	var contacts [80]avionics.Contact
	if err := s.FinalizeFollowerRoutes(func() uint16 { t.Fatal("avoidance consumed variation"); return 0 }, &contacts); err != nil {
		t.Fatal(err)
	}
	if binary.LittleEndian.Uint32(e.ControlRaw[40:]) != 0x50000-128 || binary.LittleEndian.Uint16(e.ControlRaw[48:]) != 0xaaaa || m.Entities[3].ControlRaw[40] != 0xef {
		t.Fatal(e.ControlRaw, m.Entities[3].ControlRaw)
	}
	// Ctrl+B immediately copies without proficiency and only the recipient.
	binary.LittleEndian.PutUint32(e.ControlRaw[10:], 0x60000)
	contacts[1].Flags = 0xff
	if err := s.FinalizeDirectReturn(1, &contacts); err != nil {
		t.Fatal(err)
	}
	if binary.LittleEndian.Uint32(e.ControlRaw[40:]) != 0x60000 || contacts[1].Flags != 0xf8 {
		t.Fatal(e.ControlRaw)
	}
}

// This is a supplied producer fixture, not human/OS gameplay: real ascending
// movement, objective writer, landing and attribution run without completion
// injection. Menus integration adds the actual GPS command and career owners.
func TestOrderedFollowerFliesPickupDeliveryReconWithBothPolicies(t *testing.T) {
	for _, autonomy := range []bool{false, true} {
		for _, kind := range []uint16{1, 2, 4} {
			s, in := routeFixture(t)
			s.SetWingmenAutonomyEnabled(autonomy)
			m := s.init
			e := &m.Entities[1]
			x, z := e.Pose.H0, e.Pose.H1+24000
			binary.LittleEndian.PutUint32(e.ControlRaw[10:], x)
			binary.LittleEndian.PutUint32(e.ControlRaw[14:], z)
			var contacts [80]avionics.Contact
			if err := s.FinalizeFollowerRoutes(in.Random, &contacts); err != nil {
				t.Fatal(err)
			}
			r := &m.Targets.Records[2]
			binary.LittleEndian.PutUint16(r[32:], kind)
			binary.LittleEndian.PutUint16(r[50:], 1)
			binary.LittleEndian.PutUint32(r[34:], x)
			binary.LittleEndian.PutUint32(r[38:], z)
			m.Stores.Ships[1].Flags = 6
			if kind == 1 {
				m.Stores.Ships[1].Flags |= 0x18
			}
			if kind == 4 {
				m.Stores.Ships[1].Flags = 0x25
			}
			cargo := &m.Entities[10]
			cargo.Kind, cargo.Participation = 0x15, 1
			cargo.Object.Raw[0] = 0x10
			// Held aircraft2 and pad commander never receive the route.
			held, player := m.Entities[2].Pose, m.Entities[0].Pose
			sys := flight.NewSystems([]int16{100, 100, 100, 100, 100}, []bool{true, true, true, true, true})
			steps := 0
			for ; steps < 1500 && !s.Primary.Achieved; steps++ {
				if _, err := s.AdvanceFollowers(in, &sys); err != nil {
					t.Fatal(err)
				}
				s.AdvanceObjectives(in)
			}
			if steps < 2 || !s.Primary.Achieved || m.FollowerLimits[1] != "" || sys.UnsupportedFollowerEmptyBurn != 0 {
				t.Fatalf("kind%d autonomy%v steps%d: e%+v limits%v", kind, autonomy, steps, *e, m.FollowerLimits)
			}
			if m.Entities[0].Pose != player || m.Entities[2].Pose.H0 != held.H0 || m.Entities[2].Pose.H1 != held.H1 {
				t.Fatal("unaddressed aircraft moved")
			}
			if kind == 2 && (m.ObjectiveAssignments[0] != 2 || m.ActorFlags[1]&1 == 0 || m.Stores.Ships[1].Flags&0xf8 != 0x18) {
				t.Fatal("wrong pickup owner", m.ObjectiveAssignments, m.ActorFlags)
			}
			if kind == 1 && m.Stores.Ships[1].Flags&0xf8 != 0 {
				t.Fatal("delivery load not consumed")
			}
			found := false
			for _, n := range events(s, "notification") {
				if n.Notification == 16 && n.Actor == 1 {
					found = true
				}
			}
			if !found {
				t.Fatal("no actual actor1 completion feedback")
			}
		}
	}
}

func TestAutonomyDisablesFirePreservesDamageAndLiveProjectile(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		s, in := fireFixture(t)
		s.SetOrdersEnabled(true)
		s.SetWingmenAutonomyEnabled(enabled)
		draws := 0
		in.Random = func() uint16 { draws++; return 127 }
		if err := s.advanceHealthyFollower(1, in); err != nil {
			t.Fatal(err)
		}
		if enabled && (draws != 1 || s.init.Stores.Ships[1].Stations[1].Ammunition != 9) || !enabled && (draws != 0 || s.init.Stores.Ships[1].Stations[1].Ammunition != 10) {
			t.Fatal(enabled, draws, s.init.Stores.Ships[1])
		}
		// Disabling policy cannot delete an existing shot or disable a damaged
		// aircraft's mechanical successor through A7E6.
		s.Slots[16].Lifetime = 3
		s.SetWingmenAutonomyEnabled(false)
		s.init.Stores.Damage[1][9] = 2
		if err := s.advanceHealthyFollower(1, in); err != nil {
			t.Fatal(err)
		}
		if s.Slots[16].Lifetime != 3 || s.init.Entities[1].Kind != 0x26 || s.init.Entities[1].ControlRaw[1] != 6 {
			t.Fatal("policy suppressed mechanics")
		}
	}
}
