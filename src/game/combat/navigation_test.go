package combat

import (
	"encoding/binary"
	"encoding/json"
	"gunship2000-assets/game/avionics"
	"gunship2000-assets/game/world"
	"reflect"
	"strings"
	"testing"
)

func navigationFixture(t *testing.T) (*State, Input) {
	s, in := routeFixture(t)
	s.assets.Geometries["flat"].Vertices = []world.Vertex{{-16, 0, -16}, {16, 32, 16}}
	s.SetNavigationEnabled(true)
	return s, in
}

func TestNavigationSegmentPassageAndBehindRecall(t *testing.T) {
	o := NavigationPoint{0x40000, 0x40000}
	east := NavigationPoint{o.H0 + 4096, o.H1}
	west := NavigationPoint{o.H0 - 4096, o.H1}
	for _, p := range []NavigationPoint{east, west} {
		if passed, off := navigationPassed(o, p, o); passed || off {
			t.Fatal("fresh behind/ahead point skipped", p, passed, off)
		}
	}
	if passed, off := navigationPassed(o, east, NavigationPoint{east.H0 + 64, east.H1 + 256}); !passed || off {
		t.Fatal("reached plane", passed, off)
	}
	if passed, off := navigationPassed(o, east, NavigationPoint{east.H0 + 64, east.H1 + 1024}); passed || !off {
		t.Fatal("remote corner skipped instead of replan", passed, off)
	}
	for _, target := range []NavigationPoint{east, west} {
		s, in := navigationFixture(t)
		e := &s.init.Entities[1]
		e.Participation = 1
		e.Pose.H0, e.Pose.H1 = o.H0, o.H1
		before := e.Pose
		heading := s.angle(-int16((int32(target.H0)-int32(o.H0))>>4), 0)
		got, active := s.navigationHeading(1, e, heading, target.H0, target.H1, false, in)
		if !active || got != heading || e.Pose != before {
			t.Fatal("direct heading/behind recall changed physics", active, got, heading)
		}
		if !s.navigation[1].Direct {
			t.Fatal("clear route not direct")
		}
	}
}

func TestNavigationKeysSnapshotsAndLegacyOmission(t *testing.T) {
	s, in := navigationFixture(t)
	e := &s.init.Entities[1]
	e.Participation = 1
	goal := NavigationPoint{e.Pose.H0, e.Pose.H1 + 16000}
	s.navigationHeading(1, e, 0, goal.H0, goal.H1, false, in)
	snap := s.FilmState()
	if snap.Navigation == nil || snap.Navigation[1].Count == 0 {
		t.Fatal("active state missing")
	}
	old := *snap.Navigation
	s.ResetNavigation(1)
	if *snap.Navigation != old {
		t.Fatal("snapshot aliases cache")
	}
	for _, change := range []string{"goal", "kind", "index", "altitude", "speed", "low", "support", "model", "formation"} {
		v, vin := navigationFixture(t)
		ve := &v.init.Entities[1]
		vg := NavigationPoint{ve.Pose.H0, ve.Pose.H1 + 16000}
		before, err := v.navigationKey(1, ve, vg, vin)
		if err != nil {
			t.Fatal(err)
		}
		switch change {
		case "goal":
			vg.H0++
		case "kind":
			ve.Kind = 0x22
		case "index":
			ve.ControlRaw[0]++
		case "altitude":
			binary.LittleEndian.PutUint16(ve.ControlRaw[6:], 2)
		case "speed":
			binary.LittleEndian.PutUint16(ve.ControlRaw[8:], 2)
		case "low":
			vin.LowLimit++
		case "support":
			v.init.Switches ^= 0x1000
		case "model":
			ve.ModelID = 1
		case "formation":
			child := &v.init.Entities[2]
			child.Kind = 0x21
			child.ControlRaw[5] = 1
			child.DynamicRaw[2] = 1
		}
		after, err := v.navigationKey(1, ve, vg, vin)
		if err != nil || after == before {
			t.Fatal(change, "actual owner did not invalidate key", err)
		}
	}
	// Use the real GPS finalizer: zero source skill draws nothing and produces
	// identical cache bytes. Native state must survive that unchanged/cancel epoch.
	binary.LittleEndian.PutUint32(e.ControlRaw[10:], goal.H0)
	binary.LittleEndian.PutUint32(e.ControlRaw[14:], goal.H1)
	binary.LittleEndian.PutUint32(e.ControlRaw[40:], goal.H0)
	binary.LittleEndian.PutUint32(e.ControlRaw[44:], goal.H1)
	s.navigationHeading(1, e, 0, goal.H0, goal.H1, false, in)
	keep := s.navigation[1]
	var contacts [80]avionics.Contact
	if err := s.FinalizeFollowerRoutes(func() uint16 { t.Fatal("zero skill unexpectedly drew RNG"); return 0 }, &contacts); err != nil {
		t.Fatal(err)
	}
	if s.navigation[1] != keep {
		t.Fatal("unchanged GPS finalizer cleared native path")
	}
	e.Participation = 0
	if err := s.advanceFollowerRoute(1, in); err != nil {
		t.Fatal(err)
	}
	if s.navigation[1] != (FollowerNavigation{}) {
		t.Fatal("P0 did not reset native path")
	}
	s.SetNavigationEnabled(false)
	raw, _ := json.Marshal(s.FilmState())
	if strings.Contains(string(raw), "Navigation") {
		t.Fatal("old profile snapshot gained navigation")
	}
}

func TestNavigationReferenceErrorsDoNotProveClearance(t *testing.T) {
	for _, failure := range []string{"cell", "plane", "model"} {
		s, in := navigationFixture(t)
		e := &s.init.Entities[1]
		switch failure {
		case "cell":
			s.assets.Worlds["flat"].Grid = nil
		case "plane":
			s.assets.Geometries["flat"].Heights[0].Plane[1] = 0
		case "model":
			e.ModelID = len(s.assets.Models)
		}
		points, stats := s.planFollowerNavigation(1, e, in, NavigationPoint{e.Pose.H0, e.Pose.H1}, NavigationPoint{e.Pose.H0, e.Pose.H1 + 4096})
		if len(points) != 0 || stats.Error == "" {
			t.Fatal(failure, "unsupported reference accepted", points, stats)
		}
	}
	s, in := navigationFixture(t)
	e := &s.init.Entities[1]
	points, stats := s.planFollowerNavigation(1, e, in, NavigationPoint{e.Pose.H0, e.Pose.H1}, NavigationPoint{0xffffffff, 0xffffffff})
	if len(points) > 8 || stats.Queries > 32768 {
		t.Fatal("unbounded edge search", stats)
	}
}

func TestNavigationOriginalUnknownGuardPrecedesNativePlanner(t *testing.T) {
	s, in := navigationFixture(t)
	e := &s.init.Entities[1]
	e.Participation = 1
	e.Pose.Height = 3000
	binary.LittleEndian.PutUint32(e.ControlRaw[40:], e.Pose.H0)
	binary.LittleEndian.PutUint32(e.ControlRaw[44:], e.Pose.H1+10000)
	g := s.assets.Geometries["flat"]
	lo, hi := g.Heights[0], g.Heights[0]
	lo.Threshold = 100
	lo.Plane = world.Plane{0, 1, 0, -1000}
	hi.Plane = world.Plane{0, 1, 0, -2000}
	g.Heights = []world.HeightRegion{lo, hi}
	err := s.advanceFollowerRoute(1, in)
	if err == nil || !strings.Contains(err.Error(), "shared unwritten BP-30") || s.navigation != (NavigationState{}) {
		t.Fatal("native planner bypassed original mixed guard", err, s.navigation)
	}
}

func TestNavigationUnobstructedPreservesSourceAndBothAIStates(t *testing.T) {
	for _, autonomy := range []bool{true, false} {
		old, in := routeFixture(t)
		next, _ := navigationFixture(t)
		next.SetRouteAvoidanceEnabled(true)
		next.SetWaypointReleaseEnabled(true)
		for _, s := range []*State{old, next} {
			s.SetWingmenAutonomyEnabled(autonomy)
			e := &s.init.Entities[1]
			binary.LittleEndian.PutUint32(e.ControlRaw[40:], e.Pose.H0)
			binary.LittleEndian.PutUint32(e.ControlRaw[44:], e.Pose.H1+6000)
		}
		for tick := 0; tick < 500; tick++ {
			for _, s := range []*State{old, next} {
				if err := s.advanceFollowerRoute(1, in); err != nil {
					t.Fatal(err)
				}
			}
			if !reflect.DeepEqual(old.init, next.init) {
				t.Fatal("clear route changed source actor/RNG state", autonomy, tick)
			}
			if next.init.Entities[1].Kind != 0x23 {
				break
			}
		}
		if next.init.Entities[1].Kind != 0x20 || next.navigation != (NavigationState{}) {
			t.Fatal("source arrival/reset", autonomy)
		}
	}
}

func TestNavigationFailedHeightChangeRetriesWithoutPerVisitSearch(t *testing.T) {
	s, in := navigationFixture(t)
	e := &s.init.Entities[1]
	e.Participation, e.Pose.Height = 1, 50
	s.assets.Geometries["flat"].Heights[0].Plane = world.Plane{0, 1, 0, -100}
	x, z := e.Pose.H0, e.Pose.H1+4096
	if _, active := s.navigationHeading(1, e, 0, x, z, true, in); active || !s.navigation[1].Failed {
		t.Fatal("low supplied pose unexpectedly proves a path")
	}
	grid := s.assets.Worlds["flat"].Grid
	s.assets.Worlds["flat"].Grid = nil
	for visit := 0; visit < 32; visit++ {
		s.navigationHeading(1, e, 0, x, z, true, in)
		if s.navigation[1].Failure != 1 {
			t.Fatal("unchanged failed pose repeated a query", visit)
		}
	}
	s.assets.Worlds["flat"].Grid = grid
	e.Pose.Height = 300 // Supplied reached-height boundary, never written by the planner.
	for visit := 1; visit <= 16; visit++ {
		_, active := s.navigationHeading(1, e, 0, x, z, true, in)
		if active != (visit == 16) {
			t.Fatal("height retry was immediate or permanently cached", visit, active)
		}
	}
	if e.Pose.Height != 300 || s.navigation[1].Failed {
		t.Fatal("retry changed height or retained stale failure")
	}
}
