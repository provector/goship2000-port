package combat

import (
	"encoding/binary"
	"gunship2000-assets/game/flight"
	gs "gunship2000-assets/game/state"
	"testing"
)

func TestCorridorProbeAndSweptGate(t *testing.T) {
	const mid = worldEdge / 2
	for _, v := range []struct {
		from, next, gate [2]int32
		ok               bool
	}{
		{[2]int32{139, mid}, [2]int32{-4, mid}, [2]int32{0, mid}, true},
		{[2]int32{worldEdge - 141, mid}, [2]int32{worldEdge + 9, mid}, [2]int32{worldEdge, mid}, true},
		{[2]int32{mid, 139}, [2]int32{mid, -4}, [2]int32{mid, 0}, true},
		{[2]int32{mid, worldEdge - 142}, [2]int32{mid, worldEdge + 20}, [2]int32{mid, worldEdge}, true},
		{[2]int32{100, 100}, [2]int32{-10, -10}, [2]int32{0, 0}, true},
		{[2]int32{worldEdge - 100, 100}, [2]int32{worldEdge + 10, -10}, [2]int32{worldEdge, 0}, true},
		{[2]int32{100, worldEdge - 100}, [2]int32{-10, worldEdge + 10}, [2]int32{0, worldEdge}, true},
		{[2]int32{worldEdge - 100, worldEdge - 100}, [2]int32{worldEdge + 10, worldEdge + 10}, [2]int32{worldEdge, worldEdge}, true},
		{[2]int32{139, mid + 128}, [2]int32{-10, mid + 128}, [2]int32{0, mid}, false},
		{[2]int32{139, mid}, [2]int32{50, mid}, [2]int32{0, mid}, false},
	} {
		pose := func(p [2]int32) gs.InitializedPose { return gs.InitializedPose{H0: uint32(p[0]), H1: uint32(p[1])} }
		from, next := pose(v.from), pose(v.next)
		if got := crossedCorridor(from, next, NavigationPoint{uint32(v.gate[0]), uint32(v.gate[1])}); got != v.ok {
			t.Fatal(v, got)
		}
		if !insideWorld(gridProbe(from, next)) {
			t.Fatal("probe escaped grid", v)
		}
	}
}

func TestCorridorSectionDispatchAndIncidentLookahead(t *testing.T) {
	s, in := routeFixture(t)
	s.SetSafeFlightOrdersEnabled(true)
	s.SetCorridorNavigation(true)
	m := s.init
	m.Switches |= 0x800
	lead, child := &m.Entities[1], &m.Entities[2]
	lead.Kind, lead.Participation, lead.ControlRaw[0], lead.ControlRaw[1] = 0x22, 1, 1, 8
	child.Kind, child.Participation, child.ControlRaw[1], child.ControlRaw[5] = 0x21, 1, 1, 1
	lead.Pose.H0, lead.Pose.H1, lead.Pose.Height = 3871, 391705, 147
	child.Pose.H0, child.Pose.H1, child.Pose.Height = 6000, 392816, 147
	put := binary.LittleEndian.PutUint32
	put(m.Targets.Records[0][34:], 24579)
	put(m.Targets.Records[0][38:], 393216)
	put(m.Rows[0].Raw[2:], 24579)
	put(m.Rows[0].Raw[6:], 393216)
	put(m.Rows[1].Raw[2:], 0)
	put(m.Rows[1].Raw[6:], 0xfffffe70)
	put(lead.ControlRaw[10:], 24579)
	put(lead.ControlRaw[14:], 392816)
	if !s.finalizeCorridorRoute(1) {
		t.Fatal("missing witnessed corridor")
	}
	sys := &flight.Systems{}
	for i := 0; i < 650 && lead.Kind != 0x41; i++ {
		if _, err := s.AdvanceFollowers(in, sys); err != nil {
			t.Fatal(err)
		}
		if sys.UnsupportedFollowerEmptyBurn != 0 {
			t.Fatal("navigation veto", m.FollowerLimits)
		}
	}
	if lead.Kind != 0x41 || child.Kind != 0x22 || lead.Visible || lead.ControlRaw[1] != 0 {
		t.Fatal("section did not separate on exit", lead.Kind, child.Kind)
	}
	if child.ControlRaw[5] != 1 {
		t.Fatal("child identity changed")
	}
	for i := 0; i < 650 && child.Kind != 0x41; i++ {
		if _, err := s.AdvanceFollowers(in, sys); err != nil {
			t.Fatal(err)
		}
	}
	if child.Kind != 0x41 || sys.UnsupportedFollowerEmptyBurn != 0 {
		t.Fatal("child did not independently exit", child.Kind, m.FollowerLimits)
	}
	n := 0
	for _, v := range s.Events {
		if v.Notification == 12 {
			n++
		}
	}
	if n != 2 {
		t.Fatalf("return notices %d", n)
	}
}

func TestCriticalReturnNeverClaimsCorridor(t *testing.T) {
	s, in := routeFixture(t)
	s.SetSafeFlightOrdersEnabled(true)
	s.SetMissionActorsEnabled(true)
	s.SetCorridorNavigation(true)
	s.init.Switches |= 0x800
	e := &s.init.Entities[1]
	e.Kind, e.Participation, e.ControlRaw[0], e.ControlRaw[1] = 0x22, 1, 1, 8
	e.Pose.H0, e.Pose.H1, e.Pose.Height = 50, 50000, 200
	binary.LittleEndian.PutUint32(s.init.Targets.Records[0][38:], 50000)
	binary.LittleEndian.PutUint32(e.ControlRaw[14:], 50000)
	binary.LittleEndian.PutUint32(e.ControlRaw[44:], 50000)
	s.init.Stores.Damage[1][9] = 2
	if err := s.advanceFollowerRoute(1, in); err != nil {
		t.Fatal(err)
	}
	if e.Kind != 0x26 || e.ControlRaw[1] != 6 {
		t.Fatal("disabled aircraft fate", e.Kind, e.ControlRaw[1])
	}
}

func TestSameEastingCannotInventEgress(t *testing.T) {
	s, _ := routeFixture(t)
	s.SetCorridorNavigation(true)
	m := s.init
	m.Switches |= 0x800
	e := &m.Entities[1]
	e.Kind, e.ControlRaw[0], e.ControlRaw[1] = 0x22, 1, 8
	put := binary.LittleEndian.PutUint32
	put(m.Targets.Records[0][34:], 24579)
	put(m.Targets.Records[0][38:], 393216)
	put(e.ControlRaw[10:], 24579)
	put(e.ControlRaw[14:], 90000)
	if _, ok := s.corridorGoal(e); ok {
		t.Fatal("unrelated destination admitted")
	}
	put(e.ControlRaw[14:], 393216)
	binary.LittleEndian.PutUint16(m.Targets.Records[1][32:], 1)
	put(m.Targets.Records[1][42:], 24579)
	put(m.Targets.Records[1][46:], 393216)
	if _, ok := s.corridorGoal(e); ok {
		t.Fatal("FARP acquired egress")
	}
	m.Switches = 0x1000
	if _, ok := s.corridorGoal(e); ok {
		t.Fatal("carrier acquired egress")
	}
}

func TestGuardedRouteAcceptsOnlyTheDesignatedExit(t *testing.T) {
	const mid = worldEdge / 2
	for _, tc := range []struct {
		name                         string
		homeX, homeZ, cacheX, cacheZ uint32
		kind                         byte
		current, exits               bool
	}{
		{"east", worldEdge - 24576, mid, worldEdge, mid, 0x22, true, true},
		{"upper northing", mid, worldEdge - 24576, mid, worldEdge, 0x22, true, true},
		{"old policy", worldEdge - 24576, mid, worldEdge, mid, 0x22, false, false},
		{"wrong lateral position", worldEdge - 24576, mid, worldEdge, mid + 256, 0x22, true, false},
		{"ordinary waypoint", worldEdge - 24576, mid, worldEdge, mid, 0x23, true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, in := routeFixture(t)
			s.SetCorridorNavigation(true)
			s.EnableResilientRoutes()
			if tc.current {
				s.EnableExitWaypoint()
			}
			m := s.init
			m.Switches |= 0x800
			e := &m.Entities[1]
			e.Kind, e.Participation, e.ControlRaw[0], e.ControlRaw[1] = tc.kind, 1, 1, 8
			put := binary.LittleEndian.PutUint32
			put(m.Targets.Records[0][34:], tc.homeX)
			put(m.Targets.Records[0][38:], tc.homeZ)
			put(e.ControlRaw[10:], tc.homeX)
			put(e.ControlRaw[14:], tc.homeZ)
			put(e.ControlRaw[40:], tc.cacheX)
			put(e.ControlRaw[44:], tc.cacheZ)
			e.Pose.H0, e.Pose.H1 = tc.cacheX-8, tc.cacheZ-8
			if err := s.advanceFollowerRoute(1, in); err != nil {
				t.Fatal(err)
			}
			if tc.exits {
				if e.Kind != 0x41 || e.ControlRaw[1] != 0 || e.Visible {
					t.Fatal("did not depart", e.Kind, e.ControlRaw[1])
				}
			} else if e.Kind != 0x20 || e.ControlRaw[1] != 2 || s.RouteWarnings()[1] == "" {
				t.Fatal("invalid route not held", e.Kind, e.ControlRaw[1])
			}
		})
	}
}
