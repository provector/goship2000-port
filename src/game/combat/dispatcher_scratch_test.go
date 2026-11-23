package combat

import (
	"encoding/binary"
	"gunship2000-assets/game/flight"
	"gunship2000-assets/game/world"
	"testing"
)

func scratchRegions(s *State) {
	g := s.assets.Geometries["flat"]
	low, high := g.Heights[0], g.Heights[0]
	low.Threshold = 100
	low.Plane = world.Plane{0, 1, 0, -1000}
	high.Threshold = 0
	high.Plane = world.Plane{0, 1, 0, -2000}
	g.Heights = []world.HeightRegion{low, high}
}

func TestDispatcherScratchUsesReachedWordAndKeepsUnknownGuard(t *testing.T) {
	s, _ := routeFixture(t)
	scratchRegions(s)
	a := s.init.Entities[1].Pose
	b := a
	b.H0 += 256
	s.SetDispatcherScratchEnabled(true)
	if _, err := s.followerRouteBlocked(a, b, 3000); err == nil {
		t.Fatal("unknown incoming word was invented")
	}
	for _, tc := range []struct {
		height uint16
		want   bool
	}{{99, true}, {100, false}, {32767, false}, {0x8000, true}} {
		s.retainDispatcherHeight(tc.height)
		got, err := s.followerRouteBlocked(a, b, 3000)
		if err != nil || got != tc.want {
			t.Fatal(tc, got, err)
		}
	}
	// Turning the interpretation off cannot reuse a proven R1.1 value in R1.
	s.SetDispatcherScratchEnabled(false)
	if _, err := s.followerRouteBlocked(a, b, 3000); err == nil {
		t.Fatal("historical all-word guard changed")
	}
}

func TestDispatcherScratchIsPreAltitudeAndInvocationLocal(t *testing.T) {
	s, in := routeFixture(t)
	scratchRegions(s)
	s.SetDispatcherScratchEnabled(true)
	e := &s.init.Entities[1]
	e.Kind = 0x20
	e.Participation = 0
	e.Pose.Height = 99
	if err := s.advanceIdleFollower(1, in); err != nil {
		t.Fatal(err)
	}
	if !s.dispatcherScratchKnown || s.dispatcherScratchHeight != 99 || e.Pose.Height == 99 {
		t.Fatal("DE45 donated final height", s.dispatcherScratchHeight, e.Pose.Height)
	}
	// P0 and route arrival return before ED3B, leaving the prior writer intact.
	e.Kind = 0x23
	e.Participation = 0
	e.ControlRaw[0] = 1
	e.Pose.Height = 3000
	binary.LittleEndian.PutUint32(e.ControlRaw[40:], e.Pose.H0+10000)
	binary.LittleEndian.PutUint32(e.ControlRaw[44:], e.Pose.H1)
	if err := s.advanceFollowerRoute(1, in); err != nil {
		t.Fatal(err)
	}
	if s.dispatcherScratchHeight != 99 {
		t.Fatal("P0 wrote nonexistent full pose")
	}
	if err := s.advanceFollowerRoute(1, in); err != nil {
		t.Fatal(err)
	}
	if s.dispatcherScratchHeight != 3000 {
		t.Fatal("ED3B height is not pre-altitude", s.dispatcherScratchHeight)
	}
	s.init.Entities = nil
	if _, err := s.AdvanceFollowers(in, &flight.Systems{}); err != nil {
		t.Fatal(err)
	}
	if s.dispatcherScratchKnown {
		t.Fatal("new B01C frame inherited old native invocation")
	}
}
