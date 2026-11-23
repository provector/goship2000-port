package combat

import (
	"encoding/binary"
	"encoding/json"
	"gunship2000-assets/game/world"
	"reflect"
	"strings"
	"testing"
)

func TestWaypointReleaseEntryTailAndPhase(t *testing.T) {
	s, _ := routeFixture(t)
	s.SetRouteAvoidanceEnabled(true)
	s.SetWaypointReleaseEnabled(true)
	e := &s.init.Entities[1]
	e.Participation = 1
	e.Pose.Angles[1] = 0
	active, offset := s.waypointRelease(1, e, 0, 100, 200, true, 0x200)
	if !active || offset != 8192 {
		t.Fatal(active, offset)
	}
	for clear := 1; clear <= 16; clear++ {
		active, offset = s.waypointRelease(1, e, 0, 100, 200, false, 0)
		want := int16(8192 - clear*512)
		if offset != want || active != (clear < 16) {
			t.Fatal(clear, active, offset, want)
		}
	}
	if s.routeAvoidance[1] != (RouteAvoidanceLatch{}) {
		t.Fatal("completed tail retained state")
	}
	s.waypointRelease(1, e, 0, 100, 200, true, 0)
	_, offset = s.waypointRelease(1, e, 0, 100, 200, false, 0x200)
	if offset != -7680 {
		t.Fatal("clear changed prior side", offset)
	}
	_, offset = s.waypointRelease(1, e, 0, 100, 200, true, 0x200)
	if offset != 8192 {
		t.Fatal("reblocked entry not immediate", offset)
	}
	snap := s.FilmState()
	s.waypointRelease(1, e, 0, 100, 200, false, 0x200)
	if snap.Avoidance == nil || snap.Avoidance[1].Offset != 8192 {
		t.Fatal("snapshot offset missing/aliased")
	}
}

func TestWaypointReleaseRouteAndControllerReset(t *testing.T) {
	s, _ := routeFixture(t)
	s.SetRouteAvoidanceEnabled(true)
	s.SetWaypointReleaseEnabled(true)
	e := &s.init.Entities[1]
	e.Participation = 1
	e.Pose.Angles[1] = 0
	for _, change := range []string{"index", "x", "z", "phase", "return", "otherkind", "angle"} {
		e.Kind, e.Participation, e.ControlRaw[0], e.Pose.Angles[1] = 0x23, 1, 1, 0
		s.waypointRelease(1, e, 0, 100, 200, true, 0x200)
		x, z := uint32(100), uint32(200)
		switch change {
		case "index":
			e.ControlRaw[0]++
		case "x":
			x++
		case "z":
			z++
		case "phase":
			e.Participation = 0
		case "return":
			e.Kind = 0x22
		case "otherkind":
			e.Kind = 0x24
		case "angle":
			e.Pose.Angles[1] = 0x4000
		}
		active, _ := s.waypointRelease(1, e, 0, x, z, false, 0x200)
		if active || s.routeAvoidance[1] != (RouteAvoidanceLatch{}) {
			t.Fatal(change, "stale offset")
		}
	}
	// Preserve the V3 serialized field order/omission, not a rewritten checkpoint.
	raw, _ := json.Marshal(RouteAvoidanceLatch{Stage: 1, Kind: 35, Index: 1, H0: 100, H1: 200})
	if string(raw) != `{"Stage":1,"Kind":35,"Index":1,"H0":100,"H1":200}` {
		t.Fatal("historical V3 snapshot bytes", string(raw))
	}
}

func TestWaypointReleaseOriginalSpeedArm(t *testing.T) {
	for _, tc := range []struct {
		name             string
		gradual, blocked bool
		want             int16
	}{{"v4 clear heading tail", true, false, 112}, {"v4 original blocked", true, true, 88}, {"v3 held clear historical", false, false, 88}} {
		t.Run(tc.name, func(t *testing.T) {
			s, in := routeFixture(t)
			s.SetRouteAvoidanceEnabled(true)
			s.SetWaypointReleaseEnabled(tc.gradual)
			e := &s.init.Entities[1]
			e.Participation = 1
			e.Pose.Height = 154
			e.Pose.Angles[1] = 0
			e.RawMotion = 160
			in.Phase = 0x200
			x, z := e.Pose.H0, e.Pose.H1+10000
			binary.LittleEndian.PutUint32(e.ControlRaw[40:], x)
			binary.LittleEndian.PutUint32(e.ControlRaw[44:], z)
			s.routeAvoidance[1] = RouteAvoidanceLatch{Stage: 1, Kind: 0x23, Index: 1, H0: x, H1: z}
			if tc.gradual {
				s.routeAvoidance[1].Stage = 0
				s.routeAvoidance[1].Offset = 8192
			}
			g := s.assets.Geometries["flat"]
			r := g.Heights[0]
			r.Plane = world.Plane{0, 1, 0, 0}
			if tc.blocked {
				r.Plane[3] = -100
			}
			g.Heights = []world.HeightRegion{r}
			if err := s.advanceFollowerRoute(1, in); err != nil {
				t.Fatal(err)
			}
			if e.RawMotion != tc.want {
				t.Fatal("EC41 first arm/common EC75 arm ownership", e.RawMotion, tc.want)
			}
		})
	}
}

func TestWaypointReleaseKeepsOriginalReturnAcrossBoundaries(t *testing.T) {
	for _, phase := range []uint16{0, 0x1ff, 0x200, 0x3ff, 0xffff} {
		for _, yaw := range []uint16{0, 0x3fff, 0x4000, 0x7fff, 0x8000, 0xc000, 0xffff} {
			for _, blocked := range []bool{false, true} {
				old, in := routeFixture(t)
				next, _ := routeFixture(t)
				next.SetRouteAvoidanceEnabled(true)
				next.SetWaypointReleaseEnabled(true)
				next.routeAvoidance[1] = RouteAvoidanceLatch{Kind: 0x23, Index: 1, Offset: 4096}
				in.Phase = phase
				for _, s := range []*State{old, next} {
					e := &s.init.Entities[1]
					e.Kind, e.Participation, e.ControlRaw[1] = 0x22, 1, 8
					e.Pose.Height = 154
					e.Pose.Angles[1] = yaw
					e.RawMotion = 160
					binary.LittleEndian.PutUint32(e.ControlRaw[40:], e.Pose.H0)
					binary.LittleEndian.PutUint32(e.ControlRaw[44:], e.Pose.H1+10000)
					g := s.assets.Geometries["flat"]
					r := g.Heights[0]
					r.Plane = world.Plane{0, 1, 0, 0}
					if blocked {
						r.Plane[3] = -100
					}
					g.Heights = []world.HeightRegion{r}
					if err := s.advanceFollowerRoute(1, in); err != nil {
						t.Fatal(err)
					}
				}
				if !reflect.DeepEqual(old.init, next.init) || next.routeAvoidance[1] != (RouteAvoidanceLatch{}) {
					t.Fatal("return changed", phase, yaw, blocked)
				}
			}
		}
	}
}

func TestWaypointReleaseUnknownDecisionStillRejectsFirst(t *testing.T) {
	s, in := routeFixture(t)
	s.SetRouteAvoidanceEnabled(true)
	s.SetWaypointReleaseEnabled(true)
	e := &s.init.Entities[1]
	e.Participation = 1
	e.Pose.Height = 3000
	binary.LittleEndian.PutUint32(e.ControlRaw[40:], e.Pose.H0)
	binary.LittleEndian.PutUint32(e.ControlRaw[44:], e.Pose.H1+10000)
	s.routeAvoidance[1] = RouteAvoidanceLatch{Offset: 8192}
	g := s.assets.Geometries["flat"]
	lo, hi := g.Heights[0], g.Heights[0]
	lo.Threshold = 100
	lo.Plane = world.Plane{0, 1, 0, -1000}
	hi.Plane = world.Plane{0, 1, 0, -2000}
	g.Heights = []world.HeightRegion{lo, hi}
	err := s.advanceFollowerRoute(1, in)
	if err == nil || !strings.Contains(err.Error(), "shared unwritten BP-30") || s.routeAvoidance[1].Offset != 8192 {
		t.Fatal("policy bypassed original guard", err, s.routeAvoidance[1])
	}
}

func TestWaypointReleaseUnobstructedAndBothAIStates(t *testing.T) {
	for _, ai := range []bool{false, true} {
		old, in := routeFixture(t)
		next, _ := routeFixture(t)
		next.SetRouteAvoidanceEnabled(true)
		next.SetWaypointReleaseEnabled(true)
		for _, s := range []*State{old, next} {
			s.SetWingmenAutonomyEnabled(ai)
			e := &s.init.Entities[1]
			binary.LittleEndian.PutUint32(e.ControlRaw[40:], e.Pose.H0)
			binary.LittleEndian.PutUint32(e.ControlRaw[44:], e.Pose.H1+6000)
		}
		for i := 0; i < 500; i++ {
			for _, s := range []*State{old, next} {
				if err := s.advanceFollowerRoute(1, in); err != nil {
					t.Fatal(err)
				}
			}
			if !reflect.DeepEqual(old.init, next.init) {
				t.Fatal("unobstructed state/RNG changed", ai, i)
			}
			if next.init.Entities[1].Kind != 0x23 {
				break
			}
		}
		if next.init.Entities[1].Kind != 0x20 || next.routeAvoidance[1] != (RouteAvoidanceLatch{}) {
			t.Fatal("arrival/reset", ai)
		}
	}
}
