package combat

import (
	"encoding/binary"
	"strings"
	"testing"

	"gunship2000-assets/game/flight"
	"gunship2000-assets/game/world"
)

func firstRoute(t *testing.T) (*State, Input, *flight.Systems) {
	t.Helper()
	s, in := routeFixture(t)
	scratchRegions(s)
	s.SetDispatcherScratchEnabled(true)
	for actor := 2; actor < len(s.init.Entities); actor++ {
		s.init.Entities[actor].Kind = 0x41 // Inert collected rows retain required full source carriers.
	}
	e := &s.init.Entities[1]
	e.Participation = 1
	e.Pose.Height = 2984 // EC0C: signed height+16 is3000.
	e.Pose.Angles[1] = 0xc000
	binary.LittleEndian.PutUint32(e.ControlRaw[40:], e.Pose.H0+10000)
	binary.LittleEndian.PutUint32(e.ControlRaw[44:], e.Pose.H1)
	in.Phase = 0x200
	return s, in, &flight.Systems{}
}

func TestDispatcherFirstRouteConsumesKnownPredecessor(t *testing.T) {
	for _, tc := range []struct {
		word int16
		yaw  uint16
	}{{99, 0xb800}, {100, 0xc000}, {32767, 0xc000}, {-32768, 0xb800}} {
		s, in, sys := firstRoute(t)
		in.PlayerDesiredVerticalRate = &tc.word
		if _, err := s.AdvanceFollowers(in, sys); err != nil {
			t.Fatal(err)
		}
		if sys.UnsupportedFollowerEmptyBurn != 0 || s.init.Entities[1].Pose.Angles[1] != tc.yaw {
			t.Fatal(tc, sys.UnsupportedFollowerEmptyBurn, s.init.FollowerLimits, s.init.Entities[1].Pose)
		}
	}
	// A second new invocation without a reached writer remains genuinely
	// unknown. In particular ED3B from the preceding invocation is not reused.
	s, in, sys := firstRoute(t)
	word := int16(100)
	in.PlayerDesiredVerticalRate = &word
	s.AdvanceFollowers(in, sys)
	in.PlayerDesiredVerticalRate = nil
	s.AdvanceFollowers(in, sys)
	if sys.UnsupportedFollowerEmptyBurn&2 == 0 || !strings.Contains(s.init.FollowerLimits[1], "BP-30") {
		t.Fatal("next invocation guessed its predecessor", sys.UnsupportedFollowerEmptyBurn, s.init.FollowerLimits)
	}
}

func TestDispatcherLaterFullPoseWriterHasPrecedence(t *testing.T) {
	for _, tc := range []struct {
		donor       uint16
		predecessor int16
		yaw         uint16
	}{{99, 100, 0xb800}, {362, 99, 0xc000}} {
		s, in, sys := firstRoute(t)
		// Reuse actor2's real row in the full prefix, not a compacted world.
		// The362 control reuses the accepted carrier trace's known donor value;
		// this synthetic split-height terrain is not that private carrier film.
		s.init.Entities[2] = s.init.Entities[1]
		s.init.Entities[2].RuntimeRow, s.init.Entities[2].SourceRow, s.init.Entities[2].FlightSlot = 2, 2, 2
		e := &s.init.Entities[1]
		e.Kind, e.Participation, e.Pose.Height = 0x20, 0, tc.donor
		in.PlayerDesiredVerticalRate = &tc.predecessor
		if _, err := s.AdvanceFollowers(in, sys); err != nil {
			t.Fatal(err)
		}
		if sys.UnsupportedFollowerEmptyBurn != 0 || s.init.Entities[2].Pose.Angles[1] != tc.yaw || e.Pose.Height == tc.donor {
			t.Fatal("later full pose did not replace predecessor before altitude", tc, sys.UnsupportedFollowerEmptyBurn, s.init.FollowerLimits, s.init.Entities[2].Pose, e.Pose.Height)
		}
	}
}

func TestDispatcherUntracedControllerInvalidatesPredecessor(t *testing.T) {
	s, in, sys := firstRoute(t)
	s.init.Entities[0].Kind = 2 // Supplied untraced-controller predecessor control.
	word := int16(100)
	in.PlayerDesiredVerticalRate = &word
	s.AdvanceFollowers(in, sys)
	if sys.UnsupportedFollowerEmptyBurn&2 == 0 || !strings.Contains(s.init.FollowerLimits[1], "BP-30") {
		t.Fatal("untraced earlier controller preserved an unsupported writer", sys.UnsupportedFollowerEmptyBurn, s.init.FollowerLimits)
	}
}

func TestDispatcherFormationUsesSameReachedWord(t *testing.T) {
	for _, tc := range []struct {
		word   int16
		d0, d1 uint32
	}{{99, 0, 0x40000 - 182}, {100, 100, 0x40000 - 50}} {
		s, in, sys := formationFixture(t)
		s.SetLogisticsEnabled(true)
		s.SetDispatcherScratchEnabled(true)
		for actor := 2; actor < len(s.init.Entities); actor++ {
			s.init.Entities[actor].Kind = 0x41
		}
		s.init.Entities[0].Pose.Height = 1484 // formation limit1500.
		scratchRegions(s)
		in.Random = func() uint16 { return 8 }
		in.PlayerDesiredVerticalRate = &tc.word
		if _, err := s.AdvanceFollowers(in, sys); err != nil {
			t.Fatal(err)
		}
		if sys.UnsupportedFollowerEmptyBurn != 0 || s.init.Entities[1].Pose.H0 != 0x40000+tc.d0 || s.init.Entities[1].Pose.H1 != tc.d1 {
			t.Fatal(tc, sys.UnsupportedFollowerEmptyBurn, s.init.FollowerLimits, s.init.Entities[1].Pose)
		}
	}
}

func TestPredecessorKeepsWrappedCombinedAndHistoricalDecisions(t *testing.T) {
	s, in, sys := firstRoute(t)
	g := s.assets.Geometries["flat"]
	region := g.Heights[0]
	region.Threshold = 0
	region.Plane = world.Plane{0, 1, 0, -20000}
	g.Heights = []world.HeightRegion{region}
	word := int16(0)
	in.PlayerDesiredVerticalRate = &word
	s.init.Entities = nil // Observe entry operand without a later full-pose write.
	s.AdvanceFollowers(in, sys)
	p := s.init.Player
	got, err := s.followerRouteBlocked(p, p, 0)
	if err != nil || got {
		t.Fatal("20000+20000 must wrap negative before strict compare", got, err)
	}
	// Interpretation off must not silently admit the new predecessor even if
	// a caller supplies it: historical R1 all-word guards remain unchanged.
	s, in, sys = firstRoute(t)
	s.SetDispatcherScratchEnabled(false)
	in.PlayerDesiredVerticalRate = &word
	s.AdvanceFollowers(in, sys)
	if sys.UnsupportedFollowerEmptyBurn&2 == 0 {
		t.Fatal("historical interpretation consumed new predecessor")
	}
}
