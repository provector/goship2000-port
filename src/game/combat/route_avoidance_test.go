package combat

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"gunship2000-assets/game/avionics"
	"gunship2000-assets/game/world"
	"reflect"
	"strings"
	"testing"
)

func TestAvoidanceThreeClearReleaseAndImmediateEntry(t *testing.T) {
	s, _ := routeFixture(t)
	s.SetRouteAvoidanceEnabled(true)
	e := &s.init.Entities[1]
	e.Participation = 1
	e.Pose.Angles[1] = 0
	for i, v := range []struct{ blocked, want bool }{{false, false}, {true, true}, {false, true}, {true, true}, {false, true}, {false, true}, {false, false}, {false, false}, {true, true}} {
		if got := s.orderedAvoidance(1, e, 0, 100, 200, v.blocked); got != v.want {
			t.Fatal(i, v, got, s.routeAvoidance[1])
		}
	}
	// This remains explicit ordered movement under either autonomy switch.
	for _, autonomous := range []bool{false, true} {
		s.SetWingmenAutonomyEnabled(autonomous)
		s.ResetRouteAvoidance(1)
		if !s.orderedAvoidance(1, e, 0, 100, 200, true) || !s.orderedAvoidance(1, e, 0, 100, 200, false) {
			t.Fatal("autonomy changed ordered avoidance")
		}
	}
	for _, yaw := range []uint16{0x4000, 0xc000} {
		e.Pose.Angles[1] = yaw
		s.orderedAvoidance(1, e, 0, 100, 200, false)
		if s.routeAvoidance[1].Stage != 0 {
			t.Fatal("inclusive90degree gate retained latch", yaw)
		}
	}
}

func TestAvoidanceRouteIdentityAndPhaseResets(t *testing.T) {
	s, in := routeFixture(t)
	s.SetRouteAvoidanceEnabled(true)
	e := &s.init.Entities[1]
	e.Participation = 1
	e.Pose.Angles[1] = 0
	arm := func() {
		e.Kind, e.ControlRaw[0], e.Participation = 0x23, 1, 1
		s.orderedAvoidance(1, e, 0, 100, 200, true)
	}
	for _, change := range []string{"kind", "index", "goalX", "goalZ", "P0"} {
		arm()
		x, z := uint32(100), uint32(200)
		switch change {
		case "kind":
			e.Kind = 0x22
		case "index":
			e.ControlRaw[0] = 2
		case "goalX":
			x++
		case "goalZ":
			z++
		case "P0":
			e.Participation = 0
		}
		if s.orderedAvoidance(1, e, 0, x, z, false) || s.routeAvoidance[1].Stage != 0 {
			t.Fatal(change, "stale route latch")
		}
	}
	arm()
	e.Participation = 0
	if err := s.advanceFollowerRoute(1, in); err != nil {
		t.Fatal(err)
	}
	if s.routeAvoidance[1].Stage != 0 {
		t.Fatal("phase0 retained latch")
	}
	arm()
	binary.LittleEndian.PutUint32(e.ControlRaw[40:], e.Pose.H0)
	binary.LittleEndian.PutUint32(e.ControlRaw[44:], e.Pose.H1)
	if err := s.advanceFollowerRoute(1, in); err != nil {
		t.Fatal(err)
	}
	if e.Kind != 0x20 || s.routeAvoidance[1].Stage != 0 {
		t.Fatal("arrival retained latch")
	}
	arm()
	s.init.Rows[1].Raw[10] |= 0x20
	skill := byte(0)
	s.init.Stores.Ships[1].Proficiency = &skill
	var contacts [80]avionics.Contact
	if err := s.FinalizeFollowerRoutes(func() uint16 { t.Fatal("zero proficiency consumed RNG"); return 0 }, &contacts); err != nil {
		t.Fatal(err)
	}
	if s.routeAvoidance[1].Stage != 0 {
		t.Fatal("unchanged/cancelled GPS finalizer retained latch")
	}
}

func TestAvoidanceUsesOriginalQueryAndDoesNotBypassUnknownGuard(t *testing.T) {
	s, in := routeFixture(t)
	s.SetRouteAvoidanceEnabled(true)
	e := &s.init.Entities[1]
	e.Participation = 1
	e.Pose.Height = 154
	e.Pose.Angles[1] = 0
	binary.LittleEndian.PutUint32(e.ControlRaw[40:], e.Pose.H0)
	binary.LittleEndian.PutUint32(e.ControlRaw[44:], e.Pose.H1+10000)
	g := s.assets.Geometries["flat"]
	region := g.Heights[0]
	region.Plane = world.Plane{0, 1, 0, -100}
	g.Heights = []world.HeightRegion{region}
	if err := s.advanceFollowerRoute(1, in); err != nil {
		t.Fatal(err)
	}
	if s.routeAvoidance[1].Stage != 1 {
		t.Fatal("original blocked query did not enter")
	}
	region.Plane = world.Plane{0, 1, 0, 0}
	g.Heights = []world.HeightRegion{region}
	for _, want := range []uint8{2, 3, 0} {
		if err := s.advanceFollowerRoute(1, in); err != nil {
			t.Fatal(err)
		}
		if s.routeAvoidance[1].Stage != want {
			t.Fatal("clear query cadence", want, s.routeAvoidance[1])
		}
	}
	// An active latch cannot replace the original mixed BP-30 decision/error.
	e.Pose.Height = 3000
	s.routeAvoidance[1] = RouteAvoidanceLatch{Stage: 1, Kind: e.Kind, Index: e.ControlRaw[0]}
	low, high := region, region
	low.Threshold = 100
	low.Plane = world.Plane{0, 1, 0, -1000}
	high.Plane = world.Plane{0, 1, 0, -2000}
	g.Heights = []world.HeightRegion{low, high}
	err := s.advanceFollowerRoute(1, in)
	if err == nil || !strings.Contains(err.Error(), "shared unwritten BP-30") {
		t.Fatal("unknown decision bypassed", err)
	}
	if s.routeAvoidance[1].Stage != 1 {
		t.Fatal("guard reached policy mutation")
	}
}

func TestAvoidanceUnobstructedTrajectoryAndHistoricalState(t *testing.T) {
	for _, autonomy := range []bool{false, true} {
		old, in := routeFixture(t)
		next, _ := routeFixture(t)
		next.SetRouteAvoidanceEnabled(true)
		old.SetWingmenAutonomyEnabled(autonomy)
		next.SetWingmenAutonomyEnabled(autonomy)
		for _, s := range []*State{old, next} {
			e := &s.init.Entities[1]
			binary.LittleEndian.PutUint32(e.ControlRaw[40:], e.Pose.H0)
			binary.LittleEndian.PutUint32(e.ControlRaw[44:], e.Pose.H1+6000)
		}
		for i := 0; i < 500; i++ {
			if err := old.advanceFollowerRoute(1, in); err != nil {
				t.Fatal(err)
			}
			if err := next.advanceFollowerRoute(1, in); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(old.init, next.init) {
				t.Fatal("unobstructed source state differs", autonomy, i)
			}
			if next.init.Entities[1].Kind != 0x23 {
				break
			}
		}
		if next.init.Entities[1].Kind != 0x20 || next.routeAvoidance[1].Stage != 0 {
			t.Fatal("arrival/hold", autonomy)
		}
		before, _ := json.Marshal(old.FilmState())
		after, _ := json.Marshal(next.FilmState())
		if bytes.Contains(before, []byte("Avoidance")) || !bytes.Contains(after, []byte("Avoidance")) {
			t.Fatal("historical/new snapshot field ownership")
		}
	}
}
